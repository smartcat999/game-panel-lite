package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/log"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

// Config is an alias for config.AgentConfig managed centrally in pkg/config.
type Config = config.AgentConfig

const agentStartingFollowupDelay = 1 * time.Second

func NewConfig() *Config {
	return config.NewAgentConfig()
}

type localInstance struct {
	instance *corev1.Instance
	rc       *corev1.RuntimeClass
	driver   runtime.Driver
}

// Agent runs on each compute host, reporting Node status heartbeats and reconciling assigned Instances via runtime.Driver.
type Agent struct {
	cfg      *Config
	drivers  *runtime.Registry
	clientMu sync.Mutex
	apiCli   client.Interface
	// activeToken remembers the last issued per-node JWT so renewals use the active
	// session credential instead of letting an initial 24h GP_AGENT_TOKEN expire.
	activeToken string
	// tokenRenewAt is when the per-node session token should be re-exchanged
	tokenRenewAt     time.Time
	collector        ResourceCollector
	reconcileCh      chan struct{}
	gatesMu          sync.Mutex
	gates            map[string]*uidGate
	runtimeSem       chan struct{}
	runtimeFailures  map[string]string
	reportedRestarts map[string]string
	operationTimeout time.Duration
	instancesMu      sync.RWMutex
	instances        map[string]*localInstance
	backupSem        chan struct{}
	httpServer       *http.Server
	upgrader         websocket.Upgrader
	handler          http.Handler
	warmImagesMu     sync.RWMutex
	warmImages       map[string]bool
}

func NewAgent(cfg *Config, drivers *runtime.Registry) *Agent {
	if cfg == nil {
		cfg = &config.Get().Agent
	}
	cfg.Default()
	ag := &Agent{
		cfg:         cfg,
		drivers:     drivers,
		collector:   NewHostResourceCollector(cfg.WorkDir, *cfg.ReservedResources),
		reconcileCh: make(chan struct{}, 1),
		instances:   make(map[string]*localInstance),
		backupSem:   make(chan struct{}, 1),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	ag.handler = ag.buildHandler()
	ag.installWorkspaceMaterializer()
	return ag
}

func (a *Agent) installWorkspaceMaterializer() {
	if a.drivers == nil {
		return
	}
	if drv, ok := a.drivers.Get(runtime.HandlerDocker); ok {
		if setter, ok := drv.(runtime.WorkspaceMaterializerSetter); ok {
			setter.SetWorkspaceMaterializer(&BlobWorkspaceMaterializer{
				WorkDir:  a.cfg.WorkDir,
				ClientFn: a.apiClient,
			})
		}
	}
}

func (a *Agent) apiClient() (client.Interface, error) {
	a.clientMu.Lock()
	defer a.clientMu.Unlock()
	if a.apiCli != nil && (a.tokenRenewAt.IsZero() || time.Now().Before(a.tokenRenewAt)) {
		return a.apiCli, nil
	}
	token, renewAt := a.exchangeNodeToken()
	cli, err := client.New(&config.ClientConfig{
		ServerURL: a.cfg.APIServerURL,
		Token:     token,
		Timeout:   10 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	a.apiCli = cli
	a.tokenRenewAt = renewAt
	return cli, nil
}

// exchangeNodeToken trades the configured credential (the shared bootstrap token, or a per-node
// token issued by an administrator) for this node's own session token, so every API call is made
// under the node's identity and NodeRestriction applies. If the exchange is not possible (e.g. an
// older apiserver), it falls back to the configured token.
func (a *Agent) tryIssueNodeToken(bearer string) (*iamv1.Session, error) {
	if strings.TrimSpace(bearer) == "" {
		return nil, errors.New("empty bearer token")
	}
	bootstrap, err := client.New(&config.ClientConfig{
		ServerURL: a.cfg.APIServerURL,
		Token:     bearer,
		Timeout:   10 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return bootstrap.IAMV1().Sessions().IssueNodeToken(ctx, a.cfg.NodeName)
}

func (a *Agent) exchangeNodeToken() (string, time.Time) {
	var (
		sess *iamv1.Session
		err  error
	)
	if a.activeToken != "" && a.activeToken != a.cfg.Token {
		sess, err = a.tryIssueNodeToken(a.activeToken)
	}
	if err != nil || sess == nil || sess.Token == "" {
		sess, err = a.tryIssueNodeToken(a.cfg.Token)
	}
	if err != nil || sess == nil || sess.Token == "" {
		log.S().Warnw("node token exchange failed, using configured token", "node", a.cfg.NodeName, "error", err)
		return a.cfg.Token, time.Now().Add(time.Minute)
	}
	a.activeToken = sess.Token
	renewAt := sess.ExpiresAt.Add(-10 * time.Minute)
	if sess.ExpiresAt.IsZero() || renewAt.Before(time.Now().Add(time.Minute)) {
		renewAt = time.Now().Add(time.Hour)
	}
	return sess.Token, renewAt
}

// resetAPIClient drops the cached client so the next call re-exchanges credentials
// (used after the apiserver rejects the current token).
func (a *Agent) resetAPIClient() {
	a.clientMu.Lock()
	a.apiCli = nil
	a.activeToken = ""
	a.tokenRenewAt = time.Time{}
	a.clientMu.Unlock()
}

// acceptedTokens returns the active per-node session token (if exchanged) and
// configured bootstrap token so apiserver proxies and signed file tickets match
// either credential during startup and periodic token renewal.
func (a *Agent) acceptedTokens() []string {
	a.clientMu.Lock()
	active := strings.TrimSpace(a.activeToken)
	a.clientMu.Unlock()
	var cfgTok string
	if a.cfg != nil {
		cfgTok = strings.TrimSpace(a.cfg.Token)
	}
	tokens := make([]string, 0, 2)
	if active != "" {
		tokens = append(tokens, active)
	}
	if cfgTok != "" && cfgTok != active {
		tokens = append(tokens, cfgTok)
	}
	return tokens
}

// RegisterInstance seeds the local execution cache. HTTP requests still validate
// the current primary object; registration never grants lifecycle authority.
func (a *Agent) RegisterInstance(inst *corev1.Instance, rc *corev1.RuntimeClass) error {
	if inst == nil {
		return errors.New("instance is required")
	}
	nodeName := a.cfg.NodeName
	if nodeName == "" {
		nodeName = inst.Spec.NodeName
	}
	var nodeUID string
	if c := inst.Status.ResourceControl; c != nil && c.CurrentAttempt != nil {
		nodeUID = c.CurrentAttempt.NodeUID
	}
	approved, err := approvedInstance(inst, nodeName, nodeUID)
	if err != nil {
		return err
	}
	inst = approved
	frozen, err := frozenRuntimeClass(inst)
	if err != nil {
		return err
	}
	if rc != nil && (rc.Handler != frozen.Handler || rc.Metadata.UID != "" && rc.Metadata.UID != frozen.Metadata.UID) {
		return errors.New("registered runtime class does not match the frozen backend")
	}
	rc = frozen
	drv, ok := a.drivers.Get(rc.Handler)
	if !ok {
		return fmt.Errorf("runtime handler %q not registered", rc.Handler)
	}
	a.instancesMu.Lock()
	defer a.instancesMu.Unlock()
	if a.instances == nil {
		a.instances = make(map[string]*localInstance)
	}
	a.instances[inst.Metadata.Tenant+"/"+inst.Metadata.Name] = &localInstance{
		instance: inst,
		rc:       rc,
		driver:   drv,
	}
	return nil
}

// SetCollector overrides the resource collector (useful for testing or customization).
func (a *Agent) SetCollector(collector ResourceCollector) {
	a.collector = collector
}

// Run executes the heartbeat and instance reconciliation loop until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	log.Info("starting node-agent", "nodeName", a.cfg.NodeName, "addr", a.cfg.Addr, "apiserverURL", a.cfg.APIServerURL)
	warnIfLowKernelMaxMapCount(defaultMaxMapCountPath)
	ln, err := net.Listen("tcp", a.cfg.Addr)
	if err != nil {
		return fmt.Errorf("agent listen on %s: %w", a.cfg.Addr, err)
	}
	defer func() { _ = ln.Close() }()

	srv := &http.Server{Handler: a.Handler()}
	a.httpServer = srv
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	if err := a.reportHeartbeat(ctx); err != nil {
		log.Warn("initial heartbeat failed", "error", err)
	}
	a.triggerReconcile()
	watchWg := a.startBackgroundLoops(ctx)
	return a.runReconcileEventLoop(ctx, srv, errCh, watchWg)
}

func (a *Agent) startBackgroundLoops(ctx context.Context) *sync.WaitGroup {
	var watchWg sync.WaitGroup
	watchWg.Add(3)
	go func() {
		defer watchWg.Done()
		hbTicker := time.NewTicker(a.cfg.HeartbeatInterval)
		defer hbTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-hbTicker.C:
				if err := a.reportHeartbeat(ctx); err != nil && client.IsUnauthorized(err) {
					a.resetAPIClient()
				}
			}
		}
	}()
	go func() {
		defer watchWg.Done()
		a.watchAssignedInstances(ctx)
	}()
	go func() {
		defer watchWg.Done()
		a.runPrewarmLoop(ctx)
	}()
	return &watchWg
}

func (a *Agent) runReconcileEventLoop(ctx context.Context, srv *http.Server, errCh <-chan error, watchWg *sync.WaitGroup) error {
	reconcileTicker := time.NewTicker(a.cfg.HeartbeatInterval)
	defer reconcileTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			shutdownTimeout := a.cfg.ShutdownTimeout
			if shutdownTimeout <= 0 {
				shutdownTimeout = 5 * time.Second
			}
			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
			watchWg.Wait()
			return nil
		case err := <-errCh:
			watchWg.Wait()
			return err
		case <-reconcileTicker.C:
			if err := a.reconcileAssignedInstances(ctx); err != nil && client.IsUnauthorized(err) {
				a.resetAPIClient()
			}
		case <-a.reconcileCh:
			if err := a.reconcileAssignedInstances(ctx); err != nil && client.IsUnauthorized(err) {
				a.resetAPIClient()
			}
		}
	}
}

func (a *Agent) triggerReconcile() {
	if a.reconcileCh == nil {
		return
	}
	select {
	case a.reconcileCh <- struct{}{}:
	default:
	}
}

func (a *Agent) watchAssignedInstances(ctx context.Context) {
	if strings.TrimSpace(a.cfg.APIServerURL) == "" {
		return
	}
	opts := client.ListOptions{NodeName: a.cfg.NodeName}
	for ctx.Err() == nil {
		cli, err := a.apiClient()
		if err == nil {
			err = cli.CoreV1().Instances("").Watch(ctx, opts, func(watchEv metav1.WatchEvent[map[string]any]) error {
				if a.shouldReconcileOnWatchEvent(watchEv) {
					a.triggerReconcile()
				}
				return nil
			})
		}
		if ctx.Err() != nil {
			return
		}
		backoff := 50 * time.Millisecond
		if err != nil {
			backoff = 100 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

func (a *Agent) shouldReconcileOnWatchEvent(watchEv metav1.WatchEvent[map[string]any]) bool {
	switch watchEv.Type {
	case metav1.WatchAdded, metav1.WatchDeleted:
		return true
	case metav1.WatchModified:
		return a.shouldReconcileModifiedWatchObject(watchEv.Object)
	default:
		return false
	}
}

func (a *Agent) shouldReconcileModifiedWatchObject(obj map[string]any) bool {
	meta, ok := obj["metadata"].(map[string]any)
	if !ok {
		return true
	}
	tenant, _ := meta["tenant"].(string)
	name, _ := meta["name"].(string)
	if tenant == "" || name == "" {
		return true
	}
	if delTs, ok := meta["deletionTimestamp"].(string); ok && strings.TrimSpace(delTs) != "" {
		return true
	}
	a.instancesMu.RLock()
	li, exists := a.instances[tenant+"/"+name]
	a.instancesMu.RUnlock()
	if !exists || li == nil || li.instance == nil {
		return true
	}
	if uid, ok := meta["uid"].(string); ok && uid != li.instance.Metadata.UID {
		return true
	}
	if gFloat, ok := meta["generation"].(float64); ok && int64(gFloat) > 0 && int64(gFloat) != li.instance.Status.ObservedGeneration {
		return true
	}
	var restartAt string
	if ann, ok := meta["annotations"].(map[string]any); ok {
		restartAt, _ = ann[corev1.AnnotationRestartRequestedAt].(string)
	}
	if restartAt != li.instance.Metadata.Annotations[corev1.AnnotationRestartRequestedAt] {
		return true
	}
	if specMap, ok := obj["spec"].(map[string]any); ok {
		if stateStr, ok := specMap["state"].(string); ok && stateStr != "" && stateStr != string(li.instance.Spec.State) {
			return true
		}
	}
	return watchResourceControlChanged(obj, li.instance.Status.ResourceControl)
}

func watchResourceControlChanged(obj map[string]any, previous *corev1.InstanceResourceControl) bool {
	statusMap, ok := obj["status"].(map[string]any)
	if !ok {
		return false
	}
	raw, err := json.Marshal(statusMap["resourceControl"])
	if err != nil {
		return true
	}
	var control *corev1.InstanceResourceControl
	if json.Unmarshal(raw, &control) != nil {
		return true
	}
	if control == nil || previous == nil {
		return control != nil || previous != nil
	}
	if control.ApprovedRevision != previous.ApprovedRevision || control.Suspended != previous.Suspended {
		return true
	}
	x, y := control.CurrentAttempt, previous.CurrentAttempt
	if x == nil || y == nil {
		return x != nil || y != nil
	}
	return x.ID != y.ID || x.Epoch != y.Epoch || x.NodeUID != y.NodeUID || x.NodeName != y.NodeName || x.Phase != y.Phase
}

// SyncOnce performs one heartbeat and reconciles all Instances assigned to this Node.
func (a *Agent) SyncOnce(ctx context.Context) error {
	if err := a.reportHeartbeat(ctx); err != nil {
		if client.IsUnauthorized(err) {
			a.resetAPIClient()
		}
		return err
	}
	err := a.reconcileAssignedInstances(ctx)
	if client.IsUnauthorized(err) {
		a.resetAPIClient()
	}
	return err
}

func (a *Agent) reportHeartbeat(ctx context.Context) error {
	ctx, cancel := a.operationContext(ctx)
	defer cancel()
	cli, err := a.apiClient()
	if err != nil {
		return err
	}
	node, err := a.getOrCreateNode(ctx, cli)
	if err != nil {
		return err
	}
	capacity, allocatable, nodeUsage, err := a.collectHostTelemetry(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_ = EnsureBlobCacheStorageInfo(a.cfg.WorkDir)
	cachedArtifacts, cachedBlobs := CollectNodeBlobCacheInventory(a.cfg.WorkDir)
	node.Status = corev1.NodeStatus{
		ObservedGeneration: node.Metadata.Generation,
		Phase:              corev1.NodePhaseReady,
		Addresses:          a.resolveNodeAddresses(node),
		Runtimes:           a.drivers.NodeRuntimes(),
		Capacity:           capacity,
		Allocatable:        allocatable,
		Usage:              nodeUsage,
		CachedArtifacts:    cachedArtifacts,
		CachedBlobs:        cachedBlobs,
		WarmImages:         a.snapshotWarmImages(),
		LastHeartbeatTime:  &now,
	}
	metav1.SetCondition(&node.Status.Conditions, metav1.Condition{
		Type: metav1.ConditionTypeReady, Status: metav1.ConditionTrue, ObservedGeneration: node.Metadata.Generation,
		Reason: corev1.ReasonAgentHealthy, Message: "Node agent is reporting ready",
	})
	runtimeReady, runtimeMessage := a.runtimeHealth()
	runtimeStatus, runtimeReason := metav1.ConditionTrue, corev1.ReasonRuntimeResponsive
	if !runtimeReady {
		runtimeStatus, runtimeReason = metav1.ConditionFalse, corev1.ReasonRuntimeOperationFailed
	}
	metav1.SetCondition(&node.Status.Conditions, metav1.Condition{Type: metav1.ConditionTypeRuntimeReady, Status: runtimeStatus, Reason: runtimeReason, Message: runtimeMessage, ObservedGeneration: node.Metadata.Generation})
	_, err = cli.CoreV1().Nodes().UpdateStatus(ctx, node)
	return err
}

func (a *Agent) getOrCreateNode(ctx context.Context, cli client.Interface) (*corev1.Node, error) {
	node, err := cli.CoreV1().Nodes().Get(ctx, a.cfg.NodeName)
	if err == nil || !client.IsNotFound(err) {
		return node, err
	}
	createNode := &corev1.Node{
		Metadata: metav1.ObjectMeta{
			Name:   a.cfg.NodeName,
			Labels: map[string]string{corev1.LabelTopologyRegion: a.cfg.Region, corev1.LabelKubernetesHostname: a.cfg.NodeName},
		},
		Spec: corev1.NodeSpec{Unschedulable: false, ExternalIP: strings.TrimSpace(a.cfg.ExternalIP)},
	}
	return cli.CoreV1().Nodes().Create(ctx, createNode)
}

func (a *Agent) collectHostTelemetry(ctx context.Context) (corev1.ResourceList, corev1.ResourceList, *corev1.NodeUsage, error) {
	collector := a.collector
	if collector == nil {
		collector = NewHostResourceCollector(a.cfg.WorkDir, *a.cfg.ReservedResources)
	}
	capacity, allocatable, err := collector.Collect(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("collect host resources: %w", err)
	}
	var nodeUsage *corev1.NodeUsage
	if uc, ok := collector.(UsageCollector); ok {
		if u, uErr := uc.CollectUsage(ctx); uErr == nil {
			nodeUsage = u
		}
	}
	metrics.ExportNodeUsage(ctx, a.cfg.NodeName, nodeUsage)
	return capacity, allocatable, nodeUsage, nil
}

func (a *Agent) resolveNodeAddresses(node *corev1.Node) []corev1.NodeAddress {
	internalIP := strings.TrimSpace(a.cfg.InternalIP)
	if internalIP == "" {
		internalIP = DetectInternalIP()
	}
	externalIP := strings.TrimSpace(node.Spec.ExternalIP)
	if externalIP == "" {
		externalIP = strings.TrimSpace(a.cfg.ExternalIP)
	}
	addresses := []corev1.NodeAddress{{Type: corev1.NodeAddressInternalIP, Address: internalIP}}
	if externalIP != "" {
		addresses = append(addresses, corev1.NodeAddress{Type: corev1.NodeAddressExternalIP, Address: externalIP})
	}
	return addresses
}

// reportCleanupFailure records a diagnostic only. A failed or unknown cleanup
// never removes protection, changes resource authority, or acknowledges release.
func (a *Agent) reportCleanupFailure(ctx context.Context, cli client.Interface, source *corev1.Instance, nodeUID, reason, message string) {
	ctx, cancel := a.operationContext(ctx)
	defer cancel()
	latest, err := cli.CoreV1().Instances(source.Metadata.Tenant).Get(ctx, source.Metadata.Name)
	if err != nil || latest.Metadata.DeletionTimestamp == nil || !sameExecution(source, latest) {
		return
	}
	if _, err := approvedInstance(latest, a.cfg.NodeName, nodeUID); err != nil {
		return
	}
	latest.Status.Phase = corev1.InstancePhaseTerminating
	metav1.SetCondition(&latest.Status.Conditions, metav1.Condition{Type: metav1.ConditionTypeRuntimeCleanup, Status: metav1.ConditionFalse, Reason: reason, Message: message, ObservedGeneration: latest.Metadata.Generation})
	if _, err := cli.CoreV1().Instances(latest.Metadata.Tenant).UpdateStatus(ctx, latest); err != nil {
		log.S().Warnw("failed to record cleanup diagnostic", "instance", latest.Metadata.Name, "error", err)
	}
}

func (a *Agent) deleteInstanceMetrics(inst *corev1.Instance) {
	metrics.DeleteInstanceSeries(inst.Metadata.Tenant, inst.Metadata.Name, a.cfg.NodeName)
	deleteInstanceUsageMetrics(a.cfg.NodeName, inst.Metadata.Tenant, inst.Metadata.Name)
}
