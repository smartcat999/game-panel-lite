package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/archive"
	"github.com/smartcat999/game-panel-lite/internal/worker"
)

type snapshotRuntimeFunc func(context.Context, worker.State, func(context.Context, fs.FS) error) error

func (f snapshotRuntimeFunc) ReadStoppedData(ctx context.Context, state worker.State, read func(context.Context, fs.FS) error) error {
	return f(ctx, state, read)
}

func TestPrepareStoppedSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	observed := worker.State{Exists: true, ID: "runtime", UID: "assignment", ServerID: "server", NodeID: "node", Generation: 1, Managed: true}
	source := fstest.MapFS{"worlds/save": {Data: []byte("stable world")}, "runtime": {Data: []byte("excluded")}}
	o := snapshotOptions{directory: t.TempDir(), subtree: "worlds", maxBytes: 4096, metadata: archive.Metadata{FormatVersion: 1, GameKey: "game", ProviderKey: "provider", ConfigVersion: 1}}
	runtime := snapshotRuntimeFunc(func(ctx context.Context, got worker.State, read func(context.Context, fs.FS) error) error {
		if got != observed {
			t.Fatal("snapshot identity changed")
		}
		return read(ctx, source)
	})
	snapshot, err := prepareStoppedSnapshot(ctx, runtime, observed, o, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	name := snapshot.file.Name()
	defer snapshot.Close()
	encoded := make([]byte, snapshot.size)
	if _, err := snapshot.ReadAt(encoded, 0); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	if snapshot.sha256 != hex.EncodeToString(digest[:]) || snapshot.size <= 0 || snapshot.size > o.maxBytes {
		t.Fatal("invalid archive receipt")
	}
	reader, err := zip.NewReader(snapshot, snapshot.size)
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 2 {
		t.Fatal("scope not preserved")
	}
	file, err := reader.Open("worlds/save")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	file.Close()
	if err != nil || string(data) != "stable world" {
		t.Fatal("world content changed")
	}
	if _, err := snapshot.file.Write([]byte("mutation")); err == nil {
		t.Fatal("returned archive is writable")
	}
	if err := snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("closed snapshot remains on disk")
	}
}

func TestSnapshotFailureCleanup(t *testing.T) {
	observed := worker.State{ServerID: "server"}
	for _, mode := range []string{"denied", "denied after lock", "revoked", "runtime changed", "too large", "cancelled", "no deadline"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			o := snapshotOptions{directory: t.TempDir(), subtree: ".", maxBytes: 4096, metadata: archive.Metadata{FormatVersion: 1, GameKey: "game", ProviderKey: "provider", ConfigVersion: 1}}
			denied := false
			authorize := func(context.Context) error {
				if mode == "denied" || denied {
					return errors.New("authority lost")
				}
				return nil
			}
			runtime := snapshotRuntimeFunc(func(ctx context.Context, _ worker.State, read func(context.Context, fs.FS) error) error {
				if mode == "denied after lock" {
					denied = true
				}
				err := read(ctx, fstest.MapFS{"world": {Data: []byte("world")}})
				if mode == "revoked" {
					denied = true
				}
				if mode == "runtime changed" {
					return errors.New("runtime identity changed")
				}
				if mode == "cancelled" {
					cancel()
				}
				return err
			})
			if mode == "too large" {
				o.maxBytes = 8
			}
			if mode == "no deadline" {
				ctx = context.Background()
			}
			snapshot, err := prepareStoppedSnapshot(ctx, runtime, observed, o, authorize)
			if err == nil || snapshot != nil {
				t.Fatal("failed snapshot published")
			}
			entries, err := os.ReadDir(o.directory)
			if err != nil || len(entries) != 0 {
				t.Fatal("partial snapshot retained")
			}
		})
	}
}
