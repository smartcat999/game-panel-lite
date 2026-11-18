package config

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

// Config is the single unified configuration object managed by pkg/config.
// All YAML sections and GP_* environment variables are parsed, defaulted, and validated here.
type Config struct {
	Log           LogConfig           `yaml:"log" mapstructure:"log"`
	Server        ServerConfig        `yaml:"server" mapstructure:"server"`
	Auth          AuthConfig          `yaml:"auth" mapstructure:"auth"`
	Database      DatabaseConfig      `yaml:"database" mapstructure:"database"`
	Cache         CacheConfig         `yaml:"cache" mapstructure:"cache"`
	MQ            MQConfig            `yaml:"mq" mapstructure:"mq"`
	Controller    ControllerConfig    `yaml:"controller" mapstructure:"controller"`
	Agent         AgentConfig         `yaml:"agent" mapstructure:"agent"`
	Bootstrap     BootstrapConfig     `yaml:"bootstrap" mapstructure:"bootstrap"`
	Client        ClientConfig        `yaml:"client" mapstructure:"client"`
	Network       NetworkConfig       `yaml:"network" mapstructure:"network"`
	S3            S3Config            `yaml:"s3" mapstructure:"s3"`
	Observability ObservabilityConfig `yaml:"observability" mapstructure:"observability"`
	AddonMirror   AddonMirrorConfig   `yaml:"addonMirror" mapstructure:"addonMirror"`
	Registry      RegistryConfig      `yaml:"registry" mapstructure:"registry"`
}

var (
	globalMu    sync.RWMutex
	globalCfg   = New()
	onLoadHooks []func(*Config) error
)

// RegisterOnLoad registers a callback invoked whenever Load or LoadFile validates a Config.
func RegisterOnLoad(fn func(*Config) error) {
	if fn == nil {
		return
	}
	globalMu.Lock()
	onLoadHooks = append(onLoadHooks, fn)
	globalMu.Unlock()
}

// New returns a new unified Config initialized with all default values.
func New() *Config {
	c := &Config{
		Agent: AgentConfig{PerInstancePrometheusMetrics: true},
	}
	c.Default()
	return c
}

// Get returns the active unified Config object loaded by Load / LoadFile / NewCommand.
func Get() *Config {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalCfg == nil {
		return New()
	}
	return globalCfg
}

// Set replaces the active global Config object.
func Set(cfg *Config) {
	if cfg == nil {
		cfg = New()
	}
	globalMu.Lock()
	globalCfg = cfg
	globalMu.Unlock()
}

// Default populates default values across all configuration sections.
func (c *Config) Default() {
	if strings.TrimSpace(c.Agent.Token) == "" && strings.TrimSpace(c.Client.Token) != "" {
		c.Agent.Token = strings.TrimSpace(c.Client.Token)
	}
	c.Log.Default()
	c.Server.Default()
	c.Auth.Default()
	c.Database.Default()
	c.Cache.Default()
	c.MQ.Default()
	c.Controller.Default()
	c.Agent.Default()
	c.Bootstrap.Default()
	c.Client.Default()
	c.Network.Default()
	c.S3.Default()
	c.Observability.Default()
	c.AddonMirror.Default()
	c.Registry.Default()
}

// Validate validates all configuration sections and returns any combined errors.
func (c *Config) Validate() error {
	return errors.Join(
		c.Log.Validate(),
		c.Server.Validate(),
		c.Auth.Validate(),
		c.Database.Validate(),
		c.Cache.Validate(),
		c.MQ.Validate(),
		c.Controller.Validate(),
		c.Agent.Validate(),
		c.Client.Validate(),
		c.Network.Validate(),
		c.S3.Validate(),
		c.Observability.Validate(),
		c.AddonMirror.Validate(),
		c.Registry.Validate(),
	)
}

// LogConfig configures structured logging.
type LogConfig struct {
	Level       string `yaml:"level" mapstructure:"level" env:"GP_LOG_LEVEL"`
	Format      string `yaml:"format" mapstructure:"format" env:"GP_LOG_FORMAT"`
	ServiceName string `yaml:"serviceName" mapstructure:"serviceName" env:"GP_SERVICE_NAME"`
}

func NewLogConfig() *LogConfig {
	c := &LogConfig{}
	c.Default()
	return c
}

func (c *LogConfig) Default() {
	if c.Level == "" {
		c.Level = "info"
	}
	if c.Format == "" {
		c.Format = "console"
	}
}

func (c *LogConfig) Validate() error {
	switch c.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log.level must be one of debug|info|warn|error, got %q", c.Level)
	}
	switch c.Format {
	case "json", "console":
	default:
		return fmt.Errorf("log.format must be one of json|console, got %q", c.Format)
	}
	return nil
}

// ServerConfig configures the HTTP API server.
type ServerConfig struct {
	Addr             string        `yaml:"addr" mapstructure:"addr" env:"GP_SERVER_ADDR"`
	ReadTimeout      time.Duration `yaml:"readTimeout" mapstructure:"readTimeout" env:"GP_SERVER_READ_TIMEOUT"`
	WriteTimeout     time.Duration `yaml:"writeTimeout" mapstructure:"writeTimeout" env:"GP_SERVER_WRITE_TIMEOUT"`
	ShutdownTimeout  time.Duration `yaml:"shutdownTimeout" mapstructure:"shutdownTimeout" env:"GP_SERVER_SHUTDOWN_TIMEOUT"`
	EnableController bool          `yaml:"enableController" mapstructure:"enableController" env:"GP_SERVER_ENABLE_CONTROLLER"`
}

func NewServerConfig() *ServerConfig {
	c := &ServerConfig{}
	c.Default()
	return c
}

func (c *ServerConfig) Default() {
	if c.Addr == "" {
		c.Addr = "0.0.0.0:8080"
	}
	if c.ReadTimeout <= 0 {
		c.ReadTimeout = 15 * time.Second
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = 30 * time.Second
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 15 * time.Second
	}
}

func (c *ServerConfig) Validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return errors.New("server.addr is required")
	}
	if c.ReadTimeout <= 0 || c.WriteTimeout <= 0 || c.ShutdownTimeout <= 0 {
		return errors.New("server timeouts must be positive durations")
	}
	return nil
}

// AuthConfig configures session JWT signing and token TTL.
type AuthConfig struct {
	JWTSecret string        `yaml:"jwtSecret" mapstructure:"jwtSecret" env:"GP_AUTH_JWT_SECRET"`
	TokenTTL  time.Duration `yaml:"tokenTTL" mapstructure:"tokenTTL" env:"GP_AUTH_TOKEN_TTL"`
}

func NewAuthConfig() *AuthConfig {
	c := &AuthConfig{}
	c.Default()
	return c
}

func (c *AuthConfig) Default() {
	if c.JWTSecret == "" {
		c.JWTSecret = "dev-secret-change-me"
	}
	if c.TokenTTL <= 0 {
		c.TokenTTL = 24 * time.Hour
	}
}

func (c *AuthConfig) Validate() error {
	if len(strings.TrimSpace(c.JWTSecret)) < 8 {
		return errors.New("auth.jwtSecret must be at least 8 characters")
	}
	if c.TokenTTL <= 0 {
		return errors.New("auth.tokenTTL must be positive")
	}
	return nil
}

// DatabaseConfig configures PostgreSQL connection pooling.
type DatabaseConfig struct {
	DSN             string        `yaml:"dsn" mapstructure:"dsn" env:"GP_DB_DSN"`
	MaxOpenConns    int           `yaml:"maxOpenConns" mapstructure:"maxOpenConns" env:"GP_DB_MAX_OPEN_CONNS"`
	MaxIdleConns    int           `yaml:"maxIdleConns" mapstructure:"maxIdleConns" env:"GP_DB_MAX_IDLE_CONNS"`
	ConnMaxLifetime time.Duration `yaml:"connMaxLifetime" mapstructure:"connMaxLifetime" env:"GP_DB_CONN_MAX_LIFETIME"`
}

func NewDatabaseConfig() *DatabaseConfig {
	c := &DatabaseConfig{}
	c.Default()
	return c
}

func (c *DatabaseConfig) Default() {
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 25
	}
	if c.MaxIdleConns <= 0 {
		c.MaxIdleConns = 5
	}
	if c.ConnMaxLifetime <= 0 {
		c.ConnMaxLifetime = 30 * time.Minute
	}
}

func (c *DatabaseConfig) Validate() error {
	if c.MaxOpenConns <= 0 {
		return errors.New("database.maxOpenConns must be positive")
	}
	if c.MaxIdleConns <= 0 || c.MaxIdleConns > c.MaxOpenConns {
		return errors.New("database.maxIdleConns must be between 1 and maxOpenConns")
	}
	return nil
}

// CacheConfig configures the key-value cache backend ("redis").
type CacheConfig struct {
	Driver   string `yaml:"driver" mapstructure:"driver" env:"GP_CACHE_DRIVER"`
	Addr     string `yaml:"addr" mapstructure:"addr" env:"GP_CACHE_ADDR"`
	Password string `yaml:"password" mapstructure:"password" env:"GP_CACHE_PASSWORD"`
	DB       int    `yaml:"db" mapstructure:"db" env:"GP_CACHE_DB"`
}

func NewCacheConfig() *CacheConfig {
	c := &CacheConfig{}
	c.Default()
	return c
}

func (c *CacheConfig) Default() {
	// Driver is intentionally not defaulted to prevent silent fallback.
}

func (c *CacheConfig) Validate() error {
	driver := strings.ToLower(strings.TrimSpace(c.Driver))
	if driver == "" {
		if strings.TrimSpace(c.Addr) != "" || strings.TrimSpace(c.Password) != "" || c.DB != 0 {
			return errors.New("cache.driver is required when cache settings are specified (expected 'redis')")
		}
		return nil
	}
	switch driver {
	case "redis":
		if strings.TrimSpace(c.Addr) == "" {
			return errors.New("cache.addr is required when cache.driver=redis")
		}
		return nil
	default:
		return fmt.Errorf("unsupported cache.driver: %s (expected 'redis')", c.Driver)
	}
}

// MQConfig configures the message queue backend ("redis" or "nats").
type MQConfig struct {
	Driver   string `yaml:"driver" mapstructure:"driver" env:"GP_MQ_DRIVER"`
	Addr     string `yaml:"addr" mapstructure:"addr" env:"GP_MQ_ADDR"`
	Password string `yaml:"password" mapstructure:"password" env:"GP_MQ_PASSWORD"`
	DB       int    `yaml:"db" mapstructure:"db" env:"GP_MQ_DB"`
	Group    string `yaml:"group" mapstructure:"group" env:"GP_MQ_GROUP"`
}

func NewMQConfig() *MQConfig {
	c := &MQConfig{}
	c.Default()
	return c
}

func (c *MQConfig) Default() {
	// Driver is intentionally not defaulted to prevent silent fallback.
	if strings.TrimSpace(c.Group) == "" {
		c.Group = "gamepanel-controllers"
	}
}

func (c *MQConfig) Validate() error {
	driver := strings.ToLower(strings.TrimSpace(c.Driver))
	if driver == "" {
		if strings.TrimSpace(c.Addr) != "" || strings.TrimSpace(c.Password) != "" || c.DB != 0 {
			return errors.New("mq.driver is required when mq settings are specified (expected 'redis' or 'nats')")
		}
		return nil
	}
	if driver != "redis" && driver != "nats" {
		return fmt.Errorf("mq.driver must be 'redis' or 'nats', got %q", c.Driver)
	}
	if strings.TrimSpace(c.Addr) == "" {
		return fmt.Errorf("mq.addr is required when mq.driver=%s", driver)
	}
	return nil
}

// ControllerConfig configures the asynchronous controller manager and resync loop.
type ControllerConfig struct {
	Workers                 int           `yaml:"workers" mapstructure:"workers" env:"GP_CONTROLLER_WORKERS"`
	ResyncPeriod            time.Duration `yaml:"resyncPeriod" mapstructure:"resyncPeriod" env:"GP_CONTROLLER_RESYNC_PERIOD"`
	ResyncPages             int           `yaml:"resyncPages" mapstructure:"resyncPages" env:"GP_CONTROLLER_RESYNC_PAGES"`
	ResyncEventBudget       int           `yaml:"resyncEventBudget" mapstructure:"resyncEventBudget" env:"GP_CONTROLLER_RESYNC_EVENT_BUDGET"`
	ResyncTimeout           time.Duration `yaml:"resyncTimeout" mapstructure:"resyncTimeout" env:"GP_CONTROLLER_RESYNC_TIMEOUT"`
	ResyncLeaseTTL          time.Duration `yaml:"resyncLeaseTTL" mapstructure:"resyncLeaseTTL" env:"GP_CONTROLLER_RESYNC_LEASE_TTL"`
	NodeGraceDuration       time.Duration `yaml:"nodeGraceDuration" mapstructure:"nodeGraceDuration" env:"GP_CONTROLLER_NODE_GRACE_DURATION"`
	SubscriptionGracePeriod time.Duration `yaml:"subscriptionGracePeriod" mapstructure:"subscriptionGracePeriod" env:"GP_CONTROLLER_SUBSCRIPTION_GRACE_PERIOD"`
	// Addr serves /livez, /readyz and /metrics for the controller-manager ("" disables it).
	Addr string `yaml:"addr" mapstructure:"addr" env:"GP_CONTROLLER_ADDR"`
}

func NewControllerConfig() *ControllerConfig {
	c := &ControllerConfig{}
	c.Default()
	return c
}

func (c *ControllerConfig) Default() {
	if c.Workers <= 0 {
		c.Workers = 4
	}
	if c.ResyncPeriod <= 0 {
		c.ResyncPeriod = 5 * time.Minute
	}
	if c.ResyncPages == 0 {
		c.ResyncPages = 4
	}
	if c.ResyncEventBudget == 0 {
		c.ResyncEventBudget = 4000
	}
	if c.ResyncTimeout == 0 {
		c.ResyncTimeout = 30 * time.Second
	}
	if c.ResyncLeaseTTL == 0 {
		c.ResyncLeaseTTL = 30 * time.Second
	}
	if c.NodeGraceDuration <= 0 {
		c.NodeGraceDuration = 40 * time.Second
	}
	if c.SubscriptionGracePeriod <= 0 {
		c.SubscriptionGracePeriod = 7 * 24 * time.Hour
	}
	if c.Addr == "" {
		c.Addr = "0.0.0.0:8083"
	}
}

func (c *ControllerConfig) Validate() error {
	if c.Workers <= 0 || c.Workers > 256 {
		return errors.New("controller.workers must be between 1 and 256")
	}
	if c.ResyncPeriod <= 0 || c.NodeGraceDuration <= 0 || c.ResyncTimeout <= 0 {
		return errors.New("controller durations must be positive")
	}
	if c.ResyncPages < 1 || c.ResyncPages > 1000 {
		return errors.New("controller.resyncPages must be between 1 and 1000")
	}
	if c.ResyncEventBudget < 1 || c.ResyncEventBudget > 1000000 {
		return errors.New("controller.resyncEventBudget must be between 1 and 1000000")
	}
	if c.ResyncLeaseTTL < 3*time.Millisecond {
		return errors.New("controller.resyncLeaseTTL must be at least 3ms")
	}
	return nil
}

// AgentConfig configures the compute host node-agent.
type AgentConfig struct {
	NodeName                     string                    `yaml:"nodeName" mapstructure:"nodeName" env:"GP_AGENT_NODE_NAME"`
	Region                       string                    `yaml:"region" mapstructure:"region" env:"GP_AGENT_REGION"`
	InternalIP                   string                    `yaml:"internalIP" mapstructure:"internalIP" env:"GP_AGENT_INTERNAL_IP"`
	ExternalIP                   string                    `yaml:"externalIP" mapstructure:"externalIP" env:"GP_AGENT_EXTERNAL_IP"`
	Addr                         string                    `yaml:"addr" mapstructure:"addr" env:"GP_AGENT_ADDR"`
	APIServerURL                 string                    `yaml:"apiserverURL" mapstructure:"apiserverURL" env:"GP_AGENT_APISERVER_URL"`
	Token                        string                    `yaml:"token" mapstructure:"token" env:"GP_AGENT_TOKEN"`
	HeartbeatInterval            time.Duration             `yaml:"heartbeatInterval" mapstructure:"heartbeatInterval" env:"GP_AGENT_HEARTBEAT_INTERVAL"`
	WorkDir                      string                    `yaml:"workDir" mapstructure:"workDir" env:"GP_AGENT_WORK_DIR"`
	ShutdownTimeout              time.Duration             `yaml:"shutdownTimeout" mapstructure:"shutdownTimeout" env:"GP_AGENT_SHUTDOWN_TIMEOUT"`
	ReservedCPU                  string                    `yaml:"reservedCPU" mapstructure:"reservedCPU" env:"GP_AGENT_RESERVED_CPU"`
	ReservedMemory               string                    `yaml:"reservedMemory" mapstructure:"reservedMemory" env:"GP_AGENT_RESERVED_MEMORY"`
	ReservedStorage              string                    `yaml:"reservedStorage" mapstructure:"reservedStorage" env:"GP_AGENT_RESERVED_STORAGE"`
	ReservedResources            *AgentResourceReservation `yaml:"-" mapstructure:"-"`
	PerInstancePrometheusMetrics bool                      `yaml:"perInstancePrometheusMetrics" mapstructure:"perInstancePrometheusMetrics" env:"GP_AGENT_PER_INSTANCE_PROMETHEUS_METRICS"`
	// DownloadProxy routes version package downloads through an HTTP proxy (e.g. a local Dragonfly
	// dfdaemon) for P2P distribution across large fleets. Empty means direct S3 download.
	DownloadProxy string `yaml:"downloadProxy" mapstructure:"downloadProxy" env:"GP_AGENT_DOWNLOAD_PROXY"`
	// UpstreamProxy is the egress proxy builder nodes use to reach upstream sources (GitHub, PaperMC,
	// Steam) only; platform S3 and apiserver traffic never goes through it.
	UpstreamProxy string `yaml:"upstreamProxy" mapstructure:"upstreamProxy" env:"GP_AGENT_UPSTREAM_PROXY"`
	// CacheMaxBytes bounds the node-local .app-cache size (0 = bounded only by CacheMinFreePercent).
	CacheMaxBytes int64 `yaml:"cacheMaxBytes" mapstructure:"cacheMaxBytes" env:"GP_AGENT_CACHE_MAX_BYTES"`
	// CacheMinFreePercent evicts least-recently-used cached packages while the work-dir filesystem
	// has less free space than this percentage. Packages used by local instances are never evicted.
	CacheMinFreePercent int `yaml:"cacheMinFreePercent" mapstructure:"cacheMinFreePercent" env:"GP_AGENT_CACHE_MIN_FREE_PERCENT"`
}

// AgentResourceReservation is host overhead excluded from inventory admission.
// Explicit zero is finite zero; omitted config receives conservative defaults.
type AgentResourceReservation struct {
	CPUMillicores int64
	MemoryBytes   int64
	StorageBytes  int64
}

func DefaultAgentResourceReservation() AgentResourceReservation {
	return AgentResourceReservation{CPUMillicores: 500, MemoryBytes: 512 << 20, StorageBytes: 5 << 30}
}

func (r AgentResourceReservation) Validate() error {
	if r.CPUMillicores < 0 || r.MemoryBytes < 0 || r.StorageBytes < 0 {
		return errors.New("agent.reservedResources must contain non-negative integer millicores and bytes")
	}
	return nil
}

func NewAgentConfig() *AgentConfig {
	c := &AgentConfig{PerInstancePrometheusMetrics: true}
	c.Default()
	return c
}

func (c *AgentConfig) Default() {
	if c.ReservedResources == nil {
		reserved := DefaultAgentResourceReservation()
		c.ReservedResources = &reserved
	}
	_ = syncAgentReservedQuantities(c)
	if c.NodeName == "" {
		c.NodeName = "node-local-01"
	}
	if c.Region == "" {
		c.Region = "ap-east-1"
	}
	if c.Addr == "" {
		c.Addr = "0.0.0.0:8082"
	}
	if c.APIServerURL == "" {
		c.APIServerURL = "http://127.0.0.1:8080"
	}
	if c.Token == "" {
		c.Token = "dev-agent-token"
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = 10 * time.Second
	}
	if c.CacheMinFreePercent <= 0 {
		c.CacheMinFreePercent = 5
	}
	if c.WorkDir == "" {
		c.WorkDir = "/tmp/gamepanel/instances"
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 5 * time.Second
	}
}

func (c *AgentConfig) Validate() error {
	if strings.TrimSpace(c.NodeName) == "" {
		return errors.New("agent.nodeName is required")
	}
	if strings.TrimSpace(c.Addr) == "" {
		return errors.New("agent.addr is required")
	}
	if strings.TrimSpace(c.APIServerURL) == "" {
		return errors.New("agent.apiserverURL is required")
	}
	if strings.TrimSpace(c.Token) == "" {
		return errors.New("agent.token is required")
	}
	if c.HeartbeatInterval <= 0 {
		return errors.New("agent.heartbeatInterval must be positive")
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("agent.shutdownTimeout must be positive")
	}
	if c.ReservedResources == nil {
		return errors.New("agent.reservedResources is required")
	}
	if err := syncAgentReservedQuantities(c); err != nil {
		return err
	}
	return c.ReservedResources.Validate()
}

func syncAgentReservedQuantities(c *AgentConfig) error {
	if c == nil || c.ReservedResources == nil {
		return nil
	}
	if rawCPU := strings.TrimSpace(c.ReservedCPU); rawCPU != "" {
		millicores, err := corev1.ParseCPU(corev1.Quantity(rawCPU))
		if err != nil {
			return fmt.Errorf("agent.reservedCPU: %w", err)
		}
		c.ReservedResources.CPUMillicores = millicores
	}
	if rawMem := strings.TrimSpace(c.ReservedMemory); rawMem != "" {
		bytesVal, err := corev1.ParseBytes(corev1.Quantity(rawMem))
		if err != nil {
			return fmt.Errorf("agent.reservedMemory: %w", err)
		}
		c.ReservedResources.MemoryBytes = bytesVal
	}
	if rawStorage := strings.TrimSpace(c.ReservedStorage); rawStorage != "" {
		bytesVal, err := corev1.ParseBytes(corev1.Quantity(rawStorage))
		if err != nil {
			return fmt.Errorf("agent.reservedStorage: %w", err)
		}
		c.ReservedResources.StorageBytes = bytesVal
	}
	return nil
}

func (c *AgentConfig) Port() int {
	if c.Addr != "" {
		if _, portStr, err := net.SplitHostPort(c.Addr); err == nil {
			if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
				return p
			}
		}
		if p, err := strconv.Atoi(c.Addr); err == nil && p > 0 {
			return p
		}
	}
	return 8082
}

// BootstrapConfig configures initial platform bootstrap credentials.
type BootstrapConfig struct {
	AdminUser     string `yaml:"adminUser" mapstructure:"adminUser" env:"GP_BOOTSTRAP_ADMIN"`
	AdminEmail    string `yaml:"adminEmail" mapstructure:"adminEmail" env:"GP_BOOTSTRAP_EMAIL"`
	AdminPassword string `yaml:"adminPassword" mapstructure:"adminPassword" env:"GP_BOOTSTRAP_PASSWORD"`
}

func (c *BootstrapConfig) Default() {
	if c.AdminUser == "" {
		c.AdminUser = "admin"
	}
	if c.AdminEmail == "" {
		c.AdminEmail = "admin@example.com"
	}
	if c.AdminPassword == "" {
		c.AdminPassword = "P@88w0rd"
	}
}

// ClientConfig configures the declarative operations API client.
type ClientConfig struct {
	ServerURL string        `yaml:"serverURL" mapstructure:"serverURL" env:"GP_CLIENT_SERVER_URL"`
	Token     string        `yaml:"token" mapstructure:"token" env:"GP_CLIENT_TOKEN"`
	Tenant    string        `yaml:"tenant" mapstructure:"tenant" env:"GP_TENANT"`
	Timeout   time.Duration `yaml:"timeout" mapstructure:"timeout" env:"GP_CLIENT_TIMEOUT"`
}

func NewClientConfig() *ClientConfig {
	c := &ClientConfig{}
	c.Default()
	return c
}

func (c *ClientConfig) Default() {
	if c.ServerURL == "" {
		if v := os.Getenv("GP_SERVER_URL"); v != "" {
			c.ServerURL = v
		} else {
			c.ServerURL = "http://127.0.0.1:8080"
		}
	}
	if c.Token == "" {
		if v := os.Getenv("GP_TOKEN"); v != "" {
			c.Token = v
		}
	}
	if c.Tenant == "" {
		c.Tenant = "default"
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
}

func (c *ClientConfig) Validate() error {
	if strings.TrimSpace(c.ServerURL) == "" {
		return errors.New("client.serverURL is required")
	}
	u, err := url.Parse(c.ServerURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("client.serverURL %q must be a valid absolute URL", c.ServerURL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("client.serverURL scheme must be http or https, got %q", u.Scheme)
	}
	if c.Timeout <= 0 {
		return errors.New("client.timeout must be a positive duration")
	}
	return nil
}

// DefaultNetworkDriver names the built-in host port-pool network driver.
const DefaultNetworkDriver = "hostport"

// NetworkConfig configures dynamic host port-pool allocation (pkg/infra/network).
type NetworkConfig struct {
	Driver        string `yaml:"driver" mapstructure:"driver" env:"GP_NETWORK_DRIVER"`
	PortRangeMin  int32  `yaml:"portRangeMin" mapstructure:"portRangeMin" env:"GP_NETWORK_PORT_RANGE_MIN"`
	PortRangeMax  int32  `yaml:"portRangeMax" mapstructure:"portRangeMax" env:"GP_NETWORK_PORT_RANGE_MAX"`
	MaxPortProbes int    `yaml:"maxPortProbes" mapstructure:"maxPortProbes" env:"GP_NETWORK_MAX_PORT_PROBES"`
}

func NewNetworkConfig() *NetworkConfig {
	c := &NetworkConfig{}
	c.Default()
	return c
}

func (c *NetworkConfig) Default() {
	if strings.TrimSpace(c.Driver) == "" {
		c.Driver = DefaultNetworkDriver
	}
	if c.PortRangeMin == 0 {
		c.PortRangeMin = 20000
	}
	if c.PortRangeMax == 0 {
		c.PortRangeMax = 29999
	}
	if c.MaxPortProbes == 0 {
		c.MaxPortProbes = 256
	}
}

func (c *NetworkConfig) Validate() error {
	if strings.TrimSpace(c.Driver) == "" {
		return errors.New("network.driver is required")
	}
	if c.PortRangeMin < 1 || c.PortRangeMax > 65535 || c.PortRangeMin > c.PortRangeMax {
		return fmt.Errorf("network port range [%d, %d] must be within 1..65535 with portRangeMin <= portRangeMax", c.PortRangeMin, c.PortRangeMax)
	}
	if c.MaxPortProbes < 1 || c.MaxPortProbes > 256 {
		return fmt.Errorf("network.maxPortProbes must be within 1..256, got %d", c.MaxPortProbes)
	}
	return nil
}

// S3Config configures S3-compatible object storage for platform resources and instance backups.
type S3Config struct {
	Endpoint       string        `yaml:"endpoint" mapstructure:"endpoint" env:"GP_S3_ENDPOINT"`
	PublicEndpoint string        `yaml:"publicEndpoint" mapstructure:"publicEndpoint" env:"GP_S3_PUBLIC_ENDPOINT"`
	Region         string        `yaml:"region" mapstructure:"region" env:"GP_S3_REGION"`
	Bucket         string        `yaml:"bucket" mapstructure:"bucket" env:"GP_S3_BUCKET"`
	AccessKey      string        `yaml:"accessKey" mapstructure:"accessKey" env:"GP_S3_ACCESS_KEY"`
	SecretKey      string        `yaml:"secretKey" mapstructure:"secretKey" env:"GP_S3_SECRET_KEY"`
	UsePathStyle   bool          `yaml:"usePathStyle" mapstructure:"usePathStyle" env:"GP_S3_USE_PATH_STYLE"`
	PresignTTL     time.Duration `yaml:"presignTTL" mapstructure:"presignTTL" env:"GP_S3_PRESIGN_TTL"`
}

func NewS3Config() *S3Config {
	c := &S3Config{}
	c.Default()
	return c
}

func (c *S3Config) Default() {
	if strings.TrimSpace(c.Region) == "" {
		c.Region = "us-east-1"
	}
	if strings.TrimSpace(c.Bucket) == "" {
		c.Bucket = "gamepanel-backups"
	}
	if c.PresignTTL == 0 {
		c.PresignTTL = 15 * time.Minute
	}
}

func (c *S3Config) Validate() error {
	if c.PresignTTL < 0 {
		return errors.New("s3.presignTTL must not be negative")
	}
	return nil
}

// IsConfigured returns true when S3 endpoint and credentials are set.
func (c *S3Config) IsConfigured() bool {
	return (strings.TrimSpace(c.Endpoint) != "" || strings.TrimSpace(c.PublicEndpoint) != "") &&
		strings.TrimSpace(c.AccessKey) != "" &&
		strings.TrimSpace(c.SecretKey) != ""
}

// ObservabilityConfig configures external Prometheus, Alertmanager, VictoriaLogs endpoints, and API audit level.
type ObservabilityConfig struct {
	PrometheusURL   string `yaml:"prometheusURL" mapstructure:"prometheusURL" env:"GP_PROMETHEUS_URL"`
	AlertmanagerURL string `yaml:"alertmanagerURL" mapstructure:"alertmanagerURL" env:"GP_ALERTMANAGER_URL"`
	VictoriaLogsURL string `yaml:"victoriaLogsURL" mapstructure:"victoriaLogsURL" env:"GP_VICTORIALOGS_URL"`
	AuditLevel      string `yaml:"auditLevel" mapstructure:"auditLevel" env:"GP_AUDIT_LEVEL"`
}

func (c *ObservabilityConfig) Default() {
	if strings.TrimSpace(c.AuditLevel) == "" {
		c.AuditLevel = corev1.AuditLevelRequestResponse
	}
}

func (c *ObservabilityConfig) Validate() error {
	for name, raw := range map[string]string{
		"observability.prometheusURL":   c.PrometheusURL,
		"observability.alertmanagerURL": c.AlertmanagerURL,
		"observability.victoriaLogsURL": c.VictoriaLogsURL,
	} {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		u, err := url.Parse(trimmed)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%s %q must be a valid http(s) URL", name, raw)
		}
	}
	switch strings.TrimSpace(c.AuditLevel) {
	case "", corev1.AuditLevelMetadata, corev1.AuditLevelRequest, corev1.AuditLevelRequestResponse:
	default:
		return fmt.Errorf("observability.auditLevel must be one of %s|%s|%s, got %q", corev1.AuditLevelMetadata, corev1.AuditLevelRequest, corev1.AuditLevelRequestResponse, c.AuditLevel)
	}
	return nil
}

// AddonMirrorConfig configures the pull-through game mod & plugin mirror catalog (Steam Workshop & Modrinth).
type AddonMirrorConfig struct {
	SteamWebAPIKey    string        `yaml:"steamWebAPIKey" mapstructure:"steamWebAPIKey" env:"GP_ADDON_MIRROR_STEAM_WEB_API_KEY"`
	UpstreamProxyURL  string        `yaml:"upstreamProxyURL" mapstructure:"upstreamProxyURL" env:"GP_ADDON_MIRROR_UPSTREAM_PROXY_URL"`
	ExternalEndpoint  string        `yaml:"externalEndpoint" mapstructure:"externalEndpoint" env:"GP_ADDON_MIRROR_EXTERNAL_ENDPOINT"`
	ImageHostMirrors  []string      `yaml:"imageHostMirrors" mapstructure:"imageHostMirrors" env:"GP_ADDON_MIRROR_IMAGE_HOST_MIRRORS" envSeparator:","`
	AllowedImageHosts []string      `yaml:"allowedImageHosts" mapstructure:"allowedImageHosts" env:"GP_ADDON_MIRROR_ALLOWED_IMAGE_HOSTS" envSeparator:","`
	CacheTTL          time.Duration `yaml:"cacheTTL" mapstructure:"cacheTTL" env:"GP_ADDON_MIRROR_CACHE_TTL"`
	Timeout           time.Duration `yaml:"timeout" mapstructure:"timeout" env:"GP_ADDON_MIRROR_TIMEOUT"`
}

func (c *AddonMirrorConfig) Default() {
	if strings.TrimSpace(c.SteamWebAPIKey) == "" {
		c.SteamWebAPIKey = strings.TrimSpace(os.Getenv("STEAM_WEB_API_KEY"))
	}
	if c.CacheTTL <= 0 {
		c.CacheTTL = 15 * time.Minute
	}
	if c.Timeout <= 0 {
		c.Timeout = 15 * time.Second
	}
}

func (c *AddonMirrorConfig) Validate() error {
	if raw := strings.TrimSpace(c.UpstreamProxyURL); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") || u.Host == "" {
			return fmt.Errorf("addonMirror.upstreamProxyURL %q must be a valid http(s)/socks5 URL", raw)
		}
	}
	if raw := strings.TrimSpace(c.ExternalEndpoint); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("addonMirror.externalEndpoint %q must be a valid http(s) URL", raw)
		}
	}
	for _, entry := range c.ImageHostMirrors {
		if err := validateImageHostMirrorEntry(entry); err != nil {
			return err
		}
	}
	for _, pattern := range c.AllowedImageHosts {
		if err := validateImageHostPattern(pattern, "addonMirror.allowedImageHosts"); err != nil {
			return err
		}
	}
	return nil
}

func validateImageHostMirrorEntry(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	source, target, ok := strings.Cut(trimmed, "=")
	source, target = strings.TrimSpace(source), strings.TrimSpace(target)
	if !ok || source == "" || target == "" {
		return fmt.Errorf("addonMirror.imageHostMirrors entry %q must use source=target format", raw)
	}
	if err := validateImageHostPattern(source, "addonMirror.imageHostMirrors source"); err != nil {
		return err
	}
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("addonMirror.imageHostMirrors target %q must be a valid http(s) base URL", target)
		}
		return nil
	}
	if strings.ContainsAny(target, "/?# \t\r\n") {
		return fmt.Errorf("addonMirror.imageHostMirrors target %q must be a valid hostname or http(s) base URL", target)
	}
	return nil
}

func validateImageHostPattern(raw, field string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if strings.Contains(trimmed, "://") || strings.ContainsAny(trimmed, "/?# \t\r\n") || trimmed == "." {
		return fmt.Errorf("%s %q must be a hostname or .domain suffix", field, raw)
	}
	return nil
}

// RegistryConfig configures the Harbor-style unified artifact & addon registry (registry.gamepanel.io/v1).
type RegistryConfig struct {
	Endpoint           string        `yaml:"endpoint" mapstructure:"endpoint" env:"GP_REGISTRY_ENDPOINT"`
	Token              string        `yaml:"token" mapstructure:"token" env:"GP_REGISTRY_TOKEN"`
	WorkerToken        string        `yaml:"workerToken" mapstructure:"workerToken" env:"GP_REGISTRY_WORKER_TOKEN"`
	WorkerAddr         string        `yaml:"workerAddr" mapstructure:"workerAddr" env:"GP_REGISTRY_WORKER_ADDR"`
	WorkerWorkDir      string        `yaml:"workerWorkDir" mapstructure:"workerWorkDir" env:"GP_REGISTRY_WORKER_WORK_DIR"`
	WorkerPollInterval time.Duration `yaml:"workerPollInterval" mapstructure:"workerPollInterval" env:"GP_REGISTRY_WORKER_POLL_INTERVAL"`
	WorkerLeaseTTL     time.Duration `yaml:"workerLeaseTTL" mapstructure:"workerLeaseTTL" env:"GP_REGISTRY_WORKER_LEASE_TTL"`
	WorkerConcurrency  int           `yaml:"workerConcurrency" mapstructure:"workerConcurrency" env:"GP_REGISTRY_WORKER_CONCURRENCY"`
	SteamCMDBin        string        `yaml:"steamcmdBin" mapstructure:"steamcmdBin" env:"GP_REGISTRY_STEAMCMD_BIN"`
	SteamWebAPIKey     string        `yaml:"steamWebAPIKey" mapstructure:"steamWebAPIKey" env:"GP_REGISTRY_STEAM_WEB_API_KEY"`
	UpstreamProxyURL   string        `yaml:"upstreamProxyURL" mapstructure:"upstreamProxyURL" env:"GP_REGISTRY_UPSTREAM_PROXY_URL"`
	CatalogCacheTTL    time.Duration `yaml:"catalogCacheTTL" mapstructure:"catalogCacheTTL" env:"GP_REGISTRY_CATALOG_CACHE_TTL"`
	BlobPresignTTL     time.Duration `yaml:"blobPresignTTL" mapstructure:"blobPresignTTL" env:"GP_REGISTRY_BLOB_PRESIGN_TTL"`
	Timeout            time.Duration `yaml:"timeout" mapstructure:"timeout" env:"GP_REGISTRY_TIMEOUT"`
}

func (c *RegistryConfig) Default() {
	if strings.TrimSpace(c.WorkerToken) == "" {
		c.WorkerToken = "dev-registry-worker-token"
	}
	if strings.TrimSpace(c.WorkerAddr) == "" {
		c.WorkerAddr = "0.0.0.0:8084"
	}
	if strings.TrimSpace(c.WorkerWorkDir) == "" {
		c.WorkerWorkDir = "/tmp/gamepanel/registry-worker"
	}
	if c.WorkerPollInterval <= 0 {
		c.WorkerPollInterval = 10 * time.Second
	}
	if c.WorkerLeaseTTL <= 0 {
		c.WorkerLeaseTTL = 90 * time.Second
	}
	if c.WorkerConcurrency <= 0 {
		c.WorkerConcurrency = 2
	}
	if strings.TrimSpace(c.SteamCMDBin) == "" {
		c.SteamCMDBin = "steamcmd"
	}
	if strings.TrimSpace(c.SteamWebAPIKey) == "" {
		c.SteamWebAPIKey = strings.TrimSpace(os.Getenv("STEAM_WEB_API_KEY"))
	}
	if c.CatalogCacheTTL <= 0 {
		c.CatalogCacheTTL = 15 * time.Minute
	}
	if c.BlobPresignTTL <= 0 {
		c.BlobPresignTTL = 30 * time.Minute
	}
	if c.Timeout <= 0 {
		c.Timeout = 15 * time.Second
	}
}

func (c *RegistryConfig) Validate() error {
	if raw := strings.TrimSpace(c.Endpoint); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("registry.endpoint %q must be a valid http(s) URL", raw)
		}
	}
	if raw := strings.TrimSpace(c.UpstreamProxyURL); raw != "" {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") || u.Host == "" {
			return fmt.Errorf("registry.upstreamProxyURL %q must be a valid http(s)/socks5 URL", raw)
		}
	}
	return nil
}

var envPlaceholderPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// ExpandEnv expands ${VAR} and ${VAR:-default} placeholders using os.LookupEnv.
func ExpandEnv(input string) string {
	return envPlaceholderPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := envPlaceholderPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		if val, ok := os.LookupEnv(parts[1]); ok && val != "" {
			return val
		}
		if len(parts) >= 3 {
			return parts[2]
		}
		return ""
	})
}

// AddFlags registers the standard --config / -c flag on a spf13/pflag FlagSet.
func AddFlags(fs *pflag.FlagSet, configPath *string) {
	fs.StringVarP(configPath, "config", "c", os.Getenv("GP_CONFIG_FILE"), "Path to configuration file")
	addAgentFlags(fs)
	addControllerFlags(fs)
}

// NewCommand constructs a spf13/cobra Command that parses CLI flags (--config / -c),
// loads the unified *Config via spf13/viper + caarlos0/env/v11, and invokes runE(ctx, cfg)
// with graceful SIGINT/SIGTERM cancellation.
func NewCommand(use, short string, runE func(ctx context.Context, cfg *Config) error) *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:          use,
		Short:        short,
		SilenceUsage: true,
		PersistentPreRunE: func(command *cobra.Command, _ []string) error {
			_, err := loadFile(configPath, command.Flags())
			return err
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if runE == nil {
				return nil
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return runE(ctx, Get())
		},
	}
	AddFlags(cmd.PersistentFlags(), &configPath)
	return cmd
}

// Load parses CLI flags via spf13/pflag and loads the unified *Config via spf13/viper and caarlos0/env/v11.
func Load(args []string) (*Config, error) {
	fs := pflag.NewFlagSet("config", pflag.ContinueOnError)
	var configPath string
	AddFlags(fs, &configPath)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return loadFile(configPath, fs)
}

// LoadFile loads configuration from configPath using spf13/viper and caarlos0/env/v11
// into a single unified *Config object, validates it, updates the global Config, and returns it.
func LoadFile(configPath string) (*Config, error) {
	return loadFile(configPath, nil)
}

func loadFile(configPath string, flags *pflag.FlagSet) (*Config, error) {
	cfg := New()

	v := viper.New()
	v.SetConfigType("yaml")
	if strings.TrimSpace(configPath) != "" {
		raw, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("load config file %q: %w", configPath, err)
		}
		expanded := ExpandEnv(string(raw))
		if err := v.ReadConfig(strings.NewReader(expanded)); err != nil {
			return nil, fmt.Errorf("parse config file %q: %w", configPath, err)
		}
	}

	decoderOpt := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	))
	yamlTagOpt := func(dc *mapstructure.DecoderConfig) {
		dc.TagName = "yaml"
	}

	if err := v.Unmarshal(cfg, decoderOpt, yamlTagOpt); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse env config: %w", err)
	}
	if err := applyAgentFlags(flags, &cfg.Agent); err != nil {
		return nil, err
	}
	if err := applyControllerFlags(flags, &cfg.Controller); err != nil {
		return nil, err
	}
	cfg.Default()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	globalMu.RLock()
	hooks := append([]func(*Config) error(nil), onLoadHooks...)
	globalMu.RUnlock()
	for _, fn := range hooks {
		if err := fn(cfg); err != nil {
			return nil, err
		}
	}
	Set(cfg)
	return cfg, nil
}

func addAgentFlags(fs *pflag.FlagSet) {
	fs.String("agent-reserved-cpu", "", "Host CPU quantity reserved for system overhead (e.g. 2, 500m, 0)")
	fs.String("agent-reserved-memory", "", "Host memory quantity reserved for system overhead (e.g. 8Gi, 512Mi, 0)")
	fs.String("agent-reserved-storage", "", "Host storage quantity reserved for system overhead (e.g. 5Gi, 0)")
	fs.Bool("agent-per-instance-prometheus-metrics", true, "Export per-instance Prometheus series, including custom labels/items; node metrics and API usage remain available when disabled")
}

func applyAgentFlags(fs *pflag.FlagSet, agent *AgentConfig) error {
	if fs == nil {
		return nil
	}
	if agent.ReservedResources == nil {
		reserved := DefaultAgentResourceReservation()
		agent.ReservedResources = &reserved
	}
	for _, flag := range []struct {
		name   string
		target *string
	}{
		{"agent-reserved-cpu", &agent.ReservedCPU},
		{"agent-reserved-memory", &agent.ReservedMemory},
		{"agent-reserved-storage", &agent.ReservedStorage},
	} {
		if fs.Changed(flag.name) {
			value, err := fs.GetString(flag.name)
			if err != nil {
				return err
			}
			*flag.target = value
		}
	}
	if fs.Changed("agent-per-instance-prometheus-metrics") {
		value, err := fs.GetBool("agent-per-instance-prometheus-metrics")
		if err != nil {
			return err
		}
		agent.PerInstancePrometheusMetrics = value
	}
	return nil
}

func addControllerFlags(fs *pflag.FlagSet) {
	defaults := NewControllerConfig()
	fs.Int("controller-workers", defaults.Workers, "Maximum concurrent reconciliations per controller across MQ and resync")
	fs.Duration("controller-resync-period", defaults.ResyncPeriod, "Interval between controller resync passes")
	fs.Int("controller-resync-pages", defaults.ResyncPages, "Maximum cursor pages scanned per controller in a resync pass")
	fs.Int("controller-resync-event-budget", defaults.ResyncEventBudget, "Maximum events dispatched per controller in a resync pass; remaining page events resume next pass")
	fs.Duration("controller-resync-timeout", defaults.ResyncTimeout, "Time budget for one controller's resync scanning and reconciliation")
	fs.Duration("controller-resync-lease-ttl", defaults.ResyncLeaseTTL, "Redis resync leadership lease duration; renewed every third of this duration")
	fs.Duration("controller-node-grace-duration", defaults.NodeGraceDuration, "Grace duration before a node heartbeat is considered lost")
	fs.String("controller-addr", defaults.Addr, "Controller-manager health and metrics listen address")
}

func applyControllerFlags(fs *pflag.FlagSet, controller *ControllerConfig) error {
	if fs == nil {
		return nil
	}
	for _, flag := range []struct {
		name   string
		target *int
	}{
		{"controller-workers", &controller.Workers},
		{"controller-resync-pages", &controller.ResyncPages},
		{"controller-resync-event-budget", &controller.ResyncEventBudget},
	} {
		if fs.Changed(flag.name) {
			value, err := fs.GetInt(flag.name)
			if err != nil {
				return err
			}
			*flag.target = value
		}
	}
	for _, flag := range []struct {
		name   string
		target *time.Duration
	}{
		{"controller-resync-period", &controller.ResyncPeriod},
		{"controller-resync-timeout", &controller.ResyncTimeout},
		{"controller-resync-lease-ttl", &controller.ResyncLeaseTTL},
		{"controller-node-grace-duration", &controller.NodeGraceDuration},
	} {
		if fs.Changed(flag.name) {
			value, err := fs.GetDuration(flag.name)
			if err != nil {
				return err
			}
			*flag.target = value
		}
	}
	if fs.Changed("controller-addr") {
		value, err := fs.GetString("controller-addr")
		if err != nil {
			return err
		}
		controller.Addr = value
	}
	return nil
}
