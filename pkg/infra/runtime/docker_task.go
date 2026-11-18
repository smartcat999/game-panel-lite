package runtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// TaskRunSpec describes a one-off, auto-removed batch container (e.g. steamcmd package builds).
type TaskRunSpec struct {
	Name       string
	Image      string
	User       string // optional uid[:gid]
	Entrypoint []string
	Cmd        []string
	Env        []string
	Binds      []string // host:container[:ro]
	Labels     map[string]string
}

// TaskRunner runs one-off batch containers outside of Instance lifecycle management.
type TaskRunner interface {
	RunTask(ctx context.Context, spec TaskRunSpec, onLine func(string)) (int64, error)
}

var _ TaskRunner = (*DockerDriver)(nil)

// RunTask creates, starts and streams a one-off container, returning its exit code. The container is
// always removed, including when ctx is cancelled.
func (d *DockerDriver) RunTask(ctx context.Context, spec TaskRunSpec, onLine func(string)) (int64, error) {
	if err := d.ensureClient(); err != nil {
		return -1, err
	}
	if _, err := d.ensureImage(ctx, spec.Image); err != nil {
		return -1, fmt.Errorf("pull task image %s: %w", spec.Image, err)
	}
	created, err := d.cli.ContainerCreate(ctx, &container.Config{
		Image: spec.Image, User: spec.User, Entrypoint: spec.Entrypoint, Cmd: spec.Cmd, Env: spec.Env, Labels: spec.Labels,
	}, &container.HostConfig{Binds: spec.Binds}, nil, nil, spec.Name)
	if err != nil {
		return -1, fmt.Errorf("create task container: %w", err)
	}
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = d.cli.ContainerRemove(rmCtx, created.ID, container.RemoveOptions{Force: true})
	}()
	if err := d.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return -1, fmt.Errorf("start task container: %w", err)
	}
	d.streamTaskLogs(ctx, created.ID, onLine)
	return d.waitTask(ctx, created.ID)
}

func (d *DockerDriver) streamTaskLogs(ctx context.Context, id string, onLine func(string)) {
	if onLine == nil {
		return
	}
	rc, err := d.cli.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true, Follow: true})
	if err != nil {
		return
	}
	pr, pw := io.Pipe()
	go func() {
		defer rc.Close()
		_, copyErr := stdcopy.StdCopy(pw, pw, rc)
		_ = pw.CloseWithError(copyErr)
	}()
	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		onLine(scanner.Text())
	}
}

func (d *DockerDriver) waitTask(ctx context.Context, id string) (int64, error) {
	statusCh, errCh := d.cli.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	select {
	case st := <-statusCh:
		if st.Error != nil && st.Error.Message != "" {
			return st.StatusCode, errors.New(st.Error.Message)
		}
		return st.StatusCode, nil
	case err := <-errCh:
		return -1, fmt.Errorf("wait task container: %w", err)
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}
