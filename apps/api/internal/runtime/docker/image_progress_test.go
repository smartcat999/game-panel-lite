package docker

import (
	"bytes"
	"context"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"testing"
	"time"
)

func TestImagePullTrackerKeepsDownloadSpeedSeparateFromExtraction(t *testing.T) {
	tracker := imagePullTracker{layers: map[string]*imagePullLayerProgress{}}
	at := time.Unix(0, 0)
	e := imagePullEvent{ID: "a", Status: "Downloading"}
	e.ProgressDetail.Current = 100
	e.ProgressDetail.Total = 200
	tracker.update(e, at)
	e.ProgressDetail.Current = 200
	got := tracker.update(e, at.Add(time.Second))
	if got.CurrentBytes != 200 || got.BytesPerSecond != 100 || got.TotalBytes != 0 {
		t.Fatalf("incorrect measured download: %+v", got)
	}
	e.Status = "Extracting"
	e.ProgressDetail.Current = 10
	e.ProgressDetail.Total = 1000
	got = tracker.update(e, at.Add(2*time.Second))
	if got.Phase != "extracting" || got.CurrentBytes != 10 || got.BytesPerSecond != 0 {
		t.Fatalf("extraction reused download: %+v", got)
	}
	e.ID = "b"
	e.Status = "Downloading"
	e.ProgressDetail.Current = 50
	e.ProgressDetail.Total = 9000
	got = tracker.update(e, at.Add(3*time.Second))
	if got.CurrentBytes != 250 || got.Progress != 0 {
		t.Fatalf("late layer corrupted download: %+v", got)
	}
}

func TestImagePullTrackerIgnoresRepositoryIDAndCountsCachedLayers(t *testing.T) {
	tracker := imagePullTracker{layers: map[string]*imagePullLayerProgress{}}
	tracker.update(imagePullEvent{ID: "v3", Status: "Pulling from library/game"}, time.Now())
	tracker.update(imagePullEvent{ID: "a", Status: "Already exists"}, time.Now())
	got := tracker.update(imagePullEvent{ID: "b", Status: "Pull complete"}, time.Now())
	if len(tracker.layers) != 2 || got.CompletedLayers != 2 || got.Phase != "extracting" || got.CurrentBytes != 0 {
		t.Fatalf("cached layers: %+v", got)
	}
}

func TestArchiveWriterCountsWritesAndHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer
	var got runtime.ImagePrepareProgress
	writer := imageArchiveWriter{ctx: ctx, writer: &buf, onProgress: func(p runtime.ImagePrepareProgress) { got = p }}
	if _, err := writer.Write([]byte("archive")); err != nil {
		t.Fatal(err)
	}
	if got.Phase != "saving" || got.CurrentBytes != 7 || got.TotalBytes != 0 || got.Progress != 0 {
		t.Fatalf("archive progress: %+v", got)
	}
	cancel()
	if n, err := writer.Write([]byte("more")); n != 0 || err == nil {
		t.Fatal("canceled archive copy continued")
	}
}
