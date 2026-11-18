package runtime

import (
	"context"
	"sort"
	"sync"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

// Dependencies provides the shared infrastructure needed by each resource's register.go
// to construct its own per-resource model Operator and wire its HTTP Handler into the Container.
type Dependencies struct {
	DB       *repo.Registry
	Cache    cache.Interface
	MQ       mq.Interface
	Drivers  *drvruntime.Registry
	Sessions sessionmodel.Operator
	// CanI reports whether the authenticated caller on ctx may perform verb on a resource.
	// Handlers use it for decisions that depend on the request body (e.g. Node token issuance).
	CanI AccessChecker
}

// AccessChecker evaluates the platform authorizer for the caller attached to ctx.
type AccessChecker func(ctx context.Context, apiGroup, resource, verb, name string) bool

// RouteInstaller installs a resource module's WebService routes and OpenAPI schemas into the API Server Container.
type RouteInstaller func(deps Dependencies, c *Container)

var (
	installersMu sync.RWMutex
	installers   = make(map[string]RouteInstaller)
)

// Register is called by each pkg/kapis/<group>/<version>/<resource>/register.go's init() to self-register its WebService.
func Register(name string, installer RouteInstaller) {
	installersMu.Lock()
	defer installersMu.Unlock()
	installers[name] = installer
}

// InstallAll installs all registered resource WebServices into the Container in deterministic order.
func InstallAll(deps Dependencies, c *Container) {
	installersMu.RLock()
	var names []string
	for name := range installers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		installers[name](deps, c)
	}
	installersMu.RUnlock()
}
