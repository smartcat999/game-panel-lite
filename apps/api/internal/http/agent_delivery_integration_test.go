package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/metrics"
	modfiles "github.com/smartcat999/game-panel-lite/apps/api/internal/mod"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modlibrary"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/modruntime"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/terraria"
	serverctrl "github.com/smartcat999/game-panel-lite/apps/api/internal/server"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/store"
	dockerruntime "github.com/smartcat999/game-panel-lite/internal/runtime/docker"
)

// Preserve the real provider's upload/layout/manifest contracts while executing
// a tiny file-reading process. This checks delivery, not game compatibility.
type deliveryProbeProvider struct{ terraria.TModLoaderProvider }

func (deliveryProbeProvider) ImageFor(string) string { return "alpine:3.21" }
func (p deliveryProbeProvider) RuntimeConfigForResource(s domain.GameServer) (domain.ProviderRuntimeConfig, error) {
	cfg, err := p.TModLoaderProvider.RuntimeConfigForResource(s)
	cfg.Options.DataMounts = []string{"/data"}
	cfg.Options.Cmd = []string{"sh", "-c", "cat /data/Mods/source.tmod; echo; exec sleep 300"}
	return cfg, err
}

// Opt-in: builds two real Agent processes and creates uniquely named containers.
func TestAgentArtifactDeliveryIntegration(t *testing.T) {
	host := os.Getenv("GAMEPANEL_TEST_DOCKER_HOST")
	if host == "" {
		t.Skip("set GAMEPANEL_TEST_DOCKER_HOST for real Agent/Docker delivery")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	registry, err := provider.NewRegistry(deliveryProbeProvider{terraria.NewTModLoaderProvider()})
	if err != nil {
		t.Fatal(err)
	}
	mods := modruntime.NewService(registry, db)
	files := modfiles.NewService(root, mods.StoredFileName)
	handler := &Handler{store: db, logger: slog.New(slog.NewTextHandler(io.Discard, nil)), apiMetrics: metrics.NewRegistry(), modDelivery: modlibrary.NewDelivery(db, files)}
	router := chi.NewRouter()
	router.Post("/api/agent/register", handler.agentRegister)
	router.Post("/api/agent/heartbeat", handler.agentHeartbeat)
	router.Get("/api/agent/assignments", handler.listAgentAssignments)
	router.Post("/api/agent/assignments/{uid}/lease", handler.changeAgentLease)
	router.Post("/api/agent/assignments/{uid}/status", handler.reportAgentAssignmentStatus)
	router.Get("/api/agent/assignments/{uid}/artifacts/{artifactId}", handler.downloadAgentArtifact)
	router.Get("/api/agent/tasks", handler.listAgentTasks)
	router.Get("/api/agent/tunnel/poll", handler.pollAgentTunnel)
	panel := httptest.NewServer(router)
	defer panel.Close()
	_, filename, _, _ := runtime.Caller(0)
	repo := filepath.Clean(filepath.Join(filepath.Dir(filename), "../../../.."))
	binary := filepath.Join(root, "agent")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./apps/agent")
	build.Dir = repo
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Agent: %v\n%s", err, output)
	}
	type nodeRun struct {
		node       domain.ComputeNode
		target     domain.GameServer
		assignment domain.WorkloadAssignment
		source     domain.ModFile
		adapter    *dockerruntime.Adapter
		payload    string
	}
	nodes := []nodeRun{}
	for i := 0; i < 2; i++ {
		suffix := uuid.NewString()
		org := domain.Organization{ID: "delivery-space-" + suffix, Slug: "delivery-space-" + suffix}
		if err := db.CreateOrganization(ctx, &org, "owner"); err != nil {
			t.Fatal(err)
		}
		node := domain.ComputeNode{ID: "delivery-node-" + suffix, Token: uuid.NewString()}
		if err := db.CreateComputeNode(ctx, &node); err != nil {
			t.Fatal(err)
		}
		payload := fmt.Sprintf("workspace-%d-verified", i)
		sum := sha256.Sum256([]byte(payload))
		source := domain.ModFile{ID: "delivery-source-" + suffix, OrganizationID: org.ID, InstanceID: "unassigned", ProviderKey: domain.ProviderTerrariaTModLoader, Source: "upload", FileName: "source.tmod", ModName: "Source", ContentHash: hex.EncodeToString(sum[:]), SizeBytes: int64(len(payload))}
		if _, err := files.PutLibrary(ctx, source, strings.NewReader(payload), 1024); err != nil {
			t.Fatal(err)
		}
		if err := db.CreateOwnedLibraryMod(ctx, "owner", &source); err != nil {
			t.Fatal(err)
		}
		port, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		hostPort := port.Addr().(*net.TCPAddr).Port
		port.Close()
		target := domain.GameServer{ID: "delivery-server-" + suffix, NodeID: node.ID, OrganizationID: org.ID, ProviderKey: source.ProviderKey, Spec: domain.ServerSpec{Generation: 1, DesiredState: domain.DesiredRunning, ModIDs: []string{source.ID}, Network: domain.ServerNetworkSpec{Port: 7777, HostPort: hostPort}, Resources: domain.ServerResources{CPULimitCores: 0.25, MemoryLimitMB: 64}, Runtime: domain.ServerRuntimeSpec{DataDir: filepath.Join(root, "control-must-not-create-"+suffix)}}}
		if err := db.CreateGameServer(ctx, &target); err != nil {
			t.Fatal(err)
		}
		spec, err := serverctrl.NewProviderWorkloadBuilder(registry).WithModPlanner(serverctrl.NewRuntimeModPlanner(root, db, registry)).BuildWorkloadSpec(ctx, target)
		if err != nil {
			t.Fatal(err)
		}
		assignment := domain.WorkloadAssignment{ID: suffix, UID: suffix, NodeID: node.ID, ServerID: target.ID, Generation: 1, DesiredState: domain.DesiredRunning, Spec: spec}
		if err := db.PublishWorkloadAssignment(ctx, target, &assignment); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(target.Spec.Runtime.DataDir); !os.IsNotExist(err) {
			t.Fatalf("control-plane file mutation: %v", err)
		}
		dataRoot := filepath.Join(root, "worker-"+suffix)
		adapter, err := dockerruntime.NewAdapter(host, dataRoot)
		if err != nil {
			t.Fatal(err)
		}
		// Cleanup registration order: child process, owned container, then client.
		t.Cleanup(func() { adapter.Close() })
		t.Cleanup(func() {
			cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
			defer stop()
			state, err := adapter.Inspect(cleanup, target.ID)
			if err != nil {
				t.Errorf("cleanup inspect %s: %v", target.ID, err)
				return
			}
			if state.Exists {
				if state.UID != assignment.UID || state.NodeID != node.ID {
					t.Errorf("unexpected cleanup identity: %+v", state)
					return
				}
				if err := adapter.Remove(cleanup, state); err != nil {
					t.Error(err)
				}
			}
		})
		logFile, err := os.Create(filepath.Join(root, "agent-"+suffix+".log"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary)
		cmd.Env = append(os.Environ(), "MASTER_URL="+panel.URL, "AGENT_TOKEN="+node.Token, "DOCKER_HOST="+host, "AGENT_INSTANCE_ROOT="+dataRoot)
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		if err := cmd.Start(); err != nil {
			logFile.Close()
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		t.Cleanup(func() {
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("Agent exit: %v", err)
				}
			case <-time.After(15 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Error("Agent did not stop gracefully")
			}
			logFile.Close()
			if t.Failed() {
				if content, err := os.ReadFile(logFile.Name()); err == nil {
					t.Logf("Agent log:\n%s", content)
				}
			}
		})
		nodes = append(nodes, nodeRun{node, target, assignment, source, adapter, payload})
	}
	for _, n := range nodes {
		found := false
		for ctx.Err() == nil {
			observation, err := db.GetWorkloadObservation(ctx, n.assignment.UID)
			if err == nil && observation.LastError != "" {
				t.Fatalf("Agent reconciliation failed: %s", observation.LastError)
			}
			if err == nil && observation.ObservedGeneration == 1 && observation.ActualState == domain.ActualRunning {
				state, err := n.adapter.Inspect(ctx, n.target.ID)
				if err != nil {
					t.Fatal(err)
				}
				if state.NodeID != n.node.ID || state.UID != n.assignment.UID {
					t.Fatalf("wrong runtime identity: %+v", state)
				}
				lines, err := n.adapter.Logs(ctx, state.ID)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(strings.Join(lines, "\n"), n.payload) {
					found = true
					break
				}
			}
			select {
			case <-ctx.Done():
			case <-time.After(200 * time.Millisecond):
			}
		}
		if !found {
			t.Fatal("Agent did not converge before deadline")
		}
		if ready, err := db.RemoteArtifactsAvailable(ctx, n.node.ID); err != nil || !ready {
			t.Fatalf("production Agent did not advertise ready capability: %v", err)
		}
		daemon, err := n.adapter.Info(ctx)
		if err != nil {
			t.Fatal(err)
		}
		reported, err := db.GetComputeNode(ctx, n.node.ID)
		if err != nil || daemon.Architecture == "" || reported.RuntimeArchitecture != daemon.Architecture {
			t.Fatalf("daemon architecture=%q reported=%q err=%v", daemon.Architecture, reported.RuntimeArchitecture, err)
		}
		other := nodes[0]
		if other.node.ID == n.node.ID {
			other = nodes[1]
		}
		req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, panel.URL+"/api/agent/assignments/"+n.assignment.UID+"/artifacts/"+n.source.ID+"?generation=1&holderId=other-holder&fence=1", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Node-Token", other.node.Token)
		response, err := panel.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 404 {
			t.Fatalf("cross-node download accepted: %d", response.StatusCode)
		}
	}
}
