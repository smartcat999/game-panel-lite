package backup

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RestoreDirectory accepts the native ZIP emitted by CreateDirectory and legacy
// archives rooted at the panel's parent directory. Native restores replace the
// cluster, so snapshots created after backup cannot survive a rollback.
func (s *Service) RestoreDirectory(instanceID, fileName, targetDir, directory string) error {
	clean := filepath.Clean(directory)
	if clean == "." || clean == ".." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid native backup directory")
	}
	archivePath, err := s.Path(instanceID, fileName)
	if err != nil {
		return err
	}
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	nativePrefix := filepath.Base(clean) + "/"
	legacyPrefix := filepath.ToSlash(clean) + "/"
	native, old, files := true, false, 0
	for _, entry := range archive.File {
		name := entry.Name
		if strings.Contains(name, "\\") || strings.Contains(name, "..") || strings.HasPrefix(name, "/") || entry.Mode()&os.ModeSymlink != 0 {
			archive.Close()
			return fmt.Errorf("backup contains unsafe path")
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		files++
		native = native && strings.HasPrefix(name, nativePrefix)
		old = old || strings.HasPrefix(name, legacyPrefix)
	}
	archive.Close()
	if files == 0 {
		return fmt.Errorf("backup contains no game files")
	}
	if old {
		return s.Restore(instanceID, fileName, targetDir)
	}
	if !native {
		return fmt.Errorf("backup does not contain the expected native game directory")
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(targetDir, ".gamepanel-native-restore-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := s.Restore(instanceID, fileName, staging); err != nil {
		return err
	}
	root, err := os.OpenRoot(targetDir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(clean), 0o755); err != nil {
		return err
	}
	stageName := filepath.Base(staging)
	previous := filepath.Join(stageName, "previous-save")
	moved := false
	if _, err := root.Lstat(clean); err == nil {
		if err := root.Rename(clean, previous); err != nil {
			return err
		}
		moved = true
	} else if !os.IsNotExist(err) {
		return err
	}
	next := filepath.Join(stageName, filepath.Base(clean))
	if err := root.Rename(next, clean); err != nil {
		if moved {
			if rollbackErr := root.Rename(previous, clean); rollbackErr != nil {
				cleanup = false
				return fmt.Errorf("restore failed: %v; recovery copy retained: %w", err, rollbackErr)
			}
		}
		return err
	}
	return nil
}
