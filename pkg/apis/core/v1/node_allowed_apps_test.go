package v1

import (
	"testing"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestNodeAllowedAppsHelpers(t *testing.T) {
	got := NormalizeAllowedApps([]string{" minecraft-paper ", "", "dst-server", "minecraft-paper"})
	if len(got) != 2 || got[0] != "minecraft-paper" || got[1] != "dst-server" {
		t.Fatalf("unexpected NormalizeAllowedApps result: %v", got)
	}

	unrestricted := &Node{Metadata: metav1.ObjectMeta{Name: "n1"}}
	if !unrestricted.AllowsApp("rust-dedicated") {
		t.Fatal("node with empty allowedApps should allow every app")
	}

	restricted := &Node{
		Metadata: metav1.ObjectMeta{Name: "n2"},
		Spec:     NodeSpec{AllowedApps: []string{"minecraft-paper", "dont-starve-together"}},
	}
	if !restricted.AllowsApp("minecraft-paper") || restricted.AllowsApp("rust-dedicated") {
		t.Fatalf("unexpected AllowsApp behavior for %v", restricted.Spec.AllowedApps)
	}
	if !restricted.AllowsApp("") {
		t.Fatal("empty app label should remain allowed")
	}

	mc := &Instance{Metadata: metav1.ObjectMeta{Name: "i1", Labels: map[string]string{LabelInstanceApp: "minecraft-paper"}}}
	rust := &Instance{Metadata: metav1.ObjectMeta{Name: "i2", Labels: map[string]string{LabelInstancePreset: "rust-dedicated"}}}
	raw := &Instance{Metadata: metav1.ObjectMeta{Name: "i3"}}
	if !restricted.AllowsInstance(mc) || restricted.AllowsInstance(rust) || !restricted.AllowsInstance(raw) {
		t.Fatal("unexpected AllowsInstance filtering result")
	}

	invalid := &Node{Metadata: metav1.ObjectMeta{Name: "n3"}, Spec: NodeSpec{AllowedApps: []string{"bad/app"}}}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected invalid allowedApps entry to fail Validate")
	}
}
