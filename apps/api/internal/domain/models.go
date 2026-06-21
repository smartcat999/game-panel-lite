package domain

import "time"

type ProviderKey string

type GameKey string

type ServerStatus string

type Player struct {
	Name string `json:"name,omitempty"`
}

type PlayerLogEvent string

type ProviderCapabilities struct {
	ConsoleCommands bool `json:"consoleCommands"`
	PlayerList      bool `json:"playerList"`
	KickPlayer      bool `json:"kickPlayer"`
	BanPlayer       bool `json:"banPlayer"`
	Whitelist       bool `json:"whitelist"`
	SaveSnapshots   bool `json:"saveSnapshots"`
	Backups         bool `json:"backups"`
	Mods            bool `json:"mods"`
	Versions        bool `json:"versions"`
}

type ProviderConfigField struct {
	Name     string                      `json:"name"`
	Label    string                      `json:"label"`
	Type     string                      `json:"type"`
	Required bool                        `json:"required"`
	Default  any                         `json:"default,omitempty"`
	Options  []ProviderConfigFieldOption `json:"options,omitempty"`
	Help     string                      `json:"help,omitempty"`
}

type ProviderConfigFieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type ProviderConfigSummary struct {
	ServerName string
	WorldName  string
	MaxPlayers int
	Port       int
	Password   string
	MOTD       string
	Secure     bool
}

type GameCatalogEntry struct {
	Key         GameKey           `json:"key"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	CoverImage  string            `json:"coverImage,omitempty"`
	ServerCount int               `json:"serverCount"`
	Providers   []ProviderCatalog `json:"providers"`
}

type RuntimeImageStatus struct {
	Image     string    `json:"image"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Progress  int       `json:"progress,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

type ProviderCatalog struct {
	Key                ProviderKey           `json:"key"`
	Name               string                `json:"name"`
	Description        string                `json:"description"`
	Recommended        bool                  `json:"recommended"`
	Versions           []string              `json:"versions"`
	RecommendedVersion string                `json:"recommendedVersion,omitempty"`
	Capabilities       ProviderCapabilities  `json:"capabilities"`
	ConfigSchema       []ProviderConfigField `json:"configSchema"`
	SaveDisplayName    string                `json:"saveDisplayName,omitempty"`
	RuntimeImage       RuntimeImageStatus    `json:"runtimeImage,omitempty"`
}

const (
	GameTerraria  GameKey = "terraria"
	GamePalworld  GameKey = "palworld"
	GameDST       GameKey = "dont-starve-together"
	GameMinecraft GameKey = "minecraft"

	ProviderTerrariaVanilla    ProviderKey = "terraria-vanilla"
	ProviderTerrariaTModLoader ProviderKey = "terraria-tmodloader"
	ProviderPalworld           ProviderKey = "palworld"
	ProviderDST                ProviderKey = "dont-starve-together"
	ProviderMinecraft          ProviderKey = "minecraft"

	StatusCreating   ServerStatus = "creating"
	StatusStarting   ServerStatus = "starting"
	StatusRunning    ServerStatus = "running"
	StatusStopping   ServerStatus = "stopping"
	StatusStopped    ServerStatus = "stopped"
	StatusRestarting ServerStatus = "restarting"
	StatusDeleting   ServerStatus = "deleting"
	StatusErrored    ServerStatus = "errored"

	PlayerJoined PlayerLogEvent = "joined"
	PlayerLeft   PlayerLogEvent = "left"
)

type ServerJoinInfo struct {
	Address      string   `json:"address"`
	Port         int      `json:"port"`
	Password     string   `json:"password,omitempty"`
	InviteText   string   `json:"inviteText"`
	Instructions []string `json:"instructions,omitempty"`
}

type ServerShare struct {
	Token           string    `json:"token" gorm:"primaryKey"`
	InstanceID      string    `json:"instanceId" gorm:"uniqueIndex"`
	IncludePassword bool      `json:"includePassword"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type ConfigPreset struct {
	ID                string         `json:"id" gorm:"primaryKey"`
	Name              string         `json:"name"`
	GameKey           GameKey        `json:"gameKey" gorm:"index"`
	ProviderKey       ProviderKey    `json:"providerKey" gorm:"index"`
	Version           string         `json:"version,omitempty"`
	Config            map[string]any `json:"config,omitempty" gorm:"-"`
	ConfigPayloadJSON string         `json:"-" gorm:"column:config_payload_json"`
	ConfigPayload     map[string]any `json:"configPayload,omitempty" gorm:"-"`
	CPULimitCores     float64        `json:"cpuLimitCores,omitempty"`
	MemoryLimitMB     int            `json:"memoryLimitMb,omitempty"`
	ModPackID         string         `json:"modPackId,omitempty" gorm:"index"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
}

type Backup struct {
	ID          string      `json:"id" gorm:"primaryKey"`
	InstanceID  string      `json:"instanceId" gorm:"index"`
	GameKey     GameKey     `json:"gameKey,omitempty" gorm:"-"`
	ProviderKey ProviderKey `json:"providerKey,omitempty" gorm:"-"`
	FileName    string      `json:"fileName"`
	WorldName   string      `json:"worldName"`
	SizeBytes   int64       `json:"sizeBytes"`
	Type        string      `json:"type"`
	CreatedAt   time.Time   `json:"createdAt"`
}

type World struct {
	ID                string         `json:"id" gorm:"primaryKey"`
	InstanceID        string         `json:"instanceId" gorm:"index"`
	GameKey           GameKey        `json:"gameKey,omitempty" gorm:"-"`
	ProviderKey       ProviderKey    `json:"providerKey,omitempty" gorm:"index"`
	Name              string         `json:"name"`
	FileName          string         `json:"fileName"`
	SizeBytes         int64          `json:"sizeBytes"`
	Source            string         `json:"source,omitempty"`
	Config            map[string]any `json:"config,omitempty" gorm:"-"`
	ConfigPayloadJSON string         `json:"-" gorm:"column:config_payload_json"`
	ConfigPayload     map[string]any `json:"configPayload,omitempty" gorm:"-"`
	ActiveInstanceID  string         `json:"activeInstanceId,omitempty"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
}

type ModFile struct {
	ID               string      `json:"id" gorm:"primaryKey"`
	InstanceID       string      `json:"instanceId" gorm:"index"`
	GameKey          GameKey     `json:"gameKey,omitempty" gorm:"index"`
	ProviderKey      ProviderKey `json:"providerKey,omitempty" gorm:"index"`
	FileName         string      `json:"fileName"`
	Source           string      `json:"source,omitempty" gorm:"index"`
	WorkshopID       string      `json:"workshopId,omitempty" gorm:"index"`
	ModName          string      `json:"modName,omitempty" gorm:"index"`
	Title            string      `json:"title,omitempty"`
	ModVersion       string      `json:"modVersion,omitempty"`
	TModVersion      string      `json:"tmodVersion,omitempty"`
	CreatorSteamID   string      `json:"creatorSteamId,omitempty"`
	PreviewURL       string      `json:"previewUrl,omitempty"`
	Description      string      `json:"description,omitempty"`
	ContentHash      string      `json:"contentHash,omitempty" gorm:"index"`
	TagsJSON         string      `json:"-" gorm:"column:tags_json"`
	Tags             []string    `json:"tags,omitempty" gorm:"-"`
	Subscriptions    int         `json:"subscriptions,omitempty"`
	Favorited        int         `json:"favorited,omitempty"`
	Views            int         `json:"views,omitempty"`
	UpdatedAtSteam   int64       `json:"updatedAtSteam,omitempty"`
	SizeBytes        int64       `json:"sizeBytes"`
	Enabled          bool        `json:"enabled"`
	RuntimeEnabled   *bool       `json:"runtimeEnabled,omitempty" gorm:"-"`
	RuntimePresent   *bool       `json:"runtimePresent,omitempty" gorm:"-"`
	Dependencies     []string    `json:"dependencies,omitempty" gorm:"-"`
	DependenciesJSON string      `json:"-" gorm:"column:dependencies_json"`
	CreatedAt        time.Time   `json:"createdAt"`
}

type ModPack struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ModIDsJSON  string    `json:"-" gorm:"column:mod_ids_json"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ActivityEvent struct {
	ID          string         `json:"id" gorm:"primaryKey"`
	InstanceID  string         `json:"instanceId,omitempty" gorm:"index"`
	Type        string         `json:"type"`
	Message     string         `json:"message"`
	PayloadJSON string         `json:"-" gorm:"column:payload_json"`
	Payload     map[string]any `json:"payload,omitempty" gorm:"-"`
	CreatedAt   time.Time      `json:"createdAt"`
}

type AdminAccount struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	Username     string    `json:"username" gorm:"uniqueIndex"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Session struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	AccountID string    `json:"accountId" gorm:"index"`
	TokenHash string    `json:"-" gorm:"uniqueIndex"`
	ExpiresAt time.Time `json:"expiresAt" gorm:"index"`
	CreatedAt time.Time `json:"createdAt"`
}

type Setting struct {
	Key   string `json:"key" gorm:"primaryKey"`
	Value string `json:"value"`
}
