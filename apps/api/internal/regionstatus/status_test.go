package regionstatus

import "testing"

func TestSnapshotValidation(t *testing.T) {
	valid := Snapshot{SchemaVersion: 1, EventID: "event", RegionID: "east", Sequence: 1, ObservedAtMS: 1,
		Nodes: NodeSummary{Total: 2, Online: 1, Schedulable: 2}, Capacity: CapacitySummary{CPUTotal: 8, CPUReserved: 2, MemoryTotalMB: 8192, MemoryReservedMB: 2048},
		Deployments: DeploymentSummary{Total: 2, Pending: 1, Reserved: 1}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Snapshot){
		func(s *Snapshot) { s.SchemaVersion = 2 }, func(s *Snapshot) { s.RegionID = " east" },
		func(s *Snapshot) { s.Nodes.Online = 3 }, func(s *Snapshot) { s.Capacity.CPUReserved = -1 },
		func(s *Snapshot) { s.Deployments.Rejected = 1 },
	} {
		bad := valid
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
}
