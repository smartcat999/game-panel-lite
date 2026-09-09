package app

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/gameconfig"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instanceapp"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type instanceOfferCatalog struct{ store *store.Store }

func (c instanceOfferCatalog) ResolveOffer(ctx context.Context, planID string, version int64) (instanceapp.Offer, error) {
	quote, err := c.store.QuoteListedPrepaidPlan(ctx, planID, version, 1)
	if err != nil {
		return instanceapp.Offer{}, err
	}
	return instanceapp.Offer{ProviderKey: quote.Plan.ProviderKey, RegionID: quote.Plan.RegionID, CPU: quote.Plan.CPU, MemoryMB: quote.Plan.MemoryMB}, nil
}

type instanceProviderCatalog struct{ registry *provider.Registry }

func (c instanceProviderCatalog) ResolveProvider(key, requestedVersion string) (instanceapp.ProviderSpec, bool) {
	gameProvider, ok := c.registry.Get(domain.ProviderKey(key))
	if !ok {
		return instanceapp.ProviderSpec{}, false
	}
	versions := gameProvider.Versions()
	if requestedVersion == "" && len(versions) > 0 {
		requestedVersion = versions[0]
	}
	for _, version := range versions {
		if version == requestedVersion {
			return instanceapp.ProviderSpec{GameVersion: version, ConfigSchemaVersion: gameProvider.CatalogMetadata().ConfigVersion}, true
		}
	}
	return instanceapp.ProviderSpec{}, false
}

func buildInstanceProvisioner(cfg config.Config, db *store.Store, registry *provider.Registry) (*instanceapp.Provisioner, error) {
	if cfg.ConfigurationKeyringPath == "" && cfg.FingerprintKeyringPath == "" {
		return nil, nil
	}
	if cfg.ConfigurationKeyringPath == "" || cfg.FingerprintKeyringPath == "" {
		return nil, errors.New("both logical instance keyrings are required")
	}
	maxConfigurationBytes := cfg.LogicalConfigMaxBytes
	if maxConfigurationBytes <= 0 || maxConfigurationBytes > 4<<20 {
		maxConfigurationBytes = config.DefaultLogicalConfigMaxBytes
	}
	protector, err := loadConfigurationProtector(cfg.ConfigurationKeyringPath, maxConfigurationBytes)
	if err != nil {
		return nil, errors.New("cannot initialize logical configuration protection")
	}
	fingerprinter, err := loadRequestFingerprinter(cfg.FingerprintKeyringPath)
	if err != nil {
		return nil, errors.New("cannot initialize logical request fingerprinting")
	}
	writer, err := store.NewEncryptedIntentWriter(db, protector, fingerprinter)
	if err != nil {
		return nil, errors.New("cannot initialize logical instance writer")
	}
	admission, err := store.NewGlobalIntentAdmission(db)
	if err != nil {
		return nil, errors.New("cannot initialize logical instance admission")
	}
	intents, err := instanceapp.New(writer, gameconfig.LogicalNormalizer{Providers: registry, MaxBytes: maxConfigurationBytes}, admission)
	if err != nil {
		return nil, errors.New("cannot initialize logical instance service")
	}
	provisioner, err := instanceapp.NewProvisioner(intents, instanceOfferCatalog{db}, instanceProviderCatalog{registry})
	if err != nil {
		return nil, errors.New("cannot initialize logical instance provisioner")
	}
	return provisioner, nil
}
