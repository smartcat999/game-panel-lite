package instanceprovisioning

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

var ErrInvalidRequest = errors.New("invalid provider-driven create request")

type ProviderRegistry interface {
	Verified(context.Context, string) (providercontract.Manifest, error)
	ValidateRevision(providercontract.Manifest, map[string]any, map[string]any, []providercontract.ModSelection, bool) (providercontract.ValidationResult, error)
}

type Delivery interface {
	Create(context.Context, deliverycontrol.CreateCommand, time.Time) (deliverycontrol.Instance, deliverycontrol.Operation, error)
}

type Command struct {
	WorkspaceID       string
	Name              string
	ProviderReleaseID string
	GameVersion       string
	Configuration     map[string]any
	ModSelections     []providercontract.ModSelection
	QuoteID           string
	IdempotencyKey    string
}

type Service struct {
	providers ProviderRegistry
	delivery  Delivery
}

func New(providers ProviderRegistry, delivery Delivery) *Service {
	return &Service{providers: providers, delivery: delivery}
}

func (s *Service) Create(ctx context.Context, command Command, now time.Time) (deliverycontrol.Instance, deliverycontrol.Operation, error) {
	manifest, err := s.providers.Verified(ctx, command.ProviderReleaseID)
	if err != nil {
		return deliverycontrol.Instance{}, deliverycontrol.Operation{}, err
	}
	if !slices.Contains(manifest.GameVersions, command.GameVersion) {
		return deliverycontrol.Instance{}, deliverycontrol.Operation{}, ErrInvalidRequest
	}
	validated, err := s.providers.ValidateRevision(manifest, nil, command.Configuration, command.ModSelections, true)
	if err != nil {
		return deliverycontrol.Instance{}, deliverycontrol.Operation{}, ErrInvalidRequest
	}
	return s.delivery.Create(ctx, deliverycontrol.CreateCommand{WorkspaceID: command.WorkspaceID, Name: command.Name, ProviderReleaseID: manifest.ProviderReleaseID, GameVersion: command.GameVersion, SchemaVersion: manifest.SchemaVersion, Configuration: command.Configuration, ModLock: validated.ModLock, ListenerRequirements: manifest.ListenerRequirements, QuoteID: command.QuoteID, IdempotencyKey: command.IdempotencyKey}, now)
}
