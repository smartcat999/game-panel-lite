package exporter

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
)

type Collector struct {
	store *store.Store
}

func NewCollector(store *store.Store) *Collector {
	return &Collector{store: store}
}

func (c *Collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := c.Text(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func (c *Collector) Text(ctx context.Context) (string, error) {
	servers, err := c.store.ListGameServers(ctx)
	if err != nil {
		return "", err
	}
	backups, _ := c.store.ListBackups(ctx)
	worlds, _ := c.store.ListWorlds(ctx)
	events, _ := c.store.ListActivity(ctx, 100)

	var b strings.Builder
	writeScalar(&b, "gamepanel_exporter_build_info", "GamePanel exporter build information.", "gauge", 1)
	writeScalar(&b, "gamepanel_servers_total", "Total servers managed by GamePanel.", "gauge", float64(len(servers)))
	writeServersByStatus(&b, servers)
	writeServerMetrics(&b, servers)
	writeAssets(&b, c, ctx, servers, backupsByInstance(backups), worldsByInstance(worlds))
	writeEvents(&b, events)
	return b.String(), nil
}

func writeServersByStatus(b *strings.Builder, servers []domain.GameServer) {
	counts := map[string]float64{}
	for _, server := range servers {
		counts[string(legacyServerStatus(server))]++
	}
	writeHeader(b, "gamepanel_servers_by_status", "Servers by lifecycle status.", "gauge")
	for _, status := range sortedFloatKeys(counts) {
		writeSample(b, "gamepanel_servers_by_status", map[string]string{"status": status}, counts[status])
	}
}

func writeServerMetrics(b *strings.Builder, servers []domain.GameServer) {
	writeHeader(b, "gamepanel_server_info", "Server metadata with low-cardinality labels.", "gauge")
	writeHeader(b, "gamepanel_server_running", "Whether the server is running.", "gauge")
	writeHeader(b, "gamepanel_server_uptime_seconds", "Current server uptime in seconds.", "gauge")
	writeHeader(b, "gamepanel_server_status", "Current server lifecycle status as a labelled gauge.", "gauge")
	writeHeader(b, "gamepanel_server_players_online", "Current online players.", "gauge")
	writeHeader(b, "gamepanel_server_players_max", "Configured max players.", "gauge")
	writeHeader(b, "gamepanel_server_config_revision", "Current config revision.", "gauge")
	writeHeader(b, "gamepanel_server_applied_config_revision", "Applied config revision.", "gauge")
	for _, server := range servers {
		labels := serverLabels(server)
		writeSample(b, "gamepanel_server_info", labels, 1)
		running := 0.0
		if server.Status.Phase == domain.PhaseRunning {
			running = 1
		}
		writeSample(b, "gamepanel_server_running", labels, running)
		writeSample(b, "gamepanel_server_uptime_seconds", labels, serverUptimeSeconds(server, time.Now()))
		writeSample(b, "gamepanel_server_status", serverStatusLabels(server), 1)
		writeSample(b, "gamepanel_server_players_online", labels, float64(server.Status.PlayersOnline))
		writeSample(b, "gamepanel_server_players_max", labels, float64(domain.ServerMaxPlayers(server)))
		writeSample(b, "gamepanel_server_config_revision", labels, float64(server.Spec.Generation))
		writeSample(b, "gamepanel_server_applied_config_revision", labels, float64(server.Status.AppliedGeneration))
	}
}

func serverUptimeSeconds(server domain.GameServer, now time.Time) float64 {
	if server.Status.Phase != domain.PhaseRunning || server.Status.LastTransitionAt.IsZero() {
		return 0
	}
	uptime := now.Sub(server.Status.LastTransitionAt).Seconds()
	if uptime < 0 {
		return 0
	}
	return uptime
}

func writeAssets(b *strings.Builder, collector *Collector, ctx context.Context, servers []domain.GameServer, backups assetStats, worlds assetStats) {
	writeHeader(b, "gamepanel_backups_total", "Backups by server.", "gauge")
	writeHeader(b, "gamepanel_worlds_total", "Worlds by server.", "gauge")
	writeHeader(b, "gamepanel_mods_total", "Mods by server.", "gauge")
	writeHeader(b, "gamepanel_backup_storage_bytes", "Backup storage bytes by server.", "gauge")
	writeHeader(b, "gamepanel_world_storage_bytes", "World storage bytes by server.", "gauge")
	writeHeader(b, "gamepanel_mod_storage_bytes", "Mod storage bytes by server.", "gauge")
	writeHeader(b, "gamepanel_asset_storage_bytes", "Managed asset storage bytes by server and kind.", "gauge")
	totals := map[string]float64{"backup": 0, "mod": 0, "world": 0}
	for _, server := range servers {
		mods, err := collector.store.ListMods(ctx, server.ID)
		if err != nil {
			mods = nil
		}
		modBytes := 0.0
		for _, mod := range mods {
			modBytes += float64(mod.SizeBytes)
		}
		backup := backups[server.ID]
		world := worlds[server.ID]
		labels := serverStaticLabels(server)
		writeSample(b, "gamepanel_backups_total", labels, backup.Count)
		writeSample(b, "gamepanel_worlds_total", labels, world.Count)
		writeSample(b, "gamepanel_mods_total", labels, float64(len(mods)))
		writeSample(b, "gamepanel_backup_storage_bytes", labels, backup.Bytes)
		writeSample(b, "gamepanel_world_storage_bytes", labels, world.Bytes)
		writeSample(b, "gamepanel_mod_storage_bytes", labels, modBytes)
		writeSample(b, "gamepanel_asset_storage_bytes", labelsWithKind(labels, "backup"), backup.Bytes)
		writeSample(b, "gamepanel_asset_storage_bytes", labelsWithKind(labels, "world"), world.Bytes)
		writeSample(b, "gamepanel_asset_storage_bytes", labelsWithKind(labels, "mod"), modBytes)
		totals["backup"] += backup.Bytes
		totals["world"] += world.Bytes
		totals["mod"] += modBytes
	}
	writeHeader(b, "gamepanel_asset_storage_total_bytes", "Total managed asset storage bytes by kind.", "gauge")
	for _, kind := range sortedFloatKeys(totals) {
		writeSample(b, "gamepanel_asset_storage_total_bytes", map[string]string{"kind": kind}, totals[kind])
	}
}

func writeEvents(b *strings.Builder, events []domain.ActivityEvent) {
	writeHeader(b, "gamepanel_activity_events_total", "Recent activity events by type and severity.", "counter")
	counts := map[string]float64{}
	for _, event := range events {
		key := event.Type + "\xff" + severityForEvent(event.Type)
		counts[key]++
	}
	for _, key := range sortedFloatKeys(counts) {
		parts := strings.Split(key, "\xff")
		writeSample(b, "gamepanel_activity_events_total", map[string]string{"type": parts[0], "severity": parts[1]}, counts[key])
	}
}

type assetStat struct {
	Count float64
	Bytes float64
}

type assetStats map[string]assetStat

func backupsByInstance(backups []domain.Backup) assetStats {
	counts := assetStats{}
	for _, item := range backups {
		stat := counts[item.InstanceID]
		stat.Count++
		stat.Bytes += float64(item.SizeBytes)
		counts[item.InstanceID] = stat
	}
	return counts
}

func worldsByInstance(worlds []domain.World) assetStats {
	counts := assetStats{}
	for _, item := range worlds {
		stat := counts[item.InstanceID]
		stat.Count++
		stat.Bytes += float64(item.SizeBytes)
		counts[item.InstanceID] = stat
	}
	return counts
}

func serverLabels(server domain.GameServer) map[string]string {
	return map[string]string{
		"server_id":    server.ID,
		"game_key":     string(server.GameKey),
		"provider_key": string(server.ProviderKey),
		"status":       string(legacyServerStatus(server)),
		"version":      server.Spec.Version,
	}
}

func serverStaticLabels(server domain.GameServer) map[string]string {
	return map[string]string{
		"server_id":    server.ID,
		"game_key":     string(server.GameKey),
		"provider_key": string(server.ProviderKey),
	}
}

func serverStatusLabels(server domain.GameServer) map[string]string {
	labels := serverStaticLabels(server)
	labels["status"] = string(legacyServerStatus(server))
	return labels
}

func legacyServerStatus(server domain.GameServer) domain.ServerStatus {
	return domain.ServerStatusFromRuntime(server.Spec.DesiredState, server.Status)
}

func labelsWithKind(labels map[string]string, kind string) map[string]string {
	next := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		next[key] = value
	}
	next["kind"] = kind
	return next
}

func severityForEvent(eventType string) string {
	if strings.Contains(eventType, "failed") || strings.Contains(eventType, "error") {
		return "error"
	}
	if strings.Contains(eventType, "deleted") || strings.Contains(eventType, "stop") {
		return "warning"
	}
	if strings.Contains(eventType, "created") || strings.Contains(eventType, "started") || strings.Contains(eventType, "restored") || strings.Contains(eventType, "succeeded") {
		return "success"
	}
	return "info"
}

func writeScalar(b *strings.Builder, name, help, metricType string, value float64) {
	writeHeader(b, name, help, metricType)
	fmt.Fprintf(b, "%s %s\n", name, formatFloat(value))
}

func writeHeader(b *strings.Builder, name, help, metricType string) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, metricType)
}

func writeSample(b *strings.Builder, name string, labels map[string]string, value float64) {
	b.WriteString(name)
	if len(labels) > 0 {
		keys := make([]string, 0, len(labels))
		for key := range labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for index, key := range keys {
			if index > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(b, `%s="%s"`, key, escape(labels[key]))
		}
		b.WriteByte('}')
	}
	fmt.Fprintf(b, " %s\n", formatFloat(value))
}

func sortedFloatKeys(values map[string]float64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func escape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return value
}

func formatFloat(value float64) string {
	return fmt.Sprintf("%g", value)
}

func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","time":%q}`, time.Now().UTC().Format(time.RFC3339))
}
