package metrics

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
)

// SampleRecorder is an optional hook registered by pkg/infra/prometheus so
// infrastructure metric exports can also update read-through latest-sample cache
// without pkg/infra/metrics depending on pkg/client.
type SampleRecorder interface {
	RecordLatestInstanceSample(ctx context.Context, tenant, instance string, usage *corev1.InstanceUsage)
	InvalidateInstance(ctx context.Context, tenant, instance string)
	RecordLatestNodeSample(ctx context.Context, nodeName string, usage *corev1.NodeUsage)
	InvalidateNode(ctx context.Context, nodeName string)
}

var (
	recorderMu     sync.RWMutex
	sampleRecorder SampleRecorder
)

// RegisterSampleRecorder binds an optional SampleRecorder for latest-sample cache synchronization.
func RegisterSampleRecorder(r SampleRecorder) {
	recorderMu.Lock()
	sampleRecorder = r
	recorderMu.Unlock()
}

func currentRecorder() SampleRecorder {
	recorderMu.RLock()
	defer recorderMu.RUnlock()
	return sampleRecorder
}

var instanceSeries = []*prometheus.GaugeVec{
	InstanceRunning, InstanceCPULimitMillicores, InstanceMemoryLimitBytes,
	InstanceStorageLimitBytes, InstanceCPUPercent, InstanceMemoryBytes,
	InstanceDiskBytes, InstanceNetworkRxBytesPerSec, InstanceNetworkTxBytesPerSec,
	InstanceCustomMetricGauge, InstanceCustomMetricItemInfo,
}

// DeleteInstanceSeries removes every runtime class, custom ID/label/unit and
// item variant. An empty node removes all node-label variants for the instance.
func DeleteInstanceSeries(tenant, instance, node string) {
	if tenant == "" || instance == "" {
		return
	}
	labels := prometheus.Labels{LabelTenant: tenant, LabelInstance: instance}
	if node != "" {
		labels[LabelNode] = node
	}
	deleteInstanceSeries(labels)
}

func deleteInstanceSeries(labels prometheus.Labels) {
	for _, series := range instanceSeries {
		series.DeletePartialMatch(labels)
	}
}

// ExportInstancePrometheus controls Prometheus cardinality only. Runtime usage,
// API status and durable execution observations are collected independently.
func ExportInstancePrometheus(node string, inst *corev1.Instance, enabled bool) {
	if inst == nil || node == "" || inst.Metadata.Tenant == "" || inst.Metadata.Name == "" {
		return
	}
	tenant, name := inst.Metadata.Tenant, inst.Metadata.Name
	if !enabled {
		DeleteInstanceSeries(tenant, name, node)
		return
	}
	// RuntimeClass and custom display labels can change between observations.
	InstanceRunning.DeletePartialMatch(prometheus.Labels{LabelTenant: tenant, LabelInstance: name, LabelNode: node})
	running := 0.0
	if inst.Status.Phase == corev1.InstancePhaseRunning {
		running = 1
	}
	InstanceRunning.WithLabelValues(tenant, name, node, inst.Spec.RuntimeClassName).Set(running)
	limits := inst.Spec.Resources.Limits
	InstanceCPULimitMillicores.WithLabelValues(tenant, name, node).Set(float64(limits.CPUMillicores()))
	InstanceMemoryLimitBytes.WithLabelValues(tenant, name, node).Set(float64(limits.MemoryBytes()))
	InstanceStorageLimitBytes.WithLabelValues(tenant, name, node).Set(float64(limits.StorageBytes()))
	if inst.Status.Usage != nil {
		exportInstanceUsagePrometheus(tenant, name, node, inst.Status.Usage)
	}
}

func exportInstanceUsagePrometheus(tenant, instance, node string, usage *corev1.InstanceUsage) {
	InstanceCPUPercent.WithLabelValues(tenant, instance, node).Set(float64(usage.CPUMillicores) / 10)
	InstanceMemoryBytes.WithLabelValues(tenant, instance, node).Set(float64(usage.MemoryBytes))
	InstanceDiskBytes.WithLabelValues(tenant, instance, node).Set(float64(usage.StorageBytes))
	InstanceNetworkRxBytesPerSec.WithLabelValues(tenant, instance, node).Set(float64(usage.NetworkRxBytesPerSec))
	InstanceNetworkTxBytesPerSec.WithLabelValues(tenant, instance, node).Set(float64(usage.NetworkTxBytesPerSec))
	labels := prometheus.Labels{LabelTenant: tenant, LabelInstance: instance, LabelNode: node}
	InstanceCustomMetricGauge.DeletePartialMatch(labels)
	InstanceCustomMetricItemInfo.DeletePartialMatch(labels)
	for _, custom := range usage.CustomMetrics {
		InstanceCustomMetricGauge.WithLabelValues(tenant, instance, node, custom.ID, custom.Label, custom.Unit).Set(custom.Value)
		for _, item := range custom.Items {
			InstanceCustomMetricItemInfo.WithLabelValues(tenant, instance, node, custom.ID, item).Set(1)
		}
	}
}

// ExportInstanceUsage always records the latest API/cache point. Per-instance
// Prometheus series are opt-in through the unified Agent configuration.
func ExportInstanceUsage(ctx context.Context, tenant, instance, nodeLabel string, usage *corev1.InstanceUsage) {
	if tenant == "" || instance == "" || usage == nil {
		return
	}
	if nodeLabel == "" {
		nodeLabel = DefaultControlPlaneNodeLabel
	}
	if config.Get().Agent.PerInstancePrometheusMetrics {
		exportInstanceUsagePrometheus(tenant, instance, nodeLabel, usage)
	} else {
		DeleteInstanceSeries(tenant, instance, nodeLabel)
	}
	if rec := currentRecorder(); rec != nil {
		rec.RecordLatestInstanceSample(ctx, tenant, instance, usage)
	}
}

// DeleteInstanceMetrics removes all Prometheus Gauge series and cached query entries for a deleted instance.
func DeleteInstanceMetrics(ctx context.Context, tenant, instance, nodeLabel string) {
	if tenant == "" || instance == "" {
		return
	}
	DeleteInstanceSeries(tenant, instance, "")
	if rec := currentRecorder(); rec != nil {
		rec.InvalidateInstance(ctx, tenant, instance)
	}
}

// ExportNodeUsage updates the standard Prometheus Gauges for a compute Node and records
// the latest point in the client cache so Prometheus TSDB scrapes it on its next cycle.
func ExportNodeUsage(ctx context.Context, nodeName string, usage *corev1.NodeUsage) {
	if nodeName == "" || usage == nil {
		return
	}
	NodeCPUPercent.WithLabelValues(nodeName).Set(float64(usage.CPUMillicores) / 10.0)
	NodeMemoryBytes.WithLabelValues(nodeName).Set(float64(usage.MemoryBytes))
	NodeDiskBytes.WithLabelValues(nodeName).Set(float64(usage.StorageBytes))
	NodeNetworkRxBytesPerSec.WithLabelValues(nodeName).Set(float64(usage.NetworkRxBytesPerSec))
	NodeNetworkTxBytesPerSec.WithLabelValues(nodeName).Set(float64(usage.NetworkTxBytesPerSec))
	if rec := currentRecorder(); rec != nil {
		rec.RecordLatestNodeSample(ctx, nodeName, usage)
	}
}

// DeleteNodeMetrics removes all Prometheus Gauge series and cached query entries for a deleted Node.
func DeleteNodeMetrics(ctx context.Context, nodeName string) {
	if nodeName == "" {
		return
	}
	NodeCPUPercent.DeleteLabelValues(nodeName)
	NodeMemoryBytes.DeleteLabelValues(nodeName)
	NodeDiskBytes.DeleteLabelValues(nodeName)
	NodeNetworkRxBytesPerSec.DeleteLabelValues(nodeName)
	NodeNetworkTxBytesPerSec.DeleteLabelValues(nodeName)
	deleteInstanceSeries(map[string]string{LabelNode: nodeName})
	if rec := currentRecorder(); rec != nil {
		rec.InvalidateNode(ctx, nodeName)
	}
}
