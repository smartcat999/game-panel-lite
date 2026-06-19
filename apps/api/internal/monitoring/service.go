package monitoring

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type Service struct {
	store *store.Store
	prom  *PrometheusClient
	now   func() time.Time
}

func NewService(store *store.Store, prom *PrometheusClient) *Service {
	return &Service{store: store, prom: prom, now: time.Now}
}

func (s *Service) Overview(ctx context.Context) (OverviewResponse, error) {
	now := s.now().UTC()
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return OverviewResponse{}, err
	}
	ds := s.dataSource(ctx)
	kpis := s.kpisFromStore(servers)
	kpis.StorageBytes = s.storageBytesFromStore(ctx, servers)
	if ds.Connected {
		kpis = s.kpisFromPrometheus(ctx, kpis)
	}
	failedTargets := 0
	if samples, err := s.queryVector(ctx, `up == 0`); err == nil {
		failedTargets = len(samples)
	}
	overall := "healthy"
	if !ds.Connected || failedTargets > 0 || kpis.Issues > 0 {
		overall = "warning"
	}
	return OverviewResponse{
		CollectedAt: now,
		DataSource:  ds,
		Health: Health{
			Overall:             overall,
			PrometheusConnected: ds.Connected,
			DockerRuntime:       s.dockerRuntimeStatus(ctx, ds),
			LastSync:            ds.LastQueryAt,
			FailedTargets:       failedTargets,
		},
		KPIs: kpis,
	}, nil
}

func (s *Service) kpisFromStore(servers []domain.GameServerInstance) KPIs {
	kpis := KPIs{}
	for _, server := range servers {
		kpis.TotalServers++
		kpis.OnlinePlayers += server.PlayersOnline
		kpis.PlayerCapacity += server.MaxPlayers
		if server.Status == domain.StatusRunning {
			kpis.RunningServers++
		}
		if isIssueStatus(server.Status) || server.LastError != "" {
			kpis.Issues++
		}
	}
	return kpis
}

func (s *Service) dockerRuntimeStatus(ctx context.Context, ds DataSource) string {
	if !ds.Connected {
		return "unknown"
	}
	samples, err := s.queryVector(ctx, `up{job="cadvisor"}`)
	if err != nil || len(samples) == 0 {
		return "unknown"
	}
	for _, sample := range samples {
		if sample.Value > 0 {
			return "healthy"
		}
	}
	return "down"
}

func (s *Service) kpisFromPrometheus(ctx context.Context, fallback KPIs) KPIs {
	kpis := fallback
	kpis.TotalServers = s.latestIntOr(ctx, `gamepanel_servers_total`, fallback.TotalServers)
	kpis.RunningServers = s.latestIntOr(ctx, `sum(gamepanel_servers_by_status{status="running"})`, fallback.RunningServers)
	kpis.Issues = s.latestIntOr(ctx, `sum(gamepanel_servers_by_status{status=~"errored|deleting"})`, fallback.Issues)
	kpis.OnlinePlayers = s.latestIntOr(ctx, `sum(gamepanel_server_players_online)`, fallback.OnlinePlayers)
	kpis.PlayerCapacity = s.latestIntOr(ctx, `sum(gamepanel_server_players_max)`, fallback.PlayerCapacity)
	kpis.StorageBytes = s.latestIntOr(ctx, `sum(gamepanel_asset_storage_total_bytes)`, fallback.StorageBytes)
	if cpu, err := s.latest(ctx, managedContainersCPUQuery()); err == nil && cpu != nil {
		kpis.ResourceUsagePercent = int(math.Round(*cpu))
	}
	return kpis
}

func (s *Service) storageBytesFromStore(ctx context.Context, servers []domain.GameServerInstance) int {
	total := int64(0)
	if backups, err := s.store.ListBackups(ctx); err == nil {
		for _, backup := range backups {
			total += backup.SizeBytes
		}
	}
	if worlds, err := s.store.ListWorlds(ctx); err == nil {
		for _, world := range worlds {
			total += world.SizeBytes
		}
	}
	for _, server := range servers {
		mods, err := s.store.ListMods(ctx, server.ID)
		if err != nil {
			continue
		}
		for _, mod := range mods {
			total += mod.SizeBytes
		}
	}
	return int(total)
}

func (s *Service) Metrics(ctx context.Context, window string, stepText string) (MetricsResponse, error) {
	rng := makeRange(s.now().UTC(), window, stepText)
	ds := s.dataSource(ctx)
	series := map[string]Series{
		"cpu":     s.series(ctx, rng, "cpu", "CPU Usage", "%", "area", managedContainersCPUQuery(), ptr(80)),
		"memory":  s.series(ctx, rng, "memory", "Memory Usage", "MB", "area", managedContainersMemoryQuery(), nil),
		"players": s.series(ctx, rng, "players", "Player Count", "players", "line", `sum(gamepanel_server_players_online)`, nil),
		"events":  s.series(ctx, rng, "events", "Events", "events", "bar", fmt.Sprintf(`sum(increase(gamepanel_activity_events_total[%s]))`, rng.Step), nil),
	}
	applyDataSourceEmptyReason(series, ds)
	return MetricsResponse{CollectedAt: s.now().UTC(), Range: rng, DataSource: ds, Series: series}, nil
}

func (s *Service) Platform(ctx context.Context, window string, stepText string) (PlatformResponse, error) {
	rng := makeRange(s.now().UTC(), window, stepText)
	ds := s.dataSource(ctx)
	nodeMemoryLimit, _ := s.latest(ctx, nodeMemoryTotalQuery())
	series := map[string]Series{
		"requests":   s.series(ctx, rng, "requests", "Request Rate", "req/s", "line", `sum(rate(gamepanel_api_http_requests_total[5m]))`, nil),
		"errors":     s.series(ctx, rng, "errors", "Error Rate", "errors/s", "line", `sum(rate(gamepanel_api_http_requests_total{status=~"5.."}[5m]))`, nil),
		"latencyP95": s.series(ctx, rng, "latencyP95", "API Latency p95", "ms", "line", `histogram_quantile(0.95, sum(rate(gamepanel_api_http_request_duration_seconds_bucket[5m])) by (le)) * 1000`, ptr(1000)),
		"sse":        s.series(ctx, rng, "sse", "SSE Connections", "connections", "line", `sum(gamepanel_api_sse_connections_active)`, nil),
		"nodeCpu":    s.series(ctx, rng, "nodeCpu", "Node CPU Usage", "%", "area", nodeCPUQuery(), ptr(100)),
		"nodeMemory": s.series(ctx, rng, "nodeMemory", "Node Memory Usage", "MB", "area", nodeMemoryQuery(), nodeMemoryLimit),
		"nodeDisk":   s.series(ctx, rng, "nodeDisk", "Node Disk Usage", "%", "area", nodeDiskQuery(), ptr(100)),
		"nodeNetwork": s.series(
			ctx,
			rng,
			"nodeNetwork",
			"Node Network",
			"MB/s",
			"line",
			nodeNetworkQuery(),
			nil,
		),
	}
	applyDataSourceEmptyReason(series, ds)
	services := s.platformServices(ctx, ds)
	topRoutes := s.topRoutes(ctx)
	return PlatformResponse{CollectedAt: s.now().UTC(), Range: rng, DataSource: ds, Services: services, Series: series, TopRoutes: topRoutes}, nil
}

func (s *Service) ServerMetrics(ctx context.Context, serverID string, window string, stepText string) (MetricsResponse, error) {
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return MetricsResponse{}, err
	}
	rng := makeRange(s.now().UTC(), window, stepText)
	ds := s.dataSource(ctx)
	labelFilter := fmt.Sprintf(`{server_id="%s"}`, escapePromLabel(server.ID))
	containerFilter := fmt.Sprintf(`{container_label_gamepanel_instance="%s"}`, escapePromLabel(server.ID))
	series := map[string]Series{
		"cpu":     s.series(ctx, rng, "cpu", "CPU Usage", "%", "area", containerCPUQuery(containerFilter), ptr(80)),
		"memory":  s.series(ctx, rng, "memory", "Memory Usage", "MB", "area", containerMemoryQuery(containerFilter), memoryThreshold(server)),
		"players": s.series(ctx, rng, "players", "Player Count", "players", "line", `gamepanel_server_players_online`+labelFilter, floatPtr(float64(server.MaxPlayers))),
		"uptime":  s.series(ctx, rng, "uptime", "Uptime", "s", "line", `gamepanel_server_running`+labelFilter, nil),
	}
	if server.Status != domain.StatusRunning {
		for key, item := range series {
			if len(item.Points) == 0 {
				item.EmptyReason = "server_stopped"
				series[key] = item
			}
		}
	}
	applyDataSourceEmptyReason(series, ds)
	return MetricsResponse{CollectedAt: s.now().UTC(), Range: rng, DataSource: ds, Series: series}, nil
}

func (s *Service) ServerLoad(ctx context.Context) (ServerLoadResponse, error) {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return ServerLoadResponse{}, err
	}
	ds := s.dataSource(ctx)
	rows := make([]ServerLoadRow, 0, len(servers))
	for _, server := range servers {
		containerFilter := fmt.Sprintf(`{container_label_gamepanel_instance="%s"}`, escapePromLabel(server.ID))
		cpu := s.latestQuery(ctx, containerCPUQuery(containerFilter))
		memory := s.latestQuery(ctx, containerMemoryBytesQuery(containerFilter)) / 1024 / 1024
		limit := float64(server.MemoryLimitMB)
		if limit <= 0 {
			limit = s.latestQuery(ctx, containerMemoryLimitBytesQuery(containerFilter)) / 1024 / 1024
		}
		severity := "normal"
		if isIssueStatus(server.Status) || server.LastError != "" {
			severity = "critical"
		} else if cpu > 80 || (limit > 0 && memory/limit*100 > 85) {
			severity = "warning"
		}
		rows = append(rows, ServerLoadRow{
			ServerID:      server.ID,
			ServerName:    server.Name,
			GameKey:       string(server.GameKey),
			ProviderKey:   string(server.ProviderKey),
			Version:       server.Version,
			Status:        string(server.Status),
			CPUPercent:    cpu,
			MemoryMB:      memory,
			MemoryLimitMB: limit,
			PlayersOnline: server.PlayersOnline,
			MaxPlayers:    server.MaxPlayers,
			LastActive:    server.UpdatedAt.UTC().Format(time.RFC3339),
			Severity:      severity,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return severityRank(rows[i].Severity) > severityRank(rows[j].Severity) || rows[i].CPUPercent > rows[j].CPUPercent
	})
	return ServerLoadResponse{CollectedAt: s.now().UTC(), DataSource: ds, Rows: rows}, nil
}

func (s *Service) Events(ctx context.Context, serverID string, limit int, severity, eventType, game string) (EventsResponse, error) {
	events, err := s.store.ListActivity(ctx, limit)
	if err != nil {
		return EventsResponse{}, err
	}
	servers, _ := s.store.ListServers(ctx)
	serverByID := map[string]domain.GameServerInstance{}
	for _, server := range servers {
		serverByID[server.ID] = server
	}
	items := make([]Event, 0, len(events))
	for _, event := range events {
		if serverID != "" && event.InstanceID != serverID {
			continue
		}
		server := serverByID[event.InstanceID]
		item := eventFromActivity(event, server)
		if severity != "" && severity != "all" && item.Severity != severity {
			continue
		}
		if eventType != "" && eventType != "all" && item.Type != eventType {
			continue
		}
		if game != "" && game != "all" && string(server.GameKey) != game {
			continue
		}
		items = append(items, item)
	}
	return EventsResponse{CollectedAt: s.now().UTC(), Events: items}, nil
}

func (s *Service) dataSource(ctx context.Context) DataSource {
	now := s.now().UTC()
	ds := DataSource{Kind: "prometheus", EndpointConfigured: s.prom != nil && s.prom.Configured()}
	if !ds.EndpointConfigured {
		ds.Error = "prometheus is not configured"
		return ds
	}
	_, err := s.prom.QueryVector(ctx, "up")
	ds.LastQueryAt = &now
	if err != nil {
		ds.Error = err.Error()
		return ds
	}
	ds.Connected = true
	return ds
}

func (s *Service) series(ctx context.Context, rng Range, key, title, unit, chartType, query string, threshold *float64) Series {
	item := Series{Key: key, Title: title, Unit: unit, ChartType: chartType, Threshold: threshold}
	if s.prom == nil || !s.prom.Configured() {
		item.EmptyReason = "prometheus_unconfigured"
		return item
	}
	step, _ := time.ParseDuration(rng.Step)
	points, err := s.prom.QueryRange(ctx, query, rng.Start, rng.End, step)
	if err != nil {
		item.EmptyReason = "prometheus_unavailable"
		return item
	}
	item.Points = points
	if len(points) == 0 {
		item.EmptyReason = "no_samples"
		return item
	}
	current, avg, max := summarize(points)
	item.CurrentValue = &current
	item.Avg = &avg
	item.Max = &max
	return item
}

func (s *Service) queryVector(ctx context.Context, query string) ([]VectorSample, error) {
	if s.prom == nil || !s.prom.Configured() {
		return nil, fmt.Errorf("prometheus is not configured")
	}
	return s.prom.QueryVector(ctx, query)
}

func (s *Service) latest(ctx context.Context, query string) (*float64, error) {
	samples, err := s.queryVector(ctx, query)
	if err != nil || len(samples) == 0 {
		return nil, err
	}
	value := 0.0
	for _, sample := range samples {
		value += sample.Value
	}
	return &value, nil
}

func (s *Service) latestIntOr(ctx context.Context, query string, fallback int) int {
	value, err := s.latest(ctx, query)
	if err != nil || value == nil {
		return fallback
	}
	return int(math.Round(*value))
}

func (s *Service) latestForServer(ctx context.Context, metric string, serverID string) float64 {
	value, err := s.latest(ctx, fmt.Sprintf(`%s{server_id="%s"}`, metric, escapePromLabel(serverID)))
	if err != nil || value == nil {
		return 0
	}
	return *value
}

func (s *Service) latestQuery(ctx context.Context, query string) float64 {
	value, err := s.latest(ctx, query)
	if err != nil || value == nil {
		return 0
	}
	return *value
}

func (s *Service) platformServices(ctx context.Context, ds DataSource) []PlatformService {
	services := []PlatformService{{Name: "prometheus", Status: "down"}}
	if ds.Connected {
		services[0].Status = "healthy"
	}
	samples, err := s.queryVector(ctx, `up`)
	if err != nil {
		return services
	}
	for _, sample := range samples {
		job := sample.Metric["job"]
		if job == "" {
			job = sample.Metric["instance"]
		}
		status := "down"
		if sample.Value > 0 {
			status = "healthy"
		}
		services = append(services, PlatformService{Name: job, Status: status, Instance: sample.Metric["instance"]})
	}
	return services
}

func (s *Service) topRoutes(ctx context.Context) []RouteMetric {
	rateSamples, rateErr := s.queryVector(ctx, `sum by (method, route) (rate(gamepanel_api_http_requests_total[5m]))`)
	countSamples, countErr := s.queryVector(ctx, `sum by (method, route) (increase(gamepanel_api_http_requests_total[5m]))`)
	if rateErr != nil && countErr != nil {
		return nil
	}
	rowsByKey := map[string]RouteMetric{}
	for _, sample := range rateSamples {
		method := sample.Metric["method"]
		route := sample.Metric["route"]
		key := method + "\x00" + route
		row := rowsByKey[key]
		row.Method = method
		row.Route = route
		row.RequestRate = sample.Value
		rowsByKey[key] = row
	}
	for _, sample := range countSamples {
		method := sample.Metric["method"]
		route := sample.Metric["route"]
		key := method + "\x00" + route
		row := rowsByKey[key]
		row.Method = method
		row.Route = route
		row.RequestCount = sample.Value
		rowsByKey[key] = row
	}
	rows := make([]RouteMetric, 0, len(rowsByKey))
	for _, row := range rowsByKey {
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].RequestCount == rows[j].RequestCount {
			return rows[i].RequestRate > rows[j].RequestRate
		}
		return rows[i].RequestCount > rows[j].RequestCount
	})
	if len(rows) > 8 {
		rows = rows[:8]
	}
	return rows
}

func makeRange(now time.Time, window, stepText string) Range {
	if window == "" {
		window = "15m"
	}
	if stepText == "" {
		stepText = "30s"
	}
	duration, err := time.ParseDuration(window)
	if err != nil || duration <= 0 {
		window = "15m"
		duration = 15 * time.Minute
	}
	step, err := time.ParseDuration(stepText)
	if err != nil || step <= 0 {
		stepText = "30s"
	}
	return Range{Range: window, Step: stepText, Start: now.Add(-duration), End: now}
}

func summarize(points []Point) (float64, float64, float64) {
	current := points[len(points)-1].Value
	sum := 0.0
	max := points[0].Value
	for _, point := range points {
		sum += point.Value
		if point.Value > max {
			max = point.Value
		}
	}
	return current, sum / float64(len(points)), max
}

func sortPoints(points []Point) {
	sort.Slice(points, func(i, j int) bool { return points[i].Timestamp.Before(points[j].Timestamp) })
}

func eventFromActivity(event domain.ActivityEvent, server domain.GameServerInstance) Event {
	severity := "info"
	if strings.Contains(event.Type, "failed") || strings.Contains(event.Type, "error") {
		severity = "error"
	} else if strings.Contains(event.Type, "deleted") || strings.Contains(event.Type, "stop") {
		severity = "warning"
	} else if strings.Contains(event.Type, "created") || strings.Contains(event.Type, "started") || strings.Contains(event.Type, "restored") {
		severity = "success"
	}
	return Event{
		ID:         event.ID,
		Severity:   severity,
		Type:       event.Type,
		Title:      titleFromType(event.Type),
		Message:    event.Message,
		ServerID:   event.InstanceID,
		ServerName: server.Name,
		Operator:   "system",
		Timestamp:  event.CreatedAt,
	}
}

func titleFromType(value string) string {
	value = strings.ReplaceAll(value, ".", " ")
	if value == "" {
		return "System event"
	}
	return strings.Title(value)
}

func isIssueStatus(status domain.ServerStatus) bool {
	return status == domain.StatusErrored
}

func severityRank(value string) int {
	switch value {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func applyDataSourceEmptyReason(series map[string]Series, ds DataSource) {
	if ds.Connected {
		return
	}
	reason := "prometheus_unavailable"
	if !ds.EndpointConfigured {
		reason = "prometheus_unconfigured"
	}
	for key, item := range series {
		if len(item.Points) == 0 {
			item.EmptyReason = reason
			series[key] = item
		}
	}
}

func escapePromLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func managedContainersCPUQuery() string {
	return containerCPUQuery(`{container_label_gamepanel_instance!=""}`)
}

func managedContainersMemoryQuery() string {
	return containerMemoryQuery(`{container_label_gamepanel_instance!=""}`)
}

func containerCPUQuery(filter string) string {
	return fmt.Sprintf(`sum(rate(container_cpu_usage_seconds_total%s[2m])) * 100`, filter)
}

func containerMemoryQuery(filter string) string {
	return containerMemoryBytesQuery(filter) + ` / 1024 / 1024`
}

func containerMemoryBytesQuery(filter string) string {
	return fmt.Sprintf(`sum(container_memory_working_set_bytes%s)`, filter)
}

func containerMemoryLimitBytesQuery(filter string) string {
	return fmt.Sprintf(`sum(container_spec_memory_limit_bytes%s)`, filter)
}

func nodeCPUQuery() string {
	return `100 - (avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`
}

func nodeMemoryQuery() string {
	return `(sum(node_memory_MemTotal_bytes) - sum(node_memory_MemAvailable_bytes)) / 1024 / 1024`
}

func nodeMemoryTotalQuery() string {
	return `sum(node_memory_MemTotal_bytes) / 1024 / 1024`
}

func nodeDiskQuery() string {
	filter := `{fstype!~"tmpfs|overlay|squashfs|aufs",mountpoint!~"/run($|/.*)|/var/lib/docker($|/.*)"}`
	return fmt.Sprintf(`100 * (1 - (sum(node_filesystem_avail_bytes%s) / sum(node_filesystem_size_bytes%s)))`, filter, filter)
}

func nodeNetworkQuery() string {
	filter := `{device!~"lo|docker.*|br-.*|veth.*"}`
	return fmt.Sprintf(`sum(rate(node_network_receive_bytes_total%s[5m]) + rate(node_network_transmit_bytes_total%s[5m])) / 1024 / 1024`, filter, filter)
}

func ptr(value float64) *float64 {
	return &value
}

func floatPtr(value float64) *float64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func memoryThreshold(server domain.GameServerInstance) *float64 {
	if server.MemoryLimitMB <= 0 {
		return nil
	}
	value := float64(server.MemoryLimitMB)
	return &value
}
