package provider

import "fmt"

// validateCapabilities checks declarations that have provider-owned contracts.
// Runtime-owned features (for example backup archives) are checked by the
// application when it selects an execution adapter.
func validateCapabilities(item GameProvider) error {
	caps := item.Capabilities()
	if caps.Mods {
		mods, ok := item.(ModSupportProvider)
		if !ok {
			return fmt.Errorf("provider %s declares mods without ModSupport", item.Key())
		}
		support := mods.ModSupport()
		if !support.Workshop && len(support.UploadExtensions) == 0 {
			return fmt.Errorf("provider %s declares mods without an accepted source", item.Key())
		}
		if len(support.UploadExtensions) > 0 {
			if _, ok := item.(ModFilesProvider); !ok {
				return fmt.Errorf("provider %s accepts uploads without a mod file layout", item.Key())
			}
		}
	}
	if caps.WorldRegeneration {
		if _, ok := item.(WorldRegenerationProvider); !ok {
			return fmt.Errorf("provider %s declares world regeneration without a plan", item.Key())
		}
	}
	if caps.KickPlayer || caps.BanPlayer {
		if _, ok := item.(PlayerCommandProvider); !ok {
			return fmt.Errorf("provider %s declares player actions without commands", item.Key())
		}
	}
	if caps.Whitelist {
		if _, ok := item.(WhitelistCommandProvider); !ok {
			return fmt.Errorf("provider %s declares whitelist without commands", item.Key())
		}
	}
	return nil
}
