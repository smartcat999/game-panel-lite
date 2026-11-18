package artifact

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
	artifactdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/artifact"
	repositorydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/repository"
	upstreamdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/upstream"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

type testCache struct {
	mu   sync.RWMutex
	data map[string]string
}

func (c *testCache) Get(_ context.Context, k string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.data[k]
	if !ok {
		return "", cache.ErrNotFound
	}
	return v, nil
}
func (c *testCache) Set(_ context.Context, k, v string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[k] = v
	return nil
}
func (c *testCache) Delete(_ context.Context, k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, k)
	return nil
}
func (c *testCache) Exists(_ context.Context, k string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.data[k]
	return ok, nil
}
func (c *testCache) Ping(context.Context) error { return nil }
func (c *testCache) Close() error               { return nil }

func TestBuildProxyPendingArtifactMetadata(t *testing.T) {
	pub := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	rel := &addonmirror.ResolvedRelease{
		Package:       "paper",
		Version:       "1.21.4",
		Build:         "225",
		DownloadURL:   "https://example.com/paper.jar",
		ArchiveFormat: registryv1.ArchiveFormatJar,
		PublishedAt:   &pub,
		Changelog:     "- `abcdef1` Fix chunk unload",
		UpstreamURL:   "https://papermc.io/downloads/paper",
	}
	art := buildProxyPendingArtifact("paper.paper.1.21.4", "paper", "paper", "1.21.4", rel)
	if art.Status.Build != "225" || art.Status.PublishedAt == nil || !art.Status.PublishedAt.Equal(pub) {
		t.Fatalf("unexpected build/publishedAt: %#v", art.Status)
	}
	if art.Status.Changelog != rel.Changelog || art.Status.UpstreamURL != rel.UpstreamURL {
		t.Fatalf("unexpected changelog/upstreamURL: %#v", art.Status)
	}
}

func TestApplyResolvedFieldsToStatusSteamVersionGuard(t *testing.T) {
	pub := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	oldArt := registryv1.ArtifactStatus{}
	changed := applyResolvedFieldsToStatus(&oldArt, "https://steamcommunity.com/sharedfiles/filedetails/changelog/727774324", false, &pub, "Newest patch notes", "rev-12")
	if !changed || oldArt.UpstreamURL == "" || oldArt.PublishedAt != nil || oldArt.Changelog != "" {
		t.Fatalf("expected only upstreamURL on mismatched steam version, got %#v", oldArt)
	}

	curArt := registryv1.ArtifactStatus{}
	changedCur := applyResolvedFieldsToStatus(&curArt, "https://steamcommunity.com/sharedfiles/filedetails/changelog/727774324", true, &pub, "Newest patch notes", "rev-12")
	if !changedCur || curArt.PublishedAt == nil || curArt.Changelog != "Newest patch notes" || curArt.Build != "rev-12" {
		t.Fatalf("expected full hydration on matching version, got %#v", curArt)
	}
}

func TestResolveProxyUpstreamBypassCacheQueriesLiveModrinth(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/project/luckperms":
			hits.Add(1)
			_, _ = w.Write([]byte(`{"id":"lk","slug":"luckperms","title":"LuckPerms","downloads":100,"updated":"2026-04-01T00:00:00Z"}`))
		case "/v2/project/luckperms/dependencies":
			_, _ = w.Write([]byte(`{"projects":[],"versions":[]}`))
		case "/v2/project/luckperms/version":
			n := hits.Load()
			ver := "v5.4.0"
			if n > 1 {
				ver = "v5.5.0"
			}
			_, _ = w.Write([]byte(`[{"id":"` + ver + `","version_number":"` + ver + `","game_versions":["1.21.4"],"loaders":["paper"],"date_published":"2026-04-01T00:00:00Z","files":[{"url":"https://example.com/` + ver + `.jar","filename":"luckperms.jar","primary":true,"size":1024}]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	op := &operator{cache: &testCache{data: make(map[string]string)}}
	repoObj := &registryv1.Repository{Spec: registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatModrinthAddon}}
	up := &registryv1.Upstream{Provider: registryv1.UpstreamProviderModrinth, Endpoint: srv.URL + "/v2", ModrinthLoader: "paper", ModrinthProjectType: "plugin"}

	rel1, err := op.resolveProxyUpstreamReleaseWithCache(context.Background(), repoObj, up, "luckperms", "latest", true)
	if err != nil || rel1.Version != "v5.4.0" {
		t.Fatalf("expected initial cached resolve v5.4.0, got %+v err=%v", rel1, err)
	}
	rel2, err := op.resolveProxyUpstreamReleaseWithCache(context.Background(), repoObj, up, "luckperms", "latest", false)
	if err != nil || rel2.Version != "v5.5.0" {
		t.Fatalf("expected uncached retry resolve v5.5.0, got %+v err=%v", rel2, err)
	}
}

func TestApplyResolvedFieldsUpgradesDedicatedServerNewsURL(t *testing.T) {
	st := registryv1.ArtifactStatus{UpstreamURL: "https://store.steampowered.com/news/app/2394010"}
	if !applyResolvedFieldsToStatus(&st, "", false, nil, "", "") {
		t.Fatalf("expected stale dedicated-server news URL to be flagged changed")
	}
	want := "https://store.steampowered.com/news/app/1623730"
	if st.UpstreamURL != want {
		t.Fatalf("upgraded UpstreamURL = %q, want %q", st.UpstreamURL, want)
	}
}

func seedFailedProxyArtifact(t *testing.T, ctx context.Context, fix testpg.Fixture) *registryv1.Artifact {
	t.Helper()
	if _, err := upstreamdb.NewDB(fix.Pool).Create(ctx, &registryv1.Upstream{
		Metadata:            metav1.ObjectMeta{Name: "up-modrinth"},
		Provider:            registryv1.UpstreamProviderModrinth,
		Endpoint:            "https://api.modrinth.com/v2",
		ModrinthLoader:      "paper",
		ModrinthProjectType: "plugin",
	}); err != nil {
		t.Fatalf("Create upstream: %v", err)
	}
	if _, err := repositorydb.NewDB(fix.Pool).Create(ctx, &registryv1.Repository{
		Metadata: metav1.ObjectMeta{Name: "proxy-plugins"},
		Spec: registryv1.RepositorySpec{
			Mode:         registryv1.RepositoryModeProxy,
			Format:       registryv1.FormatModrinthAddon,
			UpstreamName: "up-modrinth",
		},
	}); err != nil {
		t.Fatalf("Create repository: %v", err)
	}
	artStore := artifactdb.NewDB(fix.Pool)
	created, err := artStore.Create(ctx, &registryv1.Artifact{
		Metadata: metav1.ObjectMeta{Name: registryv1.FormatArtifactName("proxy-plugins", "luckperms", "1.0.0")},
		Spec:     registryv1.ArtifactSpec{Repository: "proxy-plugins", Package: "luckperms", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("Create artifact: %v", err)
	}
	created.Status.Phase = registryv1.ArtifactPhaseFailed
	created.Status.Stage = registryv1.ArtifactStageFailed
	created.Status.Message = "upstream timeout"
	failed, err := artStore.UpdateStatus(ctx, created)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	return failed
}

func TestGetExistingFailedProxyArtifactDoesNotRequeueUntilRetryAnnotation(t *testing.T) {
	fix := testpg.New(t, nil)
	ctx := context.Background()
	failed := seedFailedProxyArtifact(t, ctx, fix)
	op := NewOperator(fix.Registry, nil, &testCache{data: make(map[string]string)})

	got, err := op.Get(ctx, failed.Metadata.Name)
	if err != nil || got.Status.Phase != registryv1.ArtifactPhaseFailed {
		t.Fatalf("op.Get = %+v err=%v, want Failed without auto-requeue", got, err)
	}
	existing, err := op.GetExisting(ctx, failed.Metadata.Name)
	if err != nil || existing.Status.Phase != registryv1.ArtifactPhaseFailed {
		t.Fatalf("op.GetExisting = %+v err=%v, want Failed", existing, err)
	}

	cp := *got
	cp.Metadata.Annotations = maps.Clone(got.Metadata.Annotations)
	if cp.Metadata.Annotations == nil {
		cp.Metadata.Annotations = make(map[string]string, 1)
	}
	cp.Metadata.Annotations[registryv1.AnnotationRetryRequestedAt] = time.Now().UTC().Format(time.RFC3339Nano)
	retried, err := op.Update(ctx, cp.Metadata.Name, &cp)
	if err != nil || retried.Status.Phase != registryv1.ArtifactPhasePending {
		t.Fatalf("op.Update retry = %+v err=%v, want Pending", retried, err)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	testpg.Stop()
	os.Exit(code)
}
