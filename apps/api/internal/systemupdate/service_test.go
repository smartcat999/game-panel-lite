package systemupdate

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func TestCheckFindsNewerRelease(t *testing.T) {
	service := New("https://updates.example/manifest.json", "", "", time.Second)
	service.client.Transport = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"schemaVersion":1,"channel":"stable","version":"v1.3.0"}`), nil
	})
	service.current.Version = "v1.2.0"
	status, err := service.Check(context.Background(), true, 24)
	if err != nil {
		t.Fatalf("check update: %v", err)
	}
	if !status.UpdateAvailable || status.Latest == nil || status.Latest.Version != "v1.3.0" {
		t.Fatalf("expected v1.3.0 update, got %#v", status)
	}
	if status.CheckedAt == "" || status.CheckError != "" {
		t.Fatalf("expected successful check metadata, got %#v", status)
	}
}

func TestCheckRejectsInvalidManifest(t *testing.T) {
	service := New("https://updates.example/manifest.json", "", "", time.Second)
	service.client.Transport = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"schemaVersion":1,"version":"latest"}`), nil
	})
	status, err := service.Check(context.Background(), true, 24)
	if err == nil {
		t.Fatal("expected invalid manifest error")
	}
	if status.CheckError == "" || status.CheckedAt == "" {
		t.Fatalf("expected failure metadata, got %#v", status)
	}
}

func TestApplyUsesAuthenticatedUpdater(t *testing.T) {
	var authorization string
	service := New("https://updates.example/manifest.json", "http://updater.internal", "secret", time.Second)
	service.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/manifest.json":
			return jsonResponse(http.StatusOK, `{"schemaVersion":1,"channel":"stable","version":"v2.0.0"}`), nil
		case "/apply":
			authorization = r.Header.Get("Authorization")
			return jsonResponse(http.StatusAccepted, `{"id":"job-1","version":"v2.0.0","status":"running"}`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{}`), nil
		}
	})
	service.current.Version = "v1.0.0"
	if _, err := service.Check(context.Background(), true, 24); err != nil {
		t.Fatalf("check update: %v", err)
	}
	job, err := service.Apply(context.Background(), "v2.0.0")
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}
	if job.ID != "job-1" || authorization != "Bearer secret" {
		t.Fatalf("unexpected updater request: job=%#v authorization=%q", job, authorization)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"v1.2.0", "v1.1.9", 1},
		{"v1.2.0", "1.2.0", 0},
		{"v1.2.0-rc.1", "v1.2.0", -1},
	}
	for _, test := range tests {
		got := compareVersions(test.left, test.right)
		if got != test.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}
