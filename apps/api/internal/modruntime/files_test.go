package modruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

type filePlugin struct {
	terraria.TModLoaderProvider
	paths []string
}

func (p filePlugin) RuntimeModFiles(string) []string { return p.paths }

func TestInstallAndRemoveProviderFiles(t *testing.T) {
	p := filePlugin{terraria.NewTModLoaderProvider(), []string{"mods/a", "secondary/a"}}
	svc := NewService(registry(t, p), nil)
	root := t.TempDir()
	for _, data := range []string{"old", "new"} {
		if err := svc.Install(context.Background(), p.Key(), "a", root, strings.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		for _, name := range p.paths {
			info, err := os.Stat(filepath.Join(root, name))
			if err != nil || info.Mode().Perm() != 0666 {
				t.Fatalf("runtime file permissions: %v %v", info, err)
			}
			got, err := os.ReadFile(filepath.Join(root, name))
			if err != nil || string(got) != data {
				t.Fatalf("%s=%q err=%v", name, got, err)
			}
		}
	}
	for i := 0; i < 2; i++ {
		if err := svc.Remove(context.Background(), p.Key(), "a", root); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range p.paths {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("not removed: %v", err)
		}
	}
}

func TestInstallRejectsEscapingPaths(t *testing.T) {
	for _, path := range []string{"../outside", "linked/outside"} {
		t.Run(path, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "outside"), []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
				t.Fatal(err)
			}
			p := filePlugin{terraria.NewTModLoaderProvider(), []string{"valid", path}}
			svc := NewService(registry(t, p), nil)
			if err := svc.Install(context.Background(), p.Key(), "a", root, strings.NewReader("bad")); err == nil {
				t.Fatal("escape accepted")
			}
			if _, err := os.Stat(filepath.Join(root, "valid")); !os.IsNotExist(err) {
				t.Fatalf("partial publication: %v", err)
			}
			if err := svc.Remove(context.Background(), p.Key(), "a", root); err == nil {
				t.Fatal("escaping removal accepted")
			}
			got, err := os.ReadFile(filepath.Join(outside, "outside"))
			if err != nil || string(got) != "keep" {
				t.Fatalf("outside modified: %q %v", got, err)
			}
		})
	}
}

type failingModReader struct{ failure error }

func (r failingModReader) Seek(int64, int) (int64, error) { return 0, nil }
func (r failingModReader) Read([]byte) (int, error)       { return 0, r.failure }

func TestInstallFailurePreservesExistingFileAndCleansStaging(t *testing.T) {
	p := filePlugin{terraria.NewTModLoaderProvider(), []string{"mod"}}
	svc := NewService(registry(t, p), nil)
	for _, cancelled := range []bool{false, true} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "mod"), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		want := io.ErrUnexpectedEOF
		if cancelled {
			cancel()
			want = context.Canceled
		}
		err := svc.Install(ctx, p.Key(), "mod", root, failingModReader{io.ErrUnexpectedEOF})
		cancel()
		if !errors.Is(err, want) {
			t.Fatalf("err=%v want=%v", err, want)
		}
		got, err := os.ReadFile(filepath.Join(root, "mod"))
		if err != nil || string(got) != "keep" {
			t.Fatalf("old file=%q err=%v", got, err)
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 1 {
			t.Fatalf("staging leak: %v %v", entries, err)
		}
	}
}
