package filters

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
)

type stubAuditMQ struct {
	events []mq.Event
}

func (s *stubAuditMQ) Publish(_ context.Context, _ string, event mq.Event) error {
	s.events = append(s.events, event)
	return nil
}

func (s *stubAuditMQ) Subscribe(_ context.Context, _, _ string, _ mq.HandlerFunc) (mq.Subscription, error) {
	return nil, nil
}

func (s *stubAuditMQ) Ping(_ context.Context) error { return nil }
func (s *stubAuditMQ) Close() error                 { return nil }

func withTestAuditContext(req *http.Request, info *request.RequestInfo) *http.Request {
	ctx := request.WithRequestInfo(req.Context(), info)
	ctx = request.WithUser(ctx, request.UserInfo{Name: "admin", Kind: iamv1.SubjectKindUser})
	return req.WithContext(ctx)
}

func TestWithAuditLogLevelsAndRedaction(t *testing.T) {
	orig := config.Get()
	defer config.Set(orig)

	runCase := func(level string, wantReq, wantResp bool) {
		t.Helper()
		cfg := config.New()
		cfg.Observability.AuditLevel = level
		config.Set(cfg)

		bus := &stubAuditMQ{}
		var handlerSawPassword string
		h := WithAuditLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			handlerSawPassword, _ = payload["adminPassword"].(string)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"metadata":{"name":"palworld-68"},"secretKey":"real-secret"}`))
		}), bus, nil)

		req := withTestAuditContext(
			httptest.NewRequest(http.MethodPost, "/apis/core.gamepanel.io/v1/tenants/studio-alpha/instances", bytes.NewBufferString(`{"metadata":{"name":"palworld-68"},"adminPassword":"super-password"}`)),
			&request.RequestInfo{IsResourceRequest: true, Verb: iamv1.VerbCreate, APIGroup: corev1.GroupName, Resource: "instances", Tenant: "studio-alpha"},
		)
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(httptest.NewRecorder(), req)

		if handlerSawPassword != "super-password" {
			t.Fatalf("level=%s: downstream handler must still read intact body, got %q", level, handlerSawPassword)
		}
		if len(bus.events) != 1 {
			t.Fatalf("level=%s: expected 1 audit event, got %d", level, len(bus.events))
		}
		attrs := bus.events[0].Attributes
		if attrs["auditLevel"] != level {
			t.Fatalf("level=%s: expected auditLevel attr %q, got %q", level, level, attrs["auditLevel"])
		}
		if gotReq := attrs["requestBody"]; (gotReq != "") != wantReq {
			t.Fatalf("level=%s: requestBody presence=%v (%q), want %v", level, gotReq != "", gotReq, wantReq)
		} else if wantReq && (strings.Contains(gotReq, "super-password") || !strings.Contains(gotReq, auditRedactedValue)) {
			t.Fatalf("level=%s: expected adminPassword redacted in %q", level, gotReq)
		}
		if gotResp := attrs["responseBody"]; (gotResp != "") != wantResp {
			t.Fatalf("level=%s: responseBody presence=%v (%q), want %v", level, gotResp != "", gotResp, wantResp)
		} else if wantResp && (strings.Contains(gotResp, "real-secret") || !strings.Contains(gotResp, auditRedactedValue)) {
			t.Fatalf("level=%s: expected secretKey redacted in %q", level, gotResp)
		}
	}

	runCase(corev1.AuditLevelMetadata, false, false)
	runCase(corev1.AuditLevelRequest, true, false)
	runCase(corev1.AuditLevelRequestResponse, true, true)
}

func TestRedactSensitiveAuditURI(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{
			in:   "/apis/core.gamepanel.io/v1/tenants/studio-alpha/invites/eyJ0Ijoic3R1ZGlv.sig123",
			want: "/apis/core.gamepanel.io/v1/tenants/studio-alpha/invites/[REDACTED]",
		},
		{
			in:   "/apis/core.gamepanel.io/v1/tenants/studio-alpha/invites",
			want: "/apis/core.gamepanel.io/v1/tenants/studio-alpha/invites",
		},
		{
			in:   "/apis/core.gamepanel.io/v1/tenants/studio-alpha/instances/mc-01",
			want: "/apis/core.gamepanel.io/v1/tenants/studio-alpha/instances/mc-01",
		},
	}
	for _, tc := range cases {
		if got := redactSensitiveAuditURI(tc.in); got != tc.want {
			t.Fatalf("redactSensitiveAuditURI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveAuditIdentityAndTargetInviteRedaction(t *testing.T) {
	info := &request.RequestInfo{
		IsResourceRequest: true,
		Tenant:            "studio-alpha",
		Resource:          "invites",
		Name:              "eyJ0Ijoic3R1ZGlv.secret-sig",
		Verb:              "create",
	}
	anon := request.UserInfo{Name: request.AnonymousUser}

	// Failed unauthenticated attempt: never leaks raw invite token, falls back to workspace name.
	actor, tenant, resName, targetName := resolveAuditIdentityAndTarget(info, anon, &request.AuditInfo{
		Action:     "accept",
		TargetName: "studio-alpha",
	})
	if actor != "system:anonymous" || tenant != "studio-alpha" || resName != "studio-alpha" || targetName != "studio-alpha" {
		t.Fatalf("unexpected failed invite audit tuple: actor=%q tenant=%q res=%q target=%q", actor, tenant, resName, targetName)
	}

	// Successful invite acceptance: records authenticated/provisioned user and member binding without token leak.
	actor, tenant, resName, targetName = resolveAuditIdentityAndTarget(info, anon, &request.AuditInfo{
		Actor:        "dev",
		ResourceName: "member-dev",
		Action:       "accept",
		TargetName:   "dev",
	})
	if actor != "dev" || tenant != "studio-alpha" || resName != "member-dev" || targetName != "dev" {
		t.Fatalf("unexpected accepted invite audit tuple: actor=%q tenant=%q res=%q target=%q", actor, tenant, resName, targetName)
	}
}

func TestWithAuditLogCapturesFilesAndBodylessFallbacks(t *testing.T) {
	orig := config.Get()
	defer config.Set(orig)
	cfg := config.New()
	cfg.Observability.AuditLevel = corev1.AuditLevelRequestResponse
	config.Set(cfg)

	bus := &stubAuditMQ{}
	h := WithAuditLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		request.SetAuditTarget(r.Context(), "download", "palworld-68", map[string]string{"path": "Pal/Saved/Save.sav"})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ticket":"signed-secret-ticket","directURL":"https://node.local/dl?token=abc","size":128}`))
	}), bus, nil)

	delReq := withTestAuditContext(
		httptest.NewRequest(http.MethodDelete, "/apis/core.gamepanel.io/v1/tenants/studio-alpha/instances/terraria-88", nil),
		&request.RequestInfo{IsResourceRequest: true, Verb: iamv1.VerbDelete, APIGroup: corev1.GroupName, Resource: "instances", Name: "terraria-88", Tenant: "studio-alpha"},
	)
	h.ServeHTTP(httptest.NewRecorder(), delReq)

	filesReq := withTestAuditContext(
		httptest.NewRequest(http.MethodPost, "/apis/core.gamepanel.io/v1/tenants/studio-alpha/instances/palworld-68/files",
			bytes.NewBufferString(`{"action":"download","path":"Pal/Saved/Save.sav"}`)),
		&request.RequestInfo{IsResourceRequest: true, Verb: iamv1.VerbCreate, APIGroup: corev1.GroupName, Resource: "instances", Subresource: "files", Name: "palworld-68", Tenant: "studio-alpha"},
	)
	filesReq.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), filesReq)

	if len(bus.events) != 2 {
		t.Fatalf("expected 2 audit events, got %d", len(bus.events))
	}
	delAttrs := bus.events[0].Attributes
	if !strings.Contains(delAttrs["requestBody"], `"terraria-88"`) || !strings.Contains(delAttrs["responseBody"], `"statusCode":204`) {
		t.Fatalf("expected structured fallback payloads for DELETE, got req=%q resp=%q", delAttrs["requestBody"], delAttrs["responseBody"])
	}
	filesAttrs := bus.events[1].Attributes
	if !strings.Contains(filesAttrs["requestBody"], `"download"`) {
		t.Fatalf("expected files requestBody captured, got %q", filesAttrs["requestBody"])
	}
	if strings.Contains(filesAttrs["responseBody"], "signed-secret-ticket") || strings.Contains(filesAttrs["responseBody"], "https://node.local") {
		t.Fatalf("expected files ticket/directURL redacted in %q", filesAttrs["responseBody"])
	}
	if !strings.Contains(filesAttrs["responseBody"], auditRedactedValue) {
		t.Fatalf("expected redaction marker in files responseBody, got %q", filesAttrs["responseBody"])
	}
}
