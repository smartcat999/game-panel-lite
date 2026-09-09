package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"math"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/internal/worker"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type regionalInfoRuntime interface {
	Info(context.Context) (workload.RuntimeInfo, error)
}

func runRegionalAgentFromEnvironment(ctx context.Context, runtime agentRuntime, logger *slog.Logger) error {
	certificate, err := tls.LoadX509KeyPair(os.Getenv("AGENT_CLIENT_CERT"), os.Getenv("AGENT_CLIENT_KEY"))
	if err != nil {
		return errors.New("cannot load regional node certificate")
	}
	pem, err := os.ReadFile(os.Getenv("AGENT_REGION_CA"))
	if err != nil {
		return errors.New("cannot load regional server CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return errors.New("regional server CA contains no certificates")
	}
	interval := 10 * time.Second
	poll := time.Second
	timeout := 5 * time.Second
	for _, option := range []struct {
		name  string
		value *time.Duration
	}{{"AGENT_HEARTBEAT_INTERVAL", &interval}, {"AGENT_EXECUTION_POLL_INTERVAL", &poll}, {"AGENT_REGION_REQUEST_TIMEOUT", &timeout}} {
		if raw := os.Getenv(option.name); raw != "" {
			duration, err := time.ParseDuration(raw)
			if err != nil || duration <= 0 {
				return errors.New("invalid regional heartbeat duration")
			}
			*option.value = duration
		}
	}
	client, err := newRegionalClient(os.Getenv("AGENT_REGION_URL"), certificate, roots, timeout)
	if err != nil {
		return err
	}
	defer client.Close()
	return runRegionalNode(ctx, client, runtime, logger, interval, poll, timeout)
}

// Regional mode currently reports runtime observations only. It does not start
// legacy reconciliation, tunnels or console tasks against the global endpoint.
func runRegionalHeartbeats(ctx context.Context, client *regionalClient, runtime regionalInfoRuntime, logger *slog.Logger, interval, probeTimeout time.Duration) error {
	if client == nil || runtime == nil || logger == nil || interval <= 0 || probeTimeout <= 0 {
		return errors.New("invalid regional heartbeat loop")
	}
	session, err := startRegionalSession(ctx, client, logger, interval)
	if err != nil || ctx.Err() != nil {
		return err
	}
	return runRegionalHeartbeatsWithSession(ctx, client, runtime, logger, session, interval, probeTimeout)
}

func startRegionalSession(ctx context.Context, client *regionalClient, logger *slog.Logger, retry time.Duration) (workload.NodeSession, error) {
	for ctx.Err() == nil {
		session, err := client.Start(ctx)
		if err == nil {
			return session, nil
		}
		if errors.Is(err, errRegionalIdentity) || errors.Is(err, errRegionalResponse) || errors.Is(err, errRegionalSession) {
			return workload.NodeSession{}, err
		}
		logger.Warn("regional node session unavailable")
		if !retryDelay(ctx, retry) {
			return workload.NodeSession{}, nil
		}
	}
	return workload.NodeSession{}, nil
}

func runRegionalHeartbeatsWithSession(ctx context.Context, client *regionalClient, runtime regionalInfoRuntime, logger *slog.Logger, session workload.NodeSession, interval, probeTimeout time.Duration) error {
	var sequence int64
	var architecture string
	for ctx.Err() == nil {
		probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
		info, err := runtime.Info(probeCtx)
		cancel()
		ready := err == nil && workload.NormalizeArchitecture(info.Architecture) != ""
		if ready {
			architecture = workload.NormalizeArchitecture(info.Architecture)
		}
		// Before a successful runtime observation there is no verified architecture;
		// keep the new session unready instead of substituting the Agent host's CPU.
		if architecture != "" {
			if sequence == math.MaxInt64 {
				return errRegionalSession
			}
			sequence++
			err = client.Heartbeat(ctx, workload.NodeHeartbeat{SessionEpoch: session.Epoch, Sequence: sequence, Architecture: architecture, RuntimeReady: ready})
			if errors.Is(err, errRegionalIdentity) || errors.Is(err, errRegionalSession) {
				return err
			}
			if err != nil && ctx.Err() == nil {
				logger.Warn("regional heartbeat unavailable")
			}
		}
		if !retryDelay(ctx, interval) {
			break
		}
	}
	return nil
}

func runRegionalNode(ctx context.Context, client *regionalClient, runtime agentRuntime, logger *slog.Logger, heartbeatInterval, pollInterval, requestTimeout time.Duration) error {
	if client == nil || runtime == nil || logger == nil || heartbeatInterval <= 0 || pollInterval <= 0 || requestTimeout <= 0 {
		return errors.New("invalid regional node loop")
	}
	session, err := startRegionalSession(ctx, client, logger, heartbeatInterval)
	if err != nil || ctx.Err() != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan error, 2)
	var loops sync.WaitGroup
	loops.Add(2)
	go func() {
		defer loops.Done()
		results <- runRegionalHeartbeatsWithSession(runCtx, client, runtime, logger, session, heartbeatInterval, requestTimeout)
	}()
	go func() {
		defer loops.Done()
		results <- runRegionalExecutions(runCtx, client, runtime, logger, session, pollInterval, requestTimeout)
	}()
	first := <-results
	cancel()
	second := <-results
	loops.Wait()
	if first != nil {
		return first
	}
	return second
}

func runRegionalExecutions(ctx context.Context, client *regionalClient, runtime worker.Runtime, logger *slog.Logger, session workload.NodeSession, interval, requestTimeout time.Duration) error {
	if client == nil || runtime == nil || logger == nil || session.Epoch < 1 || interval <= 0 || requestTimeout <= 0 {
		return errors.New("invalid regional execution loop")
	}
	for ctx.Err() == nil {
		holderID := uuid.NewString()
		requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		claim, err := client.Claim(requestCtx, session.Epoch, holderID)
		cancel()
		if errors.Is(err, errRegionalIdentity) || errors.Is(err, errRegionalSession) || errors.Is(err, errRegionalResponse) {
			return err
		}
		if err == nil && claim.Granted != nil {
			if err := reconcileRegionalAssignment(ctx, client, session, claim, runtime, logger); err != nil && ctx.Err() == nil {
				logger.Warn("regional workload reconciliation failed", "server_id", claim.Granted.Assignment.ServerID)
			}
			continue
		}
		if err != nil && ctx.Err() == nil && !errors.Is(err, errRegionalAuthority) {
			logger.Warn("regional execution poll unavailable")
		}
		if !retryDelay(ctx, interval) {
			return nil
		}
	}
	return nil
}

func reconcileRegionalAssignment(ctx context.Context, client *regionalClient, session workload.NodeSession, claim regionalClaim, runtime worker.Runtime, logger *slog.Logger) (err error) {
	if claim.Granted == nil {
		return errors.New("regional assignment missing")
	}
	assignment, lease := claim.Granted.Assignment, claim.Granted.Lease
	deadline := claim.Started.Add(time.Duration(lease.ValidForMS)*time.Millisecond - time.Second)
	if limit := claim.Started.Add(90 * time.Second); limit.Before(deadline) {
		deadline = limit
	}
	executionCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if err := executionCtx.Err(); err != nil {
		return err
	}
	guarded := worker.AuthorizeMutations(runtime, func(renewCtx context.Context) error {
		_, err := client.Renew(renewCtx, session.Epoch, assignment, lease)
		return err
	})
	observation := worker.Reconcile(executionCtx, assignment, guarded)
	observation.LeaseHolderID = lease.HolderID
	observation.LeaseFence = lease.Fence
	if err := client.Observe(executionCtx, session.Epoch, assignment, observation); err != nil {
		return err
	}
	if observation.LastError == "" && executionCtx.Err() == nil {
		if err := client.Release(executionCtx, session.Epoch, assignment, lease); err != nil {
			logger.Warn("regional execution lease release failed", "server_id", assignment.ServerID)
		}
	}
	return nil
}
