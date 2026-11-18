package artifact

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
)

var logWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(_ *http.Request) bool { return true },
}

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

func (h *handler) AppendArtifactLogs(w http.ResponseWriter, r *http.Request) {
	var chunk registryv1.ArtifactLogChunk
	if err := json.NewDecoder(r.Body).Decode(&chunk); err != nil {
		apiruntime.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.op.AppendLogs(r.Context(), r.PathValue("name"), &chunk); err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) StreamArtifactLogs(w http.ResponseWriter, r *http.Request) {
	follow := r.URL.Query().Get("follow") == "true"
	var tailLines int
	if raw := r.URL.Query().Get("tailLines"); raw != "" {
		tailLines, _ = strconv.Atoi(raw)
	}
	rc, err := h.op.StreamLogs(r.Context(), r.PathValue("name"), follow, tailLines)
	if err != nil {
		apiruntime.WriteModelError(w, err)
		return
	}
	var closeOnce sync.Once
	closeRC := func() { closeOnce.Do(func() { _ = rc.Close() }) }
	defer closeRC()
	stop := context.AfterFunc(r.Context(), closeRC)
	defer stop()

	clearStreamDeadlines(w)
	if websocket.IsWebSocketUpgrade(r) {
		serveLogWebSocket(w, r, rc)
		return
	}
	serveLogHTTP(w, rc)
}

func clearStreamDeadlines(w http.ResponseWriter) {
	ctrl := http.NewResponseController(w)
	_ = ctrl.SetWriteDeadline(time.Time{})
	_ = ctrl.SetReadDeadline(time.Time{})
}

func serveLogWebSocket(w http.ResponseWriter, r *http.Request, rc io.ReadCloser) {
	conn, err := logWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 0, 16*1024), 512*1024)
	for scanner.Scan() {
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, []byte(scanner.Text()+"\n")); err != nil {
			return
		}
	}
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(2*time.Second))
}

func serveLogHTTP(w http.ResponseWriter, rc io.ReadCloser) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	var writer io.Writer = w
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
		writer = &flushWriter{w: w, f: flusher}
	}
	_, _ = io.Copy(writer, rc)
}
