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
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

func (h *Handler) copyLibraryModToServerCache(item domain.ModFile, targetInstanceID string) (int64, error) {
	svc := modsvc.NewService(h.cfg.DataDir)
	sourcePath, err := svc.Path(item.InstanceID, item.FileName)
	if err != nil {
		return 0, err
	}
	src, err := os.Open(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("mod file not found")
	}
	defer src.Close()
	_, size, err := svc.Upload(targetInstanceID, item.FileName, src)
	return size, err
}

func (h *Handler) upsertModRecord(ctx context.Context, instanceID string, fileName string, size int64, metadata modsvc.Metadata) (domain.ModFile, bool, error) {
	if existing, err := h.store.GetModByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.SizeBytes = size
		existing.Enabled = true
		if existing.Source == "" {
			existing.Source = "upload"
		}
		applyTModMetadata(&existing, metadata)
		hydrateModMetadata(&existing)
		return existing, false, h.store.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	item := domain.ModFile{ID: uuid.NewString(), InstanceID: instanceID, FileName: fileName, Source: "upload", SizeBytes: size, Enabled: true, CreatedAt: time.Now()}
	applyTModMetadata(&item, metadata)
	hydrateModMetadata(&item)
	return item, true, h.store.CreateMod(ctx, &item)
}

func applyTModMetadata(item *domain.ModFile, metadata modsvc.Metadata) {
	if metadata.Name != "" {
		item.ModName = metadata.Name
		item.Title = metadata.Name
	}
	if metadata.Version != "" {
		item.ModVersion = metadata.Version
	}
	if metadata.TModLoaderVersion != "" {
		item.TModVersion = metadata.TModLoaderVersion
	}
}

func metadataFromMod(item domain.ModFile) modsvc.Metadata {
	return modsvc.Metadata{
		Name:              item.Title,
		Version:           item.ModVersion,
		TModLoaderVersion: item.TModVersion,
	}
}

func (h *Handler) upsertWorkshopModRecord(ctx context.Context, instanceID string, workshopID string) (domain.ModFile, bool, error) {
	fileName := "workshop-" + workshopID
	if existing, err := h.store.GetModByInstanceAndWorkshopID(ctx, instanceID, workshopID); err == nil {
		existing.Source = "workshop"
		existing.WorkshopID = workshopID
		existing.Enabled = true
		applyRecommendedModMetadata(&existing, workshopID)
		return existing, false, h.store.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	if existing, err := h.store.GetModByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.Source = "workshop"
		existing.WorkshopID = workshopID
		existing.Enabled = true
		applyRecommendedModMetadata(&existing, workshopID)
		return existing, false, h.store.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	item := domain.ModFile{
		ID:         uuid.NewString(),
		InstanceID: instanceID,
		FileName:   fileName,
		Source:     "workshop",
		WorkshopID: workshopID,
		SizeBytes:  int64(len(workshopID) + 1),
		Enabled:    true,
		CreatedAt:  time.Now(),
	}
	applyRecommendedModMetadata(&item, workshopID)
	return item, true, h.store.CreateMod(ctx, &item)
}

func (h *Handler) ensureModDependencies(ctx context.Context, server domain.GameServer, roots []domain.ModFile) ([]domain.ModFile, error) {
	if server.ProviderKey != domain.ProviderTerrariaTModLoader || len(roots) == 0 {
		return nil, nil
	}
	added := make([]domain.ModFile, 0)
	queue := append([]domain.ModFile(nil), roots...)
	seen := make(map[string]struct{}, len(queue))
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		key := modIdentity(item)
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		for _, dependencyName := range modDependencies(item) {
			dependency, created, err := h.ensureModDependency(ctx, server, dependencyName)
			if err != nil {
				return nil, err
			}
			if created {
				added = append(added, dependency)
			}
			queue = append(queue, dependency)
		}
	}
	return added, nil
}

func (h *Handler) ensureModDependency(ctx context.Context, server domain.GameServer, dependencyName string) (domain.ModFile, bool, error) {
	dependencyName = strings.TrimSpace(dependencyName)
	if dependencyName == "" {
		return domain.ModFile{}, false, nil
	}
	if existing, ok, err := h.findServerModByModName(ctx, server.ID, dependencyName); err != nil || ok {
		return existing, false, err
	}
	if library, ok, err := h.findLibraryModByModName(ctx, dependencyName); err != nil || ok {
		if err != nil {
			return domain.ModFile{}, false, err
		}
		if library.Source == "workshop" {
			assigned, created, err := h.upsertWorkshopModRecord(ctx, server.ID, library.WorkshopID)
			return assigned, created, err
		}
		size, err := h.copyLibraryModToServerCache(library, server.ID)
		if err != nil {
			return domain.ModFile{}, false, err
		}
		assigned, created, err := h.upsertModRecord(ctx, server.ID, library.FileName, size, metadataFromMod(library))
		if err != nil {
			return domain.ModFile{}, false, err
		}
		if err := h.materializeModForRuntime(assigned, server); err != nil {
			return domain.ModFile{}, false, err
		}
		return assigned, created, nil
	}
	recommended, ok := modcatalog.RecommendedTModLoaderModByModName(dependencyName)
	if !ok || recommended.WorkshopID == "" {
		return domain.ModFile{}, false, fmt.Errorf("missing dependency %s in mod library", dependencyName)
	}
	if h.workshopSyncUnsupported() {
		return domain.ModFile{}, false, fmt.Errorf("missing dependency %s in mod library; upload the .tmod dependency file first", dependencyName)
	}
	assigned, created, err := h.upsertWorkshopModRecord(ctx, server.ID, recommended.WorkshopID)
	return assigned, created, err
}

func (h *Handler) findServerModByModName(ctx context.Context, instanceID string, modName string) (domain.ModFile, bool, error) {
	mods, err := h.store.ListMods(ctx, instanceID)
	if err != nil {
		return domain.ModFile{}, false, err
	}
	for _, item := range mods {
		if modIdentity(item) == modName {
			return item, true, nil
		}
	}
	return domain.ModFile{}, false, nil
}

func (h *Handler) findLibraryModByModName(ctx context.Context, modName string) (domain.ModFile, bool, error) {
	mods, err := h.store.ListMods(ctx, "unassigned")
	if err != nil {
		return domain.ModFile{}, false, err
	}
	for _, item := range mods {
		if modIdentity(item) == modName {
			return item, true, nil
		}
	}
	return domain.ModFile{}, false, nil
}

func modIdentity(item domain.ModFile) string {
	if item.WorkshopID != "" {
		if recommended, ok := modcatalog.RecommendedTModLoaderModByWorkshopID(item.WorkshopID); ok && strings.TrimSpace(recommended.ModName) != "" {
			return recommended.ModName
		}
	}
	for _, value := range []string{item.ModName, item.Title, strings.TrimSuffix(item.FileName, filepath.Ext(item.FileName))} {
		value = strings.TrimSpace(value)
		if value != "" && !strings.HasPrefix(value, "workshop-") {
			return value
		}
	}
	return ""
}

func modDependencies(item domain.ModFile) []string {
	if len(item.Dependencies) > 0 {
		return uniqueNonEmptyStrings(item.Dependencies)
	}
	if strings.TrimSpace(item.DependenciesJSON) != "" {
		var values []string
		if err := json.Unmarshal([]byte(item.DependenciesJSON), &values); err == nil {
			return uniqueNonEmptyStrings(values)
		}
	}
	if item.WorkshopID != "" {
		if recommended, ok := modcatalog.RecommendedTModLoaderModByWorkshopID(item.WorkshopID); ok {
			return uniqueNonEmptyStrings(recommended.Dependencies)
		}
	}
	if recommended, ok := modcatalog.RecommendedTModLoaderModByModName(modIdentity(item)); ok {
		return uniqueNonEmptyStrings(recommended.Dependencies)
	}
	return nil
}

var errWorkshopModExists = errors.New("workshop mod already exists")

func (h *Handler) createWorkshopModRecord(ctx context.Context, instanceID string, workshopID string) (domain.ModFile, bool, error) {
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
	return h.upsertWorkshopModRecord(ctx, instanceID, workshopID)
}

func applyRecommendedModMetadata(item *domain.ModFile, workshopID string) {
	recommended, ok := modcatalog.RecommendedTModLoaderModByWorkshopID(workshopID)
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
		item.ModName = modIdentity(*item)
	}
	if len(item.Dependencies) == 0 {
		item.Dependencies = modDependencies(*item)
	}
}

func hydrateModGameMetadata(item *domain.ModFile) {
	if item.GameKey == "" {
		item.GameKey = domain.GameTerraria
	}
	if item.ProviderKey == "" {
		item.ProviderKey = domain.ProviderTerrariaTModLoader
	}
}

func (h *Handler) materializeModForRuntime(item domain.ModFile, server domain.GameServer) error {
	if item.Source == "workshop" {
		return nil
	}
	sourcePath, err := modsvc.NewService(h.cfg.DataDir).Path(item.InstanceID, item.FileName)
	if err != nil {
		return err
	}
	dataDir, err := serverDataDir(server)
	if err != nil {
		return err
	}
	for _, relPath := range terraria.RuntimeModFiles(server.ProviderKey, item.FileName) {
		targetPath := filepath.Join(dataDir, relPath)
		if err := copyStoredFile(sourcePath, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) removeRuntimeMod(item domain.ModFile, server domain.GameServer) error {
	dataDir, err := serverDataDir(server)
	if err != nil {
		return err
	}
	for _, relPath := range terraria.RuntimeModFiles(server.ProviderKey, item.FileName) {
		if err := removeStoredFile(filepath.Join(dataDir, relPath)); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) syncRuntimeEnabledMods(ctx context.Context, server domain.GameServer) error {
	if server.ProviderKey != domain.ProviderTerrariaTModLoader {
		return nil
	}
	mods, err := h.store.ListMods(ctx, server.ID)
	if err != nil {
		return err
	}
	enabled := make([]string, 0, len(mods))
	workshopIDs := make([]string, 0, len(mods))
	for _, item := range mods {
		if !item.Enabled {
			continue
		}
		if item.Source == "workshop" && item.WorkshopID != "" {
			workshopIDs = append(workshopIDs, item.WorkshopID)
			if name := modIdentity(item); name != "" {
				enabled = append(enabled, name)
			}
			continue
		}
		if isTModPackage(item.FileName) {
			if name := modIdentity(item); name != "" {
				enabled = append(enabled, name)
			}
		}
	}
	sort.Strings(enabled)
	sort.Strings(workshopIDs)
	payload, err := json.MarshalIndent(enabled, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	dataDir, err := serverDataDir(server)
	if err != nil {
		return err
	}
	for _, relPath := range terraria.RuntimeModFiles(server.ProviderKey, "enabled.json") {
		targetPath := filepath.Join(dataDir, relPath)
		if err := writeRuntimeDataFile(targetPath, payload); err != nil {
			return err
		}
	}
	if err := h.writeRuntimeInstallFile(server, workshopIDs); err != nil {
		return err
	}
	return nil
}

func (h *Handler) writeRuntimeInstallFile(server domain.GameServer, workshopIDs []string) error {
	content := ""
	if len(workshopIDs) > 0 {
		content = strings.Join(workshopIDs, "\n") + "\n"
	}
	dataDir, err := serverDataDir(server)
	if err != nil {
		return err
	}
	for _, relPath := range terraria.RuntimeModFiles(server.ProviderKey, "install.txt") {
		targetPath := filepath.Join(dataDir, relPath)
		if err := writeRuntimeDataFile(targetPath, []byte(content)); err != nil {
			return err
		}
	}
	return nil
}

func isTModPackage(fileName string) bool {
	return strings.EqualFold(filepath.Ext(fileName), ".tmod")
}

func (h *Handler) visibleMods(ctx context.Context, mods []domain.ModFile) ([]domain.ModFile, error) {
	svc := modsvc.NewService(h.cfg.DataDir)
	visible := make([]domain.ModFile, 0, len(mods))
	for _, item := range mods {
		if item.Source == "workshop" {
			hydrateModMetadata(&item)
			visible = append(visible, item)
			continue
		}
		path, err := svc.Path(item.InstanceID, item.FileName)
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

func (h *Handler) visibleServerMods(ctx context.Context, server domain.GameServer, mods []domain.ModFile) ([]domain.ModFile, error) {
	visible, err := h.visibleMods(ctx, mods)
	if err != nil {
		return nil, err
	}
	runtimeEnabled, err := readRuntimeEnabledMods(server)
	if err != nil {
		h.logger.Warn("failed to read runtime enabled mods", "server", server.ID, "error", err)
		return visible, nil
	}
	for index := range visible {
		visible[index].GameKey = server.GameKey
		visible[index].ProviderKey = server.ProviderKey
		present := runtimeModPresent(server, visible[index])
		visible[index].RuntimePresent = &present
		if runtimeEnabled == nil {
			continue
		}
		enabled := false
		if _, ok := runtimeEnabled[modIdentity(visible[index])]; ok {
			enabled = true
		}
		visible[index].RuntimeEnabled = &enabled
	}
	return visible, nil
}

func runtimeModPresent(server domain.GameServer, item domain.ModFile) bool {
	dataDir := strings.TrimSpace(server.Spec.Runtime.DataDir)
	if server.ProviderKey != domain.ProviderTerrariaTModLoader || dataDir == "" {
		return true
	}
	candidates := []string{filepath.Join(dataDir, "Mods", item.FileName)}
	if identity := modIdentity(item); identity != "" {
		candidates = append(candidates, filepath.Join(dataDir, "Mods", identity+".tmod"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func readRuntimeEnabledMods(server domain.GameServer) (map[string]struct{}, error) {
	dataDir := strings.TrimSpace(server.Spec.Runtime.DataDir)
	if server.ProviderKey != domain.ProviderTerrariaTModLoader || dataDir == "" {
		return nil, nil
	}
	path := filepath.Join(dataDir, "Mods", "enabled.json")
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var values []string
	if err := json.Unmarshal(content, &values); err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result, nil
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
		mod, _, err := h.upsertWorkshopModRecord(ctx, item.InstanceID, workshopID)
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
