package artifact

import (
	"context"
	"fmt"
	"io"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/tasklog"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
)

func artifactLogKey(name string) string {
	return "registry.artifact:" + name
}

func (o *operator) AppendLogs(ctx context.Context, name string, req *registryv1.ArtifactLogChunk) error {
	if _, err := o.db.Get(ctx, name); err != nil {
		return err
	}
	if o.logs == nil || req == nil {
		return nil
	}
	cp := *req
	cp.DefaultMeta()
	if err := cp.Validate(); err != nil {
		return fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	key := artifactLogKey(name)
	switch cp.Action {
	case registryv1.ArtifactLogActionReset:
		return o.logs.Reset(ctx, key)
	case registryv1.ArtifactLogActionAppend:
		return o.logs.Append(ctx, key, cp.Lines)
	case registryv1.ArtifactLogActionComplete:
		if len(cp.Lines) > 0 {
			if err := o.logs.Append(ctx, key, cp.Lines); err != nil {
				return err
			}
		}
		return o.logs.Complete(ctx, key)
	default:
		return fmt.Errorf("%w: unsupported action %q", models.ErrBadRequest, cp.Action)
	}
}

func (o *operator) StreamLogs(ctx context.Context, name string, follow bool, tailLines int) (io.ReadCloser, error) {
	art, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if o.logs == nil {
		return io.NopCloser(noOpReader{}), nil
	}
	completed := art.Metadata.DeletionTimestamp != nil ||
		art.Status.Phase == registryv1.ArtifactPhaseReady ||
		art.Status.Phase == registryv1.ArtifactPhaseFailed
	return o.logs.Stream(ctx, artifactLogKey(name), tasklog.StreamOptions{
		Follow:    follow,
		TailLines: tailLines,
		Completed: completed,
	})
}

type noOpReader struct{}

func (noOpReader) Read(_ []byte) (int, error) { return 0, io.EOF }
