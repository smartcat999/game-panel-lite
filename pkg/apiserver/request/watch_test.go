package request

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsWatchRequestAndNewRequestInfo(t *testing.T) {
	resolver := NewRequestInfoResolver()
	cases := []struct {
		target string
		want   string
		watch  bool
	}{
		{"/apis/core.gamepanel.io/v1/tenants", "list", false},
		{"/apis/core.gamepanel.io/v1/tenants?watch=false", "list", false},
		{"/apis/core.gamepanel.io/v1/tenants?watch=0", "list", false},
		{"/apis/core.gamepanel.io/v1/tenants?watch=true", "watch", true},
		{"/apis/core.gamepanel.io/v1/tenants?watch=1", "watch", true},
		{"/apis/core.gamepanel.io/v1/tenants/t1/instances?watch=1", "watch", true},
		{"/apis/core.gamepanel.io/v1/tenants/t1/instances/s1/exec", "create", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.target, nil)
		if got := IsWatchRequest(req); got != tc.watch {
			t.Fatalf("%s: IsWatchRequest()=%v, want %v", tc.target, got, tc.watch)
		}
		info, err := resolver.NewRequestInfo(req)
		if err != nil {
			t.Fatalf("%s: NewRequestInfo: %v", tc.target, err)
		}
		if info.Verb != tc.want {
			t.Fatalf("%s: Verb=%q, want %q", tc.target, info.Verb, tc.want)
		}
	}
}

func TestWithWatchRevalidatorChaining(t *testing.T) {
	var calls []string
	ctx := WithWatchRevalidator(context.Background(), func(context.Context) error {
		calls = append(calls, "authn")
		return nil
	})
	ctx = WithWatchRevalidator(ctx, func(context.Context) error {
		calls = append(calls, "authz")
		return ErrWatchUnauthorized
	})
	err := RevalidateWatch(ctx)
	if !errors.Is(err, ErrWatchUnauthorized) {
		t.Fatalf("expected ErrWatchUnauthorized, got %v", err)
	}
	if len(calls) != 2 || calls[0] != "authn" || calls[1] != "authz" {
		t.Fatalf("unexpected call order: %#v", calls)
	}
	if err := RevalidateWatch(context.Background()); err != nil {
		t.Fatalf("expected nil on empty context, got %v", err)
	}
}
