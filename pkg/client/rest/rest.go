package rest

import (
	"context"
	"io"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// ListOptions specifies pagination, filtering, and tenancy options for list requests.
type ListOptions struct {
	Tenant        string
	NodeName      string
	LabelSelector string
	FieldSelector string
	Query         map[string]string
	Window        string
	Format        string
	Limit         int
	Offset        int
}

// LogOptions specifies log streaming parameters.
type LogOptions struct {
	Follow    bool
	TailLines int
}

// Interface abstracts the underlying generic REST and WebSocket transport used by typed API group clients.
type Interface interface {
	BaseURL() string
	DefaultTenant() string
	Get(ctx context.Context, resource, tenant, name string, out metav1.Object) error
	GetWithQuery(ctx context.Context, resource, tenant, name string, query map[string]string, out metav1.Object) error
	DownloadBlob(ctx context.Context, digest string) (io.ReadCloser, error)
	List(ctx context.Context, resource, tenant string, opts ListOptions, out any) error
	Watch(ctx context.Context, resource, tenant string, opts ListOptions, handler func(metav1.WatchEvent[map[string]any]) error) error
	Create(ctx context.Context, obj metav1.Object, out metav1.Object) error
	Update(ctx context.Context, obj metav1.Object, out metav1.Object) error
	UpdateStatus(ctx context.Context, obj metav1.Object, out metav1.Object) error
	Patch(ctx context.Context, resource, tenant, name string, patchData []byte, out metav1.Object) error
	Delete(ctx context.Context, resource, tenant, name string) error
	PostSubresource(ctx context.Context, resource, tenant, name, subresource string, body any, out any) error
	UpdateInstanceState(ctx context.Context, tenant, name string, state corev1.InstanceStateValue, out *corev1.Instance) error
	StreamLogs(ctx context.Context, tenant, name string, opts LogOptions) (io.ReadCloser, error)
	Exec(ctx context.Context, tenant, name string, cmd []string) (*corev1.InstanceExec, error)
	ExecInteractive(ctx context.Context, tenant, name string, in io.Reader, out io.Writer, cmd []string) error
	Files(ctx context.Context, tenant, name string, req *corev1.InstanceFiles) (*corev1.InstanceFiles, error)
}
