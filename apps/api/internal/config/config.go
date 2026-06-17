package config

import (
	"fmt"
	"os"
)

type Config struct {
	Host       string
	Port       string
	DataDir    string
	DBPath     string
	DockerHost string
}

func Load() Config {
	dockerHost := value("GAMEPANEL_DOCKER_HOST", value("DOCKER_HOST", ""))
	if dockerHost == "" {
		dockerHost = "unix:///var/run/docker.sock"
	}
	return Config{
		Host:       value("GAMEPANEL_HOST", "0.0.0.0"),
		Port:       value("GAMEPANEL_PORT", "4000"),
		DataDir:    value("GAMEPANEL_DATA_DIR", "./data"),
		DBPath:     value("GAMEPANEL_DB_PATH", "./data/gamepanel.db"),
		DockerHost: dockerHost,
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
