package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
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
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	tenantmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/tenant"
	globalrolemodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/globalrole"
	globalrolebindingmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/globalrolebinding"
	sessionmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/session"
	tenantrolemodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrole"
	tenantrolebindingmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrolebinding"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
	"github.com/smartcat999/game-panel-cloud/tests/fixtures"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

type memTestMQ struct {
	mu     sync.RWMutex
	nextID int
	subs   map[string]map[int]mq.HandlerFunc
}

func newMemTestMQ() *memTestMQ {
	return &memTestMQ{subs: make(map[string]map[int]mq.HandlerFunc)}
}

func (m *memTestMQ) Publish(ctx context.Context, topic string, event mq.Event) error {
	m.mu.RLock()
	topicSubs := m.subs[topic]
	handlers := make([]mq.HandlerFunc, 0, len(topicSubs))
	for _, fn := range topicSubs {
		handlers = append(handlers, fn)
	}
	m.mu.RUnlock()
	for _, fn := range handlers {
		go func() { _ = fn(context.Background(), event) }()
	}
	return nil
}

func (m *memTestMQ) Subscribe(ctx context.Context, topic, _ string, handler mq.HandlerFunc) (mq.Subscription, error) {
	m.mu.Lock()
	if m.subs[topic] == nil {
		m.subs[topic] = make(map[int]mq.HandlerFunc)
	}
	id := m.nextID
	m.nextID++
	m.subs[topic][id] = handler
	m.mu.Unlock()
	unsub := func() error {
		m.mu.Lock()
		if topicSubs := m.subs[topic]; topicSubs != nil {
			delete(topicSubs, id)
		}
		m.mu.Unlock()
		return nil
	}
	if ctx != nil {
		context.AfterFunc(ctx, func() { _ = unsub() })
	}
	return memTestSub(unsub), nil
}

func (m *memTestMQ) Ping(context.Context) error { return nil }
func (m *memTestMQ) Close() error               { return nil }

type memTestSub func() error

func (s memTestSub) Unsubscribe() error { return s() }

type securityTestHarness struct {
	reg      *repo.Registry
	bus      *memTestMQ
	drivers  *runtime.Registry
	httpSrv  *httptest.Server
	sessions sessionmodel.Operator
	tenants  tenantmodel.Operator
	gr       globalrolemodel.Operator
	grb      globalrolebindingmodel.Operator
	tr       tenantrolemodel.Operator
	trb      tenantrolebindingmodel.Operator
}

func setupSecurityTestHarness(t *testing.T) *securityTestHarness {
	t.Helper()
	ctx := context.Background()
	reg := testpg.NewRegistry(t)
	if err := apiserver.Bootstrap(ctx, reg, "admin", "admin@example.com", "P@88w0rd"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	bus := newMemTestMQ()
	cacheStore := newMemTestCache()
	drivers := runtime.NewDefaultRegistry(t.TempDir())
	srv := apiserver.NewServer(nil, nil, reg, cacheStore, drivers, bus)
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	return &securityTestHarness{
		reg:      reg,
		bus:      bus,
		drivers:  drivers,
		httpSrv:  httpSrv,
		sessions: sessionmodel.NewOperator(reg, cacheStore, nil),
		tenants:  tenantmodel.NewOperator(reg, bus),
		gr:       globalrolemodel.NewOperator(reg, bus),
		grb:      globalrolebindingmodel.NewOperator(reg, bus),
		tr:       tenantrolemodel.NewOperator(reg, bus),
		trb:      tenantrolebindingmodel.NewOperator(reg, bus),
	}
}

func (h *securityTestHarness) doGET(t *testing.T, token, path string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, h.httpSrv.URL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest(%s): %v", path, err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll(%s): %v", path, err)
	}
	return resp.StatusCode, body
}

type testWatchStream struct {
	resp    *http.Response
	cancel  context.CancelFunc
	events  chan metav1.WatchEvent[json.RawMessage]
	readMu  sync.Mutex
	readErr error
}

func (h *securityTestHarness) openWatch(t *testing.T, token, path string) *testWatchStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.httpSrv.URL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest(%s): %v", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req) //nolint:bodyclose // closed in stream reader goroutine and error paths
	if err != nil {
		cancel()
		t.Fatalf("Watch %s: %v", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		t.Fatalf("Watch %s status=%d body=%s", path, resp.StatusCode, string(body))
	}
	reader := bufio.NewReader(resp.Body)
	// Wait for the initial "\n" written once startWatchMaterializer is active.
	if _, err := reader.ReadString('\n'); err != nil {
		resp.Body.Close()
		cancel()
		t.Fatalf("Watch %s initial newline: %v", path, err)
	}
	ws := &testWatchStream{
		resp:   resp,
		cancel: cancel,
		events: make(chan metav1.WatchEvent[json.RawMessage], 32),
	}
	go func() {
		defer close(ws.events)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var ev metav1.WatchEvent[json.RawMessage]
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				ws.readMu.Lock()
				ws.readErr = err
				ws.readMu.Unlock()
				return
			}
			ws.events <- ev
		}
		ws.readMu.Lock()
		ws.readErr = scanner.Err()
		ws.readMu.Unlock()
	}()
	t.Cleanup(ws.close)
	return ws
}

func (ws *testWatchStream) close() {
	ws.cancel()
	if ws.resp != nil && ws.resp.Body != nil {
		_ = ws.resp.Body.Close()
	}
}

func (ws *testWatchStream) lastErr() error {
	ws.readMu.Lock()
	defer ws.readMu.Unlock()
	return ws.readErr
}

func (ws *testWatchStream) waitEvent(t *testing.T, timeout time.Duration) metav1.WatchEvent[json.RawMessage] {
	t.Helper()
	select {
	case ev, ok := <-ws.events:
		if !ok {
			t.Fatalf("watch closed early before expected event: %v", ws.lastErr())
		}
		return ev
	case <-time.After(timeout):
		t.Fatal("timed out waiting for watch event")
	}
	return metav1.WatchEvent[json.RawMessage]{}
}

func (ws *testWatchStream) assertNoEvent(t *testing.T, wait time.Duration) {
	t.Helper()
	select {
	case ev, ok := <-ws.events:
		if ok {
			t.Fatalf("unexpected watch event %s: %s", ev.Type, string(ev.Object))
		}
		t.Fatalf("watch closed unexpectedly: %v", ws.lastErr())
	case <-time.After(wait):
	}
}

func (ws *testWatchStream) waitErrorAndEOF(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	sawError := false
	for {
		select {
		case ev, ok := <-ws.events:
			if !ok {
				if err := ws.lastErr(); err != nil {
					t.Fatalf("watch closed with error instead of clean EOF: %v", err)
				}
				if !sawError {
					t.Fatal("watch closed without receiving ERROR event")
				}
				return
			}
			if string(ev.Type) == "ERROR" {
				sawError = true
			}
		case <-deadline.C:
			t.Fatal("timed out waiting for watch stream termination")
		}
	}
}

func (h *securityTestHarness) createUser(t *testing.T, name string) {
	t.Helper()
	if _, err := userdb.NewDB(h.reg.Pool()).Create(context.Background(), &iamv1.User{
		Metadata: metav1.ObjectMeta{Name: name},
		Spec: iamv1.UserSpec{
			Email:        name + "@example.com",
			DisplayName:  name,
			PasswordHash: "$2a$10$abcdefghijklmnopqrstuv",
			State:        iamv1.UserStateActive,
		},
	}); err != nil {
		t.Fatalf("create user %s: %v", name, err)
	}
}

func (h *securityTestHarness) createTenant(t *testing.T, name, owner string) *corev1.Tenant {
	t.Helper()
	created, err := h.tenants.Create(context.Background(), owner, &corev1.Tenant{
		Metadata: metav1.ObjectMeta{Name: name},
		Spec: corev1.TenantSpec{
			DisplayName: name,
			Type:        corev1.TenantTypeOrganization,
			Owner:       owner,
			Quota: corev1.ResourceList{
				corev1.ResourceCPU:       "32",
				corev1.ResourceMemory:    "64Gi",
				corev1.ResourceStorage:   "500Gi",
				corev1.ResourceInstances: "20",
			},
		},
	})
	if err != nil {
		t.Fatalf("create tenant %s: %v", name, err)
	}
	return created
}

func (h *securityTestHarness) createActiveInstance(t *testing.T, tenant, nodeName, name string, labels, env map[string]string) *corev1.Instance {
	t.Helper()
	inst := fixtures.CreateActive(t, context.Background(), h.reg, h.drivers, tenant, nodeName, &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec: corev1.InstanceSpec{
			RuntimeClassName: "docker",
			Image:            "alpine:3.20",
			State:            corev1.InstanceStateRunning,
			Env:              env,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:     "1000m",
					corev1.ResourceMemory:  "1Gi",
					corev1.ResourceStorage: "5Gi",
				},
			},
		},
	})
	return inst
}

func TestNodeRestrictionInstanceAndNodeIsolation(t *testing.T) {
	ctx := context.Background()
	h := setupSecurityTestHarness(t)
	h.createTenant(t, "t-1", "admin")
	h.createTenant(t, "t-2", "admin")

	a1 := h.createActiveInstance(t, "t-1", "node-a", "inst-a1", nil, map[string]string{"SECRET": "a1"})
	a2 := h.createActiveInstance(t, "t-1", "node-a", "inst-a2", nil, map[string]string{"SECRET": "a2"})
	_ = h.createActiveInstance(t, "t-2", "node-a", "inst-a3", nil, map[string]string{"SECRET": "a3"})
	b1 := h.createActiveInstance(t, "t-1", "node-b", "inst-b1", nil, map[string]string{"SECRET": "victim-b1"})
	b2 := h.createActiveInstance(t, "t-2", "node-b", "inst-b2", nil, map[string]string{"SECRET": "victim-b2"})

	nodeASess, err := h.sessions.Issue(ctx, iamv1.SubjectKindNode, "node-a", "")
	if err != nil {
		t.Fatalf("Issue node-a session: %v", err)
	}
	token := nodeASess.Token

	// Node resources: list/watch/cross-node get denied, self get allowed.
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/apis/core.gamepanel.io/v1/nodes", http.StatusForbidden},
		{"/apis/core.gamepanel.io/v1/nodes?watch=1", http.StatusForbidden},
		{"/apis/core.gamepanel.io/v1/nodes/node-b", http.StatusForbidden},
		{"/apis/core.gamepanel.io/v1/nodes/node-a", http.StatusOK},
		{"/apis/core.gamepanel.io/v1/instances?nodeName=node-b", http.StatusForbidden},
		{"/apis/core.gamepanel.io/v1/instances?nodeName=node-a&nodeName=node-b", http.StatusForbidden},
		{"/apis/core.gamepanel.io/v1/instances?fieldSelector=" + url.QueryEscape("spec.nodeName=node-b"), http.StatusForbidden},
		{"/apis/core.gamepanel.io/v1/instances?fieldSelector=" + url.QueryEscape("spec.nodeName!=node-a"), http.StatusForbidden},
		{"/apis/core.gamepanel.io/v1/instances?fieldSelector=" + url.QueryEscape("spec.nodeName!=node-b"), http.StatusForbidden},
		{"/apis/core.gamepanel.io/v1/tenants/t-1/instances?nodeName=node-b", http.StatusForbidden},
		{"/apis/core.gamepanel.io/v1/tenants/t-1/instances?nodeName=node-a&nodeName=node-b", http.StatusForbidden},
		{"/apis/core.gamepanel.io/v1/tenants/t-1/instances?fieldSelector=" + url.QueryEscape("spec.nodeName=node-b"), http.StatusForbidden},
	} {
		if code, body := h.doGET(t, token, tc.path); code != tc.want {
			t.Fatalf("GET %s status=%d, want %d (body=%s)", tc.path, code, tc.want, string(body))
		}
	}

	// Cluster list & pagination only expose node-a instances and node-a totalCount (3 total, 1+1 page).
	code, body := h.doGET(t, token, "/apis/core.gamepanel.io/v1/instances?limit=1&offset=1")
	if code != http.StatusOK {
		t.Fatalf("cluster list page status=%d body=%s", code, string(body))
	}
	var clusterPage metav1.List[*corev1.Instance]
	if err := json.Unmarshal(body, &clusterPage); err != nil {
		t.Fatalf("decode cluster list: %v", err)
	}
	if clusterPage.Metadata.TotalCount != 3 || len(clusterPage.Items) != 1 || clusterPage.Items[0].Spec.NodeName != "node-a" {
		t.Fatalf("unexpected cluster page: total=%d len=%d items=%+v", clusterPage.Metadata.TotalCount, len(clusterPage.Items), clusterPage.Items)
	}
	if strings.Contains(string(body), "victim-") {
		t.Fatalf("response leaked node-b env: %s", string(body))
	}

	// Tenant list filters before pagination and rejects nothing on matching node.
	code, body = h.doGET(t, token, "/apis/core.gamepanel.io/v1/tenants/t-1/instances")
	if code != http.StatusOK {
		t.Fatalf("tenant list status=%d body=%s", code, string(body))
	}
	var tenantList metav1.List[*corev1.Instance]
	if err := json.Unmarshal(body, &tenantList); err != nil {
		t.Fatalf("decode tenant list: %v", err)
	}
	if tenantList.Metadata.TotalCount != 2 || len(tenantList.Items) != 2 {
		t.Fatalf("unexpected tenant list counts: total=%d len=%d", tenantList.Metadata.TotalCount, len(tenantList.Items))
	}
	for _, item := range tenantList.Items {
		if item.Spec.NodeName != "node-a" {
			t.Fatalf("leaked instance on node %q", item.Spec.NodeName)
		}
	}

	// Watch with watch=1 only delivers ADDED/MODIFIED/DELETED for node-a, never node-b.
	ws := h.openWatch(t, token, "/apis/core.gamepanel.io/v1/instances?watch=1")
	_ = h.bus.Publish(ctx, mq.TopicInstance, mq.NewEvent(mq.EventCreated, corev1.GroupVersion, "Instance", b1.Metadata.Tenant, b1.Metadata.Name, b1.Metadata.Generation, b1.Metadata.ResourceVersion))
	ws.assertNoEvent(t, 200*time.Millisecond)

	_ = h.bus.Publish(ctx, mq.TopicInstance, mq.NewEvent(mq.EventCreated, corev1.GroupVersion, "Instance", a1.Metadata.Tenant, a1.Metadata.Name, a1.Metadata.Generation, a1.Metadata.ResourceVersion))
	evAdded := ws.waitEvent(t, 2*time.Second)
	var addedInst corev1.Instance
	if err := json.Unmarshal(evAdded.Object, &addedInst); err != nil || evAdded.Type != metav1.WatchAdded || addedInst.Metadata.Name != a1.Metadata.Name {
		t.Fatalf("unexpected ADDED event: type=%s obj=%s err=%v", evAdded.Type, string(evAdded.Object), err)
	}

	// Deleted watch events include nodeName; node-b deleted is filtered out while node-a deleted is delivered.
	bDel := mq.NewEvent(mq.EventDeleted, corev1.GroupVersion, "Instance", b2.Metadata.Tenant, b2.Metadata.Name, b2.Metadata.Generation, b2.Metadata.ResourceVersion)
	bDel.Attributes = map[string]string{"uid": b2.Metadata.UID, "nodeName": "node-b", "labels": "{}"}
	_ = h.bus.Publish(ctx, mq.TopicInstance, bDel)
	ws.assertNoEvent(t, 200*time.Millisecond)

	aDel := mq.NewEvent(mq.EventDeleted, corev1.GroupVersion, "Instance", a2.Metadata.Tenant, a2.Metadata.Name, a2.Metadata.Generation, a2.Metadata.ResourceVersion)
	aDel.Attributes = map[string]string{"uid": a2.Metadata.UID, "nodeName": "node-a", "labels": "{}"}
	_ = h.bus.Publish(ctx, mq.TopicInstance, aDel)
	evDeleted := ws.waitEvent(t, 2*time.Second)
	if evDeleted.Type != metav1.WatchDeleted || !strings.Contains(string(evDeleted.Object), a2.Metadata.Name) {
		t.Fatalf("unexpected DELETED event: %+v %s", evDeleted.Type, string(evDeleted.Object))
	}
	_ = instancedb.NewDB(h.reg.Pool())
}

func TestWatchVerbUnifiedAndTenantWatchIsolation(t *testing.T) {
	ctx := context.Background()
	h := setupSecurityTestHarness(t)
	h.createUser(t, "alice")
	h.createUser(t, "bob")
	h.createUser(t, "charlie")
	h.createTenant(t, "t-alice", "alice")

	// Alice has get+list on instances in t-alice, NOT watch -> both watch=true and watch=1 must return 403.
	if _, err := h.tr.Create(ctx, "t-alice", &iamv1.TenantRole{
		Metadata: metav1.ObjectMeta{Name: "inst-no-watch", Tenant: "t-alice"},
		Rules: []iamv1.PolicyRule{{
			APIGroups: []string{corev1.GroupName},
			Resources: []string{"instances"},
			Verbs:     []string{iamv1.VerbGet, iamv1.VerbList},
		}},
	}); err != nil {
		t.Fatalf("create TenantRole: %v", err)
	}
	if _, err := h.trb.Create(ctx, "t-alice", &iamv1.TenantRoleBinding{
		Metadata: metav1.ObjectMeta{Name: "alice-no-watch", Tenant: "t-alice"},
		RoleRef:  iamv1.RoleRef{Name: "inst-no-watch"},
		Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "alice"}},
	}); err != nil {
		t.Fatalf("create TenantRoleBinding: %v", err)
	}

	adminSess, err := h.sessions.Issue(ctx, iamv1.SubjectKindUser, "admin", "")
	if err != nil {
		t.Fatalf("Issue admin: %v", err)
	}
	aliceSess, err := h.sessions.Issue(ctx, iamv1.SubjectKindUser, "alice", "t-alice")
	if err != nil {
		t.Fatalf("Issue alice: %v", err)
	}
	if code, _ := h.doGET(t, aliceSess.Token, "/apis/core.gamepanel.io/v1/tenants/t-alice/instances"); code != http.StatusOK {
		t.Fatalf("expected 200 on list, got %d", code)
	}
	for _, q := range []string{"?watch=true", "?watch=1"} {
		if code, body := h.doGET(t, aliceSess.Token, "/apis/core.gamepanel.io/v1/tenants/t-alice/instances"+q); code != http.StatusForbidden {
			t.Fatalf("%s expected 403, got %d (%s)", q, code, string(body))
		}
		if code, body := h.doGET(t, aliceSess.Token, "/apis/core.gamepanel.io/v1/tenants"+q); code != http.StatusForbidden {
			t.Fatalf("tenants%s without watch role expected 403, got %d (%s)", q, code, string(body))
		}
	}

	if _, err := h.gr.Create(ctx, &iamv1.GlobalRole{
		Metadata: metav1.ObjectMeta{Name: "tenant-watcher"},
		Rules:    []iamv1.PolicyRule{{APIGroups: []string{corev1.GroupName}, Resources: []string{"tenants"}, Verbs: []string{iamv1.VerbList, iamv1.VerbWatch}}},
	}); err != nil {
		t.Fatalf("create tenant-watcher role: %v", err)
	}
	if _, err := h.grb.Create(ctx, &iamv1.GlobalRoleBinding{
		Metadata: metav1.ObjectMeta{Name: "users-tenant-watcher"},
		RoleRef:  iamv1.RoleRef{Name: "tenant-watcher"},
		Subjects: []iamv1.Subject{
			{Kind: iamv1.SubjectKindUser, Name: "alice"},
			{Kind: iamv1.SubjectKindUser, Name: "charlie"},
		},
	}); err != nil {
		t.Fatalf("create users-tenant-watcher binding: %v", err)
	}

	// Concurrent shared hub watches on /tenants?watch=1: admin vs alice.
	adminWS := h.openWatch(t, adminSess.Token, "/apis/core.gamepanel.io/v1/tenants?watch=1")
	aliceWS := h.openWatch(t, aliceSess.Token, "/apis/core.gamepanel.io/v1/tenants?watch=1")

	tBob := h.createTenant(t, "t-bob", "bob")
	tAlice2 := h.createTenant(t, "t-alice-2", "alice")

	// Update t-bob then t-alice-2 via real operator so watchHub shared object cache handles both.
	tBob.Spec.DisplayName = "Bob Updated"
	if _, err := h.tenants.Update(ctx, tBob.Metadata.Name, tBob); err != nil {
		t.Fatalf("update t-bob: %v", err)
	}
	tAlice2.Spec.DisplayName = "Alice2 Updated"
	if _, err := h.tenants.Update(ctx, tAlice2.Metadata.Name, tAlice2); err != nil {
		t.Fatalf("update t-alice-2: %v", err)
	}

	// Admin receives events for both t-bob and t-alice-2; Alice only sees t-alice-2 events.
	seenAdmin := map[string]bool{}
	for range 4 {
		ev := adminWS.waitEvent(t, 2*time.Second)
		var obj corev1.Tenant
		if err := json.Unmarshal(ev.Object, &obj); err == nil {
			seenAdmin[string(ev.Type)+":"+obj.Metadata.Name] = true
		}
	}
	if !seenAdmin["ADDED:t-bob"] || !seenAdmin["MODIFIED:t-bob"] || !seenAdmin["ADDED:t-alice-2"] || !seenAdmin["MODIFIED:t-alice-2"] {
		t.Fatalf("admin missing expected tenant watch events: %+v", seenAdmin)
	}

	for _, wantType := range []metav1.WatchEventType{metav1.WatchAdded, metav1.WatchModified} {
		ev := aliceWS.waitEvent(t, 2*time.Second)
		var obj corev1.Tenant
		_ = json.Unmarshal(ev.Object, &obj)
		if ev.Type != wantType || obj.Metadata.Name != "t-alice-2" {
			t.Fatalf("alice got unexpected event %s name=%q (raw=%s)", ev.Type, obj.Metadata.Name, string(ev.Object))
		}
	}
	aliceWS.assertNoEvent(t, 200*time.Millisecond)

	// Deleted tenant events also filter by owner/permissions on shared hub.
	delBob := mq.NewEvent(mq.EventDeleted, corev1.GroupVersion, "Tenant", "", "t-bob", tBob.Metadata.Generation, tBob.Metadata.ResourceVersion)
	delBob.Attributes = map[string]string{"uid": tBob.Metadata.UID, "owner": "bob", "labels": "{}"}
	_ = h.bus.Publish(ctx, mq.TopicTenant, delBob)
	aliceWS.assertNoEvent(t, 200*time.Millisecond)
	if ev := adminWS.waitEvent(t, 2*time.Second); ev.Type != metav1.WatchDeleted || !strings.Contains(string(ev.Object), "t-bob") {
		t.Fatalf("admin expected DELETED t-bob, got %s %s", ev.Type, string(ev.Object))
	}

	delAlice2 := mq.NewEvent(mq.EventDeleted, corev1.GroupVersion, "Tenant", "", "t-alice-2", tAlice2.Metadata.Generation, tAlice2.Metadata.ResourceVersion)
	delAlice2.Attributes = map[string]string{"uid": tAlice2.Metadata.UID, "owner": "alice", "labels": "{}"}
	_ = h.bus.Publish(ctx, mq.TopicTenant, delAlice2)
	if ev := aliceWS.waitEvent(t, 2*time.Second); ev.Type != metav1.WatchDeleted || !strings.Contains(string(ev.Object), "t-alice-2") {
		t.Fatalf("alice expected DELETED t-alice-2, got %s %s", ev.Type, string(ev.Object))
	}
	if ev := adminWS.waitEvent(t, 2*time.Second); ev.Type != metav1.WatchDeleted || !strings.Contains(string(ev.Object), "t-alice-2") {
		t.Fatalf("admin expected DELETED t-alice-2, got %s %s", ev.Type, string(ev.Object))
	}

	// Live revocation: TenantRoleBinding, GlobalRoleBinding, and Session revocation terminate active streams.
	if _, err := h.tr.Create(ctx, "t-alice", &iamv1.TenantRole{
		Metadata: metav1.ObjectMeta{Name: "inst-watcher", Tenant: "t-alice"},
		Rules:    []iamv1.PolicyRule{{APIGroups: []string{corev1.GroupName}, Resources: []string{"instances"}, Verbs: []string{iamv1.VerbList, iamv1.VerbWatch}}},
	}); err != nil {
		t.Fatalf("create TenantRole inst-watcher: %v", err)
	}
	if _, err := h.trb.Create(ctx, "t-alice", &iamv1.TenantRoleBinding{
		Metadata: metav1.ObjectMeta{Name: "charlie-inst-watch", Tenant: "t-alice"},
		RoleRef:  iamv1.RoleRef{Name: "inst-watcher"},
		Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "charlie"}},
	}); err != nil {
		t.Fatalf("create TRB charlie: %v", err)
	}
	if _, err := h.gr.Create(ctx, &iamv1.GlobalRole{
		Metadata: metav1.ObjectMeta{Name: "global-inst-watcher"},
		Rules:    []iamv1.PolicyRule{{APIGroups: []string{corev1.GroupName}, Resources: []string{"instances"}, Verbs: []string{iamv1.VerbList, iamv1.VerbWatch}}},
	}); err != nil {
		t.Fatalf("create GlobalRole: %v", err)
	}
	if _, err := h.grb.Create(ctx, &iamv1.GlobalRoleBinding{
		Metadata: metav1.ObjectMeta{Name: "charlie-global-watch"},
		RoleRef:  iamv1.RoleRef{Name: "global-inst-watcher"},
		Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "charlie"}},
	}); err != nil {
		t.Fatalf("create GRB charlie: %v", err)
	}

	charlieSess1, err := h.sessions.Issue(ctx, iamv1.SubjectKindUser, "charlie", "t-alice")
	if err != nil {
		t.Fatalf("Issue charlie1: %v", err)
	}
	trbWS := h.openWatch(t, charlieSess1.Token, "/apis/core.gamepanel.io/v1/tenants/t-alice/instances?watch=1")
	grbWS := h.openWatch(t, charlieSess1.Token, "/apis/core.gamepanel.io/v1/instances?watch=1")
	if err := h.trb.Delete(ctx, "t-alice", "charlie-inst-watch"); err != nil {
		t.Fatalf("delete TRB: %v", err)
	}
	if err := h.grb.Delete(ctx, "charlie-global-watch"); err != nil {
		t.Fatalf("delete GRB: %v", err)
	}
	trbWS.waitErrorAndEOF(t, 3*time.Second)
	grbWS.waitErrorAndEOF(t, 3*time.Second)

	charlieSess2, err := h.sessions.Issue(ctx, iamv1.SubjectKindUser, "charlie", "t-alice")
	if err != nil {
		t.Fatalf("Issue charlie2: %v", err)
	}
	sessWS := h.openWatch(t, charlieSess2.Token, "/apis/core.gamepanel.io/v1/tenants?watch=1")
	if err := h.sessions.Delete(ctx, charlieSess2.Metadata.Name); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	sessWS.waitErrorAndEOF(t, 3*time.Second)
}

func TestRBACResourceNamesAndSelectorCollectionDenial(t *testing.T) {
	ctx := context.Background()
	h := setupSecurityTestHarness(t)
	h.createUser(t, "dave")
	h.createUser(t, "eve")
	h.createTenant(t, "t-1", "admin")
	_ = h.createActiveInstance(t, "t-1", "node-a", "allowed", map[string]string{"env": "dev"}, map[string]string{"K": "ok"})
	_ = h.createActiveInstance(t, "t-1", "node-a", "victim", map[string]string{"env": "prod"}, map[string]string{"SECRET": "top-secret"})

	if _, err := h.tr.Create(ctx, "t-1", &iamv1.TenantRole{
		Metadata: metav1.ObjectMeta{Name: "only-allowed-name", Tenant: "t-1"},
		Rules: []iamv1.PolicyRule{{
			APIGroups:     []string{corev1.GroupName},
			Resources:     []string{"instances"},
			Verbs:         []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch},
			ResourceNames: []string{"allowed"},
		}},
	}); err != nil {
		t.Fatalf("create TenantRole only-allowed-name: %v", err)
	}
	if _, err := h.trb.Create(ctx, "t-1", &iamv1.TenantRoleBinding{
		Metadata: metav1.ObjectMeta{Name: "dave-allowed-only", Tenant: "t-1"},
		RoleRef:  iamv1.RoleRef{Name: "only-allowed-name"},
		Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "dave"}},
	}); err != nil {
		t.Fatalf("create TRB dave: %v", err)
	}
	daveSess, err := h.sessions.Issue(ctx, iamv1.SubjectKindUser, "dave", "t-1")
	if err != nil {
		t.Fatalf("Issue dave: %v", err)
	}
	if code, _ := h.doGET(t, daveSess.Token, "/apis/core.gamepanel.io/v1/tenants/t-1/instances/allowed"); code != http.StatusOK {
		t.Fatalf("dave get allowed expected 200, got %d", code)
	}
	for _, denied := range []string{
		"/apis/core.gamepanel.io/v1/tenants/t-1/instances/victim",
		"/apis/core.gamepanel.io/v1/tenants/t-1/instances",
		"/apis/core.gamepanel.io/v1/tenants/t-1/instances?watch=true",
		"/apis/core.gamepanel.io/v1/tenants/t-1/instances?watch=1",
	} {
		if code, body := h.doGET(t, daveSess.Token, denied); code != http.StatusForbidden {
			t.Fatalf("dave %s expected 403, got %d (%s)", denied, code, string(body))
		}
	}

	// Negative selector alone denies list & watch; additive broad role grants list.
	if _, err := h.tr.Create(ctx, "t-1", &iamv1.TenantRole{
		Metadata: metav1.ObjectMeta{Name: "neg-selector", Tenant: "t-1"},
		Rules: []iamv1.PolicyRule{{
			APIGroups: []string{corev1.GroupName},
			Resources: []string{"instances"},
			Verbs:     []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch},
			Selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "env", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"prod"}},
				},
			},
		}},
	}); err != nil {
		t.Fatalf("create TenantRole neg-selector: %v", err)
	}
	if _, err := h.trb.Create(ctx, "t-1", &iamv1.TenantRoleBinding{
		Metadata: metav1.ObjectMeta{Name: "eve-neg-selector", Tenant: "t-1"},
		RoleRef:  iamv1.RoleRef{Name: "neg-selector"},
		Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "eve"}},
	}); err != nil {
		t.Fatalf("create TRB eve: %v", err)
	}
	eveSess, err := h.sessions.Issue(ctx, iamv1.SubjectKindUser, "eve", "t-1")
	if err != nil {
		t.Fatalf("Issue eve: %v", err)
	}
	for _, denied := range []string{
		"/apis/core.gamepanel.io/v1/tenants/t-1/instances",
		"/apis/core.gamepanel.io/v1/tenants/t-1/instances?watch=1",
	} {
		if code, _ := h.doGET(t, eveSess.Token, denied); code != http.StatusForbidden {
			t.Fatalf("eve %s expected 403, got %d", denied, code)
		}
	}
	if _, err := h.tr.Create(ctx, "t-1", &iamv1.TenantRole{
		Metadata: metav1.ObjectMeta{Name: "broad-list", Tenant: "t-1"},
		Rules:    []iamv1.PolicyRule{{APIGroups: []string{corev1.GroupName}, Resources: []string{"instances"}, Verbs: []string{iamv1.VerbList, iamv1.VerbWatch}}},
	}); err != nil {
		t.Fatalf("create broad-list: %v", err)
	}
	if _, err := h.trb.Create(ctx, "t-1", &iamv1.TenantRoleBinding{
		Metadata: metav1.ObjectMeta{Name: "eve-broad-list", Tenant: "t-1"},
		RoleRef:  iamv1.RoleRef{Name: "broad-list"},
		Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "eve"}},
	}); err != nil {
		t.Fatalf("create TRB eve-broad-list: %v", err)
	}
	if code, _ := h.doGET(t, eveSess.Token, "/apis/core.gamepanel.io/v1/tenants/t-1/instances"); code != http.StatusOK {
		t.Fatalf("eve broad list expected 200, got %d", code)
	}
}
