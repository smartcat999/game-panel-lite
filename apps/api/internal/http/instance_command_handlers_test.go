package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/config"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instanceapp"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

type instanceCommandFixture struct{ command instanceapp.CreateCommand }

func (f *instanceCommandFixture) Create(_ context.Context, actor string, command instanceapp.CreateCommand) (instances.IntentResult, error) {
	if actor != "tenant-owner" {
		return instances.IntentResult{}, instanceapp.ErrInvalidCreateCommand
	}
	f.command = command
	return instances.IntentResult{
		Server:    instances.Server{ID: "logical-instance", SpecGeneration: 1, IntentVersion: 1},
		Revision:  instances.Revision{ID: "revision"},
		Placement: instances.Placement{RegionID: "east"},
		Operation: instances.Operation{ID: "operation", Status: "pending"},
	}, nil
}

func TestCreateTenantInstanceReturnsAsyncIdentityWithoutConfiguration(t *testing.T) {
	commands := &instanceCommandFixture{}
	handler := &Handler{cfg: config.Config{LogicalConfigMaxBytes: 1024}, instanceCommands: commands}
	body := `{"organizationId":"tenant","name":"server","planId":"starter","planVersion":2,"configuration":{"password":"private"}}`
	request := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(body))
	request.Header.Set("Idempotency-Key", "request-1")
	request = request.WithContext(context.WithValue(request.Context(), authAccountContextKey, domain.AdminAccount{ID: "tenant-owner"}))
	response := httptest.NewRecorder()
	handler.createTenantInstance(response, request)
	if response.Code != http.StatusAccepted || strings.Contains(response.Body.String(), "private") {
		t.Fatalf("create response: %d %s", response.Code, response.Body.String())
	}
	if commands.command.OrganizationID != "tenant" || commands.command.PlanID != "starter" || commands.command.PlanVersion != 2 || commands.command.IdempotencyKey != "request-1" || string(commands.command.Configuration) != `{"password":"private"}` {
		t.Fatalf("command mapping: %+v", commands.command)
	}
	bad := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(`{"organizationId":"tenant","name":"server","planId":"starter","planVersion":2,"configuration":{},"nodeId":"node-a"}`))
	bad = bad.WithContext(request.Context())
	badResponse := httptest.NewRecorder()
	handler.createTenantInstance(badResponse, bad)
	if badResponse.Code != http.StatusBadRequest {
		t.Fatalf("infrastructure override accepted: %d %s", badResponse.Code, badResponse.Body.String())
	}
	conflict := httptest.NewRequest(http.MethodPost, "/api/instances", strings.NewReader(`{"organizationId":"tenant","name":"server","planId":"starter","planVersion":2,"idempotencyKey":"body-key","configuration":{}}`))
	conflict.Header.Set("Idempotency-Key", "header-key")
	conflict = conflict.WithContext(request.Context())
	conflictResponse := httptest.NewRecorder()
	handler.createTenantInstance(conflictResponse, conflict)
	if conflictResponse.Code != http.StatusBadRequest {
		t.Fatalf("conflicting idempotency keys accepted: %d %s", conflictResponse.Code, conflictResponse.Body.String())
	}
}
