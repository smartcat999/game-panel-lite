package backup

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateBackupZip(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "instance")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "serverconfig.txt"), []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, size, err := NewService(root).Create("srv", source)
	if err != nil {
		t.Fatal(err)
	}
	if size == 0 || filepath.Ext(path) != ".zip" {
		t.Fatalf("expected zip backup, path=%s size=%d", path, size)
	}
}

func TestCreateBackupUsesUniqueNamesForRapidBackups(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "instance")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "serverconfig.txt"), []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	first, _, err := service.Create("srv", source)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := service.Create("srv", source)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("expected rapid backups to use unique paths, got %q", first)
	}
}

func TestRestoreBackupExtractsFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "instance")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "serverconfig.txt"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	backupPath, _, err := service.Create("srv", source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "serverconfig.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.Restore("srv", filepath.Base(backupPath), source); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(source, "serverconfig.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("expected restored content, got %q", string(got))
	}
}

func TestRestoreRejectsZipSlip(t *testing.T) {
	root := t.TempDir()
	backupDir := filepath.Join(root, "backups", "srv")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(backupDir, "bad.zip")
	out, err := os.Create(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	zipper := zip.NewWriter(out)
	writer, err := zipper.Create("../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("escape")); err != nil {
		t.Fatal(err)
	}
	if err := zipper.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	err = NewService(root).Restore("srv", "bad.zip", filepath.Join(root, "instance"))
	if err == nil {
		t.Fatal("expected zip slip restore to fail")
	}
}
