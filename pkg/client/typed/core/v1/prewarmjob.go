package v1

import (
	"context"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// PrewarmJobInterface has methods to work with cluster-scoped PrewarmJob resources.
type PrewarmJobInterface interface {
	Get(ctx context.Context, name string) (*corev1.PrewarmJob, error)
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.PrewarmJob], error)
	Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, job *corev1.PrewarmJob) (*corev1.PrewarmJob, error)
	Update(ctx context.Context, job *corev1.PrewarmJob) (*corev1.PrewarmJob, error)
	UpdateStatus(ctx context.Context, job *corev1.PrewarmJob) (*corev1.PrewarmJob, error)
	Delete(ctx context.Context, name string) error
}

type prewarmJobs struct {
	rest rest.Interface
}

func newPrewarmJobs(r rest.Interface) *prewarmJobs {
	return &prewarmJobs{rest: r}
}

func (c *prewarmJobs) Get(ctx context.Context, name string) (*corev1.PrewarmJob, error) {
	var out corev1.PrewarmJob
	if err := c.rest.Get(ctx, "prewarmjobs", "", name, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *prewarmJobs) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.PrewarmJob], error) {
	var out metav1.List[corev1.PrewarmJob]
	if err := c.rest.List(ctx, "prewarmjobs", "", opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *prewarmJobs) Watch(ctx context.Context, opts rest.ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error {
	return c.rest.Watch(ctx, "prewarmjobs", "", opts, handler)
}

func (c *prewarmJobs) Create(ctx context.Context, job *corev1.PrewarmJob) (*corev1.PrewarmJob, error) {
	var out corev1.PrewarmJob
	if err := c.rest.Create(ctx, job, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *prewarmJobs) Update(ctx context.Context, job *corev1.PrewarmJob) (*corev1.PrewarmJob, error) {
	var out corev1.PrewarmJob
	if err := c.rest.Update(ctx, job, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *prewarmJobs) UpdateStatus(ctx context.Context, job *corev1.PrewarmJob) (*corev1.PrewarmJob, error) {
	var out corev1.PrewarmJob
	if err := c.rest.UpdateStatus(ctx, job, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *prewarmJobs) Delete(ctx context.Context, name string) error {
	return c.rest.Delete(ctx, "prewarmjobs", "", name)
}
