package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Host                   string
	Port                   string
	DataDir                string
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
	return Config{
		Host:                   value("GAMEPANEL_HOST", "0.0.0.0"),
		Port:                   value("GAMEPANEL_PORT", "4000"),
		DataDir:                value("GAMEPANEL_DATA_DIR", "./data"),
		DBPath:                 value("GAMEPANEL_DB_PATH", "./data/gamepanel.db"),
		DockerHost:             dockerHost,
		PublicHost:             value("GAMEPANEL_PUBLIC_HOST", ""),
		ProviderCatalogPath:    value("GAMEPANEL_PROVIDER_CATALOG_PATH", "./config/providers.json"),
		ImageRegion:            value("GAMEPANEL_IMAGE_REGION", "global"),
		ImageRegistry:          value("GAMEPANEL_IMAGE_REGISTRY", "smartcat99999"),
		ImageTag:               value("GAMEPANEL_IMAGE_TAG", "v0.2.12"),
		PrometheusURL:          value("GAMEPANEL_PROMETHEUS_URL", ""),
		PrometheusQueryTimeout: queryTimeout,
		ReleaseManifestURL:     value("GAMEPANEL_RELEASE_MANIFEST_URL", "https://github.com/smartcat999/game-panel-lite/releases/latest/download/manifest.json"),
		SystemUpdateInterval:   updateInterval,
		UpdaterURL:             value("GAMEPANEL_UPDATER_URL", ""),
		UpdaterToken:           value("GAMEPANEL_UPDATER_TOKEN", ""),
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
