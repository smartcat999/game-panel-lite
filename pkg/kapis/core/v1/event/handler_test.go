package event

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
)

type stubEventOperator struct {
	lastOpts db.ListOptions
}

func (s *stubEventOperator) List(_ context.Context, opts db.ListOptions) (*metav1.List[*corev1.Event], error) {
	s.lastOpts = opts
	return &metav1.List[*corev1.Event]{Items: []*corev1.Event{}}, nil
}

func TestListEventsParsesFilterQueryParams(t *testing.T) {
	stub := &stubEventOperator{}
	h := newHandler(stub)

	req := httptest.NewRequest(http.MethodGet, "/apis/core.gamepanel.io/v1/events?tenant=all&window=24h&resource=instances&verb=create&actor=alice", nil)
	rec := httptest.NewRecorder()
	h.ListClusterEvents(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.lastOpts.Tenant != "" {
		t.Fatalf("expected tenant=all to clear tenant filter, got %q", stub.lastOpts.Tenant)
	}

	fs := stub.lastOpts.FieldSelector
	if fs["resource"] != "instances" || fs["verb"] != "create" || fs["actor"] != "alice" {
		t.Fatalf("unexpected fieldSelector: %#v", fs)
	}
	sinceTime, err := time.Parse(time.RFC3339Nano, fs["since"])
	if err != nil {
		t.Fatalf("expected valid RFC3339Nano since, got %q (%v)", fs["since"], err)
	}
	diff := time.Since(sinceTime) - 24*time.Hour
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Fatalf("expected ~24h window cutoff, diff=%v", diff)
	}
}

func TestListTenantEventsExplicitSinceAndAllIgnored(t *testing.T) {
	stub := &stubEventOperator{}
	h := newHandler(stub)

	req := httptest.NewRequest(http.MethodGet, "/apis/core.gamepanel.io/v1/tenants/demo/events?window=all&resource=all&since=2026-11-15T08:00:00Z", nil)
	req.SetPathValue("tenant", "demo")
	rec := httptest.NewRecorder()
	h.ListTenantEvents(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if stub.lastOpts.Tenant != "demo" {
		t.Fatalf("expected tenant demo, got %q", stub.lastOpts.Tenant)
	}
	if _, exists := stub.lastOpts.FieldSelector["resource"]; exists {
		t.Fatalf("expected resource=all to be omitted, got %#v", stub.lastOpts.FieldSelector)
	}
	if stub.lastOpts.FieldSelector["since"] != "2026-11-15T08:00:00Z" {
		t.Fatalf("expected explicit since preserved, got %#v", stub.lastOpts.FieldSelector)
	}
}

func TestListEventsRejectsInvalidWindowOrSince(t *testing.T) {
	stub := &stubEventOperator{}
	h := newHandler(stub)

	for _, url := range []string{
		"/apis/core.gamepanel.io/v1/events?window=bad",
		"/apis/core.gamepanel.io/v1/events?window=0d",
		"/apis/core.gamepanel.io/v1/events?since=not-a-timestamp",
	} {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rec := httptest.NewRecorder()
		h.ListClusterEvents(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("url %s: expected 400 Bad Request, got %d (%s)", url, rec.Code, rec.Body.String())
		}
	}
}
