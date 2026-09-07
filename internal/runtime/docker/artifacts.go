package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/smartcat999/game-panel-lite/internal/workload"
)

// ArtifactSource must resolve only the supplied assignment's authorized bytes.
// Open must honor ctx, including cancellation of a blocked network read.
type ArtifactSource interface {
	Open(context.Context, workload.Assignment, workload.Artifact) (io.ReadCloser, error)
}
type ArtifactLimits struct {
	MaxFiles      int
	MaxFileBytes  int64
	MaxTotalBytes int64
}

func NewAdapterWithArtifacts(host, dataDir string, source ArtifactSource, limits ArtifactLimits) (*Adapter, error) {
	if source == nil || limits.MaxFiles <= 0 || limits.MaxFileBytes <= 0 || limits.MaxTotalBytes <= 0 {
		return nil, fmt.Errorf("artifact source and positive limits are required")
	}
	adapter, err := NewAdapter(host, dataDir)
	if err != nil {
		return nil, err
	}
	adapter.artifactSource = source
	adapter.artifactLimits = limits
	return adapter, nil
}
func (a *Adapter) ValidateArtifacts(assignment workload.Assignment) error {
	items := assignment.Spec.Options.Artifacts
	if len(items) == 0 {
		return nil
	}
	if err := workload.ValidateArtifacts(assignment.Spec.Options); err != nil {
		return err
	}
	limits := a.artifactLimits
	if a.artifactSource == nil || limits.MaxFiles <= 0 || limits.MaxFileBytes <= 0 || limits.MaxTotalBytes <= 0 {
		return fmt.Errorf("runtime artifact delivery is not configured")
	}
	if len(items) > limits.MaxFiles {
		return fmt.Errorf("workload artifact count exceeds limit")
	}
	remaining := limits.MaxTotalBytes
	for _, item := range items {
		if item.SizeBytes > limits.MaxFileBytes || item.SizeBytes > remaining {
			return fmt.Errorf("workload artifact bytes exceed limit")
		}
		remaining -= item.SizeBytes
	}
	return nil
}
func (a *Adapter) writeArtifact(ctx context.Context, assignment workload.Assignment, item workload.Artifact, destination io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := a.artifactSource.Open(ctx, assignment, item)
	if err != nil {
		return err
	}
	digest := sha256.New()
	reader := artifactReader{ctx, source}
	size, copyErr := io.Copy(io.MultiWriter(destination, digest), io.LimitReader(reader, item.SizeBytes))
	if copyErr == nil && size != item.SizeBytes {
		copyErr = fmt.Errorf("artifact size is shorter than declared")
	}
	if copyErr == nil {
		var probe [1]byte
		n, err := io.ReadFull(reader, probe[:])
		if n != 0 {
			copyErr = fmt.Errorf("artifact exceeds declared size")
		} else if err != io.EOF {
			copyErr = err
		}
	}
	if copyErr == nil && hex.EncodeToString(digest.Sum(nil)) != item.SHA256 {
		copyErr = fmt.Errorf("artifact checksum mismatch")
	}
	return errors.Join(copyErr, source.Close(), ctx.Err())
}

type artifactReader struct {
	ctx    context.Context
	source io.Reader
}

func (r artifactReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
