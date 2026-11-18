package http

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func copyStoredFile(sourcePath string, targetPath string) error {
	if err := ensureRuntimeDataDir(filepath.Dir(targetPath)); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := target.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		return err
	}
	if err := target.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o666); err != nil {
		return err
	}
	return os.Rename(tmpName, targetPath)
}

func removeStoredFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func writeInstanceDataFile(dataDir string, name string, content string) error {
	clean := filepath.Clean(name)
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return fmt.Errorf("invalid instance data file path: %s", name)
	}
	target := filepath.Join(dataDir, clean)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o644)
}

func ensureRuntimeDataDir(path string) error {
	if err := os.MkdirAll(path, 0o777); err != nil {
		return err
	}
	return os.Chmod(path, 0o777)
}

func writeRuntimeDataFile(targetPath string, content []byte) error {
	if err := ensureRuntimeDataDir(filepath.Dir(targetPath)); err != nil {
		return err
	}
	return os.WriteFile(targetPath, content, 0o666)
}
