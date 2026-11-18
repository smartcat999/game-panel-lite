package projectzomboid

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
)

//go:embed data/options.json
var metadataJSON []byte

type option struct {
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Default   any      `json:"defaultValue"`
	Min       *float64 `json:"min"`
	Max       *float64 `json:"max"`
	MaxLength *int     `json:"maxLength"`
	Label     string   `json:"label"`
	LabelEn   string   `json:"labelEn"`
	Help      string   `json:"help"`
	HelpEn    string   `json:"helpEn"`
	Page      string   `json:"page"`
	Visible   bool     `json:"visible"`
	Generated bool     `json:"generated"`
	Secret    bool     `json:"secret"`
	Choices   []choice `json:"choices"`
}
type choice struct {
	Value   any    `json:"value"`
	Label   string `json:"label"`
	LabelEn string `json:"labelEn"`
}
type preset struct {
	ID     string         `json:"id"`
	Values map[string]any `json:"values"`
}
type region struct {
	Name string `json:"name"`
	File string `json:"file"`
}
type page struct {
	ID             string   `json:"id"`
	Fields         []string `json:"fields"`
	DefaultRegions []region `json:"defaultRegions"`
}
type metadata struct {
	Source struct {
		Status string `json:"status"`
	} `json:"source"`
	Server  []option `json:"server"`
	Sandbox []option `json:"sandbox"`
	Presets []preset `json:"presets"`
	Pages   struct {
		Server  []page `json:"server"`
		Sandbox []page `json:"sandbox"`
	} `json:"pages"`
}

var definitions = loadMetadata()

func loadMetadata() metadata {
	var data metadata
	if err := json.Unmarshal(metadataJSON, &data); err != nil {
		panic(err)
	}
	if data.Source.Status != "verified" || len(data.Presets) != 5 {
		panic("Project Zomboid official presets have not been verified")
	}
	return data
}

func (Provider) ConfigSchema() []domain.ProviderConfigField {
	fields := []domain.ProviderConfigField{{Name: "adminPassword", Label: "初始管理员密码", LabelEn: "Initial admin password", Type: "password", Default: "", Group: "pz.server.Details"}}
	for _, scope := range []struct {
		name    string
		options []option
		pages   []page
	}{{"server", definitions.Server, definitions.Pages.Server}, {"sandbox", definitions.Sandbox, definitions.Pages.Sandbox}} {
		for _, group := range scope.pages {
			for _, name := range group.Fields {
				for _, item := range scope.options {
					if item.Name != name || !item.Visible {
						continue
					}
					f := domain.ProviderConfigField{Name: scope.name + "." + name, Label: item.Label, LabelEn: item.LabelEn, Type: "text", Default: item.Default, Help: item.Help, HelpEn: item.HelpEn, Min: item.Min, Max: item.Max, Group: "pz." + scope.name + "." + group.ID}
					switch item.Type {
					case "boolean":
						f.Type = "boolean"
					case "integer", "double":
						f.Type, f.Required = "number", true
						step := 1.0
						if item.Type == "double" {
							step = 0.01
						}
						f.Step = &step
					case "enum":
						f.Type, f.Required = "select", true
						for _, c := range item.Choices {
							f.Options = append(f.Options, domain.ProviderConfigFieldOption{Value: fmt.Sprint(c.Value), Label: c.Label, LabelEn: c.LabelEn})
						}
					}
					if item.Secret {
						f.Type = "password"
					}
					if scope.name == "server" && name == "UPnP" {
						f.Default = false
					}
					fields = append(fields, f)
				}
			}
		}
	}
	return fields
}

// The native constructor loads Apocalypse before setDefaultsToCurrentValues.
// These are the verified constructor defaults, not a guessed UI preset.
func nativeDefaults() map[string]any {
	server, sandbox := map[string]any{}, map[string]any{}
	for _, o := range definitions.Server {
		if !o.Generated {
			server[o.Name] = o.Default
		}
	}
	for _, o := range definitions.Sandbox {
		setPath(sandbox, o.Name, o.Default)
	}
	// The panel owns published ports; never request automatic host-router mappings.
	server["UPnP"] = false
	return map[string]any{"server": server, "sandbox": sandbox}
}

func setPath(root map[string]any, path string, value any) {
	parts := splitPath(path)
	for _, part := range parts[:len(parts)-1] {
		next, ok := root[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			root[part] = next
		}
		root = next
	}
	root[parts[len(parts)-1]] = value
}
func getPath(root map[string]any, path string) (any, bool) {
	parts := splitPath(path)
	for _, key := range parts[:len(parts)-1] {
		next, ok := root[key].(map[string]any)
		if !ok {
			return nil, false
		}
		root = next
	}
	v, ok := root[parts[len(parts)-1]]
	return v, ok
}
func number(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		v, err := strconv.ParseFloat(string(n), 64)
		return v, err == nil
	default:
		return 0, false
	}
}
