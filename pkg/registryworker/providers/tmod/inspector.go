package tmod

import (
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

func init() {
	driver.RegisterWorkshopInspector(addonmirror.SteamAppTModLoader, workshopInspector{})
}

type workshopInspector struct{}

type tmodEntry struct {
	rel  string
	stem string
}

func (workshopInspector) InspectDir(contentDir string) driver.WorkshopInspection {
	var entries []tmodEntry
	_ = filepath.WalkDir(contentDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".tmod") {
			return nil
		}
		rel, _ := filepath.Rel(contentDir, p)
		stem := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		if stem != "" {
			entries = append(entries, tmodEntry{rel: filepath.ToSlash(rel), stem: stem})
		}
		return nil
	})
	return driver.WorkshopInspection{ModIDs: sortedUniqueStems(entries)}
}

func (workshopInspector) InspectArchive(archiveFormat, archivePath string) driver.WorkshopInspection {
	var entries []tmodEntry
	_ = driver.WalkArchiveEntries(archiveFormat, archivePath, func(slashName string, _ func() (io.ReadCloser, error)) error {
		base := filepath.Base(slashName)
		if strings.HasSuffix(strings.ToLower(base), ".tmod") {
			stem := strings.TrimSuffix(base, filepath.Ext(base))
			if stem != "" {
				entries = append(entries, tmodEntry{rel: slashName, stem: stem})
			}
		}
		return nil
	})
	return driver.WorkshopInspection{ModIDs: sortedUniqueStems(entries)}
}

func sortedUniqueStems(entries []tmodEntry) []string {
	sort.Slice(entries, func(i, j int) bool {
		return compareVersionDirDesc(filepath.ToSlash(filepath.Dir(entries[i].rel)), filepath.ToSlash(filepath.Dir(entries[j].rel))) < 0
	})
	var stems []string
	for _, e := range entries {
		if !slices.Contains(stems, e.stem) {
			stems = append(stems, e.stem)
		}
	}
	return stems
}

func compareVersionDirDesc(a, b string) int {
	pa := strings.Split(strings.Trim(a, "."), ".")
	pb := strings.Split(strings.Trim(b, "."), ".")
	limit := max(len(pa), len(pb))
	for i := range limit {
		var va, vb int
		if i < len(pa) {
			va, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			vb, _ = strconv.Atoi(pb[i])
		}
		if va != vb {
			if va > vb {
				return -1
			}
			return 1
		}
	}
	return strings.Compare(b, a)
}
