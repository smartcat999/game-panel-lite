package observability

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

const activityLimit = 100

type RuntimeStatsProvider interface {
	HostStats(ctx context.Context) (runtime.HostStats, error)
	StatsWorkload(ctx context.Context, runtimeID string) (runtime.WorkloadStats, error)
}

type Service struct {
	store   *store.Store
	runtime RuntimeStatsProvider
}

type Snapshot struct {
	CollectedAt time.Time         `json:"collectedAt"`
	Host        runtime.HostStats `json:"host"`
	Servers     []ServerMetric    `json:"servers"`
	Activity    ActivitySummary   `json:"activity"`
}

type ServerMetric struct {
	ID             string              `json:"id"`
	Name           string              `json:"name"`
	GameKey        domain.GameKey      `json:"gameKey"`
	ProviderKey    domain.ProviderKey  `json:"providerKey"`
	Status         domain.ServerStatus `json:"status"`
	PlayersOnline  int                 `json:"playersOnline"`
	MaxPlayers     int                 `json:"maxPlayers"`
	HostPort       int                 `json:"hostPort,omitempty"`
	Version        string              `json:"version,omitempty"`
	CPUPercent     float64             `json:"cpuPercent"`
	MemoryMB       int64               `json:"memoryMb"`
	MemoryLimitMB  int64               `json:"memoryLimitMb"`
	UptimeSeconds  float64             `json:"uptimeSeconds"`
	StatsAvailable bool                `json:"statsAvailable"`
}

type ActivitySummary struct {
	WindowHours int                 `json:"windowHours"`
	Total       int                 `json:"total"`
	Lifecycle   int                 `json:"lifecycle"`
	Backups     int                 `json:"backups"`
	Players     int                 `json:"players"`
	Failures    int                 `json:"failures"`
	ByType      []ActivityTypeCount `json:"byType"`
}

type ActivityTypeCount struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

func NewService(store *store.Store, runtime RuntimeStatsProvider) *Service {
	return &Service{store: store, runtime: runtime}
}

func (s *Service) Snapshot(ctx context.Context, runtimeAvailable bool) (Snapshot, error) {
	servers, err := s.store.ListGameServers(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	events, err := s.store.ListActivity(ctx, activityLimit)
	if err != nil {
		return Snapshot{}, err
	}

	snapshot := Snapshot{
		CollectedAt: time.Now(),
		Servers:     make([]ServerMetric, 0, len(servers)),
		Activity:    summarizeActivity(events),
	}
	if runtimeAvailable {
		if host, err := s.runtime.HostStats(ctx); err == nil {
			snapshot.Host = host
		}
	}
	for _, server := range servers {
		metric := ServerMetric{
			ID:            server.ID,
			Name:          server.Name,
			GameKey:       server.GameKey,
			ProviderKey:   server.ProviderKey,
			Status:        domain.ServerStatusFromRuntime(server.Spec.DesiredState, server.Status),
			PlayersOnline: server.Status.PlayersOnline,
			MaxPlayers:    domain.ServerMaxPlayers(server),
			HostPort:      server.Spec.Network.HostPort,
			Version:       server.Spec.Version,
		}
		if server.Status.Phase == domain.PhaseRunning && !server.Status.LastTransitionAt.IsZero() {
			metric.UptimeSeconds = maxSeconds(0, snapshot.CollectedAt.Sub(server.Status.LastTransitionAt).Seconds())
		}
		if runtimeAvailable && server.Status.Phase == domain.PhaseRunning && server.Status.RuntimeID != "" {
			if stats, err := s.runtime.StatsWorkload(ctx, server.Status.RuntimeID); err == nil {
				metric.CPUPercent = stats.CPUPercent
				metric.MemoryMB = stats.MemoryMB
				metric.MemoryLimitMB = stats.MemoryLimitMB
				metric.StatsAvailable = true
			}
		}
		snapshot.Servers = append(snapshot.Servers, metric)
	}
	sort.Slice(snapshot.Servers, func(i, j int) bool {
		if snapshot.Servers[i].Status == snapshot.Servers[j].Status {
			return strings.ToLower(snapshot.Servers[i].Name) < strings.ToLower(snapshot.Servers[j].Name)
		}
		return statusRank(snapshot.Servers[i].Status) < statusRank(snapshot.Servers[j].Status)
	})
	return snapshot, nil
}

func (s *Service) PrometheusText(ctx context.Context, runtimeAvailable bool) (string, error) {
	snapshot, err := s.Snapshot(ctx, runtimeAvailable)
	if err != nil {
		return "", err
	}
	return FormatPrometheus(snapshot), nil
}

func FormatPrometheus(snapshot Snapshot) string {
	var builder strings.Builder
	writeGauge := func(name string, help string, value any) {
		builder.WriteString("# HELP ")
		builder.WriteString(name)
		builder.WriteByte(' ')
		builder.WriteString(help)
		builder.WriteByte('\n')
		builder.WriteString("# TYPE ")
		builder.WriteString(name)
		builder.WriteString(" gauge\n")
		builder.WriteString(name)
		builder.WriteByte(' ')
		builder.WriteString(fmt.Sprint(value))
		builder.WriteByte('\n')
	}

	writeGauge("gamepanel_runtime_running_containers", "Number of running GamePanel-managed runtime containers.", snapshot.Host.RunningWorkloads)
	writeGauge("gamepanel_runtime_cpu_percent", "Total CPU percent used by GamePanel-managed runtime containers.", snapshot.Host.TotalCPUPercent)
	writeGauge("gamepanel_runtime_memory_bytes", "Total memory used by GamePanel-managed runtime containers in bytes.", megabytesToBytes(snapshot.Host.TotalMemoryMB))
	writeGauge("gamepanel_runtime_memory_limit_bytes", "Total memory limit for GamePanel-managed runtime containers in bytes.", megabytesToBytes(snapshot.Host.MemoryLimitMB))
	writeGauge("gamepanel_activity_events_24h_total", "Activity events recorded in the last 24 hours.", snapshot.Activity.Total)
	writeGauge("gamepanel_activity_failures_24h_total", "Failure activity events recorded in the last 24 hours.", snapshot.Activity.Failures)

	builder.WriteString("# HELP gamepanel_activity_events_24h_by_type Activity events by category recorded in the last 24 hours.\n")
	builder.WriteString("# TYPE gamepanel_activity_events_24h_by_type gauge\n")
	for _, item := range snapshot.Activity.ByType {
		builder.WriteString(`gamepanel_activity_events_24h_by_type{type="`)
		builder.WriteString(escapeLabel(item.Type))
		builder.WriteString(`"} `)
		builder.WriteString(fmt.Sprint(item.Count))
		builder.WriteByte('\n')
	}

	builder.WriteString("# HELP gamepanel_server_running Whether a server is currently running.\n")
	builder.WriteString("# TYPE gamepanel_server_running gauge\n")
	builder.WriteString("# HELP gamepanel_server_uptime_seconds Current server uptime in seconds.\n")
	builder.WriteString("# TYPE gamepanel_server_uptime_seconds gauge\n")
	builder.WriteString("# HELP gamepanel_server_cpu_percent Current server container CPU percent.\n")
	builder.WriteString("# TYPE gamepanel_server_cpu_percent gauge\n")
	builder.WriteString("# HELP gamepanel_server_memory_bytes Current server container memory usage in bytes.\n")
	builder.WriteString("# TYPE gamepanel_server_memory_bytes gauge\n")
	builder.WriteString("# HELP gamepanel_server_players_online Current players online for the server.\n")
	builder.WriteString("# TYPE gamepanel_server_players_online gauge\n")
	for _, server := range snapshot.Servers {
		labels := serverLabels(server)
		running := 0
		if server.Status == domain.StatusRunning {
			running = 1
		}
		builder.WriteString("gamepanel_server_running")
		builder.WriteString(labels)
		builder.WriteByte(' ')
		builder.WriteString(fmt.Sprint(running))
		builder.WriteByte('\n')
		builder.WriteString("gamepanel_server_uptime_seconds")
		builder.WriteString(labels)
		builder.WriteByte(' ')
		builder.WriteString(fmt.Sprint(server.UptimeSeconds))
		builder.WriteByte('\n')
		builder.WriteString("gamepanel_server_cpu_percent")
		builder.WriteString(labels)
		builder.WriteByte(' ')
		builder.WriteString(fmt.Sprint(server.CPUPercent))
		builder.WriteByte('\n')
		builder.WriteString("gamepanel_server_memory_bytes")
		builder.WriteString(labels)
		builder.WriteByte(' ')
		builder.WriteString(fmt.Sprint(megabytesToBytes(server.MemoryMB)))
		builder.WriteByte('\n')
		builder.WriteString("gamepanel_server_players_online")
		builder.WriteString(labels)
		builder.WriteByte(' ')
		builder.WriteString(fmt.Sprint(server.PlayersOnline))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func maxSeconds(minimum float64, value float64) float64 {
	if value < minimum {
		return minimum
	}
	return value
}

func summarizeActivity(events []domain.ActivityEvent) ActivitySummary {
	summary := ActivitySummary{WindowHours: 24}
	cutoff := time.Now().Add(-24 * time.Hour)
	byType := map[string]int{}
	for _, event := range events {
		if event.CreatedAt.Before(cutoff) {
			continue
		}
		summary.Total++
		category := activityCategory(event.Type)
		byType[category]++
		switch category {
		case "lifecycle":
			summary.Lifecycle++
		case "backup":
			summary.Backups++
		case "player":
			summary.Players++
		case "failure":
			summary.Failures++
		}
	}
	summary.ByType = make([]ActivityTypeCount, 0, len(byType))
	for eventType, count := range byType {
		summary.ByType = append(summary.ByType, ActivityTypeCount{Type: eventType, Count: count})
	}
	sort.Slice(summary.ByType, func(i, j int) bool {
		if summary.ByType[i].Count == summary.ByType[j].Count {
			return summary.ByType[i].Type < summary.ByType[j].Type
		}
		return summary.ByType[i].Count > summary.ByType[j].Count
	})
	return summary
}

func activityCategory(eventType string) string {
	switch {
	case strings.Contains(eventType, "failed") || strings.Contains(eventType, "errored"):
		return "failure"
	case strings.HasPrefix(eventType, "server."):
		return "lifecycle"
	case strings.HasPrefix(eventType, "backup.") || strings.HasPrefix(eventType, "save."):
		return "backup"
	case strings.HasPrefix(eventType, "player."):
		return "player"
	default:
		return "other"
	}
}

func statusRank(status domain.ServerStatus) int {
	switch status {
	case domain.StatusRunning:
		return 0
	case domain.StatusStarting, domain.StatusRestarting, domain.StatusStopping, domain.StatusCreating:
		return 1
	case domain.StatusErrored:
		return 2
	case domain.StatusStopped:
		return 3
	default:
		return 4
	}
}

func serverLabels(server ServerMetric) string {
	return fmt.Sprintf(
		`{server_id="%s",game_key="%s",provider_key="%s",status="%s"}`,
		escapeLabel(server.ID),
		escapeLabel(string(server.GameKey)),
		escapeLabel(string(server.ProviderKey)),
		escapeLabel(string(server.Status)),
	)
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func megabytesToBytes(value int64) int64 {
	return value * 1024 * 1024
}
