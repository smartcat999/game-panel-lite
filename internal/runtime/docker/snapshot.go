package docker

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strconv"

	"github.com/smartcat999/game-panel-lite/internal/worker"
)

var ErrSnapshotUnavailable = errors.New("stopped instance data unavailable")

// ReadStoppedData holds the same cross-process lock as lifecycle mutations.
// The caller must hold current execution authority and keep reads within this
// callback. External Docker/host writers are outside this cooperative lock.
func (a *Adapter) ReadStoppedData(ctx context.Context, observed worker.State, read func(context.Context, fs.FS) error) error {
	if _, err := mutationContainerID(observed); err != nil {
		return err
	}
	if read == nil {
		return ErrSnapshotUnavailable
	}
	unlock, err := lockInstanceCreation(ctx, a.dataDir, observed.ServerID)
	if err != nil {
		return err
	}
	defer unlock()
	if err := checkInstanceRecovery(a.dataDir, observed.ServerID); err != nil {
		return err
	}
	if err := a.checkStoppedSnapshot(ctx, observed); err != nil {
		return err
	}
	root, err := os.OpenRoot(a.dataDir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := rejectSymlink(root, observed.ServerID); err != nil {
		return err
	}
	data, err := root.OpenRoot(observed.ServerID)
	if err != nil {
		return err
	}
	defer data.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := read(ctx, data.FS()); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Reject changed state before the caller publishes an archive as complete.
	return a.checkStoppedSnapshot(ctx, observed)
}

func (a *Adapter) checkStoppedSnapshot(ctx context.Context, observed worker.State) error {
	name, err := containerName(observed.ServerID)
	if err != nil {
		return err
	}
	current, err := a.client.ContainerInspect(ctx, name)
	if err != nil {
		return err
	}
	if current.ID != observed.ID || current.Config == nil || current.State == nil {
		return ErrSnapshotUnavailable
	}
	labels := current.Config.Labels
	if labels[labelManaged] != "true" || labels[labelUID] != observed.UID || labels[labelServer] != observed.ServerID || labels[labelNode] != observed.NodeID || labels[labelGeneration] != strconv.Itoa(observed.Generation) {
		return ErrSnapshotUnavailable
	}
	state := current.State
	if state.Running || state.Restarting || state.Paused || state.Dead || (state.Status != "exited" && state.Status != "created") {
		return ErrSnapshotUnavailable
	}
	return nil
}
