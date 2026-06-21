package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

func (a *Adapter) ImageStatus(ctx context.Context, image string) domain.RuntimeImageStatus {
	image = strings.TrimSpace(image)
	if image == "" {
		return domain.RuntimeImageStatus{Status: runtime.ImageStatusFailed, Message: "image is empty", UpdatedAt: time.Now()}
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, _, err := a.client.ImageInspectWithRaw(ctx, image); err == nil {
		return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusReady, UpdatedAt: time.Now()}
	} else if client.IsErrNotFound(err) {
		return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusMissing, UpdatedAt: time.Now()}
	} else {
		return domain.RuntimeImageStatus{Image: image, Status: runtime.ImageStatusFailed, Message: err.Error(), UpdatedAt: time.Now()}
	}
}

func (a *Adapter) PrepareImage(ctx context.Context, image string) error {
	return a.ensureImage(ctx, image)
}

func (a *Adapter) PrepareImageWithProgress(ctx context.Context, image string, onProgress runtime.ImagePrepareProgressFunc) error {
	return a.ensureImageWithProgress(ctx, image, onProgress)
}

func (a *Adapter) SaveImageArchive(ctx context.Context, image string, path string) error {
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("image is empty")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("image archive path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	reader, err := a.client.ImageSave(ctx, []string{image})
	if err != nil {
		return err
	}
	defer reader.Close()
	tempPath := path + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tempPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return closeErr
	}
	return os.Rename(tempPath, path)
}

func (a *Adapter) LoadImageArchive(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	response, err := a.client.ImageLoad(ctx, file, true)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return nil
}

func (a *Adapter) ensureImage(ctx context.Context, image string) error {
	return a.ensureImageWithProgress(ctx, image, nil)
}

func (a *Adapter) ensureImageWithProgress(ctx context.Context, image string, onProgress runtime.ImagePrepareProgressFunc) error {
	if _, _, err := a.client.ImageInspectWithRaw(ctx, image); err == nil {
		if onProgress != nil {
			onProgress(runtime.ImagePrepareProgress{Message: "image already installed", Progress: 100})
		}
		return nil
	}
	pull, err := a.client.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return imagePullError(image, err)
	}
	defer pull.Close()
	if err := consumeImagePullWithProgress(pull, onProgress); err != nil {
		return imagePullError(image, err)
	}
	return nil
}

func imagePullError(image string, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if isDSTImage(image) && isImageAccessDenied(message) {
		return fmt.Errorf("DST runtime image %s is not available from the registry. Build and load it on this Docker host with scripts/build-game-images.sh dst --platform linux/amd64 --load, or push it to a registry this host can pull from. Original error: %w", image, err)
	}
	return err
}

func isDSTImage(image string) bool {
	return strings.Contains(image, "dst-server:")
}

func isImageAccessDenied(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "pull access denied") ||
		strings.Contains(message, "repository does not exist") ||
		strings.Contains(message, "requested access to the resource is denied")
}

type imagePullEvent struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Error       string `json:"error"`
	ErrorDetail struct {
		Message string `json:"message"`
	} `json:"errorDetail"`
	ProgressDetail struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"progressDetail"`
}

type imagePullLayerProgress struct {
	current int64
	total   int64
}

func consumeImagePull(reader io.Reader) error {
	return consumeImagePullWithProgress(reader, nil)
}

func consumeImagePullWithProgress(reader io.Reader, onProgress runtime.ImagePrepareProgressFunc) error {
	decoder := json.NewDecoder(reader)
	layers := map[string]imagePullLayerProgress{}
	maxProgress := 0
	for {
		var event imagePullEvent
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				if onProgress != nil {
					onProgress(runtime.ImagePrepareProgress{Message: "image ready", Progress: 100})
				}
				return nil
			}
			return err
		}
		if message := strings.TrimSpace(event.ErrorDetail.Message); message != "" {
			return fmt.Errorf("image pull failed: %s", message)
		}
		if message := strings.TrimSpace(event.Error); message != "" {
			return fmt.Errorf("image pull failed: %s", message)
		}
		if onProgress == nil {
			continue
		}
		message := strings.TrimSpace(event.Status)
		if event.ID != "" {
			message = strings.TrimSpace(strings.Join([]string{event.ID, event.Status}, " "))
		}
		if event.ProgressDetail.Total > 0 && event.ID != "" {
			layers[event.ID] = imagePullLayerProgress{
				current: event.ProgressDetail.Current,
				total:   event.ProgressDetail.Total,
			}
			progress := aggregateImagePullProgress(layers)
			if progress < maxProgress {
				progress = maxProgress
			} else {
				maxProgress = progress
			}
			onProgress(runtime.ImagePrepareProgress{Message: message, Progress: progress})
			continue
		}
		if message != "" {
			onProgress(runtime.ImagePrepareProgress{Message: message, Progress: maxProgress})
		}
	}
}

func aggregateImagePullProgress(layers map[string]imagePullLayerProgress) int {
	var current int64
	var total int64
	for _, layer := range layers {
		if layer.total <= 0 {
			continue
		}
		layerCurrent := layer.current
		if layerCurrent < 0 {
			layerCurrent = 0
		}
		if layerCurrent > layer.total {
			layerCurrent = layer.total
		}
		current += layerCurrent
		total += layer.total
	}
	if total <= 0 {
		return 0
	}
	progress := int(float64(current) / float64(total) * 100)
	if progress <= 0 && current > 0 {
		return 1
	}
	if progress >= 100 {
		return 99
	}
	return progress
}
