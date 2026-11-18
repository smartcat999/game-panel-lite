package pkg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
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
