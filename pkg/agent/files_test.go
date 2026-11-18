package agent

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/runtime"
)

func decodeNDJSONEvents(t *testing.T, body []byte) []corev1.InstanceTaskStreamEvent {
	t.Helper()
	var events []corev1.InstanceTaskStreamEvent
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		var ev corev1.InstanceTaskStreamEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			t.Fatalf("unmarshal NDJSON line %q: %v", scanner.Text(), err)
		}
		events = append(events, ev)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan NDJSON stream: %v", err)
	}
	return events
}

func TestExecuteStreamingBackupNDJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "world.dat"), []byte("save-data-12345"), 0o644); err != nil {
		t.Fatalf("write seed save file: %v", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer func() { _ = root.Close() }()

	var uploadedTarGz []byte
	putSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		uploadedTarGz, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer putSrv.Close()

	tmp, err := os.CreateTemp(t.TempDir(), "bak-*.tar.gz")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer func() { _ = tmp.Close() }()

	req := httptest.NewRequest(http.MethodPost, "/files", nil)
	req.Header.Set("Accept", "application/x-ndjson")
	rec := httptest.NewRecorder()
	ts := newTaskStreamWriter(rec, req)
	fileReq := &corev1.InstanceFiles{Action: corev1.FileActionBackup, PresignedURL: putSrv.URL + "/bak.tar.gz"}

	(&Agent{}).executeStreamingBackup(ts, req, &corev1.Instance{}, root, tmp, 10<<20, fileReq)
	if len(uploadedTarGz) == 0 {
		t.Fatalf("expected backup payload uploaded via PUT")
	}
	events := decodeNDJSONEvents(t, rec.Body.Bytes())
	var sawProgress, sawResult bool
	for _, ev := range events {
		if ev.Type == corev1.TaskStreamEventProgress {
			sawProgress = true
		}
		if ev.Type == corev1.TaskStreamEventResult && ev.Result != nil && ev.Result.SizeBytes > 0 && ev.Result.SHA256 != "" {
			sawResult = true
		}
		if ev.Type == corev1.TaskStreamEventError {
			t.Fatalf("unexpected error event: %s", ev.Message)
		}
	}
	if !sawProgress || !sawResult {
		t.Fatalf("expected progress (%v) and result (%v) events, got %d events", sawProgress, sawResult, len(events))
	}
}

func TestExecuteStreamingRestoreNDJSON(t *testing.T) {
	restoreDir := t.TempDir()
	restoreRoot, err := os.OpenRoot(restoreDir)
	if err != nil {
		t.Fatalf("open restore root: %v", err)
	}
	defer func() { _ = restoreRoot.Close() }()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	payload := []byte("restored-world-content")
	if err := tw.WriteHeader(&tar.Header{Name: "restored.dat", Mode: 0o644, Size: int64(len(payload))}); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	_ = tw.Close()
	_ = gzw.Close()

	getSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer getSrv.Close()

	restoreReq := httptest.NewRequest(http.MethodPost, "/files", nil)
	restoreReq.Header.Set("Accept", "application/x-ndjson")
	restoreRec := httptest.NewRecorder()
	restoreTS := newTaskStreamWriter(restoreRec, restoreReq)
	getReq, _ := http.NewRequest(http.MethodGet, getSrv.URL+"/bak.tar.gz", nil)
	(&Agent{}).executeStreamingRestore(restoreTS, restoreReq, nil, restoreRoot, getReq, 10<<20, &corev1.InstanceFiles{Action: corev1.FileActionRestore})

	gotBytes, err := os.ReadFile(filepath.Join(restoreDir, "restored.dat"))
	if err != nil || string(gotBytes) != string(payload) {
		t.Fatalf("restored file mismatch: err=%v got=%q", err, string(gotBytes))
	}
}

func TestExecuteStreamingRestoreZipMappedNDJSON(t *testing.T) {
	restoreDir := t.TempDir()
	stalePath := filepath.Join(restoreDir, "DoNotStarveTogether/Cluster_1/stale.dat")
	if err := os.MkdirAll(filepath.Dir(stalePath), 0o755); err != nil {
		t.Fatalf("mkdir stale: %v", err)
	}
	if err := os.WriteFile(stalePath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}
	restoreRoot, err := os.OpenRoot(restoreDir)
	if err != nil {
		t.Fatalf("open restore root: %v", err)
	}
	defer func() { _ = restoreRoot.Close() }()

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	w, err := zw.Create("MyCluster/cluster.ini")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	_, _ = w.Write([]byte("cluster_name=Test\n"))
	_ = zw.Close()

	getSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(zipBuf.Bytes())
	}))
	defer getSrv.Close()

	req := httptest.NewRequest(http.MethodPost, "/files", nil)
	req.Header.Set("Accept", "application/x-ndjson")
	rec := httptest.NewRecorder()
	ts := newTaskStreamWriter(rec, req)
	getReq, _ := http.NewRequest(http.MethodGet, getSrv.URL+"/lite.zip", nil)
	replace := true
	(&Agent{}).executeStreamingRestore(ts, req, nil, restoreRoot, getReq, 10<<20, &corev1.InstanceFiles{
		Action:        corev1.FileActionRestore,
		SourcePath:    "MyCluster",
		TargetPath:    "DoNotStarveTogether/Cluster_1",
		ReplaceTarget: &replace,
	})

	got, err := os.ReadFile(filepath.Join(restoreDir, "DoNotStarveTogether/Cluster_1/cluster.ini"))
	if err != nil || string(got) != "cluster_name=Test\n" {
		t.Fatalf("mapped zip restore mismatch: err=%v got=%q", err, string(got))
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale.dat removed on replaceTarget restore, err=%v", err)
	}
}

type quiesceBackupStubDriver struct {
	probeStubRuntimeDriver
	quiesceCount int
}

func (d *quiesceBackupStubDriver) WithQuiescedWorkload(_ context.Context, _ *corev1.Instance, _ *corev1.RuntimeClass, onStage func(string), fn func(string) error) error {
	d.quiesceCount++
	if onStage != nil {
		onStage(runtime.QuiesceStageStopping)
	}
	if err := fn(""); err != nil {
		return err
	}
	if onStage != nil {
		onStage(runtime.QuiesceStageRestarting)
	}
	return nil
}

func TestArchiveInstanceBackupQuiescesWhenRequired(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "world.db"), []byte("valheim-world"), 0o644)
	root, _ := os.OpenRoot(dir)
	defer func() { _ = root.Close() }()

	drv := &quiesceBackupStubDriver{}
	reg := &runtime.Registry{}
	reg.RegisterDriver(drv)
	ag := &Agent{drivers: reg}
	inst := &corev1.Instance{
		Spec: corev1.InstanceSpec{
			State: corev1.InstanceStateRunning,
			BackupPolicy: &corev1.InstanceBackupPolicy{
				BackupRequiresStop: true,
			},
		},
		Status: corev1.InstanceStatus{
			Phase: corev1.InstancePhaseRunning,
			ResourceControl: &corev1.InstanceResourceControl{
				RuntimeClass: &corev1.InstanceRuntimeBinding{UID: "rc-uid", Name: "docker", Handler: "docker"},
			},
		},
	}

	tmp1, _ := os.CreateTemp(t.TempDir(), "bak-*.tar.gz")
	defer func() { _ = tmp1.Close() }()
	size, sum, err := ag.archiveInstanceBackup(context.Background(), newTaskStreamWriter(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/files", nil)), inst, root, tmp1, 1<<20)
	if err != nil || size == 0 || sum == "" || drv.quiesceCount != 1 {
		t.Fatalf("expected quiesced backup once with non-empty archive, got size=%d quiesceCount=%d err=%v", size, drv.quiesceCount, err)
	}

	inst.Spec.BackupPolicy.BackupRequiresStop = false
	tmp2, _ := os.CreateTemp(t.TempDir(), "bak2-*.tar.gz")
	defer func() { _ = tmp2.Close() }()
	if _, _, err := ag.archiveInstanceBackup(context.Background(), newTaskStreamWriter(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/files", nil)), inst, root, tmp2, 1<<20); err != nil || drv.quiesceCount != 1 {
		t.Fatalf("expected non-stop backup to skip quiesce, got quiesceCount=%d err=%v", drv.quiesceCount, err)
	}
}
