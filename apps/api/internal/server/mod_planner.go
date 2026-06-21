package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

type ModPlanner interface {
	PlanMods(context.Context, domain.GameServer) error
}

type ModStore interface {
	GetMod(context.Context, string) (domain.ModFile, error)
	GetModByInstanceAndFile(context.Context, string, string) (domain.ModFile, error)
	GetModByInstanceAndWorkshopID(context.Context, string, string) (domain.ModFile, error)
	ListMods(context.Context, string) ([]domain.ModFile, error)
	CreateMod(context.Context, *domain.ModFile) error
	SaveMod(context.Context, *domain.ModFile) error
}

type RuntimeModPlanner struct {
	dataDir string
	store   ModStore
}

func NewRuntimeModPlanner(dataDir string, store ModStore) *RuntimeModPlanner {
	return &RuntimeModPlanner{dataDir: dataDir, store: store}
}

func (p *RuntimeModPlanner) PlanMods(ctx context.Context, server domain.GameServer) error {
	if p == nil || p.store == nil || !providerSupportsMods(server.ProviderKey) {
		return nil
	}
	roots := make([]domain.ModFile, 0, len(server.Spec.ModIDs))
	for _, modID := range uniqueModIDs(server.Spec.ModIDs) {
		item, err := p.store.GetMod(ctx, modID)
		if err != nil {
			return fmt.Errorf("resolve desired mod %s: %w", modID, err)
		}
		assigned, err := p.assignLibraryMod(ctx, server, item)
		if err != nil {
			return err
		}
		roots = append(roots, assigned)
	}
	if _, err := p.ensureModDependencies(ctx, server, roots); err != nil {
		return err
	}
	if server.ProviderKey != domain.ProviderTerrariaTModLoader {
		return nil
	}
	return p.syncRuntimeEnabledMods(ctx, server)
}

func (p *RuntimeModPlanner) assignLibraryMod(ctx context.Context, server domain.GameServer, item domain.ModFile) (domain.ModFile, error) {
	if item.InstanceID != "unassigned" && item.InstanceID != server.ID {
		return domain.ModFile{}, fmt.Errorf("mod %s is not available in the library", item.ID)
	}
	if item.Source == "workshop" {
		if !providerSupportsWorkshopMods(server.ProviderKey) {
			return domain.ModFile{}, fmt.Errorf("workshop mods are not supported for provider %s", server.ProviderKey)
		}
		assigned, _, err := p.upsertWorkshopModRecord(ctx, server.ProviderKey, server.ID, item.WorkshopID)
		return assigned, err
	}
	if !providerSupportsUploadedMods(server.ProviderKey) {
		return domain.ModFile{}, fmt.Errorf("uploaded mods are not supported for provider %s", server.ProviderKey)
	}
	size, err := p.copyLibraryModToServerCache(item, server.ID)
	if err != nil {
		return domain.ModFile{}, err
	}
	assigned, _, err := p.upsertModRecord(ctx, server.ProviderKey, server.ID, item.FileName, size, metadataFromMod(item))
	if err != nil {
		return domain.ModFile{}, err
	}
	if err := p.materializeModForRuntime(assigned, server); err != nil {
		return domain.ModFile{}, err
	}
	return assigned, nil
}

func (p *RuntimeModPlanner) copyLibraryModToServerCache(item domain.ModFile, targetInstanceID string) (int64, error) {
	svc := modsvc.NewService(p.dataDir)
	sourcePath, err := svc.Path(item.InstanceID, item.ProviderKey, item.FileName)
	if err != nil {
		return 0, err
	}
	src, err := os.Open(sourcePath)
	if err != nil {
		return 0, fmt.Errorf("mod file not found")
	}
	defer src.Close()
	_, size, err := svc.Upload(targetInstanceID, item.ProviderKey, item.FileName, src)
	return size, err
}

func (p *RuntimeModPlanner) upsertModRecord(ctx context.Context, providerKey domain.ProviderKey, instanceID string, fileName string, size int64, metadata modsvc.Metadata) (domain.ModFile, bool, error) {
	if existing, err := p.store.GetModByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.SizeBytes = size
		existing.Enabled = true
		existing.ProviderKey = providerKey
		existing.GameKey = gameKeyForProvider(providerKey)
		if existing.Source == "" {
			existing.Source = "upload"
		}
		if providerKey == domain.ProviderTerrariaTModLoader {
			applyTModMetadata(&existing, metadata)
		}
		applyFileModMetadata(&existing)
		hydrateModMetadata(&existing)
		return existing, false, p.store.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	item := domain.ModFile{ID: uuid.NewString(), InstanceID: instanceID, GameKey: gameKeyForProvider(providerKey), ProviderKey: providerKey, FileName: fileName, Source: "upload", SizeBytes: size, Enabled: true, CreatedAt: time.Now()}
	if providerKey == domain.ProviderTerrariaTModLoader {
		applyTModMetadata(&item, metadata)
	}
	applyFileModMetadata(&item)
	hydrateModMetadata(&item)
	return item, true, p.store.CreateMod(ctx, &item)
}

func (p *RuntimeModPlanner) upsertWorkshopModRecord(ctx context.Context, providerKey domain.ProviderKey, instanceID string, workshopID string) (domain.ModFile, bool, error) {
	if workshopID == "" {
		return domain.ModFile{}, false, fmt.Errorf("workshop mod is missing workshop id")
	}
	fileName := "workshop-" + workshopID
	if existing, err := p.store.GetModByInstanceAndWorkshopID(ctx, instanceID, workshopID); err == nil {
		existing.Source = "workshop"
		existing.WorkshopID = workshopID
		existing.ProviderKey = providerKey
		existing.GameKey = gameKeyForProvider(providerKey)
		existing.Enabled = true
		applyRecommendedModMetadataForProvider(&existing, providerKey, workshopID)
		return existing, false, p.store.SaveMod(ctx, &existing)
	} else if !errors.Is(err, store.ErrNotFound) {
		return domain.ModFile{}, false, err
	}
	if existing, err := p.store.GetModByInstanceAndFile(ctx, instanceID, fileName); err == nil {
		existing.Source = "workshop"
		existing.WorkshopID = workshopID
		existing.ProviderKey = providerKey
		existing.GameKey = gameKeyForProvider(providerKey)
		existing.Enabled = true
		applyRecommendedModMetadataForProvider(&existing, providerKey, workshopID)
		return existing, false, p.store.SaveMod(ctx, &existing)
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
	return item, true, p.store.CreateMod(ctx, &item)
}

func (p *RuntimeModPlanner) ensureModDependencies(ctx context.Context, server domain.GameServer, roots []domain.ModFile) ([]domain.ModFile, error) {
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
			dependency, created, err := p.ensureModDependency(ctx, server, dependencyName)
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

func (p *RuntimeModPlanner) ensureModDependency(ctx context.Context, server domain.GameServer, dependencyName string) (domain.ModFile, bool, error) {
	dependencyName = strings.TrimSpace(dependencyName)
	if dependencyName == "" {
		return domain.ModFile{}, false, nil
	}
	if existing, ok, err := p.findServerModByModName(ctx, server.ID, dependencyName); err != nil || ok {
		return existing, false, err
	}
	if library, ok, err := p.findLibraryModByModName(ctx, dependencyName); err != nil || ok {
		if err != nil {
			return domain.ModFile{}, false, err
		}
		assigned, err := p.assignLibraryMod(ctx, server, library)
		return assigned, true, err
	}
	recommended, ok := modcatalog.RecommendedTModLoaderModByModName(dependencyName)
	if !ok || recommended.WorkshopID == "" {
		return domain.ModFile{}, false, fmt.Errorf("missing dependency %s in mod library", dependencyName)
	}
	return p.upsertWorkshopModRecord(ctx, server.ProviderKey, server.ID, recommended.WorkshopID)
}

func (p *RuntimeModPlanner) findServerModByModName(ctx context.Context, instanceID string, modName string) (domain.ModFile, bool, error) {
	mods, err := p.store.ListMods(ctx, instanceID)
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

func (p *RuntimeModPlanner) findLibraryModByModName(ctx context.Context, modName string) (domain.ModFile, bool, error) {
	mods, err := p.store.ListMods(ctx, "unassigned")
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

func (p *RuntimeModPlanner) materializeModForRuntime(item domain.ModFile, server domain.GameServer) error {
	if item.Source == "workshop" {
		return nil
	}
	sourcePath, err := modsvc.NewService(p.dataDir).Path(item.InstanceID, item.ProviderKey, item.FileName)
	if err != nil {
		return err
	}
	dataDir := strings.TrimSpace(server.Spec.Runtime.DataDir)
	if dataDir == "" {
		return fmt.Errorf("server data dir is empty")
	}
	for _, relPath := range runtimeModFiles(server.ProviderKey, item.FileName) {
		targetPath := filepath.Join(dataDir, relPath)
		if err := copyFile(sourcePath, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func (p *RuntimeModPlanner) syncRuntimeEnabledMods(ctx context.Context, server domain.GameServer) error {
	mods, err := p.store.ListMods(ctx, server.ID)
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
		if strings.EqualFold(filepath.Ext(item.FileName), ".tmod") {
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
	if err := p.writeRuntimeDataFile(server, "enabled.json", payload); err != nil {
		return err
	}
	content := ""
	if len(workshopIDs) > 0 {
		content = strings.Join(workshopIDs, "\n") + "\n"
	}
	return p.writeRuntimeDataFile(server, "install.txt", []byte(content))
}

func (p *RuntimeModPlanner) writeRuntimeDataFile(server domain.GameServer, fileName string, content []byte) error {
	dataDir := strings.TrimSpace(server.Spec.Runtime.DataDir)
	if dataDir == "" {
		return fmt.Errorf("server data dir is empty")
	}
	for _, relPath := range runtimeModFiles(server.ProviderKey, fileName) {
		targetPath := filepath.Join(dataDir, relPath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, content, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(sourcePath string, targetPath string) error {
	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := out.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, targetPath)
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

func applyRecommendedModMetadata(item *domain.ModFile, workshopID string) {
	applyRecommendedModMetadataForProvider(item, item.ProviderKey, workshopID)
}

func applyRecommendedModMetadataForProvider(item *domain.ModFile, providerKey domain.ProviderKey, workshopID string) {
	recommended, ok := modcatalog.RecommendedModByProviderAndWorkshopID(providerKey, workshopID)
	if !ok {
		hydrateModMetadata(item)
		return
	}
	tags, _ := json.Marshal(recommended.Tags)
	dependencies, _ := json.Marshal(uniqueModIDs(recommended.Dependencies))
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
	if item.ProviderKey == "" {
		item.ProviderKey = domain.ProviderTerrariaTModLoader
	}
	if item.GameKey == "" {
		item.GameKey = gameKeyForProvider(item.ProviderKey)
	}
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

func modIdentity(item domain.ModFile) string {
	if item.WorkshopID != "" {
		if recommended, ok := modcatalog.RecommendedModByProviderAndWorkshopID(item.ProviderKey, item.WorkshopID); ok {
			for _, value := range []string{recommended.ModName, recommended.Title} {
				value = strings.TrimSpace(value)
				if value != "" {
					return value
				}
			}
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

func providerSupportsMods(providerKey domain.ProviderKey) bool {
	return providerKey == domain.ProviderTerrariaTModLoader || providerKey == domain.ProviderDST || providerKey == domain.ProviderPalworld
}

func providerSupportsUploadedMods(providerKey domain.ProviderKey) bool {
	return providerKey == domain.ProviderTerrariaTModLoader || providerKey == domain.ProviderPalworld
}

func providerSupportsWorkshopMods(providerKey domain.ProviderKey) bool {
	return providerKey == domain.ProviderTerrariaTModLoader || providerKey == domain.ProviderDST
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

func runtimeModFiles(providerKey domain.ProviderKey, fileName string) []string {
	switch providerKey {
	case domain.ProviderPalworld:
		return []string{filepath.Join("Pal", "Content", "Paks", "~mods", fileName)}
	default:
		return terraria.RuntimeModFiles(providerKey, fileName)
	}
}

func modDependencies(item domain.ModFile) []string {
	if len(item.Dependencies) > 0 {
		return uniqueModIDs(item.Dependencies)
	}
	if strings.TrimSpace(item.DependenciesJSON) != "" {
		var values []string
		if err := json.Unmarshal([]byte(item.DependenciesJSON), &values); err == nil {
			return uniqueModIDs(values)
		}
	}
	if item.WorkshopID != "" {
		if recommended, ok := modcatalog.RecommendedTModLoaderModByWorkshopID(item.WorkshopID); ok {
			return uniqueModIDs(recommended.Dependencies)
		}
	}
	if recommended, ok := modcatalog.RecommendedTModLoaderModByModName(modIdentity(item)); ok {
		return uniqueModIDs(recommended.Dependencies)
	}
	return nil
}

func uniqueModIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
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
