// Package regional owns notification intake before deployment authorization.
package regional

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

var ErrInvalidNotification = errors.New("invalid regional notification")
var ErrNotificationConflict = errors.New("notification identity reused with different contents")

type Notification struct {
	ID, ContentType string
	Body            []byte
}
type Inbox interface {
	RecordRevisionNotification(context.Context, instances.RevisionAvailable) error
}
type Ingress struct {
	RegionID string
	MaxBytes int
	Inbox    Inbox
}

func (h Ingress) Handle(ctx context.Context, message Notification) error {
	if h.RegionID == "" || h.MaxBytes < 1 || h.Inbox == nil {
		return errors.New("invalid regional ingress configuration")
	}
	mediaType, _, err := mime.ParseMediaType(message.ContentType)
	if err != nil || mediaType != "application/json" || len(message.Body) == 0 || len(message.Body) > h.MaxBytes {
		return ErrInvalidNotification
	}
	event, err := DecodeRevisionNotification(message.Body)
	if err != nil || event.RegionID != h.RegionID || event.EventID != message.ID {
		return ErrInvalidNotification
	}
	return h.Inbox.RecordRevisionNotification(ctx, event)
}

// DecodeRevisionNotification applies the same event contract to broker and HTTP intake.
// The caller must bound body size before decoding.
func DecodeRevisionNotification(body []byte) (instances.RevisionAvailable, error) {
	var event instances.RevisionAvailable
	// Reject duplicate keys, including casing aliases accepted by encoding/json.
	keys := json.NewDecoder(bytes.NewReader(body))
	if token, err := keys.Token(); err != nil || token != json.Delim('{') {
		return event, ErrInvalidNotification
	}
	seen := map[string]bool{}
	for keys.More() {
		token, err := keys.Token()
		if err != nil {
			return event, ErrInvalidNotification
		}
		key, ok := token.(string)
		if !ok || seen[strings.ToLower(key)] {
			return event, ErrInvalidNotification
		}
		seen[strings.ToLower(key)] = true
		if err := keys.Decode(new(json.RawMessage)); err != nil {
			return event, ErrInvalidNotification
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return event, ErrInvalidNotification
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return event, ErrInvalidNotification
	}
	if event.Validate() != nil {
		return event, ErrInvalidNotification
	}
	return event, nil
}
