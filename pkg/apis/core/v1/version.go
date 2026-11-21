package v1

import (
	"strconv"
	"strings"
	"unicode"
)

const (
	AppMinecraftPaper        = "minecraft-paper"
	AppMinecraftFabric       = "minecraft-fabric"
	AppMinecraftNeoForge     = "minecraft-neoforge"
	AppPalworldDedicated     = "palworld-dedicated"
	AppTerrariaVanilla       = "terraria-vanilla"
	AppTerrariaTModLoader    = "terraria-tmodloader"
	AppProjectZomboid        = "project-zomboid"
	AppDontStarveTogether    = "dont-starve-together"
	AppSevenDaysToDie        = "seven-days-to-die"
	AppValheimDedicated      = "valheim-dedicated"
	AppCoreKeeper            = "core-keeper"
	AppSatisfactoryDedicated = "satisfactory-dedicated"
	AppLeft4Dead2            = "left-4-dead-2"
	AppCounterStrike2        = "counter-strike-2"
	AppRustDedicated         = "rust-dedicated"

	EnvPreStartHook = "PRE_START_HOOK"
	EnvStartupCmd   = "STARTUP_CMD"
	EnvPreStopHook  = "PRE_STOP_HOOK"
)

// IsLatestAlias reports whether v is a floating alias ("latest" / "stable") rather than a pinned version.
func IsLatestAlias(v string) bool {
	low := strings.ToLower(strings.TrimSpace(v))
	return low == "latest" || low == "stable"
}

// CompareSemanticSegmentsDesc orders concrete version strings from newest to oldest by numeric segments.
func CompareSemanticSegmentsDesc(a, b string) int {
	pa, pb := splitVersionTokens(a), splitVersionTokens(b)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, errA := strconv.Atoi(pa[i])
		nb, errB := strconv.Atoi(pb[i])
		if errA == nil && errB == nil {
			if na != nb {
				if na > nb {
					return -1
				}
				return 1
			}
			continue
		}
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return -1
			}
			return 1
		}
	}
	if len(pa) != len(pb) {
		if len(pa) > len(pb) {
			return -1
		}
		return 1
	}
	return strings.Compare(b, a)
}

func splitVersionTokens(v string) []string {
	v = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
	return strings.FieldsFunc(v, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}
