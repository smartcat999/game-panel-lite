package driver

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

// WalkArchiveEntries visits each regular file entry inside a zip or tar.gz archive.
func WalkArchiveEntries(
	archiveFormat, archivePath string,
	fn func(slashName string, open func() (io.ReadCloser, error)) error,
) error {
	switch archiveFormat {
	case registryv1.ArchiveFormatZip:
		return walkZipEntries(archivePath, fn)
	case registryv1.ArchiveFormatTarGz:
		return walkTarGzEntries(archivePath, fn)
	default:
		return nil
	}
}

func walkZipEntries(zipPath string, fn func(string, func() (io.ReadCloser, error)) error) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		file := f
		if err := fn(filepath.ToSlash(file.Name), file.Open); err != nil {
			return err
		}
	}
	return nil
}

func walkTarGzEntries(tgzPath string, fn func(string, func() (io.ReadCloser, error)) error) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if err := fn(filepath.ToSlash(hdr.Name), func() (io.ReadCloser, error) {
			return io.NopCloser(tr), nil
		}); err != nil {
			return err
		}
	}
}

// ReadLimitedText reads a bounded text payload from open and strips an optional UTF-8 BOM.
func ReadLimitedText(open func() (io.ReadCloser, error), maxBytes int64) (string, bool) {
	rc, err := open()
	if err != nil {
		return "", false
	}
	defer func() { _ = rc.Close() }()
	raw, err := io.ReadAll(io.LimitReader(rc, maxBytes))
	if err != nil {
		return "", false
	}
	return strings.TrimPrefix(string(raw), "\xef\xbb\xbf"), true
}
