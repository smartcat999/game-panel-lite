package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/metrics"
)

type PrometheusClient struct {
	baseURL string
	client  *http.Client
	metrics *metrics.Registry
}

type VectorSample struct {
	Metric map[string]string
	Value  float64
}

func NewPrometheusClient(baseURL string, timeout time.Duration, registry *metrics.Registry) *PrometheusClient {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &PrometheusClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
		metrics: registry,
	}
}

func (c *PrometheusClient) Configured() bool {
	return c != nil && c.baseURL != ""
}

func (c *PrometheusClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Point, error) {
	started := time.Now()
	points, err := c.queryRange(ctx, query, start, end, step)
	if c.metrics != nil {
		c.metrics.ObservePrometheusQuery("query_range", time.Since(started), err)
	}
	return points, err
}

func (c *PrometheusClient) QueryVector(ctx context.Context, query string) ([]VectorSample, error) {
	started := time.Now()
	samples, err := c.queryVector(ctx, query)
	if c.metrics != nil {
		c.metrics.ObservePrometheusQuery("query", time.Since(started), err)
	}
	return samples, err
}

func (c *PrometheusClient) queryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Point, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("prometheus is not configured")
	}
	endpoint, err := url.Parse(c.baseURL + "/api/v1/query_range")
	if err != nil {
		return nil, err
	}
	values := endpoint.Query()
	values.Set("query", query)
	values.Set("start", formatPromTime(start))
	values.Set("end", formatPromTime(end))
	values.Set("step", fmt.Sprintf("%gs", step.Seconds()))
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("prometheus query_range returned %s", resp.Status)
	}

	var payload promRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Status != "success" {
		return nil, fmt.Errorf("prometheus query_range failed: %s", payload.Error)
	}
	result := map[int64]float64{}
	for _, item := range payload.Data.Result {
		for _, tuple := range item.Values {
			if len(tuple) != 2 {
				continue
			}
			ts, ok := numberFromAny(tuple[0])
			if !ok {
				continue
			}
			value, ok := floatFromAny(tuple[1])
			if !ok {
				continue
			}
			result[int64(ts)] += value
		}
	}
	points := make([]Point, 0, len(result))
	for timestamp, value := range result {
		points = append(points, Point{Timestamp: time.Unix(timestamp, 0).UTC(), Value: value})
	}
	sortPoints(points)
	return points, nil
}

func (c *PrometheusClient) queryVector(ctx context.Context, query string) ([]VectorSample, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("prometheus is not configured")
	}
	endpoint, err := url.Parse(c.baseURL + "/api/v1/query")
	if err != nil {
		return nil, err
	}
	values := endpoint.Query()
	values.Set("query", query)
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("prometheus query returned %s", resp.Status)
	}
	var payload promVectorResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s", payload.Error)
	}
	samples := make([]VectorSample, 0, len(payload.Data.Result))
	for _, item := range payload.Data.Result {
		if len(item.Value) != 2 {
			continue
		}
		value, ok := floatFromAny(item.Value[1])
		if !ok {
			continue
		}
		samples = append(samples, VectorSample{Metric: item.Metric, Value: value})
	}
	return samples, nil
}

type promRangeResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Values [][]any           `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

type promVectorResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

func formatPromTime(value time.Time) string {
	return strconv.FormatFloat(float64(value.Unix()), 'f', -1, 64)
}

func numberFromAny(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func floatFromAny(value any) (float64, bool) {
	return numberFromAny(value)
}
