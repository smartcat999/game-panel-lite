package regional

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/instances"
)

type schedulingTasksProbe struct {
	claim              SchedulingClaim
	retries, completed int
}

func (p *schedulingTasksProbe) ClaimScheduling(context.Context, time.Duration) (*SchedulingClaim, error) {
	return &p.claim, nil
}
func (p *schedulingTasksProbe) RetryScheduling(_ context.Context, c SchedulingClaim, _ time.Duration) error {
	if c.Token != p.claim.Token {
		return errors.New("wrong retry claim")
	}
	p.retries++
	return nil
}
func (p *schedulingTasksProbe) CompleteScheduling(context.Context, SchedulingClaim, Allocation) error {
	p.completed++
	return nil
}

type schedulingSourceFunc func(context.Context, instances.RevisionAvailable) (RevisionSnapshot, error)

func (f schedulingSourceFunc) GetRevision(ctx context.Context, event instances.RevisionAvailable) (RevisionSnapshot, error) {
	return f(ctx, event)
}

type schedulingScopeProbe struct{ calls int }

func (p *schedulingScopeProbe) SchedulingScope(context.Context, RevisionSnapshot) (SchedulingScope, error) {
	p.calls++
	return SchedulingScope{}, errors.New("fixture has no node grant")
}

func TestSchedulingChecksGlobalIntentBeforeScope(t *testing.T) {
	event := instances.RevisionAvailable{SchemaVersion: 1, EventID: "event", OperationID: "operation", OrganizationID: "tenant", ServerID: "server", RevisionID: "revision", RegionID: "east", SpecGeneration: 1, PlacementEpoch: 1}
	snapshot := RevisionSnapshot{Event: event, CurrentSpecGeneration: 1, IntentVersion: 1, DesiredState: "running", Revision: instances.Revision{ID: event.RevisionID, ServerID: event.ServerID, SpecGeneration: 1, Specification: instances.Specification{ProviderKey: "test", GameVersion: "1", ConfigSchemaVersion: 1, Configuration: instances.ProtectedConfiguration{KeyID: "key", Ciphertext: []byte("encrypted")}, Resources: instances.Resources{CPU: 1, MemoryMB: 128}}}}
	for name, mutate := range map[string]func(*RevisionSnapshot){
		"current":                func(*RevisionSnapshot) {},
		"stopped":                func(s *RevisionSnapshot) { s.DesiredState = "stopped" },
		"new intent":             func(s *RevisionSnapshot) { s.IntentVersion++ },
		"new configuration":      func(s *RevisionSnapshot) { s.CurrentSpecGeneration++ },
		"different tenant":       func(s *RevisionSnapshot) { s.Event.OrganizationID = "other" },
		"different placement":    func(s *RevisionSnapshot) { s.Event.PlacementEpoch++ },
		"different resources":    func(s *RevisionSnapshot) { s.Revision.Specification.Resources.CPU++ },
		"different game version": func(s *RevisionSnapshot) { s.Revision.Specification.GameVersion = "2" },
		"different ciphertext":   func(s *RevisionSnapshot) { s.Revision.Specification.Configuration.Ciphertext = []byte("other") },
		"unavailable":            func(*RevisionSnapshot) {},
		"timeout":                func(*RevisionSnapshot) {},
	} {
		t.Run(name, func(t *testing.T) {
			current := snapshot
			mutate(&current)
			tasks := &schedulingTasksProbe{claim: SchedulingClaim{Token: "token", Snapshot: snapshot, Deployment: Deployment{SpecGeneration: 1, IntentVersion: 1}}}
			scopes := &schedulingScopeProbe{}
			source := schedulingSourceFunc(func(ctx context.Context, got instances.RevisionAvailable) (RevisionSnapshot, error) {
				if got != event {
					t.Fatal("wrong event checked")
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Fatal("unbounded global read")
				}
				if name == "unavailable" {
					return RevisionSnapshot{}, ErrRevisionUnavailable
				}
				if name == "timeout" {
					<-ctx.Done()
					return RevisionSnapshot{}, ctx.Err()
				}
				return current, nil
			})
			worker := SchedulingWorker{Source: source, Tasks: tasks, Scopes: scopes, Lease: time.Minute, Timeout: 10 * time.Millisecond, RetryDelay: time.Second}
			done, err := worker.RunOnce(context.Background())
			if done || err == nil || tasks.retries != 1 || tasks.completed != 0 {
				t.Fatalf("failed check was not retried: %v %v %+v", done, err, tasks)
			}
			wantCalls := 0
			if name == "current" {
				wantCalls = 1
			}
			if scopes.calls != wantCalls {
				t.Fatalf("scope consulted after invalid global intent: %d", scopes.calls)
			}
		})
	}
}
