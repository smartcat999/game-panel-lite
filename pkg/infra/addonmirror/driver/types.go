package driver

import (
	"errors"
	"time"
)

const (
	ProviderSteamWorkshop = "steam_workshop"
	ProviderModrinth      = "modrinth"
	ProviderPaperMC       = "papermc"
	ProviderPurpur        = "purpur"
	ProviderVanilla       = "vanilla"
	ProviderFabricMC      = "fabricmc"
	ProviderNeoForge      = "neoforge"
	ProviderSteam         = "steam"
	ProviderHTTP          = "http"
	ProviderTerraria      = "terraria"
	ProviderTModLoader    = "tmodloader"

	FormatServerPackage = "server_package"
	FormatWorkshopMod   = "workshop_mod"
	FormatModrinthAddon = "modrinth_addon"
	FormatGeneric       = "generic"

	ArchiveFormatRaw   = "raw"
	ArchiveFormatTarGz = "tar.gz"
	ArchiveFormatZip   = "zip"
	ArchiveFormatJar   = "jar"
	ArchiveFormatTMod  = "tmod"

	SteamCMDScheme = "steamcmd://"

	SortPopular   = "popular"
	SortDownloads = "downloads"
	SortTrend     = "trend"
	SortTopRated  = "toprated"
	SortNewest    = "newest"
	SortUpdated   = "updated"

	ProjectTypePlugin = "plugin"
	ProjectTypeMod    = "mod"

	SteamAppPalworld                 = 1623730
	SteamAppSevenDaysToDie           = 251570
	SteamAppProjectZomboid           = 108600
	SteamAppDontStarveTogether       = 322330
	SteamAppTModLoader               = 1281930
	SteamAppLeft4Dead2               = 550
	SteamServerAppPalworld           = 2394010
	SteamServerAppProjectZomboid     = 380870
	SteamServerAppDontStarveTogether = 343050
	SteamServerAppSevenDaysToDie     = 294420
	SteamServerAppLeft4Dead2         = 222860

	DefaultSteamAPIBaseURL             = "https://api.steampowered.com"
	DefaultModrinthBaseURL             = "https://api.modrinth.com/v2"
	DefaultFabricMetaBaseURL           = "https://meta.fabricmc.net"
	DefaultMojangManifestURL           = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	DefaultMojangMirrorBase            = "https://bmclapi2.bangbang93.com"
	DefaultFastMirrorBase              = "https://download.fastmirror.net"
	DefaultNeoForgeMavenBaseURL        = "https://maven.neoforged.net"
	DefaultNeoForgeMirrorBase          = "https://bmclapi2.bangbang93.com"
	DefaultMinecraftLibrariesBaseURL   = "https://libraries.minecraft.net"
	DefaultMinecraftLibrariesMirrorURL = "https://bmclapi2.bangbang93.com/maven"
	DefaultFabricServerMainClass       = "net.fabricmc.loader.impl.launch.server.FabricServerLauncher"
	DefaultUserAgent                   = "GamePanel-AddonMirror/1.0 (+https://github.com/smartcat999/game-panel-cloud)"
)

var (
	ErrSteamKeyMissing    = errors.New("steam web api key is not configured")
	ErrUnsupportedSource  = errors.New("unsupported addon mirror provider")
	ErrInvalidRequest     = errors.New("invalid addon mirror request")
	ErrInvalidSignature   = errors.New("invalid addon image signature")
	ErrForbiddenImageHost = errors.New("upstream image host is not allowlisted")
	ErrNotFound           = errors.New("addon item not found")
)

// Source describes the upstream catalog binding for a specific game template.
type Source struct {
	Provider            string
	Endpoint            string
	Format              string
	SteamAppID          int
	ModrinthLoader      string
	ModrinthProjectType string
	ImageProxyPrefix    string
}

// RegistryParams configures a Service and Source from a registry Upstream/Repository pair without caller branching.
type RegistryParams struct {
	Provider            string
	Endpoint            string
	Format              string
	ProxyURL            string
	APIKey              string
	SteamAppID          int
	ModrinthLoader      string
	ModrinthProjectType string
	ImageProxyPrefix    string
}

// SearchParams controls catalog browsing and search pagination.
type SearchParams struct {
	Query       string
	Sort        string
	GameVersion string
	Page        int
	Limit       int
}

// Dependency represents a required upstream addon dependency.
type Dependency struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

// Item is a provider-agnostic mod or plugin metadata record.
type Item struct {
	ID                string       `json:"id"`
	Title             string       `json:"title"`
	Summary           string       `json:"summary,omitempty"`
	Description       string       `json:"description,omitempty"`
	Changelog         string       `json:"changelog,omitempty"`
	Author            string       `json:"author,omitempty"`
	IconURL           string       `json:"iconUrl,omitempty"`
	GalleryURLs       []string     `json:"galleryUrls,omitempty"`
	SourceURL         string       `json:"sourceUrl,omitempty"`
	UpstreamURL       string       `json:"upstreamUrl,omitempty"`
	FileURL           string       `json:"fileUrl,omitempty"`
	FileName          string       `json:"fileName,omitempty"`
	LatestVersion     string       `json:"latestVersion,omitempty"`
	AvailableVersions []string     `json:"availableVersions,omitempty"`
	Revision          int64        `json:"revision,omitempty"`
	Downloads         int64        `json:"downloads,omitempty"`
	SizeBytes         int64        `json:"sizeBytes,omitempty"`
	UpdatedAt         string       `json:"updatedAt,omitempty"`
	Tags              []string     `json:"tags,omitempty"`
	ModIDs            []string     `json:"modIds,omitempty"`
	MapFolders        []string     `json:"mapFolders,omitempty"`
	Dependencies      []Dependency `json:"dependencies,omitempty"`
}

// ResolvedRelease describes a concrete upstream artifact version and its download payload metadata.
type ResolvedRelease struct {
	Package              string
	Version              string
	Build                string
	DownloadURL          string
	FallbackDownloadURLs []string
	FileName             string
	ArchiveFormat        string
	TargetPath           string
	SizeBytes            int64
	GameVersions         []string
	Loaders              []string
	ModIDs               []string
	MapFolders           []string
	Dependencies         []Dependency
	PublishedAt          *time.Time
	Changelog            string
	UpstreamURL          string
	Item                 *Item
	VersionMismatch      bool // true when StrictVersion=false resolved metadata whose upstream latest version differs
}

// Result wraps a list/search or batch-resolve response.
type Result struct {
	Configured bool   `json:"configured"`
	Message    string `json:"message,omitempty"`
	Total      int    `json:"total"`
	Items      []Item `json:"items,omitempty"`
}

// Options allows overriding endpoints, HTTP proxy, and credentials (used by tests and standalone mode).
type Options struct {
	SteamAPIBaseURL   string
	ModrinthBaseURL   string
	SteamWebAPIKey    string
	UpstreamProxyURL  string
	SigningSecret     string
	ImageHostMirrors  []string
	AllowedImageHosts []string
	CacheTTL          time.Duration
	Timeout           time.Duration
	AllowLoopbackImg  bool // test-only hook for httptest image servers
}
