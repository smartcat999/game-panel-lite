package instance

import (
	"context"
	"sync"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
)

type spyMQBus struct {
	mu     sync.Mutex
	events []mq.Event
	onPub  func(topic string, event mq.Event)
}

func (s *spyMQBus) Publish(_ context.Context, topic string, event mq.Event) error {
	event.Topic = topic
	s.mu.Lock()
	s.events = append(s.events, event)
	cb := s.onPub
	s.mu.Unlock()
	if cb != nil {
		cb(topic, event)
	}
	return nil
}

func (s *spyMQBus) Subscribe(context.Context, string, string, mq.HandlerFunc) (mq.Subscription, error) {
	return nil, nil
}

func (s *spyMQBus) Ping(context.Context) error { return nil }
func (s *spyMQBus) Close() error               { return nil }

func newTestInstance(uid, rv string) *corev1.Instance {
	return &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			Tenant:          "admin",
			Name:            "test-inst",
			UID:             uid,
			Generation:      1,
			ResourceVersion: rv,
		},
	}
}

func TestReleaseAndPublishUnlocksBeforeWakeup(t *testing.T) {
	ctx := context.Background()
	c := &Controller{}
	inst := newTestInstance("uid-unlock-before-wakeup", "2")
	lockKey := "gamepanel:lock:instance:reconcile:" + inst.Metadata.UID

	if !c.beginReconcile(inst, false) {
		t.Fatalf("expected initial beginReconcile to succeed")
	}
	unlock, acquired := cache.TryLock(ctx, lockKey, instanceReconcileTTL)
	if !acquired {
		t.Fatalf("expected initial cache.TryLock to acquire lock")
	}

	var sawInstanceReconcile, lockReacquiredInPublish, entryReenteredInPublish bool
	bus := &spyMQBus{
		onPub: func(topic string, event mq.Event) {
			if topic != mq.TopicInstance || event.Type != mq.EventReconcile {
				return
			}
			sawInstanceReconcile = true
			if relock, ok := cache.TryLock(ctx, lockKey, instanceReconcileTTL); ok {
				lockReacquiredInPublish = true
				relock()
			}
			entryReenteredInPublish = c.beginReconcile(inst, false)
			if entryReenteredInPublish {
				c.finishReconcile(inst.Metadata.UID, nil, true, true, nil, time.Now())
			}
		},
	}

	if err := c.releaseAndPublish(ctx, bus, inst, inst, true, acquired, unlock, nil); err != nil {
		t.Fatalf("releaseAndPublish returned unexpected error: %v", err)
	}
	if !sawInstanceReconcile {
		t.Fatalf("expected EventReconcile to be published on %s", mq.TopicInstance)
	}
	if !lockReacquiredInPublish {
		t.Fatalf("expected cache lock %s to be released before publishing EventReconcile", lockKey)
	}
	if !entryReenteredInPublish {
		t.Fatalf("expected reconcileKeys entry to be released before publishing EventReconcile")
	}
}

func TestConcurrentWakeupCoalescedIntoPendingRerun(t *testing.T) {
	ctx := context.Background()
	c := &Controller{}
	inst := newTestInstance("uid-coalesce-pending", "5")
	lockKey := "gamepanel:lock:instance:reconcile:" + inst.Metadata.UID

	if !c.beginReconcile(inst, false) {
		t.Fatalf("expected initial beginReconcile to succeed")
	}
	unlock, acquired := cache.TryLock(ctx, lockKey, instanceReconcileTTL)
	if !acquired {
		t.Fatalf("expected initial cache.TryLock to acquire lock")
	}

	if c.beginReconcile(inst, true) {
		t.Fatalf("expected statusOnly beginReconcile during active run to return false")
	}
	if c.reconcileKeys[inst.Metadata.UID].pending {
		t.Fatalf("expected statusOnly event not to mark pending=true")
	}
	if c.beginReconcile(inst, false) {
		t.Fatalf("expected concurrent non-statusOnly beginReconcile to return false")
	}
	if !c.reconcileKeys[inst.Metadata.UID].pending {
		t.Fatalf("expected concurrent non-statusOnly event to set pending=true")
	}

	bus := &spyMQBus{}
	if err := c.releaseAndPublish(ctx, bus, inst, inst, false, true, unlock, nil); err != nil {
		t.Fatalf("releaseAndPublish failed: %v", err)
	}
	if len(bus.events) != 1 || bus.events[0].Topic != mq.TopicInstance || bus.events[0].Type != mq.EventReconcile {
		t.Fatalf("expected exactly 1 coalesced EventReconcile on %s, got %+v", mq.TopicInstance, bus.events)
	}

	noLockBus := &spyMQBus{}
	if !c.beginReconcile(inst, false) {
		t.Fatalf("expected beginReconcile after coalesced release to succeed")
	}
	if c.beginReconcile(inst, false) {
		t.Fatalf("expected concurrent beginReconcile during second run to return false")
	}
	if err := c.releaseAndPublish(ctx, noLockBus, inst, inst, false, false, nil, nil); err != nil {
		t.Fatalf("releaseAndPublish without lock failed: %v", err)
	}
	if len(noLockBus.events) != 0 {
		t.Fatalf("expected 0 published events when lock was not acquired, got %+v", noLockBus.events)
	}
	c.finishReconcile(inst.Metadata.UID, nil, true, true, nil, time.Now())
}
