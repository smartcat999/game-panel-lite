package agent

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

const (
	dstConsoleDelivery    = "dst-send-to-console"
	dstSaveCommand        = "c_save()"
	dstCavesConsolePrefix = "__GAMEPANEL_DST_CONSOLE__:caves:"
	dstSaveConfirmTimeout = 30 * time.Second
)

type dstSaveTarget struct {
	name   string
	marker string
	failed string
	argv   []string
}

func isDSTSaveFlush(inst *corev1.Instance, cmd string) bool {
	if inst == nil || inst.Spec.BackupPolicy == nil {
		return false
	}
	return strings.TrimSpace(inst.Spec.BackupPolicy.ConsoleDelivery) == dstConsoleDelivery &&
		strings.TrimSpace(cmd) == dstSaveCommand
}

func (a *Agent) confirmDSTBackupSave(ctx context.Context, ts *taskStreamWriter, drv runtime.Driver, inst *corev1.Instance) {
	nonce := newDSTSaveNonce()
	targets := resolveDSTSaveTargets(ctx, drv, inst, nonce)
	if ts != nil {
		ts.log(corev1.BackupStageArchiving, fmt.Sprintf("正在通知饥荒服务端（%s）执行存档并等待落盘确认...", dstTargetSummary(targets)))
	}
	saveCtx, cancel := context.WithTimeout(ctx, dstSaveConfirmTimeout)
	defer cancel()

	// Subscribe before sending: a fast save must not finish before log capture starts.
	stream, err := drv.Logs(saveCtx, inst, runtime.LogOptions{Follow: true, TailLines: 1})
	if err != nil || stream == nil {
		fallbackDSTSaveExec(ctx, drv, inst)
		return
	}
	defer func() { _ = stream.Close() }()

	done := make(chan error, 1)
	go func() { done <- awaitDSTSaveConfirmations(stream, targets) }()
	if err := sendDSTSaveCommands(saveCtx, drv, inst, targets); err != nil {
		if ts != nil {
			ts.log(corev1.BackupStageArchiving, fmt.Sprintf("发送饥荒保存指令失败，将按当前磁盘状态打包: %v", err))
		}
		return
	}
	reportDSTSaveWaitResult(saveCtx, ts, done)
}

func newDSTSaveNonce() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func dstTargetSummary(targets []dstSaveTarget) string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.name)
	}
	return strings.Join(names, "+")
}

func resolveDSTSaveTargets(ctx context.Context, drv runtime.Driver, inst *corev1.Instance, nonce string) []dstSaveTarget {
	targets := []dstSaveTarget{buildDSTSaveTarget("master", nonce)}
	if !strings.EqualFold(strings.TrimSpace(inst.Spec.Env["ENABLE_CAVES"]), "false") && isDSTCavesShardRunning(ctx, drv, inst) {
		targets = append(targets, buildDSTSaveTarget("caves", nonce))
	}
	return targets
}

func isDSTCavesShardRunning(ctx context.Context, drv runtime.Driver, inst *corev1.Instance) bool {
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := drv.Exec(probeCtx, inst, runtime.ExecRequest{
		Command: []string{"sh", "-c", "test -p /tmp/dst-caves.in && pgrep -f '[s]hard Caves' >/dev/null && echo running || true"},
	})
	return err == nil && strings.Contains(out, "running")
}

func buildDSTSaveTarget(shard, nonce string) dstSaveTarget {
	marker := "GAMEPANEL_SAVE_" + nonce + "_" + shard
	failed := "GAMEPANEL_SAVE_FAILED_" + nonce + "_" + shard
	lua := fmt.Sprintf(
		`if TheWorld ~= nil and TheWorld.ismastersim and TheNet:GetIsServer() and ShardGameIndex ~= nil then local ok = pcall(function() ShardGameIndex:SaveCurrent(function(success) if success == false then print(%q) else print(%q) end end) end); if not ok then print(%q) end else print(%q) end`,
		failed, marker, failed, marker,
	)
	payload := lua
	if shard == "caves" {
		payload = dstCavesConsolePrefix + base64.StdEncoding.EncodeToString([]byte(lua))
	}
	return dstSaveTarget{
		name:   shard,
		marker: marker,
		failed: failed,
		argv:   []string{dstConsoleDelivery, payload},
	}
}

func sendDSTSaveCommands(ctx context.Context, drv runtime.Driver, inst *corev1.Instance, targets []dstSaveTarget) error {
	for _, t := range targets {
		if _, err := drv.Exec(ctx, inst, runtime.ExecRequest{Command: t.argv}); err != nil {
			return fmt.Errorf("%s: %w", t.name, err)
		}
	}
	return nil
}

func awaitDSTSaveConfirmations(stream io.Reader, targets []dstSaveTarget) error {
	pending := make(map[string]dstSaveTarget, len(targets))
	for _, t := range targets {
		pending[t.name] = t
	}
	if len(pending) == 0 {
		return nil
	}
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		for name, t := range pending {
			if strings.Contains(line, t.failed) {
				return fmt.Errorf("shard %s save callback reported failure", name)
			}
			if strings.Contains(line, t.marker) {
				delete(pending, name)
			}
		}
		if len(pending) == 0 {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

func reportDSTSaveWaitResult(ctx context.Context, ts *taskStreamWriter, done <-chan error) {
	select {
	case err := <-done:
		if ts == nil {
			return
		}
		if err != nil {
			ts.log(corev1.BackupStageArchiving, fmt.Sprintf("等待饥荒存档落盘未完整确认（%v），继续打包...", err))
			return
		}
		ts.log(corev1.BackupStageArchiving, "饥荒服务端存档落盘已确认")
	case <-ctx.Done():
		if ts != nil {
			ts.log(corev1.BackupStageArchiving, "等待饥荒存档落盘确认超时，继续打包当前磁盘状态...")
		}
	}
}

func fallbackDSTSaveExec(ctx context.Context, drv runtime.Driver, inst *corev1.Instance) {
	flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	argv := runtime.BuildConsoleCommandArgv(dstConsoleDelivery, dstSaveCommand)
	if _, err := drv.Exec(flushCtx, inst, runtime.ExecRequest{Command: argv}); err == nil {
		time.Sleep(500 * time.Millisecond)
	}
}
