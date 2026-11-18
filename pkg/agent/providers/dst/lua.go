package dst

import (
	"regexp"
	"strings"
)

var dstConfigOptionsRe = regexp.MustCompile(`\bconfiguration_options\s*=\s*\{`)

func extractExistingDSTConfigBlock(existing, workshopKey string) string {
	if existing == "" {
		return ""
	}
	entryBlock, ok := findDSTWorkshopBlock(existing, workshopKey)
	if !ok {
		return ""
	}
	loc := dstConfigOptionsRe.FindStringIndex(entryBlock)
	if len(loc) != 2 {
		return ""
	}
	openIdx := loc[1] - 1
	endIdx, ok := findMatchingBrace(entryBlock, openIdx)
	if !ok {
		return ""
	}
	return strings.TrimSpace(entryBlock[openIdx : endIdx+1])
}

func findDSTWorkshopBlock(src, workshopKey string) (string, bool) {
	for _, marker := range []string{`["` + workshopKey + `"]`, `['` + workshopKey + `']`} {
		idx := strings.Index(src, marker)
		if idx < 0 {
			continue
		}
		rest := src[idx+len(marker):]
		eqIdx := strings.IndexByte(rest, '=')
		if eqIdx < 0 {
			continue
		}
		openOff := strings.IndexByte(rest[eqIdx:], '{')
		if openOff < 0 {
			continue
		}
		openIdx := eqIdx + openOff
		endIdx, ok := findMatchingBrace(rest, openIdx)
		if ok {
			return rest[openIdx : endIdx+1], true
		}
	}
	return "", false
}

func findMatchingBrace(src string, openIdx int) (int, bool) {
	depth := 0
	for i := openIdx; i < len(src); i++ {
		if next, jumped := skipQuoted(src, i); jumped {
			i = next - 1
			continue
		}
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

func skipQuoted(src string, i int) (int, bool) {
	if src[i] != '"' && src[i] != '\'' {
		return i, false
	}
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
