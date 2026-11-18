package agent

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

type dstSaveStubDriver struct {
	probeStubRuntimeDriver
	cavesRunning bool
	mu           sync.Mutex
	pw           *io.PipeWriter
	failNextSave bool
}

func (d *dstSaveStubDriver) Logs(_ context.Context, _ *corev1.Instance, _ runtime.LogOptions) (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	d.mu.Lock()
	d.pw = pw
	d.mu.Unlock()
	return pr, nil
}

func (d *dstSaveStubDriver) Exec(ctx context.Context, inst *corev1.Instance, req runtime.ExecRequest) (string, error) {
	_, _ = d.probeStubRuntimeDriver.Exec(ctx, inst, req)
	if len(req.Command) >= 2 && req.Command[0] == "sh" && d.cavesRunning {
		return "running", nil
	}
	if len(req.Command) == 2 && req.Command[0] == dstConsoleDelivery {
		d.emitStubSaveMarker(req.Command[1])
	}
	return "", nil
}

func (d *dstSaveStubDriver) emitStubSaveMarker(payload string) {
	d.mu.Lock()
	pw := d.pw
	fail := d.failNextSave
	d.mu.Unlock()
	if pw == nil {
		return
	}
	shard := "master"
	lua := payload
	if strings.HasPrefix(payload, dstCavesConsolePrefix) {
		shard = "caves"
		raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(payload, dstCavesConsolePrefix))
		lua = string(raw)
	}
	var idx int
	if fail {
		idx = strings.Index(lua, "GAMEPANEL_SAVE_FAILED_")
	} else {
		idx = strings.LastIndex(lua, "GAMEPANEL_SAVE_")
	}
	if idx < 0 {
		return
	}
	rest := lua[idx:]
	end := strings.IndexAny(rest, `"' `)
	if end > 0 {
		rest = rest[:end]
	}
	if shard == "caves" {
		rest = "[Caves] " + rest
	}
	go func() { _, _ = io.WriteString(pw, rest+"\n") }()
}

func TestIsDSTSaveFlushAndBuildTarget(t *testing.T) {
	inst := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			BackupPolicy: &corev1.InstanceBackupPolicy{
				ConsoleDelivery: dstConsoleDelivery,
			},
		},
	}
	if !isDSTSaveFlush(inst, "c_save()") || isDSTSaveFlush(inst, "save-all") {
		t.Fatalf("unexpected isDSTSaveFlush classification")
	}
	master := buildDSTSaveTarget("master", "n1")
	if master.marker != "GAMEPANEL_SAVE_n1_master" || len(master.argv) != 2 || master.argv[0] != dstConsoleDelivery {
		t.Fatalf("unexpected master target: %+v", master)
	}
	caves := buildDSTSaveTarget("caves", "n1")
	if caves.marker != "GAMEPANEL_SAVE_n1_caves" || !strings.HasPrefix(caves.argv[1], dstCavesConsolePrefix) {
		t.Fatalf("unexpected caves target: %+v", caves)
	}
}

func TestAwaitDSTSaveConfirmations(t *testing.T) {
	targets := []dstSaveTarget{
		buildDSTSaveTarget("master", "abc"),
		buildDSTSaveTarget("caves", "abc"),
	}
	logOK := "unrelated line\nGAMEPANEL_SAVE_abc_master\n[Caves] GAMEPANEL_SAVE_abc_caves\n"
	if err := awaitDSTSaveConfirmations(strings.NewReader(logOK), targets); err != nil {
		t.Fatalf("expected save confirmation success, got %v", err)
	}
	logFail := "GAMEPANEL_SAVE_abc_master\n[Caves] GAMEPANEL_SAVE_FAILED_abc_caves\n"
	if err := awaitDSTSaveConfirmations(strings.NewReader(logFail), targets); err == nil {
		t.Fatalf("expected error on failure marker")
	}
}

func TestConfirmDSTBackupSaveDualShardAndMasterOnly(t *testing.T) {
	drv := &dstSaveStubDriver{cavesRunning: true}
	reg := &runtime.Registry{}
	reg.RegisterDriver(drv)
	ag := &Agent{drivers: reg}
	inst := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			State:        corev1.InstanceStateRunning,
			Env:          map[string]string{"ENABLE_CAVES": "true"},
			BackupPolicy: &corev1.InstanceBackupPolicy{ConsoleDelivery: dstConsoleDelivery, PreBackupCommand: dstSaveCommand},
		},
		Status: corev1.InstanceStatus{
			Phase:           corev1.InstancePhaseRunning,
			ResourceControl: &corev1.InstanceResourceControl{RuntimeClass: &corev1.InstanceRuntimeBinding{UID: "rc-uid", Name: "docker", Handler: "docker"}},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/files", nil)
	req.Header.Set("Accept", "application/x-ndjson")
	ts := newTaskStreamWriter(rec, req)
	ag.flushInstanceBeforeBackup(context.Background(), ts, inst)
	if len(drv.execCalls) != 3 {
		t.Fatalf("expected 3 exec calls (caves probe + master + caves), got %d", len(drv.execCalls))
	}
	if !strings.Contains(rec.Body.String(), "饥荒服务端存档落盘已确认") {
		t.Fatalf("expected confirmation log in NDJSON output, got %s", rec.Body.String())
	}

	drv2 := &dstSaveStubDriver{cavesRunning: true}
	reg2 := &runtime.Registry{}
	reg2.RegisterDriver(drv2)
	ag2 := &Agent{drivers: reg2}
	inst.Spec.Env["ENABLE_CAVES"] = "false"
	ag2.flushInstanceBeforeBackup(context.Background(), nil, inst)
	if len(drv2.execCalls) != 1 || drv2.execCalls[0][0] != dstConsoleDelivery {
		t.Fatalf("expected single master exec when ENABLE_CAVES=false, got %v", drv2.execCalls)
	}
}
