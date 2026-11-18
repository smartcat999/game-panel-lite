package event

import (
	"context"
	"strconv"
	"sync"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	auditdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/audit"
)

const (
	defaultMaxEventsPerTenant = 1000
	defaultMaxEventAge        = 90 * 24 * time.Hour
	auditPruneInterval        = 15 * time.Minute
)

func init() {
	ctrlruntime.Register(&Controller{})
}

type Controller struct {
	mu          sync.Mutex
	lastPruneAt time.Time
}

func (c *Controller) Name() string  { return "04-audit-event" }
func (c *Controller) Topic() string { return mq.TopicAudit }

// Resync performs periodic TTL and per-tenant count pruning of operational audit events at a 15-minute interval.
func (c *Controller) Resync(ctx context.Context, reg *repo.Registry, cfg *ctrlruntime.Config) ([]mq.Event, error) {
	now := time.Now().UTC()
	if cfg != nil {
		c.mu.Lock()
		if !c.lastPruneAt.IsZero() && now.Sub(c.lastPruneAt) < auditPruneInterval {
			c.mu.Unlock()
			return nil, nil
		}
		c.lastPruneAt = now
		c.mu.Unlock()
	}
	if auditDB := auditdb.NewDB(reg.Pool()); auditDB != nil {
		if err := auditdb.PruneOperational(ctx, auditDB, "", defaultMaxEventsPerTenant, defaultMaxEventAge); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// Reconcile consumes audit events from mq.TopicAudit ("core.v1.events") and persists them
// idempotently to the append-only AuditStore (PostgreSQL audit_events table).
func (c *Controller) Reconcile(ctx context.Context, reg *repo.Registry, _ mq.Interface, _ *ctrlruntime.Config, event mq.Event) error {
	auditDB := auditdb.NewDB(reg.Pool())
	if auditDB == nil || len(event.Attributes) == 0 {
		return nil
	}
	statusCode, _ := strconv.Atoi(event.Attributes["statusCode"])
	ts := event.Timestamp.UTC()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	extraAttrs := make(map[string]string)
	for k, v := range event.Attributes {
		switch k {
		case "actor", "verb", "apiGroup", "resource", "subresource", "resourceName", "action", "targetName", "statusCode", "clientIP", "requestID":
		default:
			if v != "" {
				extraAttrs[k] = v
			}
		}
	}
	ev := &corev1.Event{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.GroupVersion,
			Kind:       "Event",
		},
		Metadata: metav1.ObjectMeta{
			Name:              event.ID,
			UID:               event.ID,
			Tenant:            event.Tenant,
			CreationTimestamp: ts,
		},
		Actor:        event.Attributes["actor"],
		Verb:         event.Attributes["verb"],
		APIGroup:     event.Attributes["apiGroup"],
		Resource:     event.Attributes["resource"],
		Subresource:  event.Attributes["subresource"],
		ResourceName: event.Attributes["resourceName"],
		Action:       event.Attributes["action"],
		TargetName:   event.Attributes["targetName"],
		Attributes:   extraAttrs,
		StatusCode:   statusCode,
		ClientIP:     event.Attributes["clientIP"],
		RequestID:    event.Attributes["requestID"],
		Timestamp:    ts,
	}
	return auditDB.Record(ctx, ev)
}
