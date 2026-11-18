package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

func wantsNDJSONStream(r *http.Request) bool {
	return r != nil && strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/x-ndjson")
}

type taskStreamWriter struct {
	mu     sync.Mutex
	w      http.ResponseWriter
	f      http.Flusher
	active bool
}

func newTaskStreamWriter(w http.ResponseWriter, r *http.Request) *taskStreamWriter {
	ts := &taskStreamWriter{w: w}
	if !wantsNDJSONStream(r) {
		return ts
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		ts.f = f
		f.Flush()
	}
	ts.active = true
	return ts
}

func (t *taskStreamWriter) emit(ev corev1.InstanceTaskStreamEvent) {
	if !t.active {
		return
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = t.w.Write(append(raw, '\n'))
	if t.f != nil {
		t.f.Flush()
	}
}

func (t *taskStreamWriter) log(stage, msg string) {
	t.emit(corev1.InstanceTaskStreamEvent{Type: corev1.TaskStreamEventLog, Stage: stage, Message: msg})
}

func (t *taskStreamWriter) progress(stage string, pct int32, processed, total, speed int64, msg string) {
	t.emit(corev1.InstanceTaskStreamEvent{
		Type:             corev1.TaskStreamEventProgress,
		Stage:            stage,
		Progress:         pct,
		ProcessedBytes:   processed,
		TotalBytes:       total,
		SpeedBytesPerSec: speed,
		Message:          msg,
	})
}

func (t *taskStreamWriter) result(res *corev1.InstanceFiles) {
	if t.active {
		t.emit(corev1.InstanceTaskStreamEvent{Type: corev1.TaskStreamEventResult, Stage: corev1.BackupStageCompleted, Progress: 100, Result: res})
		return
	}
	t.w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(t.w).Encode(res)
}

func (t *taskStreamWriter) fail(status int, msg string) {
	if t.active {
		t.emit(corev1.InstanceTaskStreamEvent{Type: corev1.TaskStreamEventError, Stage: corev1.BackupStageFailed, Message: msg})
		return
	}
	writeJSONError(t.w, status, msg)
}

type progressReader struct {
	r         io.Reader
	read      int64
	total     int64
	startedAt time.Time
	lastEmit  time.Time
	onTick    func(processed, total, speed int64)
}

func newProgressReader(r io.Reader, total int64, onTick func(processed, total, speed int64)) *progressReader {
	now := time.Now()
	return &progressReader{r: r, total: total, startedAt: now, lastEmit: now, onTick: onTick}
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.read += int64(n)
	}
	if p.onTick != nil && (err == io.EOF || time.Since(p.lastEmit) >= 300*time.Millisecond) {
		p.lastEmit = time.Now()
		elapsed := p.lastEmit.Sub(p.startedAt).Seconds()
		var speed int64
		if elapsed > 0.05 {
			speed = int64(float64(p.read) / elapsed)
		}
		p.onTick(p.read, p.total, speed)
	}
	return n, err
}

func computeTransferPct(base, span int32, processed, total int64) int32 {
	if total <= 0 {
		return base
	}
	pct := base + int32(float64(processed)/float64(total)*float64(span))
	if pct > base+span {
		return base + span
	}
	return pct
}
