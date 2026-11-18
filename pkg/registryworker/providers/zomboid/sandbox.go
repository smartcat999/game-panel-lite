package zomboid

import (
	"regexp"
	"strconv"
	"strings"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

type pzRawFile struct {
	path    string
	content string
}

type pzTranslationEntry struct {
	rank int
	val  string
}

var (
	pzBlockCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)
	pzOptionHeaderRe = regexp.MustCompile(`(?m)\boption\s+([A-Za-z0-9_.]+)\s*\{`)
	pzTransLineRe    = regexp.MustCompile(`^\s*([A-Za-z0-9_.]+)\s*=\s*"((?:[^"\\]|\\.)*)"`)
)

func isSandboxOptionsRel(lowSlash string) bool {
	return strings.HasSuffix("/"+lowSlash, "/media/sandbox-options.txt")
}

func translationLangRank(lowSlash string) int {
	padded := "/" + lowSlash
	switch {
	case strings.Contains(padded, "/media/lua/shared/translate/cn/sandbox_"):
		return 3
	case strings.Contains(padded, "/media/lua/shared/translate/ch/sandbox_"):
		return 2
	case strings.Contains(padded, "/media/lua/shared/translate/en/sandbox_"):
		return 1
	default:
		return 0
	}
}

func parsePZSandboxFiles(files []pzRawFile) []registryv1.AddonConfigField {
	trans := make(map[string]pzTranslationEntry)
	for _, f := range files {
		if rank := translationLangRank(strings.ToLower(f.path)); rank > 0 {
			mergePZTranslations(trans, rank, f.content)
		}
	}
	var fields []registryv1.AddonConfigField
	seen := make(map[string]bool)
	for _, f := range files {
		if !isSandboxOptionsRel(strings.ToLower(f.path)) {
			continue
		}
		for _, field := range parsePZSandboxOptions(f.content, trans) {
			if !seen[field.Name] {
				seen[field.Name] = true
				fields = append(fields, field)
			}
		}
	}
	return fields
}

func mergePZTranslations(dst map[string]pzTranslationEntry, rank int, content string) {
	cleaned := strings.ReplaceAll(content, "\r\n", "\n")
	for _, line := range strings.Split(cleaned, "\n") {
		m := pzTransLineRe.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		key := strings.TrimSpace(m[1])
		val := strings.ReplaceAll(m[2], `\"`, `"`)
		val = strings.ReplaceAll(val, `<br>`, "\n")
		val = strings.ReplaceAll(val, `<LINE>`, "\n")
		if prev, exists := dst[key]; !exists || rank >= prev.rank {
			dst[key] = pzTranslationEntry{rank: rank, val: strings.TrimSpace(val)}
		}
	}
}

func parsePZSandboxOptions(content string, trans map[string]pzTranslationEntry) []registryv1.AddonConfigField {
	stripped := stripPZComments(content)
	var fields []registryv1.AddonConfigField
	pos := 0
	for pos < len(stripped) {
		loc := pzOptionHeaderRe.FindStringSubmatchIndex(stripped[pos:])
		if len(loc) < 4 {
			break
		}
		name := strings.TrimSpace(stripped[pos+loc[2] : pos+loc[3]])
		braceStart := pos + loc[1] - 1
		body, nextPos, ok := extractBalancedBlock(stripped, braceStart)
		if !ok {
			break
		}
		pos = nextPos
		if field, valid := buildPZConfigField(name, body, trans); valid {
			fields = append(fields, field)
		}
	}
	return fields
}

func stripPZComments(src string) string {
	noBlock := pzBlockCommentRe.ReplaceAllString(src, "")
	lines := strings.Split(strings.ReplaceAll(noBlock, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "//"); idx >= 0 {
			lines[i] = line[:idx]
		}
	}
	return strings.Join(lines, "\n")
}

func extractBalancedBlock(src string, openIdx int) (string, int, bool) {
	depth := 0
	for i := openIdx; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[openIdx+1 : i], i + 1, true
			}
		}
	}
	return "", len(src), false
}

func buildPZConfigField(name, body string, trans map[string]pzTranslationEntry) (registryv1.AddonConfigField, bool) {
	attrs := parsePZOptionAttrs(body)
	rawType := strings.ToLower(strings.TrimSpace(attrs["type"]))
	if rawType == "" {
		return registryv1.AddonConfigField{}, false
	}
	tKey := firstNonEmpty(attrs["translation"], strings.ReplaceAll(name, ".", "_"))
	vKey := firstNonEmpty(attrs["valueTranslation"], tKey)
	field := registryv1.AddonConfigField{
		Name:        name,
		Label:       lookupPZTrans(trans, tKey, name),
		Description: lookupPZTrans(trans, tKey+"_tooltip", ""),
		Default:     strings.TrimSpace(attrs["default"]),
	}
	assignPZFieldType(&field, rawType, attrs, vKey, trans)
	return field, true
}

func assignPZFieldType(
	field *registryv1.AddonConfigField,
	rawType string,
	attrs map[string]string,
	vKey string,
	trans map[string]pzTranslationEntry,
) {
	switch rawType {
	case "boolean":
		field.Type = registryv1.AddonConfigTypeBoolean
		field.DefaultType = registryv1.AddonConfigValueBoolean
	case "integer", "double":
		field.Type = registryv1.AddonConfigTypeNumber
		field.DefaultType = registryv1.AddonConfigValueNumber
		field.Min = parseFloatPtr(attrs["min"])
		field.Max = parseFloatPtr(attrs["max"])
		step := 1.0
		if rawType == "double" {
			step = 0.1
		}
		field.Step = &step
	case "enum":
		field.Type = registryv1.AddonConfigTypeSelect
		field.DefaultType = registryv1.AddonConfigValueNumber
		field.Options = buildPZEnumOptions(attrs["numValues"], vKey, trans)
	default:
		field.Type = registryv1.AddonConfigTypeText
		field.DefaultType = registryv1.AddonConfigValueString
	}
}

func parsePZOptionAttrs(body string) map[string]string {
	out := make(map[string]string)
	normalized := strings.NewReplacer("\r\n", ",", "\n", ",").Replace(body)
	for _, token := range strings.Split(normalized, ",") {
		k, v, ok := strings.Cut(token, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.Trim(strings.TrimSpace(v), `"`)
		if key != "" {
			out[key] = val
		}
	}
	return out
}

func buildPZEnumOptions(rawNum, vKey string, trans map[string]pzTranslationEntry) []registryv1.AddonConfigOption {
	n, err := strconv.Atoi(strings.TrimSpace(rawNum))
	if err != nil || n <= 0 || n > 128 {
		return nil
	}
	opts := make([]registryv1.AddonConfigOption, 0, n)
	for i := 1; i <= n; i++ {
		idxStr := strconv.Itoa(i)
		label := lookupPZTrans(trans, vKey+"_option"+idxStr, idxStr)
		opts = append(opts, registryv1.AddonConfigOption{
			Label:     label,
			Value:     idxStr,
			ValueType: registryv1.AddonConfigValueNumber,
		})
	}
	return opts
}

func lookupPZTrans(trans map[string]pzTranslationEntry, key, fallback string) string {
	clean := strings.TrimSpace(key)
	if clean == "" {
		return fallback
	}
	for _, cand := range []string{"Sandbox_" + clean, clean} {
		if entry, ok := trans[cand]; ok && entry.val != "" {
			return entry.val
		}
	}
	return fallback
}

func parseFloatPtr(raw string) *float64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	v, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return nil
	}
	return &v
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
