package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

var errRegionalIdentity = errors.New("regional node identity or configuration rejected")
var errRegionalSession = errors.New("regional node session was replaced or heartbeat rejected")
var errRegionalResponse = errors.New("regional node response invalid")
var errRegionalRequest = errors.New("regional node request failed")

type regionalClient struct {
	endpoint  string
	client    *http.Client
	transport *http.Transport
}

func newRegionalClient(endpoint string, certificate tls.Certificate, roots *x509.CertPool, timeout time.Duration) (*regionalClient, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || len(certificate.Certificate) == 0 || certificate.PrivateKey == nil || roots == nil || timeout <= 0 {
		return nil, errors.New("regional HTTPS origin, client certificate, CA and positive timeout required")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, RootCAs: roots.Clone()}, TLSHandshakeTimeout: timeout, MaxConnsPerHost: 2, MaxIdleConnsPerHost: 2, IdleConnTimeout: time.Minute}
	return &regionalClient{endpoint: strings.TrimRight(endpoint, "/"), transport: transport, client: &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *regionalClient) Close() { c.transport.CloseIdleConnections() }

func (c *regionalClient) request(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return nil, errRegionalRequest
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Join(errRegionalRequest, ctx.Err())
	}
	switch resp.StatusCode {
	case 401, 403, 404:
		resp.Body.Close()
		return nil, errRegionalIdentity
	case 409:
		resp.Body.Close()
		return nil, errRegionalSession
	}
	return resp, nil
}
func (c *regionalClient) Start(ctx context.Context) (workload.NodeSession, error) {
	var session workload.NodeSession
	resp, err := c.request(ctx, "/internal/node/session", nil)
	if err != nil {
		return session, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return session, errRegionalRequest
	}
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return session, errRegionalResponse
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1025))
	if err != nil {
		return session, errors.Join(errRegionalRequest, ctx.Err())
	}
	if len(body) > 1024 {
		return session, errRegionalResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return session, errRegionalResponse
	}
	token, err = decoder.Token()
	if err != nil || token != "epoch" {
		return session, errRegionalResponse
	}
	if decoder.Decode(&session.Epoch) != nil || session.Epoch < 1 {
		return workload.NodeSession{}, errRegionalResponse
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return workload.NodeSession{}, errRegionalResponse
	}
	if _, err = decoder.Token(); err != io.EOF {
		return workload.NodeSession{}, errRegionalResponse
	}
	return session, nil
}
func (c *regionalClient) Heartbeat(ctx context.Context, heartbeat workload.NodeHeartbeat) error {
	body, err := json.Marshal(heartbeat)
	if err != nil {
		return errRegionalRequest
	}
	resp, err := c.request(ctx, "/internal/node/heartbeat", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 {
		return errRegionalIdentity
	}
	if resp.StatusCode != 204 {
		return errRegionalRequest
	}
	return nil
}
