package instance

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
	instancemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/instance"
)

type fakeTaskLogOperator struct {
	instancemodel.Operator
	payload       string
	stdoutPayload string
	lastTID       string
	stdoutCalled  bool
}

func (f *fakeTaskLogOperator) StreamLogs(_ context.Context, _, _ string, _ ...drvruntime.LogOptions) (io.ReadCloser, error) {
	f.stdoutCalled = true
	return io.NopCloser(strings.NewReader(f.stdoutPayload)), nil
}

func (f *fakeTaskLogOperator) StreamTaskLogs(_ context.Context, _, _, taskID string, _ bool, _ int) (io.ReadCloser, error) {
	f.lastTID = taskID
	return io.NopCloser(strings.NewReader(f.payload)), nil
}

func TestStreamInstanceTaskLogsRoutingAndHTTP(t *testing.T) {
	op := &fakeTaskLogOperator{
		payload:       "[12:00:00] 正在打包存档\n[12:00:02] 上传完成\n",
		stdoutPayload: "[12:00:00] Server thread/INFO: Done!\n",
	}
	h := newHandler(op)

	stdoutReq := httptest.NewRequest(http.MethodGet, "/apis/core.gamepanel.io/v1/tenants/admin/instances/mc/log?tailLines=50", nil)
	stdoutReq.SetPathValue("tenant", "admin")
	stdoutReq.SetPathValue("name", "mc")
	stdoutRec := httptest.NewRecorder()
	h.StreamInstanceLogs(stdoutRec, stdoutReq)
	if stdoutRec.Code != http.StatusOK || !op.stdoutCalled || !strings.Contains(stdoutRec.Body.String(), "Done!") {
		t.Fatalf("expected StreamLogs fallback when taskID absent, code=%d body=%q stdoutCalled=%v", stdoutRec.Code, stdoutRec.Body.String(), op.stdoutCalled)
	}

	okReq := httptest.NewRequest(http.MethodGet, "/apis/core.gamepanel.io/v1/tenants/admin/instances/mc/log?taskID=bak-123&tailLines=50", nil)
	okReq.SetPathValue("tenant", "admin")
	okReq.SetPathValue("name", "mc")
	okRec := httptest.NewRecorder()
	h.StreamInstanceLogs(okRec, okReq)
	if okRec.Code != http.StatusOK || !strings.Contains(okRec.Body.String(), "正在打包存档") || op.lastTID != "bak-123" {
		t.Fatalf("unexpected HTTP tasklog response code=%d body=%q tid=%q", okRec.Code, okRec.Body.String(), op.lastTID)
	}
}

func TestStreamInstanceTaskLogsWebSocket(t *testing.T) {
	op := &fakeTaskLogOperator{payload: "[12:01:00] 定时任务开始\n"}
	h := newHandler(op)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("tenant", "admin")
		r.SetPathValue("name", "mc")
		h.StreamInstanceLogs(w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?taskID=sched-1&follow=true"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatalf("dial tasklog websocket: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, msg, err := conn.ReadMessage()
	if err != nil || !strings.Contains(string(msg), "定时任务开始") {
		t.Fatalf("expected WS log message, got %q err=%v", string(msg), err)
	}
}

type fakeTTYSession struct {
	mu      sync.Mutex
	outR    *io.PipeReader
	outW    *io.PipeWriter
	written bytes.Buffer
	cols    uint
	rows    uint
	closed  chan struct{}
	once    sync.Once
}

func newFakeTTYSession() *fakeTTYSession {
	pr, pw := io.Pipe()
	return &fakeTTYSession{outR: pr, outW: pw, closed: make(chan struct{})}
}

func (f *fakeTTYSession) Read(p []byte) (int, error) { return f.outR.Read(p) }

func (f *fakeTTYSession) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.written.Write(p)
}

func (f *fakeTTYSession) Resize(_ context.Context, cols, rows uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cols, f.rows = cols, rows
	return nil
}

func (f *fakeTTYSession) Close() error {
	f.once.Do(func() {
		close(f.closed)
		_ = f.outW.Close()
		_ = f.outR.Close()
	})
	return nil
}

type fakeTTYOperator struct {
	instancemodel.Operator
	sess *fakeTTYSession
	req  drvruntime.InteractiveExecRequest
}

func (f *fakeTTYOperator) OpenExecTTY(_ context.Context, _, _ string, req drvruntime.InteractiveExecRequest) (drvruntime.InteractiveExecSession, error) {
	f.req = req
	return f.sess, nil
}

func TestExecInstanceTTYWebSocket(t *testing.T) {
	sess := newFakeTTYSession()
	op := &fakeTTYOperator{sess: sess}
	h := newHandler(op)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("tenant", "admin")
		r.SetPathValue("name", "mc")
		h.ExecInstance(w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?tty=true&cols=110&rows=32"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatalf("dial exec tty ws: %v", err)
	}
	go func() { _, _ = io.WriteString(sess.outW, "/data # ") }()
	mt, out, err := conn.ReadMessage()
	if err != nil || mt != websocket.BinaryMessage || string(out) != "/data # " {
		t.Fatalf("unexpected PTY banner mt=%d out=%q err=%v", mt, string(out), err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("ls\r\x03")); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := conn.WriteJSON(drvruntime.ExecControlMessage{Type: drvruntime.ExecControlResize, Cols: 128, Rows: 40}); err != nil {
		t.Fatalf("write resize: %v", err)
	}
	_ = conn.Close()
	select {
	case <-sess.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("expected session close after client disconnect")
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if op.req.Cols != 110 || op.req.Rows != 32 || sess.cols != 128 || sess.rows != 40 || !strings.Contains(sess.written.String(), "ls\r\x03") {
		t.Fatalf("unexpected TTY session req=%+v cols=%d rows=%d stdin=%q", op.req, sess.cols, sess.rows, sess.written.String())
	}
}
