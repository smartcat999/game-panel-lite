package fabricmc

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

func init() {
	driver.Register(driver.ProviderFabricMC, func() driver.Driver { return &fabricDriver{} })
}

type fabricDriver struct {
	driver.BaseDriver
}

func (d *fabricDriver) Name() string { return driver.ProviderFabricMC }

func (d *fabricDriver) Capabilities(driver.Source) driver.Capabilities {
	return driver.Capabilities{ServerPackages: true}
}

func (d *fabricDriver) ListServerVersions(ctx context.Context, rt driver.Runtime, src driver.Source, _ string) ([]string, error) {
	return listVersions(ctx, rt, src.Endpoint)
}

func (d *fabricDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	return Resolve(ctx, rt, src.Endpoint, req.Version)
}

// Resolve queries a FabricMC stable loader server profile for the target Minecraft version.
func Resolve(ctx context.Context, rt driver.Runtime, endpoint, version string) (*driver.ResolvedRelease, error) {
	base := normalizeBase(endpoint)
	resolved := strings.TrimSpace(version)
	if driver.IsLatestAlias(resolved) {
		var err error
		resolved, err = latestStableGameVersion(ctx, rt, base)
		if err != nil {
			return nil, err
		}
	}
	loaderVer, err := latestStableLoaderVersion(ctx, rt, base)
	if err != nil {
		return nil, err
	}
	pubAt, err := verifyServerProfile(ctx, rt, base, resolved, loaderVer)
	if err != nil {
		return nil, err
	}
	dlURL := fmt.Sprintf("%s/v2/versions/loader/%s/%s/server/json", base, url.PathEscape(resolved), url.PathEscape(loaderVer))
	build := fmt.Sprintf("%s+%s", resolved, loaderVer)
	fname := fmt.Sprintf("fabric-server-%s-%s.tar.gz", resolved, loaderVer)
	return &driver.ResolvedRelease{
		Package:       "fabric",
		Version:       resolved,
		Build:         build,
		DownloadURL:   dlURL,
		FileName:      fname,
		ArchiveFormat: driver.ArchiveFormatTarGz,
		TargetPath:    ".",
		GameVersions:  []string{resolved},
		Loaders:       []string{driver.ProviderFabricMC},
		PublishedAt:   pubAt,
		UpstreamURL:   "https://fabricmc.net/use/server/",
	}, nil
}

func normalizeBase(endpoint string) string {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		return driver.DefaultFabricMetaBaseURL
	}
	return base
}

type gameVersionEntry struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

func listVersions(ctx context.Context, rt driver.Runtime, endpoint string) ([]string, error) {
	var raw []gameVersionEntry
	if err := rt.DoGetJSON(ctx, normalizeBase(endpoint)+"/v2/versions/game", &raw); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(raw))
	var vers []string
	for _, g := range raw {
		clean := strings.TrimSpace(g.Version)
		if !g.Stable || clean == "" {
			continue
		}
		if _, ok := seen[clean]; !ok {
			seen[clean] = struct{}{}
			vers = append(vers, clean)
		}
	}
	slices.SortFunc(vers, driver.CompareSemanticDesc)
	return vers, nil
}

func latestStableGameVersion(ctx context.Context, rt driver.Runtime, base string) (string, error) {
	vers, err := listVersions(ctx, rt, base)
	if err != nil {
		return "", err
	}
	if len(vers) == 0 {
		return "", driver.ErrNotFound
	}
	return vers[0], nil
}

func latestStableLoaderVersion(ctx context.Context, rt driver.Runtime, base string) (string, error) {
	var loaders []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := rt.DoGetJSON(ctx, base+"/v2/versions/loader", &loaders); err != nil {
		return "", fmt.Errorf("query Fabric loader versions: %w", err)
	}
	for _, l := range loaders {
		if l.Stable && strings.TrimSpace(l.Version) != "" {
			return strings.TrimSpace(l.Version), nil
		}
	}
	if len(loaders) > 0 && strings.TrimSpace(loaders[0].Version) != "" {
		return strings.TrimSpace(loaders[0].Version), nil
	}
	return "", driver.ErrNotFound
}

func verifyServerProfile(ctx context.Context, rt driver.Runtime, base, gameVersion, loaderVersion string) (*time.Time, error) {
	var profile struct {
		ID          string `json:"id"`
		ReleaseTime string `json:"releaseTime"`
		MainClass   string `json:"mainClass"`
	}
	u := fmt.Sprintf("%s/v2/versions/loader/%s/%s/server/json", base, url.PathEscape(gameVersion), url.PathEscape(loaderVersion))
	if err := rt.DoGetJSON(ctx, u, &profile); err != nil {
		return nil, fmt.Errorf("query Fabric server profile %q/%q: %w", gameVersion, loaderVersion, err)
	}
	if strings.TrimSpace(profile.MainClass) == "" {
		return nil, fmt.Errorf("fabric profile %q/%q missing mainClass", gameVersion, loaderVersion)
	}
	return parseFabricTime(profile.ReleaseTime), nil
}

func parseFabricTime(raw string) *time.Time {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05-0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			utc := t.UTC()
			return &utc
		}
	}
	return nil
}
