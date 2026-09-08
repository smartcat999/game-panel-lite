// Package instanceapp coordinates user configuration before durable global intent.
package instanceapp

import (
	"context"
	"errors"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

type Writer interface {
	ReplayCreate(context.Context, string, instances.CreateRequest, []byte) (instances.IntentResult, bool, error)
	ReplayRevise(context.Context, string, instances.ReviseRequest, []byte) (instances.IntentResult, bool, error)
	Create(context.Context, string, instances.CreateRequest, []byte) (instances.IntentResult, error)
	Revise(context.Context, string, instances.ReviseRequest, []byte) (instances.IntentResult, error)
}
type Normalizer interface {
	// Normalize returns a new owned slice; the service clears it after writing.
	Normalize(context.Context, string, string, int, []byte) ([]byte, error)
}

// Admission checks tenant access, Region availability and versioned asset access.
// It must not receive or log raw secret configuration. Persistence must still
// recheck mutable authorization and quota in its admission transaction.
type Admission interface {
	CheckCreate(context.Context, string, instances.CreateRequest) error
	CheckRevise(context.Context, string, instances.ReviseRequest) error
}

type Service struct {
	writer     Writer
	normalizer Normalizer
	admission  Admission
}

func New(writer Writer, normalizer Normalizer, admission Admission) (*Service, error) {
	if writer == nil || normalizer == nil || admission == nil {
		return nil, errors.New("instance writer, normalizer and admission are required")
	}
	return &Service{writer: writer, normalizer: normalizer, admission: admission}, nil
}

func emptyProtected(spec instances.Specification) bool {
	return spec.Configuration.KeyID == "" && len(spec.Configuration.Ciphertext) == 0
}

func (s *Service) Create(ctx context.Context, actor string, request instances.CreateRequest, raw []byte) (instances.IntentResult, error) {
	if err := ctx.Err(); err != nil {
		return instances.IntentResult{}, err
	}
	if strings.TrimSpace(actor) == "" || request.ValidateMetadata() != nil || !emptyProtected(request.Specification) {
		return instances.IntentResult{}, instances.ErrInvalidIntent
	}
	config, err := s.normalizer.Normalize(ctx, request.Specification.ProviderKey, request.Specification.GameVersion, request.Specification.ConfigSchemaVersion, raw)
	if err != nil {
		return instances.IntentResult{}, err
	}
	defer clear(config)
	if result, found, err := s.writer.ReplayCreate(ctx, actor, request, config); err != nil || found {
		return result, err
	}
	if err := s.admission.CheckCreate(ctx, actor, request); err != nil {
		return instances.IntentResult{}, err
	}
	return s.writer.Create(ctx, actor, request, config)
}

func (s *Service) Revise(ctx context.Context, actor string, request instances.ReviseRequest, raw []byte) (instances.IntentResult, error) {
	if err := ctx.Err(); err != nil {
		return instances.IntentResult{}, err
	}
	if strings.TrimSpace(actor) == "" || request.ValidateMetadata() != nil || !emptyProtected(request.Specification) {
		return instances.IntentResult{}, instances.ErrInvalidIntent
	}
	config, err := s.normalizer.Normalize(ctx, request.Specification.ProviderKey, request.Specification.GameVersion, request.Specification.ConfigSchemaVersion, raw)
	if err != nil {
		return instances.IntentResult{}, err
	}
	defer clear(config)
	if result, found, err := s.writer.ReplayRevise(ctx, actor, request, config); err != nil || found {
		return result, err
	}
	if err := s.admission.CheckRevise(ctx, actor, request); err != nil {
		return instances.IntentResult{}, err
	}
	return s.writer.Revise(ctx, actor, request, config)
}
