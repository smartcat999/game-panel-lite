package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurationKeyring(t *testing.T) {
	valid, _ := json.Marshal(map[string]any{"active": "key", "keys": map[string][]byte{"key": bytes.Repeat([]byte{1}, 32)}})
	for name, raw := range map[string][]byte{
		"valid": valid, "trailing": append(append([]byte{}, valid...), []byte(` {}`)...),
		"short":   []byte(`{"active":"key","keys":{"key":"YQ=="}}`),
		"unknown": []byte(`{"active":"key","keys":{},"password":"private"}`),
		"missing": []byte(`{"active":"absent","keys":{}}`), "oversized": bytes.Repeat([]byte("x"), (1<<20)+1),
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "keyring.json")
			if err := os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
			protector, err := loadConfigurationKeys(file, 4096)
			if name == "valid" {
				if err != nil || protector == nil {
					t.Fatal(err)
				}
				return
			}
			if protector != nil || err == nil || err.Error() != "cannot load configuration keyring" {
				t.Fatal("invalid or leaking keyring error", err)
			}
		})
	}
}
