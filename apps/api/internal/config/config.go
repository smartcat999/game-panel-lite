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
	PrometheusURL          string
	PrometheusQueryTimeout time.Duration
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
	return Config{
		Host:                   value("GAMEPANEL_HOST", "0.0.0.0"),
		Port:                   value("GAMEPANEL_PORT", "4000"),
		DataDir:                value("GAMEPANEL_DATA_DIR", "./data"),
		DBPath:                 value("GAMEPANEL_DB_PATH", "./data/gamepanel.db"),
		DockerHost:             dockerHost,
		PublicHost:             value("GAMEPANEL_PUBLIC_HOST", ""),
		ProviderCatalogPath:    value("GAMEPANEL_PROVIDER_CATALOG_PATH", "./config/providers.json"),
		PrometheusURL:          value("GAMEPANEL_PROMETHEUS_URL", ""),
		PrometheusQueryTimeout: queryTimeout,
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
