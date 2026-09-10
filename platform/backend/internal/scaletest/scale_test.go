package scaletest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/controlplane"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/identity"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/messaging"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeexecution"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/workspace"
)

func BenchmarkStatelessControlPlane(b *testing.B) {
	identityModule := identity.New(identity.Seed{Users: []identity.User{{ID: "usr_load"}}, Sessions: map[string]contract.UserID{"load": "usr_load"}})
	handler := controlplane.NewHandler(identityModule, workspace.New(workspace.Seed{}))
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			request := httptest.NewRequest(http.MethodGet, "/v1/session", nil)
			request.Header.Set("Authorization", "Bearer load")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				b.Fatalf("status=%d", response.Code)
			}
		}
	})
}

func BenchmarkIdempotentMessageConsumers(b *testing.B) {
	messages := messaging.New()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id := contract.EventID(fmt.Sprintf("evt_%d", atomic.AddInt64(&eventSequence, 1)))
			if _, err := messages.HandleOnce(id, time.Now(), func() error { return nil }); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkRegionSchedulers(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			now := time.Now()
			region := regionexecution.New("reg_load", []regionexecution.Node{{ID: "nod_load", RegionID: "reg_load", State: regionexecution.NodeReady, Games: []string{"terraria"}, CPUCapacity: 1000, MemoryCapacityMB: 1024, LeaseUntil: now.Add(time.Minute)}})
			deployment, _, err := region.ReceiveDesired(context.Background(), regionexecution.DesiredDeployment{MessageID: "evt_load", WorkspaceID: "ws_load", LogicalInstanceID: "lin_load", RegionID: "reg_load", PlacementVersion: 1, InstanceRevisionID: "rev_load", DesiredState: "running", GameKey: "terraria", CPUUnits: 1000, MemoryMegabytes: 1024}, now)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := region.Schedule(context.Background(), deployment.ID, now); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkNodeAgents(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			now := time.Now()
			region := regionexecution.New("reg_load", []regionexecution.Node{{ID: "nod_load", RegionID: "reg_load", State: regionexecution.NodeReady, Games: []string{"terraria"}, CPUCapacity: 1000, MemoryCapacityMB: 1024, LeaseUntil: now.Add(time.Minute)}})
			deployment, _, _ := region.ReceiveDesired(context.Background(), regionexecution.DesiredDeployment{MessageID: "evt_load", WorkspaceID: "ws_load", LogicalInstanceID: "lin_load", RegionID: "reg_load", PlacementVersion: 1, InstanceRevisionID: "rev_load", DesiredState: "running", GameKey: "terraria", CPUUnits: 1000, MemoryMegabytes: 1024}, now)
			_, _ = region.Schedule(context.Background(), deployment.ID, now)
			agent := nodeexecution.Agent{NodeID: "nod_load", BatchSize: 1, ClaimTTL: time.Minute, Store: region, Reconcile: func(context.Context, regionexecution.WorkAssignment) error { return nil }}
			if processed, err := agent.RunOnce(context.Background(), now); err != nil || processed != 1 {
				b.Fatalf("processed=%d error=%v", processed, err)
			}
		}
	})
}

var eventSequence int64
