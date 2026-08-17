package main

import (
	"net/http"
	"net/http/httptest"
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

func TestParseComposeServicesSupportsArrayAndLineJSON(t *testing.T) {
	for _, input := range []string{
		`[{"Service":"api","State":"running","Health":"healthy","Image":"api:v1"}]`,
		"{\"Service\":\"api\",\"State\":\"running\",\"Health\":\"healthy\",\"Image\":\"api:v1\"}\n{\"Service\":\"web\",\"State\":\"exited\"}",
	} {
		services, err := parseComposeServices([]byte(input))
		if err != nil {
			t.Fatalf("parse compose services: %v", err)
		}
		if len(services) == 0 || services[0].Name != "api" || services[0].State != "running" {
			t.Fatalf("unexpected services: %#v", services)
		}
	}
}

func TestReadEnvValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("GAMEPANEL_DOMAIN=\"panel.example.com\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readEnvValue(path, "GAMEPANEL_DOMAIN"); got != "panel.example.com" {
		t.Fatalf("readEnvValue = %q", got)
	}
}

func TestSetupHTTPSRejectsInvalidDomain(t *testing.T) {
	u := &updater{workspace: t.TempDir()}
	request := httptest.NewRequest(http.MethodPost, "/deployment/https/setup", strings.NewReader(`{"domain":"example.com; rm -rf /"}`))
	response := httptest.NewRecorder()
	u.setupHTTPS(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestReadAutoRenewalStatus(t *testing.T) {
	workspace := t.TempDir()
	dir := filepath.Join(workspace, "data", "certbot")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "renewal-status.json"), []byte(`{"enabled":true,"method":"systemd","lastCheckedAt":"2026-08-17T00:00:00Z","lastStatus":"success"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	u := &updater{workspace: workspace}
	status := u.readAutoRenewalStatus()
	if !status.Enabled || status.Method != "systemd" || status.LastStatus != "success" {
		t.Fatalf("unexpected auto renewal status: %#v", status)
	}
}
