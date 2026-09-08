package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestMaintenanceBoundedRetryAndShutdown(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o := options{batch: 7, timeout: 20 * time.Millisecond, poll: 10 * time.Millisecond}
	calls := 0
	var failedAt time.Time
	err := maintain(ctx, o, func(task context.Context, batch int) (int, error) {
		calls++
		if batch != 7 {
			t.Fatal("batch limit changed")
		}
		if _, ok := task.Deadline(); !ok {
			t.Fatal("unbounded database call")
		}
		if calls == 1 {
			<-task.Done()
			failedAt = time.Now()
			return 0, errors.New("private DSN")
		}
		if time.Since(failedAt) < o.poll {
			t.Fatal("failure retried without backoff")
		}
		cancel()
		return 1, nil
	}, logger)
	if err != nil || calls != 2 || strings.Contains(logs.String(), "private DSN") {
		t.Fatalf("loop: %d %v %s", calls, err, logs.String())
	}
}
func TestMaintenanceOnceAndInvalidOptions(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	o := options{batch: 1, timeout: time.Second, poll: time.Second, once: true}
	calls := 0
	if err := maintain(context.Background(), o, func(context.Context, int) (int, error) { calls++; return 1, nil }, logger); err != nil || calls != 1 {
		t.Fatal("one-shot did not finish")
	}
	if err := maintain(context.Background(), o, func(context.Context, int) (int, error) { return 0, errors.New("secret") }, logger); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("one-shot failure lost or leaked")
	}
	o.batch = 201
	if err := maintain(context.Background(), o, func(context.Context, int) (int, error) { t.Fatal("invalid config reached storage"); return 0, nil }, logger); err == nil {
		t.Fatal("invalid config accepted")
	}
}
