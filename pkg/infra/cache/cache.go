package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/config"
)

var ErrNotFound = errors.New("cache key not found")

// Config is an alias for config.CacheConfig managed centrally in pkg/config.
type Config = config.CacheConfig

func NewConfig() *Config {
	return config.NewCacheConfig()
}

// Interface abstracts key-value TTL cache operations across memory and redis backends
// (modeled after KubeSphere pkg/simple/client/cache.Interface).
type Interface interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Ping(ctx context.Context) error
	Close() error
}

// Factory constructs a cache Interface for a given Config.
type Factory func(ctx context.Context, cfg *Config) (Interface, error)

var (
	factoriesMu sync.RWMutex
	factories   = make(map[string]Factory)

	defaultMu    sync.RWMutex
	defaultCache Interface
)

// Register is called by each cache backend file's init() (memory.go, redis.go) to self-register.
func Register(driver string, factory Factory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[driver] = factory
}

// Open initializes the configured cache backend ("memory" or "redis") and sets it as the process-wide default if none is set.
func Open(ctx context.Context, cfg *Config) (Interface, error) {
	if cfg == nil {
		cfg = &config.Get().Cache
	}
	cfg.Default()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	factoriesMu.RLock()
	factory, ok := factories[cfg.Driver]
	factoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("cache driver %q is not registered", cfg.Driver)
	}
	inst, err := factory(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defaultMu.Lock()
	if defaultCache == nil {
		defaultCache = inst
	}
	defaultMu.Unlock()
	return inst, nil
}

// Default returns the shared cache.Interface configured via Open or SetDefault.
func Default() Interface {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultCache
}

type lockProvider interface {
	TryLock(ctx context.Context, key string, ttl time.Duration) (func(), bool)
}

var localLocks sync.Map

// TryLock acquires a non-blocking distributed lease when the default cache supports it
// (e.g. Redis via redislock), combined with an in-process guard so concurrent workers
// across both threads and replicas cannot execute the same critical section simultaneously.
func TryLock(ctx context.Context, key string, ttl time.Duration) (func(), bool) {
	return TryLockOn(ctx, Default(), key, ttl)
}

// TryLockOn acquires a non-blocking lease on the supplied cache (falling back to the local in-process lock).
func TryLockOn(ctx context.Context, c Interface, key string, ttl time.Duration) (func(), bool) {
	if key == "" {
		return func() {}, true
	}
	if _, loaded := localLocks.LoadOrStore(key, struct{}{}); loaded {
		return nil, false
	}
	if c != nil {
		if lp, ok := c.(lockProvider); ok {
			releaseRemote, acquired := lp.TryLock(ctx, key, ttl)
			if !acquired {
				localLocks.Delete(key)
				return nil, false
			}
			var once sync.Once
			return func() {
				once.Do(func() {
					releaseRemote()
					localLocks.Delete(key)
				})
			}, true
		}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			localLocks.Delete(key)
		})
	}, true
}

// LockOn retries TryLockOn until wait elapses or ctx is canceled.
func LockOn(ctx context.Context, c Interface, key string, ttl, wait time.Duration) (func(), bool) {
	if unlock, ok := TryLockOn(ctx, c, key, ttl); ok {
		return unlock, true
	}
	if wait <= 0 {
		return nil, false
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, false
		case <-deadline.C:
			return nil, false
		case <-ticker.C:
			if unlock, ok := TryLockOn(ctx, c, key, ttl); ok {
				return unlock, true
			}
		}
	}
}

// SetDefault explicitly overrides the shared process-wide cache.Interface.
func SetDefault(c Interface) {
	if c == nil {
		return
	}
	defaultMu.Lock()
	defaultCache = c
	defaultMu.Unlock()
}

// GetJSON reads and unmarshals a JSON-encoded value from the given cache (or Default() when c is nil).
func GetJSON[T any](ctx context.Context, c Interface, key string) (T, bool) {
	var zero T
	if c == nil {
		c = Default()
	}
	if c == nil || key == "" {
		return zero, false
	}
	raw, err := c.Get(ctx, key)
	if err != nil || raw == "" {
		return zero, false
	}
	var out T
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return zero, false
	}
	return out, true
}

// SetJSON marshals val as JSON and writes it to the given cache (or Default() when c is nil) with ttl.
func SetJSON(ctx context.Context, c Interface, key string, val any, ttl time.Duration) error {
	if c == nil {
		c = Default()
	}
	if c == nil || key == "" {
		return nil
	}
	encoded, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, string(encoded), ttl)
}
