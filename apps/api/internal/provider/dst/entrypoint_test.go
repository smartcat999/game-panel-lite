package dst

import (
	"archive/zip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDSTEntrypointReusesCompleteWorkshopCacheOnStart(t *testing.T) {
	root, dataDir, script := dstEntrypointFixture(t, true)
	for _, id := range []string{"111", "222"} {
		writeTestFile(t, filepath.Join(dataDir, "ugc_mods", "content", "322330", id, "modinfo.lua"), "cached")
	}

	output, err := runDSTEntrypoint(t, root, dataDir, script, "reuse")
	if err != nil {
		t.Fatalf("run entrypoint: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Reusing verified GamePanel DST Workshop cache.") {
		t.Fatalf("expected cache reuse message, got:\n%s", output)
	}
	log := readTestFile(t, filepath.Join(dataDir, "fake-server.log"))
	if strings.Contains(log, "-only_update_server_mods") {
		t.Fatalf("expected start to skip mod download, got %q", log)
	}
	if strings.Contains(log, "-console") {
		t.Fatalf("expected shard startup to omit deprecated -console, got %q", log)
	}
	if !strings.Contains(log, "-skip_update_server_mods") {
		t.Fatalf("expected shard startup to skip duplicate Workshop updates, got %q", log)
	}
}

func TestDSTEntrypointRefreshesAllWorkshopModsOnRestart(t *testing.T) {
	root, dataDir, script := dstEntrypointFixture(t, true)
	writeTestFile(t, filepath.Join(dataDir, "ugc_mods", "old-cache"), "old")

	output, err := runDSTEntrypoint(t, root, dataDir, script, "refresh")
	if err != nil {
		t.Fatalf("run entrypoint: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Refreshed and verified all GamePanel DST server mods.") {
		t.Fatalf("expected refresh success message, got:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "ugc_mods", "old-cache")); !os.IsNotExist(err) {
		t.Fatalf("expected old cache to be atomically replaced, stat err=%v", err)
	}
	for _, id := range []string{"111", "222"} {
		if _, err := os.Stat(filepath.Join(dataDir, "ugc_mods", "content", "322330", id, "modinfo.lua")); err != nil {
			t.Fatalf("expected refreshed mod %s: %v", id, err)
		}
	}
}

func TestDSTEntrypointPreservesOldCacheWhenRefreshIsIncomplete(t *testing.T) {
	root, dataDir, script := dstEntrypointFixture(t, false)
	writeTestFile(t, filepath.Join(dataDir, "ugc_mods", "old-cache"), "old")

	output, err := runDSTEntrypoint(t, root, dataDir, script, "refresh")
	if err == nil {
		t.Fatalf("expected incomplete refresh to fail, got:\n%s", output)
	}
	if !strings.Contains(output, "DST Workshop refresh failed; missing IDs: 111, 222") {
		t.Fatalf("expected missing mod IDs, got:\n%s", output)
	}
	if got := readTestFile(t, filepath.Join(dataDir, "ugc_mods", "old-cache")); got != "old" {
		t.Fatalf("expected old cache to remain intact, got %q", got)
	}
}

func TestDSTEntrypointDownloadsAndLinksLegacyWorkshopMod(t *testing.T) {
	root, dataDir, script := dstEntrypointFixture(t, false)
	clusterDir := filepath.Join(dataDir, "dst", "GamePanelLite")
	writeTestFile(t, filepath.Join(clusterDir, "dedicated_server_mods_setup.lua"), "ServerModSetup(\"111\")\n")

	archive := filepath.Join(root, "legacy-mod.zip")
	writeTestZip(t, archive, map[string]string{
		"modinfo.lua":                "name = \"Legacy Test\"\n",
		"modmain.lua":                "return nil\n",
		"scripts\\components\\x.lua": "return nil\n",
	})
	binDir := filepath.Join(root, "fake-bin")
	fakeCurl := `#!/usr/bin/env bash
set -euo pipefail
if [[ "$*" == *"GetPublishedFileDetails"* ]]; then
  printf '{"response":{"result":1,"publishedfiledetails":[{"result":1,"file_url":"file://%s"}]}}' "${FAKE_LEGACY_ARCHIVE}"
  exit 0
fi
out=""
url=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    file://*) url="$1"; shift ;;
    *) shift ;;
  esac
done
cp "${url#file://}" "${out}"
`
	writeTestFile(t, filepath.Join(binDir, "curl"), fakeCurl)
	if err := os.Chmod(filepath.Join(binDir, "curl"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_LEGACY_ARCHIVE", archive)
	t.Setenv("DST_LEGACY_WORKSHOP_FALLBACK", "1")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	output, err := runDSTEntrypoint(t, root, dataDir, script, "refresh")
	if err != nil {
		t.Fatalf("run entrypoint: %v\n%s", err, output)
	}
	legacyDir := filepath.Join(dataDir, "ugc_mods", "legacy", "workshop-111")
	if _, err := os.Stat(filepath.Join(legacyDir, "modinfo.lua")); err != nil {
		t.Fatalf("expected persisted legacy mod: %v\n%s", err, output)
	}
	link := filepath.Join(root, "server", "mods", "workshop-111")
	if target, err := os.Readlink(link); err != nil || target != legacyDir {
		t.Fatalf("expected legacy runtime link to %q, got target=%q err=%v", legacyDir, target, err)
	}
	log := readTestFile(t, filepath.Join(dataDir, "fake-server.log"))
	if strings.Contains(log, "-only_update_server_mods") {
		t.Fatalf("expected legacy-only refresh to bypass native downloader, got %q", log)
	}
}

func dstEntrypointFixture(t *testing.T, downloaderCreatesMods bool) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	clusterDir := filepath.Join(dataDir, "dst", "GamePanelLite")
	writeTestFile(t, filepath.Join(clusterDir, "cluster_token.txt"), "token")
	writeTestFile(t, filepath.Join(clusterDir, "cluster.ini"), "[GAMEPLAY]\n")
	writeTestFile(t, filepath.Join(clusterDir, "dedicated_server_mods_setup.lua"), "ServerModSetup(\"111\")\nServerModSetup(\"222\")\n")

	fake := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${DST_PERSISTENT_ROOT}/fake-server.log"
if [[ " $* " == *" -only_update_server_mods "* ]]; then
  ugc=""
  while [[ $# -gt 0 ]]; do
    if [[ "$1" == "-ugc_directory" ]]; then ugc="$2"; break; fi
    shift
  done
  if [[ "${FAKE_DOWNLOAD_MODS:-0}" == "1" ]]; then
    for id in 111 222; do
      mkdir -p "${ugc}/content/322330/${id}"
      printf 'mod' > "${ugc}/content/322330/${id}/modinfo.lua"
    done
  fi
fi
`
	writeTestFile(t, filepath.Join(root, "server", "bin64", "dontstarve_dedicated_server_nullrenderer_x64"), fake)
	if err := os.Chmod(filepath.Join(root, "server", "bin64", "dontstarve_dedicated_server_nullrenderer_x64"), 0o755); err != nil {
		t.Fatal(err)
	}

	script, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "docker", "dst", "gamepanel-dst-entrypoint.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("locate entrypoint: %v", err)
	}
	if downloaderCreatesMods {
		t.Setenv("FAKE_DOWNLOAD_MODS", "1")
	} else {
		t.Setenv("FAKE_DOWNLOAD_MODS", "0")
	}
	t.Setenv("DST_LEGACY_WORKSHOP_FALLBACK", "0")
	return root, dataDir, script
}

func runDSTEntrypoint(t *testing.T, root, dataDir, script, mode string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"DST_ROOT_DIR="+root,
		"DST_PERSISTENT_ROOT="+dataDir,
		"DST_CONF_DIR=dst",
		"DST_CLUSTER_NAME=GamePanelLite",
		"DST_MOD_SYNC_MODE="+mode,
	)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func writeTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
