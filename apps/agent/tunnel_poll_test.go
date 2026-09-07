package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestTunnelPollBacksOffWhenNoStreamAndCancels(t *testing.T) {
	for _, status := range []int{200, 204, 401, 404, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			requests := make(chan struct{}, 16)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case requests <- struct{}{}:
				default:
				}
				w.WriteHeader(status)
				if status != 204 {
					io.WriteString(w, `{"error":"no stream"}`)
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				startTunnelLoop(ctx, AgentConfig{MasterURL: server.URL, Token: "token"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			}()
			select {
			case <-requests:
			case <-time.After(time.Second):
				t.Fatal("poll never started")
			}
			select {
			case <-requests:
				t.Error("empty/rejected response triggered immediate retry")
			case <-time.After(100 * time.Millisecond):
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("retry delay ignored cancellation")
			}
		})
	}
}
