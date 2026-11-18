package authorization

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"golang.org/x/sync/singleflight"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	globalrolemodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/globalrole"
	globalrolebindingmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/globalrolebinding"
	tenantrolemodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrole"
	tenantrolebindingmodel "github.com/smartcat999/game-panel-cloud/pkg/models/iam/tenantrolebinding"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
)

const (
	rbacCacheTTL             = 2 * time.Second
	maxCachedTenantSnapshots = 512
	casbinRBACModelText      = `
[request_definition]
r = sub, dom, apiGroup, resource, verb, resName, labels

[policy_definition]
p = sub, dom, apiGroup, resource, verb, resName, sel, bindingName

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.dom) && (p.dom == "*" || r.dom == p.dom) && wcMatch(r.apiGroup, p.apiGroup) && wcMatch(r.resource, p.resource) && wcMatch(r.verb, p.verb) && (p.resName == "*" || (r.resName != "" && wcMatch(r.resName, p.resName))) && labelMatch(r.resName, r.labels, p.sel)
`
)

type labelBag struct {
	labels map[string]string
}

type compiledBinding struct {
	bindingName string
	subjects    []iamv1.Subject
	rules       []iamv1.PolicyRule
}

type rbacSnapshot struct {
	mu           sync.Mutex
	expiresAt    time.Time
	epoch        uint64
	hasSelectors bool
	enforcer     *casbin.Enforcer
}

// RBACAuthorizer evaluates GlobalRoleBindings and TenantRoleBindings using pure additive PolicyRules with '*' wildcards
// via the per-resource model operators, accelerated by singleflight and event-invalidated snapshot caching.
type RBACAuthorizer struct {
	reg                *repo.Registry
	globalRoles        globalrolemodel.Operator
	globalRoleBindings globalrolebindingmodel.Operator
	tenantRoles        tenantrolemodel.Operator
	tenantRoleBindings tenantrolebindingmodel.Operator
	instances          instancemodel.Operator

	sf          singleflight.Group
	mu          sync.RWMutex
	epoch       atomic.Uint64
	globalSnap  *rbacSnapshot
	tenantSnaps map[string]*rbacSnapshot
}

func NewRBACAuthorizer(reg *repo.Registry, drivers *drvruntime.Registry, bus mq.Interface) *RBACAuthorizer {
	authz := &RBACAuthorizer{
		reg:                reg,
		globalRoles:        globalrolemodel.NewOperator(reg, bus),
		globalRoleBindings: globalrolebindingmodel.NewOperator(reg, bus),
		tenantRoles:        tenantrolemodel.NewOperator(reg, bus),
		tenantRoleBindings: tenantrolebindingmodel.NewOperator(reg, bus),
		instances:          instancemodel.NewOperator(reg, drivers, bus),
		tenantSnaps:        make(map[string]*rbacSnapshot),
	}
	if bus != nil {
		ctx := context.Background()
		invalidateGlobal := func(_ context.Context, _ mq.Event) error {
			authz.InvalidateGlobalCache()
			return nil
		}
		invalidateTenant := func(_ context.Context, ev mq.Event) error {
			if ev.Tenant != "" {
				authz.InvalidateTenantCache(ev.Tenant)
			} else {
				authz.InvalidateCache()
			}
			return nil
		}
		_, _ = bus.Subscribe(ctx, mq.TopicGlobalRole, "watch-rbac-gr", invalidateGlobal)
		_, _ = bus.Subscribe(ctx, mq.TopicGlobalRoleBinding, "watch-rbac-grb", invalidateGlobal)
		_, _ = bus.Subscribe(ctx, mq.TopicTenantRole, "watch-rbac-tr", invalidateTenant)
		_, _ = bus.Subscribe(ctx, mq.TopicTenantRoleBinding, "watch-rbac-trb", invalidateTenant)
	}
	return authz
}

// InvalidateCache clears all cached RBAC snapshots immediately.
func (r *RBACAuthorizer) InvalidateCache() {
	r.epoch.Add(1)
	r.mu.Lock()
	r.globalSnap = nil
	clear(r.tenantSnaps)
	r.mu.Unlock()
}

// InvalidateGlobalCache clears only the global RBAC snapshot without evicting unrelated tenant caches.
func (r *RBACAuthorizer) InvalidateGlobalCache() {
	r.mu.Lock()
	r.globalSnap = nil
	r.mu.Unlock()
}

// InvalidateTenantCache clears only the target tenant's RBAC snapshot.
func (r *RBACAuthorizer) InvalidateTenantCache(tenant string) {
	if tenant == "" {
		return
	}
	r.mu.Lock()
	delete(r.tenantSnaps, tenant)
	r.mu.Unlock()
}

func isSnapshotFresh(snap *rbacSnapshot, curEpoch uint64, now time.Time) bool {
	if snap == nil || snap.epoch != curEpoch {
		return false
	}
	return now.Before(snap.expiresAt)
}

func (r *RBACAuthorizer) getGlobalSnapshot(ctx context.Context) (*rbacSnapshot, error) {
	curEpoch := r.epoch.Load()
	r.mu.RLock()
	snap := r.globalSnap
	r.mu.RUnlock()
	if isSnapshotFresh(snap, curEpoch, time.Now()) {
		return snap, nil
	}
	v, err, _ := r.sf.Do("global", func() (any, error) {
		rolesList, err := r.globalRoles.List(ctx, db.ListOptions{})
		if err != nil {
			return nil, err
		}
		bindingsList, err := r.globalRoleBindings.List(ctx, db.ListOptions{})
		if err != nil {
			return nil, err
		}
		roleMap := make(map[string][]iamv1.PolicyRule, len(rolesList.Items))
		for _, role := range rolesList.Items {
			if role != nil {
				roleMap[role.Metadata.Name] = role.Rules
			}
		}
		compiled := make([]compiledBinding, 0, len(bindingsList.Items))
		for _, b := range bindingsList.Items {
			if b != nil {
				if rules, ok := roleMap[b.RoleRef.Name]; ok {
					compiled = append(compiled, compiledBinding{bindingName: b.Metadata.Name, subjects: b.Subjects, rules: rules})
				}
			}
		}
		enforcer, hasSelectors := buildCasbinEnforcer("*", compiled)
		built := &rbacSnapshot{expiresAt: time.Now().Add(rbacCacheTTL), epoch: r.epoch.Load(), hasSelectors: hasSelectors, enforcer: enforcer}
		r.mu.Lock()
		r.globalSnap = built
		r.mu.Unlock()
		return built, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*rbacSnapshot), nil
}

func (r *RBACAuthorizer) getTenantSnapshot(ctx context.Context, tenant string) (*rbacSnapshot, error) {
	curEpoch := r.epoch.Load()
	r.mu.RLock()
	snap := r.tenantSnaps[tenant]
	r.mu.RUnlock()
	if isSnapshotFresh(snap, curEpoch, time.Now()) {
		return snap, nil
	}
	v, err, _ := r.sf.Do("tenant:"+tenant, func() (any, error) {
		rolesList, err := r.tenantRoles.List(ctx, db.ListOptions{Tenant: tenant})
		if err != nil {
			return nil, err
		}
		bindingsList, err := r.tenantRoleBindings.List(ctx, db.ListOptions{Tenant: tenant})
		if err != nil {
			return nil, err
		}
		roleMap := make(map[string][]iamv1.PolicyRule, len(rolesList.Items))
		for _, role := range rolesList.Items {
			if role != nil {
				roleMap[role.Metadata.Name] = role.Rules
			}
		}
		compiled := make([]compiledBinding, 0, len(bindingsList.Items))
		for _, b := range bindingsList.Items {
			if b != nil {
				if rules, ok := roleMap[b.RoleRef.Name]; ok {
					compiled = append(compiled, compiledBinding{bindingName: b.Metadata.Name, subjects: b.Subjects, rules: rules})
				}
			}
		}
		enforcer, hasSelectors := buildCasbinEnforcer(tenant, compiled)
		built := &rbacSnapshot{expiresAt: time.Now().Add(rbacCacheTTL), epoch: r.epoch.Load(), hasSelectors: hasSelectors, enforcer: enforcer}
		r.storeTenantSnapshot(tenant, built)
		return built, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*rbacSnapshot), nil
}

func buildCasbinEnforcer(dom string, bindings []compiledBinding) (*casbin.Enforcer, bool) {
	m, err := model.NewModelFromString(casbinRBACModelText)
	if err != nil {
		return nil, false
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, false
	}
	var selectors []*metav1.LabelSelector
	registerCasbinFunctions(e, &selectors)
	for _, b := range bindings {
		roleID := "binding:" + b.bindingName
		for _, s := range b.subjects {
			kind := s.Kind
			if kind == "" {
				kind = "*"
			}
			_, _ = e.AddGroupingPolicy(kind+":"+s.Name, roleID, dom)
		}
		for _, rule := range b.rules {
			selID := "*"
			if rule.Selector != nil && (len(rule.Selector.MatchLabels) > 0 || len(rule.Selector.MatchExpressions) > 0) {
				selID = strconv.Itoa(len(selectors))
				selectors = append(selectors, rule.Selector)
			}
			addCasbinRulePolicies(e, roleID, dom, selID, b.bindingName, rule)
		}
	}
	return e, len(selectors) > 0
}

func registerCasbinFunctions(e *casbin.Enforcer, selectors *[]*metav1.LabelSelector) {
	_ = e.AddNamedMatchingFunc("g", "wcMatch", func(key1, key2 string) bool {
		return metav1.WildcardMatch(key2, key1)
	})
	e.AddFunction("wcMatch", func(args ...any) (any, error) {
		if len(args) < 2 {
			return false, nil
		}
		target, _ := args[0].(string)
		pattern, _ := args[1].(string)
		return metav1.WildcardMatch(pattern, target), nil
	})
	e.AddFunction("labelMatch", func(args ...any) (any, error) {
		if len(args) < 3 {
			return false, nil
		}
		selID, _ := args[2].(string)
		if selID == "" || selID == "*" {
			return true, nil
		}
		idx, err := strconv.Atoi(selID)
		if err != nil || idx < 0 || idx >= len(*selectors) {
			return false, nil
		}
		sel := (*selectors)[idx]
		if len(sel.MatchLabels) == 0 && len(sel.MatchExpressions) == 0 {
			return true, nil
		}
		resName, _ := args[0].(string)
		if resName == "" || resName == "*" {
			return false, nil
		}
		bag, _ := args[1].(labelBag)
		if bag.labels == nil {
			return false, nil
		}
		return sel.Matches(bag.labels), nil
	})
}

func addCasbinRulePolicies(e *casbin.Enforcer, roleID, dom, selID, bindingName string, rule iamv1.PolicyRule) {
	resNames := rule.ResourceNames
	if len(resNames) == 0 {
		resNames = []string{"*"}
	}
	for _, grp := range rule.APIGroups {
		for _, res := range rule.Resources {
			for _, verb := range rule.Verbs {
				for _, rn := range resNames {
					_, _ = e.AddPolicy(roleID, dom, grp, res, verb, rn, selID, bindingName)
				}
			}
		}
	}
}

func (r *RBACAuthorizer) storeTenantSnapshot(tenant string, built *rbacSnapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.tenantSnaps) >= maxCachedTenantSnapshots {
		nowEvict := time.Now()
		evicted := false
		for k, existing := range r.tenantSnaps {
			if existing == nil || nowEvict.After(existing.expiresAt) {
				delete(r.tenantSnaps, k)
				evicted = true
				break
			}
		}
		if !evicted {
			for k := range r.tenantSnaps {
				delete(r.tenantSnaps, k)
				break
			}
		}
	}
	r.tenantSnaps[tenant] = built
}

func (r *RBACAuthorizer) Authorize(ctx context.Context, a AttributesRecord) (Decision, string, error) {
	if a.User.IsAnonymous() {
		return DecisionNoOpinion, "anonymous subject", nil
	}
	labels, decision, reason := r.authorizeNodeAndSelf(ctx, a)
	if decision != DecisionNoOpinion {
		return decision, reason, nil
	}
	return r.authorizeRBAC(ctx, a, labels)
}

func (r *RBACAuthorizer) authorizeNodeAndSelf(ctx context.Context, a AttributesRecord) (map[string]string, Decision, string) {
	targetInst, labels := r.resolveTargetInstanceAndLabels(ctx, a)
	if a.User.Kind == iamv1.SubjectKindNode && a.User.Name == bootstrapNodePrincipal {
		return labels, DecisionDeny, "NodeRestriction: the bootstrap token may only be exchanged for a node session"
	}
	if a.User.Kind == iamv1.SubjectKindNode {
		if reason := nodeRestriction(a, targetInst); reason != "" {
			return labels, DecisionDeny, reason
		}
	}
	qualifiedRes := a.QualifiedResource()
	if a.User.Kind == iamv1.SubjectKindUser && a.User.Name != "" &&
		a.APIGroup == iamv1.GroupName && a.Name == a.User.Name &&
		qualifiedRes == "users" && (a.Verb == iamv1.VerbGet || a.Verb == iamv1.VerbUpdate || a.Verb == iamv1.VerbPatch) {
		return labels, DecisionAllow, "allowed by built-in self-user access rule"
	}
	return labels, DecisionNoOpinion, ""
}

func (r *RBACAuthorizer) authorizeRBAC(ctx context.Context, a AttributesRecord, labels map[string]string) (Decision, string, error) {
	qualifiedRes := a.QualifiedResource()
	labelsLoaded := labels != nil
	globalSnap, err := r.getGlobalSnapshot(ctx)
	if err != nil {
		return DecisionDeny, "", err
	}
	if !labelsLoaded && globalSnap.hasSelectors && a.Name != "" {
		labels = r.loadTargetLabels(ctx, a)
		labelsLoaded = true
	}
	if bName, ok := matchSnapshot(globalSnap, "*", a.User, a.APIGroup, qualifiedRes, a.Verb, a.Name, labels); ok {
		return DecisionAllow, "allowed by GlobalRoleBinding " + bName, nil
	}
	effectiveTenant := a.Tenant
	if effectiveTenant == "" && a.APIGroup == corev1.GroupName && a.Resource == "tenants" && a.Name != "" {
		effectiveTenant = a.Name
	}
	if effectiveTenant != "" {
		tenantSnap, err := r.getTenantSnapshot(ctx, effectiveTenant)
		if err != nil {
			return DecisionDeny, "", err
		}
		if !labelsLoaded && tenantSnap.hasSelectors && a.Name != "" {
			labels = r.loadTargetLabels(ctx, a)
		}
		if bName, ok := matchSnapshot(tenantSnap, effectiveTenant, a.User, a.APIGroup, qualifiedRes, a.Verb, a.Name, labels); ok {
			return DecisionAllow, "allowed by TenantRoleBinding " + bName, nil
		}
	}
	return DecisionNoOpinion, "RBAC: access denied for " + a.APIGroup + "/" + qualifiedRes + " (" + a.Verb + ")", nil
}

func matchSnapshot(snap *rbacSnapshot, dom string, u request.UserInfo, apiGroup, qualifiedRes, verb, name string, labels map[string]string) (string, bool) {
	if snap == nil || snap.enforcer == nil {
		return "", false
	}
	snap.mu.Lock()
	defer snap.mu.Unlock()
	bag := labelBag{labels: labels}
	candidates := []string{u.Kind + ":" + u.Name}
	for _, g := range u.Groups {
		candidates = append(candidates, iamv1.SubjectKindGroup+":"+g)
	}
	for _, sub := range candidates {
		if ok, vals, err := snap.enforcer.EnforceEx(sub, dom, apiGroup, qualifiedRes, verb, name, bag); err == nil && ok && len(vals) >= 8 {
			return vals[7], true
		}
	}
	return "", false
}

func (r *RBACAuthorizer) resolveTargetInstanceAndLabels(ctx context.Context, a AttributesRecord) (*corev1.Instance, map[string]string) {
	labels := a.Labels
	if a.User.Kind == iamv1.SubjectKindNode && a.APIGroup == corev1.GroupName && a.Resource == "instances" && a.Tenant != "" && a.Name != "" {
		if inst, err := r.instances.Get(ctx, a.Tenant, a.Name); err == nil && inst != nil {
			if labels == nil {
				labels = nonNilLabels(inst.Metadata.Labels)
			}
			return inst, labels
		}
	}
	return nil, labels
}
