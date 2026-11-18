package instance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"reflect"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	promclient "github.com/smartcat999/game-panel-cloud/pkg/infra/prometheus"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/tasklog"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	appmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/app"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	backupdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/backup"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	quotadb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/quota"
	runtimeclassdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/runtimeclass"
	tenantdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/tenant"
)

type Operator interface {
	ListCluster(ctx context.Context, nodeName string, opts db.ListOptions) (*metav1.List[*corev1.Instance], error)
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Instance], error)
	Get(ctx context.Context, tenant, name string) (*corev1.Instance, error)
	Create(ctx context.Context, tenant string, inst *corev1.Instance) (*corev1.Instance, error)
	Update(ctx context.Context, tenant, name string, inst *corev1.Instance) (*corev1.Instance, error)
	UpdateStatus(ctx context.Context, tenant, name string, inst *corev1.Instance) (*corev1.Instance, error)
	Delete(ctx context.Context, tenant, name string) error

	GetState(ctx context.Context, tenant, name string) (*corev1.InstanceState, error)
	UpdateState(ctx context.Context, tenant, name string, req *corev1.InstanceState) (*corev1.Instance, error)
	GetMetrics(ctx context.Context, tenant, name string, window time.Duration) (*corev1.InstanceMetrics, error)
	StreamLogs(ctx context.Context, tenant, name string, opts ...drvruntime.LogOptions) (io.ReadCloser, error)
	StreamTaskLogs(ctx context.Context, tenant, name, taskID string, follow bool, tailLines int) (io.ReadCloser, error)
	Exec(ctx context.Context, tenant, name string, req *corev1.InstanceExec) (*corev1.InstanceExec, error)
	OpenExecTTY(ctx context.Context, tenant, name string, req drvruntime.InteractiveExecRequest) (drvruntime.InteractiveExecSession, error)
	Files(ctx context.Context, tenant, name string, req *corev1.InstanceFiles) (*corev1.InstanceFiles, error)
	ReconcileBackups(ctx context.Context, inst *corev1.Instance) error
}

const (
	CachePrefixInstanceMeta = "inst:meta"
	CachePrefixAppSpec      = "app:spec"
	DefaultInstanceMetaTTL  = 5 * time.Minute
	DefaultAppSpecCacheTTL  = 30 * time.Second
	DefaultAlertEvalTimeout = 5 * time.Second
	DefaultAgentListenPort  = 8082
)

type cachedInstanceMeta struct {
	Metadata      metav1.ObjectMeta             `json:"metadata"`
	CustomMetrics []corev1.InstanceCustomMetric `json:"customMetrics,omitempty"`
}

func instanceMetaCacheKey(tenant, name string) string {
	return fmt.Sprintf("%s:%s/%s", CachePrefixInstanceMeta, tenant, name)
}

func appSpecCacheKey(appName string) string {
	return CachePrefixAppSpec + ":" + appName
}

type operator struct {
	db            *instancedb.DB
	tenantDB      *tenantdb.DB
	rcDB          *runtimeclassdb.DB
	appDB         *appdb.DB
	nodeDB        *nodedb.DB
	quotaDB       *quotadb.DB
	backupDB      *backupdb.DB
	reg           *repo.Registry
	drivers       *drvruntime.Registry
	mq            mq.Interface
	taskLogs      tasklog.Interface
	s3            s3.Interface
	metricsClient promclient.Client
	localNodeName string
	agentPort     int
	agentToken    string
	httpClient    *http.Client
	nowFn         func() time.Time
}

type OperatorOption func(*operator)

func WithClock(nowFn func() time.Time) OperatorOption {
	return func(o *operator) { o.nowFn = nowFn }
}

func (o *operator) now() time.Time {
	if o != nil && o.nowFn != nil {
		return o.nowFn().UTC()
	}
	return time.Now().UTC()
}

func WithS3Client(cli s3.Interface) OperatorOption {
	return func(o *operator) { o.s3 = cli }
}

func (o *operator) s3Client() s3.Interface {
	if o.s3 != nil {
		return o.s3
	}
	if cfg := config.Get(); cfg != nil {
		return s3.NewClient(&cfg.S3, o.httpClient)
	}
	return s3.Default()
}

func WithLocalNodeName(name string) OperatorOption {
	return func(o *operator) { o.localNodeName = strings.TrimSpace(name) }
}

func WithAgentPort(port int) OperatorOption {
	return func(o *operator) { o.agentPort = port }
}

func WithAgentToken(token string) OperatorOption {
	return func(o *operator) { o.agentToken = strings.TrimSpace(token) }
}

func WithHTTPClient(client *http.Client) OperatorOption {
	return func(o *operator) { o.httpClient = client }
}

func WithTaskLog(tl tasklog.Interface) OperatorOption {
	return func(o *operator) { o.taskLogs = tl }
}

func WithMetricsClient(mc promclient.Client) OperatorOption {
	return func(o *operator) {
		if mc != nil {
			o.metricsClient = mc
		}
	}
}

func NewOperator(reg *repo.Registry, drivers *drvruntime.Registry, bus mq.Interface, opts ...OperatorOption) Operator {
	cfg := config.Get()
	op := &operator{
		db:            instancedb.NewDB(reg.Pool()),
		tenantDB:      tenantdb.NewDB(reg.Pool()),
		rcDB:          runtimeclassdb.NewDB(reg.Pool()),
		appDB:         appdb.NewDB(reg.Pool()),
		nodeDB:        nodedb.NewDB(reg.Pool()),
		quotaDB:       quotadb.NewDB(reg.Pool()),
		backupDB:      backupdb.NewDB(reg.Pool()),
		reg:           reg,
		drivers:       drivers,
		mq:            bus,
		taskLogs:      tasklog.New(cache.Default(), bus),
		metricsClient: promclient.DefaultClient(),
		agentPort:     cfg.Agent.Port(),
		agentToken:    strings.TrimSpace(cfg.Agent.Token),
		httpClient:    &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(op)
	}
	if op.agentPort <= 0 {
		op.agentPort = DefaultAgentListenPort
	}
	if op.httpClient == nil {
		op.httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if op.metricsClient == nil {
		op.metricsClient = promclient.DefaultClient()
	}
	return op
}

func (o *operator) resolveAppName(inst *corev1.Instance) string {
	if inst == nil {
		return ""
	}
	for _, k := range []string{appmodel.LabelAppPreset, corev1.LabelInstanceApp} {
		if v := strings.TrimSpace(inst.Metadata.Labels[k]); v != "" {
			return v
		}
	}
	return ""
}

func (o *operator) getAppCached(ctx context.Context, appName string, localCache map[string]*corev1.App) *corev1.App {
	if appName == "" || o.appDB == nil {
		return nil
	}
	if localCache != nil {
		if cached, ok := localCache[appName]; ok {
			return cached
		}
	}
	if cached, ok := cache.GetJSON[*corev1.App](ctx, nil, appSpecCacheKey(appName)); ok && cached != nil {
		if localCache != nil {
			localCache[appName] = cached
		}
		return cached
	}
	appObj, _ := o.appDB.Get(ctx, appName)
	if appObj != nil {
		_ = cache.SetJSON(ctx, nil, appSpecCacheKey(appName), appObj, DefaultAppSpecCacheTTL)
	}
	if localCache != nil {
		localCache[appName] = appObj
	}
	return appObj
}

func needsConsoleDeliveryHydration(inst *corev1.Instance) bool {
	if inst.Spec.BackupPolicy == nil {
		return false
	}
	cur := strings.TrimSpace(inst.Spec.BackupPolicy.ConsoleDelivery)
	return cur == "" || cur == drvruntime.ConsoleDeliveryStdin
}

func hydrateInstanceConsoleDelivery(appObj *corev1.App, inst *corev1.Instance) {
	if appObj == nil || inst == nil || inst.Spec.BackupPolicy == nil {
		return
	}
	appDelivery := strings.TrimSpace(appObj.ConsoleDelivery)
	if appDelivery == "" {
		return
	}
	cur := strings.TrimSpace(inst.Spec.BackupPolicy.ConsoleDelivery)
	if cur == "" || (cur == drvruntime.ConsoleDeliveryStdin && appDelivery != drvruntime.ConsoleDeliveryStdin) {
		inst.Spec.BackupPolicy.ConsoleDelivery = appDelivery
	}
}

func (o *operator) hydrateInstanceMetricsSpec(ctx context.Context, inst *corev1.Instance) {
	o.hydrateInstanceMetricsSpecWithCache(ctx, inst, nil)
}

func (o *operator) hydrateInstanceMetricsSpecWithCache(ctx context.Context, inst *corev1.Instance, appCache map[string]*corev1.App) {
	if inst == nil || o.appDB == nil {
		return
	}
	needMetrics := len(inst.Spec.Metrics) == 0
	needDelivery := needsConsoleDeliveryHydration(inst)
	if !needMetrics && !needDelivery {
		return
	}
	appName := o.resolveAppName(inst)
	if appName == "" {
		return
	}
	appObj := o.getAppCached(ctx, appName, appCache)
	if appObj == nil {
		return
	}
	hydrateInstanceConsoleDelivery(appObj, inst)
	if !needMetrics || len(appObj.Metrics) == 0 {
		return
	}
	values := make(map[string]string, len(appObj.ConfigSchema))
	for _, f := range appObj.ConfigSchema {
		if f.Default != "" {
			values[f.Name] = f.Default
		}
	}
	if rawJSON := strings.TrimSpace(inst.Metadata.Annotations[appmodel.AnnotationAppConfig]); rawJSON != "" {
		var saved map[string]string
		if json.Unmarshal([]byte(rawJSON), &saved) == nil {
			for k, v := range saved {
				values[k] = v
			}
		}
	}
	rendered := make([]corev1.AppMetricSpec, len(appObj.Metrics))
	for i, m := range appObj.Metrics {
		cp := m
		if cp.MaxConfigKey != "" {
			if v := strings.TrimSpace(values[cp.MaxConfigKey]); v != "" {
				cp.MaxValue = v
			}
		}
		rendered[i] = cp
	}
	inst.Spec.Metrics = rendered
}

func (o *operator) ListCluster(ctx context.Context, nodeName string, opts db.ListOptions) (*metav1.List[*corev1.Instance], error) {
	if nodeName != "" {
		fs := make(map[string]string, len(opts.FieldSelector)+1)
		for k, v := range opts.FieldSelector {
			fs[k] = v
		}
		fs["spec.nodeName"] = nodeName
		opts.FieldSelector = fs
	}
	res, err := o.db.List(ctx, opts)
	if err == nil && res != nil && len(res.Items) > 0 {
		appCache := make(map[string]*corev1.App, 4)
		for _, item := range res.Items {
			o.hydrateInstanceMetricsSpecWithCache(ctx, item, appCache)
		}
	}
	return res, err
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Instance], error) {
	res, err := o.db.List(ctx, opts)
	if err == nil && res != nil && len(res.Items) > 0 {
		appCache := make(map[string]*corev1.App, 4)
		for _, item := range res.Items {
			o.hydrateInstanceMetricsSpecWithCache(ctx, item, appCache)
		}
	}
	return res, err
}

func (o *operator) Get(ctx context.Context, tenant, name string) (*corev1.Instance, error) {
	inst, err := o.db.Get(ctx, tenant, name)
	if err == nil && inst != nil {
		o.hydrateInstanceMetricsSpec(ctx, inst)
	}
	return inst, err
}

func (o *operator) GetState(ctx context.Context, tenant, name string) (*corev1.InstanceState, error) {
	existing, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	return &corev1.InstanceState{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceState"},
		Metadata: existing.Metadata,
		State:    existing.Spec.State,
	}, nil
}

func (o *operator) UpdateState(ctx context.Context, tenant, name string, req *corev1.InstanceState) (*corev1.Instance, error) {
	existing, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	target, err := applyStateTransition(existing, req, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if target.Spec.State == corev1.InstanceStateRunning && o.tenantDB != nil {
		if t, tErr := o.tenantDB.Get(ctx, tenant); tErr == nil && t != nil && t.Status.Phase == corev1.TenantPhaseSuspended {
			return nil, fmt.Errorf("%w: workspace subscription is suspended; renew subscription before starting instances", models.ErrConflict)
		}
	}
	updated, err := o.db.Update(ctx, target)
	if err != nil {
		return nil, err
	}
	o.publishInstanceEvent(ctx, mq.EventStateChanged, updated)
	return updated, nil
}

func applyStateTransition(existing *corev1.Instance, req *corev1.InstanceState, now time.Time) (*corev1.Instance, error) {
	if req.Metadata.UID != "" && existing.Metadata.UID != "" && req.Metadata.UID != existing.Metadata.UID {
		return nil, fmt.Errorf("%w: instance UID mismatch (expected %s, got %s)", models.ErrConflict, existing.Metadata.UID, req.Metadata.UID)
	}
	if existing.Metadata.DeletionTimestamp != nil {
		return nil, fmt.Errorf("%w: deleting instance spec is frozen", models.ErrConflict)
	}
	target := *existing
	if existing.Metadata.Annotations != nil {
		target.Metadata.Annotations = maps.Clone(existing.Metadata.Annotations)
	}
	if req.Metadata.ResourceVersion != "" {
		target.Metadata.ResourceVersion = req.Metadata.ResourceVersion
	}
	switch req.State {
	case corev1.InstanceStateRestart:
		target.Spec.State = corev1.InstanceStateRunning
		if target.Metadata.Annotations == nil {
			target.Metadata.Annotations = make(map[string]string)
		}
		target.Metadata.Annotations[corev1.AnnotationRestartRequestedAt] = now.Format(time.RFC3339Nano)
	case corev1.InstanceStateRunning:
		target.Spec.State = corev1.InstanceStateRunning
		if target.Status.Phase == corev1.InstancePhaseFailed {
			if target.Metadata.Annotations == nil {
				target.Metadata.Annotations = make(map[string]string)
			}
			target.Metadata.Annotations[corev1.AnnotationRestartRequestedAt] = now.Format(time.RFC3339Nano)
		}
	default:
		target.Spec.State = req.State
	}
	return &target, nil
}

func intentCopy(in *corev1.Instance) (*corev1.Instance, error) {
	if in == nil {
		return nil, fmt.Errorf("%w: instance is required", models.ErrBadRequest)
	}
	data, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out corev1.Instance
	err = json.Unmarshal(data, &out)
	return &out, err
}

func normalizedIntentResources(in *corev1.Instance) (corev1.Resources, error) {
	return corev1.NormalizeInstanceResources(in.Spec.Resources, corev1.ResourceList{corev1.ResourceCPU: "1", corev1.ResourceMemory: "1Gi", corev1.ResourceStorage: "5Gi"})
}

func (o *operator) Create(ctx context.Context, tenant string, in *corev1.Instance) (*corev1.Instance, error) {
	inst, err := intentCopy(in)
	if err != nil {
		return nil, err
	}
	inst.Metadata.Tenant, inst.Metadata.UID, inst.Metadata.ResourceVersion = tenant, "", ""
	if inst.Spec.NodeName != "" {
		return nil, fmt.Errorf("%w: use nodeSelector; nodeName is assigned by the controller", models.ErrBadRequest)
	}
	parent, err := o.tenantDB.Get(ctx, tenant)
	if err != nil {
		return nil, err
	}
	if parent.Metadata.DeletionTimestamp != nil {
		return nil, fmt.Errorf("%w: tenant is terminating", models.ErrConflict)
	}
	if inst.Spec.RuntimeClassName == "" {
		class, e := o.resolveDefaultRuntimeClass(ctx)
		if e != nil {
			return nil, e
		}
		if class != nil {
			inst.Spec.RuntimeClassName = class.Metadata.Name
		}
	}
	inst.DefaultMeta()
	if err := o.renderAndValidateInstanceSpec(ctx, inst, true); err != nil {
		return nil, err
	}
	budget, err := normalizedIntentResources(inst)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	inst.Spec.Resources = corev1.InstanceRequirements(budget)
	if err := o.validateRuntimeClassDriver(ctx, inst); err != nil {
		return nil, err
	}
	RemoveFinalizer(&inst.Metadata, corev1.FinalizerInstanceCleanup)
	inst.Status = corev1.InstanceStatus{
		Phase:           corev1.InstancePhasePending,
		ResourceControl: &corev1.InstanceResourceControl{TenantUID: parent.Metadata.UID, InitialResources: corev1.CanonicalResources(budget)},
	}
	created, err := CreateIntent(ctx, o.db, inst)
	if err != nil {
		return nil, err
	}
	o.publishInstanceEvent(ctx, mq.EventCreated, created)
	return created, nil
}

func (o *operator) renderAndValidateInstanceSpec(ctx context.Context, inst *corev1.Instance, render bool) error {
	if render && o.appDB != nil {
		if preset := o.resolveAppName(inst); preset != "" {
			app, e := o.appDB.Get(ctx, preset)
			if e != nil {
				return e
			}
			if e = appmodel.RenderInstanceSpec(app, inst); e != nil {
				return fmt.Errorf("%w: %v", models.ErrBadRequest, e)
			}
		} else {
			o.stampAppLabelFromArtifact(ctx, inst)
		}
	}
	if err := inst.Validate(); err != nil {
		return fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	return nil
}

func (o *operator) stampAppLabelFromArtifact(ctx context.Context, inst *corev1.Instance) {
	if inst == nil || inst.Spec.ServerArtifact == nil || o.appDB == nil {
		return
	}
	repo := strings.TrimSpace(inst.Spec.ServerArtifact.Repository)
	if repo == "" {
		return
	}
	apps, err := o.appDB.List(ctx, db.ListOptions{SkipTotalCount: true})
	if err != nil || apps == nil {
		return
	}
	for _, a := range apps.Items {
		if a == nil || a.ServerBinding == nil {
			continue
		}
		if strings.TrimSpace(a.ServerBinding.Repository) == repo {
			if inst.Metadata.Labels == nil {
				inst.Metadata.Labels = make(map[string]string, 1)
			}
			inst.Metadata.Labels[corev1.LabelInstanceApp] = a.Metadata.Name
			return
		}
	}
}

func (o *operator) validateRuntimeClassDriver(ctx context.Context, inst *corev1.Instance) error {
	class, err := o.rcDB.Get(ctx, inst.Spec.RuntimeClassName)
	if err != nil {
		return fmt.Errorf("%w: runtimeClass: %v", models.ErrBadRequest, err)
	}
	if o.drivers != nil {
		driver, ok := o.drivers.Get(class.Handler)
		if !ok {
			return fmt.Errorf("%w: unsupported runtime handler %s", models.ErrBadRequest, class.Handler)
		}
		if err = driver.Validate(inst, class); err != nil {
			return fmt.Errorf("%w: %v", models.ErrBadRequest, err)
		}
	}
	return nil
}

func (o *operator) Update(ctx context.Context, tenant, name string, in *corev1.Instance) (*corev1.Instance, error) {
	incoming, err := intentCopy(in)
	if err != nil {
		return nil, err
	}
	current, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	if (incoming.Metadata.UID != "" && incoming.Metadata.UID != current.Metadata.UID) ||
		(incoming.Metadata.ResourceVersion != "" && incoming.Metadata.ResourceVersion != current.Metadata.ResourceVersion) {
		return nil, db.ErrConflict
	}
	if current.Status.ResourceControl == nil {
		return nil, fmt.Errorf("%w: instance resource intent is missing", models.ErrConflict)
	}
	if err := o.prepareIncomingUpdate(ctx, tenant, name, current, incoming, in); err != nil {
		return nil, err
	}
	if err := planResourceChangeIfNeeded(current, incoming); err != nil {
		return nil, err
	}
	preserveBackupPolicyStorageFields(current.Spec.BackupPolicy, incoming.Spec.BackupPolicy)
	current.Spec = incoming.Spec
	current.Metadata.Labels = incoming.Metadata.Labels
	current.Metadata.Annotations = incoming.Metadata.Annotations
	current.Metadata.Finalizers = incoming.Metadata.Finalizers
	AddFinalizer(&current.Metadata, FinalizerResources)
	updated, err := CommitControl(ctx, o.db, current)
	if err != nil {
		return nil, err
	}
	o.publishInstanceEvent(ctx, mq.EventUpdated, updated)
	return updated, nil
}

func (o *operator) prepareIncomingUpdate(ctx context.Context, tenant, name string, current, incoming, rawIn *corev1.Instance) error {
	incoming.Metadata.Tenant, incoming.Metadata.Name = tenant, name
	if incoming.Spec.RuntimeClassName == "" {
		incoming.Spec.RuntimeClassName = current.Spec.RuntimeClassName
	}
	if incoming.Spec.NodeName == "" {
		incoming.Spec.NodeName = current.Spec.NodeName
	}
	if incoming.Spec.NodeName != current.Spec.NodeName || incoming.Spec.RuntimeClassName != current.Spec.RuntimeClassName {
		return fmt.Errorf("%w: placement and runtime class require a separate placement intent", models.ErrConflict)
	}
	incoming.DefaultMeta()
	syncServerArtifactVersionUpdate(current, incoming)
	if err := o.renderAndValidateInstanceSpec(ctx, incoming, current.Metadata.DeletionTimestamp == nil); err != nil {
		return err
	}
	if !reflect.DeepEqual(incoming.Spec.Ports, current.Spec.Ports) {
		return fmt.Errorf("%w: port changes require a separate persistent plan", models.ErrConflict)
	}
	removingCleanup := HasFinalizer(&current.Metadata, corev1.FinalizerInstanceCleanup) && !HasFinalizer(&incoming.Metadata, corev1.FinalizerInstanceCleanup)
	if removingCleanup && current.Metadata.DeletionTimestamp == nil {
		AddFinalizer(&incoming.Metadata, corev1.FinalizerInstanceCleanup)
		removingCleanup = false
	}
	if removingCleanup {
		if err := authorizeInstanceCleanup(ctx, o.nodeDB, current); err != nil {
			return err
		}
		if current.Metadata.DeletionTimestamp == nil || rawIn.Metadata.UID != current.Metadata.UID || !sameIntentExecution(rawIn.Status.ResourceControl, current.Status.ResourceControl) {
			return fmt.Errorf("%w: cleanup source does not match the current binding", models.ErrConflict)
		}
	}
	if current.Metadata.DeletionTimestamp != nil && !o.matchesFrozenDeletingSpec(ctx, current, incoming) {
		return fmt.Errorf("%w: deleting instance spec is frozen", models.ErrConflict)
	}
	return nil
}

func (o *operator) matchesFrozenDeletingSpec(ctx context.Context, current, incoming *corev1.Instance) bool {
	if reflect.DeepEqual(incoming.Spec, current.Spec) {
		return true
	}
	hydrated, err := intentCopy(current)
	if err != nil {
		return false
	}
	hydrated.DefaultMeta()
	o.hydrateInstanceMetricsSpec(ctx, hydrated)
	if !reflect.DeepEqual(incoming.Spec, hydrated.Spec) {
		return false
	}
	incoming.Spec = current.Spec
	return true
}

func syncServerArtifactVersionUpdate(current, incoming *corev1.Instance) {
	if current == nil || incoming == nil || incoming.Spec.ServerArtifact == nil {
		return
	}
	inArtVer := strings.TrimSpace(incoming.Spec.ServerArtifact.Version)
	if inArtVer == "" {
		return
	}
	var curArtVer string
	if current.Spec.ServerArtifact != nil {
		curArtVer = strings.TrimSpace(current.Spec.ServerArtifact.Version)
	}
	curAnnVer := strings.TrimSpace(current.Metadata.Annotations[appmodel.AnnotationAppVersion])
	inAnnVer := strings.TrimSpace(incoming.Metadata.Annotations[appmodel.AnnotationAppVersion])
	if inArtVer != curArtVer && inAnnVer == curAnnVer {
		if incoming.Metadata.Annotations == nil {
			incoming.Metadata.Annotations = make(map[string]string)
		}
		incoming.Metadata.Annotations[appmodel.AnnotationAppVersion] = inArtVer
	}
}

func planResourceChangeIfNeeded(current, incoming *corev1.Instance) error {
	control := current.Status.ResourceControl
	target, err := normalizedIntentResources(incoming)
	if err != nil {
		return fmt.Errorf("%w: %v", models.ErrBadRequest, err)
	}
	if current.Metadata.DeletionTimestamp == nil {
		incoming.Spec.Resources = corev1.InstanceRequirements(target)
	}
	previous, err := normalizedIntentResources(current)
	if err != nil {
		return err
	}
	if target != previous {
		if current.Metadata.DeletionTimestamp != nil || control.Stage != string(StageActive) || control.Change != nil || control.CurrentAttempt == nil || control.CurrentAttempt.Phase != AttemptBound {
			return fmt.Errorf("%w: wait for the current resource intent to settle", models.ErrConflict)
		}
		if control.ApprovedRevision == math.MaxInt64 {
			return fmt.Errorf("%w: resource revision overflow", corev1.InvariantViolation)
		}
		control.DesiredRevision = control.ApprovedRevision + 1
		control.Change = &corev1.InstanceResourceChange{
			ID: "change:" + db.NewUID(), FromRevision: control.ApprovedRevision, ToRevision: control.DesiredRevision,
			OldResources: control.ApprovedResources, TargetResources: corev1.CanonicalResources(target), Phase: ChangeBudgeting,
		}
	}
	if current.Metadata.DeletionTimestamp != nil && !reflect.DeepEqual(incoming.Spec, current.Spec) {
		return fmt.Errorf("%w: deleting instance spec is frozen", models.ErrConflict)
	}
	return nil
}

func (o *operator) Delete(ctx context.Context, tenant, name string) error {
	current, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return err
	}
	marked, err := RequestDelete(ctx, o.db, tenant, name, current.Metadata.UID)
	if err != nil {
		return err
	}
	o.publishInstanceEvent(ctx, mq.EventUpdated, marked)
	return nil
}

func preserveBackupPolicyStorageFields(existing, incoming *corev1.InstanceBackupPolicy) {
	if existing == nil || incoming == nil {
		return
	}
	if strings.TrimSpace(incoming.Subtree) == "" && strings.TrimSpace(existing.Subtree) != "" {
		incoming.Subtree = existing.Subtree
	}
	if len(incoming.AnchorFiles) == 0 && len(existing.AnchorFiles) > 0 {
		incoming.AnchorFiles = append([]string(nil), existing.AnchorFiles...)
	}
	if strings.TrimSpace(incoming.ConsoleDelivery) == "" && strings.TrimSpace(existing.ConsoleDelivery) != "" {
		incoming.ConsoleDelivery = existing.ConsoleDelivery
	}
	if strings.TrimSpace(incoming.PreBackupCommand) == "" && strings.TrimSpace(existing.PreBackupCommand) != "" {
		incoming.PreBackupCommand = existing.PreBackupCommand
	}
	if strings.TrimSpace(incoming.PostBackupCommand) == "" && strings.TrimSpace(existing.PostBackupCommand) != "" {
		incoming.PostBackupCommand = existing.PostBackupCommand
	}
	if !incoming.ReplaceOnRestore && existing.ReplaceOnRestore {
		incoming.ReplaceOnRestore = true
	}
	if !incoming.BackupRequiresStop && existing.BackupRequiresStop {
		incoming.BackupRequiresStop = true
	}
}

func effectiveBackupRetentionCount(inst *corev1.Instance) int {
	if inst == nil {
		return 0
	}
	retention := 0
	if inst.Spec.BackupPolicy != nil && inst.Spec.BackupPolicy.RetentionCount > 0 {
		retention = inst.Spec.BackupPolicy.RetentionCount
	}
	for _, s := range inst.Spec.Schedules {
		if s.Enabled && strings.EqualFold(strings.TrimSpace(s.Action), corev1.ScheduleActionBackup) && s.RetainCount > 0 {
			if retention == 0 || s.RetainCount < retention {
				retention = s.RetainCount
			}
		}
	}
	return retention
}

func (o *operator) publishInstanceEvent(ctx context.Context, eventType mq.EventType, inst *corev1.Instance) {
	if o.mq == nil || inst == nil {
		return
	}
	ev := mq.NewEvent(eventType, corev1.GroupVersion, "Instance", inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.Generation, inst.Metadata.ResourceVersion)
	ev.Attributes = map[string]string{
		"runtimeClassName": inst.Spec.RuntimeClassName,
		"state":            string(inst.Spec.State),
	}
	_ = o.mq.Publish(ctx, mq.TopicInstance, ev)
	_ = o.mq.Publish(ctx, mq.TopicTenant, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Tenant", "", inst.Metadata.Tenant, 0, ""))
}

func (o *operator) resolveDefaultRuntimeClass(ctx context.Context) (*corev1.RuntimeClass, error) {
	list, err := o.rcDB.List(ctx, db.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, rc := range list.Items {
		if rc.IsDefault {
			return rc, nil
		}
	}
	return nil, models.ErrNotFound
}

func (o *operator) driverForInstance(_ context.Context, inst *corev1.Instance) (drvruntime.Driver, error) {
	if inst.Status.ResourceControl == nil || inst.Status.ResourceControl.RuntimeClass == nil {
		return nil, fmt.Errorf("%w: instance has no frozen runtime backend", models.ErrConflict)
	}
	binding := inst.Status.ResourceControl.RuntimeClass
	if binding.UID == "" || binding.Name == "" || binding.Handler == "" {
		return nil, fmt.Errorf("%w: instance runtime binding is incomplete", models.ErrConflict)
	}
	drv, ok := o.drivers.Get(binding.Handler)
	if !ok {
		return nil, fmt.Errorf("%w: runtime handler not available: %s", models.ErrBadRequest, binding.Handler)
	}
	return drv, nil
}
