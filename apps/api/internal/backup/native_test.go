package backup

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeDirectoryZIPAndReplacementRestore(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "instance")
	cluster := filepath.Join("dst", "原生集群")
	for _, rel := range []string{"cluster.ini", "Master/save/session/world/0001", "Caves/save/session/cave/0001", "Master/modoverrides.lua"} {
		p := filepath.Join(data, cluster, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(rel), 0600); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(data, "dst", "cached_mod_manifests")
	os.MkdirAll(other, 0755)
	os.WriteFile(filepath.Join(other, "cache"), []byte("not a world"), 0600)
	svc := NewService(root)
	archive, _, err := svc.CreateDirectory("srv", data, cluster)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(z.File) != 4 {
		t.Fatalf("expected only four native files, got %d", len(z.File))
	}
	for _, f := range z.File {
		if len(f.Name) < len("原生集群/") || f.Name[:len("原生集群/")] != "原生集群/" {
			t.Fatalf("panel prefix leaked: %s", f.Name)
		}
	}
	z.Close()
	stale := filepath.Join(data, cluster, "Master/save/session/world/0002")
	os.WriteFile(stale, []byte("later snapshot"), 0600)
	os.WriteFile(filepath.Join(data, cluster, "cluster.ini"), []byte("changed"), 0600)
	if err := svc.RestoreDirectory("srv", filepath.Base(archive), data, cluster); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("newer snapshot survived rollback: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(data, cluster, "cluster.ini"))
	if err != nil || string(b) != "cluster.ini" {
		t.Fatalf("native restore failed: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(other, "cache")); err != nil {
		t.Fatalf("unrelated cache removed: %v", err)
	}
}

func TestNativeRestoreAcceptsLegacyPanelZIP(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "instance")
	rel := "dst/Cluster/Master/save/world"
	os.MkdirAll(filepath.Dir(filepath.Join(data, rel)), 0755)
	os.WriteFile(filepath.Join(data, rel), []byte("legacy"), 0600)
	os.WriteFile(filepath.Join(data, "old-panel-file"), []byte("legacy auxiliary file"), 0600)
	svc := NewService(root)
	archive, _, err := svc.Create("srv", data)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "restored")
	if err := svc.RestoreDirectory("srv", filepath.Base(archive), dest, "dst/Cluster"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dest, rel))
	if err != nil || string(b) != "legacy" {
		t.Fatalf("legacy not restored: %q %v", b, err)
	}
}

func TestNativeRestoreRejectsUnsafeAndMixedArchivesWithoutChangingSave(t *testing.T) {
	for _, name := range []string{"Cluster/../../outside", "Cluster/../outside", "Cluster\\Master\\save", "other/save", "/absolute", "Cluster/save/../../escape"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			svc := NewService(root)
			data := filepath.Join(root, "instance")
			os.MkdirAll(filepath.Join(data, "dst/Cluster"), 0755)
			sentinel := filepath.Join(data, "dst/Cluster/original")
			os.WriteFile(sentinel, []byte("keep"), 0600)
			dir := filepath.Join(root, "backups/srv")
			os.MkdirAll(dir, 0755)
			f, _ := os.Create(filepath.Join(dir, "bad.zip"))
			z := zip.NewWriter(f)
			w, _ := z.Create(name)
			w.Write([]byte("bad"))
			z.Close()
			f.Close()
			if err := svc.RestoreDirectory("srv", "bad.zip", data, "dst/Cluster"); err == nil {
				t.Fatal("expected rejection")
			}
			if b, err := os.ReadFile(sentinel); err != nil || string(b) != "keep" {
				t.Fatal("failed restore changed save")
			}
		})
	}
}

func TestNativeRestoreCannotFollowParentSymlinkOutsideInstance(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source")
	os.MkdirAll(filepath.Join(src, "dst/Cluster"), 0755)
	os.WriteFile(filepath.Join(src, "dst/Cluster/world"), []byte("new"), 0600)
	svc := NewService(root)
	archive, _, err := svc.CreateDirectory("srv", src, "dst/Cluster")
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "target")
	outside := filepath.Join(root, "outside")
	os.MkdirAll(dest, 0755)
	os.MkdirAll(outside, 0755)
	os.Symlink(outside, filepath.Join(dest, "dst"))
	if err := svc.RestoreDirectory("srv", filepath.Base(archive), dest, "dst/Cluster"); err == nil {
		t.Fatal("expected symlink rejection")
	}
	if _, err := os.Stat(filepath.Join(outside, "Cluster")); !os.IsNotExist(err) {
		t.Fatal("restore wrote outside instance")
	}
}
