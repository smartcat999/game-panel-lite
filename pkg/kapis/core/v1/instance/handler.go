package instance

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/apiserver/request"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/mq"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type handler struct {
	op instancemodel.Operator
	mq mq.Interface
}

func newHandler(op instancemodel.Operator, bus ...mq.Interface) *handler {
	var q mq.Interface
	if len(bus) > 0 {
		q = bus[0]
	}
	return &handler{op: op, mq: q}
}

func enforceNodeInstanceListScope(r *http.Request, opts *db.ListOptions, nodeName string) (string, int, string) {
	u, ok := request.UserFrom(r.Context())
	if !ok || u.Kind != request.KindNode {
		return nodeName, 0, ""
	}
	if u.Name == "" {
		return "", http.StatusForbidden, "node identity is missing name"
	}
	for _, rawNode := range r.URL.Query()["nodeName"] {
		if rawNode != "" && rawNode != u.Name {
			return "", http.StatusForbidden, "node " + u.Name + " cannot list instances for node " + rawNode
		}
	}
	if nodeName != "" && nodeName != u.Name {
		return "", http.StatusForbidden, "node " + u.Name + " cannot list instances for node " + nodeName
	}
	if sel, exists := opts.FieldSelector["spec.nodeName"]; exists && sel != u.Name {
		return "", http.StatusForbidden, "node " + u.Name + " cannot query spec.nodeName=" + sel
	}
	if _, conflict := opts.FieldSelector["spec.nodeName!="]; conflict {
		return "", http.StatusForbidden, "node " + u.Name + " cannot query spec.nodeName!="
	}
	if opts.FieldSelector == nil {
		opts.FieldSelector = make(map[string]string, 1)
	}
	opts.FieldSelector["spec.nodeName"] = u.Name
	return u.Name, 0, ""
}

func (h *handler) ListClusterInstances(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	nodeName, status, msg := enforceNodeInstanceListScope(r, &opts, r.URL.Query().Get("nodeName"))
	if status != 0 {
		apiruntime.WriteError(w, status, msg)
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(w, r, h.mq, mq.TopicInstance, opts.Tenant, nodeName, opts, h.op.Get, apiruntime.WithWatchSource(h.op), apiruntime.WithWatchSharedObject())
		return
	}
	list, err := h.op.ListCluster(r.Context(), nodeName, opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) ListInstances(w http.ResponseWriter, r *http.Request) {
	opts, err := apiruntime.ParseListOptions(r)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	nodeName, status, msg := enforceNodeInstanceListScope(r, &opts, r.URL.Query().Get("nodeName"))
	if status != 0 {
		apiruntime.WriteError(w, status, msg)
		return
	}
	if apiruntime.IsWatchRequest(r) {
		apiruntime.ServeWatch(w, r, h.mq, mq.TopicInstance, opts.Tenant, nodeName, opts, h.op.Get, apiruntime.WithWatchSource(h.op), apiruntime.WithWatchSharedObject())
		return
	}
	list, err := h.op.List(r.Context(), opts)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, list)
}

func (h *handler) CreateInstance(w http.ResponseWriter, r *http.Request) {
	var inst corev1.Instance
	if err := json.NewDecoder(r.Body).Decode(&inst); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.op.Create(r.Context(), r.PathValue("tenant"), &inst)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	request.SetAuditResourceName(r.Context(), created.Metadata.Name)
	apiruntime.WriteJSON(w, http.StatusCreated, created)
}

func (h *handler) GetInstance(w http.ResponseWriter, r *http.Request) {
	obj, err := h.op.Get(r.Context(), r.PathValue("tenant"), r.PathValue("name"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, obj)
}

func (h *handler) UpdateInstance(w http.ResponseWriter, r *http.Request) {
	var inst corev1.Instance
	if err := json.NewDecoder(r.Body).Decode(&inst); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), r.PathValue("tenant"), r.PathValue("name"), &inst)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) PatchInstance(w http.ResponseWriter, r *http.Request) {
	tenant := r.PathValue("tenant")
	name := r.PathValue("name")
	current, err := h.op.Get(r.Context(), tenant, name)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	beforeJSON, _ := json.Marshal(map[string]any{
		"spec": map[string]any{
			"state":     current.Spec.State,
			"image":     current.Spec.Image,
			"resources": current.Spec.Resources,
		},
	})
	patched, err := apiruntime.ApplyMergePatch(r, current)
	if err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.Update(r.Context(), tenant, name, patched)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	afterJSON, _ := json.Marshal(map[string]any{
		"spec": map[string]any{
			"state":     updated.Spec.State,
			"image":     updated.Spec.Image,
			"resources": updated.Spec.Resources,
		},
	})
	request.SetAuditTarget(r.Context(), "", name, map[string]string{
		"diffBefore": string(beforeJSON),
		"diffAfter":  string(afterJSON),
	})
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) UpdateInstanceStatus(w http.ResponseWriter, r *http.Request) {
	var inst corev1.Instance
	if err := json.NewDecoder(r.Body).Decode(&inst); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.op.UpdateStatus(r.Context(), r.PathValue("tenant"), r.PathValue("name"), &inst)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

func (h *handler) DeleteInstance(w http.ResponseWriter, r *http.Request) {
	if err := h.op.Delete(r.Context(), r.PathValue("tenant"), r.PathValue("name")); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *handler) GetInstanceState(w http.ResponseWriter, r *http.Request) {
	state, err := h.op.GetState(r.Context(), r.PathValue("tenant"), r.PathValue("name"))
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, state)
}

func (h *handler) GetInstanceMetrics(w http.ResponseWriter, r *http.Request) {
	var window time.Duration
	if raw := strings.TrimSpace(r.URL.Query().Get("window")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			window = d
		}
	}
	metricsObj, err := h.op.GetMetrics(r.Context(), r.PathValue("tenant"), r.PathValue("name"), window)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, metricsObj)
}

func (h *handler) UpdateInstanceState(w http.ResponseWriter, r *http.Request) {
	var req corev1.InstanceState
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	prevState := string(corev1.InstanceStateStopped)
	if req.State == corev1.InstanceStateStopped {
		prevState = string(corev1.InstanceStateRunning)
	}
	request.SetAuditTarget(r.Context(), string(req.State), r.PathValue("name"), map[string]string{
		"diffBefore": `{"state":"` + prevState + `"}`,
		"diffAfter":  `{"state":"` + string(req.State) + `"}`,
	})
	updated, err := h.op.UpdateState(r.Context(), r.PathValue("tenant"), r.PathValue("name"), &req)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, updated)
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if fw.f != nil {
		fw.f.Flush()
	}
	return n, err
}

func (h *handler) StreamInstanceLogs(w http.ResponseWriter, r *http.Request) {
	follow := r.URL.Query().Get("follow") == "true"
	var tailLines int
	if tl := r.URL.Query().Get("tailLines"); tl != "" {
		tailLines, _ = strconv.Atoi(tl)
	}
	var (
		rc  io.ReadCloser
		err error
	)
	if taskID := strings.TrimSpace(r.URL.Query().Get("taskID")); taskID != "" {
		rc, err = h.op.StreamTaskLogs(r.Context(), r.PathValue("tenant"), r.PathValue("name"), taskID, follow, tailLines)
	} else {
		rc, err = h.op.StreamLogs(r.Context(), r.PathValue("tenant"), r.PathValue("name"), drvruntime.LogOptions{
			Follow:    follow,
			TailLines: tailLines,
		})
	}
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	serveLogStream(w, r, rc)
}

func serveLogStream(w http.ResponseWriter, r *http.Request, rc io.ReadCloser) {
	var closeOnce sync.Once
	closeRC := func() { closeOnce.Do(func() { _ = rc.Close() }) }
	defer closeRC()
	stop := context.AfterFunc(r.Context(), closeRC)
	defer stop()

	ctrl := http.NewResponseController(w)
	_ = ctrl.SetWriteDeadline(time.Time{})
	_ = ctrl.SetReadDeadline(time.Time{})

	if websocket.IsWebSocketUpgrade(r) {
		serveLogWebSocket(w, r, rc)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	var writer io.Writer = w
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
		writer = &flushWriter{w: w, f: flusher}
	}
	_, _ = io.Copy(writer, rc)
}

func serveLogWebSocket(w http.ResponseWriter, r *http.Request, rc io.ReadCloser) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 0, 16*1024), 512*1024)
	for scanner.Scan() {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, []byte(scanner.Text()+"\n")); err != nil {
			return
		}
	}
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(2*time.Second))
}

func (h *handler) ExecInstance(w http.ResponseWriter, r *http.Request) {
	tenant := r.PathValue("tenant")
	name := r.PathValue("name")

	if websocket.IsWebSocketUpgrade(r) {
		h.execInstanceWebSocket(w, r, tenant, name)
		return
	}
	if r.Method != http.MethodPost {
		apiruntime.WriteError(w, http.StatusMethodNotAllowed, "HTTP exec requires POST or WebSocket upgrade")
		return
	}
	var req corev1.InstanceExec
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var attrs map[string]string
	if len(req.Command) > 0 {
		attrs = map[string]string{"command": strings.Join(req.Command, " ")}
	}
	request.SetAuditTarget(r.Context(), "exec", name, attrs)
	resp, err := h.op.Exec(r.Context(), tenant, name, &req)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, resp)
}

func (h *handler) execInstanceWebSocket(w http.ResponseWriter, r *http.Request, tenant, name string) {
	if r.URL.Query().Get("tty") == "true" {
		h.execInstanceTTYWebSocket(w, r, tenant, name)
		return
	}
	if _, err := h.op.Get(r.Context(), tenant, name); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
	_ = rc.SetReadDeadline(time.Time{})
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	if initCmd := r.URL.Query()["command"]; len(initCmd) > 0 {
		resp, err := h.op.Exec(r.Context(), tenant, name, &corev1.InstanceExec{Command: initCmd})
		if err != nil {
			_ = conn.WriteJSON(map[string]string{"error": err.Error()})
		} else if err := conn.WriteJSON(resp); err != nil {
			return
		}
	}
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var req corev1.InstanceExec
		if err := json.Unmarshal(raw, &req); err != nil || len(req.Command) == 0 {
			fields := strings.Fields(string(raw))
			if len(fields) == 0 {
				continue
			}
			req = corev1.InstanceExec{Command: fields}
		}
		resp, err := h.op.Exec(r.Context(), tenant, name, &req)
		if err != nil {
			if conn.WriteJSON(map[string]string{"error": err.Error()}) != nil {
				return
			}
			continue
		}
		if conn.WriteJSON(resp) != nil {
			return
		}
	}
}

func (h *handler) execInstanceTTYWebSocket(w http.ResponseWriter, r *http.Request, tenant, name string) {
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
	attrs := map[string]string{
		"tty":  "true",
		"cols": strconv.Itoa(int(c)),
		"rows": strconv.Itoa(int(rws)),
	}
	if len(q["command"]) > 0 {
		attrs["command"] = strings.Join(q["command"], " ")
	}
	request.SetAuditTarget(r.Context(), "exec", name, attrs)
	session, err := h.op.OpenExecTTY(r.Context(), tenant, name, drvruntime.InteractiveExecRequest{
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

func (h *handler) FilesInstance(w http.ResponseWriter, r *http.Request) {
	var req corev1.InstanceFiles
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == corev1.FileActionList || action == corev1.FileActionRead || action == corev1.FileActionInspectBackup {
		request.SkipAudit(r.Context())
	} else {
		var attrs map[string]string
		if req.Path != "" {
			attrs = map[string]string{"path": req.Path}
		}
		request.SetAuditTarget(r.Context(), action, r.PathValue("name"), attrs)
	}
	ctx := instancemodel.WithClientHost(r.Context(), extractClientHost(r))
	resp, err := h.op.Files(ctx, r.PathValue("tenant"), r.PathValue("name"), &req)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	apiruntime.WriteJSON(w, http.StatusOK, resp)
}

func extractClientHost(r *http.Request) string {
	for _, raw := range []string{r.Header.Get("Origin"), r.Header.Get("Referer")} {
		if raw = strings.TrimSpace(raw); raw != "" {
			if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
				return u.Hostname()
			}
		}
	}
	if xfh := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); xfh != "" {
		return strings.TrimSpace(strings.Split(xfh, ",")[0])
	}
	return ""
}
