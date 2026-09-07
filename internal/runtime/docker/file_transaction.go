package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

var errCreationUncertain = errors.New("container creation outcome is uncertain")

type preparedFile struct {
	name, staged, original string
	installed              bool
}
type changedMode struct {
	name string
	mode fs.FileMode
}
type fileTransaction struct {
	root  *os.Root
	stage string
	files []preparedFile
	dirs  []string
	modes []changedMode
}

func newFileTransaction(root *os.Root) (*fileTransaction, error) {
	stage := ".gamepanel-prepare-" + uuid.NewString()
	if err := root.Mkdir(stage, 0700); err != nil {
		return nil, err
	}
	return &fileTransaction{root: root, stage: stage}, nil
}
func (tx *fileTransaction) stageFile(name, content string) error {
	staged := filepath.Join(tx.stage, strconv.Itoa(len(tx.files))+".new")
	file, err := tx.root.OpenFile(staged, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(content)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}

	metadata, err := tx.root.OpenFile(filepath.Join(tx.stage, strconv.Itoa(len(tx.files))+".path.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	encodeErr := json.NewEncoder(metadata).Encode(name)
	if err := errors.Join(encodeErr, metadata.Close()); err != nil {
		return err
	}
	tx.files = append(tx.files, preparedFile{name: name, staged: staged})
	return nil
}
func (tx *fileTransaction) mkdir(path string) error {
	current := "."
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := tx.root.Lstat(current)
		if os.IsNotExist(err) {
			if err := tx.root.Mkdir(current, 0777); err != nil {
				return err
			}
			tx.dirs = append(tx.dirs, current)
		} else if err != nil {
			return err
		} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("configuration parent is not a directory: %s", current)
		}
	}
	return nil
}
func (tx *fileTransaction) install(index int) error {
	change := &tx.files[index]
	if err := tx.mkdir(filepath.Dir(change.name)); err != nil {
		return err
	}
	info, err := tx.root.Lstat(change.name)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("configuration target is not a regular file: %s", change.name)
		}
		original := filepath.Join(tx.stage, strconv.Itoa(index)+".old")
		if err := tx.root.Rename(change.name, original); err != nil {
			return err
		}
		change.original = original
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := tx.root.Rename(change.staged, change.name); err != nil {
		return err
	}
	change.installed = true
	return nil
}
func (tx *fileTransaction) normalizePermissions() error {
	return fs.WalkDir(tx.root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == tx.stage {
			return fs.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := tx.root.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		mode := fs.FileMode(0666)
		if info.IsDir() {
			mode = 0777
		}
		if info.Mode().Perm() == mode {
			return nil
		}
		tx.modes = append(tx.modes, changedMode{path, info.Mode()})
		return tx.root.Chmod(path, mode)
	})
}
func (tx *fileTransaction) finish(cause error) error {
	if errors.Is(cause, errCreationUncertain) {
		return fmt.Errorf("%w; recovery files retained in %s", cause, tx.stage)
	}
	if cause == nil {
		// Configuration and container are committed. Cleanup failure must not undo
		// files that a running container may now be using.
		if err := tx.root.RemoveAll(tx.stage); err != nil {
			return fmt.Errorf("configuration committed; recovery cleanup failed at %s: %w", tx.stage, err)
		}
		return nil
	}
	var failures []error
	for i := len(tx.modes) - 1; i >= 0; i-- {
		item := tx.modes[i]
		if err := tx.root.Chmod(item.name, item.mode); err != nil {
			failures = append(failures, err)
		}
	}
	for i := len(tx.files) - 1; i >= 0; i-- {
		item := tx.files[i]
		if item.installed {
			if err := tx.root.Remove(item.name); err != nil && !os.IsNotExist(err) {
				failures = append(failures, err)
				continue
			}
		}
		if item.original != "" {
			if err := tx.root.Rename(item.original, item.name); err != nil {
				failures = append(failures, err)
			}
		}
	}
	for i := len(tx.dirs) - 1; i >= 0; i-- {
		if err := tx.root.Remove(tx.dirs[i]); err != nil && !os.IsNotExist(err) {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return errors.Join(cause, fmt.Errorf("configuration rollback incomplete; recovery files retained in %s: %w", tx.stage, errors.Join(failures...)))
	}
	return errors.Join(cause, tx.root.RemoveAll(tx.stage))
}
