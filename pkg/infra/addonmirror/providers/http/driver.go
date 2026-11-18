package http

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

func init() {
	driver.Register(driver.ProviderHTTP, func() driver.Driver { return &httpDriver{} })
}

type httpDriver struct {
	driver.BaseDriver
}

func (d *httpDriver) Name() string { return driver.ProviderHTTP }

func (d *httpDriver) Capabilities(driver.Source) driver.Capabilities {
	return driver.Capabilities{ServerPackages: true}
}

func (d *httpDriver) ListServerVersions(ctx context.Context, rt driver.Runtime, src driver.Source, pkgName string) ([]string, error) {
	if sub, ok := driver.LookupHTTPSubDriver(src.Endpoint, pkgName); ok {
		return sub.ListServerVersions(ctx, rt, src.Endpoint, pkgName)
	}
	lowEnd := strings.ToLower(strings.TrimSpace(src.Endpoint))
	switch {
	case strings.Contains(src.Endpoint, "{version}"):
		return nil, nil
	case strings.HasPrefix(lowEnd, "http://") || strings.HasPrefix(lowEnd, "https://"):
		return []string{"latest"}, nil
	default:
		return nil, nil
	}
}

func (d *httpDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	return Resolve(ctx, rt, src.Endpoint, req.Package, req.Version, req.SourceURL)
}

// Resolve resolves specialized HTTP upstreams (via registered HTTPSubDriver) or generic URL/template releases.
func Resolve(ctx context.Context, rt driver.Runtime, endpoint, pkgName, version, explicitURL string) (*driver.ResolvedRelease, error) {
	if u := strings.TrimSpace(explicitURL); u != "" {
		fname := filepath.Base(strings.Split(u, "?")[0])
		return &driver.ResolvedRelease{
			Package: pkgName, Version: version, Build: u,
			DownloadURL: u, FileName: fname, ArchiveFormat: driver.InferArchiveFormat(fname), TargetPath: ".",
		}, nil
	}
	if sub, ok := driver.LookupHTTPSubDriver(endpoint, pkgName); ok {
		return sub.ResolveRelease(ctx, rt, endpoint, pkgName, version)
	}
	lowEnd := strings.ToLower(strings.TrimSpace(endpoint))
	switch {
	case strings.Contains(endpoint, "{version}") || strings.Contains(endpoint, "{package}"):
		u := strings.ReplaceAll(endpoint, "{package}", url.PathEscape(pkgName))
		u = strings.ReplaceAll(u, "{version}", url.PathEscape(version))
		fname := filepath.Base(strings.Split(u, "?")[0])
		return &driver.ResolvedRelease{
			Package: pkgName, Version: version, Build: u,
			DownloadURL: u, FileName: fname, ArchiveFormat: driver.InferArchiveFormat(fname), TargetPath: ".",
		}, nil
	case strings.HasPrefix(lowEnd, "http://") || strings.HasPrefix(lowEnd, "https://"):
		fname := filepath.Base(strings.Split(endpoint, "?")[0])
		ver := strings.TrimSpace(version)
		if driver.IsLatestAlias(ver) {
			ver = "latest"
		}
		return &driver.ResolvedRelease{
			Package: pkgName, Version: ver, Build: endpoint,
			DownloadURL: endpoint, FileName: fname, ArchiveFormat: driver.InferArchiveFormat(fname), TargetPath: ".",
		}, nil
	default:
		return nil, fmt.Errorf("sourceURL or upstream endpoint is required for http package %s:%s", pkgName, version)
	}
}
