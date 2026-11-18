package volume

import (
	"fmt"
	"os"
	"strings"
	"sync"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

// Driver abstracts instance workspace volume provisioning and os.Root-isolated
// filesystem access (modeled after Kubernetes pkg/volume & CSI plugins).
type Driver interface {
	Handler() string
	ResolveWorkDir(uid, tenant, name string, create bool) (string, error)
	OpenRoot(uid, tenant, name string, create bool) (*os.Root, string, error)
	RemoveWorkDir(uid, tenant, name string) error
	WriteFiles(hostDir string, files []corev1.InstanceFile) error
}

// Factory constructs a volume Driver bound to a node's base workDir.
type Factory func(workDir string) Driver

var (
	factoriesMu sync.RWMutex
	factories   = make(map[string]Factory)
)

// Register is called by each volume driver's init() to self-register.
func Register(handler string, factory Factory) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	factories[strings.ToLower(strings.TrimSpace(handler))] = factory
}

// NewDriver instantiates the registered volume driver for handler and workDir.
func NewDriver(handler, workDir string) (Driver, error) {
	name := strings.ToLower(strings.TrimSpace(handler))
	if name == "" {
		name = DefaultHostPathDriverName
	}
	factoriesMu.RLock()
	f, ok := factories[name]
	factoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown volume driver %q", handler)
	}
	return f(workDir), nil
}

// Default returns the default "hostpath" volume Driver for workDir.
func Default(workDir string) Driver {
	d, err := NewDriver(DefaultHostPathDriverName, workDir)
	if err != nil {
		return NewHostPathDriver(workDir)
	}
	return d
}
