package backup

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveMetadataRoundTripAndPreflight(t *testing.T) {
	service := NewService(t.TempDir())
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "world"), []byte("save"), 0600); err != nil {
		t.Fatal(err)
	}
	metadata := Metadata{FormatVersion: 1, GameKey: "test", ProviderKey: "test-provider", ConfigVersion: 2}
	path, _, err := service.WithMetadata(metadata).Create("instance", source)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	rejected := errors.New("incompatible archive")
	err = service.RestoreChecked("instance", filepath.Base(path), target, RestoreHooks{Validate: func(got Metadata) error {
		if got != metadata {
			t.Fatalf("metadata=%+v", got)
		}
		return rejected
	}})
	if !errors.Is(err, rejected) {
		t.Fatalf("rejection=%v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("preflight created target: %v", err)
	}
	if err := service.RestoreChecked("instance", filepath.Base(path), target, RestoreHooks{Validate: func(Metadata) error { return nil }}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "world"))
	if err != nil || string(data) != "save" {
		t.Fatalf("save=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(target, metadataPath)); !os.IsNotExist(err) {
		t.Fatal("metadata extracted into game data")
	}
}

func TestMetadataRejectsMalformedDuplicateAndOversizedEntries(t *testing.T) {
	for _, payloads := range [][]string{
		{`{}`},
		{`{"formatVersion":2,"configVersion":1,"providerKey":"p","gameKey":"g"}`},
		{`{"formatVersion":1,"providerKey":"p","gameKey":"g"}`},
		{strings.Repeat("x", maxMetadataBytes+1)},
		{`{"formatVersion":1,"configVersion":1,"providerKey":"p","gameKey":"g"}`, `{}`},
	} {
		path := filepath.Join(t.TempDir(), "archive.zip")
		out, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		writer := zip.NewWriter(out)
		for _, payload := range payloads {
			file, err := writer.Create(metadataPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = file.Write([]byte(payload)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := out.Close(); err != nil {
			t.Fatal(err)
		}
		reader, err := zip.OpenReader(path)
		if err != nil {
			t.Fatal(err)
		}
		_, err = readMetadata(reader.File)
		reader.Close()
		if err == nil {
			t.Fatalf("accepted malformed metadata: %v", payloads)
		}
	}
}

func TestRestoreCannotWriteThroughOutsideSymlink(t *testing.T) {
	service := NewService(t.TempDir())
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "linked"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "linked", "world"), []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	path, _, err := service.Create("instance", source)
	if err != nil {
		t.Fatal(err)
	}
	target, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(target, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := service.Restore("instance", filepath.Base(path), target); err == nil {
		t.Fatal("symlink escape accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "world")); !os.IsNotExist(err) {
		t.Fatalf("outside touched: %v", err)
	}
}
