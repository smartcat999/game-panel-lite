package store

import (
	"context"
	"encoding/json"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

// ReviseEncryptedGlobalServer appends an authenticated immutable configuration
// after authorization, idempotency and generation checks. Callers must validate
// and canonicalize provider configuration and authorize referenced resources.
func (s *Store) ReviseEncryptedGlobalServer(ctx context.Context, actor string, request instances.ReviseRequest, plaintext []byte, sealer ConfigurationSealer, fingerprinter RequestFingerprinter) (instances.IntentResult, error) {
	if actor == "" {
		return instances.IntentResult{}, ErrWorkspaceWriteDenied
	}
	if request.ValidateMetadata() != nil || request.Specification.Configuration.KeyID != "" || len(request.Specification.Configuration.Ciphertext) != 0 || len(plaintext) == 0 || sealer == nil || fingerprinter == nil {
		return instances.IntentResult{}, instances.ErrInvalidIntent
	}
	encoded, err := encodeProtectedRevise(request, plaintext)
	if err != nil {
		return instances.IntentResult{}, err
	}
	defer clear(encoded)
	hash, err := fingerprinter.Sum(encoded)
	if err != nil {
		return instances.IntentResult{}, err
	}
	return s.reviseGlobalServer(ctx, actor, request, hash, func(existing string) (bool, error) { return fingerprinter.Matches(existing, encoded) }, func(binding instances.ConfigurationBinding) (instances.ProtectedConfiguration, error) {
		return sealer.Seal(ctx, binding, plaintext)
	})
}

func encodeProtectedRevise(request instances.ReviseRequest, plaintext []byte) ([]byte, error) {
	return json.Marshal(struct {
		Kind          string
		Request       instances.ReviseRequest
		Configuration []byte
	}{"revise", request, plaintext})
}
