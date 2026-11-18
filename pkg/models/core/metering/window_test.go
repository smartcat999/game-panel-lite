package metering

import (
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestBuildInstanceFactDataGameFallback(t *testing.T) {
	now := time.Date(2026, 11, 11, 2, 0, 0, 0, time.UTC)
	snap := observationSnapshot{
		attemptID: "att-1",
		epoch:     1,
		revision:  1,
		resources: corev1.ResourceList{
			corev1.ResourceCPU:     "2",
			corev1.ResourceMemory:  "4Gi",
			corev1.ResourceStorage: "20Gi",
		},
	}

	appOnly := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			Name:   "alpha-mc-01",
			UID:    "uid-alpha",
			Tenant: "studio-alpha",
			Labels: map[string]string{corev1.LabelInstanceApp: "minecraft-paper"},
		},
	}
	_, data := buildInstanceFactData(appOnly, FactInstanceRunning, snap, now, []string{"run-1"})
	if data["game"] != "minecraft-paper" {
		t.Fatalf("expected LabelInstanceApp fallback minecraft-paper, got %q", data["game"])
	}

	unlabeled := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "raw-01", UID: "uid-raw", Tenant: "studio-alpha"},
	}
	_, fallbackData := buildInstanceFactData(unlabeled, FactInstanceRunning, snap, now, []string{"run-2"})
	if fallbackData["game"] != DefaultGameName {
		t.Fatalf("expected default game %q, got %q", DefaultGameName, fallbackData["game"])
	}
}

func TestComputeWindowFromInputsAppLabelFallback(t *testing.T) {
	now := time.Date(2026, 11, 11, 4, 0, 0, 0, time.UTC)
	tenantObj := &corev1.Tenant{
		Metadata: metav1.ObjectMeta{Name: "studio-alpha", UID: "t-uid-1"},
		Spec:     corev1.TenantSpec{DisplayName: "Studio Alpha", Type: corev1.TenantTypeOrganization},
		Status:   corev1.TenantStatus{Phase: corev1.TenantPhaseActive},
	}
	liveInst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			Name:   "alpha-mc-01",
			UID:    "uid-alpha",
			Tenant: "studio-alpha",
			Labels: map[string]string{corev1.LabelInstanceApp: "minecraft-paper"},
		},
		Status: corev1.InstanceStatus{
			Phase: corev1.InstancePhaseRunning,
			ResourceControl: &corev1.InstanceResourceControl{
				TenantUID:        "t-uid-1",
				Stage:            corev1.InstanceStageActive,
				ApprovedRevision: 1,
				ApprovedResources: corev1.ResourceList{
					corev1.ResourceCPU: "2", corev1.ResourceMemory: "2Gi", corev1.ResourceStorage: "10Gi",
				},
			},
		},
	}
	inputs := &meteringInputs{
		now:           now,
		tenants:       []*corev1.Tenant{tenantObj},
		byTenant:      map[string][]*corev1.Instance{"studio-alpha": {liveInst}},
		factsByTenant: map[string][]corev1.InstanceExecutionFactRecord{},
	}

	op := &operator{}
	items, err := op.computeWindowFromInputs(Window24h, inputs)
	if err != nil {
		t.Fatalf("computeWindowFromInputs returned error: %v", err)
	}
	if len(items) != 1 || len(items[0].Instances) != 1 {
		t.Fatalf("unexpected metering items: %+v", items)
	}
	if got := items[0].Instances[0].Game; got != "minecraft-paper" {
		t.Fatalf("expected live instance game minecraft-paper, got %q", got)
	}
}
