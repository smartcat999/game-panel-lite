package globalproduct

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/commerce"
	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/instancecontrol"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regiondirectory"
)

func TestFailedCheckoutLeavesNoPartialInstanceOrOrder(t *testing.T) {
	product, commerceModule, instances, _ := newTestProduct()
	command := validCommand("idem_failed")
	command.Name = ""

	if _, err := product.CreateCheckout(context.Background(), command, testNow()); !errors.Is(err, instancecontrol.ErrInvalidInstance) {
		t.Fatalf("CreateCheckout error = %v, want invalid instance", err)
	}
	if instances.Count() != 0 || commerceModule.CountOrders() != 0 {
		t.Fatalf("failed checkout left partial state: instances=%d orders=%d", instances.Count(), commerceModule.CountOrders())
	}
}

func TestCheckoutIdempotencyReturnsSameResult(t *testing.T) {
	product, commerceModule, instances, _ := newTestProduct()
	command := validCommand("idem_repeat")
	first, err := product.CreateCheckout(context.Background(), command, testNow())
	if err != nil {
		t.Fatal(err)
	}
	second, err := product.CreateCheckout(context.Background(), command, testNow().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.Instance.ID != second.Instance.ID || first.Order.ID != second.Order.ID {
		t.Fatalf("idempotent result changed: first=%#v second=%#v", first, second)
	}
	if instances.Count() != 1 || commerceModule.CountOrders() != 1 {
		t.Fatalf("repeated checkout duplicated state: instances=%d orders=%d", instances.Count(), commerceModule.CountOrders())
	}
}

func TestDeploymentAuthorityRequiresActiveEntitlement(t *testing.T) {
	product, _, _, messages := newTestProduct()
	checkout, err := product.CreateCheckout(context.Background(), validCommand("idem_authority"), testNow())
	if err != nil {
		t.Fatal(err)
	}
	if err := product.RequestDeployment(context.Background(), checkout.Instance.ID, "idem_deploy", testNow()); !errors.Is(err, ErrActiveEntitlementNeeded) {
		t.Fatalf("RequestDeployment error = %v, want active entitlement error", err)
	}
	if got := messages.Count("deployment.desired.v1"); got != 0 {
		t.Fatalf("unauthorized instance emitted %d deployment events", got)
	}
}

func TestVerifiedPaymentActivationIsIdempotentAndTransactional(t *testing.T) {
	product, _, _, messages := newTestProduct()
	checkout, err := product.CreateCheckout(context.Background(), validCommand("idem_payment"), testNow())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := product.ActivateVerifiedPayment(context.Background(), checkout.Order.ID, "provider_notice_1", false, testNow()); !errors.Is(err, commerce.ErrPaymentUnverified) {
		t.Fatalf("unverified payment error = %v", err)
	}
	if messages.Count("deployment.desired.v1") != 0 {
		t.Fatal("unverified payment emitted deployment authority")
	}
	first, err := product.ActivateVerifiedPayment(context.Background(), checkout.Order.ID, "provider_notice_1", true, testNow())
	if err != nil {
		t.Fatal(err)
	}
	second, err := product.ActivateVerifiedPayment(context.Background(), checkout.Order.ID, "provider_notice_1", true, testNow().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if first.Payment.ID != second.Payment.ID || first.Entitlement.ID != second.Entitlement.ID {
		t.Fatalf("payment redelivery changed result: first=%#v second=%#v", first, second)
	}
	if messages.Count("entitlement.changed.v1") != 1 || messages.Count("deployment.desired.v1") != 1 {
		t.Fatalf("payment redelivery duplicated outbox: %#v", messages.Outbox(context.Background()))
	}
	for _, message := range messages.Outbox(context.Background()) {
		if message.MessageType != "deployment.desired.v1" {
			continue
		}
		payload, ok := message.Payload.(map[string]any)
		if !ok || payload["gameVersion"] != "1.4.5.6" || payload["configuration"].(map[string]any)["maxPlayers"] != 8 {
			t.Fatalf("deployment event lost provider input: %#v", message.Payload)
		}
	}
}

func newTestProduct() (*Module, *commerce.Module, *instancecontrol.Module, *messaging.Module) {
	regionID := contract.RegionID("reg_test")
	regions := regiondirectory.New([]regiondirectory.Region{{ID: regionID, Code: "test", Name: "Test", Available: true}})
	commerceModule := commerce.New([]commerce.PlanVersion{{
		ID: "plv_standard_1", PlanID: "pln_standard", Version: 1, Name: "Standard", PriceMinor: 1200,
		Currency: "USD", BillingPeriod: "month", RegionIDs: []contract.RegionID{regionID}, MemoryMegabytes: 2048, CPUUnits: 1000,
	}})
	instances := instancecontrol.New()
	messages := messaging.New()
	return New(regions, commerceModule, instances, messages), commerceModule, instances, messages
}

func validCommand(key contract.IdempotencyKey) CreateCommand {
	return CreateCommand{
		Identity: contract.CommandIdentity{CommandID: "cmd_test", IdempotencyKey: key}, WorkspaceID: "ws_test",
		PlanVersionID: "plv_standard_1", RegionID: "reg_test", Name: "Terraria One", GameKey: "terraria",
		GameVersion: "1.4.5.6", Configuration: map[string]any{"maxPlayers": 8},
	}
}

func testNow() time.Time { return time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC) }
