package network

import (
	"fmt"
	"strings"
	"sync"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/config"
)

const (
	DefaultPortRangeMin  int32 = 20000
	DefaultPortRangeMax  int32 = 29999
	MaxDynamicPortProbes int   = 256
)

// Config is an alias for config.NetworkConfig managed centrally in pkg/config.
type Config = config.NetworkConfig

func NewConfig() *Config {
	return config.NewNetworkConfig()
}

// PortRange defines the bounded dynamic host-port pool range and probe cap.
type PortRange struct {
	Min       int32
	Max       int32
	MaxProbes int
}

// DefaultPortRange returns the active dynamic port pool range from config.Get().Network.
func DefaultPortRange() PortRange {
	if cfg := config.Get(); cfg != nil {
		pr := PortRange{
			Min:       cfg.Network.PortRangeMin,
			Max:       cfg.Network.PortRangeMax,
			MaxProbes: cfg.Network.MaxPortProbes,
		}
		if pr.Validate() == nil {
			return pr
		}
	}
	return PortRange{Min: DefaultPortRangeMin, Max: DefaultPortRangeMax, MaxProbes: MaxDynamicPortProbes}
}

func (p PortRange) Validate() error {
	if p.Min < 1 || p.Max > 65535 || p.Min > p.Max || p.MaxProbes < 1 || p.MaxProbes > MaxDynamicPortProbes {
		return &corev1.ValidationError{Field: "portRange", Reason: corev1.ValidationReasonInvalidPortRange}
	}
	return nil
}

func (p PortRange) Width() int32 {
	return p.Max - p.Min + 1
}

func (p PortRange) Contains(port int32) bool {
	return port >= p.Min && port <= p.Max
}

// ClaimFunc attempts to atomically claim a concrete host port for req inside the node store.
type ClaimFunc func(req corev1.PortRequest, port int32) (bool, error)

// Driver abstracts cluster network port-pool allocation and execution port verification
// (modeled after Kubernetes ServiceNodePortRange & CNI portmap plugins).
type Driver interface {
	Handler() string
	PortRange() PortRange
	AssignPorts(attemptID string, requests []corev1.PortRequest, claim ClaimFunc) ([]corev1.PortRequest, error)
	ValidateExecutionPorts(requested, approved []corev1.InstancePort) error
}

// Factory constructs a network Driver from a NetworkConfig.
type Factory func(cfg *Config) (Driver, error)

var (
	factoriesMu   sync.RWMutex
	factories     = make(map[string]Factory)
	defaultMu     sync.RWMutex
	defaultDriver Driver
)

// Register is called by each network driver's init() to self-register.
func Register(handler string, factory Factory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[strings.ToLower(strings.TrimSpace(handler))] = factory
}

// Open instantiates the configured network driver and sets it as default if none is set.
func Open(cfg *Config) (Driver, error) {
	if cfg == nil {
		cfg = &config.Get().Network
	}
	cfg.Default()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	name := strings.ToLower(strings.TrimSpace(cfg.Driver))
	factoriesMu.RLock()
	f, ok := factories[name]
	factoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown network driver %q", cfg.Driver)
	}
	d, err := f(cfg)
	if err != nil {
		return nil, err
	}
	defaultMu.Lock()
	if defaultDriver == nil {
		defaultDriver = d
	}
	defaultMu.Unlock()
	return d, nil
}

// Default returns the active network Driver (falling back to the default "hostport" driver).
func Default() Driver {
	defaultMu.RLock()
	d := defaultDriver
	defaultMu.RUnlock()
	if d != nil {
		return d
	}
	drv, err := NewHostPortDriver(&config.Get().Network)
	if err != nil {
		drv, _ = NewHostPortDriver(NewConfig())
	}
	return drv
}

// SetDefault overrides the process-wide network Driver.
func SetDefault(d Driver) {
	if d == nil {
		return
	}
	defaultMu.Lock()
	defaultDriver = d
	defaultMu.Unlock()
}
