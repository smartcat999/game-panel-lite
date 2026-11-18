package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/minio/minio-go/v7/pkg/signer"
)

// ObjectInfo describes one object returned by ListObjectsV2.
type ObjectInfo struct {
	Key          string    `json:"key"`
	SizeBytes    int64     `json:"sizeBytes"`
	ETag         string    `json:"etag,omitempty"`
	LastModified time.Time `json:"lastModified"`
}

// CompletedPart is one uploaded multipart part acknowledged by its ETag.
type CompletedPart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

type listObjectsV2Result struct {
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
	Contents              []struct {
		Key          string    `xml:"Key"`
		LastModified time.Time `xml:"LastModified"`
		ETag         string    `xml:"ETag"`
		Size         int64     `xml:"Size"`
	} `xml:"Contents"`
}

type initiateMultipartResult struct {
	UploadID string `xml:"UploadId"`
}

type completeMultipartUpload struct {
	XMLName xml.Name        `xml:"CompleteMultipartUpload"`
	Parts   []CompletedPart `xml:"Part"`
}

type s3ErrorBody struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// signedDo executes a header-signed (SigV4) request against the internal endpoint.
func (c *client) signedDo(ctx context.Context, method, key string, query url.Values, body []byte) ([]byte, int, error) {
	return c.signedDoWithHeaders(ctx, method, key, query, nil, body)
}

func (c *client) signedDoWithHeaders(ctx context.Context, method, key string, query url.Values, headers map[string]string, body []byte) ([]byte, int, error) {
	if !c.cfg.IsConfigured() {
		return nil, 0, ErrNotConfigured
	}
	u, err := url.Parse(strings.TrimSpace(c.cfg.Endpoint))
	if err != nil || u.Host == "" {
		return nil, 0, fmt.Errorf("invalid s3 endpoint %q", c.cfg.Endpoint)
	}
	host, rawPath := resolveS3HostAndPath(u, strings.Trim(c.cfg.Bucket, "/"), strings.TrimLeft(key, "/"), c.cfg.UsePathStyle)
	target := &url.URL{Scheme: u.Scheme, Host: host, Path: rawPath, RawQuery: query.Encode()}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	sum := sha256.Sum256(body)
	req.Header.Set("X-Amz-Content-Sha256", hex.EncodeToString(sum[:]))
	req.ContentLength = int64(len(body))
	signed := signer.SignV4(*req, c.cfg.AccessKey, c.cfg.SecretKey, "", c.cfg.Region)
	resp, err := c.restyClient.GetClient().Do(signed)
	if err != nil {
		return nil, 0, fmt.Errorf("s3 %s %s: %w", method, key, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 300 || bytes.Contains(data[:min(len(data), 256)], []byte("<Error>")) {
		var e s3ErrorBody
		_ = xml.Unmarshal(data, &e)
		return data, resp.StatusCode, fmt.Errorf("s3 %s %s returned HTTP %d %s %s", method, key, resp.StatusCode, e.Code, e.Message)
	}
	return data, resp.StatusCode, nil
}

func (c *client) Copy(ctx context.Context, srcKey, dstKey string) error {
	cleanSrc := strings.TrimLeft(strings.TrimSpace(srcKey), "/")
	cleanDst := strings.TrimLeft(strings.TrimSpace(dstKey), "/")
	if cleanSrc == "" || cleanDst == "" {
		return fmt.Errorf("s3 copy requires non-empty source and destination keys")
	}
	copySource := "/" + strings.Trim(c.cfg.Bucket, "/") + "/" + escapeS3KeySegments(cleanSrc)
	headers := map[string]string{"X-Amz-Copy-Source": copySource}
	_, status, err := c.signedDoWithHeaders(ctx, http.MethodPut, cleanDst, nil, headers, nil)
	if err != nil {
		if status == http.StatusNotFound || strings.Contains(err.Error(), "NoSuchKey") {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func escapeS3KeySegments(key string) string {
	parts := strings.Split(key, "/")
	for i, seg := range parts {
		parts[i] = url.PathEscape(seg)
	}
	return strings.Join(parts, "/")
}

func (c *client) List(ctx context.Context, prefix string, fn func(ObjectInfo) error) error {
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "max-keys": {"1000"}}
		if prefix != "" {
			q.Set("prefix", prefix)
		}
		if token != "" {
			q.Set("continuation-token", token)
		}
		data, _, err := c.signedDo(ctx, http.MethodGet, "", q, nil)
		if err != nil {
			return err
		}
		var res listObjectsV2Result
		if err := xml.Unmarshal(data, &res); err != nil {
			return fmt.Errorf("decode ListObjectsV2: %w", err)
		}
		for _, obj := range res.Contents {
			info := ObjectInfo{Key: obj.Key, SizeBytes: obj.Size, ETag: strings.Trim(obj.ETag, `"`), LastModified: obj.LastModified}
			if err := fn(info); err != nil {
				return err
			}
		}
		if !res.IsTruncated || res.NextContinuationToken == "" {
			return nil
		}
		token = res.NextContinuationToken
	}
}

func (c *client) CreateMultipartUpload(ctx context.Context, key, contentType string) (string, error) {
	data, _, err := c.signedDo(ctx, http.MethodPost, key, url.Values{"uploads": {""}}, nil)
	if err != nil {
		return "", err
	}
	var res initiateMultipartResult
	if err := xml.Unmarshal(data, &res); err != nil || res.UploadID == "" {
		return "", fmt.Errorf("s3 create multipart upload %s: missing UploadId", key)
	}
	return res.UploadID, nil
}

func (c *client) PresignUploadPart(_ context.Context, key, uploadID string, partNumber int, opts ...PresignOptions) (string, time.Time, error) {
	opt := firstPresignOpt(opts)
	opt.Query = url.Values{"partNumber": {strconv.Itoa(partNumber)}, "uploadId": {uploadID}}
	return c.presign(http.MethodPut, key, opt)
}

func (c *client) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error {
	normalized := make([]CompletedPart, 0, len(parts))
	for _, p := range parts {
		etag := strings.Trim(strings.TrimSpace(p.ETag), `"`)
		normalized = append(normalized, CompletedPart{PartNumber: p.PartNumber, ETag: `"` + etag + `"`})
	}
	body, err := xml.Marshal(completeMultipartUpload{Parts: normalized})
	if err != nil {
		return err
	}
	_, _, err = c.signedDo(ctx, http.MethodPost, key, url.Values{"uploadId": {uploadID}}, body)
	return err
}

func (c *client) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	_, status, err := c.signedDo(ctx, http.MethodDelete, key, url.Values{"uploadId": {uploadID}}, nil)
	if status == http.StatusNotFound {
		return nil
	}
	return err
}
