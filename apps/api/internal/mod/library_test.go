package mod

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func libraryItem() domain.ModFile {
	return domain.ModFile{ID: "one", OrganizationID: "Tenant", InstanceID: "unassigned", ProviderKey: domain.ProviderTerrariaTModLoader, FileName: "same.tmod"}
}
func TestLibraryFilesIsolateIdentitiesAndPublishOnce(t *testing.T) {
	svc := NewService(t.TempDir(), fixturePolicy)
	item := libraryItem()
	variants := []domain.ModFile{item, item, item, item}
	variants[1].OrganizationID = "tenant" // Must also differ on case-insensitive filesystems.
	variants[2].ProviderKey = "another-provider"
	variants[3].ID = "two"
	for i, record := range variants {
		content := fmt.Sprintf("content-%d", i)
		result, err := svc.PutLibrary(context.Background(), record, strings.NewReader(content), 100)
		if err != nil || result.SizeBytes != int64(len(content)) || result.ContentHash != fmt.Sprintf("%x", sha256.Sum256([]byte(content))) {
			t.Fatalf("publish: %+v %v", result, err)
		}
	}
	for i, record := range variants {
		file, err := svc.Open(record)
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(file)
		file.Close()
		if err != nil || string(content) != fmt.Sprintf("content-%d", i) {
			t.Fatalf("isolation: %q %v", content, err)
		}
	}
	if _, err := svc.PutLibrary(context.Background(), item, strings.NewReader("replacement"), 100); !os.IsExist(err) {
		t.Fatalf("overwrite: %v", err)
	}
	renamed := item
	renamed.FileName = "renamed.tmod"
	if _, err := svc.PutLibrary(context.Background(), renamed, strings.NewReader("replacement"), 100); !os.IsExist(err) {
		t.Fatalf("rename overwrite: %v", err)
	}
	if err := svc.Remove(item); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Open(item); !os.IsNotExist(err) {
		t.Fatalf("deleted read: %v", err)
	}
	if file, err := svc.Open(variants[1]); err != nil {
		t.Fatal(err)
	} else {
		file.Close()
	}
	if err := svc.Remove(item); err != nil {
		t.Fatalf("idempotent remove: %v", err)
	}
}
func TestLibraryFailedUploadsDoNotPublish(t *testing.T) {
	svc := NewService(t.TempDir(), fixturePolicy)
	item := libraryItem()
	if _, err := svc.PutLibrary(context.Background(), item, strings.NewReader("12345"), 4); err == nil {
		t.Fatal("oversize accepted")
	}
	if _, err := svc.Open(item); !os.IsNotExist(err) {
		t.Fatalf("partial publication: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelReader{cancel: cancel}
	if _, err := svc.PutLibrary(ctx, item, reader, 100); err != context.Canceled {
		t.Fatalf("canceled upload: %v", err)
	}
	root, err := svc.libraryRoot(item, false)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := root.Open(".")
	root.Close()
	if err != nil {
		t.Fatal(err)
	}
	names, err := dir.Readdirnames(-1)
	dir.Close()
	if err != nil || len(names) != 0 {
		t.Fatalf("temporary files remain: %v %v", names, err)
	}
	if _, err := svc.PutLibrary(context.Background(), item, strings.NewReader("1234"), 4); err != nil {
		t.Fatalf("exact limit: %v", err)
	}
}

type cancelReader struct{ cancel context.CancelFunc }

func (r *cancelReader) Read(p []byte) (int, error) { r.cancel(); return copy(p, "partial"), nil }
func TestLibraryConcurrentPublication(t *testing.T) {
	svc := NewService(t.TempDir(), fixturePolicy)
	item := libraryItem()
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.PutLibrary(context.Background(), item, strings.NewReader("complete"), 100)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !os.IsExist(err) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful publications: %d", successes)
	}
}
func TestLibraryRejectsSymlinkScopesAndContents(t *testing.T) {
	for _, component := range []string{"mod-library", "organization", "provider", "record", "content"} {
		t.Run(component, func(t *testing.T) {
			base := t.TempDir()
			outside := t.TempDir()
			svc := NewService(base, fixturePolicy)
			item := libraryItem()
			components := []string{"mod-library", libraryKey(item.OrganizationID), libraryKey(string(item.ProviderKey)), libraryKey(item.ID), "content"}
			index := map[string]int{"mod-library": 0, "organization": 1, "provider": 2, "record": 3, "content": 4}[component]
			link := filepath.Join(append([]string{base}, components[:index+1]...)...)
			if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
				t.Fatal(err)
			}
			target := outside
			if component == "content" {
				target = filepath.Join(outside, "victim")
				if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.PutLibrary(context.Background(), item, strings.NewReader("bad"), 100); err == nil {
				t.Fatal("symlink upload allowed")
			}
			if file, err := svc.OpenLibrary(item); err == nil {
				file.Close()
				t.Fatal("symlink read allowed")
			}
			if err := svc.RemoveLibrary(item); err == nil {
				t.Fatal("symlink delete allowed")
			}
			if component == "content" {
				data, err := os.ReadFile(target)
				if err != nil || string(data) != "private" {
					t.Fatalf("victim changed: %s %v", data, err)
				}
			}
		})
	}
}
func TestLibraryRejectsInvalidOwnershipAndFilename(t *testing.T) {
	svc := NewService(t.TempDir(), fixturePolicy)
	item := libraryItem()
	variants := []domain.ModFile{item, item, item, item, item}
	variants[0].OrganizationID = ""
	variants[1].ID = ""
	variants[2].InstanceID = "server"
	variants[3].ProviderKey = ""
	variants[4].FileName = "../bad.tmod"
	for _, record := range variants {
		if _, err := svc.PutLibrary(context.Background(), record, strings.NewReader("bad"), 100); err == nil {
			t.Fatalf("invalid record accepted: %+v", record)
		}
	}
	if _, err := svc.OpenLibrary(item); !os.IsNotExist(err) {
		t.Fatalf("missing read: %v", err)
	}
	entries, err := os.ReadDir(svc.dataDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("reads created directories: %v %v", entries, err)
	}
}

func TestLibraryPublicationAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type result struct {
		output []byte
		err    error
	}
	results := make(chan result, 4)
	for i := 0; i < 4; i++ {
		go func() {
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLibraryPublishProcess$")
			cmd.Env = append(os.Environ(), "GAMEPANEL_TEST_LIBRARY_PUBLISH_DIR="+dir)
			output, err := cmd.CombinedOutput()
			results <- result{output, err}
		}()
	}
	successes := 0
	for i := 0; i < 4; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("publication process: %v %s", r.err, r.output)
		}
		if strings.Contains(string(r.output), "LIBRARY_PUBLISHED") {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful processes: %d", successes)
	}
	file, err := NewService(dir, fixturePolicy).OpenLibrary(libraryItem())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "complete-process-content" {
		t.Fatalf("published content: %q %v", data, err)
	}
}
func TestLibraryPublishProcess(t *testing.T) {
	dir := os.Getenv("GAMEPANEL_TEST_LIBRARY_PUBLISH_DIR")
	if dir == "" {
		t.Skip("subprocess publication helper")
	}
	_, err := NewService(dir, fixturePolicy).PutLibrary(context.Background(), libraryItem(), strings.NewReader("complete-process-content"), 100)
	if err == nil {
		fmt.Println("LIBRARY_PUBLISHED")
		return
	}
	if !os.IsExist(err) {
		t.Fatal(err)
	}
}
