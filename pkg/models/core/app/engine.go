package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/santhosh-tekuri/jsonschema/v6"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

const (
	AnnotationAppConfig  = corev1.AnnotationInstanceAppConfig
	AnnotationAppVersion = corev1.AnnotationInstanceAppVersion
	LabelAppPreset       = corev1.LabelInstancePreset
)

var (
	// Matches shorthand {{identifier}} or {{scope.identifier}} (without leading dot) so both
	// Helm-style {{ .Values.maxPlayers | default "20" }} and shorthand {{maxPlayers}} / {{identity.serverName}} work.
	shorthandVarRe      = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_][a-zA-Z0-9_.]*)\s*\}\}`)
	numericUnitSuffixRe = regexp.MustCompile(`^([+-]?\d+(?:\.\d+)?)\s*([a-zA-Z]+)$`)
)

// parseConfigNumber parses a numeric configuration value, transparently converting
// memory/size unit suffixes such as "4G" / "4Gi" / "4GB" (to 4096 when the field is in MB)
// or "4096M" / "4096Mi" / "4096MB".
func parseConfigNumber(raw string, f corev1.AppConfigField) (float64, string, error) {
	trimmed := strings.TrimSpace(raw)
	if num, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return num, trimmed, nil
	}
	if m := numericUnitSuffixRe.FindStringSubmatch(trimmed); len(m) == 3 {
		base, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return 0, "", err
		}
		suffix := strings.ToUpper(m[2])
		expectsMB := strings.HasSuffix(strings.ToLower(f.Name), "mb") ||
			strings.Contains(strings.ToUpper(f.Label), "MB") ||
			strings.EqualFold(f.EnvKey, "MEMORY")
		var num float64
		switch suffix {
		case "G", "GI", "GB", "GIB":
			if expectsMB {
				num = base * 1024
			} else {
				num = base * 1024 * 1024 * 1024
			}
		case "M", "MI", "MB", "MIB":
			if expectsMB {
				num = base
			} else {
				num = base * 1024 * 1024
			}
		case "K", "KI", "KB", "KIB":
			if expectsMB {
				num = base / 1024
			} else {
				num = base * 1024
			}
		default:
			return 0, "", fmt.Errorf("strconv.ParseFloat: parsing %q: invalid syntax", trimmed)
		}
		formatted := strconv.FormatFloat(num, 'f', -1, 64)
		return num, formatted, nil
	}
	num, err := strconv.ParseFloat(trimmed, 64)
	return num, trimmed, err
}

// BuildJSONSchema compiles an App's ConfigSchema into a standard IETF JSON Schema (Draft 2020-12).
func BuildJSONSchema(app *corev1.App) map[string]any {
	var fields []corev1.AppConfigField
	if app != nil {
		fields = app.ConfigSchema
	}
	properties := make(map[string]any, len(fields))
	var required []any

	for _, f := range fields {
		if f.Name == "" {
			continue
		}
		prop := map[string]any{
			"title": f.Label,
		}
		if f.Help != "" {
			prop["description"] = f.Help
		}
		switch f.Type {
		case "number":
			prop["type"] = "number"
			if f.Min != nil {
				prop["minimum"] = *f.Min
			}
			if f.Max != nil {
				prop["maximum"] = *f.Max
			}
		case "boolean":
			prop["type"] = "boolean"
		case "select":
			prop["type"] = "string"
			if len(f.Options) > 0 && strings.TrimSpace(f.OptionsSource) == "" {
				enums := make([]any, 0, len(f.Options))
				for _, opt := range f.Options {
					enums = append(enums, opt.Value)
				}
				prop["enum"] = enums
			}
		default:
			prop["type"] = "string"
		}
		if f.Required {
			required = append(required, f.Name)
		}
		properties[f.Name] = prop
	}

	schema := map[string]any{
		"$schema":    "https://json-schema.org/draft/2020-12/schema",
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// ValidateConfigValues validates user-supplied configuration values against the App's compiled
// JSON Schema (Draft 2020-12) using github.com/santhosh-tekuri/jsonschema/v6.
func ValidateConfigValues(app *corev1.App, values map[string]string) error {
	if app == nil || len(app.ConfigSchema) == 0 {
		return nil
	}

	schemaDoc := BuildJSONSchema(app)
	compiler := jsonschema.NewCompiler()
	const schemaURL = "urn:gamepanel:app:config.schema.json"
	if err := compiler.AddResource(schemaURL, schemaDoc); err != nil {
		return fmt.Errorf("compile app jsonschema resource: %w", err)
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		return fmt.Errorf("compile app jsonschema: %w", err)
	}

	// Coerce string representations into JSON Schema native primitives (number / boolean / string).
	typedDoc := make(map[string]any, len(app.ConfigSchema))
	for _, f := range app.ConfigSchema {
		raw, ok := values[f.Name]
		if !ok || raw == "" {
			raw = f.Default
		}
		if raw == "" {
			if f.Required {
				return fmt.Errorf("config field %q (%s) is required", f.Name, f.Label)
			}
			continue
		}
		switch f.Type {
		case "number":
			num, normalized, err := parseConfigNumber(raw, f)
			if err != nil {
				return fmt.Errorf("config field %q (%s) must be a valid number: %w", f.Name, f.Label, err)
			}
			if values != nil {
				values[f.Name] = normalized
			}
			typedDoc[f.Name] = num
		case "boolean":
			lower := strings.ToLower(strings.TrimSpace(raw))
			typedDoc[f.Name] = lower == "true" || lower == "1" || lower == "yes"
		default:
			typedDoc[f.Name] = raw
		}
	}

	if err := compiled.Validate(typedDoc); err != nil {
		return fmt.Errorf("config schema validation failed: %w", err)
	}
	return nil
}

// RenderTemplate renders a configuration file or image template using Go text/template
// and Helm's sprig/v3 function library (github.com/Masterminds/sprig/v3).
func RenderTemplate(tmplText string, values map[string]string, version string) (string, error) {
	normalized := strings.ReplaceAll(tmplText, "{version}", "{{ .Version }}")
	normalized = strings.ReplaceAll(normalized, "{{VERSION}}", "{{ .Version }}")
	normalized = shorthandVarRe.ReplaceAllStringFunc(normalized, func(match string) string {
		sub := shorthandVarRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		key := sub[1]
		switch key {
		case "else", "end", "nil", "true", "false", "break", "continue":
			return match
		case "Version":
			return "{{ .Version }}"
		}
		return fmt.Sprintf(`{{ index .Values %q }}`, key)
	})

	tpl, err := template.New("app").
		Funcs(sprig.TxtFuncMap()).
		Option("missingkey=zero").
		Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	ctxData := map[string]any{
		"Values":  values,
		"Version": version,
	}
	// Also expose top-level keys for direct template access.
	for k, v := range values {
		ctxData[k] = v
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ctxData); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}

// RenderInstanceSpec merges default App config values with instance annotations/env,
// validates them with jsonschema/v6, and renders Image, Env, and Files via text/template + sprig/v3.
func RenderInstanceSpec(appObj *corev1.App, inst *corev1.Instance) error {
	if appObj == nil || inst == nil {
		return nil
	}
	values, hasSavedAnnotation, err := extractAppConfigValues(appObj, inst)
	if err != nil {
		return err
	}
	if err := ValidateConfigValues(appObj, values); err != nil {
		return err
	}
	version := resolveAppTargetVersion(appObj, inst)
	renderInstanceImageAndEnv(appObj, inst, values, version, hasSavedAnnotation)
	ensureInstanceAppMetadata(appObj, inst, version)
	if err := renderInstanceArtifactBindings(appObj, inst, values, version); err != nil {
		return err
	}
	if (hasSavedAnnotation || len(inst.Spec.Files) == 0) && len(appObj.FileTemplates) > 0 {
		if err := renderInstanceFileTemplates(appObj, inst, values, version); err != nil {
			return err
		}
	}
	renderInstanceMetricsAndStoragePolicy(appObj, inst, values)
	return nil
}

func extractAppConfigValues(appObj *corev1.App, inst *corev1.Instance) (map[string]string, bool, error) {
	values := make(map[string]string, len(appObj.ConfigSchema))
	for _, f := range appObj.ConfigSchema {
		if f.Default != "" {
			values[f.Name] = f.Default
		}
	}
	if inst.Spec.Env != nil {
		for _, f := range appObj.ConfigSchema {
			if f.EnvKey != "" {
				applyEnvFieldOverride(values, f, inst.Spec.Env[f.EnvKey])
			}
		}
	}
	hasSavedAnnotation := false
	if rawJSON := inst.Metadata.Annotations[AnnotationAppConfig]; strings.TrimSpace(rawJSON) != "" {
		var saved map[string]string
		if err := json.Unmarshal([]byte(rawJSON), &saved); err != nil {
			return nil, false, fmt.Errorf("invalid %s annotation JSON: %w", AnnotationAppConfig, err)
		}
		hasSavedAnnotation = true
		for k, v := range saved {
			values[k] = v
		}
	}
	applySchemaFallbacksAndAliases(appObj.ConfigSchema, values)
	return values, hasSavedAnnotation, nil
}

func applySchemaFallbacksAndAliases(schema []corev1.AppConfigField, values map[string]string) {
	leafCounts := make(map[string]int, len(schema))
	for _, f := range schema {
		if f.Default != "" && strings.TrimSpace(values[f.Name]) == "" {
			if f.Type == "select" || f.Type == "boolean" || f.Type == "number" || f.Required {
				values[f.Name] = f.Default
			}
		}
		if idx := strings.LastIndexByte(f.Name, '.'); idx > 0 && idx < len(f.Name)-1 {
			leafCounts[f.Name[idx+1:]]++
		}
	}
	for _, f := range schema {
		idx := strings.LastIndexByte(f.Name, '.')
		if idx <= 0 || idx >= len(f.Name)-1 {
			continue
		}
		leaf := f.Name[idx+1:]
		if leafCounts[leaf] == 1 {
			if _, exists := values[leaf]; !exists {
				values[leaf] = values[f.Name]
			}
		}
	}
}

func applyEnvFieldOverride(values map[string]string, f corev1.AppConfigField, v string) {
	if v == "" {
		return
	}
	switch f.Type {
	case "number":
		if num, normalized, err := parseConfigNumber(v, f); err == nil {
			if (f.Min == nil || num >= *f.Min) && (f.Max == nil || num <= *f.Max) {
				values[f.Name] = normalized
			}
		}
	case "select":
		if len(f.Options) > 0 {
			matched := false
			for _, opt := range f.Options {
				if strings.EqualFold(opt.Value, strings.TrimSpace(v)) {
					values[f.Name] = opt.Value
					matched = true
					break
				}
			}
			if !matched && strings.TrimSpace(f.OptionsSource) != "" {
				values[f.Name] = strings.TrimSpace(v)
			}
		} else {
			values[f.Name] = v
		}
	default:
		values[f.Name] = v
	}
}

func resolveAppTargetVersion(appObj *corev1.App, inst *corev1.Instance) string {
	version := strings.TrimSpace(inst.Metadata.Annotations[AnnotationAppVersion])
	if version == "" && inst.Spec.ServerArtifact != nil {
		version = strings.TrimSpace(inst.Spec.ServerArtifact.Version)
	}
	if version == "" {
		version = appObj.RecommendedVersion
	}
	if version == "" && len(appObj.Versions) > 0 {
		version = appObj.Versions[0]
	}
	return version
}

func ensureInstanceAppMetadata(appObj *corev1.App, inst *corev1.Instance, version string) {
	if appName := strings.TrimSpace(appObj.Metadata.Name); appName != "" {
		if inst.Metadata.Labels == nil {
			inst.Metadata.Labels = make(map[string]string)
		}
		if strings.TrimSpace(inst.Metadata.Labels[corev1.LabelInstanceApp]) == "" {
			inst.Metadata.Labels[corev1.LabelInstanceApp] = appName
		}
	}
	if strings.TrimSpace(version) != "" {
		if inst.Metadata.Annotations == nil {
			inst.Metadata.Annotations = make(map[string]string)
		}
		inst.Metadata.Annotations[AnnotationAppVersion] = strings.TrimSpace(version)
	}
}

func renderInstanceArtifactBindings(appObj *corev1.App, inst *corev1.Instance, values map[string]string, version string) error {
	if sb := appObj.ServerBinding; sb != nil && strings.TrimSpace(sb.Repository) != "" && strings.TrimSpace(sb.Package) != "" && strings.TrimSpace(version) != "" {
		targetTpl := strings.TrimSpace(sb.TargetPath)
		if targetTpl == "" {
			targetTpl = "."
		}
		targetPath, err := RenderTemplate(targetTpl, values, version)
		if err != nil {
			return fmt.Errorf("render serverBinding.targetPath: %w", err)
		}
		targetPath = strings.TrimSpace(targetPath)
		if targetPath == "" {
			targetPath = "."
		}
		inst.Spec.ServerArtifact = &corev1.ArtifactRef{
			Repository: strings.TrimSpace(sb.Repository),
			Package:    strings.TrimSpace(sb.Package),
			Version:    strings.TrimSpace(version),
			TargetPath: targetPath,
		}
	}
	if ab := appObj.AddonBinding; ab != nil {
		for i := range inst.Spec.Addons {
			matched := ab.ForRepository(inst.Spec.Addons[i].Repository)
			if strings.TrimSpace(inst.Spec.Addons[i].Repository) == "" {
				inst.Spec.Addons[i].Repository = strings.TrimSpace(matched.Repository)
			}
			if strings.TrimSpace(inst.Spec.Addons[i].InstallMode) == "" {
				inst.Spec.Addons[i].InstallMode = strings.TrimSpace(matched.InstallMode)
			}
			if strings.TrimSpace(inst.Spec.Addons[i].TargetDir) == "" {
				inst.Spec.Addons[i].TargetDir = strings.TrimSpace(matched.TargetDir)
			}
		}
	}
	return nil
}

func renderInstanceImageAndEnv(appObj *corev1.App, inst *corev1.Instance, values map[string]string, version string, hasSavedAnnotation bool) {
	explicitVersion := strings.TrimSpace(inst.Metadata.Annotations[AnnotationAppVersion])
	if inst.Spec.Image == "" || explicitVersion != "" {
		if appObj.ImageTemplate != "" && version != "" {
			if renderedImg, err := RenderTemplate(appObj.ImageTemplate, values, version); err == nil && strings.TrimSpace(renderedImg) != "" {
				inst.Spec.Image = strings.TrimSpace(renderedImg)
			}
		}
		if inst.Spec.Image == "" {
			inst.Spec.Image = appObj.Image
		}
	}
	if inst.Spec.Env == nil {
		inst.Spec.Env = make(map[string]string)
	}
	for k, v := range appObj.Env {
		if _, exists := inst.Spec.Env[k]; !exists {
			inst.Spec.Env[k] = v
		}
	}
	if explicitVersion != "" && appObj.Env != nil && appObj.Env["VERSION"] != "" {
		inst.Spec.Env["VERSION"] = explicitVersion
	}
	renderAppHookEnv(appObj, inst, values, version)
	syncInstanceJVMHeapMemory(appObj, inst)
	if !hasSavedAnnotation {
		return
	}
	for _, f := range appObj.ConfigSchema {
		if f.EnvKey != "" {
			if v, ok := values[f.Name]; ok {
				trimmed := strings.TrimSpace(v)
				if strings.EqualFold(f.EnvKey, "MEMORY") && f.Type == "number" && trimmed != "" {
					upper := strings.ToUpper(trimmed)
					if !strings.HasSuffix(upper, "M") && !strings.HasSuffix(upper, "G") {
						trimmed += "M"
					}
				}
				inst.Spec.Env[f.EnvKey] = trimmed
			}
		}
	}
}

func renderAppHookEnv(appObj *corev1.App, inst *corev1.Instance, values map[string]string, version string) {
	hooks := []struct {
		tpl    string
		envKey string
	}{
		{appObj.PreStartCommand, corev1.EnvPreStartHook},
		{appObj.StartupCommand, corev1.EnvStartupCmd},
		{appObj.PreStopCommand, corev1.EnvPreStopHook},
	}
	for _, h := range hooks {
		tpl := strings.TrimSpace(h.tpl)
		if tpl == "" {
			continue
		}
		if rendered, err := RenderTemplate(tpl, values, version); err == nil && strings.TrimSpace(rendered) != "" {
			inst.Spec.Env[h.envKey] = strings.TrimSpace(rendered)
		}
	}
}

func syncInstanceJVMHeapMemory(appObj *corev1.App, inst *corev1.Instance) {
	if inst == nil || inst.Spec.Env == nil {
		return
	}
	if (appObj == nil || appObj.Env["MEMORY"] == "") && inst.Spec.Env["MEMORY"] == "" {
		return
	}
	memBytes := inst.Spec.Resources.Limits.MemoryBytes()
	if memBytes <= 0 {
		memBytes = inst.Spec.Resources.Requests.MemoryBytes()
	}
	if memBytes <= 0 {
		return
	}
	heapMB := (memBytes / (1024 * 1024)) * 3 / 4
	if heapMB < 512 {
		heapMB = 512
	}
	inst.Spec.Env["MEMORY"] = fmt.Sprintf("%dM", heapMB)
}

func renderInstanceFileTemplates(appObj *corev1.App, inst *corev1.Instance, values map[string]string, version string) error {
	renderedFiles := make([]corev1.InstanceFile, 0, len(appObj.FileTemplates))
	renderedPaths := make(map[string]bool, len(appObj.FileTemplates))
	for _, ft := range appObj.FileTemplates {
		content, err := RenderTemplate(ft.Content, values, version)
		if err != nil {
			return fmt.Errorf("render file template %s: %w", ft.Path, err)
		}
		renderedFiles = append(renderedFiles, corev1.InstanceFile{Path: ft.Path, Content: content})
		renderedPaths[ft.Path] = true
	}
	for _, existingFile := range inst.Spec.Files {
		if !renderedPaths[existingFile.Path] {
			renderedFiles = append(renderedFiles, existingFile)
		}
	}
	inst.Spec.Files = renderedFiles
	return nil
}

func renderInstanceMetricsAndStoragePolicy(appObj *corev1.App, inst *corev1.Instance, values map[string]string) {
	if len(appObj.Metrics) > 0 {
		renderedMetrics := make([]corev1.AppMetricSpec, len(appObj.Metrics))
		for i, m := range appObj.Metrics {
			cp := m
			if cp.MaxConfigKey != "" {
				if v, ok := values[cp.MaxConfigKey]; ok && strings.TrimSpace(v) != "" {
					cp.MaxValue = strings.TrimSpace(v)
				}
			}
			renderedMetrics[i] = cp
		}
		inst.Spec.Metrics = renderedMetrics
	}
	if sp := appObj.StoragePolicy; sp != nil {
		if inst.Spec.BackupPolicy == nil {
			inst.Spec.BackupPolicy = &corev1.InstanceBackupPolicy{IntervalHours: 24, RetentionCount: 5}
		}
		if inst.Spec.BackupPolicy.Subtree == "" {
			inst.Spec.BackupPolicy.Subtree = sp.BackupSubtree
		}
		if len(inst.Spec.BackupPolicy.AnchorFiles) == 0 && len(sp.AnchorFiles) > 0 {
			inst.Spec.BackupPolicy.AnchorFiles = append([]string(nil), sp.AnchorFiles...)
		}
		if !inst.Spec.BackupPolicy.ReplaceOnRestore && sp.ReplaceOnRestore {
			inst.Spec.BackupPolicy.ReplaceOnRestore = true
		}
		if !inst.Spec.BackupPolicy.BackupRequiresStop && sp.BackupRequiresStop {
			inst.Spec.BackupPolicy.BackupRequiresStop = true
		}
		if inst.Spec.BackupPolicy.ConsoleDelivery == "" && strings.TrimSpace(appObj.ConsoleDelivery) != "" {
			inst.Spec.BackupPolicy.ConsoleDelivery = strings.TrimSpace(appObj.ConsoleDelivery)
		}
		if inst.Spec.BackupPolicy.PreBackupCommand == "" {
			inst.Spec.BackupPolicy.PreBackupCommand = sp.PreBackupCommand
		}
		if inst.Spec.BackupPolicy.PostBackupCommand == "" {
			inst.Spec.BackupPolicy.PostBackupCommand = sp.PostBackupCommand
		}
	}
}
