package worker

import (
	"context"
	"fmt"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

// AuthorizeMutations checks authority immediately before each mutation, including
// those performed through a prepared artifact runtime. It is a cooperative guard,
// not a fencing implementation for a remote runtime that ignores cancellation.
func AuthorizeMutations(runtime Runtime, authorize func(context.Context) error) Runtime {
	guarded := &authorizedRuntime{Runtime: runtime, authorize: authorize}
	if artifacts, ok := runtime.(ArtifactRuntime); ok {
		return &authorizedArtifacts{authorizedRuntime: guarded, artifacts: artifacts}
	}
	return guarded
}

type authorizedRuntime struct {
	Runtime
	authorize func(context.Context) error
}

func (r *authorizedRuntime) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.authorize == nil {
		return fmt.Errorf("mutation authorization is required")
	}
	if err := r.authorize(ctx); err != nil {
		return err
	}
	return ctx.Err()
}
func (r *authorizedRuntime) Create(ctx context.Context, a workload.Assignment) error {
	if err := r.check(ctx); err != nil {
		return err
	}
	return r.Runtime.Create(ctx, a)
}
func (r *authorizedRuntime) Start(ctx context.Context, s State) error {
	if err := r.check(ctx); err != nil {
		return err
	}
	return r.Runtime.Start(ctx, s)
}
func (r *authorizedRuntime) Stop(ctx context.Context, s State) error {
	if err := r.check(ctx); err != nil {
		return err
	}
	return r.Runtime.Stop(ctx, s)
}
func (r *authorizedRuntime) Remove(ctx context.Context, s State) error {
	if err := r.check(ctx); err != nil {
		return err
	}
	return r.Runtime.Remove(ctx, s)
}

type authorizedArtifacts struct {
	*authorizedRuntime
	artifacts ArtifactRuntime
}

func (r *authorizedArtifacts) ValidateArtifacts(a workload.Assignment) error {
	return r.artifacts.ValidateArtifacts(a)
}
func (r *authorizedArtifacts) PrepareArtifacts(ctx context.Context, a workload.Assignment) (PreparedRuntime, error) {
	if err := r.check(ctx); err != nil {
		return nil, err
	}
	prepared, err := r.artifacts.PrepareArtifacts(ctx, a)
	if err != nil {
		return nil, err
	}
	return &authorizedPrepared{authorizedRuntime: &authorizedRuntime{Runtime: prepared, authorize: r.authorize}, prepared: prepared}, nil
}

type authorizedPrepared struct {
	*authorizedRuntime
	prepared PreparedRuntime
}

func (r *authorizedPrepared) Release() error { return r.prepared.Release() }
