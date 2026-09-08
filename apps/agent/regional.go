package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

type regionalInfoRuntime interface {
	Info(context.Context) (workload.RuntimeInfo, error)
}

func runRegionalAgentFromEnvironment(ctx context.Context, runtime regionalInfoRuntime, logger *slog.Logger) error {
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
	timeout := 5 * time.Second
	for _, option := range []struct {
		name  string
		value *time.Duration
	}{{"AGENT_HEARTBEAT_INTERVAL", &interval}, {"AGENT_REGION_REQUEST_TIMEOUT", &timeout}} {
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
	return runRegionalHeartbeats(ctx, client, runtime, logger, interval, timeout)
}

// Regional mode currently reports runtime observations only. It does not start
// legacy reconciliation, tunnels or console tasks against the global endpoint.
func runRegionalHeartbeats(ctx context.Context, client *regionalClient, runtime regionalInfoRuntime, logger *slog.Logger, interval, probeTimeout time.Duration) error {
	if client == nil || runtime == nil || logger == nil || interval <= 0 || probeTimeout <= 0 {
		return errors.New("invalid regional heartbeat loop")
	}
	var session workload.NodeSession
	for ctx.Err() == nil {
		var err error
		session, err = client.Start(ctx)
		if err == nil {
			break
		}
		if errors.Is(err, errRegionalIdentity) || errors.Is(err, errRegionalResponse) || errors.Is(err, errRegionalSession) {
			return err
		}
		logger.Warn("regional node session unavailable")
		if !retryDelay(ctx, interval) {
			return nil
		}
	}
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
