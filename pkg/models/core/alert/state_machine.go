package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

const (
	stateKeyPrefix       = "alert:state:"
	legacyStateKeyPrefix = "alert:active:"
	stateEntryTTL        = 48 * time.Hour

	defaultKeepFiringDuration = 60 * time.Second
	defaultRepeatInterval     = 4 * time.Hour
	startupGracePeriod        = 2 * time.Minute
)

// kvStore abstracts the small subset of cache.Interface needed by the state machine and deduplicator,
// enabling deterministic unit tests without requiring a live Redis server.
type kvStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, val string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

var overrideStateStore kvStore

func activeStateStore() kvStore {
	if overrideStateStore != nil {
		return overrideStateStore
	}
	if s := cache.Default(); s != nil {
		return s
	}
	return nil
}

type alertKeyState struct {
	State          string    `json:"state"`
	PendingSince   time.Time `json:"pendingSince,omitempty"`
	FiringSince    time.Time `json:"firingSince,omitempty"`
	RecoverSince   time.Time `json:"recoverSince,omitempty"`
	LastNotifiedAt time.Time `json:"lastNotifiedAt,omitempty"`
}

type transitionSpec struct {
	forDuration    time.Duration
	keepFiringFor  time.Duration
	repeatInterval time.Duration
}

func evaluateTransition(alertKey string, breached, recovered bool, spec transitionSpec, now time.Time) string {
	store := activeStateStore()
	if store == nil {
		if breached && spec.forDuration <= 0 {
			return corev1.AlertStateFiring
		}
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	prev := loadAlertKeyState(ctx, store, alertKey, now)
	next, transition := stepAlertState(prev, breached, recovered, spec, now)
	saveAlertKeyState(ctx, store, alertKey, next)
	return transition
}

func loadAlertKeyState(ctx context.Context, store kvStore, alertKey string, now time.Time) alertKeyState {
	if raw, err := store.Get(ctx, stateKeyPrefix+alertKey); err == nil && strings.TrimSpace(raw) != "" {
		var st alertKeyState
		if json.Unmarshal([]byte(raw), &st) == nil && st.State != "" {
			return st
		}
	}
	if legacy, err := store.Get(ctx, legacyStateKeyPrefix+alertKey); err == nil && legacy == "1" {
		_ = store.Delete(ctx, legacyStateKeyPrefix+alertKey)
		return alertKeyState{State: corev1.AlertStateFiring, FiringSince: now, LastNotifiedAt: now}
	}
	return alertKeyState{State: corev1.AlertStateNormal}
}

func saveAlertKeyState(ctx context.Context, store kvStore, alertKey string, st alertKeyState) {
	if st.State == "" || st.State == corev1.AlertStateNormal {
		_ = store.Delete(ctx, stateKeyPrefix+alertKey)
		_ = store.Delete(ctx, legacyStateKeyPrefix+alertKey)
		return
	}
	if encoded, err := json.Marshal(st); err == nil {
		_ = store.Set(ctx, stateKeyPrefix+alertKey, string(encoded), stateEntryTTL)
	}
}

func stepAlertState(prev alertKeyState, breached, recovered bool, spec transitionSpec, now time.Time) (alertKeyState, string) {
	switch prev.State {
	case corev1.AlertStatePending:
		return stepFromPending(prev, breached, spec, now)
	case corev1.AlertStateFiring:
		return stepFromFiring(prev, breached, recovered, spec, now)
	case corev1.AlertStateRecovering:
		return stepFromRecovering(prev, recovered, spec, now)
	default:
		return stepFromNormal(breached, spec, now)
	}
}

func stepFromNormal(breached bool, spec transitionSpec, now time.Time) (alertKeyState, string) {
	if !breached {
		return alertKeyState{State: corev1.AlertStateNormal}, ""
	}
	if spec.forDuration <= 0 {
		return alertKeyState{
			State:          corev1.AlertStateFiring,
			FiringSince:    now,
			LastNotifiedAt: now,
		}, corev1.AlertStateFiring
	}
	return alertKeyState{State: corev1.AlertStatePending, PendingSince: now}, ""
}

func stepFromPending(prev alertKeyState, breached bool, spec transitionSpec, now time.Time) (alertKeyState, string) {
	if !breached {
		return alertKeyState{State: corev1.AlertStateNormal}, ""
	}
	since := prev.PendingSince
	if since.IsZero() {
		since = now
	}
	if now.Sub(since) >= spec.forDuration {
		return alertKeyState{
			State:          corev1.AlertStateFiring,
			PendingSince:   since,
			FiringSince:    now,
			LastNotifiedAt: now,
		}, corev1.AlertStateFiring
	}
	return alertKeyState{State: corev1.AlertStatePending, PendingSince: since}, ""
}

func stepFromFiring(prev alertKeyState, breached, recovered bool, spec transitionSpec, now time.Time) (alertKeyState, string) {
	if breached {
		next := prev
		next.RecoverSince = time.Time{}
		if spec.repeatInterval > 0 && !prev.LastNotifiedAt.IsZero() && now.Sub(prev.LastNotifiedAt) >= spec.repeatInterval {
			next.LastNotifiedAt = now
			return next, corev1.AlertStateFiring
		}
		return next, ""
	}
	if !recovered {
		next := prev
		next.RecoverSince = time.Time{}
		return next, ""
	}
	if spec.keepFiringFor <= 0 {
		return alertKeyState{State: corev1.AlertStateNormal}, corev1.AlertStateResolved
	}
	next := prev
	next.State = corev1.AlertStateRecovering
	next.RecoverSince = now
	return next, ""
}

func stepFromRecovering(prev alertKeyState, recovered bool, spec transitionSpec, now time.Time) (alertKeyState, string) {
	if !recovered {
		next := prev
		next.State = corev1.AlertStateFiring
		next.RecoverSince = time.Time{}
		return next, ""
	}
	since := prev.RecoverSince
	if since.IsZero() {
		since = now
	}
	if now.Sub(since) >= spec.keepFiringFor {
		return alertKeyState{State: corev1.AlertStateNormal}, corev1.AlertStateResolved
	}
	prev.RecoverSince = since
	return prev, ""
}

func ratioBreachAndRecovery(pct, threshold int) (breached, recovered bool) {
	margin := min(5, max(1, threshold/10))
	recoverAt := max(0, threshold-margin)
	return pct >= threshold, pct <= recoverAt
}

func tpsBreachAndRecovery(val float64, threshold int) (breached, recovered bool) {
	recoverAt := min(20, threshold+2)
	return val <= float64(threshold), val >= float64(recoverAt)
}

func isAlertKeyFiring(alertKey string) (firing bool, tracked bool) {
	store := activeStateStore()
	if store == nil {
		return false, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	st := loadAlertKeyState(ctx, store, alertKey, time.Now().UTC())
	return st.State == corev1.AlertStateFiring || st.State == corev1.AlertStateRecovering, true
}

func isNodeRootCauseFiring(nodeName string) bool {
	if strings.TrimSpace(nodeName) == "" {
		return false
	}
	firing, _ := isAlertKeyFiring("node:phase:" + strings.TrimSpace(nodeName))
	return firing
}

// ClearInstanceAlerts silently removes cached pending/firing alert states and temporary snoozes
// for an instance when it is intentionally stopped or deleted.
func ClearInstanceAlerts(ctx context.Context, reg *repo.Registry, tenant, name string) {
	if strings.TrimSpace(tenant) == "" || strings.TrimSpace(name) == "" {
		return
	}
	ClearTargetSilences(ctx, reg, nil, tenant, corev1.SilenceTargetInstances, name, time.Now().UTC())
	store := activeStateStore()
	if store == nil {
		return
	}
	phaseKey := fmt.Sprintf("inst:phase:%s/%s", tenant, name)
	_ = store.Delete(ctx, stateKeyPrefix+phaseKey)
	_ = store.Delete(ctx, legacyStateKeyPrefix+phaseKey)
	for _, r := range LoadClusterAlertRules(ctx, reg) {
		rk := fmt.Sprintf("inst:%s/%s:%s", tenant, name, r.ID)
		_ = store.Delete(ctx, stateKeyPrefix+rk)
		_ = store.Delete(ctx, legacyStateKeyPrefix+rk)
	}
}
