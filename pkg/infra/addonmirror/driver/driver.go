package driver

import (
	"context"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Capabilities declares which high-level upstream workflows a provider Driver supports for a given Source.
type Capabilities struct {
	Catalog        bool
	ServerPackages bool
}

// ResolveReleaseRequest parameterizes unified artifact/server release resolution across all upstream drivers.
type ResolveReleaseRequest struct {
	Package       string
	Version       string
	GameVersion   string
	SourceURL     string
	StrictVersion bool
	AllowBranches bool
}

// Runtime provides HTTP transport, caching, and endpoint configuration to upstream Driver implementations.
type Runtime interface {
	DoGetJSON(ctx context.Context, endpoint string, dst any) error
	DoGetBytes(ctx context.Context, endpoint string) ([]byte, error)
	DoPostFormJSON(ctx context.Context, endpoint string, form url.Values, dst any) error
	GetCachedJSON(ctx context.Context, key string, dst any) bool
	SetCachedJSON(ctx context.Context, key string, val any)
	SteamAPIKey() string
	SteamEndpoint(src Source) string
	ModrinthEndpoint(src Source) string
}

// Driver defines the pluggable upstream mirror provider contract implemented via init() self-registration.
type Driver interface {
	Name() string
	Capabilities(src Source) Capabilities
	ParseTokens(raw string) []string
	Search(ctx context.Context, rt Runtime, src Source, params SearchParams) (*Result, error)
	Detail(ctx context.Context, rt Runtime, src Source, id, gameVersion string) (*Item, error)
	Resolve(ctx context.Context, rt Runtime, src Source, tokens []string, gameVersion string) ([]Item, error)
	ListServerVersions(ctx context.Context, rt Runtime, src Source, pkgName string) ([]string, error)
	ResolveRelease(ctx context.Context, rt Runtime, src Source, req ResolveReleaseRequest) (*ResolvedRelease, error)
	ResolveLatestVersions(ctx context.Context, rt Runtime, src Source, pkgIDs []string) (map[string]string, error)
}

// DriverFactory instantiates a Driver.
type DriverFactory func() Driver

// HTTPSubDriver handles specialized HTTP-backed server release upstreams (e.g. Terraria, tModLoader)
// so ProviderHTTP does not hardcode product-specific URL sniffing switches.
type HTTPSubDriver interface {
	Name() string
	Match(endpoint, pkgName string) bool
	ListServerVersions(ctx context.Context, rt Runtime, endpoint, pkgName string) ([]string, error)
	ResolveRelease(ctx context.Context, rt Runtime, endpoint, pkgName, version string) (*ResolvedRelease, error)
}

var (
	driversMu      sync.RWMutex
	driverRegistry = make(map[string]DriverFactory)

	httpSubMu      sync.RWMutex
	httpSubDrivers []HTTPSubDriver
)

// Register is called by each upstream provider's init() to self-register with addonmirror.
func Register(provider string, factory DriverFactory) {
	if factory == nil {
		return
	}
	driversMu.Lock()
	defer driversMu.Unlock()
	driverRegistry[strings.ToLower(strings.TrimSpace(provider))] = factory
}

// Unregister removes a registered driver by provider name (primarily for test cleanup).
func Unregister(provider string) {
	driversMu.Lock()
	defer driversMu.Unlock()
	delete(driverRegistry, strings.ToLower(strings.TrimSpace(provider)))
}

// LookupDriver returns a newly instantiated Driver for the given upstream provider name.
func LookupDriver(provider string) (Driver, bool) {
	driversMu.RLock()
	f, ok := driverRegistry[strings.ToLower(strings.TrimSpace(provider))]
	driversMu.RUnlock()
	if !ok || f == nil {
		return nil, false
	}
	return f(), true
}

// Drivers returns all registered upstream provider names in sorted order.
func Drivers() []string {
	driversMu.RLock()
	names := make([]string, 0, len(driverRegistry))
	for k := range driverRegistry {
		names = append(names, k)
	}
	driversMu.RUnlock()
	sort.Strings(names)
	return names
}

// RegisterHTTPSubDriver registers a specialized HTTP sub-driver matched by endpoint or package name.
func RegisterHTTPSubDriver(sub HTTPSubDriver) {
	if sub == nil {
		return
	}
	httpSubMu.Lock()
	defer httpSubMu.Unlock()
	for i, existing := range httpSubDrivers {
		if existing.Name() == sub.Name() {
			httpSubDrivers[i] = sub
			return
		}
	}
	httpSubDrivers = append(httpSubDrivers, sub)
}

// LookupHTTPSubDriver returns the first registered HTTPSubDriver matching endpoint or package name.
func LookupHTTPSubDriver(endpoint, pkgName string) (HTTPSubDriver, bool) {
	httpSubMu.RLock()
	subs := slices.Clone(httpSubDrivers)
	httpSubMu.RUnlock()
	for _, sub := range subs {
		if sub.Match(endpoint, pkgName) {
			return sub, true
		}
	}
	return nil, false
}

// BaseDriver provides default no-op/unsupported implementations so concrete drivers only override what they support.
type BaseDriver struct{}

func (BaseDriver) Capabilities(Source) Capabilities { return Capabilities{} }
func (BaseDriver) ParseTokens(raw string) []string  { return SplitPlainTokens(raw, nil) }
func (BaseDriver) Search(context.Context, Runtime, Source, SearchParams) (*Result, error) {
	return nil, ErrUnsupportedSource
}
func (BaseDriver) Detail(context.Context, Runtime, Source, string, string) (*Item, error) {
	return nil, ErrUnsupportedSource
}
func (BaseDriver) Resolve(context.Context, Runtime, Source, []string, string) ([]Item, error) {
	return nil, ErrUnsupportedSource
}
func (BaseDriver) ListServerVersions(context.Context, Runtime, Source, string) ([]string, error) {
	return nil, ErrUnsupportedSource
}
func (BaseDriver) ResolveRelease(context.Context, Runtime, Source, ResolveReleaseRequest) (*ResolvedRelease, error) {
	return nil, ErrUnsupportedSource
}
func (BaseDriver) ResolveLatestVersions(context.Context, Runtime, Source, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

// SplitPlainTokens tokenizes comma/whitespace separated upstream identifiers while skipping raw HTTP URLs.
func SplitPlainTokens(raw string, transform func(string) (string, bool)) []string {
	var out []string
	for _, part := range strings.FieldsFunc(raw, isTokenSeparator) {
		low := strings.ToLower(part)
		if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
			continue
		}
		if transform != nil {
			var ok bool
			part, ok = transform(part)
			if !ok {
				continue
			}
		}
		trimmed := strings.TrimSpace(part)
		if trimmed != "" && !ContainsString(out, trimmed) {
			out = append(out, trimmed)
		}
	}
	return out
}

func isTokenSeparator(r rune) bool {
	return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
}
