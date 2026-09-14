package dst

import (
	"archive/zip"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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
	if _, err := os.Stat(filepath.Join(dataDir, "fake-steamcmd.log")); !os.IsNotExist(err) {
		t.Fatalf("expected ordinary start to reuse game files without SteamCMD, stat err=%v", err)
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
	steamLog := readTestFile(t, filepath.Join(dataDir, "fake-steamcmd.log"))
	if !strings.Contains(steamLog, "+app_update 343050") {
		t.Fatalf("expected restart to update DST game files, got %q", steamLog)
	}
	if strings.Contains(steamLog, "validate") {
		t.Fatalf("expected restart update to avoid an expensive full validation, got %q", steamLog)
	}
	if got := strings.TrimSpace(readTestFile(t, filepath.Join(dataDir, ".gamepanel", "dst-build-id"))); got != "24700372" {
		t.Fatalf("expected installed build marker, got %q", got)
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
	if _, err := os.Stat(filepath.Join(dataDir, "ugc_mods.refresh")); err != nil {
		t.Fatalf("expected partial refresh cache to be preserved: %v", err)
	}
}

func TestDSTEntrypointRetriesIncompleteWorkshopRefresh(t *testing.T) {
	root, dataDir, script := dstEntrypointFixture(t, false)
	t.Setenv("FAKE_DOWNLOAD_ONE_PER_ATTEMPT", "1")

	output, err := runDSTEntrypoint(t, root, dataDir, script, "refresh")
	if err != nil {
		t.Fatalf("expected retry to complete refresh: %v\n%s", err, output)
	}
	if !strings.Contains(output, "DST Workshop refresh attempt 2/6") {
		t.Fatalf("expected a second refresh attempt, got:\n%s", output)
	}
	for _, id := range []string{"111", "222"} {
		if _, err := os.Stat(filepath.Join(dataDir, "ugc_mods", "content", "322330", id, "modinfo.lua")); err != nil {
			t.Fatalf("expected retried mod %s: %v", id, err)
		}
	}
}

func TestDSTEntrypointResumesPreservedWorkshopRefresh(t *testing.T) {
	root, dataDir, script := dstEntrypointFixture(t, false)
	writeTestFile(t, filepath.Join(dataDir, "ugc_mods.refresh", "content", "322330", "111", "modinfo.lua"), "partial")
	t.Setenv("FAKE_DOWNLOAD_IDS", "222")

	output, err := runDSTEntrypoint(t, root, dataDir, script, "refresh")
	if err != nil {
		t.Fatalf("expected preserved refresh to resume: %v\n%s", err, output)
	}
	for _, id := range []string{"111", "222"} {
		if _, err := os.Stat(filepath.Join(dataDir, "ugc_mods", "content", "322330", id, "modinfo.lua")); err != nil {
			t.Fatalf("expected resumed mod %s: %v", id, err)
		}
	}
}

func TestDSTEntrypointFallsBackToCompleteCacheWhenRefreshIsIncomplete(t *testing.T) {
	root, dataDir, script := dstEntrypointFixture(t, false)
	for _, id := range []string{"111", "222"} {
		writeTestFile(t, filepath.Join(dataDir, "ugc_mods", "content", "322330", id, "modinfo.lua"), "cached")
	}

	output, err := runDSTEntrypoint(t, root, dataDir, script, "refresh")
	if err != nil {
		t.Fatalf("expected complete previous cache to keep the server available: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Continuing with the previous verified GamePanel DST Workshop cache.") {
		t.Fatalf("expected verified cache fallback message, got:\n%s", output)
	}
	log := readTestFile(t, filepath.Join(dataDir, "fake-server.log"))
	if !strings.Contains(log, "-skip_update_server_mods") {
		t.Fatalf("expected normal shard startup with the previous cache, got %q", log)
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

func TestDSTEntrypointFiltersKnownNativeWorkshopNoise(t *testing.T) {
	root, dataDir, script := dstEntrypointFixture(t, true)
	t.Setenv("FAKE_WORKSHOP_NOISE", "1")

	output, err := runDSTEntrypoint(t, root, dataDir, script, "refresh")
	if err != nil {
		t.Fatalf("run entrypoint: %v\n%s", err, output)
	}
	for _, noise := range []string{
		"Staging library folder not found",
		"Install library folder not found",
	} {
		if strings.Contains(output, noise) {
			t.Fatalf("expected known Steam Workshop noise to be filtered, got:\n%s", output)
		}
	}
	if !strings.Contains(output, "native downloader diagnostic") {
		t.Fatalf("expected unrelated native downloader stderr to be preserved, got:\n%s", output)
	}
}

func TestDSTEntrypointStopsMasterBeforeCaves(t *testing.T) {
	root, dataDir, script := dstEntrypointFixture(t, false)
	writeTestFile(t, filepath.Join(dataDir, "dst", "GamePanelLite", "Caves", "server.ini"), "[NETWORK]\nserver_port = 11000\n")
	for _, id := range []string{"111", "222"} {
		writeTestFile(t, filepath.Join(dataDir, "ugc_mods", "content", "322330", id, "modinfo.lua"), "cached")
	}
	t.Setenv("FAKE_SHARDS_BLOCK", "1")

	cmd := dstEntrypointCommand(root, dataDir, script, "reuse")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(dataDir, "fake-server.log")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, _ := os.ReadFile(logPath)
		if strings.Contains(string(content), "start:Master") && strings.Contains(string(content), "start:Caves") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("entrypoint did not exit cleanly: %v\n%s", err, output.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("entrypoint did not finish graceful shutdown")
	}

	log := readTestFile(t, logPath)
	masterStopped := strings.Index(log, "stop:Master")
	cavesStopped := strings.Index(log, "stop:Caves")
	if masterStopped < 0 || cavesStopped < 0 || masterStopped > cavesStopped {
		t.Fatalf("expected Master to stop before Caves, got:\n%s", log)
	}
}

func TestDSTEntrypointRoutesConsoleCommandsToSelectedShard(t *testing.T) {
	root, dataDir, script := dstEntrypointFixture(t, false)
	writeTestFile(t, filepath.Join(dataDir, "dst", "GamePanelLite", "Caves", "server.ini"), "[NETWORK]\nserver_port = 11000\n")
	for _, id := range []string{"111", "222"} {
		writeTestFile(t, filepath.Join(dataDir, "ugc_mods", "content", "322330", id, "modinfo.lua"), "cached")
	}
	t.Setenv("FAKE_CAPTURE_CONSOLE", "1")
	cmd := dstEntrypointCommand(root, dataDir, script, "reuse")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	if _, err := stdin.Write([]byte("__GAMEPANEL_DST_CONSOLE__:master:Y19zYXZlKCk=\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := stdin.Write([]byte("__GAMEPANEL_DST_CONSOLE__:caves:Y19jb3VudHByZWZhYnMoInNwaWRlciIp\n")); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		master, _ := os.ReadFile(filepath.Join(dataDir, "console-Master.log"))
		caves, _ := os.ReadFile(filepath.Join(dataDir, "console-Caves.log"))
		if strings.Contains(string(master), "c_save()") && strings.Contains(string(caves), `c_countprefabs("spider")`) &&
			strings.Contains(output.String(), "GamePanel DST console command sent to Master.") &&
			strings.Contains(output.String(), "GamePanel DST console command sent to Caves.") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("commands were not routed to their shards: master=%q caves=%q output=%q", readTestFile(t, filepath.Join(dataDir, "console-Master.log")), readTestFile(t, filepath.Join(dataDir, "console-Caves.log")), output.String())
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
  for path in content/322330 downloads/322330 temp/322330; do
    if [[ ! -d "${ugc}/${path}" ]]; then
      printf 'missing UGC directory: %s\n' "${ugc}/${path}" >&2
      exit 1
    fi
  done
  if [[ "${FAKE_WORKSHOP_NOISE:-0}" == "1" ]]; then
    printf '%s\n' 'src/clientdll/contentupdatecontext.cpp (2036) : Staging library folder not found' >&2
    printf '%s\n' 'src/clientdll/contentupdatecontext.cpp (2037) : Install library folder not found' >&2
    printf '%s\n' 'native downloader diagnostic' >&2
  fi
  download_ids="${FAKE_DOWNLOAD_IDS:-}"
  if [[ "${FAKE_DOWNLOAD_ONE_PER_ATTEMPT:-0}" == "1" ]]; then
    attempt_file="${DST_PERSISTENT_ROOT}/fake-download-attempt"
    attempt=0
    [[ ! -f "${attempt_file}" ]] || attempt="$(cat "${attempt_file}")"
    attempt=$((attempt + 1))
    printf '%s' "${attempt}" > "${attempt_file}"
    if [[ "${attempt}" == "1" ]]; then download_ids="111"; else download_ids="222"; fi
  elif [[ "${FAKE_DOWNLOAD_MODS:-0}" == "1" ]]; then
    download_ids="111 222"
  fi
  if [[ -n "${download_ids}" ]]; then
    for id in ${download_ids}; do
      mkdir -p "${ugc}/content/322330/${id}"
      printf 'mod' > "${ugc}/content/322330/${id}/modinfo.lua"
    done
  fi
fi
if [[ "${FAKE_SHARDS_BLOCK:-0}" == "1" ]]; then
  shard=""
  while [[ $# -gt 0 ]]; do
    if [[ "$1" == "-shard" ]]; then shard="$2"; break; fi
    shift
  done
  trap 'printf "stop:%s\\n" "${shard}" >> "${DST_PERSISTENT_ROOT}/fake-server.log"; exit 0' TERM
  printf 'start:%s\n' "${shard}" >> "${DST_PERSISTENT_ROOT}/fake-server.log"
  while true; do sleep 1; done
fi
if [[ "${FAKE_CAPTURE_CONSOLE:-0}" == "1" ]]; then
  shard=""
  while [[ $# -gt 0 ]]; do
    if [[ "$1" == "-shard" ]]; then shard="$2"; break; fi
    shift
  done
  while IFS= read -r line; do
    printf '%s\n' "${line}" >> "${DST_PERSISTENT_ROOT}/console-${shard}.log"
  done
fi
`
	writeTestFile(t, filepath.Join(root, "server", "bin64", "dontstarve_dedicated_server_nullrenderer_x64"), fake)
	if err := os.Chmod(filepath.Join(root, "server", "bin64", "dontstarve_dedicated_server_nullrenderer_x64"), 0o755); err != nil {
		t.Fatal(err)
	}
	fakeSteamCMD := `#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${DST_PERSISTENT_ROOT}/fake-steamcmd.log"
mkdir -p "${DST_ROOT_DIR}/server/steamapps"
cat > "${DST_ROOT_DIR}/server/steamapps/appmanifest_343050.acf" <<'EOF'
"AppState"
{
  "appid" "343050"
  "StateFlags" "4"
  "buildid" "24700372"
}
EOF
`
	writeTestFile(t, filepath.Join(root, "fake-steamcmd"), fakeSteamCMD)
	if err := os.Chmod(filepath.Join(root, "fake-steamcmd"), 0o755); err != nil {
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
	cmd := dstEntrypointCommand(root, dataDir, script, mode)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func dstEntrypointCommand(root, dataDir, script, mode string) *exec.Cmd {
	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(),
		"DST_ROOT_DIR="+root,
		"DST_PERSISTENT_ROOT="+dataDir,
		"DST_CONF_DIR=dst",
		"DST_CLUSTER_NAME=GamePanelLite",
		"DST_MOD_SYNC_MODE="+mode,
		"DST_GAME_UPDATE_MODE="+mode,
		"DST_STEAMCMD_BIN="+filepath.Join(root, "fake-steamcmd"),
	)
	return cmd
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
