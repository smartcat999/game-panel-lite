package providercontract

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var digestPattern = regexp.MustCompile("^sha256:[a-f0-9]{64}$")

func (r *Registry) ValidateRevision(manifest Manifest, current, next map[string]any, selections []ModSelection, creating bool) (ValidationResult, error) {
	if err := verifyManifest(manifest, r.key); err != nil {
		return ValidationResult{}, err
	}
	behavior, err := validateConfiguration(manifest.ConfigurationSchema, current, next, creating)
	if err != nil {
		return ValidationResult{}, err
	}
	lock, err := resolveMods(manifest, selections)
	if err != nil {
		return ValidationResult{}, err
	}
	return ValidationResult{ApplyBehavior: behavior, ModLock: lock}, nil
}

func MigrationMode(manifest Manifest, from, to int) (string, error) {
	if from == to {
		return "automatic", nil
	}
	for _, migration := range manifest.SchemaMigrations {
		if migration.FromSchemaVersion == from && migration.ToSchemaVersion == to && migration.Mode != "unsupported" {
			return migration.Mode, nil
		}
	}
	return "", ErrUnsupportedMigration
}

func validateManifest(manifest Manifest) error {
	if manifest.ProviderReleaseID == "" || manifest.GameKey == "" || manifest.DisplayName == "" || manifest.ReleaseVersion == "" || manifest.SchemaVersion < 1 || manifest.ConfigurationSchema.Type != "object" || len(manifest.GameVersions) == 0 || len(manifest.ListenerRequirements) == 0 || manifest.PublishedAt.IsZero() {
		return ErrInvalidManifest
	}
	if !uniqueNonEmpty(manifest.GameVersions) || !uniqueNonEmpty(manifest.Capabilities) || !uniqueNonEmpty(manifest.ConfigurationSchema.Required) {
		return ErrInvalidManifest
	}
	capabilities := make(map[string]bool, len(manifest.Capabilities))
	for _, capability := range manifest.Capabilities {
		switch capability {
		case "configuration", "mods", "console", "logs", "backup", "player-observation", "game-metrics":
			capabilities[capability] = true
		default:
			return ErrInvalidManifest
		}
	}
	if capabilities["mods"] != (manifest.ModCatalog != nil) {
		return ErrInvalidManifest
	}
	for name, field := range manifest.ConfigurationSchema.Properties {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(field.Title) == "" || !validField(field) || !validFieldLocalizations(field) {
			return ErrInvalidManifest
		}
	}
	for _, name := range manifest.ConfigurationSchema.Required {
		if _, ok := manifest.ConfigurationSchema.Properties[name]; !ok {
			return ErrInvalidManifest
		}
	}
	sections := make(map[string]bool, len(manifest.UISchema.Sections))
	for _, section := range manifest.UISchema.Sections {
		if section.ID == "" || section.Title == "" || sections[section.ID] || !validTextLocalizations(section.Localizations) {
			return ErrInvalidManifest
		}
		sections[section.ID] = true
	}
	if len(manifest.UISchema.Fields) != len(manifest.ConfigurationSchema.Properties) {
		return ErrInvalidManifest
	}
	for name, ui := range manifest.UISchema.Fields {
		field, exists := manifest.ConfigurationSchema.Properties[name]
		if !exists || !sections[ui.Section] || !validControl(field.Type, ui.Control) {
			return ErrInvalidManifest
		}
		if ui.VisibleWhen != nil {
			if _, exists := manifest.ConfigurationSchema.Properties[ui.VisibleWhen.Field]; !exists || ui.VisibleWhen.Field == name {
				return ErrInvalidManifest
			}
		}
	}
	primary := 0
	listenerNames := map[string]bool{}
	for _, listener := range manifest.ListenerRequirements {
		if listener.Name == "" || listener.Purpose == "" || listener.InternalPort < 1 || listener.InternalPort > 65535 || listenerNames[listener.Name] || !validStrings(listener.Transports, "tcp", "udp") || listener.ExternalPortPolicy != "allocated" && listener.ExternalPortPolicy != "default-required" || listener.AddressMode != "ip-port" && listener.AddressMode != "ip-only" {
			return ErrInvalidManifest
		}
		listenerNames[listener.Name] = true
		if listener.Primary {
			primary++
		}
	}
	if primary != 1 {
		return ErrInvalidManifest
	}
	for _, metric := range manifest.Metrics {
		if metric.Key == "" || metric.Title == "" || metric.Unit == "" || metric.FreshnessSeconds < 1 || metric.MinimumConfidence < 0 || metric.MinimumConfidence > 1 || metric.Source != "provider-api" && metric.Source != "log-parser" || !capabilities["game-metrics"] && !capabilities["player-observation"] {
			return ErrInvalidManifest
		}
	}
	for _, migration := range manifest.SchemaMigrations {
		if migration.FromSchemaVersion < 1 || migration.ToSchemaVersion <= migration.FromSchemaVersion || migration.Mode != "automatic" && migration.Mode != "manual" && migration.Mode != "unsupported" {
			return ErrInvalidManifest
		}
	}
	if manifest.ModCatalog != nil && !validCatalog(*manifest.ModCatalog) {
		return ErrInvalidManifest
	}
	return nil
}

func validField(field Field) bool {
	switch field.Type {
	case "string", "secret", "string-list", "integer", "number", "boolean", "enum":
	default:
		return false
	}
	switch field.ApplyBehavior {
	case ApplyHotReload, ApplyRestart, ApplyRecreate, ApplyCreateOnly:
	default:
		return false
	}
	if field.Minimum != nil && field.Maximum != nil && *field.Minimum > *field.Maximum || field.MinLength != nil && field.MaxLength != nil && *field.MinLength > *field.MaxLength {
		return false
	}
	if field.Pattern != "" {
		if _, err := regexp.Compile(field.Pattern); err != nil {
			return false
		}
	}
	return field.Type != "enum" || len(field.Enum) > 0
}

func validFieldLocalizations(field Field) bool {
	for locale, localization := range field.Localizations {
		if strings.TrimSpace(locale) == "" || strings.TrimSpace(localization.Title) == "" {
			return false
		}
		if localization.Default != nil {
			if err := validateValue(field, localization.Default); err != nil {
				return false
			}
		}
		if len(localization.EnumLabels) > 0 {
			if field.Type != "enum" || len(localization.EnumLabels) != len(field.Enum) {
				return false
			}
			for _, option := range field.Enum {
				if strings.TrimSpace(localization.EnumLabels[fmt.Sprint(option)]) == "" {
					return false
				}
			}
		}
	}
	return true
}

func validTextLocalizations(localizations map[string]string) bool {
	for locale, value := range localizations {
		if strings.TrimSpace(locale) == "" || strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}

func validControl(fieldType, control string) bool {
	allowed := map[string]map[string]bool{
		"string": {"text": true, "textarea": true}, "secret": {"password": true}, "string-list": {"tag-list": true},
		"integer": {"number": true}, "number": {"number": true}, "boolean": {"switch": true}, "enum": {"select": true},
	}
	return allowed[fieldType][control]
}

func validateConfiguration(schema ConfigurationSchema, current, next map[string]any, creating bool) (ApplyBehavior, error) {
	for key := range next {
		if _, ok := schema.Properties[key]; !ok {
			return "", fmt.Errorf("%w: unknown field %s", ErrInvalidConfiguration, key)
		}
	}
	for _, required := range schema.Required {
		if value, ok := next[required]; !ok || value == nil {
			return "", fmt.Errorf("%w: required field %s", ErrInvalidConfiguration, required)
		}
	}
	result := ApplyHotReload
	for name, field := range schema.Properties {
		value, exists := next[name]
		if exists {
			if err := validateValue(field, value); err != nil {
				return "", fmt.Errorf("%w: %s", ErrInvalidConfiguration, name)
			}
		}
		if creating || reflect.DeepEqual(current[name], value) {
			continue
		}
		if field.ApplyBehavior == ApplyCreateOnly {
			return "", fmt.Errorf("%w: create-only field %s", ErrInvalidConfiguration, name)
		}
		if behaviorRank(field.ApplyBehavior) > behaviorRank(result) {
			result = field.ApplyBehavior
		}
	}
	return result, nil
}

func validateValue(field Field, value any) error {
	switch field.Type {
	case "string", "secret":
		text, ok := value.(string)
		if !ok || field.MinLength != nil && len([]rune(text)) < *field.MinLength || field.MaxLength != nil && len([]rune(text)) > *field.MaxLength {
			return ErrInvalidConfiguration
		}
		if field.Pattern != "" && !regexp.MustCompile(field.Pattern).MatchString(text) {
			return ErrInvalidConfiguration
		}
	case "string-list":
		values, ok := value.([]string)
		if !ok {
			items, jsonValues := value.([]any)
			if !jsonValues {
				return ErrInvalidConfiguration
			}
			values = make([]string, len(items))
			for index, item := range items {
				var itemOK bool
				values[index], itemOK = item.(string)
				if !itemOK {
					return ErrInvalidConfiguration
				}
			}
		}
		_ = values
	case "integer", "number":
		number, ok := numeric(value)
		if !ok || field.Type == "integer" && math.Trunc(number) != number || field.Minimum != nil && number < *field.Minimum || field.Maximum != nil && number > *field.Maximum {
			return ErrInvalidConfiguration
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return ErrInvalidConfiguration
		}
	case "enum":
		for _, option := range field.Enum {
			if reflect.DeepEqual(option, value) {
				return nil
			}
		}
		return ErrInvalidConfiguration
	}
	return nil
}

func resolveMods(manifest Manifest, selections []ModSelection) ([]ModLockEntry, error) {
	if manifest.ModCatalog == nil {
		if len(selections) > 0 {
			return nil, ErrUnresolvedMod
		}
		return []ModLockEntry{}, nil
	}
	versions := map[string]map[string]ModVersion{}
	for _, entry := range manifest.ModCatalog.Entries {
		versions[entry.ModID] = map[string]ModVersion{}
		for _, version := range entry.Versions {
			versions[entry.ModID][version.Version] = version
		}
	}
	requested := map[string]string{}
	direct := map[string]bool{}
	for _, selection := range selections {
		if selection.ModID == "" || selection.Version == "" || requested[selection.ModID] != "" && requested[selection.ModID] != selection.Version {
			return nil, ErrUnresolvedMod
		}
		requested[selection.ModID], direct[selection.ModID] = selection.Version, true
	}
	queue := append([]ModSelection(nil), selections...)
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		version, ok := versions[item.ModID][item.Version]
		if !ok {
			return nil, ErrUnresolvedMod
		}
		for _, dependency := range version.Dependencies {
			if selected := requested[dependency.ModID]; selected != "" && selected != dependency.Version {
				return nil, ErrUnresolvedMod
			}
			if requested[dependency.ModID] == "" {
				requested[dependency.ModID] = dependency.Version
				queue = append(queue, ModSelection{ModID: dependency.ModID, Version: dependency.Version})
			}
		}
	}
	lock := make([]ModLockEntry, 0, len(requested))
	for id, versionName := range requested {
		version, ok := versions[id][versionName]
		if !ok || !digestPattern.MatchString(version.Digest) {
			return nil, ErrUnresolvedMod
		}
		lock = append(lock, ModLockEntry{ModID: id, Version: versionName, Digest: version.Digest, Direct: direct[id]})
	}
	sort.Slice(lock, func(i, j int) bool { return lock[i].ModID < lock[j].ModID })
	return lock, nil
}

func validCatalog(catalog ModCatalog) bool {
	if catalog.Revision < 1 {
		return false
	}
	ids := map[string]bool{}
	for _, entry := range catalog.Entries {
		if entry.ModID == "" || entry.DisplayName == "" || ids[entry.ModID] || len(entry.Versions) == 0 {
			return false
		}
		ids[entry.ModID] = true
		seen := map[string]bool{}
		for _, version := range entry.Versions {
			if version.Version == "" || seen[version.Version] || !digestPattern.MatchString(version.Digest) {
				return false
			}
			seen[version.Version] = true
		}
	}
	return true
}

func behaviorRank(value ApplyBehavior) int {
	switch value {
	case ApplyRestart:
		return 1
	case ApplyRecreate:
		return 2
	default:
		return 0
	}
}

func numeric(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}

func uniqueNonEmpty(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func validStrings(values []string, allowed ...string) bool {
	if len(values) == 0 || !uniqueNonEmpty(values) {
		return false
	}
	wanted := make(map[string]bool, len(allowed))
	for _, value := range allowed {
		wanted[value] = true
	}
	for _, value := range values {
		if !wanted[value] {
			return false
		}
	}
	return true
}
