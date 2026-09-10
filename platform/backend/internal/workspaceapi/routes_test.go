package workspaceapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutesRegistersColonActions(t *testing.T) {
	if Routes(Services{}) == nil {
		t.Fatal("expected routes")
	}
}

func TestColonActionExtractsResourceID(t *testing.T) {
	called := false
	handler := colonAction("resourceAction", "resourceId", map[string]http.Handler{
		"start": http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			called = true
			if got := request.PathValue("resourceId"); got != "instance-1" {
				t.Fatalf("resource id = %q", got)
			}
			response.WriteHeader(http.StatusAccepted)
		}),
	})
	request := httptest.NewRequest(http.MethodPost, "/instances/instance-1:start", nil)
	request.SetPathValue("resourceAction", "instance-1:start")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if !called || response.Code != http.StatusAccepted {
		t.Fatalf("called=%t status=%d", called, response.Code)
	}
}

func TestColonActionRejectsUnsupportedAction(t *testing.T) {
	handler := colonAction("resourceAction", "resourceId", map[string]http.Handler{"start": http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})})
	request := httptest.NewRequest(http.MethodPost, "/instances/instance-1:delete", nil)
	request.SetPathValue("resourceAction", "instance-1:delete")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d", response.Code)
	}
}
