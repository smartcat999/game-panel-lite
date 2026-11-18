package request

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWatchableCollectionBounds(t *testing.T) {
	resolver := NewRequestInfoResolver()
	cases := []struct {
		method string
		target string
		want   string
		watch  bool
	}{
		{http.MethodGet, "/apis/core.gamepanel.io/v1/tenants/t1/instances/s1?watch=1", "get", false},
		{http.MethodGet, "/apis/core.gamepanel.io/v1/tenants/t1/instances/s1/log?watch=1", "get", false},
		{http.MethodGet, "/apis/core.gamepanel.io/v1/tenants/t1/events?watch=1", "list", false},
		{http.MethodPost, "/apis/core.gamepanel.io/v1/tenants/t1/instances?watch=1", "create", false},
		{http.MethodGet, "/apis/iam.gamepanel.io/v1/users?watch=1", "watch", true},
		{http.MethodGet, "/apis/core.gamepanel.io/v1/apps?watch=1", "watch", true},
		{http.MethodGet, "/apis/core.gamepanel.io/v1/prewarmjobs?watch=1", "watch", true},
		{http.MethodGet, "/apis/registry.gamepanel.io/v1/repositories?watch=1", "watch", true},
		{http.MethodGet, "/apis/registry.gamepanel.io/v1/artifacts?watch=1", "watch", true},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.target, nil)
		info, err := resolver.NewRequestInfo(req)
		if err != nil {
			t.Fatalf("%s %s: NewRequestInfo error: %v", tc.method, tc.target, err)
		}
		if info.Verb != tc.want {
			t.Fatalf("%s %s: verb=%q, want %q", tc.method, tc.target, info.Verb, tc.want)
		}
		if got := IsWatchRequest(req); got != tc.watch {
			t.Fatalf("%s %s: IsWatchRequest=%v, want %v", tc.method, tc.target, got, tc.watch)
		}
	}
	if IsWatchableCollection("core.gamepanel.io", "events") || !IsWatchableCollection("core.gamepanel.io", "instances") {
		t.Fatal("unexpected IsWatchableCollection mapping")
	}
}
