package regionaldelivery_test

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
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/deliverycontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionaldelivery"
)

var deliveryNow = time.Now().UTC().Truncate(time.Second)

func TestDurableDeliveryRecoversEveryStepAndComposesEndpointTruth(t *testing.T) {
	global := openDeliveryDatabase(t, "GAMEPANEL_GLOBAL_TEST_DSN", "global", []string{"0002_product_instance_messaging.sql", "0006_resource_pricing_wallet.sql", "0007_async_delivery.sql"})
	region := openDeliveryDatabase(t, "GAMEPANEL_REGION_TEST_DSN", "region", []string{"0001_region_execution.sql", "0004_async_delivery.sql"})
	fundingKey := []byte("funding-key-01234567890123456789")
	authorityKey := []byte("authority-key-012345678901234567")
	quote := authorizedQuote(t, global, fundingKey)
	listeners := []deliverycontrol.ListenerRequirement{
		{Name: "game", Purpose: "join", Transports: []string{"tcp", "udp"}, InternalPort: 7777, ExternalPortPolicy: "allocated", AddressMode: "ip-port", Primary: true},
		{Name: "discovery", Purpose: "direct discovery", Transports: []string{"udp"}, InternalPort: 7778, ExternalPortPolicy: "allocated", AddressMode: "ip-only"},
	}
	command := deliverycontrol.CreateCommand{WorkspaceID: "ws_ember", Name: "server-one", ProviderReleaseID: "gpr_fake_v1", QuoteID: quote.ID, Configuration: map[string]any{"difficulty": "normal"}, ListenerRequirements: listeners, IdempotencyKey: "create-server-one"}
	control := deliverycontrol.NewPostgres(global, fundingKey, authorityKey)
	if _, err := global.Exec(`UPDATE resource_quotes SET funding_signature='forged' WHERE id=$1`, quote.ID); err != nil {
		t.Fatal(err)
	}
	forgedCommand := command
	forgedCommand.IdempotencyKey = "forged-funding"
	if _, _, err := control.Create(context.Background(), forgedCommand, deliveryNow); !errors.Is(err, billing.ErrInsufficientFunds) {
		t.Fatalf("forged funding error=%v", err)
	}
	if _, err := global.Exec(`UPDATE resource_quotes SET funding_signature=$2 WHERE id=$1`, quote.ID, quote.Funding.Signature); err != nil {
		t.Fatal(err)
	}
	instance, operation, err := control.Create(context.Background(), command, deliveryNow)
	if err != nil {
		t.Fatal(err)
	}
	if instance.ObservedState != "pending" || operation.Status != "queued" {
		t.Fatalf("instance=%#v operation=%#v", instance, operation)
	}
	var holdStatus string
	if err := global.QueryRow(`SELECT hold_status FROM resource_quotes WHERE id=$1`, quote.ID).Scan(&holdStatus); err != nil || holdStatus != "consumed" {
		t.Fatalf("hold status=%q err=%v", holdStatus, err)
	}
	// Idempotent replay returns the committed result even after the Quote expires.
	replayedInstance, replayedOperation, err := control.Create(context.Background(), command, quote.ExpiresAt.Add(time.Second))
	if err != nil || replayedInstance.ID != instance.ID || replayedOperation.ID != operation.ID {
		t.Fatalf("replay instance=%#v operation=%#v err=%v", replayedInstance, replayedOperation, err)
	}
	changed := command
	changed.Name = "different"
	if _, _, err := control.Create(context.Background(), changed, deliveryNow); !errors.Is(err, deliverycontrol.ErrImmutable) {
		t.Fatalf("changed replay error=%v", err)
	}

	desired := loadDesired(t, global, instance.ID)
	outbox := messaging.NewPostgresOutbox(global, messaging.GlobalOutbox)
	dispatcher := messaging.Dispatcher{Outbox: outbox, Publisher: integrationPublisher{failure: errors.New("broker offline")}, Backoff: 5 * time.Second}
	if count, err := dispatcher.Dispatch(context.Background(), 100, deliveryNow); count != 0 || err == nil {
		t.Fatalf("broker outage count=%d err=%v", count, err)
	}
	var attempts int
	var publishedAt sql.NullTime
	if err := global.QueryRow(`SELECT attempt_count,published_at FROM global_outbox WHERE payload->>'logicalInstanceId'=$1`, instance.ID).Scan(&attempts, &publishedAt); err != nil || attempts != 1 || publishedAt.Valid {
		t.Fatalf("outbox attempts=%d published=%v err=%v", attempts, publishedAt.Valid, err)
	}
	dispatcher.Publisher = integrationPublisher{}
	if count, err := dispatcher.Dispatch(context.Background(), 100, deliveryNow.Add(6*time.Second)); count != 1 || err != nil {
		t.Fatalf("broker recovery count=%d err=%v", count, err)
	}
	tampered := desired
	tampered.ResourceSpec.MemoryMiB++
	regional := regionaldelivery.NewPostgres(region, "reg_asia", authorityKey)
	if _, err := regional.ReceiveDesired(context.Background(), "msg_tampered", tampered, deliveryNow); !errors.Is(err, regionaldelivery.ErrInvalidDesired) {
		t.Fatalf("tampered authority error=%v", err)
	}
	accepted, err := regional.ReceiveDesired(context.Background(), "msg_desired_one", desired, deliveryNow)
	if err != nil || !accepted {
		t.Fatalf("receive accepted=%v err=%v", accepted, err)
	}
	if accepted, err := regional.ReceiveDesired(context.Background(), "msg_desired_one", desired, deliveryNow); err != nil || accepted {
		t.Fatalf("duplicate accepted=%v err=%v", accepted, err)
	}
	if err := regional.RegisterNode(context.Background(), regionaldelivery.Node{ID: "node_one", RegionID: "reg_asia", State: "ready", CPUCapacityMilli: 8000, MemoryCapacityMiB: 16384, DiskCapacityGiB: 100, LeaseUntil: deliveryNow.Add(time.Hour), UpdatedAt: deliveryNow}); err != nil {
		t.Fatal(err)
	}
	portStart, portEnd := 30000, 30010
	if err := regional.AddEndpointPool(context.Background(), regionaldelivery.EndpointPool{ID: "pool_gateway", RegionID: "reg_asia", DeliveryMode: "gateway", Address: "play.asia.example", PortStart: &portStart, PortEnd: &portEnd, Stability: "stable", Active: true}); err != nil {
		t.Fatal(err)
	}
	if err := regional.AddEndpointPool(context.Background(), regionaldelivery.EndpointPool{ID: "pool_ip", RegionID: "reg_asia", DeliveryMode: "dedicated-ip", Address: "203.0.113.10", Stability: "stable", Active: true}); err != nil {
		t.Fatal(err)
	}

	if _, err := region.Exec(`UPDATE regional_delivery_states SET reconcile_owner='crashed-worker',reconcile_lease_until=$2 WHERE logical_instance_id=$1`, instance.ID, deliveryNow.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if worked, err := regional.ReconcileOne(context.Background(), "early-replacement", deliveryNow.Add(time.Second)); err != nil || worked {
		t.Fatalf("unexpired reconcile lease worked=%v err=%v", worked, err)
	}
	// Each call advances one committed step. Recreating the service between calls simulates a crash at every boundary.
	for index := 0; index < 3; index++ {
		regional = regionaldelivery.NewPostgres(region, "reg_asia", authorityKey)
		worked, err := regional.ReconcileOne(context.Background(), fmt.Sprintf("region-worker-%d", index), deliveryNow.Add(time.Duration(index+31)*time.Second))
		if err != nil || !worked {
			t.Fatalf("step %d worked=%v err=%v", index, worked, err)
		}
	}
	assignment, ok, err := regional.ClaimAssignment(context.Background(), "node_one", "node-agent-one", deliveryNow.Add(34*time.Second))
	if err != nil || !ok || len(assignment.Endpoints) != 2 {
		t.Fatalf("assignment=%#v ok=%v err=%v", assignment, ok, err)
	}
	if assignment.Endpoints[0].Port == nil || assignment.Endpoints[0].DisplayAddress != "play.asia.example:30000" || assignment.Endpoints[0].Stability != "stable" {
		t.Fatalf("port endpoint=%#v", assignment.Endpoints[0])
	}
	if assignment.Endpoints[1].Port != nil || assignment.Endpoints[1].DisplayAddress != "203.0.113.10" || assignment.Endpoints[1].Stability != "stable" {
		t.Fatalf("ip-only endpoint=%#v", assignment.Endpoints[1])
	}
	if _, ok, err := regional.ClaimAssignment(context.Background(), "node_one", "node-agent-two", deliveryNow.Add(35*time.Second)); err != nil || ok {
		t.Fatalf("unexpired assignment stolen=%v err=%v", ok, err)
	}
	assignment, ok, err = regional.ClaimAssignment(context.Background(), "node_one", "node-agent-two", deliveryNow.Add(65*time.Second))
	if err != nil || !ok || assignment.Attempt != 2 {
		t.Fatalf("reclaimed assignment=%#v ok=%v err=%v", assignment, ok, err)
	}
	if _, err := regional.CompleteAssignment(context.Background(), assignment.RegionalDeliveryID, "node-agent-two", assignment.FencingToken-1, true, "", deliveryNow.Add(66*time.Second)); !errors.Is(err, regionaldelivery.ErrFencingToken) {
		t.Fatalf("stale fencing error=%v", err)
	}
	completed, err := regional.CompleteAssignment(context.Background(), assignment.RegionalDeliveryID, "node-agent-two", assignment.FencingToken, true, "", deliveryNow.Add(66*time.Second))
	if err != nil || !completed {
		t.Fatalf("completed=%v err=%v", completed, err)
	}
	if completed, err := regional.CompleteAssignment(context.Background(), assignment.RegionalDeliveryID, "node-agent-two", assignment.FencingToken, true, "", deliveryNow.Add(66*time.Second)); err != nil || completed {
		t.Fatalf("duplicate completion=%v err=%v", completed, err)
	}
	regional = regionaldelivery.NewPostgres(region, "reg_asia", authorityKey)
	if worked, err := regional.ReconcileOne(context.Background(), "publisher", deliveryNow.Add(67*time.Second)); err != nil || !worked {
		t.Fatalf("publish worked=%v err=%v", worked, err)
	}
	observation := loadObservation(t, region, instance.ID)
	applied, err := control.ApplyObservation(context.Background(), observation, deliveryNow.Add(68*time.Second))
	if err != nil || !applied {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
	if applied, err := control.ApplyObservation(context.Background(), observation, deliveryNow.Add(68*time.Second)); err != nil || applied {
		t.Fatalf("duplicate observation applied=%v err=%v", applied, err)
	}
	stored, err := control.Instance(context.Background(), "ws_ember", instance.ID)
	if err != nil || stored.ObservedState != "running" || len(stored.EndpointBindings) != 2 {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	finished, err := control.Operation(context.Background(), "ws_ember", operation.ID)
	if err != nil || finished.Status != "succeeded" || finished.Steps[len(finished.Steps)-1].Status != "succeeded" {
		t.Fatalf("operation=%#v err=%v", finished, err)
	}
	newer := observation
	newer.MessageID = "msg_observation_newer"
	newer.Sequence = observation.Sequence + 2
	newer.ObservedAt = newer.ObservedAt.Add(2 * time.Second)
	if applied, err := control.ApplyObservation(context.Background(), newer, deliveryNow.Add(69*time.Second)); err != nil || !applied {
		t.Fatalf("newer applied=%v err=%v", applied, err)
	}
	stale := observation
	stale.MessageID = "msg_observation_stale"
	stale.Sequence++
	if applied, err := control.ApplyObservation(context.Background(), stale, deliveryNow.Add(70*time.Second)); err != nil || !applied {
		t.Fatalf("stale envelope handling applied=%v err=%v", applied, err)
	}
	stored, _ = control.Instance(context.Background(), "ws_ember", instance.ID)
	if stored.ObservationSequence != newer.Sequence {
		t.Fatalf("observation sequence regressed to %d", stored.ObservationSequence)
	}
}

func TestFailedEndpointStepAutomaticallyCleansResidualCapacity(t *testing.T) {
	region := openDeliveryDatabase(t, "GAMEPANEL_REGION_TEST_DSN", "region", []string{"0001_region_execution.sql", "0004_async_delivery.sql"})
	authorityKey := []byte("authority-key-012345678901234567")
	regional := regionaldelivery.NewPostgres(region, "reg_asia", authorityKey)
	if err := regional.RegisterNode(context.Background(), regionaldelivery.Node{ID: "node_one", RegionID: "reg_asia", State: "ready", CPUCapacityMilli: 2000, MemoryCapacityMiB: 4096, DiskCapacityGiB: 50, LeaseUntil: deliveryNow.Add(time.Hour), UpdatedAt: deliveryNow}); err != nil {
		t.Fatal(err)
	}
	start, end := 30000, 30010
	if err := regional.AddEndpointPool(context.Background(), regionaldelivery.EndpointPool{ID: "pool_gateway", RegionID: "reg_asia", DeliveryMode: "gateway", Address: "play.asia.example", PortStart: &start, PortEnd: &end, Stability: "stable", Active: true}); err != nil {
		t.Fatal(err)
	}
	desired := deliverycontrol.DesiredPayload{WorkspaceID: "ws_one", LogicalInstanceID: "lin_failure", RegionID: "reg_asia", PlacementVersion: 1, InstanceRevisionID: "rev_failure", OperationID: "op_failure", DesiredState: "running", ProviderReleaseID: "gpr_fake", ResourceSpec: billing.ResourceSpec{CPUMilli: 1000, MemoryMiB: 1024, DiskGiB: 10}, Configuration: map[string]any{}, ListenerRequirements: []deliverycontrol.ListenerRequirement{{Name: "game", Purpose: "join", Transports: []string{"udp"}, InternalPort: 7777, ExternalPortPolicy: "default-required", AddressMode: "ip-port", Primary: true}}}
	grant, err := deliverycontrol.NewAuthorityGrant(desired, authorityKey, deliveryNow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	desired.AuthorityGrant = grant
	if accepted, err := regional.ReceiveDesired(context.Background(), "msg_failure", desired, deliveryNow); err != nil || !accepted {
		t.Fatalf("accepted=%v err=%v", accepted, err)
	}
	if _, err := regional.ReconcileOne(context.Background(), "scheduler", deliveryNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := regional.ReconcileOne(context.Background(), "allocator", deliveryNow.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	state, err := regional.State(context.Background(), desired.LogicalInstanceID)
	if err != nil || state.Phase != regionaldelivery.PhaseFailed || !state.ResidualCleanupRequired {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	if _, err := regional.ReconcileOne(context.Background(), "cleaner", deliveryNow.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	state, _ = regional.State(context.Background(), desired.LogicalInstanceID)
	if state.Phase != regionaldelivery.PhaseCleaned || state.ResidualCleanupRequired {
		t.Fatalf("cleaned state=%#v", state)
	}
	var cpu, memory, disk int64
	if err := region.QueryRow(`SELECT reserved_cpu_milli,reserved_memory_mib,reserved_disk_gib FROM regional_delivery_nodes WHERE id='node_one'`).Scan(&cpu, &memory, &disk); err != nil || cpu != 0 || memory != 0 || disk != 0 {
		t.Fatalf("residual capacity cpu=%d memory=%d disk=%d err=%v", cpu, memory, disk, err)
	}
}

func authorizedQuote(t *testing.T, database *sql.DB, fundingKey []byte) billing.Quote {
	t.Helper()
	module := billing.New(billing.NewPostgresStore(database), fundingKey)
	book := billing.PriceBook{ID: "pb_asia_1", RegionID: "reg_asia", Revision: 1, Currency: billing.CurrencyCNY, EffectiveAt: deliveryNow.Add(-time.Hour), CreatedAt: deliveryNow, UnitPrices: []billing.UnitPrice{{ResourceKind: billing.ResourceCPU, PriceMinor: 10, UnitQuantity: 1000, Unit: "cpu-hour"}, {ResourceKind: billing.ResourceMemory, PriceMinor: 5, UnitQuantity: 1024, Unit: "gib-hour"}, {ResourceKind: billing.ResourceInstanceDisk, PriceMinor: 1, UnitQuantity: 1, Unit: "gib-hour"}, {ResourceKind: billing.ResourceBackupStorage, PriceMinor: 1, UnitQuantity: 1, Unit: "gib-hour"}, {ResourceKind: billing.ResourceDedicatedIP, PriceMinor: 3, UnitQuantity: 1, Unit: "address-hour"}}}
	catalog := billing.RegionCatalog{ID: "cat_asia_1", RegionID: "reg_asia", Version: 1, CPU: billing.Range{Minimum: 500, Maximum: 8000, Step: 500}, Memory: billing.Range{Minimum: 1024, Maximum: 16384, Step: 1024}, Disk: billing.Range{Minimum: 10, Maximum: 100, Step: 5}, DedicatedIPAvailable: true, EndpointDeliveryModes: []string{"gateway", "dedicated-ip"}, Availability: billing.AvailabilityAvailable, EffectiveAt: deliveryNow.Add(-time.Hour), CreatedAt: deliveryNow}
	if err := module.PublishCatalog(context.Background(), catalog); err != nil {
		t.Fatal(err)
	}
	if err := module.PublishPriceBook(context.Background(), book); err != nil {
		t.Fatal(err)
	}
	if _, err := module.GrantPromotionalCredit(context.Background(), "ws_ember", "phase4-credit", 100000, "delivery test"); err != nil {
		t.Fatal(err)
	}
	quote, err := module.CreateQuote(context.Background(), "ws_ember", "reg_asia", billing.ResourceSpec{CPUMilli: 1000, MemoryMiB: 1024, DiskGiB: 10}, false)
	if err != nil {
		t.Fatal(err)
	}
	quote, err = module.AuthorizeCreate(context.Background(), "ws_ember", quote.ID)
	if err != nil {
		t.Fatal(err)
	}
	return quote
}

func loadDesired(t *testing.T, database *sql.DB, instanceID string) deliverycontrol.DesiredPayload {
	t.Helper()
	var payload []byte
	if err := database.QueryRow(`SELECT payload FROM global_outbox WHERE message_type='deployment.desired.v1' AND payload->>'logicalInstanceId'=$1`, instanceID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var desired deliverycontrol.DesiredPayload
	if err := json.Unmarshal(payload, &desired); err != nil {
		t.Fatal(err)
	}
	return desired
}

func loadObservation(t *testing.T, database *sql.DB, instanceID string) deliverycontrol.Observation {
	t.Helper()
	var messageID string
	var payload []byte
	if err := database.QueryRow(`SELECT id,payload FROM regional_outbox WHERE message_type='deployment.observed.v1' AND payload->>'logicalInstanceId'=$1 ORDER BY created_at DESC LIMIT 1`, instanceID).Scan(&messageID, &payload); err != nil {
		t.Fatal(err)
	}
	var observation deliverycontrol.Observation
	if err := json.Unmarshal(payload, &observation); err != nil {
		t.Fatal(err)
	}
	observation.MessageID = messageID
	return observation
}

func openDeliveryDatabase(t *testing.T, environment, migrationGroup string, migrations []string) *sql.DB {
	t.Helper()
	dsn := os.Getenv(environment)
	if dsn == "" {
		t.Skip(environment + " is not set")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("phase4_%s_%d", migrationGroup, time.Now().UnixNano())
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
	database.SetMaxOpenConns(10)
	_, filename, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(filename), "..", "..", "migrations", migrationGroup)
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
	inDollarQuote := false
	for index := 0; index < len(source); index++ {
		if index+1 < len(source) && source[index:index+2] == "$$" {
			inDollarQuote = !inDollarQuote
			index++
			continue
		}
		if source[index] == ';' && !inDollarQuote {
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

var _ messaging.Outbox = (*messaging.PostgresOutbox)(nil)

type integrationPublisher struct{ failure error }

func (p integrationPublisher) Publish(context.Context, string, contract.EventID, []byte) error {
	return p.failure
}
