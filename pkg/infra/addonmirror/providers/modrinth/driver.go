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

var modrinthURLSlugRe = regexp.MustCompile(`(?i)modrinth\.com/(?:plugin|mod|datapack)/([a-zA-Z0-9_-]+)`)

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

type modrinthDepsResponse struct {
	Projects []modrinthProject `json:"projects"`
	Versions []struct {
		Dependencies []struct {
			ProjectID      string `json:"project_id"`
			DependencyType string `json:"dependency_type"`
		} `json:"dependencies"`
	} `json:"versions"`
}

type modrinthVersionEntry struct {
	ID            string   `json:"id"`
	VersionNumber string   `json:"version_number"`
	VersionType   string   `json:"version_type"`
	Changelog     string   `json:"changelog"`
	DatePublished string   `json:"date_published"`
	GameVersions  []string `json:"game_versions"`
	Loaders       []string `json:"loaders"`
	Files         []struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		Primary  bool   `json:"primary"`
		Size     int64  `json:"size"`
	} `json:"files"`
	Dependencies []struct {
		ProjectID      string `json:"project_id"`
		DependencyType string `json:"dependency_type"`
	} `json:"dependencies"`
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

func detail(ctx context.Context, rt driver.Runtime, src driver.Source, slugOrID, _ string) (*driver.Item, error) {
	ck := driver.CacheHashKey("modrinth:detail", slugOrID)
	var cached driver.Item
	if rt.GetCachedJSON(ctx, ck, &cached) && cached.ID != "" {
		return &cached, nil
	}
	var proj modrinthProject
	if err := rt.DoGetJSON(ctx, rt.ModrinthEndpoint(src)+"/project/"+url.PathEscape(slugOrID), &proj); err != nil {
		return nil, err
	}
	item := mapProjectToItem(proj, src.ModrinthProjectType)
	var deps modrinthDepsResponse
	if err := rt.DoGetJSON(ctx, rt.ModrinthEndpoint(src)+"/project/"+url.PathEscape(slugOrID)+"/dependencies", &deps); err == nil {
		item.Dependencies = extractRequiredDeps(deps, item.ID)
	}
	rt.SetCachedJSON(ctx, ck, item)
	return &item, nil
}

func extractRequiredDeps(deps modrinthDepsResponse, selfID string) []driver.Dependency {
	reqIDs := make(map[string]bool)
	for _, v := range deps.Versions {
		for _, d := range v.Dependencies {
			if d.DependencyType == "required" && d.ProjectID != "" {
				reqIDs[d.ProjectID] = true
			}
		}
	}
	var out []driver.Dependency
	for _, dp := range deps.Projects {
		if len(reqIDs) > 0 && !reqIDs[dp.ID] {
			continue
		}
		id := driver.FirstNonEmpty(dp.Slug, dp.ID)
		if id != "" && id != selfID {
			out = append(out, driver.Dependency{ID: id, Title: dp.Title})
		}
	}
	return out
}

func resolveBatch(ctx context.Context, rt driver.Runtime, src driver.Source, slugs []string, gameVersion string) ([]driver.Item, error) {
	if len(slugs) > 40 {
		slugs = slugs[:40]
	}
	ck := driver.CacheHashKey("modrinth:resolve", strings.Join(slugs, ","))
	var cached []driver.Item
	if rt.GetCachedJSON(ctx, ck, &cached) && len(cached) > 0 {
		return cached, nil
	}
	out := make([]driver.Item, 0, len(slugs))
	for _, token := range slugs {
		item, err := detail(ctx, rt, src, token, gameVersion)
		if err != nil {
			if errors.Is(err, driver.ErrNotFound) {
				continue
			}
			return nil, err
		}
		out = append(out, *item)
	}
	if len(out) > 0 {
		rt.SetCachedJSON(ctx, ck, out)
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

// Resolve resolves a specific or latest Modrinth project version and its primary .jar download.
func Resolve(ctx context.Context, rt driver.Runtime, src driver.Source, slugOrID, requestedVersion, gameVersion string) (*driver.ResolvedRelease, error) {
	item, err := detail(ctx, rt, src, slugOrID, gameVersion)
	if err != nil {
		return nil, err
	}
	entries, err := queryVersions(ctx, rt, src, slugOrID, gameVersion)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 && strings.TrimSpace(gameVersion) != "" {
		entries, err = queryVersions(ctx, rt, src, slugOrID, "")
		if err != nil {
			return nil, err
		}
	}
	chosen, ok := pickVersion(entries, requestedVersion)
	if !ok || len(chosen.Files) == 0 {
		return nil, driver.ErrNotFound
	}
	file := chosen.Files[0]
	for _, f := range chosen.Files {
		if f.Primary {
			file = f
			break
		}
	}
	ver := driver.FirstNonEmpty(chosen.VersionNumber, chosen.ID)
	item.LatestVersion = ver
	verURL := strings.TrimRight(item.SourceURL, "/")
	if verURL != "" && chosen.ID != "" {
		verURL = verURL + "/version/" + url.PathEscape(chosen.ID)
	}
	return &driver.ResolvedRelease{
		Package: item.ID, Version: ver, Build: chosen.ID,
		DownloadURL: file.URL, FileName: file.Filename, ArchiveFormat: driver.ArchiveFormatJar,
		TargetPath: file.Filename, SizeBytes: file.Size, GameVersions: chosen.GameVersions,
		Loaders: chosen.Loaders, Dependencies: item.Dependencies, PublishedAt: driver.ParseRFC3339Ptr(chosen.DatePublished),
		Changelog: strings.TrimSpace(chosen.Changelog), UpstreamURL: verURL, Item: item,
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
	return modrinthVersionEntry{}, false
}
