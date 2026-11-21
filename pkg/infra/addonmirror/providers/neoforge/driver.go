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

const (
	officialMavenTimeout = 6 * time.Second
	maxRawCacheVersions  = 300
)

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
	base, isDefault := normalizeBase(endpoint)
	rawVers, usedMirror, err := fetchAllNeoForgeVersions(ctx, rt, base, isDefault, mirrorBase)
	if err != nil {
		return nil, false, err
	}
	return mapStableMCVersions(rawVers), usedMirror, nil
}

func fetchAllNeoForgeVersions(ctx context.Context, rt driver.Runtime, base string, isDefault bool, mirrorBase string) ([]string, bool, error) {
	vers, err := fetchOfficialMavenVersions(ctx, rt, base, isDefault)
	if err == nil && len(vers) > 0 {
		return vers, false, nil
	}
	if !isDefault || strings.TrimSpace(mirrorBase) == "" || ctx.Err() != nil {
		if err != nil {
			return nil, false, err
		}
		return nil, false, driver.ErrNotFound
	}
	mVers, mErr := fetchMirrorMavenXML(ctx, rt, mirrorBase)
	if mErr == nil && len(mVers) > 0 {
		return mVers, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return nil, false, mErr
}

func fetchOfficialMavenVersions(ctx context.Context, rt driver.Runtime, base string, capTimeout bool) ([]string, error) {
	ck := driver.CacheHashKey("neoforge:maven_versions", base)
	var cached []string
	if rt.GetCachedJSON(ctx, ck, &cached) && len(cached) > 0 {
		return cached, nil
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
	vers := trimRecentVersions(payload.Versions)
	if len(vers) > 0 {
		rt.SetCachedJSON(ctx, ck, vers)
	}
	return vers, nil
}

func fetchMirrorMavenXML(ctx context.Context, rt driver.Runtime, mirrorBase string) ([]string, error) {
	ck := driver.CacheHashKey("neoforge:bmcl_xml", mirrorBase)
	var cached []string
	if rt.GetCachedJSON(ctx, ck, &cached) && len(cached) > 0 {
		return cached, nil
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
	vers := trimRecentVersions(doc.Versioning.Versions)
	if len(vers) > 0 {
		rt.SetCachedJSON(ctx, ck, vers)
	}
	return vers, nil
}

func trimRecentVersions(vers []string) []string {
	if len(vers) <= maxRawCacheVersions {
		return vers
	}
	return vers[len(vers)-maxRawCacheVersions:]
}

func fetchLatestNeoForgeForMC(ctx context.Context, rt driver.Runtime, base, mcVer string, isDefault bool, mirrorBase string) (string, bool, error) {
	if vers, err := fetchOfficialMavenVersions(ctx, rt, base, isDefault); err == nil && len(vers) > 0 {
		if chosen, ok := pickLatestNeoForgeForMC(vers, mcVer); ok {
			return chosen, false, nil
		}
		if !isDefault {
			return "", false, driver.ErrNotFound
		}
	}
	if !isDefault || strings.TrimSpace(mirrorBase) == "" {
		return "", false, driver.ErrNotFound
	}
	vers, err := fetchBMCLNeoForgeList(ctx, rt, mirrorBase, mcVer)
	if err == nil {
		if chosen, ok := pickLatestNeoForgeForMC(vers, mcVer); ok {
			return chosen, true, nil
		}
		if len(vers) == 0 {
			return "", true, driver.ErrNotFound
		}
	}
	if xmlVers, xErr := fetchMirrorMavenXML(ctx, rt, mirrorBase); xErr == nil {
		if chosen, ok := pickLatestNeoForgeForMC(xmlVers, mcVer); ok {
			return chosen, true, nil
		}
	}
	if errors.Is(err, driver.ErrNotFound) {
		return "", true, driver.ErrNotFound
	}
	return "", true, fmt.Errorf("resolve NeoForge for Minecraft %s: %w", mcVer, err)
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

func mapStableMCVersions(rawVers []string) []string {
	stableMC := make(map[string]struct{})
	allMC := make(map[string]struct{})
	for _, raw := range rawVers {
		mc, isBeta, ok := neoForgeToMCVersion(raw)
		if !ok {
			continue
		}
		allMC[mc] = struct{}{}
		if !isBeta {
			stableMC[mc] = struct{}{}
		}
	}
	source := stableMC
	if len(source) == 0 {
		source = allMC
	}
	out := make([]string, 0, len(source))
	for mc := range source {
		out = append(out, mc)
	}
	slices.SortFunc(out, driver.CompareSemanticDesc)
	return out
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
