package zomboid

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

const maxPZTextBytes = 512 << 10

var modInfoIDRe = regexp.MustCompile(`(?im)^\s*id\s*=\s*([^\r\n#]+)`)

func init() {
	driver.RegisterWorkshopInspector(addonmirror.SteamAppProjectZomboid, workshopInspector{})
}

type workshopInspector struct{}

func (workshopInspector) InspectDir(contentDir string) driver.WorkshopInspection {
	return parseWorkshopDir(contentDir)
}

func (workshopInspector) InspectArchive(archiveFormat, archivePath string) driver.WorkshopInspection {
	return parseArchive(archiveFormat, archivePath)
}

type pzScanned struct {
	modIDs     []string
	mapFolders []string
	textFiles  []pzRawFile
}

func (s *pzScanned) visitFile(slashRel string, readText func() (string, bool)) {
	low := strings.ToLower(slashRel)
	base := strings.ToLower(filepath.Base(low))
	switch {
	case base == "mod.info":
		if raw, ok := readText(); ok {
			appendUnique(&s.modIDs, extractModInfoIDs(raw)...)
		}
	case isSandboxOptionsRel(low) || translationLangRank(low) > 0:
		if raw, ok := readText(); ok {
			s.textFiles = append(s.textFiles, pzRawFile{path: slashRel, content: raw})
		}
	}
}

func (s *pzScanned) result() driver.WorkshopInspection {
	sort.Slice(s.textFiles, func(i, j int) bool {
		return s.textFiles[i].path < s.textFiles[j].path
	})
	return driver.WorkshopInspection{
		ModIDs:       s.modIDs,
		MapFolders:   s.mapFolders,
		ConfigSchema: parsePZSandboxFiles(s.textFiles),
	}
}

func parseWorkshopDir(root string) driver.WorkshopInspection {
	var scanned pzScanned
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rErr := filepath.Rel(root, path)
		if rErr != nil {
			return nil
		}
		slashRel := filepath.ToSlash(rel)
		if d.IsDir() {
			if mapName := extractMapFolderFromRelPath(slashRel); mapName != "" {
				appendUnique(&scanned.mapFolders, mapName)
			}
			return nil
		}
		scanned.visitFile(slashRel, func() (string, bool) {
			return driver.ReadLimitedText(func() (io.ReadCloser, error) { return os.Open(path) }, maxPZTextBytes)
		})
		return nil
	})
	return scanned.result()
}

func parseArchive(archiveFormat, archivePath string) driver.WorkshopInspection {
	var scanned pzScanned
	_ = driver.WalkArchiveEntries(archiveFormat, archivePath, func(slashName string, open func() (io.ReadCloser, error)) error {
		if mapName := extractMapFolderFromRelPath(slashName); mapName != "" {
			appendUnique(&scanned.mapFolders, mapName)
		}
		scanned.visitFile(slashName, func() (string, bool) {
			return driver.ReadLimitedText(open, maxPZTextBytes)
		})
		return nil
	})
	return scanned.result()
}

func extractModInfoIDs(content string) []string {
	var out []string
	for _, m := range modInfoIDRe.FindAllStringSubmatch(content, -1) {
		if len(m) == 2 {
			id := strings.TrimSpace(m[1])
			if id != "" && !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}

func extractMapFolderFromRelPath(slashRel string) string {
	parts := strings.Split(slashRel, "/")
	for i := 0; i+2 < len(parts); i++ {
		if strings.EqualFold(parts[i], "media") && strings.EqualFold(parts[i+1], "maps") {
			candidate := strings.TrimSpace(parts[i+2])
			if candidate != "" && !strings.Contains(candidate, ".") {
				return candidate
			}
		}
	}
	return ""
}

func appendUnique(dst *[]string, vals ...string) {
	for _, v := range vals {
		if v != "" && !slices.Contains(*dst, v) {
			*dst = append(*dst, v)
		}
	}
}
