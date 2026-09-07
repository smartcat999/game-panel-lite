package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func prepareFiles(dir string, options workload.Options) ([]string, error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	for name, content := range options.Files {
		if err := root.MkdirAll(filepath.Dir(name), 0777); err != nil {
			return nil, err
		}
		file, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
		if err != nil {
			return nil, err
		}
		_, err = file.WriteString(content)
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, err
		}
	}
	mounts := options.DataMounts
	if len(mounts) == 0 {
		mounts = []string{"/data"}
	}
	binds := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		relative, target := ".", strings.TrimSpace(mount)
		if source, destination, ok := strings.Cut(mount, ":"); ok {
			relative, target = strings.TrimSpace(source), strings.TrimSpace(destination)
		}
		if !filepath.IsAbs(target) || strings.Contains(target, ":") {
			return nil, fmt.Errorf("invalid data mount target %q", target)
		}
		clean := filepath.Clean(relative)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("invalid data mount source %q", relative)
		}
		if err := rejectSymlink(root, clean); err != nil {
			return nil, err
		}
		if _, err := root.Stat(clean); os.IsNotExist(err) {
			if filepath.Ext(clean) != "" {
				if err := root.MkdirAll(filepath.Dir(clean), 0777); err != nil {
					return nil, err
				}
				file, err := root.OpenFile(clean, os.O_CREATE|os.O_WRONLY, 0666)
				if err != nil {
					return nil, err
				}
				if err := file.Close(); err != nil {
					return nil, err
				}
			} else if err := root.MkdirAll(clean, 0777); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
		binds = append(binds, filepath.Join(dir, clean)+":"+target)
	}
	// Retain existing worker file permissions for images with non-root users.
	if err := normalizePermissions(dir); err != nil {
		return nil, err
	}
	return binds, nil
}
func rejectSymlink(root *os.Root, path string) error {
	current := "."
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("data mount source contains a symlink")
		}
	}
	return nil
}
func normalizePermissions(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return os.Chmod(path, 0777)
		}
		return os.Chmod(path, 0666)
	})
}
