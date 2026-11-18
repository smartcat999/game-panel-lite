package instance

import (
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestEligibleRespectsNodeAllowedApps(t *testing.T) {
	rc := &corev1.RuntimeClass{Handler: "docker"}
	nodeLimited := &corev1.Node{
		Metadata: metav1.ObjectMeta{Name: "home-vm01", UID: "u2", Labels: map[string]string{corev1.LabelKubernetesHostname: "home-vm01"}},
		Spec:     corev1.NodeSpec{AllowedApps: []string{"minecraft-paper", "dont-starve-together"}},
		Status: corev1.NodeStatus{
			Phase:    corev1.NodePhaseReady,
			Runtimes: []corev1.NodeRuntime{{Handler: "docker", Version: "27.5"}},
		},
	}
	instMC := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "mc-1", Tenant: "admin", Labels: map[string]string{corev1.LabelInstanceApp: "minecraft-paper"}},
		Spec:     corev1.InstanceSpec{NodeSelector: map[string]string{corev1.LabelKubernetesHostname: "home-vm01"}},
	}
	instRust := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "rust-1", Tenant: "admin", Labels: map[string]string{corev1.LabelInstanceApp: "rust-dedicated"}},
		Spec:     corev1.InstanceSpec{NodeSelector: map[string]string{corev1.LabelKubernetesHostname: "home-vm01"}},
	}
	instRaw := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "raw-1", Tenant: "admin"},
		Spec:     corev1.InstanceSpec{NodeSelector: map[string]string{corev1.LabelKubernetesHostname: "home-vm01"}},
	}

	if !eligible(nodeLimited, instMC, rc) {
		t.Fatal("expected allowed app instance to be eligible on restricted node")
	}
	if eligible(nodeLimited, instRust, rc) {
		t.Fatal("expected disallowed app instance to be rejected on restricted node")
	}
	if !eligible(nodeLimited, instRaw, rc) {
		t.Fatal("expected generic instance without app label to remain eligible")
	}
}
