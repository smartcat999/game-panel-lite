package neoforge

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const officialMavenTimeout = 6 * time.Second

type neoForgeCatalog struct {
	MCVersions []string          `json:"mcVersions"`
	LatestByMC map[string]string `json:"latestByMC"`
}

func init() {
	driver.Register(driver.ProviderNeoForge, func() driver.Driver { return &neoforgeDriver{} })
}

type neoforgeDriver struct {
	driver.BaseDriver
}

func (d *neoforgeDriver) Name() string { return driver.ProviderNeoForge }

func (d *neoforgeDriver) Capabilities(driver.Source) driver.Capabilities {
	return driver.Capabilities{ServerPackages: true}
}

func (d *neoforgeDriver) ListServerVersions(ctx context.Context, rt driver.Runtime, src driver.Source, _ string) ([]string, error) {
	vers, _, err := listVersionsWithMirror(ctx, rt, src.Endpoint, driver.DefaultNeoForgeMirrorBase)
	return vers, err
}

func (d *neoforgeDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	return Resolve(ctx, rt, src.Endpoint, req.Version)
}

// Resolve resolves a NeoForge server installer release for the target Minecraft version.
func Resolve(ctx context.Context, rt driver.Runtime, endpoint, version string) (*driver.ResolvedRelease, error) {
	return resolveWithBases(ctx, rt, endpoint, version, driver.DefaultNeoForgeMirrorBase)
}

func resolveWithBases(ctx context.Context, rt driver.Runtime, endpoint, version, mirrorBase string) (*driver.ResolvedRelease, error) {
	base, isDefault := normalizeBase(endpoint)
	resolvedMC := strings.TrimSpace(version)
	usedMirror := false
	if driver.IsLatestAlias(resolvedMC) {
		vers, mUsed, err := listVersionsWithMirror(ctx, rt, endpoint, mirrorBase)
		if err != nil {
			return nil, err
		}
		if len(vers) == 0 {
			return nil, driver.ErrNotFound
		}
		resolvedMC = vers[0]
		usedMirror = mUsed
	}
	nfVer, verMirror, err := fetchLatestNeoForgeForMC(ctx, rt, base, resolvedMC, isDefault, mirrorBase)
	if err != nil {
		return nil, err
	}
	usedMirror = usedMirror || verMirror
	dlURL, fallbacks := buildInstallerURLs(base, nfVer, isDefault, usedMirror, mirrorBase)
	return &driver.ResolvedRelease{
		Package:              driver.ProviderNeoForge,
		Version:              resolvedMC,
		Build:                resolvedMC + "+" + nfVer,
		DownloadURL:          dlURL,
		FallbackDownloadURLs: fallbacks,
		FileName:             fmt.Sprintf("neoforge-server-%s-%s.tar.gz", resolvedMC, nfVer),
		ArchiveFormat:        driver.ArchiveFormatTarGz,
		TargetPath:           ".",
		GameVersions:         []string{resolvedMC},
		Loaders:              []string{driver.ProviderNeoForge},
		UpstreamURL:          "https://neoforged.net/",
	}, nil
}

func normalizeBase(endpoint string) (string, bool) {
	trimmed := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	defaultBase := strings.TrimRight(driver.DefaultNeoForgeMavenBaseURL, "/")
	if trimmed == "" || strings.EqualFold(trimmed, defaultBase) {
		return defaultBase, true
	}
	return trimmed, false
}

func listVersionsWithMirror(ctx context.Context, rt driver.Runtime, endpoint, mirrorBase string) ([]string, bool, error) {
	cat, usedMirror, err := fetchCatalogWithMirror(ctx, rt, endpoint, mirrorBase)
	if err != nil {
		return nil, false, err
	}
	return cat.MCVersions, usedMirror, nil
}

func fetchCatalogWithMirror(ctx context.Context, rt driver.Runtime, endpoint, mirrorBase string) (*neoForgeCatalog, bool, error) {
	base, isDefault := normalizeBase(endpoint)
	cat, err := fetchOfficialCatalog(ctx, rt, base, isDefault)
	if err == nil && len(cat.LatestByMC) > 0 {
		return cat, false, nil
	}
	if !isDefault || strings.TrimSpace(mirrorBase) == "" || ctx.Err() != nil {
		if err != nil {
			return nil, false, err
		}
		return nil, false, driver.ErrNotFound
	}
	mCat, mErr := fetchMirrorCatalogXML(ctx, rt, mirrorBase)
	if mErr == nil && len(mCat.LatestByMC) > 0 {
		return mCat, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return nil, false, mErr
}

func fetchOfficialCatalog(ctx context.Context, rt driver.Runtime, base string, capTimeout bool) (*neoForgeCatalog, error) {
	ck := driver.CacheHashKey("neoforge:catalog_v2", base)
	var cached neoForgeCatalog
	if rt.GetCachedJSON(ctx, ck, &cached) && len(cached.LatestByMC) > 0 {
		return &cached, nil
	}
	reqCtx := ctx
	var cancel context.CancelFunc
	if capTimeout {
		reqCtx, cancel = context.WithTimeout(ctx, officialMavenTimeout)
		defer cancel()
	}
	var payload struct {
		Versions []string `json:"versions"`
	}
	u := base + "/api/maven/versions/releases/net/neoforged/neoforge"
	if err := rt.DoGetJSON(reqCtx, u, &payload); err != nil {
		return nil, fmt.Errorf("query NeoForge releases: %w", err)
	}
	cat := buildNeoForgeCatalog(payload.Versions)
	if len(cat.LatestByMC) > 0 {
		rt.SetCachedJSON(ctx, ck, cat)
	}
	return cat, nil
}

func fetchMirrorCatalogXML(ctx context.Context, rt driver.Runtime, mirrorBase string) (*neoForgeCatalog, error) {
	ck := driver.CacheHashKey("neoforge:bmcl_catalog_v2", mirrorBase)
	var cached neoForgeCatalog
	if rt.GetCachedJSON(ctx, ck, &cached) && len(cached.LatestByMC) > 0 {
		return &cached, nil
	}
	u := strings.TrimRight(strings.TrimSpace(mirrorBase), "/") + "/maven/net/neoforged/neoforge/maven-metadata.xml"
	raw, err := rt.DoGetBytes(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("query NeoForge mirror metadata: %w", err)
	}
	var doc struct {
		Versioning struct {
			Versions []string `xml:"versions>version"`
		} `xml:"versioning"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse NeoForge mirror metadata XML: %w", err)
	}
	cat := buildNeoForgeCatalog(doc.Versioning.Versions)
	if len(cat.LatestByMC) > 0 {
		rt.SetCachedJSON(ctx, ck, cat)
	}
	return cat, nil
}

func fetchLatestNeoForgeForMC(ctx context.Context, rt driver.Runtime, base, mcVer string, isDefault bool, mirrorBase string) (string, bool, error) {
	want := strings.TrimSpace(mcVer)
	if cat, err := fetchOfficialCatalog(ctx, rt, base, isDefault); err == nil && len(cat.LatestByMC) > 0 {
		if chosen := cat.LatestByMC[want]; chosen != "" {
			return chosen, false, nil
		}
		if !isDefault {
			return "", false, driver.ErrNotFound
		}
	}
	if !isDefault || strings.TrimSpace(mirrorBase) == "" {
		return "", false, driver.ErrNotFound
	}
	vers, err := fetchBMCLNeoForgeList(ctx, rt, mirrorBase, want)
	if err == nil {
		if chosen, ok := pickLatestNeoForgeForMC(vers, want); ok {
			return chosen, true, nil
		}
		if len(vers) == 0 {
			return "", true, driver.ErrNotFound
		}
	}
	if xmlCat, xErr := fetchMirrorCatalogXML(ctx, rt, mirrorBase); xErr == nil {
		if chosen := xmlCat.LatestByMC[want]; chosen != "" {
			return chosen, true, nil
		}
	}
	if errors.Is(err, driver.ErrNotFound) {
		return "", true, driver.ErrNotFound
	}
	return "", true, fmt.Errorf("resolve NeoForge for Minecraft %s: %w", want, err)
}

func fetchBMCLNeoForgeList(ctx context.Context, rt driver.Runtime, mirrorBase, mcVer string) ([]string, error) {
	ck := driver.CacheHashKey("neoforge:bmcl_list", mirrorBase+"|"+mcVer)
	var cached []string
	if rt.GetCachedJSON(ctx, ck, &cached) && len(cached) > 0 {
		return cached, nil
	}
	u := strings.TrimRight(strings.TrimSpace(mirrorBase), "/") + "/neoforge/list/" + url.PathEscape(mcVer)
	var entries []struct {
		Version string `json:"version"`
	}
	if err := rt.DoGetJSON(ctx, u, &entries); err != nil {
		return nil, err
	}
	vers := make([]string, 0, len(entries))
	for _, e := range entries {
		if clean := strings.TrimSpace(e.Version); clean != "" {
			vers = append(vers, clean)
		}
	}
	if len(vers) > 0 {
		rt.SetCachedJSON(ctx, ck, vers)
	}
	return vers, nil
}

func buildInstallerURLs(base, nfVer string, isDefault, usedMirror bool, mirrorBase string) (string, []string) {
	escapedVer := url.PathEscape(nfVer)
	official := fmt.Sprintf("%s/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar", base, escapedVer, escapedVer)
	if !isDefault || strings.TrimSpace(mirrorBase) == "" {
		return official, nil
	}
	mirror := fmt.Sprintf("%s/maven/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
		strings.TrimRight(strings.TrimSpace(mirrorBase), "/"), escapedVer, escapedVer)
	if usedMirror {
		return mirror, []string{official}
	}
	return official, []string{mirror}
}

func buildNeoForgeCatalog(rawVers []string) *neoForgeCatalog {
	bestStable := make(map[string]string)
	bestBeta := make(map[string]string)
	for _, raw := range rawVers {
		trimmed := strings.TrimSpace(raw)
		mc, isBeta, ok := neoForgeToMCVersion(trimmed)
		if !ok {
			continue
		}
		if isBeta {
			bestBeta[mc] = pickNewerSemVer(bestBeta[mc], trimmed)
			continue
		}
		bestStable[mc] = pickNewerSemVer(bestStable[mc], trimmed)
	}
	latestByMC := make(map[string]string, len(bestBeta)+len(bestStable))
	for mc, v := range bestBeta {
		latestByMC[mc] = v
	}
	for mc, v := range bestStable {
		latestByMC[mc] = v
	}
	mcVers := make([]string, 0, len(bestStable))
	if len(bestStable) > 0 {
		for mc := range bestStable {
			mcVers = append(mcVers, mc)
		}
	} else {
		for mc := range latestByMC {
			mcVers = append(mcVers, mc)
		}
	}
	slices.SortFunc(mcVers, driver.CompareSemanticDesc)
	return &neoForgeCatalog{MCVersions: mcVers, LatestByMC: latestByMC}
}

func pickNewerSemVer(cur, candidate string) string {
	if cur == "" || driver.CompareSemanticDesc(candidate, cur) < 0 {
		return candidate
	}
	return cur
}

func pickLatestNeoForgeForMC(versions []string, targetMC string) (string, bool) {
	want := strings.TrimSpace(targetMC)
	var stableMatches, betaMatches []string
	for _, v := range versions {
		trimmed := strings.TrimSpace(v)
		mc, isBeta, ok := neoForgeToMCVersion(trimmed)
		if !ok || mc != want {
			continue
		}
		if isBeta {
			betaMatches = append(betaMatches, trimmed)
		} else {
			stableMatches = append(stableMatches, trimmed)
		}
	}
	pool := stableMatches
	if len(pool) == 0 {
		pool = betaMatches
	}
	if len(pool) == 0 {
		return "", false
	}
	slices.SortFunc(pool, driver.CompareSemanticDesc)
	return pool[0], true
}

func neoForgeToMCVersion(nfVer string) (string, bool, bool) {
	trimmed := strings.TrimSpace(nfVer)
	if trimmed == "" {
		return "", false, false
	}
	base, _, isBeta := strings.Cut(trimmed, "-")
	parts := strings.Split(base, ".")
	if len(parts) < 3 || len(parts) > 4 {
		return "", false, false
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return "", false, false
		}
		nums[i] = n
	}
	if nums[0] == 0 {
		return "", false, false
	}
	if nums[0] >= 26 && len(nums) == 4 {
		if nums[2] == 0 {
			return fmt.Sprintf("%d.%d", nums[0], nums[1]), isBeta, true
		}
		return fmt.Sprintf("%d.%d.%d", nums[0], nums[1], nums[2]), isBeta, true
	}
	if nums[0] < 26 && len(nums) == 3 {
		if nums[1] == 0 {
			return fmt.Sprintf("1.%d", nums[0]), isBeta, true
		}
		return fmt.Sprintf("1.%d.%d", nums[0], nums[1]), isBeta, true
	}
	return "", false, false
}
