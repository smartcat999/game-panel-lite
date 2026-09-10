package deliverycontrol

import (
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/billing"
)

func TestAuthorityGrantBindsCompleteDesiredPayload(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	key := []byte("01234567890123456789012345678901")
	payload := DesiredPayload{WorkspaceID: "ws_one", LogicalInstanceID: "lin_one", RegionID: "reg_asia", PlacementVersion: 1, InstanceRevisionID: "rev_one", OperationID: "op_one", DesiredState: "running", ProviderReleaseID: "gpr_fake", GameVersion: "1.0.0", ApplyBehavior: "recreate-required", ResourceSpec: billing.ResourceSpec{CPUMilli: 1000, MemoryMiB: 1024, DiskGiB: 10}, Configuration: map[string]any{"difficulty": "normal"}, ModLock: []ModLockEntry{}, ListenerRequirements: []ListenerRequirement{{Name: "game", Purpose: "join", Transports: []string{"tcp", "udp"}, InternalPort: 7777, ExternalPortPolicy: "allocated", AddressMode: "ip-port", Primary: true}}}
	grant, err := NewAuthorityGrant(payload, key, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	payload.AuthorityGrant = grant
	if !VerifyAuthority(payload, key, now) {
		t.Fatal("valid authority rejected")
	}
	payload.ResourceSpec.MemoryMiB++
	if VerifyAuthority(payload, key, now) {
		t.Fatal("modified desired payload retained authority")
	}
	payload.ResourceSpec.MemoryMiB--
	if VerifyAuthority(payload, key, grant.ExpiresAt) {
		t.Fatal("authority accepted at expiry boundary")
	}
}
