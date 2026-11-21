package pkg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	pkgdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/pkg"
	repositorydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/repository"
	upstreamdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/upstream"
	"github.com/smartcat999/game-panel-cloud/tests/testpg"
)

func TestIsStubPackage(t *testing.T) {
	if isStubPackage(nil) {
		t.Fatal("nil package must not be classified as stub")
	}
	stub := &registryv1.Package{
		Repository: "tmodloader-workshop",
		Package:    "2824688072",
		Title:      "2824688072",
	}
	if !isStubPackage(stub) {
		t.Fatal("expected bare workshop ID stub to be classified as stub")
	}
	rich := &registryv1.Package{
		Repository:  "tmodloader-workshop",
		Package:     "2824688072",
		Title:       "Calamity Mod",
		Description: "Large content mod",
	}
	if isStubPackage(rich) {
		t.Fatal("expected enriched package not to be classified as stub")
	}
}

func TestEnrichAndDetailServerProxyPackageMissingUpstream(t *testing.T) {
	op := &operator{}
	persisted := &registryv1.Package{
		Repository: "terraria-tmodloader",
		Package:    "terraria-tmodloader",
		Title:      "tModLoader",
	}
	repoObj := &registryv1.Repository{
		Spec: registryv1.RepositorySpec{
			Mode:   registryv1.RepositoryModeProxy,
			Format: registryv1.FormatServerPackage,
		},
	}
	got := op.enrichServerProxyPackage(context.Background(), repoObj, persisted)
	if got == nil || got.Title != "tModLoader" {
		t.Fatalf("expected persisted clone on missing upstream, got %#v", got)
	}
	if _, err := op.detailServerProxyPackage(context.Background(), repoObj, "terraria-tmodloader"); err == nil {
		t.Fatal("expected error when upstreamName is missing")
	}
}

func TestSignPackageIconWithService(t *testing.T) {
	svc := addonmirror.New(nil, addonmirror.Options{SigningSecret: "test-secret"})
	prefix := "/apis/registry.gamepanel.io/v1/packages/tmodloader-workshop.catalog/icon"
	rawPkg := &registryv1.Package{
		Repository: "tmodloader-workshop",
		Package:    "2565639705",
		Title:      "Ore Excavator",
		IconURL:    "https://images.steamusercontent.com/ugc/12345/preview.png",
	}
	signed := signPackageIconWithService(svc, prefix, rawPkg)
	if signed == nil || !strings.HasPrefix(signed.IconURL, prefix+"?u=") {
		t.Fatalf("expected signed proxy icon URL, got %#v", signed)
	}
	if !strings.HasPrefix(rawPkg.IconURL, "https://") {
		t.Fatal("expected input Package not to be mutated")
	}
	alreadySigned := signPackageIconWithService(svc, prefix, signed)
	if alreadySigned != signed {
		t.Fatal("expected already-proxied IconURL to be returned unchanged")
	}
}

func TestResolveRepoLatestVersionsAndLiveLatest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/IPublishedFileService/GetDetails/v1/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"response":{"publishedfiledetails":[{"publishedfileid":"727774324","result":1,"consumer_app_id":322330,"title":"Craft Pot","time_updated":1742000000}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc := addonmirror.New(nil, addonmirror.Options{SteamAPIBaseURL: srv.URL, SteamWebAPIKey: "k"})
	src := addonmirror.Source{Provider: addonmirror.ProviderSteamWorkshop, SteamAppID: 322330}
	op := &operator{}
	items := []*registryv1.Package{{Repository: "dst-workshop", Package: "727774324", LatestVersion: "latest"}}
	latestMap := op.resolveRepoLatestVersions(context.Background(), svc, src, items)
	if latestMap["727774324"] != "1742000000" {
		t.Fatalf("expected live steam timestamp version, got %#v", latestMap)
	}
	updated := op.applyLiveLatestVersion(context.Background(), items[0], latestMap["727774324"])
	if updated.LatestVersion != "1742000000" || items[0].LatestVersion != "latest" {
		t.Fatalf("expected cloned package with concrete LatestVersion, got %#v", updated)
	}
}

func TestNormalizeRepoPackageListCachedSkipsUpstreamHTTP(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	op := &operator{}
	repoObj := &registryv1.Repository{
		Metadata: metav1.ObjectMeta{Name: "tmodloader-workshop"},
		Spec: registryv1.RepositorySpec{
			Mode:   registryv1.RepositoryModeProxy,
			Format: registryv1.FormatWorkshopMod,
		},
	}
	list := &metav1.List[*registryv1.Package]{
		Items: []*registryv1.Package{{
			Repository: "tmodloader-workshop",
			Package:    "2824688072",
			Title:      "2824688072",
		}},
	}
	op.normalizeRepoPackageList(context.Background(), repoObj, list, true, false)
	if hits != 0 {
		t.Fatalf("expected cached listing without refresh=true to make 0 upstream calls, got %d", hits)
	}
	clean := stripCatalogSelectors(db.ListOptions{
		FieldSelector: map[string]string{"cached": "true", "refresh": "true", "repository": "paper"},
	})
	if clean.FieldSelector["cached"] != "" || clean.FieldSelector["refresh"] != "" || clean.FieldSelector["repository"] != "paper" {
		t.Fatalf("unexpected cleaned selectors: %#v", clean.FieldSelector)
	}
}

func TestConvertMirrorItemPreservesAvailableVersions(t *testing.T) {
	item := addonmirror.Item{
		ID:                "ferrite-core",
		Title:             "FerriteCore",
		LatestVersion:     "7.1.3-fabric",
		AvailableVersions: []string{"7.1.3-fabric", "7.1.1-fabric"},
		SizeBytes:         1024,
	}
	pkg := convertMirrorItem("mc-fabric-mods", item)
	if pkg.LatestVersion != "7.1.3-fabric" || !slices.Equal(pkg.AvailableVersions, []string{"7.1.3-fabric", "7.1.1-fabric"}) {
		t.Fatalf("unexpected converted package versions: %#v", pkg)
	}
}

func TestMergePersistedPackageForDetailPreservesVersionedFields(t *testing.T) {
	persisted := &registryv1.Package{
		Metadata:          metav1.ObjectMeta{Name: "mc-fabric-mods.ferrite-core", ResourceVersion: "9"},
		Repository:        "mc-fabric-mods",
		Package:           "ferrite-core",
		Title:             "FerriteCore",
		LatestVersion:     "9.0.0-fabric",
		AvailableVersions: []string{"9.0.0-fabric"},
		ModIDs:            []string{"ferritecore"},
	}
	detailed := &registryv1.Package{
		Repository:        "mc-fabric-mods",
		Package:           "ferrite-core",
		Title:             "FerriteCore",
		LatestVersion:     "7.1.3-fabric",
		AvailableVersions: []string{"7.1.3-fabric", "7.1.1-fabric"},
		Dependencies:      []registryv1.PackageDependency{{Package: "fabric-api", Title: "Fabric API"}},
	}
	merged := mergePersistedPackageForDetail(persisted, detailed)
	if merged.LatestVersion != "7.1.3-fabric" || !slices.Equal(merged.AvailableVersions, []string{"7.1.3-fabric", "7.1.1-fabric"}) {
		t.Fatalf("expected live versioned fields, got %#v", merged)
	}
	if merged.Metadata.ResourceVersion != "9" || !slices.Equal(merged.ModIDs, []string{"ferritecore"}) || len(merged.Dependencies) != 1 {
		t.Fatalf("expected persisted metadata & local modIDs plus live deps, got %#v", merged)
	}
	if persisted.LatestVersion != "9.0.0-fabric" {
		t.Fatal("expected persisted package not to be mutated")
	}
}

func TestGetForGameVersionBypassesPersistedLatestVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/project/ferrite-core":
			_, _ = w.Write([]byte(`{"id":"uXX01","slug":"ferrite-core","title":"FerriteCore"}`))
		case "/v2/project/ferrite-core/version":
			if strings.Contains(r.URL.Query().Get("game_versions"), `"1.21.4"`) {
				_, _ = w.Write([]byte(`[{"id":"v713","version_number":"7.1.3-fabric","game_versions":["1.21.4"],"loaders":["fabric"],"files":[{"url":"https://cdn.example/fc.jar","filename":"fc.jar","primary":true,"size":1024}]},{"id":"v711","version_number":"7.1.1-fabric","game_versions":["1.21.4"],"loaders":["fabric"],"files":[{"url":"https://cdn.example/fc2.jar","filename":"fc2.jar","primary":true,"size":1000}]}]`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fix := testpg.New(t, nil)
	ctx := context.Background()
	seedVersionedProxyPackage(t, ctx, fix, srv.URL+"/v2")
	op := NewOperator(fix.Registry, nil)

	globalPkg, err := op.Get(ctx, "mc-fabric-mods.ferrite-core")
	if err != nil || globalPkg.LatestVersion != "9.0.0-fabric" {
		t.Fatalf("expected unscoped Get to return persisted 9.0.0-fabric, got %#v err=%v", globalPkg, err)
	}
	v1214Pkg, err := op.GetForGameVersion(ctx, "mc-fabric-mods.ferrite-core", "1.21.4")
	if err != nil || v1214Pkg.LatestVersion != "7.1.3-fabric" || !slices.Equal(v1214Pkg.AvailableVersions, []string{"7.1.3-fabric", "7.1.1-fabric"}) {
		t.Fatalf("expected 1.21.4 GetForGameVersion to bypass persisted 26.3 latestVersion, got %#v err=%v", v1214Pkg, err)
	}
}

func seedVersionedProxyPackage(t *testing.T, ctx context.Context, fix testpg.Fixture, modrinthEndpoint string) {
	t.Helper()
	if _, err := upstreamdb.NewDB(fix.Pool).Create(ctx, &registryv1.Upstream{
		Metadata:            metav1.ObjectMeta{Name: "up-fabric"},
		Provider:            registryv1.UpstreamProviderModrinth,
		Endpoint:            modrinthEndpoint,
		ModrinthLoader:      "fabric",
		ModrinthProjectType: "mod",
	}); err != nil {
		t.Fatalf("Create upstream: %v", err)
	}
	if _, err := repositorydb.NewDB(fix.Pool).Create(ctx, &registryv1.Repository{
		Metadata: metav1.ObjectMeta{Name: "mc-fabric-mods"},
		Spec:     registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatModrinthAddon, UpstreamName: "up-fabric"},
	}); err != nil {
		t.Fatalf("Create repo: %v", err)
	}
	if _, err := pkgdb.NewDB(fix.Pool).Create(ctx, &registryv1.Package{
		Metadata:      metav1.ObjectMeta{Name: "mc-fabric-mods.ferrite-core"},
		Repository:    "mc-fabric-mods",
		Package:       "ferrite-core",
		Title:         "FerriteCore",
		LatestVersion: "9.0.0-fabric",
		ModIDs:        []string{"ferritecore"},
	}); err != nil {
		t.Fatalf("Create package: %v", err)
	}
}

func TestGetThunderstorePersistedPackageEnrichesAvailableVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/experimental/package/ValheimModding/Jotunn/":
			_, _ = w.Write([]byte(`{"namespace":"ValheimModding","name":"Jotunn","package_url":"https://thunderstore.io/c/valheim/p/ValheimModding/Jotunn/","date_updated":"2026-09-21T17:19:02Z","total_downloads":1000,"latest":{"namespace":"ValheimModding","name":"Jotunn","version_number":"2.30.2","description":"API mod","icon":"https://gcdn.thunderstore.io/icon.png","dependencies":["denikson-BepInExPack_Valheim-5.4.2360"],"file_size":4096},"community_listings":[{"community":"valheim","categories":["Mods"]}]}`))
		case "/api/cyberstorm/package/ValheimModding/Jotunn/versions/":
			_, _ = w.Write([]byte(`[{"version_number":"2.30.2"},{"version_number":"2.30.1"}]`))
		case "/api/experimental/package/ValheimModding/Jotunn/2.30.2/readme/":
			_, _ = w.Write([]byte(`{"markdown":"# Jotunn"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fix := testpg.New(t, nil)
	ctx := context.Background()
	seedThunderstoreProxyPackage(t, ctx, fix, srv.URL+"/c/valheim")

	got, err := NewOperator(fix.Registry, nil).Get(ctx, "valheim-thunderstore.ValheimModding-Jotunn")
	if err != nil {
		t.Fatalf("Get persisted Thunderstore package: %v", err)
	}
	if !slices.Equal(got.AvailableVersions, []string{"2.30.2", "2.30.1"}) || got.LatestVersion != "2.30.2" {
		t.Fatalf("expected live versions, got latest=%q available=%v", got.LatestVersion, got.AvailableVersions)
	}
	if len(got.Dependencies) != 1 || got.Dependencies[0].Package != "denikson-BepInExPack_Valheim" {
		t.Fatalf("expected BepInExPack_Valheim dep, got %#v", got.Dependencies)
	}
	if got.Metadata.ResourceVersion == "" || !strings.Contains(got.IconURL, "/icon?u=") {
		t.Fatalf("expected persisted metadata & signed icon, got rv=%q icon=%q", got.Metadata.ResourceVersion, got.IconURL)
	}
}

func seedThunderstoreProxyPackage(t *testing.T, ctx context.Context, fix testpg.Fixture, endpoint string) {
	t.Helper()
	if _, err := upstreamdb.NewDB(fix.Pool).Create(ctx, &registryv1.Upstream{
		Metadata: metav1.ObjectMeta{Name: "up-valheim-ts"},
		Provider: registryv1.UpstreamProviderThunderstore,
		Endpoint: endpoint,
	}); err != nil {
		t.Fatalf("Create upstream: %v", err)
	}
	if _, err := repositorydb.NewDB(fix.Pool).Create(ctx, &registryv1.Repository{
		Metadata: metav1.ObjectMeta{Name: "valheim-thunderstore"},
		Spec:     registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatThunderstoreMod, UpstreamName: "up-valheim-ts"},
	}); err != nil {
		t.Fatalf("Create repo: %v", err)
	}
	if _, err := pkgdb.NewDB(fix.Pool).Create(ctx, &registryv1.Package{
		Metadata:      metav1.ObjectMeta{Name: "valheim-thunderstore.ValheimModding-Jotunn"},
		Repository:    "valheim-thunderstore",
		Package:       "ValheimModding-Jotunn",
		Title:         "Jotunn",
		Description:   "cached",
		IconURL:       "https://gcdn.thunderstore.io/icon.png",
		LatestVersion: "2.30.1",
	}); err != nil {
		t.Fatalf("Create package: %v", err)
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	testpg.Stop()
	os.Exit(code)
}
