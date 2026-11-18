package agent

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

type fakeAgentHostDriver struct {
	runtime.Driver
	sess *fakeAgentTTYSession
	req  runtime.InteractiveExecRequest
}

func (d *fakeAgentHostDriver) Handler() string { return runtime.HandlerDocker }
func (d *fakeAgentHostDriver) Version() string { return "test" }
func (d *fakeAgentHostDriver) ExecHostInteractive(_ context.Context, req runtime.InteractiveExecRequest) (runtime.InteractiveExecSession, error) {
	d.req = req
	return d.sess, nil
}

type fakeAgentTTYSession struct {
	mu      sync.Mutex
	outR    *io.PipeReader
	outW    *io.PipeWriter
	written bytes.Buffer
	once    sync.Once
}

func newFakeAgentTTYSession() *fakeAgentTTYSession {
	r, w := io.Pipe()
	return &fakeAgentTTYSession{outR: r, outW: w}
}

func (f *fakeAgentTTYSession) Read(p []byte) (int, error) { return f.outR.Read(p) }
func (f *fakeAgentTTYSession) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.written.Write(p)
}
func (f *fakeAgentTTYSession) Resize(_ context.Context, _, _ uint) error { return nil }
func (f *fakeAgentTTYSession) Close() error {
	f.once.Do(func() {
		_ = f.outW.Close()
		_ = f.outR.Close()
	})
	return nil
}

func TestAgentHandleNodeExecTTYWS(t *testing.T) {
	sess := newFakeAgentTTYSession()
	drv := &fakeAgentHostDriver{sess: sess}
	reg := &runtime.Registry{}
	reg.RegisterDriver(drv)
	ag := NewAgent(&Config{NodeName: "node-um773", Token: "agent-secret"}, reg)
	srv := httptest.NewServer(ag.Handler())
	defer srv.Close()

	wsBase := "ws" + strings.TrimPrefix(srv.URL, "http")
	hdr := http.Header{"Authorization": []string{"Bearer agent-secret"}}
	_, resp, err := websocket.DefaultDialer.Dial(wsBase+"/apis/"+corev1.GroupVersion+"/nodes/other-node/exec?tty=true", hdr)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err == nil || resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong node name, got resp=%v err=%v", resp, err)
	}

	conn, resp2, err := websocket.DefaultDialer.Dial(wsBase+"/apis/"+corev1.GroupVersion+"/nodes/node-um773/exec?tty=true&cols=100&rows=30", hdr)
	if resp2 != nil && resp2.Body != nil {
		defer func() { _ = resp2.Body.Close() }()
	}
	if err != nil {
		t.Fatalf("dial agent node tty ws: %v", err)
	}
	go func() { _, _ = io.WriteString(sess.outW, "# ") }()
	mt, out, err := conn.ReadMessage()
	if err != nil || mt != websocket.BinaryMessage || string(out) != "# " {
		t.Fatalf("unexpected banner mt=%d out=%q err=%v", mt, string(out), err)
	}
	if drv.req.Cols != 100 || drv.req.Rows != 30 {
		t.Fatalf("unexpected size req=%+v", drv.req)
	}
	_ = conn.Close()
}
