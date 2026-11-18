package steam

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

var (
	bbImgRe        = regexp.MustCompile(`(?is)\[img[^\]]*\](.*?)\[/img\]`)
	bbURLTagRe     = regexp.MustCompile(`(?is)\[url=([^\]]+)\](.*?)\[/url\]`)
	bbBareURLTagRe = regexp.MustCompile(`(?is)\[url\](.*?)\[/url\]`)
	bbSimpleTagRe  = regexp.MustCompile(`(?is)\[/?(?:b|i|u|strike|spoiler|quote|code|noparse|hr|olist|list|table|tr|td|th)(?:=[^\]]*)?\]`)
	bbHeadingRe    = regexp.MustCompile(`(?is)\[h[1-3]\](.*?)\[/h[1-3]\]`)
	bbH1Re         = regexp.MustCompile(`(?is)\[h1\](.*?)\[/h1\]`)
	bbH2Re         = regexp.MustCompile(`(?is)\[h2\](.*?)\[/h2\]`)
	bbH3Re         = regexp.MustCompile(`(?is)\[h3\](.*?)\[/h3\]`)
	bbBoldRe       = regexp.MustCompile(`(?is)\[b\](.*?)\[/b\]`)
	bbItalicRe     = regexp.MustCompile(`(?is)\[i\](.*?)\[/i\]`)
	bbStrikeRe     = regexp.MustCompile(`(?is)\[strike\](.*?)\[/strike\]`)
	bbCodeRe       = regexp.MustCompile(`(?is)\[(?:code|noparse)\](.*?)\[/(?:code|noparse)\]`)
	bbQuoteRe      = regexp.MustCompile(`(?is)\[quote(?:=[^\]]*)?\](.*?)\[/quote\]`)
	bbHRRe         = regexp.MustCompile(`(?is)\[hr\](\s*\[/hr\])?`)
	bbListItemRe   = regexp.MustCompile(`(?is)\[\*\]\s*`)
	bbAnyTagRe     = regexp.MustCompile(`\[/?[a-zA-Z0-9_*=:#"'\s.-]{1,48}\]`)
	multiBlankRe   = regexp.MustCompile(`\n{3,}`)

	pzModIDLineRe     = regexp.MustCompile(`(?im)^\s*(?:mod\s*id|modid)\s*[:：]\s*(.+?)\s*$`)
	pzMapFolderLineRe = regexp.MustCompile(`(?im)^\s*map\s*folder\s*[:：]\s*(.+?)\s*$`)
	steamURLIDRe      = regexp.MustCompile(`(?i)(?:filedetails|workshop)/?\?[^\s"']*?\bid=(\d{5,20})`)
)

// CleanBBCode converts Steam Workshop BBCode descriptions into plain text for summaries and metadata parsing.
func CleanBBCode(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	s := strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
	s = bbImgRe.ReplaceAllString(s, "")
	s = bbHeadingRe.ReplaceAllString(s, "\n$1\n")
	s = bbURLTagRe.ReplaceAllString(s, "$2 ($1)")
	s = bbBareURLTagRe.ReplaceAllString(s, "$1")
	s = bbListItemRe.ReplaceAllString(s, "• ")
	s = bbSimpleTagRe.ReplaceAllString(s, "")
	s = bbAnyTagRe.ReplaceAllString(s, "")
	return normalizeCleanedLines(html.UnescapeString(s))
}

// FormatBBCodeMarkdown converts Steam Workshop BBCode into clean GFM Markdown for rich rendering.
func FormatBBCodeMarkdown(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var stash []string
	keep := func(md string) string {
		tok := fmt.Sprintf("\x00MD%d\x00", len(stash))
		stash = append(stash, md)
		return tok
	}
	s := strings.ReplaceAll(strings.ReplaceAll(raw, "\r\n", "\n"), "\r", "\n")
	s = bbCodeRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := bbCodeRe.FindStringSubmatch(m)
		return keep("\n```\n" + strings.Trim(sub[1], "\n") + "\n```\n")
	})
	s = bbH1Re.ReplaceAllString(s, "\n# $1\n")
	s = bbH2Re.ReplaceAllString(s, "\n## $1\n")
	s = bbH3Re.ReplaceAllString(s, "\n### $1\n")
	s = bbHRRe.ReplaceAllString(s, "\n---\n")
	s = bbImgRe.ReplaceAllStringFunc(s, func(m string) string {
		u := strings.TrimSpace(bbImgRe.FindStringSubmatch(m)[1])
		if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
			return keep("\n![](" + u + ")\n")
		}
		return ""
	})
	s = formatInlineBBCode(s, keep, func(in string) string {
		return expandStash(in, stash)
	})
	s = expandStash(s, stash)
	return normalizeCleanedLines(html.UnescapeString(s))
}

func expandStash(s string, stash []string) string {
	for i := len(stash) - 1; i >= 0; i-- {
		s = strings.ReplaceAll(s, fmt.Sprintf("\x00MD%d\x00", i), stash[i])
	}
	return s
}

func formatInlineBBCode(s string, keep func(string) string, expand func(string) string) string {
	s = bbURLTagRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := bbURLTagRe.FindStringSubmatch(m)
		href := strings.TrimSpace(strings.Trim(sub[1], `"'`))
		cleaned := bbAnyTagRe.ReplaceAllString(sub[2], "")
		if expand != nil {
			cleaned = expand(cleaned)
		}
		label := strings.Join(strings.Fields(cleaned), " ")
		if label == "" {
			label = href
		}
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
			return keep("[" + label + "](" + href + ")")
		}
		return label
	})
	s = bbBareURLTagRe.ReplaceAllString(s, "$1")
	s = bbQuoteRe.ReplaceAllStringFunc(s, func(m string) string {
		inner := strings.TrimSpace(bbQuoteRe.FindStringSubmatch(m)[1])
		if inner == "" {
			return ""
		}
		return "\n> " + strings.ReplaceAll(inner, "\n", "\n> ") + "\n"
	})
	s = bbBoldRe.ReplaceAllString(s, "**$1**")
	s = bbItalicRe.ReplaceAllString(s, "*$1*")
	s = bbStrikeRe.ReplaceAllString(s, "~~$1~~")
	s = bbListItemRe.ReplaceAllString(s, "\n- ")
	s = bbSimpleTagRe.ReplaceAllString(s, "")
	return bbAnyTagRe.ReplaceAllString(s, "")
}

func normalizeCleanedLines(s string) string {
	s = strings.ReplaceAll(s, "\x00", "")
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	s = strings.TrimSpace(strings.Join(lines, "\n"))
	return multiBlankRe.ReplaceAllString(s, "\n\n")
}

// SummarizeText returns a single-line plain summary truncated to maxRunes.
func SummarizeText(cleaned string, maxRunes int) string {
	for _, line := range strings.Split(cleaned, "\n") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "•"))
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "workshop id:") || strings.HasPrefix(lower, "mod id:") || strings.HasPrefix(lower, "map folder:") {
			continue
		}
		runes := []rune(trimmed)
		if maxRunes > 0 && len(runes) > maxRunes {
			return string(runes[:maxRunes]) + "…"
		}
		return trimmed
	}
	return ""
}

// ExtractProjectZomboidMeta extracts Mod IDs and Map Folders from a Steam Workshop description.
func ExtractProjectZomboidMeta(rawDesc string) (modIDs []string, mapFolders []string) {
	cleaned := CleanBBCode(rawDesc)
	for _, match := range pzModIDLineRe.FindAllStringSubmatch(cleaned, -1) {
		for _, token := range strings.FieldsFunc(match[1], func(r rune) bool { return r == ';' || r == ',' }) {
			v := strings.TrimSpace(token)
			if v != "" && !driver.ContainsString(modIDs, v) {
				modIDs = append(modIDs, v)
			}
		}
	}
	for _, match := range pzMapFolderLineRe.FindAllStringSubmatch(cleaned, -1) {
		for _, token := range strings.FieldsFunc(match[1], func(r rune) bool { return r == ';' }) {
			v := strings.TrimSpace(token)
			if v != "" && !strings.EqualFold(v, "Muldraugh, KY") && !driver.ContainsString(mapFolders, v) {
				mapFolders = append(mapFolders, v)
			}
		}
	}
	return modIDs, mapFolders
}
