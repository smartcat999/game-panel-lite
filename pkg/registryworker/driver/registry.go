package driver

import (
	"fmt"
	"strings"
	"sync"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
)

type syncDriverKey struct {
	provider string
	format   string
}

var (
	syncDriversMu sync.RWMutex
	syncDrivers   = make(map[syncDriverKey]SyncDriver)
	schemeDrivers = make(map[string]SyncDriver)

	inspectorsMu       sync.RWMutex
	workshopInspectors = make(map[int]WorkshopInspector)

	steamExecMu      sync.RWMutex
	steamExecDrivers = make(map[string]SteamExecutorFactory)
)

// RegisterSyncDriver registers a SyncDriver for an upstream provider and repository format ("*" matches any format).
func RegisterSyncDriver(provider, format string, drv SyncDriver) {
	if drv == nil {
		return
	}
	p := strings.ToLower(strings.TrimSpace(provider))
	f := strings.ToLower(strings.TrimSpace(format))
	if f == "" {
		f = "*"
	}
	if p == "" {
		return
	}
	syncDriversMu.Lock()
	defer syncDriversMu.Unlock()
	syncDrivers[syncDriverKey{provider: p, format: f}] = drv
}

// RegisterSchemeSyncDriver registers a SyncDriver for an explicit artifact SourceURL scheme.
func RegisterSchemeSyncDriver(scheme string, drv SyncDriver) {
	if drv == nil {
		return
	}
	s := strings.ToLower(strings.TrimSpace(scheme))
	if s == "" {
		return
	}
	syncDriversMu.Lock()
	defer syncDriversMu.Unlock()
	schemeDrivers[s] = drv
}

// LookupSyncDriver resolves the SyncDriver responsible for synchronizing the given artifact, repository, and upstream.
func LookupSyncDriver(art *registryv1.Artifact, repoObj *registryv1.Repository, up *registryv1.Upstream) (SyncDriver, error) {
	scheme := sourceURLScheme(art)
	syncDriversMu.RLock()
	defer syncDriversMu.RUnlock()

	if scheme != "" && scheme != "http" && scheme != "https" {
		if drv := schemeDrivers[scheme]; drv != nil {
			return drv, nil
		}
	}
	if up != nil {
		p := strings.ToLower(strings.TrimSpace(up.Provider))
		f := ""
		if repoObj != nil {
			f = strings.ToLower(strings.TrimSpace(repoObj.Spec.Format))
		}
		if f != "" {
			if drv := syncDrivers[syncDriverKey{provider: p, format: f}]; drv != nil {
				return drv, nil
			}
		}
		if drv := syncDrivers[syncDriverKey{provider: p, format: "*"}]; drv != nil {
			return drv, nil
		}
	}
	if scheme != "" {
		if drv := schemeDrivers[scheme]; drv != nil {
			return drv, nil
		}
	}
	if up == nil {
		name := ""
		if art != nil {
			name = art.Metadata.Name
		}
		return nil, fmt.Errorf("artifact %q has no sourceURL and repository has no upstream", name)
	}
	format := ""
	if repoObj != nil {
		format = repoObj.Spec.Format
	}
	return nil, fmt.Errorf("unsupported upstream provider %q (format %q)", up.Provider, format)
}

func sourceURLScheme(art *registryv1.Artifact) string {
	if art == nil {
		return ""
	}
	raw := strings.TrimSpace(art.Spec.SourceURL)
	idx := strings.Index(raw, "://")
	if idx <= 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(raw[:idx]))
}

// RegisterWorkshopInspector registers a WorkshopInspector for a Steam AppID.
func RegisterWorkshopInspector(steamAppID int, insp WorkshopInspector) {
	if steamAppID <= 0 || insp == nil {
		return
	}
	inspectorsMu.Lock()
	defer inspectorsMu.Unlock()
	workshopInspectors[steamAppID] = insp
}

// LookupWorkshopInspector returns the registered WorkshopInspector for a Steam AppID, if any.
func LookupWorkshopInspector(steamAppID int) (WorkshopInspector, bool) {
	inspectorsMu.RLock()
	defer inspectorsMu.RUnlock()
	insp, ok := workshopInspectors[steamAppID]
	return insp, ok && insp != nil
}

// RegisterSteamExecutor registers a named SteamExecutor driver factory.
func RegisterSteamExecutor(name string, factory SteamExecutorFactory) {
	steamExecMu.Lock()
	defer steamExecMu.Unlock()
	if name == "" || factory == nil {
		return
	}
	steamExecDrivers[name] = factory
}

// NewSteamExecutor instantiates a registered SteamExecutor by driver name.
func NewSteamExecutor(name string, cfg config.RegistryConfig) SteamExecutor {
	steamExecMu.RLock()
	factory := steamExecDrivers[name]
	steamExecMu.RUnlock()
	if factory != nil {
		return factory(cfg)
	}
	return nil
}
