package volume

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type testArchiveEntry struct {
	name     string
	body     string
	isDir    bool
	typeflag byte
}

func buildTestZip(t *testing.T, entries []testArchiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		name := e.name
		if e.isDir && !strings.HasSuffix(name, "/") {
			name += "/"
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", e.name, err)
		}
		if !e.isDir {
			if _, err := w.Write([]byte(e.body)); err != nil {
				t.Fatalf("write zip body %q: %v", e.name, err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func buildTestTarGz(t *testing.T, entries []testArchiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		tf := e.typeflag
		mode := int64(0o644)
		if tf == 0 {
			tf = tar.TypeReg
			if e.isDir {
				tf = tar.TypeDir
				mode = 0o755
			}
		}
		hdr := &tar.Header{Name: e.name, Typeflag: tf, Mode: mode, Size: int64(len(e.body))}
		if tf == tar.TypeSymlink {
			hdr.Linkname = "/etc/passwd"
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %q: %v", e.name, err)
		}
		if tf == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("write tar body %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestInspectArchiveStreamDSTAndTModLoader(t *testing.T) {
	dstZip := buildTestZip(t, []testArchiveEntry{
		{name: "MyDediCluster/cluster.ini", body: "[MISC]\n"},
		{name: "MyDediCluster/Master/server.ini", body: "is_master=true\n"},
		{name: "MyDediCluster/Caves/server.ini", body: "is_master=false\n"},
	})
	res, err := InspectArchiveStream(context.Background(), bytes.NewReader(dstZip), 10<<20, ArchiveInspectOptions{
		DefaultTarget: "DoNotStarveTogether/Cluster_1",
		AnchorFiles:   []string{"cluster.ini", "Master/server.ini"},
	})
	if err != nil {
		t.Fatalf("inspect dst zip: %v", err)
	}
	if res.Format != "zip" || res.RecommendedSource != "MyDediCluster" || !reflect.DeepEqual(res.CandidateSources, []string{"MyDediCluster"}) {
		t.Fatalf("unexpected dst inspection: %+v", res)
	}

	tmodTar := buildTestTarGz(t, []testArchiveEntry{
		{name: "tModLoader/Worlds", isDir: true},
		{name: "tModLoader/Worlds/foo.wld", body: "world"},
	})
	res2, err := InspectArchiveStream(context.Background(), bytes.NewReader(tmodTar), 10<<20, ArchiveInspectOptions{
		DefaultTarget: ".",
		AnchorFiles:   []string{"Worlds"},
	})
	if err != nil {
		t.Fatalf("inspect tmod tar.gz: %v", err)
	}
	if res2.Format != "tar.gz" || res2.RecommendedSource != "tModLoader" {
		t.Fatalf("unexpected tmod inspection: %+v", res2)
	}
}

func TestInspectArchiveStreamMultiCandidateAmbiguity(t *testing.T) {
	multi := buildTestZip(t, []testArchiveEntry{
		{name: "ClusterA/cluster.ini", body: "a"},
		{name: "ClusterB/cluster.ini", body: "b"},
	})
	res, err := InspectArchiveStream(context.Background(), bytes.NewReader(multi), 10<<20, ArchiveInspectOptions{
		DefaultTarget: "DoNotStarveTogether/Cluster_1",
		AnchorFiles:   []string{"cluster.ini", "Master/server.ini"},
	})
	if err != nil {
		t.Fatalf("inspect multi-cluster zip: %v", err)
	}
	if res.RecommendedSource != "" {
		t.Fatalf("expected empty RecommendedSource for ambiguous archive, got %q", res.RecommendedSource)
	}
	if !reflect.DeepEqual(res.CandidateSources, []string{"ClusterA", "ClusterB"}) {
		t.Fatalf("unexpected candidates: %v", res.CandidateSources)
	}
}

func TestStageAndPromoteZipMappedWithReplaceAndBackslashes(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "DoNotStarveTogether/Cluster_1/stale.txt"), "old")
	mustWriteFile(t, filepath.Join(dir, "DoNotStarveTogether/keep.txt"), "keep")

	zipData := buildTestZip(t, []testArchiveEntry{
		{name: "MyDediCluster/cluster.ini", body: "new-cluster"},
		{name: `MyDediCluster\images\back.tex`, body: "tex-bytes"},
	})
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer func() { _ = root.Close() }()

	staged, err := StageArchiveToRoot(context.Background(), root, bytes.NewReader(zipData), 10<<20, RestoreOptions{
		SourceSubpath: "MyDediCluster",
		TargetSubpath: "DoNotStarveTogether/Cluster_1",
		ReplaceTarget: true,
	})
	if err != nil {
		t.Fatalf("stage zip: %v", err)
	}
	defer func() { _ = staged.Close() }()
	if err := staged.Promote(context.Background()); err != nil {
		t.Fatalf("promote staged restore: %v", err)
	}

	assertFileContent(t, filepath.Join(dir, "DoNotStarveTogether/Cluster_1/cluster.ini"), "new-cluster")
	assertFileContent(t, filepath.Join(dir, "DoNotStarveTogether/Cluster_1/images/back.tex"), "tex-bytes")
	assertFileContent(t, filepath.Join(dir, "DoNotStarveTogether/keep.txt"), "keep")
	if _, err := os.Stat(filepath.Join(dir, "DoNotStarveTogether/Cluster_1/stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale.txt removed under ReplaceTarget, stat err=%v", err)
	}
}

func TestStageArchiveRejectsTraversalAndCollisions(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"zip-backslash-traversal", buildTestZip(t, []testArchiveEntry{{name: `..\evil.txt`, body: "x"}})},
		{"zip-drive-prefix", buildTestZip(t, []testArchiveEntry{{name: `C:\evil.txt`, body: "x"}})},
		{"zip-abs", buildTestZip(t, []testArchiveEntry{{name: `/etc/passwd`, body: "x"}})},
		{"zip-collision", buildTestZip(t, []testArchiveEntry{{name: "foo", body: "x"}, {name: "foo/bar", body: "y"}})},
		{"tar-symlink", buildTestTarGz(t, []testArchiveEntry{{name: "link", typeflag: tar.TypeSymlink}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatalf("open root: %v", err)
			}
			defer func() { _ = root.Close() }()
			if _, err := StageArchiveToRoot(context.Background(), root, bytes.NewReader(tc.data), 10<<20, RestoreOptions{}); err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestStageArchiveRejectsMissingSourceLeavesWorkspaceUntouched(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "DoNotStarveTogether/Cluster_1/cluster.ini"), "intact")
	zipData := buildTestZip(t, []testArchiveEntry{{name: "Other/cluster.ini", body: "other"}})

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer func() { _ = root.Close() }()

	_, err = StageArchiveToRoot(context.Background(), root, bytes.NewReader(zipData), 10<<20, RestoreOptions{
		SourceSubpath: "MissingDir",
		TargetSubpath: "DoNotStarveTogether/Cluster_1",
		ReplaceTarget: true,
	})
	if err == nil {
		t.Fatal("expected error for missing sourcePath")
	}
	assertFileContent(t, filepath.Join(dir, "DoNotStarveTogether/Cluster_1/cluster.ini"), "intact")
}

func TestAllowsRestoreMapping(t *testing.T) {
	claims := &FileTicketClaims{
		Action:        "restore",
		SourcePath:    "MyCluster",
		TargetPath:    "DoNotStarveTogether/Cluster_1",
		ReplaceTarget: true,
	}
	if err := claims.AllowsRestoreMapping("MyCluster", "DoNotStarveTogether/Cluster_1", true); err != nil {
		t.Fatalf("expected matching mapping allowed, got %v", err)
	}
	if err := claims.AllowsRestoreMapping("OtherCluster", "DoNotStarveTogether/Cluster_1", true); err == nil {
		t.Fatal("expected mismatched sourcePath error")
	}
	if err := claims.AllowsRestoreMapping("MyCluster", "OtherTarget", true); err == nil {
		t.Fatal("expected mismatched targetPath error")
	}
	if err := claims.AllowsRestoreMapping("MyCluster", "DoNotStarveTogether/Cluster_1", false); err == nil {
		t.Fatal("expected mismatched replaceTarget error")
	}
}

func TestInspectArchiveStreamTextCaptureAndImmediateEntries(t *testing.T) {
	iniBody := "\xef\xbb\xbf[NETWORK]\ncluster_name = Test Cluster\n"
	zipData := buildTestZip(t, []testArchiveEntry{
		{name: "ClusterA", isDir: true},
		{name: "ClusterA/cluster.ini", body: iniBody},
		{name: "ClusterA/cluster_token.txt", body: "pds-tok-1"},
		{name: "ClusterA/Master/server.ini", body: "[SHARD]\nis_master = true\n"},
		{name: "ClusterA/Caves/save/world.bin", body: "binary"},
		{name: "ClusterA/corrupt.ini", body: "bad\x00bytes"},
	})
	res, err := InspectArchiveStream(context.Background(), bytes.NewReader(zipData), 10<<20, ArchiveInspectOptions{
		DefaultTarget: "DoNotStarveTogether/Cluster_1",
		AnchorFiles:   []string{"cluster.ini"},
	})
	if err != nil {
		t.Fatalf("inspect zip failed: %v", err)
	}
	if res.FileCount != 5 || res.UncompressedBytes <= 0 {
		t.Fatalf("unexpected counts: fileCount=%d uncompressed=%d", res.FileCount, res.UncompressedBytes)
	}
	if res.TextFiles["ClusterA/cluster.ini"] != "[NETWORK]\ncluster_name = Test Cluster\n" {
		t.Fatalf("BOM-stripped text not captured: %q", res.TextFiles["ClusterA/cluster.ini"])
	}
	if _, hasCorrupt := res.TextFiles["ClusterA/corrupt.ini"]; hasCorrupt {
		t.Fatal("expected NUL-byte text entry to be skipped")
	}
	if _, hasBin := res.TextFiles["ClusterA/Caves/save/world.bin"]; hasBin {
		t.Fatal("expected .bin file not captured as text")
	}
	wantChildren := []string{"Caves/", "Master/", "cluster.ini", "cluster_token.txt", "corrupt.ini"}
	gotChildren := ListArchiveImmediateEntries(res.Files, res.Directories, "ClusterA", 12)
	if !reflect.DeepEqual(gotChildren, wantChildren) {
		t.Fatalf("ListArchiveImmediateEntries = %v, want %v", gotChildren, wantChildren)
	}

	tarData := buildTestTarGz(t, []testArchiveEntry{
		{name: "server.properties", body: "motd=Hello\nmax-players=12\n"},
		{name: "world/region/r.0.0.mca", body: "chunk"},
	})
	tarRes, err := InspectArchiveStream(context.Background(), bytes.NewReader(tarData), 10<<20, ArchiveInspectOptions{})
	if err != nil {
		t.Fatalf("inspect tar.gz failed: %v", err)
	}
	if tarRes.FileCount != 2 || tarRes.TextFiles["server.properties"] != "motd=Hello\nmax-players=12\n" {
		t.Fatalf("unexpected tar.gz text inspection: %+v", tarRes)
	}
	if got := ListArchiveImmediateEntries(tarRes.Files, tarRes.Directories, ".", 12); !reflect.DeepEqual(got, []string{"world/", "server.properties"}) {
		t.Fatalf("root immediate entries = %v", got)
	}
}

func mustWriteFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

func assertFileContent(t *testing.T, p, want string) {
	t.Helper()
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	if string(got) != want {
		t.Fatalf("file %s = %q, want %q", p, string(got), want)
	}
}

func TestTarGzDecompressionBombs(t *testing.T) {
	ctx := context.Background()
	bombTar := buildTestTarGz(t, []testArchiveEntry{
		{name: "world/level.dat", body: "ok"},
		{name: "other/huge.bin", body: strings.Repeat("A", 64*1024)},
	})
	if _, err := InspectArchiveStream(ctx, bytes.NewReader(bombTar), 8*1024, ArchiveInspectOptions{}); !errors.Is(err, ErrFileQuotaExceeded) {
		t.Fatalf("expected ErrFileQuotaExceeded on inspect unmapped bomb, got %v", err)
	}
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()
	if _, err := StageArchiveToRoot(ctx, root, bytes.NewReader(bombTar), 8*1024, RestoreOptions{SourceSubpath: "world", TargetSubpath: "world"}); !errors.Is(err, ErrFileQuotaExceeded) {
		t.Fatalf("expected ErrFileQuotaExceeded on stage unmapped bomb, got %v", err)
	}

	var trailerBuf bytes.Buffer
	trailerBuf.Write(buildTestTarGz(t, []testArchiveEntry{{name: "world/level.dat", body: "ok"}}))
	gz := gzip.NewWriter(&trailerBuf)
	if _, err := gz.Write(make([]byte, maxGzipTrailerBytes+64*1024)); err != nil {
		t.Fatalf("write trailing bomb member: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close trailing bomb member: %v", err)
	}
	if _, err := InspectArchiveStream(ctx, bytes.NewReader(trailerBuf.Bytes()), 64<<20, ArchiveInspectOptions{}); !errors.Is(err, ErrFileQuotaExceeded) {
		t.Fatalf("expected ErrFileQuotaExceeded on inspect trailing gzip bomb, got %v", err)
	}
	if _, err := StageArchiveToRoot(ctx, root, bytes.NewReader(trailerBuf.Bytes()), 64<<20, RestoreOptions{SourceSubpath: ".", TargetSubpath: "."}); !errors.Is(err, ErrFileQuotaExceeded) {
		t.Fatalf("expected ErrFileQuotaExceeded on stage trailing gzip bomb, got %v", err)
	}
}

func TestReservedMetadataPathProtection(t *testing.T) {
	ctx := context.Background()
	hostDir := t.TempDir()
	root, err := os.OpenRoot(hostDir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = root.Close() }()

	if _, err := WriteStreamInRoot(root, ".gp-addons-manifest.json", strings.NewReader("{}"), 1024); err == nil {
		t.Fatal("expected WriteStreamInRoot to reject .gp-addons-manifest.json")
	}
	if err := RemoveAllInRoot(root, ".gp-addons-manifest.json"); err == nil {
		t.Fatal("expected RemoveAllInRoot to reject .gp-addons-manifest.json")
	}
	mustWriteFile(t, filepath.Join(hostDir, ".gp-addons-manifest.json"), "host-secret")
	if _, err := WriteStreamInRoot(root, "world/level.dat", strings.NewReader("v1"), 1024); err != nil {
		t.Fatalf("WriteStreamInRoot world/level.dat: %v", err)
	}
	entries, err := ListEntriesInRoot(root, ".")
	if err != nil || len(entries) != 1 || entries[0].Name != "world" {
		t.Fatalf("expected ListEntriesInRoot to hide .gp-* files, got %+v err=%v", entries, err)
	}
	var archBuf bytes.Buffer
	if _, _, err := ArchivePathsToTarGz(root, nil, &archBuf, 1<<20); err != nil {
		t.Fatalf("ArchivePathsToTarGz: %v", err)
	}
	archInspect, err := InspectArchiveStream(ctx, bytes.NewReader(archBuf.Bytes()), 1<<20, ArchiveInspectOptions{})
	if err != nil || archInspect.FileCount != 1 || !reflect.DeepEqual(archInspect.Files, []string{"world/level.dat"}) {
		t.Fatalf("expected archive to exclude .gp-*, got %+v err=%v", archInspect, err)
	}

	crafted := buildTestTarGz(t, []testArchiveEntry{
		{name: ".gp-addons-manifest.json", body: "attacker"},
		{name: "world/level.dat", body: "restored"},
	})
	inspected, err := InspectArchiveStream(ctx, bytes.NewReader(crafted), 1<<20, ArchiveInspectOptions{})
	if err != nil || inspected.FileCount != 1 || !reflect.DeepEqual(inspected.Files, []string{"world/level.dat"}) {
		t.Fatalf("expected crafted inspect to skip .gp-*, got %+v err=%v", inspected, err)
	}
	staged, err := StageArchiveToRoot(ctx, root, bytes.NewReader(crafted), 1<<20, RestoreOptions{SourceSubpath: ".", TargetSubpath: "."})
	if err != nil {
		t.Fatalf("StageArchiveToRoot: %v", err)
	}
	if err := staged.Promote(ctx); err != nil {
		t.Fatalf("Promote: %v", err)
	}
	assertFileContent(t, filepath.Join(hostDir, "world/level.dat"), "restored")
	assertFileContent(t, filepath.Join(hostDir, ".gp-addons-manifest.json"), "host-secret")
}
