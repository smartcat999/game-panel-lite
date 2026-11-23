package app

import (
	"encoding/json"
	"encoding/xml"
	"regexp"
	"strings"
	"testing"

	corev1 "github.com/smartcat999/game-panel-cloud/pkg/apis/core/v1"
	metav1 "github.com/smartcat999/game-panel-cloud/pkg/apis/meta/v1"
)

func TestBuiltInAppsRenderAllTemplatesCleanly(t *testing.T) {
	apps := BuiltInApps()
	wantOrder := []string{
		"minecraft-paper",
		"minecraft-fabric",
		"minecraft-neoforge",
		"palworld-dedicated",
		"terraria-vanilla",
		"terraria-tmodloader",
		"project-zomboid",
		"dont-starve-together",
		"seven-days-to-die",
		"valheim-dedicated",
		"core-keeper",
		"satisfactory-dedicated",
		"left-4-dead-2",
		"counter-strike-2",
		"rust-dedicated",
		"custom-container",
	}
	if len(apps) != len(wantOrder) {
		t.Fatalf("expected %d built-in apps, got %d", len(wantOrder), len(apps))
	}
	for i, want := range wantOrder {
		if apps[i].Metadata.Name != want {
			t.Fatalf("built-in app[%d] = %q, want %q", i, apps[i].Metadata.Name, want)
		}
		if !apps[i].Published {
			t.Fatalf("built-in app %s should be published by default", want)
		}
		if want == "counter-strike-2" && apps[i].Resources[corev1.ResourceStorage] != "80Gi" {
			t.Fatalf("counter-strike-2 storage = %q, want 80Gi", apps[i].Resources[corev1.ResourceStorage])
		}
		verifySingleBuiltInAppRender(t, apps[i])
		verifyAppPresetsTargetsAndMetrics(t, apps[i])
		verifyAppNamingConventions(t, apps[i])
	}
}

var reParenthesizedEnglish = regexp.MustCompile(`[(（][^）)]*[A-Za-z][^）)]*[)）]`)

func verifyAppNamingConventions(t *testing.T, a *corev1.App) {
	t.Helper()
	name := a.Metadata.Name
	for _, f := range a.ConfigSchema {
		if strings.Contains(f.Group, "与") {
			t.Fatalf("%s field %s has group %q containing '与'", name, f.Name, f.Group)
		}
		if reParenthesizedEnglish.MatchString(f.Label) {
			t.Fatalf("%s field %s label %q contains parenthesized English", name, f.Name, f.Label)
		}
		for _, opt := range f.Options {
			if reParenthesizedEnglish.MatchString(opt.Label) {
				t.Fatalf("%s field %s option %q label %q contains parenthesized English", name, f.Name, opt.Value, opt.Label)
			}
		}
	}
	for _, p := range a.ConfigPresets {
		if reParenthesizedEnglish.MatchString(p.Name) {
			t.Fatalf("%s preset %s name %q contains parenthesized English", name, p.ID, p.Name)
		}
	}
	for _, act := range a.ConsoleActions {
		if strings.Contains(act.Group, "与") {
			t.Fatalf("%s consoleAction %s has group %q containing '与'", name, act.ID, act.Group)
		}
		if reParenthesizedEnglish.MatchString(act.Title) {
			t.Fatalf("%s consoleAction %s title %q contains parenthesized English", name, act.ID, act.Title)
		}
	}
}

func verifySingleBuiltInAppRender(t *testing.T, a *corev1.App) {
	t.Helper()
	inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			Name:        "test-" + a.Metadata.Name,
			Annotations: map[string]string{AnnotationAppConfig: `{}`},
		},
		Spec: corev1.InstanceSpec{
			Resources: corev1.ResourceRequirements{Requests: a.Resources, Limits: a.Resources},
		},
	}
	if err := RenderInstanceSpec(a, inst); err != nil {
		t.Fatalf("RenderInstanceSpec(%s) failed: %v", a.Metadata.Name, err)
	}
	if len(inst.Spec.Files) != len(a.FileTemplates) {
		t.Fatalf("%s rendered %d files, want %d", a.Metadata.Name, len(inst.Spec.Files), len(a.FileTemplates))
	}
	if inst.Metadata.Labels[corev1.LabelInstanceApp] != a.Metadata.Name {
		t.Fatalf("%s did not populate %s label", a.Metadata.Name, corev1.LabelInstanceApp)
	}
	if (a.RecommendedVersion != "" || len(a.Versions) > 0) && inst.Metadata.Annotations[AnnotationAppVersion] == "" {
		t.Fatalf("%s did not populate default %s annotation", a.Metadata.Name, AnnotationAppVersion)
	}
	if a.StoragePolicy != nil && strings.TrimSpace(a.ConsoleDelivery) != "" {
		if inst.Spec.BackupPolicy == nil || inst.Spec.BackupPolicy.ConsoleDelivery != strings.TrimSpace(a.ConsoleDelivery) {
			t.Fatalf("%s did not propagate consoleDelivery %q to BackupPolicy", a.Metadata.Name, a.ConsoleDelivery)
		}
	}
	if a.Metadata.Name != "custom-container" && strings.TrimSpace(a.PreStopCommand) == "" {
		t.Fatalf("%s missing PreStopCommand", a.Metadata.Name)
	}
	if strings.TrimSpace(a.PreStopCommand) != "" && strings.TrimSpace(inst.Spec.Env[corev1.EnvPreStopHook]) == "" {
		t.Fatalf("%s did not propagate %s to Env", a.Metadata.Name, corev1.EnvPreStopHook)
	}
	assertRenderedFilesValid(t, a.Metadata.Name, inst.Spec.Files)
}

func assertRenderedFilesValid(t *testing.T, appName string, files []corev1.InstanceFile) {
	t.Helper()
	byPath := make(map[string]string, len(files))
	for _, f := range files {
		if strings.Contains(f.Content, "{{") || strings.Contains(f.Content, "<no value>") {
			t.Fatalf("%s file %s contains unrendered template token:\n%s", appName, f.Path, f.Content)
		}
		byPath[f.Path] = f.Content
	}
	switch appName {
	case "minecraft-paper":
		if !strings.Contains(byPath["server.properties"], "allow-nether=true") {
			t.Fatalf("minecraft server.properties missing allow-nether")
		}
	case "minecraft-fabric":
		if !strings.Contains(byPath["server.properties"], "allow-nether=true") ||
			!strings.Contains(byPath["server.properties"], "server-name=Minecraft Fabric Server") {
			t.Fatalf("minecraft-fabric server.properties incomplete:\n%s", byPath["server.properties"])
		}
	case "minecraft-neoforge":
		if !strings.Contains(byPath["server.properties"], "allow-nether=true") ||
			!strings.Contains(byPath["server.properties"], "server-name=Minecraft NeoForge Server") {
			t.Fatalf("minecraft-neoforge server.properties incomplete:\n%s", byPath["server.properties"])
		}
	case "palworld-dedicated":
		ini := byPath["Pal/Saved/Config/LinuxServer/PalWorldSettings.ini"]
		if !strings.Contains(ini, "RCONEnabled=True,RCONPort=25575") || !strings.Contains(ini, "ItemCorruptionMultiplier=") || byPath["rcon.yaml"] == "" {
			t.Fatalf("palworld settings incomplete")
		}
	case "project-zomboid":
		if !strings.Contains(byPath["Zomboid/Server/servertest.ini"], "WorkshopItems=") ||
			!strings.Contains(byPath["Zomboid/Server/servertest_SandboxVars.lua"], "ZombieLore = {") {
			t.Fatalf("project-zomboid templates incomplete")
		}
	case "dont-starve-together":
		assertDSTFilesValid(t, byPath)
	case "seven-days-to-die":
		assert7DTDFilesValid(t, byPath)
	case "valheim-dedicated":
		conf := byPath["saves/valheim-server.conf"]
		if !strings.Contains(conf, "serverName=") || !strings.Contains(conf, "saveInterval=") {
			t.Fatalf("valheim-dedicated saves/valheim-server.conf incomplete:\n%s", conf)
		}
	default:
		assertExtraBuiltInFilesValid(t, appName, byPath)
	}
}

func assertExtraBuiltInFilesValid(t *testing.T, appName string, byPath map[string]string) {
	t.Helper()
	switch appName {
	case "core-keeper":
		raw := byPath["saves/ServerConfig.json"]
		if !json.Valid([]byte(raw)) || !strings.Contains(raw, `"worldName"`) {
			t.Fatalf("core-keeper saves/ServerConfig.json invalid:\n%s", raw)
		}
	case "satisfactory-dedicated":
		gus := byPath["FactoryGame/Saved/Config/LinuxServer/GameUserSettings.ini"]
		eng := byPath["FactoryGame/Saved/Config/LinuxServer/Engine.ini"]
		if !strings.Contains(gus, "MaxPlayers=") || !strings.Contains(eng, "NetServerMaxTickRate=") {
			t.Fatalf("satisfactory-dedicated templates incomplete")
		}
	case "left-4-dead-2":
		if !strings.Contains(byPath["left4dead2/cfg/server.cfg"], "sv_gametypes") || byPath["rcon.yaml"] == "" {
			t.Fatalf("left-4-dead-2 templates incomplete")
		}
	case "counter-strike-2":
		if !strings.Contains(byPath["game/csgo/cfg/server.cfg"], "mp_maxrounds") || byPath["rcon.yaml"] == "" {
			t.Fatalf("counter-strike-2 templates incomplete")
		}
	case "rust-dedicated":
		cfg := byPath["server/rust/cfg/server.cfg"]
		if !strings.Contains(cfg, "server.worldsize") || !strings.Contains(cfg, "server.hostname") {
			t.Fatalf("rust-dedicated server.cfg incomplete:\n%s", cfg)
		}
	}
}

func assertDSTFilesValid(t *testing.T, byPath map[string]string) {
	t.Helper()
	master := byPath["DoNotStarveTogether/Cluster_1/Master/worldgenoverride.lua"]
	caves := byPath["DoNotStarveTogether/Cluster_1/Caves/worldgenoverride.lua"]
	if !strings.HasPrefix(master, "KLEI     1\n") || !strings.Contains(master, `preset = "SURVIVAL_TOGETHER"`) {
		t.Fatalf("dont-starve-together Master worldgenoverride.lua invalid:\n%s", master)
	}
	if !strings.HasPrefix(caves, "KLEI     1\n") || !strings.Contains(caves, `preset = "DST_CAVE"`) {
		t.Fatalf("dont-starve-together Caves worldgenoverride.lua invalid:\n%s", caves)
	}
}

func assert7DTDFilesValid(t *testing.T, byPath map[string]string) {
	t.Helper()
	raw := byPath["serverconfig.xml"]
	var doc any
	if err := xml.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("seven-days-to-die serverconfig.xml invalid XML: %v", err)
	}
	if strings.Contains(raw, "SaveGameFolder") || !strings.Contains(raw, `name="SandboxCode"`) || !strings.Contains(raw, `name="TelnetEnabled" value="true"`) {
		t.Fatalf("seven-days-to-die serverconfig.xml missing V 3.4 SandboxCode/Telnet or contains legacy SaveGameFolder")
	}
}

func verifyAppPresetsTargetsAndMetrics(t *testing.T, a *corev1.App) {
	t.Helper()
	schemaKeys := make(map[string]bool, len(a.ConfigSchema))
	for _, f := range a.ConfigSchema {
		schemaKeys[f.Name] = true
	}
	for _, ct := range a.ConsoleTargets {
		if ct.EnabledField != "" && !schemaKeys[ct.EnabledField] {
			t.Fatalf("%s consoleTarget %s references unknown enabledField %q", a.Metadata.Name, ct.ID, ct.EnabledField)
		}
	}
	for _, m := range a.Metrics {
		verifyAppMetricSpec(t, a.Metadata.Name, m, schemaKeys)
	}
	for _, p := range a.ConfigPresets {
		verifySingleAppPreset(t, a, p, schemaKeys)
	}
}

func verifyAppMetricSpec(t *testing.T, appName string, m corev1.AppMetricSpec, schemaKeys map[string]bool) {
	t.Helper()
	if m.MaxConfigKey != "" && !schemaKeys[m.MaxConfigKey] {
		t.Fatalf("%s metric %s references unknown maxConfigKey %q", appName, m.ID, m.MaxConfigKey)
	}
	for _, pat := range []string{m.Pattern, m.ItemPattern, m.IncPattern, m.DecPattern, m.ResetPattern} {
		if pat != "" {
			if _, err := regexp.Compile(pat); err != nil {
				t.Fatalf("%s metric %s has invalid regex %q: %v", appName, m.ID, pat, err)
			}
		}
	}
}

func verifySingleAppPreset(t *testing.T, a *corev1.App, p corev1.AppConfigPreset, schemaKeys map[string]bool) {
	t.Helper()
	if strings.TrimSpace(p.Name) == "" {
		t.Fatalf("%s preset %s has empty name", a.Metadata.Name, p.ID)
	}
	for k := range p.Values {
		if !schemaKeys[k] {
			t.Fatalf("%s preset %s references unknown schema key %q", a.Metadata.Name, p.ID, k)
		}
	}
	rawJSON, err := json.Marshal(p.Values)
	if err != nil {
		t.Fatalf("marshal preset %s/%s: %v", a.Metadata.Name, p.ID, err)
	}
	inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			Name:        "preset-" + a.Metadata.Name + "-" + p.ID,
			Annotations: map[string]string{AnnotationAppConfig: string(rawJSON)},
		},
	}
	if err := RenderInstanceSpec(a, inst); err != nil {
		t.Fatalf("%s preset %s failed RenderInstanceSpec: %v", a.Metadata.Name, p.ID, err)
	}
}

func TestBuiltInAppsServerAndAddonBindingsRender(t *testing.T) {
	byName := make(map[string]*corev1.App)
	for _, a := range BuiltInApps() {
		byName[a.Metadata.Name] = a
	}

	mc := byName["minecraft-paper"]
	if mc == nil || mc.ServerBinding == nil || mc.AddonBinding == nil {
		t.Fatalf("expected minecraft-paper to declare ServerBinding and AddonBinding, got %+v", mc)
	}
	inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			Name:        "mc-binding-test",
			Annotations: map[string]string{AnnotationAppConfig: `{}`},
		},
		Spec: corev1.InstanceSpec{
			Addons: []corev1.InstanceAddon{{Package: "essentialsx", Version: "2.21.0"}},
		},
	}
	if err := RenderInstanceSpec(mc, inst); err != nil {
		t.Fatalf("RenderInstanceSpec(minecraft-paper): %v", err)
	}
	if inst.Spec.ServerArtifact == nil || inst.Spec.ServerArtifact.Coordinate() != "mc-paper.paper.26.3" || inst.Spec.ServerArtifact.TargetPath != "paper-26.3.jar" {
		t.Fatalf("unexpected ServerArtifact: %+v", inst.Spec.ServerArtifact)
	}
	if len(inst.Spec.Addons) != 1 || inst.Spec.Addons[0].Coordinate() != "mc-paper-plugins.essentialsx.2.21.0" || inst.Spec.Addons[0].InstallMode != corev1.AddonInstallModeFlatDir || inst.Spec.Addons[0].TargetDir != "plugins" {
		t.Fatalf("unexpected normalized Addons: %+v", inst.Spec.Addons)
	}
	assertL4D2WorkshopMapRender(t, byName["left-4-dead-2"])
}

func TestBuiltInMinecraftFabricBindingRender(t *testing.T) {
	byName := make(map[string]*corev1.App)
	for _, a := range BuiltInApps() {
		byName[a.Metadata.Name] = a
	}
	fabric := byName["minecraft-fabric"]
	if fabric == nil || fabric.ServerBinding == nil || fabric.AddonBinding == nil {
		t.Fatalf("expected minecraft-fabric to declare ServerBinding and AddonBinding, got %+v", fabric)
	}
	fabInst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			Name:        "fabric-binding-test",
			Annotations: map[string]string{AnnotationAppConfig: `{}`},
		},
		Spec: corev1.InstanceSpec{
			Addons: []corev1.InstanceAddon{{Package: "fabric-api", Version: "0.119.0"}},
		},
	}
	if err := RenderInstanceSpec(fabric, fabInst); err != nil {
		t.Fatalf("RenderInstanceSpec(minecraft-fabric): %v", err)
	}
	if fabInst.Spec.ServerArtifact == nil || fabInst.Spec.ServerArtifact.Coordinate() != "mc-fabric.fabric.26.3" || fabInst.Spec.ServerArtifact.TargetPath != "." {
		t.Fatalf("unexpected Fabric ServerArtifact: %+v", fabInst.Spec.ServerArtifact)
	}
	if len(fabInst.Spec.Addons) != 1 || fabInst.Spec.Addons[0].Coordinate() != "mc-fabric-mods.fabric-api.0.119.0" || fabInst.Spec.Addons[0].InstallMode != corev1.AddonInstallModeFlatDir || fabInst.Spec.Addons[0].TargetDir != "mods" {
		t.Fatalf("unexpected Fabric normalized Addons: %+v", fabInst.Spec.Addons)
	}
	assertBuiltInMinecraftNeoForgeBindingRender(t, byName["minecraft-neoforge"])
	assertBuiltInValheimBepInExBindingRender(t, byName["valheim-dedicated"])
	assertBuiltInRustOxideBindingRender(t, byName["rust-dedicated"])
	assertBuiltInSatisfactoryAndCS2Bindings(t, byName["satisfactory-dedicated"], byName["counter-strike-2"])
}

func assertBuiltInSatisfactoryAndCS2Bindings(t *testing.T, sf, cs2 *corev1.App) {
	t.Helper()
	if sf == nil || sf.AddonBinding == nil || sf.AddonBinding.Repository != "satisfactory-ficsit" ||
		sf.AddonBinding.InstallMode != corev1.AddonInstallModeSML || sf.AddonBinding.TargetDir != "FactoryGame/Mods" {
		t.Fatalf("unexpected satisfactory-dedicated AddonBinding: %+v", sf)
	}
	if cs2 == nil || cs2.AddonBinding == nil || cs2.AddonBinding.Repository != "cs2-plugins" ||
		cs2.AddonBinding.InstallMode != corev1.AddonInstallModeCS2Plugin || len(cs2.AddonBinding.Additional) != 1 ||
		cs2.AddonBinding.Additional[0].Repository != "cs2-workshop" || cs2.AddonBinding.Additional[0].InstallMode != corev1.AddonInstallModeCS2Workshop {
		t.Fatalf("unexpected counter-strike-2 dual AddonBinding: %+v", cs2)
	}
	cs2Inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "cs2-multi-binding", Annotations: map[string]string{AnnotationAppConfig: `{"startMap":"3070244462"}`}},
		Spec: corev1.InstanceSpec{
			Addons: []corev1.InstanceAddon{
				{Repository: "cs2-plugins", Package: "metamod", Version: "2.0.0-git1491"},
				{Repository: "cs2-workshop", Package: "3070244462", Version: "1"},
			},
		},
	}
	if err := RenderInstanceSpec(cs2, cs2Inst); err != nil {
		t.Fatalf("RenderInstanceSpec(cs2 multi-binding): %v", err)
	}
	if cs2Inst.Spec.Addons[0].InstallMode != corev1.AddonInstallModeCS2Plugin || cs2Inst.Spec.Addons[0].TargetDir != "game/csgo/addons" ||
		cs2Inst.Spec.Addons[1].InstallMode != corev1.AddonInstallModeCS2Workshop || cs2Inst.Spec.Addons[1].TargetDir != "game/csgo/addons/workshop" {
		t.Fatalf("unexpected normalized multi-binding CS2 addons: %+v", cs2Inst.Spec.Addons)
	}
}

func assertBuiltInRustOxideBindingRender(t *testing.T, rust *corev1.App) {
	t.Helper()
	if rust == nil || rust.AddonBinding == nil || rust.AddonBinding.Repository != "rust-umod" ||
		rust.AddonBinding.InstallMode != corev1.AddonInstallModeOxide || rust.AddonBinding.TargetDir != "oxide" {
		t.Fatalf("unexpected rust-dedicated AddonBinding: %+v", rust)
	}
	inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "rust-binding-test", Annotations: map[string]string{AnnotationAppConfig: `{}`}},
		Spec: corev1.InstanceSpec{
			Addons: []corev1.InstanceAddon{{Package: "gather-manager", Version: "2.2.78"}},
		},
	}
	if err := RenderInstanceSpec(rust, inst); err != nil {
		t.Fatalf("RenderInstanceSpec(rust-dedicated): %v", err)
	}
	if len(inst.Spec.Addons) != 1 || inst.Spec.Addons[0].Coordinate() != "rust-umod.gather-manager.2.2.78" ||
		inst.Spec.Addons[0].InstallMode != corev1.AddonInstallModeOxide || inst.Spec.Addons[0].TargetDir != "oxide" {
		t.Fatalf("unexpected Rust normalized Addons: %+v", inst.Spec.Addons)
	}
}

func assertBuiltInValheimBepInExBindingRender(t *testing.T, valheim *corev1.App) {
	t.Helper()
	if valheim == nil || valheim.AddonBinding == nil || valheim.AddonBinding.Repository != "valheim-thunderstore" ||
		valheim.AddonBinding.InstallMode != corev1.AddonInstallModeBepInEx || valheim.AddonBinding.TargetDir != "BepInEx" {
		t.Fatalf("unexpected valheim-dedicated AddonBinding: %+v", valheim)
	}
	if !strings.Contains(valheim.StartupCommand, "DOORSTOP_ENABLED=1") || !strings.Contains(valheim.StartupCommand, "BepInEx.Preloader.dll") {
		t.Fatalf("expected valheim-dedicated StartupCommand to enable Doorstop when BepInEx is installed")
	}
	inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{Name: "valheim-binding-test", Annotations: map[string]string{AnnotationAppConfig: `{}`}},
		Spec: corev1.InstanceSpec{
			Addons: []corev1.InstanceAddon{{Package: "ValheimModding-Jotunn", Version: "2.30.2"}},
		},
	}
	if err := RenderInstanceSpec(valheim, inst); err != nil {
		t.Fatalf("RenderInstanceSpec(valheim-dedicated): %v", err)
	}
	if len(inst.Spec.Addons) != 1 || inst.Spec.Addons[0].Coordinate() != "valheim-thunderstore.ValheimModding-Jotunn.2.30.2" ||
		inst.Spec.Addons[0].InstallMode != corev1.AddonInstallModeBepInEx || inst.Spec.Addons[0].TargetDir != "BepInEx" {
		t.Fatalf("unexpected Valheim normalized Addons: %+v", inst.Spec.Addons)
	}
}

func assertBuiltInMinecraftNeoForgeBindingRender(t *testing.T, neo *corev1.App) {
	t.Helper()
	if neo == nil || neo.ServerBinding == nil || neo.AddonBinding == nil {
		t.Fatalf("expected minecraft-neoforge to declare ServerBinding and AddonBinding, got %+v", neo)
	}
	inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			Name:        "neoforge-binding-test",
			Annotations: map[string]string{AnnotationAppConfig: `{}`},
		},
		Spec: corev1.InstanceSpec{
			Addons: []corev1.InstanceAddon{{Package: "ferrite-core", Version: "7.1.3-neoforge"}},
		},
	}
	if err := RenderInstanceSpec(neo, inst); err != nil {
		t.Fatalf("RenderInstanceSpec(minecraft-neoforge): %v", err)
	}
	if inst.Spec.ServerArtifact == nil || inst.Spec.ServerArtifact.Coordinate() != "mc-neoforge.neoforge.26.3" || inst.Spec.ServerArtifact.TargetPath != "." {
		t.Fatalf("unexpected NeoForge ServerArtifact: %+v", inst.Spec.ServerArtifact)
	}
	if len(inst.Spec.Addons) != 1 || inst.Spec.Addons[0].Coordinate() != "mc-neoforge-mods.ferrite-core.7.1.3-neoforge" || inst.Spec.Addons[0].InstallMode != corev1.AddonInstallModeFlatDir || inst.Spec.Addons[0].TargetDir != "mods" {
		t.Fatalf("unexpected NeoForge normalized Addons: %+v", inst.Spec.Addons)
	}
}

func assertL4D2WorkshopMapRender(t *testing.T, l4d2 *corev1.App) {
	t.Helper()
	if l4d2 == nil {
		t.Fatalf("expected left-4-dead-2 built-in app")
	}
	l4d2Inst := &corev1.Instance{
		Metadata: metav1.ObjectMeta{
			Name:        "l4d2-addon-map-test",
			Annotations: map[string]string{AnnotationAppConfig: `{"startMap":"uf1_boulevard"}`},
		},
	}
	if err := RenderInstanceSpec(l4d2, l4d2Inst); err != nil {
		t.Fatalf("RenderInstanceSpec(left-4-dead-2 workshop startMap): %v", err)
	}
	if l4d2Inst.Spec.Env["START_MAP"] != "uf1_boulevard" {
		t.Fatalf("expected START_MAP=uf1_boulevard, got %q", l4d2Inst.Spec.Env["START_MAP"])
	}
}
