package modlibrary

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
)

type installationRepo struct {
	server    domain.GameServer
	source    domain.ModFile
	committed *domain.GameServer
}

func (r *installationRepo) GetUserGameServer(context.Context, string, string) (domain.GameServer, error) {
	return r.server, nil
}
func (r *installationRepo) GetUserLibraryMod(context.Context, string, string) (domain.ModFile, error) {
	return r.source, nil
}
func (r *installationRepo) CheckLibraryWriter(context.Context, string, string) error { return nil }
func (r *installationRepo) SaveModInstallationIntent(_ context.Context, _ string, _ domain.GameServer, after domain.GameServer, _ domain.ModFile) error {
	r.committed = &after
	return nil
}
func TestInstallationUsesProviderUploadContract(t *testing.T) {
	plugin := uploadPlugin{terraria.NewTModLoaderProvider()}
	registry, err := provider.NewRegistry(plugin)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"accepted", "foreign-space", "foreign-provider", "workshop", "unsupported-file", "already-requested"} {
		t.Run(mode, func(t *testing.T) {
			repo := &installationRepo{server: domain.GameServer{ID: "server", OrganizationID: "space", ProviderKey: plugin.Key(), Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredStopped}, Status: domain.ServerRuntimeStatus{Phase: domain.PhaseStopped}}, source: domain.ModFile{ID: "mod", InstanceID: "unassigned", OrganizationID: "space", ProviderKey: plugin.Key(), Source: "upload", FileName: "same.tmod"}}
			switch mode {
			case "foreign-space":
				repo.source.OrganizationID = "foreign"
			case "foreign-provider":
				repo.source.ProviderKey = domain.ProviderTerrariaTModLoader
			case "workshop":
				repo.source.Source = "workshop"
			case "unsupported-file":
				repo.source.FileName = "data.exe"
			case "already-requested":
				repo.server.Spec.ModIDs = []string{"mod"}
			}
			result, err := NewInstaller(repo, registry).Request(context.Background(), "user", "server", "mod", 1)
			if mode == "accepted" || mode == "already-requested" {
				want := 2
				if mode == "already-requested" {
					want = 1
				}
				if err != nil || result.Spec.Generation != want || repo.committed == nil || len(result.Spec.ModIDs) != 1 {
					t.Fatalf("result: %+v %v", result, err)
				}
				if mode == "accepted" && len(repo.server.Spec.ModIDs) != 0 {
					t.Fatal("mutated read snapshot")
				}
			} else if !errors.Is(err, ErrInvalidInstallation) || repo.committed != nil {
				t.Fatalf("invalid source committed: %v", err)
			}
		})
	}
}
