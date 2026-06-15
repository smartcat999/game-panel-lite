package player

import (
	"bufio"
	"context"
	"log/slog"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type Syncer struct {
	store     *store.Store
	providers *provider.Registry
	runtime   runtime.Adapter
	logger    *slog.Logger
}

func NewSyncer(store *store.Store, providers *provider.Registry, runtime runtime.Adapter, _ config.Config) *Syncer {
	return &Syncer{
		store:     store,
		providers: providers,
		runtime:   runtime,
		logger:    slog.Default(),
	}
}

func (s *Syncer) WithLogger(logger *slog.Logger) *Syncer {
	if logger != nil {
		s.logger = logger
	}
	return s
}

func (s *Syncer) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.RunOnce(ctx); err != nil {
				s.logger.Warn("failed to sync online players", "error", err)
			}
		}
	}
}

func (s *Syncer) RunOnce(ctx context.Context) error {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return err
	}
	for _, server := range servers {
		if server.Status != domain.StatusRunning {
			if server.PlayersOnline != 0 {
				server.PlayersOnline = 0
				server.UpdatedAt = time.Now()
				if err := s.store.SaveServer(ctx, &server); err != nil {
					return err
				}
			}
			continue
		}
		if server.ContainerID == "" {
			continue
		}
		gameProvider, ok := s.providers.Get(server.ProviderKey)
		if !ok {
			continue
		}
		playerProvider, ok := gameProvider.(provider.PlayerListProvider)
		if !ok {
			continue
		}
		lines, err := s.recentLogLines(ctx, server)
		if err != nil {
			s.logger.Warn("failed to read player log output", "server", server.ID, "error", err)
			continue
		}
		players := playerProvider.ParsePlayerListOutput(lines)
		if players == nil {
			continue
		}
		nextCount := len(players)
		if nextCount != server.PlayersOnline {
			server.PlayersOnline = nextCount
			server.UpdatedAt = time.Now()
			if err := s.store.SaveServer(ctx, &server); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Syncer) recentLogLines(ctx context.Context, server domain.GameServerInstance) ([]string, error) {
	snapshotServer := server
	snapshotServer.Status = domain.StatusStopped
	stream, err := s.runtime.Logs(ctx, snapshotServer)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	lines := make([]string, 0, 120)
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > 120 {
			lines = lines[len(lines)-120:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}
