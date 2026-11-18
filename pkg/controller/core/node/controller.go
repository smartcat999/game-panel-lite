package node

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	ctrlruntime "github.com/smartcat999/game-panel-cloud/pkg/controller/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	alertmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/alert"
	prewarmjobmodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/prewarmjob"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	appdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/app"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
	prewarmjobdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/prewarmjob"
)

const nodeResyncBatchSize = 200

func init() {
	ctrlruntime.Register(&Controller{})
}

type Controller struct {
	mu           sync.Mutex
	resyncOffset int
}

func (c *Controller) Name() string  { return "02-node-lifecycle" }
func (c *Controller) Topic() string { return mq.TopicNode }

// Resync lists a bounded page of Nodes per pass whose heartbeat has timed out beyond cfg.NodeGraceDuration,
// whose spec.externalIP is out of sync, or whose node account policy version has drifted.
func (c *Controller) Resync(ctx context.Context, reg *repo.Registry, cfg *ctrlruntime.Config) ([]mq.Event, error) {
	c.mu.Lock()
	offset := c.resyncOffset
	c.mu.Unlock()

	nodes, err := nodedb.NewDB(reg.Pool()).List(ctx, db.ListOptions{
		Limit:          nodeResyncBatchSize,
		Offset:         offset,
		SkipTotalCount: true,
	})
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(nodes.Items) == nodeResyncBatchSize {
		c.resyncOffset = offset + nodeResyncBatchSize
	} else {
		c.resyncOffset = 0
	}
	c.mu.Unlock()

	now := time.Now().UTC()
	var grace time.Duration
	if cfg != nil {
		grace = cfg.NodeGraceDuration
	}
	var events []mq.Event
	for _, n := range nodes.Items {
		if heartbeatExpired(n, now, grace) || externalIPOutOfSync(n) || accountOutOfSync(ctx, reg, n) {
			ev := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Node", "", n.Metadata.Name, n.Metadata.Generation, n.Metadata.ResourceVersion)
			ev.Topic = mq.TopicNode
			events = append(events, ev)
		}
	}
	return events, nil
}

func (c *Controller) Reconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, cfg *ctrlruntime.Config, event mq.Event) error {
	if event.Name == "" {
		return nil
	}
	if event.Type == mq.EventDeleted {
		enqueueHostedInstancesReconcile(ctx, reg, bus, event.Name)
		return nil
	}
	n, err := nodedb.NewDB(reg.Pool()).Get(ctx, event.Name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			enqueueHostedInstancesReconcile(ctx, reg, bus, event.Name)
			return nil
		}
		return err
	}
	var prevPhase corev1.NodePhase
	if event.Attributes != nil {
		prevPhase = corev1.NodePhase(event.Attributes["prevPhase"])
	}
	alertmodel.EvaluateNodeAlerts(ctx, reg, bus, prevPhase, n)
	alertmodel.RefreshAlertSnapshotAsync(reg)
	n, err = reconcileNodeAccountAndExternalIP(ctx, reg, event.Name, event.Type == mq.EventUpdated && event.Attributes == nil)
	if err != nil || n == nil {
		return err
	}
	if event.Type == mq.EventUpdated && event.Attributes != nil && event.Attributes["phaseChanged"] == "false" {
		return nil
	}
	if cfg != nil && heartbeatExpired(n, time.Now().UTC(), cfg.NodeGraceDuration) {
		n, err = markNodeHeartbeatExpired(ctx, reg, bus, event.Name, cfg.NodeGraceDuration)
		if err != nil || n == nil {
			return err
		}
	}
	if n.Status.Phase == corev1.NodePhaseNotReady {
		enqueueHostedInstancesReconcile(ctx, reg, bus, n.Metadata.Name)
		return nil
	}
	if n.Status.Phase == corev1.NodePhaseReady && bus != nil {
		if !n.Spec.Unschedulable {
			wakeUnscheduledInstances(ctx, reg, bus)
		}
		wakeMatchingPrewarmJobs(ctx, reg, bus, n)
	}
	return nil
}

func reconcileNodeAccountAndExternalIP(ctx context.Context, reg *repo.Registry, nodeName string, specUpdated bool) (*corev1.Node, error) {
	nodeDB := nodedb.NewDB(reg.Pool())
	cur, gErr := nodeDB.Get(ctx, nodeName)
	if gErr != nil {
		if errors.Is(gErr, db.ErrNotFound) {
			return nil, nil
		}
		return nil, gErr
	}
	if err := syncNodeAccount(ctx, reg, cur); err != nil {
		return nil, err
	}
	if !syncNodeExternalIP(cur, specUpdated) {
		return cur, nil
	}
	updated, uErr := nodeDB.UpdateStatus(ctx, cur)
	if uErr != nil {
		return nil, uErr
	}
	if err := syncNodeAccount(ctx, reg, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

func markNodeHeartbeatExpired(ctx context.Context, reg *repo.Registry, bus mq.Interface, nodeName string, grace time.Duration) (*corev1.Node, error) {
	nodeDB := nodedb.NewDB(reg.Pool())
	cur, gErr := nodeDB.Get(ctx, nodeName)
	if gErr != nil {
		if errors.Is(gErr, db.ErrNotFound) {
			return nil, nil
		}
		return nil, gErr
	}
	if !heartbeatExpired(cur, time.Now().UTC(), grace) {
		return cur, nil
	}
	cur.Status.Phase = corev1.NodePhaseNotReady
	metav1.SetCondition(&cur.Status.Conditions, metav1.Condition{
		Type: metav1.ConditionTypeReady, Status: metav1.ConditionFalse, ObservedGeneration: cur.Metadata.Generation,
		Reason: corev1.ReasonHeartbeatTimeout, Message: "Node heartbeat exceeded grace duration",
	})
	updated, uErr := nodeDB.UpdateStatus(ctx, cur)
	if uErr != nil {
		return nil, uErr
	}
	if err := syncNodeAccount(ctx, reg, updated); err != nil {
		return nil, err
	}
	alertmodel.NotifyNodeHeartbeatExpired(ctx, reg, bus, updated)
	alertmodel.RefreshAlertSnapshotAsync(reg)
	return updated, nil
}

func wakeUnscheduledInstances(ctx context.Context, reg *repo.Registry, bus mq.Interface) {
	instances, err := instancedb.NewDB(reg.Pool()).List(ctx, db.ListOptions{
		FieldSelector: map[string]string{"spec.nodeName": ""},
		Limit:         nodeResyncBatchSize,
	})
	if err != nil || instances == nil {
		return
	}
	for _, inst := range instances.Items {
		if inst != nil && inst.Spec.NodeName == "" {
			_ = bus.Publish(ctx, mq.TopicInstance, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Instance", inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.Generation, inst.Metadata.ResourceVersion))
		}
	}
}

func wakeMatchingPrewarmJobs(ctx context.Context, reg *repo.Registry, bus mq.Interface, n *corev1.Node) {
	jobs, err := prewarmjobdb.NewDB(reg.Pool()).List(ctx, db.ListOptions{SkipTotalCount: true})
	if err != nil || jobs == nil || n == nil {
		return
	}
	var repoToApp map[string]string
	if apps, aErr := appdb.NewDB(reg.Pool()).List(ctx, db.ListOptions{SkipTotalCount: true}); aErr == nil && apps != nil {
		repoToApp = prewarmjobmodel.BuildRepoToAppMap(apps.Items)
	}
	for _, job := range jobs.Items {
		if job == nil {
			continue
		}
		tracked := false
		for _, ns := range job.Status.Nodes {
			if ns.NodeName == n.Metadata.Name {
				tracked = true
				break
			}
		}
		if prewarmjobmodel.NodeMatchesJob(n, job, repoToApp) != tracked {
			ev := mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, corev1.KindPrewarmJob, "", job.Metadata.Name, job.Metadata.Generation, job.Metadata.ResourceVersion)
			ev.Topic = mq.TopicPrewarmJob
			_ = bus.Publish(ctx, mq.TopicPrewarmJob, ev)
		}
	}
}

func enqueueHostedInstancesReconcile(ctx context.Context, reg *repo.Registry, bus mq.Interface, nodeName string) {
	instDB := instancedb.NewDB(reg.Pool())
	if bus == nil || instDB == nil || nodeName == "" {
		return
	}
	list, err := instDB.List(ctx, db.ListOptions{FieldSelector: map[string]string{"spec.nodeName": nodeName}})
	if err != nil || list == nil {
		return
	}
	for _, inst := range list.Items {
		if inst == nil {
			continue
		}
		_ = bus.Publish(ctx, mq.TopicInstance, mq.NewEvent(mq.EventReconcile, corev1.GroupVersion, "Instance", inst.Metadata.Tenant, inst.Metadata.Name, inst.Metadata.Generation, inst.Metadata.ResourceVersion))
	}
}

func externalIPOutOfSync(n *corev1.Node) bool {
	if n == nil {
		return false
	}
	desired := strings.TrimSpace(n.Spec.ExternalIP)
	if desired == "" {
		return false
	}
	var current string
	var count int
	for _, a := range n.Status.Addresses {
		if a.Type == corev1.NodeAddressExternalIP {
			current = strings.TrimSpace(a.Address)
			count++
		}
	}
	return count != 1 || current != desired
}

func syncNodeExternalIP(n *corev1.Node, clearWhenEmpty bool) bool {
	if n == nil {
		return false
	}
	desired := strings.TrimSpace(n.Spec.ExternalIP)
	var current string
	var count int
	for _, a := range n.Status.Addresses {
		if a.Type == corev1.NodeAddressExternalIP {
			current = strings.TrimSpace(a.Address)
			count++
		}
	}
	if desired == "" {
		if !clearWhenEmpty || count == 0 {
			return false
		}
		var next []corev1.NodeAddress
		for _, a := range n.Status.Addresses {
			if a.Type != corev1.NodeAddressExternalIP {
				next = append(next, a)
			}
		}
		n.Status.Addresses = next
		return true
	}
	if count == 1 && current == desired {
		return false
	}
	var next []corev1.NodeAddress
	for _, a := range n.Status.Addresses {
		if a.Type != corev1.NodeAddressExternalIP {
			next = append(next, a)
		}
	}
	next = append(next, corev1.NodeAddress{Type: corev1.NodeAddressExternalIP, Address: desired})
	n.Status.Addresses = next
	return true
}

func heartbeatExpired(n *corev1.Node, now time.Time, grace time.Duration) bool {
	if n == nil || n.Status.LastHeartbeatTime == nil || grace <= 0 {
		return false
	}
	return now.Sub(*n.Status.LastHeartbeatTime) > grace && n.Status.Phase != corev1.NodePhaseNotReady
}

func nodeLimits(n *corev1.Node) (corev1.Limits, error) {
	return corev1.NormalizeNodeLimits(n.Status.Capacity, n.Status.Allocatable)
}

func nodePolicyVersion(n *corev1.Node) (int64, error) {
	if n == nil || n.Metadata.UID == "" {
		return 0, fmt.Errorf("node requires a UID for capacity policy")
	}
	rv, err := strconv.ParseInt(n.Metadata.ResourceVersion, 10, 64)
	if err != nil || rv <= 0 {
		return 0, fmt.Errorf("node %q requires a positive resourceVersion for capacity policy", n.Metadata.Name)
	}
	sv := int64(1)
	if n.Metadata.StatusVersion != "" {
		sv, err = strconv.ParseInt(n.Metadata.StatusVersion, 10, 64)
		if err != nil || sv <= 0 {
			return 0, fmt.Errorf("node %q requires a positive statusVersion for capacity policy", n.Metadata.Name)
		}
	}
	return rv + sv - 1, nil
}

func syncNodeAccount(ctx context.Context, reg *repo.Registry, n *corev1.Node) error {
	pv, err := nodePolicyVersion(n)
	if err != nil {
		return err
	}
	limits, err := nodeLimits(n)
	if err != nil {
		return err
	}
	_, err = nodedb.NewDB(reg.Pool()).SyncAccount(ctx, n.Metadata.UID, pv, limits)
	return err
}

func accountOutOfSync(ctx context.Context, reg *repo.Registry, n *corev1.Node) bool {
	nodeResDB := nodedb.NewDB(reg.Pool())
	if n == nil || nodeResDB == nil {
		return false
	}
	account, err := nodeResDB.GetAccount(ctx, n.Metadata.UID)
	if err != nil {
		return true
	}
	pv, err := nodePolicyVersion(n)
	return err != nil || account.PolicyVersion < pv
}
