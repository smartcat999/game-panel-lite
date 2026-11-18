package observability

import (
	"context"
	"sync"
	"time"
)

type RuntimeAvailableFunc func() bool

type CachedService struct {
	service          *Service
	runtimeAvailable RuntimeAvailableFunc
	ttl              time.Duration

	mu         sync.Mutex
	snapshot   Snapshot
	text       string
	updatedAt  time.Time
	refreshing bool
	lastErr    error
}

func NewCachedService(service *Service, runtimeAvailable RuntimeAvailableFunc, ttl time.Duration) *CachedService {
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	if runtimeAvailable == nil {
		runtimeAvailable = func() bool { return true }
	}
	return &CachedService{service: service, runtimeAvailable: runtimeAvailable, ttl: ttl}
}

func (c *CachedService) Start(ctx context.Context) {
	if c == nil || c.service == nil {
		return
	}
	c.Refresh(ctx)
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Refresh(ctx)
		}
	}
}

func (c *CachedService) Snapshot(ctx context.Context) (Snapshot, error) {
	if snapshot, _, ok, err := c.cached(); ok {
		c.refreshAsync(ctx)
		return snapshot, err
	}
	return c.Refresh(ctx)
}

func (c *CachedService) PrometheusText(ctx context.Context) (string, error) {
	if _, text, ok, err := c.cached(); ok {
		c.refreshAsync(ctx)
		return text, err
	}
	_, err := c.Refresh(ctx)
	if _, text, ok, cachedErr := c.cached(); ok {
		if err != nil {
			return text, err
		}
		return text, cachedErr
	}
	return "", err
}

func (c *CachedService) Refresh(ctx context.Context) (Snapshot, error) {
	if c == nil || c.service == nil {
		return Snapshot{}, nil
	}
	if !c.beginRefresh() {
		snapshot, _, _, err := c.cached()
		return snapshot, err
	}
	defer c.endRefresh()
	snapshot, err := c.service.Snapshot(ctx, c.runtimeAvailable())
	text := ""
	if err == nil {
		text = FormatPrometheus(snapshot)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.snapshot = snapshot
		c.text = text
		c.updatedAt = time.Now()
	}
	c.lastErr = err
	return c.snapshot, err
}

func (c *CachedService) cached() (Snapshot, string, bool, error) {
	if c == nil {
		return Snapshot{}, "", false, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ok := !c.updatedAt.IsZero()
	return c.snapshot, c.text, ok, c.lastErr
}

func (c *CachedService) refreshAsync(ctx context.Context) {
	if c == nil || c.service == nil {
		return
	}
	c.mu.Lock()
	stale := c.updatedAt.IsZero() || time.Since(c.updatedAt) > c.ttl
	refreshing := c.refreshing
	c.mu.Unlock()
	if !stale || refreshing {
		return
	}
	go func() {
		_, _ = c.Refresh(ctx)
	}()
}

func (c *CachedService) beginRefresh() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refreshing {
		return false
	}
	c.refreshing = true
	return true
}

func (c *CachedService) endRefresh() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshing = false
}
