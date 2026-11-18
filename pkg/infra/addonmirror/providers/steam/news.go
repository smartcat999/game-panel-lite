package steam

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/smartcat999/game-panel-cloud/pkg/infra/addonmirror/driver"
)

const (
	newsURLPrefix     = "https://store.steampowered.com/news/app/"
	newsURLHTTPPrefix = "http://store.steampowered.com/news/app/"
)

var dedicatedServerBaseAppIDs = map[int]int{
	driver.SteamServerAppPalworld:           driver.SteamAppPalworld,
	driver.SteamServerAppProjectZomboid:     driver.SteamAppProjectZomboid,
	driver.SteamServerAppDontStarveTogether: driver.SteamAppDontStarveTogether,
	driver.SteamServerAppSevenDaysToDie:     driver.SteamAppSevenDaysToDie,
	896660:                                  892970,  // Valheim Dedicated Server -> Valheim
	258550:                                  252490,  // Rust Dedicated Server -> Rust
	1690800:                                 526870,  // Satisfactory Dedicated Server -> Satisfactory
	2278520:                                 1203620, // Enshrouded Dedicated Server -> Enshrouded
	1829350:                                 1604030, // V Rising Dedicated Server -> V Rising
	2465200:                                 1326470, // Sons Of The Forest Dedicated Server -> Sons Of The Forest
	1963720:                                 1621690, // Core Keeper Dedicated Server -> Core Keeper
	376030:                                  346110,  // ARK: Survival Evolved Dedicated Server -> ARK: Survival Evolved
	2430930:                                 2399830, // ARK: Survival Ascended Dedicated Server -> ARK: Survival Ascended
	740:                                     730,     // Counter-Strike Dedicated Server -> Counter-Strike 2
	222860:                                  550,     // Left 4 Dead 2 Dedicated Server -> Left 4 Dead 2
	232250:                                  440,     // Team Fortress 2 Dedicated Server -> Team Fortress 2
	4020:                                    4000,    // Garry's Mod Dedicated Server -> Garry's Mod
}

// NewsAppID maps a Steam dedicated server tool AppID to its public base game AppID.
func NewsAppID(appID int) int {
	if base, ok := dedicatedServerBaseAppIDs[appID]; ok && base > 0 {
		return base
	}
	return appID
}

// NewsURL returns the official Steam News Hub URL for the game associated with appID.
func NewsURL(appID int) string {
	if appID <= 0 {
		return ""
	}
	return fmt.Sprintf("%s%d", newsURLPrefix, NewsAppID(appID))
}

// NewsURLFromString parses a decimal Steam AppID string and returns its Steam News Hub URL.
func NewsURLFromString(rawAppID string) string {
	id, err := strconv.Atoi(strings.TrimSpace(rawAppID))
	if err != nil || id <= 0 {
		return ""
	}
	return NewsURL(id)
}

// NormalizeNewsURL rewrites dedicated-server Steam News URLs to the base game's News Hub URL.
func NormalizeNewsURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	rest := ""
	switch {
	case strings.HasPrefix(trimmed, newsURLPrefix):
		rest = strings.TrimPrefix(trimmed, newsURLPrefix)
	case strings.HasPrefix(trimmed, newsURLHTTPPrefix):
		rest = strings.TrimPrefix(trimmed, newsURLHTTPPrefix)
	default:
		return trimmed
	}
	id, suffix, ok := splitLeadingPositiveInt(rest)
	if !ok {
		return trimmed
	}
	return fmt.Sprintf("%s%d%s", newsURLPrefix, NewsAppID(id), suffix)
}

func splitLeadingPositiveInt(s string) (int, string, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, "", false
	}
	if i < len(s) && s[i] != '/' && s[i] != '?' && s[i] != '#' {
		return 0, "", false
	}
	id, err := strconv.Atoi(s[:i])
	if err != nil || id <= 0 {
		return 0, "", false
	}
	return id, s[i:], true
}
