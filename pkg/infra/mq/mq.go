package mq

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
)

// Standard resource event topics consumed by domain controllers and watch streams.
const (
	TopicInstance          = "core.v1.instances"
	TopicTenant            = "core.v1.tenants"
	TopicNode              = "core.v1.nodes"
	TopicRuntimeClass      = "core.v1.runtimeclasses"
	TopicApp               = "core.v1.apps"
	TopicPrewarmJob        = "core.v1.prewarmjobs"
	TopicUser              = "iam.v1.users"
	TopicGlobalRole        = "iam.v1.globalroles"
	TopicGlobalRoleBinding = "iam.v1.globalrolebindings"
	TopicTenantRole        = "iam.v1.tenantroles"
	TopicTenantRoleBinding = "iam.v1.tenantrolebindings"
)

// EventType classifies the lifecycle transition carried by a message queue Event.
type EventType string

const (
	EventCreated      EventType = "Created"
	EventUpdated      EventType = "Updated"
	EventDeleted      EventType = "Deleted"
	EventStateChanged EventType = "StateChanged"
	EventReconcile    EventType = "Reconcile"
)

// Event is the unified, typed message envelope published by API resource operators
// and asynchronously consumed by topic-bound controllers.
type Event struct {
	ID              string            `json:"id"`
	Topic           string            `json:"topic"`
	Type            EventType         `json:"type"`
	APIVersion      string            `json:"apiVersion"`
	Kind            string            `json:"kind"`
	Tenant          string            `json:"tenant,omitempty"`
	Name            string            `json:"name"`
	Generation      int64             `json:"generation,omitempty"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	Timestamp       time.Time         `json:"timestamp"`
	Attributes      map[string]string `json:"attributes,omitempty"`
}

// NewEvent constructs a normalized Event with a unique ID and UTC timestamp.
func NewEvent(eventType EventType, apiVersion, kind, tenant, name string, generation int64, resourceVersion string) Event {
	return Event{
		ID:              randomID(),
		Type:            eventType,
		APIVersion:      apiVersion,
		Kind:            kind,
		Tenant:          tenant,
		Name:            name,
		Generation:      generation,
		ResourceVersion: resourceVersion,
		Timestamp:       time.Now().UTC(),
	}
}

// Config is an alias for config.MQConfig managed centrally in pkg/config.
type Config = config.MQConfig

func NewConfig() *Config {
	return config.NewMQConfig()
}

// HandlerFunc processes a single Event delivered from a topic subscription.
// Implementations MUST be idempotent / re-entrant so redelivered events are safe.
type HandlerFunc func(ctx context.Context, event Event) error

// Subscription represents an active topic consumer subscription.
type Subscription interface {
	Unsubscribe() error
}

// Interface is the unified message queue abstraction supporting publish/subscribe across drivers.
type Interface interface {
	Publish(ctx context.Context, topic string, event Event) error
	Subscribe(ctx context.Context, topic, group string, handler HandlerFunc) (Subscription, error)
	Ping(ctx context.Context) error
	Close() error
}

// Factory constructs a message queue implementation for a specific driver.
type Factory func(ctx context.Context, cfg *Config) (Interface, error)

var (
	factoriesMu sync.RWMutex
	factories   = make(map[string]Factory)
)

// Register is called by each mq driver file's init() to self-register its implementation.
func Register(driver string, factory Factory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[strings.ToLower(strings.TrimSpace(driver))] = factory
}

// Open initializes the configured message queue driver.
func Open(ctx context.Context, cfg *Config) (Interface, error) {
	if cfg == nil {
		cfg = &config.Get().MQ
	}
	cfg.Default()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	factoriesMu.RLock()
	factory, ok := factories[driver]
	factoriesMu.RUnlock()
	if !ok {
		return nil, errors.New("unsupported mq driver: " + driver)
	}
	return factory(ctx, cfg)
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// recordPublishFailure records structured logs and Prometheus metrics when a message queue
// notification fails after state persistence. Declarative controllers recover via Resync.
func recordPublishFailure(driver, topic string, event Event, err error) {
	if err == nil {
		return
	}
	metrics.MQPublishErrorsTotal.WithLabelValues(topic, string(event.Type)).Inc()
	log.Warn("mq publish failed; relying on controller resync",
		"driver", driver,
		"topic", topic,
		"type", event.Type,
		"tenant", event.Tenant,
		"name", event.Name,
		"error", err,
	)
}
