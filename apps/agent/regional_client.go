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
var errRegionalAuthority = errors.New("regional execution authority unavailable")

const regionalResponseLimit int64 = 4 << 20

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
	if resp.StatusCode == http.StatusConflict {
		return errRegionalSession
	}
	if resp.StatusCode != 204 {
		return errRegionalRequest
	}
	return nil
}

type regionalClaim struct {
	Granted *workload.AuthorizedAssignment
	Started time.Time
}

func (c *regionalClient) Claim(ctx context.Context, sessionEpoch int64, holderID string) (regionalClaim, error) {
	started := time.Now()
	body, err := json.Marshal(workload.RegionalAssignmentRequest{SessionEpoch: sessionEpoch, HolderID: holderID})
	if err != nil {
		return regionalClaim{}, errRegionalRequest
	}
	resp, err := c.request(ctx, "/internal/node/assignments/claim", body)
	if err != nil {
		return regionalClaim{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return regionalClaim{Started: started}, nil
	}
	if resp.StatusCode == http.StatusConflict {
		return regionalClaim{}, errRegionalAuthority
	}
	if resp.StatusCode != http.StatusOK {
		return regionalClaim{}, errRegionalRequest
	}
	var granted workload.AuthorizedAssignment
	if err := readRegionalJSON(resp, regionalResponseLimit, &granted); err != nil {
		return regionalClaim{}, err
	}
	a, lease := granted.Assignment, granted.Lease
	if a.UID == "" || a.ID != a.UID || a.ServerID == "" || a.NodeID == "" || a.Generation < 1 || a.DesiredState != "running" || a.Spec.ServerID != a.ServerID || lease.ObservationToken == nil ||
		lease.AssignmentUID != a.UID || lease.ServerID != a.ServerID || lease.NodeID != a.NodeID || lease.Generation != a.Generation || lease.HolderID != holderID || lease.Fence < 1 || lease.ValidForMS < 1000 || lease.ValidForMS > (5*time.Minute).Milliseconds() || a.ObservationToken != *lease.ObservationToken {
		return regionalClaim{}, errRegionalResponse
	}
	if _, err := workload.ResolvePortBindings(a.Spec.Network); err != nil {
		return regionalClaim{}, errRegionalResponse
	}
	return regionalClaim{Granted: &granted, Started: started}, nil
}

func (c *regionalClient) Renew(ctx context.Context, sessionEpoch int64, assignment workload.Assignment, lease workload.LeaseGrant) (workload.LeaseGrant, error) {
	request := workload.RegionalLeaseRequest{SessionEpoch: sessionEpoch, Action: "renew", HolderID: lease.HolderID, Fence: lease.Fence}
	body, err := json.Marshal(request)
	if err != nil {
		return workload.LeaseGrant{}, errRegionalRequest
	}
	resp, err := c.request(ctx, "/internal/node/assignments/"+url.PathEscape(assignment.UID)+"/lease", body)
	if err != nil {
		return workload.LeaseGrant{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return workload.LeaseGrant{}, errRegionalAuthority
	}
	if resp.StatusCode != http.StatusOK {
		return workload.LeaseGrant{}, errRegionalRequest
	}
	var renewed workload.LeaseGrant
	if err := readRegionalJSON(resp, 4096, &renewed); err != nil {
		return workload.LeaseGrant{}, err
	}
	if renewed.AssignmentUID != assignment.UID || renewed.ServerID != assignment.ServerID || renewed.NodeID != assignment.NodeID || renewed.Generation != assignment.Generation || renewed.HolderID != lease.HolderID || renewed.Fence != lease.Fence || renewed.ValidForMS < 1000 || renewed.ValidForMS > (5*time.Minute).Milliseconds() {
		return workload.LeaseGrant{}, errRegionalResponse
	}
	return renewed, nil
}

func (c *regionalClient) Release(ctx context.Context, sessionEpoch int64, assignment workload.Assignment, lease workload.LeaseGrant) error {
	return c.finishExecution(ctx, "/internal/node/assignments/"+url.PathEscape(assignment.UID)+"/lease", workload.RegionalLeaseRequest{SessionEpoch: sessionEpoch, Action: "release", HolderID: lease.HolderID, Fence: lease.Fence})
}

func (c *regionalClient) Observe(ctx context.Context, sessionEpoch int64, assignment workload.Assignment, observation workload.Observation) error {
	return c.finishExecution(ctx, "/internal/node/assignments/"+url.PathEscape(assignment.UID)+"/observation", workload.RegionalObservationReport{SessionEpoch: sessionEpoch, Observation: observation})
}

func (c *regionalClient) finishExecution(ctx context.Context, path string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return errRegionalRequest
	}
	resp, err := c.request(ctx, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return errRegionalAuthority
	}
	if resp.StatusCode != http.StatusNoContent {
		return errRegionalRequest
	}
	return nil
}

func readRegionalJSON(resp *http.Response, limit int64, value any) error {
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return errRegionalResponse
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return errRegionalResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		return errRegionalResponse
	}
	return nil
}
