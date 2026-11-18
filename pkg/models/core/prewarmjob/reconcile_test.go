package prewarmjob

import (
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestNodeMatchesJobFiltersByAllowedApps(t *testing.T) {
	apps := []*corev1.App{
		{
			Metadata:      metav1.ObjectMeta{Name: "minecraft-paper"},
			ServerBinding: &corev1.AppServerBinding{Repository: "mc-paper"},
		},
		{
			Metadata:      metav1.ObjectMeta{Name: "rust-dedicated"},
			ServerBinding: &corev1.AppServerBinding{Repository: "rust-server"},
		},
	}
	repoToApp := BuildRepoToAppMap(apps)
	if repoToApp["mc-paper"] != "minecraft-paper" || repoToApp["rust-server"] != "rust-dedicated" {
		t.Fatalf("unexpected repoToApp: %v", repoToApp)
	}

	nodeFull := &corev1.Node{Metadata: metav1.ObjectMeta{Name: "um773", UID: "u1"}}
	nodeLimited := &corev1.Node{
		Metadata: metav1.ObjectMeta{Name: "home-vm01", UID: "u2"},
		Spec:     corev1.NodeSpec{AllowedApps: []string{"minecraft-paper"}},
	}
	jobRust := &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{Name: "auto-default-rust-dedicated", Labels: map[string]string{corev1.LabelInstanceApp: "rust-dedicated"}},
		Spec:     corev1.PrewarmJobSpec{Artifacts: []corev1.ArtifactRef{{Repository: "rust-server", Package: "rust", Version: "latest"}}},
	}
	jobMC := &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{Name: "auto-default-minecraft-paper"},
		Spec:     corev1.PrewarmJobSpec{Artifacts: []corev1.ArtifactRef{{Repository: "mc-paper", Package: "paper", Version: "26.3"}}},
	}

	if !NodeMatchesJob(nodeFull, jobRust, repoToApp) || NodeMatchesJob(nodeLimited, jobRust, repoToApp) {
		t.Fatal("expected rust prewarm job to match unrestricted node only")
	}
	if !NodeMatchesJob(nodeLimited, jobMC, repoToApp) {
		t.Fatal("expected mc prewarm job to match allowed node via repoToApp")
	}
	st := buildDesiredJobStatus(jobRust, []*corev1.Node{nodeFull, nodeLimited}, repoToApp)
	if st.TargetNodes != 1 || len(st.Nodes) != 1 || st.Nodes[0].NodeName != "um773" {
		t.Fatalf("expected only um773 targeted, got %+v", st)
	}
}
