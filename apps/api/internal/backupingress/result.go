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

// ResultInbox must atomically reconcile the original global task and persist
// deduplication before returning nil. Source identity comes from the trusted
// receiver configuration, never from a claim in the message body.
type ResultInbox interface {
	RecordBackupResult(context.Context, string, backup.ArchiveUploaded) error
}

type ResultIngress struct {
	SourceRegionID string
	MaxBytes       int
	Inbox          ResultInbox
}

func (h ResultIngress) Handle(ctx context.Context, message regional.Notification) error {
	if h.SourceRegionID == "" || h.MaxBytes < 1 || h.Inbox == nil {
		return errors.New("invalid backup result ingress configuration")
	}
	media, _, err := mime.ParseMediaType(message.ContentType)
	if err != nil || media != "application/json" || len(message.Body) == 0 || len(message.Body) > h.MaxBytes {
		return regional.ErrInvalidNotification
	}
	event, err := DecodeResult(message.Body)
	if err != nil || event.EventID != message.ID || event.Plan.RegionID != h.SourceRegionID {
		return regional.ErrInvalidNotification
	}
	return h.Inbox.RecordBackupResult(ctx, h.SourceRegionID, event)
}

// DecodeResult rejects ambiguous names in every nested object, including
// case aliases accepted by encoding/json. Callers must bound the input size.
func DecodeResult(body []byte) (backup.ArchiveUploaded, error) {
	var event backup.ArchiveUploaded
	keys := json.NewDecoder(bytes.NewReader(body))
	if uniqueResultObject(keys, 0) != nil || keys.Decode(new(any)) != io.EOF {
		return event, regional.ErrInvalidNotification
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&event) != nil || decoder.Decode(new(any)) != io.EOF || event.Validate() != nil {
		return backup.ArchiveUploaded{}, regional.ErrInvalidNotification
	}
	return event, nil
}

func uniqueResultObject(d *json.Decoder, depth int) error {
	if depth > 8 {
		return regional.ErrInvalidNotification
	}
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return regional.ErrInvalidNotification
	}
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return regional.ErrInvalidNotification
		}
		key, ok := token.(string)
		if !ok || seen[strings.ToLower(key)] {
			return regional.ErrInvalidNotification
		}
		seen[strings.ToLower(key)] = true
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return regional.ErrInvalidNotification
		}
		value = bytes.TrimSpace(value)
		if len(value) > 0 && value[0] == '{' {
			if uniqueResultObject(json.NewDecoder(bytes.NewReader(value)), depth+1) != nil {
				return regional.ErrInvalidNotification
			}
		} else if len(value) > 0 && value[0] == '[' {
			// The versioned result contract has no array-valued fields.
			return regional.ErrInvalidNotification
		}
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') {
		return regional.ErrInvalidNotification
	}
	return nil
}
