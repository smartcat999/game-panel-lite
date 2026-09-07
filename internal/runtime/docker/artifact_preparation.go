package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"

	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/internal/worker"
	"github.com/smartcat999/game-panel-lite/internal/workload"
)

const artifactPreparationDir = ".artifact-preparations"

var _ worker.ArtifactRuntime = (*Adapter)(nil)

// PrepareArtifacts verifies every stream before Worker may replace a container.
// Preparation lives outside instance data and is private to this reconciliation.
// Create reads only these cached bytes, never the remote source a second time.
func (a *Adapter) PrepareArtifacts(ctx context.Context, assignment workload.Assignment) (worker.PreparedRuntime, error) {
	if _, err := containerName(assignment.ServerID); err != nil {
		return nil, err
	}
	if assignment.UID == "" || assignment.NodeID == "" || assignment.Generation <= 0 {
		return nil, fmt.Errorf("artifact assignment identity is incomplete")
	}
	if err := a.ValidateArtifacts(assignment); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(a.dataDir, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(a.dataDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	const parentName = artifactPreparationDir
	if err := root.MkdirAll(parentName, 0700); err != nil {
		return nil, err
	}
	if err := rejectSymlink(root, parentName); err != nil {
		return nil, err
	}
	parent, err := root.OpenRoot(parentName)
	if err != nil {
		return nil, err
	}
	name := uuid.NewString()
	if err := parent.Mkdir(name, 0700); err != nil {
		parent.Close()
		return nil, err
	}
	stage, err := parent.OpenRoot(name)
	if err != nil {
		return nil, errors.Join(err, parent.RemoveAll(name), parent.Close())
	}
	cache := &preparedArtifactCache{parent: parent, root: stage, name: name, uid: assignment.UID, nodeID: assignment.NodeID, serverID: assignment.ServerID, generation: assignment.Generation, files: map[workload.Artifact]*os.File{}}
	fail := func(err error) (worker.PreparedRuntime, error) { return nil, errors.Join(err, cache.release()) }
	for i, item := range assignment.Spec.Options.Artifacts {
		filename := strconv.Itoa(i)
		file, err := stage.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fail(err)
		}
		writeErr := a.writeArtifact(ctx, assignment, item, file)
		if writeErr == nil {
			writeErr = file.Sync()
		}
		if err := errors.Join(writeErr, file.Close()); err != nil {
			return fail(err)
		}
		verified, err := stage.Open(filename)
		if err != nil {
			return fail(err)
		}
		cache.files[item] = verified
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	copied := *a
	copied.artifactSource = cache
	return &preparedArtifactRuntime{Adapter: &copied, cache: cache}, nil
}

type preparedArtifactRuntime struct {
	*Adapter
	cache *preparedArtifactCache
}

func (p *preparedArtifactRuntime) Release() error { return p.cache.release() }

type preparedArtifactCache struct {
	parent, root                *os.Root
	name, uid, nodeID, serverID string
	generation                  int
	files                       map[workload.Artifact]*os.File
	once                        sync.Once
	releaseErr                  error
}

func (c *preparedArtifactCache) Open(ctx context.Context, a workload.Assignment, item workload.Artifact) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a.UID != c.uid || a.NodeID != c.nodeID || a.ServerID != c.serverID || a.Generation != c.generation {
		return nil, fmt.Errorf("prepared artifacts belong to another assignment")
	}
	file, ok := c.files[item]
	if !ok {
		return nil, fmt.Errorf("artifact was not prepared for this manifest")
	}
	return io.NopCloser(io.NewSectionReader(file, 0, item.SizeBytes)), nil
}
func (c *preparedArtifactCache) release() error {
	c.once.Do(func() {
		var failures []error
		for _, file := range c.files {
			failures = append(failures, file.Close())
		}
		failures = append(failures, c.root.Close(), c.parent.RemoveAll(c.name), c.parent.Close())
		c.releaseErr = errors.Join(failures...)
	})
	return c.releaseErr
}
