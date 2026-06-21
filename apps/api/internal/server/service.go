package server

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

var ErrMissingServerName = errors.New("server name is required")

type Store interface {
	CreateGameServer(context.Context, *domain.GameServer) error
	SaveGameServer(context.Context, *domain.GameServer) error
	GetGameServer(context.Context, string) (domain.GameServer, error)
}

type Clock func() time.Time

type Service struct {
	store Store
	now   Clock
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

type CreateCommand struct {
	Name        string
	GameKey     domain.GameKey
	ProviderKey domain.ProviderKey
	Spec        domain.ServerSpec
}

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (domain.GameServer, error) {
	if strings.TrimSpace(cmd.Name) == "" {
		return domain.GameServer{}, ErrMissingServerName
	}
	now := s.clock()
	server := domain.GameServer{
		ID:          uuid.NewString(),
		Name:        strings.TrimSpace(cmd.Name),
		GameKey:     cmd.GameKey,
		ProviderKey: cmd.ProviderKey,
		Spec:        initialSpec(cmd.Spec),
		Status:      initialStatus(now),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.CreateGameServer(ctx, &server); err != nil {
		return domain.GameServer{}, err
	}
	return server, nil
}

func (s *Service) RequestStart(ctx context.Context, id string) (domain.GameServer, error) {
	return s.updateIntent(ctx, id, domain.DesiredRunning, markPending)
}

func (s *Service) RequestStop(ctx context.Context, id string) (domain.GameServer, error) {
	return s.updateIntent(ctx, id, domain.DesiredStopped, markPending)
}

func (s *Service) RequestRestart(ctx context.Context, id string) (domain.GameServer, error) {
	return s.updateIntent(ctx, id, domain.DesiredRunning, markPending)
}

func (s *Service) RequestDelete(ctx context.Context, id string) (domain.GameServer, error) {
	return s.updateIntent(ctx, id, domain.DesiredDeleted, markDeleting)
}

func (s *Service) updateIntent(
	ctx context.Context,
	id string,
	desired domain.ServerDesiredState,
	updateStatus func(*domain.ServerRuntimeStatus, time.Time),
) (domain.GameServer, error) {
	server, err := s.store.GetGameServer(ctx, id)
	if err != nil {
		return domain.GameServer{}, err
	}
	now := s.clock()
	server.Spec.DesiredState = desired
	bumpSpecGeneration(&server.Spec)
	updateStatus(&server.Status, now)
	server.UpdatedAt = now
	if err := s.store.SaveGameServer(ctx, &server); err != nil {
		return domain.GameServer{}, err
	}
	return server, nil
}

func (s *Service) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}
