package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

// Exec sends a non-interactive command execution request to an instance via HTTP POST.
func (c *Client) Exec(ctx context.Context, tenant, name string, cmd []string) (*corev1.InstanceExec, error) {
	if len(cmd) == 0 {
		return nil, errors.New("command cannot be empty")
	}
	if tenant == "" {
		tenant = c.cfg.Tenant
	}
	info, err := ResolveKind("Instance")
	if err != nil {
		return nil, err
	}
	reqBody := corev1.InstanceExec{Command: cmd}
	var result corev1.InstanceExec
	if err := c.executeJSONObject(ctx, http.MethodPost, SubresourcePath(info, tenant, name, "exec"), reqBody, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Files issues a signed direct-to-node file ticket or triggers S3 pre-signed URL streaming backup/restore.
func (c *Client) Files(ctx context.Context, tenant, name string, reqBody *corev1.InstanceFiles) (*corev1.InstanceFiles, error) {
	if tenant == "" {
		tenant = c.cfg.Tenant
	}
	if reqBody == nil {
		reqBody = &corev1.InstanceFiles{Action: "list"}
	}
	info, err := ResolveKind("Instance")
	if err != nil {
		return nil, err
	}
	var result corev1.InstanceFiles
	if err := c.executeJSONObject(ctx, http.MethodPost, SubresourcePath(info, tenant, name, "files"), reqBody, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ExecInteractive establishes an interactive WebSocket session for command execution.
func (c *Client) ExecInteractive(ctx context.Context, tenant, name string, in io.Reader, out io.Writer, cmd []string) error {
	if tenant == "" {
		tenant = c.cfg.Tenant
	}
	u, header, err := c.buildExecWebSocketRequest(tenant, name, cmd)
	if err != nil {
		return err
	}
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, u.String(), header)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		if resp != nil {
			return FromResponse(resp)
		}
		return fmt.Errorf("websocket dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	var writeMu, errMu sync.Mutex
	var lastErr error
	safeWrite := func(messageType int, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(messageType, data)
	}
	setErr := func(e error) { errMu.Lock(); lastErr = e; errMu.Unlock() }
	getErr := func() error { errMu.Lock(); defer errMu.Unlock(); return lastErr }

	done := make(chan struct{})
	go runExecReadLoop(conn, in, out, done, setErr)
	if in != nil {
		go runExecWriteLoop(ctx, in, done, safeWrite)
	}

	select {
	case <-ctx.Done():
		_ = safeWrite(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "context cancelled"))
		_ = conn.Close()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
		}
		return ctx.Err()
	case <-done:
		return getErr()
	}
}

func (c *Client) buildExecWebSocketRequest(tenant, name string, cmd []string) (*url.URL, http.Header, error) {
	wsURLStr := c.baseURL
	if strings.HasPrefix(wsURLStr, "http://") {
		wsURLStr = "ws://" + strings.TrimPrefix(wsURLStr, "http://")
	} else if strings.HasPrefix(wsURLStr, "https://") {
		wsURLStr = "wss://" + strings.TrimPrefix(wsURLStr, "https://")
	}
	info, err := ResolveKind("Instance")
	if err != nil {
		return nil, nil, err
	}
	u, err := url.Parse(wsURLStr + SubresourcePath(info, tenant, name, "exec"))
	if err != nil {
		return nil, nil, fmt.Errorf("parse ws url: %w", err)
	}
	if len(cmd) > 0 {
		q := u.Query()
		for _, arg := range cmd {
			q.Add("command", arg)
		}
		u.RawQuery = q.Encode()
	}
	header := http.Header{}
	if c.cfg.Token != "" {
		header.Set("Authorization", "Bearer "+c.cfg.Token)
	}
	return u, header, nil
}

func runExecReadLoop(conn *websocket.Conn, in io.Reader, out io.Writer, done chan struct{}, setErr func(error)) {
	defer close(done)
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var errPayload struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(msg, &errPayload) == nil && (errPayload.Error != "" || errPayload.Message != "") {
			errMsg := errPayload.Error
			if errMsg == "" {
				errMsg = errPayload.Message
			}
			setErr(errors.New(errMsg))
			if out != nil {
				_, _ = io.WriteString(out, "error: "+errMsg+"\n")
			}
			if in == nil {
				return
			}
			continue
		}
		var execResult corev1.InstanceExec
		if json.Unmarshal(msg, &execResult) == nil {
			if execResult.Output != "" {
				setErr(nil)
				if out != nil {
					_, _ = io.WriteString(out, execResult.Output)
				}
			}
		} else if out != nil {
			_, _ = out.Write(msg)
		}
		if in == nil {
			return
		}
	}
}

func runExecWriteLoop(ctx context.Context, in io.Reader, done <-chan struct{}, safeWrite func(int, []byte) error) {
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		default:
		}
		if safeWrite(websocket.TextMessage, []byte(scanner.Text())) != nil {
			return
		}
	}
	select {
	case <-done:
		return
	case <-ctx.Done():
		return
	default:
	}
	_ = safeWrite(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}
