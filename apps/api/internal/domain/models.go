package domain

import "time"

type ProviderKey string

type ServerStatus string

type WorldSize string

type WorldEvil string

type Difficulty string

const (
	ProviderTerrariaVanilla    ProviderKey = "terraria-vanilla"
	ProviderTerrariaTModLoader ProviderKey = "terraria-tmodloader"

	StatusCreating   ServerStatus = "creating"
	StatusStarting   ServerStatus = "starting"
	StatusRunning    ServerStatus = "running"
	StatusStopped    ServerStatus = "stopped"
	StatusRestarting ServerStatus = "restarting"
	StatusDeleting   ServerStatus = "deleting"
	StatusErrored    ServerStatus = "errored"
)

type GameServerInstance struct {
	ID          string         `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name"`
	GameKey     string         `json:"gameKey"`
	ProviderKey ProviderKey    `json:"providerKey"`
	Status      ServerStatus   `json:"status"`
	WorldName   string         `json:"worldName"`
	Port        int            `json:"port"`
	MaxPlayers  int            `json:"maxPlayers"`
	Password    string         `json:"password,omitempty"`
	DataDir     string         `json:"dataDir,omitempty"`
	ContainerID string         `json:"containerId,omitempty"`
	HostPort    int            `json:"hostPort,omitempty"`
	Version     string         `json:"version,omitempty"`
	Config      TerrariaConfig `json:"config" gorm:"embedded;embeddedPrefix:config_"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
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
	ID               string    `json:"id" gorm:"primaryKey"`
	InstanceID       string    `json:"instanceId" gorm:"index"`
	Name             string    `json:"name"`
	FileName         string    `json:"fileName"`
	SizeBytes        int64     `json:"sizeBytes"`
	ActiveInstanceID string    `json:"activeInstanceId,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type ModFile struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	InstanceID string    `json:"instanceId" gorm:"index"`
	FileName   string    `json:"fileName"`
	SizeBytes  int64     `json:"sizeBytes"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ActivityEvent struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	InstanceID string    `json:"instanceId,omitempty" gorm:"index"`
	Type       string    `json:"type"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"createdAt"`
}
