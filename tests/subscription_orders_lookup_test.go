package tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// verifyQuotaChangeOrderBatchLookup mirrors the console flow: list quota changes, then resolve the
// referenced orders with one batch call on the orders resource (no server-side cross-table join).
func verifyQuotaChangeOrderBatchLookup(t *testing.T, h *securityTestHarness, token, tenant string) {
	t.Helper()
	var changes metav1.List[*corev1.QuotaPolicyChange]
	code, raw := h.doGET(t, token, "/apis/core.gamepanel.io/v1/tenants/"+tenant+"/quotachanges?limit=50")
	if code != http.StatusOK || json.Unmarshal(raw, &changes) != nil {
		t.Fatalf("list quotachanges: %d %s", code, raw)
	}
	idSet := map[string]bool{}
	toVersions := map[int64]bool{}
	for _, c := range changes.Items {
		toVersions[c.ToVersion] = true
		for _, id := range c.OrderIDs {
			idSet[id] = true
		}
	}
	if len(idSet) < 2 {
		t.Fatalf("expected quota changes to reference at least 2 orders, got %v", idSet)
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	orders := listOrdersBySelector(t, h, token, tenant, "metadata.name.in="+strings.Join(ids, ","))
	if len(orders.Items) != len(ids) || orders.Metadata.TotalCount != len(ids) {
		t.Fatalf("batch lookup returned %d/%d orders (total=%d), want %d", len(orders.Items), len(ids), orders.Metadata.TotalCount, len(ids))
	}
	matchedTrigger := false
	for _, o := range orders.Items {
		if !idSet[o.ID] || o.Metadata.Name != o.ID {
			t.Fatalf("unexpected order %+v in batch lookup", o.Metadata)
		}
		matchedTrigger = matchedTrigger || toVersions[o.PolicyVersion]
	}
	if !matchedTrigger {
		t.Fatalf("expected an order.policyVersion to equal some change.toVersion (trigger linkage)")
	}
	single := listOrdersBySelector(t, h, token, tenant, "metadata.name="+ids[0])
	if len(single.Items) != 1 || single.Items[0].ID != ids[0] {
		t.Fatalf("metadata.name exact lookup mismatch: %+v", single.Items)
	}
	bad, _ := h.doGET(t, token, "/apis/core.gamepanel.io/v1/tenants/"+tenant+"/orders?fieldSelector="+url.QueryEscape("tenantUID=x"))
	if bad != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported order fieldSelector, got %d", bad)
	}
}

func listOrdersBySelector(t *testing.T, h *securityTestHarness, token, tenant, selector string) metav1.List[corev1.QuotaOrder] {
	t.Helper()
	var out metav1.List[corev1.QuotaOrder]
	code, raw := h.doGET(t, token, "/apis/core.gamepanel.io/v1/tenants/"+tenant+"/orders?fieldSelector="+url.QueryEscape(selector))
	if code != http.StatusOK || json.Unmarshal(raw, &out) != nil {
		t.Fatalf("list orders fieldSelector=%s: %d %s", selector, code, raw)
	}
	return out
}
