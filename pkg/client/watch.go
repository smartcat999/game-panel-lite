package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// Watch opens a long-lived HTTP stream to the resource's list endpoint with ?watch=true
// and invokes handler for each incoming newline-delimited JSON WatchEvent until ctx is cancelled or EOF.
func (c *Client) Watch(ctx context.Context, resource, tenant string, opts ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	info, err := ResolveResource(resource)
	if err != nil {
		return err
	}
	if tenant == "" && opts.Tenant != "" {
		tenant = opts.Tenant
	}
	if info.TenantScoped && tenant == "" && info.Kind != "Instance" {
		tenant = c.cfg.Tenant
	}
	q := buildListQueryParams(opts)
	q["watch"] = "true"

	resp, err := c.streamResty.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		SetQueryParams(q).
		Get(CollectionPath(info, tenant))
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	rawBody := resp.RawBody()
	defer rawBody.Close()
	if resp.StatusCode() >= 400 {
		return FromResponse(resp.RawResponse)
	}
	stop := context.AfterFunc(ctx, func() { _ = rawBody.Close() })
	defer stop()

	dec := json.NewDecoder(rawBody)
	for {
		var ev metav1.WatchEvent[map[string]any]
		if err := dec.Decode(&ev); err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		if handler != nil {
			if err := handler(ev); err != nil {
				return err
			}
		}
	}
}
