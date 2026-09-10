package regionaltask_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceaction"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instanceconfiguration"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/providercontract"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaltask"
)

func TestDurableBackupAndSameRegionRestoreFlow(t *testing.T) {
	global := openPhase5Database(t, "GAMEPANEL_GLOBAL_TEST_DSN", "global", []string{"0002_product_instance_messaging.sql", "0007_async_delivery.sql", "0008_provider_driven_operation.sql"})
	region := openPhase5Database(t, "GAMEPANEL_REGION_TEST_DSN", "region", []string{"0001_region_execution.sql", "0004_async_delivery.sql", "0005_provider_driven_operation.sql", "0006_node_task_ownership.sql"})
	now := time.Now().UTC().Truncate(time.Second)
	authorityKey := []byte("authority-key-012345678901234567")
	registry := providercontract.NewRegistry(providercontract.NewPostgresStore(global), []byte("provider-signing-key-012345678901"))
	manifest, err := registry.Publish(context.Background(), actionManifest(), now)
	if err != nil {
		t.Fatal(err)
	}
	insertGlobalInstance(t, global, manifest.ProviderReleaseID, now)
	if _, err := region.Exec(`INSERT INTO regional_delivery_states (id,workspace_id,logical_instance_id,region_id,placement_version,instance_revision_id,operation_id,desired_state,provider_release_id,game_version,cpu_milli,memory_mib,disk_gib,configuration,mod_lock,listener_requirements,phase,node_id,fencing_token,updated_at) VALUES ('rdp_one','ws_one','lin_one','reg_one',1,'rev_one','op_seed','running',$1,'1.0',1000,1024,10,'{}','[]','[]','ready','node_one',42,$2)`, manifest.ProviderReleaseID, now); err != nil {
		t.Fatal(err)
	}
	delivery := deliverycontrol.NewPostgres(global, []byte("funding-key-unused-012345678901"), authorityKey)
	actions := instanceaction.NewPostgres(global, delivery, registry, authorityKey)
	backup, operation, err := actions.RequestBackup(context.Background(), "ws_one", "lin_one", "backup-one", now)
	if err != nil {
		t.Fatal(err)
	}
	var messageID string
	var encoded []byte
	if err := global.QueryRow(`SELECT id,payload FROM global_outbox WHERE message_type='backup.requested.v1' AND idempotency_key='backup-one'`).Scan(&messageID, &encoded); err != nil {
		t.Fatal(err)
	}
	var requested instanceaction.BackupPayload
	if err := json.Unmarshal(encoded, &requested); err != nil {
		t.Fatal(err)
	}
	regional := regionaltask.NewPostgres(region, "reg_one", authorityKey)
	if accepted, err := regional.ReceiveBackup(context.Background(), messageID, requested, now.Add(time.Second)); err != nil || !accepted {
		t.Fatalf("receive accepted=%v err=%v", accepted, err)
	}
	task, ok, err := regional.Claim(context.Background(), "node_one", "node-agent", now.Add(2*time.Second))
	if err != nil || !ok {
		t.Fatalf("claim task=%#v ok=%v err=%v", task, ok, err)
	}
	if completed, err := regional.Complete(context.Background(), task.ID, "node-agent", task.FencingToken, regionaltask.BackupResult{Status: "completed", ObjectKey: requested.ObjectKey, SizeBytes: 128, Checksums: map[string]string{"sha256": "fixture"}}, now.Add(3*time.Second)); err != nil || !completed {
		t.Fatalf("complete=%v err=%v", completed, err)
	}
	if completed, err := regional.Complete(context.Background(), task.ID, "node-agent", task.FencingToken, regionaltask.BackupResult{Status: "completed"}, now.Add(4*time.Second)); err != nil || completed {
		t.Fatalf("duplicate complete=%v err=%v", completed, err)
	}
	var observedMessageID string
	if err := region.QueryRow(`SELECT id,payload FROM regional_outbox WHERE message_type='backup.observed.v1'`).Scan(&observedMessageID, &encoded); err != nil {
		t.Fatal(err)
	}
	var observed instanceaction.BackupObservation
	if err := json.Unmarshal(encoded, &observed); err != nil {
		t.Fatal(err)
	}
	observed.MessageID = observedMessageID
	if changed, err := actions.ApplyBackupObservation(context.Background(), observed); err != nil || !changed {
		t.Fatalf("observation changed=%v err=%v payload=%s", changed, err, encoded)
	}
	backups, err := actions.ListBackups(context.Background(), "ws_one", 100)
	if err != nil || len(backups) != 1 || backups[0].Status != "completed" || backups[0].OperationID != operation.ID {
		t.Fatalf("backups=%#v err=%v", backups, err)
	}
	if _, err := actions.Restore(context.Background(), "ws_one", "lin_one", backup.ID, "restore-one", now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := actions.Restore(context.Background(), "ws_one", "lin_other", backup.ID, "restore-other", now.Add(5*time.Second)); !errors.Is(err, instanceaction.ErrCrossRegionRestore) {
		t.Fatalf("cross-instance restore err=%v", err)
	}
	configuration := instanceconfiguration.New(registry, delivery, instanceconfiguration.NewPostgresStore(global))
	draft, err := configuration.CreateDraft(context.Background(), "ws_one", "lin_one", now.Add(6*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	draft, err = configuration.SaveDraft(context.Background(), instanceconfiguration.SaveCommand{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", DraftID: draft.ID, SchemaVersion: 1, Values: map[string]any{"unknown": true}}, now.Add(7*time.Second))
	if err != nil || len(draft.ValidationErrors) != 1 {
		t.Fatalf("invalid autosave=%#v err=%v", draft, err)
	}
	draft, err = configuration.SaveDraft(context.Background(), instanceconfiguration.SaveCommand{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", DraftID: draft.ID, SchemaVersion: 1, Values: map[string]any{}}, now.Add(8*time.Second))
	if err != nil || len(draft.ValidationErrors) != 0 {
		t.Fatalf("valid autosave=%#v err=%v", draft, err)
	}
	if revision, applyOperation, err := configuration.Apply(context.Background(), instanceconfiguration.ApplyCommand{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", DraftID: draft.ID, IdempotencyKey: "apply-config-one"}, now.Add(9*time.Second)); err != nil || revision.ID == "rev_one" || applyOperation.Kind != "instance.configuration.apply" {
		t.Fatalf("revision=%#v operation=%#v err=%v", revision, applyOperation, err)
	}
}

func insertGlobalInstance(t *testing.T, database *sql.DB, providerReleaseID string, now time.Time) {
	t.Helper()
	if _, err := database.Exec(`INSERT INTO managed_instances (id,workspace_id,region_id,name,provider_release_id,game_version,quote_id,instance_revision_id,placement_version,cpu_milli,memory_mib,disk_gib,configuration,mod_lock,listener_requirements,desired_state,observed_state,endpoint_bindings,latest_operation_id,created_at,updated_at) VALUES ('lin_one','ws_one','reg_one','one',$1,'1.0','quo_one','rev_one',1,1000,1024,10,'{}','[]','[]','running','running','[]','op_seed',$2,$2),('lin_other','ws_one','reg_two','other',$1,'1.0','quo_two','rev_other',1,1000,1024,10,'{}','[]','[]','running','running','[]','op_other',$2,$2)`, providerReleaseID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO managed_instance_revisions (id,operation_id,workspace_id,logical_instance_id,provider_release_id,game_version,schema_version,configuration,mod_lock,apply_behavior,created_at) VALUES ('rev_one','op_seed','ws_one','lin_one',$1,'1.0',1,'{}','[]','recreate-required',$2),('rev_other','op_other','ws_one','lin_other',$1,'1.0',1,'{}','[]','recreate-required',$2)`, providerReleaseID, now); err != nil {
		t.Fatal(err)
	}
}

func actionManifest() providercontract.Manifest {
	return providercontract.Manifest{ProviderReleaseID: "gpr_action", GameKey: "fixture", DisplayName: "Fixture", ReleaseVersion: "1", GameVersions: []string{"1.0"}, SchemaVersion: 1, ConfigurationSchema: providercontract.ConfigurationSchema{Type: "object", Properties: map[string]providercontract.Field{}}, UISchema: providercontract.UISchema{Sections: []providercontract.UISection{}, Fields: map[string]providercontract.UIField{}}, ListenerRequirements: []contract.ListenerRequirement{{Name: "game", Purpose: "join", Transports: []string{"udp"}, InternalPort: 7000, ExternalPortPolicy: "allocated", AddressMode: "ip-port", Primary: true}}, Capabilities: []string{"backup", "console"}, Metrics: []providercontract.Metric{}, SchemaMigrations: []providercontract.Migration{}}
}

func openPhase5Database(t *testing.T, environment, group string, migrations []string) *sql.DB {
	t.Helper()
	dsn := os.Getenv(environment)
	if dsn == "" {
		t.Skip(environment + " is not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("phase5_%s_%d", group, time.Now().UnixNano())
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
	root := filepath.Join(filepath.Dir(filename), "..", "..", "migrations", group)
	for _, name := range migrations {
		source, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range splitSQL(string(source)) {
			if _, err := database.Exec(statement); err != nil {
				t.Fatalf("migration %s: %v", name, err)
			}
		}
	}
	t.Cleanup(func() { database.Close(); _, _ = admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`); admin.Close() })
	return database
}

func splitSQL(source string) []string {
	var result []string
	start := 0
	inDollar := false
	for index := 0; index < len(source); index++ {
		if index+1 < len(source) && source[index:index+2] == "$$" {
			inDollar = !inDollar
			index++
			continue
		}
		if source[index] == ';' && !inDollar {
			if value := strings.TrimSpace(source[start:index]); value != "" {
				result = append(result, value)
			}
			start = index + 1
		}
	}
	if value := strings.TrimSpace(source[start:]); value != "" {
		result = append(result, value)
	}
	return result
}
