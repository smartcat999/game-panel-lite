package prometheus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
)

func init() {
	metrics.RegisterSampleRecorder(DefaultClient())
}

// VectorSample represents a single instant vector sample returned by Prometheus /api/v1/query.
type VectorSample struct {
	Metric    map[string]string `json:"metric"`
	Timestamp time.Time         `json:"timestamp"`
	Value     float64           `json:"value"`
}

// MatrixPoint represents a single (timestamp, value) pair in a range vector.
type MatrixPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

// MatrixSeries represents a single time-series returned by Prometheus /api/v1/query_range.
type MatrixSeries struct {
	Metric map[string]string `json:"metric"`
	Values []MatrixPoint     `json:"values"`
}

// Client encapsulates Prometheus TSDB queries with unified read-through caching via pkg/infra/cache.
type Client interface {
	QueryInstant(ctx context.Context, promQL string) ([]VectorSample, error)
	QueryRange(ctx context.Context, promQL string, start, end time.Time, step time.Duration) ([]MatrixSeries, error)
	QueryInstanceSamples(ctx context.Context, tenant, instance string, window time.Duration) ([]corev1.InstanceMetricSample, error)
	RecordLatestInstanceSample(ctx context.Context, tenant, instance string, usage *corev1.InstanceUsage)
	InvalidateInstance(ctx context.Context, tenant, instance string)
	QueryNodeSamples(ctx context.Context, nodeName string, window time.Duration) ([]corev1.InstanceMetricSample, error)
	RecordLatestNodeSample(ctx context.Context, nodeName string, usage *corev1.NodeUsage)
	InvalidateNode(ctx context.Context, nodeName string)
}

type ClientOption func(*promClient)

// WithBaseURL overrides the Prometheus server base URL.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *promClient) {
		c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
}

// WithHTTPClient overrides the underlying HTTP client used for TSDB requests.
func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *promClient) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithCache configures the unified cache backend and TTL used by the metrics query client.
func WithCache(store cache.Interface, ttl time.Duration) ClientOption {
	return func(c *promClient) {
		if store != nil {
			c.cache = store
		}
		if ttl > 0 {
			c.cacheTTL = ttl
		}
	}
}

type promClient struct {
	mu         sync.RWMutex
	baseURL    string
	httpClient *http.Client
	cache      cache.Interface
	cacheTTL   time.Duration
}

func (c *promClient) getCache() (cache.Interface, time.Duration) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cache, c.cacheTTL
}

var (
	defaultClientOnce sync.Once
	defaultClientInst Client
)

// DefaultClient returns the process-wide shared TSDB query client backed by pkg/infra/cache.
func DefaultClient() Client {
	defaultClientOnce.Do(func() {
		defaultClientInst = NewClient(WithCache(cache.Default(), metrics.DefaultCacheTTL))
	})
	return defaultClientInst
}

// SetDefaultCache binds the process-wide metrics query client to the platform's unified cache.Interface (e.g. Redis).
func SetDefaultCache(store cache.Interface, ttl time.Duration) {
	if store == nil {
		return
	}
	if pc, ok := DefaultClient().(*promClient); ok {
		pc.mu.Lock()
		pc.cache = store
		if ttl > 0 {
			pc.cacheTTL = ttl
		}
		pc.mu.Unlock()
	}
}

// NewClient constructs a new Prometheus TSDB query client with optional cache support.
func NewClient(opts ...ClientOption) Client {
	c := &promClient{
		httpClient: &http.Client{Timeout: metrics.DefaultQueryTimeout},
		cache:      cache.Default(),
		cacheTTL:   metrics.DefaultCacheTTL,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *promClient) candidateURLs() []string {
	if c.baseURL != "" {
		return []string{c.baseURL}
	}
	cfgURL := strings.TrimRight(strings.TrimSpace(config.Get().Observability.PrometheusURL), "/")
	if cfgURL != "" {
		return []string{cfgURL}
	}
	return []string{metrics.DefaultPromDockerURL, metrics.DefaultPromLocalURL}
}

func cacheKeyHash(prefix, raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return prefix + ":" + hex.EncodeToString(sum[:12])
}

func (c *promClient) QueryInstant(ctx context.Context, promQL string) ([]VectorSample, error) {
	cacheKey := cacheKeyHash(metrics.CachePrefixQueryInstant, promQL)
	cacheStore, cacheTTL := c.getCache()
	if cacheStore != nil && cacheTTL > 0 {
		if raw, err := cacheStore.Get(ctx, cacheKey); err == nil && raw != "" {
			var cached []VectorSample
			if json.Unmarshal([]byte(raw), &cached) == nil {
				return cached, nil
			}
		}
	}
	out, err := c.queryInstantViaSDK(ctx, promQL)
	if err != nil {
		return nil, err
	}
	if cacheStore != nil && cacheTTL > 0 {
		if encoded, err := json.Marshal(out); err == nil {
			_ = cacheStore.Set(ctx, cacheKey, string(encoded), cacheTTL)
		}
	}
	return out, nil
}

func (c *promClient) queryInstantViaSDK(ctx context.Context, promQL string) ([]VectorSample, error) {
	var lastErr error
	for _, base := range c.candidateURLs() {
		cli, err := api.NewClient(api.Config{Address: base, Client: c.httpClient})
		if err != nil {
			lastErr = err
			continue
		}
		reqCtx, cancel := context.WithTimeout(ctx, metrics.DefaultSingleFlightStep)
		val, _, err := promv1.NewAPI(cli).Query(reqCtx, promQL, time.Now())
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		vec, ok := val.(model.Vector)
		if !ok {
			lastErr = errors.New("unexpected prometheus instant query result type")
			continue
		}
		return convertPromVector(vec), nil
	}
	return nil, lastErr
}

func convertPromVector(vec model.Vector) []VectorSample {
	out := make([]VectorSample, 0, len(vec))
	for _, sample := range vec {
		v := float64(sample.Value)
		if math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		m := make(map[string]string, len(sample.Metric))
		for k, lv := range sample.Metric {
			m[string(k)] = string(lv)
		}
		out = append(out, VectorSample{Metric: m, Timestamp: sample.Timestamp.Time().UTC(), Value: v})
	}
	return out
}

func (c *promClient) QueryRange(ctx context.Context, promQL string, start, end time.Time, step time.Duration) ([]MatrixSeries, error) {
	if step <= 0 {
		step = metrics.StepShortWindow
	}
	stepSec := max(int64(step.Seconds()), int64(metrics.StepShortWindow.Seconds()))
	qStart := (start.Unix() / stepSec) * stepSec
	qEnd := (end.Unix() / 5) * 5
	cacheKey := cacheKeyHash(metrics.CachePrefixQueryRange, fmt.Sprintf("%s|%d|%d|%d", promQL, qStart, qEnd, stepSec))
	cacheStore, cacheTTL := c.getCache()
	if cacheStore != nil && cacheTTL > 0 {
		if raw, err := cacheStore.Get(ctx, cacheKey); err == nil && raw != "" {
			var cached []MatrixSeries
			if json.Unmarshal([]byte(raw), &cached) == nil {
				return cached, nil
			}
		}
	}
	out, err := c.queryRangeViaSDK(ctx, promQL, start, end, time.Duration(stepSec)*time.Second)
	if err != nil {
		return nil, err
	}
	if cacheStore != nil && cacheTTL > 0 {
		if encoded, err := json.Marshal(out); err == nil {
			_ = cacheStore.Set(ctx, cacheKey, string(encoded), cacheTTL)
		}
	}
	return out, nil
}

func (c *promClient) queryRangeViaSDK(ctx context.Context, promQL string, start, end time.Time, step time.Duration) ([]MatrixSeries, error) {
	var lastErr error
	r := promv1.Range{Start: start, End: end, Step: step}
	for _, base := range c.candidateURLs() {
		cli, err := api.NewClient(api.Config{Address: base, Client: c.httpClient})
		if err != nil {
			lastErr = err
			continue
		}
		reqCtx, cancel := context.WithTimeout(ctx, metrics.DefaultSingleFlightStep)
		val, _, err := promv1.NewAPI(cli).QueryRange(reqCtx, promQL, r)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		mat, ok := val.(model.Matrix)
		if !ok {
			lastErr = errors.New("unexpected prometheus range query result type")
			continue
		}
		return convertPromMatrix(mat), nil
	}
	return nil, lastErr
}

func convertPromMatrix(mat model.Matrix) []MatrixSeries {
	out := make([]MatrixSeries, 0, len(mat))
	for _, stream := range mat {
		pts := make([]MatrixPoint, 0, len(stream.Values))
		for _, pair := range stream.Values {
			v := float64(pair.Value)
			if math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			pts = append(pts, MatrixPoint{Timestamp: pair.Timestamp.Time().UTC(), Value: v})
		}
		if len(pts) == 0 {
			continue
		}
		m := make(map[string]string, len(stream.Metric))
		for k, lv := range stream.Metric {
			m[string(k)] = string(lv)
		}
		out = append(out, MatrixSeries{Metric: m, Values: pts})
	}
	return out
}

func latestSampleCacheKey(tenant, instance string) string {
	return fmt.Sprintf("%s:%s/%s", metrics.CachePrefixLatestInstance, tenant, instance)
}

func instanceWindowCacheKey(tenant, instance string, window time.Duration) string {
	return fmt.Sprintf("%s:%s/%s:%s", metrics.CachePrefixInstanceWindow, tenant, instance, window.String())
}

func (c *promClient) RecordLatestInstanceSample(ctx context.Context, tenant, instance string, usage *corev1.InstanceUsage) {
	cacheStore, _ := c.getCache()
	if cacheStore == nil || tenant == "" || instance == "" || usage == nil {
		return
	}
	var customMap map[string]float64
	if len(usage.CustomMetrics) > 0 {
		customMap = make(map[string]float64, len(usage.CustomMetrics))
		for _, cm := range usage.CustomMetrics {
			customMap[cm.ID] = cm.Value
		}
	}
	sample := corev1.InstanceMetricSample{
		Timestamp:            time.Now().UTC(),
		CPUMillicores:        usage.CPUMillicores,
		MemoryBytes:          usage.MemoryBytes,
		StorageBytes:         usage.StorageBytes,
		NetworkRxBytesPerSec: usage.NetworkRxBytesPerSec,
		NetworkTxBytesPerSec: usage.NetworkTxBytesPerSec,
		CustomMetrics:        customMap,
	}
	if encoded, err := json.Marshal(sample); err == nil {
		_ = cacheStore.Set(ctx, latestSampleCacheKey(tenant, instance), string(encoded), metrics.DefaultLatestSampleTTL)
	}
}

func (c *promClient) InvalidateInstance(ctx context.Context, tenant, instance string) {
	cacheStore, _ := c.getCache()
	if cacheStore == nil || tenant == "" || instance == "" {
		return
	}
	_ = cacheStore.Delete(ctx, latestSampleCacheKey(tenant, instance))
	for _, w := range []time.Duration{metrics.DefaultWindowDuration, metrics.WindowDuration1Hour, metrics.WindowDuration6Hours} {
		_ = cacheStore.Delete(ctx, instanceWindowCacheKey(tenant, instance, w))
	}
}

func resolveWindowStep(window time.Duration) (time.Duration, time.Duration) {
	if window <= 0 {
		window = metrics.DefaultWindowDuration
	}
	step := metrics.StepShortWindow
	if window > metrics.LongWindowThreshold {
		step = metrics.StepLongWindow
	} else if window > metrics.MediumWindowThreshold {
		step = metrics.StepMediumWindow
	}
	return window, step
}

// QueryInstanceSamples queries Prometheus TSDB for an instance's historical CPU, memory, disk,
// and declarative game metrics over the requested window, using unified pkg/infra/cache read-through caching.
func (c *promClient) QueryInstanceSamples(ctx context.Context, tenant, instance string, window time.Duration) ([]corev1.InstanceMetricSample, error) {
	window, step := resolveWindowStep(window)
	winKey := instanceWindowCacheKey(tenant, instance, window)
	cacheStore, cacheTTL := c.getCache()
	if cacheStore != nil && cacheTTL > 0 {
		if raw, err := cacheStore.Get(ctx, winKey); err == nil && raw != "" {
			var cached []corev1.InstanceMetricSample
			if json.Unmarshal([]byte(raw), &cached) == nil && len(cached) > 0 {
				return cached, nil
			}
		}
	}
	now := time.Now().UTC()
	promQL := fmt.Sprintf(`{%s=~"%s|%s|%s|%s|%s|%s",%s=%q}`,
		metrics.LabelName, metrics.MetricNameInstanceCPUPercent, metrics.MetricNameInstanceMemoryBytes,
		metrics.MetricNameInstanceDiskBytes, metrics.MetricNameInstanceNetworkRxBytesPerSec,
		metrics.MetricNameInstanceNetworkTxBytesPerSec, metrics.MetricNameInstanceCustomMetric,
		metrics.LabelTenant, tenant,
	)
	seriesList, err := c.QueryRange(ctx, promQL, now.Add(-window), now, step)
	byTS := aggregateInstanceSeriesSamples(seriesList, instance)
	samples := finalizeAndCacheMetricSamples(ctx, cacheStore, cacheTTL, winKey, latestSampleCacheKey(tenant, instance), now, window, byTS)
	return samples, err
}

func aggregateInstanceSeriesSamples(seriesList []MatrixSeries, instance string) map[int64]*corev1.InstanceMetricSample {
	byTS := make(map[int64]*corev1.InstanceMetricSample)
	for _, series := range seriesList {
		instLabel := series.Metric[metrics.LabelExportedInstance]
		if instLabel == "" {
			instLabel = series.Metric[metrics.LabelInstance]
		}
		if instLabel != instance {
			continue
		}
		metricName := series.Metric[metrics.LabelName]
		customID := series.Metric[metrics.LabelMetricID]
		for _, ptVal := range series.Values {
			sec := ptVal.Timestamp.Unix()
			pt, exists := byTS[sec]
			if !exists {
				pt = &corev1.InstanceMetricSample{Timestamp: ptVal.Timestamp}
				byTS[sec] = pt
			}
			applyInstanceMetricPoint(pt, metricName, customID, ptVal.Value)
		}
	}
	return byTS
}

func applyInstanceMetricPoint(pt *corev1.InstanceMetricSample, metricName, customID string, val float64) {
	switch metricName {
	case metrics.MetricNameInstanceCPUPercent:
		if milli := int64(math.Round(val * 10)); milli > pt.CPUMillicores {
			pt.CPUMillicores = milli
		}
	case metrics.MetricNameInstanceMemoryBytes:
		if mb := int64(math.Round(val)); mb > pt.MemoryBytes {
			pt.MemoryBytes = mb
		}
	case metrics.MetricNameInstanceDiskBytes:
		if db := int64(math.Round(val)); db > pt.StorageBytes {
			pt.StorageBytes = db
		}
	case metrics.MetricNameInstanceNetworkRxBytesPerSec:
		if rx := int64(math.Round(val)); rx > pt.NetworkRxBytesPerSec {
			pt.NetworkRxBytesPerSec = rx
		}
	case metrics.MetricNameInstanceNetworkTxBytesPerSec:
		if tx := int64(math.Round(val)); tx > pt.NetworkTxBytesPerSec {
			pt.NetworkTxBytesPerSec = tx
		}
	case metrics.MetricNameInstanceCustomMetric:
		if customID != "" {
			if pt.CustomMetrics == nil {
				pt.CustomMetrics = make(map[string]float64)
			}
			if prev, has := pt.CustomMetrics[customID]; !has || val > prev {
				pt.CustomMetrics[customID] = val
			}
		}
	}
}

func finalizeAndCacheMetricSamples(ctx context.Context, cacheStore cache.Interface, cacheTTL time.Duration, winKey, latestKey string, now time.Time, window time.Duration, byTS map[int64]*corev1.InstanceMetricSample) []corev1.InstanceMetricSample {
	if cacheStore != nil {
		if rawLatest, cErr := cacheStore.Get(ctx, latestKey); cErr == nil && rawLatest != "" {
			var latest corev1.InstanceMetricSample
			if json.Unmarshal([]byte(rawLatest), &latest) == nil && !latest.Timestamp.Before(now.Add(-window)) {
				byTS[latest.Timestamp.Unix()] = &latest
			}
		}
	}
	samples := make([]corev1.InstanceMetricSample, 0, len(byTS))
	for _, pt := range byTS {
		samples = append(samples, *pt)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].Timestamp.Before(samples[j].Timestamp) })
	if len(samples) > 0 && cacheStore != nil && cacheTTL > 0 {
		if encoded, mErr := json.Marshal(samples); mErr == nil {
			_ = cacheStore.Set(ctx, winKey, string(encoded), cacheTTL)
		}
	}
	return samples
}

func latestNodeSampleCacheKey(nodeName string) string {
	return fmt.Sprintf("%s:%s", metrics.CachePrefixLatestNode, nodeName)
}

func nodeWindowCacheKey(nodeName string, window time.Duration) string {
	return fmt.Sprintf("%s:%s:%s", metrics.CachePrefixNodeWindow, nodeName, window.String())
}

func (c *promClient) RecordLatestNodeSample(ctx context.Context, nodeName string, usage *corev1.NodeUsage) {
	cacheStore, _ := c.getCache()
	if cacheStore == nil || nodeName == "" || usage == nil {
		return
	}
	sample := corev1.InstanceMetricSample{
		Timestamp:            time.Now().UTC(),
		CPUMillicores:        usage.CPUMillicores,
		MemoryBytes:          usage.MemoryBytes,
		StorageBytes:         usage.StorageBytes,
		NetworkRxBytesPerSec: usage.NetworkRxBytesPerSec,
		NetworkTxBytesPerSec: usage.NetworkTxBytesPerSec,
	}
	if encoded, err := json.Marshal(sample); err == nil {
		_ = cacheStore.Set(ctx, latestNodeSampleCacheKey(nodeName), string(encoded), metrics.DefaultLatestSampleTTL)
	}
}

func (c *promClient) InvalidateNode(ctx context.Context, nodeName string) {
	cacheStore, _ := c.getCache()
	if cacheStore == nil || nodeName == "" {
		return
	}
	_ = cacheStore.Delete(ctx, latestNodeSampleCacheKey(nodeName))
	for _, w := range []time.Duration{metrics.DefaultWindowDuration, metrics.WindowDuration1Hour, metrics.WindowDuration6Hours} {
		_ = cacheStore.Delete(ctx, nodeWindowCacheKey(nodeName, w))
	}
}

// QueryNodeSamples queries Prometheus TSDB for a compute Node's historical CPU, memory, and disk
// usage over the requested window, using unified pkg/infra/cache read-through caching.
func (c *promClient) QueryNodeSamples(ctx context.Context, nodeName string, window time.Duration) ([]corev1.InstanceMetricSample, error) {
	window, step := resolveWindowStep(window)
	winKey := nodeWindowCacheKey(nodeName, window)
	cacheStore, cacheTTL := c.getCache()
	if cacheStore != nil && cacheTTL > 0 {
		if raw, err := cacheStore.Get(ctx, winKey); err == nil && raw != "" {
			var cached []corev1.InstanceMetricSample
			if json.Unmarshal([]byte(raw), &cached) == nil && len(cached) > 0 {
				return cached, nil
			}
		}
	}
	now := time.Now().UTC()
	promQL := fmt.Sprintf(`{%s=~"%s|%s|%s|%s|%s",%s=%q}`,
		metrics.LabelName, metrics.MetricNameNodeCPUPercent, metrics.MetricNameNodeMemoryBytes,
		metrics.MetricNameNodeDiskBytes, metrics.MetricNameNodeNetworkRxBytesPerSec,
		metrics.MetricNameNodeNetworkTxBytesPerSec, metrics.LabelNode, nodeName,
	)
	seriesList, err := c.QueryRange(ctx, promQL, now.Add(-window), now, step)
	byTS := aggregateNodeSeriesSamples(seriesList)
	samples := finalizeAndCacheMetricSamples(ctx, cacheStore, cacheTTL, winKey, latestNodeSampleCacheKey(nodeName), now, window, byTS)
	return samples, err
}

func aggregateNodeSeriesSamples(seriesList []MatrixSeries) map[int64]*corev1.InstanceMetricSample {
	byTS := make(map[int64]*corev1.InstanceMetricSample)
	for _, series := range seriesList {
		metricName := series.Metric[metrics.LabelName]
		for _, ptVal := range series.Values {
			sec := ptVal.Timestamp.Unix()
			pt, exists := byTS[sec]
			if !exists {
				pt = &corev1.InstanceMetricSample{Timestamp: ptVal.Timestamp}
				byTS[sec] = pt
			}
			applyNodeMetricPoint(pt, metricName, ptVal.Value)
		}
	}
	return byTS
}

func applyNodeMetricPoint(pt *corev1.InstanceMetricSample, metricName string, val float64) {
	switch metricName {
	case metrics.MetricNameNodeCPUPercent:
		if milli := int64(math.Round(val * 10)); milli > pt.CPUMillicores {
			pt.CPUMillicores = milli
		}
	case metrics.MetricNameNodeMemoryBytes:
		if mb := int64(math.Round(val)); mb > pt.MemoryBytes {
			pt.MemoryBytes = mb
		}
	case metrics.MetricNameNodeDiskBytes:
		if db := int64(math.Round(val)); db > pt.StorageBytes {
			pt.StorageBytes = db
		}
	case metrics.MetricNameNodeNetworkRxBytesPerSec:
		if rx := int64(math.Round(val)); rx > pt.NetworkRxBytesPerSec {
			pt.NetworkRxBytesPerSec = rx
		}
	case metrics.MetricNameNodeNetworkTxBytesPerSec:
		if tx := int64(math.Round(val)); tx > pt.NetworkTxBytesPerSec {
			pt.NetworkTxBytesPerSec = tx
		}
	}
}
