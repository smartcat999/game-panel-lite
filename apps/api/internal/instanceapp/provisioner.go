package instanceapp

import (
	"context"
	"errors"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

var ErrInvalidCreateCommand = errors.New("invalid instance create command")

type OfferCatalog interface {
	ResolveOffer(context.Context, string, int64) (Offer, error)
}

type ProviderCatalog interface {
	ResolveProvider(string, string) (ProviderSpec, bool)
}

type Offer struct {
	ProviderKey string
	RegionID    string
	CPU         float64
	MemoryMB    int64
}

type ProviderSpec struct {
	GameVersion         string
	ConfigSchemaVersion int
}

type CreateCommand struct {
	OrganizationID string
	Name           string
	PlanID         string
	PlanVersion    int64
	GameVersion    string
	IdempotencyKey string
	Configuration  []byte
}

type Provisioner struct {
	intents   *Service
	offers    OfferCatalog
	providers ProviderCatalog
}

func NewProvisioner(intents *Service, offers OfferCatalog, providers ProviderCatalog) (*Provisioner, error) {
	if intents == nil || offers == nil || providers == nil {
		return nil, errors.New("instance provisioner dependencies are required")
	}
	return &Provisioner{intents: intents, offers: offers, providers: providers}, nil
}

// Create derives deployable metadata from a server-owned immutable plan. The
// tenant supplies configuration and may choose a supported game version, but
// cannot override Provider, Region, resources, schema version or Node.
func (p *Provisioner) Create(ctx context.Context, actor string, command CreateCommand) (instances.IntentResult, error) {
	if !createIdentifier(command.OrganizationID) || !createIdentifier(command.Name) || !createIdentifier(command.PlanID) ||
		!createIdentifier(command.IdempotencyKey) || command.PlanVersion < 1 || len(command.Configuration) == 0 {
		return instances.IntentResult{}, ErrInvalidCreateCommand
	}
	offer, err := p.offers.ResolveOffer(ctx, command.PlanID, command.PlanVersion)
	if err != nil {
		return instances.IntentResult{}, err
	}
	providerSpec, ok := p.providers.ResolveProvider(offer.ProviderKey, command.GameVersion)
	if !ok || providerSpec.GameVersion == "" || providerSpec.ConfigSchemaVersion < 1 {
		return instances.IntentResult{}, ErrInvalidCreateCommand
	}
	request := instances.CreateRequest{
		OrganizationID: command.OrganizationID,
		Name:           command.Name,
		RegionID:       offer.RegionID,
		IdempotencyKey: command.IdempotencyKey,
		Specification: instances.Specification{
			ProviderKey:         offer.ProviderKey,
			GameVersion:         providerSpec.GameVersion,
			ConfigSchemaVersion: providerSpec.ConfigSchemaVersion,
			Resources:           instances.Resources{CPU: offer.CPU, MemoryMB: offer.MemoryMB},
		},
	}
	return p.intents.Create(ctx, actor, request, command.Configuration)
}

func createIdentifier(value string) bool {
	return value != "" && len(value) <= 128 && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\x00\r\n")
}
