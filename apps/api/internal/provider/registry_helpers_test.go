package provider

import (
	"testing"
)

func mustRegistry(t *testing.T, providers ...GameProvider) *Registry {
	t.Helper()
	registry, err := NewRegistry(providers...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
