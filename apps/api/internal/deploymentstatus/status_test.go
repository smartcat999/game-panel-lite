package deploymentstatus

import "testing"

func TestEventValidation(t *testing.T) {
	valid := Event{SchemaVersion: 1, EventID: "event", RegionID: "east", OrganizationID: "tenant", OperationID: "operation", ServerID: "server", RevisionID: "revision", TaskID: "task", NodeID: "node", PlacementEpoch: 1, SpecGeneration: 1, IntentVersion: 1, Fence: 1, ActualState: "running", Outcome: "succeeded", RuntimeID: "container", ObservedAtMS: 1}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Event){
		"wrong schema":            func(e *Event) { e.SchemaVersion = 2 },
		"missing identity":        func(e *Event) { e.ServerID = "" },
		"invalid fence":           func(e *Event) { e.Fence = 0 },
		"unknown state":           func(e *Event) { e.ActualState = "starting" },
		"success without runtime": func(e *Event) { e.RuntimeID = "" },
		"success while stopped":   func(e *Event) { e.ActualState = "stopped" },
	} {
		t.Run(name, func(t *testing.T) {
			item := valid
			mutate(&item)
			if item.Validate() == nil {
				t.Fatal("invalid event accepted")
			}
		})
	}
	failed := valid
	failed.Outcome = "failed"
	failed.ActualState = "unknown"
	failed.RuntimeID = ""
	if failed.Validate() != nil {
		t.Fatal("bounded failure event rejected")
	}
}
