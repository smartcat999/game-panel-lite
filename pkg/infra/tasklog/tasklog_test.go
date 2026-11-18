package tasklog

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
)

type memSub struct {
	cancel func()
}

func (s *memSub) Unsubscribe() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

type memMQ struct {
	mu   sync.Mutex
	subs map[string]map[int]mq.HandlerFunc
	seq  int
}

func newMemMQ() *memMQ {
	return &memMQ{subs: make(map[string]map[int]mq.HandlerFunc)}
}

func (m *memMQ) Publish(ctx context.Context, topic string, event mq.Event) error {
	m.mu.Lock()
	handlers := make([]mq.HandlerFunc, 0, len(m.subs[topic]))
	for _, h := range m.subs[topic] {
		handlers = append(handlers, h)
	}
	m.mu.Unlock()
	for _, h := range handlers {
		_ = h(ctx, event)
	}
	return nil
}

func (m *memMQ) Subscribe(_ context.Context, topic, _ string, handler mq.HandlerFunc) (mq.Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := m.seq
	if m.subs[topic] == nil {
		m.subs[topic] = make(map[int]mq.HandlerFunc)
	}
	m.subs[topic][id] = handler
	return &memSub{cancel: func() {
		m.mu.Lock()
		delete(m.subs[topic], id)
		m.mu.Unlock()
	}}, nil
}

func (m *memMQ) Ping(_ context.Context) error { return nil }
func (m *memMQ) Close() error                 { return nil }

func TestTaskLogAppendSnapshotAndStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tl := New(nil, newMemMQ())
	key := "registry.artifact:steam-dst.dst.latest"

	if err := tl.Append(ctx, key, []string{"line 1", "line 2"}); err != nil {
		t.Fatalf("append initial lines: %v", err)
	}
	lines, done, err := tl.Snapshot(ctx, key, 1)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if done || len(lines) != 1 || lines[0] != "line 2" {
		t.Fatalf("unexpected tail snapshot: lines=%v done=%v", lines, done)
	}

	rc, err := tl.Stream(ctx, key, StreamOptions{Follow: true, TailLines: 10})
	if err != nil {
		t.Fatalf("stream follow: %v", err)
	}
	defer func() { _ = rc.Close() }()

	if err := tl.Append(ctx, key, []string{"line 3"}); err != nil {
		t.Fatalf("append live line: %v", err)
	}
	if err := tl.Complete(ctx, key); err != nil {
		t.Fatalf("complete log: %v", err)
	}

	raw, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read followed stream: %v", err)
	}
	got := strings.TrimSpace(string(raw))
	want := "line 1\nline 2\nline 3"
	if got != want {
		t.Fatalf("unexpected followed stream output:\ngot:  %q\nwant: %q", got, want)
	}

	if err := tl.Reset(ctx, key); err != nil {
		t.Fatalf("reset log: %v", err)
	}
	afterReset, doneAfter, err := tl.Snapshot(ctx, key, 10)
	if err != nil || doneAfter || len(afterReset) != 0 {
		t.Fatalf("expected empty state after reset, got lines=%v done=%v err=%v", afterReset, doneAfter, err)
	}
}
