package tasklog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
)

const (
	TopicPrefix     = "infra.v1.tasklogs."
	CacheKeyPrefix  = "tasklog:v1:"
	DefaultMaxLines = 600
	DefaultTTL      = 72 * time.Hour

	EventLine mq.EventType = "TaskLogLine"
	EventEOF  mq.EventType = "TaskLogEOF"
)

// StreamOptions configures Snapshot + Follow streaming behavior for a task log.
type StreamOptions struct {
	Follow    bool
	TailLines int
	Completed bool
}

// Interface provides bounded persistent task log buffers paired with MQ live tailing.
type Interface interface {
	Reset(ctx context.Context, key string) error
	Append(ctx context.Context, key string, lines []string) error
	Complete(ctx context.Context, key string) error
	Snapshot(ctx context.Context, key string, tailLines int) ([]string, bool, error)
	Stream(ctx context.Context, key string, opts StreamOptions) (io.ReadCloser, error)
}

type logState struct {
	Seq       int64     `json:"seq"`
	Lines     []string  `json:"lines,omitempty"`
	Completed bool      `json:"completed,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type store struct {
	kv    cache.Interface
	bus   mq.Interface
	ttl   time.Duration
	max   int
	mu    sync.Mutex
	local map[string]string
}

// New constructs a tasklog Interface backed by cache and message queue.
func New(kv cache.Interface, bus mq.Interface) Interface {
	return &store{
		kv:    kv,
		bus:   bus,
		ttl:   DefaultTTL,
		max:   DefaultMaxLines,
		local: make(map[string]string),
	}
}

func (s *store) Reset(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("tasklog key is required")
	}
	if s.kv != nil {
		if unlock, ok := cache.LockOn(ctx, s.kv, "tasklog:lock:"+key, 5*time.Second, 500*time.Millisecond); ok {
			defer unlock()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kv != nil {
		return s.kv.Delete(ctx, CacheKeyPrefix+key)
	}
	delete(s.local, key)
	return nil
}

func (s *store) Append(ctx context.Context, key string, lines []string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("tasklog key is required")
	}
	cleaned := sanitizeLines(lines)
	if len(cleaned) == 0 {
		return nil
	}
	st, err := s.mutateState(ctx, key, func(cur *logState) {
		cur.Completed = false
		cur.Seq++
		cur.Lines = trimTail(append(cur.Lines, cleaned...), s.max)
	})
	if err != nil {
		return err
	}
	s.publishEvent(ctx, key, EventLine, st.Seq, strings.Join(cleaned, "\n"))
	return nil
}

func (s *store) Complete(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("tasklog key is required")
	}
	st, err := s.mutateState(ctx, key, func(cur *logState) {
		cur.Completed = true
		cur.Seq++
	})
	if err != nil {
		return err
	}
	s.publishEvent(ctx, key, EventEOF, st.Seq, "")
	return nil
}

func (s *store) Snapshot(ctx context.Context, key string, tailLines int) ([]string, bool, error) {
	st, err := s.loadState(ctx, key)
	if err != nil {
		return nil, false, err
	}
	return tailSlice(st.Lines, tailLines), st.Completed, nil
}

func (s *store) mutateState(ctx context.Context, key string, fn func(*logState)) (logState, error) {
	if s.kv != nil {
		if unlock, ok := cache.LockOn(ctx, s.kv, "tasklog:lock:"+key, 5*time.Second, 500*time.Millisecond); ok {
			defer unlock()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, err := s.loadStateUnlocked(ctx, key)
	if err != nil {
		return logState{}, err
	}
	fn(&cur)
	cur.UpdatedAt = time.Now().UTC()
	if err := s.saveStateUnlocked(ctx, key, cur); err != nil {
		return logState{}, err
	}
	return cur, nil
}

func (s *store) loadState(ctx context.Context, key string) (logState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadStateUnlocked(ctx, key)
}

func (s *store) loadStateUnlocked(ctx context.Context, key string) (logState, error) {
	var raw string
	var err error
	if s.kv != nil {
		raw, err = s.kv.Get(ctx, CacheKeyPrefix+key)
		if errors.Is(err, cache.ErrNotFound) {
			return logState{}, nil
		}
		if err != nil {
			return logState{}, err
		}
	} else {
		raw = s.local[key]
	}
	if raw == "" {
		return logState{}, nil
	}
	var st logState
	if uErr := json.Unmarshal([]byte(raw), &st); uErr != nil {
		return logState{}, nil
	}
	return st, nil
}

func (s *store) saveStateUnlocked(ctx context.Context, key string, st logState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if s.kv != nil {
		return s.kv.Set(ctx, CacheKeyPrefix+key, string(raw), s.ttl)
	}
	s.local[key] = string(raw)
	return nil
}

func (s *store) publishEvent(ctx context.Context, key string, evType mq.EventType, seq int64, chunk string) {
	if s.bus == nil {
		return
	}
	ev := mq.NewEvent(evType, "infra.gamepanel.io/v1", "TaskLog", "", key, seq, "")
	ev.Topic = topicForKey(key)
	ev.Attributes = map[string]string{
		"seq": strconv.FormatInt(seq, 10),
	}
	if chunk != "" {
		ev.Attributes["chunk"] = chunk
	}
	_ = s.bus.Publish(ctx, ev.Topic, ev)
}

func topicForKey(key string) string {
	var b strings.Builder
	b.Grow(len(TopicPrefix) + len(key))
	b.WriteString(TopicPrefix)
	for i := range len(key) {
		c := key[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteByte(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func sanitizeLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.ReplaceAll(raw, "\r", "")
		line = strings.TrimRight(line, "\n")
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func trimTail(lines []string, max int) []string {
	if max <= 0 || len(lines) <= max {
		return lines
	}
	cp := make([]string, max)
	copy(cp, lines[len(lines)-max:])
	return cp
}

func tailSlice(lines []string, tail int) []string {
	if tail > 0 && len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return append([]string(nil), lines...)
}

func randomGroupID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "watch-tasklog-" + hex.EncodeToString(b)
}
