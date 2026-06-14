package runtime

import (
	"context"
	"io"
	"time"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type ContainerSpec struct {
	InstanceID string
	Name       string
	Image      string
	Port       int
	DataDir    string
	ConfigText string
	Options    ContainerOptions
}

type ContainerOptions struct {
	Env        []string
	Cmd        []string
	DataMounts []string
	Files      map[string]string
}

type DockerStatus struct {
	Available     bool      `json:"available"`
	Message       string    `json:"message"`
	Host          string    `json:"host"`
	LastCheckedAt time.Time `json:"lastCheckedAt"`
}

type Adapter interface {
	Check(ctx context.Context) DockerStatus
	Create(ctx context.Context, spec ContainerSpec) (string, error)
	Start(ctx context.Context, instance domain.GameServerInstance) error
	Stop(ctx context.Context, instance domain.GameServerInstance) error
	Restart(ctx context.Context, instance domain.GameServerInstance) error
	Remove(ctx context.Context, instance domain.GameServerInstance) error
	Inspect(ctx context.Context, instance domain.GameServerInstance) (domain.ServerStatus, error)
	Logs(ctx context.Context, instance domain.GameServerInstance) (io.ReadCloser, error)
	SendCommand(ctx context.Context, instance domain.GameServerInstance, command string) error
}
