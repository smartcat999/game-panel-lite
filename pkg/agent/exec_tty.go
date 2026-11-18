package agent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

func parseInteractiveExecQuery(r *http.Request) runtime.InteractiveExecRequest {
	q := r.URL.Query()
	cols, _ := strconv.Atoi(q.Get("cols"))
	rows, _ := strconv.Atoi(q.Get("rows"))
	if cols < 0 {
		cols = 0
	}
	if rows < 0 {
		rows = 0
	}
	c, rws := runtime.NormalizeTerminalSize(uint(cols), uint(rows))
	return runtime.InteractiveExecRequest{
		Command: q["command"],
		Cols:    c,
		Rows:    rws,
	}
}

func (a *Agent) handleExecTTYWS(w http.ResponseWriter, r *http.Request) {
	if !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "WebSocket upgrade required", http.StatusBadRequest)
		return
	}
	req := parseInteractiveExecQuery(r)
	session, status, err := a.openInteractiveSession(r.Context(), r.PathValue("tenant"), r.PathValue("name"), r.URL.Query().Get("uid"), req)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	a.serveInteractivePTYWS(w, r, session)
}

func (a *Agent) handleNodeExecTTYWS(w http.ResponseWriter, r *http.Request) {
	if !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "WebSocket upgrade required", http.StatusBadRequest)
		return
	}
	req := parseInteractiveExecQuery(r)
	session, status, err := a.openNodeInteractiveSession(r.Context(), r.PathValue("name"), req)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	a.serveInteractivePTYWS(w, r, session)
}

func (a *Agent) serveInteractivePTYWS(w http.ResponseWriter, r *http.Request, session runtime.InteractiveExecSession) {
	defer func() { _ = session.Close() }()
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Time{})
	runtime.ServePTYWebSocket(r.Context(), conn, session)
}

func (a *Agent) openNodeInteractiveSession(
	parent context.Context,
	nodeName string,
	req runtime.InteractiveExecRequest,
) (runtime.InteractiveExecSession, int, error) {
	want := strings.TrimSpace(nodeName)
	local := strings.TrimSpace(a.cfg.NodeName)
	if want == "" || want != local {
		return nil, http.StatusNotFound, errors.New("node " + nodeName + " is not served by agent " + local)
	}
	if a.drivers == nil {
		return nil, http.StatusNotImplemented, runtime.ErrInteractiveUnsupported
	}
	drv, ok := a.drivers.Get(runtime.HandlerDocker)
	if !ok || drv == nil {
		return nil, http.StatusNotImplemented, runtime.ErrInteractiveUnsupported
	}
	hostExec, ok := drv.(runtime.HostInteractiveExecutor)
	if !ok {
		return nil, http.StatusNotImplemented, runtime.ErrInteractiveUnsupported
	}
	ctx, cancel := a.operationContext(parent)
	defer cancel()
	releaseSlot, err := a.acquireRuntimeSlot(ctx)
	if err != nil {
		return nil, http.StatusServiceUnavailable, err
	}
	defer releaseSlot()
	sess, err := hostExec.ExecHostInteractive(ctx, req)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}
	return sess, http.StatusOK, nil
}

func (a *Agent) openInteractiveSession(
	parent context.Context,
	tenant, name, wantUID string,
	req runtime.InteractiveExecRequest,
) (runtime.InteractiveExecSession, int, error) {
	ctx, cancel := a.operationContext(parent)
	defer cancel()
	rec, err := a.resolveInstance(ctx, tenant, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, http.StatusNotFound, err
		}
		return nil, http.StatusBadRequest, err
	}
	if rec.instance != nil && corev1.EffectiveInstanceState(rec.instance) == corev1.InstanceStateStopped {
		return nil, http.StatusBadRequest, errors.New("cannot execute command in stopped instance")
	}
	if wantUID != "" && rec.instance != nil && wantUID != rec.instance.Metadata.UID {
		return nil, http.StatusConflict, errors.New("instance UID does not match the requested execution target")
	}
	sess, err := a.startResolvedInteractiveSession(ctx, rec, req)
	if err != nil {
		if errors.Is(err, runtime.ErrInteractiveUnsupported) {
			return nil, http.StatusNotImplemented, err
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, http.StatusNotFound, err
		}
		return nil, http.StatusBadRequest, err
	}
	return sess, http.StatusOK, nil
}

func (a *Agent) startResolvedInteractiveSession(
	ctx context.Context,
	rec *localInstance,
	req runtime.InteractiveExecRequest,
) (runtime.InteractiveExecSession, error) {
	release, err := a.gateForUID(rec.instance.Metadata.UID).acquire(ctx, false)
	if err != nil {
		return nil, err
	}
	lease := &executionLease{releases: []func(){release}}
	defer lease.finish()

	current, err := a.resolveInstance(ctx, rec.instance.Metadata.Tenant, rec.instance.Metadata.Name)
	if err != nil || current == nil || !sameExecution(rec.instance, current.instance) {
		return nil, os.ErrNotExist
	}
	if corev1.EffectiveInstanceState(current.instance) == corev1.InstanceStateStopped {
		return nil, errors.New("cannot execute command in stopped instance")
	}
	ie, ok := current.driver.(runtime.InteractiveExecutor)
	if !ok {
		return nil, runtime.ErrInteractiveUnsupported
	}
	releaseSlot, err := a.acquireRuntimeSlot(ctx)
	if err != nil {
		return nil, err
	}
	lease.releases = append(lease.releases, releaseSlot)
	return runRuntimeOperation(ctx, lease, func() (runtime.InteractiveExecSession, error) {
		return ie.ExecInteractive(ctx, current.instance, req)
	})
}
