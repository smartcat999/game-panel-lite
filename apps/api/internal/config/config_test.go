package config

import "testing"

func TestLoadUsesGamePanelDockerHost(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///ignored.sock")
	t.Setenv("GAMEPANEL_DOCKER_HOST", "unix:///custom/docker.sock")

	cfg := Load()

	if cfg.DockerHost != "unix:///custom/docker.sock" {
		t.Fatalf("expected GAMEPANEL_DOCKER_HOST to win, got %q", cfg.DockerHost)
	}
}

func TestLoadFallsBackToDockerHost(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375")

	cfg := Load()

	if cfg.DockerHost != "tcp://127.0.0.1:2375" {
		t.Fatalf("expected DOCKER_HOST fallback, got %q", cfg.DockerHost)
	}
}

func TestLoadUsesDockerDefaultHost(t *testing.T) {
	t.Setenv("GAMEPANEL_DOCKER_HOST", "")
	t.Setenv("DOCKER_HOST", "")

	cfg := Load()

	if cfg.DockerHost != "unix:///var/run/docker.sock" {
		t.Fatalf("expected Docker default host, got %q", cfg.DockerHost)
	}
}
