package addonmirror

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/steam"
)

func TestExtractProjectZomboidMetaAndBBCode(t *testing.T) {
	raw := `[h1]Brita's Weapon Pack[/h1]
Adds 300+ tactical weapons to survival servers.
[b]Workshop ID:[/b] 2200148440
[b]Mod ID:[/b] Brita; Brita_2
[b]Map Folder:[/b] RavenCreek
[b]Map Folder:[/b] Muldraugh, KY`

	mods, maps := ExtractProjectZomboidMeta(raw)
	if len(mods) != 2 || mods[0] != "Brita" || mods[1] != "Brita_2" {
		t.Fatalf("unexpected modIDs: %#v", mods)
	}
	if len(maps) != 1 || maps[0] != "RavenCreek" {
		t.Fatalf("unexpected mapFolders: %#v", maps)
	}
	summary := SummarizeText(CleanSteamBBCode(raw), 80)
	if !strings.Contains(summary, "Brita's Weapon Pack") {
		t.Fatalf("unexpected summary: %q", summary)
	}
	md := FormatSteamBBCodeMarkdown(raw + "\n[url=https://example.com]Docs[/url]\n[*]Item A")
	if !strings.Contains(md, "# Brita's Weapon Pack") || !strings.Contains(md, "[Docs](https://example.com)") || !strings.Contains(md, "- Item A") {
		t.Fatalf("unexpected formatted markdown: %q", md)
	}
}

func TestFormatSteamBBCodeNestedURLImageNoNullBytes(t *testing.T) {
	raw := "Intro text\x00\n" +
		"[url=https://steamcommunity.com/id/CrazyContraption/myworkshopfiles/?appid=1281930][img]http://i.imgur.com/aJmBEpZ.png[/img][/url]\n" +
		"[code]whitelist.txt[/code]"
	md := FormatSteamBBCodeMarkdown(raw)
	if strings.Contains(md, "\x00") {
		t.Fatalf("expected no NUL bytes in markdown, got %q", md)
	}
	want := "[![](http://i.imgur.com/aJmBEpZ.png)](https://steamcommunity.com/id/CrazyContraption/myworkshopfiles/?appid=1281930)"
	if !strings.Contains(md, want) {
		t.Fatalf("expected %q in markdown, got %q", want, md)
	}
	cleaned := CleanSteamBBCode(raw)
	if strings.Contains(cleaned, "\x00") {
		t.Fatalf("expected no NUL bytes in cleaned text, got %q", cleaned)
	}
}

func TestBuildSteamQueryParamsSorting(t *testing.T) {
	qTop := steam.BuildQueryParams("k", SteamAppDontStarveTogether, SearchParams{Sort: SortDownloads, Page: 1, Limit: 12})
	if qTop.Get("query_type") != "9" || qTop.Get("days") != "" {
		t.Fatalf("expected query_type=9 for downloads, got %v", qTop)
	}
	qTrend := steam.BuildQueryParams("k", SteamAppDontStarveTogether, SearchParams{Sort: SortTrend, Page: 1, Limit: 12})
	if qTrend.Get("query_type") != "3" || qTrend.Get("days") != "7" {
		t.Fatalf("expected query_type=3 days=7 for trend, got %v", qTrend)
	}
	qVote := steam.BuildQueryParams("k", SteamAppDontStarveTogether, SearchParams{Sort: SortTopRated, Page: 1, Limit: 12})
	if qVote.Get("query_type") != "0" {
		t.Fatalf("expected query_type=0 for toprated, got %v", qVote)
	}
	qSearch := steam.BuildQueryParams("k", SteamAppDontStarveTogether, SearchParams{Query: "Show Me", Sort: SortDownloads, Page: 1, Limit: 12})
	if qSearch.Get("query_type") != "12" || qSearch.Get("search_text") != "Show Me" {
		t.Fatalf("expected query_type=12 for text search, got %v", qSearch)
	}
}

func TestParseInputTokens(t *testing.T) {
	steamTokens := ParseInputTokens(ProviderSteamWorkshop,
		"https://steamcommunity.com/sharedfiles/filedetails/?id=2875848298&searchtext=common; 2200148440, invalid")
	if len(steamTokens) != 2 || steamTokens[0] != "2875848298" || steamTokens[1] != "2200148440" {
		t.Fatalf("unexpected steam tokens: %#v", steamTokens)
	}
	mrTokens := ParseInputTokens(ProviderModrinth, "https://modrinth.com/plugin/essentialsx, viaversion")
	if len(mrTokens) != 2 || mrTokens[0] != "essentialsx" || mrTokens[1] != "viaversion" {
		t.Fatalf("unexpected modrinth tokens: %#v", mrTokens)
	}
}

func TestSteamWorkshopSearchAndCollectionResolve(t *testing.T) {
	srv := newMockSteamServer(t)
	defer srv.Close()

	svc := New(nil, Options{
		SteamAPIBaseURL: srv.URL,
		SteamWebAPIKey:  "test-steam-key",
		SigningSecret:   "unit-test-secret",
	})
	src := Source{
		Provider:         ProviderSteamWorkshop,
		SteamAppID:       SteamAppProjectZomboid,
		ImageProxyPrefix: "/apis/core.gamepanel.io/v1/apps/project-zomboid/addon-image",
	}
	res, err := svc.Search(context.Background(), src, SearchParams{Query: "Common", Sort: SortPopular})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if !res.Configured || len(res.Items) != 1 || res.Items[0].ID != "2875848298" {
		t.Fatalf("unexpected search result: %#v", res)
	}
	if len(res.Items[0].ModIDs) != 1 || res.Items[0].ModIDs[0] != "CommonSense" {
		t.Fatalf("expected CommonSense ModID, got %#v", res.Items[0].ModIDs)
	}
	if !strings.HasPrefix(res.Items[0].IconURL, src.ImageProxyPrefix+"?u=") {
		t.Fatalf("expected signed proxy icon URL, got %q", res.Items[0].IconURL)
	}

	colRes, err := svc.Resolve(context.Background(), src, "https://steamcommunity.com/sharedfiles/filedetails/?id=999999999", "")
	if err != nil || len(colRes.Items) != 1 || colRes.Items[0].ID != "2875848298" {
		t.Fatalf("expected collection child expansion, got %#v, err=%v", colRes, err)
	}
}

func newMockSteamServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	itemJSON := `{"response":{"total":"1","publishedfiledetails":[{"publishedfileid":"2875848298","result":1,"consumer_app_id":108600,"title":"Common Sense","file_description":"Practical survival tweaks.\nWorkshop ID: 2875848298\nMod ID: CommonSense","preview_url":"https://images.steamusercontent.com/ugc/123/icon.png","file_size":"14889779","subscriptions":"950000","time_updated":1731000000}]}}`
	mux.HandleFunc("/IPublishedFileService/QueryFiles/v1/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(itemJSON))
	})
	mux.HandleFunc("/IPublishedFileService/GetDetails/v1/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(itemJSON))
	})
	mux.HandleFunc("/ISteamRemoteStorage/GetCollectionDetails/v1/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("publishedfileids[0]") == "999999999" {
			_, _ = w.Write([]byte(`{"response":{"collectiondetails":[{"publishedfileid":"999999999","result":1,"children":[{"publishedfileid":"2875848298"}]}]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"response":{"collectiondetails":[]}}`))
	})
	return httptest.NewServer(mux)
}

func TestModrinthSearchDetailAndImageProxy(t *testing.T) {
	pngHeader := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 32)...)
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"total_hits":1,"hits":[{"project_id":"hXiIvTyT","slug":"essentialsx","title":"EssentialsX","description":"Essential commands for Paper servers","icon_url":"https://cdn.modrinth.com/data/hXiIvTyT/icon.png","downloads":4200000}]}`))
	})
	mux.HandleFunc("/img.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngHeader)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc := New(nil, Options{ModrinthBaseURL: srv.URL, SigningSecret: "sec", AllowLoopbackImg: true})
	src := Source{Provider: ProviderModrinth, ModrinthLoader: "paper", ModrinthProjectType: ProjectTypePlugin}
	res, err := svc.Search(context.Background(), src, SearchParams{GameVersion: "1.21.4"})
	if err != nil || len(res.Items) != 1 || res.Items[0].ID != "essentialsx" {
		t.Fatalf("unexpected modrinth search: %#v, err=%v", res, err)
	}

	signed := svc.SignImageProxyURL("/proxy", srv.URL+"/img.png")
	u, _ := url.Parse(signed)
	data, ctype, err := svc.FetchImage(context.Background(), u.Query().Get("u"), u.Query().Get("sig"))
	if err != nil || ctype != "image/png" || len(data) != len(pngHeader) {
		t.Fatalf("unexpected FetchImage: ctype=%q len=%d err=%v", ctype, len(data), err)
	}
	if _, _, err := svc.FetchImage(context.Background(), u.Query().Get("u"), "bad-sig"); err == nil {
		t.Fatal("expected tampered signature error")
	}
}

func TestListServerVersionsPaperAndVanilla(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/projects/paper", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"versions":{"1.21":["1.21.4","1.21.5-rc1","1.21.3"],"1.20":["1.20.6"]}}`))
	})
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"versions":[{"id":"25w02a","type":"snapshot"},{"id":"1.21.4","type":"release"},{"id":"1.21.3","type":"release"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc := New(nil)
	paper, err := svc.ListServerVersions(context.Background(), ProviderPaperMC, srv.URL, "paper")
	if err != nil || len(paper) != 3 || paper[0] != "1.21.4" || paper[2] != "1.20.6" {
		t.Fatalf("unexpected paper versions: %#v, err=%v", paper, err)
	}
	vanilla, err := svc.ListServerVersions(context.Background(), ProviderVanilla, srv.URL+"/manifest.json", "vanilla")
	if err != nil || len(vanilla) != 2 || vanilla[0] != "1.21.4" || vanilla[1] != "1.21.3" {
		t.Fatalf("unexpected vanilla versions: %#v, err=%v", vanilla, err)
	}
}

func TestListServerVersionsHTTPTerrariaTModLoaderAndSteam(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/get/dedicated-servers-names", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`["terraria-server-1449.zip","terraria-server-mobile-1449.zip","terraria-server-1458.zip"]`))
	})
	mux.HandleFunc("/repos/tModLoader/tModLoader/releases", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"v2026.09.3.1","draft":false,"prerelease":false},{"tag_name":"v2026.08.3.0","draft":false,"prerelease":false},{"tag_name":"v2026.10.0.0-alpha","draft":false,"prerelease":true}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc := New(nil)
	terraria, err := svc.ListServerVersions(context.Background(), ProviderHTTP, srv.URL, "terraria-vanilla")
	if err != nil || len(terraria) != 2 || terraria[0] != "1.4.5.8" || terraria[1] != "1.4.4.9" {
		t.Fatalf("unexpected terraria versions: %#v, err=%v", terraria, err)
	}
	tml, err := svc.ListServerVersions(context.Background(), ProviderHTTP, srv.URL, "terraria-tmodloader")
	if err != nil || len(tml) != 2 || tml[0] != "v2026.09.3.1" || tml[1] != "v2026.08.3.0" {
		t.Fatalf("unexpected tmodloader versions: %#v, err=%v", tml, err)
	}
	steam, err := svc.ListServerVersions(context.Background(), ProviderSteam, "", "dst-server")
	if err != nil || len(steam) != 1 || steam[0] != "latest" {
		t.Fatalf("unexpected steam versions: %#v, err=%v", steam, err)
	}
}

func TestConfigurableImageHostMirrorsAndFallback(t *testing.T) {
	raw := "https://images.steamusercontent.com/ugc/18354810480491550448/hash/"
	defaultSvc := New(nil)
	if got := defaultSvc.candidateImageURLs(raw); len(got) != 1 || got[0] != raw {
		t.Fatalf("expected no hardcoded rewrites by default, got %#v", got)
	}

	mirroredSvc := New(nil, Options{
		ImageHostMirrors: []string{".steamusercontent.com=mirror-cdn.example.com"},
	})
	want := "https://mirror-cdn.example.com/ugc/18354810480491550448/hash/"
	if got := mirroredSvc.candidateImageURLs(raw); len(got) != 2 || got[0] != want || got[1] != raw {
		t.Fatalf("unexpected configured candidates: %#v", got)
	}
	if signed := mirroredSvc.SignImageProxyURL("/proxy", want); signed == "" {
		t.Fatal("expected configured mirror target host to be allowlisted")
	}

	pngHeader := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, make([]byte, 32)...)
	badMirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer badMirror.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngHeader)
	}))
	defer origin.Close()

	originURL, _ := url.Parse(origin.URL)
	fbSvc := New(nil, Options{
		SigningSecret:    "sec",
		AllowLoopbackImg: true,
		ImageHostMirrors: []string{originURL.Hostname() + "=" + badMirror.URL},
	})
	signed := fbSvc.SignImageProxyURL("/proxy", origin.URL+"/icon.png")
	u, _ := url.Parse(signed)
	data, ctype, err := fbSvc.FetchImage(context.Background(), u.Query().Get("u"), u.Query().Get("sig"))
	if err != nil || ctype != "image/png" || len(data) != len(pngHeader) {
		t.Fatalf("expected fallback to origin image: ctype=%q len=%d err=%v", ctype, len(data), err)
	}
}

func TestResolveChangelogAndMetadata(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v3/projects/paper/versions/1.21.4/builds/latest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":225,"time":"2026-03-10T12:00:00Z","commits":[{"sha":"abcdef123456","message":"Fix chunk unload\nDetails"}],"downloads":{"server:default":{"name":"paper-1.21.4-225.jar","url":"https://example.com/paper.jar"}}}`))
	})
	mux.HandleFunc("/v2/purpur/1.21.4/latest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"build":"2381","timestamp":1741608000000,"commits":[{"hash":"1234567890","description":"Update upstream"}]}`))
	})
	mux.HandleFunc("/project/luckperms", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"lp","slug":"luckperms","title":"LuckPerms","project_type":"plugin","description":"Permissions plugin"}`))
	})
	mux.HandleFunc("/project/luckperms/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"id":"v550","version_number":"5.5.0","changelog":"## Fixes\n- Fixed reload","date_published":"2026-03-11T08:30:00Z","files":[{"url":"https://example.com/lp.jar","filename":"lp.jar","primary":true,"size":1024}]}]`))
	})
	mux.HandleFunc("/IPublishedFileService/GetDetails/v1/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"response":{"publishedfiledetails":[{"publishedfileid":"727774324","result":1,"consumer_app_id":322330,"title":"Craft Pot","change_description":"[b]Fixed recipe[/b]","revision":12,"time_updated":1693158031}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc := New(nil, Options{ModrinthBaseURL: srv.URL, SteamAPIBaseURL: srv.URL, SteamWebAPIKey: "k"})
	paper, err := svc.ResolvePaperMC(context.Background(), srv.URL, "1.21.4")
	if err != nil || paper.PublishedAt == nil || !strings.Contains(paper.Changelog, "Fix chunk unload") || paper.UpstreamURL == "" {
		t.Fatalf("unexpected paper metadata: %#v, err=%v", paper, err)
	}
	purpur, err := svc.ResolvePurpur(context.Background(), srv.URL, "1.21.4")
	if err != nil || purpur.PublishedAt == nil || !strings.Contains(purpur.Changelog, "Update upstream") {
		t.Fatalf("unexpected purpur metadata: %#v, err=%v", purpur, err)
	}
	mr, err := svc.ResolveModrinthVersion(context.Background(), Source{Provider: ProviderModrinth}, "luckperms", "", "")
	if err != nil || mr.PublishedAt == nil || !strings.Contains(mr.Changelog, "Fixed reload") || !strings.Contains(mr.UpstreamURL, "/version/v550") {
		t.Fatalf("unexpected modrinth metadata: %#v, err=%v", mr, err)
	}
	st, err := svc.ResolveSteamWorkshopItem(context.Background(), 322330, "727774324")
	if err != nil || st.Revision != 12 || !strings.Contains(st.Changelog, "Fixed recipe") || !strings.Contains(st.UpstreamURL, "/changelog/727774324") {
		t.Fatalf("unexpected steam metadata: %#v, err=%v", st, err)
	}
}

func TestSteamNewsURLMapsDedicatedServerToBaseGame(t *testing.T) {
	cases := []struct {
		appID int
		want  string
	}{
		{SteamServerAppPalworld, "https://store.steampowered.com/news/app/1623730"},
		{SteamServerAppProjectZomboid, "https://store.steampowered.com/news/app/108600"},
		{SteamServerAppDontStarveTogether, "https://store.steampowered.com/news/app/322330"},
		{SteamServerAppSevenDaysToDie, "https://store.steampowered.com/news/app/251570"},
		{105600, "https://store.steampowered.com/news/app/105600"},
		{0, ""},
	}
	for _, tc := range cases {
		if got := SteamNewsURL(tc.appID); got != tc.want {
			t.Fatalf("SteamNewsURL(%d) = %q, want %q", tc.appID, got, tc.want)
		}
	}
	normCases := [][2]string{
		{"https://store.steampowered.com/news/app/2394010", "https://store.steampowered.com/news/app/1623730"},
		{"http://store.steampowered.com/news/app/380870?l=schinese", "https://store.steampowered.com/news/app/108600?l=schinese"},
		{"https://papermc.io/downloads/paper", "https://papermc.io/downloads/paper"},
	}
	for _, tc := range normCases {
		if got := NormalizeSteamNewsURL(tc[0]); got != tc[1] {
			t.Fatalf("NormalizeSteamNewsURL(%q) = %q, want %q", tc[0], got, tc[1])
		}
	}
}

func TestRegisteredDriversAndCapabilities(t *testing.T) {
	registered := Drivers()
	for _, want := range []string{
		ProviderPaperMC, ProviderPurpur, ProviderVanilla, ProviderHTTP,
		ProviderModrinth, ProviderSteam, ProviderSteamWorkshop,
		ProviderTerraria, ProviderTModLoader,
	} {
		if !slices.Contains(registered, want) {
			t.Fatalf("missing registered driver %q in %v", want, registered)
		}
	}
	svc, modSrc := NewForRegistry(nil, config.RegistryConfig{}, RegistryParams{Provider: ProviderModrinth, Format: "mod"})
	if !svc.SupportsCatalog(modSrc) {
		t.Fatalf("expected Modrinth catalog support")
	}
	_, wsSrc := NewForRegistry(nil, config.RegistryConfig{}, RegistryParams{Provider: ProviderSteam, Format: FormatWorkshopMod})
	if !svc.SupportsCatalog(wsSrc) {
		t.Fatalf("expected Steam workshop catalog support")
	}
	_, srvSrc := NewForRegistry(nil, config.RegistryConfig{}, RegistryParams{Provider: ProviderSteam, Format: FormatServerPackage})
	if svc.SupportsCatalog(srvSrc) {
		t.Fatalf("unexpected catalog support for Steam server_package")
	}
	_, paperSrc := NewForRegistry(nil, config.RegistryConfig{}, RegistryParams{Provider: ProviderPaperMC, Format: "jar"})
	if svc.SupportsCatalog(paperSrc) {
		t.Fatalf("unexpected catalog support for PaperMC")
	}
}

func TestResolveReleaseSteamModesAndMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ISteamRemoteStorage/GetPublishedFileDetails/v1/" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"response":{"publishedfiledetails":[{"publishedfileid":"2200148440","result":1,"consumer_app_id":108600,"title":"Brita","revision":5,"time_updated":1700000000,"file_url":"https://cdn.example.com/brita.zip","filename":"brita.zip"}]}}`))
	}))
	defer srv.Close()

	svc, srvSrc := NewForRegistry(nil, config.RegistryConfig{}, RegistryParams{
		Provider: ProviderSteam, Format: FormatServerPackage, SteamAppID: 108600,
	})
	rel, err := svc.ResolveRelease(context.Background(), srvSrc, ResolveReleaseRequest{Package: "zomboid", AllowBranches: true})
	if err != nil || rel.Version != "stable" || rel.DownloadURL != "steamcmd://108600" {
		t.Fatalf("unexpected steam server release: %#v, err=%v", rel, err)
	}
	if _, err := svc.ResolveRelease(context.Background(), srvSrc, ResolveReleaseRequest{Package: "zomboid", Version: "beta", StrictVersion: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for strict non-latest steam server version, got %v", err)
	}

	svc, wsSrc := NewForRegistry(nil, config.RegistryConfig{}, RegistryParams{
		Provider: ProviderSteam, Format: FormatWorkshopMod, Endpoint: srv.URL, SteamAppID: 108600,
	})
	wsRel, err := svc.ResolveRelease(context.Background(), wsSrc, ResolveReleaseRequest{Package: "2200148440", Version: "old-ver", StrictVersion: false})
	if err != nil || !wsRel.VersionMismatch || wsRel.Build != "rev-5" {
		t.Fatalf("expected VersionMismatch=true and rev-5, got %#v, err=%v", wsRel, err)
	}
	if _, err := svc.ResolveRelease(context.Background(), wsSrc, ResolveReleaseRequest{Package: "2200148440", Version: "old-ver", StrictVersion: true}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on strict workshop mismatch, got %v", err)
	}
	exact, err := svc.ResolveRelease(context.Background(), wsSrc, ResolveReleaseRequest{Package: "2200148440", Version: "1700000000", StrictVersion: true})
	if err != nil || exact.VersionMismatch || exact.DownloadURL == "" {
		t.Fatalf("expected exact workshop match, got %#v, err=%v", exact, err)
	}
}

type customExtTestDriver struct{ BaseDriver }

func (customExtTestDriver) Name() string { return "custom-ext-test" }
func (customExtTestDriver) ResolveRelease(_ context.Context, _ Runtime, _ Source, req ResolveReleaseRequest) (*ResolvedRelease, error) {
	return &ResolvedRelease{Package: req.Package, Version: req.Version, DownloadURL: "https://custom.invalid/" + req.Package}, nil
}

func TestCustomDriverRegistrationExtensibility(t *testing.T) {
	Register("custom-ext-test", func() Driver { return customExtTestDriver{} })
	t.Cleanup(func() { Unregister("custom-ext-test") })
	svc, src := NewForRegistry(nil, config.RegistryConfig{}, RegistryParams{Provider: "custom-ext-test"})
	rel, err := svc.ResolveRelease(context.Background(), src, ResolveReleaseRequest{Package: "demo", Version: "1.0"})
	if err != nil || rel.DownloadURL != "https://custom.invalid/demo" {
		t.Fatalf("expected custom driver release, got %#v, err=%v", rel, err)
	}
}

func TestSanitizeUpstreamHTTPErrorRedactsQuerySecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := srv.URL
	srv.Close()

	svc := New(nil, Options{Timeout: time.Second})
	var dst map[string]any
	err := svc.DoGetJSON(context.Background(), baseURL+"/IPublishedFileService/QueryFiles/v1/?key=SECRET_STEAM_KEY&appid=322330", &dst)
	if err == nil {
		t.Fatal("expected error from closed server")
	}
	if strings.Contains(err.Error(), "SECRET_STEAM_KEY") || strings.Contains(err.Error(), "QueryFiles") {
		t.Fatalf("expected query params and API key stripped from error, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "request failed:") {
		t.Fatalf("expected sanitized host error message, got %q", err.Error())
	}
}

func TestDoGetJSONRetriesUpToThreeAttempts(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			http.Error(w, "transient gateway error", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	svc := New(nil, Options{Timeout: 2 * time.Second})
	var out struct {
		OK bool `json:"ok"`
	}
	if err := svc.DoGetJSON(context.Background(), srv.URL+"/test", &out); err != nil {
		t.Fatalf("expected DoGetJSON to succeed on 3rd attempt, got %v", err)
	}
	if !out.OK || hits.Load() != 3 {
		t.Fatalf("expected 3 attempts and ok=true, got hits=%d out=%+v", hits.Load(), out)
	}
}
