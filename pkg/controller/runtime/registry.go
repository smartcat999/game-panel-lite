package runtime

import (
	"context"
	"sort"
	"sync"

	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

// Config is an alias for config.ControllerConfig managed centrally in pkg/config.
type Config = config.ControllerConfig

func NewConfig() *Config {
	return config.NewControllerConfig()
}

// Reconciler is implemented by each domain controller subpackage and self-registered via Register().
// To guarantee high availability and zero code duplication between real-time MQ consumers
// and periodic resync passes:
//  1. Reconcile() is the single unified, idempotent reconciliation entrypoint for a resource event.
//  2. Resync() lists out-of-sync / pending resources from storage on each ResyncPeriod tick
//     and emits mq.EventReconcile events that Manager feeds into the same Reconcile() pipeline.
type Reconciler interface {
	Name() string
	Topic() string
	Reconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, cfg *Config, event mq.Event) error
	Resync(ctx context.Context, reg *repo.Registry, cfg *Config) ([]mq.Event, error)
}

var (
	reconcilersMu sync.RWMutex
	reconcilers   = make(map[string]Reconciler)
)

// Register is called by each pkg/controller/<group>/<resource>/controller.go's init() to self-register.
func Register(r Reconciler) {
	reconcilersMu.Lock()
	defer reconcilersMu.Unlock()
	reconcilers[r.Name()] = r
}

// OrderedReconcilers returns all registered Reconcilers in deterministic name order.
func OrderedReconcilers() []Reconciler {
	reconcilersMu.RLock()
	defer reconcilersMu.RUnlock()
	var names []string
	for name := range reconcilers {
		names = append(names, name)
	}
	sort.Strings(names)
	list := make([]Reconciler, 0, len(names))
	for _, name := range names {
		list = append(list, reconcilers[name])
	}
	return list
}
