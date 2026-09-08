package controlclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/assets"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

// ResolveAsset returns point-in-time authorized metadata, not a download ticket.
func (c *Client) ResolveAsset(ctx context.Context, event instances.RevisionAvailable, ref instances.AssetVersion) (assets.PublishedVersion, error) {
	var result assets.PublishedVersion
	if event.Validate() != nil || event.RegionID != c.region || ref.AssetID == "" || ref.Version == "" {
		return result, regional.ErrInvalidNotification
	}
	endpoint, err := url.Parse(c.endpoint)
	if err != nil {
		return result, ErrRequestFailed
	}
	endpoint.Path = "/internal/region/assets/resolve"
	endpoint.RawQuery = url.Values{"assetId": {ref.AssetID}, "version": {ref.Version}}.Encode()
	body, err := json.Marshal(event)
	if err != nil {
		return result, regional.ErrInvalidNotification
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return result, ErrRequestFailed
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return result, errors.Join(ErrRequestFailed, ctx.Err())
		}
		return result, ErrRequestFailed
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNotFound:
		return result, regional.ErrRevisionUnavailable
	case http.StatusUnauthorized, http.StatusForbidden:
		return result, ErrAccessDenied
	case http.StatusOK:
	default:
		return result, ErrRequestFailed
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return result, ErrInvalidResponse
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, c.maxBytes+1))
	if err != nil {
		return result, ErrRequestFailed
	}
	if int64(len(encoded)) > c.maxBytes {
		return result, ErrInvalidResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.Validate() != nil || result.OrganizationID != event.OrganizationID || result.AssetID != ref.AssetID || result.Version != ref.Version {
		return assets.PublishedVersion{}, ErrInvalidResponse
	}
	return result, nil
}
