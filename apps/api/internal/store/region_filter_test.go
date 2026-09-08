package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestServerRegionFilterUsesExplicitOwnership(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "region-filter.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for id, region := range map[string]any{"hk-node": "HK", "us-node": "us", "empty-node": "", "null-node": nil} {
		if err := db.db.Table("compute_nodes").Create(map[string]any{"id": id, "region": region}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.db.Create(&domain.GameServer{ID: id, NodeID: id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, region := range []string{"hk", " HK "} {
		page, err := db.ListGameServersPage(context.Background(), GameServerListOptions{Region: region})
		if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].NodeID != "hk-node" {
			t.Fatalf("unknown region included in hk: %+v %v", page, err)
		}
	}
	page, err := db.ListGameServersPage(context.Background(), GameServerListOptions{Region: "missing"})
	if err != nil || page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("empty region lookup: %+v %v", page, err)
	}
	page, err = db.ListGameServersPage(context.Background(), GameServerListOptions{Region: "all"})
	if err != nil || page.Total != 4 {
		t.Fatalf("unfiltered history lost: %+v %v", page, err)
	}
}
