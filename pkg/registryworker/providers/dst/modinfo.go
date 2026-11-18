package dst

import (
	"regexp"
	"strconv"
	"strings"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

var (
	dstConfigHeaderRe = regexp.MustCompile(`\bconfiguration_options\s*=\s*\{`)
	dstNumberLitRe    = regexp.MustCompile(`^[+-]?(?:\d+\.?\d*|\.\d+)$`)
	dstQuotedLitRe    = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`)
)

func parseDSTModInfoSchema(src string) []registryv1.AddonConfigField {
	cleaned := stripLuaComments(src)
	loc := dstConfigHeaderRe.FindStringIndex(cleaned)
	if len(loc) != 2 {
		return nil
	}
	block, ok := extractLuaBalancedBlock(cleaned, loc[1]-1)
	if !ok {
		return nil
	}
	var fields []registryv1.AddonConfigField
	seen := make(map[string]bool)
	for _, entry := range splitLuaTableElements(block) {
		field, valid := parseDSTConfigEntry(entry)
		if !valid || seen[field.Name] {
			continue
		}
		seen[field.Name] = true
		fields = append(fields, field)
	}
	return fields
}

func parseDSTConfigEntry(body string) (registryv1.AddonConfigField, bool) {
	pairs := splitLuaKeyValues(body)
	name := evalLuaStringExpr(pairs["name"])
	if strings.TrimSpace(name) == "" {
		return registryv1.AddonConfigField{}, false
	}
	label := evalLuaStringExpr(pairs["label"])
	if label == "" {
		label = name
	}
	defVal, defType := evalLuaScalarExpr(pairs["default"])
	opts := parseDSTConfigOptions(pairs["options"])
	if len(opts) == 1 && strings.TrimSpace(opts[0].Label) == "" && opts[0].Value == "" {
		return registryv1.AddonConfigField{}, false
	}
	field := registryv1.AddonConfigField{
		Name:        name,
		Label:       label,
		Description: evalLuaStringExpr(pairs["hover"]),
		Default:     defVal,
		DefaultType: defType,
		Options:     opts,
	}
	assignDSTFieldType(&field)
	return field, true
}

func assignDSTFieldType(field *registryv1.AddonConfigField) {
	switch {
	case len(field.Options) > 0:
		field.Type = registryv1.AddonConfigTypeSelect
	case field.DefaultType == registryv1.AddonConfigValueBoolean:
		field.Type = registryv1.AddonConfigTypeBoolean
	case field.DefaultType == registryv1.AddonConfigValueNumber:
		field.Type = registryv1.AddonConfigTypeNumber
	default:
		field.Type = registryv1.AddonConfigTypeText
	}
}

func parseDSTConfigOptions(raw string) []registryv1.AddonConfigOption {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return nil
	}
	inner, ok := extractLuaBalancedBlock(trimmed, 0)
	if !ok {
		return nil
	}
	var opts []registryv1.AddonConfigOption
	for _, elem := range splitLuaTableElements(inner) {
		pairs := splitLuaKeyValues(elem)
		val, valType := evalLuaScalarExpr(pairs["data"])
		label := evalLuaStringExpr(pairs["description"])
		if label == "" {
			label = val
		}
		opts = append(opts, registryv1.AddonConfigOption{
			Label:       label,
			Value:       val,
			ValueType:   valType,
			Description: evalLuaStringExpr(pairs["hover"]),
		})
	}
	return opts
}

func splitLuaTableElements(block string) []string {
	var out []string
	i := 0
	for i < len(block) {
		if block[i] == '{' {
			body, next, ok := extractLuaBalancedBlockWithEnd(block, i)
			if !ok {
				break
			}
			out = append(out, body)
			i = next
			continue
		}
		i = advancePastLuaToken(block, i)
	}
	return out
}

func splitLuaKeyValues(body string) map[string]string {
	out := make(map[string]string)
	for _, part := range splitTopLevelComma(body) {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		if strings.HasPrefix(key, "[") && strings.HasSuffix(key, "]") {
			key = evalLuaStringExpr(key[1 : len(key)-1])
		}
		if key != "" {
			out[key] = strings.TrimSpace(v)
		}
	}
	return out
}

func splitTopLevelComma(s string) []string {
	var parts []string
	start, braces, parens := 0, 0, 0
	for i := 0; i < len(s); i++ {
		next, jumped := skipLuaQuotedOrLong(s, i)
		if jumped {
			i = next - 1
			continue
		}
		switch s[i] {
		case '{':
			braces++
		case '}':
			braces--
		case '(':
			parens++
		case ')':
			parens--
		case ',', ';':
			if braces == 0 && parens == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(s[start:]); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func evalLuaScalarExpr(expr string) (string, registryv1.AddonConfigValueType) {
	trimmed := trimOuterParens(strings.TrimSpace(expr))
	switch {
	case trimmed == "" || trimmed == "nil":
		return "", ""
	case trimmed == "true" || trimmed == "false":
		return trimmed, registryv1.AddonConfigValueBoolean
	case dstNumberLitRe.MatchString(trimmed):
		return normalizeNumberString(trimmed), registryv1.AddonConfigValueNumber
	case strings.HasPrefix(trimmed, "\"") || strings.HasPrefix(trimmed, "'") || strings.HasPrefix(trimmed, "[["):
		return unquoteLuaString(trimmed), registryv1.AddonConfigValueString
	default:
		if val, ok := evalSimpleLuaArithmetic(trimmed); ok {
			return val, registryv1.AddonConfigValueNumber
		}
		if s := evalLuaStringExpr(trimmed); s != "" {
			return s, registryv1.AddonConfigValueString
		}
		return trimmed, registryv1.AddonConfigValueString
	}
}

func evalLuaStringExpr(expr string) string {
	trimmed := trimOuterParens(strings.TrimSpace(expr))
	if trimmed == "" || trimmed == "nil" {
		return ""
	}
	if strings.HasPrefix(trimmed, "\"") || strings.HasPrefix(trimmed, "'") || strings.HasPrefix(trimmed, "[[") {
		if !strings.Contains(trimmed, "..") {
			return unquoteLuaString(trimmed)
		}
	}
	matches := dstQuotedLitRe.FindAllString(trimmed, -1)
	if len(matches) == 0 {
		return ""
	}
	if strings.Contains(trimmed, " and ") && strings.Contains(trimmed, " or ") && len(matches) >= 2 {
		low := strings.ToLower(trimmed)
		preferSecond := (!strings.Contains(low, "~=") && (strings.Contains(low, `"en"`) || strings.Contains(low, `'en'`))) ||
			(strings.Contains(low, "~=") && (strings.Contains(low, `"zh"`) || strings.Contains(low, `'zh'`)))
		if preferSecond {
			return unquoteLuaString(matches[1])
		}
		return unquoteLuaString(matches[0])
	}
	var b strings.Builder
	for _, m := range matches {
		b.WriteString(unquoteLuaString(m))
	}
	return b.String()
}

func unquoteLuaString(lit string) string {
	if strings.HasPrefix(lit, "[[") && strings.HasSuffix(lit, "]]") && len(lit) >= 4 {
		return strings.TrimSpace(lit[2 : len(lit)-2])
	}
	if len(lit) >= 2 && (lit[0] == '"' || lit[0] == '\'') && lit[len(lit)-1] == lit[0] {
		inner := lit[1 : len(lit)-1]
		repl := strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\"`, `"`, `\'`, `'`, `\\`, `\`)
		return strings.TrimSpace(repl.Replace(inner))
	}
	return strings.TrimSpace(lit)
}

func evalSimpleLuaArithmetic(expr string) (string, bool) {
	if parts := strings.Split(expr, "*"); len(parts) == 2 {
		a, errA := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		b, errB := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errA == nil && errB == nil {
			return strconv.FormatFloat(a*b, 'g', -1, 64), true
		}
	}
	return "", false
}

func normalizeNumberString(raw string) string {
	if strings.HasPrefix(raw, ".") {
		raw = "0" + raw
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	return raw
}

func trimOuterParens(s string) string {
	for strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

func extractLuaBalancedBlock(src string, openIdx int) (string, bool) {
	body, _, ok := extractLuaBalancedBlockWithEnd(src, openIdx)
	return body, ok
}

func extractLuaBalancedBlockWithEnd(src string, openIdx int) (string, int, bool) {
	depth := 0
	for i := openIdx; i < len(src); i++ {
		next, jumped := skipLuaQuotedOrLong(src, i)
		if jumped {
			i = next - 1
			continue
		}
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

func advancePastLuaToken(src string, i int) int {
	if next, jumped := skipLuaQuotedOrLong(src, i); jumped {
		return next
	}
	return i + 1
}

func skipLuaQuotedOrLong(src string, i int) (int, bool) {
	if i+1 < len(src) && src[i] == '[' && src[i+1] == '[' {
		if end := strings.Index(src[i+2:], "]]"); end >= 0 {
			return i + 2 + end + 2, true
		}
		return len(src), true
	}
	if src[i] == '"' || src[i] == '\'' {
		q := src[i]
		for j := i + 1; j < len(src); j++ {
			if src[j] == '\\' {
				j++
				continue
			}
			if src[j] == q {
				return j + 1, true
			}
		}
		return len(src), true
	}
	return i, false
}

func stripLuaComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for i := 0; i < len(src); {
		if src[i] == '-' && i+1 < len(src) && src[i+1] == '-' {
			i = skipLuaComment(src, i+2)
			continue
		}
		if next, jumped := skipLuaQuotedOrLong(src, i); jumped {
			b.WriteString(src[i:next])
			i = next
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

func skipLuaComment(src string, i int) int {
	if i+1 < len(src) && src[i] == '[' && src[i+1] == '[' {
		if end := strings.Index(src[i+2:], "]]"); end >= 0 {
			return i + 2 + end + 2
		}
		return len(src)
	}
	if nl := strings.IndexByte(src[i:], '\n'); nl >= 0 {
		return i + nl
	}
	return len(src)
}
