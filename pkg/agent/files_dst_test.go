package agent

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

type dstSaveStubDriver struct {
	probeStubRuntimeDriver
	cavesRunning bool
	mu           sync.Mutex
	pw           *io.PipeWriter
	failShards   map[string]bool
	silentShards map[string]bool
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
	fail := d.failShards
	silent := d.silentShards
	d.mu.Unlock()
	if pw == nil {
		return
	}
	shard, lua := decodeDSTPayload(payload)
	if silent[shard] {
		return
	}
	token := extractToken(lua, fail[shard])
	if token == "" {
		return
	}
	if shard == "caves" {
		token = "[Caves] " + token
	}
	go func() { _, _ = io.WriteString(pw, token+"\n") }()
}

func decodeDSTPayload(payload string) (string, string) {
	if strings.HasPrefix(payload, dstCavesConsolePrefix) {
		raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(payload, dstCavesConsolePrefix))
		return "caves", string(raw)
	}
	return "master", payload
}

func extractToken(lua string, wantFailed bool) string {
	prefix := "GAMEPANEL_SAVE_"
	if wantFailed {
		prefix = "GAMEPANEL_SAVE_FAILED_"
	}
	for offset := 0; offset < len(lua); {
		rel := strings.Index(lua[offset:], prefix)
		if rel < 0 {
			return ""
		}
		rest := lua[offset+rel:]
		if !wantFailed && strings.HasPrefix(rest, "GAMEPANEL_SAVE_FAILED_") {
			offset += rel + len("GAMEPANEL_SAVE_FAILED_")
			continue
		}
		if end := strings.IndexAny(rest, `"' `); end > 0 {
			return rest[:end]
		}
		return rest
	}
	return ""
}

func newDSTTestAgent(drv runtime.Driver) *Agent {
	reg := &runtime.Registry{}
	reg.RegisterDriver(drv)
	return &Agent{drivers: reg}
}

func newDSTTestInstance(enableCaves string) *corev1.Instance {
	return &corev1.Instance{
		Spec: corev1.InstanceSpec{
			State:        corev1.InstanceStateRunning,
			Env:          map[string]string{"ENABLE_CAVES": enableCaves},
			BackupPolicy: &corev1.InstanceBackupPolicy{ConsoleDelivery: dstConsoleDelivery, PreBackupCommand: dstSaveCommand},
		},
		Status: corev1.InstanceStatus{
			Phase:           corev1.InstancePhaseRunning,
			ResourceControl: &corev1.InstanceResourceControl{RuntimeClass: &corev1.InstanceRuntimeBinding{UID: "rc-uid", Name: "docker", Handler: "docker"}},
		},
	}
}

func TestIsDSTSaveFlushAndLuaElsePrintsFailed(t *testing.T) {
	inst := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			BackupPolicy: &corev1.InstanceBackupPolicy{ConsoleDelivery: dstConsoleDelivery},
		},
	}
	if !isDSTSaveFlush(inst, "c_save()") || isDSTSaveFlush(inst, "save-all") {
		t.Fatalf("unexpected isDSTSaveFlush classification")
	}
	master := buildDSTSaveTarget("master", "n1")
	if strings.Count(master.argv[1], master.marker) != 1 || strings.Count(master.argv[1], master.failed) != 3 {
		t.Fatalf("expected 1 success marker and 3 failed markers in lua else/failure branches: %s", master.argv[1])
	}
	caves := buildDSTSaveTarget("caves", "n1")
	if !strings.HasPrefix(caves.argv[1], dstCavesConsolePrefix) {
		t.Fatalf("unexpected caves prefix: %+v", caves)
	}
}

func TestAwaitDSTSaveConfirmations(t *testing.T) {
	targets := []dstSaveTarget{buildDSTSaveTarget("master", "abc"), buildDSTSaveTarget("caves", "abc")}
	okOut := awaitDSTSaveConfirmations(strings.NewReader("noise\nGAMEPANEL_SAVE_abc_master\n[Caves] GAMEPANEL_SAVE_abc_caves\n"), targets)
	if !okOut.confirmed["master"] || !okOut.confirmed["caves"] || len(okOut.failed) != 0 {
		t.Fatalf("expected dual confirmed, got %+v", okOut)
	}
	failOut := awaitDSTSaveConfirmations(strings.NewReader("GAMEPANEL_SAVE_abc_master\n[Caves] GAMEPANEL_SAVE_FAILED_abc_caves\n"), targets)
	if !failOut.confirmed["master"] || !failOut.failed["caves"] {
		t.Fatalf("expected master confirmed and caves failed, got %+v", failOut)
	}
}

func TestConfirmDSTBackupSaveDualShardAndDisabledCaves(t *testing.T) {
	drv := &dstSaveStubDriver{cavesRunning: true}
	ag := newDSTTestAgent(drv)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/files", nil)
	req.Header.Set("Accept", "application/x-ndjson")
	ts := newTaskStreamWriter(rec, req)

	conf := ag.flushInstanceBeforeBackup(context.Background(), ts, newDSTTestInstance("true"))
	if conf == nil || conf.Status != corev1.BackupSaveConfirmationConfirmed || !reflect.DeepEqual(conf.ConfirmedShards, []string{"master", "caves"}) {
		t.Fatalf("expected dual confirmed, got %+v", conf)
	}
	if len(drv.execCalls) != 3 || !strings.Contains(rec.Body.String(), "饥荒服务端存档落盘已确认") {
		t.Fatalf("unexpected execCalls=%d body=%s", len(drv.execCalls), rec.Body.String())
	}

	ag2 := newDSTTestAgent(&dstSaveStubDriver{cavesRunning: true})
	conf2 := ag2.flushInstanceBeforeBackup(context.Background(), nil, newDSTTestInstance("false"))
	if conf2 == nil || conf2.Status != corev1.BackupSaveConfirmationConfirmed || !reflect.DeepEqual(conf2.ExpectedShards, []string{"master"}) {
		t.Fatalf("expected master-only confirmed when caves disabled, got %+v", conf2)
	}
}

func TestConfirmDSTBackupSaveUnconfirmedBranches(t *testing.T) {
	agMissing := newDSTTestAgent(&dstSaveStubDriver{cavesRunning: false})
	confMissing := agMissing.flushInstanceBeforeBackup(context.Background(), nil, newDSTTestInstance("true"))
	if confMissing == nil || confMissing.Status != corev1.BackupSaveConfirmationUnconfirmed ||
		!reflect.DeepEqual(confMissing.UnconfirmedShards, []string{"caves"}) || !strings.Contains(confMissing.Warning, "洞穴") {
		t.Fatalf("expected caves missing warning, got %+v", confMissing)
	}

	agFail := newDSTTestAgent(&dstSaveStubDriver{cavesRunning: true, failShards: map[string]bool{"caves": true}})
	confFail := agFail.flushInstanceBeforeBackup(context.Background(), nil, newDSTTestInstance("true"))
	if confFail == nil || confFail.Status != corev1.BackupSaveConfirmationUnconfirmed ||
		!reflect.DeepEqual(confFail.ConfirmedShards, []string{"master"}) ||
		!reflect.DeepEqual(confFail.UnconfirmedShards, []string{"caves"}) {
		t.Fatalf("expected caves failure unconfirmed, got %+v", confFail)
	}

	agTimeout := newDSTTestAgent(&dstSaveStubDriver{cavesRunning: true, silentShards: map[string]bool{"caves": true}})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	confTimeout := agTimeout.flushInstanceBeforeBackup(ctx, nil, newDSTTestInstance("true"))
	if confTimeout == nil || confTimeout.Status != corev1.BackupSaveConfirmationUnconfirmed ||
		!reflect.DeepEqual(confTimeout.UnconfirmedShards, []string{"caves"}) || !strings.Contains(confTimeout.Warning, "超时") {
		t.Fatalf("expected caves timeout warning, got %+v", confTimeout)
	}
}
