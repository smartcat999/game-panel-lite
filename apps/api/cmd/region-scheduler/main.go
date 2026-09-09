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
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/entitlements"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/gameconfig"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/dst"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/minecraft"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/palworld"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/runtimecatalog"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/regional"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type options struct {
	configurationKeys, catalog, architecture        string
	maxConfigurationBytes, firstPort, lastPort      int
	heartbeatAge                                    time.Duration
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
	flag.DurationVar(&o.requestTimeout, "request-timeout", 10*time.Second, "maximum global HTTP request and scheduling work time")
	flag.DurationVar(&o.taskTimeout, "task-timeout", 20*time.Second, "maximum claim, request and save time")
	flag.DurationVar(&o.retry, "retry-delay", 5*time.Second, "persisted delay after unsuccessful scheduling")
	flag.DurationVar(&o.poll, "poll-interval", time.Second, "delay after no work or failure")
	flag.Int64Var(&o.maxBytes, "max-response-bytes", 4<<20, "maximum revision snapshot bytes")
	flag.StringVar(&o.configurationKeys, "configuration-keys", "", "private JSON keyring for protected global revisions")
	flag.StringVar(&o.catalog, "provider-catalog", "", "provider catalog shared with global configuration validation")
	flag.StringVar(&o.architecture, "architecture", "", "canonical runtime architecture for this scheduling worker")
	flag.IntVar(&o.firstPort, "first-port", 0, "first allowed primary host port")
	flag.IntVar(&o.lastPort, "last-port", 0, "last allowed primary host port; at most 64 ports per window")
	flag.IntVar(&o.maxConfigurationBytes, "max-configuration-bytes", 65536, "maximum plaintext provider configuration bytes")
	flag.DurationVar(&o.heartbeatAge, "max-heartbeat-age", 30*time.Second, "maximum node observation age at admission")
	flag.Parse()
	o.dsn = os.Getenv("GAMEPANEL_REGIONAL_DATABASE_URL")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, o); err != nil {
		slog.Error("regional scheduler stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, o options) error {
	if o.architecture == "" || o.firstPort < 1 || o.lastPort < o.firstPort || o.lastPort > 65535 || o.lastPort-o.firstPort >= 64 || o.maxConfigurationBytes < 1 || o.maxConfigurationBytes > 4<<20 || o.heartbeatAge < time.Millisecond || o.heartbeatAge > time.Hour || strings.TrimSpace(o.region) == "" || o.region != strings.TrimSpace(o.region) || strings.TrimSpace(o.dsn) == "" || o.requestTimeout <= 0 || o.taskTimeout <= o.requestTimeout || o.lease <= o.taskTimeout || o.lease > time.Hour || o.retry < time.Millisecond || o.retry > 24*time.Hour || o.poll < time.Millisecond || o.poll > time.Hour {
		return errors.New("invalid regional scheduler settings")
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
		return errors.New("cannot open regional database")
	}
	defer db.Close()
	keys, err := loadConfigurationKeys(o.configurationKeys, o.maxConfigurationBytes)
	if err != nil {
		return err
	}
	if o.catalog != "" {
		if _, err := os.Stat(o.catalog); err != nil {
			return errors.New("cannot read configured provider catalog")
		}
	}
	catalog, err := runtimecatalog.Load(o.catalog)
	if err != nil {
		return errors.New("cannot load provider catalog")
	}
	registry, err := provider.NewRegistry(terraria.NewVanillaProvider(catalog), terraria.NewTModLoaderProvider(catalog), palworld.NewProvider(catalog), dst.NewProvider(catalog), minecraft.NewProvider(catalog))
	if err != nil {
		return errors.New("cannot initialize providers")
	}
	scheduler := regional.Scheduler{Resources: db, Networks: gameconfig.RegionalRenderer{Normalizer: gameconfig.LogicalNormalizer{Providers: registry, MaxBytes: o.maxConfigurationBytes}, Configurations: keys}, MaxHeartbeatAge: o.heartbeatAge}
	worker := regional.SchedulingWorker{Tasks: db, Source: source, Scheduler: scheduler, Scopes: schedulingScopes{db: db, entitlements: source, registry: registry, architecture: o.architecture, firstPort: o.firstPort, lastPort: o.lastPort}, Lease: o.lease, Timeout: o.requestTimeout, RetryDelay: o.retry}
	for ctx.Err() == nil {
		workCtx, cancel := context.WithTimeout(ctx, o.taskTimeout)
		done, err := worker.RunOnce(workCtx)
		cancel()
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			// Driver and remote errors can contain configuration details. Task state
			// retains attempts and retry timing; logs expose only the failure category.
			slog.Warn("regional scheduling failed", "region", o.region)
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

// This adapter first resolves current global commercial rights, then derives
// candidates from regional operator policy and Provider constraints.
type schedulingScopes struct {
	db                  nodeAccessReader
	entitlements        runEntitlementReader
	registry            *provider.Registry
	architecture        string
	firstPort, lastPort int
}

type nodeAccessReader interface {
	RegionalNodeAccessPolicy(context.Context, string) (regional.NodeAccessPolicy, error)
}

type runEntitlementReader interface {
	GetRunEntitlement(context.Context, instances.RevisionAvailable, int64) (entitlements.Record, error)
}

func (s schedulingScopes) SchedulingScope(ctx context.Context, snapshot regional.RevisionSnapshot) (regional.SchedulingScope, error) {
	if s.entitlements == nil {
		return regional.SchedulingScope{}, entitlements.ErrUnavailable
	}
	// Global verifies tenant, current placement/intent, policy lifetime and the
	// requested resources in one read snapshot. This admission is deliberately
	// repeated for every scheduling attempt and is never cached as execution authority.
	right, err := s.entitlements.GetRunEntitlement(ctx, snapshot.Event, snapshot.IntentVersion)
	if err != nil {
		return regional.SchedulingScope{}, err
	}
	needed := snapshot.Revision.Specification.Resources
	if err := entitlements.ValidateRunGrant(right, snapshot.Event.OrganizationID, snapshot.Event.ServerID, needed.CPU, needed.MemoryMB); err != nil {
		return regional.SchedulingScope{}, err
	}
	policy, err := s.db.RegionalNodeAccessPolicy(ctx, snapshot.Event.OrganizationID)
	if err != nil {
		return regional.SchedulingScope{}, err
	}
	requirements, err := s.registry.NodeRequirements(domain.ProviderKey(snapshot.Revision.Specification.ProviderKey))
	if err != nil {
		return regional.SchedulingScope{}, err
	}
	if len(requirements.Architectures) > 0 {
		supported := false
		for _, arch := range requirements.Architectures {
			if arch == s.architecture {
				supported = true
				break
			}
		}
		if !supported {
			return regional.SchedulingScope{}, regional.ErrNodeUnavailable
		}
	}
	return regional.SchedulingScope{AllowedNodeIDs: policy.NodeIDs, Architecture: s.architecture, HostPort: s.firstPort, MaxHostPort: s.lastPort}, nil
}
