package request

import (
	"context"
	"errors"
	"net/http"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
)

var watchableCollections = map[string]map[string]bool{
	iamv1.GroupName: {
		"users": true, "globalroles": true, "globalrolebindings": true, "tenantroles": true, "tenantrolebindings": true,
	},
	corev1.GroupName: {
		"tenants": true, "runtimeclasses": true, "apps": true, "nodes": true, "prewarmjobs": true, "instances": true,
	},
	registryv1.GroupName: {
		"repositories": true, "artifacts": true,
	},
}

// IsWatchableCollection reports whether apiGroup/resource serves a collection watch stream.
func IsWatchableCollection(apiGroup, resource string) bool {
	return watchableCollections[apiGroup][resource]
}

// ErrWatchUnauthorized indicates that watch stream authentication or authorization was revoked.
var ErrWatchUnauthorized = errors.New("watch access revoked")

// WatchRevalidator re-verifies that an active watch stream remains authenticated and authorized.
type WatchRevalidator func(context.Context) error

type watchRevalidatorKey struct{}

// WithWatchRevalidator attaches fn to ctx, chaining it after any previously registered revalidator.
func WithWatchRevalidator(ctx context.Context, fn WatchRevalidator) context.Context {
	if fn == nil {
		return ctx
	}
	prev := WatchRevalidatorFrom(ctx)
	if prev == nil {
		return context.WithValue(ctx, watchRevalidatorKey{}, fn)
	}
	return context.WithValue(ctx, watchRevalidatorKey{}, WatchRevalidator(func(checkCtx context.Context) error {
		if err := prev(checkCtx); err != nil {
			return err
		}
		return fn(checkCtx)
	}))
}

// WithWatchRequestRevalidator chains an HTTP-request-based revalidation callback onto ctx.
func WithWatchRequestRevalidator(ctx context.Context, req *http.Request, fn func(*http.Request) error) context.Context {
	if fn == nil || req == nil {
		return ctx
	}
	return WithWatchRevalidator(ctx, func(checkCtx context.Context) error {
		cloned := req.Clone(checkCtx)
		cloned.Body = nil
		return fn(cloned)
	})
}

// WatchRevalidatorFrom extracts the chained watch revalidator from ctx, if any.
func WatchRevalidatorFrom(ctx context.Context) WatchRevalidator {
	if ctx == nil {
		return nil
	}
	fn, _ := ctx.Value(watchRevalidatorKey{}).(WatchRevalidator)
	return fn
}

// RevalidateWatch runs all watch revalidation checks attached to ctx.
func RevalidateWatch(ctx context.Context) error {
	fn := WatchRevalidatorFrom(ctx)
	if fn == nil {
		return nil
	}
	return fn(ctx)
}
