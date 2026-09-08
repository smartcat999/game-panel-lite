package store

import (
	"context"
	"encoding/json"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

// These ports perform local operations using already loaded keys. Network key
// retrieval must happen before the admission transaction.
type ConfigurationSealer interface {
	Seal(context.Context, instances.ConfigurationBinding, []byte) (instances.ProtectedConfiguration, error)
}
type RequestFingerprinter interface {
	Sum([]byte) (string, error)
	Matches(string, []byte) (bool, error)
}

// CreateEncryptedGlobalServer is an internal persistence boundary, not a public
// tenant API. Callers must validate provider config, Region and asset access.
// Configuration bytes must already be canonicalized by the trusted use case.
func (s *Store) CreateEncryptedGlobalServer(ctx context.Context, actor string, request instances.CreateRequest, plaintext []byte, sealer ConfigurationSealer, fingerprinter RequestFingerprinter) (instances.IntentResult, error) {
	if actor == "" {
		return instances.IntentResult{}, ErrWorkspaceWriteDenied
	}
	if request.ValidateMetadata() != nil || request.Specification.Configuration.KeyID != "" || len(request.Specification.Configuration.Ciphertext) != 0 || len(plaintext) == 0 || sealer == nil || fingerprinter == nil {
		return instances.IntentResult{}, instances.ErrInvalidIntent
	}
	encoded, err := encodeProtectedCreate(request, plaintext)
	if err != nil {
		return instances.IntentResult{}, err
	}
	defer clear(encoded)
	hash, err := fingerprinter.Sum(encoded)
	if err != nil {
		return instances.IntentResult{}, err
	}
	return s.createGlobalServer(ctx, actor, request, hash, func(existing string) (bool, error) { return fingerprinter.Matches(existing, encoded) }, func(server instances.Server) (instances.ProtectedConfiguration, error) {
		return sealer.Seal(ctx, instances.ConfigurationBinding{OrganizationID: server.OrganizationID, ServerID: server.ID, RevisionID: server.CurrentRevisionID, SpecGeneration: server.SpecGeneration, ProviderKey: request.Specification.ProviderKey, ConfigSchemaVersion: request.Specification.ConfigSchemaVersion}, plaintext)
	}, true)
}

func encodeProtectedCreate(request instances.CreateRequest, plaintext []byte) ([]byte, error) {
	return json.Marshal(struct {
		Request       instances.CreateRequest
		Configuration []byte
	}{request, plaintext})
}
