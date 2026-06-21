package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestListActivityByInstanceFiltersBeforeLimit(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "gamepanel.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	base := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	if err := db.CreateActivity(ctx, &domain.ActivityEvent{
		ID:         "target-old",
		InstanceID: "target-server",
		Type:       "server.started",
		Message:    "Started target server",
		CreatedAt:  base,
	}); err != nil {
		t.Fatalf("create target activity: %v", err)
	}
	for i := 0; i < 60; i++ {
		if err := db.CreateActivity(ctx, &domain.ActivityEvent{
			ID:         "other-" + time.Duration(i).String(),
			InstanceID: "other-server",
			Type:       "server.updated",
			Message:    "Updated other server",
			CreatedAt:  base.Add(time.Duration(i+1) * time.Minute),
		}); err != nil {
			t.Fatalf("create other activity %d: %v", i, err)
		}
	}

	global, err := db.ListActivity(ctx, 50)
	if err != nil {
		t.Fatalf("list global activity: %v", err)
	}
	for _, event := range global {
		if event.InstanceID == "target-server" {
			t.Fatalf("target activity unexpectedly present in global limited result")
		}
	}

	target, err := db.ListActivityByInstance(ctx, "target-server", 50)
	if err != nil {
		t.Fatalf("list target activity: %v", err)
	}
	if len(target) != 1 || target[0].ID != "target-old" {
		t.Fatalf("expected target activity after instance filter, got %#v", target)
	}
}

func TestListActivityByInstanceOrdersEqualTimestampsByInsertOrder(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "gamepanel.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	createdAt := time.Date(2026, 6, 21, 12, 30, 0, 0, time.UTC)
	for _, event := range []domain.ActivityEvent{
		{ID: "first", InstanceID: "server-1", Type: "server.runtime.created", Message: "Runtime created", CreatedAt: createdAt},
		{ID: "second", InstanceID: "server-1", Type: "server.started", Message: "Server started", CreatedAt: createdAt},
	} {
		item := event
		if err := db.CreateActivity(ctx, &item); err != nil {
			t.Fatalf("create activity %s: %v", item.ID, err)
		}
	}

	events, err := db.ListActivityByInstance(ctx, "server-1", 50)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(events) != 2 || events[0].ID != "second" || events[1].ID != "first" {
		t.Fatalf("expected equal timestamps ordered by newest insert first, got %#v", events)
	}
}
