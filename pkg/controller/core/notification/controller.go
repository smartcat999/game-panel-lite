package notification

import (
	"context"
	"strings"
	"sync"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	alertmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

const notificationResyncBatchSize = 100

func init() {
	ctrlruntime.Register(&Controller{})
}

type Controller struct {
	pendingTimers sync.Map // recordID -> *time.Timer
}

func (c *Controller) Name() string  { return "05-alert-notification" }
func (c *Controller) Topic() string { return mq.TopicNotification }

// Resync scans the notification Outbox for due batched, retrying, or lease-expired records
// and feeds them into the unified Reconcile pipeline for crash-safe high availability.
func (c *Controller) Resync(ctx context.Context, reg *repo.Registry, _ *ctrlruntime.Config) ([]mq.Event, error) {
	dueList, err := alertmodel.ListDueNotificationEvents(ctx, reg, time.Now().UTC(), notificationResyncBatchSize)
	if err != nil || len(dueList) == 0 {
		return nil, err
	}
	events := make([]mq.Event, 0, len(dueList))
	for _, item := range dueList {
		if item == nil || item.Metadata.Name == "" {
			continue
		}
		tenant := strings.TrimSpace(item.Attributes["tenant"])
		if tenant == "" {
			tenant = item.Metadata.Tenant
		}
		rv := item.Action + ":" + item.Attributes["attempts"]
		ev := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "AlertNotification", tenant, item.Metadata.Name, 0, rv)
		ev.Topic = mq.TopicNotification
		events = append(events, ev)
	}
	return events, nil
}

// Reconcile processes real-time MQ wakeups, scheduled group_wait flushes, and Resync retries
// by delegating atomic Outbox claiming, grouped rendering, and channel health persistence to the alert domain.
func (c *Controller) Reconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, _ *ctrlruntime.Config, event mq.Event) error {
	recordID := resolveNotificationRecordID(event)
	if recordID == "" {
		return nil
	}
	fromResync := event.Type == mq.EventReconcile
	retryAfter, err := alertmodel.ProcessNotification(ctx, reg, bus, recordID, fromResync)
	if err != nil {
		return err
	}
	if retryAfter > 0 {
		c.scheduleDelayedFlush(bus, event.Tenant, recordID, retryAfter)
	} else {
		c.cancelDelayedFlush(recordID)
	}
	return nil
}

func (c *Controller) scheduleDelayedFlush(bus mq.Interface, tenant, recordID string, delay time.Duration) {
	if bus == nil || recordID == "" || delay <= 0 {
		return
	}
	if _, loaded := c.pendingTimers.Load(recordID); loaded {
		return
	}
	timer := time.AfterFunc(delay, func() {
		c.pendingTimers.Delete(recordID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ev := mq.NewEvent(mq.EventCreated, corev1.GroupVersion, "AlertNotification", tenant, recordID, 0, "")
		ev.Topic = mq.TopicNotification
		_ = bus.Publish(ctx, mq.TopicNotification, ev)
	})
	if _, loaded := c.pendingTimers.LoadOrStore(recordID, timer); loaded {
		timer.Stop()
	}
}

func (c *Controller) cancelDelayedFlush(recordID string) {
	if prev, ok := c.pendingTimers.LoadAndDelete(recordID); ok {
		if t, ok := prev.(*time.Timer); ok {
			t.Stop()
		}
	}
}

func resolveNotificationRecordID(event mq.Event) string {
	name := strings.TrimSpace(event.Name)
	if strings.HasPrefix(name, "ntf-") {
		return name
	}
	if dID := strings.TrimSpace(event.Attributes["deliveryID"]); dID != "" {
		if strings.HasPrefix(dID, "ntf-") {
			return dID
		}
		return "ntf-" + dID
	}
	return ""
}
