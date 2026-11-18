package instance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
)

const (
	remoteExecTTYHandshakeTimeout = 35 * time.Second
	remoteExecTTYPingInterval     = 25 * time.Second
	remoteExecTTYWriteTimeout     = 10 * time.Second
)

// OpenExecTTY opens an interactive PTY exec session against the target runtime driver or node-agent.
func (o *operator) OpenExecTTY(ctx context.Context, tenant, name string, req drvruntime.InteractiveExecRequest) (drvruntime.InteractiveExecSession, error) {
	existing, err := o.db.Get(ctx, tenant, name)
	if err != nil {
		return nil, err
	}
	if existing.Metadata.DeletionTimestamp != nil {
		return nil, fmt.Errorf("%w: terminating instance cannot execute commands", models.ErrConflict)
	}
	if corev1.EffectiveInstanceState(existing) == corev1.InstanceStateStopped {
		return nil, fmt.Errorf("%w: cannot execute command in stopped instance %s/%s", models.ErrBadRequest, tenant, name)
	}
	targetHost, isLocal, err := o.resolveTargetNode(ctx, existing)
	if err != nil {
		return nil, err
	}
	if isLocal {
		drv, err := o.driverForInstance(ctx, existing)
		if err != nil {
			return nil, err
		}
		ie, ok := drv.(drvruntime.InteractiveExecutor)
		if !ok {
			return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, drvruntime.ErrInteractiveUnsupported)
		}
		return ie.ExecInteractive(ctx, existing, req)
	}
	return o.dialRemoteExecTTY(ctx, targetHost, existing.Spec.NodeName, tenant, name, existing.Metadata.UID, req)
}

func (o *operator) dialRemoteExecTTY(ctx context.Context, targetHost, nodeName, tenant, name, uid string, req drvruntime.InteractiveExecRequest) (drvruntime.InteractiveExecSession, error) {
	cols, rows := drvruntime.NormalizeTerminalSize(req.Cols, req.Rows)
	q := url.Values{}
	q.Set("tty", "true")
	q.Set("cols", strconv.FormatUint(uint64(cols), 10))
	q.Set("rows", strconv.FormatUint(uint64(rows), 10))
	if uid != "" {
		q.Set("uid", uid)
	}
	for _, arg := range req.Command {
		q.Add("command", arg)
	}
	wsURL := "ws://" + targetHost + instanceSubresourcePath(tenant, name, "exec") + "?" + q.Encode()
	hdr := http.Header{}
	if tok := o.resolveNodeAgentToken(ctx, nodeName); tok != "" {
		hdr.Set("Authorization", "Bearer "+tok)
	}
	dialer := websocket.Dialer{HandshakeTimeout: remoteExecTTYHandshakeTimeout}
	dialCtx, cancelDial := context.WithTimeout(ctx, remoteExecTTYHandshakeTimeout)
	defer cancelDial()
	conn, resp, err := dialer.DialContext(dialCtx, wsURL, hdr)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		if resp != nil {
			return nil, mapRemoteExecTTYError(resp, targetHost, tenant, name)
		}
		return nil, fmt.Errorf("%w: failed to connect to node-agent on %s: %v", models.ErrBadGateway, targetHost, err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	return newRemoteWSExecSession(conn), nil
}

func mapRemoteExecTTYError(resp *http.Response, targetHost, tenant, name string) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(raw))
	switch resp.StatusCode {
	case http.StatusBadRequest, http.StatusNotImplemented:
		return fmt.Errorf("%w: %s", models.ErrBadRequest, msg)
	case http.StatusConflict:
		return fmt.Errorf("%w: %s", models.ErrConflict, msg)
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: node-agent authentication failed on %s: %s", models.ErrUnauthorized, targetHost, msg)
	case http.StatusNotFound:
		return fmt.Errorf("%w: instance %s/%s not found on node %s: %s", models.ErrNotFound, tenant, name, targetHost, msg)
	default:
		return fmt.Errorf("%w: node-agent exec returned HTTP %d: %s", models.ErrBadGateway, resp.StatusCode, msg)
	}
}

type remoteWSExecSession struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	curReader io.Reader
	closeOnce sync.Once
	stopKeep  context.CancelFunc
}

func newRemoteWSExecSession(conn *websocket.Conn) *remoteWSExecSession {
	ctx, cancel := context.WithCancel(context.Background())
	s := &remoteWSExecSession{conn: conn, stopKeep: cancel}
	conn.SetPingHandler(func(appData string) error {
		return s.writeControl(websocket.PongMessage, []byte(appData))
	})
	go s.runKeepAlive(ctx)
	return s
}

func (s *remoteWSExecSession) runKeepAlive(ctx context.Context) {
	ticker := time.NewTicker(remoteExecTTYPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.writeControl(websocket.PingMessage, []byte("keepalive")); err != nil {
				return
			}
		}
	}
}

func (s *remoteWSExecSession) writeControl(msgType int, data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteControl(msgType, data, time.Now().Add(remoteExecTTYWriteTimeout))
}

func (s *remoteWSExecSession) Read(p []byte) (int, error) {
	for {
		if s.curReader == nil {
			mt, r, err := s.conn.NextReader()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					return 0, io.EOF
				}
				return 0, err
			}
			if mt == websocket.TextMessage {
				if ctrlErr := readControlError(r); ctrlErr != nil {
					return 0, ctrlErr
				}
				continue
			}
			if mt != websocket.BinaryMessage {
				_, _ = io.Copy(io.Discard, io.LimitReader(r, 8192))
				continue
			}
			s.curReader = r
		}
		n, err := s.curReader.Read(p)
		if errors.Is(err, io.EOF) {
			s.curReader = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func readControlError(r io.Reader) error {
	raw, _ := io.ReadAll(io.LimitReader(r, 8192))
	var ctrl drvruntime.ExecControlMessage
	if json.Unmarshal(raw, &ctrl) == nil && ctrl.Type == drvruntime.ExecControlError && ctrl.Message != "" {
		return errors.New(ctrl.Message)
	}
	return nil
}

func (s *remoteWSExecSession) Write(p []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(remoteExecTTYWriteTimeout))
	if err := s.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *remoteWSExecSession) Resize(_ context.Context, cols, rows uint) error {
	c, r := drvruntime.NormalizeTerminalSize(cols, rows)
	payload, err := json.Marshal(drvruntime.ExecControlMessage{
		Type: drvruntime.ExecControlResize,
		Cols: c,
		Rows: r,
	})
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(remoteExecTTYWriteTimeout))
	return s.conn.WriteMessage(websocket.TextMessage, payload)
}

func (s *remoteWSExecSession) Close() error {
	s.closeOnce.Do(func() {
		s.stopKeep()
		_ = s.writeControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		_ = s.conn.Close()
	})
	return nil
}
