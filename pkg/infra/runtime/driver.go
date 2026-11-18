package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

// RuntimeObservation reports the observed state of an Instance from its underlying handler.
type RuntimeObservation struct {
	Phase     corev1.InstancePhase
	RuntimeID string
	Endpoint  corev1.InstanceEndpoint
	StartedAt *time.Time
	Reason    string
	Message   string

	// Applied fields are a complete, independently verified execution receipt.
	// Starting/Failed observations and unverified resources must leave them unset.
	// A desired spec or a successful start API call is not execution evidence.
	AppliedResources corev1.ResourceList
	AppliedPorts     []corev1.InstancePort
	ObservedAt       *time.Time
}

type LogOptions struct {
	Follow    bool
	TailLines int
}

type ExecRequest struct {
	Command []string
}

// InteractiveExecRequest configures an interactive PTY exec session inside a container.
type InteractiveExecRequest struct {
	Command []string
	Cols    uint
	Rows    uint
}

// ExecControlMessage encodes JSON control frames exchanged over TTY WebSocket sessions.
type ExecControlMessage struct {
	Type    string `json:"type"`
	Cols    uint   `json:"cols,omitempty"`
	Rows    uint   `json:"rows,omitempty"`
	Message string `json:"message,omitempty"`
}

const (
	HandlerDocker          = "docker"
	ExecStdinMarker        = "__gp_stdin__"
	ConsoleDeliveryStdin   = "stdin"
	ExecControlResize      = "resize"
	ExecControlPing        = "ping"
	ExecControlPong        = "pong"
	ExecControlError       = "error"
	DefaultPTYCols         = 80
	DefaultPTYRows         = 24
	QuiesceStageStopping   = "stopping"
	QuiesceStageReapply    = "reapply"
	QuiesceStageRestarting = "restarting"
)

// PostQuiesceError indicates host maintenance succeeded while a follow-up reapply or restart step failed.
type PostQuiesceError struct {
	Stage string
	Err   error
}

func (e *PostQuiesceError) Error() string {
	if e == nil || e.Err == nil {
		return "post-quiesce maintenance failed"
	}
	return fmt.Sprintf("post-quiesce maintenance (%s): %v", e.Stage, e.Err)
}

func (e *PostQuiesceError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ErrInteractiveUnsupported indicates a runtime driver does not support interactive PTY sessions.
var ErrInteractiveUnsupported = errors.New("interactive PTY execution is not supported by runtime driver")

// InteractiveExecSession represents an attached, resizable full-duplex PTY session.
type InteractiveExecSession interface {
	io.ReadWriteCloser
	Resize(ctx context.Context, cols, rows uint) error
}

// InteractiveExecutor is optionally implemented by drivers supporting interactive PTY sessions.
type InteractiveExecutor interface {
	ExecInteractive(ctx context.Context, instance *corev1.Instance, req InteractiveExecRequest) (InteractiveExecSession, error)
}

// HostInteractiveExecutor is optionally implemented by drivers supporting node-level interactive host PTY sessions.
type HostInteractiveExecutor interface {
	ExecHostInteractive(ctx context.Context, req InteractiveExecRequest) (InteractiveExecSession, error)
}

// NormalizeTerminalSize clamps terminal dimensions into safe PTY bounds with 80x24 defaults.
func NormalizeTerminalSize(cols, rows uint) (uint, uint) {
	if cols == 0 {
		cols = DefaultPTYCols
	} else if cols < 10 {
		cols = 10
	} else if cols > 500 {
		cols = 500
	}
	if rows == 0 {
		rows = DefaultPTYRows
	} else if rows < 5 {
		rows = 5
	} else if rows > 200 {
		rows = 200
	}
	return cols, rows
}

// BuildConsoleCommandArgv builds an ExecRequest.Command slice for a single game console line
// according to the App's consoleDelivery mode ("", "stdin", helper binary, or "{{cmd}}" shell template).
func BuildConsoleCommandArgv(delivery, line string) []string {
	ch := strings.TrimSpace(delivery)
	if ch == "" || ch == ConsoleDeliveryStdin {
		return []string{ExecStdinMarker, line}
	}
	if strings.Contains(ch, "{{cmd}}") {
		safeShell := strings.ReplaceAll(ch, "{{cmd}}", `"$1"`)
		return []string{"sh", "-c", safeShell, "gp-cmd", line}
	}
	return []string{ch, line}
}

// Driver abstracts any workload execution backend handler referenced by a RuntimeClass.spec.handler
// (docker, kubernetes, cloud, memory).
type Driver interface {
	Handler() string
	Version() string
	Validate(instance *corev1.Instance, rc *corev1.RuntimeClass) error
	Reconcile(ctx context.Context, instance *corev1.Instance, rc *corev1.RuntimeClass) (*RuntimeObservation, error)
	Destroy(ctx context.Context, instance *corev1.Instance, rc *corev1.RuntimeClass) error
	Logs(ctx context.Context, instance *corev1.Instance, opts LogOptions) (io.ReadCloser, error)
	Exec(ctx context.Context, instance *corev1.Instance, req ExecRequest) (string, error)
}

// ImagePrewarmer is an optional capability implemented by container runtime drivers
// to pre-pull and cache application images on compute nodes in the background.
type ImagePrewarmer interface {
	PrePullImage(ctx context.Context, imageRef string) (bool, error)
}

// MaintenanceQuiescer temporarily stops a running workload across host-side filesystem
// mutations (such as backup promotion), re-materializes managed files/addons, and restarts if desired.
type MaintenanceQuiescer interface {
	WithQuiescedWorkload(ctx context.Context, instance *corev1.Instance, rc *corev1.RuntimeClass, onStage func(stage string), fn func(hostDir string) error) error
}

// Factory constructs a Driver instance for a node working directory.
type Factory func(workDir string) Driver

var (
	factoriesMu sync.RWMutex
	factories   = make(map[string]Factory)
)

// Register is called by each driver implementation's init() to self-register with the runtime subsystem.
func Register(handler string, factory Factory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[handler] = factory
}

// Registry manages instantiated runtime handlers.
type Registry struct {
	mu      sync.RWMutex
	drivers map[string]Driver
}

// NewDefaultRegistry instantiates all self-registered runtime drivers without hardcoding driver lists externally.
func NewDefaultRegistry(workDir string) *Registry {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()

	r := &Registry{drivers: make(map[string]Driver, len(factories))}
	for _, f := range factories {
		d := f(workDir)
		r.drivers[d.Handler()] = d
	}
	return r
}

func (r *Registry) RegisterDriver(d Driver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.drivers == nil {
		r.drivers = make(map[string]Driver)
	}
	r.drivers[d.Handler()] = d
}

func (r *Registry) Get(handler string) (Driver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.drivers[handler]
	return d, ok
}

func (r *Registry) NodeRuntimes() []corev1.NodeRuntime {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for name := range r.drivers {
		names = append(names, name)
	}
	sort.Strings(names)
	var list []corev1.NodeRuntime
	for _, name := range names {
		d := r.drivers[name]
		list = append(list, corev1.NodeRuntime{
			Handler: d.Handler(),
			Version: d.Version(),
		})
	}
	return list
}
