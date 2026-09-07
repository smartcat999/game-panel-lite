package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("GAMEPANEL_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("set GAMEPANEL_TEST_POSTGRES_URL for a PostgreSQL integration run")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("test database must be a PostgreSQL URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal("open integration database")
	}
	defer admin.Close()
	schema := "gamepanel_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := OpenConfigured("", parsed.String(), 4)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := domain.GameServer{ID: "server", Name: "test", ProviderKey: domain.ProviderTerrariaVanilla, Spec: domain.ServerSpec{Generation: 1, ConfigVersion: 1, Config: map[string]any{"worldName": "世界"}}}
	if err := db.CreateGameServer(ctx, &server); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetGameServer(ctx, server.ID)
	if err != nil || got.Spec.ConfigVersion != 1 || got.Spec.Config["worldName"] != "世界" {
		t.Fatalf("JSON roundtrip: %+v %v", got, err)
	}
	reject := errors.New("rollback")
	err = db.Transaction(ctx, func(tx *Store) error {
		got.Name = "should-rollback"
		if err := tx.SaveGameServer(ctx, &got); err != nil {
			return err
		}
		return reject
	})
	if !errors.Is(err, reject) {
		t.Fatalf("transaction: %v", err)
	}
	got, err = db.GetGameServer(ctx, server.ID)
	if err != nil || got.Name != "test" {
		t.Fatalf("rollback not retained: %+v %v", got, err)
	}
	stamp := time.Now().UTC()
	for _, id := range []string{"a", "z"} {
		event := domain.ActivityEvent{ID: id, InstanceID: server.ID, CreatedAt: stamp}
		if err := db.CreateActivity(ctx, &event); err != nil {
			t.Fatal(err)
		}
		job := domain.GameUpdateJob{ID: id, InstanceID: server.ID, CreatedAt: stamp}
		if err := db.CreateGameUpdateJob(ctx, &job); err != nil {
			t.Fatal(err)
		}
	}
	events, err := db.ListActivityByInstance(ctx, server.ID, 10)
	if err != nil || len(events) != 2 || events[0].ID != "z" {
		t.Fatalf("activity order: %+v %v", events, err)
	}
	job, err := db.GetLatestGameUpdateJobByInstance(ctx, server.ID)
	if err != nil || job.ID != "z" {
		t.Fatalf("job query: %+v %v", job, err)
	}
	if _, err := db.ListActiveGameUpdateJobs(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetGameServer(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("not found: %v", err)
	}
}

func TestPostgresConfigRejectsInvalidPoolAndRedactsDSN(t *testing.T) {
	if _, err := OpenConfigured("", "postgres://example", -1); err == nil {
		t.Fatal("negative connection pool accepted")
	}
	_, err := OpenConfigured("", "postgres://user:secret%not-a-url@example", 1)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe connection error: %v", err)
	}
}
