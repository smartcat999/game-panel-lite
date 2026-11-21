package modrinth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const (
	maxAvailableVersions       = 25
	modrinthResolveConcurrency = 4
)

var (
	modrinthURLSlugRe     = regexp.MustCompile(`(?i)modrinth\.com/(?:plugin|mod|datapack)/([a-zA-Z0-9_-]+)`)
	modrinthCoordUnsafeRe = regexp.MustCompile(`[^A-Za-z0-9._+-]+`)
)

func init() {
	driver.Register(driver.ProviderModrinth, func() driver.Driver { return &modrinthDriver{} })
}

type modrinthDriver struct {
	driver.BaseDriver
}

func (d *modrinthDriver) Name() string { return driver.ProviderModrinth }

func (d *modrinthDriver) Capabilities(src driver.Source) driver.Capabilities {
	return driver.Capabilities{Catalog: src.Format != driver.FormatServerPackage}
}

func (d *modrinthDriver) ParseTokens(raw string) []string {
	var extra []string
	for _, m := range modrinthURLSlugRe.FindAllStringSubmatch(raw, -1) {
		if v := strings.TrimSpace(m[1]); v != "" && !driver.ContainsString(extra, v) {
			extra = append(extra, v)
		}
	}
	for _, tok := range driver.SplitPlainTokens(raw, nil) {
		if !driver.ContainsString(extra, tok) {
			extra = append(extra, tok)
		}
	}
	return extra
}

func (d *modrinthDriver) Search(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	return search(ctx, rt, src, params)
}

func (d *modrinthDriver) Detail(ctx context.Context, rt driver.Runtime, src driver.Source, id, gameVersion string) (*driver.Item, error) {
	return detail(ctx, rt, src, id, gameVersion)
}

func (d *modrinthDriver) Resolve(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string, gameVersion string) ([]driver.Item, error) {
	return resolveBatch(ctx, rt, src, tokens, gameVersion)
}

func (d *modrinthDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	if src.Format == driver.FormatServerPackage {
		return nil, driver.ErrUnsupportedSource
	}
	return Resolve(ctx, rt, src, req.Package, req.Version, req.GameVersion)
}

func (d *modrinthDriver) ResolveLatestVersions(ctx context.Context, rt driver.Runtime, src driver.Source, pkgIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(pkgIDs))
	rCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, id := range pkgIDs {
		if id == "" {
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
			if rel, err := Resolve(rCtx, rt, src, pkgID, "", ""); err == nil && rel != nil && rel.Version != "" {
				mu.Lock()
				out[pkgID] = rel.Version
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	return out, nil
}

type modrinthSearchHit struct {
	ProjectID    string   `json:"project_id"`
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Author       string   `json:"author"`
	IconURL      string   `json:"icon_url"`
	Downloads    int64    `json:"downloads"`
	Categories   []string `json:"categories"`
	DateModified string   `json:"date_modified"`
	Gallery      []string `json:"gallery"`
}

type modrinthSearchResponse struct {
	Hits      []modrinthSearchHit `json:"hits"`
	TotalHits int                 `json:"total_hits"`
}

type modrinthProject struct {
	ID          string            `json:"id"`
	Slug        string            `json:"slug"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Body        string            `json:"body"`
	IconURL     string            `json:"icon_url"`
	Downloads   int64             `json:"downloads"`
	Categories  []string          `json:"categories"`
	Updated     string            `json:"updated"`
	ProjectType string            `json:"project_type"`
	Gallery     []modrinthGallery `json:"gallery"`
}

type modrinthGallery struct {
	URL string `json:"url"`
}

type modrinthVersionFile struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Primary  bool   `json:"primary"`
	Size     int64  `json:"size"`
}

type modrinthVersionDep struct {
	ProjectID      string `json:"project_id"`
	DependencyType string `json:"dependency_type"`
}

type modrinthDepsResponse struct {
	Projects []modrinthProject `json:"projects"`
	Versions []struct {
		Dependencies []modrinthVersionDep `json:"dependencies"`
	} `json:"versions"`
}

type modrinthVersionEntry struct {
	ID            string                `json:"id"`
	VersionNumber string                `json:"version_number"`
	VersionType   string                `json:"version_type"`
	Changelog     string                `json:"changelog"`
	DatePublished string                `json:"date_published"`
	GameVersions  []string              `json:"game_versions"`
	Loaders       []string              `json:"loaders"`
	Files         []modrinthVersionFile `json:"files"`
	Dependencies  []modrinthVersionDep  `json:"dependencies"`
}

func search(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	rawKey := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d",
		src.ModrinthLoader, src.ModrinthProjectType, params.GameVersion,
		strings.TrimSpace(params.Query), params.Sort, params.Page, params.Limit)
	ck := driver.CacheHashKey("modrinth:search", rawKey)
	var cached driver.Result
	if rt.GetCachedJSON(ctx, ck, &cached) {
		return &cached, nil
	}
	res, err := querySearch(ctx, rt, src, params, params.GameVersion)
	if err != nil {
		return nil, err
	}
	// Brand-new Minecraft patch releases may have fewer tagged plugins than minor series; fall back gracefully.
	if res.Total == 0 && strings.TrimSpace(params.GameVersion) != "" && strings.TrimSpace(params.Query) == "" {
		res, err = querySearch(ctx, rt, src, params, "")
		if err != nil {
			return nil, err
		}
	}
	rt.SetCachedJSON(ctx, ck, res)
	return res, nil
}

func querySearch(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams, versionFilter string) (*driver.Result, error) {
	q := url.Values{}
	if trimmed := strings.TrimSpace(params.Query); trimmed != "" {
		q.Set("query", trimmed)
	}
	q.Set("limit", strconv.Itoa(params.Limit))
	q.Set("offset", strconv.Itoa((params.Page-1)*params.Limit))
	q.Set("index", sortIndex(params))
	if facets := buildFacets(src, versionFilter); facets != "" {
		q.Set("facets", facets)
	}
	var raw modrinthSearchResponse
	if err := rt.DoGetJSON(ctx, rt.ModrinthEndpoint(src)+"/search?"+q.Encode(), &raw); err != nil {
		return nil, err
	}
	items := make([]driver.Item, 0, len(raw.Hits))
	for _, h := range raw.Hits {
		items = append(items, mapHitToItem(h, src.ModrinthProjectType))
	}
	return &driver.Result{Configured: true, Total: raw.TotalHits, Items: items}, nil
}

func sortIndex(params driver.SearchParams) string {
	switch params.Sort {
	case driver.SortTrend, driver.SortTopRated:
		return "follows"
	case driver.SortNewest:
		return "newest"
	case driver.SortUpdated:
		return "updated"
	default:
		if strings.TrimSpace(params.Query) != "" {
			return "relevance"
		}
		return "downloads"
	}
}

func buildFacets(src driver.Source, gameVersion string) string {
	var groups [][]string
	pType := driver.FirstNonEmpty(src.ModrinthProjectType, driver.ProjectTypePlugin)
	groups = append(groups, []string{"project_type:" + pType})
	switch strings.ToLower(strings.TrimSpace(src.ModrinthLoader)) {
	case "paper":
		groups = append(groups, []string{"categories:paper", "categories:spigot", "categories:bukkit", "categories:purpur"})
	case "":
	default:
		groups = append(groups, []string{"categories:" + strings.ToLower(strings.TrimSpace(src.ModrinthLoader))})
	}
	if pType == driver.ProjectTypeMod {
		groups = append(groups, []string{"server_side:required", "server_side:optional"})
	}
	if v := strings.TrimSpace(gameVersion); v != "" && !strings.EqualFold(v, "latest") {
		groups = append(groups, []string{"versions:" + v})
	}
	raw, err := json.Marshal(groups)
	if err != nil {
		return ""
	}
	return string(raw)
}

func normalizeGameVersion(raw string) string {
	gv := strings.TrimSpace(raw)
	if gv == "" || driver.IsLatestAlias(gv) {
		return ""
	}
	return gv
}

func isStrictModSource(src driver.Source) bool {
	return strings.EqualFold(strings.TrimSpace(src.ModrinthProjectType), driver.ProjectTypeMod)
}

func sanitizeVersionCoord(raw string) string {
	val := strings.TrimSpace(raw)
	val = modrinthCoordUnsafeRe.ReplaceAllString(val, "-")
	return strings.Trim(val, "-")
}

func normalizeVersionCoord(entry modrinthVersionEntry) string {
	if v := sanitizeVersionCoord(entry.VersionNumber); v != "" {
		return v
	}
	return strings.TrimSpace(entry.ID)
}

func extractAvailableVersions(entries []modrinthVersionEntry, limit int) []string {
	if len(entries) == 0 || limit <= 0 {
		return nil
	}
	out := make([]string, 0, min(len(entries), limit))
	seen := make(map[string]bool, limit)
	for _, e := range entries {
		ver := normalizeVersionCoord(e)
		if ver == "" || seen[ver] {
			continue
		}
		seen[ver] = true
		out = append(out, ver)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func primaryFile(entry modrinthVersionEntry) (modrinthVersionFile, bool) {
	if len(entry.Files) == 0 {
		return modrinthVersionFile{}, false
	}
	for _, f := range entry.Files {
		if f.Primary {
			return f, true
		}
	}
	return entry.Files[0], true
}

func detailCacheKey(src driver.Source, slugOrID, gameVersion string) string {
	raw := fmt.Sprintf("%s|%s|%s|%s",
		strings.ToLower(strings.TrimSpace(src.ModrinthLoader)),
		strings.ToLower(strings.TrimSpace(src.ModrinthProjectType)),
		normalizeGameVersion(gameVersion),
		strings.TrimSpace(slugOrID),
	)
	return driver.CacheHashKey("modrinth:detail", raw)
}

func detail(ctx context.Context, rt driver.Runtime, src driver.Source, slugOrID, gameVersion string) (*driver.Item, error) {
	ck := detailCacheKey(src, slugOrID, gameVersion)
	var cached driver.Item
	if rt.GetCachedJSON(ctx, ck, &cached) && cached.ID != "" {
		return &cached, nil
	}
	var proj modrinthProject
	if err := rt.DoGetJSON(ctx, rt.ModrinthEndpoint(src)+"/project/"+url.PathEscape(slugOrID), &proj); err != nil {
		return nil, err
	}
	item := mapProjectToItem(proj, src.ModrinthProjectType)
	entries, vErr := fetchCompatibleVersions(ctx, rt, src, slugOrID, gameVersion)
	applyVersionMetaToItem(ctx, rt, src, slugOrID, &item, entries)
	if vErr == nil {
		rt.SetCachedJSON(ctx, ck, item)
	}
	return &item, nil
}

func applyVersionMetaToItem(ctx context.Context, rt driver.Runtime, src driver.Source, slugOrID string, item *driver.Item, entries []modrinthVersionEntry) {
	if item == nil || len(entries) == 0 {
		return
	}
	vers := extractAvailableVersions(entries, maxAvailableVersions)
	if len(vers) > 0 {
		item.AvailableVersions = vers
		item.LatestVersion = vers[0]
	}
	if file, ok := primaryFile(entries[0]); ok && file.Size > 0 {
		item.SizeBytes = file.Size
	}
	item.Dependencies = resolveVersionRequiredDeps(ctx, rt, src, slugOrID, item.ID, entries[0])
}

func resolveVersionRequiredDeps(ctx context.Context, rt driver.Runtime, src driver.Source, slugOrID, selfID string, entry modrinthVersionEntry) []driver.Dependency {
	reqIDs := make(map[string]bool)
	for _, d := range entry.Dependencies {
		if d.DependencyType == "required" && strings.TrimSpace(d.ProjectID) != "" {
			reqIDs[d.ProjectID] = true
		}
	}
	if len(reqIDs) == 0 {
		return nil
	}
	var deps modrinthDepsResponse
	endpoint := rt.ModrinthEndpoint(src) + "/project/" + url.PathEscape(slugOrID) + "/dependencies"
	if err := rt.DoGetJSON(ctx, endpoint, &deps); err != nil {
		return nil
	}
	var out []driver.Dependency
	for _, dp := range deps.Projects {
		if !reqIDs[dp.ID] {
			continue
		}
		id := driver.FirstNonEmpty(dp.Slug, dp.ID)
		if id != "" && id != selfID {
			out = append(out, driver.Dependency{ID: id, Title: dp.Title})
		}
	}
	return out
}

func resolveBatchCacheKey(src driver.Source, slugs []string, gameVersion string) string {
	raw := fmt.Sprintf("%s|%s|%s|%s",
		strings.ToLower(strings.TrimSpace(src.ModrinthLoader)),
		strings.ToLower(strings.TrimSpace(src.ModrinthProjectType)),
		normalizeGameVersion(gameVersion),
		strings.Join(slugs, ","),
	)
	return driver.CacheHashKey("modrinth:resolve", raw)
}

func resolveBatch(ctx context.Context, rt driver.Runtime, src driver.Source, slugs []string, gameVersion string) ([]driver.Item, error) {
	if len(slugs) > 40 {
		slugs = slugs[:40]
	}
	ck := resolveBatchCacheKey(src, slugs, gameVersion)
	var cached []driver.Item
	if rt.GetCachedJSON(ctx, ck, &cached) && len(cached) > 0 {
		return cached, nil
	}
	out, err := resolveSlugsConcurrent(ctx, rt, src, slugs, gameVersion)
	if err != nil {
		return nil, err
	}
	if len(out) > 0 {
		rt.SetCachedJSON(ctx, ck, out)
	}
	return out, nil
}

func resolveSlugsConcurrent(ctx context.Context, rt driver.Runtime, src driver.Source, slugs []string, gameVersion string) ([]driver.Item, error) {
	items := make([]*driver.Item, len(slugs))
	errs := make([]error, len(slugs))
	sem := make(chan struct{}, modrinthResolveConcurrency)
	var wg sync.WaitGroup
	for i, tok := range slugs {
		wg.Add(1)
		go func(idx int, token string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			items[idx], errs[idx] = detail(ctx, rt, src, token, gameVersion)
		}(i, tok)
	}
	wg.Wait()
	out := make([]driver.Item, 0, len(slugs))
	for i, err := range errs {
		if err != nil {
			if errors.Is(err, driver.ErrNotFound) {
				continue
			}
			return nil, err
		}
		if items[i] != nil {
			out = append(out, *items[i])
		}
	}
	return out, nil
}

func mapHitToItem(h modrinthSearchHit, fallbackType string) driver.Item {
	id := driver.FirstNonEmpty(h.Slug, h.ProjectID)
	pType := driver.FirstNonEmpty(fallbackType, driver.ProjectTypePlugin)
	return driver.Item{
		ID:          id,
		Title:       strings.TrimSpace(h.Title),
		Summary:     strings.TrimSpace(h.Description),
		Description: strings.TrimSpace(h.Description),
		Author:      strings.TrimSpace(h.Author),
		IconURL:     strings.TrimSpace(h.IconURL),
		GalleryURLs: h.Gallery,
		SourceURL:   fmt.Sprintf("https://modrinth.com/%s/%s", pType, id),
		Downloads:   h.Downloads,
		UpdatedAt:   h.DateModified,
		Tags:        h.Categories,
	}
}

func mapProjectToItem(p modrinthProject, fallbackType string) driver.Item {
	id := driver.FirstNonEmpty(p.Slug, p.ID)
	pType := driver.FirstNonEmpty(p.ProjectType, fallbackType, driver.ProjectTypePlugin)
	gallery := make([]string, 0, len(p.Gallery))
	for _, g := range p.Gallery {
		if strings.TrimSpace(g.URL) != "" {
			gallery = append(gallery, strings.TrimSpace(g.URL))
		}
	}
	return driver.Item{
		ID:          id,
		Title:       strings.TrimSpace(p.Title),
		Summary:     strings.TrimSpace(p.Description),
		Description: driver.FirstNonEmpty(p.Body, p.Description),
		IconURL:     strings.TrimSpace(p.IconURL),
		GalleryURLs: gallery,
		SourceURL:   fmt.Sprintf("https://modrinth.com/%s/%s", pType, id),
		Downloads:   p.Downloads,
		UpdatedAt:   p.Updated,
		Tags:        p.Categories,
	}
}

func fetchCompatibleVersions(ctx context.Context, rt driver.Runtime, src driver.Source, slugOrID, gameVersion string) ([]modrinthVersionEntry, error) {
	gv := normalizeGameVersion(gameVersion)
	entries, err := queryVersions(ctx, rt, src, slugOrID, gv)
	if err != nil {
		return nil, err
	}
	// Preserve fallback on untagged patch releases for server plugins, never for mods whose loader/API breaks across versions.
	if len(entries) == 0 && gv != "" && !isStrictModSource(src) {
		return queryVersions(ctx, rt, src, slugOrID, "")
	}
	return entries, nil
}

// Resolve resolves a specific or latest Modrinth project version and its primary .jar download.
func Resolve(ctx context.Context, rt driver.Runtime, src driver.Source, slugOrID, requestedVersion, gameVersion string) (*driver.ResolvedRelease, error) {
	item, err := detail(ctx, rt, src, slugOrID, gameVersion)
	if err != nil {
		return nil, err
	}
	entries, err := fetchCompatibleVersions(ctx, rt, src, slugOrID, gameVersion)
	if err != nil {
		return nil, err
	}
	chosen, ok := pickVersion(entries, requestedVersion)
	if !ok {
		return nil, driver.ErrNotFound
	}
	file, ok := primaryFile(chosen)
	if !ok {
		return nil, driver.ErrNotFound
	}
	ver := normalizeVersionCoord(chosen)
	deps := resolveVersionRequiredDeps(ctx, rt, src, slugOrID, item.ID, chosen)
	itemCopy := *item
	itemCopy.LatestVersion = ver
	itemCopy.Dependencies = deps
	verURL := strings.TrimRight(itemCopy.SourceURL, "/")
	if verURL != "" && chosen.ID != "" {
		verURL = verURL + "/version/" + url.PathEscape(chosen.ID)
	}
	return &driver.ResolvedRelease{
		Package: itemCopy.ID, Version: ver, Build: chosen.ID,
		DownloadURL: file.URL, FileName: file.Filename, ArchiveFormat: driver.ArchiveFormatJar,
		TargetPath: file.Filename, SizeBytes: file.Size, GameVersions: chosen.GameVersions,
		Loaders: chosen.Loaders, Dependencies: deps, PublishedAt: driver.ParseRFC3339Ptr(chosen.DatePublished),
		Changelog: strings.TrimSpace(chosen.Changelog), UpstreamURL: verURL, Item: &itemCopy,
	}, nil
}

func queryVersions(ctx context.Context, rt driver.Runtime, src driver.Source, slugOrID, gameVersion string) ([]modrinthVersionEntry, error) {
	q := url.Values{}
	if loaders := loaderList(src.ModrinthLoader); len(loaders) > 0 {
		raw, _ := json.Marshal(loaders)
		q.Set("loaders", string(raw))
	}
	if gv := strings.TrimSpace(gameVersion); gv != "" && !driver.IsLatestAlias(gv) {
		raw, _ := json.Marshal([]string{gv})
		q.Set("game_versions", string(raw))
	}
	endpoint := rt.ModrinthEndpoint(src) + "/project/" + url.PathEscape(strings.TrimSpace(slugOrID)) + "/version"
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	ck := driver.CacheHashKey("modrinth:versions", endpoint)
	var entries []modrinthVersionEntry
	if rt.GetCachedJSON(ctx, ck, &entries) && len(entries) > 0 {
		return entries, nil
	}
	if err := rt.DoGetJSON(ctx, endpoint, &entries); err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		rt.SetCachedJSON(ctx, ck, entries)
	}
	return entries, nil
}

func loaderList(loader string) []string {
	switch strings.ToLower(strings.TrimSpace(loader)) {
	case "paper":
		return []string{"paper", "spigot", "bukkit", "purpur"}
	case "":
		return nil
	default:
		return []string{strings.ToLower(strings.TrimSpace(loader))}
	}
}

func pickVersion(entries []modrinthVersionEntry, requested string) (modrinthVersionEntry, bool) {
	if len(entries) == 0 {
		return modrinthVersionEntry{}, false
	}
	req := strings.TrimSpace(requested)
	if req == "" || driver.IsLatestAlias(req) {
		return entries[0], true
	}
	for _, e := range entries {
		if strings.EqualFold(e.VersionNumber, req) || e.ID == req {
			return e, true
		}
	}
	sanitizedReq := sanitizeVersionCoord(req)
	for _, e := range entries {
		if sanitizedReq != "" && strings.EqualFold(sanitizeVersionCoord(e.VersionNumber), sanitizedReq) {
			return e, true
		}
	}
	return modrinthVersionEntry{}, false
}
