package app

import (
	"reflect"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func findBuiltInApp(t *testing.T, name string) *corev1.App {
	t.Helper()
	for _, a := range BuiltInApps() {
		if a.Metadata.Name == name {
			return a
		}
	}
	t.Fatalf("built-in app %q not found", name)
	return nil
}

func TestBuiltInAppsRoundTripNoFalseDiffs(t *testing.T) {
	for _, a := range BuiltInApps() {
		if len(a.ConfigSchema) == 0 || len(a.FileTemplates) == 0 {
			continue
		}
		inst := &corev1.Instance{
			Metadata: metav1.ObjectMeta{UID: "uid-" + a.Metadata.Name, Name: "inst-" + a.Metadata.Name, Annotations: map[string]string{AnnotationAppConfig: `{}`}},
			Spec:     corev1.InstanceSpec{Resources: corev1.ResourceRequirements{Requests: a.Resources, Limits: a.Resources}},
		}
		if err := RenderInstanceSpec(a, inst); err != nil {
			t.Fatalf("RenderInstanceSpec(%s): %v", a.Metadata.Name, err)
		}
		archFiles := make(map[string]string, len(inst.Spec.Files))
		for _, f := range inst.Spec.Files {
			archFiles[f.Path] = f.Content
		}
		extracted := ExtractArchiveConfigFields(a, inst.Metadata.UID, archFiles, ".")
		if len(extracted) == 0 {
			t.Fatalf("%s extracted 0 fields from rendered files", a.Metadata.Name)
		}
		items := BuildBackupConfigPreview(a, inst, extracted, ".", ".")
		if len(items) == 0 {
			t.Fatalf("%s built 0 preview items", a.Metadata.Name)
		}
		seenNames := make(map[string]bool, len(extracted))
		for _, ex := range extracted {
			seenNames[ex.Name] = true
		}
		switch a.Metadata.Name {
		case "valheim-dedicated", "core-keeper", "satisfactory-dedicated", "left-4-dead-2", "counter-strike-2", "rust-dedicated":
			if len(seenNames) != len(a.ConfigSchema) {
				t.Errorf("%s extracted %d/%d ConfigSchema fields on round-trip", a.Metadata.Name, len(seenNames), len(a.ConfigSchema))
			}
		}
		for _, it := range items {
			if it.Different {
				t.Errorf("%s round-trip false diff on %s: arch=%q cur=%q (archSet=%v curSet=%v)", a.Metadata.Name, it.Name, it.ArchiveValue, it.CurrentValue, it.ArchiveSet, it.CurrentSet)
			}
		}
	}
}

func TestExtractDSTArchiveForeignRootAndSecrets(t *testing.T) {
	dstApp := findBuiltInApp(t, "dont-starve-together")
	inst := newDSTTestInstance(t, dstApp)
	archFiles := map[string]string{
		"饥荒联机版 集群 0909-2008/cluster.ini": "[GAMEPLAY]\ngame_mode = survival\nmax_players = 8\npvp = true\npause_when_empty = true\n" +
			"[NETWORK]\ncluster_name = Lite 房间\ncluster_description = 由 Lite 导出\ncluster_password = secret-pass\nlan_only_cluster = false\noffline_cluster = false\n" +
			"[MISC]\nconsole_enabled = true\n",
		"饥荒联机版 集群 0909-2008/cluster_token.txt":           "pds-g^secret-token-archive\n",
		"饥荒联机版 集群 0909-2008/Master/worldgenoverride.lua": "KLEI 1\nreturn {\n  override_enabled = true,\n  overrides = {\n    spiders = \"often\",\n  },\n}\n",
	}
	extracted := ExtractArchiveConfigFields(dstApp, inst.Metadata.UID, archFiles, "DoNotStarveTogether/Cluster_1")
	assertNoPlaintextSecrets(t, extracted, "secret-pass", "pds-g^secret-token-archive")
	items := BuildBackupConfigPreview(dstApp, inst, extracted, "饥荒联机版 集群 0909-2008", "DoNotStarveTogether/Cluster_1")
	byName := indexPreviewItems(items)
	if got := byName["identity.serverName"]; got.ArchiveValue != "Lite 房间" || !got.Different || !got.OverriddenByPanel {
		t.Fatalf("unexpected serverName item: %+v", got)
	}
	if got := byName["identity.visibility"]; got.ArchiveValue != "公开" || got.Different {
		t.Fatalf("unexpected visibility item: %+v", got)
	}
	if got := byName["gameplay.pvp"]; got.ArchiveValue != "true" || got.CurrentValue != "false" || !got.Different {
		t.Fatalf("unexpected boolean pvp item: %+v", got)
	}
	if got := byName["identity.password"]; !got.Secret || !got.ArchiveSet || got.CurrentSet || got.ArchiveValue != "" || got.CurrentValue != "" || !got.Different {
		t.Fatalf("unexpected password secret item: %+v", got)
	}
	if got := byName["identity.clusterToken"]; !got.Secret || !got.ArchiveSet || !got.CurrentSet || got.ArchiveValue != "" || got.CurrentValue != "" || !got.Different {
		t.Fatalf("unexpected token secret item (different token values should differ): %+v", got)
	}
	if got := byName["world.overrides.spiders"]; got.ArchiveValue != "较多" || got.Group == "" || !got.Different {
		t.Fatalf("expected Master worldgenoverride spiders across non-primary group, got %+v", got)
	}
}

func TestExtractDSTVisibilityVariantsAndPalworldTuple(t *testing.T) {
	dstApp := findBuiltInApp(t, "dont-starve-together")
	inst := newDSTTestInstance(t, dstApp)
	archFiles := map[string]string{
		"AltCluster/cluster.ini": "[NETWORK]\nlan_only_cluster = true\n",
	}
	extracted := ExtractArchiveConfigFields(dstApp, inst.Metadata.UID, archFiles, "DoNotStarveTogether/Cluster_1")
	items := BuildBackupConfigPreview(dstApp, inst, extracted, "AltCluster", "DoNotStarveTogether/Cluster_1")
	if got := indexPreviewItems(items)["identity.visibility"]; got.ArchiveValue != "局域网" || !got.Different {
		t.Fatalf("expected LAN visibility, got %+v", got)
	}

	palApp := findBuiltInApp(t, "palworld-dedicated")
	palFiles := map[string]string{
		"Pal/Saved/Config/LinuxServer/PalWorldSettings.ini": `[/Script/Pal.PalGameWorldSettings]` + "\n" +
			"OptionSettings=(\n" +
			`  Difficulty=Hard,` + "\n" +
			`  ServerPlayerMaxNum=16,` + "\n" +
			`  ServerName="Pal,With,Comma",` + "\n" +
			`  ServerDescription="Fun",` + "\n" +
			`  AdminPassword="adm",` + "\n" +
			`  ServerPassword="pw",` + "\n" +
			`  DedicatedServerName="MyPal"` + "\n" +
			")\n",
	}
	palEx := ExtractArchiveConfigFields(palApp, "pal-uid", palFiles, ".")
	assertNoPlaintextSecrets(t, palEx, "adm", "pw")
	palItems := BuildBackupConfigPreview(palApp, nil, palEx, ".", ".")
	palByName := indexPreviewItems(palItems)
	if got := palByName["serverName"]; got.ArchiveValue != "Pal,With,Comma" {
		t.Fatalf("expected comma-preserving Palworld ServerName, got %+v", got)
	}
	if got := palByName["adminPassword"]; !got.Secret || !got.ArchiveSet || got.ArchiveValue != "" {
		t.Fatalf("expected redacted Palworld adminPassword secret, got %+v", got)
	}
}

func TestExtractGenericXMLAndScopedLuaDisambiguation(t *testing.T) {
	sevenApp := findBuiltInApp(t, "seven-days-to-die")
	xmlFiles := map[string]string{
		"My7DTD/serverconfig.xml": `<ServerSettings>
  <property name="ServerName" value="My 7DTD Server" />
  <property name="ServerPassword" value="topsecret" />
  <property name="ServerVisibility" value="1" />
  <property name="EACEnabled" value="false" />
  <property name="SandboxCode" value="AAAKABKARDBNB"/>
</ServerSettings>`,
	}
	xmlEx := ExtractArchiveConfigFields(sevenApp, "uid-7d", xmlFiles, ".")
	assertNoPlaintextSecrets(t, xmlEx, "topsecret")
	xmlItems := indexPreviewItems(BuildBackupConfigPreview(sevenApp, nil, xmlEx, "My7DTD", "."))
	if got := xmlItems["serverName"]; got.ArchiveValue != "My 7DTD Server" {
		t.Fatalf("expected XML ServerName via generic template clause, got %+v", got)
	}
	if got := xmlItems["serverPassword"]; !got.Secret || !got.ArchiveSet || got.ArchiveValue != "" {
		t.Fatalf("expected redacted XML ServerPassword, got %+v", got)
	}
	if got := xmlItems["serverVisibility"]; got.ArchiveValue != "仅好友可见" {
		t.Fatalf("expected XML ServerVisibility select label, got %+v", got)
	}
	if got := xmlItems["eacEnabled"]; got.ArchiveValue != "false" {
		t.Fatalf("expected XML boolean false, got %+v", got)
	}
	if got := xmlItems["sandboxPreset"]; got.ArchiveValue != "拾荒者（轻松休闲）" {
		t.Fatalf("expected conditional SandboxCode preset match, got %+v", got)
	}

	pzApp := findBuiltInApp(t, "project-zomboid")
	pzFiles := map[string]string{
		"Zomboid/Server/servertest_SandboxVars.lua": `SandboxVars = {
  ZombieLore = {
    Strength = 3,
  },
  MultiplierConfig = {
    Strength = 2.5,
  },
}`,
	}
	pzEx := ExtractArchiveConfigFields(pzApp, "uid-pz", pzFiles, ".")
	pzItems := indexPreviewItems(BuildBackupConfigPreview(pzApp, nil, pzEx, ".", "."))
	if got := pzItems["sandbox.ZombieLore.Strength"]; got.ArchiveValue != "弱鸡" || got.CurrentValue != "普通" || !got.Different {
		t.Fatalf("expected scoped ZombieLore.Strength=3 (弱鸡), got %+v", got)
	}
	if got := pzItems["sandbox.MultiplierConfig.Strength"]; got.ArchiveValue != "2.5" {
		t.Fatalf("expected scoped MultiplierConfig.Strength=2.5, got %+v", got)
	}
}

func newDSTTestInstance(t *testing.T, dstApp *corev1.App) *corev1.Instance {
	t.Helper()
	inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			UID:  "uid-dst-test",
			Name: "dst-test",
			Annotations: map[string]string{
				AnnotationAppConfig: `{"identity.serverName":"Cloud 房间","identity.visibility":"public","identity.clusterToken":"pds-g^cloud-token","gameplay.gameMode":"endless","gameplay.maxPlayers":"6"}`,
			},
		},
		Spec: corev1.InstanceSpec{Resources: corev1.ResourceRequirements{Requests: dstApp.Resources, Limits: dstApp.Resources}},
	}
	if err := RenderInstanceSpec(dstApp, inst); err != nil {
		t.Fatalf("RenderInstanceSpec DST: %v", err)
	}
	return inst
}

func assertNoPlaintextSecrets(t *testing.T, fields []corev1.InstanceBackupExtractedField, secrets ...string) {
	t.Helper()
	for _, ex := range fields {
		for _, s := range secrets {
			if ex.Value == s {
				t.Fatalf("plaintext secret %q leaked into ExtractedFields[%s].Value", s, ex.Name)
			}
		}
	}
}

func indexPreviewItems(items []corev1.InstanceBackupConfigItem) map[string]corev1.InstanceBackupConfigItem {
	m := make(map[string]corev1.InstanceBackupConfigItem, len(items))
	for _, it := range items {
		m[it.Name] = it
	}
	return m
}

func TestSnapshotConfigAndAddonsPreview(t *testing.T) {
	dstApp := findBuiltInApp(t, "dont-starve-together")
	inst := newDSTTestInstance(t, dstApp)
	inst.Spec.Addons = []corev1.InstanceAddon{
		{Repository: "dst-workshop", Package: "378160973", Version: "1700000001", Title: "Global Positions", Config: map[string]string{"ENABLEPINGS": "true"}},
		{Repository: "dst-workshop", Package: "362175979", Version: "1700000002", Title: "Wormhole Marks", Disabled: true},
	}
	snapFields := SnapshotInstanceConfigFields(dstApp, inst)
	assertNoPlaintextSecrets(t, snapFields, "pds-g^cloud-token")
	snapAddons := SnapshotInstanceAddons(inst.Spec.Addons)
	if len(snapAddons) != 2 || snapAddons[0].ConfigCount != 1 || !snapAddons[1].Disabled {
		t.Fatalf("unexpected snapshot addons: %+v", snapAddons)
	}

	// Mutate live instance after snapshot: change serverName, enable password, remove/toggle mods.
	inst.Metadata.Annotations[AnnotationAppConfig] = `{"identity.serverName":"新房间","identity.visibility":"public","identity.password":"new-pass","identity.clusterToken":"pds-g^cloud-token","gameplay.gameMode":"endless","gameplay.maxPlayers":"6"}`
	inst.Spec.Addons = []corev1.InstanceAddon{
		{Repository: "dst-workshop", Package: "378160973", Version: "1700000999", Title: "Global Positions"},
	}
	cfgPreview := indexPreviewItems(BuildSnapshotConfigPreview(dstApp, inst, snapFields))
	if got := cfgPreview["identity.serverName"]; got.ArchiveValue != "Cloud 房间" || got.CurrentValue != "新房间" || !got.Different {
		t.Fatalf("unexpected snapshot serverName preview: %+v", got)
	}
	if got := cfgPreview["identity.password"]; !got.Secret || got.ArchiveSet || !got.CurrentSet || !got.Different {
		t.Fatalf("expected password unset at backup vs set now: %+v", got)
	}
	if got := cfgPreview["identity.clusterToken"]; !got.Secret || !got.ArchiveSet || !got.CurrentSet || got.Different {
		t.Fatalf("expected matching clusterToken digest without diff: %+v", got)
	}
	addonPreview := BuildBackupAddonPreview(dstApp, inst, snapAddons, nil, ".")
	if len(addonPreview) != 2 || !addonPreview[0].InstalledNow || !addonPreview[0].Different || addonPreview[1].InstalledNow || !addonPreview[1].Different {
		t.Fatalf("unexpected snapshot addon preview: %+v", addonPreview)
	}
}

func TestExtractArchiveAddonsDST(t *testing.T) {
	dstApp := findBuiltInApp(t, "dont-starve-together")
	dstText := map[string]string{
		"LiteCluster/Master/modoverrides.lua": `return {
  ["workshop-378160973"] = {
    configuration_options = { SHOWME = true, LANG = "zh" },
    enabled = true,
  },
  ["workshop-362175979"] = {
    configuration_options = {},
    enabled = false,
  },
}`,
		"LiteCluster/Caves/modoverrides.lua": `return {
  ["workshop-378160973"] = {
    configuration_options = { SHOWME = true },
    enabled = true,
  },
}`,
		"LiteCluster/dedicated_server_mods_setup.lua": "ServerModSetup(\"378160973\")\nServerModSetup(\"1207269058\")\n",
		"mods/workshop-362175979/modinfo.lua":         "name = \"Wormhole Marks\"\nversion = \"2.1\"\n",
	}
	dstAddons := ExtractArchiveAddons(dstApp, []string{"mods/workshop-362175979/modmain.lua"}, dstText)
	dstPreview := BuildBackupAddonPreview(dstApp, nil, nil, dstAddons, "LiteCluster")
	if len(dstPreview) != 3 || dstPreview[1].Title != "Wormhole Marks" || !dstPreview[1].BackupDisabled {
		t.Fatalf("expected 3 deduplicated DST mods with preserved disabled state & title, got %+v", dstPreview)
	}
	if len(dstAddons) != 3 {
		t.Fatalf("expected 3 deduplicated DST records, got %d: %+v", len(dstAddons), dstAddons)
	}
	if !reflect.DeepEqual(dstAddons[0].Scopes, []string{"Master", "Caves"}) || dstAddons[0].ConfigCount != 2 || dstAddons[0].Disabled {
		t.Fatalf("unexpected merged first DST mod: %+v", dstAddons[0])
	}
	if !dstAddons[1].Disabled || dstAddons[2].Package != "1207269058" {
		t.Fatalf("unexpected DST mod states: %+v", dstAddons)
	}
}

func TestExtractArchiveAddonsGenericModes(t *testing.T) {
	tmodApp := findBuiltInApp(t, "terraria-tmodloader")
	tmodAddons := ExtractArchiveAddons(tmodApp, []string{"Mods/CalamityMod.tmod", "Mods/MagicStorage.tmod"}, map[string]string{
		"Mods/enabled.json": `["CalamityMod"]`,
	})
	if len(tmodAddons) != 2 || tmodAddons[0].Disabled || !tmodAddons[1].Disabled {
		t.Fatalf("unexpected tModLoader extracted addons: %+v", tmodAddons)
	}

	mcApp := findBuiltInApp(t, "minecraft-paper")
	mcAddons := ExtractArchiveAddons(mcApp, []string{"MyMC/plugins/EssentialsX-2.20.1.jar", "MyMC/plugins/LuckPerms.jar"}, nil)
	if len(mcAddons) != 2 || mcAddons[0].Package != "EssentialsX" || mcAddons[0].Version != "2.20.1" || mcAddons[1].Package != "LuckPerms" {
		t.Fatalf("unexpected Paper extracted plugins: %+v", mcAddons)
	}

	l4d2App := findBuiltInApp(t, "left-4-dead-2")
	l4d2Addons := ExtractArchiveAddons(l4d2App, []string{"left4dead2/addons/121339498-1358581160.vpk"}, nil)
	if len(l4d2Addons) != 1 || l4d2Addons[0].Package != "121339498" || l4d2Addons[0].Version != "1358581160" ||
		l4d2Addons[0].Repository != "l4d2-workshop" || l4d2Addons[0].InstallMode != corev1.AddonInstallModeFlatDir ||
		l4d2Addons[0].TargetDir != "left4dead2/addons" {
		t.Fatalf("unexpected L4D2 extracted workshop addons: %+v", l4d2Addons)
	}

	pzApp := findBuiltInApp(t, "project-zomboid")
	pzAddons := ExtractArchiveAddons(pzApp, []string{"steamapps/workshop/content/108600/2169435993/mods/ModOptions/mod.info"}, map[string]string{
		"Zomboid/Server/servertest.ini": "WorkshopItems=2169435993;2392709985\n",
	})
	if len(pzAddons) != 2 || pzAddons[0].Package != "2169435993" || pzAddons[1].Package != "2392709985" {
		t.Fatalf("unexpected PZ extracted workshop addons: %+v", pzAddons)
	}
}

func TestExtractArchiveAddonsFallbackAndNonDSTMerge(t *testing.T) {
	nilApp := &corev1.App{}
	tmod := ExtractArchiveAddons(nilApp, []string{"Mods/CalamityMod.tmod"}, nil)
	if len(tmod) != 1 || tmod[0].InstallMode != corev1.AddonInstallModeTModLatest || tmod[0].Package != "CalamityMod" {
		t.Fatalf("unexpected fallback tmod extraction: %+v", tmod)
	}
	flat := ExtractArchiveAddons(nilApp, []string{"plugins/EssentialsX-2.20.1.jar"}, nil)
	if len(flat) != 1 || flat[0].InstallMode != corev1.AddonInstallModeFlatDir || flat[0].Package != "EssentialsX" {
		t.Fatalf("unexpected fallback flatdir extraction: %+v", flat)
	}
	dst := ExtractArchiveAddons(nilApp, nil, map[string]string{
		"Cluster_1/Master/modoverrides.lua": `return { ["workshop-378160973"] = { enabled = true } }`,
	})
	if len(dst) != 1 || dst[0].InstallMode != corev1.AddonInstallModeDSTLocalMods || dst[0].Package != "378160973" {
		t.Fatalf("unexpected fallback dst extraction: %+v", dst)
	}
	pz := ExtractArchiveAddons(nilApp, nil, map[string]string{
		"Zomboid/Server/servertest.ini": "WorkshopItems=2169435993\n",
	})
	if len(pz) != 1 || pz[0].InstallMode != corev1.AddonInstallModeWorkshopTree || pz[0].Package != "2169435993" {
		t.Fatalf("unexpected fallback workshop extraction: %+v", pz)
	}

	mcApp := findBuiltInApp(t, "minecraft-paper")
	merged := ExtractArchiveAddons(mcApp, []string{"plugins/EssentialsX-2.20.0.jar", "plugins/EssentialsX-2.20.1.jar"}, nil)
	if len(merged) != 1 || merged[0].Package != "EssentialsX" || len(merged[0].Scopes) != 0 {
		t.Fatalf("expected non-DST merge without Master/Caves scopes, got %+v", merged)
	}
}

func TestExtractNewBuiltInGamesConfigAndSecrets(t *testing.T) {
	check := func(appName, uid string, files map[string]string, secrets []string, want map[string]string) {
		t.Helper()
		appObj := findBuiltInApp(t, appName)
		ex := ExtractArchiveConfigFields(appObj, uid, files, ".")
		assertNoPlaintextSecrets(t, ex, secrets...)
		items := indexPreviewItems(BuildBackupConfigPreview(appObj, nil, ex, ".", "."))
		for k, wv := range want {
			if items[k].ArchiveValue != wv {
				t.Fatalf("%s[%s] got %q, want %q (item=%+v)", appName, k, items[k].ArchiveValue, wv, items[k])
			}
		}
	}
	check("valheim-dedicated", "u-vh", map[string]string{"saves/valheim-server.conf": "serverName=V\nserverPassword=secret-vh\npreset=casual\nresources=more\ncrossplay=true\n"},
		[]string{"secret-vh"}, map[string]string{"preset": "休闲探索（柔和战斗·低惩罚）", "resources": "丰饶（1.5倍）", "crossplay": "true"})
	check("core-keeper", "u-ck", map[string]string{"saves/ServerConfig.json": "{\n  \"worldMode\": 3,\n  \"seasonOverride\": 6\n}\n"},
		nil, map[string]string{"worldMode": "休闲模式", "seasonOverride": "樱花祭"})
	check("satisfactory-dedicated", "u-sf", map[string]string{
		"FactoryGame/Saved/Config/LinuxServer/GameUserSettings.ini": "[/Script/FactoryGame.FGGameUserSettings]\nmNetworkQuality=2\n[ServerSettings]\nAutoPause=false\n",
		"FactoryGame/Saved/Config/LinuxServer/Engine.ini":           "[/Script/OnlineSubsystemUtils.IpNetDriver]\nNetServerMaxTickRate=60\n",
	}, nil, map[string]string{"networkQuality": "高带宽", "autoPause": "false", "tickRate": "60"})
	check("left-4-dead-2", "u-l4d2", map[string]string{
		"left4dead2/cfg/server.cfg": "// Managed by GamePanel Lite (startMap=c8m1_apartment | gamemode=versus)\nrcon_password \"secret-rcon\"\nsv_gametypes \"versus\"\nz_difficulty \"Impossible\"\n",
	}, []string{"secret-rcon"}, map[string]string{"startMap": "毫不留情", "gamemode": "感染对抗", "difficulty": "专家"})
	check("counter-strike-2", "u-cs2", map[string]string{
		"game/csgo/cfg/server.cfg": "// Managed by GamePanel Lite (preset=wingman | startMap=de_dust2 | maxPlayers=4 | gslt=secret-gslt)\nrcon_password \"secret-cs2\"\nmp_maxrounds 16\n",
	}, []string{"secret-gslt", "secret-cs2"}, map[string]string{"gameModePreset": "搭档模式（二对二）", "startMap": "炙热沙城", "maxPlayers": "4", "mpMaxrounds": "16"})
	check("rust-dedicated", "u-rust", map[string]string{
		"server/rust/cfg/server.cfg": "server.pve true\nserver.worldsize 2000\nrcon.password \"secret-rust\"\n",
	}, []string{"secret-rust"}, map[string]string{"pve": "合作生存模式（关闭玩家伤害）", "worldSize": "2000"})

	csItems := indexPreviewItems(BuildBackupConfigPreview(findBuiltInApp(t, "counter-strike-2"), nil,
		ExtractArchiveConfigFields(findBuiltInApp(t, "counter-strike-2"), "u-cs2", map[string]string{
			"game/csgo/cfg/server.cfg": "// Managed by GamePanel Lite (gslt=secret-gslt)\nrcon_password \"secret-cs2\"\n",
		}, "."), ".", "."))
	if !csItems["gsltToken"].Secret || !csItems["gsltToken"].ArchiveSet || !csItems["rconPassword"].Secret || !csItems["rconPassword"].ArchiveSet {
		t.Fatalf("expected redacted gsltToken & rconPassword in CS2 preview, got gslt=%+v rcon=%+v", csItems["gsltToken"], csItems["rconPassword"])
	}
}
