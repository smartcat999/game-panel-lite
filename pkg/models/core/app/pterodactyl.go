package app

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

// PterodactylEgg represents the industry-standard Pterodactyl / Pelican Egg JSON schema (PTDL_v1 / PTDL_v2).
type PterodactylEgg struct {
	Meta struct {
		Version string `json:"version"`
	} `json:"meta"`
	Name         string            `json:"name"`
	Author       string            `json:"author"`
	Description  string            `json:"description"`
	Startup      string            `json:"startup"`
	Image        string            `json:"image"`
	Images       []string          `json:"images"`
	DockerImages map[string]string `json:"docker_images"`
	Variables    []struct {
		Name         string `json:"name"`
		Description  string `json:"description"`
		EnvVariable  string `json:"env_variable"`
		DefaultValue string `json:"default_value"`
		UserViewable bool   `json:"user_viewable"`
		UserEditable bool   `json:"user_editable"`
		Rules        string `json:"rules"`
		FieldType    string `json:"field_type"`
	} `json:"variables"`
}

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

// IsPterodactylEgg detects whether a raw JSON payload is a Pterodactyl / Pelican Egg definition.
func IsPterodactylEgg(raw []byte) bool {
	var probe struct {
		Meta struct {
			Version string `json:"version"`
		} `json:"meta"`
		DockerImages map[string]string `json:"docker_images"`
		Variables    []any             `json:"variables"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return strings.HasPrefix(probe.Meta.Version, "PTDL_") || (len(probe.DockerImages) > 0 && len(probe.Variables) > 0)
}

// ConvertPterodactylEgg converts a standard Pterodactyl / Pelican Egg JSON into a declarative corev1.App.
func ConvertPterodactylEgg(raw []byte) (*corev1.App, error) {
	var egg PterodactylEgg
	if err := json.Unmarshal(raw, &egg); err != nil {
		return nil, fmt.Errorf("parse pterodactyl egg json: %w", err)
	}
	if strings.TrimSpace(egg.Name) == "" {
		return nil, fmt.Errorf("pterodactyl egg name is required")
	}
	slug := normalizeEggSlug(egg.Name)
	primaryImage, versions := extractEggImagesAndVersions(egg)
	env, schema := extractEggEnvAndSchema(egg)

	appObj := &corev1.App{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.GroupVersion, Kind: "App"},
		Metadata: metav1.ObjectMeta{
			Name:        slug,
			Labels:      map[string]string{corev1.LabelAppCategory: "survival", corev1.LabelAppSource: "pterodactyl-egg"},
			Annotations: map[string]string{corev1.AnnotationEggVersion: egg.Meta.Version, corev1.AnnotationEggAuthor: egg.Author},
		},
		DisplayName:        egg.Name,
		Tagline:            "Pterodactyl Egg 导入模板",
		Summary:            egg.Description,
		Category:           "survival",
		CoverImage:         "/landing/games/default-cover.jpg",
		Image:              primaryImage,
		Versions:           versions,
		RecommendedVersion: firstOrEmpty(versions),
		RecommendedPlan:    "standard",
		Resources:          corev1.ResourceList{corev1.ResourceCPU: "2", corev1.ResourceMemory: "4Gi", corev1.ResourceStorage: "20Gi"},
		Ports:              []corev1.InstancePort{{Name: "game", Protocol: corev1.ProtocolTCP, TargetPort: 25565, PublishedPort: 0}},
		Env:                env,
		ConfigSchema:       schema,
		Capabilities:       []string{"console", "files", "backups"},
		Published:          true,
	}
	appObj.DefaultMeta()
	return appObj, nil
}

func normalizeEggSlug(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = strings.Trim(slugRe.ReplaceAllString(slug, "-"), "-")
	if len(slug) < 3 {
		slug = "egg-" + slug
	}
	return slug
}

func extractEggImagesAndVersions(egg PterodactylEgg) (string, []string) {
	var primaryImage string
	var versions []string
	if len(egg.DockerImages) > 0 {
		keys := make([]string, 0, len(egg.DockerImages))
		for k := range egg.DockerImages {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			img := strings.TrimSpace(egg.DockerImages[k])
			if img == "" {
				continue
			}
			if primaryImage == "" {
				primaryImage = img
			}
			versions = append(versions, k)
		}
	}
	if primaryImage == "" && len(egg.Images) > 0 {
		primaryImage = strings.TrimSpace(egg.Images[0])
	}
	if primaryImage == "" {
		primaryImage = strings.TrimSpace(egg.Image)
	}
	if primaryImage == "" {
		primaryImage = corev1.DefaultAppCustomImage
	}
	return primaryImage, versions
}

func extractEggEnvAndSchema(egg PterodactylEgg) (map[string]string, []corev1.AppConfigField) {
	env := make(map[string]string)
	var schema []corev1.AppConfigField
	for _, v := range egg.Variables {
		envKey := strings.TrimSpace(v.EnvVariable)
		if envKey == "" {
			continue
		}
		if v.DefaultValue != "" {
			env[envKey] = v.DefaultValue
		}
		if v.UserViewable {
			schema = append(schema, parsePterodactylVariable(v.Name, v.Description, envKey, v.DefaultValue, v.Rules, v.FieldType))
		}
	}
	return env, schema
}

func firstOrEmpty(list []string) string {
	if len(list) > 0 {
		return list[0]
	}
	return "latest"
}

func parsePterodactylVariable(name, desc, envKey, defVal, rules, fieldType string) corev1.AppConfigField {
	f := corev1.AppConfigField{
		Name:    strings.ToLower(envKey),
		Label:   name,
		Type:    "text",
		Group:   "基础设置",
		Default: defVal,
		Help:    desc,
		EnvKey:  envKey,
	}
	if strings.Contains(strings.ToLower(envKey), "password") || strings.Contains(strings.ToLower(envKey), "token") {
		f.Type = "password"
	}

	parts := strings.Split(rules, "|")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch {
		case part == "required":
			f.Required = true
		case part == "boolean" || part == "bool":
			f.Type = "boolean"
		case part == "numeric" || part == "integer" || part == "int":
			f.Type = "number"
		case strings.HasPrefix(part, "between:"):
			bounds := strings.Split(strings.TrimPrefix(part, "between:"), ",")
			if len(bounds) == 2 {
				if minV, err := strconv.ParseFloat(bounds[0], 64); err == nil {
					f.Min = &minV
				}
				if maxV, err := strconv.ParseFloat(bounds[1], 64); err == nil {
					f.Max = &maxV
				}
			}
		case strings.HasPrefix(part, "min:") && f.Type == "number":
			if minV, err := strconv.ParseFloat(strings.TrimPrefix(part, "min:"), 64); err == nil {
				f.Min = &minV
			}
		case strings.HasPrefix(part, "max:") && f.Type == "number":
			if maxV, err := strconv.ParseFloat(strings.TrimPrefix(part, "max:"), 64); err == nil {
				f.Max = &maxV
			}
		case strings.HasPrefix(part, "in:"):
			opts := strings.Split(strings.TrimPrefix(part, "in:"), ",")
			if len(opts) > 0 {
				f.Type = "select"
				for _, opt := range opts {
					opt = strings.TrimSpace(opt)
					if opt != "" {
						f.Options = append(f.Options, corev1.AppConfigOption{
							Value: opt,
							Label: opt,
						})
					}
				}
			}
		}
	}
	return f
}
