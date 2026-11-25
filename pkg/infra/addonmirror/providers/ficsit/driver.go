package ficsit

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const (
	targetLinuxServer     = "LinuxServer"
	searchVersionLimit    = 5
	detailVersionLimit    = 30
	resolveBatchLimit     = 20
	resolveBatchWorkerMax = 4

	searchModsGraphQL = `query SearchMods($limit: Int!, $offset: Int!, $orderBy: ModFields!, $order: Order!, $search: String) {
  getMods(filter: {limit: $limit, offset: $offset, order_by: $orderBy, order: $order, search: $search}) {
    count
    mods {
      id
      mod_reference
      name
      short_description
      logo
      downloads
      last_version_date
      authors { user { username } }
      tags { name }
      versions(filter: {limit: 5}) {
        id
        version
        created_at
        targets { targetName link hash size }
        dependencies { mod_id condition optional }
      }
    }
  }
}`

	detailModGraphQL = `query ModDetail($ref: String!, $vlimit: Int!) {
  getModByIdOrReference(modIdOrReference: $ref) {
    id
    mod_reference
    name
    short_description
    full_description
    logo
    downloads
    last_version_date
    source_url
    authors { user { username } }
    tags { name }
    versions(filter: {limit: $vlimit}) {
      id
      version
      changelog
      created_at
      targets { targetName link hash size }
      dependencies { mod_id condition optional }
    }
  }
}`
)

var ficsitURLRe = regexp.MustCompile(`(?i)ficsit\.app/mod/([^\s,;/?#"<>]+)`)

func init() {
	driver.Register(driver.ProviderFicsit, func() driver.Driver { return &ficsitDriver{} })
}

type ficsitDriver struct {
	driver.BaseDriver
}

func (d *ficsitDriver) Name() string { return driver.ProviderFicsit }

func (d *ficsitDriver) Capabilities(src driver.Source) driver.Capabilities {
	isCatalog := src.Format != driver.FormatServerPackage
	return driver.Capabilities{Catalog: isCatalog, LiveDetailOverlay: isCatalog}
}

func (d *ficsitDriver) ParseTokens(raw string) []string {
	var out []string
	add := func(tok string) {
		if tok != "" && !driver.ContainsString(out, tok) {
			out = append(out, tok)
		}
	}
	for _, m := range ficsitURLRe.FindAllStringSubmatch(raw, -1) {
		add(strings.Trim(m[1], "/"))
	}
	for _, tok := range driver.SplitPlainTokens(raw, func(part string) (string, bool) {
		cleaned := strings.TrimSpace(part)
		return cleaned, cleaned != ""
	}) {
		add(tok)
	}
	return out
}

func (d *ficsitDriver) Search(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	return searchMods(ctx, rt, src, params)
}

func (d *ficsitDriver) Detail(ctx context.Context, rt driver.Runtime, src driver.Source, id, _ string) (*driver.Item, error) {
	item, _, err := fetchDetailedMod(ctx, rt, src, id)
	return item, err
}

func (d *ficsitDriver) Resolve(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string, _ string) ([]driver.Item, error) {
	if len(tokens) > resolveBatchLimit {
		tokens = tokens[:resolveBatchLimit]
	}
	return resolveBatch(ctx, rt, src, tokens), nil
}

func (d *ficsitDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	if src.Format == driver.FormatServerPackage {
		return nil, driver.ErrUnsupportedSource
	}
	return resolveRelease(ctx, rt, src, req)
}

func (d *ficsitDriver) ResolveLatestVersions(ctx context.Context, rt driver.Runtime, src driver.Source, pkgIDs []string) (map[string]string, error) {
	items := resolveBatch(ctx, rt, src, pkgIDs)
	out := make(map[string]string, len(items))
	for _, it := range items {
		if it.ID != "" && it.LatestVersion != "" {
			out[it.ID] = it.LatestVersion
		}
	}
	return out, nil
}

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type gqlError struct {
	Message string `json:"message"`
}

type ficsitMod struct {
	ID               string          `json:"id"`
	ModReference     string          `json:"mod_reference"`
	Name             string          `json:"name"`
	ShortDescription string          `json:"short_description"`
	FullDescription  string          `json:"full_description"`
	Logo             string          `json:"logo"`
	Downloads        int64           `json:"downloads"`
	LastVersionDate  string          `json:"last_version_date"`
	SourceURL        string          `json:"source_url"`
	Authors          []ficsitAuthor  `json:"authors"`
	Tags             []ficsitTag     `json:"tags"`
	Versions         []ficsitVersion `json:"versions"`
}

type ficsitAuthor struct {
	User struct {
		Username string `json:"username"`
	} `json:"user"`
}

type ficsitTag struct {
	Name string `json:"name"`
}

type ficsitVersion struct {
	ID           string             `json:"id"`
	Version      string             `json:"version"`
	Changelog    string             `json:"changelog"`
	CreatedAt    string             `json:"created_at"`
	Targets      []ficsitTarget     `json:"targets"`
	Dependencies []ficsitDependency `json:"dependencies"`
}

type ficsitTarget struct {
	TargetName string `json:"targetName"`
	Link       string `json:"link"`
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
}

type ficsitDependency struct {
	ModID     string `json:"mod_id"`
	Condition string `json:"condition"`
	Optional  bool   `json:"optional"`
}

func resolveGraphQLEndpoint(raw string) (queryURL, origin string) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return driver.DefaultFicsitBaseURL + "/v2/query", driver.DefaultFicsitBaseURL
	}
	queryURL = trimmed
	if !strings.HasSuffix(strings.ToLower(trimmed), "/query") {
		queryURL = trimmed + "/v2/query"
	}
	origin = driver.DefaultFicsitBaseURL
	if u, err := url.Parse(queryURL); err == nil && u.Scheme != "" && u.Host != "" {
		origin = u.Scheme + "://" + u.Host
	}
	return queryURL, origin
}

func searchMods(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	queryURL, origin := resolveGraphQLEndpoint(src.Endpoint)
	offset := max(params.Page-1, 0) * params.Limit
	orderBy, order := mapSortField(params.Sort, params.Query)
	ck := driver.CacheHashKey("ficsit:search", fmt.Sprintf("%s|%s|%s|%s|%d|%d", queryURL, params.Query, orderBy, order, offset, params.Limit))
	var cached driver.Result
	if rt.GetCachedJSON(ctx, ck, &cached) {
		return &cached, nil
	}
	vars := map[string]any{"limit": params.Limit, "offset": offset, "orderBy": orderBy, "order": order}
	if q := strings.TrimSpace(params.Query); q != "" {
		vars["search"] = q
	}
	items, total, err := executeSearchQuery(ctx, rt, queryURL, origin, vars)
	if err != nil {
		return nil, err
	}
	out := driver.Result{Configured: true, Total: total, Items: items}
	rt.SetCachedJSON(ctx, ck, out)
	return &out, nil
}

func executeSearchQuery(ctx context.Context, rt driver.Runtime, queryURL, origin string, vars map[string]any) ([]driver.Item, int, error) {
	var env struct {
		Errors []gqlError `json:"errors"`
		Data   *struct {
			GetMods struct {
				Count int         `json:"count"`
				Mods  []ficsitMod `json:"mods"`
			} `json:"getMods"`
		} `json:"data"`
	}
	if err := rt.DoPostJSON(ctx, queryURL, gqlRequest{Query: searchModsGraphQL, Variables: vars}, &env); err != nil {
		return nil, 0, err
	}
	if env.Data == nil {
		return nil, 0, firstGQLErr(env.Errors, "ficsit getMods returned empty data")
	}
	items := make([]driver.Item, 0, len(env.Data.GetMods.Mods))
	for _, m := range env.Data.GetMods.Mods {
		if it, ok := mapModToItem(origin, m, false); ok {
			items = append(items, it)
		}
	}
	return items, env.Data.GetMods.Count, nil
}

func fetchDetailedMod(ctx context.Context, rt driver.Runtime, src driver.Source, idOrRef string) (*driver.Item, *ficsitMod, error) {
	queryURL, origin := resolveGraphQLEndpoint(src.Endpoint)
	trimmed := strings.TrimSpace(idOrRef)
	ck := driver.CacheHashKey("ficsit:detail", queryURL+"|"+strings.ToLower(trimmed))
	type cachedDetail struct {
		Item driver.Item `json:"item"`
		Mod  ficsitMod   `json:"mod"`
	}
	var cached cachedDetail
	if rt.GetCachedJSON(ctx, ck, &cached) && cached.Item.ID != "" {
		return &cached.Item, &cached.Mod, nil
	}
	var env struct {
		Errors []gqlError `json:"errors"`
		Data   *struct {
			Mod *ficsitMod `json:"getModByIdOrReference"`
		} `json:"data"`
	}
	err := rt.DoPostJSON(ctx, queryURL, gqlRequest{
		Query:     detailModGraphQL,
		Variables: map[string]any{"ref": trimmed, "vlimit": detailVersionLimit},
	}, &env)
	if err != nil {
		return nil, nil, err
	}
	if env.Data == nil {
		return nil, nil, firstGQLErr(env.Errors, "ficsit detail query returned empty data")
	}
	if env.Data.Mod == nil {
		return nil, nil, driver.ErrNotFound
	}
	item, ok := mapModToItem(origin, *env.Data.Mod, true)
	if !ok {
		return nil, nil, driver.ErrNotFound
	}
	rt.SetCachedJSON(ctx, ck, cachedDetail{Item: item, Mod: *env.Data.Mod})
	return &item, env.Data.Mod, nil
}

func resolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	_, origin := resolveGraphQLEndpoint(src.Endpoint)
	item, mod, err := fetchDetailedMod(ctx, rt, src, req.Package)
	if err != nil {
		return nil, err
	}
	ver, target, ok := pickLinuxServerVersion(mod.Versions, req.Version)
	if !ok {
		if req.StrictVersion || driver.IsLatestAlias(req.Version) {
			return nil, driver.ErrNotFound
		}
		ver, target, ok = pickLinuxServerVersion(mod.Versions, "")
		if !ok {
			return nil, driver.ErrNotFound
		}
	}
	dlURL := absFicsitURL(origin, target.Link)
	pkgRef := driver.FirstNonEmpty(mod.ModReference, item.ID, req.Package)
	return &driver.ResolvedRelease{
		Package:       pkgRef,
		Version:       ver.Version,
		Build:         driver.FirstNonEmpty(target.Hash, ver.ID),
		DownloadURL:   dlURL,
		FileName:      fmt.Sprintf("%s-LinuxServer-%s.smod", pkgRef, ver.Version),
		ArchiveFormat: driver.ArchiveFormatZip,
		SizeBytes:     target.Size,
		Dependencies:  mapDependencies(ver.Dependencies),
		PublishedAt:   driver.ParseRFC3339Ptr(ver.CreatedAt),
		Changelog:     strings.TrimSpace(ver.Changelog),
		UpstreamURL:   "https://ficsit.app/mod/" + pkgRef,
		Item:          item,
	}, nil
}

func resolveBatch(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string) []driver.Item {
	results := make([]*driver.Item, len(tokens))
	sem := make(chan struct{}, resolveBatchWorkerMax)
	var wg sync.WaitGroup
	for i, tok := range tokens {
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if it, _, err := fetchDetailedMod(ctx, rt, src, id); err == nil && it != nil {
				results[idx] = it
			}
		}(i, tok)
	}
	wg.Wait()
	out := make([]driver.Item, 0, len(tokens))
	for _, it := range results {
		if it != nil {
			out = append(out, *it)
		}
	}
	return out
}

func mapModToItem(origin string, m ficsitMod, includeDesc bool) (driver.Item, bool) {
	pkgRef := driver.FirstNonEmpty(m.ModReference, m.ID)
	if pkgRef == "" {
		return driver.Item{}, false
	}
	avail, latestVer, latestTarget, latestFound := extractLinuxServerVersions(m.Versions)
	if !latestFound && !includeDesc {
		return driver.Item{}, false
	}
	var authorNames []string
	for _, a := range m.Authors {
		if name := strings.TrimSpace(a.User.Username); name != "" {
			authorNames = append(authorNames, name)
		}
	}
	var tagNames []string
	for _, t := range m.Tags {
		if name := strings.TrimSpace(t.Name); name != "" {
			tagNames = append(tagNames, name)
		}
	}
	item := driver.Item{
		ID: pkgRef, Title: driver.FirstNonEmpty(m.Name, pkgRef),
		Summary: strings.TrimSpace(m.ShortDescription), Author: strings.Join(authorNames, ", "),
		IconURL: absFicsitURL(origin, m.Logo), SourceURL: "https://ficsit.app/mod/" + pkgRef,
		UpstreamURL:   driver.FirstNonEmpty(m.SourceURL, "https://ficsit.app/mod/"+pkgRef),
		LatestVersion: latestVer.Version, AvailableVersions: avail,
		Downloads: m.Downloads, SizeBytes: latestTarget.Size,
		UpdatedAt: driver.FirstNonEmpty(latestVer.CreatedAt, m.LastVersionDate),
		Tags:      tagNames, Dependencies: mapDependencies(latestVer.Dependencies),
	}
	if includeDesc {
		item.Description = strings.TrimSpace(m.FullDescription)
		item.Changelog = strings.TrimSpace(latestVer.Changelog)
	}
	return item, true
}

func extractLinuxServerVersions(vers []ficsitVersion) (avail []string, latestVer ficsitVersion, latestTarget ficsitTarget, found bool) {
	for _, v := range vers {
		t, ok := findLinuxServerTarget(v.Targets)
		if !ok || strings.TrimSpace(v.Version) == "" {
			continue
		}
		avail = append(avail, strings.TrimSpace(v.Version))
		if !found {
			latestVer, latestTarget, found = v, t, true
		}
	}
	return avail, latestVer, latestTarget, found
}

func pickLinuxServerVersion(vers []ficsitVersion, reqVer string) (ficsitVersion, ficsitTarget, bool) {
	trimmed := strings.TrimSpace(reqVer)
	wantLatest := driver.IsLatestAlias(trimmed)
	for _, v := range vers {
		t, ok := findLinuxServerTarget(v.Targets)
		if !ok {
			continue
		}
		if wantLatest || strings.EqualFold(strings.TrimSpace(v.Version), trimmed) {
			return v, t, true
		}
	}
	return ficsitVersion{}, ficsitTarget{}, false
}

func findLinuxServerTarget(targets []ficsitTarget) (ficsitTarget, bool) {
	for _, t := range targets {
		if strings.EqualFold(strings.TrimSpace(t.TargetName), targetLinuxServer) && strings.TrimSpace(t.Link) != "" {
			return t, true
		}
	}
	return ficsitTarget{}, false
}

func mapDependencies(deps []ficsitDependency) []driver.Dependency {
	var out []driver.Dependency
	seen := make(map[string]struct{}, len(deps))
	for _, d := range deps {
		if d.Optional {
			continue
		}
		id := strings.TrimSpace(d.ModID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, driver.Dependency{ID: id, Title: id})
	}
	return out
}

func mapSortField(sortMode, query string) (orderBy, order string) {
	if strings.TrimSpace(query) != "" && (sortMode == "" || sortMode == driver.SortPopular) {
		return "search", "desc"
	}
	switch sortMode {
	case driver.SortTrend, driver.SortPopular:
		return "popularity", "desc"
	case driver.SortTopRated:
		return "hotness", "desc"
	case driver.SortNewest:
		return "created_at", "desc"
	case driver.SortUpdated:
		return "last_version_date", "desc"
	default:
		return "downloads", "desc"
	}
}

func absFicsitURL(origin, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "/") {
		return origin + trimmed
	}
	return trimmed
}

func firstGQLErr(errs []gqlError, fallback string) error {
	if len(errs) > 0 && strings.TrimSpace(errs[0].Message) != "" {
		return fmt.Errorf("ficsit graphql: %s", errs[0].Message)
	}
	return fmt.Errorf("%s", fallback)
}
