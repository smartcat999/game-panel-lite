package runtime

import (
	"context"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

// WorkspaceMaterializer prepares and materializes immutable Artifact Registry blobs into an Instance workspace via isolated CoW/copy.
// Prepare runs before container start: if syncingMessage != "" and err == nil, startup pauses in Starting/SyncingPackage;
// if err != nil, startup fails closed and never boots with missing or stale artifacts.
type WorkspaceMaterializer interface {
	Prepare(ctx context.Context, in *corev1.Instance) (syncingMessage string, err error)
	Materialize(ctx context.Context, hostDir string, in *corev1.Instance) error
}

// WorkspaceMaterializerSetter is implemented by runtime drivers that materialize workspace artifacts before container start.
type WorkspaceMaterializerSetter interface {
	SetWorkspaceMaterializer(materializer WorkspaceMaterializer)
}

// WithWorkspaceMaterializer configures a WorkspaceMaterializer on DockerDriver.
func WithWorkspaceMaterializer(materializer WorkspaceMaterializer) DockerOption {
	return func(driver *DockerDriver) { driver.materializer = materializer }
}

// SetWorkspaceMaterializer installs the node-agent workspace materializer before reconciliation starts.
func (d *DockerDriver) SetWorkspaceMaterializer(materializer WorkspaceMaterializer) {
	d.materializer = materializer
}
