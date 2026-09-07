// Package modlibrary coordinates authorized workspace uploads without HTTP or
// runtime installation. Provider formats and physical file layout stay outside.
package modlibrary

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modfiles "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modruntime"
)

var (
	ErrInvalidUpload  = errors.New("invalid library upload")
	ErrRetainedUpload = errors.New("upload file retained for reconciliation")
)

type Repository interface {
	CheckLibraryWriter(context.Context, string, string) error
	CommitLibraryUpload(context.Context, string, *domain.ModFile) (domain.LibraryCommitOutcome, error)
}
type Files interface {
	PutLibrary(context.Context, domain.ModFile, io.Reader, int64) (modfiles.StoredFile, error)
	OpenLibrary(domain.ModFile) (*os.File, error)
	RemoveLibrary(domain.ModFile) error
}
type Service struct {
	repo      Repository
	files     Files
	providers modruntime.Registry
	metadata  *modruntime.Service
}

func NewService(repo Repository, files Files, providers modruntime.Registry) *Service {
	return &Service{repo: repo, files: files, providers: providers, metadata: modruntime.NewService(providers, nil)}
}

// Upload generates its own immutable file ID. On an error the returned ID may
// identify a retained file; callers must not interpret that record as committed.
func (s *Service) Upload(ctx context.Context, userID, orgID string, key domain.ProviderKey, name string, reader io.Reader, maxBytes int64) (domain.ModFile, error) {
	if userID == "" || orgID == "" || maxBytes <= 0 {
		return domain.ModFile{}, ErrInvalidUpload
	}
	if err := s.repo.CheckLibraryWriter(ctx, userID, orgID); err != nil {
		return domain.ModFile{}, err
	}
	safeName, err := s.metadata.UploadFileName(key, name)
	if err != nil {
		return domain.ModFile{}, fmt.Errorf("%w: %v", ErrInvalidUpload, err)
	}
	game, ok := s.providers.Get(key)
	if !ok {
		return domain.ModFile{}, ErrInvalidUpload
	}
	item := domain.ModFile{ID: uuid.NewString(), OrganizationID: orgID, InstanceID: "unassigned", ProviderKey: key, GameKey: game.GameKey(), FileName: safeName, Source: "upload", Enabled: true, CreatedAt: time.Now()}
	stored, err := s.files.PutLibrary(ctx, item, reader, maxBytes)
	if err != nil {
		return item, err
	}
	if stored.SizeBytes == 0 {
		return item, s.discard(item, ErrInvalidUpload)
	}
	item.SizeBytes = stored.SizeBytes
	item.ContentHash = stored.ContentHash
	file, err := s.files.OpenLibrary(item)
	if err != nil {
		return item, s.discard(item, err)
	}
	metadata, inspectErr := s.metadata.InspectReader(ctx, key, file)
	closeErr := file.Close()
	if inspectErr != nil {
		return item, s.discard(item, fmt.Errorf("%w: %w", ErrInvalidUpload, inspectErr))
	}
	if closeErr != nil {
		return item, s.discard(item, closeErr)
	}
	item.ModName = metadata.Name
	item.Title = metadata.Name
	item.ModVersion = metadata.Version
	item.TModVersion = metadata.LoaderVersion
	outcome, err := s.repo.CommitLibraryUpload(ctx, userID, &item)
	if err == nil && outcome == domain.LibraryCommitApplied {
		return item, nil
	}
	if err == nil {
		err = errors.New("metadata commit not confirmed")
	}
	if outcome == domain.LibraryCommitRejected {
		return item, s.discard(item, err)
	}
	return item, errors.Join(ErrRetainedUpload, err)
}
func (s *Service) discard(item domain.ModFile, cause error) error {
	if err := s.files.RemoveLibrary(item); err != nil {
		return errors.Join(ErrRetainedUpload, cause, err)
	}
	return cause
}
