package backup

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func restoreFixture(t *testing.T, names, values []string, corrupt bool) []*zip.File {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for index, name := range names {
		file, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(values[index])); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	payload := buffer.Bytes()
	if corrupt {
		at := bytes.Index(payload, []byte(values[len(values)-1]))
		if at < 0 {
			t.Fatal("fixture payload missing")
		}
		payload[at] ^= 0xff
	}
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	return reader.File
}

func TestRestoreValidatesEntireArchiveBeforeReplacingFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "world"), []byte("original"), 0640); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	files := restoreFixture(t, []string{"world", "later"}, []string{"replacement", "unique corrupt payload"}, true)
	if err := restoreFiles(root, files); err == nil {
		t.Fatal("corrupt archive accepted")
	}
	got, err := os.ReadFile(filepath.Join(dir, "world"))
	if err != nil || string(got) != "original" {
		t.Fatalf("original changed: %q %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("staging leaked: %v %v", entries, err)
	}
}

func TestRestoreRollsBackPublishedFilesOnLaterConflict(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "world"), []byte("original"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "blocked"), 0755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	files := restoreFixture(t, []string{"world", "new-file", "blocked"}, []string{"replacement", "new", "conflict"}, false)
	if err := restoreFiles(root, files); err == nil {
		t.Fatal("target directory accepted as file")
	}
	got, err := os.ReadFile(filepath.Join(dir, "world"))
	if err != nil || string(got) != "original" {
		t.Fatalf("rollback failed: %q %v", got, err)
	}
	info, err := os.Stat(filepath.Join(dir, "world"))
	if err != nil || info.Mode().Perm() != 0640 {
		t.Fatalf("original permissions lost: %v %v", info, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("new files or staging leaked: %v %v", entries, err)
	}
}

func TestRestoreRejectsDuplicatePathsBeforeMutation(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	files := restoreFixture(t, []string{"world", "./world"}, []string{"one", "two"}, false)
	if err := restoreFiles(root, files); err == nil {
		t.Fatal("duplicate normalized paths accepted")
	}
	if _, err := root.Stat("world"); !os.IsNotExist(err) {
		t.Fatalf("target modified: %v", err)
	}
}
