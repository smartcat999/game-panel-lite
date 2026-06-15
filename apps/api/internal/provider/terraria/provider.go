package terraria

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/smartcat999/game-panel-lite/apps/api/internal/domain"
	"github.com/smartcat999/game-panel-lite/apps/api/internal/runtime"
)

type VanillaProvider struct{}
type TModLoaderProvider struct{}

var vanillaVersions = []string{"1.4.5.6", "1.4.4.9"}
var vanillaImageTags = map[string]string{
	"1.4.5.6": "tshock-1.4.5.6-6.1.0",
	"1.4.4.9": "tshock-1.4.4.9-5.2.4",
}
var tmodloaderVersions = []string{"v2026.04.3.0", "v2026.02.3.1"}

func NewVanillaProvider() VanillaProvider       { return VanillaProvider{} }
func NewTModLoaderProvider() TModLoaderProvider { return TModLoaderProvider{} }

func (VanillaProvider) Key() domain.ProviderKey { return domain.ProviderTerrariaVanilla }
func (VanillaProvider) Name() string            { return "Terraria Vanilla" }
func (VanillaProvider) Image() string           { return "ryshe/terraria:latest" }
func (VanillaProvider) Versions() []string      { return vanillaVersions }
func (VanillaProvider) ImageFor(version string) string {
	if tag, ok := vanillaImageTags[version]; ok {
		return "ryshe/terraria:" + tag
	}
	return "ryshe/terraria:latest"
}
func (VanillaProvider) DefaultConfig() domain.TerrariaConfig {
	return Presets[0].Config
}
func (VanillaProvider) ValidateConfig(config domain.TerrariaConfig) error {
	return ValidateConfig(config)
}
func (VanillaProvider) RenderConfig(config domain.TerrariaConfig) (string, error) {
	return RenderServerConfig(config)
}
func (VanillaProvider) RuntimeOptions(config domain.TerrariaConfig) runtime.ContainerOptions {
	return vanillaRuntimeOptions(config)
}

func (TModLoaderProvider) Key() domain.ProviderKey { return domain.ProviderTerrariaTModLoader }
func (TModLoaderProvider) Name() string            { return "Terraria tModLoader" }
func (TModLoaderProvider) Image() string           { return TModLoaderImageForVersion(tmodloaderVersions[0]) }
func (TModLoaderProvider) Versions() []string      { return tmodloaderVersions }
func (TModLoaderProvider) ImageFor(version string) string {
	return TModLoaderImageForVersion(version)
}
func (TModLoaderProvider) DefaultConfig() domain.TerrariaConfig {
	return Presets[4].Config
}
func (TModLoaderProvider) ValidateConfig(config domain.TerrariaConfig) error {
	return ValidateConfig(config)
}
func (TModLoaderProvider) RenderConfig(config domain.TerrariaConfig) (string, error) {
	return RenderServerConfig(config)
}
func (TModLoaderProvider) RuntimeOptions(config domain.TerrariaConfig) runtime.ContainerOptions {
	return tModLoaderRuntimeOptions(config)
}

func RuntimeWorldFiles(providerKey domain.ProviderKey, config domain.TerrariaConfig) []string {
	switch providerKey {
	case domain.ProviderTerrariaTModLoader:
		return []string{filepath.Join("Worlds", config.WorldName+".wld")}
	default:
		return []string{filepath.Join("worlds", config.WorldName+".wld")}
	}
}

func RuntimeModFiles(providerKey domain.ProviderKey, fileName string) []string {
	if providerKey != domain.ProviderTerrariaTModLoader {
		return nil
	}
	return []string{filepath.Join("Mods", fileName)}
}

func vanillaRuntimeOptions(config domain.TerrariaConfig) runtime.ContainerOptions {
	worldSizes := map[domain.WorldSize]int{"small": 1, "medium": 2, "large": 3}
	cmd := []string{
		"-autocreate", fmt.Sprintf("%d", worldSizes[config.WorldSize]),
		"-worldname", config.WorldName,
		"-port", fmt.Sprintf("%d", config.Port),
		"-maxplayers", fmt.Sprintf("%d", config.MaxPlayers),
	}
	if config.Password != "" {
		cmd = append(cmd, "-password", config.Password)
	}
	return runtime.ContainerOptions{
		Env: []string{
			"WORLD_FILENAME=" + config.WorldName + ".wld",
			"CONFIGPATH=/data",
			"LOGPATH=/data/logs",
		},
		Cmd:        cmd,
		DataMounts: []string{"/data", "/root/.local/share/Terraria/Worlds"},
		Files: map[string]string{
			"config.json": `{"Settings":{"StorageType":"sqlite"}}`,
		},
	}
}

func tModLoaderRuntimeOptions(config domain.TerrariaConfig) runtime.ContainerOptions {
	worldSizes := map[domain.WorldSize]int{"small": 1, "medium": 2, "large": 3}
	return runtime.ContainerOptions{
		Env: []string{
			"HOME=/home/container",
			"TMOD_HOME=/home/container",
			"WORLD_NAME=" + config.WorldName,
			fmt.Sprintf("WORLD_SIZE=%d", worldSizes[config.WorldSize]),
		},
		Cmd: []string{
			"sh",
			"-c",
			"cd /home/container && exec ./server/start-tModLoaderServer.sh -nosteam -config /home/container/serverconfig.txt -tmlsavedirectory /home/container -world \"/home/container/Worlds/${WORLD_NAME}.wld\" -autocreate \"${WORLD_SIZE}\" -noupnp",
		},
		DataMounts: []string{
			"Worlds:/home/container/Worlds",
			"Mods:/home/container/Mods",
			"logs:/home/container/logs",
			"steamapps:/home/container/steamapps",
			"serverconfig.txt:/home/container/serverconfig.txt",
		},
		Files: map[string]string{
			"serverconfig.txt": renderTModLoaderRuntimeConfig(config),
		},
	}
}

func TModLoaderImageForVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = tmodloaderVersions[0]
	}
	return "smartcat99999/tmodloader:" + version
}

func renderTModLoaderRuntimeConfig(config domain.TerrariaConfig) string {
	worldSizes := map[domain.WorldSize]int{"small": 1, "medium": 2, "large": 3}
	worldEvils := map[domain.WorldEvil]int{"": 0, "random": 0, "corruption": 1, "crimson": 2}
	difficulties := map[domain.Difficulty]int{"journey": 0, "classic": 1, "expert": 2, "master": 3}
	lines := []string{
		fmt.Sprintf("world=/home/container/Worlds/%s.wld", config.WorldName),
		fmt.Sprintf("autocreate=%d", worldSizes[config.WorldSize]),
		fmt.Sprintf("worldname=%s", config.WorldName),
		fmt.Sprintf("worldevil=%d", worldEvils[config.WorldEvil]),
		fmt.Sprintf("difficulty=%d", difficulties[config.Difficulty]),
		fmt.Sprintf("maxplayers=%d", config.MaxPlayers),
		fmt.Sprintf("port=%d", config.Port),
		fmt.Sprintf("password=%s", config.Password),
		fmt.Sprintf("motd=%s", value(config.MOTD, "Welcome to GamePanel Lite")),
		fmt.Sprintf("seed=%s", config.Seed),
		fmt.Sprintf("worldpath=%s", "/home/container/Worlds"),
		fmt.Sprintf("secure=%d", boolInt(config.Secure)),
		fmt.Sprintf("language=%s", value(config.Language, "zh-Hans")),
		"upnp=0",
		"priority=1",
	}
	return strings.Join(lines, "\n")
}
