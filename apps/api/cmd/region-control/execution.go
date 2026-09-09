package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/configprotection"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/controlclient"
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

type executionOptions struct {
	endpoint, certificate, key, ca, configurationKeys, catalog string
	maxConfigurationBytes                                      int
	maxResponseBytes                                           int64
	leaseTTL                                                   time.Duration
	maxHeartbeatAge                                            time.Duration
}

func buildExecutionAuthorizer(region string, db *store.RegionalStore, timeout time.Duration, options executionOptions) (*regional.ExecutionAuthorizer, func(), error) {
	if db == nil || db.RegionID() != region || strings.TrimSpace(options.endpoint) == "" || options.maxConfigurationBytes < 1 || options.maxConfigurationBytes > 4<<20 || options.maxResponseBytes < 1 || options.leaseTTL < time.Second || options.leaseTTL > 5*time.Minute || options.maxHeartbeatAge < time.Second || options.maxHeartbeatAge > time.Hour {
		return nil, func() {}, errors.New("invalid regional execution configuration")
	}
	certificate, err := tls.LoadX509KeyPair(options.certificate, options.key)
	if err != nil {
		return nil, func() {}, errors.New("cannot load global client certificate")
	}
	pem, err := os.ReadFile(options.ca)
	if err != nil {
		return nil, func() {}, errors.New("cannot load global server CA")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, func() {}, errors.New("global server CA contains no certificates")
	}
	source, err := controlclient.New(controlclient.Options{Endpoint: options.endpoint, RegionID: region, Certificate: certificate, ServerCAs: pool, Timeout: timeout, MaxResponseBytes: options.maxResponseBytes})
	if err != nil {
		return nil, func() {}, err
	}
	keys, err := loadExecutionKeys(options.configurationKeys, options.maxConfigurationBytes)
	if err != nil {
		source.Close()
		return nil, func() {}, err
	}
	catalog, err := runtimecatalog.Load(options.catalog)
	if err != nil {
		source.Close()
		return nil, func() {}, errors.New("cannot load provider catalog")
	}
	registry, err := provider.NewRegistry(terraria.NewVanillaProvider(catalog), terraria.NewTModLoaderProvider(catalog), palworld.NewProvider(catalog), dst.NewProvider(catalog), minecraft.NewProvider(catalog))
	if err != nil {
		source.Close()
		return nil, func() {}, errors.New("cannot initialize providers")
	}
	renderer := gameconfig.RegionalRenderer{Normalizer: gameconfig.LogicalNormalizer{Providers: registry, MaxBytes: options.maxConfigurationBytes}, Configurations: keys}
	authorizer := &regional.ExecutionAuthorizer{Tasks: db, Source: source, Policy: executionPolicy{source: source}, Renderer: renderer, LeaseTTL: options.leaseTTL, MaxHeartbeatAge: options.maxHeartbeatAge}
	return authorizer, source.Close, nil
}

type executionPolicy struct {
	source interface {
		GetRunEntitlement(context.Context, instances.RevisionAvailable, int64) (entitlements.Record, error)
	}
}

func (p executionPolicy) AuthorizeExecution(ctx context.Context, snapshot regional.RevisionSnapshot, allocation regional.Allocation) error {
	if p.source == nil {
		return entitlements.ErrUnavailable
	}
	right, err := p.source.GetRunEntitlement(ctx, snapshot.Event, snapshot.IntentVersion)
	if err != nil {
		return err
	}
	return entitlements.ValidateRunGrant(right, snapshot.Event.OrganizationID, snapshot.Event.ServerID, allocation.CPU, allocation.MemoryMB)
}

func loadExecutionKeys(path string, maxBytes int) (*configprotection.Protector, error) {
	rejected := errors.New("cannot load configuration keyring")
	file, err := os.Open(path)
	if err != nil {
		return nil, rejected
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, rejected
	}
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	defer clear(raw)
	if err != nil || len(raw) > 1<<20 {
		return nil, rejected
	}
	var config struct {
		Active string            `json:"active"`
		Keys   map[string][]byte `json:"keys"`
	}
	defer func() {
		for _, key := range config.Keys {
			clear(key)
		}
	}()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&config) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, rejected
	}
	protector, err := configprotection.New(config.Active, config.Keys, maxBytes)
	if err != nil {
		return nil, rejected
	}
	return protector, nil
}
