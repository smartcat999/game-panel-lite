package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Host       string
	Port       string
	DataDir    string
	DBPath     string
	DockerHost string
}

type DockerHostCandidate struct {
	Host   string `json:"host"`
	Label  string `json:"label"`
	Source string `json:"source"`
	Exists bool   `json:"exists"`
	Active bool   `json:"active"`
}

func Load() Config {
	return Config{
		Host:       value("GAMEPANEL_HOST", "0.0.0.0"),
		Port:       value("GAMEPANEL_PORT", "4000"),
		DataDir:    value("GAMEPANEL_DATA_DIR", "./data"),
		DBPath:     value("GAMEPANEL_DB_PATH", "./data/gamepanel.db"),
		DockerHost: value("GAMEPANEL_DOCKER_HOST", value("DOCKER_HOST", "unix:///var/run/docker.sock")),
	}
}

func DockerHostCandidates(currentHost string) []DockerHostCandidate {
	seen := map[string]bool{}
	candidates := make([]DockerHostCandidate, 0, 8)
	add := func(host, label, source string) {
		if host == "" || seen[host] {
			return
		}
		seen[host] = true
		candidates = append(candidates, DockerHostCandidate{
			Host:   host,
			Label:  label,
			Source: source,
			Exists: dockerHostExists(host),
			Active: host == currentHost,
		})
	}

	add(currentHost, "Current configured host", "current")
	add(os.Getenv("GAMEPANEL_DOCKER_HOST"), "GAMEPANEL_DOCKER_HOST", "env")
	add(os.Getenv("DOCKER_HOST"), "DOCKER_HOST", "env")
	add("unix:///var/run/docker.sock", "Docker Engine default", "common")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		add("unix://"+filepath.Join(home, ".docker", "run", "docker.sock"), "Docker Desktop user socket", "common")
		add("unix://"+filepath.Join(home, ".colima", "default", "docker.sock"), "Colima default socket", "common")
		add("unix://"+filepath.Join(home, ".rd", "docker.sock"), "Rancher Desktop socket", "common")
		add("unix://"+filepath.Join(home, ".orbstack", "run", "docker.sock"), "OrbStack socket", "common")
	}
	add("tcp://127.0.0.1:2375", "Local TCP daemon", "common")
	return candidates
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

func dockerHostExists(host string) bool {
	path, ok := strings.CutPrefix(host, "unix://")
	if !ok || path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}
