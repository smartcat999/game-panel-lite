package monitoring

import "time"

type DataSource struct {
	Kind               string     `json:"kind"`
	Connected          bool       `json:"connected"`
	EndpointConfigured bool       `json:"endpointConfigured"`
	LastQueryAt        *time.Time `json:"lastQueryAt"`
	Error              string     `json:"error,omitempty"`
}

type Range struct {
	Range string    `json:"range"`
	Step  string    `json:"step"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type Point struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type Series struct {
	Key          string   `json:"key"`
	Title        string   `json:"title"`
	Unit         string   `json:"unit"`
	ChartType    string   `json:"chartType"`
	CurrentValue *float64 `json:"currentValue"`
	Avg          *float64 `json:"avg"`
	Max          *float64 `json:"max"`
	Threshold    *float64 `json:"threshold,omitempty"`
	Points       []Point  `json:"points"`
	EmptyReason  string   `json:"emptyReason,omitempty"`
}

type OverviewResponse struct {
	CollectedAt time.Time  `json:"collectedAt"`
	DataSource  DataSource `json:"dataSource"`
	Health      Health     `json:"health"`
	KPIs        KPIs       `json:"kpis"`
}

type Health struct {
	Overall             string     `json:"overall"`
	PrometheusConnected bool       `json:"prometheusConnected"`
	DockerRuntime       string     `json:"dockerRuntime"`
	LastSync            *time.Time `json:"lastSync"`
	FailedTargets       int        `json:"failedTargets"`
}

type KPIs struct {
	TotalServers         int `json:"totalServers"`
	RunningServers       int `json:"runningServers"`
	Issues               int `json:"issues"`
	OnlinePlayers        int `json:"onlinePlayers"`
	PlayerCapacity       int `json:"playerCapacity"`
	ResourceUsagePercent int `json:"resourceUsagePercent"`
	StorageBytes         int `json:"storageBytes"`
}

type MetricsResponse struct {
	CollectedAt time.Time         `json:"collectedAt"`
	Range       Range             `json:"range"`
	DataSource  DataSource        `json:"dataSource"`
	Series      map[string]Series `json:"series"`
}

type PlatformResponse struct {
	CollectedAt time.Time         `json:"collectedAt"`
	Range       Range             `json:"range"`
	DataSource  DataSource        `json:"dataSource"`
	Services    []PlatformService `json:"services"`
	Series      map[string]Series `json:"series"`
	TopRoutes   []RouteMetric     `json:"topRoutes"`
}

type PlatformService struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Instance  string `json:"instance,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

type RouteMetric struct {
	Route       string  `json:"route"`
	Method      string  `json:"method"`
	RequestRate float64 `json:"requestRate"`
	ErrorRate   float64 `json:"errorRate"`
	P95Ms       float64 `json:"p95Ms"`
}

type ServerLoadResponse struct {
	CollectedAt time.Time       `json:"collectedAt"`
	DataSource  DataSource      `json:"dataSource"`
	Rows        []ServerLoadRow `json:"rows"`
}

type ServerLoadRow struct {
	ServerID      string  `json:"serverId"`
	ServerName    string  `json:"serverName"`
	GameKey       string  `json:"gameKey"`
	ProviderKey   string  `json:"providerKey"`
	Version       string  `json:"version,omitempty"`
	Status        string  `json:"status"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemoryMB      float64 `json:"memoryMb"`
	MemoryLimitMB float64 `json:"memoryLimitMb"`
	PlayersOnline int     `json:"playersOnline"`
	MaxPlayers    int     `json:"maxPlayers"`
	LastActive    string  `json:"lastActive"`
	Severity      string  `json:"severity"`
}

type EventsResponse struct {
	CollectedAt time.Time `json:"collectedAt"`
	Events      []Event   `json:"events"`
}

type Event struct {
	ID         string            `json:"id"`
	Severity   string            `json:"severity"`
	Type       string            `json:"type"`
	Title      string            `json:"title"`
	Message    string            `json:"message"`
	ServerID   string            `json:"serverId,omitempty"`
	ServerName string            `json:"serverName,omitempty"`
	Operator   string            `json:"operator"`
	Timestamp  time.Time         `json:"timestamp"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}
