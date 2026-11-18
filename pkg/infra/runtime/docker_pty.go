package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

const (
	interactiveShellScript     = `export TERM=${TERM:-xterm-256color}; if [ -x /bin/bash ]; then exec /bin/bash -il; elif [ -x /usr/bin/bash ]; then exec /usr/bin/bash -il; else exec /bin/sh -i; fi`
	hostInteractiveShellScript = `export TERM=${TERM:-xterm-256color} COLORTERM=truecolor; if command -v nsenter >/dev/null 2>&1 && nsenter -t 1 -m -u -i -n -p true >/dev/null 2>&1; then exec nsenter -t 1 -m -u -i -n -p -- sh -c 'cd /root 2>/dev/null || cd /; if [ -x /bin/bash ]; then exec /bin/bash -il; elif [ -x /usr/bin/bash ]; then exec /usr/bin/bash -il; else exec /bin/sh -i; fi'; fi; cd /root 2>/dev/null || cd /; if [ -x /bin/bash ]; then exec /bin/bash -il; elif [ -x /usr/bin/bash ]; then exec /usr/bin/bash -il; else exec /bin/sh -i; fi`
)

func interactiveCommand(cmd []string) []string {
	if len(cmd) > 0 {
		return cmd
	}
	return []string{"sh", "-c", interactiveShellScript}
}

func hostInteractiveCommand(cmd []string) []string {
	if len(cmd) > 0 {
		return append([]string{"sh", "-c", `if command -v nsenter >/dev/null 2>&1 && nsenter -t 1 -m -u -i -n -p true >/dev/null 2>&1; then exec nsenter -t 1 -m -u -i -n -p -- "$@"; fi; exec "$@"`, "gp-host-exec"}, cmd...)
	}
	return []string{"sh", "-c", hostInteractiveShellScript}
}

// ExecInteractive opens an attached Docker PTY exec session for full-duplex shell interaction.
func (d *DockerDriver) ExecInteractive(ctx context.Context, in *corev1.Instance, req InteractiveExecRequest) (InteractiveExecSession, error) {
	if err := d.ensureClient(); err != nil {
		return nil, err
	}
	release, err := beginDockerExecution(ctx, d.workDir, in)
	if err != nil {
		return nil, err
	}
	defer release()

	inspected, err := d.inspectInstanceContainer(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("inspect container for interactive exec: %w", err)
	}
	if err := verifyExactContainerBinding(inspected, in); err != nil {
		return nil, err
	}
	cols, rows := NormalizeTerminalSize(req.Cols, req.Rows)
	return d.attachInteractiveExec(ctx, inspected.ID, interactiveCommand(req.Command), cols, rows)
}

func (d *DockerDriver) attachInteractiveExec(ctx context.Context, containerID string, cmd []string, cols, rows uint) (InteractiveExecSession, error) {
	size := [2]uint{rows, cols}
	execCreated, err := d.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		ConsoleSize:  &size,
		Env:          []string{"TERM=xterm-256color", "COLORTERM=truecolor", "LANG=C.UTF-8"},
	})
	if err != nil {
		return nil, fmt.Errorf("create interactive exec: %w", err)
	}
	hijacked, err := d.cli.ContainerExecAttach(ctx, execCreated.ID, container.ExecStartOptions{
		Tty:         true,
		ConsoleSize: &size,
	})
	if err != nil {
		return nil, fmt.Errorf("attach interactive exec: %w", err)
	}
	_ = d.cli.ContainerExecResize(ctx, execCreated.ID, container.ResizeOptions{Height: rows, Width: cols})
	return &dockerPTYSession{cli: d.cli, execID: execCreated.ID, hijacked: hijacked}, nil
}

type dockerPTYSession struct {
	cli       *client.Client
	execID    string
	hijacked  types.HijackedResponse
	closeOnce sync.Once
}

func (s *dockerPTYSession) Read(p []byte) (int, error) {
	return s.hijacked.Reader.Read(p)
}

func (s *dockerPTYSession) Write(p []byte) (int, error) {
	return s.hijacked.Conn.Write(p)
}

func (s *dockerPTYSession) Resize(ctx context.Context, cols, rows uint) error {
	c, r := NormalizeTerminalSize(cols, rows)
	return s.cli.ContainerExecResize(ctx, s.execID, container.ResizeOptions{Height: r, Width: c})
}

func (s *dockerPTYSession) Close() error {
	s.closeOnce.Do(func() {
		s.hijacked.Close()
	})
	return nil
}

// ExecHostInteractive opens an interactive PTY shell entering the host namespaces via a persistent reusable privileged container.
func (d *DockerDriver) ExecHostInteractive(ctx context.Context, req InteractiveExecRequest) (InteractiveExecSession, error) {
	if err := d.ensureClient(); err != nil {
		return nil, err
	}
	cid, err := d.ensureHostShellContainer(ctx)
	if err != nil {
		return nil, err
	}
	cols, rows := NormalizeTerminalSize(req.Cols, req.Rows)
	cmd := hostInteractiveCommand(req.Command)
	sess, err := d.attachInteractiveExec(ctx, cid, cmd, cols, rows)
	if err != nil {
		// Retry once in case the container stopped or was removed between inspect and exec attach.
		cid, ensureErr := d.ensureHostShellContainer(ctx)
		if ensureErr != nil {
			return nil, err
		}
		return d.attachInteractiveExec(ctx, cid, cmd, cols, rows)
	}
	return sess, nil
}

func (d *DockerDriver) ensureHostShellContainer(ctx context.Context) (string, error) {
	d.hostShellMu.Lock()
	defer d.hostShellMu.Unlock()

	d.reapLegacyHostShellContainers(ctx)
	inspected, err := d.cli.ContainerInspect(ctx, corev1.DefaultNodeShellContainerName)
	if err == nil {
		if inspected.State != nil && inspected.State.Running {
			return inspected.ID, nil
		}
		if sErr := d.cli.ContainerStart(ctx, inspected.ID, container.StartOptions{}); sErr == nil {
			return inspected.ID, nil
		}
		_ = d.cli.ContainerRemove(ctx, inspected.ID, container.RemoveOptions{Force: true})
	} else if !errdefs.IsNotFound(err) {
		return "", fmt.Errorf("inspect host shell container: %w", err)
	}
	img, err := d.resolveHostShellImage(ctx)
	if err != nil {
		return "", err
	}
	return d.createAndStartHostShellContainer(ctx, img)
}

func (d *DockerDriver) reapLegacyHostShellContainers(ctx context.Context) {
	reapCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	list, err := d.cli.ContainerList(reapCtx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", corev1.LabelRuntimeNodeShell+"=true")),
	})
	if err != nil {
		return
	}
	wantName := "/" + corev1.DefaultNodeShellContainerName
	for _, c := range list {
		if !slices.Contains(c.Names, wantName) {
			_ = d.cli.ContainerRemove(reapCtx, c.ID, container.RemoveOptions{Force: true})
		}
	}
}

func isShellCapableImageTag(tag string) bool {
	low := strings.ToLower(strings.TrimSpace(tag))
	if low == "" || low == "<none>:<none>" {
		return false
	}
	for _, kw := range []string{"gamepanel", "alpine", "debian", "ubuntu", "busybox"} {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

func (d *DockerDriver) resolveHostShellImage(ctx context.Context) (string, error) {
	candidates := []string{
		"registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/gamepanel-lite-node-agent:v2.0.0",
		"registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/gamepanel-lite-node-agent:latest",
		"gamepanel-lite-node-agent:latest",
		"gamepanel-node-agent:latest",
		"registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/alpine:3.22",
		"alpine:3.22",
		"alpine:latest",
		corev1.DefaultAppRuntimeImage,
		"gamepanel/runtime:latest",
	}
	for _, ref := range candidates {
		if _, _, err := d.cli.ImageInspectWithRaw(ctx, ref); err == nil {
			return ref, nil
		}
	}
	imgs, err := d.cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list local docker images for node shell: %w", err)
	}
	for _, img := range imgs {
		for _, tag := range img.RepoTags {
			if isShellCapableImageTag(tag) {
				return tag, nil
			}
		}
	}
	const fallbackAlpine = "registry.cn-hangzhou.aliyuncs.com/gamepanel-lite/alpine:3.22"
	_, _ = d.ensureImage(ctx, fallbackAlpine)
	if _, _, err := d.cli.ImageInspectWithRaw(ctx, fallbackAlpine); err == nil {
		return fallbackAlpine, nil
	}
	return "", errors.New("no shell-capable docker image available to launch node shell")
}

func (d *DockerDriver) createAndStartHostShellContainer(ctx context.Context, img string) (string, error) {
	cfg := &container.Config{
		Image:      img,
		Entrypoint: []string{"sh", "-c"},
		Cmd:        []string{"trap '[ -n \"$pid\" ] && kill \"$pid\" 2>/dev/null; exit 0' TERM INT; while :; do sleep 86400 & pid=$!; wait \"$pid\"; pid=''; done"},
		Labels:     map[string]string{corev1.LabelRuntimeNodeShell: "true"},
	}
	hostCfg := &container.HostConfig{
		AutoRemove:    false,
		Privileged:    true,
		PidMode:       "host",
		NetworkMode:   "host",
		IpcMode:       "host",
		UTSMode:       "host",
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyUnlessStopped},
	}
	created, err := d.cli.ContainerCreate(ctx, cfg, hostCfg, nil, nil, corev1.DefaultNodeShellContainerName)
	if err != nil {
		return "", fmt.Errorf("create node shell container: %w", err)
	}
	if err := d.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		rmCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = d.cli.ContainerRemove(rmCtx, created.ID, container.RemoveOptions{Force: true})
		cancel()
		return "", fmt.Errorf("start node shell container: %w", err)
	}
	return created.ID, nil
}
