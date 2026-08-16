package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateEnvFileReplacesTagAndPreservesOtherValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("GAMEPANEL_IMAGE_REGISTRY=smartcat99999\nGAMEPANEL_IMAGE_TAG=v0.1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := updateEnvFile(path, "GAMEPANEL_IMAGE_TAG", "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "GAMEPANEL_IMAGE_REGISTRY=smartcat99999") || !strings.Contains(content, `GAMEPANEL_IMAGE_TAG="v0.2.0"`) {
		t.Fatalf("unexpected env file:\n%s", content)
	}
}

func TestNormalizeVersion(t *testing.T) {
	if got := normalizeVersion("1.2.3"); got != "v1.2.3" {
		t.Fatalf("normalizeVersion = %q", got)
	}
	if got := normalizeVersion("v1.2.3"); got != "v1.2.3" {
		t.Fatalf("normalizeVersion = %q", got)
	}
}
