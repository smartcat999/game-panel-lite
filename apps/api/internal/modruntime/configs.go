package modruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/safety"
)

const MaxConfigBytes = 1 << 20

var ErrInvalidConfig = errors.New("invalid mod configuration")

var ErrConfigConflict = errors.New("mod configuration is unavailable")

type ConfigFile struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"sizeBytes"`
	UpdatedAt time.Time `json:"updatedAt"`
	Content   string    `json:"content,omitempty"`
}

// Configuration operations require an authorized server snapshot. The caller
// serializes mutations and checks maintenance; this module owns format and IO.
func (s *Service) ListConfigs(ctx context.Context, server domain.GameServer) ([]ConfigFile, error) {
	root, err := s.configRoot(ctx, server, false)
	if errors.Is(err, os.ErrNotExist) {
		return []ConfigFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	items := make([]ConfigFile, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name, err := configName(entry.Name())
		if err != nil || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > MaxConfigBytes {
			continue
		}
		items = append(items, ConfigFile{Name: name, SizeBytes: info.Size(), UpdatedAt: info.ModTime()})
	}
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name) })
	return items, nil
}

func (s *Service) ReadConfig(ctx context.Context, server domain.GameServer, rawName string) (ConfigFile, error) {
	name, err := configName(rawName)
	if err != nil {
		return ConfigFile{}, err
	}
	root, err := s.configRoot(ctx, server, false)
	if err != nil {
		return ConfigFile{}, err
	}
	defer root.Close()
	if _, err := regularConfig(root, name); err != nil {
		return ConfigFile{}, err
	}
	file, err := root.Open(name)
	if err != nil {
		return ConfigFile{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return ConfigFile{}, err
	}
	if !info.Mode().IsRegular() {
		return ConfigFile{}, fmt.Errorf("%w: invalid mod config file", ErrInvalidConfig)
	}
	content, err := io.ReadAll(io.LimitReader(file, MaxConfigBytes+1))
	if err != nil {
		return ConfigFile{}, err
	}
	if len(content) > MaxConfigBytes || !json.Valid(content) {
		return ConfigFile{}, fmt.Errorf("%w: invalid mod config JSON or size", ErrInvalidConfig)
	}
	if err := ctx.Err(); err != nil {
		return ConfigFile{}, err
	}
	return ConfigFile{Name: name, SizeBytes: int64(len(content)), UpdatedAt: info.ModTime(), Content: string(content)}, nil
}

func (s *Service) WriteConfig(ctx context.Context, server domain.GameServer, rawName string, content []byte) (ConfigFile, error) {
	name, err := configName(rawName)
	if err != nil {
		return ConfigFile{}, err
	}
	if len(content) == 0 || len(content) > MaxConfigBytes {
		return ConfigFile{}, fmt.Errorf("%w: mod config must be between 1 byte and 1 MiB", ErrInvalidConfig)
	}
	var object map[string]any
	if err := json.Unmarshal(content, &object); err != nil || object == nil {
		return ConfigFile{}, fmt.Errorf("%w: mod config must contain a valid JSON object", ErrInvalidConfig)
	}
	root, err := s.configRoot(ctx, server, true)
	if err != nil {
		return ConfigFile{}, err
	}
	defer root.Close()
	if _, err := regularConfig(root, name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ConfigFile{}, err
	}
	temporary := ".mod-config-" + uuid.NewString() + ".tmp"
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ConfigFile{}, err
	}
	defer root.Remove(temporary)
	if _, err = file.Write(content); err == nil {
		err = file.Chmod(0666)
	}
	info, statErr := file.Stat()
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return ConfigFile{}, err
	}
	if statErr != nil {
		return ConfigFile{}, statErr
	}
	if err := ctx.Err(); err != nil {
		return ConfigFile{}, err
	}
	if err := root.Rename(temporary, name); err != nil {
		return ConfigFile{}, err
	}
	return ConfigFile{Name: name, SizeBytes: int64(len(content)), UpdatedAt: info.ModTime(), Content: string(content)}, nil
}

func (s *Service) DeleteConfig(ctx context.Context, server domain.GameServer, rawName string) (string, error) {
	name, err := configName(rawName)
	if err != nil {
		return "", err
	}
	root, err := s.configRoot(ctx, server, true)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if _, err := regularConfig(root, name); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return name, root.Remove(name)
}

func configName(raw string) (string, error) {
	name, err := safety.SafeFileName(strings.TrimSpace(raw), ".json")
	if err != nil {
		return "", fmt.Errorf("%w: invalid mod config name: %v", ErrInvalidConfig, err)
	}
	return name, nil
}
func regularConfig(root *os.Root, name string) (os.FileInfo, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: invalid mod config file", ErrInvalidConfig)
	}
	return info, nil
}

func (s *Service) configRoot(ctx context.Context, server domain.GameServer, mutate bool) (*os.Root, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	item, ok := s.providers.Get(server.ProviderKey)
	if !ok {
		return nil, fmt.Errorf("%w: unknown provider", provider.ErrUnsupported)
	}
	layout, ok := item.(provider.JSONModConfigProvider)
	if !ok {
		return nil, fmt.Errorf("%w: JSON mod configs", provider.ErrUnsupported)
	}
	if err := provider.CheckConfigVersion(item, server.Spec.ConfigVersion); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfigConflict, err)
	}
	if mutate {
		switch server.Status.Phase {
		case domain.PhasePending, domain.PhaseReconciling, domain.PhaseDeleting:
			return nil, fmt.Errorf("%w: server lifecycle action already in progress", ErrConfigConflict)
		}
	}
	directory := layout.JSONModConfigDirectory()
	if !filepath.IsLocal(directory) || filepath.Clean(directory) == "." || strings.Contains(directory, "\\") {
		return nil, fmt.Errorf("%w: invalid provider mod config directory", ErrConfigConflict)
	}
	dataDir := strings.TrimSpace(server.Spec.Runtime.DataDir)
	if dataDir == "" {
		return nil, fmt.Errorf("%w: server data directory is not ready", ErrConfigConflict)
	}
	if mutate {
		if err := os.MkdirAll(dataDir, 0777); err != nil {
			return nil, err
		}
	}
	root, err := os.OpenRoot(dataDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	// Reject links in each existing component before creating missing directories.
	current := "."
	for _, part := range strings.Split(filepath.Clean(directory), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: invalid mod config directory", ErrConfigConflict)
		}
	}
	if mutate {
		if err := root.MkdirAll(directory, 0777); err != nil {
			return nil, err
		}
	}
	configs, err := root.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	if mutate {
		if err := configs.Chmod(".", 0777); err != nil {
			configs.Close()
			return nil, err
		}
	}
	return configs, nil
}
