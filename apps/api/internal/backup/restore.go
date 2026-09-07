package backup

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// ErrCommit identifies failure of the caller's configuration/persistence step.
var ErrCommit = errors.New("restored configuration commit failed")

// RestoreHooks keeps originals available until the caller commits restored state.
// The caller must leave its state unchanged when Commit returns an error.
type RestoreHooks struct {
	Validate func(Metadata) error
	Commit   func() error
}

type restoredFile struct {
	name, staged, original string
	published              bool
}

// restoreFiles preserves original files until all archive reads and CRC checks
// finish. Callers must stop the game and serialize mutations. Rollback handles
// returned errors, not process crashes or concurrent external filesystem writes.
func restoreFiles(root *os.Root, files []*zip.File, commit func() error) error {
	names := map[string]bool{}
	var entries []*zip.File
	for _, file := range files {
		if file.Name == metadataPath {
			continue
		}
		if strings.Contains(file.Name, "..") || filepath.IsAbs(file.Name) || !filepath.IsLocal(file.Name) {
			return fmt.Errorf("backup contains unsafe path")
		}
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("backup contains symbolic link")
		}
		if file.FileInfo().IsDir() {
			continue
		}
		name := filepath.Clean(file.Name)
		if names[name] {
			return fmt.Errorf("backup contains duplicate path %q", name)
		}
		names[name] = true
		entries = append(entries, file)
	}
	stage := ".gamepanel-restore-" + uuid.NewString()
	if err := root.Mkdir(stage, 0700); err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = root.RemoveAll(stage)
		}
	}()
	changes := make([]restoredFile, 0, len(entries))
	for index, entry := range entries {
		staged := filepath.Join(stage, strconv.Itoa(index)+".new")
		out, err := root.OpenFile(staged, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		in, err := entry.Open()
		if err != nil {
			_ = out.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := errors.Join(in.Close(), out.Close())
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
		changes = append(changes, restoredFile{name: filepath.Clean(entry.Name), staged: staged})
	}
	fail := func(cause error) error {
		var rollbackErrors []error
		for index := len(changes) - 1; index >= 0; index-- {
			change := changes[index]
			if change.published {
				if err := root.Remove(change.name); err != nil {
					rollbackErrors = append(rollbackErrors, err)
					continue
				}
			}
			if change.original != "" {
				if err := root.Rename(change.original, change.name); err != nil {
					rollbackErrors = append(rollbackErrors, err)
				}
			}
		}
		if len(rollbackErrors) > 0 {
			cleanup = false
			return errors.Join(cause, fmt.Errorf("restore rollback incomplete; recovery files retained in %s: %w", stage, errors.Join(rollbackErrors...)))
		}
		return cause
	}
	for index := range changes {
		change := &changes[index]
		if err := checkRestoreParents(root, change.name); err != nil {
			return fail(err)
		}
		if err := root.MkdirAll(filepath.Dir(change.name), 0755); err != nil {
			return fail(err)
		}
		info, err := root.Lstat(change.name)
		if err == nil {
			if !info.Mode().IsRegular() {
				return fail(fmt.Errorf("restore target %q is not a regular file", change.name))
			}
			original := filepath.Join(stage, strconv.Itoa(index)+".old")
			if err := root.Rename(change.name, original); err != nil {
				return fail(err)
			}
			change.original = original
		} else if !os.IsNotExist(err) {
			return fail(err)
		}
		if err := root.Rename(change.staged, change.name); err != nil {
			return fail(err)
		}
		change.published = true
	}
	if commit != nil {
		if err := commit(); err != nil {
			return fail(fmt.Errorf("%w: %w", ErrCommit, err))
		}
	}
	return nil
}

func checkRestoreParents(root *os.Root, name string) error {
	current := "."
	for _, part := range strings.Split(filepath.Dir(name), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("restore parent %q is not a directory", current)
		}
	}
	return nil
}
