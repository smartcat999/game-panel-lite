package regional

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

type inboxFunc func(context.Context, instances.RevisionAvailable) error

func (f inboxFunc) RecordRevisionNotification(ctx context.Context, e instances.RevisionAvailable) error {
	return f(ctx, e)
}

func TestIngressValidationAndCommitResult(t *testing.T) {
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "owner", ServerID: "server", RevisionID: "revision", RegionID: "east", PlacementEpoch: 1, SpecGeneration: 1}
	body, _ := json.Marshal(event)
	calls := 0
	injected := errors.New("transaction rolled back")
	h := Ingress{RegionID: "east", MaxBytes: 2048, Inbox: inboxFunc(func(_ context.Context, e instances.RevisionAvailable) error {
		calls++
		if e != event {
			t.Fatal("changed decoded event")
		}
		return injected
	})}
	valid := Notification{ID: "event", ContentType: "application/json", Body: body}
	if err := h.Handle(context.Background(), valid); !errors.Is(err, injected) {
		t.Fatalf("transaction failure suppressed: %v", err)
	}
	for name, message := range map[string]Notification{
		"header":       {ID: "different", ContentType: valid.ContentType, Body: body},
		"content_type": {ID: valid.ID, ContentType: "text/plain", Body: body},
		"trailing":     {ID: valid.ID, ContentType: valid.ContentType, Body: append(append([]byte{}, body...), []byte(" {}")...)},
		"region":       {ID: valid.ID, ContentType: valid.ContentType, Body: []byte(strings.Replace(string(body), "east", "west", 1))},
		"version":      {ID: valid.ID, ContentType: valid.ContentType, Body: []byte(strings.Replace(string(body), `"schemaVersion":1`, `"schemaVersion":2`, 1))},
		"duplicate":    {ID: valid.ID, ContentType: valid.ContentType, Body: []byte(strings.Replace(string(body), `"eventId":"event"`, `"eventId":"event","EventId":"event"`, 1))},
		"oversize":     {ID: valid.ID, ContentType: valid.ContentType, Body: []byte(strings.Repeat(" ", 2049))},
	} {
		t.Run(name, func(t *testing.T) {
			if err := h.Handle(context.Background(), message); !errors.Is(err, ErrInvalidNotification) {
				t.Fatalf("invalid notification accepted: %v", err)
			}
		})
	}
	if calls != 1 {
		t.Fatalf("invalid messages reached persistence: %d", calls)
	}
	h.Inbox = inboxFunc(func(context.Context, instances.RevisionAvailable) error { return nil })
	if err := h.Handle(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
}
