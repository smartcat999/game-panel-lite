package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

func resolvePreStopHook(in *corev1.Instance) string {
	if in == nil {
		return ""
	}
	return strings.TrimSpace(in.Spec.Env[corev1.EnvPreStopHook])
}

func (d *DockerDriver) runPreStopHook(ctx context.Context, containerID, hook string) {
	if d == nil || d.cli == nil || containerID == "" || strings.TrimSpace(hook) == "" {
		return
	}
	hookCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	execCreated, err := d.cli.ContainerExecCreate(hookCtx, containerID, container.ExecOptions{
		Cmd:          []string{"sh", "-c", hook},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return
	}
	hijacked, err := d.cli.ContainerExecAttach(hookCtx, execCreated.ID, container.ExecAttachOptions{})
	if err != nil {
		return
	}
	defer hijacked.Close()
	_, _ = stdcopy.StdCopy(io.Discard, io.Discard, io.LimitReader(hijacked.Reader, 256*1024))
}

func (d *DockerDriver) gracefulStopContainer(ctx context.Context, inspected types.ContainerJSON, in *corev1.Instance, timeoutSec int) error {
	if d == nil || d.cli == nil || inspected.ID == "" || inspected.State == nil {
		return nil
	}
	if !inspected.State.Running && !inspected.State.Restarting {
		return nil
	}
	if hook := resolvePreStopHook(in); hook != "" {
		d.runPreStopHook(ctx, inspected.ID, hook)
	}
	if timeoutSec <= 0 {
		timeoutSec = 15
	}
	return d.cli.ContainerStop(ctx, inspected.ID, container.StopOptions{Timeout: &timeoutSec})
}

func isContainerGamePortReady(in *corev1.Instance, pid int, started time.Time) bool {
	if in == nil || pid <= 0 || resolvePreStopHook(in) == "" {
		return true
	}
	if !started.IsZero() && time.Since(started) >= 90*time.Second {
		return true
	}
	primary, ok := primaryInstancePort(in.Spec.Ports)
	if !ok {
		return true
	}
	return isProcessPortBound(pid, primary)
}

func primaryInstancePort(ports []corev1.InstancePort) (corev1.InstancePort, bool) {
	for _, p := range ports {
		if p.TargetPort > 0 {
			return p, true
		}
	}
	return corev1.InstancePort{}, false
}

func isProcessPortBound(pid int, p corev1.InstancePort) bool {
	proto := strings.ToUpper(strings.TrimSpace(p.Protocol))
	if proto == "" {
		proto = corev1.ProtocolTCP
	}
	tables := []string{"tcp", "tcp6"}
	if proto == corev1.ProtocolUDP {
		tables = []string{"udp", "udp6"}
	}
	readAny := false
	for _, tbl := range tables {
		raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/net/%s", pid, tbl))
		if err != nil {
			continue
		}
		readAny = true
		if hasCandidatePortInProcNet(string(raw), p.TargetPort, proto) {
			return true
		}
	}
	return !readAny
}

func hasCandidatePortInProcNet(content string, targetPort int, proto string) bool {
	if hasBoundPortInProcNet(content, targetPort, proto) {
		return true
	}
	// Steamworks dedicated servers (e.g. Core Keeper) bind the query socket on targetPort+1 UDP.
	if strings.EqualFold(strings.TrimSpace(proto), corev1.ProtocolUDP) && targetPort > 0 && targetPort < 65535 {
		return hasBoundPortInProcNet(content, targetPort+1, proto)
	}
	return false
}

func hasBoundPortInProcNet(content string, targetPort int, proto string) bool {
	if targetPort <= 0 {
		return false
	}
	suffix := fmt.Sprintf(":%04X", targetPort)
	isTCP := !strings.EqualFold(strings.TrimSpace(proto), corev1.ProtocolUDP)
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if !strings.HasSuffix(strings.ToUpper(fields[1]), suffix) {
			continue
		}
		if isTCP && !strings.EqualFold(fields[3], "0A") {
			continue
		}
		return true
	}
	return false
}
