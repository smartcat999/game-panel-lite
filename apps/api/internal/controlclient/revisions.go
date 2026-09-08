// Package controlclient retrieves global revision data over authenticated HTTPS.
package controlclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
)

var (
	ErrRequestFailed   = errors.New("global revision request failed")
	ErrAccessDenied    = errors.New("global revision access denied")
	ErrInvalidResponse = errors.New("invalid global revision response")
)

type Options struct {
	Endpoint, RegionID string
	Certificate        tls.Certificate
	ServerCAs          *x509.CertPool
	Timeout            time.Duration
	MaxResponseBytes   int64
}

type Client struct {
	endpoint, region string
	maxBytes         int64
	http             *http.Client
	transport        *http.Transport
}

func New(options Options) (*Client, error) {
	u, err := url.Parse(options.Endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		options.RegionID == "" || strings.TrimSpace(options.RegionID) != options.RegionID ||
		len(options.Certificate.Certificate) == 0 || options.Certificate.PrivateKey == nil || options.ServerCAs == nil ||
		options.Timeout <= 0 || options.MaxResponseBytes < 1 || options.MaxResponseBytes == math.MaxInt64 {
		return nil, errors.New("invalid global revision client configuration")
	}
	u.Path = "/internal/region/revisions/resolve"
	transport := &http.Transport{
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{options.Certificate}, RootCAs: options.ServerCAs.Clone()},
		DialContext:         (&net.Dialer{Timeout: options.Timeout}).DialContext,
		TLSHandshakeTimeout: options.Timeout, ResponseHeaderTimeout: options.Timeout,
		IdleConnTimeout: time.Minute, MaxIdleConnsPerHost: 2,
	}
	client := &http.Client{Transport: transport, Timeout: options.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Client{endpoint: u.String(), region: options.RegionID, maxBytes: options.MaxResponseBytes, http: client, transport: transport}, nil
}

// Close releases idle connections. Active requests remain bounded by their context and timeout.
func (c *Client) Close() { c.transport.CloseIdleConnections() }

func (c *Client) GetRevision(ctx context.Context, event instances.RevisionAvailable) (regional.RevisionSnapshot, error) {
	var snapshot regional.RevisionSnapshot
	if event.Validate() != nil || event.RegionID != c.region {
		return snapshot, regional.ErrInvalidNotification
	}
	body, err := json.Marshal(event)
	if err != nil {
		return snapshot, regional.ErrInvalidNotification
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
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
		return snapshot, regional.ErrRevisionUnavailable
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
	if decoder.Decode(&snapshot) != nil || decoder.Decode(new(any)) != io.EOF || snapshot.ValidateFor(event) != nil {
		return regional.RevisionSnapshot{}, ErrInvalidResponse
	}
	return snapshot, nil
}
