package registryworker

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

const (
	logFlushInterval   = 350 * time.Millisecond
	logFlushBatchSize  = 16
	statusPushInterval = 500 * time.Millisecond
)

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

type syncReporter struct {
	ctx       context.Context
	cli       client.Interface
	art       *registryv1.Artifact
	startedAt time.Time

	mu             sync.Mutex
	buf            []string
	lastLogFlush   time.Time
	lastStatusPush time.Time
	lastStage      registryv1.ArtifactSyncStage
}

func newSyncReporter(ctx context.Context, cli client.Interface, art *registryv1.Artifact) *syncReporter {
	now := time.Now().UTC()
	if art.Status.StartedAt == nil {
		art.Status.StartedAt = &now
	}
	return &syncReporter{
		ctx:       ctx,
		cli:       cli,
		art:       art,
		startedAt: *art.Status.StartedAt,
	}
}

func (r *syncReporter) ResetLogs() {
	if r == nil || r.cli == nil || r.art == nil {
		return
	}
	chunk := &registryv1.ArtifactLogChunk{Action: registryv1.ArtifactLogActionReset}
	chunk.DefaultMeta()
	_ = r.cli.RegistryV1().Artifacts().PushLogs(r.ctx, r.art.Metadata.Name, chunk)
}

func (r *syncReporter) Logf(format string, args ...any) {
	r.LogLine(fmt.Sprintf(format, args...))
}

func (r *syncReporter) LogLine(raw string) {
	if r == nil || r.cli == nil || r.art == nil {
		return
	}
	cleaned := sanitizeLogLine(raw)
	if cleaned == "" {
		return
	}
	stamped := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), cleaned)
	var batch []string
	r.mu.Lock()
	r.buf = append(r.buf, stamped)
	if len(r.buf) >= logFlushBatchSize || time.Since(r.lastLogFlush) >= logFlushInterval {
		batch = r.takeBatchLocked()
	}
	r.mu.Unlock()
	if len(batch) > 0 {
		r.sendChunk(registryv1.ArtifactLogActionAppend, batch)
	}
}

func (r *syncReporter) Flush() {
	if r == nil || r.cli == nil || r.art == nil {
		return
	}
	r.mu.Lock()
	batch := r.takeBatchLocked()
	r.mu.Unlock()
	if len(batch) > 0 {
		r.sendChunk(registryv1.ArtifactLogActionAppend, batch)
	}
}

func (r *syncReporter) Finish(finalLine string) {
	if finalLine != "" {
		r.LogLine(finalLine)
	}
	if r == nil || r.cli == nil || r.art == nil {
		return
	}
	r.mu.Lock()
	batch := r.takeBatchLocked()
	r.mu.Unlock()
	r.sendChunk(registryv1.ArtifactLogActionComplete, batch)
}

func (r *syncReporter) PushNotice(ctx context.Context, line string) {
	if r == nil || r.cli == nil || r.art == nil {
		return
	}
	r.mu.Lock()
	if cleaned := sanitizeLogLine(line); cleaned != "" {
		r.buf = append(r.buf, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), cleaned))
	}
	batch := r.takeBatchLocked()
	r.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	chunk := &registryv1.ArtifactLogChunk{Action: registryv1.ArtifactLogActionAppend, Lines: batch}
	chunk.DefaultMeta()
	_ = r.cli.RegistryV1().Artifacts().PushLogs(ctx, r.art.Metadata.Name, chunk)
}

func (r *syncReporter) takeBatchLocked() []string {
	if len(r.buf) == 0 {
		return nil
	}
	batch := r.buf
	r.buf = nil
	r.lastLogFlush = time.Now()
	return batch
}

func (r *syncReporter) sendChunk(action registryv1.ArtifactLogAction, lines []string) {
	chunk := &registryv1.ArtifactLogChunk{Action: action, Lines: lines}
	chunk.DefaultMeta()
	_ = r.cli.RegistryV1().Artifacts().PushLogs(r.ctx, r.art.Metadata.Name, chunk)
}

func (r *syncReporter) Stage(stage registryv1.ArtifactSyncStage, pct int32, msg string) {
	if r == nil || r.cli == nil || r.art == nil {
		return
	}
	r.Flush()
	r.pushStatus(stage, pct, r.art.Status.ProcessedBytes, r.art.Status.TotalBytes, 0, msg, true)
}

func (r *syncReporter) Transfer(stage registryv1.ArtifactSyncStage, pct int32, processed, total, speed int64, msg string) {
	if r == nil || r.cli == nil || r.art == nil {
		return
	}
	r.pushStatus(stage, pct, processed, total, speed, msg, false)
}

func (r *syncReporter) pushStatus(stage registryv1.ArtifactSyncStage, pct int32, processed, total, speed int64, msg string, force bool) {
	r.mu.Lock()
	now := time.Now()
	if !force && stage == r.lastStage && now.Sub(r.lastStatusPush) < statusPushInterval {
		r.mu.Unlock()
		return
	}
	r.lastStage = stage
	r.lastStatusPush = now
	r.art.Status.Phase = registryv1.ArtifactPhaseSyncing
	r.art.Status.Stage = stage
	r.art.Status.Progress = pct
	r.art.Status.ProcessedBytes = processed
	r.art.Status.TotalBytes = total
	r.art.Status.SpeedBytesPerSec = speed
	r.art.Status.StartedAt = &r.startedAt
	r.art.Status.Message = msg
	snap := *r.art
	r.mu.Unlock()

	if updated, err := r.cli.RegistryV1().Artifacts().UpdateStatus(r.ctx, &snap); err == nil && updated != nil {
		r.mu.Lock()
		r.art.Metadata.StatusVersion = updated.Metadata.StatusVersion
		r.mu.Unlock()
	}
}

type progressReader struct {
	r        io.Reader
	total    int64
	base     int64
	done     int64
	from, to int32
	speed    *driver.ByteSpeedTracker
	onStep   func(pct int32, done, total, speedBps int64)
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.done += int64(n)
		cur := p.base + p.done
		var spd int64
		if p.speed != nil {
			spd = p.speed.Update(cur)
		}
		if p.onStep != nil && p.total > 0 {
			pct := driver.ScalePercent(cur, p.total, p.from, p.to)
			p.onStep(pct, cur, p.total, spd)
		}
	}
	return n, err
}

func sanitizeLogLine(raw string) string {
	s := ansiEscapeRe.ReplaceAllString(raw, "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", "  ")
	return strings.TrimSpace(s)
}
