package projectzomboid

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/provider/runtimecatalog"
)

const DefaultInternalPort = 16261
const UDPPort = 16262
const ServerIdentity = "gamepanel"

type Provider struct{ runtime runtimecatalog.RuntimeConfig }

func runtimeConfig() runtimecatalog.RuntimeConfig {
	return runtimecatalog.RuntimeConfig{ImageTemplate: "smartcat99999/project-zomboid:{version}", Versions: []string{"42.21"}}
}
func NewProvider(catalog ...runtimecatalog.Catalog) Provider {
	return Provider{runtime: runtimecatalog.FromCatalog(catalog, domain.ProviderProjectZomboid, runtimeConfig())}
}
func (Provider) Key() domain.ProviderKey { return domain.ProviderProjectZomboid }
func (Provider) GameKey() domain.GameKey { return domain.GameProjectZomboid }
func (Provider) Name() string            { return "Project Zomboid" }
func (Provider) Description() string     { return "Vanilla Project Zomboid 42.21 dedicated server." }
func (Provider) Capabilities() domain.ProviderCapabilities {
	return domain.ProviderCapabilities{Backups: true, BackupRequiresStop: true, Versions: true}
}
func (p Provider) Versions() []string { return p.runtime.WithFallback(runtimeConfig()).VersionList() }
func (p Provider) Image() string      { return p.ImageFor(p.Versions()[0]) }
func (p Provider) ImageFor(version string) string {
	return p.runtime.WithFallback(runtimeConfig()).ImageFor(version)
}
func (Provider) SaveDisplayName() string                 { return "world" }
func (Provider) BackupSubtree(domain.GameServer) string  { return "Zomboid" }
func (Provider) RestoreSubtree(domain.GameServer) string { return "Zomboid" }

func splitPath(path string) []string { return strings.Split(path, ".") }
func clone(input map[string]any) map[string]any {
	data, _ := json.Marshal(input)
	out := map[string]any{}
	_ = json.Unmarshal(data, &out)
	return out
}
func randomResetID() float64 {
	var bytes [4]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return float64(binary.BigEndian.Uint32(bytes[:])&0x7ffffffe) + 1
}

func (Provider) DefaultConfigPayload() map[string]any {
	out := nativeDefaults()
	server := out["server"].(map[string]any)
	server["ResetID"], server["ServerPlayerID"] = randomResetID(), rand.Text()
	out["adminPassword"] = rand.Text()
	return out
}

func (Provider) NormalizeConfigPayload(input map[string]any) (map[string]any, error) {
	out := nativeDefaults()
	for key := range input {
		if key != "server" && key != "sandbox" && key != "adminPassword" && key != "preset" {
			return nil, fmt.Errorf("unknown Project Zomboid config field: %s", key)
		}
	}
	for _, scope := range []struct {
		name    string
		options []option
	}{{"server", definitions.Server}, {"sandbox", definitions.Sandbox}} {
		provided, ok := input[scope.name].(map[string]any)
		if input[scope.name] != nil && !ok {
			return nil, fmt.Errorf("%s must be an object", scope.name)
		}
		allowed := map[string]bool{}
		for _, item := range scope.options {
			allowed[item.Name] = true
		}
		if err := validatePaths(provided, "", allowed); err != nil {
			return nil, err
		}
		target := out[scope.name].(map[string]any)
		for _, item := range scope.options {
			value, exists := getPath(provided, item.Name)
			if !exists {
				if item.Generated {
					if item.Name == "ResetID" {
						value = randomResetID()
					} else {
						value = rand.Text()
					}
				} else {
					value = item.Default
				}
				if scope.name == "server" && item.Name == "UPnP" {
					value = false
				}
			}
			normalized, err := validateValue(item, value, scope.name == "server")
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", scope.name, item.Name, err)
			}
			setPath(target, item.Name, normalized)
		}
	}
	password, ok := input["adminPassword"].(string)
	if input["adminPassword"] != nil && !ok {
		return nil, fmt.Errorf("initial admin password must be text")
	}
	if password == "" {
		password = rand.Text()
	}
	if strings.ContainsAny(password, "\r\n\x00") {
		return nil, fmt.Errorf("initial admin password must be a single line")
	}
	out["adminPassword"] = password
	if value, exists := input["preset"]; exists {
		id, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid sandbox preset")
		}
		if id != "" {
			found := false
			for _, preset := range definitions.Presets {
				if preset.ID == id {
					found = true
				}
			}
			if !found {
				return nil, fmt.Errorf("unknown official sandbox preset")
			}
			out["preset"] = id
		}
	}
	if err := validatePolicy(out); err != nil {
		return nil, err
	}
	return out, nil
}

func validatePaths(input map[string]any, prefix string, allowed map[string]bool) error {
	for key, value := range input {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if child, ok := value.(map[string]any); ok {
			if err := validatePaths(child, path, allowed); err != nil {
				return err
			}
		} else if !allowed[path] {
			return fmt.Errorf("unknown native configuration option: %s", path)
		}
	}
	return nil
}

func validateValue(o option, value any, ini bool) (any, error) {
	switch o.Type {
	case "boolean":
		if _, ok := value.(bool); !ok {
			return nil, fmt.Errorf("must be a boolean")
		}
	case "integer", "double", "enum":
		n, ok := number(value)
		if !ok || math.IsInf(n, 0) || math.IsNaN(n) || (o.Type != "double" && math.Trunc(n) != n) {
			return nil, fmt.Errorf("must be a valid %s", o.Type)
		}
		if (o.Min != nil && n < *o.Min) || (o.Max != nil && n > *o.Max) {
			return nil, fmt.Errorf("outside the official option range")
		}
		if o.Type == "enum" {
			found := false
			for _, c := range o.Choices {
				v, ok := number(c.Value)
				if ok && n == v {
					found = true
				}
			}
			if !found {
				return nil, fmt.Errorf("unknown official enum value")
			}
		}
		return n, nil
	case "string", "text":
		s, ok := value.(string)
		if !ok || !utf8.ValidString(s) {
			return nil, fmt.Errorf("must be valid text")
		}
		if strings.ContainsRune(s, 0) || (ini && (strings.ContainsRune(s, '=') || strings.IndexFunc(s, func(r rune) bool { return r < 0x20 }) >= 0)) {
			return nil, fmt.Errorf("INI text cannot contain ASCII control characters or equals; use <LINE> for display line breaks")
		}
		if ini && strings.TrimRightFunc(s, unicode.IsSpace) != s {
			return nil, fmt.Errorf("trailing whitespace is discarded by the official INI reader")
		}
		if o.MaxLength != nil && *o.MaxLength >= 0 && utf8.RuneCountInString(s) > *o.MaxLength {
			return nil, fmt.Errorf("exceeds the official maximum length")
		}
	default:
		return nil, fmt.Errorf("unsupported native option type")
	}
	return value, nil
}

func validatePolicy(config map[string]any) error {
	s := config["server"].(map[string]any)
	if s["DefaultPort"] != float64(DefaultInternalPort) || s["UDPPort"] != float64(UDPPort) {
		return fmt.Errorf("internal UDP ports are fixed at 16261/16262; configure the external port separately")
	}
	if s["Map"] != "Muldraugh, KY" || s["Mods"] != "" || s["WorkshopItems"] != "" {
		return fmt.Errorf("this provider supports the official vanilla map without Workshop or mods")
	}
	if s["RCONPassword"] != "" || s["UPnP"] != false {
		return fmt.Errorf("RCON and automatic router mapping are not enabled by this runtime")
	}
	if strings.TrimSpace(s["PublicName"].(string)) == "" {
		return fmt.Errorf("PublicName is required")
	}
	coordinates := strings.Split(s["SpawnPoint"].(string), ",")
	if len(coordinates) != 3 {
		return fmt.Errorf("SpawnPoint must be x,y,z")
	}
	for _, coord := range coordinates {
		value, err := strconv.ParseInt(coord, 10, 32)
		if err != nil || value < 0 {
			return fmt.Errorf("SpawnPoint coordinates must be non-negative native integers")
		}
	}
	sandbox := config["sandbox"].(map[string]any)
	month, _ := number(sandbox["StartMonth"])
	day, _ := number(sandbox["StartDay"])
	// Native ServerSettingsScreen.syncStartDay uses getFirstYear(), not StartYear.
	maxDay := time.Date(1993, time.Month(int(month)+1), 0, 0, 0, 0, 0, time.UTC).Day()
	if day > float64(maxDay) {
		return fmt.Errorf("StartDay is invalid for the selected month")
	}
	return nil
}

func (p Provider) ValidateConfigPayload(input map[string]any) error {
	_, err := p.NormalizeConfigPayload(input)
	return err
}
func (p Provider) ValidateConfigUpdate(current, next map[string]any) error {
	old, err := p.NormalizeConfigPayload(current)
	if err != nil {
		return err
	}
	updated, err := p.NormalizeConfigPayload(next)
	if err != nil {
		return err
	}
	for _, name := range []string{"server.ResetID", "server.ServerPlayerID", "adminPassword"} {
		a, _ := getPath(old, name)
		b, _ := getPath(updated, name)
		if !reflect.DeepEqual(a, b) {
			return fmt.Errorf("%s is an existing server identity and cannot be replaced", name)
		}
	}
	return nil
}

func ApplyPreset(input map[string]any, id string) (map[string]any, error) {
	for _, preset := range definitions.Presets {
		if preset.ID != id {
			continue
		}
		out := clone(input)
		sandbox := nativeDefaults()["sandbox"].(map[string]any)
		for name, value := range preset.Values {
			setPath(sandbox, name, value)
		}
		out["sandbox"], out["preset"] = sandbox, id
		return out, nil
	}
	return nil, fmt.Errorf("unknown official sandbox preset")
}

func (Provider) SanitizeConfigPreset(input map[string]any) map[string]any {
	out := clone(input)
	delete(out, "adminPassword")
	if server, ok := out["server"].(map[string]any); ok {
		for _, option := range definitions.Server {
			if option.Secret || option.Generated {
				delete(server, option.Name)
			}
		}
	}
	return out
}

func (p Provider) ConfigSummary(input map[string]any) (domain.ProviderConfigSummary, error) {
	c, err := p.NormalizeConfigPayload(input)
	if err != nil {
		return domain.ProviderConfigSummary{}, err
	}
	s := c["server"].(map[string]any)
	players, _ := number(s["MaxPlayers"])
	return domain.ProviderConfigSummary{ServerName: s["PublicName"].(string), WorldName: ServerIdentity, MaxPlayers: int(players), Port: DefaultInternalPort, Password: s["Password"].(string)}, nil
}

func luaString(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '\\':
			out.WriteString("\\\\")
		case '"':
			out.WriteString("\\\"")
		case '\n':
			out.WriteString("\\n")
		case '\r':
			out.WriteString("\\r")
		case '\t':
			out.WriteString("\\t")
		default:
			if r < 32 || r == 127 {
				fmt.Fprintf(&out, "\\%03d", r)
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return out.String()
}

func luaValue(value any, indent string) string {
	if object, ok := value.(map[string]any); ok {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var out strings.Builder
		out.WriteString("{\n")
		for _, key := range keys {
			fmt.Fprintf(&out, "%s  [%s] = %s,\n", indent, luaString(key), luaValue(object[key], indent+"  "))
		}
		out.WriteString(indent + "}")
		return out.String()
	}
	if s, ok := value.(string); ok {
		return luaString(s)
	}
	return fmt.Sprint(value)
}

func (p Provider) RuntimeConfigForResource(resource domain.GameServer) (domain.ProviderRuntimeConfig, error) {
	c, err := p.NormalizeConfigPayload(resource.Spec.Config)
	if err != nil {
		return domain.ProviderRuntimeConfig{}, err
	}
	if resource.Spec.Network.Port != 0 && resource.Spec.Network.Port != DefaultInternalPort {
		return domain.ProviderRuntimeConfig{}, fmt.Errorf("invalid Project Zomboid internal port")
	}
	s := c["server"].(map[string]any)
	var ini strings.Builder
	for _, item := range definitions.Server {
		value := fmt.Sprint(s[item.Name])
		if n, ok := number(s[item.Name]); ok && (item.Type == "integer" || item.Type == "enum") {
			value = strconv.FormatFloat(n, 'f', 0, 64)
		}
		fmt.Fprintf(&ini, "%s=%s\n", item.Name, value)
	}
	sandbox := c["sandbox"].(map[string]any)
	// Server SandboxVars files use VERSION; built-in preset tables use Version.
	// Missing VERSION invokes legacy option migrations and changes native values.
	sandbox["VERSION"] = float64(6)
	var regions strings.Builder
	regions.WriteString("function SpawnRegions()\n  return {\n")
	for _, page := range definitions.Pages.Server {
		if page.ID == "SpawnRegions" {
			for _, r := range page.DefaultRegions {
				fmt.Fprintf(&regions, "    { name = %s, file = %s },\n", luaString(r.Name), luaString(r.File))
			}
		}
	}
	regions.WriteString("  }\nend\n")
	return domain.ProviderRuntimeConfig{Port: DefaultInternalPort, Protocol: "udp", AdditionalPorts: []int{UDPPort}, Options: domain.WorkloadOptions{
		DataMounts: []string{"/data"}, Env: []string{"PZ_ADMIN_PASSWORD=" + c["adminPassword"].(string)}, Files: map[string]string{
			// Native world creation can change ResetID. The runtime merges this
			// desired configuration while preserving existing native identities.
			"Zomboid/Server/gamepanel.ini.pending":      ini.String(),
			"Zomboid/Server/gamepanel_SandboxVars.lua":  "SandboxVars = " + luaValue(sandbox, "") + "\n",
			"Zomboid/Server/gamepanel_spawnregions.lua": regions.String(),
		},
	}}, nil
}

func (p Provider) JoinInfo(server domain.GameServer) domain.ServerJoinInfo {
	s, _ := p.ConfigSummary(server.Spec.Config)
	port := server.Spec.Network.HostPort
	if port == 0 {
		port = DefaultInternalPort
	}
	return domain.ServerJoinInfo{Address: "127.0.0.1", Port: port, Password: s.Password, InviteText: fmt.Sprintf("Join %s in Project Zomboid at 127.0.0.1:%d", s.ServerName, port), Instructions: []string{"Use the 42.21 Steam client. Open Join, add this address and port, and create a player username and password; the server password is separate."}}
}
