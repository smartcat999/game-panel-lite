package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-resty/resty/v2"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	corev1client "github.com/smartcat999/game-panel-cloud/pkg/client/typed/core/v1"
	iamv1client "github.com/smartcat999/game-panel-cloud/pkg/client/typed/iam/v1"
	registryv1client "github.com/smartcat999/game-panel-cloud/pkg/client/typed/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
)

// Interface specifies all declarative and REST capabilities of the GamePanel client library.
type Interface interface {
	Config() *config.ClientConfig

	// Typed API group clients
	CoreV1() corev1client.CoreV1Interface
	IAMV1() iamv1client.IAMV1Interface
	RegistryV1() registryv1client.RegistryV1Interface

	// Generic REST operations
	BaseURL() string
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
	DeleteObject(ctx context.Context, obj metav1.Object) error
	PostSubresource(ctx context.Context, resource, tenant, name, subresource string, body any, out any) error

	// Declarative operations
	Apply(ctx context.Context, obj metav1.Object) (*ApplyResult, error)

	// Workload subresources
	UpdateInstanceState(ctx context.Context, tenant, name string, state corev1.InstanceStateValue, out *corev1.Instance) error
	StreamLogs(ctx context.Context, tenant, name string, opts LogOptions) (io.ReadCloser, error)
	Exec(ctx context.Context, tenant, name string, cmd []string) (*corev1.InstanceExec, error)
	ExecInteractive(ctx context.Context, tenant, name string, in io.Reader, out io.Writer, cmd []string) error
	Files(ctx context.Context, tenant, name string, req *corev1.InstanceFiles) (*corev1.InstanceFiles, error)
}

// Client implements Interface using go-resty/resty/v2 and gorilla/websocket transports.
type Client struct {
	cfg         *config.ClientConfig
	baseURL     string
	restyClient *resty.Client
	streamResty *resty.Client
	coreV1      *corev1client.CoreV1Client
	iamV1       *iamv1client.IAMV1Client
	registryV1  *registryv1client.RegistryV1Client
}

// New constructs a new API client with the given ClientConfig.
func New(cfg *config.ClientConfig) (Interface, error) {
	if cfg == nil {
		return nil, errors.New("client config is nil")
	}
	c := *cfg
	c.Default()
	if err := c.Validate(); err != nil {
		return nil, err
	}
	trimmedURL := strings.TrimRight(c.ServerURL, "/")
	streamHC := &http.Client{
		Timeout: 0,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if len(via) > 0 && (req.URL.Host != via[0].URL.Host || req.URL.Query().Get("X-Amz-Signature") != "") {
				req.Header.Del("Authorization")
			}
			return nil
		},
	}
	cli := &Client{
		cfg:         &c,
		baseURL:     trimmedURL,
		restyClient: newRestyClient(trimmedURL, c.Token, &http.Client{Timeout: c.Timeout}),
		streamResty: newRestyClient(trimmedURL, c.Token, streamHC),
	}
	cli.coreV1 = corev1client.New(cli)
	cli.iamV1 = iamv1client.New(cli)
	cli.registryV1 = registryv1client.New(cli)
	return cli, nil
}

func newRestyClient(baseURL, token string, hc *http.Client) *resty.Client {
	rc := resty.NewWithClient(hc).
		SetBaseURL(baseURL).
		SetHeader("Accept", "application/json")
	if token != "" {
		rc.SetAuthToken(token)
	}
	return rc
}

// NewFromConfig constructs a new API client from the unified platform Config.
func NewFromConfig(cfg *config.Config) (Interface, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	return New(&cfg.Client)
}

// NewRegistryFromConfig constructs a Registry client targeting cfg.Registry.Endpoint when set,
// falling back to the local cluster API server (cfg.Client) when running co-located.
func NewRegistryFromConfig(cfg *config.Config) (Interface, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	cc := cfg.Client
	if ep := strings.TrimSpace(cfg.Registry.Endpoint); ep != "" {
		cc.ServerURL = ep
	}
	if tok := strings.TrimSpace(cfg.Registry.Token); tok != "" {
		cc.Token = tok
	}
	if cfg.Registry.Timeout > 0 {
		cc.Timeout = cfg.Registry.Timeout
	}
	return New(&cc)
}

// NewRegistryWorkerFromConfig constructs a Registry worker client authenticated via cfg.Registry.WorkerToken.
func NewRegistryWorkerFromConfig(cfg *config.Config) (Interface, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}
	cc := cfg.Client
	if ep := strings.TrimSpace(cfg.Registry.Endpoint); ep != "" {
		cc.ServerURL = ep
	}
	if tok := strings.TrimSpace(cfg.Registry.WorkerToken); tok != "" {
		cc.Token = tok
	} else if tok := strings.TrimSpace(cfg.Registry.Token); tok != "" {
		cc.Token = tok
	}
	if cfg.Registry.Timeout > 0 {
		cc.Timeout = cfg.Registry.Timeout
	}
	return New(&cc)
}

func (c *Client) Config() *config.ClientConfig {
	return c.cfg
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) DefaultTenant() string {
	if c.cfg == nil || c.cfg.Tenant == "" {
		return "default"
	}
	return c.cfg.Tenant
}

func (c *Client) CoreV1() corev1client.CoreV1Interface {
	if c.coreV1 == nil {
		return corev1client.New(c)
	}
	return c.coreV1
}

func (c *Client) IAMV1() iamv1client.IAMV1Interface {
	if c.iamV1 == nil {
		return iamv1client.New(c)
	}
	return c.iamV1
}

func (c *Client) RegistryV1() registryv1client.RegistryV1Interface {
	if c.registryV1 == nil {
		return registryv1client.New(c)
	}
	return c.registryV1
}

func (c *Client) Get(ctx context.Context, resource, tenant, name string, out metav1.Object) error {
	return c.GetWithQuery(ctx, resource, tenant, name, nil, out)
}

func (c *Client) GetWithQuery(ctx context.Context, resource, tenant, name string, query map[string]string, out metav1.Object) error {
	info, err := ResolveResource(resource)
	if err != nil {
		return err
	}
	if info.TenantScoped && tenant == "" {
		tenant = c.cfg.Tenant
	}
	req := c.restyClient.R().SetContext(ctx)
	if len(query) > 0 {
		req.SetQueryParams(query)
	}
	if out != nil {
		req.SetResult(out)
	}
	resp, err := req.Get(ItemPath(info, tenant, name))
	if err != nil {
		return err
	}
	return FromRestyResponse(resp)
}

func (c *Client) List(ctx context.Context, resource, tenant string, opts ListOptions, out any) error {
	info, err := ResolveResource(resource)
	if err != nil {
		return err
	}
	if tenant == "" && opts.Tenant != "" {
		tenant = opts.Tenant
	}
	if info.TenantScoped && tenant == "" && info.Kind != "Instance" && info.Kind != "TenantMetering" {
		tenant = c.cfg.Tenant
	}
	req := c.restyClient.R().SetContext(ctx).SetQueryParams(buildListQueryParams(opts))
	if out != nil {
		req.SetResult(out)
	}
	resp, err := req.Get(CollectionPath(info, tenant))
	if err != nil {
		return err
	}
	return FromRestyResponse(resp)
}

func buildListQueryParams(opts ListOptions) map[string]string {
	q := make(map[string]string)
	for k, v := range opts.Query {
		if v != "" {
			q[k] = v
		}
	}
	if opts.NodeName != "" {
		q["nodeName"] = opts.NodeName
	}
	if opts.LabelSelector != "" {
		q["labelSelector"] = opts.LabelSelector
	}
	if opts.FieldSelector != "" {
		q["fieldSelector"] = opts.FieldSelector
	}
	if opts.Window != "" {
		q["window"] = opts.Window
	}
	if opts.Format != "" {
		q["format"] = opts.Format
	}
	if opts.Limit > 0 {
		q["limit"] = strconv.Itoa(opts.Limit)
	}
	if opts.Offset > 0 {
		q["offset"] = strconv.Itoa(opts.Offset)
	}
	return q
}

func (c *Client) Create(ctx context.Context, obj metav1.Object, out metav1.Object) error {
	if obj == nil {
		return errors.New("cannot create nil object")
	}
	info, err := ResolveObject(obj)
	if err != nil {
		return err
	}
	meta := obj.GetObjectMeta()
	if info.TenantScoped && meta.Tenant == "" {
		meta.Tenant = c.cfg.Tenant
	}
	return c.executeJSONObject(ctx, http.MethodPost, CollectionPath(info, meta.Tenant), obj, out)
}

func (c *Client) Update(ctx context.Context, obj metav1.Object, out metav1.Object) error {
	if obj == nil {
		return errors.New("cannot update nil object")
	}
	info, err := ResolveObject(obj)
	if err != nil {
		return err
	}
	meta := obj.GetObjectMeta()
	if info.TenantScoped && meta.Tenant == "" {
		meta.Tenant = c.cfg.Tenant
	}
	return c.executeJSONObject(ctx, http.MethodPut, ItemPath(info, meta.Tenant, meta.Name), obj, out)
}

func (c *Client) UpdateStatus(ctx context.Context, obj metav1.Object, out metav1.Object) error {
	if obj == nil {
		return errors.New("cannot update status of nil object")
	}
	info, err := ResolveObject(obj)
	if err != nil {
		return err
	}
	meta := obj.GetObjectMeta()
	if info.TenantScoped && meta.Tenant == "" {
		meta.Tenant = c.cfg.Tenant
	}
	return c.executeJSONObject(ctx, http.MethodPut, SubresourcePath(info, meta.Tenant, meta.Name, "status"), obj, out)
}

func (c *Client) executeJSONObject(ctx context.Context, method, relPath string, body any, out any) error {
	req := c.restyClient.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body)
	if out != nil {
		req.SetResult(out)
	}
	resp, err := req.Execute(method, relPath)
	if err != nil {
		return err
	}
	return FromRestyResponse(resp)
}

func (c *Client) Patch(ctx context.Context, resource, tenant, name string, patchData []byte, out metav1.Object) error {
	info, err := ResolveResource(resource)
	if err != nil {
		return err
	}
	if info.TenantScoped && tenant == "" {
		tenant = c.cfg.Tenant
	}
	req := c.restyClient.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/merge-patch+json").
		SetBody(patchData)
	if out != nil {
		req.SetResult(out)
	}
	resp, err := req.Patch(ItemPath(info, tenant, name))
	if err != nil {
		return err
	}
	return FromRestyResponse(resp)
}

func (c *Client) PostSubresource(ctx context.Context, resource, tenant, name, subresource string, body any, out any) error {
	info, err := ResolveResource(resource)
	if err != nil {
		return err
	}
	if info.TenantScoped && tenant == "" {
		tenant = c.cfg.Tenant
	}
	return c.executeJSONObject(ctx, http.MethodPost, SubresourcePath(info, tenant, name, subresource), body, out)
}

func (c *Client) UpdateInstanceState(ctx context.Context, tenant, name string, state corev1.InstanceStateValue, out *corev1.Instance) error {
	info, err := ResolveKind("Instance")
	if err != nil {
		return err
	}
	if tenant == "" {
		tenant = c.cfg.Tenant
	}
	payload := corev1.InstanceState{State: state}
	var resTarget any
	if out != nil {
		resTarget = out
	}
	return c.executeJSONObject(ctx, http.MethodPut, SubresourcePath(info, tenant, name, "state"), payload, resTarget)
}

func (c *Client) Delete(ctx context.Context, resource, tenant, name string) error {
	info, err := ResolveResource(resource)
	if err != nil {
		return err
	}
	if info.TenantScoped && tenant == "" {
		tenant = c.cfg.Tenant
	}
	resp, err := c.restyClient.R().SetContext(ctx).Delete(ItemPath(info, tenant, name))
	if err != nil {
		return err
	}
	return FromRestyResponse(resp)
}

func (c *Client) DeleteObject(ctx context.Context, obj metav1.Object) error {
	if obj == nil {
		return errors.New("cannot delete nil object")
	}
	info, err := ResolveObject(obj)
	if err != nil {
		return err
	}
	meta := obj.GetObjectMeta()
	return c.Delete(ctx, info.Plural, meta.Tenant, meta.Name)
}
