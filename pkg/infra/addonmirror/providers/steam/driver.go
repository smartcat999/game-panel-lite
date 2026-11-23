package steam

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

func init() {
	newDrv := func() driver.Driver { return &steamDriver{} }
	driver.Register(driver.ProviderSteam, newDrv)
	driver.Register(driver.ProviderSteamWorkshop, newDrv)
}

type steamDriver struct {
	driver.BaseDriver
}

func (d *steamDriver) Name() string { return driver.ProviderSteam }

func (d *steamDriver) Capabilities(src driver.Source) driver.Capabilities {
	if src.Format == driver.FormatServerPackage {
		return driver.Capabilities{ServerPackages: true}
	}
	return driver.Capabilities{Catalog: true}
}

func (d *steamDriver) ParseTokens(raw string) []string {
	var out []string
	for _, m := range steamURLIDRe.FindAllStringSubmatch(raw, -1) {
		if v := strings.TrimSpace(m[1]); v != "" && !driver.ContainsString(out, v) {
			out = append(out, v)
		}
	}
	for _, tok := range driver.SplitPlainTokens(raw, normalizeWorkshopToken) {
		if !driver.ContainsString(out, tok) {
			out = append(out, tok)
		}
	}
	return out
}

func normalizeWorkshopToken(part string) (string, bool) {
	if strings.HasPrefix(strings.ToLower(part), "workshop-") {
		part = part[len("workshop-"):]
	}
	return part, driver.IsDigitsOnly(part)
}

func (d *steamDriver) Search(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	if rt.SteamAPIKey() == "" {
		return &driver.Result{
			Configured: false,
			Message:    "尚未配置 STEAM_WEB_API_KEY，暂无法在线检索创意工坊榜单；你仍可直接粘贴创意工坊单品或合集链接导入。",
		}, nil
	}
	return searchWorkshop(ctx, rt, src, params)
}

func (d *steamDriver) Detail(ctx context.Context, rt driver.Runtime, src driver.Source, id, _ string) (*driver.Item, error) {
	return detailWorkshop(ctx, rt, src, id)
}

func (d *steamDriver) Resolve(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string, _ string) ([]driver.Item, error) {
	return resolveWorkshop(ctx, rt, src, tokens)
}

func (d *steamDriver) ListServerVersions(context.Context, driver.Runtime, driver.Source, string) ([]string, error) {
	return []string{"latest"}, nil
}

func (d *steamDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	if src.Format == driver.FormatServerPackage {
		return resolveServerRelease(src, req)
	}
	return resolveWorkshopRelease(ctx, rt, src, req)
}

func (d *steamDriver) ResolveLatestVersions(ctx context.Context, rt driver.Runtime, src driver.Source, pkgIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(pkgIDs))
	for start := 0; start < len(pkgIDs); start += 50 {
		end := min(start+50, len(pkgIDs))
		items, err := resolveWorkshop(ctx, rt, src, pkgIDs[start:end])
		if err != nil {
			continue
		}
		for _, resolved := range items {
			if resolved.ID != "" && resolved.LatestVersion != "" {
				out[resolved.ID] = resolved.LatestVersion
			}
		}
	}
	return out, nil
}

// ResolveWorkshopItem queries live metadata for a Workshop ID using Source endpoint overrides.
func ResolveWorkshopItem(ctx context.Context, rt driver.Runtime, src driver.Source, workshopID string) (*driver.Item, error) {
	trimmed := strings.TrimSpace(workshopID)
	if !driver.IsDigitsOnly(trimmed) {
		return nil, driver.ErrInvalidRequest
	}
	base := rt.SteamEndpoint(src)
	items, err := fetchFilesByIDs(ctx, rt, base, src.SteamAppID, []string{trimmed})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, driver.ErrNotFound
	}
	item := items[0]
	hydrateDependencyTitles(ctx, rt, base, src.SteamAppID, &item)
	return &item, nil
}

func resolveServerRelease(src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	ver := strings.TrimSpace(req.Version)
	if req.StrictVersion && !req.AllowBranches && !driver.IsLatestAlias(ver) {
		return nil, driver.ErrNotFound
	}
	if req.AllowBranches && ver == "" {
		ver = "stable"
	} else if !req.AllowBranches {
		ver = req.Version
	}
	return &driver.ResolvedRelease{
		Package:       req.Package,
		Version:       ver,
		DownloadURL:   driver.SteamCMDScheme + strconv.Itoa(src.SteamAppID),
		FileName:      "package.tar.gz",
		ArchiveFormat: driver.ArchiveFormatTarGz,
		TargetPath:    ".",
		UpstreamURL:   NewsURL(src.SteamAppID),
	}, nil
}

func resolveWorkshopRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	it, err := ResolveWorkshopItem(ctx, rt, src, req.Package)
	if err != nil {
		return nil, err
	}
	trimmedReq := strings.TrimSpace(req.Version)
	mismatch := !driver.IsLatestAlias(trimmedReq) && it.LatestVersion != "" && it.LatestVersion != trimmedReq
	if req.StrictVersion && mismatch {
		return nil, driver.ErrNotFound
	}
	ver := req.Version
	if trimmedReq == "" || driver.IsLatestAlias(trimmedReq) {
		ver = it.LatestVersion
	}
	archFmt := driver.ArchiveFormatTarGz
	if it.FileURL != "" {
		archFmt = inferWorkshopArchiveFormat(it.FileName, it.FileURL, req.StrictVersion)
	}
	var build string
	if it.Revision > 0 {
		build = "rev-" + strconv.FormatInt(it.Revision, 10)
	}
	return &driver.ResolvedRelease{
		Package:         driver.FirstNonEmpty(it.ID, req.Package),
		Version:         ver,
		Build:           build,
		DownloadURL:     it.FileURL,
		FileName:        it.FileName,
		ArchiveFormat:   archFmt,
		SizeBytes:       it.SizeBytes,
		ModIDs:          it.ModIDs,
		MapFolders:      it.MapFolders,
		Dependencies:    it.Dependencies,
		PublishedAt:     driver.ParseRFC3339Ptr(it.UpdatedAt),
		Changelog:       it.Changelog,
		UpstreamURL:     it.UpstreamURL,
		Item:            it,
		VersionMismatch: mismatch,
	}, nil
}

func inferWorkshopArchiveFormat(fileName, dlURL string, strictDefaultTar bool) string {
	low := strings.ToLower(fileName + " " + dlURL)
	switch {
	case strings.Contains(low, ".zip"):
		return driver.ArchiveFormatZip
	case strings.Contains(low, ".tmod"):
		return driver.ArchiveFormatTMod
	case strings.Contains(low, ".jar"):
		return driver.ArchiveFormatJar
	case strings.Contains(low, ".tar.gz") || strings.Contains(low, ".tgz"):
		return driver.ArchiveFormatTarGz
	case strings.Contains(low, ".vpk"):
		return driver.ArchiveFormatRaw
	default:
		if strictDefaultTar {
			return driver.ArchiveFormatTarGz
		}
		return driver.ArchiveFormatRaw
	}
}

func searchWorkshop(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	rawKey := fmt.Sprintf("%d|%s|%s|%d|%d", src.SteamAppID, strings.TrimSpace(params.Query), params.Sort, params.Page, params.Limit)
	ck := driver.CacheHashKey("steam:search", rawKey)
	var cached driver.Result
	if rt.GetCachedJSON(ctx, ck, &cached) {
		return &cached, nil
	}
	q := BuildQueryParams(rt.SteamAPIKey(), src.SteamAppID, params)
	endpoint := rt.SteamEndpoint(src) + "/IPublishedFileService/QueryFiles/v1/?" + q.Encode()
	var env queryEnvelope
	if err := rt.DoGetJSON(ctx, endpoint, &env); err != nil {
		return nil, err
	}
	items := make([]driver.Item, 0, len(env.Response.PublishedFileDetails))
	for _, pf := range env.Response.PublishedFileDetails {
		if pf.PublishedFileID == "" || strings.TrimSpace(pf.Title) == "" {
			continue
		}
		items = append(items, mapFileToItem(pf, src.SteamAppID))
	}
	out := driver.Result{Configured: true, Total: int(env.Response.Total), Items: items}
	rt.SetCachedJSON(ctx, ck, out)
	return &out, nil
}

// BuildQueryParams constructs Steam IPublishedFileService/QueryFiles query parameters.
func BuildQueryParams(apiKey string, appID int, params driver.SearchParams) url.Values {
	q := url.Values{}
	q.Set("key", apiKey)
	q.Set("appid", strconv.Itoa(appID))
	q.Set("page", strconv.Itoa(params.Page))
	q.Set("numperpage", strconv.Itoa(params.Limit))
	q.Set("return_vote_data", "true")
	q.Set("return_tags", "true")
	q.Set("return_previews", "true")
	q.Set("return_children", "true")
	q.Set("return_short_description", "true")
	q.Set("return_details", "true")
	q.Set("language", "6") // Simplified Chinese localization when provided by author
	if appID == driver.SteamAppCS2 {
		q.Set("requiredtags[0]", "Map")
	}
	if trimmed := strings.TrimSpace(params.Query); trimmed != "" {
		q.Set("search_text", trimmed)
		q.Set("query_type", "12") // RankedByTextSearch (Steam ignores search_text under query_type=9)
		return q
	}
	switch params.Sort {
	case driver.SortTrend:
		q.Set("query_type", "3") // RankedByTrend (Most Popular - 1 Week)
		q.Set("days", "7")
	case driver.SortTopRated:
		q.Set("query_type", "0") // RankedByVote (Top Rated All Time)
	case driver.SortNewest:
		q.Set("query_type", "1") // RankedByPublicationDate (Most Recent)
	case driver.SortUpdated:
		q.Set("query_type", "21") // RankedByLastUpdatedDate (Last Updated)
	default:
		q.Set("query_type", "9") // RankedByTotalUniqueSubscriptions (Most Subscribed All Time)
	}
	return q
}

func detailWorkshop(ctx context.Context, rt driver.Runtime, src driver.Source, id string) (*driver.Item, error) {
	if !driver.IsDigitsOnly(id) {
		return nil, driver.ErrInvalidRequest
	}
	ck := driver.CacheHashKey("steam:detail", fmt.Sprintf("%d|%s", src.SteamAppID, id))
	var cached driver.Item
	if rt.GetCachedJSON(ctx, ck, &cached) && cached.ID != "" {
		return &cached, nil
	}
	base := rt.SteamEndpoint(src)
	items, err := fetchFilesByIDs(ctx, rt, base, src.SteamAppID, []string{id})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, driver.ErrNotFound
	}
	item := items[0]
	hydrateDependencyTitles(ctx, rt, base, src.SteamAppID, &item)
	rt.SetCachedJSON(ctx, ck, item)
	return &item, nil
}

func resolveWorkshop(ctx context.Context, rt driver.Runtime, src driver.Source, ids []string) ([]driver.Item, error) {
	base := rt.SteamEndpoint(src)
	if len(ids) == 1 {
		if colIDs := fetchCollectionChildren(ctx, rt, base, ids[0]); len(colIDs) > 0 {
			ids = colIDs
		}
	}
	if len(ids) > 60 {
		ids = ids[:60]
	}
	ck := driver.CacheHashKey("steam:resolve", fmt.Sprintf("%d|%s", src.SteamAppID, strings.Join(ids, ",")))
	var cached []driver.Item
	if rt.GetCachedJSON(ctx, ck, &cached) && len(cached) > 0 {
		return cached, nil
	}
	items, err := fetchFilesByIDs(ctx, rt, base, src.SteamAppID, ids)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		rt.SetCachedJSON(ctx, ck, items)
	}
	return items, nil
}

// Keep unused time import check clean if time is used in client.go.
var _ = time.RFC3339
