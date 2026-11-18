package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	tenantcontroller "github.com/smartcat999/game-panel-cloud/pkg/controller/core/tenant"
	usercontroller "github.com/smartcat999/game-panel-cloud/pkg/controller/iam/user"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
)

func (h *securityTestHarness) doJSON(t *testing.T, method, token, path string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Marshal(%s %s): %v", method, path, err)
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, h.httpSrv.URL+path, r)
	if err != nil {
		t.Fatalf("NewRequest(%s %s): %v", method, path, err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll(%s %s): %v", method, path, err)
	}
	return resp.StatusCode, data
}

func (h *securityTestHarness) mustLogin(t *testing.T, identifier, password string) string {
	t.Helper()
	code, raw := h.doJSON(t, http.MethodPost, "", "/apis/iam.gamepanel.io/v1/sessions", map[string]any{
		"apiVersion": iamv1.GroupVersion,
		"kind":       iamv1.SubjectKindUser,
		"identifier": identifier,
		"password":   password,
	})
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("login %s failed: %d %s", identifier, code, raw)
	}
	var sess iamv1.Session
	if err := json.Unmarshal(raw, &sess); err != nil || sess.Token == "" {
		t.Fatalf("decode login %s session: err=%v body=%s", identifier, err, raw)
	}
	return sess.Token
}

// Exercise a personal and an organization workspace through the Lite API.
func TestLiteWorkspacesWithoutSubscription(t *testing.T) {
	ctx := context.Background()
	h := setupSecurityTestHarness(t)
	admin := h.mustLogin(t, "admin", "P@88w0rd")
	code, body := h.doJSON(t, http.MethodPost, admin, "/apis/iam.gamepanel.io/v1/users", &iamv1.User{
		Metadata: metav1.ObjectMeta{Name: "alice"}, Spec: iamv1.UserSpec{Email: "alice@example.com", DisplayName: "Alice", Password: "AlicePassword1!"}})
	if code != http.StatusCreated {
		t.Fatalf("create user: %d %s", code, body)
	}
	if err := (&usercontroller.Controller{}).Reconcile(ctx, h.reg, h.bus, nil, mq.Event{Name: "alice"}); err != nil {
		t.Fatal(err)
	}
	token := h.mustLogin(t, "alice", "AlicePassword1!")
	code, body = h.doJSON(t, http.MethodPost, token, "/apis/core.gamepanel.io/v1/tenants", &corev1.Tenant{Metadata: metav1.ObjectMeta{Name: "studio"}, Spec: corev1.TenantSpec{Type: corev1.TenantTypeOrganization, DisplayName: "Studio", Owner: "alice"}})
	if code != http.StatusCreated {
		t.Fatalf("create workspace: %d %s", code, body)
	}
	for _, name := range []string{"alice", "studio"} {
		if err := (&tenantcontroller.Controller{}).Reconcile(ctx, h.reg, h.bus, nil, mq.Event{Name: name}); err != nil {
			t.Fatal(err)
		}
		code, body := h.doGET(t, token, "/apis/core.gamepanel.io/v1/tenants/"+name)
		if code != http.StatusOK {
			t.Fatalf("GET workspace %s: %d %s", name, code, body)
		}
		var ws corev1.Tenant
		if err := json.Unmarshal(body, &ws); err != nil {
			t.Fatal(err)
		}
		if ws.Metadata.Name != name || ws.Spec.Owner != "alice" || ws.Status.Phase != corev1.TenantPhaseActive || ws.Spec.BillingMode != corev1.TenantBillingModeManual || ws.Status.Subscription != nil {
			t.Fatalf("unexpected workspace: %+v", ws)
		}
		for _, dim := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory, corev1.ResourceStorage, corev1.ResourceInstances, corev1.ResourceBackups, corev1.ResourceBackupStorage} {
			if !ws.Spec.IsQuotaUnlimited(dim) {
				t.Errorf("%s: expected unlimited %s", name, dim)
			}
		}
	}
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, "resourceplans"}, {http.MethodGet, "redeemcodes"},
		{http.MethodPost, "tenants/studio/orders"}, {http.MethodGet, "tenants/studio/orders"},
		{http.MethodPost, "tenants/studio/redeem"}, {http.MethodPost, "tenants/studio/reclaim"},
		{http.MethodGet, "tenants/studio/quotachanges"}, {http.MethodGet, "tenants/studio/quotachanges?verify=true"},
	} {
		code, body := h.doJSON(t, endpoint.method, admin, "/apis/core.gamepanel.io/v1/"+endpoint.path, nil)
		if code != http.StatusNotFound {
			t.Errorf("%s %s: got %d, want 404: %s", endpoint.method, endpoint.path, code, body)
		}
	}
}
