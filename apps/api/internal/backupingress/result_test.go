package backupingress

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

type resultInboxFunc func(context.Context, string, backup.ArchiveUploaded) error

func (f resultInboxFunc) RecordBackupResult(ctx context.Context, region string, e backup.ArchiveUploaded) error {
	return f(ctx, region, e)
}

func TestResultIngress(t *testing.T) {
	asset := assets.PublishedVersion{AssetID: "asset", OrganizationID: "tenant", Version: "v1", SHA256: strings.Repeat("a", 64), SizeBytes: 12}
	plan := backup.UploadPlan{OperationID: "operation", RequestEventID: "request", ID: "upload", RegionID: "east", ServerID: "server", DeploymentID: "deployment", NodeID: "node", SnapshotID: "snapshot", PlacementEpoch: 1, StorageID: "storage", ObjectKey: "object", Asset: asset}
	event := backup.ArchiveUploaded{SchemaVersion: 1, EventID: "result", Plan: plan, Receipt: backup.StoredArchive{StorageID: plan.StorageID, ObjectKey: plan.ObjectKey, ObjectVersion: "version", Asset: asset}}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	failure := errors.New("database unavailable")
	h := ResultIngress{SourceRegionID: "east", MaxBytes: 32768, Inbox: resultInboxFunc(func(_ context.Context, source string, got backup.ArchiveUploaded) error {
		calls++
		if source != "east" || got != event {
			t.Fatal("result identity changed")
		}
		return failure
	})}
	message := regional.Notification{ID: event.EventID, ContentType: "application/json", Body: body}
	if err := h.Handle(context.Background(), message); !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("persistence error lost: %v calls %d", err, calls)
	}
	h.Inbox = resultInboxFunc(func(context.Context, string, backup.ArchiveUploaded) error { calls++; return nil })
	if err := h.Handle(context.Background(), message); err != nil || calls != 2 {
		t.Fatalf("retry failed: %v", err)
	}
	for name, mutate := range map[string]func(*backup.ArchiveUploaded){
		"source":  func(e *backup.ArchiveUploaded) { e.Plan.RegionID = "west" },
		"event":   func(e *backup.ArchiveUploaded) { e.EventID = "other" },
		"schema":  func(e *backup.ArchiveUploaded) { e.SchemaVersion = 2 },
		"tenant":  func(e *backup.ArchiveUploaded) { e.Receipt.Asset.OrganizationID = "other" },
		"digest":  func(e *backup.ArchiveUploaded) { e.Receipt.Asset.SHA256 = strings.Repeat("b", 64) },
		"storage": func(e *backup.ArchiveUploaded) { e.Receipt.StorageID = "other" },
		"object":  func(e *backup.ArchiveUploaded) { e.Receipt.ObjectKey = "other" },
		"version": func(e *backup.ArchiveUploaded) { e.Receipt.ObjectVersion = strings.Repeat("v", 1025) },
		"epoch":   func(e *backup.ArchiveUploaded) { e.Plan.PlacementEpoch = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			changed := event
			mutate(&changed)
			payload, _ := json.Marshal(changed)
			m := message
			m.Body = payload
			if err := h.Handle(context.Background(), m); !errors.Is(err, regional.ErrInvalidNotification) {
				t.Fatalf("accepted invalid result: %v", err)
			}
		})
	}
	raw := string(body)
	for _, payload := range []string{
		raw + ` {}`, `null`, `[]`,
		strings.Replace(raw, `"schemaVersion":1`, `"schemaVersion":1,"SchemaVersion":1`, 1),
		strings.Replace(raw, `"operationId":"operation"`, `"operationId":"operation","OperationId":"operation"`, 1),
		strings.Replace(raw, `"assetId":"asset"`, `"assetId":"asset","assetId":"asset"`, 1),
		strings.Replace(raw, `"objectVersion":"version"`, `"objectVersion":"version","extra":1`, 1),
		strings.Replace(raw, `"plan":{`, `"plan":{"unknown":null,`, 1),
	} {
		m := message
		m.Body = []byte(payload)
		if err := h.Handle(context.Background(), m); !errors.Is(err, regional.ErrInvalidNotification) {
			t.Fatalf("accepted ambiguous result: %v", err)
		}
	}
	for _, media := range []string{"", "text/plain", "application/json; invalid"} {
		m := message
		m.ContentType = media
		if err := h.Handle(context.Background(), m); !errors.Is(err, regional.ErrInvalidNotification) {
			t.Fatal(err)
		}
	}
	h.MaxBytes = len(body) - 1
	if err := h.Handle(context.Background(), message); !errors.Is(err, regional.ErrInvalidNotification) {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("invalid result reached persistence: %d", calls)
	}
}
