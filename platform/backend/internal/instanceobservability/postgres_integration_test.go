package instanceobservability

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
)

func TestTelemetryRemainsInstanceKeyedAcrossRuntimeAttempts(t *testing.T) {
	database := openObservabilityDatabase(t)
	registry := providercontract.NewRegistry(providercontract.NewPostgresStore(database), []byte("provider-signing-key-012345678901"))
	now := time.Now().UTC().Truncate(time.Second)
	manifest, err := registry.Publish(context.Background(), observabilityManifest(), now)
	if err != nil {
		t.Fatal(err)
	}
	listeners := `[{"name":"game","purpose":"join","transports":["udp"],"internalPort":7000,"externalPortPolicy":"allocated","addressMode":"ip-port","primary":true}]`
	if _, err := database.Exec(`INSERT INTO managed_instances (id,workspace_id,region_id,name,provider_release_id,game_version,quote_id,instance_revision_id,placement_version,cpu_milli,memory_mib,disk_gib,configuration,mod_lock,listener_requirements,desired_state,observed_state,endpoint_bindings,latest_operation_id,created_at,updated_at) VALUES ('lin_one','ws_one','reg_one','one',$1,'1.0','quo_one','rev_one',1,1000,1024,10,'{}','[]',$2,'running','running','[]','op_one',$3,$3)`, manifest.ProviderReleaseID, listeners, now); err != nil {
		t.Fatal(err)
	}
	service := NewPostgres(database, registry)
	confidence, lowConfidence := 1.0, 0.2
	first := Observation{MessageID: "msg_first", WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", RegionID: "reg_one", RegionalDeploymentID: "rdp_one", RuntimeAttemptID: "rta_alpha", FencingToken: 1, Sequence: 1, Logs: []LogEntry{{ID: "log_alpha", Stream: "system", Message: "alpha ready", ObservedAt: now}}, Metrics: []MetricSample{{ID: "met_cpu", Metric: "cpu.utilization", Value: .1, Unit: "ratio", Source: "platform", SampledAt: now}, {ID: "met_players", Metric: "players.online", Value: 4, Unit: "count", Source: "provider-api", Confidence: &confidence, FreshUntil: pointerTime(now.Add(time.Minute)), SampledAt: now}, {ID: "met_untrusted", Metric: "players.online", Value: 9, Unit: "count", Source: "provider-api", Confidence: &lowConfidence, FreshUntil: pointerTime(now.Add(time.Minute)), SampledAt: now}}, ObservedAt: now}
	if changed, err := service.Handle(context.Background(), first); err != nil || !changed {
		t.Fatalf("first changed=%v err=%v", changed, err)
	}
	second := Observation{MessageID: "msg_second", WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", RegionID: "reg_one", RegionalDeploymentID: "rdp_two", RuntimeAttemptID: "rta_beta", FencingToken: 2, Sequence: 2, Logs: []LogEntry{{ID: "log_beta", Stream: "system", Message: "beta ready", ObservedAt: now.Add(time.Second)}}, Metrics: []MetricSample{}, ObservedAt: now.Add(time.Second)}
	if changed, err := service.Handle(context.Background(), second); err != nil || !changed {
		t.Fatalf("second changed=%v err=%v", changed, err)
	}
	empty := Observation{MessageID: "msg_empty", WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", RegionID: "reg_one", RegionalDeploymentID: "rdp_two", RuntimeAttemptID: "rta_beta", FencingToken: 2, Sequence: 3, ObservedAt: now.Add(2 * time.Second)}
	if changed, err := service.Handle(context.Background(), empty); err != nil || !changed {
		t.Fatalf("empty changed=%v err=%v", changed, err)
	}
	logs, err := service.Logs(context.Background(), "ws_one", "lin_one", 500)
	if err != nil || len(logs) != 2 || logs[0].RuntimeAttemptID == logs[1].RuntimeAttemptID {
		t.Fatalf("logs=%#v err=%v", logs, err)
	}
	metrics, err := service.Metrics(context.Background(), "ws_one", "lin_one", now.Add(10*time.Second), 1000)
	if err != nil || len(metrics) != 2 {
		t.Fatalf("metrics=%#v err=%v", metrics, err)
	}
	if changed, err := service.Handle(context.Background(), first); err != nil || changed {
		t.Fatalf("duplicate changed=%v err=%v", changed, err)
	}
}

func observabilityManifest() providercontract.Manifest {
	return providercontract.Manifest{ProviderReleaseID: "gpr_observe", GameKey: "fixture", DisplayName: "Fixture", ReleaseVersion: "1", GameVersions: []string{"1.0"}, SchemaVersion: 1, ConfigurationSchema: providercontract.ConfigurationSchema{Type: "object", Properties: map[string]providercontract.Field{}}, UISchema: providercontract.UISchema{Sections: []providercontract.UISection{}, Fields: map[string]providercontract.UIField{}}, ListenerRequirements: []contract.ListenerRequirement{{Name: "game", Purpose: "join", Transports: []string{"udp"}, InternalPort: 7000, ExternalPortPolicy: "allocated", AddressMode: "ip-port", Primary: true}}, Capabilities: []string{"logs", "game-metrics"}, Metrics: []providercontract.Metric{{Key: "players.online", Title: "Players", Unit: "count", Source: "provider-api", FreshnessSeconds: 30, MinimumConfidence: 1}}, SchemaMigrations: []providercontract.Migration{}}
}

func openObservabilityDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("GAMEPANEL_GLOBAL_TEST_DSN")
	if dsn == "" {
		t.Skip("GAMEPANEL_GLOBAL_TEST_DSN is not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("phase5_observability_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	database, err := sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filename), "..", "..", "migrations", "global")
	for _, name := range []string{"0002_product_instance_messaging.sql", "0007_async_delivery.sql", "0008_provider_driven_operation.sql"} {
		source, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range strings.Split(string(source), ";") {
			if statement = strings.TrimSpace(statement); statement != "" {
				if _, err := database.Exec(statement); err != nil {
					t.Fatalf("migration %s: %v", name, err)
				}
			}
		}
	}
	t.Cleanup(func() { database.Close(); _, _ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`); admin.Close() })
	return database
}

func pointerTime(value time.Time) *time.Time { return &value }
