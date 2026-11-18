package alert

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

func parseSilenceTime(raw string) (time.Time, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

// NormalizeAlertSilences validates, normalizes, and prunes expired maintenance silences at now.
func NormalizeAlertSilences(list []corev1.AlertSilence, idPrefix, defaultScope string, isPlatform bool, now time.Time) []corev1.AlertSilence {
	out := make([]corev1.AlertSilence, 0, len(list))
	for i, item := range list {
		norm, ok := normalizeSingleSilence(item, idPrefix, defaultScope, isPlatform, now, i)
		if ok {
			out = append(out, norm)
		}
	}
	return out
}

func normalizeSingleSilence(s corev1.AlertSilence, idPrefix, defaultScope string, isPlatform bool, now time.Time, idx int) (corev1.AlertSilence, bool) {
	exp, ok := parseSilenceTime(s.ExpiresAt)
	if !ok || !exp.After(now) {
		return corev1.AlertSilence{}, false
	}
	kind := strings.TrimSpace(s.TargetKind)
	if kind == "" {
		kind = corev1.SilenceTargetWildcard
	}
	if kind != corev1.SilenceTargetInstances && kind != corev1.SilenceTargetNodes && kind != corev1.SilenceTargetWildcard {
		return corev1.AlertSilence{}, false
	}
	if !isPlatform && kind == corev1.SilenceTargetNodes {
		return corev1.AlertSilence{}, false
	}
	s.TargetKind = kind
	s.TargetName = strings.TrimSpace(s.TargetName)
	if s.TargetName == "" {
		s.TargetName = corev1.SilenceTargetWildcard
	}
	s.RuleID = strings.TrimSpace(s.RuleID)
	if s.RuleID == corev1.SilenceTargetWildcard {
		s.RuleID = ""
	}
	s.Reason = strings.TrimSpace(s.Reason)
	s.CreatedBy = strings.TrimSpace(s.CreatedBy)
	s.Scope = normalizeSilenceScope(s.Scope, defaultScope, isPlatform)
	start, hasStart := parseSilenceTime(s.StartsAt)
	if !hasStart {
		start = now.UTC()
	}
	s.StartsAt = start.Format(time.RFC3339)
	s.ExpiresAt = exp.Format(time.RFC3339)
	s.ID = strings.TrimSpace(s.ID)
	if s.ID == "" {
		s.ID = fmt.Sprintf("%s%d-%d", idPrefix, now.Unix(), idx+1)
	}
	return s, true
}

func normalizeSilenceScope(raw, defaultScope string, isPlatform bool) string {
	if !isPlatform {
		if defaultScope != "" {
			return defaultScope
		}
		return strings.TrimSpace(raw)
	}
	sc := strings.TrimSpace(raw)
	if sc == "" || sc == corev1.SilenceTargetWildcard {
		return corev1.AlertScopeAll
	}
	return sc
}

// ExtractActiveTenantSilences returns non-expired workspace AlertSilences for a Tenant.
func ExtractActiveTenantSilences(t *corev1.Tenant, now time.Time) []corev1.AlertSilence {
	if t == nil {
		return nil
	}
	tName := strings.TrimSpace(t.Metadata.Name)
	if t.Spec.AlertSilences != nil {
		return NormalizeAlertSilences(t.Spec.AlertSilences, "sil-", tName, false, now)
	}
	return unmarshalSilencesAnnotation(t.Metadata.Annotations[corev1.AnnotationAlertSilences], "sil-", tName, false, now)
}

// ExtractActivePlatformSilences returns non-expired cluster/platform AlertSilences hosted on a carrier Tenant.
func ExtractActivePlatformSilences(t *corev1.Tenant, now time.Time) []corev1.AlertSilence {
	if t == nil {
		return nil
	}
	if t.Spec.PlatformAlertSilences != nil {
		return NormalizeAlertSilences(t.Spec.PlatformAlertSilences, "psil-", corev1.AlertScopeAll, true, now)
	}
	return unmarshalSilencesAnnotation(t.Metadata.Annotations[corev1.AnnotationPlatformAlertSilences], "psil-", corev1.AlertScopeAll, true, now)
}

func unmarshalSilencesAnnotation(raw, idPrefix, defaultScope string, isPlatform bool, now time.Time) []corev1.AlertSilence {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var list []corev1.AlertSilence
	if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
		return nil
	}
	return NormalizeAlertSilences(list, idPrefix, defaultScope, isPlatform, now)
}

// LoadActivePlatformSilences loads active platform-level AlertSilences across carrier tenants.
func LoadActivePlatformSilences(ctx context.Context, reg *repo.Registry, now time.Time) []corev1.AlertSilence {
	if reg == nil || reg.Pool() == nil {
		return nil
	}
	tenantDB := tenantdb.NewDB(reg.Pool())
	if tenantDB == nil {
		return nil
	}
	list, err := tenantDB.List(ctx, db.ListOptions{})
	if err != nil || list == nil {
		return nil
	}
	var out []corev1.AlertSilence
	for _, t := range list.Items {
		if t != nil {
			out = append(out, ExtractActivePlatformSilences(t, now)...)
		}
	}
	return out
}

// MatchAlertSilence checks workspace and platform silences and returns the active silence with the latest expiry.
func MatchAlertSilence(tenantSilences, platformSilences []corev1.AlertSilence, tenant, targetKind, targetName, ruleID string, now time.Time) (corev1.AlertSilence, bool) {
	var best corev1.AlertSilence
	var bestExp time.Time
	consider := func(s corev1.AlertSilence, isPlatform bool) {
		exp, ok := silenceMatchesTarget(s, isPlatform, tenant, targetKind, targetName, ruleID, now)
		if ok && (bestExp.IsZero() || exp.After(bestExp)) {
			best = s
			bestExp = exp
		}
	}
	for _, s := range tenantSilences {
		consider(s, false)
	}
	for _, s := range platformSilences {
		consider(s, true)
	}
	return best, !bestExp.IsZero()
}

func silenceMatchesTarget(s corev1.AlertSilence, isPlatform bool, tenant, targetKind, targetName, ruleID string, now time.Time) (time.Time, bool) {
	exp, ok := parseSilenceTime(s.ExpiresAt)
	if !ok || !exp.After(now) {
		return time.Time{}, false
	}
	if start, hasStart := parseSilenceTime(s.StartsAt); hasStart && now.Before(start) {
		return time.Time{}, false
	}
	if !silenceScopeMatches(s.Scope, isPlatform, tenant, targetKind) {
		return time.Time{}, false
	}
	if s.TargetKind != "" && s.TargetKind != corev1.SilenceTargetWildcard && s.TargetKind != targetKind {
		return time.Time{}, false
	}
	if s.TargetName != "" && s.TargetName != corev1.SilenceTargetWildcard && s.TargetName != targetName {
		return time.Time{}, false
	}
	if s.RuleID != "" && s.RuleID != corev1.SilenceTargetWildcard && (ruleID == "" || s.RuleID != ruleID) {
		return time.Time{}, false
	}
	return exp, true
}

func silenceScopeMatches(silenceScope string, isPlatform bool, tenant, targetKind string) bool {
	sc := strings.TrimSpace(silenceScope)
	if !isPlatform {
		if targetKind == corev1.SilenceTargetNodes || tenant == "" {
			return false
		}
		return sc == "" || sc == tenant
	}
	if sc == "" || sc == corev1.AlertScopeAll || sc == corev1.SilenceTargetWildcard {
		return true
	}
	if sc == corev1.AlertScopePlatform {
		return tenant == "" || targetKind == corev1.SilenceTargetNodes
	}
	return tenant == sc
}

// IsAlertEventSilenced returns true when an alertEvent is muted by an active workspace or platform maintenance window.
func IsAlertEventSilenced(ctx context.Context, reg *repo.Registry, carrier *corev1.Tenant, ev alertEvent, now time.Time) bool {
	if ev.State == corev1.AlertStateTest {
		return false
	}
	var tenantSilences []corev1.AlertSilence
	if carrier != nil && ev.Scope == corev1.AlertScopeTenant {
		tenantSilences = ExtractActiveTenantSilences(carrier, now)
	}
	platformSilences := LoadActivePlatformSilences(ctx, reg, now)
	_, silenced := MatchAlertSilence(tenantSilences, platformSilences, ev.Tenant, ev.TargetKind, ev.TargetName, ev.RuleID, now)
	return silenced
}

func filterUnsilencedNotificationItems(ctx context.Context, reg *repo.Registry, scope, carrierTenant string, items []notificationItem, now time.Time) []notificationItem {
	if len(items) == 0 || reg == nil || reg.Pool() == nil {
		return items
	}
	var tenantSilences []corev1.AlertSilence
	if scope == corev1.AlertScopeTenant && carrierTenant != "" {
		if tDB := tenantdb.NewDB(reg.Pool()); tDB != nil {
			if t, err := tDB.Get(ctx, carrierTenant); err == nil && t != nil {
				tenantSilences = ExtractActiveTenantSilences(t, now)
			}
		}
	}
	platformSilences := LoadActivePlatformSilences(ctx, reg, now)
	if len(tenantSilences) == 0 && len(platformSilences) == 0 {
		return items
	}
	out := make([]notificationItem, 0, len(items))
	for _, it := range items {
		if it.State != corev1.AlertStateTest {
			if _, ok := MatchAlertSilence(tenantSilences, platformSilences, it.Tenant, it.TargetKind, it.TargetName, it.RuleID, now); ok {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

func overlayActiveSilences(ctx context.Context, reg *repo.Registry, alerts []*corev1.Alert, now time.Time) []*corev1.Alert {
	if len(alerts) == 0 || reg == nil || reg.Pool() == nil {
		return alerts
	}
	tenantDB := tenantdb.NewDB(reg.Pool())
	if tenantDB == nil {
		return alerts
	}
	tList, err := tenantDB.List(ctx, db.ListOptions{})
	if err != nil || tList == nil {
		return alerts
	}
	byTenant := make(map[string][]corev1.AlertSilence, len(tList.Items))
	var platformSilences []corev1.AlertSilence
	for _, t := range tList.Items {
		if t == nil {
			continue
		}
		if ts := ExtractActiveTenantSilences(t, now); len(ts) > 0 {
			byTenant[t.Metadata.Name] = ts
		}
		platformSilences = append(platformSilences, ExtractActivePlatformSilences(t, now)...)
	}
	out := make([]*corev1.Alert, len(alerts))
	for i, a := range alerts {
		if a == nil {
			continue
		}
		cp := *a
		cp.Silenced, cp.SilencedUntil, cp.SilenceID = false, "", ""
		if cp.State != corev1.AlertStateResolved {
			ruleID := ""
			if cp.Metadata.Labels != nil {
				ruleID = cp.Metadata.Labels["ruleID"]
			}
			if matched, ok := MatchAlertSilence(byTenant[cp.Metadata.Tenant], platformSilences, cp.Metadata.Tenant, cp.Resource, cp.TargetName, ruleID, now); ok {
				cp.Silenced = true
				cp.SilencedUntil = matched.ExpiresAt
				cp.SilenceID = matched.ID
			}
		}
		out[i] = &cp
	}
	return out
}

// PruneTargetSilences removes target-specific snoozes for (tenant, targetKind, targetName) while preserving wildcard silences.
func PruneTargetSilences(list []corev1.AlertSilence, isPlatform bool, tenant, targetKind, targetName string) ([]corev1.AlertSilence, bool) {
	tName := strings.TrimSpace(targetName)
	if len(list) == 0 || tName == "" || tName == corev1.SilenceTargetWildcard {
		return list, false
	}
	out := make([]corev1.AlertSilence, 0, len(list))
	changed := false
	for _, s := range list {
		sameTarget := s.TargetKind == targetKind && s.TargetName == tName
		sameScope := !isPlatform || targetKind == corev1.SilenceTargetNodes || s.Scope == "" || s.Scope == corev1.AlertScopeAll || s.Scope == tenant
		if sameTarget && sameScope {
			changed = true
			continue
		}
		out = append(out, s)
	}
	return out, changed
}

// ClearTargetSilences prunes temporary incident snoozes targeting a specific instance or node once it recovers or stops.
func ClearTargetSilences(ctx context.Context, reg *repo.Registry, bus mq.Interface, tenant, targetKind, targetName string, now time.Time) {
	if reg == nil || reg.Pool() == nil || strings.TrimSpace(targetName) == "" || targetName == corev1.SilenceTargetWildcard {
		return
	}
	tenantDB := tenantdb.NewDB(reg.Pool())
	if tenantDB == nil {
		return
	}
	tList, err := tenantDB.List(ctx, db.ListOptions{})
	if err != nil || tList == nil {
		return
	}
	for _, t := range tList.Items {
		if t != nil {
			persistPrunedTenantSilences(ctx, tenantDB, bus, t, tenant, targetKind, targetName, now)
		}
	}
}

func persistPrunedTenantSilences(ctx context.Context, tenantDB *tenantdb.DB, bus mq.Interface, t *corev1.Tenant, tenant, targetKind, targetName string, now time.Time) {
	dirty := false
	if targetKind == corev1.SilenceTargetInstances && t.Metadata.Name == tenant {
		if next, changed := PruneTargetSilences(ExtractActiveTenantSilences(t, now), false, tenant, targetKind, targetName); changed {
			t.Spec.AlertSilences = next
			setSilenceAnnotationJSON(t, corev1.AnnotationAlertSilences, next)
			dirty = true
		}
	}
	if nextP, changedP := PruneTargetSilences(ExtractActivePlatformSilences(t, now), true, tenant, targetKind, targetName); changedP {
		t.Spec.PlatformAlertSilences = nextP
		setSilenceAnnotationJSON(t, corev1.AnnotationPlatformAlertSilences, nextP)
		dirty = true
	}
	if !dirty {
		return
	}
	if updated, err := tenantDB.Update(ctx, t); err == nil && updated != nil && bus != nil {
		_ = bus.Publish(ctx, mq.TopicTenant, mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Tenant", "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion))
	}
}

func setSilenceAnnotationJSON(t *corev1.Tenant, key string, list []corev1.AlertSilence) {
	if t.Metadata.Annotations == nil {
		t.Metadata.Annotations = make(map[string]string)
	}
	if b, err := json.Marshal(list); err == nil {
		t.Metadata.Annotations[key] = string(b)
	}
}

func clearResolvedTargetSilencesIfHealthy(ctx context.Context, reg *repo.Registry, bus mq.Interface, ev alertEvent, now time.Time) {
	if ev.State != corev1.AlertStateResolved {
		return
	}
	if hasAnyFiringAlertForTarget(ctx, reg, ev.Tenant, ev.TargetKind, ev.TargetName) {
		return
	}
	ClearTargetSilences(ctx, reg, bus, ev.Tenant, ev.TargetKind, ev.TargetName, now)
}

func hasAnyFiringAlertForTarget(ctx context.Context, reg *repo.Registry, tenant, targetKind, targetName string) bool {
	if targetKind == corev1.SilenceTargetInstances {
		if firing, _ := isAlertKeyFiring(fmt.Sprintf("inst:phase:%s/%s", tenant, targetName)); firing {
			return true
		}
		for _, r := range LoadClusterAlertRules(ctx, reg) {
			if firing, _ := isAlertKeyFiring(fmt.Sprintf("inst:%s/%s:%s", tenant, targetName, r.ID)); firing {
				return true
			}
		}
		return false
	}
	if targetKind == corev1.SilenceTargetNodes {
		if firing, _ := isAlertKeyFiring("node:phase:" + targetName); firing {
			return true
		}
		if firing, _ := isAlertKeyFiring("node:clock:" + targetName); firing {
			return true
		}
		for _, r := range LoadClusterAlertRules(ctx, reg) {
			if firing, _ := isAlertKeyFiring(fmt.Sprintf("node:%s:%s", targetName, r.ID)); firing {
				return true
			}
		}
	}
	return false
}
