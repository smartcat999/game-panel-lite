package instance

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
)

type stubAppCache struct{ data map[string]string }

func (s *stubAppCache) Get(_ context.Context, key string) (string, error) {
	v, ok := s.data[key]
	if !ok {
		return "", cache.ErrNotFound
	}
	return v, nil
}
func (s *stubAppCache) Set(_ context.Context, key, val string, _ time.Duration) error {
	s.data[key] = val
	return nil
}
func (s *stubAppCache) Delete(_ context.Context, key string) error { delete(s.data, key); return nil }
func (s *stubAppCache) Exists(_ context.Context, key string) (bool, error) {
	_, ok := s.data[key]
	return ok, nil
}
func (s *stubAppCache) Ping(context.Context) error { return nil }
func (s *stubAppCache) Close() error               { return nil }

func TestMatchesFrozenDeletingSpecHydration(t *testing.T) {
	ctx := context.Background()
	mem := &stubAppCache{data: make(map[string]string)}
	prevCache := cache.Default()
	cache.SetDefault(mem)
	t.Cleanup(func() { cache.SetDefault(prevCache) })
	app := &corev1.App{Metadata: metav1.ObjectMeta{Name: "minecraft-paper"}, ConsoleDelivery: "mc-send-to-console"}
	if err := cache.SetJSON(ctx, mem, appSpecCacheKey(app.Metadata.Name), app, time.Minute); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	op := &operator{appDB: &appdb.DB{}}
	current := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "demo-mc-01", Labels: map[string]string{corev1.LabelInstanceApp: "minecraft-paper"}},
		Spec: corev1.InstanceSpec{
			State: corev1.InstanceStateStopped, Image: "alpine:3.20",
			Ports:        []corev1.InstancePort{{Name: "game", Protocol: corev1.ProtocolTCP, TargetPort: 25565}},
			BackupPolicy: &corev1.InstanceBackupPolicy{Subtree: "data/world"},
		},
	}
	current.DefaultMeta()
	incoming, err := intentCopy(current)
	if err != nil {
		t.Fatalf("intentCopy: %v", err)
	}
	op.hydrateInstanceMetricsSpec(ctx, incoming)
	if incoming.Spec.BackupPolicy.ConsoleDelivery != "mc-send-to-console" {
		t.Fatalf("expected hydrated consoleDelivery, got %q", incoming.Spec.BackupPolicy.ConsoleDelivery)
	}
	if !op.matchesFrozenDeletingSpec(ctx, current, incoming) {
		t.Fatal("expected hydrated read round-trip to match frozen deleting spec")
	}
	if !reflect.DeepEqual(incoming.Spec, current.Spec) {
		t.Fatalf("expected incoming.Spec restored to DB spec, got %+v", incoming.Spec.BackupPolicy)
	}
	mutated, _ := intentCopy(incoming)
	mutated.Spec.Image = "busybox:latest"
	if op.matchesFrozenDeletingSpec(ctx, current, mutated) {
		t.Fatal("expected real spec mutation on deleting instance to be rejected")
	}
}

func TestHydrateInstanceConsoleDelivery(t *testing.T) {
	app := &corev1.App{ConsoleDelivery: "tml-send-to-console"}

	for _, initial := range []string{"", drvruntime.ConsoleDeliveryStdin} {
		inst := &corev1.Instance{
			Spec: corev1.InstanceSpec{
				BackupPolicy: &corev1.InstanceBackupPolicy{ConsoleDelivery: initial},
			},
		}
		if !needsConsoleDeliveryHydration(inst) {
			t.Fatalf("expected initial delivery %q to need hydration", initial)
		}
		hydrateInstanceConsoleDelivery(app, inst)
		if got := inst.Spec.BackupPolicy.ConsoleDelivery; got != "tml-send-to-console" {
			t.Fatalf("initial %q: expected tml-send-to-console, got %q", initial, got)
		}
	}

	custom := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			BackupPolicy: &corev1.InstanceBackupPolicy{ConsoleDelivery: "custom-helper"},
		},
	}
	if needsConsoleDeliveryHydration(custom) {
		t.Fatal("expected custom delivery not to need hydration")
	}
	hydrateInstanceConsoleDelivery(app, custom)
	if got := custom.Spec.BackupPolicy.ConsoleDelivery; got != "custom-helper" {
		t.Fatalf("expected custom delivery preserved, got %q", got)
	}
}

func TestApplyStateTransitionRejectsDeletingInstance(t *testing.T) {
	now := time.Now().UTC()
	deleting := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			UID:               "uid-1",
			DeletionTimestamp: &now,
		},
		Spec: corev1.InstanceSpec{State: corev1.InstanceStateStopped},
	}
	if _, err := applyStateTransition(deleting, &corev1.InstanceState{State: corev1.InstanceStateRunning}, time.Now()); !errors.Is(err, models.ErrConflict) {
		t.Fatalf("expected ErrConflict on deleting instance state transition, got %v", err)
	}

	active := &corev1.Instance{
		Metadata: metav1.ObjectMeta{UID: "uid-2"},
		Spec:     corev1.InstanceSpec{State: corev1.InstanceStateStopped},
	}
	next, err := applyStateTransition(active, &corev1.InstanceState{State: corev1.InstanceStateRestart}, time.Unix(1700000000, 0).UTC())
	if err != nil {
		t.Fatalf("expected active restart transition to succeed, got %v", err)
	}
	if next.Spec.State != corev1.InstanceStateRunning || next.Metadata.Annotations[corev1.AnnotationRestartRequestedAt] == "" {
		t.Fatalf("unexpected restart transition result: %+v", next)
	}
	if active.Metadata.Annotations != nil {
		t.Fatalf("expected original annotations not mutated, got %+v", active.Metadata.Annotations)
	}
}

func TestLogStreamHTTPClientDisablesTimeoutWhenFollow(t *testing.T) {
	tr := &http.Transport{}
	base := &http.Client{Timeout: 30 * time.Second, Transport: tr}
	op := &operator{httpClient: base}

	nonFollow := op.logStreamHTTPClient(false)
	if nonFollow.Timeout != 30*time.Second || nonFollow.Transport != tr {
		t.Fatalf("expected non-follow client to preserve 30s timeout and transport, got timeout=%v transport=%p", nonFollow.Timeout, nonFollow.Transport)
	}

	follow := op.logStreamHTTPClient(true)
	if follow.Timeout != 0 {
		t.Fatalf("expected follow log stream client timeout to be 0, got %v", follow.Timeout)
	}
	if follow.Transport != tr {
		t.Fatalf("expected follow client to preserve transport")
	}
	if base.Timeout != 30*time.Second {
		t.Fatalf("expected base httpClient timeout untouched, got %v", base.Timeout)
	}
}
