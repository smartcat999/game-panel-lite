package cs2plugins

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt string    `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Body        string    `json:"body"`
	Assets      []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type ghRepoInfo struct {
	FullName    string   `json:"full_name"`
	Description string   `json:"description"`
	HTMLURL     string   `json:"html_url"`
	Stars       int64    `json:"stargazers_count"`
	Topics      []string `json:"topics"`
	Owner       struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"owner"`
}

var (
	atomTagRe       = regexp.MustCompile(`<link[^>]+href="https://github\.com/[^"]+/releases/tag/([^"]+)"`)
	expandedAssetRe = regexp.MustCompile(`href="(/[^"]+/releases/download/([^/"\s]+)/([^"\s]+))"`)
)

func githubAPIBase(src driver.Source) string {
	ep := strings.TrimRight(strings.TrimSpace(src.Endpoint), "/")
	if ep == "" {
		return driver.DefaultGitHubAPIBaseURL
	}
	return ep
}

func githubHeaders(src driver.Source) map[string]string {
	h := map[string]string{
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
	}
	if key := strings.TrimSpace(src.APIKey); key != "" {
		h["Authorization"] = "Bearer " + key
	}
	return h
}

func fetchRepoInfo(ctx context.Context, rt driver.Runtime, src driver.Source, owner, repo string) *ghRepoInfo {
	base := githubAPIBase(src)
	ck := driver.CacheHashKey("cs2p:ghrepo", base+"|"+owner+"/"+repo)
	var cached ghRepoInfo
	if rt.GetCachedJSON(ctx, ck, &cached) && cached.FullName != "" {
		return &cached
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s", base, url.PathEscape(owner), url.PathEscape(repo))
	var info ghRepoInfo
	if err := rt.DoGetJSONWithHeaders(ctx, endpoint, githubHeaders(src), &info); err != nil || info.FullName == "" {
		return nil
	}
	rt.SetCachedJSON(ctx, ck, info)
	return &info
}

func fetchRepoReleases(ctx context.Context, rt driver.Runtime, src driver.Source, owner, repo string) ([]ghRelease, error) {
	base := githubAPIBase(src)
	ck := driver.CacheHashKey("cs2p:ghrel", base+"|"+owner+"/"+repo)
	var cached []ghRelease
	if rt.GetCachedJSON(ctx, ck, &cached) && len(cached) > 0 {
		return cached, nil
	}
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=15", base, url.PathEscape(owner), url.PathEscape(repo))
	var raw []ghRelease
	err := rt.DoGetJSONWithHeaders(ctx, endpoint, githubHeaders(src), &raw)
	if err != nil {
		if base == driver.DefaultGitHubAPIBaseURL {
			if fb, fbErr := fetchReleasesFromHTML(ctx, rt, owner, repo); fbErr == nil && len(fb) > 0 {
				rt.SetCachedJSON(ctx, ck, fb)
				return fb, nil
			}
		}
		return nil, err
	}
	filtered := filterValidReleases(raw)
	if len(filtered) > 0 {
		rt.SetCachedJSON(ctx, ck, filtered)
	}
	return filtered, nil
}

func filterValidReleases(raw []ghRelease) []ghRelease {
	out := make([]ghRelease, 0, len(raw))
	for _, r := range raw {
		if r.Draft || strings.TrimSpace(r.TagName) == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

func fetchReleasesFromHTML(ctx context.Context, rt driver.Runtime, owner, repo string) ([]ghRelease, error) {
	atomURL := fmt.Sprintf("https://github.com/%s/%s/releases.atom", url.PathEscape(owner), url.PathEscape(repo))
	atomBody, err := rt.DoGetBytes(ctx, atomURL)
	if err != nil {
		return nil, err
	}
	tags := parseAtomTags(string(atomBody), 6)
	if len(tags) == 0 {
		return nil, driver.ErrNotFound
	}
	var out []ghRelease
	for _, tag := range tags {
		expURL := fmt.Sprintf("https://github.com/%s/%s/releases/expanded_assets/%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(tag))
		htmlBody, hErr := rt.DoGetBytes(ctx, expURL)
		if hErr != nil {
			continue
		}
		assets := parseExpandedAssets(string(htmlBody))
		out = append(out, ghRelease{
			TagName: tag,
			HTMLURL: fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, tag),
			Assets:  assets,
		})
	}
	if len(out) == 0 {
		return nil, driver.ErrNotFound
	}
	return out, nil
}

func parseAtomTags(atom string, limit int) []string {
	var tags []string
	for _, m := range atomTagRe.FindAllStringSubmatch(atom, -1) {
		if decoded, err := url.PathUnescape(m[1]); err == nil && decoded != "" && !driver.ContainsString(tags, decoded) {
			tags = append(tags, decoded)
			if len(tags) >= limit {
				break
			}
		}
	}
	return tags
}

func parseExpandedAssets(html string) []ghAsset {
	var out []ghAsset
	seen := make(map[string]bool)
	for _, m := range expandedAssetRe.FindAllStringSubmatch(html, -1) {
		href, name := m[1], m[3]
		if decoded, err := url.PathUnescape(name); err == nil && decoded != "" {
			name = decoded
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, ghAsset{Name: name, BrowserDownloadURL: "https://github.com" + href})
	}
	return out
}

func selectPluginAsset(assets []ghAsset, isCSS bool) (ghAsset, bool) {
	bestScore := -1
	var best ghAsset
	for _, a := range assets {
		score := scoreAsset(a.Name, isCSS)
		if score > bestScore {
			bestScore = score
			best = a
		}
	}
	return best, bestScore >= 0
}

func scoreAsset(name string, isCSS bool) int {
	low := strings.ToLower(strings.TrimSpace(name))
	if !strings.HasSuffix(low, ".zip") && !strings.HasSuffix(low, ".tar.gz") && !strings.HasSuffix(low, ".tgz") {
		return -1
	}
	if strings.Contains(low, "windows") || strings.Contains(low, "win32") || strings.Contains(low, "win64") || strings.Contains(low, "-source") {
		return -1
	}
	if isCSS {
		return scoreCSSAsset(low)
	}
	return scoreGenericPluginAsset(low)
}

func scoreCSSAsset(low string) int {
	if !strings.Contains(low, "with-runtime") || !strings.Contains(low, "linux") {
		return -1
	}
	return 100
}

func scoreGenericPluginAsset(low string) int {
	score := 50
	if strings.Contains(low, "with-cssharp") || strings.Contains(low, "with-runtime") {
		score -= 20
	}
	if strings.Contains(low, "linux") {
		score += 15
	}
	if strings.Contains(low, "no-map-configs") {
		score -= 5
	}
	if strings.HasSuffix(low, ".zip") {
		score += 5
	}
	return score
}

func searchGitHubPlugins(ctx context.Context, rt driver.Runtime, src driver.Source, query string) []driver.Item {
	base := githubAPIBase(src)
	q := url.Values{}
	q.Set("q", strings.TrimSpace(query)+" topic:counterstrikesharp-plugin")
	q.Set("sort", "stars")
	q.Set("order", "desc")
	q.Set("per_page", "12")
	var env struct {
		Items []ghRepoInfo `json:"items"`
	}
	if err := rt.DoGetJSONWithHeaders(ctx, base+"/search/repositories?"+q.Encode(), githubHeaders(src), &env); err != nil {
		return nil
	}
	out := make([]driver.Item, 0, len(env.Items))
	for _, r := range env.Items {
		if r.Owner.Login == "" || strings.TrimSpace(r.FullName) == "" {
			continue
		}
		_, repoName, ok := strings.Cut(r.FullName, "/")
		if !ok {
			continue
		}
		id := toCanonicalPackageID(r.Owner.Login, repoName)
		out = append(out, driver.Item{
			ID:           id,
			Title:        repoName,
			Summary:      strings.TrimSpace(r.Description),
			Author:       r.Owner.Login,
			IconURL:      strings.TrimSpace(r.Owner.AvatarURL),
			SourceURL:    driver.FirstNonEmpty(r.HTMLURL, "https://github.com/"+r.FullName),
			UpstreamURL:  driver.FirstNonEmpty(r.HTMLURL, "https://github.com/"+r.FullName),
			Downloads:    r.Stars,
			Tags:         r.Topics,
			Dependencies: []driver.Dependency{{ID: PackageCounterStrikeSharp, Title: "CounterStrikeSharp (.NET 运行时插件框架)"}},
		})
	}
	return out
}
