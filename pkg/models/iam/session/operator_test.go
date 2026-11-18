package session

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
)

type testSessionCache struct {
	mu    sync.Mutex
	items map[string]string
}

func newTestSessionCache() *testSessionCache {
	return &testSessionCache{items: make(map[string]string)}
}

func (c *testSessionCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.items[key]
	if !ok {
		return "", cache.ErrNotFound
	}
	return v, nil
}

func (c *testSessionCache) Set(_ context.Context, key, value string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = value
	return nil
}

func (c *testSessionCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
	return nil
}

func (c *testSessionCache) Exists(_ context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.items[key]
	return ok, nil
}

func (c *testSessionCache) Ping(context.Context) error { return nil }
func (c *testSessionCache) Close() error               { return nil }

func TestVerifySessionRejectsTrailingBitTampering(t *testing.T) {
	ctx := context.Background()
	op := NewOperator(nil, newTestSessionCache(), &Config{
		JWTSecret: "0123456789abcdef0123456789abcdef",
		TokenTTL:  time.Hour,
	})
	var canonical string
	for i := range 1024 {
		sess, err := op.Issue(ctx, iamv1.SubjectKindUser, fmt.Sprintf("u-%d", i), "default")
		if err != nil {
			t.Fatalf("Issue: %v", err)
		}
		if strings.HasSuffix(sess.Token, "A") {
			canonical = sess.Token
			break
		}
	}
	if canonical == "" {
		t.Fatal("failed to find issued session JWT ending with 'A' within 1024 iterations")
	}
	if _, err := op.Verify(ctx, canonical); err != nil {
		t.Fatalf("Verify rejected canonical session JWT: %v", err)
	}
	tampered := []string{
		canonical[:len(canonical)-1] + "B",
		canonical[:len(canonical)-1] + "C",
		canonical[:len(canonical)-1] + "D",
		canonical[:len(canonical)-1] + "Q",
		canonical + "=",
	}
	for _, tok := range tampered {
		if _, err := op.Verify(ctx, tok); err == nil {
			t.Fatalf("Verify accepted non-canonical tampered session JWT %q", tok)
		}
	}
}
