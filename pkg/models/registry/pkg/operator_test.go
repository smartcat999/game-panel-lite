package pkg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
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

func TestGetUModPersistedPackageEnrichesAvailableVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plugins/gather-manager.json":
			_, _ = w.Write([]byte(`{"slug":"gather-manager","name":"GatherManager","title":"Gather Manager","description":"Gather rates","icon_url":"https://assets.umod.org/icon.png","latest_release_version":"2.2.78","download_url":"https://umod.org/plugins/GatherManager.cs"}`))
		case "/plugins/gather-manager/versions.json":
			_, _ = w.Write([]byte(`{"data":[{"version":"2.2.78"},{"version":"2.2.77"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fix := testpg.New(t, nil)
	ctx := context.Background()
	seedUModProxyPackage(t, ctx, fix, srv.URL+"/games/rust")

	got, err := NewOperator(fix.Registry, nil).Get(ctx, "rust-umod.gather-manager")
	if err != nil {
		t.Fatalf("Get persisted uMod package: %v", err)
	}
	if !slices.Equal(got.AvailableVersions, []string{"2.2.78", "2.2.77"}) || got.LatestVersion != "2.2.78" {
		t.Fatalf("expected live uMod versions, got latest=%q available=%v", got.LatestVersion, got.AvailableVersions)
	}
	if len(got.Dependencies) != 1 || got.Dependencies[0].Package != "oxide-rust" {
		t.Fatalf("expected oxide-rust dependency, got %#v", got.Dependencies)
	}
	if !strings.Contains(got.IconURL, "/icon?u=") {
		t.Fatalf("expected signed uMod icon URL, got %q", got.IconURL)
	}
}

func TestGetPersistedFicsitAndCS2ProxyPackageOverlaysLiveVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/query":
			_, _ = w.Write([]byte(`{"data":{"getModByIdOrReference":{"id":"id1","mod_reference":"InfiniteZoop","name":"Infinite Zoop","versions":[{"id":"v2","version":"1.11.2","targets":[{"targetName":"LinuxServer","link":"/v1/version/v2/LinuxServer/download","size":123}],"dependencies":[{"mod_id":"SML","condition":"^3.12.0","optional":false}]}]}}}`))
		case "/repos/abnerfs/cs2-rockthevote/releases":
			_, _ = w.Write([]byte(`[{"tag_name":"v1.8.5","assets":[{"name":"RockTheVote_v1.8.5.zip","browser_download_url":"https://dl/rtv.zip","size":500}]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fix := testpg.New(t, nil)
	ctx := context.Background()
	seedFicsitAndCS2ProxyPackages(t, ctx, fix, srv.URL)
	op := NewOperator(fix.Registry, nil)
	sfPkg, err := op.Get(ctx, "satisfactory-ficsit.InfiniteZoop")
	if err != nil || sfPkg.LatestVersion != "1.11.2" || len(sfPkg.Dependencies) != 1 || sfPkg.Dependencies[0].Package != "SML" {
		t.Fatalf("unexpected live ficsit package overlay: %+v err=%v", sfPkg, err)
	}
	cs2Pkg, err := op.Get(ctx, "cs2-plugins.abnerfs--cs2-rockthevote")
	if err != nil || cs2Pkg.LatestVersion != "v1.8.5" || len(cs2Pkg.Dependencies) != 1 || cs2Pkg.Dependencies[0].Package != "counterstrikesharp" {
		t.Fatalf("unexpected live cs2 plugin overlay: %+v err=%v", cs2Pkg, err)
	}
}

func seedFicsitAndCS2ProxyPackages(t *testing.T, ctx context.Context, fix testpg.Fixture, endpoint string) {
	t.Helper()
	for _, up := range []*registryv1.Upstream{
		{Metadata: metav1.ObjectMeta{Name: "up-ficsit"}, Provider: registryv1.UpstreamProviderFicsit, Endpoint: endpoint + "/v2/query"},
		{Metadata: metav1.ObjectMeta{Name: "up-cs2p"}, Provider: registryv1.UpstreamProviderCS2Plugins, Endpoint: endpoint},
	} {
		if _, err := upstreamdb.NewDB(fix.Pool).Create(ctx, up); err != nil {
			t.Fatalf("Create upstream: %v", err)
		}
	}
	for _, repo := range []*registryv1.Repository{
		{Metadata: metav1.ObjectMeta{Name: "satisfactory-ficsit"}, Spec: registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatFicsitMod, UpstreamName: "up-ficsit"}},
		{Metadata: metav1.ObjectMeta{Name: "cs2-plugins"}, Spec: registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatCS2Plugin, UpstreamName: "up-cs2p"}},
	} {
		if _, err := repositorydb.NewDB(fix.Pool).Create(ctx, repo); err != nil {
			t.Fatalf("Create repo: %v", err)
		}
	}
	for _, pkg := range []*registryv1.Package{
		{Metadata: metav1.ObjectMeta{Name: "satisfactory-ficsit.InfiniteZoop"}, Repository: "satisfactory-ficsit", Package: "InfiniteZoop", Title: "Infinite Zoop", LatestVersion: "1.0.0"},
		{Metadata: metav1.ObjectMeta{Name: "cs2-plugins.abnerfs--cs2-rockthevote"}, Repository: "cs2-plugins", Package: "abnerfs--cs2-rockthevote", Title: "CS2 RTV", LatestVersion: "v1.0.0"},
	} {
		if _, err := pkgdb.NewDB(fix.Pool).Create(ctx, pkg); err != nil {
			t.Fatalf("Create pkg: %v", err)
		}
	}
}

func TestGetPersistedSevenDaysAndModIOProxyPackageOverlaysLiveVersions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/OCB7D2D/OcbStopFuelWaste/releases":
			_, _ = w.Write([]byte(`[{"tag_name":"1.2.0","assets":[{"name":"OcbStopFuelWaste-1.2.0.zip","browser_download_url":"https://dl/ocb.zip","size":5042}]}]`))
		case "/v1/games/5289/mods/55001":
			_, _ = w.Write([]byte(`{"id":55001,"name_id":"placement","name":"Placement Mod","modfile":{"id":902,"version":"1.2.0","filename":"placement.zip","download":{"binary_url":"https://dl/p.zip"}}}`))
		case "/v1/games/5289/mods/55001/files":
			_, _ = w.Write([]byte(`{"data":[{"id":902,"version":"1.2.0","filename":"placement.zip","download":{"binary_url":"https://dl/p.zip"}}]}`))
		case "/v1/games/5289/mods/55001/dependencies":
			_, _ = w.Write([]byte(`{"data":[{"mod_id":55000,"name":"CoreLib"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fix := testpg.New(t, nil)
	ctx := context.Background()
	seedSevenDaysAndModIOProxyPackages(t, ctx, fix, srv.URL)
	op := NewOperator(fix.Registry, nil)
	sdPkg, err := op.Get(ctx, "seven-days-mods.OCB7D2D--OcbStopFuelWaste")
	if err != nil || sdPkg.LatestVersion != "1.2.0" {
		t.Fatalf("unexpected live 7dtd mod overlay: %+v err=%v", sdPkg, err)
	}
	ckPkg, err := op.Get(ctx, "core-keeper-modio.55001")
	if err != nil || ckPkg.LatestVersion != "1.2.0" || len(ckPkg.Dependencies) != 1 || ckPkg.Dependencies[0].Package != "55000" {
		t.Fatalf("unexpected live mod.io overlay: %+v err=%v", ckPkg, err)
	}
}

func seedSevenDaysAndModIOProxyPackages(t *testing.T, ctx context.Context, fix testpg.Fixture, endpoint string) {
	t.Helper()
	for _, up := range []*registryv1.Upstream{
		{Metadata: metav1.ObjectMeta{Name: "up-7dtd"}, Provider: registryv1.UpstreamProviderSevenDaysMods, Endpoint: endpoint},
		{Metadata: metav1.ObjectMeta{Name: "up-modio"}, Provider: registryv1.UpstreamProviderModIO, Endpoint: endpoint + "/v1/games/5289", APIKey: "k-modio"},
	} {
		if _, err := upstreamdb.NewDB(fix.Pool).Create(ctx, up); err != nil {
			t.Fatalf("Create upstream: %v", err)
		}
	}
	for _, repo := range []*registryv1.Repository{
		{Metadata: metav1.ObjectMeta{Name: "seven-days-mods"}, Spec: registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatSevenDaysMod, UpstreamName: "up-7dtd"}},
		{Metadata: metav1.ObjectMeta{Name: "core-keeper-modio"}, Spec: registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatModIOMod, UpstreamName: "up-modio"}},
	} {
		if _, err := repositorydb.NewDB(fix.Pool).Create(ctx, repo); err != nil {
			t.Fatalf("Create repo: %v", err)
		}
	}
	for _, pkg := range []*registryv1.Package{
		{Metadata: metav1.ObjectMeta{Name: "seven-days-mods.OCB7D2D--OcbStopFuelWaste"}, Repository: "seven-days-mods", Package: "OCB7D2D--OcbStopFuelWaste", Title: "OCB Stop Fuel Waste", LatestVersion: "1.0.0"},
		{Metadata: metav1.ObjectMeta{Name: "core-keeper-modio.55001"}, Repository: "core-keeper-modio", Package: "55001", Title: "Placement Mod", LatestVersion: "1.0.0"},
	} {
		if _, err := pkgdb.NewDB(fix.Pool).Create(ctx, pkg); err != nil {
			t.Fatalf("Create pkg: %v", err)
		}
	}
}

func seedUModProxyPackage(t *testing.T, ctx context.Context, fix testpg.Fixture, endpoint string) {
	t.Helper()
	if _, err := upstreamdb.NewDB(fix.Pool).Create(ctx, &registryv1.Upstream{
		Metadata: metav1.ObjectMeta{Name: "up-rust-umod"},
		Provider: registryv1.UpstreamProviderUMod,
		Endpoint: endpoint,
	}); err != nil {
		t.Fatalf("Create upstream: %v", err)
	}
	if _, err := repositorydb.NewDB(fix.Pool).Create(ctx, &registryv1.Repository{
		Metadata: metav1.ObjectMeta{Name: "rust-umod"},
		Spec:     registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatUModPlugin, UpstreamName: "up-rust-umod"},
	}); err != nil {
		t.Fatalf("Create repo: %v", err)
	}
	if _, err := pkgdb.NewDB(fix.Pool).Create(ctx, &registryv1.Package{
		Metadata:      metav1.ObjectMeta{Name: "rust-umod.gather-manager"},
		Repository:    "rust-umod",
		Package:       "gather-manager",
		Title:         "Gather Manager",
		Description:   "cached",
		IconURL:       "https://assets.umod.org/icon.png",
		LatestVersion: "2.2.77",
	}); err != nil {
		t.Fatalf("Create package: %v", err)
	}
}

type mockPkgCache struct {
	mu   sync.RWMutex
	data map[string]string
}

func (m *mockPkgCache) Get(_ context.Context, k string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[k]
	if !ok {
		return "", cache.ErrNotFound
	}
	return v, nil
}
func (m *mockPkgCache) Set(_ context.Context, k, v string, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[k] = v
	return nil
}
func (m *mockPkgCache) Delete(_ context.Context, k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, k)
	return nil
}
func (m *mockPkgCache) Exists(_ context.Context, k string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[k]
	return ok, nil
}
func (m *mockPkgCache) Ping(_ context.Context) error { return nil }
func (m *mockPkgCache) Close() error                 { return nil }

func TestHydratePersistedPackagePersistsRawIconAndCoolsDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mmsource-latest-linux":
			_, _ = w.Write([]byte("mmsource-2.0.0-git1491-linux.tar.gz\n"))
		case "/repos/roflmuffin/CounterStrikeSharp/releases":
			_, _ = w.Write([]byte(`[{"tag_name":"v1.0.381","assets":[{"name":"counterstrikesharp-with-runtime-linux-1.0.381.zip","browser_download_url":"https://dl/css.zip","size":100}]}]`))
		case "/repos/roflmuffin/CounterStrikeSharp":
			_, _ = w.Write([]byte(`{"full_name":"roflmuffin/CounterStrikeSharp","description":"CSS","owner":{"login":"roflmuffin","avatar_url":"https://avatars.githubusercontent.com/u/4160975?v=4"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fix := testpg.New(t, nil)
	ctx := context.Background()
	seedCS2HydrationPackages(t, ctx, fix, srv.URL)

	kv := &mockPkgCache{data: make(map[string]string)}
	op := NewOperator(fix.Registry, kv)
	list, err := op.List(ctx, db.ListOptions{FieldSelector: map[string]string{"cached": "true", "refresh": "true"}})
	if err != nil || len(list.Items) != 2 {
		t.Fatalf("List refresh failed: %v %+v", err, list)
	}
	for _, item := range list.Items {
		if !strings.Contains(item.IconURL, "/icon?u=") {
			t.Fatalf("expected signed proxy icon in API response for %s, got %q", item.Package, item.IconURL)
		}
	}
	storedMMS, err := pkgdb.NewDB(fix.Pool).Get(ctx, "cs2-plugins.metamod")
	if err != nil || storedMMS.IconURL != addonmirror.DefaultGitHubAvatarBaseURL+"/alliedmodders?s=128" || !strings.Contains(storedMMS.Title, "Metamod:Source") {
		t.Fatalf("expected hydrated metamod persisted with raw icon & title, got %#v err=%v", storedMMS, err)
	}
	if storedMMS.Metadata.Generation != 1 {
		t.Fatalf("expected metadata hydration not to bump generation, got %d", storedMMS.Metadata.Generation)
	}
	storedCSS, err := pkgdb.NewDB(fix.Pool).Get(ctx, "cs2-plugins.counterstrikesharp")
	if err != nil || storedCSS.IconURL != "https://avatars.githubusercontent.com/u/4160975?v=4" {
		t.Fatalf("expected hydrated counterstrikesharp persisted with raw icon, got %#v err=%v", storedCSS, err)
	}
	if storedCSS.Metadata.Generation != 1 {
		t.Fatalf("expected CSS metadata hydration not to bump generation, got %d", storedCSS.Metadata.Generation)
	}
	if noopMerged, changed := mergeHydratedPackageMetadata(storedCSS, storedCSS); changed || noopMerged.IconURL != storedCSS.IconURL {
		t.Fatalf("expected identical metadata merge to report changed=false")
	}
	paged, err := op.List(ctx, db.ListOptions{Limit: 1})
	if err != nil || paged.Metadata.Continue == "" {
		t.Fatalf("expected Continue token on paged DB list, got meta=%+v err=%v", paged, err)
	}
}

func seedCS2HydrationPackages(t *testing.T, ctx context.Context, fix testpg.Fixture, endpoint string) {
	t.Helper()
	if _, err := upstreamdb.NewDB(fix.Pool).Create(ctx, &registryv1.Upstream{
		Metadata: metav1.ObjectMeta{Name: "up-cs2p-hydrate"},
		Provider: registryv1.UpstreamProviderCS2Plugins,
		Endpoint: endpoint,
	}); err != nil {
		t.Fatalf("Create upstream: %v", err)
	}
	if _, err := repositorydb.NewDB(fix.Pool).Create(ctx, &registryv1.Repository{
		Metadata: metav1.ObjectMeta{Name: "cs2-plugins"},
		Spec:     registryv1.RepositorySpec{Mode: registryv1.RepositoryModeProxy, Format: registryv1.FormatCS2Plugin, UpstreamName: "up-cs2p-hydrate"},
	}); err != nil {
		t.Fatalf("Create repo: %v", err)
	}
	for _, pkg := range []*registryv1.Package{
		{Metadata: metav1.ObjectMeta{Name: "cs2-plugins.metamod"}, Repository: "cs2-plugins", Package: "metamod", Title: "metamod", LatestVersion: "2.0.0-git1491"},
		{Metadata: metav1.ObjectMeta{Name: "cs2-plugins.counterstrikesharp"}, Repository: "cs2-plugins", Package: "counterstrikesharp", Title: "CounterStrikeSharp", Author: "roflmuffin", LatestVersion: "v1.0.381"},
	} {
		if _, err := pkgdb.NewDB(fix.Pool).Create(ctx, pkg); err != nil {
			t.Fatalf("Create pkg: %v", err)
		}
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	testpg.Stop()
	os.Exit(code)
}
