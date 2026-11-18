package l4d2

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

func init() {
	driver.RegisterWorkshopInspector(addonmirror.SteamAppLeft4Dead2, workshopInspector{})
}

type workshopInspector struct{}

type scannedMaps struct {
	missions []vpkTextFile
	bsps     []string
}

func (workshopInspector) InspectDir(contentDir string) driver.WorkshopInspection {
	var scanned scannedMaps
	_ = filepath.WalkDir(contentDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, rErr := filepath.Rel(contentDir, p)
		if rErr != nil {
			return nil
		}
		slashRel := filepath.ToSlash(rel)
		if strings.HasSuffix(strings.ToLower(d.Name()), ".vpk") {
			if missions, bsps, mErr := extractVPKComponentsFromFile(p); mErr == nil {
				scanned.add(missions, bsps)
			}
			return nil
		}
		scanned.visitLooseEntry(slashRel, func() (io.ReadCloser, error) { return os.Open(p) })
		return nil
	})
	return driver.WorkshopInspection{MapFolders: scanned.result()}
}

func (workshopInspector) InspectArchive(archiveFormat, archivePath string) driver.WorkshopInspection {
	if archiveFormat != registryv1.ArchiveFormatZip && archiveFormat != registryv1.ArchiveFormatTarGz {
		maps, err := extractVPKMapsFromFile(archivePath)
		if err != nil {
			return driver.WorkshopInspection{}
		}
		return driver.WorkshopInspection{MapFolders: maps}
	}
	var scanned scannedMaps
	_ = driver.WalkArchiveEntries(archiveFormat, archivePath, func(slashName string, open func() (io.ReadCloser, error)) error {
		if strings.HasSuffix(strings.ToLower(slashName), ".vpk") {
			if missions, bsps, err := inspectVPKStreamEntry(open); err == nil {
				scanned.add(missions, bsps)
			}
			return nil
		}
		scanned.visitLooseEntry(slashName, open)
		return nil
	})
	return driver.WorkshopInspection{MapFolders: scanned.result()}
}

func inspectVPKStreamEntry(open func() (io.ReadCloser, error)) ([]vpkTextFile, []string, error) {
	rc, err := open()
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rc.Close() }()
	return extractVPKComponentsFromStream(rc)
}

func (s *scannedMaps) add(missions []vpkTextFile, bsps []string) {
	s.missions = append(s.missions, missions...)
	appendUniqueMaps(&s.bsps, bsps...)
}

func (s *scannedMaps) visitLooseEntry(slashRel string, open func() (io.ReadCloser, error)) {
	norm := normalizeLooseMissionOrBSP(slashRel)
	switch {
	case isL4D2MissionTextRel(norm):
		if body, ok := driver.ReadLimitedText(open, maxVPKMissionBytes); ok {
			s.missions = append(s.missions, vpkTextFile{relPath: slashRel, content: body})
		}
	case extractBSPMapStem(norm) != "":
		appendUniqueMaps(&s.bsps, extractBSPMapStem(norm))
	}
}

func (s *scannedMaps) result() []string {
	return resolveVPKMaps(s.missions, s.bsps)
}

func normalizeLooseMissionOrBSP(slashRel string) string {
	clean := strings.TrimLeft(strings.ReplaceAll(slashRel, "\\", "/"), "./")
	parts := strings.Split(clean, "/")
	for i := len(parts) - 2; i >= 0; i-- {
		if strings.EqualFold(parts[i], "missions") || strings.EqualFold(parts[i], "maps") {
			return path.Join(strings.ToLower(parts[i]), parts[i+1])
		}
	}
	return clean
}

func appendUniqueMaps(dst *[]string, vals ...string) {
	for _, v := range vals {
		if v != "" && !slices.Contains(*dst, v) {
			*dst = append(*dst, v)
		}
	}
}
