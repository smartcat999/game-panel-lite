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

func TestDockerHostCandidatesIncludesCurrentAndCommonHosts(t *testing.T) {
	t.Setenv("GAMEPANEL_DOCKER_HOST", "unix:///tmp/gamepanel-custom.sock")
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375")

	candidates := DockerHostCandidates("unix:///tmp/current.sock")
	hosts := map[string]DockerHostCandidate{}
	for _, candidate := range candidates {
		hosts[candidate.Host] = candidate
	}

	if !hosts["unix:///tmp/current.sock"].Active {
		t.Fatalf("expected current host to be active")
	}
	if _, ok := hosts["unix:///tmp/gamepanel-custom.sock"]; !ok {
		t.Fatalf("expected GAMEPANEL_DOCKER_HOST candidate")
	}
	if _, ok := hosts["tcp://127.0.0.1:2375"]; !ok {
		t.Fatalf("expected DOCKER_HOST/local TCP candidate")
	}
}
