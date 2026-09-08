package backup

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreArchiveFromIndependentSource(t *testing.T) {
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	metadata := Metadata{FormatVersion: 1, GameKey: "game", ProviderKey: "provider", ConfigVersion: 2}
	if err := writeMetadata(w, metadata); err != nil {
		t.Fatal(err)
	}
	entry, err := w.Create("worlds/save.dat")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("saved world")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "compatibility denial", "commit failure", "truncated"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "worlds"), 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "worlds/save.dat")
			if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			validated, committed := false, false
			denied := errors.New("denied")
			hooks := RestoreHooks{
				Validate: func(got Metadata) error {
					validated = true
					if got != metadata {
						t.Fatalf("metadata = %+v", got)
					}
					if scenario == "compatibility denial" {
						return denied
					}
					return nil
				},
				Commit: func() error {
					committed = true
					got, err := os.ReadFile(path)
					if err != nil || string(got) != "saved world" {
						t.Fatal("commit before files published")
					}
					if scenario == "commit failure" {
						return denied
					}
					return nil
				},
			}
			data := archive.Bytes()
			if scenario == "truncated" {
				data = data[:len(data)-10]
			}
			err := RestoreArchiveChecked(bytes.NewReader(data), int64(len(data)), dir, hooks)
			want := "original"
			if scenario == "success" {
				want = "saved world"
				if err != nil || !validated || !committed {
					t.Fatalf("restore: %v", err)
				}
			} else if err == nil {
				t.Fatal("failure accepted")
			}
			if scenario == "commit failure" && !errors.Is(err, ErrCommit) {
				t.Fatalf("commit error: %v", err)
			}
			if scenario == "compatibility denial" && committed {
				t.Fatal("denied archive committed")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != want {
				t.Fatalf("world=%q error=%v", got, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("staging leaked: %v %v", entries, err)
			}
		})
	}
}
