package registryworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"
)

// Option customizes a stateless Harbor-style registry Worker.
type Option func(*Worker)

// WithSteamExecutor overrides the SteamCMD process runner.
func WithSteamExecutor(exec SteamExecutor) Option {
	return func(w *Worker) {
		if exec != nil {
			w.steamExec = exec
		}
	}
}

// WithWorkerID sets an explicit replica claim identity.
func WithWorkerID(id string) Option {
	return func(w *Worker) {
		if id != "" {
			w.workerID = id
		}
	}
}

// Worker polls Pending registry Artifacts via client.RegistryV1(), synchronizes upstream payloads into S3 Blobs,
// and reports completion via ArtifactStatus with zero compute-node or Docker socket coupling.
type Worker struct {
	cfg              *config.Config
	cli              client.Interface
	workerID         string
	steamExec        SteamExecutor
	uploadHC         *http.Client
	uploadRetryDelay time.Duration
}

// New constructs a stateless registry Worker.
func New(cfg *config.Config, cli client.Interface, opts ...Option) *Worker {
	host, _ := os.Hostname()
	if host == "" {
		host = "worker"
	}
	w := &Worker{
		cfg:       cfg,
		cli:       cli,
		workerID:  fmt.Sprintf("%s-%d", host, os.Getpid()),
		steamExec: newSteamExecutor(DefaultSteamExecutorDriver, cfg.Registry),
		uploadHC:  &http.Client{Timeout: 0},
	}
	for _, fn := range opts {
		fn(w)
	}
	return w
}

// WorkDir returns the worker's staging workspace directory.
func (w *Worker) WorkDir() string { return w.cfg.Registry.WorkerWorkDir }

// SteamExec returns the active SteamExecutor configured on the worker.
func (w *Worker) SteamExec() driver.SteamExecutor { return w.steamExec }

// HealthHandler exposes /healthz, /livez, /readyz, and /metrics for standalone container orchestration.
func (w *Worker) HealthHandler() http.Handler {
	mux := http.NewServeMux()
	live := func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(map[string]string{"status": "ok", "worker": w.workerID})
	}
	mux.HandleFunc("GET /healthz", live)
	mux.HandleFunc("GET /livez", live)
	mux.HandleFunc("GET /readyz", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		_, err := w.cli.RegistryV1().Repositories().List(r.Context(), rest.ListOptions{Limit: 1})
		if err != nil {
			rw.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(rw).Encode(map[string]string{"status": "not_ready", "error": err.Error()})
			return
		}
		_ = json.NewEncoder(rw).Encode(map[string]string{"status": "ready", "worker": w.workerID})
	})
	mux.Handle("GET /metrics", metrics.Handler())
	return mux
}

// Run executes the continuous artifact synchronization poll loop until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	if err := os.MkdirAll(w.cfg.Registry.WorkerWorkDir, 0o755); err != nil {
		return fmt.Errorf("init worker workDir: %w", err)
	}
	interval := w.cfg.Registry.WorkerPollInterval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	log.Info("starting registry-worker", "workerID", w.workerID, "workDir", w.cfg.Registry.WorkerWorkDir, "concurrency", w.cfg.Registry.WorkerConcurrency)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := w.SyncOnce(ctx); err != nil && ctx.Err() == nil {
			log.Warn("registry-worker sync pass error", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

const defaultWorkerLeaseTTL = 90 * time.Second

func (w *Worker) leaseTTL() time.Duration {
	if w.cfg != nil && w.cfg.Registry.WorkerLeaseTTL > 0 {
		return w.cfg.Registry.WorkerLeaseTTL
	}
	return defaultWorkerLeaseTTL
}

// SyncOnce claims and processes Pending Artifacts and expired Syncing leases up to WorkerConcurrency in parallel.
func (w *Worker) SyncOnce(ctx context.Context) (int, error) {
	if err := os.MkdirAll(w.cfg.Registry.WorkerWorkDir, 0o755); err != nil {
		return 0, err
	}
	candidates, err := w.listSyncCandidates(ctx)
	if err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return 0, nil
	}
	conc := w.cfg.Registry.WorkerConcurrency
	if conc <= 0 {
		conc = 2
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	var mu sync.Mutex
	processed := 0
	for i := range candidates {
		sem <- struct{}{}
		wg.Add(1)
		go func(candidate registryv1.Artifact) {
			defer wg.Done()
			defer func() { <-sem }()
			if w.claimAndSync(ctx, &candidate) {
				mu.Lock()
				processed++
				mu.Unlock()
			}
		}(candidates[i])
	}
	wg.Wait()
	return processed, nil
}

func (w *Worker) listSyncCandidates(ctx context.Context) ([]registryv1.Artifact, error) {
	pendingList, err := w.cli.RegistryV1().Artifacts().List(ctx, rest.ListOptions{
		FieldSelector: "status.phase=" + string(registryv1.ArtifactPhasePending),
		Limit:         50,
	})
	if err != nil {
		return nil, err
	}
	var out []registryv1.Artifact
	seen := make(map[string]bool)
	for i := range pendingList.Items {
		art := pendingList.Items[i]
		if art.Metadata.DeletionTimestamp == nil && art.Status.Phase == registryv1.ArtifactPhasePending {
			seen[art.Metadata.Name] = true
			out = append(out, art)
		}
	}
	syncingList, err := w.cli.RegistryV1().Artifacts().List(ctx, rest.ListOptions{
		FieldSelector: "status.phase=" + string(registryv1.ArtifactPhaseSyncing),
		Limit:         50,
	})
	if err != nil {
		return out, nil
	}
	now := time.Now().UTC()
	for i := range syncingList.Items {
		art := syncingList.Items[i]
		if !seen[art.Metadata.Name] && art.Metadata.DeletionTimestamp == nil && isExpiredSyncLease(&art, now) {
			seen[art.Metadata.Name] = true
			out = append(out, art)
		}
	}
	return out, nil
}

func isExpiredSyncLease(art *registryv1.Artifact, now time.Time) bool {
	if art == nil || art.Status.Phase != registryv1.ArtifactPhaseSyncing {
		return false
	}
	raw := strings.TrimSpace(art.Metadata.Annotations[registryv1.AnnotationSyncLeaseUntil])
	if raw == "" {
		return true
	}
	until, err := time.Parse(time.RFC3339, raw)
	return err != nil || !now.Before(until)
}

func (w *Worker) claimAndSync(ctx context.Context, candidate *registryv1.Artifact) bool {
	claimed, ok := w.tryClaim(ctx, candidate)
	if !ok {
		return false
	}
	syncCtx, abortSync := w.startLeaseRenewal(ctx, claimed.Metadata.Name)
	defer abortSync(nil)

	rep := newSyncReporter(syncCtx, w.cli, claimed, abortSync)
	rep.ResetLogs()
	rep.Logf("开始同步资源 %s (Worker: %s)", claimed.Metadata.Name, w.workerID)
	rep.Stage(registryv1.ArtifactStageResolving, 2, "正在准备上游同步")
	tmpDir, err := os.MkdirTemp(w.cfg.Registry.WorkerWorkDir, "sync-*")
	if err != nil {
		w.markFailed(ctx, rep, claimed, err)
		return true
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	err = w.processClaimedArtifact(syncCtx, rep, claimed, tmpDir)
	abortSync(nil)
	if err != nil {
		if ctx.Err() != nil {
			w.releaseClaimOnShutdown(ctx, rep, claimed)
			return false
		}
		if errors.Is(context.Cause(syncCtx), driver.ErrArtifactAborted) {
			log.Info("artifact sync aborted", "artifact", claimed.Metadata.Name)
			return false
		}
		w.markFailed(ctx, rep, claimed, err)
		return true
	}
	return true
}

func (w *Worker) tryClaim(ctx context.Context, art *registryv1.Artifact) (*registryv1.Artifact, bool) {
	leaseTTL := w.leaseTTL()
	cp := *art
	cp.Metadata.Annotations = maps.Clone(art.Metadata.Annotations)
	if cp.Metadata.Annotations == nil {
		cp.Metadata.Annotations = make(map[string]string, 2)
	}
	cp.Metadata.Annotations[registryv1.AnnotationSyncClaimedBy] = w.workerID
	cp.Metadata.Annotations[registryv1.AnnotationSyncLeaseUntil] = time.Now().UTC().Add(leaseTTL).Format(time.RFC3339)
	updated, err := w.cli.RegistryV1().Artifacts().Update(ctx, &cp)
	if err != nil {
		if !client.IsConflict(err) && !client.IsNotFound(err) {
			log.Warn("claim artifact failed", "artifact", art.Metadata.Name, "error", err)
		}
		return nil, false
	}
	now := time.Now().UTC()
	updated.Status.Phase = registryv1.ArtifactPhaseSyncing
	updated.Status.Stage = registryv1.ArtifactStageResolving
	updated.Status.Progress = 1
	updated.Status.SpeedBytesPerSec = 0
	updated.Status.StartedAt = &now
	updated.Status.Message = "claimed by " + w.workerID
	withStatus, err := w.cli.RegistryV1().Artifacts().UpdateStatus(ctx, updated)
	if err != nil {
		return updated, true
	}
	return withStatus, true
}

func (w *Worker) startLeaseRenewal(parent context.Context, artName string) (context.Context, context.CancelCauseFunc) {
	leaseTTL := w.leaseTTL()
	ctx, cancel := context.WithCancelCause(parent)
	go func() {
		ticker := time.NewTicker(leaseTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !w.renewLeaseOnce(ctx, artName, leaseTTL, cancel) {
					return
				}
			}
		}
	}()
	return ctx, cancel
}

func (w *Worker) renewLeaseOnce(ctx context.Context, artName string, leaseTTL time.Duration, cancel context.CancelCauseFunc) bool {
	cur, err := w.cli.RegistryV1().Artifacts().Get(ctx, artName)
	if shouldAbortSyncLease(cur, err, w.workerID) {
		cancel(driver.ErrArtifactAborted)
		return false
	}
	if err != nil {
		return true
	}
	cur.Metadata.Annotations = maps.Clone(cur.Metadata.Annotations)
	if cur.Metadata.Annotations == nil {
		cur.Metadata.Annotations = make(map[string]string, 1)
	}
	cur.Metadata.Annotations[registryv1.AnnotationSyncLeaseUntil] = time.Now().UTC().Add(leaseTTL).Format(time.RFC3339)
	if _, err := w.cli.RegistryV1().Artifacts().Update(ctx, cur); client.IsNotFound(err) {
		cancel(driver.ErrArtifactAborted)
		return false
	}
	return true
}

func shouldAbortSyncLease(cur *registryv1.Artifact, err error, workerID string) bool {
	if client.IsNotFound(err) {
		return true
	}
	if err != nil || cur == nil {
		return false
	}
	return cur.Metadata.DeletionTimestamp != nil || cur.Metadata.Annotations[registryv1.AnnotationSyncClaimedBy] != workerID
}

func (w *Worker) releaseClaimOnShutdown(ctx context.Context, rep *syncReporter, art *registryv1.Artifact) {
	relCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	cur, err := w.cli.RegistryV1().Artifacts().Get(relCtx, art.Metadata.Name)
	if err == nil && cur.Metadata.Annotations[registryv1.AnnotationSyncClaimedBy] != w.workerID {
		return
	}
	if err != nil {
		if client.IsNotFound(err) {
			return
		}
		cur = art
	}
	nextAnn := maps.Clone(cur.Metadata.Annotations)
	delete(nextAnn, registryv1.AnnotationSyncClaimedBy)
	delete(nextAnn, registryv1.AnnotationSyncLeaseUntil)
	cur.Metadata.Annotations = nextAnn
	if updated, uErr := w.cli.RegistryV1().Artifacts().Update(relCtx, cur); uErr == nil && updated != nil {
		cur = updated
	}
	cur.Status.Phase = registryv1.ArtifactPhasePending
	cur.Status.Stage = registryv1.ArtifactStageQueued
	cur.Status.SpeedBytesPerSec = 0
	cur.Status.Message = "worker restarting; queued for retry"
	_, _ = w.cli.RegistryV1().Artifacts().UpdateStatus(relCtx, cur)
	if rep != nil {
		rep.PushNotice(relCtx, "Worker 正在重启，任务已重新入队等待接管")
	}
}

func (w *Worker) markFailed(ctx context.Context, rep *syncReporter, art *registryv1.Artifact, syncErr error) {
	log.Warn("artifact sync failed", "artifact", art.Metadata.Name, "error", syncErr)
	statusCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	cur, err := w.cli.RegistryV1().Artifacts().Get(statusCtx, art.Metadata.Name)
	if shouldAbortSyncLease(cur, err, w.workerID) {
		return
	}
	if err != nil {
		cur = art
	}
	cur.Status.ObservedGeneration = cur.Metadata.Generation
	cur.Status.Phase = registryv1.ArtifactPhaseFailed
	cur.Status.Stage = registryv1.ArtifactStageFailed
	cur.Status.SpeedBytesPerSec = 0
	cur.Status.Message = syncErr.Error()
	cur.Status.Conditions = []metav1.Condition{{
		Type:               metav1.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		Reason:             registryv1.ConditionReasonSyncFailed,
		Message:            syncErr.Error(),
		LastTransitionTime: time.Now().UTC(),
	}}
	_, _ = w.cli.RegistryV1().Artifacts().UpdateStatus(statusCtx, cur)
	if rep != nil {
		rep.Finish("[ERROR] " + syncErr.Error())
	}
}
