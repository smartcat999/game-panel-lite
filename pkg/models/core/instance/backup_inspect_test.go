package instance

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/s3"
	"github.com/smartcat999/game-panel-cloud/pkg/models"
	backupdb "github.com/smartcat999/game-panel-cloud/pkg/repo/core/backup"
)

type fakeInspectS3 struct {
	s3.Interface
	objects  map[string][]byte
	getCalls int
}

func (f *fakeInspectS3) IsConfigured() bool { return true }

func (f *fakeInspectS3) Get(_ context.Context, key string) (io.ReadCloser, *s3.ObjectStat, error) {
	f.getCalls++
	data, ok := f.objects[key]
	if !ok {
		return nil, nil, s3.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), &s3.ObjectStat{SizeBytes: int64(len(data))}, nil
}

func buildTestZipBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	_ = zw.Close()
	return buf.Bytes()
}

func newTestDSTInspectInstance(key string) *corev1.Instance {
	return &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			UID: "uid-dst-01", Tenant: "default", Name: "dst-01",
			Labels:      map[string]string{corev1.LabelInstanceApp: "dont-starve-together"},
			Annotations: map[string]string{"core.gamepanel.io/app-config": `{"identity.serverName":"Panel Server","identity.clusterToken":"cur-token"}`},
		},
		Spec: corev1.InstanceSpec{
			Files: []corev1.InstanceFile{
				{Path: "DoNotStarveTogether/Cluster_1/cluster.ini"},
				{Path: "DoNotStarveTogether/Cluster_1/Master/server.ini"},
				{Path: "other/unrelated.txt"},
			},
			BackupPolicy: &corev1.InstanceBackupPolicy{
				Subtree: "DoNotStarveTogether/Cluster_1", AnchorFiles: []string{"cluster.ini", "Master/server.ini"}, ReplaceOnRestore: true,
			},
		},
		Status: corev1.InstanceStatus{Backups: []corev1.InstanceBackupRecord{{Path: key, Phase: corev1.BackupPhaseCompleted, SizeBytes: 4096}}},
	}
}

func TestHandleInspectBackupActionAsyncBoundary(t *testing.T) {
	key := "tenants/default/instances/dst-01/backup.zip"
	zipBytes := buildTestZipBytes(t, map[string]string{
		"LiteCluster/cluster.ini":         "[GAMEPLAY]\ngame_mode = endless\nmax_players = 8\n[NETWORK]\ncluster_name = Lite Room\ncluster_password = secret-pw\n",
		"LiteCluster/cluster_token.txt":   "pds-g^arch-token\n",
		"LiteCluster/Master/server.ini":   "[NETWORK]\n",
		"LiteCluster/Caves/server.ini":    "[NETWORK]\n",
		"LiteCluster/Master/save/001.bin": "raw",
	})
	fakeS3 := &fakeInspectS3{objects: map[string][]byte{key: zipBytes}}
	op := &operator{s3: fakeS3}
	inst := newTestDSTInspectInstance(key)

	first, err := op.handleInspectBackupWithKey(context.Background(), inst, key, &corev1.InstanceFiles{Path: key}, 10<<20)
	if err != nil {
		t.Fatalf("handleInspectBackupAction enqueue failed: %v", err)
	}
	if first.Phase != corev1.BackupInspectPhasePending || fakeS3.getCalls != 0 {
		t.Fatalf("expected Pending with 0 S3 calls in API path, got phase=%q s3Calls=%d", first.Phase, fakeS3.getCalls)
	}
	if err := op.reconcilePendingBackupInspections(context.Background(), inst); err != nil {
		t.Fatalf("reconcilePendingBackupInspections failed: %v", err)
	}
	if fakeS3.getCalls != 1 {
		t.Fatalf("expected exactly 1 S3 Get during controller reconcile, got %d", fakeS3.getCalls)
	}
	verifyCompletedInspectOutput(t, op, inst, fakeS3, key)
}

func verifyCompletedInspectOutput(t *testing.T, op *operator, inst *corev1.Instance, fakeS3 *fakeInspectS3, key string) {
	t.Helper()
	out, err := op.handleInspectBackupWithKey(context.Background(), inst, key, &corev1.InstanceFiles{Path: key}, 10<<20)
	if err != nil {
		t.Fatalf("completed handleInspectBackupWithKey failed: %v", err)
	}
	if fakeS3.getCalls != 1 {
		t.Fatalf("API read must not hit S3 again, got s3Calls=%d", fakeS3.getCalls)
	}
	if out.Phase != corev1.BackupInspectPhaseCompleted || out.ArchiveFormat != corev1.ArchiveFormatZip || out.SourcePath != "LiteCluster" || out.TargetPath != "DoNotStarveTogether/Cluster_1" {
		t.Fatalf("unexpected completed output: %+v", out)
	}
	if out.FileCount != 5 || len(out.SourceEntries) != 4 || out.ReplaceTarget == nil || !*out.ReplaceTarget {
		t.Fatalf("unexpected counts/entries: fileCount=%d entries=%v replace=%v", out.FileCount, out.SourceEntries, out.ReplaceTarget)
	}
	wantManaged := []string{"DoNotStarveTogether/Cluster_1/Master/server.ini", "DoNotStarveTogether/Cluster_1/cluster.ini"}
	if !reflect.DeepEqual(out.ManagedFiles, wantManaged) {
		t.Fatalf("managedFiles = %v, want %v", out.ManagedFiles, wantManaged)
	}
	verifyPreviewAndOverride(t, op, inst, fakeS3, key, out.ConfigPreview)

	// Simulate a cached pre-v2 inspection: API must re-enqueue async reconciliation with 0 S3 calls.
	inst.Status.Backups[0].Inspection.SchemaVersion = 0
	staleResp, err := op.handleInspectBackupWithKey(context.Background(), inst, key, &corev1.InstanceFiles{Path: key}, 10<<20)
	if err != nil || staleResp.Phase != corev1.BackupInspectPhasePending || fakeS3.getCalls != 1 {
		t.Fatalf("expected stale SchemaVersion to re-enqueue Pending with 0 extra S3 calls, got err=%v resp=%+v s3Calls=%d", err, staleResp, fakeS3.getCalls)
	}
	if err := op.reconcilePendingBackupInspections(context.Background(), inst); err != nil {
		t.Fatalf("reconcilePendingBackupInspections upgrade failed: %v", err)
	}
	if fakeS3.getCalls != 2 || inst.Status.Backups[0].Inspection.SchemaVersion != corev1.CurrentBackupInspectSchemaVersion {
		t.Fatalf("expected schema v%d after reconcile with s3Calls=2, got v%d s3Calls=%d", corev1.CurrentBackupInspectSchemaVersion, inst.Status.Backups[0].Inspection.SchemaVersion, fakeS3.getCalls)
	}
}

func verifyPreviewAndOverride(t *testing.T, op *operator, inst *corev1.Instance, fakeS3 *fakeInspectS3, key string, items []corev1.InstanceBackupConfigItem) {
	t.Helper()
	byName := make(map[string]corev1.InstanceBackupConfigItem, len(items))
	for _, it := range items {
		if strings.Contains(it.ArchiveValue, "secret-pw") || strings.Contains(it.ArchiveValue, "pds-g^arch-token") {
			t.Fatalf("plaintext secret leaked in preview item: %+v", it)
		}
		byName[it.Name] = it
	}
	if got := byName["identity.serverName"]; got.ArchiveValue != "Lite Room" || !got.Different || !got.OverriddenByPanel {
		t.Fatalf("unexpected serverName preview: %+v", got)
	}
	if got := byName["identity.password"]; !got.Secret || !got.ArchiveSet || got.CurrentSet || got.ArchiveValue != "" {
		t.Fatalf("unexpected password preview: %+v", got)
	}
	mapped, err := op.handleInspectBackupWithKey(context.Background(), inst, key, &corev1.InstanceFiles{Path: key, SourcePath: ".", TargetPath: "CustomDir"}, 10<<20)
	if err != nil || fakeS3.getCalls != 1 || mapped.TargetPath != "CustomDir" || len(mapped.ConfigPreview) != 0 {
		t.Fatalf("override mapping failed: err=%v s3Calls=%d mapped=%+v", err, fakeS3.getCalls, mapped)
	}
}

func TestResolveRestoreMappingAndMergeTelemetry(t *testing.T) {
	inst := &corev1.Instance{
		Spec: corev1.InstanceSpec{BackupPolicy: &corev1.InstanceBackupPolicy{Subtree: "Worlds,Mods,SaveData", ReplaceOnRestore: true}},
	}
	src, tgt, rpl, err := resolveRestoreMapping(inst, "tModLoader", "", nil)
	if err != nil || src != "tModLoader" || tgt != "." || rpl {
		t.Fatalf("got (%q, %q, %v, %v), want (\"tModLoader\", \".\", false, nil)", src, tgt, rpl, err)
	}
	if _, _, _, err := resolveRestoreMapping(inst, "../bad", ".", nil); err == nil {
		t.Fatal("expected error for traversal sourcePath")
	}

	dstInst := &corev1.Instance{
		Spec: corev1.InstanceSpec{BackupPolicy: &corev1.InstanceBackupPolicy{Subtree: "DoNotStarveTogether/Cluster_1", ReplaceOnRestore: true}},
	}
	if s, g, p, e := resolveRestoreMapping(dstInst, "", "", nil); e != nil || s != "DoNotStarveTogether/Cluster_1" || g != "DoNotStarveTogether/Cluster_1" || !p {
		t.Fatalf("native restore mapping got (%q, %q, %v, %v), want symmetric Cluster_1 with replace=true", s, g, p, e)
	}
	if s, g, p, e := resolveRestoreMapping(dstInst, ".", "", nil); e != nil || s != "." || g != "DoNotStarveTogether/Cluster_1" || !p {
		t.Fatalf("explicit root restore mapping got (%q, %q, %v, %v), want (\".\", \"DoNotStarveTogether/Cluster_1\", true)", s, g, p, e)
	}

	rpl = true
	ins := &corev1.InstanceBackupInspection{SchemaVersion: corev1.CurrentBackupInspectSchemaVersion, Phase: corev1.BackupInspectPhaseCompleted, FileCount: 12}
	oldRec := &corev1.InstanceBackupRecord{
		Phase: corev1.BackupPhaseRestoring, RestoreSourcePath: "LiteCluster", RestoreTargetPath: "DoNotStarveTogether/Cluster_1", RestoreReplace: &rpl, Inspection: ins,
	}
	nextRec := &corev1.InstanceBackupRecord{}
	mergeBackupTelemetry(oldRec, nextRec, backupdb.Completed)
	if nextRec.Phase != corev1.BackupPhaseRestoring || nextRec.Inspection == nil || nextRec.Inspection.FileCount != 12 {
		t.Fatalf("mergeBackupTelemetry lost inspection/restore fields: %+v", nextRec)
	}

	nativeDone := &corev1.InstanceBackupRecord{Imported: false}
	mergeBackupTelemetry(&corev1.InstanceBackupRecord{Phase: corev1.BackupPhaseUploading}, nativeDone, backupdb.Completed)
	if nativeDone.Phase != corev1.BackupPhaseCompleted || nativeDone.Inspection != nil {
		t.Fatalf("expected native backup to complete with nil Inspection, got phase=%q inspection=%+v", nativeDone.Phase, nativeDone.Inspection)
	}

	importedDone := &corev1.InstanceBackupRecord{Imported: true}
	mergeBackupTelemetry(&corev1.InstanceBackupRecord{Phase: corev1.BackupPhaseUploading, Imported: true}, importedDone, backupdb.Completed)
	if importedDone.Phase != corev1.BackupPhaseInspecting || importedDone.Stage != corev1.BackupStageInspecting || importedDone.Inspection == nil || importedDone.Inspection.Phase != corev1.BackupInspectPhasePending {
		t.Fatalf("expected imported backup to enter Inspecting phase with Pending inspection, got phase=%q stage=%q ins=%+v", importedDone.Phase, importedDone.Stage, importedDone.Inspection)
	}
}

func TestImportedBackupInspectLifecycleAndCorruptArchive(t *testing.T) {
	okKey := "tenants/default/instances/dst-01/imported.zip"
	badKey := "tenants/default/instances/dst-01/corrupt.zip"
	zipBytes := buildTestZipBytes(t, map[string]string{
		"LiteCluster/cluster.ini": "[NETWORK]\ncluster_name = OK\n",
	})
	fakeS3 := &fakeInspectS3{objects: map[string][]byte{okKey: zipBytes, badKey: []byte("not-a-valid-archive")}}
	op := &operator{s3: fakeS3}
	inst := newTestDSTInspectInstance(okKey)
	inst.Status.Backups = []corev1.InstanceBackupRecord{
		{
			Path: okKey, Imported: true, Phase: corev1.BackupPhaseInspecting, Stage: corev1.BackupStageInspecting,
			Inspection: &corev1.InstanceBackupInspection{Phase: corev1.BackupInspectPhasePending},
		},
		{
			Path: badKey, Imported: true, Phase: corev1.BackupPhaseInspecting, Stage: corev1.BackupStageInspecting,
			Inspection: &corev1.InstanceBackupInspection{Phase: corev1.BackupInspectPhasePending},
		},
	}

	if err := validateBackupRecordRestorable(inst, okKey); !errors.Is(err, models.ErrConflict) {
		t.Fatalf("expected ErrConflict while Inspecting, got %v", err)
	}
	if err := op.reconcilePendingBackupInspections(context.Background(), inst); err != nil {
		t.Fatalf("reconcilePendingBackupInspections failed: %v", err)
	}
	okRec, badRec := inst.Status.Backups[0], inst.Status.Backups[1]
	if okRec.Phase != corev1.BackupPhaseCompleted || okRec.Inspection == nil || okRec.Inspection.Phase != corev1.BackupInspectPhaseCompleted {
		t.Fatalf("expected valid imported archive to become Completed, got phase=%q ins=%+v", okRec.Phase, okRec.Inspection)
	}
	if badRec.Phase != corev1.BackupPhaseInspectFailed || badRec.Inspection == nil || badRec.Inspection.Phase != corev1.BackupInspectPhaseFailed || badRec.Message == "" {
		t.Fatalf("expected corrupt imported archive to become InspectFailed, got %+v", badRec)
	}
	if err := validateBackupRecordRestorable(inst, badKey); !errors.Is(err, models.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest restoring InspectFailed backup, got %v", err)
	}
}

func TestNativeBackupSnapshotAndInspectFallback(t *testing.T) {
	key := "tenants/default/instances/dst-01/native.tar.gz"
	inst := newTestDSTInspectInstance(key)
	inst.Metadata.Annotations[corev1.AnnotationInstanceAppVersion] = "2026.11"
	inst.Metadata.Annotations["core.gamepanel.io/app-config"] = `{"identity.serverName":"Snapshot Room","identity.password":"sec-pw","identity.clusterToken":"cur-token"}`
	inst.Spec.Addons = []corev1.InstanceAddon{
		{Repository: "dst-ws", Package: "378160973", Version: "1", Title: "Global Positions", Config: map[string]string{"ENABLEPINGS": "true"}},
		{Repository: "dst-ws", Package: "362175979", Version: "1700", Disabled: true},
	}

	op := &operator{s3: &fakeInspectS3{objects: map[string][]byte{}}}
	snap := op.captureInstanceBackupSnapshot(context.Background(), inst)
	if snap == nil || snap.ServerVersion != "2026.11" || snap.AddonCount != 2 || len(snap.ConfigFields) == 0 {
		t.Fatalf("unexpected native snapshot: %+v", snap)
	}
	for _, f := range snap.ConfigFields {
		if strings.Contains(f.Value, "sec-pw") || strings.Contains(f.Value, "cur-token") {
			t.Fatalf("secret leaked in snapshot ConfigFields: %+v", f)
		}
	}

	inst.Metadata.Annotations["core.gamepanel.io/app-config"] = `{"identity.serverName":"Mutated Room","identity.clusterToken":"cur-token"}`
	inst.Spec.Addons = inst.Spec.Addons[:1]
	inst.Metadata.Annotations[corev1.AnnotationInstanceAppVersion] = "2026.12"
	inst.Status.Backups[0].Snapshot = snap

	resp, err := op.handleInspectBackupWithKey(context.Background(), inst, key, &corev1.InstanceFiles{Path: key}, 10<<20)
	if err != nil || resp.Phase != corev1.BackupInspectPhasePending {
		t.Fatalf("expected immediate preview during Pending inspection, got err=%v resp=%+v", err, resp)
	}
	if resp.ServerVersion != "2026.11" || resp.CurrentServerVersion != "2026.12" || len(resp.ConfigPreview) == 0 || len(resp.AddonPreview) != 2 {
		t.Fatalf("expected snapshot previews populated while Pending: %+v", resp)
	}
	if err := op.reconcilePendingBackupInspections(context.Background(), inst); err != nil {
		t.Fatalf("reconcilePendingBackupInspections on missing S3 native key returned err: %v", err)
	}
	if b := inst.Status.Backups[0]; b.Phase != corev1.BackupPhaseCompleted || b.Inspection == nil || b.Inspection.Phase != corev1.BackupInspectPhaseFailed {
		t.Fatalf("expected native Phase=Completed and Inspection.Phase=Failed, got phase=%q ins=%+v", b.Phase, b.Inspection)
	}
	fallback, err := op.handleInspectBackupWithKey(context.Background(), inst, key, &corev1.InstanceFiles{Path: key}, 10<<20)
	if err != nil || fallback.Phase != corev1.BackupInspectPhaseCompleted || len(fallback.ConfigPreview) == 0 || len(fallback.AddonPreview) != 2 {
		t.Fatalf("expected snapshot fallback to serve Completed previews on native backup, got err=%v out=%+v", err, fallback)
	}
}

func TestImportedSchemaV3AddonsPreview(t *testing.T) {
	key := "tenants/default/instances/dst-01/lite-v3.zip"
	zipBytes := buildTestZipBytes(t, map[string]string{
		"Cluster_Lite/cluster.ini":             "[NETWORK]\ncluster_name = LiteModRoom\n",
		"Cluster_Lite/Master/server.ini":       "[NETWORK]\n",
		"Cluster_Lite/Master/modoverrides.lua": "return {\n  [\"workshop-378160973\"] = { enabled = true, configuration_options = { SHOWME = true, LANG = \"zh\" } },\n  [\"workshop-362175979\"] = { enabled = false },\n}\n",
		"Cluster_Lite/Caves/modoverrides.lua":  "return {\n  [\"workshop-378160973\"] = { enabled = true },\n}\n",
	})
	op := &operator{s3: &fakeInspectS3{objects: map[string][]byte{key: zipBytes}}}
	inst := newTestDSTInspectInstance(key)
	inst.Spec.Addons = []corev1.InstanceAddon{{Repository: "dst-ws", Package: "378160973", Version: "1"}}
	inst.Status.Backups[0] = corev1.InstanceBackupRecord{
		Path: key, Imported: true, Phase: corev1.BackupPhaseInspecting,
		Inspection: &corev1.InstanceBackupInspection{Phase: corev1.BackupInspectPhasePending},
	}
	if err := op.reconcilePendingBackupInspections(context.Background(), inst); err != nil {
		t.Fatalf("reconcilePendingBackupInspections v3 failed: %v", err)
	}
	out, err := op.handleInspectBackupWithKey(context.Background(), inst, key, &corev1.InstanceFiles{Path: key}, 10<<20)
	if err != nil || len(out.AddonPreview) != 2 {
		t.Fatalf("expected 2 AddonPreview items at schema v3, got err=%v addons=%+v", err, out.AddonPreview)
	}
	byPkg := map[string]corev1.InstanceBackupAddonItem{}
	for _, a := range out.AddonPreview {
		byPkg[a.Package] = a
	}
	if gp := byPkg["378160973"]; !gp.InstalledNow || gp.Different || gp.ConfigCount != 2 || !reflect.DeepEqual(gp.Scopes, []string{"Master", "Caves"}) {
		t.Fatalf("unexpected Master+Caves addon preview item: %+v", gp)
	}
	if wm := byPkg["362175979"]; wm.InstalledNow || !wm.BackupDisabled || !wm.Different {
		t.Fatalf("expected disabled un-installed 362175979 item: %+v", wm)
	}
}

func TestValidateDirectFilePathsRejectsReservedPrefix(t *testing.T) {
	if err := validateDirectFilePaths(&corev1.InstanceFiles{Action: corev1.FileActionList, Path: "/"}); err != nil {
		t.Fatalf("expected root list to pass, got %v", err)
	}
	if err := validateDirectFilePaths(&corev1.InstanceFiles{Action: corev1.FileActionRead, Path: "save/world.dat"}); err != nil {
		t.Fatalf("expected normal path to pass, got %v", err)
	}
	err := validateDirectFilePaths(&corev1.InstanceFiles{Action: corev1.FileActionRead, Path: "mapped-restore/.gp-injected/evil.txt"})
	if !errors.Is(err, models.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest for reserved read path, got %v", err)
	}
	err = validateDirectFilePaths(&corev1.InstanceFiles{Action: corev1.FileActionRename, Path: "save/a.txt", TargetPath: ".gp-evil/a.txt"})
	if !errors.Is(err, models.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest for reserved rename targetPath, got %v", err)
	}
}
