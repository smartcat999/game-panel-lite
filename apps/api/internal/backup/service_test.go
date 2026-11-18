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

func TestCreateSubtreePreservesPathRelativeToDataRoot(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "instance")
	savedDir := filepath.Join(dataDir, "Pal", "Saved")
	if err := os.MkdirAll(savedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(savedDir, "world.sav"), []byte("palworld-save"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "PalServer.sh"), []byte("large-runtime-file"), 0o700); err != nil {
		t.Fatal(err)
	}

	service := NewService(filepath.Join(root, "panel-data"))
	path, _, err := service.CreateSubtree("server-1", dataDir, filepath.Join("Pal", "Saved"))
	if err != nil {
		t.Fatalf("create save-only backup: %v", err)
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != 1 || reader.File[0].Name != "Pal/Saved/world.sav" {
		t.Fatalf("expected only the save subtree with its original path, got %#v", reader.File)
	}

	restoreDir := filepath.Join(root, "restored")
	if err := service.Restore("server-1", filepath.Base(path), restoreDir); err != nil {
		t.Fatalf("restore save-only backup: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(restoreDir, "Pal", "Saved", "world.sav"))
	if err != nil || string(content) != "palworld-save" {
		t.Fatalf("expected save restored to Pal/Saved, content=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(restoreDir, "PalServer.sh")); !os.IsNotExist(err) {
		t.Fatalf("runtime files must not be included in save-only backup, stat err=%v", err)
	}
}

func TestCreateSubtreeRejectsTraversal(t *testing.T) {
	service := NewService(t.TempDir())
	if _, _, err := service.CreateSubtree("server-1", t.TempDir(), "../outside"); err == nil {
		t.Fatal("expected traversal subtree to be rejected")
	}
}

func TestCreateSubtreeRejectsSymlinkedParentOutsideDataRoot(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "instance")
	outsideSaved := filepath.Join(root, "outside", "Saved")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideSaved, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideSaved, "secret.sav"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outsideSaved), filepath.Join(dataDir, "Pal")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewService(filepath.Join(root, "panel-data")).CreateSubtree("server-1", dataDir, filepath.Join("Pal", "Saved")); err == nil {
		t.Fatal("expected a subtree whose parent symlink escapes the data root to be rejected")
	}
}

func TestCreateRejectsSymbolicLinks(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "instance")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewService(filepath.Join(root, "panel-data")).Create("server-1", source); err == nil {
		t.Fatal("expected backup containing a symbolic link to be rejected")
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
