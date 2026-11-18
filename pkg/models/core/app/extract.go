package app

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"regexp"
	"strconv"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/models/core/app/driver"
)

const maxExtractedBackupFields = 1024

var (
	templateActionRe   = regexp.MustCompile(`\{\{.*?\}\}`)
	wholePlaceholderRe = regexp.MustCompile(`^\{\{\s*(?:\.Values\.)?([a-zA-Z0-9_][a-zA-Z0-9_.]*)\s*\}\}$`)
	defaultExprRe      = regexp.MustCompile(`^\{\{\s*default\s+"[^"]*"\s+\.Values\.([a-zA-Z0-9_][a-zA-Z0-9_.]*)\s*\}\}$`)
	pipeDefaultExprRe  = regexp.MustCompile(`^\{\{\s*\.Values\.([a-zA-Z0-9_][a-zA-Z0-9_.]*)\s*\|\s*default\s+"[^"]*"\s*\}\}$`)
	flexTokenRe        = regexp.MustCompile(`[ \t]*=[ \t]*|:[ \t]+|[ \t]*/?>|[ \t]+`)
)

type scopedLine struct {
	scope string
	text  string
}

type templateClause struct {
	scope    string
	text     string
	isInline bool
	prefix   string
	suffix   string
	field    string
	quoted   bool
	isAffix  bool
	regex    *regexp.Regexp
}

// ExtractArchiveConfigFields reverse-extracts declarative ConfigSchema values from small archive
// text files matched against App.FileTemplates. Secrets are digested, never stored in plaintext.
func ExtractArchiveConfigFields(appObj *corev1.App, instanceUID string, archiveTextFiles map[string]string, defaultTarget string) []corev1.InstanceBackupExtractedField {
	if appObj == nil || len(appObj.ConfigSchema) == 0 || len(appObj.FileTemplates) == 0 || len(archiveTextFiles) == 0 {
		return nil
	}
	schemaByName, orderedNames := indexSchemaFields(appObj.ConfigSchema)
	archPaths := driver.SortedTextFileKeys(archiveTextFiles)
	seen := make(map[string]struct{})
	out := make([]corev1.InstanceBackupExtractedField, 0, 32)
	for _, ft := range appObj.FileTemplates {
		tplPath := driver.NormalizeRelConfigPath(ft.Path)
		if tplPath == "" {
			continue
		}
		for _, archPath := range matchTemplateArchivePaths(tplPath, defaultTarget, archPaths) {
			rawMap := extractTemplateValues(ft.Content, archiveTextFiles[archPath], appObj.ConfigSchema)
			out = appendExtractedFields(out, seen, orderedNames, schemaByName, rawMap, tplPath, archPath, instanceUID)
			if len(out) >= maxExtractedBackupFields {
				return out[:maxExtractedBackupFields]
			}
		}
	}
	return out
}

// BuildBackupConfigPreview compares controller-extracted archive fields for (sourcePath, targetPath)
// against the Instance's active configuration in memory without external storage I/O.
func BuildBackupConfigPreview(appObj *corev1.App, inst *corev1.Instance, extracted []corev1.InstanceBackupExtractedField, sourcePath, targetPath string) []corev1.InstanceBackupConfigItem {
	if appObj == nil || len(appObj.ConfigSchema) == 0 || len(extracted) == 0 || strings.TrimSpace(sourcePath) == "" {
		return nil
	}
	archByField := indexExtractedForMapping(appObj.FileTemplates, extracted, sourcePath, targetPath)
	if len(archByField) == 0 {
		return nil
	}
	curVals := resolveCurrentPreviewValues(appObj, inst)
	managedSet := buildManagedFileSet(inst)
	uid := ""
	if inst != nil {
		uid = inst.Metadata.UID
	}
	return selectPreviewItems(appObj.ConfigSchema, archByField, curVals, managedSet, uid)
}

func indexSchemaFields(schema []corev1.AppConfigField) (map[string]corev1.AppConfigField, []string) {
	byName := make(map[string]corev1.AppConfigField, len(schema))
	names := make([]string, 0, len(schema))
	for _, f := range schema {
		if f.Name == "" {
			continue
		}
		if _, dup := byName[f.Name]; !dup {
			names = append(names, f.Name)
		}
		byName[f.Name] = f
	}
	return byName, names
}

func matchTemplateArchivePaths(tplPath, defaultTarget string, archPaths []string) []string {
	relSuffix := tplPath
	normTarget, err := metav1.NormalizeArchiveEntryPath(defaultTarget, true)
	if err == nil && normTarget != "." && strings.HasPrefix(tplPath, normTarget+"/") {
		relSuffix = strings.TrimPrefix(tplPath, normTarget+"/")
	}
	var out []string
	for _, ap := range archPaths {
		if ap == tplPath || ap == relSuffix || strings.HasSuffix(ap, "/"+relSuffix) {
			out = append(out, ap)
		}
	}
	return out
}

func appendExtractedFields(dst []corev1.InstanceBackupExtractedField, seen map[string]struct{}, orderedNames []string, byName map[string]corev1.AppConfigField, rawMap map[string]string, tplPath, archPath, instanceUID string) []corev1.InstanceBackupExtractedField {
	for _, name := range orderedNames {
		if len(dst) >= maxExtractedBackupFields {
			return dst
		}
		val, ok := rawMap[name]
		if !ok {
			continue
		}
		k := tplPath + "\x00" + archPath + "\x00" + name
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		dst = append(dst, buildExtractedFieldItem(byName[name], val, tplPath, archPath, instanceUID))
	}
	return dst
}

func buildExtractedFieldItem(f corev1.AppConfigField, rawVal, tplPath, archPath, instanceUID string) corev1.InstanceBackupExtractedField {
	trimmed := strings.TrimSpace(rawVal)
	item := corev1.InstanceBackupExtractedField{
		Name:         f.Name,
		TemplatePath: tplPath,
		ArchivePath:  archPath,
	}
	if isSecretConfigField(f) {
		item.Secret = true
		item.HasValue = trimmed != ""
		if item.HasValue {
			item.SecretDigest = secretFieldDigest(instanceUID, trimmed)
		}
		return item
	}
	item.Value = trimmed
	return item
}

func extractTemplateValues(tplContent, actualContent string, schema []corev1.AppConfigField) map[string]string {
	out := make(map[string]string)
	if field := parseSingleActionField(strings.TrimSpace(tplContent)); field != "" {
		out[field] = strings.TrimSpace(actualContent)
		return out
	}
	tplLines := scanScopedLines(tplContent)
	actLines := scanScopedLines(actualContent)
	clauses := buildTemplateClauses(tplLines)
	matchTemplateClauses(clauses, actLines, out)
	extractConditionalFields(tplLines, actLines, schema, out)
	return out
}

func scanScopedLines(content string) []scopedLine {
	var items []scopedLine
	var braceStack []string
	sectionHeader := ""
	for _, logical := range mergeContinuationLines(content) {
		if strings.HasPrefix(logical, "[") && strings.HasSuffix(logical, "]") && !strings.Contains(logical, "{{") {
			sectionHeader = strings.TrimSpace(logical[1 : len(logical)-1])
			braceStack = nil
			continue
		}
		items = append(items, scopedLine{scope: joinNonEmptyScope(sectionHeader, braceStack), text: logical})
		updateBraceScope(logical, &braceStack)
	}
	return items
}

func mergeContinuationLines(content string) []string {
	var out []string
	var buf string
	parenDepth := 0
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if parenDepth == 0 {
			if isIgnoredConfigLine(line) {
				continue
			}
			buf = line
		} else {
			buf = strings.TrimSpace(buf + " " + line)
		}
		parenDepth += parenDeltaOutsideQuotes(line)
		if parenDepth < 0 {
			parenDepth = 0
		}
		if parenDepth == 0 || (!strings.HasSuffix(line, "(") && !strings.HasSuffix(line, ",")) {
			out = append(out, buf)
			parenDepth = 0
		}
	}
	if parenDepth > 0 && buf != "" {
		out = append(out, buf)
	}
	return out
}

func parenDeltaOutsideQuotes(s string) int {
	delta, inQuote, escaped := 0, false, false
	for i := range len(s) {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inQuote {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote {
			switch ch {
			case '(':
				delta++
			case ')':
				delta--
			}
		}
	}
	return delta
}

func isIgnoredConfigLine(line string) bool {
	if strings.HasPrefix(line, "# Managed by GamePanel ") || strings.HasPrefix(line, "// Managed by GamePanel ") {
		return false
	}
	return line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") ||
		strings.HasPrefix(line, "--") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "<!--")
}

func joinNonEmptyScope(header string, stack []string) string {
	var parts []string
	if header != "" {
		parts = append(parts, header)
	}
	for _, s := range stack {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "/")
}

func updateBraceScope(line string, stack *[]string) {
	if strings.Contains(line, "{{") {
		return
	}
	opens, closes, inQuote, escaped := 0, 0, false, false
	firstOpen := -1
	for i := range len(line) {
		ch := line[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inQuote {
			escaped = true
			continue
		}
		if ch == '"' {
			inQuote = !inQuote
			continue
		}
		if !inQuote {
			switch ch {
			case '{':
				opens++
				if firstOpen < 0 {
					firstOpen = i
				}
			case '}':
				closes++
			}
		}
	}
	applyBraceDelta(line, firstOpen, opens, closes, stack)
}

func applyBraceDelta(line string, firstOpen, opens, closes int, stack *[]string) {
	if opens > closes {
		label := ""
		if firstOpen > 0 {
			before := line[:firstOpen]
			if strings.ContainsAny(before, "=:") {
				label = strings.TrimSpace(strings.TrimRight(before, " \t=:"))
			}
		}
		for range opens - closes {
			*stack = append(*stack, label)
		}
	} else if closes > opens {
		pop := min(len(*stack), closes-opens)
		*stack = (*stack)[:len(*stack)-pop]
	}
}

func buildTemplateClauses(tplLines []scopedLine) []templateClause {
	var clauses []templateClause
	for _, tl := range tplLines {
		if !strings.Contains(tl.text, "{{") {
			continue
		}
		if body, ok := findMultiPlaceholderInlineBody(tl.text); ok {
			for _, part := range splitInlineTupleBody(body) {
				if c, compiled := compileTemplateClause(tl.scope, strings.TrimSpace(part), true); compiled {
					clauses = append(clauses, c)
				}
			}
			continue
		}
		if c, compiled := compileTemplateClause(tl.scope, tl.text, false); compiled {
			clauses = append(clauses, c)
		}
	}
	return clauses
}

func findMultiPlaceholderInlineBody(line string) (string, bool) {
	loc := templateActionRe.FindStringIndex(line)
	if loc == nil {
		return "", false
	}
	if parseSingleActionField(line[loc[0]:loc[1]]) == "" {
		return "", false
	}
	openIdx := strings.IndexAny(line[:loc[0]], "({")
	if openIdx < 0 {
		return "", false
	}
	openCh := line[openIdx]
	closeCh := byte(')')
	if openCh == '{' {
		closeCh = '}'
	}
	body, ok := driver.SliceBalancedDelimBody(line[openIdx+1:], openCh, closeCh)
	managedHeader := strings.HasPrefix(line, "# Managed by GamePanel ") || strings.HasPrefix(line, "// Managed by GamePanel ")
	if !ok || (len(templateActionRe.FindAllStringIndex(body, -1)) < 2 && !managedHeader) {
		return "", false
	}
	return body, true
}

func splitInlineTupleBody(body string) []string {
	var parts []string
	start, inQuote, escaped, tmplDepth := 0, false, false, 0
	for i := 0; i < len(body); i++ {
		if i+1 < len(body) && body[i] == '{' && body[i+1] == '{' {
			tmplDepth++
			i++
			continue
		}
		if i+1 < len(body) && body[i] == '}' && body[i+1] == '}' && tmplDepth > 0 {
			tmplDepth--
			i++
			continue
		}
		if tmplDepth > 0 {
			continue
		}
		inQuote, escaped = stepQuoteState(body[i], inQuote, escaped)
		if !inQuote && (body[i] == ',' || body[i] == '|') {
			parts = append(parts, body[start:i])
			start = i + 1
		}
	}
	return append(parts, body[start:])
}

func stepQuoteState(ch byte, inQuote, escaped bool) (bool, bool) {
	if escaped {
		return inQuote, false
	}
	if ch == '\\' && inQuote {
		return inQuote, true
	}
	if ch == '"' {
		return !inQuote, false
	}
	return inQuote, false
}

func compileTemplateClause(scope, raw string, isInline bool) (templateClause, bool) {
	locs := templateActionRe.FindAllStringIndex(raw, -1)
	if len(locs) != 1 {
		return templateClause{}, false
	}
	field := parseSingleActionField(raw[locs[0][0]:locs[0][1]])
	if field == "" {
		return templateClause{}, false
	}
	prefix, suffix := raw[:locs[0][0]], raw[locs[0][1]:]
	if strings.TrimSpace(prefix) == "" && strings.TrimSpace(suffix) == "" {
		return templateClause{}, false
	}
	quoted := strings.HasSuffix(prefix, `"`) && strings.HasPrefix(suffix, `"`)
	isAffix := !quoted && hasLiteralAffixes(prefix, suffix)
	re, err := compileClauseRegex(prefix, suffix, quoted, isInline)
	if err != nil {
		return templateClause{}, false
	}
	return templateClause{
		scope: scope, text: raw, isInline: isInline,
		prefix: prefix, suffix: suffix, field: field,
		quoted: quoted, isAffix: isAffix, regex: re,
	}, true
}

func hasLiteralAffixes(prefix, suffix string) bool {
	trimmedPre := strings.TrimSpace(prefix)
	trimmedSuf := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(suffix), ","))
	endsAssign := strings.HasSuffix(trimmedPre, "=") || strings.HasSuffix(trimmedPre, ":")
	return !endsAssign || trimmedSuf != ""
}

func compileClauseRegex(prefix, suffix string, quoted, isInline bool) (*regexp.Regexp, error) {
	capture := `([^\r\n]*?)`
	if quoted {
		capture = `([^"\r\n]*)`
	} else if isInline {
		capture = `([^,|)\r\n]*?)`
	}
	cleanSuf := suffix
	if !isInline {
		cleanSuf = strings.TrimRight(strings.TrimSpace(suffix), ",")
	}
	escPre := escapeFlexibleLiteral(prefix)
	escSuf := escapeFlexibleLiteral(cleanSuf)
	if isInline {
		return regexp.Compile(`(?i)(?:^|[(,|{ \t])` + escPre + capture + escSuf + `(?:[ \t]*[,|)}]|$)`)
	}
	return regexp.Compile(`(?i)^[ \t]*` + escPre + capture + escSuf + `[ \t]*,?[ \t]*$`)
}

func escapeFlexibleLiteral(s string) string {
	var b strings.Builder
	last := 0
	for _, loc := range flexTokenRe.FindAllStringIndex(s, -1) {
		b.WriteString(regexp.QuoteMeta(s[last:loc[0]]))
		tok := s[loc[0]:loc[1]]
		switch {
		case strings.Contains(tok, "="):
			b.WriteString(`[ \t]*=[ \t]*`)
		case strings.HasPrefix(tok, ":"):
			b.WriteString(`:[ \t]*`)
		case strings.HasSuffix(tok, ">"):
			b.WriteString(`[ \t]*/?>`)
		default:
			b.WriteString(`[ \t]+`)
		}
		last = loc[1]
	}
	b.WriteString(regexp.QuoteMeta(s[last:]))
	return b.String()
}

func parseSingleActionField(raw string) string {
	trimmed := strings.TrimSpace(raw)
	var key string
	switch {
	case wholePlaceholderRe.MatchString(trimmed):
		key = wholePlaceholderRe.FindStringSubmatch(trimmed)[1]
	case defaultExprRe.MatchString(trimmed):
		key = defaultExprRe.FindStringSubmatch(trimmed)[1]
	case pipeDefaultExprRe.MatchString(trimmed):
		key = pipeDefaultExprRe.FindStringSubmatch(trimmed)[1]
	default:
		return ""
	}
	switch key {
	case "else", "end", "nil", "true", "false", "break", "continue", "Version":
		return ""
	}
	return key
}

func matchTemplateClauses(clauses []templateClause, actLines []scopedLine, out map[string]string) {
	prefixCounts := countClausePrefixes(clauses)
	fallbacks := make(map[string]string)
	for _, c := range clauses {
		preKey := strings.ToLower(strings.TrimSpace(c.prefix))
		val, ok := matchOneClause(c, actLines, prefixCounts[preKey] == 1)
		if !ok {
			continue
		}
		if c.isAffix {
			fallbacks[c.field] = val
		} else {
			out[c.field] = val
		}
	}
	for k, v := range fallbacks {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
}

func countClausePrefixes(clauses []templateClause) map[string]int {
	counts := make(map[string]int, len(clauses))
	for _, c := range clauses {
		counts[strings.ToLower(strings.TrimSpace(c.prefix))]++
	}
	return counts
}

func matchOneClause(c templateClause, actLines []scopedLine, allowUniqueFallback bool) (string, bool) {
	for _, al := range actLines {
		if strings.EqualFold(al.scope, c.scope) {
			if m := c.regex.FindStringSubmatch(al.text); len(m) >= 2 {
				return strings.TrimSpace(m[1]), true
			}
		}
	}
	if !allowUniqueFallback {
		return "", false
	}
	matches, val := 0, ""
	for _, al := range actLines {
		if m := c.regex.FindStringSubmatch(al.text); len(m) >= 2 {
			matches++
			val = strings.TrimSpace(m[1])
		}
	}
	if matches == 1 {
		return val, true
	}
	return "", false
}

func extractConditionalFields(tplLines, actLines []scopedLine, schema []corev1.AppConfigField, out map[string]string) {
	for _, f := range schema {
		if out[f.Name] != "" {
			continue
		}
		candidates := conditionalCandidates(f)
		if len(candidates) < 2 {
			continue
		}
		condLines := findConditionalLines(tplLines, f.Name)
		if len(condLines) == 0 {
			continue
		}
		if matched := matchUniqueCandidate(condLines, actLines, f.Name, candidates); matched != "" {
			out[f.Name] = matched
		}
	}
}

func conditionalCandidates(f corev1.AppConfigField) []string {
	if f.Type == corev1.ConfigFieldTypeSelect && len(f.Options) >= 2 {
		out := make([]string, 0, len(f.Options))
		for _, opt := range f.Options {
			out = append(out, opt.Value)
		}
		return out
	}
	if f.Type == corev1.ConfigFieldTypeBoolean {
		return []string{"true", "false"}
	}
	return nil
}

func findConditionalLines(tplLines []scopedLine, fieldName string) []scopedLine {
	var out []scopedLine
	for _, tl := range tplLines {
		if !strings.Contains(tl.text, "{{") || !lineMentionsField(tl.text, fieldName) {
			continue
		}
		locs := templateActionRe.FindAllStringIndex(tl.text, -1)
		if len(locs) == 1 && parseSingleActionField(tl.text[locs[0][0]:locs[0][1]]) != "" {
			continue
		}
		out = append(out, tl)
	}
	return out
}

func lineMentionsField(line, fieldName string) bool {
	if strings.Contains(line, `"`+fieldName+`"`) {
		return true
	}
	quoted := regexp.QuoteMeta(fieldName)
	re, err := regexp.Compile(`(^|[^a-zA-Z0-9_])\.?(?:Values\.)?` + quoted + `([^a-zA-Z0-9_]|$)`)
	if err != nil {
		return false
	}
	for _, act := range templateActionRe.FindAllString(line, -1) {
		if re.MatchString(act) {
			return true
		}
	}
	return false
}

func matchUniqueCandidate(condLines, actLines []scopedLine, fieldName string, candidates []string) string {
	bestVal, bestScore, ties := "", 0, 0
	for _, cand := range candidates {
		score := scoreCandidateLines(condLines, actLines, fieldName, cand)
		if score == 0 {
			continue
		}
		if score > bestScore {
			bestVal, bestScore, ties = cand, score, 1
		} else if score == bestScore {
			ties++
		}
	}
	if ties == 1 {
		return bestVal
	}
	return ""
}

func scoreCandidateLines(condLines, actLines []scopedLine, fieldName, candVal string) int {
	vals := map[string]string{fieldName: candVal}
	score := 0
	for _, cl := range condLines {
		rendered, err := RenderTemplate(cl.text, vals, "")
		if err != nil || strings.Contains(rendered, "{{") {
			continue
		}
		trimmed := strings.TrimRight(strings.TrimSpace(rendered), ",")
		if trimmed == "" || isIgnoredConfigLine(trimmed) {
			continue
		}
		re, err := regexp.Compile(`(?i)^[ \t]*` + escapeFlexibleLiteral(trimmed) + `[ \t]*,?[ \t]*$`)
		if err != nil {
			continue
		}
		if matchRenderedLiteralLine(re, cl.scope, actLines) {
			score++
		}
	}
	return score
}

func matchRenderedLiteralLine(re *regexp.Regexp, scope string, actLines []scopedLine) bool {
	for _, al := range actLines {
		if strings.EqualFold(al.scope, scope) && re.MatchString(al.text) {
			return true
		}
	}
	for _, al := range actLines {
		if re.MatchString(al.text) {
			return true
		}
	}
	return false
}

func indexExtractedForMapping(templates []corev1.AppFileTemplate, extracted []corev1.InstanceBackupExtractedField, sourcePath, targetPath string) map[string]corev1.InstanceBackupExtractedField {
	res := make(map[string]corev1.InstanceBackupExtractedField)
	for _, ft := range templates {
		tplPath := driver.NormalizeRelConfigPath(ft.Path)
		mapped, ok := mapTemplateToArchivePath(tplPath, sourcePath, targetPath)
		if !ok {
			continue
		}
		for _, ex := range extracted {
			if ex.TemplatePath == tplPath && ex.ArchivePath == mapped {
				if _, exists := res[ex.Name]; !exists {
					res[ex.Name] = ex
				}
			}
		}
	}
	return res
}

func mapTemplateToArchivePath(tplPath, sourcePath, targetPath string) (string, bool) {
	if tplPath == "" {
		return "", false
	}
	src, err := metav1.NormalizeArchiveEntryPath(sourcePath, true)
	if err != nil {
		return "", false
	}
	tgt := normalizeInspectTarget(targetPath)
	rel, ok := stripTemplateTargetPrefix(tplPath, src, tgt)
	if !ok {
		return "", false
	}
	if src == "." {
		return rel, rel != ""
	}
	if rel == "" {
		return src, true
	}
	return path.Join(src, rel), true
}

func normalizeInspectTarget(targetPath string) string {
	if strings.TrimSpace(targetPath) == "" {
		return "."
	}
	if norm, err := metav1.NormalizeArchiveEntryPath(targetPath, true); err == nil {
		return norm
	}
	return "."
}

func stripTemplateTargetPrefix(tplPath, src, tgt string) (string, bool) {
	switch {
	case tgt == ".":
		return tplPath, true
	case tplPath == tgt:
		return "", true
	case strings.HasPrefix(tplPath, tgt+"/"):
		return strings.TrimPrefix(tplPath, tgt+"/"), true
	case src == ".":
		return tplPath, true
	default:
		return "", false
	}
}

func resolveCurrentPreviewValues(appObj *corev1.App, inst *corev1.Instance) map[string]string {
	if inst == nil {
		values := make(map[string]string, len(appObj.ConfigSchema))
		for _, f := range appObj.ConfigSchema {
			if f.Default != "" {
				values[f.Name] = f.Default
			}
		}
		return values
	}
	values, hasSavedAnnotation, err := extractAppConfigValues(appObj, inst)
	if err != nil {
		values = make(map[string]string)
	}
	if !hasSavedAnnotation && len(inst.Spec.Files) > 0 {
		overlaySpecFileValues(appObj, inst.Spec.Files, values)
	}
	return values
}

func overlaySpecFileValues(appObj *corev1.App, files []corev1.InstanceFile, dst map[string]string) {
	fileMap := make(map[string]string, len(files))
	for _, f := range files {
		if norm := driver.NormalizeRelConfigPath(f.Path); norm != "" {
			fileMap[norm] = f.Content
		}
	}
	for _, ft := range appObj.FileTemplates {
		tplPath := driver.NormalizeRelConfigPath(ft.Path)
		content, ok := fileMap[tplPath]
		if !ok {
			continue
		}
		for k, v := range extractTemplateValues(ft.Content, content, appObj.ConfigSchema) {
			dst[k] = v
		}
	}
}

func buildManagedFileSet(inst *corev1.Instance) map[string]bool {
	out := make(map[string]bool)
	if inst == nil {
		return out
	}
	for _, f := range inst.Spec.Files {
		if norm := driver.NormalizeRelConfigPath(f.Path); norm != "" {
			out[norm] = true
		}
	}
	return out
}

func selectPreviewItems(schema []corev1.AppConfigField, arch map[string]corev1.InstanceBackupExtractedField, curVals map[string]string, managedSet map[string]bool, instanceUID string) []corev1.InstanceBackupConfigItem {
	items := make([]corev1.InstanceBackupConfigItem, 0, len(arch))
	for _, f := range schema {
		ex, ok := arch[f.Name]
		if !ok {
			continue
		}
		if item, keep := buildPreviewItem(f, ex, curVals[f.Name], managedSet[ex.TemplatePath], instanceUID); keep {
			items = append(items, item)
		}
	}
	return items
}

func buildPreviewItem(f corev1.AppConfigField, ex corev1.InstanceBackupExtractedField, curRaw string, managed bool, instanceUID string) (corev1.InstanceBackupConfigItem, bool) {
	item := corev1.InstanceBackupConfigItem{
		Name: f.Name, Label: driver.FirstNonEmpty(f.Label, f.Name), Group: f.Group, Type: f.Type,
		OverriddenByPanel: managed, Secret: ex.Secret || isSecretConfigField(f),
	}
	if item.Secret {
		curTrim := strings.TrimSpace(curRaw)
		item.ArchiveSet = ex.HasValue
		item.CurrentSet = curTrim != ""
		if !item.ArchiveSet && !item.CurrentSet {
			return item, false
		}
		curDigest := ""
		if item.CurrentSet {
			curDigest = secretFieldDigest(instanceUID, curTrim)
		}
		item.Different = item.ArchiveSet != item.CurrentSet || (item.ArchiveSet && ex.SecretDigest != curDigest)
		return item, true
	}
	archTrim, curTrim := strings.TrimSpace(ex.Value), strings.TrimSpace(curRaw)
	item.ArchiveSet = archTrim != ""
	item.CurrentSet = curTrim != ""
	if !item.ArchiveSet && !item.CurrentSet {
		return item, false
	}
	item.Different = !configValuesEqual(f.Type, archTrim, curTrim)
	item.ArchiveValue = normalizePreviewFieldValue(f, archTrim)
	item.CurrentValue = normalizePreviewFieldValue(f, curTrim)
	return item, true
}

func configValuesEqual(fieldType, a, b string) bool {
	switch fieldType {
	case corev1.ConfigFieldTypeBoolean:
		if (a == "") != (b == "") {
			return false
		}
		return parseBoolConfig(a) == parseBoolConfig(b)
	case corev1.ConfigFieldTypeNumber:
		fa, ea := strconv.ParseFloat(a, 64)
		fb, eb := strconv.ParseFloat(b, 64)
		if ea == nil && eb == nil {
			return fa == fb
		}
	}
	return a == b
}

func normalizePreviewFieldValue(f corev1.AppConfigField, trimmed string) string {
	if trimmed == "" {
		return ""
	}
	switch f.Type {
	case corev1.ConfigFieldTypeBoolean:
		return strconv.FormatBool(parseBoolConfig(trimmed))
	case corev1.ConfigFieldTypeSelect:
		for _, opt := range f.Options {
			if strings.EqualFold(opt.Value, trimmed) && strings.TrimSpace(opt.Label) != "" {
				return opt.Label
			}
		}
	}
	return trimmed
}

func parseBoolConfig(raw string) bool {
	lower := strings.ToLower(strings.TrimSpace(raw))
	return lower == "true" || lower == "1" || lower == "yes"
}

func isSecretConfigField(f corev1.AppConfigField) bool {
	return f.Type == corev1.ConfigFieldTypePassword
}

func secretFieldDigest(instanceUID, val string) string {
	sum := sha256.Sum256([]byte(instanceUID + "\x00" + val))
	return hex.EncodeToString(sum[:])
}
