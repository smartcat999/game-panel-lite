package runtimecatalog

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

type Catalog struct {
	Providers map[domain.ProviderKey]RuntimeConfig `json:"providers"`
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
	if got, ok := catalogs[0].Providers[providerKey]; ok {
		return got.WithFallback(fallback)
	}
	return fallback
}

func (config RuntimeConfig) WithFallback(fallback RuntimeConfig) RuntimeConfig {
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

func (config RuntimeConfig) VersionList() []string {
	return append([]string{}, config.Versions...)
}

func (config RuntimeConfig) ImageFor(version string) string {
	version = strings.TrimSpace(version)
	if version == "" && len(config.Versions) > 0 {
		version = config.Versions[0]
	}
	if strings.TrimSpace(config.ImageTemplate) != "" {
		return strings.ReplaceAll(config.ImageTemplate, "{version}", version)
	}
	return config.Image
}
