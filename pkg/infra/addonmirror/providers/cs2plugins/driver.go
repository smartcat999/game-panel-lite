package cs2plugins

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const resolveConcurrency = 4

var (
	ghURLRe   = regexp.MustCompile(`(?i)github\.com/([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)`)
	ghSlugRe  = regexp.MustCompile(`^([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)$`)
	mmsNameRe = regexp.MustCompile(`^mmsource-(.+)-linux\.tar\.gz$`)
)

func init() {
	driver.Register(driver.ProviderCS2Plugins, func() driver.Driver { return &cs2PluginsDriver{} })
}

type cs2PluginsDriver struct {
	driver.BaseDriver
}

func (d *cs2PluginsDriver) Name() string { return driver.ProviderCS2Plugins }

func (d *cs2PluginsDriver) Capabilities(src driver.Source) driver.Capabilities {
	return driver.Capabilities{Catalog: src.Format != driver.FormatServerPackage}
}

func (d *cs2PluginsDriver) ParseTokens(raw string) []string {
	var out []string
	add := func(tok string) {
		if tok != "" && !driver.ContainsString(out, tok) {
			out = append(out, tok)
		}
	}
	for _, m := range ghURLRe.FindAllStringSubmatch(raw, -1) {
		add(toCanonicalPackageID(m[1], strings.TrimSuffix(m[2], ".git")))
	}
	for _, tok := range driver.SplitPlainTokens(raw, normalizeToken) {
		add(tok)
	}
	return out
}

func normalizeToken(part string) (string, bool) {
	trimmed := strings.TrimSpace(part)
	if trimmed == "" {
		return "", false
	}
	if c, ok := findCurated(trimmed); ok {
		return c.ID, true
	}
	if m := ghSlugRe.FindStringSubmatch(trimmed); len(m) == 3 {
		return toCanonicalPackageID(m[1], strings.TrimSuffix(m[2], ".git")), true
	}
	return trimmed, true
}

func (d *cs2PluginsDriver) Search(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	q := strings.TrimSpace(params.Query)
	ck := driver.CacheHashKey("cs2p:search", fmt.Sprintf("%s|%s|%d|%d", src.Endpoint, q, params.Page, params.Limit))
	var cached driver.Result
	if rt.GetCachedJSON(ctx, ck, &cached) {
		return &cached, nil
	}
	items := buildSearchResults(ctx, rt, src, q)
	total := len(items)
	paged := paginateItems(items, params.Page, params.Limit)
	out := driver.Result{Configured: true, Total: total, Items: paged}
	rt.SetCachedJSON(ctx, ck, out)
	return &out, nil
}

func buildSearchResults(ctx context.Context, rt driver.Runtime, src driver.Source, query string) []driver.Item {
	var curated []curatedPlugin
	for _, c := range curatedCatalog {
		if matchesQuery(c, query) {
			curated = append(curated, c)
		}
	}
	resolved := resolveCuratedBatch(ctx, rt, src, curated)
	seen := make(map[string]bool, len(resolved))
	baseItems := make([]driver.Item, 0, len(resolved))
	for i, c := range curated {
		if resolved[i] != nil {
			baseItems = append(baseItems, *resolved[i])
		} else {
			baseItems = append(baseItems, curatedBaseItem(c))
		}
		seen[strings.ToLower(c.ID)] = true
	}
	if query != "" {
		for _, gh := range searchGitHubPlugins(ctx, rt, src, query) {
			if !seen[strings.ToLower(gh.ID)] {
				seen[strings.ToLower(gh.ID)] = true
				baseItems = append(baseItems, gh)
			}
		}
	}
	return baseItems
}

func resolveCuratedBatch(ctx context.Context, rt driver.Runtime, src driver.Source, list []curatedPlugin) []*driver.Item {
	out := make([]*driver.Item, len(list))
	sem := make(chan struct{}, resolveConcurrency)
	var wg sync.WaitGroup
	for i, c := range list {
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if it, err := detailPlugin(ctx, rt, src, id); err == nil && it != nil {
				out[idx] = it
			}
		}(i, c.ID)
	}
	wg.Wait()
	return out
}

func paginateItems(items []driver.Item, page, limit int) []driver.Item {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	start := (page - 1) * limit
	if start >= len(items) {
		return nil
	}
	return items[start:min(start+limit, len(items))]
}

func (d *cs2PluginsDriver) Detail(ctx context.Context, rt driver.Runtime, src driver.Source, id, _ string) (*driver.Item, error) {
	return detailPlugin(ctx, rt, src, id)
}

func (d *cs2PluginsDriver) Resolve(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string, _ string) ([]driver.Item, error) {
	return resolveBatch(ctx, rt, src, tokens), nil
}

func (d *cs2PluginsDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	if src.Format == driver.FormatServerPackage {
		return nil, driver.ErrUnsupportedSource
	}
	return resolveRelease(ctx, rt, src, req)
}

func (d *cs2PluginsDriver) ResolveLatestVersions(ctx context.Context, rt driver.Runtime, src driver.Source, pkgIDs []string) (map[string]string, error) {
	items := resolveBatch(ctx, rt, src, pkgIDs)
	out := make(map[string]string, len(items))
	for _, it := range items {
		if it.ID != "" && it.LatestVersion != "" {
			out[it.ID] = it.LatestVersion
		}
	}
	return out, nil
}

func resolveBatch(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string) []driver.Item {
	results := make([]*driver.Item, len(tokens))
	sem := make(chan struct{}, resolveConcurrency)
	var wg sync.WaitGroup
	for i, tok := range tokens {
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if it, err := detailPlugin(ctx, rt, src, id); err == nil && it != nil {
				results[idx] = it
			}
		}(i, tok)
	}
	wg.Wait()
	var out []driver.Item
	for _, it := range results {
		if it != nil {
			out = append(out, *it)
		}
	}
	return out
}

func metamodBase(src driver.Source) string {
	ep := strings.TrimRight(strings.TrimSpace(src.Endpoint), "/")
	if ep != "" && !strings.Contains(strings.ToLower(ep), "github.com") {
		return ep
	}
	return driver.DefaultMetamodBaseURL
}

func detailMetamod(ctx context.Context, rt driver.Runtime, src driver.Source) (*driver.Item, string, string, error) {
	base := metamodBase(src)
	ck := driver.CacheHashKey("cs2p:mms", base)
	type cachedMMS struct {
		Version string `json:"version"`
		URL     string `json:"url"`
	}
	var cached cachedMMS
	if !rt.GetCachedJSON(ctx, ck, &cached) || cached.Version == "" {
		raw, err := rt.DoGetBytes(ctx, base+"/mmsource-latest-linux")
		if err != nil {
			return nil, "", "", err
		}
		fname := strings.TrimSpace(string(raw))
		if fname == "" {
			return nil, "", "", driver.ErrNotFound
		}
		cached.Version = parseMetamodVersion(fname)
		cached.URL = base + "/" + fname
		rt.SetCachedJSON(ctx, ck, cached)
	}
	c, _ := findCurated(PackageMetamod)
	item := curatedBaseItem(c)
	item.LatestVersion = cached.Version
	item.AvailableVersions = []string{cached.Version}
	item.FileName = fmt.Sprintf("mmsource-%s-linux.tar.gz", cached.Version)
	item.FileURL = cached.URL
	return &item, cached.Version, cached.URL, nil
}

func parseMetamodVersion(fname string) string {
	if m := mmsNameRe.FindStringSubmatch(strings.TrimSpace(fname)); len(m) == 2 {
		return m[1]
	}
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(fname), "mmsource-"), ".tar.gz")
}

func detailPlugin(ctx context.Context, rt driver.Runtime, src driver.Source, id string) (*driver.Item, error) {
	if strings.EqualFold(strings.TrimSpace(id), PackageMetamod) {
		item, _, _, err := detailMetamod(ctx, rt, src)
		return item, err
	}
	owner, repo, isCSS, ok := parseGitHubSlug(id)
	if !ok {
		return nil, driver.ErrNotFound
	}
	rels, err := fetchRepoReleases(ctx, rt, src, owner, repo)
	if err != nil {
		return nil, err
	}
	item := buildGitHubItem(id, owner, repo, isCSS, rels, fetchRepoInfo(ctx, rt, src, owner, repo))
	if len(item.AvailableVersions) == 0 {
		return nil, driver.ErrNotFound
	}
	return &item, nil
}

func buildGitHubItem(rawID, owner, repo string, isCSS bool, rels []ghRelease, info *ghRepoInfo) driver.Item {
	canonID := toCanonicalPackageID(owner, repo)
	if c, found := findCurated(rawID); found {
		canonID = c.ID
	}
	item := defaultPluginItem(canonID, owner, repo)
	if info != nil {
		applyRepoInfo(&item, info)
	}
	var latestAsset ghAsset
	var latestRel ghRelease
	for _, r := range rels {
		asset, ok := selectPluginAsset(r.Assets, isCSS)
		if !ok {
			continue
		}
		ver := strings.TrimSpace(r.TagName)
		item.AvailableVersions = append(item.AvailableVersions, ver)
		if item.LatestVersion == "" {
			item.LatestVersion, latestAsset, latestRel = ver, asset, r
		}
	}
	item.SizeBytes = latestAsset.Size
	item.FileURL = latestAsset.BrowserDownloadURL
	item.FileName = latestAsset.Name
	item.UpdatedAt = strings.TrimSpace(latestRel.PublishedAt)
	item.Changelog = strings.TrimSpace(latestRel.Body)
	if strings.TrimSpace(latestRel.HTMLURL) != "" {
		item.UpstreamURL = latestRel.HTMLURL
	}
	return item
}

func defaultPluginItem(canonID, owner, repo string) driver.Item {
	if c, found := findCurated(canonID); found {
		return curatedBaseItem(c)
	}
	ghURL := "https://github.com/" + owner + "/" + repo
	return driver.Item{
		ID:           canonID,
		Title:        repo,
		Author:       owner,
		SourceURL:    ghURL,
		UpstreamURL:  ghURL,
		Dependencies: []driver.Dependency{{ID: PackageCounterStrikeSharp, Title: "CounterStrikeSharp (.NET 运行时插件框架)"}},
	}
}

func applyRepoInfo(item *driver.Item, info *ghRepoInfo) {
	if item.Summary == "" && strings.TrimSpace(info.Description) != "" {
		item.Summary = strings.TrimSpace(info.Description)
	}
	if strings.TrimSpace(info.Description) != "" {
		item.Description = strings.TrimSpace(info.Description)
	}
	if strings.TrimSpace(info.Owner.AvatarURL) != "" {
		item.IconURL = strings.TrimSpace(info.Owner.AvatarURL)
	}
	if info.Stars > 0 {
		item.Downloads = info.Stars
	}
	if len(item.Tags) == 0 && len(info.Topics) > 0 {
		item.Tags = append([]string(nil), info.Topics...)
	}
}

func resolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	if strings.EqualFold(strings.TrimSpace(req.Package), PackageMetamod) {
		return resolveMetamodRelease(ctx, rt, src, req)
	}
	owner, repo, isCSS, ok := parseGitHubSlug(req.Package)
	if !ok {
		return nil, driver.ErrNotFound
	}
	rels, err := fetchRepoReleases(ctx, rt, src, owner, repo)
	if err != nil {
		return nil, err
	}
	rel, asset, matched := pickMatchingRelease(rels, req.Version, isCSS)
	if !matched {
		return nil, driver.ErrNotFound
	}
	canonID := toCanonicalPackageID(owner, repo)
	item := buildGitHubItem(canonID, owner, repo, isCSS, rels, nil)
	return &driver.ResolvedRelease{
		Package:       canonID,
		Version:       rel.TagName,
		Build:         rel.TagName + "/" + asset.Name,
		DownloadURL:   asset.BrowserDownloadURL,
		FileName:      asset.Name,
		ArchiveFormat: driver.InferArchiveFormat(asset.Name),
		SizeBytes:     asset.Size,
		Dependencies:  item.Dependencies,
		PublishedAt:   driver.ParseRFC3339Ptr(rel.PublishedAt),
		Changelog:     strings.TrimSpace(rel.Body),
		UpstreamURL:   driver.FirstNonEmpty(rel.HTMLURL, item.UpstreamURL),
		Item:          &item,
	}, nil
}

func resolveMetamodRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	item, latestVer, dlURL, err := detailMetamod(ctx, rt, src)
	if err != nil {
		return nil, err
	}
	ver := latestVer
	fname := fmt.Sprintf("mmsource-%s-linux.tar.gz", ver)
	if !driver.IsLatestAlias(req.Version) && strings.TrimSpace(req.Version) != latestVer {
		if req.StrictVersion {
			return nil, driver.ErrNotFound
		}
		ver = strings.TrimSpace(req.Version)
		fname = fmt.Sprintf("mmsource-%s-linux.tar.gz", ver)
		dlURL = metamodBase(src) + "/" + fname
	}
	return &driver.ResolvedRelease{
		Package:       PackageMetamod,
		Version:       ver,
		Build:         ver,
		DownloadURL:   dlURL,
		FileName:      fname,
		ArchiveFormat: driver.ArchiveFormatTarGz,
		UpstreamURL:   "https://www.sourcemm.net/downloads.php/?branch=master",
		Item:          item,
	}, nil
}

func pickMatchingRelease(rels []ghRelease, reqVer string, isCSS bool) (ghRelease, ghAsset, bool) {
	wantLatest := driver.IsLatestAlias(reqVer)
	trimmed := strings.TrimSpace(reqVer)
	for _, r := range rels {
		asset, ok := selectPluginAsset(r.Assets, isCSS)
		if !ok {
			continue
		}
		if wantLatest || strings.EqualFold(strings.TrimSpace(r.TagName), trimmed) {
			return r, asset, true
		}
	}
	return ghRelease{}, ghAsset{}, false
}
