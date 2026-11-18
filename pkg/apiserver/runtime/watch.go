package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"time"

	"github.com/gorilla/websocket"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
)

var watchWSUpgrader = websocket.Upgrader{
	ReadBufferSize: 4096, WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool { return true },
}

const watchWriteTimeout = 5 * time.Second

type deletedResourceObject struct {
	metav1.TypeMeta `json:",inline"`
	Metadata        metav1.ObjectMeta `json:"metadata"`
}

// IsWatchRequest returns true if the request specifies ?watch=true or ?watch=1.
func IsWatchRequest(r *http.Request) bool {
	return request.IsWatchRequest(r)
}

// ServeWatch streams WatchEvents over NDJSON or WebSocket. The optional source
// enables view-scoped getter merging; old callers retain independent reads.
func ServeWatch[T metav1.Object](
	w http.ResponseWriter, r *http.Request, bus mq.Interface, topic, tenant, nodeName string,
	opts db.ListOptions, getter func(context.Context, string, string) (T, error), options ...WatchOption,
) {
	if bus == nil {
		WriteError(w, http.StatusServiceUnavailable, "watch requires message queue")
		return
	}
	cfg := watchConfig{}
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	listener, err := sharedWatchHubs.listen(ctx, bus, topic, func(ev mq.Event) bool {
		return watchEventType(ev.Type) != "" && (tenant == "" || ev.Tenant == tenant || topic == mq.TopicTenant && ev.Tenant == "" && ev.Name == tenant)
	})
	if err != nil {
		if ctx.Err() == nil {
			WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	defer listener.release()
	out := make(chan metav1.WatchEvent[any], watchQueueSize)
	workerDone := startWatchMaterializer(ctx, listener, cfg, nodeName, opts, getter, out)
	defer func() { cancel(); <-workerDone }()
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Time{})
	if websocket.IsWebSocketUpgrade(r) {
		serveWatchWebSocket(w, r, ctx, cancel, listener, out)
		return
	}
	serveWatchNDJSON(w, ctx, controller, listener, out)
}

const (
	watchRevalidateInterval                       = time.Second
	watchRevalidateTimeout                        = 2 * time.Second
	watchErrorEventType     metav1.WatchEventType = "ERROR"
)

func startWatchMaterializer[T metav1.Object](
	ctx context.Context, listener *watchListener, cfg watchConfig, nodeName string,
	opts db.ListOptions, getter func(context.Context, string, string) (T, error), out chan<- metav1.WatchEvent[any],
) <-chan struct{} {
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		ticker := time.NewTicker(watchRevalidateInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-listener.slow:
				return
			case <-ticker.C:
				if !ensureWatchAuthorized(ctx, listener, out) {
					return
				}
			case event := <-listener.events:
				if !ensureWatchAuthorized(ctx, listener, out) {
					return
				}
				watchEvent, ok := materializeWatchEvent(ctx, listener.hub, cfg, event, nodeName, opts, getter)
				if !ok {
					continue
				}
				if !emitWatchEvent(ctx, listener, out, watchEvent) || watchEvent.Type == watchErrorEventType {
					return
				}
			}
		}
	}()
	return workerDone
}

func ensureWatchAuthorized(ctx context.Context, listener *watchListener, out chan<- metav1.WatchEvent[any]) bool {
	checkCtx, cancel := context.WithTimeout(ctx, watchRevalidateTimeout)
	defer cancel()
	if err := request.RevalidateWatch(checkCtx); err != nil {
		if ctx.Err() == nil {
			_ = emitWatchEvent(ctx, listener, out, watchAccessRevokedEvent())
		}
		return false
	}
	return true
}

func emitWatchEvent(ctx context.Context, listener *watchListener, out chan<- metav1.WatchEvent[any], ev metav1.WatchEvent[any]) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	default:
		listener.failSlow()
		return false
	}
}

func watchAccessRevokedEvent() metav1.WatchEvent[any] {
	return metav1.WatchEvent[any]{Type: watchErrorEventType, Object: map[string]any{
		"kind": "Status", "apiVersion": "v1", "status": "Failure", "reason": "Forbidden", "code": http.StatusForbidden, "message": request.ErrWatchUnauthorized.Error(),
	}}
}

func serveWatchWebSocket(w http.ResponseWriter, r *http.Request, ctx context.Context, cancel context.CancelFunc, listener *watchListener, out <-chan metav1.WatchEvent[any]) {
	conn, err := watchWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()
	defer func() { _ = conn.Close(); <-readDone }()
	write := func(event metav1.WatchEvent[any]) error {
		_ = conn.SetWriteDeadline(time.Now().Add(watchWriteTimeout))
		return conn.WriteJSON(event)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-listener.slow:
			_ = write(watchSlowEvent())
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, errWatchSlow.Error()), time.Now().Add(watchWriteTimeout))
			return
		case event := <-out:
			if err := write(event); err != nil || event.Type == "ERROR" {
				return
			}
		}
	}
}

func serveWatchNDJSON(w http.ResponseWriter, ctx context.Context, controller *http.ResponseController, listener *watchListener, out <-chan metav1.WatchEvent[any]) {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Watch-Reconnect", "relist")
	_ = controller.SetWriteDeadline(time.Now().Add(watchWriteTimeout))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("\n")); err != nil {
		return
	}
	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}
	enc := json.NewEncoder(w)
	write := func(event metav1.WatchEvent[any]) error {
		_ = controller.SetWriteDeadline(time.Now().Add(watchWriteTimeout))
		if err := enc.Encode(event); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-listener.slow:
			_ = write(watchSlowEvent())
			return
		case event := <-out:
			if err := write(event); err != nil || event.Type == "ERROR" {
				return
			}
		}
	}
}

func watchSlowEvent() metav1.WatchEvent[any] { return watchRelistEvent(errWatchSlow.Error()) }

func watchRelistEvent(message string) metav1.WatchEvent[any] {
	return metav1.WatchEvent[any]{Type: metav1.WatchEventType("ERROR"), Object: map[string]any{
		"kind": "Status", "apiVersion": "v1", "status": "Failure", "reason": "Expired", "code": http.StatusGone, "message": message,
	}}
}

func watchEventType(t mq.EventType) metav1.WatchEventType {
	switch t {
	case mq.EventCreated:
		return metav1.WatchAdded
	case mq.EventUpdated, mq.EventStateChanged, mq.EventReconcile:
		return metav1.WatchModified
	case mq.EventDeleted:
		return metav1.WatchDeleted
	}
	return ""
}

func materializeWatchEvent[T metav1.Object](ctx context.Context, hub *watchHub, cfg watchConfig, ev mq.Event, nodeName string, opts db.ListOptions, getter func(context.Context, string, string) (T, error)) (metav1.WatchEvent[any], bool) {
	kind := watchEventType(ev.Type)
	targetNode := nodeName
	if targetNode == "" {
		targetNode = opts.FieldSelector["spec.nodeName"]
	}
	if kind == metav1.WatchDeleted {
		watchEvent, ok := materializeDeletedWatchEvent(ev, targetNode, opts)
		if !ok || watchEvent.Type == watchErrorEventType {
			return watchEvent, ok
		}
		if cfg.filter != nil && !cfg.filter(ctx, kind, ev, watchEvent.Object) {
			return metav1.WatchEvent[any]{}, false
		}
		return watchEvent, true
	}
	if getter == nil || kind == "" {
		return metav1.WatchEvent[any]{}, false
	}
	raw, err := hub.readWithMode(ctx, cfg.source, cfg.sharedObject, reflect.TypeFor[T](), ev, func(getCtx context.Context) ([]byte, error) {
		obj, err := getter(getCtx, ev.Tenant, ev.Name)
		if err != nil {
			return nil, err
		}
		if isNilWatchObject(obj) {
			return nil, db.ErrNotFound
		}
		return json.Marshal(obj)
	})
	if err != nil {
		return metav1.WatchEvent[any]{}, false
	}
	var obj T
	if json.Unmarshal(raw, &obj) != nil || isNilWatchObject(obj) {
		return metav1.WatchEvent[any]{}, false
	}
	meta := obj.GetObjectMeta()
	if meta != nil && opts.LabelSelector != nil && !opts.LabelSelector.Matches(meta.Labels) {
		return metav1.WatchEvent[any]{}, false
	}
	if targetNode != "" {
		if inst, ok := any(obj).(*corev1.Instance); ok && inst.Spec.NodeName != targetNode {
			return metav1.WatchEvent[any]{}, false
		}
	}
	if cfg.filter != nil && !cfg.filter(ctx, kind, ev, obj) {
		return metav1.WatchEvent[any]{}, false
	}
	return metav1.WatchEvent[any]{Type: kind, Object: obj}, true
}

func materializeDeletedWatchEvent(ev mq.Event, targetNode string, opts db.ListOptions) (metav1.WatchEvent[any], bool) {
	if targetNode != "" {
		evNode, known := ev.Attributes["nodeName"]
		if !known {
			return watchRelistEvent("deleted object has no node scope observation; relist resources and reconnect"), true
		}
		if evNode != targetNode {
			return metav1.WatchEvent[any]{}, false
		}
	}
	var labels map[string]string
	if raw := ev.Attributes["labels"]; raw != "" {
		if json.Unmarshal([]byte(raw), &labels) != nil {
			return watchRelistEvent("deleted object has invalid label observations; relist resources and reconnect"), true
		}
	} else if opts.LabelSelector != nil {
		return watchRelistEvent("deleted object has no label scope observation; relist resources and reconnect"), true
	}
	if opts.LabelSelector != nil && !opts.LabelSelector.Matches(labels) {
		return metav1.WatchEvent[any]{}, false
	}
	return metav1.WatchEvent[any]{Type: metav1.WatchDeleted, Object: deletedResourceObject{
		TypeMeta: metav1.TypeMeta{APIVersion: ev.APIVersion, Kind: ev.Kind},
		Metadata: metav1.ObjectMeta{Name: ev.Name, Tenant: ev.Tenant, Generation: ev.Generation, ResourceVersion: ev.ResourceVersion, Labels: labels},
	}}, true
}

func isNilWatchObject[T any](obj T) bool {
	v := reflect.ValueOf(obj)
	return !v.IsValid() || (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil()
}
