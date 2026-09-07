package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

func TestMutationAuthorizationChecksEveryOperation(t *testing.T) {
	for _, operation := range []string{"create", "start", "stop", "remove"} {
		t.Run(operation, func(t *testing.T) {
			runtime := &memoryRuntime{}
			revoked := errors.New("revoked")
			guarded := AuthorizeMutations(runtime, func(context.Context) error { return revoked })
			var err error
			switch operation {
			case "create":
				err = guarded.Create(context.Background(), assignment())
			case "start":
				err = guarded.Start(context.Background(), State{})
			case "stop":
				err = guarded.Stop(context.Background(), State{})
			case "remove":
				err = guarded.Remove(context.Background(), State{})
			}
			if !errors.Is(err, revoked) || len(runtime.calls) != 0 {
				t.Fatalf("unauthorized %s: %v %v", operation, err, runtime.calls)
			}
		})
	}
	runtime := &memoryRuntime{}
	ctx, cancel := context.WithCancel(context.Background())
	guarded := AuthorizeMutations(runtime, func(context.Context) error { cancel(); return nil })
	if err := guarded.Create(ctx, assignment()); !errors.Is(err, context.Canceled) || len(runtime.calls) != 0 {
		t.Fatalf("mutation after canceled authorization: %v", err)
	}
}

type authorizationArtifactRuntime struct {
	*memoryRuntime
	released bool
}

func (r *authorizationArtifactRuntime) ValidateArtifacts(workload.Assignment) error { return nil }
func (r *authorizationArtifactRuntime) PrepareArtifacts(context.Context, workload.Assignment) (PreparedRuntime, error) {
	return r, nil
}
func (r *authorizationArtifactRuntime) Release() error { r.released = true; return nil }

func TestPreparedRuntimePreservesMutationAuthorization(t *testing.T) {
	runtime := &authorizationArtifactRuntime{memoryRuntime: &memoryRuntime{}}
	calls := 0
	guarded := AuthorizeMutations(runtime, func(context.Context) error {
		calls++
		if calls > 1 {
			return errors.New("revoked after preparation")
		}
		return nil
	})
	prepared, err := guarded.(ArtifactRuntime).PrepareArtifacts(context.Background(), assignment())
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Create(context.Background(), assignment()); err == nil || len(runtime.calls) != 0 {
		t.Fatal("prepared runtime bypassed authorization")
	}
	if err := prepared.Release(); err != nil || !runtime.released {
		t.Fatal("revocation prevented cleanup")
	}
}
