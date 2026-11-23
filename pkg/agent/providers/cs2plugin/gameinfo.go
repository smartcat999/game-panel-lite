package cs2plugin

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/volume"
)

const (
	gameInfoRelPath  = "game/csgo/gameinfo.gi"
	metamodSearchVal = "csgo/addons/metamod"
)

func patchGameInfoMetamod(root *os.Root) error {
	if root == nil {
		return nil
	}
	raw, err := volume.ReadOptionalFileInRoot(root, gameInfoRelPath)
	if err != nil || len(raw) == 0 {
		return err
	}
	patched, changed := ensureMetamodInGameInfo(string(raw))
	if !changed {
		return nil
	}
	return volume.WriteFileAtomicInRoot(root, gameInfoRelPath, []byte(patched), 0o644)
}

func unpatchGameInfoMetamod(root *os.Root) error {
	if root == nil {
		return nil
	}
	raw, err := volume.ReadOptionalFileInRoot(root, gameInfoRelPath)
	if err != nil || len(raw) == 0 {
		return err
	}
	cleaned, changed := removeMetamodFromGameInfo(string(raw))
	if !changed {
		return nil
	}
	return volume.WriteFileAtomicInRoot(root, gameInfoRelPath, []byte(cleaned), 0o644)
}

func ensureMetamodInGameInfo(content string) (string, bool) {
	if strings.Contains(strings.ToLower(content), metamodSearchVal) {
		return content, false
	}
	eol := detectLineEnding(content)
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if idx := findGameLowViolenceLine(lines); idx >= 0 {
		indent := leadingWhitespace(lines[idx])
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:idx+1]...)
		out = append(out, indent+"Game\t"+metamodSearchVal)
		out = append(out, lines[idx+1:]...)
		return strings.Join(out, eol), true
	}
	if idx := findSearchPathsOpenBrace(lines); idx >= 0 {
		indent := leadingWhitespace(lines[idx]) + "\t"
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:idx+1]...)
		out = append(out, indent+"Game\t"+metamodSearchVal)
		out = append(out, lines[idx+1:]...)
		return strings.Join(out, eol), true
	}
	return content, false
}

func removeMetamodFromGameInfo(content string) (string, bool) {
	if !strings.Contains(strings.ToLower(content), metamodSearchVal) {
		return content, false
	}
	eol := detectLineEnding(content)
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	changed := false
	for _, ln := range lines {
		fields := strings.Fields(strings.ToLower(ln))
		if len(fields) == 2 && fields[0] == "game" && fields[1] == metamodSearchVal {
			changed = true
			continue
		}
		out = append(out, ln)
	}
	if !changed {
		return content, false
	}
	return strings.Join(out, eol), true
}

func findGameLowViolenceLine(lines []string) int {
	for i, ln := range lines {
		fields := strings.Fields(strings.ToLower(ln))
		if len(fields) >= 2 && fields[0] == "game_lowviolence" && fields[1] == "csgo_lv" {
			return i
		}
	}
	return -1
}

func findSearchPathsOpenBrace(lines []string) int {
	for i, ln := range lines {
		if strings.EqualFold(strings.TrimSpace(ln), "searchpaths") {
			for j := i + 1; j < len(lines) && j <= i+3; j++ {
				if strings.TrimSpace(lines[j]) == "{" {
					return j
				}
			}
		}
	}
	return -1
}

func detectLineEnding(s string) string {
	if strings.Contains(s, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func leadingWhitespace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[:i]
}

func hasMetamodOnDisk(root *os.Root) bool {
	if root == nil {
		return false
	}
	info, err := root.Stat(filepath.FromSlash("game/csgo/addons/metamod/bin"))
	return err == nil && info.IsDir()
}
