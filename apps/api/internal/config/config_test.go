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

func TestModUploadSizePolicy(t *testing.T) {
	for _, raw := range []string{"", "0", "-1", "invalid"} {
		t.Setenv("GAMEPANEL_MOD_UPLOAD_MAX_BYTES", raw)
		if got := Load().ModUploadLimit(); got != DefaultModUploadMaxBytes {
			t.Fatalf("fallback %q: %d", raw, got)
		}
	}
	t.Setenv("GAMEPANEL_MOD_UPLOAD_MAX_BYTES", "12345")
	if got := Load().ModUploadLimit(); got != 12345 {
		t.Fatalf("configured limit: %d", got)
	}
}

func TestLogicalInstanceConfiguration(t *testing.T) {
	t.Setenv("GAMEPANEL_CONFIGURATION_KEYRING", "/run/secrets/configuration.json")
	t.Setenv("GAMEPANEL_FINGERPRINT_KEYRING", "/run/secrets/fingerprint.json")
	t.Setenv("GAMEPANEL_LOGICAL_CONFIG_MAX_BYTES", "2048")
	cfg := Load()
	if cfg.ConfigurationKeyringPath != "/run/secrets/configuration.json" || cfg.FingerprintKeyringPath != "/run/secrets/fingerprint.json" || cfg.LogicalConfigMaxBytes != 2048 {
		t.Fatalf("logical instance configuration: %+v", cfg)
	}
	t.Setenv("GAMEPANEL_LOGICAL_CONFIG_MAX_BYTES", "999999999")
	if got := Load().LogicalConfigMaxBytes; got != DefaultLogicalConfigMaxBytes {
		t.Fatalf("invalid maximum did not fall back: %d", got)
	}
}
