package sevendaysmods

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
	ghURLRe  = regexp.MustCompile(`(?i)github\.com/([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)`)
	ghSlugRe = regexp.MustCompile(`^([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)$`)
)

func init() {
	driver.Register(driver.ProviderSevenDaysMods, func() driver.Driver { return &sevenDaysModsDriver{} })
}

type sevenDaysModsDriver struct {
	driver.BaseDriver
}

func (d *sevenDaysModsDriver) Name() string { return driver.ProviderSevenDaysMods }

func (d *sevenDaysModsDriver) Capabilities(src driver.Source) driver.Capabilities {
	return driver.Capabilities{Catalog: src.Format != driver.FormatServerPackage}
}

func (d *sevenDaysModsDriver) ParseTokens(raw string) []string {
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

func (d *sevenDaysModsDriver) Search(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	q := strings.TrimSpace(params.Query)
	ck := driver.CacheHashKey("7dtd:search", fmt.Sprintf("%s|%s|%d|%d", src.Endpoint, q, params.Page, params.Limit))
	var cached driver.Result
	if rt.GetCachedJSON(ctx, ck, &cached) {
		return &cached, nil
	}
	items := buildSearchResults(ctx, rt, src, q)
	out := driver.Result{Configured: true, Total: len(items), Items: paginateItems(items, params.Page, params.Limit)}
	rt.SetCachedJSON(ctx, ck, out)
	return &out, nil
}

func buildSearchResults(ctx context.Context, rt driver.Runtime, src driver.Source, query string) []driver.Item {
	var curated []curatedMod
	for _, c := range curatedCatalog {
		if matchesQuery(c, query) {
			curated = append(curated, c)
		}
	}
	resolved := resolveCuratedBatch(ctx, rt, src, curated)
	seen := make(map[string]bool, len(curated))
	items := make([]driver.Item, 0, len(curated))
	for i, c := range curated {
		if resolved[i] != nil {
			items = append(items, *resolved[i])
		} else {
			items = append(items, curatedBaseItem(c))
		}
		seen[strings.ToLower(c.ID)] = true
	}
	if query != "" {
		for _, gh := range searchGitHubMods(ctx, rt, src, query) {
			if !seen[strings.ToLower(gh.ID)] {
				seen[strings.ToLower(gh.ID)] = true
				items = append(items, gh)
			}
		}
	}
	return items
}

func resolveCuratedBatch(ctx context.Context, rt driver.Runtime, src driver.Source, list []curatedMod) []*driver.Item {
	out := make([]*driver.Item, len(list))
	sem := make(chan struct{}, resolveConcurrency)
	var wg sync.WaitGroup
	for i, c := range list {
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if it, err := detailMod(ctx, rt, src, id); err == nil && it != nil {
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

func (d *sevenDaysModsDriver) Detail(ctx context.Context, rt driver.Runtime, src driver.Source, id, _ string) (*driver.Item, error) {
	return detailMod(ctx, rt, src, id)
}

func (d *sevenDaysModsDriver) Resolve(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string, _ string) ([]driver.Item, error) {
	return resolveBatch(ctx, rt, src, tokens), nil
}

func (d *sevenDaysModsDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	if src.Format == driver.FormatServerPackage {
		return nil, driver.ErrUnsupportedSource
	}
	return resolveRelease(ctx, rt, src, req)
}

func (d *sevenDaysModsDriver) ResolveLatestVersions(ctx context.Context, rt driver.Runtime, src driver.Source, pkgIDs []string) (map[string]string, error) {
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
			if it, err := detailMod(ctx, rt, src, id); err == nil && it != nil {
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

func detailMod(ctx context.Context, rt driver.Runtime, src driver.Source, id string) (*driver.Item, error) {
	owner, repo, ok := parseGitHubSlug(id)
	if !ok {
		return nil, driver.ErrNotFound
	}
	rels, err := fetchRepoReleases(ctx, rt, src, owner, repo)
	if err != nil {
		return nil, err
	}
	item := buildGitHubItem(id, owner, repo, rels, fetchRepoInfo(ctx, rt, src, owner, repo))
	if len(item.AvailableVersions) == 0 {
		return nil, driver.ErrNotFound
	}
	return &item, nil
}

func buildGitHubItem(rawID, owner, repo string, rels []ghRelease, info *ghRepoInfo) driver.Item {
	canonID := toCanonicalPackageID(owner, repo)
	if c, found := findCurated(rawID); found {
		canonID = c.ID
	}
	item := defaultModItem(canonID, owner, repo)
	if info != nil {
		applyRepoInfo(&item, info)
	}
	var latestAsset ghAsset
	var latestRel ghRelease
	for _, r := range rels {
		asset, ok := selectModAsset(r.Assets, repo)
		if !ok {
			continue
		}
		item.AvailableVersions = append(item.AvailableVersions, strings.TrimSpace(r.TagName))
		if item.LatestVersion == "" {
			item.LatestVersion, latestAsset, latestRel = strings.TrimSpace(r.TagName), asset, r
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

func defaultModItem(canonID, owner, repo string) driver.Item {
	if c, found := findCurated(canonID); found {
		return curatedBaseItem(c)
	}
	ghURL := "https://github.com/" + owner + "/" + repo
	return driver.Item{ID: canonID, Title: repo, Author: owner, SourceURL: ghURL, UpstreamURL: ghURL}
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
	owner, repo, ok := parseGitHubSlug(req.Package)
	if !ok {
		return nil, driver.ErrNotFound
	}
	rels, err := fetchRepoReleases(ctx, rt, src, owner, repo)
	if err != nil {
		return nil, err
	}
	rel, asset, matched := pickMatchingRelease(rels, req.Version, repo)
	if !matched {
		return nil, driver.ErrNotFound
	}
	canonID := toCanonicalPackageID(owner, repo)
	item := buildGitHubItem(canonID, owner, repo, rels, nil)
	return &driver.ResolvedRelease{
		Package:       canonID,
		Version:       rel.TagName,
		Build:         rel.TagName + "/" + asset.Name,
		DownloadURL:   asset.BrowserDownloadURL,
		FileName:      asset.Name,
		ArchiveFormat: driver.InferArchiveFormat(asset.Name),
		SizeBytes:     asset.Size,
		PublishedAt:   driver.ParseRFC3339Ptr(rel.PublishedAt),
		Changelog:     strings.TrimSpace(rel.Body),
		UpstreamURL:   driver.FirstNonEmpty(rel.HTMLURL, item.UpstreamURL),
		Item:          &item,
	}, nil
}

func pickMatchingRelease(rels []ghRelease, reqVer, repo string) (ghRelease, ghAsset, bool) {
	wantLatest := driver.IsLatestAlias(reqVer)
	trimmed := strings.TrimSpace(reqVer)
	for _, r := range rels {
		asset, ok := selectModAsset(r.Assets, repo)
		if ok && (wantLatest || strings.EqualFold(strings.TrimSpace(r.TagName), trimmed)) {
			return r, asset, true
		}
	}
	return ghRelease{}, ghAsset{}, false
}
