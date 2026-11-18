package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const ptyWriteTimeout = 10 * time.Second

type wsSafeWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *wsSafeWriter) writeMessage(msgType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(ptyWriteTimeout))
	return w.conn.WriteMessage(msgType, data)
}

func (w *wsSafeWriter) writeJSON(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return w.writeMessage(websocket.TextMessage, raw)
}

func (w *wsSafeWriter) writeControl(msgType int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteControl(msgType, data, time.Now().Add(ptyWriteTimeout))
}

func (w *wsSafeWriter) closeNormal() {
	_ = w.writeControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = w.conn.Close()
}

// ServePTYWebSocket bridges a WebSocket connection with an InteractiveExecSession.
// Binary frames carry raw PTY bytes; text frames carry JSON control messages (resize/ping/error).
func ServePTYWebSocket(ctx context.Context, conn *websocket.Conn, session InteractiveExecSession) {
	sessCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() { _ = session.Close() }()

	sw := &wsSafeWriter{conn: conn}
	conn.SetPingHandler(func(appData string) error {
		return sw.writeControl(websocket.PongMessage, []byte(appData))
	})
	stop := context.AfterFunc(sessCtx, func() {
		_ = session.Close()
		_ = conn.Close()
	})
	defer stop()

	readDone := make(chan struct{})
	go pipePTYOutput(sessCtx, session, sw, readDone)

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if err := dispatchPTYClientFrame(sessCtx, msgType, data, session, sw); err != nil {
			break
		}
	}
	cancel()
	_ = session.Close()
	_ = conn.Close()
	<-readDone
}

func pipePTYOutput(ctx context.Context, session InteractiveExecSession, sw *wsSafeWriter, done chan struct{}) {
	defer close(done)
	buf := make([]byte, 32*1024)
	for {
		n, err := session.Read(buf)
		if n > 0 {
			if wErr := sw.writeMessage(websocket.BinaryMessage, buf[:n]); wErr != nil {
				return
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				_ = sw.writeJSON(ExecControlMessage{Type: ExecControlError, Message: err.Error()})
			}
			sw.closeNormal()
			return
		}
	}
}

func dispatchPTYClientFrame(ctx context.Context, msgType int, data []byte, session InteractiveExecSession, sw *wsSafeWriter) error {
	if msgType == websocket.BinaryMessage {
		_, err := session.Write(data)
		return err
	}
	if msgType != websocket.TextMessage {
		return nil
	}
	var ctrl ExecControlMessage
	if err := json.Unmarshal(data, &ctrl); err == nil && ctrl.Type != "" {
		switch ctrl.Type {
		case ExecControlResize:
			rCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			_ = session.Resize(rCtx, ctrl.Cols, ctrl.Rows)
			return nil
		case ExecControlPing:
			_ = sw.writeJSON(ExecControlMessage{Type: ExecControlPong})
			return nil
		}
	}
	_, err := session.Write(data)
	return err
}
