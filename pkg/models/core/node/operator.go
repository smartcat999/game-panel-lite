package node

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	promclient "github.com/smartcat999/game-panel-cloud/pkg/infra/prometheus"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	instancedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/instance"
	nodedb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/node"
)

const (
	MaxNodeClockSkew       = 30 * time.Second
	defaultAgentListenPort = 8082
)

type Operator interface {
	List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Node], error)
	Get(ctx context.Context, name string) (*corev1.Node, error)
	Create(ctx context.Context, node *corev1.Node) (*corev1.Node, error)
	Update(ctx context.Context, name string, node *corev1.Node) (*corev1.Node, error)
	UpdateStatus(ctx context.Context, name string, node *corev1.Node) (*corev1.Node, error)
	GetMetrics(ctx context.Context, name string, window time.Duration) (*corev1.NodeMetrics, error)
	OpenExecTTY(ctx context.Context, name string, req drvruntime.InteractiveExecRequest) (drvruntime.InteractiveExecSession, error)
	Delete(ctx context.Context, name string) error
}

type operator struct {
	db            *nodedb.DB
	instanceDB    *instancedb.DB
	drivers       *drvruntime.Registry
	mq            mq.Interface
	localNodeName string
	agentPort     int
	agentToken    string
}

type OperatorOption func(*operator)

func WithDrivers(drivers *drvruntime.Registry) OperatorOption {
	return func(o *operator) { o.drivers = drivers }
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

func NewOperator(reg *repo.Registry, bus mq.Interface, opts ...OperatorOption) Operator {
	op := &operator{mq: bus}
	if cfg := config.Get(); cfg != nil {
		op.agentPort = cfg.Agent.Port()
		op.agentToken = strings.TrimSpace(cfg.Agent.Token)
	}
	if reg != nil {
		op.db = nodedb.NewDB(reg.Pool())
		op.instanceDB = instancedb.NewDB(reg.Pool())
	}
	for _, opt := range opts {
		if opt != nil {
			opt(op)
		}
	}
	if op.agentPort <= 0 {
		op.agentPort = defaultAgentListenPort
	}
	return op
}

func (o *operator) List(ctx context.Context, opts db.ListOptions) (*metav1.List[*corev1.Node], error) {
	list, err := o.db.List(ctx, opts)
	if err != nil {
		return nil, err
	}
	for _, node := range list.Items {
		if node != nil {
			if err := o.projectReserved(ctx, node); err != nil {
				return nil, err
			}
		}
	}
	return list, nil
}

func (o *operator) Get(ctx context.Context, name string) (*corev1.Node, error) {
	node, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := o.projectReserved(ctx, node); err != nil {
		return nil, err
	}
	return node, nil
}

func (o *operator) projectReserved(ctx context.Context, node *corev1.Node) error {
	account, err := o.db.GetAccount(ctx, node.Metadata.UID)
	if err != nil && !errors.Is(err, nodedb.ErrNotFound) {
		return err
	}
	node.Status.Reserved = corev1.CanonicalResources(account.Allocated)
	return nil
}

func ensureNodeLabels(node *corev1.Node) {
	if node == nil || strings.TrimSpace(node.Metadata.Name) == "" {
		return
	}
	if node.Metadata.Labels == nil {
		node.Metadata.Labels = make(map[string]string, 1)
	}
	if strings.TrimSpace(node.Metadata.Labels[corev1.LabelKubernetesHostname]) == "" {
		node.Metadata.Labels[corev1.LabelKubernetesHostname] = node.Metadata.Name
	}
}

func (o *operator) resolveAgentToken(ctx context.Context, nodeName string) string {
	if name := strings.TrimSpace(nodeName); name != "" {
		if kv := cache.Default(); kv != nil {
			if tok, err := kv.Get(ctx, iamv1.CacheKeyPrefixNodeToken+name); err == nil && strings.TrimSpace(tok) != "" {
				return strings.TrimSpace(tok)
			}
		}
	}
	return o.agentToken
}

func (o *operator) Create(ctx context.Context, node *corev1.Node) (*corev1.Node, error) {
	ensureNodeLabels(node)
	node.Status.Reserved = corev1.CanonicalResources(corev1.Resources{})
	created, err := o.db.Create(ctx, node)
	if err != nil {
		return nil, err
	}
	if o.mq != nil {
		_ = o.mq.Publish(ctx, mq.TopicNode, mq.NewEvent(mq.EventCreated, corev1.GroupVersion, "Node", "", created.Metadata.Name, created.Metadata.Generation, created.Metadata.ResourceVersion))
	}
	return created, nil
}

func (o *operator) Update(ctx context.Context, name string, node *corev1.Node) (*corev1.Node, error) {
	node.Metadata.Name = name
	ensureNodeLabels(node)
	updated, err := o.db.Update(ctx, node)
	if err != nil {
		return nil, err
	}
	if err := o.projectReserved(ctx, updated); err != nil {
		return nil, err
	}
	if o.mq != nil {
		_ = o.mq.Publish(ctx, mq.TopicNode, mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Node", "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion))
	}
	return updated, nil
}

func (o *operator) UpdateStatus(ctx context.Context, name string, node *corev1.Node) (*corev1.Node, error) {
	node.Metadata.Name = name
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(existing.Metadata.Labels[corev1.LabelKubernetesHostname]) == "" {
		ensureNodeLabels(existing)
		if updatedMeta, uErr := o.db.Update(ctx, existing); uErr == nil && updatedMeta != nil {
			existing = updatedMeta
		}
	}
	prevPhase := existing.Status.Phase
	if err := o.projectReserved(ctx, existing); err != nil {
		return nil, err
	}
	node.Status.Reserved = existing.Status.Reserved
	EvaluateNodeClockSync(node, existing.Metadata.Generation)
	updated, err := o.db.UpdateStatus(ctx, node)
	if err != nil {
		return nil, err
	}
	if updated.Status.Usage != nil {
		metrics.ExportNodeUsage(ctx, updated.Metadata.Name, updated.Status.Usage)
	}
	if o.mq != nil {
		ev := mq.NewEvent(mq.EventUpdated, corev1.GroupVersion, "Node", "", updated.Metadata.Name, updated.Metadata.Generation, updated.Metadata.ResourceVersion)
		ev.Attributes = map[string]string{
			"prevPhase":    string(prevPhase),
			"phaseChanged": strconv.FormatBool(prevPhase != updated.Status.Phase),
		}
		_ = o.mq.Publish(ctx, mq.TopicNode, ev)
	}
	return updated, nil
}

func (o *operator) GetMetrics(ctx context.Context, name string, window time.Duration) (*corev1.NodeMetrics, error) {
	node, err := o.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	samples, _ := promclient.DefaultClient().QueryNodeSamples(ctx, name, window)
	if len(samples) == 0 && node.Status.Usage != nil {
		ts := time.Now().UTC()
		if node.Status.LastHeartbeatTime != nil && !node.Status.LastHeartbeatTime.IsZero() {
			ts = node.Status.LastHeartbeatTime.UTC()
		}
		samples = []corev1.InstanceMetricSample{
			{
				Timestamp:            ts,
				CPUMillicores:        node.Status.Usage.CPUMillicores,
				MemoryBytes:          node.Status.Usage.MemoryBytes,
				StorageBytes:         node.Status.Usage.StorageBytes,
				NetworkRxBytesPerSec: node.Status.Usage.NetworkRxBytesPerSec,
				NetworkTxBytesPerSec: node.Status.Usage.NetworkTxBytesPerSec,
			},
		}
	}
	if samples == nil {
		samples = []corev1.InstanceMetricSample{}
	}
	return &corev1.NodeMetrics{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "NodeMetrics"},
		Metadata: metav1.ObjectMeta{
			Name:            node.Metadata.Name,
			ResourceVersion: node.Metadata.ResourceVersion,
		},
		Samples:     samples,
		Capacity:    node.Status.Capacity,
		Allocatable: node.Status.Allocatable,
		Usage:       node.Status.Usage,
	}, nil
}

func (o *operator) Delete(ctx context.Context, name string) error {
	if o.instanceDB != nil {
		insts, err := o.instanceDB.List(ctx, db.ListOptions{FieldSelector: map[string]string{"spec.nodeName": name}})
		if err != nil {
			return err
		}
		inUse := 0
		for _, inst := range insts.Items {
			if inst != nil && inst.Spec.NodeName == name {
				inUse++
			}
		}
		if inUse > 0 {
			return fmt.Errorf("%w: cannot delete node %q because it still hosts %d instance(s); wait for instance cleanup to finish", models.ErrConflict, name, inUse)
		}
	}
	if err := o.db.Delete(ctx, name); err != nil {
		return err
	}
	metrics.DeleteNodeMetrics(ctx, name)
	if o.mq != nil {
		_ = o.mq.Publish(ctx, mq.TopicNode, mq.NewEvent(mq.EventDeleted, corev1.GroupVersion, "Node", "", name, 0, ""))
	}
	return nil
}

func EvaluateNodeClockSync(node *corev1.Node, generation int64) {
	if node == nil || node.Status.LastHeartbeatTime == nil || node.Status.LastHeartbeatTime.IsZero() {
		return
	}
	skew := time.Now().UTC().Sub(node.Status.LastHeartbeatTime.UTC())
	if skew < 0 {
		skew = -skew
	}
	if skew <= MaxNodeClockSkew {
		metav1.SetCondition(&node.Status.Conditions, metav1.Condition{
			Type:               metav1.ConditionTypeClockSynced,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: generation,
			Reason:             corev1.ReasonClockSynced,
			Message:            "Node clock is synchronized with control plane",
		})
		return
	}
	metav1.SetCondition(&node.Status.Conditions, metav1.Condition{
		Type:               metav1.ConditionTypeClockSynced,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: generation,
		Reason:             corev1.ReasonClockSkewDetected,
		Message:            fmt.Sprintf("Node clock skew (%s) exceeds 30s threshold; enable NTP/chrony on host", skew.Round(time.Second)),
	})
}
