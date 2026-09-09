package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/commerce"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

func TestCommerceHTTPFlow(t *testing.T) {
	router, db, _ := newTestRouter(t)
	ctx := context.Background()

	// 1. Initial plans listing is empty
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/commerce/plans", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var plans []commerce.PlanVersion
	if err := json.Unmarshal(rec.Body.Bytes(), &plans); err != nil || len(plans) != 0 {
		t.Fatalf("expected empty plans, got: %v", rec.Body.String())
	}

	// 2. Publish a plan as admin
	admin := domain.AdminAccount{ID: "commerce-admin", Username: "commerce-admin", Role: domain.RoleAdmin, PasswordHash: "test"}
	if err := db.CreateAdminAccount(ctx, &admin); err != nil {
		t.Fatal(err)
	}
	plan := commerce.PlanVersion{
		PlanID:          "terraria-starter",
		Version:         1,
		ProviderKey:     "test",
		RegionID:        "default-region",
		CPU:             1,
		MemoryMB:        2048,
		StorageBytes:    1024 * 1024 * 1024,
		Currency:        "CNY",
		UnitAmountMinor: 1000,
		PeriodSeconds:   86400 * 30,
	}
	if err := db.PublishPrepaidPlan(ctx, admin.ID, plan); err != nil {
		t.Fatalf("publish plan failed: %v", err)
	}
	if err := db.SetPrepaidPlanSale(ctx, admin.ID, plan.PlanID, 1, 1, true); err != nil {
		t.Fatalf("enable plan sale failed: %v", err)
	}

	// Verify plans endpoint now returns the plan
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/commerce/plans", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &plans); err != nil || len(plans) != 1 || plans[0].PlanID != "terraria-starter" {
		t.Fatalf("expected 1 plan, got: %v", rec.Body.String())
	}

	// 3. User registers and sets up server
	if err := db.SetSetting(ctx, domain.SettingKeyAllowRegistration, "true"); err != nil {
		t.Fatal(err)
	}
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"gamer1","password":"password123"}`)))
	if regRec.Code != http.StatusCreated {
		t.Fatalf("register failed: %d %s", regRec.Code, regRec.Body.String())
	}
	cookie := authCookieFromRecorder(t, regRec)

	user, err := db.GetAdminAccountByUsername(ctx, "gamer1")
	if err != nil {
		t.Fatal(err)
	}
	orgs, err := db.ListUserOrganizations(ctx, user.ID)
	if err != nil || len(orgs) == 0 {
		t.Fatalf("expected user organization: %v", err)
	}
	orgID := orgs[0].ID

	// Create a logical server for the user
	createReq := instances.CreateRequest{
		OrganizationID: orgID,
		Name:           "my-server",
		RegionID:       "default-region",
		IdempotencyKey: "create-key-1",
		Specification: instances.Specification{
			ProviderKey:         "test",
			GameVersion:         "1.4.4.9",
			ConfigSchemaVersion: 1,
			Configuration: instances.ProtectedConfiguration{
				KeyID:      "test-key",
				Ciphertext: []byte("test-ciphertext"),
			},
			Resources: instances.Resources{
				CPU:      1,
				MemoryMB: 2048,
			},
		},
	}
	res, err := db.CreateGlobalServer(ctx, user.ID, createReq)
	if err != nil {
		t.Fatalf("create server failed: %v", err)
	}
	serverID := res.Server.ID

	// 4. Create an order via HTTP
	orderReq := createOrderRequest{
		OrganizationID: orgID,
		ServerID:       serverID,
		PlanID:         "terraria-starter",
		PlanVersion:    1,
		Periods:        1,
		IdempotencyKey: "order-test-1",
	}
	reqBytes, _ := json.Marshal(orderReq)
	orderHttpReq := httptest.NewRequest(http.MethodPost, "/api/commerce/orders", bytes.NewReader(reqBytes))
	orderHttpReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, orderHttpReq)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}
	var createdOrder commerce.Order
	if err := json.Unmarshal(rec.Body.Bytes(), &createdOrder); err != nil || createdOrder.ID == "" {
		t.Fatalf("failed to decode order: %v", rec.Body.String())
	}

	// Cancel the order
	cancelHttpReq := httptest.NewRequest(http.MethodPost, "/api/commerce/orders/"+createdOrder.ID+"/cancel", strings.NewReader(`{"reason":"changed mind"}`))
	cancelHttpReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, cancelHttpReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK cancel, got %d: %s", rec.Code, rec.Body.String())
	}

	// 5. Create a second order and apply a capture from a trusted test adapter.
	orderReq.IdempotencyKey = "order-test-2"
	reqBytes, _ = json.Marshal(orderReq)
	orderHttpReq = httptest.NewRequest(http.MethodPost, "/api/commerce/orders", bytes.NewReader(reqBytes))
	orderHttpReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, orderHttpReq)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}
	var paidOrder commerce.Order
	_ = json.Unmarshal(rec.Body.Bytes(), &paidOrder)

	capture := commerce.CapturedPayment{
		Provider:      "wechat",
		MerchantID:    "mch_123",
		TransactionID: "wx_tx_999",
		EventID:       "evt_999",
		OrderID:       paidOrder.ID,
		AmountMinor:   paidOrder.Quote.AmountMinor,
		Currency:      "CNY",
		PaidAtMS:      time.Now().UnixMilli(),
	}
	if receipt, err := db.RecordCapturedPayment(ctx, capture); err != nil || receipt.Disposition != "applied" {
		t.Fatalf("record trusted test capture: %+v %v", receipt, err)
	}
	if _, _, err := db.FulfillPrepaidSubscription(ctx, paidOrder.ID); err != nil {
		t.Fatalf("fulfill paid subscription: %v", err)
	}

	// 6. Verify subscription is active via GET /api/commerce/subscriptions
	subHttpReq := httptest.NewRequest(http.MethodGet, "/api/commerce/subscriptions?organizationId="+orgID, nil)
	subHttpReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, subHttpReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for subscriptions, got %d: %s", rec.Code, rec.Body.String())
	}
	var subs []commerce.Subscription
	if err := json.Unmarshal(rec.Body.Bytes(), &subs); err != nil || len(subs) != 1 || subs[0].Status != "active" {
		t.Fatalf("expected 1 active subscription, got %v (%s)", subs, rec.Body.String())
	}

	// 7. Verify GET /api/operations/{id}
	opHttpReq := httptest.NewRequest(http.MethodGet, "/api/operations/"+res.Operation.ID, nil)
	opHttpReq.AddCookie(cookie)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, opHttpReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for operation, got %d: %s", rec.Code, rec.Body.String())
	}
	var op instances.Operation
	if err := json.Unmarshal(rec.Body.Bytes(), &op); err != nil || op.ID != res.Operation.ID {
		t.Fatalf("expected operation %s, got: %v", res.Operation.ID, rec.Body.String())
	}
}

func TestCommerceEndToEndPrepaidLifecycle(t *testing.T) {
	router, db, _ := newTestRouter(t)
	ctx := context.Background()

	// 1. Seed catalog plans
	if err := db.SeedDefaultPrepaidPlans(ctx); err != nil {
		t.Fatalf("seed catalog failed: %v", err)
	}

	// 2. Allow registration & register user
	if err := db.SetSetting(ctx, domain.SettingKeyAllowRegistration, "true"); err != nil {
		t.Fatal(err)
	}
	regRec := httptest.NewRecorder()
	router.ServeHTTP(regRec, httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"saas_buyer","password":"password123"}`)))
	if regRec.Code != http.StatusCreated {
		t.Fatalf("register failed: %d %s", regRec.Code, regRec.Body.String())
	}
	cookie := authCookieFromRecorder(t, regRec)

	user, err := db.GetAdminAccountByUsername(ctx, "saas_buyer")
	if err != nil {
		t.Fatal(err)
	}
	orgs, err := db.ListUserOrganizations(ctx, user.ID)
	if err != nil || len(orgs) == 0 {
		t.Fatalf("expected user organization: %v", err)
	}
	orgID := orgs[0].ID

	// 3. Query available plans via GET /api/commerce/plans
	planRec := httptest.NewRecorder()
	router.ServeHTTP(planRec, httptest.NewRequest(http.MethodGet, "/api/commerce/plans?providerKey=terraria-vanilla", nil))
	if planRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", planRec.Code)
	}
	var plans []commerce.PlanVersion
	if err := json.Unmarshal(planRec.Body.Bytes(), &plans); err != nil || len(plans) == 0 {
		t.Fatalf("expected plans, got: %s", planRec.Body.String())
	}
	selectedPlan := plans[0]

	// 4. Create server with prepaidPlanId
	srvBody := fmt.Sprintf(`{
		"name": "Terraria-SaaS-Server",
		"game": "terraria",
		"providerKey": "terraria-vanilla",
		"organizationId": "%s",
		"prepaidPlanId": "%s",
		"serverPort": 7777
	}`, orgID, selectedPlan.PlanID)
	createSrvReq := httptest.NewRequest(http.MethodPost, "/api/servers", strings.NewReader(srvBody))
	createSrvReq.AddCookie(cookie)
	srvRec := httptest.NewRecorder()
	router.ServeHTTP(srvRec, createSrvReq)
	if srvRec.Code != http.StatusCreated {
		t.Fatalf("create server failed: %d %s", srvRec.Code, srvRec.Body.String())
	}
	var createdSrv domain.GameServer
	if err := json.Unmarshal(srvRec.Body.Bytes(), &createdSrv); err != nil || createdSrv.ID == "" {
		t.Fatalf("failed to decode created server: %s", srvRec.Body.String())
	}

	// 5. Create order for the server
	orderReq := createOrderRequest{
		OrganizationID: orgID,
		ServerID:       createdSrv.ID,
		PlanID:         selectedPlan.PlanID,
		PlanVersion:    selectedPlan.Version,
		Periods:        1,
		IdempotencyKey: "test-lifecycle-order-1",
	}
	reqBytes, _ := json.Marshal(orderReq)
	orderHttpReq := httptest.NewRequest(http.MethodPost, "/api/commerce/orders", bytes.NewReader(reqBytes))
	orderHttpReq.AddCookie(cookie)
	orderRec := httptest.NewRecorder()
	router.ServeHTTP(orderRec, orderHttpReq)
	if orderRec.Code != http.StatusCreated {
		t.Fatalf("create order failed: %d %s", orderRec.Code, orderRec.Body.String())
	}
	var order commerce.Order
	if err := json.Unmarshal(orderRec.Body.Bytes(), &order); err != nil || order.ID == "" {
		t.Fatalf("failed to decode order: %s", orderRec.Body.String())
	}

	// 6. Apply a capture from a trusted test adapter and run fulfillment.
	capture := commerce.CapturedPayment{
		Provider:      "wechat",
		MerchantID:    "mch_test",
		TransactionID: "wx_tx_lifecycle_1",
		EventID:       "evt_lifecycle_1",
		OrderID:       order.ID,
		AmountMinor:   order.Quote.AmountMinor,
		Currency:      order.Quote.Plan.Currency,
		PaidAtMS:      time.Now().UnixMilli(),
	}
	if receipt, err := db.RecordCapturedPayment(ctx, capture); err != nil || receipt.Disposition != "applied" {
		t.Fatalf("record trusted test capture: %+v %v", receipt, err)
	}
	if _, _, err := db.FulfillPrepaidSubscription(ctx, order.ID); err != nil {
		t.Fatalf("fulfill paid subscription: %v", err)
	}

	// 7. Verify subscription is active
	subReq := httptest.NewRequest(http.MethodGet, "/api/commerce/subscriptions?organizationId="+orgID, nil)
	subReq.AddCookie(cookie)
	subRec := httptest.NewRecorder()
	router.ServeHTTP(subRec, subReq)
	if subRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", subRec.Code)
	}
	var subs []commerce.Subscription
	if err := json.Unmarshal(subRec.Body.Bytes(), &subs); err != nil || len(subs) == 0 {
		t.Fatalf("expected active subscription, got: %s", subRec.Body.String())
	}
	found := false
	for _, s := range subs {
		if s.ServerID == createdSrv.ID && s.Status == "active" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("subscription for server %s not found active in %v", createdSrv.ID, subs)
	}
}
