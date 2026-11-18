package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	tenantmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/tenant"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	prewarmjobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/prewarmjob"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
)

func (h *securityTestHarness) doMethod(t *testing.T, method, token, path string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, h.httpSrv.URL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest(%s %s): %v", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var buf [4096]byte
	n, _ := resp.Body.Read(buf[:])
	return resp.StatusCode, buf[:n]
}

func (h *securityTestHarness) grantGlobalSelectorRole(t *testing.T, roleName, userName, apiGroup, resource string, verbs []string, sel *metav1.LabelSelector) {
	t.Helper()
	ctx := context.Background()
	if _, err := h.gr.Create(ctx, &iamv1.GlobalRole{
		Metadata: metav1.ObjectMeta{Name: roleName},
		Rules: []iamv1.PolicyRule{{
			APIGroups: []string{apiGroup},
			Resources: []string{resource},
			Verbs:     verbs,
			Selector:  sel,
		}},
	}); err != nil {
		t.Fatalf("create GlobalRole %s: %v", roleName, err)
	}
	if _, err := h.grb.Create(ctx, &iamv1.GlobalRoleBinding{
		Metadata: metav1.ObjectMeta{Name: roleName + "-binding"},
		RoleRef:  iamv1.RoleRef{Name: roleName},
		Subjects: []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: userName}},
	}); err != nil {
		t.Fatalf("create GlobalRoleBinding %s: %v", roleName, err)
	}
}

func TestNonInstanceSelectorRBACAndUnlabeledBounds(t *testing.T) {
	ctx := context.Background()
	h := setupSecurityTestHarness(t)
	apps := appdb.NewDB(h.reg.Pool())
	for _, a := range []*corev1.App{
		{Metadata: metav1.ObjectMeta{Name: "app-dev", Labels: map[string]string{"env": "dev"}}, DisplayName: "Dev", Image: "alpine:3.20", Published: true},
		{Metadata: metav1.ObjectMeta{Name: "app-unlabeled"}, DisplayName: "Plain", Image: "alpine:3.20", Published: true},
	} {
		if _, err := apps.Create(ctx, a); err != nil {
			t.Fatalf("seed app %s: %v", a.Metadata.Name, err)
		}
	}
	for _, u := range []struct {
		name   string
		labels map[string]string
	}{{"u-dev", map[string]string{"env": "dev"}}, {"u-plain", nil}} {
		if _, err := userdb.NewDB(h.reg.Pool()).Create(ctx, &iamv1.User{
			Metadata: metav1.ObjectMeta{Name: u.name, Labels: u.labels},
			Spec:     iamv1.UserSpec{Email: u.name + "@example.com", PasswordHash: "$2a$10$abcdefghijklmnopqrstuv", State: iamv1.UserStateActive},
		}); err != nil {
			t.Fatalf("seed user %s: %v", u.name, err)
		}
	}
	h.createUser(t, "frank")
	sess, err := h.sessions.Issue(ctx, iamv1.SubjectKindUser, "frank", "")
	if err != nil {
		t.Fatalf("Issue frank: %v", err)
	}
	posSel := &metav1.LabelSelector{MatchLabels: map[string]string{"env": "dev"}}
	negSel := &metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "restricted", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"secret"}}}}
	h.grantGlobalSelectorRole(t, "frank-u-pos", "frank", iamv1.GroupName, "users", []string{iamv1.VerbGet, iamv1.VerbList}, posSel)
	h.grantGlobalSelectorRole(t, "frank-a-pos", "frank", corev1.GroupName, "apps", []string{iamv1.VerbDelete}, posSel)

	assertSelectorHTTP(t, h, sess.Token, http.MethodGet, "/apis/iam.gamepanel.io/v1/users/u-dev", http.StatusOK)
	assertSelectorHTTP(t, h, sess.Token, http.MethodGet, "/apis/iam.gamepanel.io/v1/users/u-plain", http.StatusForbidden)
	assertSelectorHTTP(t, h, sess.Token, http.MethodGet, "/apis/iam.gamepanel.io/v1/users/u-missing", http.StatusForbidden)
	assertSelectorHTTP(t, h, sess.Token, http.MethodGet, "/apis/iam.gamepanel.io/v1/users", http.StatusForbidden)
	assertSelectorHTTP(t, h, sess.Token, http.MethodDelete, "/apis/core.gamepanel.io/v1/apps/app-dev", http.StatusNoContent)
	assertSelectorHTTP(t, h, sess.Token, http.MethodDelete, "/apis/core.gamepanel.io/v1/apps/app-unlabeled", http.StatusForbidden)
	assertSelectorHTTP(t, h, sess.Token, http.MethodDelete, "/apis/core.gamepanel.io/v1/apps/app-missing", http.StatusForbidden)

	h.grantGlobalSelectorRole(t, "frank-u-neg", "frank", iamv1.GroupName, "users", []string{iamv1.VerbGet, iamv1.VerbList}, negSel)
	h.grantGlobalSelectorRole(t, "frank-a-neg", "frank", corev1.GroupName, "apps", []string{iamv1.VerbDelete}, negSel)
	assertSelectorHTTP(t, h, sess.Token, http.MethodGet, "/apis/iam.gamepanel.io/v1/users/u-plain", http.StatusOK)
	assertSelectorHTTP(t, h, sess.Token, http.MethodGet, "/apis/iam.gamepanel.io/v1/users/u-missing", http.StatusForbidden)
	assertSelectorHTTP(t, h, sess.Token, http.MethodGet, "/apis/iam.gamepanel.io/v1/users", http.StatusForbidden)
	assertSelectorHTTP(t, h, sess.Token, http.MethodDelete, "/apis/core.gamepanel.io/v1/apps/app-unlabeled", http.StatusNoContent)
	assertSelectorHTTP(t, h, sess.Token, http.MethodDelete, "/apis/core.gamepanel.io/v1/apps/app-missing", http.StatusForbidden)
}

func assertSelectorHTTP(t *testing.T, h *securityTestHarness, token, method, path string, want int) {
	t.Helper()
	if code, body := h.doMethod(t, method, token, path); code != want {
		t.Fatalf("%s %s=%d, want %d (body=%s)", method, path, code, want, string(body))
	}
}

func TestPrewarmJobNodeStatusRedaction(t *testing.T) {
	ctx := context.Background()
	h := setupSecurityTestHarness(t)
	if _, err := prewarmjobdb.NewDB(h.reg.Pool()).Create(ctx, &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{Name: "pw-1"},
		Spec:     corev1.PrewarmJobSpec{Images: []string{"alpine:3.20"}},
		Status: corev1.PrewarmJobStatus{
			Phase: corev1.PrewarmJobPhaseRunning,
			Nodes: []corev1.PrewarmNodeStatus{
				{NodeName: "node-a", Phase: corev1.PrewarmJobPhaseSucceeded, Message: "ok-a"},
				{NodeName: "node-b", Phase: corev1.PrewarmJobPhaseFailed, Message: "secret-b"},
			},
		},
	}); err != nil {
		t.Fatalf("create PrewarmJob: %v", err)
	}
	nodeSess, err := h.sessions.Issue(ctx, iamv1.SubjectKindNode, "node-a", "")
	if err != nil {
		t.Fatalf("Issue node-a: %v", err)
	}
	code, body := h.doGET(t, nodeSess.Token, "/apis/core.gamepanel.io/v1/prewarmjobs/pw-1")
	if code != http.StatusOK {
		t.Fatalf("node GET prewarmjob=%d (%s)", code, string(body))
	}
	var gotJob corev1.PrewarmJob
	if err := json.Unmarshal(body, &gotJob); err != nil {
		t.Fatalf("decode prewarmjob: %v", err)
	}
	if len(gotJob.Status.Nodes) != 1 || gotJob.Status.Nodes[0].NodeName != "node-a" {
		t.Fatalf("expected only node-a status visible, got %+v", gotJob.Status.Nodes)
	}
	code, listBody := h.doGET(t, nodeSess.Token, "/apis/core.gamepanel.io/v1/prewarmjobs")
	if code != http.StatusOK {
		t.Fatalf("node LIST prewarmjobs=%d (%s)", code, string(listBody))
	}
	var gotList metav1.List[corev1.PrewarmJob]
	if err := json.Unmarshal(listBody, &gotList); err != nil || len(gotList.Items) != 1 || len(gotList.Items[0].Status.Nodes) != 1 || gotList.Items[0].Status.Nodes[0].NodeName != "node-a" {
		t.Fatalf("expected redacted prewarmjob list for node-a, got list=%+v err=%v", gotList, err)
	}
	adminSess, err := h.sessions.Issue(ctx, iamv1.SubjectKindUser, "admin", "")
	if err != nil {
		t.Fatalf("Issue admin: %v", err)
	}
	if cAdmin, bAdmin := h.doGET(t, adminSess.Token, "/apis/core.gamepanel.io/v1/prewarmjobs/pw-1"); cAdmin != http.StatusOK {
		t.Fatalf("admin GET prewarmjob=%d", cAdmin)
	} else if err := json.Unmarshal(bAdmin, &gotJob); err != nil || len(gotJob.Status.Nodes) != 2 {
		t.Fatalf("expected admin to see both nodes, got %+v err=%v", gotJob.Status.Nodes, err)
	}
}

func TestTenantOperatorListAndCanViewBounds(t *testing.T) {
	ctx := context.Background()
	h := setupSecurityTestHarness(t)
	for _, spec := range []struct {
		name   string
		labels map[string]string
	}{
		{"t-dev", map[string]string{"env": "dev"}},
		{"t-plain", nil},
	} {
		if _, err := h.tenants.Create(ctx, "admin", &corev1.Tenant{
			Metadata: metav1.ObjectMeta{Name: spec.name, Labels: spec.labels},
			Spec:     corev1.TenantSpec{DisplayName: spec.name, Type: corev1.TenantTypeOrganization, Owner: "admin"},
		}); err != nil {
			t.Fatalf("create tenant %s: %v", spec.name, err)
		}
	}
	op := tenantmodel.NewOperator(h.reg, h.bus)
	anonCtx := iamv1.WithUser(ctx, iamv1.UserInfo{Name: iamv1.AnonymousUser, Groups: []string{iamv1.GroupUnauthenticated}})
	nodeCtx := iamv1.WithUser(ctx, iamv1.UserInfo{Kind: iamv1.SubjectKindNode, Name: "node-a", Groups: []string{"system:nodes"}})
	if list, err := op.List(anonCtx, db.ListOptions{}); err != nil || len(list.Items) != 0 {
		t.Fatalf("expected anonymous tenant List empty, got %+v err=%v", list, err)
	}
	if list, err := op.List(nodeCtx, db.ListOptions{}); err != nil || len(list.Items) != 0 {
		t.Fatalf("expected node tenant List empty, got %+v err=%v", list, err)
	}

	h.createUser(t, "grace")
	h.grantGlobalSelectorRole(t, "grace-tenant-dev", "grace", corev1.GroupName, "tenants", []string{iamv1.VerbGet},
		&metav1.LabelSelector{MatchLabels: map[string]string{"env": "dev"}})
	graceCtx := iamv1.WithUser(ctx, iamv1.UserInfo{Kind: iamv1.SubjectKindUser, Name: "grace"})
	if !op.CanView(graceCtx, "t-dev", nil, "") || op.CanView(graceCtx, "t-plain", nil, "") {
		t.Fatalf("unexpected CanView results with selector rule (t-dev=%v t-plain=%v)",
			op.CanView(graceCtx, "t-dev", nil, ""), op.CanView(graceCtx, "t-plain", nil, ""))
	}
	if list, err := op.List(graceCtx, db.ListOptions{}); err != nil || len(list.Items) != 0 {
		t.Fatalf("expected grace List not to bypass selector via *, got %d err=%v", len(list.Items), err)
	}
}
