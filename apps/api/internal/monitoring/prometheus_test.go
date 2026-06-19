package monitoring

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPrometheusClientQueryRangeParsesMatrix(t *testing.T) {
	client := NewPrometheusClient("http://prometheus.test", time.Second, nil)
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if !strings.Contains(r.URL.Query().Get("query"), "gamepanel_runtime_cpu_percent") {
			t.Fatalf("unexpected query %q", r.URL.Query().Get("query"))
		}
		return response(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1000,"1.5"],[1030,"2.5"]]}]}}`), nil
	})

	points, err := client.QueryRange(context.Background(), "gamepanel_runtime_cpu_percent", time.Unix(1000, 0), time.Unix(1030, 0), 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 || points[0].Value != 1.5 || points[1].Value != 2.5 {
		t.Fatalf("unexpected points %+v", points)
	}
}

func TestPrometheusClientQueryVectorReturnsErrorStatus(t *testing.T) {
	client := NewPrometheusClient("http://prometheus.test", time.Second, nil)
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return response(`{"status":"error","error":"bad query"}`), nil
	})
	if _, err := client.QueryVector(context.Background(), "up"); err == nil {
		t.Fatal("expected query error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func response(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}
