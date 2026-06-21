package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type ImageRuntime interface {
	ImageStatus(ctx context.Context, image string) domain.RuntimeImageStatus
	LoadImageArchive(ctx context.Context, path string) error
}

type RuntimeImageLoader struct {
	dataDir string
	runtime ImageRuntime
}

type runtimeInstallMarker struct {
	ProviderKey domain.ProviderKey `json:"providerKey"`
	Version     string             `json:"version"`
	Image       string             `json:"image"`
	ArchivePath string             `json:"archivePath,omitempty"`
	InstalledAt time.Time          `json:"installedAt"`
}

func NewRuntimeImageLoader(dataDir string, runtime ImageRuntime) *RuntimeImageLoader {
	return &RuntimeImageLoader{dataDir: dataDir, runtime: runtime}
}

func (l *RuntimeImageLoader) EnsureImage(ctx context.Context, server domain.GameServer, image string) error {
	if l == nil || l.runtime == nil || strings.TrimSpace(image) == "" {
		return nil
	}
	if status := l.runtime.ImageStatus(ctx, image); status.Status == runtime.ImageStatusReady {
		return nil
	}
	archivePath, err := l.runtimeInstallArchivePath(server.ProviderKey, server.Spec.Version, image)
	if err != nil {
		return err
	}
	if err := l.runtime.LoadImageArchive(ctx, archivePath); err != nil {
		return fmt.Errorf("load server runtime image from local archive: %w", err)
	}
	if status := l.runtime.ImageStatus(ctx, image); status.Status != runtime.ImageStatusReady {
		if status.Message != "" {
			return fmt.Errorf("load server runtime image from local archive: %s", status.Message)
		}
		return fmt.Errorf("load server runtime image from local archive failed")
	}
	return nil
}

func (l *RuntimeImageLoader) runtimeInstallArchivePath(providerKey domain.ProviderKey, version string, image string) (string, error) {
	path := filepath.Join(l.dataDir, "runtime-installs", safeRuntimeInstallPathPart(string(providerKey)), safeRuntimeInstallPathPart(version)+".json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("server runtime is not installed; install it from Game Library first")
	}
	if err != nil {
		return "", err
	}
	var marker runtimeInstallMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return "", err
	}
	if marker.ProviderKey != "" && marker.ProviderKey != providerKey {
		return "", fmt.Errorf("runtime install marker provider mismatch")
	}
	if marker.Version != "" && marker.Version != version {
		return "", fmt.Errorf("runtime install marker version mismatch")
	}
	if marker.Image != "" && marker.Image != image {
		return "", fmt.Errorf("runtime install marker image mismatch")
	}
	archivePath := strings.TrimSpace(marker.ArchivePath)
	if archivePath == "" {
		archivePath = filepath.Join("runtime-images", safeRuntimeInstallPathPart(string(providerKey)), safeRuntimeInstallPathPart(version)+".tar")
	}
	clean := filepath.Clean(archivePath)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("runtime install marker archive path is invalid")
	}
	fullPath := filepath.Join(l.dataDir, clean)
	stat, err := os.Stat(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("runtime image archive is missing")
	}
	if err != nil {
		return "", err
	}
	if stat.IsDir() {
		return "", fmt.Errorf("runtime image archive path is a directory")
	}
	if stat.Size() <= 0 {
		return "", fmt.Errorf("runtime image archive is empty")
	}
	return fullPath, nil
}

func safeRuntimeInstallPathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= 'A' && char <= 'Z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '.', char == '-', char == '_':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	return builder.String()
}
