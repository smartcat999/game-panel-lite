package docker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

func TestDataBindsUsesAbsoluteHostPath(t *testing.T) {
	binds, err := dataBinds("data/instances/example", []string{"/data"})
	if err != nil {
		t.Fatalf("expected data binds to succeed, got %v", err)
	}
	if len(binds) != 1 {
		t.Fatalf("expected one bind, got %v", binds)
	}
	host, _, ok := strings.Cut(binds[0], ":")
	if !ok {
		t.Fatalf("expected host:container bind, got %q", binds[0])
	}
	if !filepath.IsAbs(host) {
		t.Fatalf("expected absolute host path, got %q", host)
	}
}

func TestDataBindsSupportsSubPathMounts(t *testing.T) {
	binds, err := dataBinds("data/instances/example", []string{"Worlds:/home/container/Worlds"})
	if err != nil {
		t.Fatalf("expected data binds to succeed, got %v", err)
	}
	if len(binds) != 1 {
		t.Fatalf("expected one bind, got %v", binds)
	}
	host, container, ok := strings.Cut(binds[0], ":")
	if !ok {
		t.Fatalf("expected host:container bind, got %q", binds[0])
	}
	if !filepath.IsAbs(host) {
		t.Fatalf("expected absolute host path, got %q", host)
	}
	if filepath.Base(host) != "Worlds" {
		t.Fatalf("expected host bind to target Worlds subdir, got %q", host)
	}
	if container != "/home/container/Worlds" {
		t.Fatalf("expected container bind path, got %q", container)
	}
}

func TestDataBindsRejectsEscapingHostSubPath(t *testing.T) {
	_, err := dataBinds("data/instances/example", []string{"../escape:/data"})
	if err == nil {
		t.Fatal("expected escaping data mount host path to fail")
	}
	if !strings.Contains(err.Error(), "invalid data mount host path") {
		t.Fatalf("expected invalid host path error, got %v", err)
	}
}

func TestDataBindsRejectsRelativeContainerPath(t *testing.T) {
	_, err := dataBinds("data/instances/example", []string{"Worlds:home/container/Worlds"})
	if err == nil {
		t.Fatal("expected relative container path to fail")
	}
	if !strings.Contains(err.Error(), "invalid data mount container path") {
		t.Fatalf("expected invalid container path error, got %v", err)
	}
}

func TestPrepareDataMountsRejectsEscapingHostSubPath(t *testing.T) {
	dataDir := t.TempDir()
	err := prepareDataMounts(dataDir, []string{"../escape:/data"})
	if err == nil {
		t.Fatal("expected escaping data mount host path to fail")
	}
}

func TestNatPortSetExposesContainerPort(t *testing.T) {
	ports := natPortSet(7777, "")
	if _, ok := ports["7777/tcp"]; !ok {
		t.Fatalf("expected exposed 7777/tcp port, got %v", ports)
	}
}

func TestNatPortSetSupportsUdp(t *testing.T) {
	ports := natPortSet(8211, "udp")
	if _, ok := ports["8211/udp"]; !ok {
		t.Fatalf("expected exposed 8211/udp port, got %v", ports)
	}
}

func TestPrepareDataMountsRepairsExistingDirectoryPermissions(t *testing.T) {
	dataDir := t.TempDir()
	if err := os.Chmod(dataDir, 0o755); err != nil {
		t.Fatalf("failed to chmod temp dir: %v", err)
	}

	if err := prepareDataMounts(dataDir, []string{"/data"}); err != nil {
		t.Fatalf("expected mount preparation to succeed, got %v", err)
	}

	info, err := os.Stat(dataDir)
	if err != nil {
		t.Fatalf("expected data dir to exist: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o777 {
		t.Fatalf("expected writable data dir permissions, got %o", got)
	}
}

func TestPrepareDataMountsRepairsExistingNestedDSTSavePermissions(t *testing.T) {
	dataDir := t.TempDir()
	saveDir := filepath.Join(dataDir, "dst", "Cluster", "Master", "save")
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		t.Fatalf("failed to create save dir: %v", err)
	}
	shardIndex := filepath.Join(saveDir, "shardindex")
	if err := os.WriteFile(shardIndex, []byte("existing"), 0o644); err != nil {
		t.Fatalf("failed to create shardindex: %v", err)
	}

	if err := prepareDataMounts(dataDir, []string{"/data"}); err != nil {
		t.Fatalf("expected mount preparation to succeed, got %v", err)
	}

	dirInfo, err := os.Stat(saveDir)
	if err != nil {
		t.Fatalf("expected save dir to exist: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o777 {
		t.Fatalf("expected writable save dir permissions, got %o", got)
	}
	fileInfo, err := os.Stat(shardIndex)
	if err != nil {
		t.Fatalf("expected shardindex to exist: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o666 {
		t.Fatalf("expected writable shardindex permissions, got %o", got)
	}
}

func TestWriteDataFileCreatesWritableRuntimeFiles(t *testing.T) {
	dataDir := t.TempDir()
	if err := writeDataFile(dataDir, "dst/Cluster/Master/server.ini", "server_port = 10999\n"); err != nil {
		t.Fatalf("expected runtime data file to be written, got %v", err)
	}

	dirInfo, err := os.Stat(filepath.Join(dataDir, "dst", "Cluster", "Master"))
	if err != nil {
		t.Fatalf("expected runtime data dir to exist: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o777 {
		t.Fatalf("expected writable runtime data dir permissions, got %o", got)
	}

	fileInfo, err := os.Stat(filepath.Join(dataDir, "dst", "Cluster", "Master", "server.ini"))
	if err != nil {
		t.Fatalf("expected runtime data file to exist: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o666 {
		t.Fatalf("expected writable runtime data file permissions, got %o", got)
	}
}

func TestConsumeImagePullSuccess(t *testing.T) {
	stream := strings.NewReader(`{"status":"Pulling from smartcat99999/terraria-vanilla"}
{"status":"Digest: sha256:example"}
`)
	if err := consumeImagePull(stream); err != nil {
		t.Fatalf("expected successful pull stream, got %v", err)
	}
}

func TestConsumeImagePullReturnsStreamError(t *testing.T) {
	stream := strings.NewReader(`{"error":"manifest unknown: manifest unknown"}`)
	err := consumeImagePull(stream)
	if err == nil {
		t.Fatal("expected pull stream error")
	}
	if !strings.Contains(err.Error(), "manifest unknown") {
		t.Fatalf("expected manifest error, got %v", err)
	}
}

func TestConsumeImagePullReturnsErrorDetail(t *testing.T) {
	stream := strings.NewReader(`{"errorDetail":{"message":"no matching manifest for linux/arm64/v8 in the manifest list entries"},"error":"no matching manifest"}`)
	err := consumeImagePull(stream)
	if err == nil {
		t.Fatal("expected pull stream error detail")
	}
	if !strings.Contains(err.Error(), "no matching manifest") {
		t.Fatalf("expected platform error, got %v", err)
	}
}

func TestImagePullErrorExplainsMissingDSTRegistryImage(t *testing.T) {
	err := imagePullError(
		"smartcat99999/dst-server:latest",
		errors.New("pull access denied for smartcat99999/dst-server, repository does not exist or may require 'docker login': denied: requested access to the resource is denied"),
	)
	if err == nil {
		t.Fatal("expected image pull error")
	}
	for _, want := range []string{"DST runtime image", "scripts/build-game-images.sh dst", "Original error"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestConsumeImagePullReportsProgress(t *testing.T) {
	stream := strings.NewReader(`{"id":"layer-a","status":"Downloading","progressDetail":{"current":25,"total":100}}
{"id":"layer-b","status":"Downloading","progressDetail":{"current":50,"total":100}}
{"id":"layer-a","status":"Downloading","progressDetail":{"current":100,"total":100}}
`)
	var progresses []int
	if err := consumeImagePullWithProgress(stream, func(progress runtime.ImagePrepareProgress) {
		progresses = append(progresses, progress.Progress)
	}); err != nil {
		t.Fatalf("expected successful pull stream, got %v", err)
	}
	if len(progresses) == 0 {
		t.Fatal("expected progress callbacks")
	}
	for i := 1; i < len(progresses); i++ {
		if progresses[i] < progresses[i-1] {
			t.Fatalf("expected monotonic progress, got %v", progresses)
		}
	}
	if progresses[len(progresses)-1] != 100 {
		t.Fatalf("expected final progress to be 100, got %v", progresses)
	}
}
