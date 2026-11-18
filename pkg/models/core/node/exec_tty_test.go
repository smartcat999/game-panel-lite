package node

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	drvruntime "github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

func TestResolveNodeTargetHost(t *testing.T) {
	n := &corev1.Node{
		Metadata: metav1.ObjectMeta{Name: "node-1"},
		Status: corev1.NodeStatus{Addresses: []corev1.NodeAddress{
			{Type: corev1.NodeAddressExternalIP, Address: "1.2.3.4"},
			{Type: corev1.NodeAddressInternalIP, Address: "10.0.0.8"},
		}},
	}
	got, err := resolveNodeTargetHost(n, 8082)
	if err != nil || got != "10.0.0.8:8082" {
		t.Fatalf("expected 10.0.0.8:8082, got %q err=%v", got, err)
	}
	n.Status.Addresses = []corev1.NodeAddress{{Type: corev1.NodeAddressExternalIP, Address: "5.6.7.8:9000"}}
	got, err = resolveNodeTargetHost(n, 8082)
	if err != nil || got != "5.6.7.8:9000" {
		t.Fatalf("expected 5.6.7.8:9000, got %q err=%v", got, err)
	}
	if _, err := resolveNodeTargetHost(&corev1.Node{Metadata: metav1.ObjectMeta{Name: "empty"}}, 8082); err == nil {
		t.Fatalf("expected error when node has no addresses")
	}
}

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

func TestDialRemoteNodeExecTTY(t *testing.T) {
	sess := newFakeNodeTTYSession()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	var gotAuth, gotPath, gotTTY string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotTTY = r.URL.Query().Get("tty")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		drvruntime.ServePTYWebSocket(r.Context(), conn, sess)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	op := &operator{agentToken: "tok-123"}
	remoteSess, err := op.dialRemoteNodeExecTTY(context.Background(), u.Host, "node-um773", drvruntime.InteractiveExecRequest{Cols: 120, Rows: 36})
	if err != nil {
		t.Fatalf("dial remote node tty: %v", err)
	}
	defer func() { _ = remoteSess.Close() }()
	if gotAuth != "Bearer tok-123" || gotPath != "/apis/core.gamepanel.io/v1/nodes/node-um773/exec" || gotTTY != "true" {
		t.Fatalf("unexpected remote request headers/path auth=%q path=%q tty=%q", gotAuth, gotPath, gotTTY)
	}
	go func() { _, _ = io.WriteString(sess.outW, "root@um773:~# ") }()
	buf := make([]byte, 64)
	n, err := remoteSess.Read(buf)
	if err != nil || !strings.Contains(string(buf[:n]), "root@um773:~# ") {
		t.Fatalf("unexpected banner read n=%d out=%q err=%v", n, string(buf[:n]), err)
	}
	if _, err := io.WriteString(remoteSess, "uname -n\r"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := remoteSess.Resize(context.Background(), 140, 42); err != nil {
		t.Fatalf("resize: %v", err)
	}
}
