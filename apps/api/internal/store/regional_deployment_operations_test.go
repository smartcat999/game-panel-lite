package store

import (
	"context"
	"testing"
)

func testRegionalDeploymentOperations(t *testing.T, db *RegionalStore) {
	t.Helper()
	page, err := db.ListRegionalDeploymentOperations(context.Background(), "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.RegionID != db.regionID || page.ObservedAtMS < 1 || len(page.Deployments) != 1 {
		t.Fatalf("deployment operations page: %+v", page)
	}
	deployment := page.Deployments[0]
	if deployment.OrganizationID != "tenant" || deployment.ServerID != "server" || deployment.SpecGeneration != 3 || deployment.IntentVersion != 3 || deployment.DesiredState != "stopped" || deployment.SchedulingStatus != "pending" || deployment.NodeID != "" {
		t.Fatalf("deployment operations identity: %+v", deployment)
	}
	if _, err := db.ListRegionalDeploymentOperations(context.Background(), "", 0); err == nil {
		t.Fatal("unbounded deployment operations query accepted")
	}
}

func testRegionalDeploymentAllocationOperations(t *testing.T, db *RegionalStore) {
	t.Helper()
	page, err := db.ListRegionalDeploymentOperations(context.Background(), "", 200)
	if err != nil {
		t.Fatal(err)
	}
	byServerID := make(map[string]string, len(page.Deployments))
	for _, deployment := range page.Deployments {
		byServerID[deployment.ServerID] = deployment.NodeID
	}
	if byServerID["schedule-server-a"] != "schedule-a" || byServerID["schedule-server-b"] != "schedule-b" {
		t.Fatalf("deployment allocation mapping: %+v", byServerID)
	}
}
