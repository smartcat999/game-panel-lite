// Package backupingress validates broker requests before regional persistence.
package backupingress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

type Inbox interface {
	RecordBackupRequest(context.Context, backup.Requested) error
}
type Ingress struct {
	RegionID string
	MaxBytes int
	Inbox    Inbox
}

func (h Ingress) Handle(ctx context.Context, message regional.Notification) error {
	if h.RegionID == "" || h.MaxBytes < 1 || h.Inbox == nil {
		return errors.New("invalid backup ingress configuration")
	}
	media, _, err := mime.ParseMediaType(message.ContentType)
	if err != nil || media != "application/json" || len(message.Body) == 0 || len(message.Body) > h.MaxBytes {
		return regional.ErrInvalidNotification
	}
	event, err := DecodeRequest(message.Body)
	if err != nil || event.EventID != message.ID || event.RegionID != h.RegionID {
		return regional.ErrInvalidNotification
	}
	return h.Inbox.RecordBackupRequest(ctx, event)
}

// DecodeRequest rejects duplicate names (including encoding/json casing aliases).
// The caller bounds message size before decoding.
func DecodeRequest(body []byte) (backup.Requested, error) {
	var event backup.Requested
	keys := json.NewDecoder(bytes.NewReader(body))
	if token, err := keys.Token(); err != nil || token != json.Delim('{') {
		return event, regional.ErrInvalidNotification
	}
	seen := map[string]bool{}
	for keys.More() {
		token, err := keys.Token()
		if err != nil {
			return event, regional.ErrInvalidNotification
		}
		key, ok := token.(string)
		if !ok || seen[strings.ToLower(key)] {
			return event, regional.ErrInvalidNotification
		}
		seen[strings.ToLower(key)] = true
		if keys.Decode(new(json.RawMessage)) != nil {
			return event, regional.ErrInvalidNotification
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&event) != nil || decoder.Decode(new(any)) != io.EOF || event.Validate() != nil {
		return backup.Requested{}, regional.ErrInvalidNotification
	}
	return event, nil
}
