package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/gorilla/websocket"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

type memoryPTYSession struct {
	mu      sync.Mutex
	outR    *io.PipeReader
	outW    *io.PipeWriter
	written bytes.Buffer
	cols    uint
	rows    uint
	closed  chan struct{}
	once    sync.Once
}

func newMemoryPTYSession() *memoryPTYSession {
	pr, pw := io.Pipe()
	return &memoryPTYSession{outR: pr, outW: pw, closed: make(chan struct{})}
}

func (m *memoryPTYSession) Read(p []byte) (int, error) { return m.outR.Read(p) }

func (m *memoryPTYSession) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.written.Write(p)
}

func (m *memoryPTYSession) Resize(_ context.Context, cols, rows uint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cols, m.rows = cols, rows
	return nil
}

func (m *memoryPTYSession) Close() error {
	m.once.Do(func() {
		close(m.closed)
		_ = m.outW.Close()
		_ = m.outR.Close()
	})
	return nil
}

func (m *memoryPTYSession) snapshot() (string, uint, uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.written.String(), m.cols, m.rows
}

func TestServePTYWebSocketBinaryCtrlCAndResize(t *testing.T) {
	sess := newMemoryPTYSession()
	srv := newTestPTYWSServer(t, sess)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatalf("dial pty ws: %v", err)
	}
	defer func() { _ = conn.Close() }()

	go func() { _, _ = io.WriteString(sess.outW, "root@box:/# ") }()
	mt, out, err := conn.ReadMessage()
	if err != nil || mt != websocket.BinaryMessage || string(out) != "root@box:/# " {
		t.Fatalf("unexpected initial PTY output mt=%d out=%q err=%v", mt, string(out), err)
	}
	sendAndVerifyPTYFrames(t, conn, sess)
}

func newTestPTYWSServer(t *testing.T, sess *memoryPTYSession) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ServePTYWebSocket(r.Context(), conn, sess)
	}))
}

func sendAndVerifyPTYFrames(t *testing.T, conn *websocket.Conn, sess *memoryPTYSession) {
	t.Helper()
	literalJSON := `{"type":"resize","cols":1}`
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("pwd\n\x03"+literalJSON)); err != nil {
		t.Fatalf("write binary stdin: %v", err)
	}
	if err := conn.WriteJSON(ExecControlMessage{Type: ExecControlResize, Cols: 132, Rows: 43}); err != nil {
		t.Fatalf("write resize: %v", err)
	}
	if err := conn.WriteJSON(ExecControlMessage{Type: ExecControlPing}); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	mt, pongRaw, err := conn.ReadMessage()
	if err != nil || mt != websocket.TextMessage {
		t.Fatalf("read pong mt=%d err=%v", mt, err)
	}
	var pong ExecControlMessage
	if err := json.Unmarshal(pongRaw, &pong); err != nil || pong.Type != ExecControlPong {
		t.Fatalf("unexpected pong payload: %s (%v)", string(pongRaw), err)
	}
	_ = conn.Close()
	select {
	case <-sess.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("expected PTY session to close on websocket disconnect")
	}
	written, cols, rows := sess.snapshot()
	if !strings.Contains(written, "pwd\n\x03"+literalJSON) || cols != 132 || rows != 43 {
		t.Fatalf("unexpected session state written=%q size=%dx%d", written, cols, rows)
	}
}

func TestHostInteractiveCommand(t *testing.T) {
	if cmd := hostInteractiveCommand(nil); len(cmd) != 3 || cmd[0] != "sh" || !strings.Contains(cmd[2], "nsenter -t 1") {
		t.Fatalf("unexpected default host command: %+v", cmd)
	}
	if cmd := hostInteractiveCommand([]string{"uname", "-a"}); len(cmd) != 6 || cmd[3] != "gp-host-exec" || cmd[4] != "uname" {
		t.Fatalf("unexpected explicit host command: %+v", cmd)
	}
}

type mockHostShellState struct {
	mu       sync.Mutex
	creates  int
	removed  []string
	running  bool
	legacyUp bool
}

func newMockHostShellDriver(t *testing.T, state *mockHostShellState) *DockerDriver {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleMockHostShellRequest(w, r, state)
	}))
	t.Cleanup(srv.Close)
	cli, err := dockerclient.NewClientWithOpts(dockerclient.WithHost(srv.URL), dockerclient.WithHTTPClient(srv.Client()), dockerclient.WithVersion("1.47"))
	if err != nil {
		t.Fatalf("create mock docker client: %v", err)
	}
	return NewDockerDriverWithClient(t.TempDir(), cli)
}

func handleMockHostShellRequest(w http.ResponseWriter, r *http.Request, state *mockHostShellState) {
	w.Header().Set("Content-Type", "application/json")
	state.mu.Lock()
	defer state.mu.Unlock()
	p := r.URL.Path
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/containers/json"):
		if state.legacyUp {
			_ = json.NewEncoder(w).Encode([]map[string]any{{"Id": "legacy-1", "Names": []string{"/gp-node-shell"}}})
			return
		}
		_ = json.NewEncoder(w).Encode([]any{})
	case r.Method == http.MethodDelete && strings.Contains(p, "/containers/legacy-1"):
		state.legacyUp = false
		state.removed = append(state.removed, "legacy-1")
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/containers/"+corev1.DefaultNodeShellContainerName+"/json"):
		if !state.running {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "No such container"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": "shell-cid-1", "State": map[string]any{"Running": true}})
	case r.Method == http.MethodGet && strings.Contains(p, "/images/gamepanel-node-agent:latest/json"):
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": "sha256:agent"})
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/containers/create"):
		state.creates++
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": "shell-cid-1"})
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/containers/shell-cid-1/start"):
		state.running = true
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestEnsureHostShellContainerReusesRunning(t *testing.T) {
	state := &mockHostShellState{legacyUp: true}
	drv := newMockHostShellDriver(t, state)
	ctx := context.Background()

	cid1, err := drv.ensureHostShellContainer(ctx)
	if err != nil || cid1 != "shell-cid-1" {
		t.Fatalf("first ensureHostShellContainer failed: cid=%q err=%v", cid1, err)
	}
	cid2, err := drv.ensureHostShellContainer(ctx)
	if err != nil || cid2 != "shell-cid-1" {
		t.Fatalf("second ensureHostShellContainer failed: cid=%q err=%v", cid2, err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.creates != 1 || len(state.removed) != 1 || state.removed[0] != "legacy-1" {
		t.Fatalf("expected 1 create and 1 legacy removal, got creates=%d removed=%v", state.creates, state.removed)
	}
}
