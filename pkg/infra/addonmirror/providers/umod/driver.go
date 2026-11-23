package umod

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const (
	maxAvailableVersions = 25
	resolveConcurrency   = 4
	frameworkPrefix      = "oxide-"
)

var (
	pluginURLRe = regexp.MustCompile(`(?i)umod\.org/plugins/([^\s,;/?#"<>]+)`)
	gameURLRe   = regexp.MustCompile(`(?i)umod\.org/games/([^\s,;/?#"<>]+)`)
	liTagRe     = regexp.MustCompile(`(?i)<li[^>]*>`)
	htmlTagRe   = regexp.MustCompile(`<[^>]+>`)
)

func init() {
	driver.Register(driver.ProviderUMod, func() driver.Driver { return &umodDriver{} })
}

type umodDriver struct {
	driver.BaseDriver
}

func (d *umodDriver) Name() string { return driver.ProviderUMod }

func (d *umodDriver) Capabilities(src driver.Source) driver.Capabilities {
	return driver.Capabilities{Catalog: src.Format != driver.FormatServerPackage}
}

func (d *umodDriver) ParseTokens(raw string) []string {
	var out []string
	add := func(tok string) {
		if tok != "" && !driver.ContainsString(out, tok) {
			out = append(out, tok)
		}
	}
	for _, m := range pluginURLRe.FindAllStringSubmatch(raw, -1) {
		add(stripKnownExt(m[1]))
	}
	for _, m := range gameURLRe.FindAllStringSubmatch(raw, -1) {
		if game := strings.ToLower(stripKnownExt(m[1])); game != "" {
			add(frameworkPrefix + game)
		}
	}
	for _, tok := range driver.SplitPlainTokens(raw, normalizePlainToken) {
		add(tok)
	}
	return out
}

func (d *umodDriver) Search(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	return search(ctx, rt, src, params)
}

func (d *umodDriver) Detail(ctx context.Context, rt driver.Runtime, src driver.Source, id, _ string) (*driver.Item, error) {
	return detail(ctx, rt, src, id)
}

func (d *umodDriver) Resolve(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string, _ string) ([]driver.Item, error) {
	return resolveBatch(ctx, rt, src, tokens)
}

func (d *umodDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	if src.Format == driver.FormatServerPackage {
		return nil, driver.ErrUnsupportedSource
	}
	return resolveRelease(ctx, rt, src, req.Package, req.Version)
}

func (d *umodDriver) ResolveLatestVersions(ctx context.Context, rt driver.Runtime, src driver.Source, pkgIDs []string) (map[string]string, error) {
	baseURL, gameSlug, err := parseUModEndpoint(src.Endpoint)
	if err != nil {
		return nil, err
	}
	return resolveLatestBatch(ctx, rt, baseURL, gameSlug, pkgIDs), nil
}

type pluginSearchResponse struct {
	Total int          `json:"total"`
	Data  []pluginJSON `json:"data"`
}

type pluginJSON struct {
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	Title             string `json:"title"`
	Description       string `json:"description"`
	Author            string `json:"author"`
	IconURL           string `json:"icon_url"`
	URL               string `json:"url"`
	DownloadURL       string `json:"download_url"`
	Downloads         int64  `json:"downloads"`
	LatestVersion     string `json:"latest_release_version"`
	LatestReleaseAtom string `json:"latest_release_at_atom"`
	UpdatedAtAtom     string `json:"updated_at_atom"`
	TagsAll           string `json:"tags_all"`
}

type gameJSON struct {
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	Description       string `json:"description"`
	SnapshotURL       string `json:"snapshot_url"`
	IconURL           string `json:"icon_url"`
	Repository        string `json:"repository"`
	DownloadURL       string `json:"download_url"`
	URL               string `json:"url"`
	LatestVersion     string `json:"latest_release_version"`
	LatestReleaseAtom string `json:"latest_release_at_atom"`
	Watchers          int64  `json:"watchers"`
}

type versionsResponse struct {
	Data []versionJSON `json:"data"`
}

type versionJSON struct {
	Version       string `json:"version"`
	Description   string `json:"description"`
	DescriptionMD string `json:"description_md"`
	DownloadURL   string `json:"download_url"`
	CreatedAtAtom string `json:"created_at_atom"`
}

func parseUModEndpoint(rawEndpoint string) (string, string, error) {
	trimmed := strings.TrimSpace(rawEndpoint)
	if trimmed == "" {
		trimmed = driver.DefaultUModBaseURL + "/games/rust"
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("%w: invalid umod endpoint %q", driver.ErrInvalidRequest, rawEndpoint)
	}
	baseURL := u.Scheme + "://" + u.Host
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i+1 < len(segs); i++ {
		if strings.EqualFold(segs[i], "games") && segs[i+1] != "" {
			return baseURL, strings.ToLower(stripKnownExt(segs[i+1])), nil
		}
	}
	return "", "", fmt.Errorf("%w: umod endpoint %q must include /games/<game>", driver.ErrInvalidRequest, rawEndpoint)
}

func stripKnownExt(raw string) string {
	s := strings.TrimSpace(raw)
	for _, ext := range []string{".json", ".cs"} {
		if len(s) > len(ext) && strings.EqualFold(s[len(s)-len(ext):], ext) {
			return s[:len(s)-len(ext)]
		}
	}
	return s
}

func normalizePlainToken(tok string) (string, bool) {
	clean := stripKnownExt(tok)
	if clean == "" {
		return "", false
	}
	return clean, true
}

func frameworkID(gameSlug string) string {
	return frameworkPrefix + strings.ToLower(strings.TrimSpace(gameSlug))
}

func isFrameworkID(id, gameSlug string) bool {
	return strings.EqualFold(strings.TrimSpace(id), frameworkID(gameSlug))
}

func search(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	baseURL, gameSlug, err := parseUModEndpoint(src.Endpoint)
	if err != nil {
		return nil, err
	}
	page := max(params.Page, 1)
	sortKey := mapUModSort(params.Sort)
	ck := driver.CacheHashKey("umod:search", fmt.Sprintf("%s|%s|%s|%s|%d", baseURL, gameSlug, sortKey, strings.TrimSpace(params.Query), page))
	var cached driver.Result
	if rt.GetCachedJSON(ctx, ck, &cached) {
		return sliceResultLimit(&cached, params.Limit), nil
	}
	res, err := querySearch(ctx, rt, baseURL, gameSlug, params.Query, sortKey, page)
	if err != nil {
		return nil, err
	}
	rt.SetCachedJSON(ctx, ck, res)
	return sliceResultLimit(res, params.Limit), nil
}

func querySearch(ctx context.Context, rt driver.Runtime, baseURL, gameSlug, query, sortKey string, page int) (*driver.Result, error) {
	q := url.Values{}
	q.Set("query", strings.TrimSpace(query))
	q.Set("page", strconv.Itoa(page))
	q.Set("sort", sortKey)
	q.Set("sortdir", "desc")
	q.Set("filter", gameSlug)
	u := fmt.Sprintf("%s/plugins/search.json?%s", baseURL, q.Encode())
	var raw pluginSearchResponse
	if err := rt.DoGetJSON(ctx, u, &raw); err != nil {
		return nil, err
	}
	dep := frameworkDependency(gameSlug)
	items := make([]driver.Item, 0, len(raw.Data)+1)
	if page == 1 && matchesFrameworkQuery(query) {
		if fw, fErr := detailFramework(ctx, rt, baseURL, gameSlug); fErr == nil && fw != nil {
			items = append(items, *fw)
		}
	}
	for _, p := range raw.Data {
		items = append(items, buildPluginItem(&p, gameSlug, nil, "", dep))
	}
	return &driver.Result{Configured: true, Total: raw.Total, Items: items}, nil
}

func mapUModSort(sortMode string) string {
	switch sortMode {
	case driver.SortUpdated:
		return "latest_release_at"
	case driver.SortNewest:
		return "published_at"
	case driver.SortTopRated, driver.SortTrend:
		return "watchers"
	default:
		return "downloads"
	}
}

func matchesFrameworkQuery(q string) bool {
	low := strings.ToLower(strings.TrimSpace(q))
	return low == "oxide" || low == "umod" || strings.HasPrefix(low, "oxide-")
}

func sliceResultLimit(res *driver.Result, limit int) *driver.Result {
	if res == nil || limit <= 0 || len(res.Items) <= limit {
		return res
	}
	cp := *res
	cp.Items = res.Items[:limit]
	return &cp
}

func detail(ctx context.Context, rt driver.Runtime, src driver.Source, id string) (*driver.Item, error) {
	baseURL, gameSlug, err := parseUModEndpoint(src.Endpoint)
	if err != nil {
		return nil, err
	}
	if isFrameworkID(id, gameSlug) {
		return detailFramework(ctx, rt, baseURL, gameSlug)
	}
	return detailPlugin(ctx, rt, baseURL, gameSlug, id)
}

func detailFramework(ctx context.Context, rt driver.Runtime, baseURL, gameSlug string) (*driver.Item, error) {
	game, err := fetchGameMeta(ctx, rt, baseURL, gameSlug)
	if err != nil {
		return nil, err
	}
	vers, _ := fetchGameVersions(ctx, rt, baseURL, gameSlug)
	avail, changelog := collectVersionMeta(vers, game.LatestVersion)
	titleName := driver.FirstNonEmpty(game.Name, gameSlug)
	dlURL := selectFrameworkLatestURL(game, baseURL, gameSlug)
	item := &driver.Item{
		ID:                frameworkID(gameSlug),
		Title:             "Oxide (" + titleName + ")",
		Summary:           driver.FirstNonEmpty(game.Description, "Oxide/uMod framework for "+titleName),
		Description:       driver.FirstNonEmpty(game.Description, "Oxide/uMod framework for "+titleName),
		Author:            "OxideMod",
		IconURL:           strings.TrimSpace(game.IconURL),
		SourceURL:         driver.FirstNonEmpty(game.URL, fmt.Sprintf("%s/games/%s", baseURL, url.PathEscape(gameSlug))),
		FileURL:           dlURL,
		FileName:          oxideArchiveFileName(gameSlug, game.LatestVersion),
		LatestVersion:     strings.TrimSpace(game.LatestVersion),
		AvailableVersions: avail,
		Changelog:         changelog,
		Downloads:         game.Watchers,
		UpdatedAt:         strings.TrimSpace(game.LatestReleaseAtom),
		Tags:              []string{"Oxide", "Framework"},
	}
	return item, nil
}

func detailPlugin(ctx context.Context, rt driver.Runtime, baseURL, gameSlug, id string) (*driver.Item, error) {
	p, err := fetchPluginMeta(ctx, rt, baseURL, id)
	if err != nil {
		return nil, err
	}
	slug := driver.FirstNonEmpty(p.Slug, id)
	vers, _ := fetchPluginVersions(ctx, rt, baseURL, slug)
	avail, changelog := collectVersionMeta(vers, p.LatestVersion)
	item := buildPluginItem(p, gameSlug, avail, changelog, frameworkDependency(gameSlug))
	return &item, nil
}

func buildPluginItem(p *pluginJSON, gameSlug string, versions []string, changelog string, dep driver.Dependency) driver.Item {
	id := strings.ToLower(strings.TrimSpace(p.Slug))
	if id == "" {
		id = strings.ToLower(strings.TrimSpace(p.Name))
	}
	latest := strings.TrimSpace(p.LatestVersion)
	if len(versions) == 0 && latest != "" {
		versions = []string{latest}
	}
	return driver.Item{
		ID:                id,
		Title:             driver.FirstNonEmpty(p.Title, p.Name, id),
		Summary:           strings.TrimSpace(p.Description),
		Description:       strings.TrimSpace(p.Description),
		Changelog:         changelog,
		Author:            strings.TrimSpace(p.Author),
		IconURL:           strings.TrimSpace(p.IconURL),
		SourceURL:         strings.TrimSpace(p.URL),
		FileURL:           strings.TrimSpace(p.DownloadURL),
		FileName:          extractPluginFileName(p.DownloadURL, p.Name, id),
		LatestVersion:     latest,
		AvailableVersions: versions,
		Downloads:         p.Downloads,
		UpdatedAt:         driver.FirstNonEmpty(p.LatestReleaseAtom, p.UpdatedAtAtom),
		Tags:              splitTags(p.TagsAll, gameSlug),
		Dependencies:      []driver.Dependency{dep},
	}
}

func frameworkDependency(gameSlug string) driver.Dependency {
	title := gameSlug
	if len(title) > 0 {
		title = strings.ToUpper(title[:1]) + title[1:]
	}
	return driver.Dependency{
		ID:    frameworkID(gameSlug),
		Title: "Oxide (" + title + ")",
	}
}

func fetchGameMeta(ctx context.Context, rt driver.Runtime, baseURL, gameSlug string) (*gameJSON, error) {
	u := fmt.Sprintf("%s/games/%s.json", baseURL, url.PathEscape(gameSlug))
	ck := driver.CacheHashKey("umod:game", u)
	var cached gameJSON
	if rt.GetCachedJSON(ctx, ck, &cached) && cached.Slug != "" {
		return &cached, nil
	}
	var game gameJSON
	if err := rt.DoGetJSON(ctx, u, &game); err != nil {
		return nil, err
	}
	if strings.TrimSpace(game.Slug) == "" {
		return nil, driver.ErrNotFound
	}
	rt.SetCachedJSON(ctx, ck, game)
	return &game, nil
}

func fetchPluginMeta(ctx context.Context, rt driver.Runtime, baseURL, id string) (*pluginJSON, error) {
	cleanID := stripKnownExt(strings.TrimSpace(id))
	if cleanID == "" {
		return nil, driver.ErrNotFound
	}
	u := fmt.Sprintf("%s/plugins/%s.json", baseURL, url.PathEscape(cleanID))
	ck := driver.CacheHashKey("umod:plugin", u)
	var cached pluginJSON
	if rt.GetCachedJSON(ctx, ck, &cached) && cached.Slug != "" {
		return &cached, nil
	}
	var p pluginJSON
	if err := rt.DoGetJSON(ctx, u, &p); err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Slug) == "" && strings.TrimSpace(p.Name) == "" {
		return nil, driver.ErrNotFound
	}
	rt.SetCachedJSON(ctx, ck, p)
	return &p, nil
}

func fetchGameVersions(ctx context.Context, rt driver.Runtime, baseURL, gameSlug string) ([]versionJSON, error) {
	u := fmt.Sprintf("%s/games/%s/versions.json", baseURL, url.PathEscape(gameSlug))
	return fetchVersionsList(ctx, rt, "umod:game_vers", u)
}

func fetchPluginVersions(ctx context.Context, rt driver.Runtime, baseURL, slug string) ([]versionJSON, error) {
	u := fmt.Sprintf("%s/plugins/%s/versions.json", baseURL, url.PathEscape(slug))
	return fetchVersionsList(ctx, rt, "umod:plugin_vers", u)
}

func fetchVersionsList(ctx context.Context, rt driver.Runtime, ns, u string) ([]versionJSON, error) {
	ck := driver.CacheHashKey(ns, u)
	var cached []versionJSON
	if rt.GetCachedJSON(ctx, ck, &cached) && len(cached) > 0 {
		return cached, nil
	}
	var resp versionsResponse
	if err := rt.DoGetJSON(ctx, u, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) > 0 {
		rt.SetCachedJSON(ctx, ck, resp.Data)
	}
	return resp.Data, nil
}

func collectVersionMeta(entries []versionJSON, latestVer string) ([]string, string) {
	seen := make(map[string]bool, len(entries)+1)
	var out []string
	if cleanLatest := strings.TrimSpace(latestVer); cleanLatest != "" {
		seen[cleanLatest] = true
		out = append(out, cleanLatest)
	}
	changelog := ""
	for _, entry := range entries {
		v := strings.TrimSpace(entry.Version)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
		if changelog == "" {
			changelog = formatHTMLChangelog(driver.FirstNonEmpty(entry.DescriptionMD, entry.Description))
		}
		if len(out) >= maxAvailableVersions {
			break
		}
	}
	return out, changelog
}

func formatHTMLChangelog(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	s := liTagRe.ReplaceAllString(trimmed, "\n- ")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	var lines []string
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			lines = append(lines, t)
		}
	}
	return strings.Join(lines, "\n")
}

func splitTags(raw, gameSlug string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		clean := strings.TrimSpace(part)
		if clean != "" && !strings.EqualFold(clean, gameSlug) {
			out = append(out, clean)
		}
	}
	return out
}

func extractPluginFileName(dlURL, className, slug string) string {
	noQuery := strings.Split(strings.TrimSpace(dlURL), "?")[0]
	base := path.Base(noQuery)
	if base != "" && base != "." && base != "/" && strings.HasSuffix(strings.ToLower(base), ".cs") {
		return base
	}
	stem := driver.FirstNonEmpty(className, slug, "Plugin")
	if !strings.HasSuffix(strings.ToLower(stem), ".cs") {
		return stem + ".cs"
	}
	return stem
}

func oxideArchiveFileName(gameSlug, ver string) string {
	cleanVer := driver.FirstNonEmpty(ver, "latest")
	return fmt.Sprintf("oxide-%s-%s-linux.zip", gameSlug, cleanVer)
}

func selectFrameworkLatestURL(game *gameJSON, baseURL, gameSlug string) string {
	if snap := strings.TrimSpace(game.SnapshotURL); snap != "" {
		return snap
	}
	if repo := strings.TrimSpace(game.Repository); repo != "" && game.LatestVersion != "" {
		return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, url.PathEscape(game.LatestVersion), oxideLinuxAssetName(game.SnapshotURL, game.Repository))
	}
	return driver.FirstNonEmpty(game.DownloadURL, fmt.Sprintf("%s/games/%s/download", baseURL, url.PathEscape(gameSlug)))
}

func oxideLinuxAssetName(snapshotURL, repo string) string {
	if snap := strings.TrimSpace(snapshotURL); snap != "" {
		if base := path.Base(strings.Split(snap, "?")[0]); strings.HasSuffix(strings.ToLower(base), ".zip") {
			return base
		}
	}
	if _, name, ok := strings.Cut(strings.TrimSpace(repo), "/"); ok && name != "" {
		return name + "-linux.zip"
	}
	return "Oxide.Rust-linux.zip"
}

func resolveBatch(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string) ([]driver.Item, error) {
	items := make([]*driver.Item, len(tokens))
	sem := make(chan struct{}, resolveConcurrency)
	var wg sync.WaitGroup
	for i, tok := range tokens {
		if strings.TrimSpace(tok) == "" {
			continue
		}
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if it, err := detail(ctx, rt, src, id); err == nil && it != nil {
				items[idx] = it
			}
		}(i, tok)
	}
	wg.Wait()
	out := make([]driver.Item, 0, len(items))
	for _, it := range items {
		if it != nil {
			out = append(out, *it)
		}
	}
	return out, nil
}

func resolveLatestBatch(ctx context.Context, rt driver.Runtime, baseURL, gameSlug string, pkgIDs []string) map[string]string {
	out := make(map[string]string, len(pkgIDs))
	rCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	sem := make(chan struct{}, resolveConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, id := range pkgIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		wg.Add(1)
		go func(pkgID string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-rCtx.Done():
				return
			}
			if ver := lookupLatestVersion(rCtx, rt, baseURL, gameSlug, pkgID); ver != "" {
				mu.Lock()
				out[pkgID] = ver
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	return out
}

func lookupLatestVersion(ctx context.Context, rt driver.Runtime, baseURL, gameSlug, pkgID string) string {
	if isFrameworkID(pkgID, gameSlug) {
		if g, err := fetchGameMeta(ctx, rt, baseURL, gameSlug); err == nil && g != nil {
			return strings.TrimSpace(g.LatestVersion)
		}
		return ""
	}
	if p, err := fetchPluginMeta(ctx, rt, baseURL, pkgID); err == nil && p != nil {
		return strings.TrimSpace(p.LatestVersion)
	}
	return ""
}

func resolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, pkgID, requestedVer string) (*driver.ResolvedRelease, error) {
	baseURL, gameSlug, err := parseUModEndpoint(src.Endpoint)
	if err != nil {
		return nil, err
	}
	if isFrameworkID(pkgID, gameSlug) {
		return resolveFrameworkRelease(ctx, rt, baseURL, gameSlug, requestedVer)
	}
	return resolvePluginRelease(ctx, rt, baseURL, gameSlug, pkgID, requestedVer)
}

func resolveFrameworkRelease(ctx context.Context, rt driver.Runtime, baseURL, gameSlug, requestedVer string) (*driver.ResolvedRelease, error) {
	item, err := detailFramework(ctx, rt, baseURL, gameSlug)
	if err != nil {
		return nil, err
	}
	game, err := fetchGameMeta(ctx, rt, baseURL, gameSlug)
	if err != nil {
		return nil, err
	}
	ver := resolveChosenFrameworkVersion(game.LatestVersion, requestedVer)
	if ver == "" {
		return nil, driver.ErrNotFound
	}
	primary, fallbacks := frameworkDownloadURLs(game, baseURL, gameSlug, ver)
	item.LatestVersion = ver
	return &driver.ResolvedRelease{
		Package: item.ID, Version: ver, Build: item.ID + "-" + ver,
		DownloadURL: primary, FallbackDownloadURLs: fallbacks,
		FileName: oxideArchiveFileName(gameSlug, ver), ArchiveFormat: driver.ArchiveFormatZip,
		PublishedAt: driver.ParseRFC3339Ptr(game.LatestReleaseAtom), Changelog: item.Changelog,
		UpstreamURL: item.SourceURL, Item: item,
	}, nil
}

func resolveChosenFrameworkVersion(latestVer, requestedVer string) string {
	reqVer := strings.TrimPrefix(strings.TrimSpace(requestedVer), "v")
	if driver.IsLatestAlias(reqVer) {
		return strings.TrimSpace(latestVer)
	}
	return reqVer
}

func frameworkDownloadURLs(game *gameJSON, baseURL, gameSlug, ver string) (string, []string) {
	developURL := fmt.Sprintf("%s/games/%s/download/develop", baseURL, url.PathEscape(gameSlug))
	umodVerURL := fmt.Sprintf("%s/games/%s/download/%s?tag=public", baseURL, url.PathEscape(gameSlug), url.PathEscape(ver))
	if strings.EqualFold(ver, strings.TrimSpace(game.LatestVersion)) {
		primary := selectFrameworkLatestURL(game, baseURL, gameSlug)
		return primary, []string{umodVerURL, developURL}
	}
	asset := oxideLinuxAssetName(game.SnapshotURL, game.Repository)
	if repo := strings.TrimSpace(game.Repository); repo != "" {
		ghURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, url.PathEscape(ver), asset)
		return ghURL, []string{umodVerURL}
	}
	return umodVerURL, []string{developURL}
}

func resolvePluginRelease(ctx context.Context, rt driver.Runtime, baseURL, gameSlug, pkgID, requestedVer string) (*driver.ResolvedRelease, error) {
	p, err := fetchPluginMeta(ctx, rt, baseURL, pkgID)
	if err != nil {
		return nil, err
	}
	slug := strings.ToLower(driver.FirstNonEmpty(p.Slug, pkgID))
	vers, _ := fetchPluginVersions(ctx, rt, baseURL, slug)
	ver, dlURL, fallbacks, pubAt, clog := selectPluginVersion(p, vers, requestedVer)
	if dlURL == "" || ver == "" {
		return nil, driver.ErrNotFound
	}
	dep := frameworkDependency(gameSlug)
	avail, _ := collectVersionMeta(vers, p.LatestVersion)
	item := buildPluginItem(p, gameSlug, avail, clog, dep)
	item.LatestVersion = ver
	fname := extractPluginFileName(dlURL, p.Name, slug)
	return &driver.ResolvedRelease{
		Package: slug, Version: ver, Build: slug + "-" + ver,
		DownloadURL: dlURL, FallbackDownloadURLs: fallbacks,
		FileName: fname, ArchiveFormat: driver.ArchiveFormatRaw,
		Dependencies: []driver.Dependency{dep}, PublishedAt: pubAt,
		Changelog: clog, UpstreamURL: item.SourceURL, Item: &item,
	}, nil
}

func selectPluginVersion(p *pluginJSON, vers []versionJSON, requestedVer string) (string, string, []string, *time.Time, string) {
	reqVer := strings.TrimPrefix(strings.TrimSpace(requestedVer), "v")
	latestVer := strings.TrimSpace(p.LatestVersion)
	isLatest := driver.IsLatestAlias(reqVer) || strings.EqualFold(reqVer, latestVer)
	targetVer := latestVer
	if !isLatest {
		targetVer = reqVer
	}
	for _, entry := range vers {
		ev := strings.TrimPrefix(strings.TrimSpace(entry.Version), "v")
		if strings.EqualFold(ev, strings.TrimPrefix(targetVer, "v")) {
			primary := driver.FirstNonEmpty(entry.DownloadURL, buildVersionedCSURL(p.DownloadURL, targetVer))
			var fallbacks []string
			if isLatest && strings.TrimSpace(p.DownloadURL) != "" && p.DownloadURL != primary {
				fallbacks = []string{p.DownloadURL}
			}
			return ev, primary, fallbacks, driver.ParseRFC3339Ptr(entry.CreatedAtAtom), formatHTMLChangelog(driver.FirstNonEmpty(entry.DescriptionMD, entry.Description))
		}
	}
	primary := strings.TrimSpace(p.DownloadURL)
	if !isLatest && primary != "" {
		primary = buildVersionedCSURL(primary, targetVer)
	}
	return targetVer, primary, nil, driver.ParseRFC3339Ptr(p.LatestReleaseAtom), ""
}

func buildVersionedCSURL(baseDL, ver string) string {
	clean := strings.TrimSpace(baseDL)
	if clean == "" || ver == "" {
		return clean
	}
	sep := "?"
	if strings.Contains(clean, "?") {
		sep = "&"
	}
	return clean + sep + "version=" + url.QueryEscape(ver)
}
