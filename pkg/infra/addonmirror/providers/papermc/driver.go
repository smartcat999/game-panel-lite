package papermc

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const defaultEndpoint = "https://fill.papermc.io"

func init() {
	driver.Register(driver.ProviderPaperMC, func() driver.Driver { return &paperDriver{} })
}

type paperDriver struct {
	driver.BaseDriver
}

func (d *paperDriver) Name() string { return driver.ProviderPaperMC }

func (d *paperDriver) Capabilities(driver.Source) driver.Capabilities {
	return driver.Capabilities{ServerPackages: true}
}

func (d *paperDriver) ListServerVersions(ctx context.Context, rt driver.Runtime, src driver.Source, _ string) ([]string, error) {
	return listVersions(ctx, rt, src.Endpoint)
}

func (d *paperDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	return Resolve(ctx, rt, src.Endpoint, req.Version)
}

// Resolve queries a PaperMC server release version and its download artifact.
func Resolve(ctx context.Context, rt driver.Runtime, endpoint, version string) (*driver.ResolvedRelease, error) {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		base = defaultEndpoint
	}
	resolved := strings.TrimSpace(version)
	if driver.IsLatestAlias(resolved) {
		var err error
		resolved, err = latestVersion(ctx, rt, base)
		if err != nil {
			return nil, err
		}
	}
	build, fname, dlURL, pubAt, changelog, err := fetchBuild(ctx, rt, base, resolved)
	if err != nil {
		return nil, err
	}
	return &driver.ResolvedRelease{
		Package: "paper", Version: resolved, Build: build, DownloadURL: dlURL,
		FileName: fname, ArchiveFormat: driver.ArchiveFormatJar, TargetPath: fname,
		PublishedAt: pubAt, Changelog: changelog, UpstreamURL: "https://papermc.io/downloads/paper",
	}, nil
}

func listVersions(ctx context.Context, rt driver.Runtime, endpoint string) ([]string, error) {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		base = defaultEndpoint
	}
	var proj struct {
		Versions map[string][]string `json:"versions"`
	}
	if err := rt.DoGetJSON(ctx, base+"/v3/projects/paper", &proj); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var vers []string
	for _, list := range proj.Versions {
		for _, v := range list {
			clean := strings.TrimSpace(v)
			if clean == "" || strings.Contains(clean, "-") {
				continue
			}
			if _, ok := seen[clean]; !ok {
				seen[clean] = struct{}{}
				vers = append(vers, clean)
			}
		}
	}
	slices.SortFunc(vers, driver.CompareSemanticDesc)
	return vers, nil
}

func fetchBuild(ctx context.Context, rt driver.Runtime, base, version string) (string, string, string, *time.Time, string, error) {
	var meta struct {
		ID        int    `json:"id"`
		Time      string `json:"time"`
		Downloads map[string]struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"downloads"`
		Commits []driver.CommitEntry `json:"commits"`
	}
	u := fmt.Sprintf("%s/v3/projects/paper/versions/%s/builds/latest", base, url.PathEscape(version))
	if err := rt.DoGetJSON(ctx, u, &meta); err != nil {
		return "", "", "", nil, "", fmt.Errorf("query PaperMC build %q: %w", version, err)
	}
	dl, ok := meta.Downloads["server:default"]
	if !ok || dl.URL == "" {
		return "", "", "", nil, "", fmt.Errorf("PaperMC %q has no server:default download", version)
	}
	fname := filepath.Base(dl.Name)
	if fname == "" || fname == "." {
		fname = "paper-" + version + ".jar"
	}
	build := fname
	if meta.ID > 0 {
		build = strconv.Itoa(meta.ID)
	}
	return build, fname, dl.URL, driver.ParseRFC3339Ptr(meta.Time), driver.FormatCommitChangelog(meta.Commits), nil
}

func latestVersion(ctx context.Context, rt driver.Runtime, base string) (string, error) {
	var proj struct {
		Versions map[string][]string `json:"versions"`
	}
	if err := rt.DoGetJSON(ctx, base+"/v3/projects/paper", &proj); err != nil {
		return "", fmt.Errorf("query PaperMC versions: %w", err)
	}
	best := ""
	for _, list := range proj.Versions {
		for _, v := range list {
			if strings.Contains(v, "-") {
				continue
			}
			if best == "" || driver.CompareSemanticDesc(v, best) < 0 {
				best = v
			}
		}
	}
	if best == "" {
		return "", driver.ErrNotFound
	}
	return best, nil
}
