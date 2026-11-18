package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

// OpenExecTTY opens an interactive PTY shell session targeting a compute Node via its local runtime or node-agent.
func (o *operator) OpenExecTTY(ctx context.Context, name string, req drvruntime.InteractiveExecRequest) (drvruntime.InteractiveExecSession, error) {
	if o.db == nil {
		return nil, fmt.Errorf("%w: node store is not configured", models.ErrBadRequest)
	}
	existing, err := o.db.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing.Metadata.DeletionTimestamp != nil {
		return nil, fmt.Errorf("%w: terminating node %s cannot open terminal", models.ErrConflict, name)
	}
	if o.drivers != nil && o.localNodeName != "" && name == o.localNodeName {
		drv, ok := o.drivers.Get(drvruntime.HandlerDocker)
		if !ok || drv == nil {
			return nil, fmt.Errorf("%w: docker runtime driver is not registered", models.ErrBadRequest)
		}
		hostExec, ok := drv.(drvruntime.HostInteractiveExecutor)
		if !ok {
			return nil, fmt.Errorf("%w: %v", models.ErrBadRequest, drvruntime.ErrInteractiveUnsupported)
		}
		return hostExec.ExecHostInteractive(ctx, req)
	}
	targetHost, err := resolveNodeTargetHost(existing, o.agentPort)
	if err != nil {
		return nil, err
	}
	return o.dialRemoteNodeExecTTY(ctx, targetHost, name, req)
}

func resolveNodeTargetHost(node *corev1.Node, port int) (string, error) {
	if node == nil {
		return "", fmt.Errorf("%w: node is required", models.ErrBadRequest)
	}
	var ip string
	for _, want := range []string{corev1.NodeAddressInternalIP, corev1.NodeAddressExternalIP} {
		for _, addr := range node.Status.Addresses {
			if addr.Type == want && strings.TrimSpace(addr.Address) != "" {
				ip = strings.TrimSpace(addr.Address)
				break
			}
		}
		if ip != "" {
			break
		}
	}
	if ip == "" {
		return "", fmt.Errorf("%w: node %q has no reachable address reported", models.ErrBadRequest, node.Metadata.Name)
	}
	if _, _, err := net.SplitHostPort(ip); err == nil {
		return ip, nil
	}
	return net.JoinHostPort(ip, strconv.Itoa(port)), nil
}

func (o *operator) dialRemoteNodeExecTTY(ctx context.Context, targetHost, name string, req drvruntime.InteractiveExecRequest) (drvruntime.InteractiveExecSession, error) {
	sess, err := o.dialRemoteNodeExecTTYHost(ctx, targetHost, name, req, remoteExecTTYHandshakeTimeout)
	if err == nil || ctx.Err() != nil {
		return sess, err
	}
	port := o.agentPort
	if port <= 0 {
		port = defaultAgentListenPort
	}
	fallbackHost := net.JoinHostPort("node-agent", strconv.Itoa(port))
	if fallbackHost == targetHost {
		return nil, err
	}
	if fallbackSess, fallbackErr := o.dialRemoteNodeExecTTYHost(ctx, fallbackHost, name, req, 5*time.Second); fallbackErr == nil {
		return fallbackSess, nil
	}
	return nil, err
}

func (o *operator) dialRemoteNodeExecTTYHost(ctx context.Context, targetHost, name string, req drvruntime.InteractiveExecRequest, timeout time.Duration) (drvruntime.InteractiveExecSession, error) {
	cols, rows := drvruntime.NormalizeTerminalSize(req.Cols, req.Rows)
	q := url.Values{}
	q.Set("tty", "true")
	q.Set("cols", strconv.FormatUint(uint64(cols), 10))
	q.Set("rows", strconv.FormatUint(uint64(rows), 10))
	for _, arg := range req.Command {
		q.Add("command", arg)
	}
	wsURL := fmt.Sprintf("ws://%s/apis/%s/nodes/%s/exec?%s", targetHost, corev1.GroupVersion, url.PathEscape(name), q.Encode())
	hdr := http.Header{}
	if tok := o.resolveAgentToken(ctx, name); tok != "" {
		hdr.Set("Authorization", "Bearer "+tok)
	}
	dialer := websocket.Dialer{HandshakeTimeout: timeout}
	dialCtx, cancelDial := context.WithTimeout(ctx, timeout)
	defer cancelDial()
	conn, resp, err := dialer.DialContext(dialCtx, wsURL, hdr)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		if resp != nil {
			return nil, mapRemoteNodeExecError(resp, targetHost, name)
		}
		return nil, fmt.Errorf("%w: failed to connect to node-agent on %s: %v", models.ErrBadGateway, targetHost, err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	return newNodeRemoteWSExecSession(conn), nil
}

func mapRemoteNodeExecError(resp *http.Response, targetHost, name string) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	msg := strings.TrimSpace(string(raw))
	switch resp.StatusCode {
	case http.StatusBadRequest, http.StatusNotImplemented:
		return fmt.Errorf("%w: %s", models.ErrBadRequest, msg)
	case http.StatusUnauthorized:
		return fmt.Errorf("%w: node-agent authentication failed on %s: %s", models.ErrUnauthorized, targetHost, msg)
	case http.StatusNotFound:
		return fmt.Errorf("%w: node %q not served by node-agent on %s: %s", models.ErrNotFound, name, targetHost, msg)
	default:
		return fmt.Errorf("%w: node-agent exec returned HTTP %d: %s", models.ErrBadGateway, resp.StatusCode, msg)
	}
}

type nodeRemoteWSExecSession struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	curReader io.Reader
	closeOnce sync.Once
	stopKeep  context.CancelFunc
}

func newNodeRemoteWSExecSession(conn *websocket.Conn) *nodeRemoteWSExecSession {
	ctx, cancel := context.WithCancel(context.Background())
	s := &nodeRemoteWSExecSession{conn: conn, stopKeep: cancel}
	conn.SetPingHandler(func(appData string) error {
		return s.writeControl(websocket.PongMessage, []byte(appData))
	})
	go s.runKeepAlive(ctx)
	return s
}

func (s *nodeRemoteWSExecSession) runKeepAlive(ctx context.Context) {
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

func (s *nodeRemoteWSExecSession) writeControl(msgType int, data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteControl(msgType, data, time.Now().Add(remoteExecTTYWriteTimeout))
}

func (s *nodeRemoteWSExecSession) Read(p []byte) (int, error) {
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
				if ctrlErr := readNodeControlError(r); ctrlErr != nil {
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

func readNodeControlError(r io.Reader) error {
	raw, _ := io.ReadAll(io.LimitReader(r, 8192))
	var ctrl drvruntime.ExecControlMessage
	if json.Unmarshal(raw, &ctrl) == nil && ctrl.Type == drvruntime.ExecControlError && ctrl.Message != "" {
		return errors.New(ctrl.Message)
	}
	return nil
}

func (s *nodeRemoteWSExecSession) Write(p []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.conn.SetWriteDeadline(time.Now().Add(remoteExecTTYWriteTimeout))
	if err := s.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *nodeRemoteWSExecSession) Resize(_ context.Context, cols, rows uint) error {
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

func (s *nodeRemoteWSExecSession) Close() error {
	s.closeOnce.Do(func() {
		s.stopKeep()
		_ = s.writeControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		_ = s.conn.Close()
	})
	return nil
}
