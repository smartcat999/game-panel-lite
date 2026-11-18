package agent

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	agentOperationTimeout        = 30 * time.Second
	agentRuntimeReconcileTimeout = 15 * time.Minute
	agentTransferTimeout         = 30 * time.Minute
	agentRuntimeWorkers          = 4
)

var errInstanceClosing = errors.New("instance file access is closed for cleanup")

// uidGate serializes execution and file access for an immutable UID. Closing is
// monotonic: a cleanup retry never reopens admission. After process restart the
// current primary object remains the authority for admission.
type uidGate struct {
	mu      sync.Mutex
	closing bool
	token   chan struct{}
}

func (a *Agent) gateForUID(uid string) *uidGate {
	a.gatesMu.Lock()
	defer a.gatesMu.Unlock()
	if a.gates == nil {
		a.gates = make(map[string]*uidGate)
	}
	gate := a.gates[uid]
	if gate == nil {
		gate = &uidGate{token: make(chan struct{}, 1)}
		a.gates[uid] = gate
	}
	return gate
}

func (g *uidGate) closeAdmission() {
	g.mu.Lock()
	g.closing = true
	g.mu.Unlock()
}

func (g *uidGate) acquire(ctx context.Context, cleanup bool) (func(), error) {
	g.mu.Lock()
	closed := g.closing
	g.mu.Unlock()
	if closed && !cleanup {
		return nil, errInstanceClosing
	}
	select {
	case g.token <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	g.mu.Lock()
	closed = g.closing
	g.mu.Unlock()
	if closed && !cleanup {
		<-g.token
		return nil, errInstanceClosing
	}
	return func() { <-g.token }, nil
}

func (a *Agent) operationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := a.operationTimeout
	if timeout <= 0 {
		timeout = agentOperationTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func (a *Agent) runtimeOperationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := a.operationTimeout
	if timeout <= 0 {
		timeout = agentRuntimeReconcileTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

func (a *Agent) acquireRuntimeSlot(ctx context.Context) (func(), error) {
	a.gatesMu.Lock()
	if a.runtimeSem == nil {
		a.runtimeSem = make(chan struct{}, agentRuntimeWorkers)
	}
	sem := a.runtimeSem
	a.gatesMu.Unlock()
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Agent) recordRuntimeHealth(uid string, err error) {
	a.gatesMu.Lock()
	defer a.gatesMu.Unlock()
	if a.runtimeFailures == nil {
		a.runtimeFailures = make(map[string]string)
	}
	if err == nil {
		delete(a.runtimeFailures, uid)
	} else {
		a.runtimeFailures[uid] = err.Error()
	}
}

// executionLease keeps both the UID exclusion and global capacity until a
// backend actually returns, even when a broken implementation ignores ctx. The
// caller can report a deadline without permitting overlapping work or cleanup.
type executionLease struct {
	once     sync.Once
	detached bool
	releases []func()
}

func (l *executionLease) release() {
	l.once.Do(func() {
		for i := len(l.releases) - 1; i >= 0; i-- {
			l.releases[i]()
		}
	})
}

func (l *executionLease) finish() {
	if !l.detached {
		l.release()
	}
}

func runRuntimeOperation[T any](ctx context.Context, lease *executionLease, operation func() (T, error)) (T, error) {
	if err := ctx.Err(); err != nil {
		var zero T
		return zero, err
	}
	type result struct {
		value T
		err   error
	}
	results := make(chan result, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := ctx.Err(); err != nil {
			var zero T
			results <- result{zero, err}
			return
		}
		value, err := operation()
		results <- result{value, err}
	}()
	select {
	case result := <-results:
		if err := ctx.Err(); err != nil {
			var zero T
			return zero, err
		}
		return result.value, result.err
	case <-ctx.Done():
		lease.detached = true
		go func() {
			<-done
			lease.release()
		}()
		var zero T
		return zero, ctx.Err()
	}
}

func (a *Agent) runtimeHealth() (bool, string) {
	a.gatesMu.Lock()
	defer a.gatesMu.Unlock()
	if len(a.runtimeFailures) == 0 {
		return true, "Runtime operations are responding"
	}
	return false, "One or more instance runtime operations failed; heartbeat remains healthy"
}
