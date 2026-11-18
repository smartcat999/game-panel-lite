package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestShouldMaterializeWorkspaceSkipsActiveObservationTicks(t *testing.T) {
	drv := &DockerDriver{}
	inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Tenant: "admin", Name: "tml-1"},
	}
	startedAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	runningInspected := types.ContainerJSON{
		ContainerJSONBase: &types.ContainerJSONBase{
			State: &types.ContainerState{Running: true, StartedAt: startedAt},
		},
		Config: &container.Config{},
	}
	if drv.shouldMaterializeWorkspace(inst, runningInspected, false, false) {
		t.Fatalf("expected running unchanged container observation tick to skip workspace file overwrite")
	}
	if !drv.shouldMaterializeWorkspace(inst, runningInspected, true, false) {
		t.Fatalf("expected needsCreate to materialize workspace")
	}
	if !drv.shouldMaterializeWorkspace(inst, runningInspected, false, true) {
		t.Fatalf("expected needsRecreate to materialize workspace")
	}
	inst.Metadata.Annotations = map[string]string{
		corev1.AnnotationRestartRequestedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if !drv.shouldMaterializeWorkspace(inst, runningInspected, false, false) {
		t.Fatalf("expected pending restart to materialize workspace")
	}
}

func newTestDockerImageInspectDriver(t *testing.T, imageID string) *DockerDriver {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"Id": imageID})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.WithHost(srv.URL),
		dockerclient.WithHTTPClient(srv.Client()),
		dockerclient.WithVersion("1.47"),
	)
	if err != nil {
		t.Fatalf("create mock docker client: %v", err)
	}
	return NewDockerDriverWithClient(t.TempDir(), cli)
}

func TestShouldRecreateContainerWhenLocalImageIDChanges(t *testing.T) {
	drv := newTestDockerImageInspectDriver(t, "sha256:new-image-hash")
	inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Tenant: "studio-alpha", Name: "palworld-68", UID: "uid-1"},
		Spec:     corev1.InstanceSpec{State: corev1.InstanceStateRunning, Image: "gamepanel/runtime:latest"},
	}
	specHash := computeRuntimeSpecHash(inst)
	startedAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	mkInspected := func(running bool, imageID string) types.ContainerJSON {
		return types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				Image: imageID,
				State: &types.ContainerState{Running: running, StartedAt: startedAt},
			},
			Config: &container.Config{Labels: map[string]string{corev1.LabelRuntimeSpecHash: specHash}},
		}
	}
	if drv.shouldRecreateContainer(context.Background(), inst, mkInspected(true, "sha256:old-image-hash")) {
		t.Fatalf("expected active running container to skip image-hash recreate without restart/start")
	}
	if !drv.shouldRecreateContainer(context.Background(), inst, mkInspected(false, "sha256:old-image-hash")) {
		t.Fatalf("expected stopped container starting with stale image hash to recreate")
	}
	if drv.shouldRecreateContainer(context.Background(), inst, mkInspected(false, "sha256:new-image-hash")) {
		t.Fatalf("expected container matching local image hash not to recreate")
	}
	stoppedInst := *inst
	stoppedInst.Spec.State = corev1.InstanceStateStopped
	if drv.shouldRecreateContainer(context.Background(), &stoppedInst, mkInspected(false, "sha256:old-image-hash")) {
		t.Fatalf("expected stopped desired state not to recreate stale container")
	}
	inst.Metadata.Annotations = map[string]string{
		corev1.AnnotationRestartRequestedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if !drv.shouldRecreateContainer(context.Background(), inst, mkInspected(true, "sha256:old-image-hash")) {
		t.Fatalf("expected explicit restart with stale image hash to trigger recreate")
	}
}

func TestPrePullImageWaitsPastFastTimeout(t *testing.T) {
	mkDriver := func(status int, body string) *DockerDriver {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/images/create") {
				time.Sleep(dockerImagePullFastWaitTimeout + 80*time.Millisecond)
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
				return
			}
			http.NotFound(w, r)
		}))
		t.Cleanup(srv.Close)
		cli, err := dockerclient.NewClientWithOpts(
			dockerclient.WithHost(srv.URL),
			dockerclient.WithHTTPClient(srv.Client()),
			dockerclient.WithVersion("1.47"),
		)
		if err != nil {
			t.Fatalf("create mock docker client: %v", err)
		}
		return NewDockerDriverWithClient(t.TempDir(), cli)
	}

	okDrv := mkDriver(http.StatusOK, `{"status":"Downloaded"}`)
	ready, err := okDrv.PrePullImage(context.Background(), "gamepanel/runtime:latest")
	if err != nil || !ready {
		t.Fatalf("expected PrePullImage to wait for completion and return ready=true, err=nil; got ready=%v err=%v", ready, err)
	}

	failDrv := mkDriver(http.StatusForbidden, `{"message":"pull access denied"}`)
	ready, err = failDrv.PrePullImage(context.Background(), "gamepanel/runtime:latest")
	if err == nil || ready {
		t.Fatalf("expected PrePullImage to wait for delayed failure and return ready=false, err!=nil; got ready=%v err=%v", ready, err)
	}
}
