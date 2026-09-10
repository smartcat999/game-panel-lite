package regioncontrol

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/identity"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/regionexecution"
)

func TestPlacementOverrideRequiresScopedAuthorityAndAuditsReason(t *testing.T) {
	now := time.Now().UTC()
	operatorID, unscopedID := contract.UserID("usr_operator"), contract.UserID("usr_unscoped")
	identityModule := identity.New(identity.Seed{Users: []identity.User{{ID: operatorID}, {ID: unscopedID}}, Sessions: map[string]contract.UserID{"operator": operatorID, "unscoped": unscopedID}, PlatformOperators: []contract.UserID{operatorID, unscopedID}, RegionOperators: map[contract.UserID][]contract.RegionID{operatorID: {"reg_test"}}})
	region := regionexecution.New("reg_test", []regionexecution.Node{
		{ID: "nod_primary", RegionID: "reg_test", State: regionexecution.NodeReady, Games: []string{"terraria"}, CPUCapacity: 2000, MemoryCapacityMB: 4096, LeaseUntil: now.Add(time.Hour)},
		{ID: "nod_override", RegionID: "reg_test", State: regionexecution.NodeReady, Games: []string{"terraria"}, CPUCapacity: 4000, MemoryCapacityMB: 8192, LeaseUntil: now.Add(time.Hour)},
	})
	deployment, _, err := region.ReceiveDesired(context.Background(), regionexecution.DesiredDeployment{MessageID: "evt_test", WorkspaceID: "ws_test", LogicalInstanceID: "lin_test", RegionID: "reg_test", PlacementVersion: 1, InstanceRevisionID: "rev_test", DesiredState: "running", GameKey: "terraria", CPUUnits: 1000, MemoryMegabytes: 1024}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := region.Schedule(context.Background(), deployment.ID, now); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(identityModule, region)
	path := "/v1/regions/reg_test/deployments/" + string(deployment.ID) + "/placement-override"
	assertRegionStatus(t, handler, path, "unscoped", `{"nodeId":"nod_override","reason":"maintenance"}`, http.StatusForbidden)
	assertRegionStatus(t, handler, path, "operator", `{"nodeId":"nod_override","reason":""}`, http.StatusBadRequest)
	assertRegionStatus(t, handler, path, "operator", `{"nodeId":"nod_override","reason":"maintenance window"}`, http.StatusOK)
	audits := region.Audits(context.Background())
	if len(audits) != 1 || audits[0].ActorUserID != operatorID || audits[0].Reason != "maintenance window" || audits[0].RequestedNodeID != "nod_override" {
		t.Fatalf("audit=%#v", audits)
	}
}

func assertRegionStatus(t *testing.T, handler http.Handler, path, token, body string, want int) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, want, recorder.Body.String())
	}
}
