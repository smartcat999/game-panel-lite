package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func TestLoadSeparateLogicalInstanceKeyrings(t *testing.T) {
	write := func(name string, fill byte) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), name)
		encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 32))
		if err := os.WriteFile(path, []byte(`{"active":"v1","keys":{"v1":"`+encoded+`"}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	protector, err := loadConfigurationProtector(write("configuration.json", 1), 1024)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := loadRequestFingerprinter(write("fingerprint.json", 2))
	if err != nil {
		t.Fatal(err)
	}
	binding := instances.ConfigurationBinding{OrganizationID: "tenant", ServerID: "server", RevisionID: "revision", SpecGeneration: 1, ProviderKey: "provider", ConfigSchemaVersion: 1}
	sealed, err := protector.Seal(context.Background(), binding, []byte(`{"secret":true}`))
	if err != nil || sealed.KeyID != "v1" {
		t.Fatalf("seal: %+v %v", sealed, err)
	}
	digest, err := fingerprint.Sum([]byte("request"))
	if err != nil || digest == "" {
		t.Fatalf("fingerprint: %q %v", digest, err)
	}
	invalid := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"active":"v1","keys":{},"extra":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfigurationProtector(invalid, 1024); err == nil {
		t.Fatal("invalid keyring accepted")
	}
}
