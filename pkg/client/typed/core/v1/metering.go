package v1

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client/rest"
)

// MeteringInterface has methods to query TenantMetering summaries and CloudEvents usage batches.
// Pass an empty tenant ("") to CoreV1().Metering("") for cluster-wide queries.
type MeteringInterface interface {
	List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.TenantMetering], error)
	ListCloudEvents(ctx context.Context, opts rest.ListOptions) ([]cloudevents.Event, error)
}

type meteringClient struct {
	rest   rest.Interface
	tenant string
}

func newMetering(r rest.Interface, tenant string) *meteringClient {
	return &meteringClient{
		rest:   r,
		tenant: tenant,
	}
}

func (c *meteringClient) List(ctx context.Context, opts rest.ListOptions) (*metav1.List[corev1.TenantMetering], error) {
	var out metav1.List[corev1.TenantMetering]
	if err := c.rest.List(ctx, "metering", c.tenant, opts, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *meteringClient) ListCloudEvents(ctx context.Context, opts rest.ListOptions) ([]cloudevents.Event, error) {
	opts.Format = corev1.MeteringFormatCloudEvents
	var out []cloudevents.Event
	if err := c.rest.List(ctx, "metering", c.tenant, opts, &out); err != nil {
		return nil, err
	}
	return out, nil
}
