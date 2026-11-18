package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
)

const (
	watchQueueSize   = 64
	watchReadTTL     = time.Second
	watchReadTimeout = 5 * time.Second
)

var errWatchSlow = errors.New("watch consumer is too slow; relist resources and reconnect")

// WatchFilterFunc filters materialized and deleted watch events per subscriber.
type WatchFilterFunc func(ctx context.Context, kind metav1.WatchEventType, ev mq.Event, obj any) bool

// WatchOption configures transport-local sharing and subscriber filtering, independently of ListOptions.
type WatchOption func(*watchConfig)
type watchConfig struct {
	source       any
	sharedObject bool
	filter       WatchFilterFunc
}

// WithWatchSource identifies the exact object view behind a getter. Callers must
// use the same stable operator/store instance for the same view.
func WithWatchSource(source any) WatchOption {
	return func(c *watchConfig) { c.source = comparableWatchIdentity(source) }
}

// WithWatchSharedObject marks the watch getter as principal-independent so subscribers share object reads.
func WithWatchSharedObject() WatchOption {
	return func(c *watchConfig) { c.sharedObject = true }
}

// WithWatchFilter applies a per-subscriber visibility filter on ADDED, MODIFIED, and DELETED events.
func WithWatchFilter(filter WatchFilterFunc) WatchOption {
	return func(c *watchConfig) { c.filter = filter }
}

func comparableWatchIdentity(value any) any {
	if value == nil {
		return nil
	}
	v := reflect.ValueOf(value)
	if !v.Comparable() {
		return nil
	}
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return nil
		}
	}
	return value
}

type watchHubKey struct {
	bus   any
	topic string
}

type watchHubManager struct {
	mu   sync.Mutex
	hubs map[watchHubKey]*watchHub
}

var sharedWatchHubs = watchHubManager{hubs: map[watchHubKey]*watchHub{}}

type watchHub struct {
	manager   *watchHubManager
	key       watchHubKey
	shared    bool
	ctx       context.Context
	cancel    context.CancelFunc
	ready     chan struct{}
	done      chan struct{}
	listeners map[*watchListener]struct{}
	sub       mq.Subscription
	err       error
	closing   bool
	sf        singleflight.Group
}

type watchListener struct {
	hub    *watchHub
	ctx    context.Context
	events chan mq.Event
	slow   chan struct{}
	accept func(mq.Event) bool
	once   sync.Once
}

func (m *watchHubManager) listen(ctx context.Context, bus mq.Interface, topic string, accept func(mq.Event) bool) (*watchListener, error) {
	identity := comparableWatchIdentity(bus)
	key := watchHubKey{identity, topic}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m.mu.Lock()
		h := m.hubs[key]
		if identity == nil {
			h = nil
		}
		if h != nil && h.closing {
			done := h.done
			m.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if h == nil {
			hubCtx, cancel := context.WithCancel(context.Background())
			h = &watchHub{manager: m, key: key, shared: identity != nil, ctx: hubCtx, cancel: cancel,
				ready: make(chan struct{}), done: make(chan struct{}), listeners: map[*watchListener]struct{}{}}
			if h.shared {
				m.hubs[key] = h
			}
			go h.start(bus, topic)
		}
		l := &watchListener{hub: h, ctx: ctx, events: make(chan mq.Event, watchQueueSize), slow: make(chan struct{}), accept: accept}
		h.listeners[l] = struct{}{}
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			l.release()
			return nil, ctx.Err()
		case <-h.ready:
			m.mu.Lock()
			err := h.err
			m.mu.Unlock()
			if err != nil {
				l.release()
				return nil, err
			}
			return l, nil
		}
	}
}

func (h *watchHub) start(bus mq.Interface, topic string) {
	sub, err := bus.Subscribe(h.ctx, topic, "watch-hub-"+uuid.NewString(), h.dispatch)
	if err == nil && sub == nil {
		err = errors.New("watch subscription was not created")
	}
	h.manager.mu.Lock()
	h.sub, h.err = sub, err
	close(h.ready)
	h.manager.mu.Unlock()
}

func (h *watchHub) dispatch(_ context.Context, event mq.Event) error {
	h.manager.mu.Lock()
	for listener := range h.listeners {
		if listener.ctx.Err() != nil || listener.accept != nil && !listener.accept(event) {
			continue
		}
		select {
		case <-listener.slow:
			continue
		default:
		}
		select {
		case listener.events <- event:
		default:
			listener.failSlow()
		}
	}
	h.manager.mu.Unlock()
	return nil
}

func (l *watchListener) failSlow() { l.once.Do(func() { close(l.slow) }) }

func (l *watchListener) release() {
	h := l.hub
	h.manager.mu.Lock()
	if _, ok := h.listeners[l]; !ok {
		h.manager.mu.Unlock()
		return
	}
	delete(h.listeners, l)
	if len(h.listeners) == 0 && !h.closing {
		h.closing = true
		h.cancel()
		go h.stop()
	}
	h.manager.mu.Unlock()
}

func (h *watchHub) stop() {
	<-h.ready
	h.manager.mu.Lock()
	sub := h.sub
	h.manager.mu.Unlock()
	if sub != nil {
		_ = sub.Unsubscribe()
	}
	h.manager.mu.Lock()
	if h.shared && h.manager.hubs[h.key] == h {
		delete(h.manager.hubs, h.key)
	}
	close(h.done)
	h.manager.mu.Unlock()
}

func watchPrincipal(ctx context.Context) (string, bool) {
	user, ok := request.UserFrom(ctx)
	if !ok {
		return "", false
	}
	user.Groups = append([]string(nil), user.Groups...)
	sort.Strings(user.Groups)
	raw, err := json.Marshal(user)
	return string(raw), err == nil
}

func (h *watchHub) readWithMode(ctx context.Context, source any, shared bool, object reflect.Type, event mq.Event, fetch func(context.Context) ([]byte, error)) ([]byte, error) {
	cacheKey, ok, err := watchReadCacheKey(ctx, source, shared, object, event)
	if err != nil {
		return nil, err
	}
	if !ok {
		if shared {
			fetchCtx, cleanup := watchFetchContext(ctx, context.Background())
			defer cleanup()
			return fetch(fetchCtx)
		}
		return fetch(ctx)
	}
	store := cache.Default()
	if store != nil {
		if cached, err := store.Get(ctx, cacheKey); err == nil && cached != "" {
			return []byte(cached), nil
		}
	}
	ch := h.sf.DoChan(cacheKey, func() (any, error) {
		base := context.WithoutCancel(ctx)
		if shared {
			base = context.Background()
		}
		fetchCtx, cleanup := watchFetchContext(h.ctx, base)
		defer cleanup()
		raw, err := fetch(fetchCtx)
		if err == nil && store != nil && len(raw) <= 1<<20 {
			_ = store.Set(fetchCtx, cacheKey, string(raw), watchReadTTL)
		}
		return raw, err
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.([]byte), nil
	}
}

func watchReadCacheKey(ctx context.Context, source any, shared bool, object reflect.Type, event mq.Event) (string, bool, error) {
	source = comparableWatchIdentity(source)
	scopeKey := "shared"
	if !shared {
		principal, known := watchPrincipal(ctx)
		if !known {
			return "", false, nil
		}
		scopeKey = principal
	}
	if source == nil || event.ID == "" {
		return "", false, nil
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return "", false, err
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%p|%s|%v|%s", source, scopeKey, object, encoded)))
	return "watch:read:" + hex.EncodeToString(sum[:16]), true, nil
}

func watchFetchContext(stopParent, base context.Context) (context.Context, func()) {
	fetchCtx, cancel := context.WithTimeout(base, watchReadTimeout)
	if stopParent == nil {
		return fetchCtx, cancel
	}
	stop := context.AfterFunc(stopParent, cancel)
	return fetchCtx, func() { stop(); cancel() }
}
