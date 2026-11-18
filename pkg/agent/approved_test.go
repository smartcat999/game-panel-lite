package agent

import (
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

func testApprovedFixture(suspended bool) *corev1.Instance {
	res := corev1.ResourceList{
		corev1.ResourceCPU:       "1",
		corev1.ResourceMemory:    "1Gi",
		corev1.ResourceStorage:   "10Gi",
		corev1.ResourceInstances: "1",
	}
	return &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "srv-1", UID: "inst-uid-1", Tenant: "ws-1", Generation: 1},
		Spec: corev1.InstanceSpec{
			RuntimeClassName: "docker",
			NodeName:         "node-1",
			State:            corev1.InstanceStateRunning,
			Image:            "alpine:3.20",
			Resources:        corev1.ResourceRequirements{Requests: res, Limits: res},
		},
		Status: corev1.InstanceStatus{
			Phase: corev1.InstancePhaseRunning,
			ResourceControl: &corev1.InstanceResourceControl{
				Stage:             corev1.InstanceStageActive,
				ApprovedRevision:  2,
				Suspended:         suspended,
				RuntimeClass:      &corev1.InstanceRuntimeBinding{UID: "rc-uid-1", Name: "docker", Handler: "docker"},
				ApprovedResources: res,
				ApprovedPorts:     []corev1.InstancePort{{TargetPort: 25565, PublishedPort: 25565, Protocol: corev1.ProtocolTCP}},
				CurrentAttempt: &corev1.InstancePlacementAttempt{
					ID: "att-1", NodeUID: "node-uid-1", NodeName: "node-1", Epoch: 1, Phase: corev1.AttemptPhaseBound,
				},
			},
		},
	}
}

func TestApprovedInstanceSuspendedFreezeAndObservation(t *testing.T) {
	unsuspended := testApprovedFixture(false)
	suspended := testApprovedFixture(true)

	execOut, err := approvedInstance(suspended, "node-1", "node-uid-1")
	if err != nil {
		t.Fatalf("approvedInstance failed: %v", err)
	}
	if execOut.Spec.State != corev1.InstanceStateStopped {
		t.Fatalf("expected runtime execution state Stopped when suspended, got %s", execOut.Spec.State)
	}
	if suspended.Spec.State != corev1.InstanceStateRunning {
		t.Fatalf("expected desired spec.state to remain Running, got %s", suspended.Spec.State)
	}
	if sameExecution(unsuspended, suspended) {
		t.Fatalf("expected sameExecution=false across Suspended toggle")
	}
	if !sameExecution(suspended, testApprovedFixture(true)) {
		t.Fatalf("expected sameExecution=true for identical suspended fixtures")
	}

	latest := testApprovedFixture(true)
	now := time.Now().UTC()
	obs := &runtime.RuntimeObservation{
		Phase:            corev1.InstancePhaseStopped,
		ObservedAt:       &now,
		AppliedResources: latest.Status.ResourceControl.ApprovedResources,
		AppliedPorts:     latest.Status.ResourceControl.ApprovedPorts,
	}
	(&Agent{}).applyRuntimeObservationToStatus(suspended, latest, obs, nil, "10.0.0.2")
	if latest.Status.Phase != corev1.InstancePhaseStopped {
		t.Fatalf("expected observed Phase=Stopped, got %s", latest.Status.Phase)
	}
	ready := metav1.GetCondition(latest.Status.Conditions, metav1.ConditionTypeReady)
	if ready == nil || ready.Reason != corev1.ReasonInstanceSuspendedByTenant {
		t.Fatalf("expected Ready reason=%s, got %+v", corev1.ReasonInstanceSuspendedByTenant, ready)
	}
}
