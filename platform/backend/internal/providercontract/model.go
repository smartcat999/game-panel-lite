package providercontract

import (
	"errors"
	"time"

	contract "github.com/smartcat999/game-panel-lite/platform/backend/internal/contracts/v1"
)

var (
	ErrInvalidManifest      = errors.New("invalid provider manifest")
	ErrInvalidSignature     = errors.New("invalid provider signature")
	ErrInvalidConfiguration = errors.New("invalid provider configuration")
	ErrUnsupportedMigration = errors.New("unsupported configuration migration")
	ErrUnresolvedMod        = errors.New("mod selection cannot be resolved")
	ErrImmutableRelease     = errors.New("provider release is immutable")
)

type ApplyBehavior string

const (
	ApplyHotReload  ApplyBehavior = "hot-reload"
	ApplyRestart    ApplyBehavior = "restart-required"
	ApplyRecreate   ApplyBehavior = "recreate-required"
	ApplyCreateOnly ApplyBehavior = "create-only"
)

type Field struct {
	Type          string                       `json:"type"`
	Title         string                       `json:"title"`
	Description   string                       `json:"description,omitempty"`
	Localizations map[string]FieldLocalization `json:"localizations,omitempty"`
	ApplyBehavior ApplyBehavior                `json:"applyBehavior"`
	Default       any                          `json:"default,omitempty"`
	Minimum       *float64                     `json:"minimum,omitempty"`
	Maximum       *float64                     `json:"maximum,omitempty"`
	Enum          []any                        `json:"enum,omitempty"`
	Pattern       string                       `json:"pattern,omitempty"`
	MinLength     *int                         `json:"minLength,omitempty"`
	MaxLength     *int                         `json:"maxLength,omitempty"`
}

type FieldLocalization struct {
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Default     any               `json:"default,omitempty"`
	EnumLabels  map[string]string `json:"enumLabels,omitempty"`
}

type ConfigurationSchema struct {
	Type       string           `json:"type"`
	Properties map[string]Field `json:"properties"`
	Required   []string         `json:"required,omitempty"`
}

type UISection struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Localizations map[string]string `json:"localizations,omitempty"`
	Order         int               `json:"order"`
}

type Visibility struct {
	Field  string `json:"field"`
	Equals any    `json:"equals"`
}

type UIField struct {
	Section     string      `json:"section"`
	Order       int         `json:"order"`
	Control     string      `json:"control"`
	VisibleWhen *Visibility `json:"visibleWhen,omitempty"`
}

type UISchema struct {
	Sections []UISection        `json:"sections"`
	Fields   map[string]UIField `json:"fields"`
}

type Migration struct {
	FromSchemaVersion int    `json:"fromSchemaVersion"`
	ToSchemaVersion   int    `json:"toSchemaVersion"`
	Mode              string `json:"mode"`
}

type Metric struct {
	Key               string  `json:"key"`
	Title             string  `json:"title"`
	Unit              string  `json:"unit"`
	Source            string  `json:"source"`
	FreshnessSeconds  int     `json:"freshnessSeconds"`
	MinimumConfidence float64 `json:"minimumConfidence"`
}

type ModDependency struct {
	ModID   string `json:"modId"`
	Version string `json:"version"`
}

type ModVersion struct {
	Version      string          `json:"version"`
	Digest       string          `json:"digest"`
	Dependencies []ModDependency `json:"dependencies"`
}

type ModCatalogEntry struct {
	ModID       string       `json:"modId"`
	DisplayName string       `json:"displayName"`
	Versions    []ModVersion `json:"versions"`
}

type ModCatalog struct {
	Revision  int               `json:"revision"`
	Entries   []ModCatalogEntry `json:"entries"`
	Digest    string            `json:"digest"`
	Signature string            `json:"signature"`
}

type ModSelection struct {
	ModID   string `json:"modId"`
	Version string `json:"version"`
}

type ModLockEntry = contract.ModLockEntry

type Manifest struct {
	ProviderReleaseID    string                         `json:"providerReleaseId"`
	GameKey              string                         `json:"gameKey"`
	DisplayName          string                         `json:"displayName"`
	ReleaseVersion       string                         `json:"releaseVersion"`
	GameVersions         []string                       `json:"gameVersions"`
	SchemaVersion        int                            `json:"schemaVersion"`
	ConfigurationSchema  ConfigurationSchema            `json:"configurationSchema"`
	UISchema             UISchema                       `json:"uiSchema"`
	ListenerRequirements []contract.ListenerRequirement `json:"listenerRequirements"`
	Capabilities         []string                       `json:"capabilities"`
	Metrics              []Metric                       `json:"metrics"`
	SchemaMigrations     []Migration                    `json:"schemaMigrations"`
	ModCatalog           *ModCatalog                    `json:"modCatalog,omitempty"`
	ManifestDigest       string                         `json:"manifestDigest"`
	Signature            string                         `json:"signature"`
	PublishedAt          time.Time                      `json:"publishedAt"`
}

type ValidationResult struct {
	ApplyBehavior ApplyBehavior
	ModLock       []ModLockEntry
}
