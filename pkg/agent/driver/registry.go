package driver

import (
	"sort"
	"strings"
	"sync"
)

var (
	injectorsMu sync.RWMutex
	injectors   = make(map[string]AddonInjector)
)

// RegisterAddonInjector registers an AddonInjector implementation keyed by its Mode().
func RegisterAddonInjector(inj AddonInjector) {
	if inj == nil {
		return
	}
	mode := normalizeMode(inj.Mode())
	if mode == "" {
		return
	}
	injectorsMu.Lock()
	defer injectorsMu.Unlock()
	injectors[mode] = inj
}

// LookupAddonInjector returns the registered AddonInjector for mode.
func LookupAddonInjector(mode string) (AddonInjector, bool) {
	key := normalizeMode(mode)
	injectorsMu.RLock()
	defer injectorsMu.RUnlock()
	inj, ok := injectors[key]
	return inj, ok
}

// RegisteredAddonModes returns sorted registered addon installMode identifiers.
func RegisteredAddonModes() []string {
	injectorsMu.RLock()
	defer injectorsMu.RUnlock()
	out := make([]string, 0, len(injectors))
	for mode := range injectors {
		out = append(out, mode)
	}
	sort.Strings(out)
	return out
}

func normalizeMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}
