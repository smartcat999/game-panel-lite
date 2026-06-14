package runtime

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type testAdapter struct {
	status DockerStatus
}

func (a testAdapter) Check(context.Context) DockerStatus { return a.status }
func (a testAdapter) Create(context.Context, ContainerSpec) (string, error) {
	return "", nil
}
func (a testAdapter) Start(context.Context, domain.GameServerInstance) error   { return nil }
func (a testAdapter) Stop(context.Context, domain.GameServerInstance) error    { return nil }
func (a testAdapter) Restart(context.Context, domain.GameServerInstance) error { return nil }
func (a testAdapter) Remove(context.Context, domain.GameServerInstance) error  { return nil }
func (a testAdapter) Inspect(context.Context, domain.GameServerInstance) (domain.ServerStatus, error) {
	return domain.StatusStopped, nil
}
func (a testAdapter) Logs(context.Context, domain.GameServerInstance) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (a testAdapter) SendCommand(context.Context, domain.GameServerInstance, string) error {
	return nil
}

func TestSwitchableAdapterSetChangesDelegatedRuntime(t *testing.T) {
	switchable := NewSwitchableAdapter(testAdapter{
		status: DockerStatus{Available: false, Message: "first", Host: "unix:///first.sock"},
	})

	if got := switchable.Check(context.Background()); got.Host != "unix:///first.sock" {
		t.Fatalf("expected first host, got %q", got.Host)
	}

	switchable.Set(testAdapter{
		status: DockerStatus{Available: true, Message: "second", Host: "unix:///second.sock"},
	})

	got := switchable.Check(context.Background())
	if !got.Available || got.Host != "unix:///second.sock" {
		t.Fatalf("expected switched available host, got %+v", got)
	}
}
