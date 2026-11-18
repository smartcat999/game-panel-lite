package runtime

import (
	"context"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestVerifyDockerReadAccessNonBlockingAndFenced(t *testing.T) {
	workDir := t.TempDir()
	in := &corev1.Instance{
		Metadata: metav1.ObjectMeta{UID: "inst-uid-1", Generation: 2},
		Spec:     corev1.InstanceSpec{NodeName: "node-1"},
		Status: corev1.InstanceStatus{
			ResourceControl: &corev1.InstanceResourceControl{
				Stage:            corev1.InstanceStageActive,
				ApprovedRevision: 3,
				CurrentAttempt:   &corev1.InstancePlacementAttempt{ID: "att-1", NodeUID: "node-uid-1", NodeName: "node-1", Epoch: 1, Phase: corev1.AttemptPhaseBound},
			},
		},
	}
	if err := verifyDockerReadAccess(workDir, in); err != nil {
		t.Fatalf("read access before journal write: %v", err)
	}

	release, err := beginDockerExecution(context.Background(), workDir, in)
	if err != nil {
		t.Fatal(err)
	}
	release()

	_, _, privatePath, err := resolveExecutionJournalLocation(workDir, in.Metadata.UID)
	if err != nil {
		t.Fatal(err)
	}
	held, err := lockExecution(context.Background(), privatePath)
	if err != nil {
		t.Fatal(err)
	}
	defer held()

	// Must not block while exclusive execution lock is held.
	done := make(chan error, 1)
	go func() {
		done <- verifyDockerReadAccess(workDir, in)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("read access while lock held: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("verifyDockerReadAccess blocked while execution lock was held")
	}

	newer := *in
	newer.Metadata.Generation = 3
	if err := verifyDockerReadAccess(workDir, &newer); err != nil {
		t.Fatalf("read access for newer generation while lock held: %v", err)
	}
	stale := *in
	stale.Metadata.Generation = 1
	if err := verifyDockerReadAccess(workDir, &stale); err == nil {
		t.Fatal("expected read access for stale generation to fail")
	}
}
