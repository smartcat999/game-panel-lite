package store

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"gorm.io/gorm"
)

func testGlobalAssets(t *testing.T, db *Store, owner string) []instances.AssetVersion {
	t.Helper()
	ctx := context.Background()
	foreign := domain.Organization{ID: "asset-other", Slug: "asset-other"}
	if err := db.CreateOrganization(ctx, &foreign, "asset-other-owner"); err != nil {
		t.Fatal(err)
	}
	v := assets.PublishedVersion{AssetID: "foreign-asset", OrganizationID: foreign.ID, Version: "v1", SHA256: strings.Repeat("a", 64), SizeBytes: 1}
	if err := db.PublishAssetVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
	v.OrganizationID = owner
	if err := db.PublishAssetVersion(ctx, v); err == nil {
		t.Fatal("asset owner reassigned")
	}
	refs := make([]instances.AssetVersion, 0, 101)
	for i := 0; i < 101; i++ {
		v.AssetID = fmt.Sprintf("asset-%03d", i)
		v.Version = "v1"
		if err := db.PublishAssetVersion(ctx, v); err != nil {
			t.Fatal(err)
		}
		if err := db.PublishAssetVersion(ctx, v); err != nil {
			t.Fatalf("idempotent publication: %v", err)
		}
		refs = append(refs, instances.AssetVersion{AssetID: v.AssetID, Version: v.Version})
	}
	v.SHA256 = strings.Repeat("b", 64)
	if err := db.PublishAssetVersion(ctx, v); err == nil {
		t.Fatal("version digest changed")
	}
	v.Version = "v2"
	if err := db.PublishAssetVersion(ctx, v); err != nil {
		t.Fatalf("new immutable version: %v", err)
	}
	var queries atomic.Int64
	const callback = "test:asset-batch-queries"
	if err := db.db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "global_assets" || tx.Statement.Table == "global_asset_versions" {
			queries.Add(1)
			if strings.Contains(strings.ToUpper(tx.Statement.SQL.String()), " JOIN ") {
				t.Error("asset admission generated a JOIN")
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := db.checkGlobalAssets(ctx, owner, refs)
	if removeErr := db.db.Callback().Query().Remove(callback); removeErr != nil {
		t.Fatal(removeErr)
	}
	if err != nil {
		t.Fatalf("batch boundary: %v", err)
	}
	if queries.Load() != 4 {
		t.Fatalf("101 references should use four queries, got %d", queries.Load())
	}
	ordered := []instances.AssetVersion{refs[100], refs[0], refs[50]}
	resolved, err := db.resolveGlobalAssets(ctx, owner, ordered)
	if err != nil || len(resolved) != len(ordered) {
		t.Fatalf("resolve metadata: %v", err)
	}
	for i, ref := range ordered {
		if resolved[i].AssetID != ref.AssetID || resolved[i].Version != ref.Version {
			t.Fatal("resolver changed reference order")
		}
	}
	invalid := append(append([]instances.AssetVersion(nil), refs...), instances.AssetVersion{AssetID: "missing", Version: "v1"})
	if partial, err := db.resolveGlobalAssets(ctx, owner, invalid); err == nil || partial != nil {
		t.Fatal("failed batch returned partial metadata")
	}
	for _, statement := range []string{
		"UPDATE global_assets SET organization_id = 'asset-other' WHERE id = 'asset-000'",
		"DELETE FROM global_assets WHERE id = 'foreign-asset'",
		"UPDATE global_asset_versions SET size_bytes = 2 WHERE asset_id = 'asset-000'",
		"DELETE FROM global_asset_versions WHERE asset_id = 'asset-000'",
	} {
		if err := db.db.Exec(statement).Error; err == nil {
			t.Fatalf("immutable record mutated: %s", statement)
		}
	}
	if db.db.Dialector.Name() == "sqlite" {
		if err := db.db.Exec("INSERT OR REPLACE INTO global_asset_versions(asset_id,version,sha256,size_bytes) VALUES(?,?,?,?)", "asset-000", "v1", strings.Repeat("c", 64), 2).Error; err == nil {
			t.Fatal("replace bypassed immutability")
		}
		if err := migrateSQLiteGlobalAssets(db.db); err != nil {
			t.Fatal(err)
		}
	}
	return refs
}
