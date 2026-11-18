package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/minio/minio-go/v7/pkg/signer"

	"github.com/smartcat999/game-panel-cloud/pkg/config"
)

const (
	defaultS3PresignExpires = 15 * time.Minute
	maxS3PresignExpires     = 7 * 24 * time.Hour
)

var (
	ErrNotFound      = errors.New("s3 object not found")
	ErrNotConfigured = errors.New("s3 object storage is not configured")
)

// Config is an alias for config.S3Config managed centrally in pkg/config.
type Config = config.S3Config

func NewConfig() *Config {
	return config.NewS3Config()
}

// Binding captures the normalized object storage target coordinates.
type Binding struct {
	Endpoint       string `json:"endpoint"`
	PublicEndpoint string `json:"publicEndpoint,omitempty"`
	Region         string `json:"region"`
	Bucket         string `json:"bucket"`
	UsePathStyle   bool   `json:"usePathStyle"`
}

// ObjectStat describes metadata returned by an S3 object HEAD/GET operation.
type ObjectStat struct {
	Key         string            `json:"key"`
	SizeBytes   int64             `json:"sizeBytes"`
	ETag        string            `json:"etag,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	SHA256      string            `json:"sha256,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// PresignOptions customizes a presigned URL generation request.
type PresignOptions struct {
	PublicHostOverride         string
	Expires                    time.Duration
	ResponseContentDisposition string
	// Query adds extra signed query parameters (e.g. partNumber/uploadId for multipart UploadPart).
	Query url.Values
}

// PresignRequest configures AWS Signature Version 4 query-string URL presigning backed by minio-go/v7.
type PresignRequest struct {
	Endpoint                   string
	PublicEndpoint             string
	Region                     string
	Bucket                     string
	ObjectKey                  string
	Method                     string
	AccessKey                  string
	SecretKey                  string
	UsePathStyle               bool
	Expires                    time.Duration
	Now                        time.Time
	ResponseContentDisposition string
	Query                      url.Values
}

// Interface abstracts S3/MinIO object storage operations across the platform.
type Interface interface {
	IsConfigured() bool
	Binding() Binding
	PresignGet(ctx context.Context, key string, opts ...PresignOptions) (string, time.Time, error)
	PresignPut(ctx context.Context, key string, opts ...PresignOptions) (string, time.Time, error)
	PresignHead(ctx context.Context, key string, opts ...PresignOptions) (string, time.Time, error)
	PresignDelete(ctx context.Context, key string, opts ...PresignOptions) (string, time.Time, error)
	Stat(ctx context.Context, key string) (*ObjectStat, error)
	Get(ctx context.Context, key string) (io.ReadCloser, *ObjectStat, error)
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Copy(ctx context.Context, srcKey, dstKey string) error
	Delete(ctx context.Context, key string) error
	// List iterates objects under prefix (ListObjectsV2), invoking fn for each object.
	List(ctx context.Context, prefix string, fn func(ObjectInfo) error) error
	CreateMultipartUpload(ctx context.Context, key, contentType string) (string, error)
	PresignUploadPart(ctx context.Context, key, uploadID string, partNumber int, opts ...PresignOptions) (string, time.Time, error)
	CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error
	AbortMultipartUpload(ctx context.Context, key, uploadID string) error
	Ping(ctx context.Context) error
	Close() error
}

var (
	defaultMu     sync.RWMutex
	defaultClient Interface
)

// Open initializes the configured S3 client and sets it as the process-wide default if none is set.
func Open(_ context.Context, cfg *Config) (Interface, error) {
	if cfg == nil {
		cfg = &config.Get().S3
	}
	cfg.Default()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	c := NewClient(cfg, nil)
	defaultMu.Lock()
	if defaultClient == nil {
		defaultClient = c
	}
	defaultMu.Unlock()
	return c, nil
}

// Default returns the process-wide S3 client (or constructs one from config.Get().S3).
func Default() Interface {
	defaultMu.RLock()
	c := defaultClient
	defaultMu.RUnlock()
	if c != nil {
		return c
	}
	if cfg := config.Get(); cfg != nil {
		return NewClient(&cfg.S3, nil)
	}
	return NewClient(NewConfig(), nil)
}

// SetDefault overrides the process-wide S3 client.
func SetDefault(c Interface) {
	if c == nil {
		return
	}
	defaultMu.Lock()
	defaultClient = c
	defaultMu.Unlock()
}

type client struct {
	cfg         Config
	restyClient *resty.Client
}

// NewClient constructs an S3 client bound to cfg and httpClient.
func NewClient(cfg *Config, httpClient *http.Client) Interface {
	var val Config
	if cfg != nil {
		val = *cfg
	}
	val.Default()
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &client{cfg: val, restyClient: resty.NewWithClient(httpClient)}
}

func (c *client) IsConfigured() bool {
	return c.cfg.IsConfigured()
}

func (c *client) Binding() Binding {
	return Binding{
		Endpoint:       strings.TrimRight(strings.TrimSpace(c.cfg.Endpoint), "/"),
		PublicEndpoint: strings.TrimRight(strings.TrimSpace(c.cfg.PublicEndpoint), "/"),
		Region:         c.cfg.Region,
		Bucket:         c.cfg.Bucket,
		UsePathStyle:   c.cfg.UsePathStyle,
	}
}

func (c *client) PresignGet(_ context.Context, key string, opts ...PresignOptions) (string, time.Time, error) {
	return c.presign(http.MethodGet, key, firstPresignOpt(opts))
}

func (c *client) PresignPut(_ context.Context, key string, opts ...PresignOptions) (string, time.Time, error) {
	return c.presign(http.MethodPut, key, firstPresignOpt(opts))
}

func (c *client) PresignHead(_ context.Context, key string, opts ...PresignOptions) (string, time.Time, error) {
	return c.presign(http.MethodHead, key, firstPresignOpt(opts))
}

func (c *client) PresignDelete(_ context.Context, key string, opts ...PresignOptions) (string, time.Time, error) {
	return c.presign(http.MethodDelete, key, firstPresignOpt(opts))
}

func firstPresignOpt(opts []PresignOptions) PresignOptions {
	if len(opts) > 0 {
		return opts[0]
	}
	return PresignOptions{}
}

func (c *client) presign(method, key string, opt PresignOptions) (string, time.Time, error) {
	return c.presignWithPublic(method, key, c.resolvePublicEndpoint(opt.PublicHostOverride), opt)
}

func (c *client) presignInternal(method, key string) (string, time.Time, error) {
	return c.presignWithPublic(method, key, "", PresignOptions{})
}

func (c *client) presignWithPublic(method, key, publicEndpoint string, opt PresignOptions) (string, time.Time, error) {
	if !c.cfg.IsConfigured() {
		return "", time.Time{}, ErrNotConfigured
	}
	ttl := opt.Expires
	if ttl <= 0 {
		ttl = c.cfg.PresignTTL
	}
	if ttl <= 0 {
		ttl = defaultS3PresignExpires
	}
	now := time.Now().UTC()
	signedURL, err := PresignURL(PresignRequest{
		Endpoint:                   c.cfg.Endpoint,
		PublicEndpoint:             publicEndpoint,
		Region:                     c.cfg.Region,
		Bucket:                     c.cfg.Bucket,
		ObjectKey:                  key,
		Method:                     method,
		AccessKey:                  c.cfg.AccessKey,
		SecretKey:                  c.cfg.SecretKey,
		UsePathStyle:               c.cfg.UsePathStyle,
		Expires:                    ttl,
		Now:                        now,
		ResponseContentDisposition: opt.ResponseContentDisposition,
		Query:                      opt.Query,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return signedURL, now.Add(ttl), nil
}

func isInternalS3Host(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "", "localhost", "127.0.0.1", "::1", "minio", "seaweedfs", "apiserver":
		return true
	default:
		return false
	}
}

func (c *client) resolvePublicEndpoint(hostOverride string) string {
	targetHost := strings.TrimSpace(hostOverride)
	if spHost, _, err := net.SplitHostPort(targetHost); err == nil && spHost != "" {
		targetHost = spHost
	}
	targetHost = strings.Trim(targetHost, "[]")
	pub := strings.TrimSpace(c.cfg.PublicEndpoint)
	if !isInternalS3Host(targetHost) {
		baseRaw := pub
		if baseRaw == "" {
			if eu, err := url.Parse(strings.TrimSpace(c.cfg.Endpoint)); err == nil && isInternalS3Host(eu.Hostname()) {
				baseRaw = strings.TrimSpace(c.cfg.Endpoint)
			}
		}
		if baseRaw != "" {
			if u, err := url.Parse(baseRaw); err == nil && u.Host != "" {
				if port := u.Port(); port != "" {
					u.Host = net.JoinHostPort(targetHost, port)
				} else {
					u.Host = targetHost
				}
				return u.String()
			}
		}
	}
	return pub
}

func (c *client) Stat(ctx context.Context, key string) (*ObjectStat, error) {
	signedURL, _, err := c.presignInternal(http.MethodHead, key)
	if err != nil {
		return nil, err
	}
	resp, err := c.restyClient.R().SetContext(ctx).SetDoNotParseResponse(true).Head(signedURL)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("s3 stat %s: %w", key, err)
	}
	if resp.RawBody() != nil {
		_ = resp.RawBody().Close()
	}
	if resp.StatusCode() == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode() != http.StatusOK || resp.RawResponse.ContentLength < 0 {
		return nil, fmt.Errorf("s3 stat %s returned HTTP %d", key, resp.StatusCode())
	}
	return parseObjectStat(key, resp.RawResponse), nil
}

func (c *client) Get(ctx context.Context, key string) (io.ReadCloser, *ObjectStat, error) {
	signedURL, _, err := c.presignInternal(http.MethodGet, key)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.restyClient.R().SetContext(ctx).SetDoNotParseResponse(true).Get(signedURL)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, fmt.Errorf("s3 get %s: %w", key, err)
	}
	if resp.StatusCode() == http.StatusNotFound {
		_ = resp.RawBody().Close()
		return nil, nil, ErrNotFound
	}
	if resp.StatusCode() != http.StatusOK {
		_ = resp.RawBody().Close()
		return nil, nil, fmt.Errorf("s3 get %s returned HTTP %d", key, resp.StatusCode())
	}
	return resp.RawBody(), parseObjectStat(key, resp.RawResponse), nil
}

func (c *client) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	signedURL, _, err := c.presignInternal(http.MethodPut, key)
	if err != nil {
		return err
	}
	req := c.restyClient.R().SetContext(ctx).SetBody(body)
	if size >= 0 {
		req.SetContentLength(true)
	}
	if strings.TrimSpace(contentType) != "" {
		req.SetHeader("Content-Type", contentType)
	}
	resp, err := req.Put(signedURL)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	if resp.StatusCode() >= 300 {
		return fmt.Errorf("s3 put %s returned HTTP %d", key, resp.StatusCode())
	}
	return nil
}

func (c *client) Delete(ctx context.Context, key string) error {
	signedURL, _, err := c.presignInternal(http.MethodDelete, key)
	if err != nil {
		return err
	}
	resp, err := c.restyClient.R().SetContext(ctx).Delete(signedURL)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("s3 delete %s: object storage acknowledgement unavailable", key)
	}
	if resp.StatusCode() >= 300 && resp.StatusCode() != http.StatusNotFound {
		return fmt.Errorf("s3 delete %s returned HTTP %d", key, resp.StatusCode())
	}
	return nil
}

func (c *client) Ping(ctx context.Context) error {
	if !c.cfg.IsConfigured() {
		return nil
	}
	_, err := c.Stat(ctx, "__gp_healthcheck__")
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (c *client) Close() error {
	return nil
}

func parseObjectStat(key string, resp *http.Response) *ObjectStat {
	meta := make(map[string]string)
	for k, vals := range resp.Header {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "x-amz-meta-") && len(vals) > 0 {
			meta[strings.TrimPrefix(lower, "x-amz-meta-")] = vals[0]
		}
	}
	return &ObjectStat{
		Key:         key,
		SizeBytes:   resp.ContentLength,
		ETag:        strings.Trim(resp.Header.Get("ETag"), `"`),
		ContentType: resp.Header.Get("Content-Type"),
		SHA256:      resp.Header.Get("X-Amz-Meta-Sha256"),
		Metadata:    meta,
	}
}

// PresignURL generates an AWS Signature Version 4 query-string presigned URL for S3/MinIO GET, HEAD, PUT or DELETE
// requests using github.com/minio/minio-go/v7/pkg/signer.
func PresignURL(opts PresignRequest) (string, error) {
	u, bucket, objectKey, method, region, expires, err := normalizePresignRequest(opts)
	if err != nil {
		return "", err
	}
	host, rawPath := resolveS3HostAndPath(u, bucket, objectKey, opts.UsePathStyle)
	targetURL := &url.URL{Scheme: u.Scheme, Host: host, Path: rawPath}
	q := targetURL.Query()
	for k, vals := range opts.Query {
		for _, v := range vals {
			q.Add(k, v)
		}
	}
	if method == http.MethodGet && strings.TrimSpace(opts.ResponseContentDisposition) != "" {
		q.Set("response-content-disposition", strings.TrimSpace(opts.ResponseContentDisposition))
	}
	targetURL.RawQuery = q.Encode()
	req, err := http.NewRequest(method, targetURL.String(), nil)
	if err != nil {
		return "", err
	}
	req = signer.PreSignV4(*req, strings.TrimSpace(opts.AccessKey), strings.TrimSpace(opts.SecretKey), "", region, int64(expires/time.Second))
	return req.URL.String(), nil
}

func normalizePresignRequest(opts PresignRequest) (*url.URL, string, string, string, string, time.Duration, error) {
	rawEndpoint := strings.TrimSpace(opts.PublicEndpoint)
	if rawEndpoint == "" {
		rawEndpoint = strings.TrimSpace(opts.Endpoint)
	}
	if rawEndpoint == "" {
		return nil, "", "", "", "", 0, errors.New("s3 endpoint is required")
	}
	u, err := url.Parse(rawEndpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || strings.TrimSpace(u.Host) == "" {
		return nil, "", "", "", "", 0, fmt.Errorf("invalid s3 endpoint %q: must be an http or https URL", rawEndpoint)
	}
	bucket := strings.Trim(strings.TrimSpace(opts.Bucket), "/")
	if bucket == "" {
		return nil, "", "", "", "", 0, errors.New("s3 bucket is required")
	}
	objectKey := strings.TrimLeft(strings.TrimSpace(opts.ObjectKey), "/")
	if objectKey == "" {
		return nil, "", "", "", "", 0, errors.New("s3 objectKey is required")
	}
	if strings.TrimSpace(opts.AccessKey) == "" || strings.TrimSpace(opts.SecretKey) == "" {
		return nil, "", "", "", "", 0, errors.New("s3 accessKey and secretKey are required")
	}
	method := strings.ToUpper(strings.TrimSpace(opts.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodPut && method != http.MethodDelete {
		return nil, "", "", "", "", 0, fmt.Errorf("unsupported s3 presign method %q (expected GET, HEAD, PUT or DELETE)", opts.Method)
	}
	region := strings.TrimSpace(opts.Region)
	if region == "" {
		region = "us-east-1"
	}
	expires := opts.Expires
	if expires <= 0 {
		expires = defaultS3PresignExpires
	}
	expires = min(max(expires, time.Second), maxS3PresignExpires)
	return u, bucket, objectKey, method, region, expires, nil
}

func resolveS3HostAndPath(u *url.URL, bucket, objectKey string, forcePathStyle bool) (string, string) {
	hostname := strings.ToLower(u.Hostname())
	usePathStyle := forcePathStyle || hostname == "localhost" || strings.Contains(hostname, "minio") || net.ParseIP(hostname) != nil || !strings.Contains(hostname, ".")
	basePath := strings.Trim(u.Path, "/")
	if usePathStyle {
		if basePath != "" {
			return u.Host, "/" + basePath + "/" + bucket + "/" + objectKey
		}
		return u.Host, "/" + bucket + "/" + objectKey
	}
	if basePath != "" {
		return bucket + "." + u.Host, "/" + basePath + "/" + objectKey
	}
	return bucket + "." + u.Host, "/" + objectKey
}
