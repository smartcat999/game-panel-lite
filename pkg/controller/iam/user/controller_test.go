package user

import (
	"context"
	"os"
	"testing"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	userdb "github.com/smartcat999/game-panel-cloud/pkg/repo/iam/user"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

func TestPersonalTenantDisabledUserReconcileAndResyncConverges(t *testing.T) {
	fix := testpg.New(t, nil)
	ctx := context.Background()
	uDB := userdb.NewDB(fix.Pool)

	u, err := uDB.Create(ctx, &iamv1.User{
		Metadata: metav1.ObjectMeta{
			Name:        "viewer",
			Annotations: map[string]string{iamv1.AnnotationDisablePersonalTenant: "true"},
		},
		Spec:   iamv1.UserSpec{Email: "viewer@example.invalid", State: iamv1.UserStateActive},
		Status: iamv1.UserStatus{DefaultTenant: "admin"},
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	ctrl := &Controller{}
	events, err := ctrl.Resync(ctx, fix.Registry, nil)
	if err != nil {
		t.Fatalf("initial Resync: %v", err)
	}
	if len(events) != 1 || events[0].Name != "viewer" {
		t.Fatalf("expected 1 reconcile event before status convergence, got %#v", events)
	}

	ev := mq.NewEvent(mq.EventReconcile, iamv1.GroupVersion, "User", "", u.Metadata.Name, u.Metadata.Generation, u.Metadata.ResourceVersion)
	if err := ctrl.Reconcile(ctx, fix.Registry, nil, nil, ev); err != nil {
		t.Fatalf("Reconcile opt-out user: %v", err)
	}

	updated, err := uDB.Get(ctx, "viewer")
	if err != nil {
		t.Fatalf("get updated user: %v", err)
	}
	if updated.Status.DefaultTenant != "admin" || updated.Status.Phase != iamv1.UserStateActive || updated.Status.ObservedGeneration < updated.Metadata.Generation {
		t.Fatalf("unexpected converged user status: %+v", updated.Status)
	}

	eventsAfter, err := ctrl.Resync(ctx, fix.Registry, nil)
	if err != nil {
		t.Fatalf("second Resync: %v", err)
	}
	if len(eventsAfter) != 0 {
		t.Fatalf("expected 0 resync events after convergence, got %#v", eventsAfter)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	testpg.Stop()
	os.Exit(code)
}
