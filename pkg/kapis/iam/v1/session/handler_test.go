package session

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/authorization"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
)

type memSessionCache struct {
	mu   sync.RWMutex
	data map[string]string
}

func newMemSessionCache() *memSessionCache {
	return &memSessionCache{data: make(map[string]string)}
}

func (m *memSessionCache) Get(_ context.Context, key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return "", cache.ErrNotFound
	}
	return v, nil
}

func (m *memSessionCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *memSessionCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *memSessionCache) Exists(_ context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[key]
	return ok, nil
}

func (m *memSessionCache) Ping(context.Context) error { return nil }
func (m *memSessionCache) Close() error               { return nil }

func newTestSessionServer(t *testing.T) (sessionmodel.Operator, cache.Interface, http.Handler) {
	t.Helper()
	mem := newMemSessionCache()
	op := sessionmodel.NewOperator(nil, mem, &config.AuthConfig{
		JWTSecret: "unit-test-secret",
		TokenTTL:  time.Hour,
	})
	c := apiruntime.NewContainer(nil)
	AddToContainer(apiruntime.Dependencies{Sessions: op}, c)
	return op, mem, c.RestfulContainer()
}

func issueBrowserSession(t *testing.T, op sessionmodel.Operator, mem cache.Interface, user string) *iamv1.Session {
	t.Helper()
	ctx := context.Background()
	sess, err := op.Issue(ctx, iamv1.SubjectKindUser, user, user)
	if err != nil {
		t.Fatalf("Issue(%s): %v", user, err)
	}
	sess.Metadata.Annotations = map[string]string{AnnotationSessionUserAgent: "Mozilla/5.0 Chrome/130.0"}
	raw, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("Marshal(%s): %v", user, err)
	}
	if err := mem.Set(ctx, "gp:iam:session:"+sess.Metadata.Name, string(raw), time.Hour); err != nil {
		t.Fatalf("Set(%s): %v", user, err)
	}
	return sess
}

func serveWithUser(h http.Handler, method, target string, u request.UserInfo) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if !u.IsAnonymous() {
		req = req.WithContext(request.WithUser(req.Context(), u))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func assertPathDecision(t *testing.T, authz *authorization.PathAuthorizer, method, rawURL string, u request.UserInfo, want authorization.Decision) {
	t.Helper()
	req := httptest.NewRequest(method, rawURL, nil)
	info, err := request.NewRequestInfoResolver().NewRequestInfo(req)
	if err != nil {
		t.Fatalf("NewRequestInfo(%s %s): %v", method, rawURL, err)
	}
	dec, _, err := authz.Authorize(context.Background(), authorization.AttributesRecord{
		User:              u,
		IsResourceRequest: info.IsResourceRequest,
		Path:              info.Path,
		Verb:              info.Verb,
		APIGroup:          info.APIGroup,
		APIVersion:        info.APIVersion,
		Resource:          info.Resource,
		Subresource:       info.Subresource,
		Name:              info.Name,
	})
	if err != nil {
		t.Fatalf("Authorize(%s %s): %v", method, rawURL, err)
	}
	if dec != want {
		t.Fatalf("Authorize(%s %s) = %v, want %v", method, rawURL, dec, want)
	}
}

func TestGetSessionByNameAndRejectCurrentAlias(t *testing.T) {
	ctx := context.Background()
	op, _, h := newTestSessionServer(t)
	s1, err := op.Issue(ctx, iamv1.SubjectKindUser, "alice", "alice")
	if err != nil {
		t.Fatalf("Issue s1: %v", err)
	}
	alice := request.UserInfo{SessionID: s1.Metadata.Name, Kind: iamv1.SubjectKindUser, Name: "alice", Groups: []string{request.GroupAuthenticated}}

	authz := authorization.NewPathAuthorizer()
	assertPathDecision(t, authz, http.MethodGet, "/apis/iam.gamepanel.io/v1/sessions/"+s1.Metadata.Name, alice, authorization.DecisionAllow)
	assertPathDecision(t, authz, http.MethodGet, "/apis/iam.gamepanel.io/v1/sessions/current", alice, authorization.DecisionNoOpinion)
	assertPathDecision(t, authz, http.MethodDelete, "/apis/iam.gamepanel.io/v1/sessions/current", alice, authorization.DecisionNoOpinion)
	assertPathDecision(t, authz, http.MethodDelete, "/apis/iam.gamepanel.io/v1/sessions/others", alice, authorization.DecisionNoOpinion)
	assertPathDecision(t, authz, http.MethodGet, "/apis/iam.gamepanel.io/v1/sessions/alice-bob-0123456789abcdef", alice, authorization.DecisionNoOpinion)

	rec := serveWithUser(h, http.MethodGet, "/apis/iam.gamepanel.io/v1/sessions/"+s1.Metadata.Name, alice)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET own session = %d: %s", rec.Code, rec.Body.String())
	}
	var got iamv1.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if got.Metadata.Name != s1.Metadata.Name || got.Metadata.Annotations[AnnotationSessionCurrent] != "true" {
		t.Fatalf("unexpected session response: %+v", got)
	}

	admin := request.UserInfo{Kind: iamv1.SubjectKindUser, Name: "admin", Groups: []string{request.GroupAuthenticated}}
	recCurrent := serveWithUser(h, http.MethodGet, "/apis/iam.gamepanel.io/v1/sessions/current", admin)
	if recCurrent.Code != http.StatusNotFound {
		t.Fatalf("GET /sessions/current = %d, want 404", recCurrent.Code)
	}
}

func TestCrossUserSessionAccessForbidden(t *testing.T) {
	ctx := context.Background()
	op, _, h := newTestSessionServer(t)
	sAlice, err := op.Issue(ctx, iamv1.SubjectKindUser, "alice", "alice")
	if err != nil {
		t.Fatalf("Issue alice: %v", err)
	}
	sBob, err := op.Issue(ctx, iamv1.SubjectKindUser, "bob", "bob")
	if err != nil {
		t.Fatalf("Issue bob: %v", err)
	}
	alice := request.UserInfo{SessionID: sAlice.Metadata.Name, Kind: iamv1.SubjectKindUser, Name: "alice", Groups: []string{request.GroupAuthenticated}}

	if rec := serveWithUser(h, http.MethodGet, "/apis/iam.gamepanel.io/v1/sessions/"+sBob.Metadata.Name, alice); rec.Code != http.StatusForbidden {
		t.Fatalf("GET bob session by alice = %d, want 403", rec.Code)
	}
	if rec := serveWithUser(h, http.MethodDelete, "/apis/iam.gamepanel.io/v1/sessions/"+sBob.Metadata.Name, alice); rec.Code != http.StatusForbidden {
		t.Fatalf("DELETE bob session by alice = %d, want 403", rec.Code)
	}
}

func TestDeleteSessionCollectionAndSingle(t *testing.T) {
	ctx := context.Background()
	op, _, h := newTestSessionServer(t)
	s1, err := op.Issue(ctx, iamv1.SubjectKindUser, "alice", "alice")
	if err != nil {
		t.Fatalf("Issue s1: %v", err)
	}
	s2, err := op.Issue(ctx, iamv1.SubjectKindUser, "alice", "alice")
	if err != nil {
		t.Fatalf("Issue s2: %v", err)
	}
	alice := request.UserInfo{SessionID: s1.Metadata.Name, Kind: iamv1.SubjectKindUser, Name: "alice", Groups: []string{request.GroupAuthenticated}}

	delOthersURL := "/apis/iam.gamepanel.io/v1/sessions?fieldSelector=" + url.QueryEscape("metadata.name!="+s1.Metadata.Name)
	authz := authorization.NewPathAuthorizer()
	assertPathDecision(t, authz, http.MethodDelete, delOthersURL, alice, authorization.DecisionAllow)

	if rec := serveWithUser(h, http.MethodDelete, delOthersURL, alice); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE collection = %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := op.Verify(ctx, s2.Token); err == nil {
		t.Fatal("expected s2 to be revoked after DeleteCollection")
	}
	if _, err := op.Verify(ctx, s1.Token); err != nil {
		t.Fatalf("expected s1 to remain active, got %v", err)
	}

	if rec := serveWithUser(h, http.MethodDelete, "/apis/iam.gamepanel.io/v1/sessions/"+s1.Metadata.Name, alice); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE own session = %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := op.Verify(ctx, s1.Token); err == nil {
		t.Fatal("expected s1 to be revoked after single DELETE")
	}
}

func TestListSessionsFieldSelector(t *testing.T) {
	op, mem, h := newTestSessionServer(t)
	s1 := issueBrowserSession(t, op, mem, "alice")
	s2 := issueBrowserSession(t, op, mem, "alice")
	alice := request.UserInfo{SessionID: s1.Metadata.Name, Kind: iamv1.SubjectKindUser, Name: "alice", Groups: []string{request.GroupAuthenticated}}

	rec := serveWithUser(h, http.MethodGet, "/apis/iam.gamepanel.io/v1/sessions?fieldSelector="+url.QueryEscape("metadata.name!="+s1.Metadata.Name), alice)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET sessions with fieldSelector = %d: %s", rec.Code, rec.Body.String())
	}
	var list metav1.List[*iamv1.Session]
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Metadata.Name != s2.Metadata.Name {
		t.Fatalf("expected only s2 (%s), got %+v", s2.Metadata.Name, list.Items)
	}
}

func issueNodeSessionViaHTTP(t *testing.T, h http.Handler, u request.UserInfo, nodeName string) iamv1.Session {
	t.Helper()
	body, _ := json.Marshal(iamv1.Session{Kind: iamv1.SubjectKindNode, Identifier: nodeName})
	req := httptest.NewRequest(http.MethodPost, "/apis/iam.gamepanel.io/v1/sessions", bytes.NewReader(body))
	req = req.WithContext(request.WithUser(req.Context(), u))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /sessions(Node) = %d: %s", rec.Code, rec.Body.String())
	}
	var out iamv1.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode node session: %v", err)
	}
	return out
}

func TestNodeTokenOneTimeStorageRotationAndRenewal(t *testing.T) {
	ctx := context.Background()
	mem := newMemSessionCache()
	op := sessionmodel.NewOperator(nil, mem, &config.AuthConfig{JWTSecret: "unit-test-secret", TokenTTL: time.Hour})
	c := apiruntime.NewContainer(nil)
	canI := func(_ context.Context, group, resource, verb, _ string) bool {
		return group == corev1.GroupName && resource == "nodes" && verb == iamv1.VerbCreate
	}
	AddToContainer(apiruntime.Dependencies{Sessions: op, CanI: canI}, c)
	h := c.RestfulContainer()

	admin := request.UserInfo{Kind: iamv1.SubjectKindUser, Name: "admin", Groups: []string{request.GroupAuthenticated}}
	nodeUser := request.UserInfo{Kind: iamv1.SubjectKindNode, Name: "node-1", Groups: []string{"system:nodes"}}

	s1 := issueNodeSessionViaHTTP(t, h, admin, "node-1")
	if raw, err := mem.Get(ctx, "gp:iam:session:node-node-1"); err != nil || strings.Contains(raw, s1.Token) {
		t.Fatalf("cached node session leaked raw token or missing: err=%v", err)
	}
	if _, err := op.Verify(ctx, s1.Token); err != nil {
		t.Fatalf("expected t1 to verify: %v", err)
	}

	s2 := issueNodeSessionViaHTTP(t, h, nodeUser, "node-1")
	if s2.Token == "" || s2.Token == s1.Token {
		t.Fatalf("expected node self-renewal to mint fresh token, got identical/empty")
	}
	if _, err := op.Verify(ctx, s1.Token); err != nil {
		t.Fatalf("expected t1 to remain valid during smooth node renewal: %v", err)
	}
	if _, err := op.Verify(ctx, s2.Token); err != nil {
		t.Fatalf("expected t2 to verify: %v", err)
	}

	s3 := issueNodeSessionViaHTTP(t, h, admin, "node-1")
	if _, err := op.Verify(ctx, s1.Token); err == nil {
		t.Fatal("expected t1 to be revoked after admin rotation")
	}
	if _, err := op.Verify(ctx, s2.Token); err == nil {
		t.Fatal("expected t2 to be revoked after admin rotation")
	}
	if _, err := op.Verify(ctx, s3.Token); err != nil {
		t.Fatalf("expected t3 to verify after admin rotation: %v", err)
	}
}
