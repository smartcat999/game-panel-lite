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
