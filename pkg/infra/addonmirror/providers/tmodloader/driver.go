package tmodloader

import (
	"context"
	"net/url"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const defaultGitHubAPI = "https://api.github.com"

func init() {
	drv := &tModLoaderDriver{}
	driver.Register(driver.ProviderTModLoader, func() driver.Driver { return drv })
	driver.RegisterHTTPSubDriver(httpAdapter{d: drv})
}

type tModLoaderDriver struct {
	driver.BaseDriver
}

func (d *tModLoaderDriver) Name() string { return driver.ProviderTModLoader }

func (d *tModLoaderDriver) Capabilities(driver.Source) driver.Capabilities {
	return driver.Capabilities{ServerPackages: true}
}

func (d *tModLoaderDriver) Match(endpoint, pkgName string) bool {
	lowEnd := strings.ToLower(strings.TrimSpace(endpoint))
	lowPkg := strings.ToLower(strings.TrimSpace(pkgName))
	return strings.Contains(lowEnd, "tmodloader") || lowPkg == "terraria-tmodloader" || lowPkg == "tmodloader"
}

func (d *tModLoaderDriver) ListServerVersions(ctx context.Context, rt driver.Runtime, src driver.Source, _ string) ([]string, error) {
	return d.listVersions(ctx, rt, src.Endpoint)
}

func (d *tModLoaderDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	return d.resolveRelease(ctx, rt, src.Endpoint, req.Package, req.Version)
}

type httpAdapter struct {
	d *tModLoaderDriver
}

func (a httpAdapter) Name() string               { return a.d.Name() }
func (a httpAdapter) Match(end, pkg string) bool { return a.d.Match(end, pkg) }
func (a httpAdapter) ListServerVersions(ctx context.Context, rt driver.Runtime, end, _ string) ([]string, error) {
	return a.d.listVersions(ctx, rt, end)
}
func (a httpAdapter) ResolveRelease(ctx context.Context, rt driver.Runtime, end, pkg, ver string) (*driver.ResolvedRelease, error) {
	return a.d.resolveRelease(ctx, rt, end, pkg, ver)
}

func (d *tModLoaderDriver) listVersions(ctx context.Context, rt driver.Runtime, endpoint string) ([]string, error) {
	base := repoBase(endpoint)
	var releases []struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := rt.DoGetJSON(ctx, base+"/releases?per_page=25", &releases); err != nil {
		return nil, err
	}
	var stable, fallback []string
	for _, r := range releases {
		tag := strings.TrimSpace(r.TagName)
		if r.Draft || tag == "" {
			continue
		}
		fallback = append(fallback, tag)
		if !r.Prerelease {
			stable = append(stable, tag)
		}
	}
	if len(stable) > 0 {
		return driver.DedupeTrimmed(stable), nil
	}
	return driver.DedupeTrimmed(fallback), nil
}

func (d *tModLoaderDriver) resolveRelease(ctx context.Context, rt driver.Runtime, endpoint, pkgName, version string) (*driver.ResolvedRelease, error) {
	base := repoBase(endpoint)
	tag := strings.TrimSpace(version)
	apiURL := base + "/releases/latest"
	if !driver.IsLatestAlias(tag) {
		if !strings.HasPrefix(tag, "v") {
			tag = "v" + tag
		}
		apiURL = base + "/releases/tags/" + url.PathEscape(tag)
	}
	var rel struct {
		TagName     string `json:"tag_name"`
		PublishedAt string `json:"published_at"`
		HTMLURL     string `json:"html_url"`
		Body        string `json:"body"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	var dlURL, dlName string
	if err := rt.DoGetJSON(ctx, apiURL, &rel); err == nil {
		if strings.TrimSpace(rel.TagName) != "" {
			tag = strings.TrimSpace(rel.TagName)
		}
		for _, a := range rel.Assets {
			if strings.EqualFold(a.Name, "tModLoader.zip") {
				dlURL, dlName = a.BrowserDownloadURL, a.Name
				break
			}
		}
	}
	if driver.IsLatestAlias(tag) {
		tag = "v2026.09.3.1"
	}
	if dlURL == "" {
		dlName = "tModLoader.zip"
		dlURL = "https://github.com/tModLoader/tModLoader/releases/download/" + tag + "/" + dlName
	}
	upURL := strings.TrimSpace(rel.HTMLURL)
	if upURL == "" {
		upURL = "https://github.com/tModLoader/tModLoader/releases/tag/" + tag
	}
	return &driver.ResolvedRelease{
		Package: driver.FirstNonEmpty(pkgName, "tmodloader"), Version: tag, Build: tag + "/" + dlName,
		DownloadURL: dlURL, FileName: dlName, ArchiveFormat: driver.ArchiveFormatZip, TargetPath: ".",
		PublishedAt: driver.ParseRFC3339Ptr(rel.PublishedAt), Changelog: strings.TrimSpace(rel.Body), UpstreamURL: upURL,
	}, nil
}

func repoBase(endpoint string) string {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	low := strings.ToLower(base)
	if base == "" || strings.Contains(low, "github.com/tmodloader") {
		return defaultGitHubAPI + "/repos/tModLoader/tModLoader"
	}
	if !strings.Contains(low, "/repos/") {
		return base + "/repos/tModLoader/tModLoader"
	}
	return base
}
