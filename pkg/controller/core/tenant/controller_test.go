package tenant

import (
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestNeedsReconcileIgnoresUnpersistedReserved(t *testing.T) {
	instances := []*corev1.Instance{
		{
			Status: corev1.InstanceStatus{
				Usage: &corev1.InstanceUsage{CPUMillicores: 2000, MemoryBytes: 4 << 30, StorageBytes: 20 << 30},
				Backups: []corev1.InstanceBackupRecord{
					{UID: "bk-1", SizeBytes: 512 << 20},
				},
			},
		},
	}
	expectedUsage := computeUsage(instances)
	tenant := &corev1.Tenant{
		Metadata: metav1.ObjectMeta{Name: "studio-alpha", Generation: 2},
		Status: corev1.TenantStatus{
			ObservedGeneration: 2,
			Phase:              corev1.TenantPhaseActive,
			Reserved:           nil, // tenantdb does not persist Status.Reserved.
			Usage:              expectedUsage,
		},
	}
	if needsReconcile(tenant, expectedUsage) {
		t.Fatal("expected tenant with matching usage and nil Reserved not to require status write")
	}

	tenant.Status.Usage = corev1.CanonicalResources(corev1.Resources{})
	if !needsReconcile(tenant, expectedUsage) {
		t.Fatal("expected changed usage to trigger reconcile")
	}

	tenant.Status.Usage = expectedUsage
	tenant.Status.ObservedGeneration = 1
	if !needsReconcile(tenant, expectedUsage) {
		t.Fatal("expected stale observedGeneration to trigger reconcile")
	}

	tenant.Status.ObservedGeneration = 2
	tenant.Status.Phase = corev1.TenantPhaseSuspended
	if !needsReconcile(tenant, expectedUsage) {
		t.Fatal("expected non-Active phase to trigger reconcile")
	}
}
