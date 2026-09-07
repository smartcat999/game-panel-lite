package modlibrary

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type deliveryRepoStub struct {
	item  domain.ModFile
	ref   workload.Artifact
	calls int
	mode  string
}

func (r *deliveryRepoStub) ResolveArtifactForNode(context.Context, string, string, int, string) (domain.ModFile, workload.Artifact, error) {
	r.calls++
	if r.mode == "denied" || r.mode == "revoked" && r.calls == 2 {
		return domain.ModFile{}, workload.Artifact{}, errors.New("denied")
	}
	item := r.item
	if r.mode == "changed" && r.calls == 2 {
		item.OrganizationID = "another-workspace"
	}
	return item, r.ref, nil
}

type deliveryFilesStub struct {
	path   string
	opened *os.File
}

func (f *deliveryFilesStub) OpenLibrary(domain.ModFile) (*os.File, error) {
	file, err := os.Open(f.path)
	f.opened = file
	return file, err
}
func TestDeliveryRechecksAuthorizationAfterOpening(t *testing.T) {
	for _, mode := range []string{"success", "denied", "revoked", "changed", "size"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "content")
			if err := os.WriteFile(path, []byte("bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			repo := &deliveryRepoStub{mode: mode, item: domain.ModFile{ID: "source", OrganizationID: "space", ProviderKey: "provider", FileName: "file", ContentHash: strings.Repeat("a", 64), SizeBytes: 5}, ref: workload.Artifact{ID: "source", SizeBytes: 5, SHA256: strings.Repeat("a", 64)}}
			if mode == "size" {
				repo.ref.SizeBytes = 6
			}
			files := &deliveryFilesStub{path: path}
			file, _, err := NewDelivery(repo, files).Open(context.Background(), "node", "uid", 1, "source")
			if mode == "success" {
				if err != nil || file == nil || repo.calls != 2 {
					t.Fatalf("open: %v", err)
				}
				file.Close()
			} else {
				if err == nil || file != nil {
					t.Fatal("unauthorized handle returned")
				}
				if files.opened != nil {
					if _, err := files.opened.Stat(); !errors.Is(err, os.ErrClosed) {
						t.Fatalf("handle leaked: %v", err)
					}
				}
				if mode == "denied" && files.opened != nil {
					t.Fatal("opened file before authorization")
				}
			}
		})
	}
}
