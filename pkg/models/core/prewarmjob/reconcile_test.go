package prewarmjob

import (
	"testing"
	"time"

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

func TestBuildDesiredJobStatusEmptyTargetNodesIdempotent(t *testing.T) {
	cst := time.FixedZone("CST", 8*3600)
	ts := time.Date(2026, 11, 20, 21, 46, 30, 310603703, cst)
	cur := &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{Name: "auto-default-counter-strike-2", Generation: 1},
		Spec:     corev1.PrewarmJobSpec{Images: []string{"cs2:latest"}},
		Status: corev1.PrewarmJobStatus{
			ObservedGeneration: 1,
			Phase:              corev1.PrewarmJobPhasePending,
			Nodes:              nil,
			Conditions: []metav1.Condition{{
				Type:               metav1.ConditionTypeReconciled,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: 1,
				LastTransitionTime: ts,
				Reason:             corev1.ReasonPrewarmPending,
				Message:            "prewarm job targeting 0 node(s)",
			}},
		},
	}
	next := buildDesiredJobStatus(cur, nil, nil)
	if next.Nodes != nil {
		t.Fatalf("expected nil Nodes slice for 0 targets, got %#v", next.Nodes)
	}
	if !prewarmJobStatusEqual(cur.Status, next) {
		t.Fatalf("expected idempotent status equality for 0-target job: cur=%+v next=%+v", cur.Status, next)
	}

	cur.Status.Conditions[0].Reason = "OldReason"
	cur.Status.Conditions[0].Message = "old message"
	updated := buildDesiredJobStatus(cur, nil, nil)
	if cur.Status.Conditions[0].Reason != "OldReason" || cur.Status.Conditions[0].Message != "old message" {
		t.Fatalf("expected cur.Status.Conditions not to be mutated in-place: %+v", cur.Status.Conditions[0])
	}
	if prewarmJobStatusEqual(cur.Status, updated) {
		t.Fatal("expected status equality to detect changed condition reason/message")
	}
}
