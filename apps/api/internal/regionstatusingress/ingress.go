// Package regionstatusingress validates Region status messages before they
// reach the global projection store.
package regionstatusingress

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/deploymentstatus"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regionstatus"
)

type Projection interface {
	RecordRegionStatus(context.Context, string, regionstatus.Snapshot) error
}

type DeploymentProjection interface {
	RecordDeploymentStatus(context.Context, string, deploymentstatus.Event) error
}

type Ingress struct {
	SourceRegionID string
	MaxBytes       int
	Projection     Projection
	Deployments    DeploymentProjection
}

func (i Ingress) Handle(ctx context.Context, notification regional.Notification) error {
	if i.SourceRegionID == "" || i.MaxBytes < 1 || i.Projection == nil || i.Deployments == nil {
		return errors.New("invalid Region status ingress configuration")
	}
	mediaType, _, err := mime.ParseMediaType(notification.ContentType)
	if err != nil || mediaType != "application/json" || len(notification.Body) == 0 || len(notification.Body) > i.MaxBytes {
		return regional.ErrInvalidNotification
	}
	keys := json.NewDecoder(bytes.NewReader(notification.Body))
	if uniqueObject(keys, 0) != nil || keys.Decode(new(any)) != io.EOF {
		return regional.ErrInvalidNotification
	}
	if notification.Type == "deployment.status.observed" {
		return i.handleDeployment(ctx, notification)
	}
	if notification.Type != "" && notification.Type != "region.status.observed" {
		return regional.ErrInvalidNotification
	}
	decoder := json.NewDecoder(bytes.NewReader(notification.Body))
	decoder.DisallowUnknownFields()
	var snapshot regionstatus.Snapshot
	if err := decoder.Decode(&snapshot); err != nil || snapshot.Validate() != nil || snapshot.EventID != notification.ID || snapshot.RegionID != i.SourceRegionID {
		return regional.ErrInvalidNotification
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return regional.ErrInvalidNotification
	}
	err = i.Projection.RecordRegionStatus(ctx, i.SourceRegionID, snapshot)
	if errors.Is(err, regionstatus.ErrInvalidSnapshot) || errors.Is(err, regionstatus.ErrSnapshotConflict) {
		return regional.ErrNotificationConflict
	}
	return err
}

func (i Ingress) handleDeployment(ctx context.Context, notification regional.Notification) error {
	decoder := json.NewDecoder(bytes.NewReader(notification.Body))
	decoder.DisallowUnknownFields()
	var event deploymentstatus.Event
	if err := decoder.Decode(&event); err != nil || event.Validate() != nil || event.EventID != notification.ID || event.RegionID != i.SourceRegionID {
		return regional.ErrInvalidNotification
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return regional.ErrInvalidNotification
	}
	err := i.Deployments.RecordDeploymentStatus(ctx, i.SourceRegionID, event)
	if errors.Is(err, deploymentstatus.ErrInvalidEvent) || errors.Is(err, deploymentstatus.ErrEventConflict) {
		return regional.ErrNotificationConflict
	}
	return err
}

func uniqueObject(decoder *json.Decoder, depth int) error {
	if depth > 4 {
		return regional.ErrInvalidNotification
	}
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return regional.ErrInvalidNotification
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[strings.ToLower(key)] {
			return regional.ErrInvalidNotification
		}
		seen[strings.ToLower(key)] = true
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return regional.ErrInvalidNotification
		}
		raw = bytes.TrimSpace(raw)
		if len(raw) > 0 && raw[0] == '{' && uniqueObject(json.NewDecoder(bytes.NewReader(raw)), depth+1) != nil {
			return regional.ErrInvalidNotification
		}
		if len(raw) > 0 && raw[0] == '[' {
			return regional.ErrInvalidNotification
		}
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return regional.ErrInvalidNotification
	}
	return nil
}
