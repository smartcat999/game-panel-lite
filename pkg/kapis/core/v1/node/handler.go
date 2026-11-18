package node

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	iamv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/iam/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	nodemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/node"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type handler struct {
	op nodemodel.Operator
	mq mq.Interface
}

func newHandler(op nodemodel.Operator, bus ...mq.Interface) *handler {
	var q mq.Interface
	if len(bus) > 0 {
		q = bus[0]
	}
	return &handler{op: op, mq: q}
}

func (h *handler) ListNodes(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(w, r, h.mq, mq.TopicNode, "", "", opts, func(ctx context.Context, _, name string) (*corev1.Node, error) {
			return h.op.Get(ctx, name)
		}, apiruntime.WithWatchSource(h.op))
		return
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) CreateNode(w http.ResponseWriter, r *http.Request) {
	var node corev1.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if u, ok := request.UserFrom(r.Context()); ok && u.Kind == request.KindNode {
		// a node may only register itself; its identity comes from its signed session token
		if node.Metadata.Name == "" {
			node.Metadata.Name = u.Name
		}
		if node.Metadata.Name != u.Name {
			apiruntime.WriteError(w, http.StatusForbidden, "NodeRestriction: node "+u.Name+" cannot register node "+node.Metadata.Name)
			return
		}
	}
	created, err := h.op.Create(r.Context(), &node)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), created.Metadata.Name)
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetNode(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, obj)
}

func (h *handler) GetNodeMetrics(w http.ResponseWriter, r *http.Request) {
	var window time.Duration
	if raw := strings.TrimSpace(r.URL.Query().Get("window")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			window = d
		}
	}
	metricsObj, err := h.op.GetMetrics(r.Context(), r.PathValue("name"), window)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, metricsObj)
}

func (h *handler) UpdateNode(w http.ResponseWriter, r *http.Request) {
	var node corev1.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), r.PathValue("name"), &node)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) PatchNode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	current, err := h.op.Get(r.Context(), name)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	beforeJSON, _ := json.Marshal(map[string]any{"spec": current.Spec})
	patched, err := apiruntime.ApplyMergePatch(r, current)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), name, patched)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	afterJSON, _ := json.Marshal(map[string]any{"spec": updated.Spec})
	request.SetAuditTarget(r.Context(), "", name, map[string]string{
		"diffBefore": string(beforeJSON),
		"diffAfter":  string(afterJSON),
	})
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) UpdateNodeStatus(w http.ResponseWriter, r *http.Request) {
	var node corev1.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	nodeName := r.PathValue("name")
	if u, ok := request.UserFrom(r.Context()); ok && u.Kind == request.KindNode && u.Name != "" && u.Name != iamv1.PrincipalNodeAgent && u.Name == nodeName {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			if rawToken := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")); rawToken != "" {
				if kv := cache.Default(); kv != nil {
					_ = kv.Set(r.Context(), iamv1.CacheKeyPrefixNodeToken+u.Name, rawToken, 365*24*time.Hour)
				}
			}
		}
	}
	updated, err := h.op.UpdateStatus(r.Context(), nodeName, &node)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) DeleteNode(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("name")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) ExecNode(w http.ResponseWriter, r *http.Request) {
	if !websocket.IsWebSocketUpgrade(r) {
		apiruntime.WriteError(w, http.StatusMethodNotAllowed, "node exec requires WebSocket upgrade")
		return
	}
	name := r.PathValue("name")
	q := r.URL.Query()
	cols, _ := strconv.Atoi(q.Get("cols"))
	rows, _ := strconv.Atoi(q.Get("rows"))
	if cols < 0 {
		cols = 0
	}
	if rows < 0 {
		rows = 0
	}
	c, rws := drvruntime.NormalizeTerminalSize(uint(cols), uint(rows))
	attrs := map[string]string{"tty": "true", "cols": strconv.Itoa(int(c)), "rows": strconv.Itoa(int(rws))}
	if len(q["command"]) > 0 {
		attrs["command"] = strings.Join(q["command"], " ")
	}
	request.SetAuditTarget(r.Context(), "exec", name, attrs)
	session, err := h.op.OpenExecTTY(r.Context(), name, drvruntime.InteractiveExecRequest{
		Command: q["command"],
		Cols:    c,
		Rows:    rws,
	})
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	defer func() { _ = session.Close() }()

	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	_ = rc.SetReadDeadline(time.Time{})
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Time{})
	drvruntime.ServePTYWebSocket(r.Context(), conn, session)
}
