package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const DefaultModUploadMaxBytes int64 = 256 << 20

type Config struct {
	ModUploadMaxBytes      int64
	Host                   string
	Port                   string
	DataDir                string
	DatabaseURL            string
	DBMaxConnections       int
	DBPath                 string
	DockerHost             string
	PublicHost             string
	ProviderCatalogPath    string
	ImageRegion            string
	ImageRegistry          string
	ImageTag               string
	PrometheusURL          string
	PrometheusQueryTimeout time.Duration
	ReleaseManifestURL     string
	SystemUpdateInterval   time.Duration
	UpdaterURL             string
	UpdaterToken           string
	GithubClientID         string
	GithubClientSecret     string
	GoogleClientID         string
	GoogleClientSecret     string
}

func Load() Config {
	dockerHost := value("GAMEPANEL_DOCKER_HOST", value("DOCKER_HOST", ""))
	if dockerHost == "" {
		dockerHost = "unix:///var/run/docker.sock"
	}
	queryTimeout := 2 * time.Second
	if raw := value("GAMEPANEL_PROMETHEUS_QUERY_TIMEOUT", ""); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed > 0 {
			queryTimeout = parsed
		}
	}
	updateInterval := 24 * time.Hour
	if raw := value("GAMEPANEL_SYSTEM_UPDATE_INTERVAL", ""); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil && parsed >= time.Hour {
			updateInterval = parsed
		}
	}
	maxConnections, _ := strconv.Atoi(value("GAMEPANEL_DB_MAX_CONNECTIONS", "20"))
	if maxConnections <= 0 {
		maxConnections = 20
	}
	uploadMax, _ := strconv.ParseInt(value("GAMEPANEL_MOD_UPLOAD_MAX_BYTES", ""), 10, 64)
	if uploadMax <= 0 {
		uploadMax = DefaultModUploadMaxBytes
	}
	return Config{
		ModUploadMaxBytes:      uploadMax,
		DatabaseURL:            value("GAMEPANEL_DATABASE_URL", ""),
		DBMaxConnections:       maxConnections,
		Host:                   value("GAMEPANEL_HOST", "0.0.0.0"),
		Port:                   value("GAMEPANEL_PORT", "4000"),
		DataDir:                value("GAMEPANEL_DATA_DIR", "./data"),
		DBPath:                 value("GAMEPANEL_DB_PATH", "./data/gamepanel.db"),
		DockerHost:             dockerHost,
		PublicHost:             value("GAMEPANEL_PUBLIC_HOST", ""),
		ProviderCatalogPath:    value("GAMEPANEL_PROVIDER_CATALOG_PATH", "./config/providers.json"),
		ImageRegion:            value("GAMEPANEL_IMAGE_REGION", "global"),
		ImageRegistry:          value("GAMEPANEL_IMAGE_REGISTRY", "smartcat99999"),
		ImageTag:               value("GAMEPANEL_IMAGE_TAG", "v0.2.13"),
		PrometheusURL:          value("GAMEPANEL_PROMETHEUS_URL", ""),
		PrometheusQueryTimeout: queryTimeout,
		ReleaseManifestURL:     value("GAMEPANEL_RELEASE_MANIFEST_URL", "https://github.com/smartcat999/game-panel-lite/releases/latest/download/manifest.json"),
		SystemUpdateInterval:   updateInterval,
		UpdaterURL:             value("GAMEPANEL_UPDATER_URL", ""),
		UpdaterToken:           value("GAMEPANEL_UPDATER_TOKEN", ""),
		GithubClientID:         value("GITHUB_CLIENT_ID", value("GAMEPANEL_GITHUB_CLIENT_ID", "")),
		GithubClientSecret:     value("GITHUB_CLIENT_SECRET", value("GAMEPANEL_GITHUB_CLIENT_SECRET", "")),
		GoogleClientID:         value("GOOGLE_CLIENT_ID", value("GAMEPANEL_GOOGLE_CLIENT_ID", "")),
		GoogleClientSecret:     value("GOOGLE_CLIENT_SECRET", value("GAMEPANEL_GOOGLE_CLIENT_SECRET", "")),
	}
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

func value(key string, fallback string) string {
	if got := os.Getenv(key); got != "" {
		return got
	}
	return fallback
}

func (c Config) ModUploadLimit() int64 {
	if c.ModUploadMaxBytes > 0 {
		return c.ModUploadMaxBytes
	}
	return DefaultModUploadMaxBytes
}
