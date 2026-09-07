package provider

import (
	"fmt"
	"strconv"
	"strings"
)

// Plugin releases use three nonnegative decimal components. Game release IDs
// are independent and retain their existing provider-specific format.
func validateVersionContract(item GameProvider) error {
	metadata := item.CatalogMetadata()
	parts := strings.Split(metadata.PluginVersion, ".")
	if len(parts) != 3 {
		return fmt.Errorf("provider %s requires a major.minor.patch plugin version", item.Key())
	}
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || strconv.Itoa(n) != part {
			return fmt.Errorf("provider %s has invalid plugin version %q", item.Key(), metadata.PluginVersion)
		}
	}
	if metadata.ConfigVersion <= 0 {
		return fmt.Errorf("provider %s requires a positive config version", item.Key())
	}
	return nil
}

// CheckConfigVersion gates interpretation of persisted configuration. Version
// zero denotes pre-versioning records in format 1, never the latest format.
// Migrations must be explicit before a provider can consume a different version.
func CheckConfigVersion(item GameProvider, stored int) error {
	if stored == 0 {
		stored = 1
	}
	expected := item.CatalogMetadata().ConfigVersion
	if stored != expected {
		return fmt.Errorf("provider %s config version %d is incompatible with version %d", item.Key(), stored, expected)
	}
	return nil
}
