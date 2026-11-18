package player

import (
	"bufio"
	"context"
	"log/slog"
	"sync"
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
	runtime   runtime.WorkloadIOAdapter
	logger    *slog.Logger

	mu    sync.RWMutex
	cache map[string][]domain.Player
}

func NewSyncer(store *store.Store, providers *provider.Registry, runtime runtime.WorkloadIOAdapter, _ config.Config) *Syncer {
	return &Syncer{
		store:     store,
		providers: providers,
		runtime:   runtime,
		logger:    slog.Default(),
		cache:     make(map[string][]domain.Player),
	}
}

func (s *Syncer) WithLogger(logger *slog.Logger) *Syncer {
	if logger != nil {
		s.logger = logger
	}
	return s
}

func (s *Syncer) GetCachedPlayers(serverID string) ([]domain.Player, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	players, ok := s.cache[serverID]
	if !ok {
		return nil, false
	}
	out := make([]domain.Player, len(players))
	copy(out, players)
	return out, true
}

func (s *Syncer) SetCachedPlayers(serverID string, players []domain.Player) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if players == nil {
		players = []domain.Player{}
	}
	out := make([]domain.Player, len(players))
	copy(out, players)
	s.cache[serverID] = out
}

func (s *Syncer) ClearServer(serverID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, serverID)
}

func (s *Syncer) RemovePlayer(serverID string, identifier string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	players, ok := s.cache[serverID]
	if !ok {
		return
	}
	filtered := make([]domain.Player, 0, len(players))
	for _, p := range players {
		if p.UserID != identifier && p.Name != identifier {
			filtered = append(filtered, p)
		}
	}
	s.cache[serverID] = filtered
}

func (s *Syncer) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if err := s.RunOnce(ctx); err != nil {
		s.logger.Warn("initial sync online players failed", "error", err)
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

func (s *Syncer) SyncServer(ctx context.Context, server domain.GameServer) ([]domain.Player, error) {
	if server.Status.Phase != domain.PhaseRunning || server.Status.RuntimeID == "" {
		s.ClearServer(server.ID)
		if server.Status.PlayersOnline != 0 {
			server.Status.PlayersOnline = 0
			server.UpdatedAt = time.Now()
			_ = s.store.SaveGameServer(ctx, &server)
		}
		return []domain.Player{}, nil
	}

	gameProvider, ok := s.providers.Get(server.ProviderKey)
	if !ok {
		return []domain.Player{}, nil
	}
	playerProvider, hasPlayerList := gameProvider.(provider.PlayerListProvider)
	countProvider, hasPlayerCount := gameProvider.(provider.PlayerCountLogProvider)
	if !hasPlayerList && !hasPlayerCount {
		return []domain.Player{}, nil
	}

	if hasPlayerList {
		if cmd := playerProvider.PlayerListCommand(server); cmd != "" {
			_ = s.runtime.SendCommandWorkload(ctx, server.Status.RuntimeID, cmd)
			time.Sleep(100 * time.Millisecond)
		}
	}

	lines, err := s.recentLogLines(ctx, server.Status.RuntimeID)
	if err != nil {
		s.logger.Warn("failed to read player log output", "server", server.ID, "error", err)
		return nil, err
	}

	var parsedPlayers []domain.Player
	var nextCount *int

	if hasPlayerList {
		parsedPlayers = playerProvider.ParsePlayerListOutput(lines)
		if parsedPlayers == nil {
			parsedPlayers = []domain.Player{}
		}
		s.SetCachedPlayers(server.ID, parsedPlayers)
		count := len(parsedPlayers)
		nextCount = &count
	}
	if hasPlayerCount {
		if c := countProvider.ParsePlayerCount(lines); c != nil {
			nextCount = c
		}
	}

	if nextCount != nil && *nextCount != server.Status.PlayersOnline {
		server.Status.PlayersOnline = *nextCount
		server.UpdatedAt = time.Now()
		if err := s.store.SaveGameServer(ctx, &server); err != nil {
			s.logger.Warn("failed to update server player count", "server", server.ID, "error", err)
		}
	}

	if parsedPlayers == nil {
		parsedPlayers = []domain.Player{}
	}
	return parsedPlayers, nil
}

func (s *Syncer) RunOnce(ctx context.Context) error {
	servers, err := s.store.ListGameServers(ctx)
	if err != nil {
		return err
	}
	for _, server := range servers {
		if server.Status.Phase != domain.PhaseRunning {
			s.ClearServer(server.ID)
			if server.Status.PlayersOnline != 0 {
				server.Status.PlayersOnline = 0
				server.UpdatedAt = time.Now()
				if err := s.store.SaveGameServer(ctx, &server); err != nil {
					return err
				}
			}
			continue
		}
		if server.Status.RuntimeID == "" {
			continue
		}
		_, _ = s.SyncServer(ctx, server)
	}
	return nil
}

func (s *Syncer) recentLogLines(ctx context.Context, runtimeID string) ([]string, error) {
	stream, err := s.runtime.LogSnapshotWorkload(ctx, runtimeID)
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
