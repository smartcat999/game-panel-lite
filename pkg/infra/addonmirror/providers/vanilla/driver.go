package vanilla

import (
	"context"
	"fmt"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const defaultEndpoint = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

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
	return listVersions(ctx, rt, src.Endpoint)
}

func (d *vanillaDriver) ResolveRelease(ctx context.Context, rt driver.Runtime, src driver.Source, req driver.ResolveReleaseRequest) (*driver.ResolvedRelease, error) {
	return Resolve(ctx, rt, src.Endpoint, req.Version)
}

// Resolve resolves an official Mojang Minecraft server jar via piston-meta.
func Resolve(ctx context.Context, rt driver.Runtime, endpoint, version string) (*driver.ResolvedRelease, error) {
	resolved, metaURL, err := lookupVersionURL(ctx, rt, endpoint, version)
	if err != nil {
		return nil, err
	}
	var verMeta struct {
		ReleaseTime string `json:"releaseTime"`
		Downloads   struct {
			Server struct {
				URL  string `json:"url"`
				SHA1 string `json:"sha1"`
			} `json:"server"`
		} `json:"downloads"`
	}
	if err := rt.DoGetJSON(ctx, metaURL, &verMeta); err != nil || verMeta.Downloads.Server.URL == "" {
		return nil, fmt.Errorf("query Mojang server %q: %w", resolved, err)
	}
	fname := "minecraft_server." + resolved + ".jar"
	return &driver.ResolvedRelease{
		Package: "vanilla", Version: resolved, Build: verMeta.Downloads.Server.SHA1,
		DownloadURL: verMeta.Downloads.Server.URL, FileName: fname, ArchiveFormat: driver.ArchiveFormatJar, TargetPath: fname,
		PublishedAt: driver.ParseRFC3339Ptr(verMeta.ReleaseTime),
		UpstreamURL: "https://www.minecraft.net/en-us/download/server",
	}, nil
}

func listVersions(ctx context.Context, rt driver.Runtime, endpoint string) ([]string, error) {
	manifestURL := strings.TrimSpace(endpoint)
	if manifestURL == "" {
		manifestURL = defaultEndpoint
	}
	var manifest struct {
		Versions []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"versions"`
	}
	if err := rt.DoGetJSON(ctx, manifestURL, &manifest); err != nil {
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

func lookupVersionURL(ctx context.Context, rt driver.Runtime, endpoint, version string) (string, string, error) {
	manifestURL := strings.TrimSpace(endpoint)
	if manifestURL == "" {
		manifestURL = defaultEndpoint
	}
	var manifest struct {
		Latest struct {
			Release string `json:"release"`
		} `json:"latest"`
		Versions []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"versions"`
	}
	if err := rt.DoGetJSON(ctx, manifestURL, &manifest); err != nil {
		return "", "", fmt.Errorf("query Mojang manifest: %w", err)
	}
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
