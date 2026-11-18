package node

import (
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestNodePolicyVersionAndHeartbeatExpired(t *testing.T) {
	n := &corev1.Node{Metadata: metav1.ObjectMeta{Name: "node-1", UID: "uid-1", ResourceVersion: "2", StatusVersion: "10"}}
	pv, err := nodePolicyVersion(n)
	if err != nil || pv != 11 {
		t.Fatalf("expected policy version 11, got %d (err=%v)", pv, err)
	}
	if _, err := nodePolicyVersion(&corev1.Node{Metadata: metav1.ObjectMeta{Name: "no-uid", ResourceVersion: "1"}}); err == nil {
		t.Fatal("expected missing UID error")
	}
	if _, err := nodePolicyVersion(&corev1.Node{Metadata: metav1.ObjectMeta{Name: "bad-rv", UID: "uid-1", ResourceVersion: "0"}}); err == nil {
		t.Fatal("expected non-positive resourceVersion error")
	}

	now := time.Now().UTC()
	recent := now.Add(-15 * time.Second)
	expired := now.Add(-45 * time.Second)
	n.Status.Phase = corev1.NodePhaseReady
	n.Status.LastHeartbeatTime = &recent
	if heartbeatExpired(n, now, 30*time.Second) {
		t.Fatal("expected recent heartbeat not to be expired")
	}
	n.Status.LastHeartbeatTime = &expired
	if !heartbeatExpired(n, now, 30*time.Second) {
		t.Fatal("expected expired Ready heartbeat to return true")
	}
	n.Status.Phase = corev1.NodePhaseNotReady
	if heartbeatExpired(n, now, 30*time.Second) {
		t.Fatal("expected NotReady node not to trigger expiration again")
	}
}

func TestSyncNodeExternalIPWithoutRedundantWrite(t *testing.T) {
	n := &corev1.Node{
		Metadata: metav1.ObjectMeta{Name: "node-1", UID: "uid-1"},
		Status: corev1.NodeStatus{
			Reserved:  nil, // nodedb.Get does not populate Status.Reserved.
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeAddressInternalIP, Address: "192.168.2.4"}},
		},
	}
	if externalIPOutOfSync(n) || syncNodeExternalIP(n, false) {
		t.Fatal("expected node without externalIP not to require status update even when Reserved is nil")
	}

	n.Spec.ExternalIP = "100.88.0.2"
	if !externalIPOutOfSync(n) || !syncNodeExternalIP(n, false) {
		t.Fatal("expected spec.externalIP update to sync ExternalIP address")
	}
	if externalIPOutOfSync(n) || syncNodeExternalIP(n, false) {
		t.Fatal("expected synced ExternalIP to be idempotent on subsequent heartbeats")
	}

	n.Spec.ExternalIP = ""
	if !syncNodeExternalIP(n, true) {
		t.Fatal("expected clearing spec.externalIP with clearWhenEmpty=true to remove ExternalIP address")
	}
	if len(n.Status.Addresses) != 1 || n.Status.Addresses[0].Type != corev1.NodeAddressInternalIP {
		t.Fatalf("unexpected addresses after clear: %+v", n.Status.Addresses)
	}
}
