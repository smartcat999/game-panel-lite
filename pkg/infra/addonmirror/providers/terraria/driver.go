package terraria

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const defaultEndpoint = "https://terraria.org"

func init() {
	drv := &terrariaDriver{}
	driver.Register(driver.ProviderTerraria, func() driver.Driver { return drv })
	driver.RegisterHTTPSubDriver(httpAdapter{d: drv})
}

type terrariaDriver struct {
	driver.BaseDriver
}

func (d *terrariaDriver) Name() string { return driver.ProviderTerraria }

func (d *terrariaDriver) Capabilities(driver.Source) driver.Capabilities {
	return driver.Capabilities{ServerPackages: true}
}

func (d *terrariaDriver) Match(endpoint, pkgName string) bool {
	lowEnd := strings.ToLower(strings.TrimSpace(endpoint))
	lowPkg := strings.ToLower(strings.TrimSpace(pkgName))
	return strings.Contains(lowEnd, "terraria.org") || lowPkg == "terraria-vanilla"
}

func (d *terrariaDriver) ListServerVersions(ctx context.Context, rt driver.Runtime, src driver.Source, pkgName string) ([]string, error) {
	return d.listVersions(ctx, rt, src.Endpoint, pkgName)
}

func (d *terrariaDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	return d.resolveRelease(ctx, rt, src.Endpoint, req.Package, req.Version)
}

func (d *terrariaDriver) listVersions(ctx context.Context, rt driver.Runtime, endpoint, _ string) ([]string, error) {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		base = defaultEndpoint
	}
	var names []string
	if err := rt.DoGetJSON(ctx, base+"/api/get/dedicated-servers-names", &names); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var vers []string
	for _, fname := range names {
		ver, ok := extractZipVersion(fname)
		if !ok {
			continue
		}
		if _, dup := seen[ver]; !dup {
			seen[ver] = struct{}{}
			vers = append(vers, ver)
		}
	}
	slices.SortFunc(vers, driver.CompareSemanticDesc)
	return vers, nil
}

type httpAdapter struct {
	d *terrariaDriver
}

func (a httpAdapter) Name() string               { return a.d.Name() }
func (a httpAdapter) Match(end, pkg string) bool { return a.d.Match(end, pkg) }
func (a httpAdapter) ListServerVersions(ctx context.Context, rt driver.Runtime, end, pkg string) ([]string, error) {
	return a.d.listVersions(ctx, rt, end, pkg)
}
func (a httpAdapter) ResolveRelease(ctx context.Context, rt driver.Runtime, end, pkg, ver string) (*driver.ResolvedRelease, error) {
	return a.d.resolveRelease(ctx, rt, end, pkg, ver)
}

func (d *terrariaDriver) resolveRelease(ctx context.Context, rt driver.Runtime, endpoint, pkgName, version string) (*driver.ResolvedRelease, error) {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		base = defaultEndpoint
	}
	v := strings.TrimSpace(version)
	fname := ""
	if driver.IsLatestAlias(v) {
		var names []string
		if err := rt.DoGetJSON(ctx, base+"/api/get/dedicated-servers-names", &names); err == nil {
			for _, n := range names {
				if strings.HasPrefix(n, "terraria-server-") && strings.HasSuffix(n, ".zip") && !strings.Contains(n, "mobile") {
					fname = n
					break
				}
			}
		}
		if fname == "" {
			fname = "terraria-server-1458.zip"
		}
		v = parseZipVersion(fname)
	} else {
		digits := strings.ReplaceAll(strings.TrimPrefix(v, "vanilla-"), ".", "")
		fname = fmt.Sprintf("terraria-server-%s.zip", digits)
	}
	dlURL := base + "/api/download/pc-dedicated-server/" + fname
	return &driver.ResolvedRelease{
		Package: driver.FirstNonEmpty(pkgName, "terraria"), Version: v, Build: fname,
		DownloadURL: dlURL, FileName: fname, ArchiveFormat: driver.ArchiveFormatZip, TargetPath: ".",
		UpstreamURL: "https://terraria.org",
	}, nil
}

func extractZipVersion(fname string) (string, bool) {
	trimmed := strings.TrimSpace(fname)
	if !strings.HasPrefix(trimmed, "terraria-server-") || !strings.HasSuffix(trimmed, ".zip") || strings.Contains(trimmed, "mobile") {
		return "", false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(trimmed, "terraria-server-"), ".zip")
	if idx := strings.IndexByte(raw, '-'); idx > 0 {
		raw = raw[:idx]
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	if len(raw) != 4 && len(raw) != 5 {
		return "", false
	}
	return parseZipVersion(trimmed), true
}

func parseZipVersion(fname string) string {
	s := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(fname), "terraria-server-"), ".zip")
	if idx := strings.IndexByte(s, '-'); idx > 0 {
		s = s[:idx]
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return "1.4.5.8"
		}
	}
	if len(s) == 4 {
		return fmt.Sprintf("%c.%c.%c.%c", s[0], s[1], s[2], s[3])
	}
	if len(s) == 5 {
		return fmt.Sprintf("%c.%c.%c.%s", s[0], s[1], s[2], s[3:])
	}
	return "1.4.5.8"
}
