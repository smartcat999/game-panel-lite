package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

func testRegionalNodeSessions(t *testing.T, db *RegionalStore) {
	ctx := context.Background()
	nodeID := "node-m"
	if _, err := db.StartRegionalNodeSession(ctx, "unknown"); !errors.Is(err, regional.ErrNodeUnavailable) {
		t.Fatal("unknown node enrolled")
	}
	session, err := db.StartRegionalNodeSession(ctx, nodeID)
	if err != nil || session.Epoch != 1 {
		t.Fatal("start session", err)
	}
	read := func() regionalNodeSessionRow {
		t.Helper()
		var row regionalNodeSessionRow
		if err := db.db.Table("regional_node_sessions").Where("node_id = ?", nodeID).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	if row := read(); row.LastSeenMS != 0 || row.RuntimeReady || row.Sequence != 0 {
		t.Fatal("new session fabricated liveness")
	}
	h := regional.NodeHeartbeat{SessionEpoch: session.Epoch, Sequence: 1, Architecture: "amd64", RuntimeReady: true}
	if err := db.RecordRegionalNodeHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	if row := read(); row.LastSeenMS <= 0 || !row.RuntimeReady || row.Sequence != 1 {
		t.Fatal("heartbeat not persisted")
	}
	if err := db.db.Table("regional_node_sessions").Where("node_id = ?", nodeID).Update("last_seen_ms", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.RecordRegionalNodeHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	if read().LastSeenMS != 1 {
		t.Fatal("duplicate refreshed liveness")
	}
	h.Sequence = 2
	h.RuntimeReady = false
	if err := db.RecordRegionalNodeHeartbeat(ctx, nodeID, h); err != nil {
		t.Fatal(err)
	}
	current := read()
	old := h
	old.Sequence = 1
	if !errors.Is(db.RecordRegionalNodeHeartbeat(ctx, nodeID, old), regional.ErrNodeHeartbeatStale) {
		t.Fatal("old sequence accepted")
	}
	conflict := h
	conflict.RuntimeReady = true
	if !errors.Is(db.RecordRegionalNodeHeartbeat(ctx, nodeID, conflict), regional.ErrNodeHeartbeatStale) {
		t.Fatal("conflicting retry accepted")
	}
	wrong := h
	wrong.Sequence = 3
	wrong.Architecture = "arm64"
	if !errors.Is(db.RecordRegionalNodeHeartbeat(ctx, nodeID, wrong), regional.ErrInvalidNode) {
		t.Fatal("unconfigured architecture accepted")
	}
	if read() != current {
		t.Fatal("rejected heartbeat mutated state")
	}
	next, err := db.StartRegionalNodeSession(ctx, nodeID)
	if err != nil || next.Epoch != 2 {
		t.Fatal("session not advanced")
	}
	if !errors.Is(db.RecordRegionalNodeHeartbeat(ctx, nodeID, h), regional.ErrNodeHeartbeatStale) {
		t.Fatal("old session refreshed liveness")
	}
	if row := read(); row.LastSeenMS != 0 || row.RuntimeReady {
		t.Fatal("rotated session retained liveness")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 1; i <= 8; i++ {
		wg.Add(1)
		go func(seq int64) {
			defer wg.Done()
			errs <- db.RecordRegionalNodeHeartbeat(ctx, nodeID, regional.NodeHeartbeat{SessionEpoch: next.Epoch, Sequence: seq, Architecture: "amd64", RuntimeReady: true})
		}(int64(i))
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, regional.ErrNodeHeartbeatStale) {
			t.Fatal(err)
		}
	}
	if row := read(); row.Sequence != 8 || !row.RuntimeReady || row.LastSeenMS <= 0 {
		t.Fatal("concurrent heartbeat regressed")
	}
	nodes, err := db.ListRegionalNodes(ctx, "node-c", 1)
	if err != nil || len(nodes) != 1 || nodes[0].Version != 2 || nodes[0].Schedulable || nodes[0].CPU != 4 {
		t.Fatal("heartbeat changed operator configuration")
	}
}
