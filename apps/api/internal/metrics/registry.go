package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type Registry struct {
	mu sync.Mutex

	httpRequests map[string]float64
	httpDuration map[string]*histogram
	httpInFlight map[string]float64

	sseConnections map[string]float64
	sseEvents      map[string]float64

	dbDuration map[string]*histogram
	dbErrors   map[string]float64

	prometheusDuration map[string]*histogram
	prometheusErrors   map[string]float64
}

type histogram struct {
	count   float64
	sum     float64
	buckets map[float64]float64
}

func NewRegistry() *Registry {
	return &Registry{
		httpRequests:       map[string]float64{},
		httpDuration:       map[string]*histogram{},
		httpInFlight:       map[string]float64{},
		sseConnections:     map[string]float64{},
		sseEvents:          map[string]float64{},
		dbDuration:         map[string]*histogram{},
		dbErrors:           map[string]float64{},
		prometheusDuration: map[string]*histogram{},
		prometheusErrors:   map[string]float64{},
	}
}

func (r *Registry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		method := req.Method
		route := routePattern(req)
		r.addInFlight(method, route, 1)
		defer r.addInFlight(method, route, -1)

		next.ServeHTTP(recorder, req)
		if route == "unknown" {
			route = routePattern(req)
		}
		r.ObserveHTTPRequest(method, route, recorder.status, time.Since(start))
	})
}

func (r *Registry) ObserveHTTPRequest(method, route string, status int, elapsed time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	statusLabel := strconv.Itoa(status)
	r.httpRequests[labelKey(method, route, statusLabel)]++
	h := r.histogramLocked(r.httpDuration, labelKey(method, route, statusLabel))
	h.observe(elapsed.Seconds())
}

func (r *Registry) ObservePrometheusQuery(queryType string, elapsed time.Duration, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.histogramLocked(r.prometheusDuration, labelKey(queryType)).observe(elapsed.Seconds())
	if err != nil {
		r.prometheusErrors[labelKey(queryType)]++
	}
}

func (r *Registry) ObserveDBQuery(operation string, elapsed time.Duration, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.histogramLocked(r.dbDuration, labelKey(operation)).observe(elapsed.Seconds())
	if err != nil {
		r.dbErrors[labelKey(operation)]++
	}
}

func (r *Registry) AddSSEConnection(stream string, delta float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sseConnections[labelKey(stream)] += delta
	if r.sseConnections[labelKey(stream)] < 0 {
		r.sseConnections[labelKey(stream)] = 0
	}
}

func (r *Registry) AddSSEEvent(stream, eventType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sseEvents[labelKey(stream, eventType)]++
}

func (r *Registry) PrometheusText() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var b strings.Builder
	writeCounterVec(&b, "gamepanel_api_http_requests_total", "Total HTTP requests handled by GamePanel API.", []string{"method", "route", "status"}, r.httpRequests)
	writeHistogramVec(&b, "gamepanel_api_http_request_duration_seconds", "HTTP request duration in seconds.", []string{"method", "route", "status"}, r.httpDuration)
	writeGaugeVec(&b, "gamepanel_api_http_requests_in_flight", "HTTP requests currently in flight.", []string{"method", "route"}, r.httpInFlight)
	writeGaugeVec(&b, "gamepanel_api_sse_connections_active", "Active Server-Sent Events connections.", []string{"stream"}, r.sseConnections)
	writeCounterVec(&b, "gamepanel_api_sse_events_total", "Server-Sent Events emitted by stream and type.", []string{"stream", "type"}, r.sseEvents)
	writeHistogramVec(&b, "gamepanel_api_db_query_duration_seconds", "Database query duration in seconds.", []string{"operation"}, r.dbDuration)
	writeCounterVec(&b, "gamepanel_api_db_errors_total", "Database errors by operation.", []string{"operation"}, r.dbErrors)
	writeHistogramVec(&b, "gamepanel_api_prometheus_query_duration_seconds", "Prometheus query duration in seconds.", []string{"query_type"}, r.prometheusDuration)
	writeCounterVec(&b, "gamepanel_api_prometheus_query_errors_total", "Prometheus query errors by query type.", []string{"query_type"}, r.prometheusErrors)
	return b.String()
}

func (r *Registry) addInFlight(method, route string, delta float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := labelKey(method, route)
	r.httpInFlight[key] += delta
	if r.httpInFlight[key] < 0 {
		r.httpInFlight[key] = 0
	}
}

func (r *Registry) histogramLocked(values map[string]*histogram, key string) *histogram {
	h, ok := values[key]
	if !ok {
		h = &histogram{buckets: map[float64]float64{}}
		values[key] = h
	}
	return h
}

func (h *histogram) observe(value float64) {
	h.count++
	h.sum += value
	for _, bucket := range durationBuckets {
		if value <= bucket {
			h.buckets[bucket]++
		}
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func routePattern(r *http.Request) string {
	if ctx := chi.RouteContext(r.Context()); ctx != nil {
		if pattern := ctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return "unknown"
}

func labelKey(values ...string) string {
	escaped := make([]string, len(values))
	for i, value := range values {
		escaped[i] = strings.ReplaceAll(value, "\xff", "")
	}
	return strings.Join(escaped, "\xff")
}

func splitKey(key string) []string {
	if key == "" {
		return nil
	}
	return strings.Split(key, "\xff")
}

func writeCounterVec(b *strings.Builder, name, help string, labels []string, values map[string]float64) {
	writeHeader(b, name, help, "counter")
	for _, key := range sortedKeys(values) {
		writeSample(b, name, labels, splitKey(key), values[key], "")
	}
}

func writeGaugeVec(b *strings.Builder, name, help string, labels []string, values map[string]float64) {
	writeHeader(b, name, help, "gauge")
	for _, key := range sortedKeys(values) {
		writeSample(b, name, labels, splitKey(key), values[key], "")
	}
}

func writeHistogramVec(b *strings.Builder, name, help string, labels []string, values map[string]*histogram) {
	writeHeader(b, name, help, "histogram")
	for _, key := range sortedHistogramKeys(values) {
		labelValues := splitKey(key)
		h := values[key]
		for _, bucket := range durationBuckets {
			writeSample(b, name+"_bucket", append(labels, "le"), append(labelValues, formatFloat(bucket)), h.buckets[bucket], "")
		}
		writeSample(b, name+"_bucket", append(labels, "le"), append(labelValues, "+Inf"), h.count, "")
		writeSample(b, name+"_sum", labels, labelValues, h.sum, "")
		writeSample(b, name+"_count", labels, labelValues, h.count, "")
	}
}

func writeHeader(b *strings.Builder, name, help, metricType string) {
	b.WriteString("# HELP ")
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(help)
	b.WriteByte('\n')
	b.WriteString("# TYPE ")
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(metricType)
	b.WriteByte('\n')
}

func writeSample(b *strings.Builder, name string, labelNames, labelValues []string, value float64, suffix string) {
	b.WriteString(name)
	if len(labelNames) > 0 {
		b.WriteByte('{')
		for i, labelName := range labelNames {
			if i > 0 {
				b.WriteByte(',')
			}
			labelValue := ""
			if i < len(labelValues) {
				labelValue = labelValues[i]
			}
			b.WriteString(labelName)
			b.WriteString("=\"")
			b.WriteString(escapeLabel(labelValue))
			b.WriteByte('"')
		}
		b.WriteByte('}')
	}
	if suffix != "" {
		b.WriteString(suffix)
	}
	b.WriteByte(' ')
	b.WriteString(formatFloat(value))
	b.WriteByte('\n')
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func sortedKeys(values map[string]float64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedHistogramKeys(values map[string]*histogram) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func FormatGauge(name, help string, labels []string, samples map[string]float64) string {
	var b strings.Builder
	writeGaugeVec(&b, name, help, labels, samples)
	return b.String()
}

func FormatCounter(name, help string, labels []string, samples map[string]float64) string {
	var b strings.Builder
	writeCounterVec(&b, name, help, labels, samples)
	return b.String()
}

func FormatScalar(name, help, metricType string, value float64) string {
	return fmt.Sprintf("# HELP %s %s\n# TYPE %s %s\n%s %s\n", name, help, name, metricType, name, formatFloat(value))
}
