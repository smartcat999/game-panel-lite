package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestGameUpdateJobCRUDAndLatestByInstance(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "gamepanel.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	createdAt := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	first := domain.GameUpdateJob{
		ID:               "update-1",
		InstanceID:       "server-1",
		ProviderKey:      domain.ProviderPalworld,
		Operation:        domain.GameUpdateOperationCheck,
		Status:           domain.GameUpdateJobSucceeded,
		Stage:            domain.GameUpdateStageCompleted,
		Progress:         100,
		InstalledBuildID: "100",
		LatestBuildID:    "101",
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt,
	}
	if err := db.CreateGameUpdateJob(ctx, &first); err != nil {
		t.Fatalf("create first update job: %v", err)
	}
	second := domain.GameUpdateJob{
		ID:               "update-2",
		InstanceID:       "server-1",
		ProviderKey:      domain.ProviderPalworld,
		Operation:        domain.GameUpdateOperationApply,
		Status:           domain.GameUpdateJobQueued,
		Stage:            domain.GameUpdateStageQueued,
		InstalledBuildID: "101",
		LatestBuildID:    "102",
		StartAfterUpdate: true,
		WasRunning:       true,
		CreatedAt:        createdAt.Add(time.Minute),
		UpdatedAt:        createdAt.Add(time.Minute),
	}
	if err := db.CreateGameUpdateJob(ctx, &second); err != nil {
		t.Fatalf("create second update job: %v", err)
	}

	stored, err := db.GetGameUpdateJobByID(ctx, second.ID)
	if err != nil {
		t.Fatalf("get update job by id: %v", err)
	}
	if stored.ProviderKey != domain.ProviderPalworld || stored.Operation != domain.GameUpdateOperationApply || !stored.StartAfterUpdate || !stored.WasRunning {
		t.Fatalf("expected update job fields to round trip, got %#v", stored)
	}

	stored.Status = domain.GameUpdateJobRunning
	stored.Stage = domain.GameUpdateStageDownloading
	stored.Progress = 55
	stored.Error = "transient download message"
	if err := db.SaveGameUpdateJob(ctx, &stored); err != nil {
		t.Fatalf("save update job: %v", err)
	}

	latest, err := db.GetLatestGameUpdateJobByInstance(ctx, "server-1")
	if err != nil {
		t.Fatalf("get latest update job: %v", err)
	}
	if latest.ID != second.ID || latest.Status != domain.GameUpdateJobRunning || latest.Stage != domain.GameUpdateStageDownloading || latest.Progress != 55 {
		t.Fatalf("expected saved latest job, got %#v", latest)
	}
	if latest.Error != "transient download message" {
		t.Fatalf("expected error to round trip, got %q", latest.Error)
	}

	completedAt := createdAt.Add(2 * time.Minute)
	checkedAt := createdAt.Add(90 * time.Second)
	latest.Status = domain.GameUpdateJobSucceeded
	latest.Stage = domain.GameUpdateStageCompleted
	latest.Progress = 100
	latest.CheckedAt = &checkedAt
	latest.CompletedAt = &completedAt
	if err := db.SaveGameUpdateJob(ctx, &latest); err != nil {
		t.Fatalf("save completed update job: %v", err)
	}
	completed, err := db.GetGameUpdateJobByID(ctx, latest.ID)
	if err != nil {
		t.Fatalf("get completed update job: %v", err)
	}
	if completed.CheckedAt == nil || !completed.CheckedAt.Equal(checkedAt) || completed.CompletedAt == nil || !completed.CompletedAt.Equal(completedAt) {
		t.Fatalf("expected completion timestamp %v, got %v", completedAt, completed.CompletedAt)
	}
}

func TestGameUpdateJobActiveQueries(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "gamepanel.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	createdAt := time.Date(2026, 7, 15, 13, 0, 0, 0, time.UTC)
	jobs := []domain.GameUpdateJob{
		{ID: "completed", InstanceID: "server-1", ProviderKey: domain.ProviderPalworld, Status: domain.GameUpdateJobSucceeded, Stage: domain.GameUpdateStageCompleted, CreatedAt: createdAt},
		{ID: "queued", InstanceID: "server-1", ProviderKey: domain.ProviderPalworld, Status: domain.GameUpdateJobQueued, Stage: domain.GameUpdateStageQueued, CreatedAt: createdAt.Add(time.Minute)},
		{ID: "running", InstanceID: "server-2", ProviderKey: domain.ProviderPalworld, Status: domain.GameUpdateJobRunning, Stage: domain.GameUpdateStageValidating, CreatedAt: createdAt.Add(2 * time.Minute)},
		{ID: "failed", InstanceID: "server-3", ProviderKey: domain.ProviderPalworld, Status: domain.GameUpdateJobFailed, Stage: domain.GameUpdateStageInstalling, CreatedAt: createdAt.Add(3 * time.Minute)},
	}
	for index := range jobs {
		if err := db.CreateGameUpdateJob(ctx, &jobs[index]); err != nil {
			t.Fatalf("create update job %s: %v", jobs[index].ID, err)
		}
	}

	active, err := db.GetActiveGameUpdateJobByInstance(ctx, "server-1")
	if err != nil {
		t.Fatalf("get active update job: %v", err)
	}
	if active.ID != "queued" {
		t.Fatalf("expected queued active job, got %#v", active)
	}

	allActive, err := db.ListActiveGameUpdateJobs(ctx)
	if err != nil {
		t.Fatalf("list active update jobs: %v", err)
	}
	if len(allActive) != 2 || allActive[0].ID != "queued" || allActive[1].ID != "running" {
		t.Fatalf("expected queued and running jobs in creation order, got %#v", allActive)
	}

	if _, err := db.GetActiveGameUpdateJobByInstance(ctx, "server-3"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected inactive instance to return ErrNotFound, got %v", err)
	}
	if _, err := db.GetGameUpdateJobByID(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing job to return ErrNotFound, got %v", err)
	}
	if _, err := db.GetLatestGameUpdateJobByInstance(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing instance to return ErrNotFound, got %v", err)
	}
}
