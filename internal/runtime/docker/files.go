package docker

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type plannedMount struct{ source, target string }

func prepareFiles(dir string, options workload.Options) ([]string, error) {
	return prepareFilesAndCommit(dir, options, nil)
}

// The caller holds the per-instance creation lock. Commit runs only after all
// files and mounts are prepared; a definite failure restores prior files.
func prepareFilesAndCommit(dir string, options workload.Options, commit func([]string) error) (binds []string, err error) {
	names := make([]string, 0, len(options.Files))
	contents := map[string]string{}
	for name, content := range options.Files {
		clean := filepath.Clean(name)
		if !filepath.IsLocal(name) || clean == "." || strings.Contains(name, "\\") || strings.HasPrefix(clean, ".gamepanel-prepare-") {
			return nil, fmt.Errorf("invalid configuration path %q", name)
		}
		if _, exists := contents[clean]; exists {
			return nil, fmt.Errorf("duplicate configuration path %q", clean)
		}
		names = append(names, clean)
		contents[clean] = content
	}
	sort.Strings(names)
	for _, name := range names {
		for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
			if _, exists := contents[parent]; exists {
				return nil, fmt.Errorf("configuration path conflicts with file %q", parent)
			}
		}
	}
	mounts := options.DataMounts
	if len(mounts) == 0 {
		mounts = []string{"/data"}
	}
	planned := make([]plannedMount, 0, len(mounts))
	targets := map[string]bool{}
	for _, mount := range mounts {
		relative, target := ".", strings.TrimSpace(mount)
		if source, destination, ok := strings.Cut(mount, ":"); ok {
			relative, target = strings.TrimSpace(source), strings.TrimSpace(destination)
		}
		if !filepath.IsAbs(target) || strings.Contains(target, ":") {
			return nil, fmt.Errorf("invalid data mount target %q", target)
		}
		clean := filepath.Clean(relative)
		if !filepath.IsLocal(relative) || strings.Contains(relative, "\\") || strings.HasPrefix(clean, ".gamepanel-prepare-") {
			return nil, fmt.Errorf("invalid data mount source %q", relative)
		}
		target = filepath.Clean(target)
		if targets[target] {
			return nil, fmt.Errorf("duplicate data mount target %q", target)
		}
		targets[target] = true
		planned = append(planned, plannedMount{clean, target})
	}
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := checkPreparationRecovery(root); err != nil {
		return nil, err
	}

	for _, name := range names {
		if err := rejectSymlink(root, name); err != nil {
			return nil, err
		}
	}
	for _, mount := range planned {
		if err := rejectSymlink(root, mount.source); err != nil {
			return nil, err
		}
	}
	tx, err := newFileTransaction(root)
	if err != nil {
		return nil, err
	}
	defer func() { err = tx.finish(err) }()
	for _, name := range names {
		if err := tx.stageFile(name, contents[name]); err != nil {
			return nil, err
		}
	}
	for i := range tx.files {
		if err := tx.install(i); err != nil {
			return nil, err
		}
	}
	for _, mount := range planned {
		if _, err := root.Stat(mount.source); os.IsNotExist(err) {
			if filepath.Ext(mount.source) != "" {
				if err := tx.stageFile(mount.source, ""); err != nil {
					return nil, err
				}
				if err := tx.install(len(tx.files) - 1); err != nil {
					return nil, err
				}
			} else if err := tx.mkdir(mount.source); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
		binds = append(binds, filepath.Join(dir, mount.source)+":"+mount.target)
	}
	if err := tx.normalizePermissions(); err != nil {
		return nil, err
	}
	if commit != nil {
		if err := commit(binds); err != nil {
			return nil, err
		}
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

// Missing instance directories are valid for containers without local data.
// Existing directories must be confined and free of pending file transactions.
func checkInstanceRecovery(dataDir, serverID string) error {
	root, err := os.OpenRoot(dataDir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := rejectSymlink(root, serverID); err != nil {
		return err
	}
	instance, err := root.OpenRoot(serverID)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer instance.Close()
	return checkPreparationRecovery(instance)
}

func checkPreparationRecovery(root *os.Root) error {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".gamepanel-prepare-") {
			return fmt.Errorf("configuration recovery must be resolved before retry: %s", entry.Name())
		}
	}
	return nil
}
