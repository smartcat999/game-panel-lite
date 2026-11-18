package addonmirror

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/smartcat999/game-panel-cloud/pkg/config"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
	httpdrv "github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/http"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/modrinth"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/papermc"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/purpur"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/steam"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/providers/vanilla"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/cache"
)

const (
	maxListedServerVersions = 40
	maxJSONGetAttempts      = 3
)

// Service provides stateless + Redis pull-through cached catalog browsing, resolution, and image proxying.
type Service struct {
	cache cache.Interface
	opts  Options

	mu        sync.Mutex
	lastProxy string
	lastTO    time.Duration
	apiCli    *http.Client
	imgCli    *http.Client
}

// New constructs an addonmirror Service backed by an optional cache.Interface.
func New(c cache.Interface, opts ...Options) *Service {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	return &Service{cache: c, opts: o}
}

// NewForRegistry constructs a Service and Source uniformly from registry Upstream/Repository parameters.
func NewForRegistry(c cache.Interface, regCfg config.RegistryConfig, p RegistryParams) (*Service, Source) {
	opts := Options{
		SteamWebAPIKey:   driver.FirstNonEmpty(p.APIKey, regCfg.SteamWebAPIKey),
		UpstreamProxyURL: driver.FirstNonEmpty(p.ProxyURL, regCfg.UpstreamProxyURL),
		CacheTTL:         regCfg.CatalogCacheTTL,
		Timeout:          regCfg.Timeout,
	}
	src := Source{
		Provider:            p.Provider,
		Endpoint:            strings.TrimSpace(p.Endpoint),
		Format:              p.Format,
		SteamAppID:          p.SteamAppID,
		ModrinthLoader:      p.ModrinthLoader,
		ModrinthProjectType: p.ModrinthProjectType,
		ImageProxyPrefix:    p.ImageProxyPrefix,
	}
	return New(c, opts), src
}

// SteamEndpoint returns the effective Steam API base URL for src.
func (s *Service) SteamEndpoint(src Source) string {
	if ep := strings.TrimRight(strings.TrimSpace(src.Endpoint), "/"); ep != "" {
		return ep
	}
	if strings.TrimSpace(s.opts.SteamAPIBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(s.opts.SteamAPIBaseURL), "/")
	}
	return driver.DefaultSteamAPIBaseURL
}

// ModrinthEndpoint returns the effective Modrinth API base URL for src.
func (s *Service) ModrinthEndpoint(src Source) string {
	if ep := strings.TrimRight(strings.TrimSpace(src.Endpoint), "/"); ep != "" {
		return ep
	}
	if strings.TrimSpace(s.opts.ModrinthBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(s.opts.ModrinthBaseURL), "/")
	}
	return driver.DefaultModrinthBaseURL
}

// SteamAPIKey returns the configured Steam Web API key.
func (s *Service) SteamAPIKey() string {
	if strings.TrimSpace(s.opts.SteamWebAPIKey) != "" {
		return strings.TrimSpace(s.opts.SteamWebAPIKey)
	}
	return strings.TrimSpace(config.Get().AddonMirror.SteamWebAPIKey)
}

func (s *Service) signingSecret() string {
	if strings.TrimSpace(s.opts.SigningSecret) != "" {
		return s.opts.SigningSecret
	}
	if sec := strings.TrimSpace(config.Get().Auth.JWTSecret); sec != "" {
		return sec
	}
	return "gamepanel-addon-mirror-default-key"
}

func (s *Service) cacheTTL() time.Duration {
	if s.opts.CacheTTL > 0 {
		return s.opts.CacheTTL
	}
	if ttl := config.Get().AddonMirror.CacheTTL; ttl > 0 {
		return ttl
	}
	return 15 * time.Minute
}

func (s *Service) imageHostMirrors() []string {
	if len(s.opts.ImageHostMirrors) > 0 {
		return s.opts.ImageHostMirrors
	}
	return config.Get().AddonMirror.ImageHostMirrors
}

func (s *Service) extraAllowedImageHosts() []string {
	var out []string
	out = append(out, config.Get().AddonMirror.AllowedImageHosts...)
	out = append(out, s.opts.AllowedImageHosts...)
	for _, rule := range parseImageHostMirrors(s.imageHostMirrors()) {
		if host := extractMirrorTargetHost(rule.target); host != "" {
			out = append(out, host)
		}
	}
	return out
}

func (s *Service) ensureClients() (*http.Client, *http.Client) {
	proxyStr := driver.FirstNonEmpty(s.opts.UpstreamProxyURL, config.Get().AddonMirror.UpstreamProxyURL)
	to := s.opts.Timeout
	if to <= 0 {
		to = config.Get().AddonMirror.Timeout
	}
	if to <= 0 {
		to = 15 * time.Second
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.apiCli != nil && s.imgCli != nil && s.lastProxy == proxyStr && s.lastTO == to {
		return s.apiCli, s.imgCli
	}
	apiTransport := &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 32, IdleConnTimeout: 60 * time.Second}
	imgTransport := &http.Transport{MaxIdleConns: 32, IdleConnTimeout: 60 * time.Second}
	if proxyStr != "" {
		if pu, err := url.Parse(proxyStr); err == nil {
			apiTransport.Proxy = http.ProxyURL(pu)
			imgTransport.Proxy = http.ProxyURL(pu)
		}
	} else {
		imgTransport.DialContext = newSafeImageDialContext(s.opts.AllowLoopbackImg)
	}
	s.apiCli = &http.Client{Timeout: to, Transport: apiTransport}
	s.imgCli = &http.Client{
		Timeout:   to,
		Transport: imgTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return ErrForbiddenImageHost
			}
			return s.validateUpstreamImageURL(req.URL.String())
		},
	}
	s.lastProxy, s.lastTO = proxyStr, to
	return s.apiCli, s.imgCli
}

func (s *Service) httpClient() *http.Client {
	api, _ := s.ensureClients()
	return api
}

// DownloadHTTPClient returns a streaming HTTP client (no total body timeout) honoring UpstreamProxyURL.
func (s *Service) DownloadHTTPClient() *http.Client {
	proxyStr := driver.FirstNonEmpty(
		s.opts.UpstreamProxyURL,
		config.Get().Registry.UpstreamProxyURL,
		config.Get().AddonMirror.UpstreamProxyURL,
	)
	tr := &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 16, IdleConnTimeout: 60 * time.Second}
	if proxyStr != "" {
		if pu, err := url.Parse(proxyStr); err == nil {
			tr.Proxy = http.ProxyURL(pu)
		}
	}
	return &http.Client{Transport: tr}
}

func (s *Service) imageHTTPClient() *http.Client {
	_, img := s.ensureClients()
	return img
}

// DoGetJSON performs a GET request expecting JSON, retrying up to 3 attempts on transient network or 5xx failure.
func (s *Service) DoGetJSON(ctx context.Context, endpoint string, dst any) error {
	var err error
	for attempt := range maxJSONGetAttempts {
		var retry bool
		retry, err = s.doGetJSONOnce(ctx, endpoint, dst)
		if err == nil || !retry || attempt+1 >= maxJSONGetAttempts || ctx.Err() != nil {
			return err
		}
		s.httpClient().CloseIdleConnections()
		timer := time.NewTimer(120 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return err
		case <-timer.C:
		}
	}
	return err
}

func (s *Service) doGetJSONOnce(ctx context.Context, endpoint string, dst any) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return true, sanitizeUpstreamHTTPError(endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return false, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode >= 500, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	return false, json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(dst)
}

func sanitizeUpstreamHTTPError(endpoint string, err error) error {
	if err == nil {
		return nil
	}
	host := ""
	if u, perr := url.Parse(endpoint); perr == nil && u.Host != "" {
		host = u.Host
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		if host == "" {
			if parsed, perr := url.Parse(ue.URL); perr == nil {
				host = parsed.Host
			}
		}
		err = ue.Err
	}
	if host != "" {
		return fmt.Errorf("upstream %s request failed: %w", host, err)
	}
	return err
}

// DoPostFormJSON sends a form-urlencoded POST request and decodes the JSON response into dst.
func (s *Service) DoPostFormJSON(ctx context.Context, endpoint string, form url.Values, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return sanitizeUpstreamHTTPError(endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(dst)
}

// SupportsCatalog reports whether src's registered Driver exposes browsable catalog search/detail/resolve.
func (s *Service) SupportsCatalog(src Source) bool {
	drv, ok := LookupDriver(src.Provider)
	return ok && drv.Capabilities(src).Catalog
}

// ListServerVersions queries official upstream versions for a server_package provider via its registered Driver.
func (s *Service) ListServerVersions(ctx context.Context, provider, endpoint, pkgName string) ([]string, error) {
	prov := strings.ToLower(strings.TrimSpace(provider))
	drv, ok := LookupDriver(prov)
	if !ok {
		return nil, ErrUnsupportedSource
	}
	src := Source{Provider: prov, Endpoint: strings.TrimSpace(endpoint), Format: FormatServerPackage}
	if !drv.Capabilities(src).ServerPackages {
		return nil, ErrUnsupportedSource
	}
	cacheKey := driver.CacheHashKey("server_versions", prov+"|"+src.Endpoint+"|"+strings.TrimSpace(pkgName))
	var cached []string
	if s.GetCachedJSON(ctx, cacheKey, &cached) && len(cached) > 0 {
		return cached, nil
	}
	vers, err := drv.ListServerVersions(ctx, s, src, pkgName)
	if err != nil {
		return nil, err
	}
	if len(vers) > maxListedServerVersions {
		vers = vers[:maxListedServerVersions]
	}
	if len(vers) > 0 {
		s.SetCachedJSON(ctx, cacheKey, vers)
	}
	return vers, nil
}

// ResolveRelease resolves a specific or latest upstream release using the provider's registered Driver.
func (s *Service) ResolveRelease(ctx context.Context, src Source, req ResolveReleaseRequest) (*ResolvedRelease, error) {
	drv, ok := LookupDriver(src.Provider)
	if !ok {
		return nil, ErrUnsupportedSource
	}
	return drv.ResolveRelease(ctx, s, src, req)
}

// ResolveLatestVersions batch-resolves latest upstream version strings via the provider's registered Driver.
func (s *Service) ResolveLatestVersions(ctx context.Context, src Source, pkgIDs []string) (map[string]string, error) {
	drv, ok := LookupDriver(src.Provider)
	if !ok || len(pkgIDs) == 0 {
		return map[string]string{}, nil
	}
	return drv.ResolveLatestVersions(ctx, s, src, pkgIDs)
}

// Search queries the upstream catalog (with Redis read-through caching) and rewrites image URLs.
func (s *Service) Search(ctx context.Context, src Source, params SearchParams) (*Result, error) {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.Limit <= 0 || params.Limit > 30 {
		params.Limit = 20
	}
	drv, ok := LookupDriver(src.Provider)
	if !ok || !drv.Capabilities(src).Catalog {
		return nil, ErrUnsupportedSource
	}
	res, err := drv.Search(ctx, s, src, params)
	if err != nil {
		return nil, err
	}
	s.rewriteResultImages(src.ImageProxyPrefix, res)
	return res, nil
}

// Detail fetches full description, gallery images, and required dependencies for one addon item.
func (s *Service) Detail(ctx context.Context, src Source, id, gameVersion string) (*Item, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil, ErrInvalidRequest
	}
	drv, ok := LookupDriver(src.Provider)
	if !ok || !drv.Capabilities(src).Catalog {
		return nil, ErrUnsupportedSource
	}
	item, err := drv.Detail(ctx, s, src, trimmed, gameVersion)
	if err != nil {
		return nil, err
	}
	s.rewriteItemImages(src.ImageProxyPrefix, item)
	return item, nil
}

// Resolve expands URLs (including Steam Workshop Collection URLs) or batch ID lists into normalized Items.
func (s *Service) Resolve(ctx context.Context, src Source, rawInput, gameVersion string) (*Result, error) {
	tokens := ParseInputTokens(src.Provider, rawInput)
	if len(tokens) == 0 {
		return &Result{Configured: true, Items: nil, Total: 0}, nil
	}
	drv, ok := LookupDriver(src.Provider)
	if !ok || !drv.Capabilities(src).Catalog {
		return nil, ErrUnsupportedSource
	}
	items, err := drv.Resolve(ctx, s, src, tokens, gameVersion)
	if err != nil {
		return nil, err
	}
	res := &Result{Configured: true, Total: len(items), Items: items}
	s.rewriteResultImages(src.ImageProxyPrefix, res)
	return res, nil
}

// ResolvePaperMC resolves a PaperMC server release version and download URL.
func (s *Service) ResolvePaperMC(ctx context.Context, endpoint, version string) (*ResolvedRelease, error) {
	return papermc.Resolve(ctx, s, endpoint, version)
}

// ResolvePurpur resolves a PurpurMC server release version and download URL.
func (s *Service) ResolvePurpur(ctx context.Context, endpoint, version string) (*ResolvedRelease, error) {
	return purpur.Resolve(ctx, s, endpoint, version)
}

// ResolveVanillaMC resolves an official Mojang Minecraft server jar via piston-meta.
func (s *Service) ResolveVanillaMC(ctx context.Context, endpoint, version string) (*ResolvedRelease, error) {
	return vanilla.Resolve(ctx, s, endpoint, version)
}

// ResolveHTTPServerRelease resolves specialized HTTP upstreams or generic URL/template releases.
func (s *Service) ResolveHTTPServerRelease(ctx context.Context, endpoint, pkgName, version, explicitURL string) (*ResolvedRelease, error) {
	return httpdrv.Resolve(ctx, s, endpoint, pkgName, version, explicitURL)
}

// ResolveModrinthVersion resolves a specific or latest Modrinth project version and its primary .jar download.
func (s *Service) ResolveModrinthVersion(ctx context.Context, src Source, slugOrID, requestedVersion, gameVersion string) (*ResolvedRelease, error) {
	return modrinth.Resolve(ctx, s, src, slugOrID, requestedVersion, gameVersion)
}

// ResolveSteamWorkshopItem queries live metadata for a Workshop ID.
func (s *Service) ResolveSteamWorkshopItem(ctx context.Context, appID int, workshopID string) (*Item, error) {
	return steam.ResolveWorkshopItem(ctx, s, Source{Provider: ProviderSteamWorkshop, SteamAppID: appID}, workshopID)
}

// ResolveSteamWorkshopItemWithSource queries live metadata for a Workshop ID using Source endpoint overrides.
func (s *Service) ResolveSteamWorkshopItemWithSource(ctx context.Context, src Source, workshopID string) (*Item, error) {
	return steam.ResolveWorkshopItem(ctx, s, src, workshopID)
}

func (s *Service) rewriteResultImages(prefix string, res *Result) {
	if res == nil || prefix == "" {
		return
	}
	for i := range res.Items {
		s.rewriteItemImages(prefix, &res.Items[i])
	}
}

var descInlineURLRe = regexp.MustCompile(`https?://[^\s"'<>)\]]+`)

func (s *Service) rewriteItemImages(prefix string, item *Item) {
	if item == nil || prefix == "" {
		return
	}
	if item.IconURL != "" {
		if signed := s.SignImageProxyURL(prefix, item.IconURL); signed != "" {
			item.IconURL = signed
		}
	}
	if len(item.GalleryURLs) > 0 {
		out := make([]string, 0, len(item.GalleryURLs))
		for _, g := range item.GalleryURLs {
			if signed := s.SignImageProxyURL(prefix, g); signed != "" {
				out = append(out, signed)
			}
		}
		item.GalleryURLs = out
	}
	if item.Description != "" {
		item.Description = s.rewriteDescriptionImages(prefix, item.Description)
	}
}

func (s *Service) rewriteDescriptionImages(prefix, desc string) string {
	return descInlineURLRe.ReplaceAllStringFunc(desc, func(rawURL string) string {
		low := strings.ToLower(rawURL)
		if strings.HasSuffix(low, ".jar") || strings.HasSuffix(low, ".zip") || strings.HasSuffix(low, ".mrpack") {
			return rawURL
		}
		if signed := s.SignImageProxyURL(prefix, rawURL); signed != "" {
			return signed
		}
		return rawURL
	})
}

// GetCachedJSON retrieves a cached JSON value for key into dst.
func (s *Service) GetCachedJSON(ctx context.Context, key string, dst any) bool {
	if s.cache == nil {
		return false
	}
	val, err := s.cache.Get(ctx, key)
	if err != nil || val == "" {
		return false
	}
	return json.Unmarshal([]byte(val), dst) == nil
}

// SetCachedJSON stores src as JSON under key using the Service's configured TTL.
func (s *Service) SetCachedJSON(ctx context.Context, key string, src any) {
	s.setCachedJSONWithTTL(ctx, key, src, s.cacheTTL())
}

func (s *Service) setCachedJSONWithTTL(ctx context.Context, key string, src any, ttl time.Duration) {
	if s.cache == nil {
		return
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return
	}
	_ = s.cache.Set(ctx, key, string(raw), ttl)
}

type cachedImageEnvelope struct {
	ContentType string `json:"ct"`
	DataB64     string `json:"b64"`
}

func (s *Service) loadCachedImage(ctx context.Context, key string) ([]byte, string, bool) {
	var env cachedImageEnvelope
	if !s.GetCachedJSON(ctx, key, &env) || env.DataB64 == "" {
		return nil, "", false
	}
	raw, err := base64.StdEncoding.DecodeString(env.DataB64)
	if err != nil || len(raw) == 0 {
		return nil, "", false
	}
	return raw, env.ContentType, true
}

func (s *Service) saveCachedImage(ctx context.Context, key string, data []byte, ctype string) {
	s.setCachedJSONWithTTL(ctx, key, cachedImageEnvelope{
		ContentType: ctype,
		DataB64:     base64.StdEncoding.EncodeToString(data),
	}, 2*time.Hour)
}
