package bootstrap

import (
	"context"
	"errors"
	"fmt"

	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
	registryv1 "github.com/smartcat999/game-panel-cloud/pkg/apis/registry/v1"
	"github.com/smartcat999/game-panel-cloud/pkg/infra/db"
	"github.com/smartcat999/game-panel-cloud/pkg/repo"
	policydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/policy"
	repositorydb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/repository"
	upstreamdb "github.com/smartcat999/game-panel-cloud/pkg/repo/registry/upstream"
)

// EnsureBuiltInRegistry seeds default Harbor-style Upstreams, Proxy Repositories, and ReplicationPolicy templates.
func EnsureBuiltInRegistry(ctx context.Context, reg *repo.Registry) error {
	if reg == nil || reg.Pool() == nil {
		return nil
	}
	upDB := upstreamdb.NewDB(reg.Pool())
	for _, up := range defaultBuiltInUpstreams() {
		if err := upsertBuiltInUpstream(ctx, upDB, up); err != nil {
			return err
		}
	}
	repoDB := repositorydb.NewDB(reg.Pool())
	for _, r := range defaultBuiltInRepositories() {
		if _, err := repoDB.Get(ctx, r.Metadata.Name); errors.Is(err, db.ErrNotFound) {
			if _, err := repoDB.Create(ctx, r); err != nil && !errors.Is(err, db.ErrAlreadyExists) {
				return fmt.Errorf("create built-in repository %s: %w", r.Metadata.Name, err)
			}
		}
	}
	polDB := policydb.NewDB(reg.Pool())
	for _, p := range defaultBuiltInPolicies() {
		if _, err := polDB.Get(ctx, p.Metadata.Name); errors.Is(err, db.ErrNotFound) {
			if _, err := polDB.Create(ctx, p); err != nil && !errors.Is(err, db.ErrAlreadyExists) {
				return fmt.Errorf("create built-in replicationpolicy %s: %w", p.Metadata.Name, err)
			}
		}
	}
	return nil
}

func upsertBuiltInUpstream(ctx context.Context, upDB *upstreamdb.DB, up *registryv1.Upstream) error {
	existing, err := upDB.Get(ctx, up.Metadata.Name)
	if errors.Is(err, db.ErrNotFound) {
		if _, cErr := upDB.Create(ctx, up); cErr != nil && !errors.Is(cErr, db.ErrAlreadyExists) {
			return fmt.Errorf("create built-in upstream %s: %w", up.Metadata.Name, cErr)
		}
		return nil
	}
	if err != nil || existing == nil {
		return nil
	}
	want := up.Metadata.Annotations[registryv1.AnnotationSteamPlatforms]
	if want != "" && existing.Metadata.Annotations[registryv1.AnnotationSteamPlatforms] != want {
		if existing.Metadata.Annotations == nil {
			existing.Metadata.Annotations = make(map[string]string, 1)
		}
		existing.Metadata.Annotations[registryv1.AnnotationSteamPlatforms] = want
		_, _ = upDB.Update(ctx, existing)
	}
	return nil
}

func defaultBuiltInUpstreams() []*registryv1.Upstream {
	return []*registryv1.Upstream{
		{Metadata: metav1.ObjectMeta{Name: "papermc-official"}, Provider: registryv1.UpstreamProviderPaperMC, Endpoint: "https://fill.papermc.io"},
		{Metadata: metav1.ObjectMeta{Name: "fabricmc-official"}, Provider: registryv1.UpstreamProviderFabricMC, Endpoint: "https://meta.fabricmc.net"},
		{Metadata: metav1.ObjectMeta{Name: "neoforge-official"}, Provider: registryv1.UpstreamProviderNeoForge, Endpoint: "https://maven.neoforged.net"},
		{Metadata: metav1.ObjectMeta{Name: "terraria-official"}, Provider: registryv1.UpstreamProviderHTTP, Endpoint: "https://terraria.org"},
		{Metadata: metav1.ObjectMeta{Name: "tmodloader-github"}, Provider: registryv1.UpstreamProviderHTTP, Endpoint: "https://api.github.com"},
		{Metadata: metav1.ObjectMeta{Name: "steam-palworld"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 2394010},
		{Metadata: metav1.ObjectMeta{Name: "steam-pz-server"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 380870},
		{Metadata: metav1.ObjectMeta{Name: "steam-dst-server"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 343050},
		{Metadata: metav1.ObjectMeta{Name: "steam-7dtd-server"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 294420},
		{Metadata: metav1.ObjectMeta{Name: "steam-valheim-server"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 896660},
		{Metadata: metav1.ObjectMeta{Name: "steam-core-keeper-server"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 1963720},
		{Metadata: metav1.ObjectMeta{Name: "steam-satisfactory-server"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 1690800},
		{Metadata: metav1.ObjectMeta{Name: "steam-l4d2-server", Annotations: map[string]string{registryv1.AnnotationSteamPlatforms: "windows,linux"}}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 222860},
		{Metadata: metav1.ObjectMeta{Name: "steam-cs2-server"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 730},
		{Metadata: metav1.ObjectMeta{Name: "steam-rust-server"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 258550},
		{Metadata: metav1.ObjectMeta{Name: "modrinth-paper"}, Provider: registryv1.UpstreamProviderModrinth, Endpoint: "https://api.modrinth.com/v2", ModrinthLoader: registryv1.ModrinthLoaderPaper, ModrinthProjectType: registryv1.ModrinthProjectPlugin},
		{Metadata: metav1.ObjectMeta{Name: "modrinth-fabric"}, Provider: registryv1.UpstreamProviderModrinth, Endpoint: "https://api.modrinth.com/v2", ModrinthLoader: registryv1.ModrinthLoaderFabric, ModrinthProjectType: registryv1.ModrinthProjectMod},
		{Metadata: metav1.ObjectMeta{Name: "modrinth-neoforge"}, Provider: registryv1.UpstreamProviderModrinth, Endpoint: "https://api.modrinth.com/v2", ModrinthLoader: registryv1.ModrinthLoaderNeoForge, ModrinthProjectType: registryv1.ModrinthProjectMod},
		{Metadata: metav1.ObjectMeta{Name: "thunderstore-valheim"}, Provider: registryv1.UpstreamProviderThunderstore, Endpoint: "https://thunderstore.io/c/valheim"},
		{Metadata: metav1.ObjectMeta{Name: "thunderstore-palworld"}, Provider: registryv1.UpstreamProviderThunderstore, Endpoint: "https://thunderstore.io/c/palworld"},
		{Metadata: metav1.ObjectMeta{Name: "thunderstore-core-keeper"}, Provider: registryv1.UpstreamProviderThunderstore, Endpoint: "https://thunderstore.io/c/core-keeper"},
		{Metadata: metav1.ObjectMeta{Name: "modio-core-keeper"}, Provider: registryv1.UpstreamProviderModIO, Endpoint: "https://api.mod.io/v1/games/5289"},
		{Metadata: metav1.ObjectMeta{Name: "umod-rust"}, Provider: registryv1.UpstreamProviderUMod, Endpoint: "https://umod.org/games/rust"},
		{Metadata: metav1.ObjectMeta{Name: "ficsit-satisfactory"}, Provider: registryv1.UpstreamProviderFicsit, Endpoint: "https://api.ficsit.app/v2/query"},
		{Metadata: metav1.ObjectMeta{Name: "cs2plugins-github"}, Provider: registryv1.UpstreamProviderCS2Plugins, Endpoint: "https://api.github.com"},
		{Metadata: metav1.ObjectMeta{Name: "sevendaysmods-github"}, Provider: registryv1.UpstreamProviderSevenDaysMods, Endpoint: "https://api.github.com"},
		{Metadata: metav1.ObjectMeta{Name: "steam-tmodloader-workshop"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 1281930},
		{Metadata: metav1.ObjectMeta{Name: "steam-pz-workshop"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 108600},
		{Metadata: metav1.ObjectMeta{Name: "steam-dst-workshop"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 322330},
		{Metadata: metav1.ObjectMeta{Name: "steam-l4d2-workshop"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 550},
		{Metadata: metav1.ObjectMeta{Name: "steam-cs2-workshop"}, Provider: registryv1.UpstreamProviderSteam, SteamAppID: 730},
	}
}

func defaultBuiltInRepositories() []*registryv1.Repository {
	return []*registryv1.Repository{
		proxyRepo("mc-paper", "Minecraft Paper 服务端", registryv1.FormatServerPackage, "papermc-official"),
		proxyRepo("mc-fabric", "Minecraft Fabric 服务端", registryv1.FormatServerPackage, "fabricmc-official"),
		proxyRepo("mc-neoforge", "Minecraft NeoForge 服务端", registryv1.FormatServerPackage, "neoforge-official"),
		proxyRepo("terraria-vanilla", "泰拉瑞亚原版服务端", registryv1.FormatServerPackage, "terraria-official"),
		proxyRepo("terraria-tmodloader", "泰拉瑞亚 tModLoader 服务端", registryv1.FormatServerPackage, "tmodloader-github"),
		proxyRepo("palworld-server", "幻兽帕鲁服务端", registryv1.FormatServerPackage, "steam-palworld"),
		proxyRepo("pz-server", "僵尸毁灭工程服务端", registryv1.FormatServerPackage, "steam-pz-server"),
		proxyRepo("dst-server", "饥荒联机版服务端", registryv1.FormatServerPackage, "steam-dst-server"),
		proxyRepo("seven-days-server", "七日杀服务端", registryv1.FormatServerPackage, "steam-7dtd-server"),
		proxyRepo("valheim-server", "英灵神殿服务端", registryv1.FormatServerPackage, "steam-valheim-server"),
		proxyRepo("core-keeper-server", "护核纪元服务端", registryv1.FormatServerPackage, "steam-core-keeper-server"),
		proxyRepo("satisfactory-server", "幸福工厂服务端", registryv1.FormatServerPackage, "steam-satisfactory-server"),
		proxyRepo("l4d2-server", "求生之路 2 服务端", registryv1.FormatServerPackage, "steam-l4d2-server"),
		proxyRepo("cs2-server", "反恐精英 2 服务端", registryv1.FormatServerPackage, "steam-cs2-server"),
		proxyRepo("rust-server", "腐蚀服务端", registryv1.FormatServerPackage, "steam-rust-server"),
		proxyRepo("mc-paper-plugins", "Minecraft Paper 插件镜像", registryv1.FormatModrinthAddon, "modrinth-paper"),
		proxyRepo("mc-fabric-mods", "Minecraft Fabric 模组镜像", registryv1.FormatModrinthAddon, "modrinth-fabric"),
		proxyRepo("mc-neoforge-mods", "Minecraft NeoForge 模组镜像", registryv1.FormatModrinthAddon, "modrinth-neoforge"),
		proxyRepo("valheim-thunderstore", "英灵神殿 Thunderstore 模组库", registryv1.FormatThunderstoreMod, "thunderstore-valheim"),
		proxyRepo("palworld-thunderstore", "幻兽帕鲁 Thunderstore 模组库", registryv1.FormatThunderstoreMod, "thunderstore-palworld"),
		proxyRepo("core-keeper-thunderstore", "护核纪元 Thunderstore 模组库", registryv1.FormatThunderstoreMod, "thunderstore-core-keeper"),
		proxyRepo("core-keeper-modio", "护核纪元 mod.io 官方模组库", registryv1.FormatModIOMod, "modio-core-keeper"),
		proxyRepo("rust-umod", "腐蚀 Rust uMod/Oxide 插件库", registryv1.FormatUModPlugin, "umod-rust"),
		proxyRepo("satisfactory-ficsit", "幸福工厂 ficsit.app 模组库", registryv1.FormatFicsitMod, "ficsit-satisfactory"),
		proxyRepo("cs2-plugins", "CS2 Metamod / CounterStrikeSharp 插件库", registryv1.FormatCS2Plugin, "cs2plugins-github"),
		proxyRepo("seven-days-mods", "七日杀 7DTD 服务端模组库", registryv1.FormatSevenDaysMod, "sevendaysmods-github"),
		proxyRepo("tmodloader-workshop", "泰拉瑞亚 tModLoader 创意工坊", registryv1.FormatWorkshopMod, "steam-tmodloader-workshop"),
		proxyRepo("pz-workshop", "僵尸毁灭工程创意工坊", registryv1.FormatWorkshopMod, "steam-pz-workshop"),
		proxyRepo("dst-workshop", "饥荒联机版创意工坊", registryv1.FormatWorkshopMod, "steam-dst-workshop"),
		proxyRepo("l4d2-workshop", "求生之路 2 创意工坊", registryv1.FormatWorkshopMod, "steam-l4d2-workshop"),
		proxyRepo("cs2-workshop", "反恐精英 2 创意工坊地图", registryv1.FormatWorkshopMod, "steam-cs2-workshop"),
	}
}

func proxyRepo(name, displayName, format, upstream string) *registryv1.Repository {
	return &registryv1.Repository{
		Metadata: metav1.ObjectMeta{Name: name},
		Spec: registryv1.RepositorySpec{
			DisplayName:  displayName,
			Mode:         registryv1.RepositoryModeProxy,
			Format:       format,
			UpstreamName: upstream,
		},
		Status: registryv1.RepositoryStatus{Phase: registryv1.RepositoryPhaseActive},
	}
}

func defaultBuiltInPolicies() []*registryv1.ReplicationPolicy {
	return []*registryv1.ReplicationPolicy{
		{Metadata: metav1.ObjectMeta{Name: "sync-mc-paper"}, Repository: "mc-paper", Enabled: false, Mode: registryv1.ReplicationModeTrackReleases, Packages: []string{"paper"}, IntervalMinutes: 360},
		{Metadata: metav1.ObjectMeta{Name: "sync-mc-fabric"}, Repository: "mc-fabric", Enabled: false, Mode: registryv1.ReplicationModeTrackReleases, Packages: []string{"fabric"}, IntervalMinutes: 360},
		{Metadata: metav1.ObjectMeta{Name: "sync-mc-neoforge"}, Repository: "mc-neoforge", Enabled: false, Mode: registryv1.ReplicationModeTrackReleases, Packages: []string{"neoforge"}, IntervalMinutes: 360},
		{Metadata: metav1.ObjectMeta{Name: "sync-terraria-vanilla"}, Repository: "terraria-vanilla", Enabled: false, Mode: registryv1.ReplicationModeTrackReleases, Packages: []string{"terraria-vanilla"}, IntervalMinutes: 720},
		{Metadata: metav1.ObjectMeta{Name: "sync-terraria-tmodloader"}, Repository: "terraria-tmodloader", Enabled: false, Mode: registryv1.ReplicationModeTrackReleases, Packages: []string{"terraria-tmodloader"}, IntervalMinutes: 720},
		{Metadata: metav1.ObjectMeta{Name: "sync-popular-tmodloader"}, Repository: "tmodloader-workshop", Enabled: false, Mode: registryv1.ReplicationModeTopPopular, TopN: 10, IntervalMinutes: 1440},
		{Metadata: metav1.ObjectMeta{Name: "sync-popular-dst"}, Repository: "dst-workshop", Enabled: false, Mode: registryv1.ReplicationModeTopPopular, TopN: 10, IntervalMinutes: 1440},
	}
}
