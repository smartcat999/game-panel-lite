package http

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	modsvc "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modcatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modruntime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func (h *Handler) copyLibraryModToServerCache(item domain.ModFile, targetInstanceID string) (int64, error) {
	hydrateModGameMetadata(&item)
	svc := modsvc.NewService(h.cfg.DataDir, h.modRuntime.StoredFileName)
	src, err := svc.Open(item)
	if err != nil {
		return 0, fmt.Errorf("mod file not found")
	}
	defer src.Close()
	_, size, err := svc.Upload(targetInstanceID, item.ProviderKey, item.FileName, src)
	return size, err
}

func (h *Handler) upsertModRecord(ctx context.Context, server domain.GameServer, instanceID string, fileName string, size int64, metadata domain.ModMetadata) (domain.ModFile, bool, error) {
	return h.upsertModRecordForProvider(ctx, server.ProviderKey, instanceID, fileName, size, metadata)
}

func (h *Handler) upsertModRecordForProvider(ctx context.Context, providerKey domain.ProviderKey, instanceID string, fileName string, size int64, metadata domain.ModMetadata) (domain.ModFile, bool, error) {
	if existing, err := h.store.GetModByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.SizeBytes = size
		existing.Enabled = true
		existing.GameKey = gameKeyForProvider(providerKey)
		existing.ProviderKey = providerKey
		if existing.Source == "" {
			existing.Source = "upload"
		}
		applyModMetadata(&existing, metadata)
		applyFileModMetadata(&existing)
		hydrateModMetadata(&existing)
		return existing, false, h.store.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	item := domain.ModFile{ID: uuid.NewString(), InstanceID: instanceID, GameKey: gameKeyForProvider(providerKey), ProviderKey: providerKey, FileName: fileName, Source: "upload", SizeBytes: size, Enabled: true, CreatedAt: time.Now()}
	applyModMetadata(&item, metadata)
	applyFileModMetadata(&item)
	hydrateModMetadata(&item)
	return item, true, h.store.CreateMod(ctx, &item)
}

func applyModMetadata(item *domain.ModFile, metadata domain.ModMetadata) {
	if metadata.Name != "" {
		item.ModName = metadata.Name
		item.Title = metadata.Name
	}
	if metadata.Version != "" {
		item.ModVersion = metadata.Version
	}
	if metadata.LoaderVersion != "" {
		item.TModVersion = metadata.LoaderVersion
	}
}

func metadataFromMod(item domain.ModFile) domain.ModMetadata {
	return domain.ModMetadata{
		Name:          item.Title,
		Version:       item.ModVersion,
		LoaderVersion: item.TModVersion,
	}
}

func applyFileModMetadata(item *domain.ModFile) {
	if item.Title != "" {
		return
	}
	switch item.ProviderKey {
	case domain.ProviderPalworld:
		item.Title = strings.TrimSuffix(item.FileName, filepath.Ext(item.FileName))
		item.ModName = item.Title
	case domain.ProviderTerrariaTModLoader:
		return
	default:
		if item.ModName == "" {
			item.ModName = strings.TrimPrefix(item.FileName, "workshop-")
		}
		if item.Title == "" {
			item.Title = item.ModName
		}
	}
}

func (h *Handler) upsertWorkshopModRecord(ctx context.Context, server domain.GameServer, instanceID string, workshopID string) (domain.ModFile, bool, error) {
	return h.upsertWorkshopModRecordForProvider(ctx, server.ProviderKey, instanceID, workshopID)
}

func (h *Handler) upsertWorkshopModRecordForProvider(ctx context.Context, providerKey domain.ProviderKey, instanceID string, workshopID string) (domain.ModFile, bool, error) {
	return h.upsertWorkshopModRecordWithStore(ctx, h.store, providerKey, instanceID, workshopID)
}

func (h *Handler) upsertWorkshopModRecordWithStore(ctx context.Context, modStore *store.Store, providerKey domain.ProviderKey, instanceID string, workshopID string) (domain.ModFile, bool, error) {
	fileName := "workshop-" + workshopID
	if existing, err := modStore.GetModByInstanceAndWorkshopID(ctx, instanceID, workshopID); err == nil {
		existing.Source = "workshop"
		existing.WorkshopID = workshopID
		existing.GameKey = gameKeyForProvider(providerKey)
		existing.ProviderKey = providerKey
		existing.Enabled = true
		applyRecommendedModMetadataForProvider(&existing, providerKey, workshopID)
		return existing, false, modStore.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	if existing, err := modStore.GetModByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.Source = "workshop"
		existing.WorkshopID = workshopID
		existing.GameKey = gameKeyForProvider(providerKey)
		existing.ProviderKey = providerKey
		existing.Enabled = true
		applyRecommendedModMetadataForProvider(&existing, providerKey, workshopID)
		return existing, false, modStore.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	item := domain.ModFile{
		ID:          uuid.NewString(),
		InstanceID:  instanceID,
		GameKey:     gameKeyForProvider(providerKey),
		ProviderKey: providerKey,
		FileName:    fileName,
		Source:      "workshop",
		WorkshopID:  workshopID,
		SizeBytes:   int64(len(workshopID) + 1),
		Enabled:     true,
		CreatedAt:   time.Now(),
	}
	applyRecommendedModMetadataForProvider(&item, providerKey, workshopID)
	return item, true, modStore.CreateMod(ctx, &item)
}

func (h *Handler) ensureModDependencies(ctx context.Context, server domain.GameServer, roots []domain.ModFile) ([]domain.ModFile, error) {
	if server.ProviderKey != domain.ProviderTerrariaTModLoader || len(roots) == 0 {
		return nil, nil
	}
	return modruntime.ResolveDependencies(ctx, roots, func(ctx context.Context, name string) (domain.ModFile, bool, error) {
		return h.ensureModDependency(ctx, server, name)
	})
}

func (h *Handler) ensureModDependency(ctx context.Context, server domain.GameServer, dependencyName string) (domain.ModFile, bool, error) {
	dependencyName = strings.TrimSpace(dependencyName)
	if dependencyName == "" {
		return domain.ModFile{}, false, nil
	}
	if existing, ok, err := h.findServerModByModName(ctx, server.ID, server.ProviderKey, dependencyName); err != nil || ok {
		return existing, false, err
	}
	if library, ok, err := h.findLibraryModByModName(ctx, server, dependencyName); err != nil || ok {
		if err != nil {
			return domain.ModFile{}, false, err
		}
		if library.Source == "workshop" {
			assigned, created, err := h.upsertWorkshopModRecord(ctx, server, server.ID, library.WorkshopID)
			return assigned, created, err
		}
		size, err := h.copyLibraryModToServerCache(library, server.ID)
		if err != nil {
			return domain.ModFile{}, false, err
		}
		assigned, created, err := h.upsertModRecord(ctx, server, server.ID, library.FileName, size, metadataFromMod(library))
		if err != nil {
			return domain.ModFile{}, false, err
		}
		return assigned, created, nil
	}
	recommended, ok := modcatalog.RecommendedModByProviderAndModName(server.ProviderKey, dependencyName)
	if !ok || recommended.WorkshopID == "" {
		return domain.ModFile{}, false, fmt.Errorf("missing dependency %s in mod library", dependencyName)
	}
	if h.workshopSyncUnsupported() {
		return domain.ModFile{}, false, fmt.Errorf("missing dependency %s in mod library; upload the .tmod dependency file first", dependencyName)
	}
	assigned, created, err := h.upsertWorkshopModRecord(ctx, server, server.ID, recommended.WorkshopID)
	return assigned, created, err
}

func (h *Handler) findServerModByModName(ctx context.Context, instanceID string, providerKey domain.ProviderKey, modName string) (domain.ModFile, bool, error) {
	mods, err := h.store.ListMods(ctx, instanceID)
	if err != nil {
		return domain.ModFile{}, false, err
	}
	for _, item := range mods {
		// Normalize pre-provider legacy records before applying the provider scope.
		hydrateModMetadata(&item)
		if item.ProviderKey == providerKey && modcatalog.Identity(item) == modName {
			return item, true, nil
		}
	}
	return domain.ModFile{}, false, nil
}

func (h *Handler) findLibraryModByModName(ctx context.Context, server domain.GameServer, modName string) (domain.ModFile, bool, error) {
	mods, err := h.store.ListLibraryModsForServer(ctx, server)
	if err != nil {
		return domain.ModFile{}, false, err
	}
	for _, item := range mods {
		// Normalize pre-provider legacy records before applying the provider scope.
		hydrateModMetadata(&item)
		if item.ProviderKey == server.ProviderKey && modcatalog.Identity(item) == modName {
			return item, true, nil
		}
	}
	return domain.ModFile{}, false, nil
}

var errWorkshopModExists = errors.New("workshop mod already exists")
var errRecommendedFileModExists = errors.New("recommended file mod already exists")

func (h *Handler) createWorkshopModRecord(ctx context.Context, providerKey domain.ProviderKey, instanceID string, workshopID string) (domain.ModFile, bool, error) {
	if _, err := h.store.GetModByInstanceAndWorkshopID(ctx, instanceID, workshopID); err == nil {
		return domain.ModFile{}, false, errWorkshopModExists
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	if _, err := h.store.GetModByInstanceAndFile(ctx, instanceID, "workshop-"+workshopID); err == nil {
		return domain.ModFile{}, false, errWorkshopModExists
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	return h.upsertWorkshopModRecordForProvider(ctx, providerKey, instanceID, workshopID)
}

func (h *Handler) createRecommendedFileModRecord(ctx context.Context, providerKey domain.ProviderKey, instanceID string, externalID string) (domain.ModFile, bool, error) {
	recommended, ok := modcatalog.RecommendedModByProviderAndExternalID(providerKey, strings.TrimSpace(externalID))
	if !ok {
		return domain.ModFile{}, false, fmt.Errorf("recommended mod not found")
	}
	fileName := strings.TrimSpace(recommended.FileName)
	if fileName == "" {
		fileName = safeRecommendedFileName(recommended.Title, providerKey)
	}
	if _, err := h.store.GetModByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		return domain.ModFile{}, false, errRecommendedFileModExists
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	mods, err := h.store.ListMods(ctx, instanceID)
	if err != nil {
		return domain.ModFile{}, false, err
	}
	for _, item := range mods {
		if item.ProviderKey == providerKey && item.Source != "workshop" && item.ModName == recommended.ExternalID {
			return domain.ModFile{}, false, errRecommendedFileModExists
		}
	}
	tags, _ := json.Marshal(recommended.Tags)
	dependencies, _ := json.Marshal(uniqueNonEmptyStrings(recommended.Dependencies))
	item := domain.ModFile{
		ID:               uuid.NewString(),
		InstanceID:       instanceID,
		GameKey:          gameKeyForProvider(providerKey),
		ProviderKey:      providerKey,
		FileName:         fileName,
		Source:           "file-recommendation",
		ModName:          recommended.ExternalID,
		Title:            recommended.Title,
		PreviewURL:       recommended.PreviewURL,
		Description:      recommended.Description,
		TagsJSON:         string(tags),
		DependenciesJSON: string(dependencies),
		SizeBytes:        recommended.FileSize,
		Enabled:          true,
		CreatedAt:        time.Now(),
	}
	hydrateModMetadata(&item)
	return item, true, h.store.CreateMod(ctx, &item)
}

func applyRecommendedModMetadata(item *domain.ModFile, workshopID string) {
	applyRecommendedModMetadataForProvider(item, item.ProviderKey, workshopID)
}

func safeRecommendedFileName(title string, providerKey domain.ProviderKey) string {
	extension := ".tmod"
	if providerKey == domain.ProviderPalworld {
		extension = ".pak"
	}
	base := strings.ToLower(strings.TrimSpace(title))
	if base == "" {
		base = "recommended-mod"
	}
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ":", "-", "'", "", "\"", "")
	base = replacer.Replace(base)
	base = strings.Trim(base, ".-")
	if base == "" {
		base = "recommended-mod"
	}
	return base + extension
}

func applyRecommendedModMetadataForProvider(item *domain.ModFile, providerKey domain.ProviderKey, workshopID string) {
	recommended, ok := modcatalog.RecommendedModByProviderAndWorkshopID(providerKey, workshopID)
	if !ok {
		return
	}
	tags, _ := json.Marshal(recommended.Tags)
	dependencies, _ := json.Marshal(uniqueNonEmptyStrings(recommended.Dependencies))
	item.ModName = recommended.ModName
	item.Title = recommended.Title
	item.CreatorSteamID = recommended.CreatorSteamID
	item.PreviewURL = recommended.PreviewURL
	item.Description = recommended.Description
	item.TagsJSON = string(tags)
	item.DependenciesJSON = string(dependencies)
	item.Subscriptions = recommended.Subscriptions
	item.Favorited = recommended.Favorited
	item.Views = recommended.Views
	item.UpdatedAtSteam = recommended.TimeUpdated
	if recommended.FileSize > 0 {
		item.SizeBytes = recommended.FileSize
	}
	hydrateModMetadata(item)
}

func hydrateModMetadata(item *domain.ModFile) {
	hydrateModGameMetadata(item)
	if item.TagsJSON != "" {
		_ = json.Unmarshal([]byte(item.TagsJSON), &item.Tags)
	}
	if item.DependenciesJSON != "" {
		_ = json.Unmarshal([]byte(item.DependenciesJSON), &item.Dependencies)
	}
	if item.Source == "workshop" && item.Title == "" && item.WorkshopID != "" {
		item.Title = "Workshop " + item.WorkshopID
	}
	if item.ModName == "" {
		item.ModName = modcatalog.Identity(*item)
	}
	if len(item.Dependencies) == 0 {
		item.Dependencies = modcatalog.Dependencies(*item)
	}
}

func hydrateModGameMetadata(item *domain.ModFile) {
	if item.ProviderKey == "" {
		item.ProviderKey = domain.ProviderTerrariaTModLoader
	}
	if item.GameKey == "" {
		item.GameKey = gameKeyForProvider(item.ProviderKey)
	}
}

func isTModPackage(fileName string) bool {
	return strings.EqualFold(filepath.Ext(fileName), ".tmod")
}

func (h *Handler) providerSupportsMods(providerKey domain.ProviderKey) bool {
	return len(h.modRuntime.Support(providerKey).UploadExtensions) > 0 || h.modRuntime.Support(providerKey).Workshop
}

func (h *Handler) providerSupportsUploadedMods(providerKey domain.ProviderKey) bool {
	return len(h.modRuntime.Support(providerKey).UploadExtensions) > 0
}

func (h *Handler) providerSupportsWorkshopMods(providerKey domain.ProviderKey) bool {
	return h.modRuntime.Support(providerKey).Workshop
}

func gameKeyForProvider(providerKey domain.ProviderKey) domain.GameKey {
	switch providerKey {
	case domain.ProviderDST:
		return domain.GameDST
	case domain.ProviderPalworld:
		return domain.GamePalworld
	case domain.ProviderMinecraft:
		return domain.GameMinecraft
	default:
		return domain.GameTerraria
	}
}

func modIDs(items []domain.ModFile) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return uniqueNonEmptyStrings(ids)
}

func (h *Handler) markModDesired(ctx context.Context, server *domain.GameServer, modID string) error {
	return h.markModsDesired(ctx, server, []string{modID})
}

func (h *Handler) unmarkModDesired(ctx context.Context, server *domain.GameServer, modID string) error {
	next := make([]string, 0, len(server.Spec.ModIDs))
	for _, id := range server.Spec.ModIDs {
		if id != modID {
			next = append(next, id)
		}
	}
	server.Spec.ModIDs = uniqueNonEmptyStrings(next)
	if server.ProviderKey == domain.ProviderDST {
		if err := h.syncDSTDesiredWorkshopConfig(ctx, server); err != nil {
			return err
		}
	}
	server.Spec.Generation++
	if server.Spec.Generation <= 0 {
		server.Spec.Generation = 1
	}
	server.UpdatedAt = time.Now()
	return h.store.SaveGameServer(ctx, server)
}

func (h *Handler) markModsDesired(ctx context.Context, server *domain.GameServer, modIDs []string) error {
	next := uniqueNonEmptyStrings(append(server.Spec.ModIDs, modIDs...))
	server.Spec.ModIDs = next
	if server.ProviderKey == domain.ProviderDST {
		if err := h.syncDSTDesiredWorkshopConfig(ctx, server); err != nil {
			return err
		}
	}
	server.Spec.Generation++
	if server.Spec.Generation <= 0 {
		server.Spec.Generation = 1
	}
	server.UpdatedAt = time.Now()
	return h.store.SaveGameServer(ctx, server)
}

func (h *Handler) syncDSTDesiredWorkshopConfig(ctx context.Context, server *domain.GameServer) error {
	mods, err := h.store.ListMods(ctx, server.ID)
	if err != nil {
		return err
	}
	desired := map[string]struct{}{}
	for _, id := range server.Spec.ModIDs {
		desired[id] = struct{}{}
	}
	seenDesired := map[string]struct{}{}
	workshopIDs := make([]string, 0, len(mods))
	for _, item := range mods {
		if !item.Enabled || item.Source != "workshop" || item.WorkshopID == "" {
			continue
		}
		if _, ok := desired[item.ID]; !ok {
			continue
		}
		workshopIDs = append(workshopIDs, item.WorkshopID)
		seenDesired[item.ID] = struct{}{}
	}
	for _, id := range server.Spec.ModIDs {
		if _, ok := seenDesired[id]; !ok {
			if item, err := h.store.GetModForServer(ctx, *server, id); err == nil {
				if item.Source == "workshop" && item.WorkshopID != "" {
					workshopIDs = append(workshopIDs, item.WorkshopID)
				}
			} else if item, err := h.store.GetMod(ctx, id); err == nil {
				if item.Source == "workshop" && item.WorkshopID != "" {
					workshopIDs = append(workshopIDs, item.WorkshopID)
				}
			}
		}
	}
	sort.Strings(workshopIDs)
	workshopIDs = uniqueNonEmptyStrings(workshopIDs)
	if server.Spec.Config == nil {
		server.Spec.Config = map[string]any{}
	}
	modsPayload, _ := server.Spec.Config["mods"].(map[string]any)
	if modsPayload == nil {
		modsPayload = map[string]any{}
	}
	modsPayload["workshopIds"] = workshopIDs
	server.Spec.Config["mods"] = modsPayload
	return nil
}

func (h *Handler) visibleMods(ctx context.Context, mods []domain.ModFile) ([]domain.ModFile, error) {
	svc := modsvc.NewService(h.cfg.DataDir, h.modRuntime.StoredFileName)
	visible := make([]domain.ModFile, 0, len(mods))
	for _, item := range mods {
		hydrateModGameMetadata(&item)
		if item.Source == "workshop" {
			hydrateModMetadata(&item)
			visible = append(visible, item)
			continue
		}
		path, err := svc.Path(item.InstanceID, item.ProviderKey, item.FileName)
		if err != nil {
			continue
		}
		if item.FileName == "install.txt" && item.Source == "" {
			items, err := h.migrateLegacyWorkshopInstall(ctx, item, path)
			if err != nil {
				return nil, err
			}
			visible = append(visible, items...)
			continue
		}
		if _, err := os.Stat(path); err != nil {
			h.logger.Warn("mod file missing, pruning orphaned record", "modId", item.ID, "path", path)
			if err := h.store.DeleteMod(ctx, item.ID); err != nil {
				return nil, err
			}
			continue
		}
		hydrateModMetadata(&item)
		visible = append(visible, item)
	}
	return visible, nil
}

func (h *Handler) enrichServerModMetadata(ctx context.Context, mods []domain.ModFile) ([]domain.ModFile, error) {
	workshopIDs := make([]string, 0, len(mods))
	seen := make(map[string]struct{}, len(mods))
	for _, item := range mods {
		if item.Source != "workshop" || item.WorkshopID == "" {
			continue
		}
		if _, exists := seen[item.WorkshopID]; exists {
			continue
		}
		seen[item.WorkshopID] = struct{}{}
		workshopIDs = append(workshopIDs, item.WorkshopID)
	}

	libraryMods, err := h.store.ListLibraryModsByWorkshopIDs(ctx, workshopIDs)
	if err != nil {
		return nil, err
	}
	metadataByWorkshopID := make(map[string]domain.ModFile, len(libraryMods))
	for _, item := range libraryMods {
		metadataByWorkshopID[item.WorkshopID] = item
	}
	for index := range mods {
		metadata, exists := metadataByWorkshopID[mods[index].WorkshopID]
		if !exists {
			continue
		}
		applyLibraryModMetadata(&mods[index], metadata)
	}
	return mods, nil
}

func applyLibraryModMetadata(item *domain.ModFile, metadata domain.ModFile) {
	if metadata.ModName != "" {
		item.ModName = metadata.ModName
	}
	if metadata.Title != "" {
		item.Title = metadata.Title
	}
	if metadata.ModVersion != "" {
		item.ModVersion = metadata.ModVersion
	}
	if metadata.TModVersion != "" {
		item.TModVersion = metadata.TModVersion
	}
	if metadata.CreatorSteamID != "" {
		item.CreatorSteamID = metadata.CreatorSteamID
	}
	if metadata.PreviewURL != "" {
		item.PreviewURL = metadata.PreviewURL
	}
	if metadata.Description != "" {
		item.Description = metadata.Description
	}
	if metadata.TagsJSON != "" {
		item.TagsJSON = metadata.TagsJSON
	}
	if metadata.DependenciesJSON != "" {
		item.DependenciesJSON = metadata.DependenciesJSON
	}
	item.Subscriptions = metadata.Subscriptions
	item.Favorited = metadata.Favorited
	item.Views = metadata.Views
	item.UpdatedAtSteam = metadata.UpdatedAtSteam
	if metadata.SizeBytes > 0 {
		item.SizeBytes = metadata.SizeBytes
	}
}

func (h *Handler) visibleServerMods(ctx context.Context, server domain.GameServer, mods []domain.ModFile) ([]domain.ModFile, error) {
	visible, err := h.visibleMods(ctx, mods)
	if err != nil {
		return nil, err
	}
	desiredSet := make(map[string]struct{}, len(server.Spec.ModIDs))
	for _, id := range server.Spec.ModIDs {
		desiredSet[id] = struct{}{}
	}
	for index := range visible {
		visible[index].GameKey = server.GameKey
		visible[index].ProviderKey = server.ProviderKey
		present := runtimeModPresent(server, visible[index])
		visible[index].RuntimePresent = &present
		_, isDesired := desiredSet[visible[index].ID]
		enabled := visible[index].Enabled || isDesired
		visible[index].RuntimeEnabled = &enabled
	}
	return visible, nil
}

func (h *Handler) resolvePendingDesiredMods(ctx context.Context, server domain.GameServer, visible []domain.ModFile) []domain.ModFile {
	existingIDs := make(map[string]struct{}, len(visible))
	existingWorkshopIDs := make(map[string]struct{}, len(visible))
	existingFileNames := make(map[string]struct{}, len(visible))
	for _, m := range visible {
		existingIDs[m.ID] = struct{}{}
		if m.WorkshopID != "" {
			existingWorkshopIDs[m.WorkshopID] = struct{}{}
		}
		if m.FileName != "" {
			existingFileNames[m.FileName] = struct{}{}
		}
	}

	var pending []domain.ModFile
	for _, modID := range uniqueNonEmptyStrings(server.Spec.ModIDs) {
		if _, ok := existingIDs[modID]; ok {
			continue
		}
		item, err := h.store.GetModForServer(ctx, server, modID)
		if err != nil {
			item, err = h.store.GetMod(ctx, modID)
			if err != nil {
				continue
			}
		}
		if item.WorkshopID != "" {
			if _, ok := existingWorkshopIDs[item.WorkshopID]; ok {
				continue
			}
		}
		if item.FileName != "" {
			if _, ok := existingFileNames[item.FileName]; ok {
				continue
			}
		}
		hydrateModGameMetadata(&item)
		hydrateModMetadata(&item)
		item.InstanceID = server.ID
		item.GameKey = server.GameKey
		item.ProviderKey = server.ProviderKey
		item.Enabled = true
		present := runtimeModPresent(server, item)
		item.RuntimePresent = &present
		pending = append(pending, item)
		existingIDs[item.ID] = struct{}{}
		if item.WorkshopID != "" {
			existingWorkshopIDs[item.WorkshopID] = struct{}{}
		}
		if item.FileName != "" {
			existingFileNames[item.FileName] = struct{}{}
		}
	}

	if server.ProviderKey == domain.ProviderDST {
		if modsCfg, ok := server.Spec.Config["mods"].(map[string]any); ok {
			if rawList, ok := modsCfg["workshopIds"].([]any); ok {
				for _, raw := range rawList {
					wID, ok := raw.(string)
					if !ok || strings.TrimSpace(wID) == "" {
						continue
					}
					wID = strings.TrimSpace(wID)
					if _, ok := existingWorkshopIDs[wID]; ok {
						continue
					}
					item, err := h.store.GetModByInstanceAndWorkshopID(ctx, "unassigned", wID)
					if err == nil {
						hydrateModGameMetadata(&item)
						hydrateModMetadata(&item)
						item.InstanceID = server.ID
						item.GameKey = server.GameKey
						item.ProviderKey = server.ProviderKey
						item.Enabled = true
						present := true
						item.RuntimePresent = &present
						pending = append(pending, item)
						existingWorkshopIDs[wID] = struct{}{}
					} else {
						item = domain.ModFile{
							ID:          "workshop-" + wID,
							InstanceID:  server.ID,
							GameKey:     server.GameKey,
							ProviderKey: server.ProviderKey,
							FileName:    "workshop-" + wID,
							Source:      "workshop",
							WorkshopID:  wID,
							SizeBytes:   int64(len(wID) + 1),
							Enabled:     true,
							CreatedAt:   time.Now(),
						}
						applyRecommendedModMetadataForProvider(&item, server.ProviderKey, wID)
						present := true
						item.RuntimePresent = &present
						pending = append(pending, item)
						existingWorkshopIDs[wID] = struct{}{}
					}
				}
			}
		}
	}

	return pending
}

func runtimeModPresent(server domain.GameServer, item domain.ModFile) bool {
	if cond, ok := workload.FindCondition(server.Status.Conditions, workload.ConditionArtifactsReady); ok {
		return cond.Status == workload.ConditionStatusTrue
	}
	return server.Status.ActualState == domain.ActualRunning && server.Status.AppliedGeneration >= server.Spec.Generation
}

func readRuntimeEnabledMods(server domain.GameServer) (map[string]struct{}, error) {
	return nil, nil
}

func (h *Handler) migrateLegacyWorkshopInstall(ctx context.Context, item domain.ModFile, path string) ([]domain.ModFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, h.store.DeleteMod(ctx, item.ID)
		}
		return nil, err
	}
	workshopIDs := workshopIDsFromInstallContent(string(content))
	items := make([]domain.ModFile, 0, len(workshopIDs))
	for _, workshopID := range workshopIDs {
		mod, _, err := h.upsertWorkshopModRecordForProvider(ctx, domain.ProviderTerrariaTModLoader, item.InstanceID, workshopID)
		if err != nil {
			return nil, err
		}
		items = append(items, mod)
	}
	if err := h.store.DeleteMod(ctx, item.ID); err != nil {
		return nil, err
	}
	if err := removeStoredFile(path); err != nil {
		return nil, err
	}
	return items, nil
}

func workshopIDsFromInstallContent(content string) []string {
	ids := make([]string, 0)
	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		id := strings.TrimSpace(scanner.Text())
		if id == "" || !isDigitsOnly(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func isDigitsOnly(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (h *Handler) removeCachedMod(item domain.ModFile) error {
	hydrateModGameMetadata(&item)
	return modsvc.NewService(h.cfg.DataDir, h.modRuntime.StoredFileName).Remove(item)
}
