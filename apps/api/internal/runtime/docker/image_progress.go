package docker

import (
	"context"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"io"
	"time"
)

type imagePullLayerProgress struct {
	downloaded, downloadTotal, extracted int64
	downloadedAll, complete, extracting  bool
}

type imagePullTracker struct {
	layers              map[string]*imagePullLayerProgress
	sampledAt           time.Time
	sampledBytes, speed int64
}

// Docker discovers layers incrementally and mixes download and extraction events.
// Their byte counters must stay separate; the stream does not establish a fixed
// whole-image download denominator, so no percentage or guessed total is emitted.
func (t *imagePullTracker) update(event imagePullEvent, now time.Time) runtime.ImagePrepareProgress {
	layerEvent := event.Status == "Pulling fs layer" || event.Status == "Waiting" || event.Status == "Downloading" || event.Status == "Download complete" || event.Status == "Extracting" || event.Status == "Pull complete" || event.Status == "Already exists"
	if event.ID != "" && layerEvent {
		layer := t.layers[event.ID]
		if layer == nil {
			layer = &imagePullLayerProgress{}
			t.layers[event.ID] = layer
		}
		switch event.Status {
		case "Downloading":
			layer.downloaded = max(layer.downloaded, event.ProgressDetail.Current)
			layer.downloadTotal = max(layer.downloadTotal, event.ProgressDetail.Total)
		case "Download complete":
			layer.downloadedAll = true
			layer.downloaded = max(layer.downloaded, layer.downloadTotal)
		case "Extracting":
			layer.downloaded = max(layer.downloaded, layer.downloadTotal)
			layer.downloadedAll = true
			layer.extracting = true
			layer.extracted = max(layer.extracted, event.ProgressDetail.Current)
		case "Pull complete", "Already exists":
			layer.downloaded = max(layer.downloaded, layer.downloadTotal)
			layer.downloadedAll = true
			layer.complete = true
		}
	}
	var downloaded, extracted int64
	var completed int
	var pending, extracting bool
	for _, layer := range t.layers {
		downloaded += layer.downloaded
		extracted += layer.extracted
		pending = pending || !layer.downloadedAll
		extracting = extracting || (layer.extracting && !layer.complete)
		if layer.complete {
			completed++
		}
	}
	if t.sampledAt.IsZero() {
		t.sampledAt = now
		t.sampledBytes = downloaded
	}
	if elapsed := now.Sub(t.sampledAt); elapsed >= 250*time.Millisecond {
		t.speed = int64(float64(max(int64(0), downloaded-t.sampledBytes)) / elapsed.Seconds())
		t.sampledAt, t.sampledBytes = now, downloaded
	}
	p := runtime.ImagePrepareProgress{Phase: "resolving", CompletedLayers: completed}
	switch {
	case pending:
		p.Phase = "downloading"
		p.CurrentBytes = downloaded
		p.BytesPerSecond = t.speed
	case extracting:
		p.Phase = "extracting"
		p.CurrentBytes = extracted
	case len(t.layers) > 0:
		// Individual layer completion does not finish the pull. Only EOF
		// advances to finalizing, after all layers have been processed.
		p.Phase = "extracting"
	}
	return p
}

// Archive size is not the same as image size (tar metadata and shared layers).
// Count successful writes instead of inventing a denominator from image inspect.
type imageArchiveWriter struct {
	ctx        context.Context
	writer     io.Writer
	onProgress runtime.ImagePrepareProgressFunc
	written    int64
	lastReport time.Time
}

func (w *imageArchiveWriter) Write(data []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := w.writer.Write(data)
	w.written += int64(n)
	if w.onProgress != nil && time.Since(w.lastReport) >= 250*time.Millisecond {
		w.onProgress(runtime.ImagePrepareProgress{Phase: "saving", CurrentBytes: w.written})
		w.lastReport = time.Now()
	}
	return n, err
}
