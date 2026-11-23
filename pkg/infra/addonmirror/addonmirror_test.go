package addonmirror

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/steam"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
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
		ProviderPaperMC, ProviderPurpur, ProviderVanilla, ProviderFabricMC, ProviderNeoForge, ProviderHTTP,
		ProviderModrinth, ProviderThunderstore, ProviderSteam, ProviderSteamWorkshop,
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
	_, tsSrc := NewForRegistry(nil, config.RegistryConfig{}, RegistryParams{Provider: ProviderThunderstore, Format: FormatThunderstoreMod})
	if !svc.SupportsCatalog(tsSrc) {
		t.Fatalf("expected Thunderstore catalog support")
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

func TestFabricMCListVersionsAndResolveRelease(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/versions/game", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"version":"26.4-snapshot-1","stable":false},{"version":"26.3","stable":true},{"version":"1.21.11","stable":true}]`))
	})
	mux.HandleFunc("/v2/versions/loader", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"version":"0.20.2","stable":false},{"version":"0.20.1","stable":true}]`))
	})
	mux.HandleFunc("/v2/versions/loader/26.3/0.20.1/server/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"fabric-loader-0.20.1-26.3","releaseTime":"2026-11-20T19:08:25+0000","mainClass":"net.fabricmc.loader.impl.launch.knot.KnotServer"}`))
	})
	mux.HandleFunc("/v2/versions/loader/1.21.11/0.20.1/server/json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"fabric-loader-0.20.1-1.21.11","releaseTime":"2025-12-09T12:00:00Z","mainClass":"net.fabricmc.loader.impl.launch.knot.KnotServer"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	svc := New(nil)
	vers, err := svc.ListServerVersions(context.Background(), ProviderFabricMC, srv.URL, "fabric")
	if err != nil || len(vers) != 2 || vers[0] != "26.3" || vers[1] != "1.21.11" {
		t.Fatalf("unexpected fabric game versions: %#v, err=%v", vers, err)
	}
	latestRel, err := svc.ResolveFabricMC(context.Background(), srv.URL, "latest")
	if err != nil || latestRel.Version != "26.3" || latestRel.Build != "26.3+0.20.1" ||
		latestRel.ArchiveFormat != ArchiveFormatTarGz || latestRel.PublishedAt == nil ||
		!strings.HasSuffix(latestRel.DownloadURL, "/v2/versions/loader/26.3/0.20.1/server/json") {
		t.Fatalf("unexpected latest fabric release: %#v, err=%v", latestRel, err)
	}
	pinnedRel, err := svc.ResolveRelease(context.Background(), Source{
		Provider: ProviderFabricMC, Endpoint: srv.URL, Format: FormatServerPackage,
	}, ResolveReleaseRequest{Package: "fabric", Version: "1.21.11"})
	if err != nil || pinnedRel.Version != "1.21.11" || pinnedRel.FileName != "fabric-server-1.21.11-0.20.1.tar.gz" {
		t.Fatalf("unexpected pinned fabric release: %#v, err=%v", pinnedRel, err)
	}
}

func TestModrinthModSearchIncludesServerSideFacetAndPluginExcludesIt(t *testing.T) {
	var seenFacets []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			http.NotFound(w, r)
			return
		}
		seenFacets = append(seenFacets, r.URL.Query().Get("facets"))
		_, _ = w.Write([]byte(`{"total_hits":1,"hits":[{"project_id":"P7dR8mSH","slug":"fabric-api","title":"Fabric API"}]}`))
	}))
	defer srv.Close()

	svc := New(nil, Options{ModrinthBaseURL: srv.URL})
	if _, err := svc.Search(context.Background(), Source{
		Provider: ProviderModrinth, ModrinthLoader: "fabric", ModrinthProjectType: ProjectTypeMod,
	}, SearchParams{GameVersion: "26.3"}); err != nil {
		t.Fatalf("mod search failed: %v", err)
	}
	if _, err := svc.Search(context.Background(), Source{
		Provider: ProviderModrinth, ModrinthLoader: "paper", ModrinthProjectType: ProjectTypePlugin,
	}, SearchParams{GameVersion: "26.3"}); err != nil {
		t.Fatalf("plugin search failed: %v", err)
	}
	if len(seenFacets) != 2 || !strings.Contains(seenFacets[0], `"categories:fabric"`) || !strings.Contains(seenFacets[0], `"server_side:required"`) {
		t.Fatalf("expected server_side facet on fabric mod search, got %#v", seenFacets)
	}
	if strings.Contains(seenFacets[1], "server_side") {
		t.Fatalf("did not expect server_side facet on plugin search, got %q", seenFacets[1])
	}
}

type unitTestCache struct {
	mu   sync.RWMutex
	data map[string]string
}

func (c *unitTestCache) Get(_ context.Context, k string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.data[k]
	if !ok {
		return "", cache.ErrNotFound
	}
	return v, nil
}
func (c *unitTestCache) Set(_ context.Context, k, v string, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[k] = v
	return nil
}
func (c *unitTestCache) Delete(_ context.Context, k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, k)
	return nil
}
func (c *unitTestCache) Exists(_ context.Context, k string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.data[k]
	return ok, nil
}
func (c *unitTestCache) Ping(context.Context) error { return nil }
func (c *unitTestCache) Close() error               { return nil }

func newVersionedModrinthServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/project/ferrite-core", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"uXX01","slug":"ferrite-core","title":"FerriteCore"}`))
	})
	mux.HandleFunc("/project/ferrite-core/version", handleFerriteVersions)
	mux.HandleFunc("/project/fabric-api", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"P7dR8mSH","slug":"fabric-api","title":"Fabric API"}`))
	})
	mux.HandleFunc("/project/fabric-api/version", handleFabricAPIVersions)
	mux.HandleFunc("/project/my-mod", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"m1","slug":"my-mod","title":"My Mod"}`))
	})
	mux.HandleFunc("/project/my-mod/dependencies", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"projects":[{"id":"P7dR8mSH","slug":"fabric-api","title":"Fabric API"},{"id":"cloth","slug":"cloth-config","title":"Cloth Config"}]}`))
	})
	mux.HandleFunc("/project/my-mod/version", handleMyModVersions)
	mux.HandleFunc("/project/essentialsx", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"hXiIvTyT","slug":"essentialsx","title":"EssentialsX"}`))
	})
	mux.HandleFunc("/project/essentialsx/version", handlePaperPluginVersions)
	return httptest.NewServer(mux)
}

func handleFerriteVersions(w http.ResponseWriter, r *http.Request) {
	gv := r.URL.Query().Get("game_versions")
	switch {
	case strings.Contains(gv, `"26.3"`):
		_, _ = w.Write([]byte(`[{"id":"v90","version_number":"9.0.0-fabric","game_versions":["26.3"],"loaders":["fabric"],"files":[{"url":"https://cdn.example/fc-9.0.0.jar","filename":"ferritecore-9.0.0.jar","primary":true,"size":2048}]},{"id":"v80","version_number":"8.0.0-fabric","game_versions":["26.3"],"loaders":["fabric"],"files":[{"url":"https://cdn.example/fc-8.0.0.jar","filename":"ferritecore-8.0.0.jar","primary":true,"size":1900}]}]`))
	case strings.Contains(gv, `"1.21.4"`):
		_, _ = w.Write([]byte(`[{"id":"v713","version_number":"7.1.3-fabric","game_versions":["1.21.4"],"loaders":["fabric"],"files":[{"url":"https://cdn.example/fc-7.1.3.jar","filename":"ferritecore-7.1.3.jar","primary":true,"size":1024}]},{"id":"v711","version_number":"7.1.1-fabric","game_versions":["1.21.4"],"loaders":["fabric"],"files":[{"url":"https://cdn.example/fc-7.1.1.jar","filename":"ferritecore-7.1.1.jar","primary":true,"size":1000}]}]`))
	default:
		_, _ = w.Write([]byte(`[]`))
	}
}

func handleFabricAPIVersions(w http.ResponseWriter, r *http.Request) {
	gv := r.URL.Query().Get("game_versions")
	if strings.Contains(gv, `"1.21.4"`) {
		_, _ = w.Write([]byte(`[{"id":"fa1214","version_number":"0.119.4+1.21.4","game_versions":["1.21.4"],"loaders":["fabric"],"files":[{"url":"https://cdn.example/fa-1214.jar","filename":"fabric-api-0.119.4+1.21.4.jar","primary":true,"size":4096}]}]`))
		return
	}
	_, _ = w.Write([]byte(`[]`))
}

func handleMyModVersions(w http.ResponseWriter, r *http.Request) {
	gv := r.URL.Query().Get("game_versions")
	if strings.Contains(gv, `"1.21.4"`) {
		_, _ = w.Write([]byte(`[{"id":"mm1","version_number":"[1.21.4] 1.0.0","game_versions":["1.21.4"],"loaders":["fabric"],"dependencies":[{"project_id":"P7dR8mSH","dependency_type":"required"}],"files":[{"url":"https://cdn.example/mm-1.0.0.jar","filename":"mymod-1.0.0.jar","primary":true,"size":512}]}]`))
		return
	}
	_, _ = w.Write([]byte(`[]`))
}

func handlePaperPluginVersions(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("game_versions") != "" {
		_, _ = w.Write([]byte(`[]`))
		return
	}
	_, _ = w.Write([]byte(`[{"id":"ess21","version_number":"2.21.0","game_versions":["1.21.4"],"loaders":["paper"],"files":[{"url":"https://cdn.example/ess.jar","filename":"EssentialsX-2.21.0.jar","primary":true,"size":8192}]}]`))
}

func TestModrinthCrossVersionDetailResolveAndCacheIsolation(t *testing.T) {
	srv := newVersionedModrinthServer(t)
	defer srv.Close()

	svc := New(&unitTestCache{data: make(map[string]string)}, Options{ModrinthBaseURL: srv.URL})
	src := Source{Provider: ProviderModrinth, ModrinthLoader: "fabric", ModrinthProjectType: ProjectTypeMod}
	ctx := context.Background()

	d263, err := svc.Detail(ctx, src, "ferrite-core", "26.3")
	if err != nil || d263.LatestVersion != "9.0.0-fabric" || len(d263.AvailableVersions) != 2 {
		t.Fatalf("unexpected 26.3 detail: %#v err=%v", d263, err)
	}
	d1214, err := svc.Detail(ctx, src, "ferrite-core", "1.21.4")
	if err != nil || d1214.LatestVersion != "7.1.3-fabric" || d1214.SizeBytes != 1024 ||
		!slices.Equal(d1214.AvailableVersions, []string{"7.1.3-fabric", "7.1.1-fabric"}) {
		t.Fatalf("unexpected 1.21.4 detail (cache collision?): %#v err=%v", d1214, err)
	}
	d1122, err := svc.Detail(ctx, src, "ferrite-core", "1.12.2")
	if err != nil || d1122.LatestVersion != "" || len(d1122.AvailableVersions) != 0 {
		t.Fatalf("expected empty versions on incompatible 1.12.2 mod, got %#v err=%v", d1122, err)
	}
	batch, err := svc.Resolve(ctx, src, "ferrite-core", "1.21.4")
	if err != nil || len(batch.Items) != 1 || batch.Items[0].LatestVersion != "7.1.3-fabric" {
		t.Fatalf("unexpected batch resolve for 1.21.4: %#v err=%v", batch, err)
	}
}

func TestModrinthStrictModFallbackPlusAndSanitizedCoords(t *testing.T) {
	srv := newVersionedModrinthServer(t)
	defer srv.Close()

	svc := New(&unitTestCache{data: make(map[string]string)}, Options{ModrinthBaseURL: srv.URL})
	modSrc := Source{Provider: ProviderModrinth, ModrinthLoader: "fabric", ModrinthProjectType: ProjectTypeMod}
	ctx := context.Background()

	if _, err := svc.ResolveRelease(ctx, modSrc, ResolveReleaseRequest{Package: "ferrite-core", GameVersion: "1.12.2"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for strict mod version mismatch, got %v", err)
	}
	fapi, err := svc.ResolveRelease(ctx, modSrc, ResolveReleaseRequest{Package: "fabric-api", Version: "0.119.4+1.21.4", GameVersion: "1.21.4"})
	if err != nil || fapi.Version != "0.119.4+1.21.4" {
		t.Fatalf("expected '+' version resolution, got %#v err=%v", fapi, err)
	}
	mm, err := svc.ResolveRelease(ctx, modSrc, ResolveReleaseRequest{Package: "my-mod", Version: "1.21.4-1.0.0", GameVersion: "1.21.4"})
	if err != nil || mm.Version != "1.21.4-1.0.0" || len(mm.Dependencies) != 1 || mm.Dependencies[0].ID != "fabric-api" {
		t.Fatalf("expected sanitized coord and version-scoped dep, got %#v err=%v", mm, err)
	}
	pluginSrc := Source{Provider: ProviderModrinth, ModrinthLoader: "paper", ModrinthProjectType: ProjectTypePlugin}
	if ess, err := svc.ResolveRelease(ctx, pluginSrc, ResolveReleaseRequest{Package: "essentialsx", GameVersion: "1.21.11"}); err != nil || ess.Version != "2.21.0" {
		t.Fatalf("expected graceful fallback for untagged plugin release, got %#v err=%v", ess, err)
	}
}

func TestNeoForgeListVersionsAndResolveRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/maven/versions/releases/net/neoforged/neoforge" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"isSnapshot":false,"versions":["0.25w14craftmine.3-beta","21.1.260","21.1.267","21.4.156","21.4.161","21.4.162-beta","26.1.0.19-beta","26.3.0.114","26.3.0.115"]}`))
	}))
	defer srv.Close()

	svc := New(nil)
	vers, err := svc.ListServerVersions(context.Background(), ProviderNeoForge, srv.URL, "neoforge")
	if err != nil || !slices.Equal(vers, []string{"26.3", "1.21.4", "1.21.1"}) {
		t.Fatalf("unexpected NeoForge MC versions: %#v err=%v", vers, err)
	}
	latestRel, err := svc.ResolveNeoForge(context.Background(), srv.URL, "latest")
	if err != nil || latestRel.Version != "26.3" || latestRel.Build != "26.3+26.3.0.115" ||
		latestRel.ArchiveFormat != ArchiveFormatTarGz ||
		!strings.HasSuffix(latestRel.DownloadURL, "/releases/net/neoforged/neoforge/26.3.0.115/neoforge-26.3.0.115-installer.jar") {
		t.Fatalf("unexpected latest NeoForge release: %#v err=%v", latestRel, err)
	}
	pinnedRel, err := svc.ResolveRelease(context.Background(), Source{
		Provider: ProviderNeoForge, Endpoint: srv.URL, Format: FormatServerPackage,
	}, ResolveReleaseRequest{Package: "neoforge", Version: "1.21.4"})
	if err != nil || pinnedRel.Version != "1.21.4" || pinnedRel.Build != "1.21.4+21.4.161" {
		t.Fatalf("expected 1.21.4 to pick latest non-beta 21.4.161, got %#v err=%v", pinnedRel, err)
	}
	betaRel, err := svc.ResolveNeoForge(context.Background(), srv.URL, "26.1")
	if err != nil || betaRel.Build != "26.1+26.1.0.19-beta" {
		t.Fatalf("expected beta fallback when no stable build exists for 26.1, got %#v err=%v", betaRel, err)
	}
}

func TestVanillaResolveServerMappings(t *testing.T) {
	mux := http.NewServeMux()
	var baseURL string
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"latest":{"release":"26.3"},"versions":[{"id":"26.3","type":"release","url":"` + baseURL + `/26.3.json"},{"id":"1.21.4","type":"release","url":"` + baseURL + `/1.21.4.json"}]}`))
	})
	mux.HandleFunc("/1.21.4.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"releaseTime":"2024-12-03T10:12:57Z","downloads":{"server":{"url":"` + baseURL + `/s1214.jar","sha1":"aaa"},"server_mappings":{"url":"` + baseURL + `/m1214.txt","sha1":"0b1e60cc509cfb0172573ae56b436c29febbc187"}}}`))
	})
	mux.HandleFunc("/26.3.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"releaseTime":"2026-11-18T12:00:00Z","downloads":{"server":{"url":"` + baseURL + `/s263.jar","sha1":"bbb"}}}`))
	})
	srv := httptest.NewServer(mux)
	baseURL = srv.URL
	defer srv.Close()

	svc := New(nil)
	maps, err := svc.ResolveVanillaMappings(context.Background(), srv.URL+"/manifest.json", "1.21.4")
	if err != nil || maps.Build != "0b1e60cc509cfb0172573ae56b436c29febbc187" || !strings.HasSuffix(maps.DownloadURL, "/m1214.txt") {
		t.Fatalf("unexpected 1.21.4 mappings: %#v err=%v", maps, err)
	}
	if _, err := svc.ResolveVanillaMappings(context.Background(), srv.URL+"/manifest.json", "26.3"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unobfuscated 26.3 mappings, got %v", err)
	}
}

func TestNeoForgeCatalogRetainsOlderMCVersionsBeyond300Builds(t *testing.T) {
	versions := []string{"21.1.219", "21.4.161"}
	for i := 1; i <= 360; i++ {
		versions = append(versions, "26.3.0."+strconv.Itoa(i))
	}
	payload := `{"versions":["` + strings.Join(versions, `","`) + `"]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/maven/versions/releases/net/neoforged/neoforge" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	memCache := &unitTestCache{data: make(map[string]string)}
	svc := New(memCache)
	ctx := context.Background()

	mcVers, err := svc.ListServerVersions(ctx, ProviderNeoForge, srv.URL, "neoforge")
	if err != nil || !slices.Equal(mcVers, []string{"26.3", "1.21.4", "1.21.1"}) {
		t.Fatalf("expected [26.3 1.21.4 1.21.1] retained beyond 360 builds, got %#v err=%v", mcVers, err)
	}
	rel, err := svc.ResolveNeoForge(ctx, srv.URL, "1.21.4")
	if err != nil || rel.Version != "1.21.4" || rel.Build != "1.21.4+21.4.161" {
		t.Fatalf("expected 1.21.4+21.4.161 from cached catalog, got %#v err=%v", rel, err)
	}
}

func newMockThunderstoreServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cyberstorm/listing/valheim/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ordering") != "most-downloaded" {
			t.Fatalf("expected ordering=most-downloaded, got %q", r.URL.Query().Get("ordering"))
		}
		_, _ = w.Write([]byte(`{"count":1,"results":[{"namespace":"ValheimModding","name":"Jotunn","description":"Valheim Library","icon_url":"https://ccdn.thunderstore.io/live/repository/icons/ValheimModding-Jotunn-2.30.2.png","download_count":4979287,"size":835193,"last_updated":"2026-09-21T17:19:02Z","categories":[{"name":"Libraries"}]}]}`))
	})
	mux.HandleFunc("/api/experimental/package/ValheimModding/Jotunn/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"namespace":"ValheimModding","name":"Jotunn","full_name":"ValheimModding-Jotunn","owner":"ValheimModding","package_url":"https://thunderstore.io/package/ValheimModding/Jotunn/","date_updated":"2026-09-21T17:19:02Z","community_listings":[{"community":"valheim","categories":["Libraries"]}],"latest":{"namespace":"ValheimModding","name":"Jotunn","version_number":"2.30.2","full_name":"ValheimModding-Jotunn-2.30.2","description":"Valheim Library","icon":"https://ccdn.thunderstore.io/live/repository/icons/ValheimModding-Jotunn-2.30.2.png","dependencies":["denikson-BepInExPack_Valheim-5.4.2333"],"download_url":"https://thunderstore.io/package/download/ValheimModding/Jotunn/2.30.2/","downloads":123456,"date_created":"2026-09-21T17:19:02Z"}}`))
	})
	mux.HandleFunc("/api/experimental/package/ValheimModding/Jotunn/2.29.0/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"namespace":"ValheimModding","name":"Jotunn","version_number":"2.29.0","full_name":"ValheimModding-Jotunn-2.29.0","dependencies":["denikson-BepInExPack_Valheim-5.4.2332"],"download_url":"https://thunderstore.io/package/download/ValheimModding/Jotunn/2.29.0/","date_created":"2026-08-01T00:00:00Z"}`))
	})
	mux.HandleFunc("/api/cyberstorm/package/ValheimModding/Jotunn/versions/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"version_number":"2.1.0"},{"version_number":"2.29.0"},{"version_number":"2.30.2"}]`))
	})
	mux.HandleFunc("/api/experimental/package/ValheimModding/Jotunn/2.30.2/readme/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"markdown":"# Jotunn README"}`))
	})
	return httptest.NewServer(mux)
}

func TestThunderstoreCatalogDetailResolveAndImageProxy(t *testing.T) {
	toks := ParseInputTokens(ProviderThunderstore, "https://thunderstore.io/c/valheim/p/ValheimModding/Jotunn/, Advize-PlantEverything-1.19.1, denikson/BepInExPack_Valheim")
	if !slices.Equal(toks, []string{"ValheimModding-Jotunn", "Advize-PlantEverything", "denikson-BepInExPack_Valheim"}) {
		t.Fatalf("unexpected Thunderstore tokens: %#v", toks)
	}
	srv := newMockThunderstoreServer(t)
	defer srv.Close()

	svc := New(&unitTestCache{data: make(map[string]string)}, Options{SigningSecret: "sec"})
	src := Source{Provider: ProviderThunderstore, Endpoint: srv.URL + "/c/valheim", Format: FormatThunderstoreMod, ImageProxyPrefix: "/proxy"}
	res, err := svc.Search(context.Background(), src, SearchParams{Query: "Jotunn", Page: 1, Limit: 10})
	if err != nil || len(res.Items) != 1 || res.Items[0].ID != "ValheimModding-Jotunn" || res.Items[0].IconURL == "" {
		t.Fatalf("unexpected Thunderstore search: %#v err=%v", res, err)
	}
	det, err := svc.Detail(context.Background(), src, "ValheimModding-Jotunn", "")
	if err != nil || det.LatestVersion != "2.30.2" || !slices.Equal(det.AvailableVersions, []string{"2.30.2", "2.29.0", "2.1.0"}) ||
		det.Description != "# Jotunn README" || len(det.Dependencies) != 1 || det.Dependencies[0].ID != "denikson-BepInExPack_Valheim" {
		t.Fatalf("unexpected Thunderstore detail: %#v err=%v", det, err)
	}
	resolved, err := svc.Resolve(context.Background(), src, "https://thunderstore.io/c/valheim/p/ValheimModding/Jotunn/", "")
	if err != nil || len(resolved.Items) != 1 || resolved.Items[0].LatestVersion != "2.30.2" {
		t.Fatalf("unexpected Thunderstore batch resolve: %#v err=%v", resolved, err)
	}
	relLatest, err := svc.ResolveRelease(context.Background(), src, ResolveReleaseRequest{Package: "ValheimModding-Jotunn"})
	if err != nil || relLatest.ArchiveFormat != ArchiveFormatZip || relLatest.Version != "2.30.2" {
		t.Fatalf("unexpected latest Thunderstore release: %#v err=%v", relLatest, err)
	}
	relPin, err := svc.ResolveRelease(context.Background(), src, ResolveReleaseRequest{Package: "ValheimModding-Jotunn", Version: "2.29.0"})
	if err != nil || relPin.Version != "2.29.0" || !strings.HasSuffix(relPin.DownloadURL, "/2.29.0/") {
		t.Fatalf("unexpected pinned Thunderstore release: %#v err=%v", relPin, err)
	}
	if signed := svc.SignImageProxyURL("/proxy", "https://ccdn.thunderstore.io/live/repository/icons/Jotunn.png"); signed == "" {
		t.Fatal("expected ccdn.thunderstore.io to be allowlisted for image proxy")
	}
}

func newMockUModServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/plugins/search.json", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("filter") != "rust" {
			t.Fatalf("expected filter=rust, got %q", r.URL.Query().Get("filter"))
		}
		_, _ = w.Write([]byte(`{"total":1,"data":[{"slug":"gather-manager","name":"GatherManager","title":"Gather Manager","description":"Resource rate multiplier","author":"Ryan","icon_url":"https://assets.umod.org/images/icons/plugin/gm.png","url":"https://umod.org/plugins/gather-manager","download_url":"https://umod.org/plugins/GatherManager.cs","downloads":757729,"latest_release_version":"2.2.78","latest_release_at_atom":"2022-08-06T12:25:08Z","tags_all":"rust,mechanics,resources"}]}`))
	})
	mux.HandleFunc("/games/rust.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"name":"Rust","slug":"rust","description":"Oxide modification for the game Rust","snapshot_url":"https://github.com/OxideMod/Oxide.Rust/releases/download/2.0.7912/Oxide.Rust-linux.zip","icon_url":"https://assets.umod.org/images/icons/game/rust.png","repository":"OxideMod/Oxide.Rust","url":"https://umod.org/games/rust","latest_release_version":"2.0.7912","latest_release_at_atom":"2026-11-19T07:22:15Z","watchers":3001}`))
	})
	mux.HandleFunc("/games/rust/versions.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"version":"2.0.7909","description_md":"<ul><li>Hotfix patch</li></ul>","created_at_atom":"2026-11-19T01:51:14Z"}]}`))
	})
	mux.HandleFunc("/plugins/gather-manager.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"slug":"gather-manager","name":"GatherManager","title":"Gather Manager","description":"Resource rate multiplier","author":"Ryan","icon_url":"https://assets.umod.org/images/icons/plugin/gm.png","url":"https://umod.org/plugins/gather-manager","download_url":"https://umod.org/plugins/GatherManager.cs","downloads":757729,"latest_release_version":"2.2.78","latest_release_at_atom":"2022-08-06T12:25:08Z","tags_all":"rust,mechanics,resources"}`))
	})
	mux.HandleFunc("/plugins/gather-manager/versions.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"version":"2.2.78","description_md":"<ul><li>Pickup fix</li></ul>","download_url":"https://umod.org/plugins/GatherManager.cs?version=2.2.78","created_at_atom":"2022-08-06T12:25:08Z"},{"version":"2.2.77","description_md":"Hook update","download_url":"https://umod.org/plugins/GatherManager.cs?version=2.2.77","created_at_atom":"2022-08-05T20:31:24Z"}]}`))
	})
	return httptest.NewServer(mux)
}

func TestUModCatalogDetailResolveAndImageProxy(t *testing.T) {
	toks := ParseInputTokens(ProviderUMod, "https://umod.org/plugins/gather-manager, https://umod.org/games/rust, https://umod.org/plugins/GatherManager.cs, vanish")
	if !slices.Equal(toks, []string{"gather-manager", "GatherManager", "oxide-rust", "vanish"}) {
		t.Fatalf("unexpected uMod tokens: %#v", toks)
	}
	srv := newMockUModServer(t)
	defer srv.Close()

	svc := New(&unitTestCache{data: make(map[string]string)}, Options{SigningSecret: "sec"})
	src := Source{Provider: ProviderUMod, Endpoint: srv.URL + "/games/rust", Format: FormatUModPlugin, ImageProxyPrefix: "/proxy"}
	res, err := svc.Search(context.Background(), src, SearchParams{Query: "oxide", Page: 1, Limit: 10})
	if err != nil || len(res.Items) != 2 || res.Items[0].ID != "oxide-rust" || res.Items[1].ID != "gather-manager" {
		t.Fatalf("unexpected uMod search: %#v err=%v", res, err)
	}
	det, err := svc.Detail(context.Background(), src, "gather-manager", "")
	if err != nil || det.FileName != "GatherManager.cs" || !slices.Equal(det.AvailableVersions, []string{"2.2.78", "2.2.77"}) ||
		!slices.Equal(det.Tags, []string{"mechanics", "resources"}) || len(det.Dependencies) != 1 || det.Dependencies[0].ID != "oxide-rust" {
		t.Fatalf("unexpected uMod plugin detail: %#v err=%v", det, err)
	}
	relFW, err := svc.ResolveRelease(context.Background(), src, ResolveReleaseRequest{Package: "oxide-rust"})
	if err != nil || relFW.ArchiveFormat != ArchiveFormatZip || relFW.Version != "2.0.7912" || !strings.Contains(relFW.DownloadURL, "Oxide.Rust-linux.zip") {
		t.Fatalf("unexpected Oxide framework release: %#v err=%v", relFW, err)
	}
	relPin, err := svc.ResolveRelease(context.Background(), src, ResolveReleaseRequest{Package: "gather-manager", Version: "2.2.77"})
	if err != nil || relPin.ArchiveFormat != ArchiveFormatRaw || relPin.FileName != "GatherManager.cs" || !strings.HasSuffix(relPin.DownloadURL, "version=2.2.77") {
		t.Fatalf("unexpected pinned uMod release: %#v err=%v", relPin, err)
	}
	latestMap, err := svc.ResolveLatestVersions(context.Background(), src, []string{"oxide-rust", "gather-manager"})
	if err != nil || latestMap["oxide-rust"] != "2.0.7912" || latestMap["gather-manager"] != "2.2.78" {
		t.Fatalf("unexpected uMod latest versions: %#v err=%v", latestMap, err)
	}
	if signed := svc.SignImageProxyURL("/proxy", "https://assets.umod.org/images/icons/plugin/gm.png"); signed == "" {
		t.Fatal("expected assets.umod.org to be allowlisted for image proxy")
	}
	if signed := svc.SignImageProxyURL("/proxy", "https://storage.ficsit.app/file/smr-prod-s3/images/mods/x/logo.webp"); signed == "" {
		t.Fatal("expected storage.ficsit.app to be allowlisted for image proxy")
	}
}

func TestFicsitCatalogDetailResolveAndRelease(t *testing.T) {
	toks := ParseInputTokens(ProviderFicsit, "https://ficsit.app/mod/RefinedPower, SML")
	if !slices.Equal(toks, []string{"RefinedPower", "SML"}) {
		t.Fatalf("unexpected ficsit tokens: %#v", toks)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(req.Query, "getMods") {
			_, _ = w.Write([]byte(`{"data":{"getMods":{"count":1,"mods":[{"id":"id1","mod_reference":"RefinedPower","name":"Refined Power","short_description":"Power expansion","logo":"https://storage.ficsit.app/logo.webp","downloads":1500,"authors":[{"user":{"username":"mrhid6"}}],"tags":[{"name":"power"}],"versions":[{"id":"v1","version":"2026.3.28","created_at":"2026-08-14T10:47:17Z","targets":[{"targetName":"LinuxServer","link":"/v1/version/v1/LinuxServer/download","hash":"shahex1","size":40384686}],"dependencies":[{"mod_id":"SML","condition":"^3.12.0","optional":false},{"mod_id":"Opt","condition":"*","optional":true}]}]}]}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"getModByIdOrReference":{"id":"id1","mod_reference":"RefinedPower","name":"Refined Power","short_description":"Power expansion","full_description":"Full docs","logo":"https://storage.ficsit.app/logo.webp","downloads":1500,"versions":[{"id":"v1","version":"2026.3.28","changelog":"Maintenance","created_at":"2026-08-14T10:47:17Z","targets":[{"targetName":"Windows","link":"/win","size":10},{"targetName":"LinuxServer","link":"/v1/version/v1/LinuxServer/download","hash":"shahex1","size":40384686}],"dependencies":[{"mod_id":"SML","condition":"^3.12.0","optional":false}]},{"id":"v0","version":"2026.1.0","created_at":"2026-01-01T00:00:00Z","targets":[{"targetName":"Windows","link":"/win-only","size":10}]}]}}}`))
	}))
	defer srv.Close()

	svc := New(&unitTestCache{data: make(map[string]string)}, Options{SigningSecret: "sec"})
	src := Source{Provider: ProviderFicsit, Endpoint: srv.URL + "/v2/query", Format: FormatFicsitMod, ImageProxyPrefix: "/proxy"}
	res, err := svc.Search(context.Background(), src, SearchParams{Page: 1, Limit: 10})
	if err != nil || len(res.Items) != 1 || res.Items[0].ID != "RefinedPower" || res.Items[0].LatestVersion != "2026.3.28" {
		t.Fatalf("unexpected ficsit search: %#v err=%v", res, err)
	}
	det, err := svc.Detail(context.Background(), src, "RefinedPower", "")
	if err != nil || !slices.Equal(det.AvailableVersions, []string{"2026.3.28"}) || len(det.Dependencies) != 1 || det.Dependencies[0].ID != "SML" {
		t.Fatalf("expected only LinuxServer versions & non-optional SML dep, got %#v err=%v", det, err)
	}
	rel, err := svc.ResolveRelease(context.Background(), src, ResolveReleaseRequest{Package: "RefinedPower", Version: "latest"})
	if err != nil || rel.ArchiveFormat != ArchiveFormatZip || !strings.HasPrefix(rel.DownloadURL, srv.URL+"/v1/version/v1/LinuxServer/download") || !strings.HasSuffix(rel.FileName, ".smod") {
		t.Fatalf("unexpected ficsit release: %#v err=%v", rel, err)
	}
}

func TestCS2PluginsAndWorkshopMapTags(t *testing.T) {
	toks := ParseInputTokens(ProviderCS2Plugins, "metamod, https://github.com/abnerfs/cs2-rockthevote, roflmuffin/CounterStrikeSharp")
	if !slices.Equal(toks, []string{"abnerfs--cs2-rockthevote", "metamod", "counterstrikesharp"}) {
		t.Fatalf("unexpected cs2plugins tokens: %#v", toks)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mmsource-latest-linux":
			_, _ = w.Write([]byte("mmsource-2.0.0-git1491-linux.tar.gz\n"))
		case "/repos/roflmuffin/CounterStrikeSharp/releases":
			_, _ = w.Write([]byte(`[{"tag_name":"v1.0.380","assets":[{"name":"counterstrikesharp-linux-1.0.380.zip","browser_download_url":"https://dl/no-rt.zip","size":10},{"name":"counterstrikesharp-with-runtime-linux-1.0.380.zip","browser_download_url":"https://dl/css-rt.zip","size":52}]}]`))
		case "/repos/shobhit-pathak/MatchZy/releases":
			_, _ = w.Write([]byte(`[{"tag_name":"0.9.3","assets":[{"name":"MatchZy-0.9.3-with-cssharp-linux.zip","browser_download_url":"https://dl/mz-bundle.zip","size":70},{"name":"MatchZy-0.9.3.zip","browser_download_url":"https://dl/mz.zip","size":17}]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	svc := New(&unitTestCache{data: make(map[string]string)}, Options{SigningSecret: "sec"})
	src := Source{Provider: ProviderCS2Plugins, Endpoint: srv.URL, Format: FormatCS2Plugin}
	mms, err := svc.ResolveRelease(context.Background(), src, ResolveReleaseRequest{Package: "metamod", Version: "latest"})
	if err != nil || mms.Version != "2.0.0-git1491" || mms.ArchiveFormat != ArchiveFormatTarGz {
		t.Fatalf("unexpected metamod release: %#v err=%v", mms, err)
	}
	css, err := svc.ResolveRelease(context.Background(), src, ResolveReleaseRequest{Package: "counterstrikesharp"})
	if err != nil || css.FileName != "counterstrikesharp-with-runtime-linux-1.0.380.zip" || len(css.Dependencies) != 1 || css.Dependencies[0].ID != "metamod" {
		t.Fatalf("unexpected css release: %#v err=%v", css, err)
	}
	mz, err := svc.ResolveRelease(context.Background(), src, ResolveReleaseRequest{Package: "shobhit-pathak--MatchZy"})
	if err != nil || mz.FileName != "MatchZy-0.9.3.zip" {
		t.Fatalf("expected MatchZy standalone plugin zip without cssharp bundle, got %#v err=%v", mz, err)
	}
	q := steam.BuildQueryParams("k", SteamAppCS2, SearchParams{Query: "mirage", Page: 1, Limit: 10})
	if q.Get("requiredtags[0]") != "Map" {
		t.Fatalf("expected CS2 Steam Workshop query to filter Map tag, got %v", q)
	}
}

func TestSevenDaysModsProvider(t *testing.T) {
	toks := ParseInputTokens(ProviderSevenDaysMods, "https://github.com/ZehMatt/7dtd-WalkerSim2, OCB7D2D/OcbStopFuelWaste")
	if !slices.Equal(toks, []string{"ZehMatt--7dtd-WalkerSim2", "OCB7D2D--OcbStopFuelWaste"}) {
		t.Fatalf("unexpected sevendaysmods tokens: %#v", toks)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/ZehMatt/7dtd-WalkerSim2/releases":
			_, _ = w.Write([]byte(`[{"tag_name":"2.0.5","assets":[{"name":"WalkerSim2-Editor-Windows-x64-2.0.5.zip","browser_download_url":"https://dl/win.zip","size":10},{"name":"WalkerSim2-2.0.5.zip","browser_download_url":"https://dl/ws2.zip","size":1066}]}]`))
		case "/repos/OCB7D2D/OcbStopFuelWaste/releases":
			_, _ = w.Write([]byte(`[{"tag_name":"1.1.0","assets":[{"name":"OcbStopFuelWaste-1.1.0.zip","browser_download_url":"https://dl/ocb.zip","size":5042}]}]`))
		default:
			_, _ = w.Write([]byte(`{"items":[]}`))
		}
	}))
	defer srv.Close()

	svc := New(&unitTestCache{data: make(map[string]string)}, Options{SigningSecret: "sec"})
	src := Source{Provider: ProviderSevenDaysMods, Endpoint: srv.URL, Format: FormatSevenDaysMod}
	res, err := svc.Search(context.Background(), src, SearchParams{Page: 1, Limit: 10})
	if err != nil || !res.Configured || len(res.Items) == 0 {
		t.Fatalf("unexpected sevendaysmods search: %#v err=%v", res, err)
	}
	rel, err := svc.ResolveRelease(context.Background(), src, ResolveReleaseRequest{Package: "ZehMatt--7dtd-WalkerSim2", Version: "latest"})
	if err != nil || rel.FileName != "WalkerSim2-2.0.5.zip" || rel.Version != "2.0.5" {
		t.Fatalf("expected non-Editor mod zip for WalkerSim2, got %#v err=%v", rel, err)
	}
}

func TestModIOProvider(t *testing.T) {
	toks := ParseInputTokens(ProviderModIO, "https://mod.io/g/corekeeper/m/corelib, 123456")
	if !slices.Equal(toks, []string{"corelib", "123456"}) {
		t.Fatalf("unexpected modio tokens: %#v", toks)
	}
	srv := newMockModIOServer()
	defer srv.Close()

	src := Source{Provider: ProviderModIO, Endpoint: srv.URL + "/v1/games/5289", Format: FormatModIOMod, ImageProxyPrefix: "/proxy"}
	unconfSvc := New(nil, Options{})
	if unconfRes, err := unconfSvc.Search(context.Background(), src, SearchParams{}); err != nil || unconfRes.Configured {
		t.Fatalf("expected unconfigured response when key missing, got %#v err=%v", unconfRes, err)
	}
	svc := New(&unitTestCache{data: make(map[string]string)}, Options{ModIOAPIKey: "test-modio-key", SigningSecret: "sec"})
	res, err := svc.Search(context.Background(), src, SearchParams{Page: 1, Limit: 10})
	if err != nil || !res.Configured || len(res.Items) != 1 || res.Items[0].ID != "55001" {
		t.Fatalf("unexpected modio search: %#v err=%v", res, err)
	}
	det, err := svc.Detail(context.Background(), src, "55001", "")
	if err != nil || len(det.Dependencies) != 1 || det.Dependencies[0].ID != "55000" {
		t.Fatalf("expected modio dependencies, got %#v err=%v", det, err)
	}
	rel, err := svc.ResolveRelease(context.Background(), src, ResolveReleaseRequest{Package: "corelib", Version: "1.0.0"})
	if err != nil || rel.Package != "55001" || rel.FileName != "placement-1.0.0.zip" || rel.ArchiveFormat != ArchiveFormatZip {
		t.Fatalf("unexpected modio resolved release: %#v err=%v", rel, err)
	}
}

func newMockModIOServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "test-modio-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/games/5289/mods":
			if r.URL.Query().Get("name_id") == "corelib" {
				_, _ = w.Write([]byte(`{"data":[{"id":55001,"name_id":"corelib","name":"Placement Mod","modfile":{"id":901,"version":"1.0.0","filename":"placement-1.0.0.zip","download":{"binary_url":"https://dl/placement.zip"}}}],"result_total":1}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":55001,"name_id":"corelib","name":"Placement Mod","summary":"s","profile_url":"https://mod.io/g/corekeeper/m/corelib","stats":{"downloads_total":42},"modfile":{"id":901,"version":"1.0.0","filename":"placement-1.0.0.zip","download":{"binary_url":"https://dl/placement.zip"}}}],"result_total":1}`))
		case "/v1/games/5289/mods/55001":
			_, _ = w.Write([]byte(`{"id":55001,"name_id":"corelib","name":"Placement Mod","profile_url":"https://mod.io/g/corekeeper/m/corelib","modfile":{"id":901,"version":"1.0.0","filename":"placement-1.0.0.zip","download":{"binary_url":"https://dl/placement.zip"}}}`))
		case "/v1/games/5289/mods/55001/files":
			_, _ = w.Write([]byte(`{"data":[{"id":901,"version":"1.0.0","filename":"placement-1.0.0.zip","download":{"binary_url":"https://dl/placement.zip"}}]}`))
		case "/v1/games/5289/mods/55001/dependencies":
			_, _ = w.Write([]byte(`{"data":[{"mod_id":55000,"name":"CoreLib"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}
