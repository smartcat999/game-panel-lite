package authorization

import (
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
)

func TestNodeRestrictionNodesSelfOnly(t *testing.T) {
	nodeUser := request.UserInfo{Kind: iamv1.SubjectKindNode, Name: "node-a", Groups: []string{"system:nodes"}}
	cases := []struct {
		name  string
		attrs AttributesRecord
		allow bool
	}{
		{"create empty", AttributesRecord{User: nodeUser, APIGroup: "core.gamepanel.io", Resource: "nodes", Verb: iamv1.VerbCreate}, true},
		{"create self", AttributesRecord{User: nodeUser, APIGroup: "core.gamepanel.io", Resource: "nodes", Verb: iamv1.VerbCreate, Name: "node-a"}, true},
		{"create other", AttributesRecord{User: nodeUser, APIGroup: "core.gamepanel.io", Resource: "nodes", Verb: iamv1.VerbCreate, Name: "node-b"}, false},
		{"get self", AttributesRecord{User: nodeUser, APIGroup: "core.gamepanel.io", Resource: "nodes", Verb: iamv1.VerbGet, Name: "node-a"}, true},
		{"get other", AttributesRecord{User: nodeUser, APIGroup: "core.gamepanel.io", Resource: "nodes", Verb: iamv1.VerbGet, Name: "node-b"}, false},
		{"patch self status", AttributesRecord{User: nodeUser, APIGroup: "core.gamepanel.io", Resource: "nodes", Subresource: "status", Verb: iamv1.VerbPatch, Name: "node-a"}, true},
		{"patch other status", AttributesRecord{User: nodeUser, APIGroup: "core.gamepanel.io", Resource: "nodes", Subresource: "status", Verb: iamv1.VerbPatch, Name: "node-b"}, false},
		{"list nodes", AttributesRecord{User: nodeUser, APIGroup: "core.gamepanel.io", Resource: "nodes", Verb: iamv1.VerbList}, false},
		{"watch nodes", AttributesRecord{User: nodeUser, APIGroup: "core.gamepanel.io", Resource: "nodes", Verb: iamv1.VerbWatch}, false},
		{"delete self", AttributesRecord{User: nodeUser, APIGroup: "core.gamepanel.io", Resource: "nodes", Verb: iamv1.VerbDelete, Name: "node-a"}, false},
		{"unknown subresource", AttributesRecord{User: nodeUser, APIGroup: "core.gamepanel.io", Resource: "nodes", Subresource: "exec", Verb: iamv1.VerbGet, Name: "node-a"}, false},
	}
	for _, tc := range cases {
		reason := nodeRestriction(tc.attrs, nil)
		if (reason == "") != tc.allow {
			t.Fatalf("%s: allow=%v (reason=%q), want %v", tc.name, reason == "", reason, tc.allow)
		}
	}
}

func TestCasbinLabelMatchWildcardAndNilLabels(t *testing.T) {
	user := request.UserInfo{Kind: iamv1.SubjectKindUser, Name: "erin"}
	bindings := []compiledBinding{{
		bindingName: "erin-selectors",
		subjects:    []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "erin"}},
		rules: []iamv1.PolicyRule{
			{
				APIGroups: []string{"core.gamepanel.io"},
				Resources: []string{"apps"},
				Verbs:     []string{iamv1.VerbGet},
				Selector:  &metav1.LabelSelector{MatchLabels: map[string]string{"env": "dev"}},
			},
			{
				APIGroups: []string{"core.gamepanel.io"},
				Resources: []string{"instances"},
				Verbs:     []string{iamv1.VerbGet},
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "restricted", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"secret"}},
					},
				},
			},
		},
	}}
	enforcer, hasSelectors := buildCasbinEnforcer("t1", bindings)
	if !hasSelectors {
		t.Fatal("expected hasSelectors=true for selector bindings")
	}
	snap := &rbacSnapshot{hasSelectors: hasSelectors, enforcer: enforcer}
	assertRuleSnapshotParity(t, snap, bindings[0].rules[0], user, "t1", "apps", iamv1.VerbGet, "*", map[string]string{"env": "dev"}, false)
	assertRuleSnapshotParity(t, snap, bindings[0].rules[1], user, "t1", "instances", iamv1.VerbGet, "*", map[string]string{}, false)
	assertRuleSnapshotParity(t, snap, bindings[0].rules[1], user, "t1", "instances", iamv1.VerbGet, "s1", nil, false)
	assertRuleSnapshotParity(t, snap, bindings[0].rules[1], user, "t1", "instances", iamv1.VerbGet, "s1", nonNilLabels(nil), true)
}

func TestRBACSnapshotCollectionBoundsParity(t *testing.T) {
	user := request.UserInfo{Kind: iamv1.SubjectKindUser, Name: "dave"}
	bindings := []compiledBinding{{
		bindingName: "dave-scoped",
		subjects:    []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "dave"}},
		rules: []iamv1.PolicyRule{
			{
				APIGroups:     []string{"core.gamepanel.io"},
				Resources:     []string{"instances"},
				Verbs:         []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch},
				ResourceNames: []string{"allowed"},
			},
			{
				APIGroups: []string{"core.gamepanel.io"},
				Resources: []string{"apps"},
				Verbs:     []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch},
				Selector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{Key: "env", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"prod"}},
					},
				},
			},
		},
	}}
	enforcer, hasSelectors := buildCasbinEnforcer("t1", bindings)
	snap := &rbacSnapshot{hasSelectors: hasSelectors, enforcer: enforcer}
	assertRuleSnapshotParity(t, snap, bindings[0].rules[0], user, "t1", "instances", iamv1.VerbList, "", nil, false)
	assertRuleSnapshotParity(t, snap, bindings[0].rules[0], user, "t1", "instances", iamv1.VerbWatch, "", nil, false)
	assertRuleSnapshotParity(t, snap, bindings[0].rules[0], user, "t1", "instances", iamv1.VerbGet, "allowed", nil, true)
	assertRuleSnapshotParity(t, snap, bindings[0].rules[0], user, "t1", "instances", iamv1.VerbGet, "victim", nil, false)
	assertRuleSnapshotParity(t, snap, bindings[0].rules[1], user, "t1", "apps", iamv1.VerbList, "", nil, false)
	assertRuleSnapshotParity(t, snap, bindings[0].rules[1], user, "t1", "apps", iamv1.VerbWatch, "", nil, false)
	assertRuleSnapshotParity(t, snap, bindings[0].rules[1], user, "t1", "apps", iamv1.VerbGet, "a1", map[string]string{"env": "dev"}, true)
	assertRuleSnapshotParity(t, snap, bindings[0].rules[1], user, "t1", "apps", iamv1.VerbGet, "a2", map[string]string{"env": "prod"}, false)
}

func assertRuleSnapshotParity(
	t *testing.T, snap *rbacSnapshot, rule iamv1.PolicyRule, u request.UserInfo,
	dom, res, verb, name string, labels map[string]string, want bool,
) {
	t.Helper()
	if gotRule := rule.Matches("core.gamepanel.io", res, verb, name, labels); gotRule != want {
		t.Fatalf("%s/%s (%q): PolicyRule.Matches=%v, want %v", res, verb, name, gotRule, want)
	}
	if _, gotSnap := matchSnapshot(snap, dom, u, "core.gamepanel.io", res, verb, name, labels); gotSnap != want {
		t.Fatalf("%s/%s (%q): matchSnapshot=%v, want %v", res, verb, name, gotSnap, want)
	}
}

func TestInstanceOperatorBuiltInRolePermissions(t *testing.T) {
	user := request.UserInfo{Kind: iamv1.SubjectKindUser, Name: "steve01"}
	bindings := []compiledBinding{{
		bindingName: "steve01-operator",
		subjects:    []iamv1.Subject{{Kind: iamv1.SubjectKindUser, Name: "steve01"}},
		rules: []iamv1.PolicyRule{
			{APIGroups: []string{corev1.GroupName}, Resources: []string{"tenants", "instances", "instances/*", "events", "alerts", "metering", "logs", "quotachanges", "orders"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch}},
			{APIGroups: []string{iamv1.GroupName}, Resources: []string{"tenantroles", "tenantrolebindings"}, Verbs: []string{iamv1.VerbGet, iamv1.VerbList, iamv1.VerbWatch}},
			{APIGroups: []string{corev1.GroupName}, Resources: []string{"instances"}, Verbs: []string{iamv1.VerbUpdate, iamv1.VerbPatch}},
			{APIGroups: []string{corev1.GroupName}, Resources: []string{"instances/state", "instances/exec", "instances/files"}, Verbs: []string{iamv1.VerbCreate, iamv1.VerbUpdate, iamv1.VerbPatch}},
		},
	}}
	enforcer, hasSelectors := buildCasbinEnforcer("studio-alpha", bindings)
	snap := &rbacSnapshot{hasSelectors: hasSelectors, enforcer: enforcer}
	cases := []struct {
		group, res, verb, name string
		want                   bool
	}{
		{corev1.GroupName, "tenants", iamv1.VerbGet, "studio-alpha", true},
		{corev1.GroupName, "instances", iamv1.VerbGet, "mc-01", true},
		{corev1.GroupName, "instances", iamv1.VerbUpdate, "mc-01", true},
		{corev1.GroupName, "instances", iamv1.VerbPatch, "mc-01", true},
		{corev1.GroupName, "instances/state", iamv1.VerbUpdate, "mc-01", true},
		{corev1.GroupName, "instances/exec", iamv1.VerbCreate, "mc-01", true},
		{corev1.GroupName, "instances/files", iamv1.VerbCreate, "mc-01", true},
		{corev1.GroupName, "logs", iamv1.VerbList, "", true},
		{corev1.GroupName, "events", iamv1.VerbList, "", true},
		{corev1.GroupName, "alerts", iamv1.VerbList, "", true},
		{corev1.GroupName, "metering", iamv1.VerbGet, "studio-alpha", true},
		{corev1.GroupName, "quotachanges", iamv1.VerbList, "", true},
		{corev1.GroupName, "orders", iamv1.VerbList, "", true},
		{corev1.GroupName, "redeem", iamv1.VerbCreate, "", false},
		{iamv1.GroupName, "tenantroles", iamv1.VerbList, "", true},
		{corev1.GroupName, "instances", iamv1.VerbCreate, "mc-02", false},
		{corev1.GroupName, "instances", iamv1.VerbDelete, "mc-01", false},
		{corev1.GroupName, "tenants", iamv1.VerbUpdate, "studio-alpha", false},
		{corev1.GroupName, "tenants/invites", iamv1.VerbCreate, "studio-alpha", false},
		{iamv1.GroupName, "tenantrolebindings", iamv1.VerbCreate, "", false},
		{iamv1.GroupName, "tenantrolebindings", iamv1.VerbDelete, "steve01-operator", false},
	}
	for _, tc := range cases {
		if _, got := matchSnapshot(snap, "studio-alpha", user, tc.group, tc.res, tc.verb, tc.name, nil); got != tc.want {
			t.Fatalf("%s/%s %s (%q): got=%v, want %v", tc.group, tc.res, tc.verb, tc.name, got, tc.want)
		}
	}
}
