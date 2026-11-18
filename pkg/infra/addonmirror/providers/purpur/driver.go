package purpur

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const defaultEndpoint = "https://api.purpurmc.org"

func init() {
	driver.Register(driver.ProviderPurpur, func() driver.Driver { return &purpurDriver{} })
}

type purpurDriver struct {
	driver.BaseDriver
}

func (d *purpurDriver) Name() string { return driver.ProviderPurpur }

func (d *purpurDriver) Capabilities(driver.Source) driver.Capabilities {
	return driver.Capabilities{ServerPackages: true}
}

func (d *purpurDriver) ListServerVersions(ctx context.Context, rt driver.Runtime, src driver.Source, _ string) ([]string, error) {
	return listVersions(ctx, rt, src.Endpoint)
}

func (d *purpurDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	return Resolve(ctx, rt, src.Endpoint, req.Version)
}

// Resolve queries a PurpurMC server release version and its download artifact.
func Resolve(ctx context.Context, rt driver.Runtime, endpoint, version string) (*driver.ResolvedRelease, error) {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		base = defaultEndpoint
	}
	resolved := strings.TrimSpace(version)
	if driver.IsLatestAlias(resolved) {
		var proj struct {
			Versions []string `json:"versions"`
		}
		if err := rt.DoGetJSON(ctx, base+"/v2/purpur", &proj); err != nil || len(proj.Versions) == 0 {
			return nil, fmt.Errorf("query Purpur versions: %w", err)
		}
		resolved = proj.Versions[len(proj.Versions)-1]
	}
	build, pubAt, changelog, err := fetchBuild(ctx, rt, base, resolved)
	if err != nil {
		return nil, err
	}
	fname := fmt.Sprintf("purpur-%s-%s.jar", resolved, build)
	dlURL := fmt.Sprintf("%s/v2/purpur/%s/latest/download", base, url.PathEscape(resolved))
	return &driver.ResolvedRelease{
		Package: "purpur", Version: resolved, Build: build,
		DownloadURL: dlURL, FileName: fname, ArchiveFormat: driver.ArchiveFormatJar, TargetPath: fname,
		PublishedAt: pubAt, Changelog: changelog, UpstreamURL: "https://purpurmc.org/downloads",
	}, nil
}

func listVersions(ctx context.Context, rt driver.Runtime, endpoint string) ([]string, error) {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		base = defaultEndpoint
	}
	var proj struct {
		Versions []string `json:"versions"`
	}
	if err := rt.DoGetJSON(ctx, base+"/v2/purpur", &proj); err != nil {
		return nil, err
	}
	vers := driver.DedupeTrimmed(proj.Versions)
	slices.SortFunc(vers, driver.CompareSemanticDesc)
	return vers, nil
}

func fetchBuild(ctx context.Context, rt driver.Runtime, base, version string) (string, *time.Time, string, error) {
	var bMeta struct {
		Build     string               `json:"build"`
		Timestamp int64                `json:"timestamp"`
		Commits   []driver.CommitEntry `json:"commits"`
	}
	if err := rt.DoGetJSON(ctx, base+"/v2/purpur/"+url.PathEscape(version)+"/latest", &bMeta); err == nil && strings.TrimSpace(bMeta.Build) != "" {
		return strings.TrimSpace(bMeta.Build), driver.ParseMillisPtr(bMeta.Timestamp), driver.FormatCommitChangelog(bMeta.Commits), nil
	}
	var verMeta struct {
		Builds struct {
			Latest string `json:"latest"`
		} `json:"builds"`
	}
	if err := rt.DoGetJSON(ctx, base+"/v2/purpur/"+url.PathEscape(version), &verMeta); err != nil {
		return "", nil, "", fmt.Errorf("query Purpur %q: %w", version, err)
	}
	return verMeta.Builds.Latest, nil, "", nil
}
