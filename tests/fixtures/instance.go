// Package fixtures builds approved in-memory workload lifecycles for integration tests.
package fixtures

import (
	"context"
	"errors"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	tenantctrl "github.com/smartcat999/game-panel-cloud/pkg/controller/core/tenant"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	runtimeclassdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/runtimeclass"
)

const NodeLabel = "test.gamepanel.io/node"

// ReadyNode declares finite physical capacity and a deterministic node selector.
func ReadyNode(t testing.TB, ctx context.Context, reg *repo.Registry, name string) *corev1.Node {
	t.Helper()
	nodeDB := nodedb.NewDB(reg.Pool())
	node, err := nodeDB.Get(ctx, name)
	if errors.Is(err, db.ErrNotFound) {
		node, err = nodeDB.Create(ctx, &corev1.Node{Metadata: metav1.ObjectMeta{Name: name}})
	}
	if err != nil {
		t.Fatal(err)
	}
	if node.Metadata.Labels == nil {
		node.Metadata.Labels = map[string]string{}
	}
	node.Metadata.Labels[NodeLabel] = name
	node.Spec.Unschedulable = false
	node, err = nodeDB.Update(ctx, node)
	if err != nil {
		t.Fatal(err)
	}
	node.Status.Phase = corev1.NodePhaseReady
	if len(node.Status.Runtimes) == 0 {
		node.Status.Runtimes = []corev1.NodeRuntime{{Handler: "docker", Version: "test"}}
	}
	if node.Status.Allocatable == nil {
		node.Status.Allocatable = corev1.ResourceList{}
	}
	for dimension, value := range (corev1.ResourceList{corev1.ResourceCPU: "64", corev1.ResourceMemory: "256Gi", corev1.ResourceStorage: "1Ti"}) {
		if _, present := node.Status.Allocatable[dimension]; !present {
			node.Status.Allocatable[dimension] = value
		}
	}
	node, err = nodeDB.UpdateStatus(ctx, node)
	if err != nil {
		t.Fatal(err)
	}
	return node
}

func ReconcileActive(t testing.TB, ctx context.Context, reg *repo.Registry, tenant, name string) *corev1.Instance {
	t.Helper()
	if err := (&tenantctrl.Controller{}).Reconcile(ctx, reg, nil, nil, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", tenant, 0, "")); err != nil {
		t.Fatal(err)
	}
	reconciler := instancemodel.NewStageReconciler(reg)
	var current *corev1.Instance
	for range 16 {
		var err error
		current, err = reconciler.ReconcileStep(ctx, tenant, name)
		if err != nil {
			t.Fatal(err)
		}
		if current != nil && instancemodel.GetStage(current) == instancemodel.StageActive {
			return current
		}
	}
	t.Fatalf("instance %s/%s did not acquire an approved binding: %+v", tenant, name, current)
	return nil
}

func CreateActive(t testing.TB, ctx context.Context, reg *repo.Registry, drivers *runtime.Registry, tenant, nodeName string, input *corev1.Instance) *corev1.Instance {
	t.Helper()
	ReadyNode(t, ctx, reg, nodeName)
	input.Spec.NodeName = ""
	input.Spec.NodeSelector = map[string]string{NodeLabel: nodeName}
	created, err := instancemodel.NewOperator(reg, drivers, nil).Create(ctx, tenant, input)
	if err != nil {
		t.Fatal(err)
	}
	return ReconcileActive(t, ctx, reg, tenant, created.Metadata.Name)
}

// ReconcileDeleted acknowledges runtime cleanup only after the actual memory
// driver Destroy succeeds, then waits for both inventory domains and object deletion.
func ReconcileDeleted(t testing.TB, ctx context.Context, reg *repo.Registry, drivers *runtime.Registry, tenant, name string) {
	t.Helper()
	instDB := instancedb.NewDB(reg.Pool())
	rcDB := runtimeclassdb.NewDB(reg.Pool())
	current, err := instDB.Get(ctx, tenant, name)
	if errors.Is(err, db.ErrNotFound) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if current.Metadata.DeletionTimestamp == nil {
		t.Fatal("delete intent must be persisted before cleanup")
	}
	if instancemodel.HasFinalizer(&current.Metadata, corev1.FinalizerInstanceCleanup) {
		class, err := rcDB.Get(ctx, current.Spec.RuntimeClassName)
		if err != nil {
			t.Fatal(err)
		}
		if driver, ok := drivers.Get(class.Handler); ok {
			if err := driver.Destroy(ctx, current, class); err != nil {
				t.Fatal(err)
			}
		}
		instancemodel.RemoveFinalizer(&current.Metadata, corev1.FinalizerInstanceCleanup)
		if _, err := instancemodel.NewOperator(reg, drivers, nil).Update(ctx, tenant, name, current); err != nil {
			t.Fatal(err)
		}
	}
	reconciler := instancemodel.NewStageReconciler(reg)
	for range 16 {
		current, err = reconciler.ReconcileStep(ctx, tenant, name)
		if errors.Is(err, db.ErrNotFound) || err == nil && current == nil {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatalf("instance %s/%s retained resources after cleanup", tenant, name)
}
