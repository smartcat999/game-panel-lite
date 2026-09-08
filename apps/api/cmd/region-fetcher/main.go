package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/controlclient"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type options struct {
	region, endpoint, certificate, key, ca, dsn     string
	lease, requestTimeout, taskTimeout, retry, poll time.Duration
	maxBytes                                        int64
}

func main() {
	var o options
	flag.StringVar(&o.region, "region", "", "region owning these tasks")
	flag.StringVar(&o.endpoint, "control-endpoint", "", "global control HTTPS origin")
	flag.StringVar(&o.certificate, "certificate", "", "regional client certificate PEM file")
	flag.StringVar(&o.key, "key", "", "regional client private key PEM file")
	flag.StringVar(&o.ca, "server-ca", "", "trusted global server CA PEM file")
	flag.DurationVar(&o.lease, "lease", time.Minute, "database task claim lifetime")
	flag.DurationVar(&o.requestTimeout, "request-timeout", 10*time.Second, "maximum revision HTTP request time")
	flag.DurationVar(&o.taskTimeout, "task-timeout", 20*time.Second, "maximum claim, request and save time")
	flag.DurationVar(&o.retry, "retry-delay", 5*time.Second, "persisted delay after unsuccessful fetch")
	flag.DurationVar(&o.poll, "poll-interval", time.Second, "delay after no work or failure")
	flag.Int64Var(&o.maxBytes, "max-response-bytes", 4<<20, "maximum revision snapshot bytes")
	flag.Parse()
	o.dsn = os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, o); err != nil {
		slog.Error("regional fetcher stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, o options) error {
	if strings.TrimSpace(o.region) == "" || o.region != strings.TrimSpace(o.region) || strings.TrimSpace(o.dsn) == "" || o.requestTimeout <= 0 || o.taskTimeout <= o.requestTimeout || o.lease <= o.taskTimeout || o.lease > time.Hour || o.retry < time.Millisecond || o.retry > 24*time.Hour || o.poll < time.Millisecond || o.poll > time.Hour {
		return errors.New("invalid regional fetcher settings")
	}
	certificate, err := tls.LoadX509KeyPair(o.certificate, o.key)
	if err != nil {
		return errors.New("cannot load regional client certificate")
	}
	ca, err := os.ReadFile(o.ca)
	if err != nil {
		return errors.New("cannot load control server CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return errors.New("control server CA contains no certificates")
	}
	source, err := controlclient.New(controlclient.Options{Endpoint: o.endpoint, RegionID: o.region, Certificate: certificate, ServerCAs: pool, Timeout: o.requestTimeout, MaxResponseBytes: o.maxBytes})
	if err != nil {
		return err
	}
	defer source.Close()
	db, err := store.OpenRegionalPostgres(o.dsn, o.region, 2)
	if err != nil {
		return err
	}
	defer db.Close()
	fetcher := regional.Fetcher{Tasks: db, Source: source, Lease: o.lease, Timeout: o.requestTimeout, RetryDelay: o.retry}
	for ctx.Err() == nil {
		workCtx, cancel := context.WithTimeout(ctx, o.taskTimeout)
		done, err := fetcher.RunOnce(workCtx)
		cancel()
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			// Driver and remote errors can contain configuration details. Task state
			// retains attempts and retry timing; logs expose only the failure category.
			slog.Warn("regional revision fetch failed", "region", o.region)
		}
		if done {
			continue
		}
		timer := time.NewTimer(o.poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}
