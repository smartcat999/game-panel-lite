package modruntime

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

// Install stages every provider-owned destination before replacing files. Each
// rename is atomic; multiple renames and database writes are not a transaction.
// The caller owns source and serializes mutations for this instance.
func (s *Service) Install(ctx context.Context, key domain.ProviderKey, filename, directory string, source io.ReadSeeker) error {
	paths, err := s.paths(key, filename)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	if strings.TrimSpace(directory) == "" {
		return fmt.Errorf("server data dir is empty")
	}
	if err = os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	staged := make([]string, 0, len(paths))
	defer func() {
		for _, name := range staged {
			_ = root.Remove(name)
		}
	}()
	for _, path := range paths {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = root.MkdirAll(filepath.Dir(path), 0777); err != nil {
			return err
		}
		if err = root.Chmod(filepath.Dir(path), 0777); err != nil {
			return err
		}
		name := filepath.Join(filepath.Dir(path), ".mod-"+uuid.NewString()+".tmp")
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		staged = append(staged, name)
		// Preserve runtime image compatibility with the existing writable mod files.
		if err = file.Chmod(0666); err != nil {
			_ = file.Close()
			return err
		}
		if _, err = source.Seek(0, io.SeekStart); err == nil {
			_, err = io.Copy(file, contextReader{ctx, source})
		}
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	for i, path := range paths {
		if err = root.Rename(staged[i], path); err != nil {
			return err
		}
	}
	return nil
}

// Remove removes only provider-owned paths inside directory. Missing files and
// directories are idempotent; symlink escapes return errors.
func (s *Service) Remove(ctx context.Context, key domain.ProviderKey, filename, directory string) error {
	paths, err := s.paths(key, filename)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	if strings.TrimSpace(directory) == "" {
		return fmt.Errorf("server data dir is empty")
	}
	root, err := os.OpenRoot(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	for _, path := range paths {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = root.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
