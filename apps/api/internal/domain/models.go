package domain

import "time"

type ProviderKey string

type GameKey string

type ServerStatus string

type WorldSize string

type WorldEvil string

type Difficulty string

type Player struct {
	Name string `json:"name,omitempty"`
}

type PlayerLogEvent string

type ProviderCapabilities struct {
	ConsoleCommands bool `json:"consoleCommands"`
	PlayerList      bool `json:"playerList"`
	KickPlayer      bool `json:"kickPlayer"`
	BanPlayer       bool `json:"banPlayer"`
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

type GameCatalogEntry struct {
	Key         GameKey           `json:"key"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	CoverImage  string            `json:"coverImage,omitempty"`
	ServerCount int               `json:"serverCount"`
	Providers   []ProviderCatalog `json:"providers"`
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

type GameServerInstance struct {
	ID                    string         `json:"id" gorm:"primaryKey"`
	Name                  string         `json:"name"`
	GameKey               GameKey        `json:"gameKey"`
	ProviderKey           ProviderKey    `json:"providerKey"`
	Status                ServerStatus   `json:"status"`
	WorldName             string         `json:"worldName"`
	PlayersOnline         int            `json:"playersOnline"`
	Port                  int            `json:"port"`
	MaxPlayers            int            `json:"maxPlayers"`
	Password              string         `json:"password,omitempty"`
	DataDir               string         `json:"dataDir,omitempty"`
	ContainerID           string         `json:"containerId,omitempty"`
	HostPort              int            `json:"hostPort,omitempty"`
	CPULimitCores         float64        `json:"cpuLimitCores,omitempty"`
	MemoryLimitMB         int            `json:"memoryLimitMb,omitempty"`
	Version               string         `json:"version,omitempty"`
	LastError             string         `json:"lastError,omitempty"`
	SourceWorldID         string         `json:"sourceWorldId,omitempty"`
	SourceWorldName       string         `json:"sourceWorldName,omitempty"`
	Config                TerrariaConfig `json:"config" gorm:"embedded;embeddedPrefix:config_"`
	ConfigPayloadJSON     string         `json:"-" gorm:"column:config_payload_json"`
	ConfigPayload         map[string]any `json:"configPayload,omitempty" gorm:"-"`
	JoinInfo              ServerJoinInfo `json:"joinInfo,omitempty" gorm:"-"`
	ConfigRevision        int            `json:"configRevision"`
	AppliedConfigRevision int            `json:"appliedConfigRevision"`
	CreatedAt             time.Time      `json:"createdAt"`
	UpdatedAt             time.Time      `json:"updatedAt"`
}

type ServerJoinInfo struct {
	Address      string   `json:"address"`
	Port         int      `json:"port"`
	Password     string   `json:"password,omitempty"`
	InviteText   string   `json:"inviteText"`
	Instructions []string `json:"instructions,omitempty"`
}

type TerrariaConfig struct {
	ServerName      string     `json:"serverName"`
	WorldName       string     `json:"worldName"`
	WorldSize       WorldSize  `json:"worldSize"`
	WorldEvil       WorldEvil  `json:"worldEvil"`
	Difficulty      Difficulty `json:"difficulty"`
	MaxPlayers      int        `json:"maxPlayers"`
	Port            int        `json:"port"`
	Password        string     `json:"password,omitempty"`
	MOTD            string     `json:"motd,omitempty"`
	Seed            string     `json:"seed,omitempty"`
	Secure          bool       `json:"secure"`
	Language        string     `json:"language"`
	AutoCreateWorld bool       `json:"autoCreateWorld"`
}

type Backup struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	InstanceID string    `json:"instanceId" gorm:"index"`
	FileName   string    `json:"fileName"`
	WorldName  string    `json:"worldName"`
	SizeBytes  int64     `json:"sizeBytes"`
	Type       string    `json:"type"`
	CreatedAt  time.Time `json:"createdAt"`
}

type World struct {
	ID               string         `json:"id" gorm:"primaryKey"`
	InstanceID       string         `json:"instanceId" gorm:"index"`
	ProviderKey      ProviderKey    `json:"providerKey,omitempty" gorm:"index"`
	Name             string         `json:"name"`
	FileName         string         `json:"fileName"`
	SizeBytes        int64          `json:"sizeBytes"`
	Source           string         `json:"source,omitempty"`
	Config           TerrariaConfig `json:"config" gorm:"embedded;embeddedPrefix:config_"`
	ActiveInstanceID string         `json:"activeInstanceId,omitempty"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

type ModFile struct {
	ID               string    `json:"id" gorm:"primaryKey"`
	InstanceID       string    `json:"instanceId" gorm:"index"`
	FileName         string    `json:"fileName"`
	Source           string    `json:"source,omitempty" gorm:"index"`
	WorkshopID       string    `json:"workshopId,omitempty" gorm:"index"`
	ModName          string    `json:"modName,omitempty" gorm:"index"`
	Title            string    `json:"title,omitempty"`
	ModVersion       string    `json:"modVersion,omitempty"`
	TModVersion      string    `json:"tmodVersion,omitempty"`
	CreatorSteamID   string    `json:"creatorSteamId,omitempty"`
	PreviewURL       string    `json:"previewUrl,omitempty"`
	Description      string    `json:"description,omitempty"`
	TagsJSON         string    `json:"-" gorm:"column:tags_json"`
	Tags             []string  `json:"tags,omitempty" gorm:"-"`
	Subscriptions    int       `json:"subscriptions,omitempty"`
	Favorited        int       `json:"favorited,omitempty"`
	Views            int       `json:"views,omitempty"`
	UpdatedAtSteam   int64     `json:"updatedAtSteam,omitempty"`
	SizeBytes        int64     `json:"sizeBytes"`
	Enabled          bool      `json:"enabled"`
	RuntimeEnabled   *bool     `json:"runtimeEnabled,omitempty" gorm:"-"`
	RuntimePresent   *bool     `json:"runtimePresent,omitempty" gorm:"-"`
	Dependencies     []string  `json:"dependencies,omitempty" gorm:"-"`
	DependenciesJSON string    `json:"-" gorm:"column:dependencies_json"`
	CreatedAt        time.Time `json:"createdAt"`
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
	ID         string    `json:"id" gorm:"primaryKey"`
	InstanceID string    `json:"instanceId,omitempty" gorm:"index"`
	Type       string    `json:"type"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"createdAt"`
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
