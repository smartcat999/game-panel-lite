package docker

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeworkload"
)

func validSpec() nodeworkload.Specification {
	return nodeworkload.Specification{LogicalInstanceID: "lin_test", DesiredState: "running", Artifact: "image:test", DataScope: "instances/lin_test", Listeners: []nodeworkload.Listener{{InternalPort: 7777, HostPort: 31777, Protocol: "tcp"}}, CPUMilli: 1000, MemoryMiB: 2048, RunAsUID: os.Getuid(), RunAsGID: os.Getgid(), Files: map[string]string{"serverconfig.txt": "world=test"}, Mounts: map[string]string{"Worlds": "/data/Worlds", "serverconfig.txt": "/data/serverconfig.txt"}, FencingToken: 7}
}

func TestDockerAdapterIntegration(t *testing.T) {
	if os.Getenv("GAMEPANEL_DOCKER_INTEGRATION") != "1" {
		t.Skip("set GAMEPANEL_DOCKER_INTEGRATION=1 for the read-only daemon check")
	}
	adapter, err := New(os.Getenv("GAMEPANEL_DOCKER_HOST"), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDockerSecurityResourceAndNetworkIsolation(t *testing.T) {
	spec := validSpec()
	config := secureHostConfig(spec, []string{"/host/data:/data"}, nil, defaultNetwork)
	if !reflect.DeepEqual(config.SecurityOpt, []string{"no-new-privileges:true"}) || !reflect.DeepEqual([]string(config.CapDrop), []string{"ALL"}) || config.Resources.PidsLimit == nil || *config.Resources.PidsLimit != 512 || !config.ReadonlyRootfs {
		t.Fatalf("security config=%#v", config)
	}
	if config.NetworkMode != defaultNetwork || config.Resources.NanoCPUs != 1_000_000_000 || config.Resources.Memory != 2048*1024*1024 {
		t.Fatalf("resources=%#v network=%s", config.Resources, config.NetworkMode)
	}
	if config.Tmpfs["/tmp"] != "rw,noexec,nosuid,nodev,size=256m" {
		t.Fatalf("default tmpfs=%q", config.Tmpfs["/tmp"])
	}
	spec.ExecutableTemp = true
	if temporary := secureHostConfig(spec, nil, nil, defaultNetwork).Tmpfs["/tmp"]; temporary != "rw,exec,nosuid,nodev,size=256m" {
		t.Fatalf("executable tmpfs=%q", temporary)
	}
}

func TestDockerFilesAndMountsCannotEscapeOrTraverseSymlinks(t *testing.T) {
	dataRoot := t.TempDir()
	adapter, err := New("unix:///var/run/docker.sock", dataRoot, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := validSpec()
	for _, invalid := range []string{"..", "../escape", "/absolute"} {
		spec.Files = map[string]string{invalid: "bad"}
		if _, err := adapter.prepareData(spec); err == nil {
			t.Fatalf("accepted file path %q", invalid)
		}
	}
	spec = validSpec()
	instanceRoot, err := scopedPath(dataRoot, spec.DataScope)
	if err != nil || os.MkdirAll(instanceRoot, 0o750) != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(instanceRoot, "escape")); err != nil {
		t.Fatal(err)
	}
	spec.Files = map[string]string{"escape/config.txt": "bad"}
	if _, err := adapter.prepareData(spec); err == nil {
		t.Fatal("accepted symlink traversal")
	}
	if _, err := bindConfiguration(instanceRoot, map[string]string{"Worlds": "relative/path"}); err == nil {
		t.Fatal("accepted relative container mount")
	}
}

func TestPrepareDataCreatesDotPrefixedMountAsDirectory(t *testing.T) {
	dataRoot := t.TempDir()
	adapter, err := New("unix:///var/run/docker.sock", dataRoot, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := validSpec()
	spec.Mounts[".local"] = "/home/container/.local"
	root, err := adapter.prepareData(spec)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, ".local")); err != nil || !info.IsDir() {
		t.Fatalf("dot mount info=%#v err=%v", info, err)
	}
}

func TestDockerValidatesBeforeDaemonOperations(t *testing.T) {
	adapter, err := New("unix:///var/run/docker.sock", t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := validSpec()
	spec.DataScope = "../escape"
	if _, err := adapter.Reconcile(t.Context(), "lin_test", spec, nodeworkload.NetworkPolicy{InternetEgressAllowed: true, DeniedManagementCIDRs: []string{"10.0.0.0/8"}}); err == nil {
		t.Fatal("invalid spec reached Docker client")
	}
}

func TestObserveAbsentRuntimeProducesInstanceScopedAttempt(t *testing.T) {
	adapter := &Adapter{}
	first, err := adapter.observe(t.Context(), "lin_one", "", "stopped", 0)
	if err != nil || first.State != "stopped" || first.Handle.RuntimeAttemptID == "" || first.Handle.RuntimeID != "" || len(first.Metrics) != 2 {
		t.Fatalf("result=%#v err=%v", first, err)
	}
	second, err := adapter.observe(t.Context(), "lin_two", "", "stopped", 0)
	if err != nil || second.Handle.RuntimeAttemptID == first.Handle.RuntimeAttemptID {
		t.Fatalf("absent attempts must remain instance-scoped: first=%#v second=%#v err=%v", first.Handle, second.Handle, err)
	}
}

func TestTCPListenerControlsRuntimeReadiness(t *testing.T) {
	result := withReadinessCheck(nodeworkload.RuntimeResult{State: "running"}, []nodeworkload.Listener{{InternalPort: 7777, HostPort: 32000, Protocol: "tcp"}}, func(port int) bool { return port == 7777 })
	if result.State != "running" {
		t.Fatalf("open listener state=%q", result.State)
	}
	result = withReadinessCheck(nodeworkload.RuntimeResult{State: "running"}, []nodeworkload.Listener{{InternalPort: 7778, HostPort: 32001, Protocol: "tcp"}}, func(port int) bool { return port == 7777 })
	if result.State != "starting" {
		t.Fatalf("closed listener state=%q", result.State)
	}
}

func TestMissingWorkloadNetworkAddressIsNotReady(t *testing.T) {
	result := withReadinessCheck(nodeworkload.RuntimeResult{State: "running"}, []nodeworkload.Listener{{InternalPort: 7777, HostPort: 32000, Protocol: "tcp"}}, func(int) bool { return false })
	if result.State != "starting" {
		t.Fatalf("missing network address state=%q", result.State)
	}
}

func TestDecodeDockerLogsRemovesMultiplexHeadersAndNullBytes(t *testing.T) {
	var frames bytes.Buffer
	stdout := stdcopy.NewStdWriter(&frames, stdcopy.Stdout)
	stderr := stdcopy.NewStdWriter(&frames, stdcopy.Stderr)
	_, _ = stdout.Write([]byte("server started\x00\n"))
	_, _ = stderr.Write([]byte("warning\n"))

	entries, err := decodeDockerLogs(&frames, "lin_test", "rta_test", time.Unix(1, 0).UTC())
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries=%#v err=%v", entries, err)
	}
	if entries[0].Stream != "stdout" || entries[0].Message != "server started" || entries[1].Stream != "stderr" || entries[1].Message != "warning" {
		t.Fatalf("entries=%#v", entries)
	}
}

func TestBackupRestoreIsAtomicAndRejectsArchiveTraversal(t *testing.T) {
	dataRoot, backupRoot := t.TempDir(), t.TempDir()
	adapter, err := New("unix:///var/run/docker.sock", dataRoot, backupRoot)
	if err != nil {
		t.Fatal(err)
	}
	instanceRoot, err := scopedPath(dataRoot, "instances/lin_test")
	if err != nil || os.MkdirAll(instanceRoot, 0o750) != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instanceRoot, "world.wld"), []byte("world"), 0o640); err != nil {
		t.Fatal(err)
	}
	artifact, err := adapter.Backup(context.Background(), "instances/lin_test", "object://regions/reg/backup.tar.gz")
	if err != nil || artifact.SizeBytes == 0 || artifact.Checksums["sha256"] == "" {
		t.Fatalf("artifact=%#v err=%v", artifact, err)
	}
	if err := os.Remove(filepath.Join(instanceRoot, "world.wld")); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Restore(context.Background(), "instances/lin_test", artifact); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(filepath.Join(instanceRoot, "world.wld")); err != nil || !bytes.Equal(content, []byte("world")) {
		t.Fatalf("content=%q err=%v", content, err)
	}
}
