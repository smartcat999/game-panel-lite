package runtimecatalog

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type Catalog struct {
	ActiveRegistry string                               `json:"activeRegistry,omitempty"`
	Registries     map[string]string                    `json:"registries,omitempty"`
	Providers      map[domain.ProviderKey]RuntimeConfig `json:"providers"`
}

type RuntimeConfig struct {
	Image         string   `json:"image,omitempty"`
	ImageTemplate string   `json:"imageTemplate,omitempty"`
	Versions      []string `json:"versions,omitempty"`
}

func Load(path string) (Catalog, error) {
	if strings.TrimSpace(path) == "" {
		return Catalog{}, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Catalog{}, nil
		}
		return Catalog{}, fmt.Errorf("load provider catalog: %w", err)
	}
	var catalog Catalog
	if err := json.Unmarshal(content, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("parse provider catalog: %w", err)
	}
	return catalog, nil
}

func FromCatalog(catalogs []Catalog, providerKey domain.ProviderKey, fallback RuntimeConfig) RuntimeConfig {
	if len(catalogs) == 0 {
		return fallback
	}
	catalog := catalogs[0]
	if got, ok := catalog.Providers[providerKey]; ok {
		return got.WithFallback(fallback).WithRegistry(catalog.Registry())
	}
	return fallback
}

func (catalog Catalog) WithActiveRegistry(region string) Catalog {
	region = strings.TrimSpace(region)
	if region == "" {
		return catalog
	}
	catalog.ActiveRegistry = region
	return catalog
}

func (catalog Catalog) Registry() string {
	active := strings.TrimSpace(catalog.ActiveRegistry)
	if active == "" {
		active = "global"
	}
	if catalog.Registries == nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSpace(catalog.Registries[active]), "/")
}

func (config RuntimeConfig) WithFallback(fallback RuntimeConfig) RuntimeConfig {
	config.Versions = concreteVersions(config.Versions)
	fallback.Versions = concreteVersions(fallback.Versions)
	if strings.TrimSpace(config.Image) == "" {
		config.Image = fallback.Image
	}
	if strings.TrimSpace(config.ImageTemplate) == "" {
		config.ImageTemplate = fallback.ImageTemplate
	}
	if len(config.Versions) == 0 {
		config.Versions = fallback.Versions
	}
	return config
}

func (config RuntimeConfig) WithRegistry(registry string) RuntimeConfig {
	registry = strings.TrimSuffix(strings.TrimSpace(registry), "/")
	if registry == "" {
		return config
	}
	config.Image = strings.ReplaceAll(config.Image, "{registry}", registry)
	config.ImageTemplate = strings.ReplaceAll(config.ImageTemplate, "{registry}", registry)
	return config
}

func (config RuntimeConfig) VersionList() []string {
	return append([]string{}, concreteVersions(config.Versions)...)
}

func (config RuntimeConfig) ImageFor(version string) string {
	version = strings.TrimSpace(version)
	versions := concreteVersions(config.Versions)
	if (version == "" || strings.EqualFold(version, "latest")) && len(versions) > 0 {
		version = versions[0]
	}
	if strings.TrimSpace(config.ImageTemplate) != "" {
		return strings.ReplaceAll(config.ImageTemplate, "{version}", version)
	}
	return config.Image
}

func concreteVersions(versions []string) []string {
	out := make([]string, 0, len(versions))
	seen := map[string]bool{}
	for _, version := range versions {
		version = strings.TrimSpace(version)
		if version == "" || strings.EqualFold(version, "latest") || seen[version] {
			continue
		}
		seen[version] = true
		out = append(out, version)
	}
	return out
}
