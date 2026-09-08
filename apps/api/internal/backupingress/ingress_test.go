package backupingress

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

type inboxFunc func(context.Context, backup.Requested) error

func (f inboxFunc) RecordBackupRequest(ctx context.Context, e backup.Requested) error {
	return f(ctx, e)
}

func TestBackupIngressValidation(t *testing.T) {
	e := backup.Requested{SchemaVersion: 1, EventID: "event", OperationID: "operation", BackupID: "backup", OrganizationID: "tenant", ServerID: "server", RegionID: "east", RevisionID: "revision", SpecGeneration: 1, IntentVersion: 1, PlacementEpoch: 1, Scope: "world"}
	payload, _ := json.Marshal(e)
	calls := 0
	failure := errors.New("database unavailable")
	h := Ingress{RegionID: "east", MaxBytes: 4096, Inbox: inboxFunc(func(_ context.Context, got backup.Requested) error {
		calls++
		if got != e {
			t.Fatal("identity changed")
		}
		return failure
	})}
	message := regional.Notification{ID: e.EventID, ContentType: "application/json", Body: payload}
	if !errors.Is(h.Handle(context.Background(), message), failure) || calls != 1 {
		t.Fatal("persistence failure swallowed")
	}
	for _, body := range []string{
		string(payload) + " {}",
		strings.Replace(string(payload), `"regionId":"east"`, `"regionId":"east","RegionId":"west"`, 1),
		strings.Replace(string(payload), `"scope":"world"`, `"scope":"world","path":"/host"`, 1),
		strings.Replace(string(payload), `"schemaVersion":1`, `"schemaVersion":2`, 1),
		strings.Replace(string(payload), `"regionId":"east"`, `"regionId":"west"`, 1),
		strings.Repeat("x", 4097),
	} {
		bad := message
		bad.Body = []byte(body)
		if !errors.Is(h.Handle(context.Background(), bad), regional.ErrInvalidNotification) {
			t.Fatal("invalid request accepted")
		}
	}
	bad := message
	bad.ID = "other"
	if !errors.Is(h.Handle(context.Background(), bad), regional.ErrInvalidNotification) {
		t.Fatal("envelope mismatch accepted")
	}
	bad = message
	bad.ContentType = "text/plain"
	if !errors.Is(h.Handle(context.Background(), bad), regional.ErrInvalidNotification) {
		t.Fatal("wrong content type accepted")
	}
	if calls != 1 {
		t.Fatal("invalid input reached storage")
	}
}
