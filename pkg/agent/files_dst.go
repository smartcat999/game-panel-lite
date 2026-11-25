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

type dstSavePlan struct {
	expected []string
	targets  []dstSaveTarget
	missing  []string
}

type dstSaveOutcome struct {
	confirmed map[string]bool
	failed    map[string]bool
	streamErr error
}

func isDSTSaveFlush(inst *corev1.Instance, cmd string) bool {
	if inst == nil || inst.Spec.BackupPolicy == nil {
		return false
	}
	return strings.TrimSpace(inst.Spec.BackupPolicy.ConsoleDelivery) == dstConsoleDelivery &&
		strings.TrimSpace(cmd) == dstSaveCommand
}

func (a *Agent) confirmDSTBackupSave(ctx context.Context, ts *taskStreamWriter, drv runtime.Driver, inst *corev1.Instance) *corev1.InstanceBackupSaveConfirmation {
	nonce := newDSTSaveNonce()
	plan := resolveDSTSavePlan(ctx, drv, inst, nonce)
	if ts != nil {
		ts.log(corev1.BackupStageArchiving, fmt.Sprintf("正在通知饥荒服务端（%s）执行存档并等待落盘确认...", dstTargetSummary(plan.targets)))
	}
	saveCtx, cancel := context.WithTimeout(ctx, dstSaveConfirmTimeout)
	defer cancel()

	// Subscribe before sending: a fast save must not finish before log capture starts.
	stream, err := drv.Logs(saveCtx, inst, runtime.LogOptions{Follow: true, TailLines: 1})
	if err != nil || stream == nil {
		fallbackDSTSaveExec(ctx, drv, inst)
		conf := buildUnconfirmedDSTSave(plan.expected, nil, plan.expected, "无法订阅日志流确认存档落盘（"+dstShardListLabel(plan.expected)+"）")
		logDSTSaveConfirmation(ts, conf)
		return conf
	}
	done := make(chan dstSaveOutcome, 1)
	go func() { done <- awaitDSTSaveConfirmations(stream, plan.targets) }()
	failedTarget, sendErr := sendDSTSaveCommands(saveCtx, drv, inst, plan.targets)
	if sendErr != nil {
		_ = stream.Close()
		msg := fmt.Sprintf("发送保存指令失败（%s）：%v", dstShardLabel(failedTarget), sendErr)
		conf := buildUnconfirmedDSTSave(plan.expected, nil, plan.expected, msg)
		logDSTSaveConfirmation(ts, conf)
		return conf
	}
	conf := waitDSTSaveConfirmation(saveCtx, stream, plan, done)
	logDSTSaveConfirmation(ts, conf)
	return conf
}

func newDSTSaveNonce() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func dstShardLabel(shard string) string {
	switch shard {
	case "master":
		return "地上"
	case "caves":
		return "洞穴"
	default:
		return shard
	}
}

func dstShardListLabel(shards []string) string {
	labels := make([]string, 0, len(shards))
	for _, s := range shards {
		labels = append(labels, dstShardLabel(s))
	}
	return strings.Join(labels, "/")
}

func dstTargetSummary(targets []dstSaveTarget) string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.name)
	}
	return strings.Join(names, "+")
}

func resolveDSTSavePlan(ctx context.Context, drv runtime.Driver, inst *corev1.Instance, nonce string) dstSavePlan {
	plan := dstSavePlan{
		expected: []string{"master"},
		targets:  []dstSaveTarget{buildDSTSaveTarget("master", nonce)},
	}
	if !strings.EqualFold(strings.TrimSpace(inst.Spec.Env["ENABLE_CAVES"]), "false") {
		plan.expected = append(plan.expected, "caves")
		if isDSTCavesShardRunning(ctx, drv, inst) {
			plan.targets = append(plan.targets, buildDSTSaveTarget("caves", nonce))
		} else {
			plan.missing = append(plan.missing, "caves")
		}
	}
	return plan
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
		failed, marker, failed, failed,
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

func sendDSTSaveCommands(ctx context.Context, drv runtime.Driver, inst *corev1.Instance, targets []dstSaveTarget) (string, error) {
	for _, t := range targets {
		if _, err := drv.Exec(ctx, inst, runtime.ExecRequest{Command: t.argv}); err != nil {
			return t.name, err
		}
	}
	return "", nil
}

func awaitDSTSaveConfirmations(stream io.Reader, targets []dstSaveTarget) dstSaveOutcome {
	out := dstSaveOutcome{confirmed: map[string]bool{}, failed: map[string]bool{}}
	pending := make(map[string]dstSaveTarget, len(targets))
	for _, t := range targets {
		pending[t.name] = t
	}
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for len(pending) > 0 && scanner.Scan() {
		line := scanner.Text()
		for name, t := range pending {
			if strings.Contains(line, t.failed) {
				out.failed[name] = true
				delete(pending, name)
			} else if strings.Contains(line, t.marker) {
				out.confirmed[name] = true
				delete(pending, name)
			}
		}
	}
	if len(pending) > 0 {
		out.streamErr = scanner.Err()
		if out.streamErr == nil {
			out.streamErr = io.EOF
		}
	}
	return out
}

func waitDSTSaveConfirmation(ctx context.Context, stream io.Closer, plan dstSavePlan, done <-chan dstSaveOutcome) *corev1.InstanceBackupSaveConfirmation {
	var out dstSaveOutcome
	timedOut := false
	select {
	case out = <-done:
		_ = stream.Close()
	case <-ctx.Done():
		timedOut = true
		_ = stream.Close()
		select {
		case out = <-done:
		case <-time.After(200 * time.Millisecond):
			out = dstSaveOutcome{confirmed: map[string]bool{}, failed: map[string]bool{}}
		}
	}
	return finalizeDSTSaveConfirmation(plan, out, timedOut)
}

func finalizeDSTSaveConfirmation(plan dstSavePlan, out dstSaveOutcome, timedOut bool) *corev1.InstanceBackupSaveConfirmation {
	confirmed, missing, failed, rest := classifyDSTShards(plan, out)
	if len(missing) == 0 && len(failed) == 0 && len(rest) == 0 && len(confirmed) == len(plan.expected) {
		return &corev1.InstanceBackupSaveConfirmation{
			Status:          corev1.BackupSaveConfirmationConfirmed,
			ExpectedShards:  plan.expected,
			ConfirmedShards: confirmed,
		}
	}
	unconfirmed := append(append(append([]string(nil), missing...), failed...), rest...)
	reasons := buildDSTWarningReasons(missing, failed, rest, timedOut, out.streamErr)
	return buildUnconfirmedDSTSave(plan.expected, confirmed, unconfirmed, strings.Join(reasons, "；"))
}

func classifyDSTShards(plan dstSavePlan, out dstSaveOutcome) (confirmed, missing, failed, rest []string) {
	missingSet := make(map[string]bool, len(plan.missing))
	for _, s := range plan.missing {
		missingSet[s] = true
	}
	for _, s := range plan.expected {
		switch {
		case missingSet[s]:
			missing = append(missing, s)
		case out.confirmed[s]:
			confirmed = append(confirmed, s)
		case out.failed[s]:
			failed = append(failed, s)
		default:
			rest = append(rest, s)
		}
	}
	return confirmed, missing, failed, rest
}

func buildDSTWarningReasons(missing, failed, rest []string, timedOut bool, streamErr error) []string {
	var reasons []string
	if len(missing) > 0 {
		reasons = append(reasons, fmt.Sprintf("%s分片未探测到运行中或控制台管道不可用", dstShardListLabel(missing)))
	}
	if len(failed) > 0 {
		reasons = append(reasons, fmt.Sprintf("%s分片保存前置条件未满足或回调失败", dstShardListLabel(failed)))
	}
	if len(rest) > 0 {
		if timedOut {
			reasons = append(reasons, fmt.Sprintf("%s分片等待落盘确认超时", dstShardListLabel(rest)))
		} else if streamErr != nil {
			reasons = append(reasons, fmt.Sprintf("%s分片未确认落盘（%v）", dstShardListLabel(rest), streamErr))
		} else {
			reasons = append(reasons, fmt.Sprintf("%s分片未收到落盘回执", dstShardListLabel(rest)))
		}
	}
	return reasons
}

func buildUnconfirmedDSTSave(expected, confirmed, unconfirmed []string, warning string) *corev1.InstanceBackupSaveConfirmation {
	return &corev1.InstanceBackupSaveConfirmation{
		Status:            corev1.BackupSaveConfirmationUnconfirmed,
		ExpectedShards:    expected,
		ConfirmedShards:   confirmed,
		UnconfirmedShards: unconfirmed,
		Warning:           warning,
	}
}

func logDSTSaveConfirmation(ts *taskStreamWriter, conf *corev1.InstanceBackupSaveConfirmation) {
	if ts == nil || conf == nil {
		return
	}
	if conf.Status == corev1.BackupSaveConfirmationConfirmed {
		ts.log(corev1.BackupStageArchiving, fmt.Sprintf("饥荒服务端存档落盘已确认（%s）", dstShardListLabel(conf.ConfirmedShards)))
		return
	}
	ts.log(corev1.BackupStageArchiving, fmt.Sprintf("等待饥荒存档落盘未完整确认（%s），继续打包当前磁盘状态...", conf.Warning))
}

func fallbackDSTSaveExec(ctx context.Context, drv runtime.Driver, inst *corev1.Instance) {
	flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	argv := runtime.BuildConsoleCommandArgv(dstConsoleDelivery, dstSaveCommand)
	if _, err := drv.Exec(flushCtx, inst, runtime.ExecRequest{Command: argv}); err == nil {
		time.Sleep(500 * time.Millisecond)
	}
}
