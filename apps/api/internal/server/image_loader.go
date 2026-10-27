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
	PrepareImage(ctx context.Context, image string) error
}

type RuntimeImageLoader struct {
	dataDir string
	runtime ImageRuntime
}

type runtimeInstallMarker struct {
	Storage     string             `json:"storage,omitempty"`
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
	archivePath, archiveErr := l.runtimeInstallArchivePath(server.ProviderKey, server.Spec.Version, image)
	if archiveErr != nil {
		archivePath, archiveErr = l.runtimeInstallArchiveForImage(server.ProviderKey, image, archiveErr)
	}
	if archiveErr == nil {
		archiveErr = l.runtime.LoadImageArchive(ctx, archivePath)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if l.runtime.ImageStatus(ctx, image).Status == runtime.ImageStatusReady {
			return nil
		}
	}
	if !l.runtimeImageRegistered(server.ProviderKey, server.Spec.Version, image) {
		if archiveErr != nil {
			return archiveErr
		}
		return fmt.Errorf("server runtime is not installed; install it from Game Library first")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Only registered installations may recover automatically. Explicit uninstall
	// removes the registration, so a start cannot silently reinstall it.
	if err := l.runtime.PrepareImage(ctx, image); err != nil {
		return fmt.Errorf("recover server runtime image: %w", err)
	}
	if status := l.runtime.ImageStatus(ctx, image); status.Status != runtime.ImageStatusReady {
		return fmt.Errorf("runtime image verification failed after recovery")
	}
	return nil
}

// Game-file versions can share one runtime image. After reinstalling the latest
// runtime, use its archive rather than looking for an old game-version archive.
func (l *RuntimeImageLoader) runtimeInstallArchiveForImage(providerKey domain.ProviderKey, image string, originalErr error) (string, error) {
	directory := filepath.Join(l.dataDir, "runtime-installs", safeRuntimeInstallPathPart(string(providerKey)))
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", originalErr
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			continue
		}
		var marker runtimeInstallMarker
		if json.Unmarshal(data, &marker) != nil || marker.Image != image || marker.Version == "" || safeRuntimeInstallPathPart(marker.Version)+".json" != entry.Name() {
			continue
		}
		if path, err := l.runtimeInstallArchivePath(providerKey, marker.Version, image); err == nil {
			return path, nil
		}
	}
	return "", originalErr
}

func (l *RuntimeImageLoader) readRuntimeInstallMarker(providerKey domain.ProviderKey, version, image string) (runtimeInstallMarker, error) {
	path := filepath.Join(l.dataDir, "runtime-installs", safeRuntimeInstallPathPart(string(providerKey)), safeRuntimeInstallPathPart(version)+".json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return runtimeInstallMarker{}, fmt.Errorf("server runtime is not installed; install it from Game Library first")
	}
	if err != nil {
		return runtimeInstallMarker{}, err
	}
	var marker runtimeInstallMarker
	if err = json.Unmarshal(data, &marker); err != nil {
		return marker, err
	}
	if (marker.ProviderKey != "" && marker.ProviderKey != providerKey) || (marker.Version != "" && marker.Version != version) || (marker.Image != "" && marker.Image != image) {
		return marker, fmt.Errorf("runtime install marker identity mismatch")
	}
	if marker.Storage != "" && marker.Storage != "docker" {
		return marker, fmt.Errorf("runtime install marker storage is invalid")
	}
	expected := filepath.Join("runtime-images", safeRuntimeInstallPathPart(string(providerKey)), safeRuntimeInstallPathPart(version)+".tar")
	if marker.ArchivePath != "" && filepath.Clean(marker.ArchivePath) != expected {
		return marker, fmt.Errorf("runtime install marker archive path is invalid")
	}
	return marker, nil
}

func (l *RuntimeImageLoader) runtimeImageRegistered(providerKey domain.ProviderKey, version, image string) bool {
	if m, err := l.readRuntimeInstallMarker(providerKey, version, image); err == nil && m.Image == image {
		return true
	}
	entries, err := os.ReadDir(filepath.Join(l.dataDir, "runtime-installs", safeRuntimeInstallPathPart(string(providerKey))))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		version := strings.TrimSuffix(entry.Name(), ".json")
		if version != safeRuntimeInstallPathPart(version) {
			continue
		}
		m, err := l.readRuntimeInstallMarker(providerKey, version, image)
		if err == nil && m.Image == image {
			return true
		}
	}
	return false
}

func (l *RuntimeImageLoader) runtimeInstallArchivePath(providerKey domain.ProviderKey, version string, image string) (string, error) {
	if _, err := l.readRuntimeInstallMarker(providerKey, version, image); err != nil {
		return "", err
	}
	path := filepath.Join(l.dataDir, "runtime-images", safeRuntimeInstallPathPart(string(providerKey)), safeRuntimeInstallPathPart(version)+".tar")
	stat, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("runtime image archive is missing")
	}
	if err != nil {
		return "", err
	}
	if stat.IsDir() || stat.Size() <= 0 {
		return "", fmt.Errorf("runtime image archive is empty")
	}
	return path, nil
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
