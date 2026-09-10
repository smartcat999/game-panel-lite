package instanceconfiguration

import (
	"context"
	"errors"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

type ProviderRegistry interface {
	Verified(context.Context, string) (providercontract.Manifest, error)
	ValidateRevision(providercontract.Manifest, map[string]any, map[string]any, []providercontract.ModSelection, bool) (providercontract.ValidationResult, error)
}

type DraftStore interface {
	Create(context.Context, Draft) error
	Save(context.Context, Draft) error
	ByID(context.Context, string, string, string) (Draft, error)
}

type Service struct {
	providers ProviderRegistry
	delivery  Delivery
	drafts    DraftStore
}

func New(providers ProviderRegistry, delivery Delivery, drafts DraftStore) *Service {
	return &Service{providers: providers, delivery: delivery, drafts: drafts}
}

func (s *Service) CreateDraft(ctx context.Context, workspaceID, instanceID string, now time.Time) (Draft, error) {
	instance, err := s.delivery.Instance(ctx, workspaceID, instanceID)
	if err != nil {
		return Draft{}, err
	}
	revision, err := s.delivery.Revision(ctx, workspaceID, instanceID, instance.InstanceRevisionID)
	if err != nil {
		return Draft{}, err
	}
	id, err := newDraftID()
	if err != nil {
		return Draft{}, err
	}
	selections := make([]providercontract.ModSelection, 0, len(revision.ModLock))
	for _, item := range revision.ModLock {
		if item.Direct {
			selections = append(selections, providercontract.ModSelection{ModID: item.ModID, Version: item.Version})
		}
	}
	draft := Draft{ID: id, WorkspaceID: workspaceID, LogicalInstanceID: instanceID, BaseRevisionID: revision.ID, SchemaVersion: revision.SchemaVersion, Values: cloneValues(revision.Configuration), ModSelections: selections, ValidationErrors: []string{}, UpdatedAt: now.UTC()}
	if err := s.drafts.Create(ctx, draft); err != nil {
		return Draft{}, err
	}
	return draft, nil
}

func (s *Service) SaveDraft(ctx context.Context, command SaveCommand, now time.Time) (Draft, error) {
	if command.WorkspaceID == "" || command.LogicalInstanceID == "" || command.DraftID == "" || command.SchemaVersion < 1 || now.IsZero() {
		return Draft{}, ErrInvalidDraft
	}
	draft, err := s.drafts.ByID(ctx, command.WorkspaceID, command.LogicalInstanceID, command.DraftID)
	if err != nil {
		return Draft{}, err
	}
	instance, err := s.delivery.Instance(ctx, command.WorkspaceID, command.LogicalInstanceID)
	if err != nil {
		return Draft{}, err
	}
	base, err := s.delivery.Revision(ctx, command.WorkspaceID, command.LogicalInstanceID, draft.BaseRevisionID)
	if err != nil {
		return Draft{}, err
	}
	manifest, err := s.providers.Verified(ctx, instance.ProviderReleaseID)
	if err != nil {
		return Draft{}, err
	}
	draft.SchemaVersion = command.SchemaVersion
	draft.Values = cloneValues(command.Values)
	draft.ModSelections = append([]providercontract.ModSelection(nil), command.ModSelections...)
	draft.ValidationErrors = validateDraft(s.providers, manifest, base, draft)
	draft.UpdatedAt = now.UTC()
	if err := s.drafts.Save(ctx, draft); err != nil {
		return Draft{}, err
	}
	return draft, nil
}

func (s *Service) Apply(ctx context.Context, command ApplyCommand, now time.Time) (deliverycontrol.Revision, deliverycontrol.Operation, error) {
	if len(command.IdempotencyKey) < 8 || now.IsZero() {
		return deliverycontrol.Revision{}, deliverycontrol.Operation{}, ErrInvalidDraft
	}
	draft, err := s.drafts.ByID(ctx, command.WorkspaceID, command.LogicalInstanceID, command.DraftID)
	if err != nil {
		return deliverycontrol.Revision{}, deliverycontrol.Operation{}, err
	}
	instance, err := s.delivery.Instance(ctx, command.WorkspaceID, command.LogicalInstanceID)
	if err != nil {
		return deliverycontrol.Revision{}, deliverycontrol.Operation{}, err
	}
	if instance.InstanceRevisionID != draft.BaseRevisionID {
		return deliverycontrol.Revision{}, deliverycontrol.Operation{}, ErrStaleDraft
	}
	base, err := s.delivery.Revision(ctx, command.WorkspaceID, command.LogicalInstanceID, draft.BaseRevisionID)
	if err != nil {
		return deliverycontrol.Revision{}, deliverycontrol.Operation{}, err
	}
	manifest, err := s.providers.Verified(ctx, instance.ProviderReleaseID)
	if err != nil {
		return deliverycontrol.Revision{}, deliverycontrol.Operation{}, err
	}
	if validationErrors := validateDraft(s.providers, manifest, base, draft); len(validationErrors) > 0 {
		return deliverycontrol.Revision{}, deliverycontrol.Operation{}, ErrInvalidDraft
	}
	result, err := s.providers.ValidateRevision(manifest, base.Configuration, draft.Values, draft.ModSelections, false)
	if err != nil {
		return deliverycontrol.Revision{}, deliverycontrol.Operation{}, ErrInvalidDraft
	}
	return s.delivery.ApplyRevision(ctx, deliverycontrol.ApplyRevisionCommand{WorkspaceID: command.WorkspaceID, LogicalInstanceID: command.LogicalInstanceID, BaseRevisionID: base.ID, ProviderReleaseID: manifest.ProviderReleaseID, GameVersion: base.GameVersion, SchemaVersion: draft.SchemaVersion, Configuration: draft.Values, ModLock: result.ModLock, ApplyBehavior: string(result.ApplyBehavior), IdempotencyKey: command.IdempotencyKey}, now)
}

func validateDraft(providers ProviderRegistry, manifest providercontract.Manifest, base deliverycontrol.Revision, draft Draft) []string {
	if draft.SchemaVersion != manifest.SchemaVersion {
		mode, err := providercontract.MigrationMode(manifest, base.SchemaVersion, draft.SchemaVersion)
		if err != nil || mode != "automatic" {
			return []string{"schema_migration_required"}
		}
	}
	if _, err := providers.ValidateRevision(manifest, base.Configuration, draft.Values, draft.ModSelections, false); err != nil {
		switch {
		case errors.Is(err, providercontract.ErrUnresolvedMod):
			return []string{"mod_lock_unresolved"}
		default:
			return []string{"configuration_invalid"}
		}
	}
	return []string{}
}

func cloneValues(source map[string]any) map[string]any {
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
