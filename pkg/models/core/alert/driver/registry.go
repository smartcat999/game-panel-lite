package driver

import (
	"sort"
	"strings"
	"sync"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
)

var (
	driversMu sync.RWMutex
	drivers   = make(map[string]ChannelDriver)
)

// RegisterChannelDriver registers a ChannelDriver keyed by its canonical Type().
func RegisterChannelDriver(drv ChannelDriver) {
	if drv == nil {
		return
	}
	key := normalizeType(drv.Type())
	if key == "" {
		return
	}
	driversMu.Lock()
	defer driversMu.Unlock()
	drivers[key] = drv
}

// LookupChannelDriver returns the registered ChannelDriver for typeName.
func LookupChannelDriver(typeName string) (ChannelDriver, bool) {
	key := normalizeType(typeName)
	driversMu.RLock()
	defer driversMu.RUnlock()
	drv, ok := drivers[key]
	return drv, ok
}

// DetectChannelType infers the provider type from rawURL using registered drivers in ascending Priority() order.
func DetectChannelType(rawURL string) string {
	for _, drv := range orderedDrivers() {
		if drv.MatchURL(rawURL) {
			return drv.Type()
		}
	}
	if IsHTTPURL(rawURL) {
		return corev1.ChannelTypeWebhook
	}
	return corev1.ChannelTypeShoutrrr
}

// ResolveDriver selects the ChannelDriver responsible for delivering to ch/rawURL.
func ResolveDriver(ch corev1.AlertChannel, rawURL string) (ChannelDriver, bool) {
	explicit := normalizeType(ch.Type)
	if !IsHTTPURL(rawURL) {
		return resolveNonHTTPDriver(explicit, rawURL)
	}
	if explicit != "" && explicit != corev1.ChannelTypeWebhook && explicit != corev1.ChannelTypeShoutrrr {
		if drv, ok := LookupChannelDriver(explicit); ok {
			return drv, true
		}
	}
	for _, drv := range orderedDrivers() {
		if drv.MatchURL(rawURL) {
			return drv, true
		}
	}
	return LookupChannelDriver(corev1.ChannelTypeWebhook)
}

func resolveNonHTTPDriver(explicit, rawURL string) (ChannelDriver, bool) {
	for _, drv := range orderedDrivers() {
		if drv.Priority() < 100 && drv.MatchURL(rawURL) {
			return drv, true
		}
	}
	if explicit != "" && explicit != corev1.ChannelTypeWebhook {
		if drv, ok := LookupChannelDriver(explicit); ok {
			return drv, true
		}
	}
	return LookupChannelDriver(corev1.ChannelTypeShoutrrr)
}

func orderedDrivers() []ChannelDriver {
	driversMu.RLock()
	list := make([]ChannelDriver, 0, len(drivers))
	for _, drv := range drivers {
		list = append(list, drv)
	}
	driversMu.RUnlock()
	sort.Slice(list, func(i, j int) bool {
		if list[i].Priority() != list[j].Priority() {
			return list[i].Priority() < list[j].Priority()
		}
		return list[i].Type() < list[j].Type()
	})
	return list
}

// IsHTTPURL reports whether rawURL begins with http:// or https:// (case-insensitive).
func IsHTTPURL(rawURL string) bool {
	u := strings.ToLower(strings.TrimSpace(rawURL))
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

func normalizeType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
