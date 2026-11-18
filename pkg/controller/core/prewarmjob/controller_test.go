package prewarmjob

import (
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestJobNeedsResyncWhenNewNodeJoins(t *testing.T) {
	job := &corev1.PrewarmJob{
		Metadata: metav1.ObjectMeta{Name: "prewarm-terraria-1-4-5-8", Generation: 1},
		Status: corev1.PrewarmJobStatus{
			ObservedGeneration: 1,
			Phase:              corev1.PrewarmJobPhaseSucceeded,
			TargetNodes:        1,
			ReadyNodes:         1,
			Nodes: []corev1.PrewarmNodeStatus{
				{NodeName: "node-1", Phase: corev1.PrewarmJobPhaseSucceeded},
			},
		},
	}
	existingNodes := []*corev1.Node{
		{Metadata: metav1.ObjectMeta{Name: "node-1"}},
	}
	if jobNeedsResync(job, existingNodes) {
		t.Fatal("expected Succeeded job with unchanged node set not to require resync")
	}

	expandedNodes := []*corev1.Node{
		{Metadata: metav1.ObjectMeta{Name: "node-1"}},
		{Metadata: metav1.ObjectMeta{Name: "node-2"}},
	}
	if !jobNeedsResync(job, expandedNodes) {
		t.Fatal("expected Succeeded cluster-wide prewarm job to resync when a new node joins")
	}
}
