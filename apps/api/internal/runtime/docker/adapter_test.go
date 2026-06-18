package docker

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDataBindsUsesAbsoluteHostPath(t *testing.T) {
	binds := dataBinds("data/instances/example", []string{"/data"})
	if len(binds) != 1 {
		t.Fatalf("expected one bind, got %v", binds)
	}
	host, _, ok := strings.Cut(binds[0], ":")
	if !ok {
		t.Fatalf("expected host:container bind, got %q", binds[0])
	}
	if !filepath.IsAbs(host) {
		t.Fatalf("expected absolute host path, got %q", host)
	}
}

func TestDataBindsSupportsSubPathMounts(t *testing.T) {
	binds := dataBinds("data/instances/example", []string{"Worlds:/home/container/Worlds"})
	if len(binds) != 1 {
		t.Fatalf("expected one bind, got %v", binds)
	}
	host, container, ok := strings.Cut(binds[0], ":")
	if !ok {
		t.Fatalf("expected host:container bind, got %q", binds[0])
	}
	if !filepath.IsAbs(host) {
		t.Fatalf("expected absolute host path, got %q", host)
	}
	if filepath.Base(host) != "Worlds" {
		t.Fatalf("expected host bind to target Worlds subdir, got %q", host)
	}
	if container != "/home/container/Worlds" {
		t.Fatalf("expected container bind path, got %q", container)
	}
}

func TestNatPortSetExposesContainerPort(t *testing.T) {
	ports := natPortSet(7777, "")
	if _, ok := ports["7777/tcp"]; !ok {
		t.Fatalf("expected exposed 7777/tcp port, got %v", ports)
	}
}

func TestNatPortSetSupportsUdp(t *testing.T) {
	ports := natPortSet(8211, "udp")
	if _, ok := ports["8211/udp"]; !ok {
		t.Fatalf("expected exposed 8211/udp port, got %v", ports)
	}
}

func TestConsumeImagePullSuccess(t *testing.T) {
	stream := strings.NewReader(`{"status":"Pulling from smartcat99999/terraria-vanilla"}
{"status":"Digest: sha256:example"}
`)
	if err := consumeImagePull(stream); err != nil {
		t.Fatalf("expected successful pull stream, got %v", err)
	}
}

func TestConsumeImagePullReturnsStreamError(t *testing.T) {
	stream := strings.NewReader(`{"error":"manifest unknown: manifest unknown"}`)
	err := consumeImagePull(stream)
	if err == nil {
		t.Fatal("expected pull stream error")
	}
	if !strings.Contains(err.Error(), "manifest unknown") {
		t.Fatalf("expected manifest error, got %v", err)
	}
}

func TestConsumeImagePullReturnsErrorDetail(t *testing.T) {
	stream := strings.NewReader(`{"errorDetail":{"message":"no matching manifest for linux/arm64/v8 in the manifest list entries"},"error":"no matching manifest"}`)
	err := consumeImagePull(stream)
	if err == nil {
		t.Fatal("expected pull stream error detail")
	}
	if !strings.Contains(err.Error(), "no matching manifest") {
		t.Fatalf("expected platform error, got %v", err)
	}
}
