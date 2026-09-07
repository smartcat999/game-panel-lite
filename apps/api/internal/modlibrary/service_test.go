package modlibrary

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modfiles "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modruntime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

type uploadPlugin struct{ terraria.TModLoaderProvider }

func (uploadPlugin) Key() domain.ProviderKey { return "upload-test-plugin" }
func (uploadPlugin) InspectMod(r io.Reader) (domain.ModMetadata, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return domain.ModMetadata{}, err
	}
	if string(data) != "valid" {
		return domain.ModMetadata{}, errors.New("bad package")
	}
	return domain.ModMetadata{Name: "Plugin metadata", Version: "1"}, nil
}

type repositoryStub struct {
	deny      error
	outcome   domain.LibraryCommitOutcome
	commitErr error
	commits   int
	saved     domain.ModFile
}

func (r *repositoryStub) CheckLibraryWriter(context.Context, string, string) error { return r.deny }
func (r *repositoryStub) CommitLibraryUpload(_ context.Context, _ string, item *domain.ModFile) (domain.LibraryCommitOutcome, error) {
	r.commits++
	r.saved = *item
	return r.outcome, r.commitErr
}
func uploadFixture(t *testing.T, repo *repositoryStub) (*Service, *modfiles.Service, domain.ProviderKey) {
	t.Helper()
	plugin := uploadPlugin{terraria.NewTModLoaderProvider()}
	registry, err := provider.NewRegistry(plugin)
	if err != nil {
		t.Fatal(err)
	}
	files := modfiles.NewService(t.TempDir(), modruntime.NewService(registry, nil).StoredFileName)
	return NewService(repo, files, registry), files, plugin.Key()
}
func TestUploadCommitOutcomes(t *testing.T) {
	for _, outcome := range []domain.LibraryCommitOutcome{domain.LibraryCommitApplied, domain.LibraryCommitRejected, domain.LibraryCommitUncertain} {
		t.Run(string(outcome), func(t *testing.T) {
			repo := &repositoryStub{outcome: outcome}
			if outcome != domain.LibraryCommitApplied {
				repo.commitErr = errors.New("database failure")
			}
			svc, files, key := uploadFixture(t, repo)
			item, err := svc.Upload(context.Background(), "user", "space", key, "same.tmod", strings.NewReader("valid"), 100)
			if repo.commits != 1 || repo.saved.ModName != "Plugin metadata" || repo.saved.ContentHash == "" || repo.saved.SizeBytes != 5 {
				t.Fatalf("metadata not prepared: %+v", repo)
			}
			if (err == nil) != (outcome == domain.LibraryCommitApplied) {
				t.Fatalf("commit error: %v", err)
			}
			if errors.Is(err, ErrRetainedUpload) != (outcome == domain.LibraryCommitUncertain) {
				t.Fatalf("retention classification: %v", err)
			}
			file, openErr := files.OpenLibrary(item)
			if outcome == domain.LibraryCommitRejected {
				if !os.IsNotExist(openErr) {
					t.Fatalf("rejected file retained: %v", openErr)
				}
			} else {
				if openErr != nil {
					t.Fatal(openErr)
				}
				file.Close()
			}
		})
	}
}
func TestUploadRejectsBeforeConsumptionAndCleansInvalidPackages(t *testing.T) {
	denied := errors.New("not authorized")
	repo := &repositoryStub{deny: denied}
	svc, files, key := uploadFixture(t, repo)
	if _, err := svc.Upload(context.Background(), "user", "space", key, "same.tmod", neverRead{t}, 100); !errors.Is(err, denied) {
		t.Fatalf("authorization: %v", err)
	}
	repo.deny = nil
	for _, body := range []string{"invalid", ""} {
		item, err := svc.Upload(context.Background(), "user", "space", key, "same.tmod", strings.NewReader(body), 100)
		if !errors.Is(err, ErrInvalidUpload) || repo.commits != 0 {
			t.Fatalf("invalid package committed: %v", err)
		}
		if file, err := files.OpenLibrary(item); !os.IsNotExist(err) {
			if file != nil {
				file.Close()
			}
			t.Fatalf("invalid file retained: %v", err)
		}
	}
}

type neverRead struct{ t *testing.T }

func (r neverRead) Read([]byte) (int, error) {
	r.t.Fatal("unauthorized body consumed")
	return 0, io.EOF
}

type cleanupFailure struct{ Files }

func (cleanupFailure) RemoveLibrary(domain.ModFile) error { return errors.New("disk unavailable") }
func TestFailedCleanupRetainsUploadID(t *testing.T) {
	repo := &repositoryStub{outcome: domain.LibraryCommitRejected, commitErr: errors.New("revoked")}
	svc, files, key := uploadFixture(t, repo)
	svc.files = cleanupFailure{files}
	item, err := svc.Upload(context.Background(), "user", "space", key, "same.tmod", strings.NewReader("valid"), 100)
	if item.ID == "" || !errors.Is(err, ErrRetainedUpload) {
		t.Fatalf("missing recovery identity: %+v %v", item, err)
	}
	file, err := files.OpenLibrary(item)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
}
