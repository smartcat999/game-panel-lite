package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bsm/redislock"
	"github.com/cenkalti/backoff/v4"
	"github.com/redis/go-redis/v9"

	"github.com/smartcat999/game-panel-cloud/pkg/config"
	_ "github.com/smartcat999/game-panel-cloud/pkg/controller/core"
	_ "github.com/smartcat999/game-panel-cloud/pkg/controller/iam"
	_ "github.com/smartcat999/game-panel-cloud/pkg/controller/registry"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

// Config is an alias for ctrlruntime.Config bound to the "controller" configuration section.
type Config = ctrlruntime.Config

// NewConfig creates a new controller configuration with defaults.
func NewConfig() *Config {
	return ctrlruntime.NewConfig()
}

// Reconciler is an alias for ctrlruntime.Reconciler.
type Reconciler = ctrlruntime.Reconciler

// Register registers a domain controller with the global controller runtime.
func Register(r Reconciler) {
	ctrlruntime.Register(r)
}

type resyncLease interface {
	Refresh(context.Context, time.Duration, *redislock.Options) error
	Release(context.Context) error
}

// Manager coordinates both real-time MQ topic consumers and periodic Resync passes.
type Manager struct {
	reg          *repo.Registry
	cfg          *Config
	mq           mq.Interface
	leaseRedis   *redis.Client
	acquireLease func(context.Context, time.Duration) (resyncLease, error)
	workerSlots  sync.Map // controller name -> shared MQ/resync concurrency limit
	resyncGate   chan struct{}
	pending      map[string][]mq.Event // guarded by resyncGate; preserves unconsumed cursor-page tails
	// lastResyncOK holds the UnixNano time of the last error-free resync pass
	lastResyncOK atomic.Int64
}

func NewManager(reg *repo.Registry, cfg *Config, bus mq.Interface) *Manager {
	if cfg == nil {
		cfg = &config.Get().Controller
	}
	controllerCfg := *cfg
	controllerCfg.Default()
	m := &Manager{reg: reg, cfg: &controllerCfg, mq: bus, resyncGate: make(chan struct{}, 1), pending: make(map[string][]mq.Event)}
	cacheCfg := config.Get().Cache
	if strings.EqualFold(strings.TrimSpace(cacheCfg.Driver), "redis") && strings.TrimSpace(cacheCfg.Addr) != "" {
		rc := redis.NewClient(&redis.Options{
			Addr:                  cacheCfg.Addr,
			Password:              cacheCfg.Password,
			DB:                    cacheCfg.DB,
			DialTimeout:           3 * time.Second,
			ReadTimeout:           3 * time.Second,
			WriteTimeout:          3 * time.Second,
			ContextTimeoutEnabled: true,
		})
		m.leaseRedis = rc
		locker := redislock.New(rc)
		m.acquireLease = func(ctx context.Context, ttl time.Duration) (resyncLease, error) {
			return locker.Obtain(ctx, "gamepanel:controller:resync:leader", ttl, nil)
		}
	}
	return m
}

func (m *Manager) MQ() mq.Interface {
	return m.mq
}

// StartSubscribers subscribes all registered controllers to their dedicated MQ topics,
// routing incoming events directly to each controller's unified Reconcile method.
func (m *Manager) StartSubscribers(ctx context.Context) ([]mq.Subscription, error) {
	list := m.orderedReconcilers()
	var subs []mq.Subscription
	for _, r := range list {
		topic := r.Topic()
		if topic == "" {
			continue
		}
		ctrl := r
		group := "controller-" + ctrl.Name()
		for range m.cfg.Workers {
			sub, err := m.mq.Subscribe(ctx, topic, group, func(evCtx context.Context, ev mq.Event) error {
				return m.runReconcile(evCtx, ctrl, ev)
			})
			if err != nil {
				for _, s := range subs {
					_ = s.Unsubscribe()
				}
				return nil, err
			}
			subs = append(subs, sub)
		}
	}
	return subs, nil
}

// ReconcileEvent synchronously dispatches an Event to the controller subscribed to event.Topic.
func (m *Manager) ReconcileEvent(ctx context.Context, event mq.Event) error {
	for _, r := range m.orderedReconcilers() {
		if r.Topic() == event.Topic {
			if err := m.runReconcile(ctx, r, event); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) invokeReconcilerSafely(ctx context.Context, r Reconciler, ev mq.Event) (err error, panicked bool) {
	defer func() {
		if rec := recover(); rec != nil {
			panicked = true
			log.Error("controller reconcile panicked", "controller", r.Name(), "topic", ev.Topic, "name", ev.Name, "panic", rec)
			err = fmt.Errorf("reconciler %s panicked: %v", r.Name(), rec)
		}
	}()
	return r.Reconcile(ctx, m.reg, m.mq, m.cfg, ev), false
}

func (m *Manager) runReconcile(ctx context.Context, r Reconciler, ev mq.Event) error {
	limit, exists := m.workerSlots.Load(r.Name())
	if !exists {
		limit, _ = m.workerSlots.LoadOrStore(r.Name(), make(chan struct{}, m.cfg.Workers))
	}
	slots := limit.(chan struct{})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case slots <- struct{}{}:
	}
	defer func() { <-slots }()
	if err := ctx.Err(); err != nil {
		return err
	}
	start := time.Now()
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 10 * time.Millisecond
	expBackoff.Multiplier = 2.0
	expBackoff.RandomizationFactor = 0
	expBackoff.MaxInterval = 100 * time.Millisecond
	expBackoff.MaxElapsedTime = 0
	expBackoff.Reset()

	b := backoff.WithContext(backoff.WithMaxRetries(expBackoff, 3), ctx)
	err := backoff.Retry(func() error {
		recErr, panicked := m.invokeReconcilerSafely(ctx, r, ev)
		if recErr == nil {
			return nil
		}
		if panicked {
			return backoff.Permanent(recErr)
		}
		return recErr
	}, b)

	if err == nil {
		metrics.ControllerReconcileDuration.WithLabelValues(r.Name()).Observe(time.Since(start).Seconds())
		metrics.ControllerReconcileTotal.WithLabelValues(r.Name(), "success").Inc()
		return nil
	}

	metrics.ControllerReconcileDuration.WithLabelValues(r.Name()).Observe(time.Since(start).Seconds())
	metrics.ControllerReconcileTotal.WithLabelValues(r.Name(), "error").Inc()
	log.Warn("controller reconcile failed", "controller", r.Name(), "topic", ev.Topic, "tenant", ev.Tenant, "name", ev.Name, "error", err)
	return err
}

// Run starts all MQ topic consumers and runs periodic Resync passes on ResyncPeriod
// until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) error {
	if m.leaseRedis != nil {
		defer func() { _ = m.leaseRedis.Close() }()
	}
	subs, err := m.StartSubscribers(ctx)
	if err != nil {
		return err
	}
	defer func() {
		for _, s := range subs {
			_ = s.Unsubscribe()
		}
	}()

	ticker := time.NewTicker(m.cfg.ResyncPeriod)
	defer ticker.Stop()

	// A failing resync must never take the whole controller-manager down: log it and let the
	// next period (and the MQ consumers) retry.
	m.resyncAndRecord(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			m.resyncAndRecord(ctx)
		}
	}
}

func (m *Manager) resyncAndRecord(ctx context.Context) {
	if err := m.resyncPass(ctx, m.ReconcileAll); err != nil {
		if errors.Is(err, redislock.ErrNotObtained) {
			log.Debug("controller resync leadership is held by another replica")
			return
		}
		log.Warn("controller resync pass had errors", "error", err)
	}
}

func (m *Manager) resyncPass(ctx context.Context, work func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	passCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	lease, stopRenew, renewDone, renewalErrPtr, err := m.startResyncLeaseRenewer(passCtx, cancel)
	if err != nil {
		return err
	}
	workErr := func() (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("controller resync pass panicked: %v", recovered)
			}
		}()
		return work(passCtx)
	}()
	renewErr, releaseErr := m.finishResyncLease(ctx, lease, stopRenew, renewDone, renewalErrPtr)
	if err := errors.Join(workErr, renewErr, releaseErr, context.Cause(passCtx)); err != nil {
		return err
	}
	m.lastResyncOK.Store(time.Now().UnixNano())
	return nil
}

func (m *Manager) startResyncLeaseRenewer(passCtx context.Context, cancel context.CancelCauseFunc) (resyncLease, context.CancelFunc, chan struct{}, *error, error) {
	if m.acquireLease == nil {
		return nil, nil, nil, nil, nil
	}
	opCtx, stop := context.WithTimeout(passCtx, min(m.cfg.ResyncLeaseTTL/3, 3*time.Second))
	lease, err := m.acquireLease(opCtx, m.cfg.ResyncLeaseTTL)
	stop()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("obtain controller resync leadership: %w", err)
	}
	if lease == nil {
		return nil, nil, nil, nil, errors.New("obtain controller resync leadership: empty lease")
	}
	renewCtx, stopRenew := context.WithCancel(passCtx)
	renewDone := make(chan struct{})
	var renewalErr error
	go func() {
		defer close(renewDone)
		ticker := time.NewTicker(m.cfg.ResyncLeaseTTL / 3)
		defer ticker.Stop()
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				refreshCtx, stopRefresh := context.WithTimeout(renewCtx, min(m.cfg.ResyncLeaseTTL/3, 3*time.Second))
				err := lease.Refresh(refreshCtx, m.cfg.ResyncLeaseTTL, nil)
				stopRefresh()
				if err != nil && renewCtx.Err() == nil {
					renewalErr = fmt.Errorf("controller resync leadership renewal failed: %w", err)
					cancel(renewalErr)
					return
				}
			}
		}
	}()
	return lease, stopRenew, renewDone, &renewalErr, nil
}

func (m *Manager) finishResyncLease(ctx context.Context, lease resyncLease, stopRenew context.CancelFunc, renewDone chan struct{}, renewalErrPtr *error) (error, error) {
	if lease == nil {
		return nil, nil
	}
	stopRenew()
	<-renewDone
	if renewalErrPtr != nil && *renewalErrPtr != nil {
		return *renewalErrPtr, nil
	}
	releaseCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), min(m.cfg.ResyncLeaseTTL/3, 3*time.Second))
	releaseErr := lease.Release(releaseCtx)
	stop()
	if releaseErr != nil {
		releaseErr = fmt.Errorf("release controller resync leadership: %w", releaseErr)
	}
	return nil, releaseErr
}

// LastSuccessfulResync reports when a full resync pass last completed without errors.
func (m *Manager) LastSuccessfulResync() time.Time {
	ns := m.lastResyncOK.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// ReconcileAll preserves controller dependency order. Within one controller,
// a bounded worker pool consumes a bounded queue while cursor scans advance.
func (m *Manager) ReconcileAll(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case m.resyncGate <- struct{}{}:
	}
	defer func() { <-m.resyncGate }()
	return m.reconcileControllers(ctx, m.orderedReconcilers())
}

func (m *Manager) reconcileControllers(ctx context.Context, reconcilers []Reconciler) error {
	var errs []error
	for _, r := range reconcilers {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		controllerCtx, cancel := context.WithTimeout(ctx, m.cfg.ResyncTimeout)
		err := m.reconcileController(controllerCtx, r)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s resync: %w", r.Name(), err))
		}
	}
	return errors.Join(errs...)
}

type resyncEventKey struct {
	tenant, name, version, kind, uid string
	typeName                         mq.EventType
}

func eventKey(event mq.Event) resyncEventKey {
	version := event.ResourceVersion
	if version == "" {
		version = event.ID
	}
	return resyncEventKey{event.Tenant, event.Name, version, event.Kind, event.Attributes["uid"], event.Type}
}

type resyncWork struct {
	event mq.Event
	done  bool
	err   error
}

func (m *Manager) scanController(ctx context.Context, r Reconciler) (events []mq.Event, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("controller %s resync panicked: %v", r.Name(), recovered)
		}
	}()
	return r.Resync(ctx, m.reg, m.cfg)
}

func (m *Manager) reconcileController(ctx context.Context, r Reconciler) error {
	jobs, workers := m.startResyncWorkers(ctx, r)
	queued, remainder, scanErr := m.dispatchResyncWork(ctx, r, jobs)
	close(jobs)
	workers.Wait()
	return m.collectResyncResults(ctx, r, queued, remainder, scanErr)
}

func (m *Manager) startResyncWorkers(ctx context.Context, r Reconciler) (chan *resyncWork, *sync.WaitGroup) {
	jobs := make(chan *resyncWork, m.cfg.Workers)
	var workers sync.WaitGroup
	for range m.cfg.Workers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case item, open := <-jobs:
					if !open {
						return
					}
					item.err = m.runReconcile(ctx, r, item.event)
					item.done = true
				}
			}
		}()
	}
	return jobs, &workers
}

func (m *Manager) dispatchResyncWork(ctx context.Context, r Reconciler, jobs chan<- *resyncWork) ([]*resyncWork, []mq.Event, error) {
	events := m.pending[r.Name()]
	delete(m.pending, r.Name())
	var queued []*resyncWork
	var remainder []mq.Event
	var scanErr error
	pages := 0
	seen := make(map[resyncEventKey]struct{})
dispatch:
	for {
		if len(events) == 0 {
			if pages >= m.cfg.ResyncPages || len(queued) >= m.cfg.ResyncEventBudget || ctx.Err() != nil {
				break
			}
			events, scanErr = m.scanController(ctx, r)
			pages++
			if scanErr != nil || len(events) == 0 {
				break
			}
		}
		for index, event := range events {
			key := eventKey(event)
			knownIdentity := event.ResourceVersion != "" || event.ID != ""
			if _, duplicate := seen[key]; knownIdentity && duplicate {
				continue
			}
			if len(queued) >= m.cfg.ResyncEventBudget || ctx.Err() != nil {
				remainder = append([]mq.Event(nil), events[index:]...)
				break dispatch
			}
			if event.Topic == "" {
				event.Topic = r.Topic()
			}
			item := &resyncWork{event: event}
			select {
			case <-ctx.Done():
				remainder = append([]mq.Event(nil), events[index:]...)
				break dispatch
			case jobs <- item:
				queued = append(queued, item)
				if knownIdentity {
					seen[key] = struct{}{}
				}
			}
		}
		events = nil
	}
	return queued, remainder, scanErr
}

func (m *Manager) collectResyncResults(ctx context.Context, r Reconciler, queued []*resyncWork, remainder []mq.Event, scanErr error) error {
	var errs []error
	if scanErr != nil {
		errs = append(errs, scanErr)
	}
	var pending []mq.Event
	for _, item := range queued {
		if !item.done || item.err != nil {
			pending = append(pending, item.event)
		}
		if item.err != nil {
			errs = append(errs, fmt.Errorf("reconcile %s/%s: %w", item.event.Tenant, item.event.Name, item.err))
		}
	}
	pending = append(pending, remainder...)
	if len(pending) > 0 {
		m.pending[r.Name()] = pending
	}
	if err := ctx.Err(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (m *Manager) orderedReconcilers() []Reconciler {
	return ctrlruntime.OrderedReconcilers()
}

// HealthHandler serves /livez, /readyz (storage + MQ reachable) and Prometheus /metrics so the
// controller-manager can be probed and its ControllerReconcile* metrics scraped.
func (m *Manager) HealthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := m.reg.Ping(ctx); err != nil {
			http.Error(w, "storage: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		if m.mq != nil {
			if err := m.mq.Ping(ctx); err != nil {
				http.Error(w, "mq: "+err.Error(), http.StatusServiceUnavailable)
				return
			}
		}
		_, _ = fmt.Fprintf(w, "ok (last clean resync: %s)", m.LastSuccessfulResync().Format(time.RFC3339))
	})
	mux.Handle("GET /metrics", metrics.Handler())
	return mux
}
