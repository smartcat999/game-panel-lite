package controlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/backup"
)

// CheckBackup checks the original request against current global intent. Each
// call goes to the control plane; success is not cached or treated as a lease.
func (c *Client) CheckBackup(ctx context.Context, event backup.Requested) error {
	if event.Validate() != nil || event.RegionID != c.region {
		return backup.ErrInvalidRequest
	}
	endpoint, err := url.Parse(c.endpoint)
	if err != nil {
		return ErrRequestFailed
	}
	endpoint.Path = "/internal/region/backups/check"
	body, err := json.Marshal(event)
	if err != nil {
		return backup.ErrInvalidRequest
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return ErrRequestFailed
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return errors.Join(ErrRequestFailed, ctx.Err())
		}
		return ErrRequestFailed
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return backup.ErrRequestUnavailable
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrAccessDenied
	default:
		return ErrRequestFailed
	}
}
