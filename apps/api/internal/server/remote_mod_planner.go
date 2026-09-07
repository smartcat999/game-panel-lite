package server

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modcatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modruntime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

// RemoteModPlanner returns immutable references and provider manifests without
// copying files into control-plane instance directories or creating installed rows.
type RemoteModPlanner interface {
	PlanRemoteMods(context.Context, domain.GameServer) (workload.Options, error)
}

func (p *RuntimeModPlanner) PlanRemoteMods(ctx context.Context, target domain.GameServer) (workload.Options, error) {
	result := workload.Options{}
	game, ok := p.providers.Get(target.ProviderKey)
	if !ok {
		return result, fmt.Errorf("unknown mod provider")
	}
	if !game.Capabilities().Mods {
		if len(target.Spec.ModIDs) > 0 {
			return result, fmt.Errorf("provider does not support mods")
		}
		return result, nil
	}
	layout, layoutOK := game.(provider.ModFilesProvider)
	manifest, manifestOK := game.(provider.ModManifestProvider)
	if len(target.Spec.ModIDs) > 0 && !layoutOK {
		return result, fmt.Errorf("provider lacks a remote mod file contract")
	}
	if err := p.store.CheckModTarget(ctx, target); err != nil {
		return result, err
	}
	selected := map[string]domain.ModFile{}
	roots := []domain.ModFile{}
	validate := func(item domain.ModFile) error {
		if target.OrganizationID == "" || item.OrganizationID != target.OrganizationID || item.ProviderKey != target.ProviderKey || item.InstanceID != "unassigned" || item.Source != "upload" {
			return fmt.Errorf("remote mods require an uploaded library source in the target workspace and provider")
		}
		filename, err := p.runtime.UploadFileName(target.ProviderKey, item.FileName)
		if err != nil || filename != item.FileName {
			return fmt.Errorf("invalid uploaded mod filename")
		}
		if item.DependenciesJSON != "" {
			var dependencies []string
			if err := json.Unmarshal([]byte(item.DependenciesJSON), &dependencies); err != nil {
				return fmt.Errorf("invalid dependency metadata for %s", item.ID)
			}
		}
		return nil
	}
	add := func(item domain.ModFile) error {
		if err := validate(item); err != nil {
			return err
		}
		name := modcatalog.Identity(item)
		if name == "" {
			return fmt.Errorf("missing mod identity")
		}
		if existing, ok := selected[name]; ok && existing.ID != item.ID {
			return fmt.Errorf("ambiguous mod identity %q", name)
		}
		item.Enabled = true
		selected[name] = item
		return nil
	}
	for _, id := range uniqueModIDs(target.Spec.ModIDs) {
		item, err := p.store.GetModForServer(ctx, target, id)
		if err != nil {
			return result, err
		}
		if err := add(item); err != nil {
			return result, err
		}
		roots = append(roots, item)
	}
	var library []domain.ModFile
	loaded := false
	_, err := modruntime.ResolveDependencies(ctx, roots, func(ctx context.Context, name string) (domain.ModFile, bool, error) {
		if item, ok := selected[name]; ok {
			return item, false, nil
		}
		if !loaded {
			var err error
			library, err = p.store.ListLibraryModsForServer(ctx, target)
			if err != nil {
				return domain.ModFile{}, false, err
			}
			loaded = true
		}
		var found domain.ModFile
		for _, item := range library {
			if item.ProviderKey != target.ProviderKey || modcatalog.Identity(item) != name {
				continue
			}
			if err := validate(item); err != nil {
				continue
			}
			if found.ID != "" && found.ID != item.ID {
				return domain.ModFile{}, false, fmt.Errorf("ambiguous dependency %q", name)
			}
			found = item
		}
		if found.ID == "" {
			return found, false, fmt.Errorf("missing uploaded dependency %q in workspace", name)
		}
		if err := add(found); err != nil {
			return found, false, err
		}
		return found, true, nil
	})
	if err != nil {
		return result, err
	}
	items := make([]domain.ModFile, 0, len(selected))
	for _, item := range selected {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	for _, item := range items {
		paths := layout.RuntimeModFiles(item.FileName)
		if len(paths) == 0 {
			return result, fmt.Errorf("provider has no destination for mod %s", item.ID)
		}
		for _, path := range paths {
			result.Artifacts = append(result.Artifacts, workload.Artifact{ID: item.ID, Revision: item.Revision, Path: filepath.ToSlash(path), SHA256: item.ContentHash, SizeBytes: item.SizeBytes})
		}
	}
	if manifestOK {
		result.Files, err = manifest.ModManifest(items)
		if err != nil {
			return workload.Options{}, err
		}
	}
	if err := workload.ValidateArtifacts(result); err != nil {
		return workload.Options{}, err
	}
	if err := p.store.CheckModTarget(ctx, target); err != nil {
		return workload.Options{}, err
	}
	return result, nil
}
