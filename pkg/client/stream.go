package client

import (
	"context"
	"io"
	"strconv"
)

// StreamLogs opens a chunked HTTP stream to read logs from an instance.
// The caller is responsible for closing the returned io.ReadCloser.
func (c *Client) StreamLogs(ctx context.Context, tenant, name string, opts LogOptions) (io.ReadCloser, error) {
	info, err := ResolveKind("Instance")
	if err != nil {
		return nil, err
	}
	if tenant == "" {
		tenant = c.cfg.Tenant
	}
	q := make(map[string]string)
	if opts.Follow {
		q["follow"] = "true"
	}
	if opts.TailLines > 0 {
		q["tailLines"] = strconv.Itoa(opts.TailLines)
	}
	resp, err := c.streamResty.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		SetQueryParams(q).
		Get(SubresourcePath(info, tenant, name, "log"))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() >= 400 {
		defer resp.RawBody().Close()
		return nil, FromResponse(resp.RawResponse)
	}
	return resp.RawBody(), nil
}

// DownloadBlob streams a content-addressed Blob by digest, transparently following any 307 S3/CDN redirect.
func (c *Client) DownloadBlob(ctx context.Context, digest string) (io.ReadCloser, error) {
	info, err := ResolveKind("Blob")
	if err != nil {
		return nil, err
	}
	resp, err := c.streamResty.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		Get(ItemPath(info, "", digest))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() >= 400 {
		defer resp.RawBody().Close()
		return nil, FromResponse(resp.RawResponse)
	}
	return resp.RawBody(), nil
}
