package modlibrary

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modruntime"
)

var ErrInvalidInstallation = errors.New("invalid mod installation request")
var ErrInstallationConflict = errors.New("refresh the instance and stop it before requesting installation")
var ErrRemoteInstallation = errors.New("remote node must be online with artifacts-v1 support; update the Agent and refresh")

type InstallationRepository interface {
	RemoteArtifactsAvailable(context.Context, string) (bool, error)
	GetUserGameServer(context.Context, string, string) (domain.GameServer, error)
	GetUserLibraryMod(context.Context, string, string) (domain.ModFile, error)
	CheckLibraryWriter(context.Context, string, string) error
	SaveModInstallationIntent(context.Context, string, domain.GameServer, domain.GameServer, domain.ModFile) error
}

type Installer struct {
	repo    InstallationRepository
	formats *modruntime.Service
}

func NewInstaller(repo InstallationRepository, providers modruntime.Registry) *Installer {
	return &Installer{repo: repo, formats: modruntime.NewService(providers, nil)}
}

// Request appends one immutable library source to the desired configuration.
// Success acknowledges intent only; the runtime planner installs the files.
// Repeating a source at the current generation is a no-op.
func (s *Installer) Request(ctx context.Context, userID, serverID, modID string, generation int) (domain.GameServer, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(serverID) == "" || strings.TrimSpace(modID) == "" || generation < 1 {
		return domain.GameServer{}, ErrInvalidInstallation
	}
	before, err := s.repo.GetUserGameServer(ctx, userID, serverID)
	if err != nil {
		return domain.GameServer{}, err
	}
	if err := s.repo.CheckLibraryWriter(ctx, userID, before.OrganizationID); err != nil {
		return domain.GameServer{}, err
	}
	if before.Spec.Generation != generation || before.Spec.DesiredState != domain.DesiredStopped || before.Status.Phase != domain.PhaseStopped {
		return domain.GameServer{}, ErrInstallationConflict
	}
	if !before.IsLocal() {
		ready, err := s.repo.RemoteArtifactsAvailable(ctx, before.NodeID)
		if err != nil {
			return domain.GameServer{}, err
		}
		if !ready {
			return domain.GameServer{}, ErrRemoteInstallation
		}
	}
	source, err := s.repo.GetUserLibraryMod(ctx, userID, modID)
	if err != nil {
		return domain.GameServer{}, err
	}
	if source.OrganizationID != before.OrganizationID || source.InstanceID != "unassigned" || source.ProviderKey != before.ProviderKey || source.Source != "upload" {
		return domain.GameServer{}, ErrInvalidInstallation
	}
	if _, err := s.formats.UploadFileName(before.ProviderKey, source.FileName); err != nil {
		return domain.GameServer{}, ErrInvalidInstallation
	}
	after := before
	if !slices.Contains(before.Spec.ModIDs, source.ID) {
		after.Spec.ModIDs = append(slices.Clone(before.Spec.ModIDs), source.ID)
		after.Spec.Generation++
		after.UpdatedAt = time.Now().UTC()
	}
	if err := s.repo.SaveModInstallationIntent(ctx, userID, before, after, source); err != nil {
		return domain.GameServer{}, err
	}
	return after, nil
}
