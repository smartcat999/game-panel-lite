package controlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
)

// GetRunEntitlement reads current global policy without caching. The returned
// record is not an execution lease and must never extend a Node authorization.
func (c *Client) GetRunEntitlement(ctx context.Context, event instances.RevisionAvailable, intent int64) (entitlements.Record, error) {
	var snapshot entitlements.Record
	if event.Validate() != nil || event.RegionID != c.region || intent < 1 {
		return snapshot, entitlements.ErrInvalid
	}
	body, err := json.Marshal(event)
	if err != nil {
		return snapshot, entitlements.ErrInvalid
	}
	endpoint, err := url.Parse(c.endpoint)
	if err != nil {
		return snapshot, ErrRequestFailed
	}
	endpoint.Path = "/internal/region/entitlements/resolve"
	endpoint.RawQuery = url.Values{"intentVersion": {strconv.FormatInt(intent, 10)}}.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return snapshot, ErrRequestFailed
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		// Do not expose URLs, transport details or server error bodies to task logs.
		if ctx.Err() != nil {
			return snapshot, errors.Join(ErrRequestFailed, ctx.Err())
		}
		return snapshot, ErrRequestFailed
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNotFound:
		return snapshot, entitlements.ErrUnavailable
	case http.StatusUnauthorized, http.StatusForbidden:
		return snapshot, ErrAccessDenied
	case http.StatusOK:
	default:
		return snapshot, ErrRequestFailed
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return snapshot, ErrInvalidResponse
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, c.maxBytes+1))
	if err != nil {
		return snapshot, ErrRequestFailed
	}
	if int64(len(encoded)) > c.maxBytes {
		return snapshot, ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&snapshot) != nil || decoder.Decode(new(any)) != io.EOF || snapshot.Policy.Validate() != nil || snapshot.Version < 1 || snapshot.SourceID == "" || snapshot.SourceKind == "" || snapshot.Status != "active" || snapshot.ServerID != event.ServerID || snapshot.OrganizationID != event.OrganizationID {
		return entitlements.Record{}, ErrInvalidResponse
	}
	return snapshot, nil
}
