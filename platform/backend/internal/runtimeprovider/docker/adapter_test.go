package docker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/smartcat999/game-panel-lite/platform/backend/internal/nodeexecution"
)

func validSpec(t *testing.T) nodeexecution.WorkloadSpec {
	t.Helper()
	return nodeexecution.WorkloadSpec{LogicalInstanceID: "lin_test", DesiredState: "running", Image: "image:test", DataDir: t.TempDir(), Port: 7777, Protocol: "tcp", CPUUnits: 1000, MemoryMegabytes: 2048, Files: map[string]string{"serverconfig.txt": "world=test"}, DataMounts: map[string]string{"Worlds": "/home/container/Worlds", "serverconfig.txt": "/home/container/serverconfig.txt"}, FencingToken: 7}
}

func TestDockerAdapterIntegration(t *testing.T) {
	if os.Getenv("GAMEPANEL_DOCKER_INTEGRATION") != "1" {
		t.Skip("set GAMEPANEL_DOCKER_INTEGRATION=1 for the read-only daemon check")
	}
	adapter, err := New(os.Getenv("GAMEPANEL_DOCKER_HOST"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.client.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDockerSecurityAndResourceLimits(t *testing.T) {
	spec := validSpec(t)
	config := secureHostConfig(spec, []string{"/host/data:/data"}, nil)
	if !reflect.DeepEqual(config.SecurityOpt, []string{"no-new-privileges:true"}) || !reflect.DeepEqual([]string(config.CapDrop), []string{"ALL"}) || config.Resources.PidsLimit == nil || *config.Resources.PidsLimit != 512 {
		t.Fatalf("security config=%#v", config)
	}
	if config.Resources.NanoCPUs != 1_000_000_000 || config.Resources.Memory != 2048*1024*1024 {
		t.Fatalf("resources=%#v", config.Resources)
	}
}

func TestDockerFilesAndMountsCannotEscapeOrTraverseSymlinks(t *testing.T) {
	spec := validSpec(t)
	for _, invalid := range []string{"..", "../escape", "/absolute"} {
		spec.Files = map[string]string{invalid: "bad"}
		if err := prepareData(spec); err == nil {
			t.Fatalf("accepted file path %q", invalid)
		}
	}
	spec = validSpec(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(spec.DataDir, "escape")); err != nil {
		t.Fatal(err)
	}
	spec.Files = map[string]string{"escape/config.txt": "bad"}
	if err := prepareData(spec); err == nil {
		t.Fatal("accepted symlink traversal")
	}
	spec = validSpec(t)
	spec.DataMounts = map[string]string{"Worlds": "relative/path"}
	if _, err := bindConfiguration(spec); err == nil {
		t.Fatal("accepted relative container mount")
	}
}

func TestDockerValidatesBeforeDaemonOperations(t *testing.T) {
	adapter := &Adapter{}
	spec := validSpec(t)
	spec.DataDir = "relative/path"
	if _, err := adapter.Reconcile(t.Context(), spec); err == nil {
		t.Fatal("invalid spec reached Docker client")
	}
}
