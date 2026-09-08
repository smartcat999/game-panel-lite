package store

import (
	"context"
	"errors"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

// EncryptedIntentWriter binds trusted crypto dependencies once at composition,
// rather than accepting cryptographic implementations from an HTTP request.
type EncryptedIntentWriter struct {
	store         *Store
	sealer        ConfigurationSealer
	fingerprinter RequestFingerprinter
}

func NewEncryptedIntentWriter(store *Store, sealer ConfigurationSealer, fingerprinter RequestFingerprinter) (*EncryptedIntentWriter, error) {
	if store == nil || sealer == nil || fingerprinter == nil {
		return nil, errors.New("encrypted intent writer dependencies are required")
	}
	return &EncryptedIntentWriter{store: store, sealer: sealer, fingerprinter: fingerprinter}, nil
}
func (w *EncryptedIntentWriter) Create(ctx context.Context, actor string, request instances.CreateRequest, config []byte) (instances.IntentResult, error) {
	return w.store.CreateEncryptedGlobalServer(ctx, actor, request, config, w.sealer, w.fingerprinter)
}
func (w *EncryptedIntentWriter) Revise(ctx context.Context, actor string, request instances.ReviseRequest, config []byte) (instances.IntentResult, error) {
	return w.store.ReviseEncryptedGlobalServer(ctx, actor, request, config, w.sealer, w.fingerprinter)
}
