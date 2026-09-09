package v1

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEnvelopePreservesTypedIdentity(t *testing.T) {
	want := Envelope[struct {
		WorkspaceID WorkspaceID `json:"workspaceId"`
	}]{
		SchemaVersion:  1,
		MessageID:      EventID("evt_01"),
		MessageType:    "workspace.selected.v1",
		OccurredAt:     time.Date(2026, time.September, 10, 1, 2, 3, 0, time.UTC),
		IdempotencyKey: IdempotencyKey("idem_01"),
		Payload: struct {
			WorkspaceID WorkspaceID `json:"workspaceId"`
		}{WorkspaceID: WorkspaceID("ws_01")},
	}

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Envelope[struct {
		WorkspaceID WorkspaceID `json:"workspaceId"`
	}]
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.MessageID != want.MessageID || got.Payload.WorkspaceID != want.Payload.WorkspaceID {
		t.Fatalf("typed identity changed during round trip: got %#v", got)
	}
}
