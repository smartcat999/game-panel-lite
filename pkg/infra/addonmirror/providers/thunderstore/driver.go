package thunderstore

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const (
	maxAvailableVersions = 25
	resolveConcurrency   = 4
)

var (
	communityPkgURLRe = regexp.MustCompile(`(?i)thunderstore\.io/c/([^/?#]+)/p/([^/?#]+)/([^/?#]+)`)
	legacyPkgURLRe    = regexp.MustCompile(`(?i)thunderstore\.io/package/([^/?#]+)/([^/?#]+)`)
	trailingSemVerRe  = regexp.MustCompile(`-\d+\.\d+\.\d+(?:[-+][A-Za-z0-9._-]+)?$`)
)

func init() {
	driver.Register(driver.ProviderThunderstore, func() driver.Driver { return &thunderstoreDriver{} })
}

type thunderstoreDriver struct {
	driver.BaseDriver
}

func (d *thunderstoreDriver) Name() string { return driver.ProviderThunderstore }

func (d *thunderstoreDriver) Capabilities(src driver.Source) driver.Capabilities {
	isCatalog := src.Format != driver.FormatServerPackage
	return driver.Capabilities{Catalog: isCatalog, LiveDetailOverlay: isCatalog}
}

func (d *thunderstoreDriver) ParseTokens(raw string) []string {
	var extra []string
	for _, re := range []*regexp.Regexp{communityPkgURLRe, legacyPkgURLRe} {
		for _, m := range re.FindAllStringSubmatch(raw, -1) {
			ns, name := m[len(m)-2], m[len(m)-1]
			if id, ok := formatCleanID(ns, name); ok && !driver.ContainsString(extra, id) {
				extra = append(extra, id)
			}
		}
	}
	for _, tok := range driver.SplitPlainTokens(raw, parsePlainToken) {
		if !driver.ContainsString(extra, tok) {
			extra = append(extra, tok)
		}
	}
	return extra
}

func (d *thunderstoreDriver) Search(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	return search(ctx, rt, src, params)
}

func (d *thunderstoreDriver) Detail(ctx context.Context, rt driver.Runtime, src driver.Source, id, _ string) (*driver.Item, error) {
	return detail(ctx, rt, src, id)
}

func (d *thunderstoreDriver) Resolve(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string, _ string) ([]driver.Item, error) {
	return resolveBatch(ctx, rt, src, tokens)
}

func (d *thunderstoreDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	if src.Format == driver.FormatServerPackage {
		return nil, driver.ErrUnsupportedSource
	}
	return resolveRelease(ctx, rt, src, req.Package, req.Version)
}

func (d *thunderstoreDriver) ResolveLatestVersions(ctx context.Context, rt driver.Runtime, src driver.Source, pkgIDs []string) (map[string]string, error) {
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
			if pkg, _, _, err := fetchExperimentalPackage(rCtx, rt, src, pkgID); err == nil && pkg != nil && pkg.Latest.VersionNumber != "" {
				mu.Lock()
				out[pkgID] = pkg.Latest.VersionNumber
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	return out, nil
}

type cyberstormListingResponse struct {
	Count   int                     `json:"count"`
	Results []cyberstormListingItem `json:"results"`
}

type cyberstormListingItem struct {
	Namespace     string               `json:"namespace"`
	Name          string               `json:"name"`
	Description   string               `json:"description"`
	IconURL       string               `json:"icon_url"`
	DownloadCount int64                `json:"download_count"`
	RatingCount   int64                `json:"rating_count"`
	Size          int64                `json:"size"`
	LastUpdated   string               `json:"last_updated"`
	Categories    []cyberstormCategory `json:"categories"`
}

type cyberstormCategory struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type cyberstormVersionEntry struct {
	VersionNumber string `json:"version_number"`
	DateCreated   string `json:"datetime_created"`
	DownloadCount int64  `json:"download_count"`
	DownloadURL   string `json:"download_url"`
}

type experimentalPackage struct {
	Namespace     string              `json:"namespace"`
	Name          string              `json:"name"`
	FullName      string              `json:"full_name"`
	Owner         string              `json:"owner"`
	PackageURL    string              `json:"package_url"`
	DateCreated   string              `json:"date_created"`
	DateUpdated   string              `json:"date_updated"`
	IsDeprecated  bool                `json:"is_deprecated"`
	Downloads     int64               `json:"total_downloads"`
	Latest        experimentalVersion `json:"latest"`
	CommunityList []communityListing  `json:"community_listings"`
}

type communityListing struct {
	Categories []string `json:"categories"`
	Community  string   `json:"community"`
}

type experimentalVersion struct {
	Namespace     string   `json:"namespace"`
	Name          string   `json:"name"`
	VersionNumber string   `json:"version_number"`
	FullName      string   `json:"full_name"`
	Description   string   `json:"description"`
	Icon          string   `json:"icon"`
	Dependencies  []string `json:"dependencies"`
	DownloadURL   string   `json:"download_url"`
	Downloads     int64    `json:"downloads"`
	FileSize      int64    `json:"file_size"`
	DateCreated   string   `json:"date_created"`
	WebsiteURL    string   `json:"website_url"`
}

func parseThunderstoreEndpoint(rawEndpoint string) (string, string, error) {
	trimmed := strings.TrimSpace(rawEndpoint)
	if trimmed == "" {
		return "", "", fmt.Errorf("%w: thunderstore upstream requires endpoint with /c/<community>", driver.ErrInvalidRequest)
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", "", fmt.Errorf("%w: invalid thunderstore endpoint %q", driver.ErrInvalidRequest, rawEndpoint)
	}
	baseURL := u.Scheme + "://" + u.Host
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	community := ""
	for i := 0; i+1 < len(segs); i++ {
		if strings.EqualFold(segs[i], "c") && segs[i+1] != "" {
			community = segs[i+1]
			break
		}
	}
	if community == "" {
		return "", "", fmt.Errorf("%w: thunderstore endpoint %q must include /c/<community>", driver.ErrInvalidRequest, rawEndpoint)
	}
	return baseURL, community, nil
}

func formatCleanID(ns, name string) (string, bool) {
	cleanNS := strings.TrimSpace(ns)
	cleanName := strings.TrimSpace(name)
	if cleanNS == "" || cleanName == "" {
		return "", false
	}
	return cleanNS + "-" + cleanName, true
}

func parsePlainToken(tok string) (string, bool) {
	if ns, name, ok := parseThunderstoreID(tok); ok {
		return ns + "-" + name, true
	}
	return "", false
}

func parseThunderstoreID(raw string) (string, string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", false
	}
	for _, re := range []*regexp.Regexp{communityPkgURLRe, legacyPkgURLRe} {
		if m := re.FindStringSubmatch(trimmed); len(m) >= 3 {
			return strings.TrimSpace(m[len(m)-2]), strings.TrimSpace(m[len(m)-1]), true
		}
	}
	if ns, rest, ok := strings.Cut(trimmed, "/"); ok {
		return formatSplitID(ns, rest)
	}
	ns, rest, ok := strings.Cut(trimmed, "-")
	if !ok {
		return "", "", false
	}
	return formatSplitID(ns, rest)
}

func formatSplitID(ns, rawName string) (string, string, bool) {
	cleanNS := strings.TrimSpace(ns)
	cleanName := trailingSemVerRe.ReplaceAllString(strings.TrimSpace(rawName), "")
	if cleanNS == "" || cleanName == "" {
		return "", "", false
	}
	return cleanNS, cleanName, true
}

func search(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	baseURL, community, err := parseThunderstoreEndpoint(src.Endpoint)
	if err != nil {
		return nil, err
	}
	page := max(params.Page, 1)
	rawKey := fmt.Sprintf("%s|%s|%s|%s|%d", baseURL, community, params.Sort, strings.TrimSpace(params.Query), page)
	ck := driver.CacheHashKey("thunderstore:search", rawKey)
	var cached driver.Result
	if rt.GetCachedJSON(ctx, ck, &cached) {
		return sliceResultLimit(&cached, params.Limit), nil
	}
	res, err := queryCyberstormListing(ctx, rt, baseURL, community, params, page)
	if err != nil {
		return nil, err
	}
	rt.SetCachedJSON(ctx, ck, res)
	return sliceResultLimit(res, params.Limit), nil
}

func queryCyberstormListing(ctx context.Context, rt driver.Runtime, baseURL, community string, params driver.SearchParams, page int) (*driver.Result, error) {
	q := url.Values{}
	q.Set("deprecated", "False")
	q.Set("nsfw", "False")
	q.Set("ordering", mapCyberstormOrdering(params.Sort))
	q.Set("page", strconv.Itoa(page))
	if trimmed := strings.TrimSpace(params.Query); trimmed != "" {
		q.Set("q", trimmed)
	}
	u := fmt.Sprintf("%s/api/cyberstorm/listing/%s/?%s", baseURL, url.PathEscape(community), q.Encode())
	var raw cyberstormListingResponse
	if err := rt.DoGetJSON(ctx, u, &raw); err != nil {
		return nil, err
	}
	items := make([]driver.Item, 0, len(raw.Results))
	for _, entry := range raw.Results {
		items = append(items, mapListingToItem(baseURL, community, entry))
	}
	return &driver.Result{Configured: true, Total: raw.Count, Items: items}, nil
}

func mapCyberstormOrdering(sortMode string) string {
	switch sortMode {
	case driver.SortTrend, driver.SortTopRated:
		return "top-rated"
	case driver.SortNewest:
		return "newest"
	case driver.SortUpdated:
		return "last-updated"
	default:
		return "most-downloaded"
	}
}

func sliceResultLimit(res *driver.Result, limit int) *driver.Result {
	if res == nil || limit <= 0 || len(res.Items) <= limit {
		return res
	}
	cp := *res
	cp.Items = res.Items[:limit]
	return &cp
}

func mapListingToItem(baseURL, community string, r cyberstormListingItem) driver.Item {
	tags := make([]string, 0, len(r.Categories))
	for _, cat := range r.Categories {
		if t := strings.TrimSpace(cat.Name); t != "" {
			tags = append(tags, t)
		}
	}
	id := r.Namespace + "-" + r.Name
	return driver.Item{
		ID:          id,
		Title:       strings.TrimSpace(r.Name),
		Summary:     strings.TrimSpace(r.Description),
		Description: strings.TrimSpace(r.Description),
		Author:      strings.TrimSpace(r.Namespace),
		IconURL:     strings.TrimSpace(r.IconURL),
		SourceURL:   fmt.Sprintf("%s/c/%s/p/%s/%s/", baseURL, url.PathEscape(community), url.PathEscape(r.Namespace), url.PathEscape(r.Name)),
		Downloads:   r.DownloadCount,
		SizeBytes:   r.Size,
		UpdatedAt:   r.LastUpdated,
		Tags:        tags,
	}
}

func detail(ctx context.Context, rt driver.Runtime, src driver.Source, id string) (*driver.Item, error) {
	pkg, ns, name, err := fetchExperimentalPackage(ctx, rt, src, id)
	if err != nil {
		return nil, err
	}
	baseURL, community, _ := parseThunderstoreEndpoint(src.Endpoint)
	versions := fetchSortedVersions(ctx, rt, baseURL, ns, name, pkg.Latest.VersionNumber)
	readme := fetchVersionReadme(ctx, rt, baseURL, ns, name, pkg.Latest.VersionNumber)
	item := buildPackageItem(pkg, baseURL, community, versions, readme)
	return &item, nil
}

func fetchExperimentalPackage(ctx context.Context, rt driver.Runtime, src driver.Source, id string) (*experimentalPackage, string, string, error) {
	baseURL, _, err := parseThunderstoreEndpoint(src.Endpoint)
	if err != nil {
		return nil, "", "", err
	}
	ns, name, ok := parseThunderstoreID(id)
	if !ok {
		return nil, "", "", driver.ErrNotFound
	}
	u := fmt.Sprintf("%s/api/experimental/package/%s/%s/", baseURL, url.PathEscape(ns), url.PathEscape(name))
	ck := driver.CacheHashKey("thunderstore:exp_pkg", u)
	var cached experimentalPackage
	if rt.GetCachedJSON(ctx, ck, &cached) && cached.Name != "" {
		return &cached, cached.Namespace, cached.Name, nil
	}
	var pkg experimentalPackage
	if err := rt.DoGetJSON(ctx, u, &pkg); err != nil {
		return nil, "", "", err
	}
	if pkg.Namespace == "" || pkg.Name == "" {
		return nil, "", "", driver.ErrNotFound
	}
	rt.SetCachedJSON(ctx, ck, pkg)
	return &pkg, pkg.Namespace, pkg.Name, nil
}

func fetchExperimentalVersion(ctx context.Context, rt driver.Runtime, baseURL, ns, name, ver string) (*experimentalVersion, error) {
	u := fmt.Sprintf("%s/api/experimental/package/%s/%s/%s/", baseURL, url.PathEscape(ns), url.PathEscape(name), url.PathEscape(ver))
	ck := driver.CacheHashKey("thunderstore:exp_ver", u)
	var cached experimentalVersion
	if rt.GetCachedJSON(ctx, ck, &cached) && cached.VersionNumber != "" {
		return &cached, nil
	}
	var entry experimentalVersion
	if err := rt.DoGetJSON(ctx, u, &entry); err != nil {
		return nil, err
	}
	if entry.VersionNumber == "" {
		return nil, driver.ErrNotFound
	}
	rt.SetCachedJSON(ctx, ck, entry)
	return &entry, nil
}

func fetchSortedVersions(ctx context.Context, rt driver.Runtime, baseURL, ns, name, latestVer string) []string {
	u := fmt.Sprintf("%s/api/cyberstorm/package/%s/%s/versions/", baseURL, url.PathEscape(ns), url.PathEscape(name))
	ck := driver.CacheHashKey("thunderstore:versions", u)
	var entries []cyberstormVersionEntry
	if !rt.GetCachedJSON(ctx, ck, &entries) || len(entries) == 0 {
		if err := rt.DoGetJSON(ctx, u, &entries); err == nil && len(entries) > 0 {
			rt.SetCachedJSON(ctx, ck, entries)
		}
	}
	return normalizeAvailableVersions(entries, latestVer)
}

func normalizeAvailableVersions(entries []cyberstormVersionEntry, latestVer string) []string {
	seen := make(map[string]bool, len(entries)+1)
	var vers []string
	for _, e := range entries {
		v := strings.TrimSpace(e.VersionNumber)
		if v != "" && !seen[v] {
			seen[v] = true
			vers = append(vers, v)
		}
	}
	slices.SortFunc(vers, driver.CompareSemanticDesc)
	cleanLatest := strings.TrimSpace(latestVer)
	if cleanLatest != "" {
		vers = append([]string{cleanLatest}, slices.DeleteFunc(vers, func(s string) bool { return s == cleanLatest })...)
	}
	if len(vers) > maxAvailableVersions {
		vers = vers[:maxAvailableVersions]
	}
	return vers
}

func fetchVersionReadme(ctx context.Context, rt driver.Runtime, baseURL, ns, name, ver string) string {
	if ver == "" {
		return ""
	}
	u := fmt.Sprintf("%s/api/experimental/package/%s/%s/%s/readme/", baseURL, url.PathEscape(ns), url.PathEscape(name), url.PathEscape(ver))
	ck := driver.CacheHashKey("thunderstore:readme", u)
	var doc struct {
		Markdown string `json:"markdown"`
	}
	if rt.GetCachedJSON(ctx, ck, &doc) && doc.Markdown != "" {
		return doc.Markdown
	}
	if err := rt.DoGetJSON(ctx, u, &doc); err == nil && strings.TrimSpace(doc.Markdown) != "" {
		rt.SetCachedJSON(ctx, ck, doc)
	}
	return strings.TrimSpace(doc.Markdown)
}

func buildPackageItem(pkg *experimentalPackage, baseURL, community string, versions []string, readme string) driver.Item {
	id := pkg.Namespace + "-" + pkg.Name
	srcURL := strings.TrimSpace(pkg.PackageURL)
	if srcURL == "" {
		srcURL = fmt.Sprintf("%s/c/%s/p/%s/%s/", baseURL, url.PathEscape(community), url.PathEscape(pkg.Namespace), url.PathEscape(pkg.Name))
	}
	tags := extractCommunityTags(pkg.CommunityList, community)
	desc := driver.FirstNonEmpty(readme, pkg.Latest.Description)
	latestVer := strings.TrimSpace(pkg.Latest.VersionNumber)
	return driver.Item{
		ID: id, Title: pkg.Name, Summary: strings.TrimSpace(pkg.Latest.Description), Description: desc,
		Author: driver.FirstNonEmpty(pkg.Owner, pkg.Namespace), IconURL: strings.TrimSpace(pkg.Latest.Icon),
		SourceURL: srcURL, UpstreamURL: strings.TrimSpace(pkg.Latest.WebsiteURL),
		FileURL: strings.TrimSpace(pkg.Latest.DownloadURL), FileName: fmt.Sprintf("%s-%s.zip", id, latestVer),
		LatestVersion: latestVer, AvailableVersions: versions, Downloads: pkg.Latest.Downloads,
		SizeBytes: pkg.Latest.FileSize, UpdatedAt: driver.FirstNonEmpty(pkg.DateUpdated, pkg.Latest.DateCreated),
		Tags: tags, Dependencies: parseDependencies(pkg.Latest.Dependencies, id),
	}
}

func extractCommunityTags(listings []communityListing, community string) []string {
	for _, cl := range listings {
		if strings.EqualFold(strings.TrimSpace(cl.Community), community) && len(cl.Categories) > 0 {
			return slices.Clone(cl.Categories)
		}
	}
	if len(listings) > 0 {
		return slices.Clone(listings[0].Categories)
	}
	return nil
}

func parseDependencies(raw []string, selfID string) []driver.Dependency {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(raw))
	out := make([]driver.Dependency, 0, len(raw))
	for _, entry := range raw {
		ns, name, ok := parseThunderstoreID(entry)
		if !ok {
			continue
		}
		depID := ns + "-" + name
		if strings.EqualFold(depID, selfID) || seen[depID] {
			continue
		}
		seen[depID] = true
		out = append(out, driver.Dependency{ID: depID, Title: name})
	}
	return out
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

func resolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, pkgID, requestedVer string) (*driver.ResolvedRelease, error) {
	pkg, ns, name, err := fetchExperimentalPackage(ctx, rt, src, pkgID)
	if err != nil {
		return nil, err
	}
	baseURL, community, _ := parseThunderstoreEndpoint(src.Endpoint)
	chosen := &pkg.Latest
	reqVer := strings.TrimSpace(requestedVer)
	if reqVer != "" && !driver.IsLatestAlias(reqVer) && !strings.EqualFold(reqVer, pkg.Latest.VersionNumber) {
		chosen, err = fetchExperimentalVersion(ctx, rt, baseURL, ns, name, reqVer)
		if err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(chosen.DownloadURL) == "" {
		return nil, driver.ErrNotFound
	}
	id := ns + "-" + name
	deps := parseDependencies(chosen.Dependencies, id)
	item := buildPackageItem(pkg, baseURL, community, []string{chosen.VersionNumber}, "")
	item.LatestVersion = chosen.VersionNumber
	item.Dependencies = deps
	return &driver.ResolvedRelease{
		Package: id, Version: chosen.VersionNumber, Build: driver.FirstNonEmpty(chosen.FullName, id+"-"+chosen.VersionNumber),
		DownloadURL: chosen.DownloadURL, FileName: fmt.Sprintf("%s-%s.zip", id, chosen.VersionNumber),
		ArchiveFormat: driver.ArchiveFormatZip, SizeBytes: chosen.FileSize, Dependencies: deps,
		PublishedAt: driver.ParseRFC3339Ptr(chosen.DateCreated), UpstreamURL: item.SourceURL, Item: &item,
	}, nil
}
