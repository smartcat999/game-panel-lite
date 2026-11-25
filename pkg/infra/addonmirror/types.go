package addonmirror

import "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"

const (
	ProviderSteamWorkshop = driver.ProviderSteamWorkshop
	ProviderModrinth      = driver.ProviderModrinth
	ProviderPaperMC       = driver.ProviderPaperMC
	ProviderPurpur        = driver.ProviderPurpur
	ProviderVanilla       = driver.ProviderVanilla
	ProviderFabricMC      = driver.ProviderFabricMC
	ProviderNeoForge      = driver.ProviderNeoForge
	ProviderThunderstore  = driver.ProviderThunderstore
	ProviderUMod          = driver.ProviderUMod
	ProviderFicsit        = driver.ProviderFicsit
	ProviderCS2Plugins    = driver.ProviderCS2Plugins
	ProviderSevenDaysMods = driver.ProviderSevenDaysMods
	ProviderModIO         = driver.ProviderModIO
	ProviderSteam         = driver.ProviderSteam
	ProviderHTTP          = driver.ProviderHTTP
	ProviderTerraria      = driver.ProviderTerraria
	ProviderTModLoader    = driver.ProviderTModLoader

	FormatServerPackage   = driver.FormatServerPackage
	FormatWorkshopMod     = driver.FormatWorkshopMod
	FormatModrinthAddon   = driver.FormatModrinthAddon
	FormatThunderstoreMod = driver.FormatThunderstoreMod
	FormatUModPlugin      = driver.FormatUModPlugin
	FormatFicsitMod       = driver.FormatFicsitMod
	FormatCS2Plugin       = driver.FormatCS2Plugin
	FormatSevenDaysMod    = driver.FormatSevenDaysMod
	FormatModIOMod        = driver.FormatModIOMod
	FormatGeneric         = driver.FormatGeneric

	ArchiveFormatRaw   = driver.ArchiveFormatRaw
	ArchiveFormatTarGz = driver.ArchiveFormatTarGz
	ArchiveFormatZip   = driver.ArchiveFormatZip
	ArchiveFormatJar   = driver.ArchiveFormatJar
	ArchiveFormatTMod  = driver.ArchiveFormatTMod

	SteamCMDScheme         = driver.SteamCMDScheme
	PackageVanillaMappings = driver.PackageVanillaMappings

	SortPopular   = driver.SortPopular
	SortDownloads = driver.SortDownloads
	SortTrend     = driver.SortTrend
	SortTopRated  = driver.SortTopRated
	SortNewest    = driver.SortNewest
	SortUpdated   = driver.SortUpdated

	ProjectTypePlugin = driver.ProjectTypePlugin
	ProjectTypeMod    = driver.ProjectTypeMod

	SteamAppPalworld                 = driver.SteamAppPalworld
	SteamAppSevenDaysToDie           = driver.SteamAppSevenDaysToDie
	SteamAppProjectZomboid           = driver.SteamAppProjectZomboid
	SteamAppDontStarveTogether       = driver.SteamAppDontStarveTogether
	SteamAppTModLoader               = driver.SteamAppTModLoader
	SteamAppLeft4Dead2               = driver.SteamAppLeft4Dead2
	SteamAppCS2                      = driver.SteamAppCS2
	SteamServerAppPalworld           = driver.SteamServerAppPalworld
	SteamServerAppProjectZomboid     = driver.SteamServerAppProjectZomboid
	SteamServerAppDontStarveTogether = driver.SteamServerAppDontStarveTogether
	SteamServerAppSevenDaysToDie     = driver.SteamServerAppSevenDaysToDie
	SteamServerAppLeft4Dead2         = driver.SteamServerAppLeft4Dead2

	defaultUserAgent                   = driver.DefaultUserAgent
	DefaultMojangManifestURL           = driver.DefaultMojangManifestURL
	DefaultMojangMirrorBase            = driver.DefaultMojangMirrorBase
	DefaultFastMirrorBase              = driver.DefaultFastMirrorBase
	DefaultNeoForgeMavenBaseURL        = driver.DefaultNeoForgeMavenBaseURL
	DefaultNeoForgeMirrorBase          = driver.DefaultNeoForgeMirrorBase
	DefaultMinecraftLibrariesBaseURL   = driver.DefaultMinecraftLibrariesBaseURL
	DefaultMinecraftLibrariesMirrorURL = driver.DefaultMinecraftLibrariesMirrorURL
	DefaultFabricServerMainClass       = driver.DefaultFabricServerMainClass
	DefaultThunderstoreBaseURL         = driver.DefaultThunderstoreBaseURL
	DefaultUModBaseURL                 = driver.DefaultUModBaseURL
	DefaultFicsitBaseURL               = driver.DefaultFicsitBaseURL
	DefaultMetamodBaseURL              = driver.DefaultMetamodBaseURL
	DefaultGitHubAPIBaseURL            = driver.DefaultGitHubAPIBaseURL
	DefaultGitHubAvatarBaseURL         = driver.DefaultGitHubAvatarBaseURL
	DefaultModIOBaseURL                = driver.DefaultModIOBaseURL
	maxProxiedImageBytes               = 8 << 20 // 8 MiB
	maxCachedThumbnailBytes            = 256 << 10
)

var (
	ErrSteamKeyMissing    = driver.ErrSteamKeyMissing
	ErrModIOKeyMissing    = driver.ErrModIOKeyMissing
	ErrUnsupportedSource  = driver.ErrUnsupportedSource
	ErrInvalidRequest     = driver.ErrInvalidRequest
	ErrInvalidSignature   = driver.ErrInvalidSignature
	ErrForbiddenImageHost = driver.ErrForbiddenImageHost
	ErrNotFound           = driver.ErrNotFound
)

type (
	Source                = driver.Source
	RegistryParams        = driver.RegistryParams
	SearchParams          = driver.SearchParams
	Dependency            = driver.Dependency
	Item                  = driver.Item
	ResolvedRelease       = driver.ResolvedRelease
	Result                = driver.Result
	Options               = driver.Options
	Capabilities          = driver.Capabilities
	ResolveReleaseRequest = driver.ResolveReleaseRequest
	Runtime               = driver.Runtime
	Driver                = driver.Driver
	DriverFactory         = driver.DriverFactory
	HTTPSubDriver         = driver.HTTPSubDriver
	BaseDriver            = driver.BaseDriver
)

// Register registers an upstream provider Driver factory.
func Register(provider string, factory DriverFactory) { driver.Register(provider, factory) }

// Unregister removes a registered driver by provider name (primarily for test cleanup).
func Unregister(provider string) { driver.Unregister(provider) }

// LookupDriver returns a newly instantiated Driver for provider.
func LookupDriver(provider string) (Driver, bool) { return driver.LookupDriver(provider) }

// Drivers returns all registered upstream provider names in sorted order.
func Drivers() []string { return driver.Drivers() }

// RegisterHTTPSubDriver registers a specialized HTTP sub-driver matched by endpoint or package name.
func RegisterHTTPSubDriver(sub HTTPSubDriver) { driver.RegisterHTTPSubDriver(sub) }

// IsLatestAlias reports whether v represents a floating release alias rather than a concrete tag.
func IsLatestAlias(v string) bool { return driver.IsLatestAlias(v) }

// ParseInputTokens delegates upstream token extraction to the provider's registered Driver.
func ParseInputTokens(provider, raw string) []string {
	if drv, ok := LookupDriver(provider); ok {
		return drv.ParseTokens(raw)
	}
	return driver.SplitPlainTokens(raw, nil)
}

// SteamNewsAppID maps a Steam dedicated server tool AppID to its public base game AppID.
func SteamNewsAppID(appID int) int { return driver.SteamNewsAppID(appID) }

// SteamNewsURL returns the official Steam News Hub URL for appID.
func SteamNewsURL(appID int) string { return driver.SteamNewsURL(appID) }

// SteamNewsURLFromString parses a decimal Steam AppID string and returns its Steam News Hub URL.
func SteamNewsURLFromString(raw string) string { return driver.SteamNewsURLFromString(raw) }

// NormalizeSteamNewsURL rewrites dedicated-server Steam News URLs to the base game's News Hub URL.
func NormalizeSteamNewsURL(raw string) string { return driver.NormalizeSteamNewsURL(raw) }
