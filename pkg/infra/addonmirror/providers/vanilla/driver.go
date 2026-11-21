package vanilla

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

var mojangOfficialOrigins = []string{
	"https://piston-meta.mojang.com",
	"https://launchermeta.mojang.com",
}

type mojangManifest struct {
	Latest struct {
		Release string `json:"release"`
	} `json:"latest"`
	Versions []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"versions"`
}

type mojangVersionMeta struct {
	ReleaseTime string `json:"releaseTime"`
	Downloads   struct {
		Server struct {
			URL  string `json:"url"`
			SHA1 string `json:"sha1"`
		} `json:"server"`
	} `json:"downloads"`
}

func init() {
	driver.Register(driver.ProviderVanilla, func() driver.Driver { return &vanillaDriver{} })
}

type vanillaDriver struct {
	driver.BaseDriver
}

func (d *vanillaDriver) Name() string { return driver.ProviderVanilla }

func (d *vanillaDriver) Capabilities(driver.Source) driver.Capabilities {
	return driver.Capabilities{ServerPackages: true}
}

func (d *vanillaDriver) ListServerVersions(ctx context.Context, rt driver.Runtime, src driver.Source, _ string) ([]string, error) {
	return listVersionsWithMirror(ctx, rt, src.Endpoint, driver.DefaultMojangManifestURL, driver.DefaultMojangMirrorBase)
}

func (d *vanillaDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	return Resolve(ctx, rt, src.Endpoint, req.Version)
}

// Resolve resolves an official Mojang Minecraft server jar via piston-meta, falling back to mirror bases when endpoint is empty.
func Resolve(ctx context.Context, rt driver.Runtime, endpoint, version string) (*driver.ResolvedRelease, error) {
	return resolveWithBases(ctx, rt, endpoint, version, driver.DefaultMojangManifestURL, driver.DefaultMojangMirrorBase, driver.DefaultFastMirrorBase)
}

func resolveWithBases(ctx context.Context, rt driver.Runtime, endpoint, version, primaryManifestURL, mirrorBase, fastMirrorBase string) (*driver.ResolvedRelease, error) {
	manifest, usedMirror, customOrigin, err := fetchManifest(ctx, rt, endpoint, primaryManifestURL, mirrorBase)
	if err != nil {
		return nil, err
	}
	resolved, metaURL, err := lookupManifestVersion(manifest, version)
	if err != nil {
		return nil, err
	}
	verMeta, metaUsedMirror, err := fetchVersionMeta(ctx, rt, metaURL, usedMirror, mirrorBase, customOrigin)
	if err != nil {
		return nil, fmt.Errorf("query Mojang server %q: %w", resolved, err)
	}
	downloadURL, fallbacks := buildServerDownloadCandidates(resolved, verMeta.Downloads.Server.URL, endpoint != "", metaUsedMirror || usedMirror, mirrorBase, fastMirrorBase)
	if downloadURL == "" {
		return nil, fmt.Errorf("query Mojang server %q: empty download URL", resolved)
	}
	fname := "minecraft_server." + resolved + ".jar"
	return &driver.ResolvedRelease{
		Package:              "vanilla",
		Version:              resolved,
		Build:                verMeta.Downloads.Server.SHA1,
		DownloadURL:          downloadURL,
		FallbackDownloadURLs: fallbacks,
		FileName:             fname,
		ArchiveFormat:        driver.ArchiveFormatJar,
		TargetPath:           fname,
		PublishedAt:          driver.ParseRFC3339Ptr(verMeta.ReleaseTime),
		UpstreamURL:          "https://www.minecraft.net/en-us/download/server",
	}, nil
}

func listVersionsWithMirror(ctx context.Context, rt driver.Runtime, endpoint, primaryManifestURL, mirrorBase string) ([]string, error) {
	manifest, _, _, err := fetchManifest(ctx, rt, endpoint, primaryManifestURL, mirrorBase)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var vers []string
	for _, v := range manifest.Versions {
		clean := strings.TrimSpace(v.ID)
		if v.Type != "release" || clean == "" {
			continue
		}
		if _, ok := seen[clean]; !ok {
			seen[clean] = struct{}{}
			vers = append(vers, clean)
		}
	}
	return vers, nil
}

func fetchManifest(ctx context.Context, rt driver.Runtime, endpoint, primaryManifestURL, mirrorBase string) (*mojangManifest, bool, string, error) {
	explicit := strings.TrimSpace(endpoint)
	if explicit != "" {
		var m mojangManifest
		if err := rt.DoGetJSON(ctx, explicit, &m); err != nil {
			return nil, false, "", fmt.Errorf("query Mojang manifest: %w", err)
		}
		customOrigin := nonOfficialOrigin(explicit)
		return &m, false, customOrigin, nil
	}
	var m mojangManifest
	err := rt.DoGetJSON(ctx, primaryManifestURL, &m)
	if err == nil {
		return &m, false, "", nil
	}
	if mirrorBase != "" && !errors.Is(err, driver.ErrNotFound) && ctx.Err() == nil {
		mirrorManifest := strings.TrimRight(mirrorBase, "/") + "/mc/game/version_manifest_v2.json"
		var mirrorM mojangManifest
		if mErr := rt.DoGetJSON(ctx, mirrorManifest, &mirrorM); mErr == nil {
			return &mirrorM, true, "", nil
		}
	}
	return nil, false, "", fmt.Errorf("query Mojang manifest: %w", err)
}

func lookupManifestVersion(manifest *mojangManifest, version string) (string, string, error) {
	resolved := strings.TrimSpace(version)
	if driver.IsLatestAlias(resolved) {
		resolved = manifest.Latest.Release
	}
	for _, v := range manifest.Versions {
		if v.ID == resolved {
			return resolved, v.URL, nil
		}
	}
	return "", "", driver.ErrNotFound
}

func fetchVersionMeta(ctx context.Context, rt driver.Runtime, metaURL string, usedMirror bool, mirrorBase, customOrigin string) (*mojangVersionMeta, bool, error) {
	if customOrigin != "" {
		var meta mojangVersionMeta
		target := rewriteMojangOrigin(metaURL, customOrigin)
		if err := rt.DoGetJSON(ctx, target, &meta); err != nil {
			return nil, false, err
		}
		if meta.Downloads.Server.URL == "" {
			return nil, false, errors.New("missing server download URL")
		}
		return &meta, false, nil
	}
	primary, secondary, primaryMirror := metaCandidateOrder(metaURL, usedMirror, mirrorBase)
	var meta mojangVersionMeta
	err := rt.DoGetJSON(ctx, primary, &meta)
	if err == nil && meta.Downloads.Server.URL != "" {
		return &meta, primaryMirror, nil
	}
	if err == nil {
		err = errors.New("missing server download URL")
	}
	if secondary != "" && secondary != primary && !errors.Is(err, driver.ErrNotFound) && ctx.Err() == nil {
		var secMeta mojangVersionMeta
		if sErr := rt.DoGetJSON(ctx, secondary, &secMeta); sErr == nil && secMeta.Downloads.Server.URL != "" {
			return &secMeta, !primaryMirror, nil
		}
	}
	return nil, primaryMirror, err
}

func metaCandidateOrder(metaURL string, usedMirror bool, mirrorBase string) (string, string, bool) {
	mirrorMeta := rewriteMojangOrigin(metaURL, mirrorBase)
	if usedMirror && mirrorMeta != "" && mirrorMeta != metaURL {
		return mirrorMeta, metaURL, true
	}
	if mirrorMeta != "" && mirrorMeta != metaURL {
		return metaURL, mirrorMeta, false
	}
	return metaURL, "", usedMirror
}

func buildServerDownloadCandidates(version, officialURL string, hasExplicitEndpoint, usedMirror bool, mirrorBase, fastMirrorBase string) (string, []string) {
	official := strings.TrimSpace(officialURL)
	if hasExplicitEndpoint {
		return official, nil
	}
	var fastJar, bmclJar string
	if fb := strings.TrimRight(strings.TrimSpace(fastMirrorBase), "/"); fb != "" {
		fastJar = fb + "/download/Vanilla/" + url.PathEscape(version) + "/release"
	}
	if mb := strings.TrimRight(strings.TrimSpace(mirrorBase), "/"); mb != "" {
		bmclJar = mb + "/version/" + url.PathEscape(version) + "/server"
	}
	rawOrder := []string{official, fastJar, bmclJar}
	if usedMirror {
		rawOrder = []string{fastJar, bmclJar, official}
	}
	seen := make(map[string]struct{}, len(rawOrder))
	candidates := make([]string, 0, len(rawOrder))
	for _, raw := range rawOrder {
		if raw == "" {
			continue
		}
		if _, ok := seen[raw]; !ok {
			seen[raw] = struct{}{}
			candidates = append(candidates, raw)
		}
	}
	if len(candidates) == 0 {
		return "", nil
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return candidates[0], candidates[1:]
}

func nonOfficialOrigin(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	origin := u.Scheme + "://" + u.Host
	for _, official := range mojangOfficialOrigins {
		if strings.EqualFold(origin, official) {
			return ""
		}
	}
	return origin
}

func rewriteMojangOrigin(rawURL, targetOrigin string) string {
	trimmedTarget := strings.TrimRight(strings.TrimSpace(targetOrigin), "/")
	if trimmedTarget == "" {
		return rawURL
	}
	for _, official := range mojangOfficialOrigins {
		if strings.HasPrefix(strings.ToLower(rawURL), official) {
			return trimmedTarget + rawURL[len(official):]
		}
	}
	return rawURL
}
