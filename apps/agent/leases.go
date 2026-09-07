package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/internal/worker"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type agentLeaseClient struct {
	client          http.Client
	endpoint, token string
	assignment      workload.Assignment
	holder          string
}

func newAgentLeaseClient(client *http.Client, cfg AgentConfig, a workload.Assignment) (*agentLeaseClient, error) {
	base, err := url.Parse(cfg.MasterURL)
	if err != nil || base == nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("invalid execution lease control-plane configuration")
	}
	if a.UID == "" || len(a.UID) > 128 || a.ServerID == "" || a.NodeID == "" || a.Generation <= 0 {
		return nil, fmt.Errorf("invalid execution lease assignment")
	}
	for _, ch := range a.UID {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return nil, fmt.Errorf("invalid execution lease assignment UID")
		}
	}
	copied := *client
	copied.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	copied.Jar = nil
	copied.Timeout = 10 * time.Second
	return &agentLeaseClient{client: copied, endpoint: strings.TrimRight(base.String(), "/") + "/api/agent/assignments/" + url.PathEscape(a.UID) + "/lease", token: cfg.Token, assignment: a, holder: uuid.NewString()}, nil
}

func (c *agentLeaseClient) exchange(ctx context.Context, action string, fence int64) (workload.LeaseGrant, error) {
	payload, err := json.Marshal(workload.LeaseRequest{Action: action, Generation: c.assignment.Generation, HolderID: c.holder, Fence: fence})
	if err != nil {
		return workload.LeaseGrant{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return workload.LeaseGrant{}, err
	}
	req.Header.Set("X-Node-Token", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return workload.LeaseGrant{}, err
	}
	defer resp.Body.Close()
	if action == "release" && resp.StatusCode == http.StatusNoContent {
		return workload.LeaseGrant{}, nil
	}
	if resp.StatusCode != http.StatusOK || action == "release" {
		return workload.LeaseGrant{}, fmt.Errorf("execution lease %s rejected: status %d", action, resp.StatusCode)
	}
	var grant workload.LeaseGrant
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil {
		return grant, err
	}
	if len(body) > 4096 {
		return grant, fmt.Errorf("execution lease response too large")
	}
	if err := json.Unmarshal(body, &grant); err != nil {
		return grant, err
	}
	a := c.assignment
	if grant.AssignmentUID != a.UID || grant.NodeID != a.NodeID || grant.ServerID != a.ServerID || grant.Generation != a.Generation || grant.HolderID != c.holder || grant.Fence <= 0 || (fence > 0 && fence != grant.Fence) || grant.ValidForMS < 1000 || grant.ValidForMS > 300000 {
		return grant, fmt.Errorf("execution lease grant identity or duration mismatch")
	}
	return grant, nil
}

type leaseCredentialsContextKey struct{}

type leaseCredentials struct {
	holderID string
	fence    int64
}

func withLeaseCredentials(ctx context.Context, holderID string, fence int64) context.Context {
	return context.WithValue(ctx, leaseCredentialsContextKey{}, leaseCredentials{holderID: holderID, fence: fence})
}

func leaseCredentialsFromContext(ctx context.Context) (leaseCredentials, bool) {
	creds, ok := ctx.Value(leaseCredentialsContextKey{}).(leaseCredentials)
	return creds, ok
}

func reconcileLeasedAssignment(ctx context.Context, client *http.Client, cfg AgentConfig, logger *slog.Logger, a workload.Assignment, runtime agentRuntime) (workload.Observation, error) {
	lease, err := newAgentLeaseClient(client, cfg, a)
	if err != nil {
		return workload.Observation{}, err
	}
	started := time.Now()
	grant, err := lease.exchange(ctx, "acquire", 0)
	if err != nil {
		return workload.Observation{}, err
	}
	// Start the local window before the HTTP request, never from receipt of a
	// possibly delayed grant. Do not extend this deadline when renewing authority.
	deadline := started.Add(time.Duration(grant.ValidForMS)*time.Millisecond - time.Second)
	if limit := started.Add(90 * time.Second); limit.Before(deadline) {
		deadline = limit
	}
	executionCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if err := executionCtx.Err(); err != nil {
		return workload.Observation{}, err
	}
	executionCtx = withLeaseCredentials(executionCtx, lease.holder, grant.Fence)
	guarded := worker.AuthorizeMutations(runtime, func(ctx context.Context) error { _, err := lease.exchange(ctx, "renew", grant.Fence); return err })
	observation := worker.Reconcile(executionCtx, a, guarded)
	observation.LeaseHolderID = lease.holder
	observation.LeaseFence = grant.Fence
	// Persist while the lease is still held. An uncertain response retains the
	// grant; a subsequent poll will fetch the latest observation token.
	if err := reportWorkloadObservation(executionCtx, &lease.client, cfg, a, observation); err != nil {
		return observation, fmt.Errorf("report leased observation: %w", err)
	}
	// Runtime errors may describe an in-flight or ambiguous Docker operation.
	// Keep the grant until expiration in that case, including cancellation.
	if observation.LastError == "" && executionCtx.Err() == nil {
		if _, err := lease.exchange(executionCtx, "release", grant.Fence); err != nil {
			logger.Warn("failed to release execution lease", "server_id", a.ServerID, "error", err)
		}
	}
	return observation, nil
}
