package nodeexecution

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type memoryTransfer struct {
	data      []byte
	uploads   int
	downloads int
}

func TestRestoreRejectsArchiveTraversal(t *testing.T) {
	root, _ := NewScopedRoot(t.TempDir())
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte("escape")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "../../outside", Mode: 0o640, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	transfer := &memoryTransfer{data: archive.Bytes()}
	if err := root.Restore(context.Background(), transfer, RestoreJob{TargetRelative: "instances/lin_test/world", SignedDownloadURL: "memory://malicious"}); err != ErrPathOutsideRoot {
		t.Fatalf("malicious restore error=%v", err)
	}
}

func (m *memoryTransfer) Upload(_ context.Context, _ string, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err == nil {
		m.data, m.uploads = data, m.uploads+1
	}
	return err
}
func (m *memoryTransfer) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	m.downloads++
	return io.NopCloser(bytes.NewReader(m.data)), nil
}

func TestBackupTransfersDirectlyAndRestoreStaysWithinRegionRoot(t *testing.T) {
	rootPath := t.TempDir()
	root, err := NewScopedRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	source, _ := root.Resolve("instances/lin_test/world")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "world.wld"), []byte("world-data"), 0o640); err != nil {
		t.Fatal(err)
	}
	transfer := &memoryTransfer{}
	result, err := root.Backup(context.Background(), transfer, BackupJob{SourceRelative: "instances/lin_test/world", SignedUploadURL: "memory://region/backups/one"})
	if err != nil || result.SizeBytes == 0 || result.Checksum == "" || transfer.uploads != 1 {
		t.Fatalf("backup=%#v uploads=%d error=%v", result, transfer.uploads, err)
	}
	if err := root.Restore(context.Background(), transfer, RestoreJob{TargetRelative: "instances/lin_restore/world", SignedDownloadURL: "memory://region/backups/one"}); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(filepath.Join(rootPath, "instances/lin_restore/world/world.wld"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != "world-data" || transfer.downloads != 1 {
		t.Fatalf("restored=%q downloads=%d", restored, transfer.downloads)
	}
}

func TestScopedRootRejectsTraversalAbsoluteAndSymlinkPaths(t *testing.T) {
	rootPath := t.TempDir()
	root, _ := NewScopedRoot(rootPath)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(rootPath, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{"../outside", "/tmp/outside", "escape/file"} {
		if _, err := root.Resolve(candidate); err != ErrPathOutsideRoot {
			t.Fatalf("Resolve(%q) error=%v", candidate, err)
		}
	}
}
