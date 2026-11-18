package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	Namespace = "gamepanel"

	SubsystemAPIServer  = "apiserver"
	SubsystemMQ         = "mq"
	SubsystemController = "controller"
	SubsystemNode       = "node"
	SubsystemInstance   = "instance"

	MetricNameNodeCPUPercent           = Namespace + "_" + SubsystemNode + "_cpu_usage_percent"
	MetricNameNodeMemoryBytes          = Namespace + "_" + SubsystemNode + "_memory_usage_bytes"
	MetricNameNodeDiskBytes            = Namespace + "_" + SubsystemNode + "_disk_usage_bytes"
	MetricNameNodeNetworkRxBytesPerSec = Namespace + "_" + SubsystemNode + "_network_rx_bytes_per_sec"
	MetricNameNodeNetworkTxBytesPerSec = Namespace + "_" + SubsystemNode + "_network_tx_bytes_per_sec"

	MetricNameInstanceCPUPercent           = Namespace + "_" + SubsystemInstance + "_cpu_usage_percent"
	MetricNameInstanceMemoryBytes          = Namespace + "_" + SubsystemInstance + "_memory_usage_bytes"
	MetricNameInstanceDiskBytes            = Namespace + "_" + SubsystemInstance + "_disk_usage_bytes"
	MetricNameInstanceNetworkRxBytesPerSec = Namespace + "_" + SubsystemInstance + "_network_rx_bytes_per_sec"
	MetricNameInstanceNetworkTxBytesPerSec = Namespace + "_" + SubsystemInstance + "_network_tx_bytes_per_sec"
	MetricNameInstanceCustomMetric         = Namespace + "_" + SubsystemInstance + "_custom_metric"
	MetricNameInstanceCustomMetricItem     = Namespace + "_" + SubsystemInstance + "_custom_metric_item_info"

	LabelMethod           = "method"
	LabelResource         = "resource"
	LabelStatus           = "status"
	LabelTopic            = "topic"
	LabelType             = "type"
	LabelController       = "controller"
	LabelResult           = "result"
	LabelNode             = "node"
	LabelTenant           = "tenant"
	LabelInstance         = "instance"
	LabelExportedInstance = "exported_instance"
	LabelRuntimeClass     = "runtime_class"
	LabelMetricID         = "metric_id"
	LabelMetricLabel      = "label"
	LabelMetricUnit       = "unit"
	LabelMetricItem       = "item"
	LabelWindow           = "window"
	LabelName             = "__name__"

	DefaultControlPlaneNodeLabel = "control-plane"

	PromAPIQueryPath      = "/api/v1/query"
	PromAPIQueryRangePath = "/api/v1/query_range"

	DefaultPromDockerURL = "http://prometheus:9090"
	DefaultPromLocalURL  = "http://127.0.0.1:9090"

	CachePrefixQueryInstant   = "metrics:qinst"
	CachePrefixQueryRange     = "metrics:qrange"
	CachePrefixLatestInstance = "metrics:latest"
	CachePrefixInstanceWindow = "metrics:inst"
	CachePrefixLatestNode     = "metrics:latest_node"
	CachePrefixNodeWindow     = "metrics:node"

	DefaultQueryTimeout      = 2500 * time.Millisecond
	DefaultSingleFlightStep  = 2 * time.Second
	DefaultCacheTTL          = 5 * time.Second
	DefaultLatestSampleTTL   = 5 * time.Minute
	DefaultWindowDuration    = 15 * time.Minute
	WindowDuration1Hour      = time.Hour
	WindowDuration6Hours     = 6 * time.Hour
	MediumWindowThreshold    = 20 * time.Minute
	LongWindowThreshold      = 2 * time.Hour
	StepShortWindow          = 10 * time.Second
	StepMediumWindow         = 30 * time.Second
	StepLongWindow           = 120 * time.Second
	MaxPromResponseBodyBytes = 2 << 20
)

var (
	// HTTPRequestsTotal counts incoming HTTP requests partitioned by method, resource/path, and status code.
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gamepanel",
			Subsystem: "apiserver",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests processed by the API server.",
		},
		[]string{"method", "resource", "status"},
	)

	// HTTPRequestDuration tracks HTTP request latency in seconds.
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gamepanel",
			Subsystem: "apiserver",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency distributions in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "resource"},
	)

	// MQEventsPublishedTotal counts message queue events published partitioned by topic and event type.
	MQEventsPublishedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gamepanel",
			Subsystem: "mq",
			Name:      "events_published_total",
			Help:      "Total number of declarative resource events published to the message queue.",
		},
		[]string{"topic", "type"},
	)

	// MQPublishErrorsTotal counts message queue publish errors partitioned by topic and event type.
	MQPublishErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gamepanel",
			Subsystem: "mq",
			Name:      "publish_errors_total",
			Help:      "Total number of message queue publish failures (recovered asynchronously via controller Resync).",
		},
		[]string{"topic", "type"},
	)

	// ControllerReconcileTotal counts controller reconcile executions partitioned by controller name and result.
	ControllerReconcileTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "gamepanel",
			Subsystem: "controller",
			Name:      "reconcile_total",
			Help:      "Total number of controller reconcile executions.",
		},
		[]string{"controller", "result"},
	)

	// ControllerReconcileDuration tracks controller reconcile duration in seconds.
	ControllerReconcileDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gamepanel",
			Subsystem: "controller",
			Name:      "reconcile_duration_seconds",
			Help:      "Duration of controller reconcile executions in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"controller"},
	)

	// NodeCPUPercent reports live host CPU usage percentage (100% per physical/logical core) for a Node.
	NodeCPUPercent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemNode,
			Name:      "cpu_usage_percent",
			Help:      "Current host CPU usage percentage (100% per core) on the compute node.",
		},
		[]string{LabelNode},
	)

	// NodeMemoryBytes reports live host physical memory usage in bytes for a Node.
	NodeMemoryBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemNode,
			Name:      "memory_usage_bytes",
			Help:      "Current host physical memory usage in bytes on the compute node.",
		},
		[]string{LabelNode},
	)

	// NodeDiskBytes reports live host working directory filesystem disk usage in bytes for a Node.
	NodeDiskBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemNode,
			Name:      "disk_usage_bytes",
			Help:      "Current host working directory disk usage in bytes on the compute node.",
		},
		[]string{LabelNode},
	)

	// NodeNetworkRxBytesPerSec reports live host inbound network throughput in bytes/sec for a Node.
	NodeNetworkRxBytesPerSec = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemNode,
			Name:      "network_rx_bytes_per_sec",
			Help:      "Current host inbound network throughput in bytes per second on the compute node.",
		},
		[]string{LabelNode},
	)

	// NodeNetworkTxBytesPerSec reports live host outbound network throughput in bytes/sec for a Node.
	NodeNetworkTxBytesPerSec = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemNode,
			Name:      "network_tx_bytes_per_sec",
			Help:      "Current host outbound network throughput in bytes per second on the compute node.",
		},
		[]string{LabelNode},
	)

	// InstanceRunning tracks whether a scheduled Instance is currently Running (1) or not (0) on a Node.
	InstanceRunning = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemInstance,
			Name:      "running",
			Help:      "Whether the game server instance is currently running (1 for Running, 0 otherwise).",
		},
		[]string{LabelTenant, LabelInstance, LabelNode, LabelRuntimeClass},
	)

	// InstanceCPULimitMillicores reports the configured CPU limit in millicores for each Instance on a Node.
	InstanceCPULimitMillicores = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemInstance,
			Name:      "cpu_limit_millicores",
			Help:      "Configured CPU limit in millicores for the game server instance.",
		},
		[]string{LabelTenant, LabelInstance, LabelNode},
	)

	// InstanceMemoryLimitBytes reports the configured memory limit in bytes for each Instance on a Node.
	InstanceMemoryLimitBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemInstance,
			Name:      "memory_limit_bytes",
			Help:      "Configured memory limit in bytes for the game server instance.",
		},
		[]string{LabelTenant, LabelInstance, LabelNode},
	)

	// InstanceStorageLimitBytes reports the configured storage limit in bytes for each Instance on a Node.
	InstanceStorageLimitBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemInstance,
			Name:      "storage_limit_bytes",
			Help:      "Configured storage limit in bytes for the game server instance.",
		},
		[]string{LabelTenant, LabelInstance, LabelNode},
	)

	// InstanceCPUPercent reports live CPU usage percentage for each Instance on a Node.
	InstanceCPUPercent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemInstance,
			Name:      "cpu_usage_percent",
			Help:      "Current CPU usage percentage for the game server instance.",
		},
		[]string{LabelTenant, LabelInstance, LabelNode},
	)

	// InstanceMemoryBytes reports live resident memory usage in bytes for each Instance on a Node.
	InstanceMemoryBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemInstance,
			Name:      "memory_usage_bytes",
			Help:      "Current resident memory usage in bytes for the game server instance.",
		},
		[]string{LabelTenant, LabelInstance, LabelNode},
	)

	// InstanceDiskBytes reports live working directory disk usage in bytes for each Instance on a Node.
	InstanceDiskBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemInstance,
			Name:      "disk_usage_bytes",
			Help:      "Current working directory disk usage in bytes for the game server instance.",
		},
		[]string{LabelTenant, LabelInstance, LabelNode},
	)

	// InstanceNetworkRxBytesPerSec reports live inbound network throughput in bytes/sec for each Instance on a Node.
	InstanceNetworkRxBytesPerSec = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemInstance,
			Name:      "network_rx_bytes_per_sec",
			Help:      "Current inbound network throughput in bytes per second for the game server instance.",
		},
		[]string{LabelTenant, LabelInstance, LabelNode},
	)

	// InstanceNetworkTxBytesPerSec reports live outbound network throughput in bytes/sec for each Instance on a Node.
	InstanceNetworkTxBytesPerSec = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemInstance,
			Name:      "network_tx_bytes_per_sec",
			Help:      "Current outbound network throughput in bytes per second for the game server instance.",
		},
		[]string{LabelTenant, LabelInstance, LabelNode},
	)

	// InstanceCustomMetricGauge reports declarative game-level scalar metrics (e.g. online_players, server_tps, world_cycles).
	InstanceCustomMetricGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemInstance,
			Name:      "custom_metric",
			Help:      "Declarative game-level custom metric value collected from the instance.",
		},
		[]string{LabelTenant, LabelInstance, LabelNode, LabelMetricID, LabelMetricLabel, LabelMetricUnit},
	)

	// InstanceCustomMetricItemInfo exposes active items of a list-bearing custom metric (e.g. online player names/IDs) as 1.
	InstanceCustomMetricItemInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: SubsystemInstance,
			Name:      "custom_metric_item_info",
			Help:      "Active entity item belonging to a declarative list-bearing custom metric (1 for present).",
		},
		[]string{LabelTenant, LabelInstance, LabelNode, LabelMetricID, LabelMetricItem},
	)
)

// Handler returns the standard Prometheus metrics HTTP handler for GET /metrics.
func Handler() http.Handler {
	return promhttp.Handler()
}
