package volume

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

func TestMaterializeRootIsolationAndPermissions(t *testing.T) {
	srcDir, workDir := t.TempDir(), t.TempDir()
	srcJar := filepath.Join(srcDir, "server.jar")
	srcBin := filepath.Join(srcDir, "run.sh")
	mustWriteHostFile(t, srcJar, "jar-bytes", 0o444)
	mustWriteHostFile(t, srcBin, "#!/bin/sh\necho ok\n", 0o555)

	root, err := os.OpenRoot(workDir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	if err := MaterializeFileInRoot(srcJar, root, "server.jar", MaterializeOptions{ForceReplace: true}); err != nil {
		t.Fatalf("MaterializeFileInRoot(jar): %v", err)
	}
	if err := MaterializeFileInRoot(srcBin, root, "bin/run.sh", MaterializeOptions{ForceReplace: true}); err != nil {
		t.Fatalf("MaterializeFileInRoot(bin): %v", err)
	}

	assertIsolatedMaterializedFile(t, srcJar, filepath.Join(workDir, "server.jar"), 0o644)
	assertIsolatedMaterializedFile(t, srcBin, filepath.Join(workDir, "bin", "run.sh"), 0o755)
	mutateFileInPlace(t, filepath.Join(workDir, "server.jar"), "tampered!")
	if got := mustReadHostFile(t, srcJar); got != "jar-bytes" {
		t.Fatalf("source mutated via materialized copy: got %q", got)
	}
}

func TestMaterializeTreeAndListRegularFilesInRoot(t *testing.T) {
	srcDir, workDir := t.TempDir(), t.TempDir()
	mustWriteHostFile(t, filepath.Join(srcDir, "ModA.tmod"), "mod-a", 0o444)
	mustWriteHostFile(t, filepath.Join(srcDir, "sub", "ModB.tmod"), "mod-b", 0o444)
	mustWriteHostFile(t, filepath.Join(srcDir, ".gp-ignore.json"), "skip", 0o444)
	mustWriteHostFile(t, filepath.Join(srcDir, "partial.tmp"), "skip", 0o444)

	root, err := os.OpenRoot(workDir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	got, err := MaterializeTreeInRoot(srcDir, root, "mods", MaterializeOptions{ForceReplace: true})
	if err != nil {
		t.Fatalf("MaterializeTreeInRoot: %v", err)
	}
	if !slices.Equal(got, []string{"ModA.tmod", "sub/ModB.tmod"}) {
		t.Fatalf("unexpected materialized paths: %v", got)
	}
	if body := mustReadHostFile(t, filepath.Join(workDir, "mods", "sub", "ModB.tmod")); body != "mod-b" {
		t.Fatalf("materialized nested file content = %q", body)
	}
	if err := os.Symlink(srcDir, filepath.Join(workDir, "mods", "escape.tmod")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	listed, err := ListRegularFilesInRoot(root, "mods")
	if err != nil {
		t.Fatalf("ListRegularFilesInRoot: %v", err)
	}
	if !slices.Equal(listed, []string{"ModA.tmod"}) {
		t.Fatalf("unexpected listed regular files: %v", listed)
	}
}

func TestRootOperationsRejectParentSymlinkEscapes(t *testing.T) {
	srcDir, workDir, outsideDir := t.TempDir(), t.TempDir(), t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	mustWriteHostFile(t, outsideFile, "HOST_SECRET", 0o600)
	srcFile := filepath.Join(srcDir, "plugin.jar")
	mustWriteHostFile(t, srcFile, "plugin-data", 0o444)

	if err := os.Symlink(outsideDir, filepath.Join(workDir, "plugins")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	root, err := os.OpenRoot(workDir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	if _, err := EnsureSafeDirInRoot(root, "plugins/nested"); err == nil {
		t.Fatalf("expected EnsureSafeDirInRoot to reject symlinked parent")
	}
	if err := WriteFileAtomicInRoot(root, "plugins/pwn.txt", []byte("evil"), 0o644); err == nil {
		t.Fatalf("expected WriteFileAtomicInRoot to reject symlinked parent")
	}
	if _, err := ReadOptionalFileInRoot(root, "plugins/secret.txt"); err == nil {
		t.Fatalf("expected ReadOptionalFileInRoot to reject symlinked parent")
	}
	if _, err := ListRegularFilesInRoot(root, "plugins"); err == nil {
		t.Fatalf("expected ListRegularFilesInRoot to reject symlinked parent")
	}
	if err := MaterializeFileInRoot(srcFile, root, "plugins/plugin.jar", MaterializeOptions{ForceReplace: true}); err == nil {
		t.Fatalf("expected MaterializeFileInRoot to reject symlinked parent")
	}
	assertOutsideDirUnchanged(t, outsideDir, outsideFile, "HOST_SECRET")
}

func TestRootOperationsHandleLeafSymlinksSafely(t *testing.T) {
	srcDir, workDir, outsideDir := t.TempDir(), t.TempDir(), t.TempDir()
	outsideCanary := filepath.Join(outsideDir, "canary.txt")
	mustWriteHostFile(t, outsideCanary, "OUTSIDE_CANARY", 0o600)
	srcFile := filepath.Join(srcDir, "mod.bin")
	mustWriteHostFile(t, srcFile, "MOD_DATA", 0o444)

	if err := os.MkdirAll(filepath.Join(workDir, "mods"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	linkPath := filepath.Join(workDir, "mods", "mod.bin")
	if err := os.Symlink(outsideCanary, linkPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	root, err := os.OpenRoot(workDir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	if _, err := ReadOptionalFileInRoot(root, "mods/mod.bin"); err == nil {
		t.Fatalf("expected ReadOptionalFileInRoot to reject leaf symlink")
	}
	if err := MaterializeFileInRoot(srcFile, root, "mods/mod.bin", MaterializeOptions{ForceReplace: false}); err == nil {
		t.Fatalf("expected MaterializeFileInRoot(ForceReplace=false) to reject leaf symlink")
	}
	if err := WriteFileAtomicInRoot(root, "mods/mod.bin", []byte("REPLACED"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomicInRoot replace leaf symlink: %v", err)
	}
	if got := mustReadHostFile(t, outsideCanary); got != "OUTSIDE_CANARY" {
		t.Fatalf("outside canary modified via leaf symlink: %q", got)
	}
}

func TestRootWriteConcurrentSymlinkSwapNeverEscapes(t *testing.T) {
	srcDir, workDir, outsideDir := t.TempDir(), t.TempDir(), t.TempDir()
	outsideCanary := filepath.Join(outsideDir, "canary.txt")
	mustWriteHostFile(t, outsideCanary, "OUTSIDE_CANARY", 0o600)
	srcFile := filepath.Join(srcDir, "payload.bin")
	mustWriteHostFile(t, srcFile, "SAFE_PAYLOAD", 0o444)

	root, err := os.OpenRoot(workDir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	var stop atomic.Bool
	done := make(chan struct{})
	go runSymlinkSwapAttacker(workDir, outsideDir, outsideCanary, &stop, done)

	for i := 0; i < 150; i++ {
		_ = WriteFileAtomicInRoot(root, "swap/leaf.txt", []byte("SAFE"), 0o644)
		_ = MaterializeFileInRoot(srcFile, root, "swap/leaf.txt", MaterializeOptions{ForceReplace: true})
		data, err := ReadOptionalFileInRoot(root, "swap/leaf.txt")
		if err == nil && string(data) == "OUTSIDE_CANARY" {
			stop.Store(true)
			<-done
			t.Fatalf("read leaked outside canary through swapped symlink")
		}
	}
	stop.Store(true)
	<-done
	assertOutsideDirUnchanged(t, outsideDir, outsideCanary, "OUTSIDE_CANARY")
}

func TestMaterializeRequireReflinkFailClosedAndConfig(t *testing.T) {
	srcDir, workDir := t.TempDir(), t.TempDir()
	srcFile := filepath.Join(srcDir, "server.jar")
	mustWriteHostFile(t, srcFile, "jar", 0o444)

	root, err := os.OpenRoot(workDir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	err = finishMaterializeFile(srcFile, root, ".", "server.jar", ".gp-cow-test", 0o644, false, true)
	if !errors.Is(err, ErrReflinkUnsupported) {
		t.Fatalf("expected ErrReflinkUnsupported when cloned=false and requireReflink=true, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workDir, "server.jar")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no fallback copy when RequireReflink fails closed")
	}

	if RequiresReflink(nil) || RequiresReflink(&corev1.RuntimeClass{}) {
		t.Fatalf("expected default RuntimeClass not to require reflink")
	}
	if !RequiresReflink(&corev1.RuntimeClass{Parameters: map[string]string{"storageQuotaProvider": DefaultXFSQuotaProviderName}}) {
		t.Fatalf("expected XFS RuntimeClass to require reflink")
	}
	if !RequiresReflink(&corev1.RuntimeClass{Parameters: map[string]string{"requireReflink": "true"}}) {
		t.Fatalf("expected requireReflink=true RuntimeClass to require reflink")
	}
	if RequireReflinkFromContext(context.Background()) || !RequireReflinkFromContext(WithRequireReflink(context.Background(), true)) {
		t.Fatalf("unexpected RequireReflinkFromContext result")
	}
}

func TestRemoveInstanceWorkDirCleansExternalState(t *testing.T) {
	workDir := t.TempDir()
	uid := "inst-uid-123"
	stateRoot, err := OpenInstanceStateRoot(workDir, uid, true)
	if err != nil {
		t.Fatalf("OpenInstanceStateRoot: %v", err)
	}
	if err := WriteJSONAtomicInRoot(stateRoot, "server-artifact.json", map[string]string{"ok": "1"}); err != nil {
		t.Fatalf("WriteJSONAtomicInRoot: %v", err)
	}
	_ = stateRoot.Close()

	instDir, err := ResolveInstanceWorkDir(workDir, uid, "tenant-a", "mc-1", true)
	if err != nil {
		t.Fatalf("ResolveInstanceWorkDir: %v", err)
	}
	mustWriteHostFile(t, filepath.Join(instDir, "world.dat"), "data", 0o644)

	if err := RemoveInstanceWorkDir(workDir, uid, "tenant-a", "mc-1"); err != nil {
		t.Fatalf("RemoveInstanceWorkDir: %v", err)
	}
	if _, err := os.Stat(instDir); !os.IsNotExist(err) {
		t.Fatalf("expected instance workspace removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "state", "instances", uid)); !os.IsNotExist(err) {
		t.Fatalf("expected external instance state removed, stat err=%v", err)
	}
}

func TestIsImmutableRegularCacheFileAndHardlinkRejection(t *testing.T) {
	dir := t.TempDir()
	roPath := filepath.Join(dir, "ro.bin")
	rwPath := filepath.Join(dir, "rw.bin")
	symPath := filepath.Join(dir, "sym.bin")
	linkPath := filepath.Join(dir, "ro-link.bin")
	if err := os.WriteFile(roPath, []byte("ro"), 0o444); err != nil {
		t.Fatalf("write ro: %v", err)
	}
	if err := os.WriteFile(rwPath, []byte("rw"), 0o644); err != nil {
		t.Fatalf("write rw: %v", err)
	}
	if err := os.Symlink(roPath, symPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	roInfo, _ := os.Lstat(roPath)
	rwInfo, _ := os.Lstat(rwPath)
	symInfo, _ := os.Lstat(symPath)
	if !IsImmutableRegularCacheFile(roInfo) || IsImmutableRegularCacheFile(rwInfo) || IsImmutableRegularCacheFile(symInfo) {
		t.Fatalf("unexpected IsImmutableRegularCacheFile results: ro=%v rw=%v sym=%v",
			IsImmutableRegularCacheFile(roInfo), IsImmutableRegularCacheFile(rwInfo), IsImmutableRegularCacheFile(symInfo))
	}
	if err := os.Link(roPath, linkPath); err != nil {
		t.Fatalf("hardlink: %v", err)
	}
	roLinkedInfo, _ := os.Lstat(roPath)
	if IsImmutableRegularCacheFile(roLinkedInfo) {
		t.Fatal("expected IsImmutableRegularCacheFile=false when hardlink count > 1")
	}

	wsDir := filepath.Join(dir, "ws")
	if err := os.MkdirAll(filepath.Join(wsDir, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir ws: %v", err)
	}
	dstFile := filepath.Join(wsDir, "bin", "target.bin")
	mustWriteHostFile(t, dstFile, "EXISTING", 0o644)
	if err := os.Link(dstFile, filepath.Join(wsDir, "bin", "extra.bin")); err != nil {
		t.Fatalf("hardlink dst: %v", err)
	}
	root, err := os.OpenRoot(wsDir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()
	if err := MaterializeFileInRoot(roPath, root, "bin/target.bin", MaterializeOptions{ForceReplace: false}); err == nil {
		t.Fatal("expected MaterializeFileInRoot to reject hard-linked existing destination")
	}
}

func TestProbeDirReflinkAndDisableOption(t *testing.T) {
	probeDir := filepath.Join(t.TempDir(), ".blob-cache")
	devID, supported, err := ProbeDirReflink(probeDir)
	if err != nil {
		t.Fatalf("ProbeDirReflink: %v", err)
	}
	gotDev, ok := DirDeviceID(probeDir)
	if ok && devID != gotDev {
		t.Fatalf("ProbeDirReflink dev=%d != DirDeviceID=%d", devID, gotDev)
	}
	if !supported {
		srcInfo, _ := os.Stat(probeDir)
		root, err := os.OpenRoot(probeDir)
		if err != nil {
			t.Fatal(err)
		}
		dstInfo, _ := root.Lstat(".")
		_ = root.Close()
		if !isFilePairReflinkUnsupported(srcInfo, dstInfo) {
			t.Fatal("expected unsupported ProbeDirReflink to cache device pair as unsupported")
		}
	}
	srcFile := filepath.Join(t.TempDir(), "src.bin")
	wsDir := t.TempDir()
	mustWriteHostFile(t, srcFile, "payload", 0o444)
	wsRoot, err := os.OpenRoot(wsDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = wsRoot.Close() }()
	if err := MaterializeFileInRoot(srcFile, wsRoot, "out.bin", MaterializeOptions{ForceReplace: true, DisableReflink: true}); err != nil {
		t.Fatalf("MaterializeFileInRoot(DisableReflink): %v", err)
	}
	assertIsolatedMaterializedFile(t, srcFile, filepath.Join(wsDir, "out.bin"), 0o644)
}

func runSymlinkSwapAttacker(workDir, outsideDir, outsideCanary string, stop *atomic.Bool, done chan struct{}) {
	defer close(done)
	swapDir := filepath.Join(workDir, "swap")
	swapLeaf := filepath.Join(swapDir, "leaf.txt")
	for !stop.Load() {
		_ = os.Remove(swapDir)
		_ = os.Symlink(outsideDir, swapDir)
		_ = os.Remove(swapDir)
		_ = os.MkdirAll(swapDir, 0o755)
		_ = os.Remove(swapLeaf)
		_ = os.Symlink(outsideCanary, swapLeaf)
	}
}

func assertIsolatedMaterializedFile(t *testing.T, srcPath, dstPath string, wantPerm os.FileMode) {
	t.Helper()
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		t.Fatalf("Stat(%s): %v", srcPath, err)
	}
	dstInfo, err := os.Lstat(dstPath)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", dstPath, err)
	}
	if os.SameFile(srcInfo, dstInfo) {
		t.Fatalf("%s shares inode with source cache %s", dstPath, srcPath)
	}
	if got := dstInfo.Mode().Perm(); got != wantPerm {
		t.Fatalf("%s perm = %o, want %o", dstPath, got, wantPerm)
	}
}

func assertOutsideDirUnchanged(t *testing.T, outsideDir, canaryPath, wantCanary string) {
	t.Helper()
	entries, err := os.ReadDir(outsideDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", outsideDir, err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(canaryPath) {
		t.Fatalf("outsideDir mutated unexpectedly: %v", entries)
	}
	if got := mustReadHostFile(t, canaryPath); got != wantCanary {
		t.Fatalf("outside canary content = %q, want %q", got, wantCanary)
	}
}

func mustWriteHostFile(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func mustReadHostFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(data)
}

func mutateFileInPlace(t *testing.T, path, replacement string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR|os.O_TRUNC, 0)
	if err != nil {
		t.Fatalf("OpenFile(%s): %v", path, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(replacement); err != nil {
		t.Fatalf("WriteString(%s): %v", path, err)
	}
}
