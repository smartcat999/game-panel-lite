package app

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
)

const (
	AnnotationBuiltInHash = corev1.AnnotationAppBuiltInHash
	appSpecCachePrefix    = "app:spec:"
)

//go:embed manifests/*.yaml
var manifestsFS embed.FS

var (
	builtInOnce sync.Once
	builtInRaw  [][]byte
)

func loadBuiltInJSON() [][]byte {
	builtInOnce.Do(func() {
		entries, err := fs.ReadDir(manifestsFS, "manifests")
		if err != nil {
			return
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name() < entries[j].Name()
		})
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if rawJSON, ok := parseBuiltInManifest(entry.Name()); ok {
				builtInRaw = append(builtInRaw, rawJSON)
			}
		}
	})
	return builtInRaw
}

func parseBuiltInManifest(filename string) ([]byte, bool) {
	rawYAML, err := manifestsFS.ReadFile("manifests/" + filename)
	if err != nil {
		return nil, false
	}
	var doc map[string]any
	if err := yaml.Unmarshal(rawYAML, &doc); err != nil {
		return nil, false
	}
	if specMap, ok := doc["spec"].(map[string]any); ok {
		delete(doc, "spec")
		for k, v := range specMap {
			doc[k] = v
		}
	}
	sum := sha256.Sum256(rawYAML)
	metaMap, _ := doc["metadata"].(map[string]any)
	if metaMap == nil {
		metaMap = map[string]any{}
		doc["metadata"] = metaMap
	}
	annMap, _ := metaMap["annotations"].(map[string]any)
	if annMap == nil {
		annMap = map[string]any{}
		metaMap["annotations"] = annMap
	}
	annMap[AnnotationBuiltInHash] = hex.EncodeToString(sum[:16])
	rawJSON, err := json.Marshal(doc)
	if err != nil {
		return nil, false
	}
	return rawJSON, true
}

// BuiltInApps loads the official declarative App YAML manifests embedded via embed.FS.
func BuiltInApps() []*corev1.App {
	rawList := loadBuiltInJSON()
	out := make([]*corev1.App, 0, len(rawList))
	for _, rawJSON := range rawList {
		var a corev1.App
		if err := json.Unmarshal(rawJSON, &a); err != nil {
			continue
		}
		a.DefaultMeta()
		a.BuiltIn = true
		out = append(out, &a)
	}
	return out
}

// EnsureBuiltInApps seeds default built-in game applications if they do not exist yet,
// and synchronizes built-in App definitions when embedded YAML manifests evolve.
func EnsureBuiltInApps(ctx context.Context, reg *repo.Registry) error {
	if reg == nil || reg.Pool() == nil {
		return nil
	}
	appDB := appdb.NewDB(reg.Pool())
	for _, a := range BuiltInApps() {
		a.DefaultMeta()
		normalizeAndSortAppVersions(a)
		if err := upsertBuiltInApp(ctx, reg, appDB, a); err != nil {
			return err
		}
	}
	return nil
}

func upsertBuiltInApp(ctx context.Context, reg *repo.Registry, appDB *appdb.DB, a *corev1.App) error {
	existing, err := appDB.Get(ctx, a.Metadata.Name)
	if errors.Is(err, db.ErrNotFound) {
		if _, err := appDB.Create(ctx, a); err != nil && !errors.Is(err, db.ErrAlreadyExists) {
			return fmt.Errorf("create built-in app %s: %w", a.Metadata.Name, err)
		}
		return nil
	}
	if err == nil && existing != nil && existing.BuiltIn {
		newHash := a.Metadata.Annotations[AnnotationBuiltInHash]
		oldHash := existing.Metadata.Annotations[AnnotationBuiltInHash]
		if newHash != "" && oldHash != newHash {
			mergeBuiltInAppUpdate(existing, a)
			if _, uErr := appDB.Update(ctx, existing); uErr != nil && !errors.Is(uErr, db.ErrConflict) {
				return fmt.Errorf("update built-in app %s: %w", a.Metadata.Name, uErr)
			}
			invalidateAndSyncUpdatedApp(ctx, reg, a.Metadata.Name)
		}
	}
	return nil
}

func invalidateAndSyncUpdatedApp(ctx context.Context, reg *repo.Registry, appName string) {
	if c := cache.Default(); c != nil {
		_ = c.Delete(ctx, appSpecCachePrefix+appName)
	}
	_ = SyncAppInstanceMetrics(ctx, reg, nil, appName)
}

func mergeBuiltInAppUpdate(existing, latest *corev1.App) {
	meta := existing.Metadata
	published := existing.Published
	customCover := existing.CoverImage
	savedVersions := existing.Versions
	prevRec := strings.TrimSpace(existing.RecommendedVersion)
	*existing = *latest
	existing.Metadata = meta
	existing.Published = published
	existing.BuiltIn = true
	for _, v := range savedVersions {
		v = strings.TrimSpace(v)
		if v != "" && !slicesContainsString(existing.Versions, v) {
			existing.Versions = append(existing.Versions, v)
		}
	}
	if prevRec != "" && !isLatestAlias(prevRec) && slicesContainsString(existing.Versions, prevRec) &&
		CompareVersionsDesc(prevRec, existing.RecommendedVersion, "") < 0 {
		existing.RecommendedVersion = prevRec
	}
	normalizeAndSortAppVersions(existing)
	if strings.HasPrefix(customCover, "data:") {
		existing.CoverImage = customCover
	}
	if existing.Metadata.Annotations == nil {
		existing.Metadata.Annotations = make(map[string]string)
	}
	if h := latest.Metadata.Annotations[AnnotationBuiltInHash]; h != "" {
		existing.Metadata.Annotations[AnnotationBuiltInHash] = h
	}
}

func slicesContainsString(items []string, target string) bool {
	for _, s := range items {
		if s == target {
			return true
		}
	}
	return false
}
