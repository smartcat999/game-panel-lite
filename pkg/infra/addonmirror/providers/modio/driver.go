package modio

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const resolveConcurrency = 4

var (
	modioModURLRe = regexp.MustCompile(`(?i)mod\.io/g/[^/?#]+/m/([A-Za-z0-9._-]+)`)
	modioSlugRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

func init() {
	driver.Register(driver.ProviderModIO, func() driver.Driver { return &modIODriver{} })
}

type modIODriver struct {
	driver.BaseDriver
}

func (d *modIODriver) Name() string { return driver.ProviderModIO }

func (d *modIODriver) Capabilities(src driver.Source) driver.Capabilities {
	isCatalog := src.Format != driver.FormatServerPackage
	return driver.Capabilities{Catalog: isCatalog, LiveDetailOverlay: isCatalog}
}

func (d *modIODriver) ParseTokens(raw string) []string {
	var out []string
	add := func(tok string) {
		if tok != "" && !driver.ContainsString(out, tok) {
			out = append(out, tok)
		}
	}
	for _, m := range modioModURLRe.FindAllStringSubmatch(raw, -1) {
		add(strings.TrimSpace(m[1]))
	}
	for _, tok := range driver.SplitPlainTokens(raw, normalizeToken) {
		add(tok)
	}
	return out
}

func normalizeToken(part string) (string, bool) {
	trimmed := strings.TrimSpace(part)
	if trimmed == "" || !modioSlugRe.MatchString(trimmed) {
		return "", false
	}
	return trimmed, true
}

func (d *modIODriver) Search(ctx context.Context, rt driver.Runtime, src driver.Source, params driver.SearchParams) (*driver.Result, error) {
	key := effectiveAPIKey(rt, src)
	if key == "" {
		return &driver.Result{Configured: false, Message: unconfiguredMessage}, nil
	}
	apiBase, gameID, err := parseModIOEndpoint(src.Endpoint)
	if err != nil {
		return nil, err
	}
	limit := min(max(params.Limit, 1), 30)
	offset := max(params.Page-1, 0) * limit
	ck := driver.CacheHashKey("modio:search", fmt.Sprintf("%s|%s|%s|%s|%d|%d", apiBase, gameID, params.Sort, strings.TrimSpace(params.Query), offset, limit))
	var cached driver.Result
	if rt.GetCachedJSON(ctx, ck, &cached) {
		return &cached, nil
	}
	res, err := queryMods(ctx, rt, apiBase, gameID, key, params, offset, limit)
	if err != nil {
		return nil, err
	}
	rt.SetCachedJSON(ctx, ck, res)
	return res, nil
}

func queryMods(ctx context.Context, rt driver.Runtime, apiBase, gameID, key string, params driver.SearchParams, offset, limit int) (*driver.Result, error) {
	q := url.Values{}
	q.Set("api_key", key)
	q.Set("_limit", strconv.Itoa(limit))
	q.Set("_offset", strconv.Itoa(offset))
	q.Set("_sort", mapModIOSort(params.Sort))
	if trimmed := strings.TrimSpace(params.Query); trimmed != "" {
		q.Set("_q", trimmed)
	}
	u := fmt.Sprintf("%s/games/%s/mods?%s", apiBase, url.PathEscape(gameID), q.Encode())
	var env modsResponse
	if err := rt.DoGetJSON(ctx, u, &env); err != nil {
		return nil, err
	}
	items := make([]driver.Item, 0, len(env.Data))
	for _, m := range env.Data {
		items = append(items, buildModIOItem(m, nil, nil, false))
	}
	return &driver.Result{Configured: true, Total: env.ResultTotal, Items: items}, nil
}

func (d *modIODriver) Detail(ctx context.Context, rt driver.Runtime, src driver.Source, id, _ string) (*driver.Item, error) {
	item, _, _, err := fetchFullMod(ctx, rt, src, id)
	return item, err
}

func (d *modIODriver) Resolve(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string, _ string) ([]driver.Item, error) {
	if effectiveAPIKey(rt, src) == "" {
		return nil, driver.ErrModIOKeyMissing
	}
	return resolveBatch(ctx, rt, src, tokens), nil
}

func (d *modIODriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	if src.Format == driver.FormatServerPackage {
		return nil, driver.ErrUnsupportedSource
	}
	item, mod, files, err := fetchFullMod(ctx, rt, src, req.Package)
	if err != nil {
		return nil, err
	}
	chosen, ok := pickModIOFile(mod.Modfile, files, req.Version, req.StrictVersion)
	if !ok || strings.TrimSpace(chosen.Download.BinaryURL) == "" {
		return nil, driver.ErrNotFound
	}
	ver := driver.FirstNonEmpty(fileVersionString(chosen), "latest")
	var pubAt *time.Time
	if chosen.DateAdded > 0 {
		t := time.Unix(chosen.DateAdded, 0).UTC()
		pubAt = &t
	}
	fname := driver.FirstNonEmpty(chosen.Filename, fmt.Sprintf("%s-%s.zip", item.ID, ver))
	return &driver.ResolvedRelease{
		Package:       item.ID,
		Version:       ver,
		Build:         strconv.FormatInt(chosen.ID, 10),
		DownloadURL:   chosen.Download.BinaryURL,
		FileName:      fname,
		ArchiveFormat: driver.InferArchiveFormat(fname),
		SizeBytes:     chosen.Filesize,
		Dependencies:  item.Dependencies,
		PublishedAt:   pubAt,
		Changelog:     strings.TrimSpace(chosen.Changelog),
		UpstreamURL:   item.SourceURL,
		Item:          item,
	}, nil
}

func (d *modIODriver) ResolveLatestVersions(ctx context.Context, rt driver.Runtime, src driver.Source, pkgIDs []string) (map[string]string, error) {
	if effectiveAPIKey(rt, src) == "" {
		return map[string]string{}, nil
	}
	items := resolveBatch(ctx, rt, src, pkgIDs)
	out := make(map[string]string, len(items))
	for _, it := range items {
		if it.ID != "" && it.LatestVersion != "" {
			out[it.ID] = it.LatestVersion
		}
	}
	return out, nil
}

func fetchFullMod(ctx context.Context, rt driver.Runtime, src driver.Source, id string) (*driver.Item, *modioMod, []modioFile, error) {
	key := effectiveAPIKey(rt, src)
	if key == "" {
		return nil, nil, nil, driver.ErrModIOKeyMissing
	}
	apiBase, gameID, err := parseModIOEndpoint(src.Endpoint)
	if err != nil {
		return nil, nil, nil, err
	}
	mod, err := fetchModRecord(ctx, rt, apiBase, gameID, key, id)
	if err != nil {
		return nil, nil, nil, err
	}
	files := fetchModFiles(ctx, rt, apiBase, gameID, key, mod.ID)
	deps := fetchModDependencies(ctx, rt, apiBase, gameID, key, mod.ID)
	item := buildModIOItem(*mod, files, deps, true)
	return &item, mod, files, nil
}

func resolveBatch(ctx context.Context, rt driver.Runtime, src driver.Source, tokens []string) []driver.Item {
	outPtrs := make([]*driver.Item, len(tokens))
	sem := make(chan struct{}, resolveConcurrency)
	var wg sync.WaitGroup
	for i, tok := range tokens {
		wg.Add(1)
		go func(idx int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if it, _, _, err := fetchFullMod(ctx, rt, src, id); err == nil && it != nil {
				outPtrs[idx] = it
			}
		}(i, tok)
	}
	wg.Wait()
	var out []driver.Item
	for _, it := range outPtrs {
		if it != nil {
			out = append(out, *it)
		}
	}
	return out
}

func pickModIOFile(primary modioFile, files []modioFile, reqVer string, strict bool) (modioFile, bool) {
	if driver.IsLatestAlias(reqVer) {
		if primary.Download.BinaryURL != "" {
			return primary, true
		}
		if len(files) > 0 && files[0].Download.BinaryURL != "" {
			return files[0], true
		}
		return modioFile{}, false
	}
	trimmed := strings.TrimSpace(reqVer)
	for _, f := range append([]modioFile{primary}, files...) {
		if strings.EqualFold(fileVersionString(f), trimmed) || strconv.FormatInt(f.ID, 10) == trimmed {
			if f.Download.BinaryURL != "" {
				return f, true
			}
		}
	}
	if strict || primary.Download.BinaryURL == "" {
		return modioFile{}, false
	}
	return primary, true
}
