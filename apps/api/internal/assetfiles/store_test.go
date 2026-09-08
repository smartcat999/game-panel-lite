package assetfiles_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assetfiles"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
)

func manifest(body string) assets.PublishedVersion {
	digest := sha256.Sum256([]byte(body))
	return assets.PublishedVersion{OrganizationID: "tenant", AssetID: "world", Version: "v1", SHA256: hex.EncodeToString(digest[:]), SizeBytes: int64(len(body))}
}

func TestVerifiedPublicationAndIsolation(t *testing.T) {
	directory := t.TempDir()
	s, err := assetfiles.New(directory, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	v := manifest("world-data")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Put(ctx, v, io.NopCloser(strings.NewReader("world-data"))); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := s.Put(ctx, v, io.NopCloser(strings.NewReader("wrong-data"))); err == nil {
		t.Fatal("invalid replacement accepted")
	}
	f, err := s.Open(ctx, v)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(f)
	f.Close()
	if err != nil || string(body) != "world-data" {
		t.Fatal("published bytes differ")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary files remain")
	}
	for _, mutate := range []func(*assets.PublishedVersion){func(v *assets.PublishedVersion) { v.OrganizationID = "foreign" }, func(v *assets.PublishedVersion) { v.Version = "v2" }, func(v *assets.PublishedVersion) { v.AssetID = "other" }} {
		other := v
		mutate(&other)
		if f, err := s.Open(ctx, other); err == nil {
			f.Close()
			t.Fatal("different identity read cached file")
		}
	}
	path := filepath.Join(directory, entries[0].Name())
	if err := os.WriteFile(path, []byte("wrong-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if f, err := s.Open(ctx, v); err == nil {
		f.Close()
		t.Fatal("corrupt cache accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("world-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if f, err := s.Open(ctx, v); err == nil {
		f.Close()
		t.Fatal("symlink used as asset")
	}
}

type failingSource struct {
	io.Reader
	closeErr error
}

func (s failingSource) Close() error { return s.closeErr }

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("source failed") }

func TestFailedContentNeverPublished(t *testing.T) {
	for name, source := range map[string]io.ReadCloser{
		"short":  io.NopCloser(strings.NewReader("data")),
		"long":   io.NopCloser(strings.NewReader("world-data-extra")),
		"digest": io.NopCloser(strings.NewReader("wrong-data")),
		"read":   io.NopCloser(errorReader{}),
		"close":  failingSource{strings.NewReader("world-data"), errors.New("close failed")},
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			s, err := assetfiles.New(directory, 1024)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err := s.Put(context.Background(), manifest("world-data"), source); err == nil {
				t.Fatal("bad source published")
			}
			entries, err := os.ReadDir(directory)
			if err != nil || len(entries) != 0 {
				t.Fatal("failed publication left bytes")
			}
		})
	}
}

type blockedSource struct {
	entered, closed chan struct{}
	once            sync.Once
}

func (s *blockedSource) Read([]byte) (int, error) {
	close(s.entered)
	<-s.closed
	return 0, io.ErrClosedPipe
}
func (s *blockedSource) Close() error { s.once.Do(func() { close(s.closed) }); return nil }

func TestCancellationUnblocksSourceAndCleansStage(t *testing.T) {
	directory := t.TempDir()
	s, err := assetfiles.New(directory, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &blockedSource{entered: make(chan struct{}), closed: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- s.Put(ctx, manifest("world-data"), source) }()
	select {
	case <-source.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("read not started")
	}
	if f, err := s.Open(ctx, manifest("world-data")); err == nil {
		f.Close()
		t.Fatal("partial stage readable")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("source did not unblock")
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatal("cancelled stage remains")
	}
}

func TestEmptyContentLimitAndRestart(t *testing.T) {
	directory := t.TempDir()
	s, err := assetfiles.New(directory, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), manifest("ab"), io.NopCloser(strings.NewReader("ab"))); !errors.Is(err, assets.ErrInvalidVersion) {
		t.Fatal("configured limit ignored")
	}
	if err := s.Put(context.Background(), manifest(""), io.NopCloser(strings.NewReader(""))); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = assetfiles.New(directory, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f, err := s.Open(context.Background(), manifest(""))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil || len(data) != 0 {
		t.Fatal("empty asset did not survive reopen")
	}
}
