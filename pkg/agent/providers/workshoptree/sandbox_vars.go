package workshoptree

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/agent/driver"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

func writePZSandboxVarsConfigs(root *os.Root, serverRelDir string, iniRelFiles []string, configs []pzModConfig) error {
	topLevel, sections := collectPZSandboxOverrides(configs)
	if len(topLevel) == 0 && len(sections) == 0 {
		return nil
	}
	for _, iniRel := range iniRelFiles {
		base := strings.TrimSuffix(path.Base(iniRel), path.Ext(iniRel))
		if base == "" {
			continue
		}
		varsRelPath := serverRelDir + "/" + base + "_SandboxVars.lua"
		rawBytes, err := volume.ReadOptionalFileInRoot(root, varsRelPath)
		if err != nil {
			return err
		}
		updated := patchPZSandboxVarsLua(string(rawBytes), topLevel, sections)
		if err := volume.WriteFileAtomicInRoot(root, varsRelPath, []byte(updated), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func collectPZSandboxOverrides(configs []pzModConfig) (map[string]string, map[string]map[string]string) {
	topLevel := make(map[string]string)
	sections := make(map[string]map[string]string)
	for _, cfg := range configs {
		for k, v := range cfg.config {
			lit := driver.FormatLuaScalar(k, v, cfg.schema)
			if sec, key, ok := strings.Cut(k, "."); ok && sec != "" && key != "" {
				if sections[sec] == nil {
					sections[sec] = make(map[string]string)
				}
				sections[sec][key] = lit
			} else if k != "" {
				topLevel[k] = lit
			}
		}
	}
	return topLevel, sections
}

func patchPZSandboxVarsLua(raw string, topLevel map[string]string, sections map[string]map[string]string) string {
	out := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.Contains(out, "SandboxVars") {
		out = "SandboxVars = {\n  VERSION = 5,\n}\n"
	}
	out = applyTopLevelSandboxKeys(out, topLevel)
	secNames := make([]string, 0, len(sections))
	for name := range sections {
		secNames = append(secNames, name)
	}
	sort.Strings(secNames)
	for _, sec := range secNames {
		out = upsertLuaSubtable(out, sec, sections[sec])
	}
	return out
}

func applyTopLevelSandboxKeys(src string, kv map[string]string) string {
	if len(kv) == 0 {
		return src
	}
	keys := sortedKeys(kv)
	out := src
	for _, k := range keys {
		re := regexp.MustCompile(`(?m)^([ \t]*)` + regexp.QuoteMeta(k) + `\s*=\s*[^,\n]+,?`)
		if re.MatchString(out) {
			out = re.ReplaceAllString(out, "${1}"+k+" = "+kv[k]+",")
		} else {
			out = insertBeforeClosingTableBrace(out, fmt.Sprintf("  %s = %s,\n", k, kv[k]))
		}
	}
	return out
}

func upsertLuaSubtable(src, secName string, entries map[string]string) string {
	re := regexp.MustCompile(`(?m)^([ \t]*)` + regexp.QuoteMeta(secName) + `\s*=\s*\{`)
	loc := re.FindStringIndex(src)
	if len(loc) != 2 {
		return insertBeforeClosingTableBrace(src, renderLuaSubtable("  ", secName, entries))
	}
	openIdx := loc[1] - 1
	closeIdx, ok := findMatchingBrace(src, openIdx)
	if !ok {
		return src
	}
	inner := src[openIdx+1 : closeIdx]
	for _, k := range sortedKeys(entries) {
		keyRe := regexp.MustCompile(`(?m)^([ \t]*)` + regexp.QuoteMeta(k) + `\s*=\s*[^,\n]+,?`)
		if keyRe.MatchString(inner) {
			inner = keyRe.ReplaceAllString(inner, "${1}"+k+" = "+entries[k]+",")
		} else {
			if !strings.HasSuffix(inner, "\n") {
				inner += "\n"
			}
			inner += fmt.Sprintf("    %s = %s,\n  ", k, entries[k])
		}
	}
	return src[:openIdx+1] + inner + src[closeIdx:]
}

func renderLuaSubtable(indent, name string, entries map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s%s = {\n", indent, name)
	for _, k := range sortedKeys(entries) {
		fmt.Fprintf(&b, "%s  %s = %s,\n", indent, k, entries[k])
	}
	fmt.Fprintf(&b, "%s},\n", indent)
	return b.String()
}

func insertBeforeClosingTableBrace(src, block string) string {
	idx := strings.LastIndexByte(src, '}')
	if idx < 0 {
		return src + "\n" + block
	}
	prefix := src[:idx]
	if prefix != "" && !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	return prefix + block + src[idx:]
}

func findMatchingBrace(src string, openIdx int) (int, bool) {
	depth := 0
	for i := openIdx; i < len(src); i++ {
		if src[i] == '"' || src[i] == '\'' {
			q := src[i]
			for i++; i < len(src) && src[i] != q; i++ {
				if src[i] == '\\' {
					i++
				}
			}
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

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
