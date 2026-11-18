package node

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
	nodemodel "github.com/smartcat999/game-panel-cloud/pkg/models/core/node"
)

type fakeNodeTTYSession struct {
	mu      sync.Mutex
	outR    *io.PipeReader
	outW    *io.PipeWriter
	written bytes.Buffer
	cols    uint
	rows    uint
	once    sync.Once
}

func newFakeNodeTTYSession() *fakeNodeTTYSession {
	r, w := io.Pipe()
	return &fakeNodeTTYSession{outR: r, outW: w}
}

func (f *fakeNodeTTYSession) Read(p []byte) (int, error) { return f.outR.Read(p) }
func (f *fakeNodeTTYSession) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.written.Write(p)
}
func (f *fakeNodeTTYSession) Resize(_ context.Context, cols, rows uint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cols, f.rows = cols, rows
	return nil
}
func (f *fakeNodeTTYSession) Close() error {
	f.once.Do(func() {
		_ = f.outW.Close()
		_ = f.outR.Close()
	})
	return nil
}

type fakeNodeTTYOperator struct {
	nodemodel.Operator
	name string
	req  drvruntime.InteractiveExecRequest
	sess *fakeNodeTTYSession
}

func (f *fakeNodeTTYOperator) OpenExecTTY(_ context.Context, name string, req drvruntime.InteractiveExecRequest) (drvruntime.InteractiveExecSession, error) {
	f.name = name
	f.req = req
	return f.sess, nil
}

func TestExecNodeRequiresWebSocketAndStreamsPTY(t *testing.T) {
	sess := newFakeNodeTTYSession()
	op := &fakeNodeTTYOperator{sess: sess}
	h := newHandler(op)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/apis/core.gamepanel.io/v1/nodes/node-um773/exec", nil)
	req.SetPathValue("name", "node-um773")
	h.ExecNode(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for non-ws request, got %d", rec.Code)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("name", "node-um773")
		h.ExecNode(w, r)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "?tty=true&cols=112&rows=34"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		t.Fatalf("dial node exec tty ws: %v", err)
	}
	go func() { _, _ = io.WriteString(sess.outW, "root@node-um773:~# ") }()
	mt, out, err := conn.ReadMessage()
	if err != nil || mt != websocket.BinaryMessage || string(out) != "root@node-um773:~# " {
		t.Fatalf("unexpected PTY banner mt=%d out=%q err=%v", mt, string(out), err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("uptime\r")); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := conn.WriteJSON(drvruntime.ExecControlMessage{Type: drvruntime.ExecControlResize, Cols: 132, Rows: 44}); err != nil {
		t.Fatalf("write resize: %v", err)
	}
	_ = conn.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		sess.mu.Lock()
		gotInput := sess.written.String()
		gotCols, gotRows := sess.cols, sess.rows
		sess.mu.Unlock()
		if gotInput == "uptime\r" && gotCols == 132 && gotRows == 44 {
			if op.name != "node-um773" || op.req.Cols != 112 || op.req.Rows != 34 {
				t.Fatalf("unexpected operator args name=%q req=%+v", op.name, op.req)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for node PTY input/resize")
}
