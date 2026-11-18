package agent

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/client"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/metrics"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

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

// NewHTTPServer returns a configured http.Handler for the agent server.
func NewHTTPServer(cfg *Config, drivers *runtime.Registry) http.Handler {
	ag := NewAgent(cfg, drivers)
	return ag.Handler()
}

// Handler returns the fully configured http.Handler (including auth middleware).
func (a *Agent) Handler() http.Handler {
	if a.handler == nil {
		a.handler = a.buildHandler()
	}
	return a.handler
}

// ServeHTTP implements http.Handler directly on Agent.
func (a *Agent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.Handler().ServeHTTP(w, r)
}

func (a *Agent) buildHandler() http.Handler {
	mux := http.NewServeMux()

	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.Handle("GET /healthz", healthHandler)
	mux.Handle("GET /livez", healthHandler)
	mux.Handle("GET /metrics", metrics.Handler())

	mux.Handle("GET /apis/core.gamepanel.io/v1/nodes/{name}/exec", a.authMiddleware(http.HandlerFunc(a.handleNodeExecTTYWS)))
	mux.Handle("GET /apis/core.gamepanel.io/v1/tenants/{tenant}/instances/{name}/log", a.authMiddleware(http.HandlerFunc(a.handleLog)))
	mux.Handle("POST /apis/core.gamepanel.io/v1/tenants/{tenant}/instances/{name}/exec", a.authMiddleware(http.HandlerFunc(a.handleExecPost)))
	mux.Handle("GET /apis/core.gamepanel.io/v1/tenants/{tenant}/instances/{name}/exec", a.authMiddleware(http.HandlerFunc(a.handleExecWS)))
	mux.HandleFunc("GET /apis/core.gamepanel.io/v1/tenants/{tenant}/instances/{name}/files", a.handleFiles)
	mux.HandleFunc("PUT /apis/core.gamepanel.io/v1/tenants/{tenant}/instances/{name}/files", a.handleFiles)
	mux.HandleFunc("DELETE /apis/core.gamepanel.io/v1/tenants/{tenant}/instances/{name}/files", a.handleFiles)
	mux.HandleFunc("POST /apis/core.gamepanel.io/v1/tenants/{tenant}/instances/{name}/files", a.handleFiles)
	mux.HandleFunc("OPTIONS /apis/core.gamepanel.io/v1/tenants/{tenant}/instances/{name}/files", a.handleFiles)

	return mux
}

func (a *Agent) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else if qToken := r.URL.Query().Get("token"); qToken != "" {
			token = qToken
		}

		authorized := false
		if token != "" {
			for _, expected := range a.acceptedTokens() {
				if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1 {
					authorized = true
					break
				}
			}
		}
		if !authorized {
			w.Header().Set("WWW-Authenticate", `Bearer realm="node-agent"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *Agent) handleLog(w http.ResponseWriter, r *http.Request) {
	follow := r.URL.Query().Get("follow") == "true"
	ctrl := http.NewResponseController(w)
	if follow {
		_ = ctrl.SetWriteDeadline(time.Time{})
		_ = ctrl.SetReadDeadline(time.Time{})
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), agentTransferTimeout)
		defer cancel()
		r = r.WithContext(ctx)
		deadline, _ := ctx.Deadline()
		_ = ctrl.SetWriteDeadline(deadline)
	}
	resolveCtx, resolveCancel := a.operationContext(r.Context())
	rec, err := a.resolveInstance(resolveCtx, r.PathValue("tenant"), r.PathValue("name"))
	resolveCancel()
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	var tailLines int
	if tl := r.URL.Query().Get("tailLines"); tl != "" {
		tailLines, _ = strconv.Atoi(tl)
	}
	rc, err := rec.driver.Logs(r.Context(), rec.instance, runtime.LogOptions{Follow: follow, TailLines: tailLines})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var closeOnce sync.Once
	closeRC := func() { closeOnce.Do(func() { _ = rc.Close() }) }
	defer closeRC()
	stop := context.AfterFunc(r.Context(), closeRC)
	defer stop()
	if websocket.IsWebSocketUpgrade(r) {
		a.streamLogWebSocket(w, r, rc)
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

func (a *Agent) streamLogWebSocket(w http.ResponseWriter, r *http.Request, rc io.Reader) {
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	stopSocket := context.AfterFunc(r.Context(), func() { _ = conn.Close() })
	defer stopSocket()
	scanner := bufio.NewScanner(rc)
	for scanner.Scan() {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(scanner.Text()+"\n")); err != nil {
			return
		}
	}
	_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

func (a *Agent) handleExecPost(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := a.operationContext(r.Context())
	defer cancel()
	r = r.WithContext(ctx)
	deadline, _ := ctx.Deadline()
	_ = http.NewResponseController(w).SetReadDeadline(deadline)
	_ = http.NewResponseController(w).SetWriteDeadline(deadline)
	tenant := r.PathValue("tenant")
	name := r.PathValue("name")
	rec, err := a.resolveInstance(r.Context(), tenant, name)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}

	if rec.instance != nil && corev1.EffectiveInstanceState(rec.instance) == corev1.InstanceStateStopped {
		http.Error(w, "cannot execute command in stopped instance", http.StatusBadRequest)
		return
	}

	var req corev1.InstanceExec
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Metadata.UID != "" && req.Metadata.UID != rec.instance.Metadata.UID {
		http.Error(w, "instance UID does not match the requested execution target", http.StatusConflict)
		return
	}
	if len(req.Command) == 0 {
		http.Error(w, "command is required", http.StatusBadRequest)
		return
	}

	out, err := a.execResolved(r.Context(), rec, runtime.ExecRequest{Command: req.Command}, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := corev1.InstanceExec{
		TypeMeta:   metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceExec"},
		Metadata:   rec.instance.Metadata,
		Command:    req.Command,
		Output:     out,
		ExecutedAt: time.Now().UTC(),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *Agent) handleExecWS(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("tty") == "true" {
		a.handleExecTTYWS(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), agentTransferTimeout)
	defer cancel()
	r = r.WithContext(ctx)
	rec, err := a.resolveInstance(r.Context(), r.PathValue("tenant"), r.PathValue("name"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	if rec.instance != nil && corev1.EffectiveInstanceState(rec.instance) == corev1.InstanceStateStopped {
		http.Error(w, "cannot execute command in stopped instance", http.StatusBadRequest)
		return
	}
	if !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "WebSocket upgrade required", http.StatusBadRequest)
		return
	}
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	stopSocket := context.AfterFunc(r.Context(), func() { _ = conn.Close() })
	defer stopSocket()
	if initCmd := r.URL.Query()["command"]; len(initCmd) > 0 {
		if !a.writeWSExecResult(r.Context(), conn, rec, initCmd) {
			return
		}
	}
	a.serveWSExecLoop(r.Context(), conn, rec)
}

func (a *Agent) serveWSExecLoop(ctx context.Context, conn *websocket.Conn, rec *localInstance) {
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
		if req.Metadata.UID != "" && req.Metadata.UID != rec.instance.Metadata.UID {
			if err := conn.WriteJSON(map[string]string{"error": "instance UID does not match the requested execution target"}); err != nil {
				return
			}
			continue
		}
		if !a.writeWSExecResult(ctx, conn, rec, req.Command) {
			return
		}
	}
}

func (a *Agent) writeWSExecResult(ctx context.Context, conn *websocket.Conn, rec *localInstance, cmd []string) bool {
	out, err := a.execResolved(ctx, rec, runtime.ExecRequest{Command: cmd}, true)
	if err != nil {
		return conn.WriteJSON(map[string]string{"error": err.Error()}) == nil
	}
	resp := corev1.InstanceExec{
		TypeMeta:   metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "InstanceExec"},
		Metadata:   rec.instance.Metadata,
		Command:    cmd,
		Output:     out,
		ExecutedAt: time.Now().UTC(),
	}
	return conn.WriteJSON(resp) == nil
}

// currentInstanceForAccess always reads the primary API. A cache seed from
// RegisterInstance cannot authorize logs, exec or file access after deletion,
// rebinding or control-plane unavailability.
func (a *Agent) currentInstanceForAccess(ctx context.Context, tenant, name string) (*corev1.Instance, client.Interface, error) {
	ctx, cancel := a.operationContext(ctx)
	defer cancel()
	if strings.TrimSpace(a.cfg.APIServerURL) == "" {
		return nil, nil, os.ErrNotExist
	}
	cli, err := a.apiClient()
	if err != nil {
		return nil, nil, os.ErrNotExist
	}
	inst, err := cli.CoreV1().Instances(tenant).Get(ctx, name)
	if err != nil || inst == nil || inst.Metadata.Tenant != tenant || inst.Metadata.Name != name {
		return nil, nil, os.ErrNotExist
	}
	if inst.Metadata.DeletionTimestamp != nil {
		if inst.Metadata.UID != "" {
			a.gateForUID(inst.Metadata.UID).closeAdmission()
		}
		return nil, nil, os.ErrNotExist
	}
	node, err := cli.CoreV1().Nodes().Get(ctx, a.cfg.NodeName)
	if err != nil || node == nil {
		return nil, nil, os.ErrNotExist
	}
	inst, err = approvedInstance(inst, a.cfg.NodeName, node.Metadata.UID)
	if err != nil {
		return nil, nil, os.ErrNotExist
	}
	return inst, cli, nil
}

func (a *Agent) execResolved(parent context.Context, rec *localInstance, req runtime.ExecRequest, reverify bool) (string, error) {
	ctx, cancel := a.operationContext(parent)
	defer cancel()
	gate := a.gateForUID(rec.instance.Metadata.UID)
	release, err := gate.acquire(ctx, false)
	if err != nil {
		return "", err
	}
	lease := &executionLease{releases: []func(){release}}
	defer lease.finish()
	target := rec
	if reverify {
		current, err := a.resolveInstance(ctx, rec.instance.Metadata.Tenant, rec.instance.Metadata.Name)
		if err != nil || current == nil || !sameExecution(rec.instance, current.instance) {
			return "", os.ErrNotExist
		}
		if corev1.EffectiveInstanceState(current.instance) == corev1.InstanceStateStopped {
			return "", errors.New("cannot execute command in stopped instance")
		}
		target = current
	}
	releaseSlot, err := a.acquireRuntimeSlot(ctx)
	if err != nil {
		return "", err
	}
	lease.releases = append(lease.releases, releaseSlot)
	return runRuntimeOperation(ctx, lease, func() (string, error) {
		return target.driver.Exec(ctx, target.instance, req)
	})
}

func (a *Agent) resolveInstance(ctx context.Context, tenant, name string) (*localInstance, error) {
	inst, _, err := a.currentInstanceForAccess(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	rc, err := frozenRuntimeClass(inst)
	if err != nil {
		return nil, fmt.Errorf("runtime class %s not found", inst.Spec.RuntimeClassName)
	}
	drv, ok := a.drivers.Get(rc.Handler)
	if !ok {
		return nil, fmt.Errorf("driver %s not available", rc.Handler)
	}
	localRec := &localInstance{instance: inst, rc: rc, driver: drv}
	a.cacheInstance(tenant+"/"+name, localRec)
	return localRec, nil
}
