package addonmirror

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var defaultAllowedImageHostSuffixes = []string{
	".steamusercontent.com",
	".steamstatic.com",
	".akamaihd.net",
	"cdn.modrinth.com",
	"raw.githubusercontent.com",
	"media.forgecdn.net",
}

type imageHostMirrorRule struct {
	source string
	target string
}

// SignImageProxyURL returns a local HMAC-signed proxy URL when rawURL belongs to an allowlisted host.
func (s *Service) SignImageProxyURL(prefix, rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" || strings.TrimSpace(prefix) == "" {
		return trimmed
	}
	if err := s.validateUpstreamImageURL(trimmed); err != nil {
		return ""
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte(trimmed))
	sig := s.computeImageHMAC(encoded)
	sep := "?"
	if strings.Contains(prefix, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%su=%s&sig=%s", prefix, sep, encoded, sig)
}

func (s *Service) computeImageHMAC(encodedURL string) string {
	secret := s.signingSecret()
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("addon-img:v1:"))
	_, _ = mac.Write([]byte(encodedURL))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

// FetchImage verifies the HMAC signature, checks the host/IP allowlist, and streams/caches the image.
func (s *Service) FetchImage(ctx context.Context, encodedURL, sig string) ([]byte, string, error) {
	if encodedURL == "" || sig == "" {
		return nil, "", ErrInvalidSignature
	}
	expected := s.computeImageHMAC(encodedURL)
	if !hmac.Equal([]byte(expected), []byte(strings.ToLower(strings.TrimSpace(sig)))) {
		return nil, "", ErrInvalidSignature
	}
	rawBytes, err := base64.RawURLEncoding.DecodeString(encodedURL)
	if err != nil {
		return nil, "", ErrInvalidRequest
	}
	targetURL := string(rawBytes)
	if err := s.validateUpstreamImageURL(targetURL); err != nil {
		return nil, "", err
	}
	cacheKey := "addonmirror:v1:img:" + expected
	if data, ctype, ok := s.loadCachedImage(ctx, cacheKey); ok {
		return data, ctype, nil
	}
	data, ctype, err := s.downloadUpstreamImage(ctx, targetURL)
	if err != nil {
		return nil, "", err
	}
	if len(data) <= maxCachedThumbnailBytes {
		s.saveCachedImage(ctx, cacheKey, data, ctype)
	}
	return data, ctype, nil
}

func (s *Service) downloadUpstreamImage(ctx context.Context, targetURL string) ([]byte, string, error) {
	var lastErr error
	for _, candidate := range s.candidateImageURLs(targetURL) {
		data, ctype, err := s.fetchSingleUpstreamImage(ctx, candidate)
		if err == nil {
			return data, ctype, nil
		}
		lastErr = err
	}
	return nil, "", lastErr
}

func (s *Service) candidateImageURLs(targetURL string) []string {
	trimmed := strings.TrimSpace(targetURL)
	if trimmed == "" {
		return nil
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return []string{trimmed}
	}
	candidates := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	addCandidate := func(val string) {
		if val == "" {
			return
		}
		if _, ok := seen[val]; ok {
			return
		}
		seen[val] = struct{}{}
		candidates = append(candidates, val)
	}
	for _, rule := range parseImageHostMirrors(s.imageHostMirrors()) {
		if matchesImageHostPattern(u.Hostname(), rule.source) {
			addCandidate(applyImageMirrorTarget(u, rule.target))
		}
	}
	addCandidate(trimmed)
	return candidates
}

func parseImageHostMirrors(raw []string) []imageHostMirrorRule {
	var rules []imageHostMirrorRule
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		source, target, ok := strings.Cut(trimmed, "=")
		source, target = strings.TrimSpace(source), strings.TrimSpace(target)
		if !ok || source == "" || target == "" {
			continue
		}
		rules = append(rules, imageHostMirrorRule{source: source, target: target})
	}
	return rules
}

func applyImageMirrorTarget(u *url.URL, target string) string {
	if u == nil {
		return ""
	}
	if strings.Contains(target, "://") {
		base, err := url.Parse(target)
		if err != nil || base.Host == "" {
			return ""
		}
		mirror := *u
		mirror.Scheme = base.Scheme
		mirror.Host = base.Host
		mirror.Path = strings.TrimRight(base.Path, "/") + u.Path
		return mirror.String()
	}
	mirror := *u
	mirror.Host = target
	return mirror.String()
}

func extractMirrorTargetHost(target string) string {
	if strings.Contains(target, "://") {
		if u, err := url.Parse(target); err == nil {
			return strings.ToLower(u.Hostname())
		}
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(target))
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

func (s *Service) fetchSingleUpstreamImage(ctx context.Context, targetURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, "", ErrInvalidRequest
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	resp, err := s.imageHTTPClient().Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("upstream image status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxProxiedImageBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxProxiedImageBytes {
		return nil, "", ErrInvalidRequest
	}
	ctype := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(strings.ToLower(ctype), "image/") {
		ctype = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(ctype), "image/") {
		return nil, "", ErrForbiddenImageHost
	}
	return data, ctype, nil
}

func (s *Service) validateUpstreamImageURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ErrForbiddenImageHost
	}
	host := strings.ToLower(u.Hostname())
	if s.opts.AllowLoopbackImg && (host == "127.0.0.1" || host == "localhost" || host == "::1") {
		return nil
	}
	if u.Scheme != "https" {
		return ErrForbiddenImageHost
	}
	for _, pattern := range defaultAllowedImageHostSuffixes {
		if matchesImageHostPattern(host, pattern) {
			return nil
		}
	}
	for _, pattern := range s.extraAllowedImageHosts() {
		if matchesImageHostPattern(host, pattern) {
			return nil
		}
	}
	return ErrForbiddenImageHost
}

func matchesImageHostPattern(host, pattern string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	p := strings.ToLower(strings.TrimSpace(pattern))
	if h == "" || p == "" {
		return false
	}
	if strings.HasPrefix(p, ".") {
		return strings.HasSuffix(h, p) || h == strings.TrimPrefix(p, ".")
	}
	return h == p
}

func newSafeImageDialContext(allowLoopback bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 8 * time.Second}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("resolve image host: %w", err)
		}
		for _, ipAddr := range ips {
			ip := ipAddr.IP
			if !allowLoopback && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()) {
				return nil, ErrForbiddenImageHost
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
}
