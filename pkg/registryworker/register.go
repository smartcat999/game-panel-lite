package registryworker

import (
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/registryworker/driver"

	// Self-register built-in artifact sync drivers, SteamCMD executor, and workshop content inspectors.
	_ "github.com/smartcat999/game-panel-cloud/pkg/registryworker/providers/dst"
	_ "github.com/smartcat999/game-panel-cloud/pkg/registryworker/providers/http"
	_ "github.com/smartcat999/game-panel-cloud/pkg/registryworker/providers/l4d2"
	_ "github.com/smartcat999/game-panel-cloud/pkg/registryworker/providers/steam"
	_ "github.com/smartcat999/game-panel-cloud/pkg/registryworker/providers/steamcmd"
	_ "github.com/smartcat999/game-panel-cloud/pkg/registryworker/providers/tmod"
	_ "github.com/smartcat999/game-panel-cloud/pkg/registryworker/providers/zomboid"
)

// DefaultSteamExecutorDriver is the built-in OS subprocess SteamCMD runner driver name.
const DefaultSteamExecutorDriver = driver.DefaultSteamExecutorDriver

type (
	BuiltPayload         = driver.BuiltPayload
	Reporter             = driver.Reporter
	SteamHooks           = driver.SteamHooks
	SteamExecutor        = driver.SteamExecutor
	SteamExecutorFactory = driver.SteamExecutorFactory
	Runtime              = driver.Runtime
	SyncRequest          = driver.SyncRequest
	SyncDriver           = driver.SyncDriver
	WorkshopInspection   = driver.WorkshopInspection
	WorkshopInspector    = driver.WorkshopInspector

	builtPayload     = driver.BuiltPayload
	byteSpeedTracker = driver.ByteSpeedTracker
)

// RegisterSyncDriver registers a SyncDriver for an upstream provider and repository format.
func RegisterSyncDriver(provider, format string, drv SyncDriver) {
	driver.RegisterSyncDriver(provider, format, drv)
}

// RegisterSchemeSyncDriver registers a SyncDriver for an explicit artifact SourceURL scheme.
func RegisterSchemeSyncDriver(scheme string, drv SyncDriver) {
	driver.RegisterSchemeSyncDriver(scheme, drv)
}

// LookupSyncDriver resolves the SyncDriver responsible for synchronizing the given artifact, repository, and upstream.
func LookupSyncDriver(art *registryv1.Artifact, repoObj *registryv1.Repository, up *registryv1.Upstream) (SyncDriver, error) {
	return driver.LookupSyncDriver(art, repoObj, up)
}

// RegisterWorkshopInspector registers a WorkshopInspector for a Steam AppID.
func RegisterWorkshopInspector(steamAppID int, insp WorkshopInspector) {
	driver.RegisterWorkshopInspector(steamAppID, insp)
}

// LookupWorkshopInspector returns the registered WorkshopInspector for a Steam AppID, if any.
func LookupWorkshopInspector(steamAppID int) (WorkshopInspector, bool) {
	return driver.LookupWorkshopInspector(steamAppID)
}

// RegisterSteamExecutor registers a named SteamExecutor driver factory.
func RegisterSteamExecutor(name string, factory SteamExecutorFactory) {
	driver.RegisterSteamExecutor(name, factory)
}

func newSteamExecutor(name string, cfg config.RegistryConfig) SteamExecutor {
	return driver.NewSteamExecutor(name, cfg)
}
