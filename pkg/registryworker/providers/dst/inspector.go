package dst

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

const (
	maxDSTModInfoBytes   = 512 << 10
	maxDSTLegacyZipBytes = 64 << 20
)

func init() {
	driver.RegisterWorkshopInspector(addonmirror.SteamAppDontStarveTogether, workshopInspector{})
}

type workshopInspector struct{}

type dstModInfoCandidate struct {
	base    string
	content string
}

func (workshopInspector) InspectDir(contentDir string) driver.WorkshopInspection {
	cands, zipPaths := scanDSTDir(contentDir)
	if len(cands) == 0 {
		for _, zp := range zipPaths {
			cands = append(cands, scanDSTArchive(registryv1.ArchiveFormatZip, zp)...)
		}
	}
	return driver.WorkshopInspection{
		ConfigSchema: selectBestDSTConfigSchema(cands),
	}
}

func (workshopInspector) InspectArchive(archiveFormat, archivePath string) driver.WorkshopInspection {
	cands := scanDSTArchive(archiveFormat, archivePath)
	return driver.WorkshopInspection{
		ConfigSchema: selectBestDSTConfigSchema(cands),
	}
}

func scanDSTDir(root string) ([]dstModInfoCandidate, []string) {
	var cands []dstModInfoCandidate
	var zips []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		base := strings.ToLower(d.Name())
		if isDSTModInfoFile(base) {
			if txt, ok := driver.ReadLimitedText(func() (io.ReadCloser, error) { return os.Open(p) }, maxDSTModInfoBytes); ok {
				cands = append(cands, dstModInfoCandidate{base: base, content: txt})
			}
			return nil
		}
		if (strings.HasSuffix(base, ".zip") || strings.HasSuffix(base, "_legacy.bin")) && isBoundedFile(d) {
			zips = append(zips, p)
		}
		return nil
	})
	return cands, zips
}

const maxDSTNestedArchives = 8

func scanDSTArchive(archiveFormat, archivePath string) []dstModInfoCandidate {
	return scanDSTArchiveDepth(archiveFormat, archivePath, true)
}

func scanDSTArchiveDepth(archiveFormat, archivePath string, allowNested bool) []dstModInfoCandidate {
	var cands []dstModInfoCandidate
	var nested []string
	var tmpDir string
	defer func() {
		if tmpDir != "" {
			_ = os.RemoveAll(tmpDir)
		}
	}()
	_ = driver.WalkArchiveEntries(archiveFormat, archivePath, func(slashName string, open func() (io.ReadCloser, error)) error {
		base := strings.ToLower(filepath.Base(slashName))
		if isDSTModInfoFile(base) {
			if txt, ok := driver.ReadLimitedText(open, maxDSTModInfoBytes); ok {
				cands = append(cands, dstModInfoCandidate{base: base, content: txt})
			}
			return nil
		}
		if allowNested && isDSTNestedZipEntry(base) && len(nested) < maxDSTNestedArchives {
			if zp := spillNestedDSTZip(&tmpDir, open); zp != "" {
				nested = append(nested, zp)
			}
		}
		return nil
	})
	if len(cands) == 0 {
		for _, zp := range nested {
			cands = append(cands, scanDSTArchiveDepth(registryv1.ArchiveFormatZip, zp, false)...)
		}
	}
	return cands
}

func isDSTNestedZipEntry(base string) bool {
	return strings.HasSuffix(base, ".zip") || strings.HasSuffix(base, "_legacy.bin")
}

func spillNestedDSTZip(tmpDir *string, open func() (io.ReadCloser, error)) string {
	if *tmpDir == "" {
		dir, err := os.MkdirTemp("", "dst-nested-*")
		if err != nil {
			return ""
		}
		*tmpDir = dir
	}
	rc, err := open()
	if err != nil {
		return ""
	}
	defer func() { _ = rc.Close() }()
	f, err := os.CreateTemp(*tmpDir, "nested-*.zip")
	if err != nil {
		return ""
	}
	n, err := io.Copy(f, io.LimitReader(rc, maxDSTLegacyZipBytes+1))
	_ = f.Close()
	if err != nil || n == 0 || n > maxDSTLegacyZipBytes {
		_ = os.Remove(f.Name())
		return ""
	}
	return f.Name()
}

func isDSTModInfoFile(base string) bool {
	return base == "modinfo_chs.lua" || base == "modinfo_zh.lua" || base == "modinfo.lua"
}

func isBoundedFile(d fs.DirEntry) bool {
	info, err := d.Info()
	return err == nil && info.Size() <= maxDSTLegacyZipBytes
}

func selectBestDSTConfigSchema(cands []dstModInfoCandidate) []registryv1.AddonConfigField {
	for _, pref := range []string{"modinfo_chs.lua", "modinfo_zh.lua", "modinfo.lua"} {
		for _, c := range cands {
			if c.base != pref {
				continue
			}
			if schema := parseDSTModInfoSchema(c.content); len(schema) > 0 {
				return schema
			}
		}
	}
	return nil
}
