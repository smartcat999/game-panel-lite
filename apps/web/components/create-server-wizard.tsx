"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bookmark, Check, ChevronDown, ChevronLeft, ChevronRight, FileArchive, Gamepad2, Globe, Hammer, Package, Search, Settings2, X } from "lucide-react";
import Image from "next/image";
import { useEffect, useMemo, useState } from "react";
import { Button, Card, Input } from "@/components/ui";
import { ProviderConfigEditor } from "@/components/provider-config-editor";
import { ResourceLimitSlider, formatCpuResourceLimit, formatMemoryResourceLimit } from "@/components/resource-limit-slider";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { modDisplayName } from "@/lib/mod-display";
import { showWorldAndBackupFeatures } from "@/lib/feature-flags";
import { getGameArt } from "@/lib/game-art";
import { gameDescription, gameDisplayName } from "@/lib/game-display";
import { providerDescription, providerDisplayName } from "@/lib/provider-display";
import { formatCreateServerError } from "@/lib/runtime-errors";
import { cn } from "@/lib/utils";
import { createConfigPreset, getGameVersions, getRuntimeStats, getSettings, listConfigPresets, listGames, listGlobalMods, listModPacks, listWorlds } from "@/lib/api";
import { defaultCreateServerConfig, defaultCreateServerMode, defaultCreateServerPreset } from "@/lib/create-server-defaults";
import { createGameServerWithResources } from "@/lib/create-server-flow";
import { createReviewInvitePreview, reviewJoinInstructionKey } from "@/lib/create-server-review";
import { filterModResources } from "@/lib/mod-filters";
import { createDefaultProviderConfigPayload, isAdvancedProviderConfigField, isProviderFieldModified, providerConfigValue, restoreProviderConfigDefaults, updateProviderConfigPayload, type ProviderConfigPayload } from "@/lib/provider-config";
import { providerOptionLabel } from "@/lib/provider-option-label";
import { isRuntimeImageReady, runtimeImageLabelKey, runtimeImageTone } from "@/lib/runtime-image";
import {
  getTerrariaPreset,
  isTerrariaVersionAtLeast,
  secretSeedKeyFor,
  terrariaInternalPort,
  terrariaLegacySpecialWorldSeeds,
  terrariaSecretWorldSeeds145,
  terrariaSeedModeCodes,
  terrariaSecretSeeds,
  terrariaSpecialWorldSeeds,
  type TerrariaConfig
} from "@gamepanel-lite/shared";
import type { ConfigPreset, GameCatalogEntry, ModFile, ModPack, ProviderCatalog, ProviderConfigField, ProviderKey, ResourceLimits, RuntimeImageStatus } from "@/lib/types";

const stepLabelKeys = {
  setup: "stepGameMode",
  config: "stepConfig",
  resources: "stepResources",
  mods: "stepMods",
  review: "stepReview"
} as const;
type StepId = keyof typeof stepLabelKeys;
const presets = [
  { key: "friends-casual", labelKey: "presetFriendsCasual", descriptionKey: "presetFriendsCasualDescription", tags: ["tagClassic", "tagMediumWorld", "8"] },
  { key: "building-world", labelKey: "presetBuildingWorld", descriptionKey: "presetBuildingWorldDescription", tags: ["tagClassic", "tagLargeWorld", "12"] },
  { key: "expert-adventure", labelKey: "presetExpertAdventure", descriptionKey: "presetExpertAdventureDescription", tags: ["tagExpert", "tagLargeWorld", "8"] },
  { key: "master-challenge", labelKey: "presetMasterChallenge", descriptionKey: "presetMasterChallengeDescription", tags: ["tagMaster", "tagLargeWorld", "6"] }
] as const;
const customPreset = { key: "custom", labelKey: "presetCustom", descriptionKey: "presetCustomDescription", tags: ["tagCustom"] } as const;
const tmodLoaderBasePreset = "modded-starter" as const;

const terrariaSeedChineseNames: Record<string, string> = {
  "05162020": "Drunk（醉酒世界）",
  "for the worthy": "For the Worthy",
  "not the bees": "Not the Bees",
  celebrationmk10: "Celebration Mk 10",
  "the constant": "The Constant",
  "no traps": "No Traps",
  dontdigup: "Don't dig up",
  getfixedboi: "Zenith",
  skyblock: "Skyblock"
};

function terrariaSeedDisplayName(seed: { key: string; label: string }, locale: string) {
  return locale.startsWith("zh") ? terrariaSeedChineseNames[seed.key] ?? seed.label : seed.label;
}

type BuiltInPresetKey = (typeof presets)[number]["key"];
type PresetKey = BuiltInPresetKey | typeof customPreset.key;
type PresetTag = (typeof presets)[number]["tags"][number] | (typeof customPreset)["tags"][number];

type ConfigValidationErrors = Record<string, string>;
type ReviewConfigField = { label: string; value: string };
type ReviewConfigModel = {
  serverName: string;
  password: string;
  fields: ReviewConfigField[];
};
const modeProviderPriority: Record<string, number> = {
  "terraria-vanilla": 10,
  "terraria-tmodloader": 20
};

const providerFieldLabelKeys: Record<string, MessageKey> = {
  adminPassword: "adminPassword",
  cavesEnabled: "cavesEnabled",
  clusterDescription: "clusterDescription",
  clusterName: "clusterName",
  clusterToken: "clusterToken",
  consoleEnabled: "consoleEnabled",
  difficulty: "difficulty",
  eulaAccepted: "minecraftEulaAccepted",
  gameMode: "gameMode",
  maxPlayers: "maxPlayersInput",
  offlineServer: "offlineServer",
  onlineMode: "onlineMode",
  pauseWhenEmpty: "pauseWhenEmpty",
  pvp: "pvp",
  saveName: "saveName",
  serverName: "serverName",
  serverPassword: "serverPassword",
  whitelistEnabled: "whitelistEnabled",
  worldName: "worldName",
  worldPreset: "worldPreset"
};

Object.assign(providerFieldLabelKeys, {
  "caves.enabled": "cavesEnabled",
  "gameplay.consoleEnabled": "consoleEnabled",
  "gameplay.gameMode": "gameMode",
  "gameplay.maxPlayers": "maxPlayersInput",
  "gameplay.pauseWhenEmpty": "pauseWhenEmpty",
  "gameplay.pvp": "pvp",
  "identity.clusterName": "clusterName",
  "identity.clusterToken": "clusterToken",
  "identity.description": "clusterDescription",
  "identity.password": "serverPassword",
  "identity.serverName": "serverName",
  "identity.visibility": "visibility",
  "world.preset": "worldPreset"
} satisfies Record<string, MessageKey>);

Object.assign(providerFieldLabelKeys, {
  "world.overrides.world_size": "dstWorldSize",
  "world.overrides.day": "dstDayCycle",
  "world.overrides.season_start": "dstSeasonStart",
  "world.overrides.autumn": "dstAutumnLength",
  "world.overrides.winter": "dstWinterLength",
  "world.overrides.spring": "dstSpringLength",
  "world.overrides.summer": "dstSummerLength",
  "world.overrides.grass": "dstGrass",
  "world.overrides.sapling": "dstSaplings",
  "world.overrides.berrybush": "dstBerryBushes",
  "world.overrides.flint": "dstFlint",
  "world.overrides.rock": "dstRocks",
  "world.overrides.rabbits": "dstRabbits",
  "world.overrides.pigs": "dstPigs",
  "world.overrides.beefalo": "dstBeefalo",
  "world.overrides.spiders": "dstSpiderDens",
  "world.overrides.houndmound": "dstHoundMounds",
  "world.overrides.hounds": "dstHoundAttacks",
  "world.overrides.wildfires": "dstWildfires",
  "world.overrides.deerclops": "dstDeerclops",
  "world.overrides.bearger": "dstBearger",
  "world.overrides.goosemoose": "dstMooseGoose",
  "world.overrides.dragonfly": "dstDragonfly",
  "world.overrides.regrowth": "dstRegrowth",
  "caves.overrides.world_size": "dstCaveSize",
  "caves.overrides.cavelight": "dstCaveLight",
  "caves.overrides.grass": "dstCaveGrass",
  "caves.overrides.sapling": "dstCaveSaplings",
  "caves.overrides.berrybush": "dstCaveBerryBushes",
  "caves.overrides.flower_cave": "dstLightFlowers",
  "caves.overrides.wormlights": "dstGlowBerries",
  "caves.overrides.cave_spiders": "dstCaveSpiderDens",
  "caves.overrides.bats": "dstBatCaves",
  "caves.overrides.earthquakes": "dstEarthquakes",
  "caves.overrides.wormattacks": "dstWormAttacks",
  "caves.overrides.toadstool": "dstToadstool"
} satisfies Record<string, MessageKey>);

function createNameSuffix(date = new Date()) {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${pad(date.getMonth() + 1)}${pad(date.getDate())}-${pad(date.getHours())}${pad(date.getMinutes())}`;
}

function appendNameSuffix(name: string, suffix: string) {
  const nextName = `${name} ${suffix}`;
  if (nextName.length <= 80) return nextName;
  return `${name.slice(0, Math.max(1, 79 - suffix.length)).trim()} ${suffix}`;
}

function createNamedTerrariaConfig(presetKey: BuiltInPresetKey | typeof tmodLoaderBasePreset) {
  const suffix = createNameSuffix();
  const presetConfig = getTerrariaPreset(presetKey).config;
  return editableTerrariaConfig({
    ...presetConfig,
    serverName: appendNameSuffix(presetConfig.serverName || "Terraria Server", suffix),
    worldName: appendNameSuffix(presetConfig.worldName || "Terraria World", suffix)
  });
}

type EditableTerrariaConfigInput = Omit<TerrariaConfig, "specialSeeds" | "secretSeeds"> & {
  specialSeeds?: readonly string[];
  secretSeeds?: readonly string[];
};

function editableTerrariaConfig(config: EditableTerrariaConfigInput): TerrariaConfig {
  return {
    ...config,
    specialSeeds: [...(config.specialSeeds ?? [])],
    secretSeeds: [...(config.secretSeeds ?? [])]
  };
}

function createGameDefaultNames(gameName: string, locale: "zh" | "en") {
  const suffix = createNameSuffix();
  const label = (zh: string, en: string) => locale === "zh" ? zh : en;
  return {
    clusterName: appendNameSuffix(`${gameName} ${label("集群", "Cluster")}`, suffix),
    saveName: appendNameSuffix(`${gameName} ${label("存档", "Save")}`, suffix),
    serverName: appendNameSuffix(`${gameName} ${label("服务器", "Server")}`, suffix),
    worldName: appendNameSuffix(`${gameName} ${label("世界", "World")}`, suffix)
  };
}

function createProviderDefaultOverrides(names: ReturnType<typeof createGameDefaultNames>, gameKey: string, locale: "zh" | "en") {
  const overrides: ProviderConfigPayload = {
    clusterName: names.clusterName,
    saveName: names.saveName,
    serverName: names.serverName,
    worldName: names.worldName
  };
  if (gameKey === "dont-starve-together") {
    overrides.identity = {
      description: locale === "zh" ? "由 GamePanel Lite 管理" : "Managed by GamePanel Lite",
      serverName: names.serverName
    };
  }
  return overrides;
}

function formatCpuLimitLabel(value: number, t: (key: MessageKey, values?: Record<string, string | number>) => string) {
  return value > 0 ? t("cpuCoresValue", { cores: value }) : t("unlimited");
}

function formatMemoryLimitLabel(value: number, t: (key: MessageKey, values?: Record<string, string | number>) => string) {
  return value > 0 ? t("memoryGbValue", { gb: value / 1024 }) : t("unlimited");
}

function orderModeProviders(providers: ProviderCatalog[]) {
  return [...providers].sort((left, right) => {
    const leftPriority = modeProviderPriority[left.key] ?? 100;
    const rightPriority = modeProviderPriority[right.key] ?? 100;
    if (leftPriority !== rightPriority) return leftPriority - rightPriority;
    return left.key.localeCompare(right.key);
  });
}

function providerFieldLabel(field: ProviderConfigField, t: (key: MessageKey, values?: Record<string, string | number>) => string) {
  const key = providerFieldLabelKeys[field.name];
  return key ? t(key) : field.label;
}

function providerFieldHelp(field: ProviderConfigField, t: (key: MessageKey, values?: Record<string, string | number>) => string) {
  if (field.name === "adminPassword") return t("adminPasswordHelp");
  if (field.name === "clusterToken" || field.name === "identity.clusterToken") return t("clusterTokenHelp");
  if (field.name === "eulaAccepted") return t("minecraftEulaHelp");
  return field.help ?? "";
}

function validateCreateConfig({
  config,
  gameKey,
  provider,
  providerConfigPayload,
  t
}: {
  config: TerrariaConfig;
  gameKey: string;
  provider?: ProviderCatalog;
  providerConfigPayload: ProviderConfigPayload;
  t: (key: MessageKey, values?: Record<string, string | number>) => string;
}) {
  const errors: ConfigValidationErrors = {};
  if (gameKey === "terraria") {
    if (!String(config.serverName ?? "").trim()) errors.serverName = t("requiredFieldError", { field: t("serverName") });
    if (!String(config.worldName ?? "").trim()) errors.worldName = t("requiredFieldError", { field: t("worldName") });
    if (!Number.isFinite(config.maxPlayers) || config.maxPlayers < 1) errors.maxPlayers = t("positiveNumberFieldError", { field: t("maxPlayersInput") });
    return errors;
  }

  for (const field of provider?.configSchema ?? []) {
    if (!field.required) continue;
    const value = providerConfigValue(providerConfigPayload, field.name);
    const label = providerFieldLabel(field, t);
    if (field.type === "boolean") {
      if (value !== true) errors[field.name] = t("requiredAgreementError", { field: label });
      continue;
    }
    if (field.type === "number") {
      const numberValue = typeof value === "number" ? value : Number(value);
      if (!Number.isFinite(numberValue) || numberValue < 1) errors[field.name] = t("positiveNumberFieldError", { field: label });
      continue;
    }
    if (!String(value ?? "").trim()) {
      errors[field.name] = t("requiredFieldError", { field: label });
    }
  }
  return errors;
}

function stringPayloadValue(payload: ProviderConfigPayload | undefined, key: string): string {
  const value = providerConfigValue(payload, key) ?? payload?.[key];
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return "";
}

function numberPayloadValue(payload: ProviderConfigPayload | undefined, key: string, fallback: number): number {
  const value = providerConfigValue(payload, key) ?? payload?.[key];
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    if (Number.isFinite(parsed)) return parsed;
  }
  return fallback;
}

function arrayPayloadValue(payload: ProviderConfigPayload | undefined, key: string): string[] {
  const value = payload?.[key];
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

function terrariaConfigFromPayload(payload: ProviderConfigPayload | undefined, fallback: TerrariaConfig = defaultCreateServerConfig): TerrariaConfig {
  return editableTerrariaConfig({
    ...fallback,
    serverName: stringPayloadValue(payload, "serverName") || fallback.serverName,
    worldName: stringPayloadValue(payload, "worldName") || fallback.worldName,
    worldSize: (stringPayloadValue(payload, "worldSize") || fallback.worldSize) as TerrariaConfig["worldSize"],
    worldEvil: (stringPayloadValue(payload, "worldEvil") || fallback.worldEvil) as TerrariaConfig["worldEvil"],
    difficulty: (stringPayloadValue(payload, "difficulty") || fallback.difficulty) as TerrariaConfig["difficulty"],
    maxPlayers: numberPayloadValue(payload, "maxPlayers", fallback.maxPlayers),
    port: numberPayloadValue(payload, "port", fallback.port),
    password: stringPayloadValue(payload, "password") || fallback.password || "",
    motd: stringPayloadValue(payload, "motd") || fallback.motd || "",
    seed: stringPayloadValue(payload, "seed") || fallback.seed || "",
    specialSeeds: arrayPayloadValue(payload, "specialSeeds"),
    secretSeeds: arrayPayloadValue(payload, "secretSeeds"),
    secure: typeof payload?.secure === "boolean" ? payload.secure : fallback.secure,
    language: stringPayloadValue(payload, "language") || fallback.language,
    autoCreateWorld: typeof payload?.autoCreateWorld === "boolean" ? payload.autoCreateWorld : fallback.autoCreateWorld
  });
}

function providerServerName(payload: ProviderConfigPayload, fallback: string) {
  return stringPayloadValue(payload, "serverName") || stringPayloadValue(payload, "identity.serverName") || stringPayloadValue(payload, "clusterName") || stringPayloadValue(payload, "identity.clusterName") || fallback;
}

function providerJoinPassword(payload: ProviderConfigPayload) {
  return stringPayloadValue(payload, "password") || stringPayloadValue(payload, "identity.password") || stringPayloadValue(payload, "serverPassword");
}

function providerReviewValue(field: ProviderConfigField, value: unknown, t: (key: MessageKey, values?: Record<string, string | number>) => string): string {
  if (field.type === "boolean") return value === true ? t("enabled") : t("disabled");
  if (field.type === "password") return String(value ?? "").trim() ? t("enabled") : t("none");
  if (field.type === "select") {
    const option = field.options?.find((item) => item.value === value);
    return option ? providerOptionLabel(field, option.value, option.label, t) : String(value ?? "");
  }
  return String(value ?? "");
}

function createReviewConfigModel({
  config,
  gameKey,
  gameName,
  hostPortLabel,
  locale,
  provider,
  providerConfigPayload,
  t,
  version
}: {
  config: TerrariaConfig;
  gameKey: string;
  gameName: string;
  hostPortLabel: string;
  locale: string;
  provider?: ProviderCatalog;
  providerConfigPayload: ProviderConfigPayload;
  t: (key: MessageKey, values?: Record<string, string | number>) => string;
  version: string;
}): ReviewConfigModel {
  if (gameKey === "terraria") {
    const secretSeed = secretSeedKeyFor(config.seed);
    const secretSeedDefinition = terrariaSecretSeeds.find((seed) => seed.key === secretSeed);
    const selectedSeedModeNames = terrariaSeedModeCodes(config).map((key) => {
      const definition = terrariaSpecialWorldSeeds.find((seed) => seed.key === key)
        ?? terrariaSecretWorldSeeds145.find((seed) => seed.key === key);
      return definition ? terrariaSeedDisplayName(definition, locale) : key;
    });
    const baseSeedLabel = secretSeed
      ? `${secretSeedDefinition ? terrariaSeedDisplayName(secretSeedDefinition, locale) : secretSeed} · ${secretSeed}`
      : config.seed?.trim() || t("tagRandom");
    const seedModeList = selectedSeedModeNames.join(locale.startsWith("zh") ? "、" : ", ");
    const seedLabel = selectedSeedModeNames.length > 0
      ? config.seed?.trim()
        ? t("worldSeedWithModes", { seed: config.seed.trim(), modes: seedModeList })
        : t("combinedWorldSeed", { modes: seedModeList })
      : baseSeedLabel;
    const worldSizeLabel = config.worldSize === "small" ? t("tagSmallWorld") : config.worldSize === "medium" ? t("tagMediumWorld") : t("tagLargeWorld");
    const worldEvilLabel = config.worldEvil === "corruption" ? t("tagCorruption") : config.worldEvil === "crimson" ? t("tagCrimson") : t("tagRandom");
    const difficultyLabel = config.difficulty === "journey" ? t("tagJourney") : config.difficulty === "classic" ? t("tagClassic") : config.difficulty === "expert" ? t("tagExpert") : t("tagMaster");
    return {
      serverName: config.serverName || gameName,
      password: config.password ?? "",
      fields: [
        { label: t("serverName"), value: config.serverName || gameName },
        { label: t("worldName"), value: config.worldName },
        { label: t("worldSize"), value: worldSizeLabel },
        { label: t("worldEvil"), value: worldEvilLabel },
        { label: t("difficulty"), value: difficultyLabel },
        { label: t("worldSeed"), value: seedLabel },
        { label: t("maxPlayersInput"), value: String(config.maxPlayers) },
        { label: t("password"), value: config.password ? t("enabled") : t("none") },
        { label: t("secureMode"), value: config.secure ? t("enabled") : t("disabled") },
        { label: t("autoCreateWorld"), value: config.autoCreateWorld ? t("enabled") : t("disabled") },
        ...(version ? [{ label: t("gameVersion"), value: version }] : []),
        { label: t("externalPort"), value: hostPortLabel }
      ]
    };
  }

  const providerFields = provider?.configSchema ?? [];
  const modifiedAdvancedCount = providerFields.filter((field) => isAdvancedProviderConfigField(provider?.key ?? "", field) && isProviderFieldModified(providerConfigPayload, field)).length;
  const fields = providerFields
    .filter((field) => !isAdvancedProviderConfigField(provider?.key ?? "", field))
    .map((field): ReviewConfigField | null => {
      const value = providerConfigValue(providerConfigPayload, field.name);
      const formatted = providerReviewValue(field, value, t);
      if (!field.required && field.type !== "boolean" && formatted.trim() === "") return null;
      return { label: providerFieldLabel(field, t), value: formatted };
    })
    .filter((field): field is ReviewConfigField => Boolean(field));

  return {
    serverName: providerServerName(providerConfigPayload, gameName),
    password: providerJoinPassword(providerConfigPayload),
    fields: [
      ...fields,
      ...(providerFields.some((field) => isAdvancedProviderConfigField(provider?.key ?? "", field))
        ? [{ label: t("advancedGameSettings"), value: t("modifiedSettingsCount", { count: modifiedAdvancedCount }) }]
        : []),
      ...(version ? [{ label: t("gameVersion"), value: version }] : []),
      { label: t("externalPort"), value: hostPortLabel }
    ]
  };
}

export function CreateServerWizard() {
  const { locale, t } = useI18n();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [step, setStep] = useState(0);
  const [selectedGameKey, setSelectedGameKey] = useState("");
  const [selectedProviderKey, setSelectedProviderKey] = useState<ProviderKey>("terraria-vanilla");
  const [mode, setMode] = useState<"vanilla" | "tmodloader">(defaultCreateServerMode);
  const [selectedPreset, setSelectedPreset] = useState<PresetKey>(defaultCreateServerPreset);
  const [config, setConfig] = useState<TerrariaConfig>(() => createNamedTerrariaConfig(defaultCreateServerPreset));
  const [providerConfigPayload, setProviderConfigPayload] = useState<ProviderConfigPayload>({});
  const [hostPortMode, setHostPortMode] = useState<"auto" | "manual">("auto");
  const [hostPort, setHostPort] = useState(terrariaInternalPort);
  const [resourceLimits, setResourceLimits] = useState<ResourceLimits>({ cpuLimitCores: 0, memoryLimitMb: 0 });
  const [version, setVersion] = useState("");
  const [configValidationErrors, setConfigValidationErrors] = useState<ConfigValidationErrors>({});
  const [saveAsPreset, setSaveAsPreset] = useState(false);
  const [presetName, setPresetName] = useState("");
  const [presetSavedForCurrentSubmit, setPresetSavedForCurrentSubmit] = useState(false);
  const [selectedWorldId, setSelectedWorldId] = useState("");
  const [appliedWorldConfigId, setAppliedWorldConfigId] = useState("");
  const [appliedConfigPresetId, setAppliedConfigPresetId] = useState("");
  const [appliedGameQueryKey, setAppliedGameQueryKey] = useState("");
  const [selectedModIds, setSelectedModIds] = useState<string[]>([]);
  const [selectedModPackId, setSelectedModPackId] = useState("");
  const gamesQuery = useQuery({ queryKey: ["games"], queryFn: listGames, staleTime: 5 * 60 * 1000 });
  const versionsQuery = useQuery({ queryKey: ["game-versions", selectedGameKey], queryFn: () => getGameVersions(selectedGameKey), enabled: selectedGameKey.length > 0, staleTime: 5 * 60 * 1000 });
  const worldsQuery = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, enabled: showWorldAndBackupFeatures, retry: false });
  const modsQuery = useQuery({ queryKey: ["global-mods"], queryFn: listGlobalMods, retry: false });
  const modPacksQuery = useQuery({ queryKey: ["mod-packs"], queryFn: listModPacks, retry: false });
  const configPresetsQuery = useQuery({ queryKey: ["config-presets"], queryFn: listConfigPresets, retry: false });
  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, staleTime: 5 * 60 * 1000, retry: false });
  const runtimeStatsQuery = useQuery({ queryKey: ["runtime-stats"], queryFn: getRuntimeStats, retry: false, staleTime: 30_000 });
  const games = gamesQuery.data ?? [];
  const selectedGame = games.find((game) => game.key === selectedGameKey) ?? games[0] ?? games.find((game) => game.key === "terraria");
  const selectedGameArt = getGameArt(selectedGame?.coverImage ?? selectedGame?.key ?? selectedGameKey);
  const SelectedGameIcon = selectedGameArt.icon;
  const selectedProvider = selectedGame?.providers.find((provider) => provider.key === selectedProviderKey) ?? selectedGame?.providers.find((provider) => provider.recommended) ?? selectedGame?.providers[0];
  const providerKey = selectedProvider?.key ?? selectedProviderKey;
  const stepIds: StepId[] = ["setup", "config", "resources", "mods", "review"];
  const availableVersions = versionsQuery.data?.[providerKey] ?? [];
  const selectedVersion = availableVersions.includes(version) ? version : availableVersions[0] || "";
  const allWorlds = showWorldAndBackupFeatures ? worldsQuery.data ?? [] : [];
  const selectedWorld = allWorlds.find((w) => w.id === selectedWorldId);
  const allMods = modsQuery.data ?? [];
  const allModPacks = modPacksQuery.data ?? [];
  const availableMods = filterModResources(allMods, selectedGameKey);
  const modPacks = filterModResources(allModPacks, selectedGameKey);
  const configPresets = configPresetsQuery.data ?? [];
  const gameConfigPresets = configPresets.filter((preset) => preset.gameKey === selectedGameKey);
  const selectedModNames = availableMods.filter((m) => selectedModIds.includes(m.id)).map((m) => modDisplayName(m, locale));
  const fallbackStepId: StepId = "review";
  const currentStepId = stepIds[step] ?? fallbackStepId;
  const nextStepId = stepIds[Math.min(stepIds.length - 1, step + 1)] ?? fallbackStepId;
  const currentStepKey = stepLabelKeys[currentStepId];
  const nextStepKey = stepLabelKeys[nextStepId];
  const selectedTitle = useMemo(() => t(currentStepKey), [currentStepKey, t]);
  const selectedProviderReady = isRuntimeImageReady(selectedProvider?.runtimeImage);
  const selectedGameHasReadyProvider = Boolean(selectedGame?.providers.some((provider) => isRuntimeImageReady(provider.runtimeImage)));
  const canContinueCurrentStep = currentStepId === "setup"
    ? selectedGame?.status === "available" && selectedGameHasReadyProvider && selectedProviderReady
    : true;
  const canCreateSelectedProvider = selectedGame?.status === "available" && Boolean(selectedProvider) && selectedProviderReady;
  const validateCurrentConfig = () => {
    const errors = validateCreateConfig({
      config,
      gameKey: selectedGameKey,
      provider: selectedProvider,
      providerConfigPayload,
      t
    });
    setConfigValidationErrors(errors);
    if (Object.keys(errors).length === 0) return true;
    const configStep = stepIds.indexOf("config");
    if (configStep >= 0) {
      setStep(configStep);
    }
    return false;
  };
  const create = useMutation({
    mutationFn: () => createGameServerWithResources({
      name: selectedGameKey === "terraria"
        ? config.serverName || "Terraria Server"
        : providerServerName(providerConfigPayload, providerDisplayName(providerKey, providerKey, t) || "Game Server"),
      config: selectedGameKey === "terraria" ? { ...config, port: terrariaInternalPort } : providerConfigPayload,
      hostPort: hostPortMode === "manual" ? hostPort : undefined,
      mode,
      providerKey,
      resources: resourceLimits,
      worldId: showWorldAndBackupFeatures ? selectedWorldId || undefined : undefined,
      modIds: selectedModIds,
      version: selectedVersion
    }),
    onSuccess: async ({ server }) => {
      await queryClient.invalidateQueries({ queryKey: ["game-servers"] });
      if (showWorldAndBackupFeatures) {
        await queryClient.invalidateQueries({ queryKey: ["worlds"] });
        await queryClient.invalidateQueries({ queryKey: ["backups"] });
      }
      await queryClient.invalidateQueries({ queryKey: ["mods", server.id] });
      queryClient.setQueryData(["game-server", server.id], server);
      router.push(`/servers/${server.id}`);
    }
  });
  const saveConfigPreset = useMutation({
    mutationFn: (name: string) => createConfigPreset({
      name,
      providerKey,
      config: selectedGameKey === "terraria" ? config : providerConfigPayload,
      resources: resourceLimits,
      version: selectedVersion,
      modPackId: selectedModPackId || undefined,
      modIds: selectedModIds
    }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["config-presets"] });
    }
  });
  const defaultPresetName = () => {
    const defaultPresetSource = selectedGameKey === "terraria"
      ? config.serverName
      : providerServerName(providerConfigPayload, "");
    return `${defaultPresetSource || gameDisplayName(selectedGame?.key ?? selectedGameKey, selectedGame?.name ?? selectedGameKey, t)} ${t("configurationPreset")}`;
  };
  const toggleSaveAsPreset = () => {
    saveConfigPreset.reset();
    setPresetSavedForCurrentSubmit(false);
    const next = !saveAsPreset;
    if (next && !presetName.trim()) setPresetName(defaultPresetName());
    setSaveAsPreset(next);
  };
  const submitCreate = () => {
    if (!validateCurrentConfig()) return;
    if (!saveAsPreset || presetSavedForCurrentSubmit) {
      create.mutate();
      return;
    }
    const name = presetName.trim();
    if (!name) return;
    saveConfigPreset.mutate(name, {
      onSuccess: () => {
        setPresetSavedForCurrentSubmit(true);
        create.mutate();
      }
    });
  };
  const chooseMode = (nextMode: "vanilla" | "tmodloader") => {
    const basePreset = nextMode === "tmodloader" ? tmodLoaderBasePreset : "friends-casual";
    const visiblePreset: PresetKey = nextMode === "tmodloader" ? "custom" : "friends-casual";
    setSelectedProviderKey(nextMode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla");
    setMode(nextMode);
    setSelectedPreset(visiblePreset);
    setConfig(createNamedTerrariaConfig(basePreset));
    setProviderConfigPayload({});
    setSelectedWorldId("");
    setAppliedWorldConfigId("");
    setSelectedModIds([]);
    setSelectedModPackId("");
  };
  const chooseGame = (game: GameCatalogEntry, preferredProviderKey?: ProviderKey | null) => {
    if (game.status !== "available" || game.providers.length === 0) return;
    setSelectedGameKey(game.key);
    const nextProvider = game.providers.find((provider) => provider.key === preferredProviderKey) ?? game.providers.find((provider) => provider.recommended) ?? game.providers[0];
    if (!nextProvider) return;
    setSelectedProviderKey(nextProvider.key);
    if (game.key === "terraria") {
      chooseMode(nextProvider.key === "terraria-tmodloader" ? "tmodloader" : "vanilla");
    } else {
      const localizedGameName = gameDisplayName(game.key, game.name, t);
      const names = createGameDefaultNames(localizedGameName, locale);
      setSelectedPreset("custom");
      setConfig(editableTerrariaConfig({
        ...defaultCreateServerConfig,
        serverName: names.serverName,
        worldName: names.worldName,
        maxPlayers: 8,
        password: "",
        motd: ""
      }));
      setProviderConfigPayload(createDefaultProviderConfigPayload(nextProvider, createProviderDefaultOverrides(names, game.key, locale)));
      setSelectedWorldId("");
      setAppliedWorldConfigId("");
      setSelectedModIds([]);
      setSelectedModPackId("");
    }
  };
  const chooseProvider = (provider: ProviderCatalog) => {
    setSelectedProviderKey(provider.key);
    if (provider.key === "terraria-tmodloader" || provider.key === "terraria-vanilla") {
      chooseMode(provider.key === "terraria-tmodloader" ? "tmodloader" : "vanilla");
    } else {
      const gameKey = selectedGame?.key ?? selectedGameKey;
      const localizedGameName = gameDisplayName(gameKey, selectedGame?.name ?? provider.name, t);
      const names = createGameDefaultNames(localizedGameName, locale);
      setSelectedPreset("custom");
      setProviderConfigPayload(createDefaultProviderConfigPayload(provider, createProviderDefaultOverrides(names, gameKey, locale)));
      setSelectedModIds([]);
      setSelectedModPackId("");
    }
  };
  const choosePreset = (preset: PresetKey) => {
    setSelectedPreset(preset);
    if (preset === "custom") return;
    setConfig(createNamedTerrariaConfig(preset));
  };
  const chooseModPack = (packId: string) => {
    setSelectedModPackId(packId);
    const pack = modPacks.find((item) => item.id === packId);
    setSelectedModIds(pack?.modIds ?? []);
  };
  const applyConfigPreset = (preset: ConfigPreset) => {
    const game = games.find((item) => item.key === preset.gameKey);
    const provider = game?.providers.find((item) => item.key === preset.providerKey);
    if (!game || !provider) return;
    setSelectedGameKey(game.key);
    setSelectedProviderKey(provider.key);
    setMode(provider.key === "terraria-tmodloader" ? "tmodloader" : "vanilla");
    setSelectedPreset("custom");
    const presetPayload = preset.configPayload ?? preset.config;
    if (game.key === "terraria") {
      setConfig(terrariaConfigFromPayload({ ...presetPayload, password: "" }));
    }
    setProviderConfigPayload(presetPayload);
    setResourceLimits({ cpuLimitCores: preset.cpuLimitCores ?? 0, memoryLimitMb: preset.memoryLimitMb ?? 0 });
    setVersion(preset.version ?? "");
    setSelectedWorldId("");
    setAppliedWorldConfigId("");
    setSelectedModPackId(preset.modPackId ?? "");
    setSelectedModIds(preset.modIds);
    setStep(1);
  };

  useEffect(() => {
    if (!showWorldAndBackupFeatures) return;
    if (typeof window === "undefined" || selectedWorldId) return;
    const worldId = new URLSearchParams(window.location.search).get("worldId");
    if (!worldId) return;
    setSelectedWorldId(worldId);
  }, [selectedWorldId]);

  useEffect(() => {
    if (selectedGameKey || games.length === 0 || appliedConfigPresetId || selectedWorldId) return;
    if (typeof window !== "undefined") {
      const params = new URLSearchParams(window.location.search);
      if (params.get("game") || params.get("presetId") || params.get("worldId")) return;
    }
    const defaultGame = games.find((game) => game.status === "available") ?? games[0];
    if (defaultGame) {
      chooseGame(defaultGame);
    }
  }, [appliedConfigPresetId, games, selectedGameKey, selectedWorldId]);

  useEffect(() => {
    if (typeof window === "undefined" || games.length === 0) return;
    const params = new URLSearchParams(window.location.search);
    const gameKey = params.get("game");
    const providerKey = params.get("provider") as ProviderKey | null;
    const requestedVersion = params.get("version") ?? "";
    const queryKey = `${gameKey ?? ""}:${providerKey ?? ""}:${requestedVersion}`;
    if (!gameKey || appliedGameQueryKey === queryKey || selectedWorldId || appliedConfigPresetId) return;
    const game = games.find((item) => item.key === gameKey);
    if (!game || game.status !== "available") return;
    chooseGame(game, providerKey);
    setVersion(requestedVersion);
    setStep(0);
    setAppliedGameQueryKey(queryKey);
  }, [appliedConfigPresetId, appliedGameQueryKey, games, selectedWorldId]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const presetId = new URLSearchParams(window.location.search).get("presetId");
    if (!presetId || appliedConfigPresetId === presetId || games.length === 0 || configPresets.length === 0) return;
    const preset = configPresets.find((item) => item.id === presetId);
    if (!preset) return;
    applyConfigPreset(preset);
    setAppliedConfigPresetId(presetId);
  }, [appliedConfigPresetId, configPresets, games.length]);

  useEffect(() => {
    if (!showWorldAndBackupFeatures) return;
    if (!selectedWorld || appliedWorldConfigId === selectedWorld.id) return;
    const nextMode = selectedWorld.providerKey === "terraria-tmodloader" ? "tmodloader" : "vanilla";
    const basePreset = nextMode === "tmodloader" ? tmodLoaderBasePreset : "friends-casual";
    const presetConfig = editableTerrariaConfig(getTerrariaPreset(basePreset).config);
    setMode(nextMode);
    setSelectedPreset("custom");
    if ((selectedWorld.gameKey ?? "terraria") === "terraria") {
      const worldConfig = selectedWorld.config ? terrariaConfigFromPayload(selectedWorld.config, presetConfig) : { ...presetConfig, worldName: selectedWorld.name };
      setConfig(editableTerrariaConfig(worldConfig));
    } else if (selectedWorld.config) {
      setProviderConfigPayload(selectedWorld.config);
    }
    setAppliedWorldConfigId(selectedWorld.id);
    setStep(1);
  }, [appliedWorldConfigId, selectedWorld]);
  useEffect(() => {
    if (step > stepIds.length - 1) {
      setStep(stepIds.length - 1);
    }
  }, [step, stepIds.length]);

  return (
    <Card className="overflow-hidden">
      <div className="grid min-h-[640px] lg:grid-cols-[280px_1fr]">
        <aside className="hidden border-r border-panel-line bg-[linear-gradient(180deg,#111827,#07111b)] p-6 lg:block">
          <div className="overflow-hidden rounded-lg border border-panel-line bg-slate-950 shadow-[0_0_0_1px_rgba(123,217,120,0.08)]">
            {selectedGameArt.imageSrc ? (
              <Image
                src={selectedGameArt.imageSrc}
                alt={selectedGameArt.alt}
                width={1200}
                height={1800}
                className="aspect-[2/3] w-full object-cover"
                priority
              />
            ) : (
              <div className={cn("flex aspect-[2/3] w-full items-center justify-center bg-gradient-to-br", selectedGameArt.gradient)}>
                <SelectedGameIcon aria-hidden="true" className="size-20 text-white/75" />
              </div>
            )}
          </div>
        </aside>
        <div className="p-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <h1 className="text-2xl font-semibold">{t("createWizardTitle")}</h1>
            <Link
              href="/servers"
              aria-label={t("cancelCreateServer")}
              title={t("cancelCreateServer")}
              className="flex size-10 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/40 text-slate-400 transition hover:border-panel-green hover:bg-slate-900 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50"
            >
                <X aria-hidden="true" />
                <span className="sr-only">{t("cancelCreateServer")}</span>
            </Link>
          </div>
          <div className="mt-7 grid grid-cols-5 gap-2 sm:gap-3">
            {stepIds.map((stepId, index) => {
              const labelKey = stepLabelKeys[stepId];
              return (
              <button key={labelKey} className="flex flex-col items-center gap-2 text-xs text-slate-400" onClick={() => setStep(index)}>
                <span className={cn("flex size-8 items-center justify-center rounded-full border border-panel-line", index <= step && "border-panel-green bg-panel-green text-slate-950")}>
                  {index < step ? <Check aria-hidden="true" /> : index + 1}
                </span>
                {t(labelKey)}
              </button>
              );
            })}
          </div>
          <motion.div key={step} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.18 }} className="mt-8">
            {currentStepId === "setup" && (
              <div className="space-y-7">
                <GameStep
                  games={games}
                  isLoading={gamesQuery.isLoading}
                  selectedGameKey={selectedGameKey}
                  onSelectGame={chooseGame}
                />
                {selectedGameKey ? (
                  <ModeStep
                    mode={mode}
                    providers={selectedGame?.providers ?? []}
                    selectedProviderKey={providerKey}
                    setMode={chooseMode}
                    onSelectProvider={chooseProvider}
                  />
                ) : null}
              </div>
            )}
            {currentStepId === "config" && (
              <div className="space-y-6">
                <ConfigPresetPicker presets={gameConfigPresets} onSelect={applyConfigPreset} />
                {selectedGameKey === "terraria" ? <PresetStep selectedPreset={selectedPreset} setPreset={choosePreset} compact /> : null}
                <ConfigStep
                  config={config}
                  gameKey={selectedGameKey}
                  hostPort={hostPort}
                  hostPortMode={hostPortMode}
                  provider={selectedProvider}
                  providerConfigPayload={providerConfigPayload}
                  validationErrors={configValidationErrors}
                  setConfig={setConfig}
                  setProviderConfigPayload={setProviderConfigPayload}
                  onClearValidationError={(field) => setConfigValidationErrors((current) => {
                    if (!current[field]) return current;
                    const next = { ...current };
                    delete next[field];
                    return next;
                  })}
                  onCustomize={() => setSelectedPreset("custom")}
                  setHostPort={setHostPort}
                  setHostPortMode={setHostPortMode}
                  versions={availableVersions}
                  version={selectedVersion}
                  setVersion={setVersion}
                />
              </div>
            )}
            {currentStepId === "resources" && (
              <ResourcesStep
                hostCpuCores={runtimeStatsQuery.data?.cpuCores}
                hostMemoryMb={runtimeStatsQuery.data?.memoryLimitMb}
                resourceLimits={resourceLimits}
                onChange={(limits) => {
                  setSelectedPreset("custom");
                  setResourceLimits(limits);
                }}
              />
            )}
            {currentStepId === "mods" && (
              <ModsStep
                locale={locale}
                supportsMods={Boolean(selectedProvider?.capabilities.mods)}
                worldName={showWorldAndBackupFeatures ? selectedWorld?.name : undefined}
                mods={availableMods}
                modPacks={modPacks}
                selectedModPackId={selectedModPackId}
                selectedModIds={selectedModIds}
                onSelectModPack={chooseModPack}
                onToggleMod={(modId) => {
                  setSelectedModPackId("");
                  setSelectedModIds((current) => current.includes(modId) ? current.filter((id) => id !== modId) : [...current, modId]);
                }}
              />
            )}
            {currentStepId === "review" && (
              <ReviewStep
                address={settingsQuery.data?.publicHost.trim() || undefined}
                configModel={createReviewConfigModel({
                  config,
                  gameKey: selectedGameKey,
                  gameName: selectedGame ? gameDisplayName(selectedGame.key, selectedGame.name, t) : t("gameNameTerraria"),
                  hostPortLabel: hostPortMode === "manual" ? String(hostPort) : t("automaticPort"),
                  locale,
                  provider: selectedProvider,
                  providerConfigPayload,
                  t,
                  version: selectedVersion
                })}
                gameKey={selectedGameKey}
                gameName={selectedGame ? gameDisplayName(selectedGame.key, selectedGame.name, t) : t("gameNameTerraria")}
                hostPortLabel={hostPortMode === "manual" ? String(hostPort) : t("automaticPort")}
                resourceLimits={resourceLimits}
                selectedWorldName={showWorldAndBackupFeatures ? selectedWorld?.name : undefined}
                selectedModNames={selectedModNames}
                saveAsPreset={saveAsPreset}
                presetName={presetName}
                presetSaveError={saveConfigPreset.error instanceof Error ? saveConfigPreset.error.message : ""}
                presetSavePending={saveConfigPreset.isPending}
                onChangePresetName={(name) => {
                  saveConfigPreset.reset();
                  setPresetSavedForCurrentSubmit(false);
                  setPresetName(name);
                }}
                onToggleSaveAsPreset={toggleSaveAsPreset}
              />
            )}
          </motion.div>
          <div className="mt-8 flex justify-between">
            <Button
              variant="secondary"
              disabled={step === 0}
              onClick={() => {
                setPresetSavedForCurrentSubmit(false);
                setStep((value) => Math.max(0, value - 1));
              }}
            >
              <ChevronLeft aria-hidden="true" />
              {t("back")}
            </Button>
            <Button
              onClick={() => {
                if (currentStepId === "config" && !validateCurrentConfig()) return;
                if (step === stepIds.length - 1) {
                  submitCreate();
                  return;
                }
                setStep((value) => Math.min(stepIds.length - 1, value + 1));
              }}
              disabled={create.isPending || saveConfigPreset.isPending || !canContinueCurrentStep || (step === stepIds.length - 1 && (!canCreateSelectedProvider || (saveAsPreset && presetName.trim().length === 0)))}
            >
              {step === stepIds.length - 1 ? create.isPending ? t("creating") : saveConfigPreset.isPending ? t("saving") : t("createServerLower") : t("nextStep", { step: t(nextStepKey) })}
              <ChevronRight aria-hidden="true" />
            </Button>
          </div>
          {!canContinueCurrentStep && currentStepId !== "review" && (
            <p className="mt-4 text-sm text-panel-gold">
              {t("runtimeNotInstalledForCreate")}{" "}
              <Link href="/games" className="font-medium text-panel-green hover:text-panel-green/80">
                {t("openGameLibrary")}
              </Link>
            </p>
          )}
          {!canCreateSelectedProvider && currentStepId === "review" && <p className="mt-4 text-sm text-panel-gold">{t("providerNotCreatableYet")}</p>}
          {Object.keys(configValidationErrors).length > 0 && <p className="mt-4 text-sm text-panel-gold">{t("requiredConfigSummary")}</p>}
          {create.isError && <p className="mt-4 text-sm text-red-200">{formatCreateServerError(create.error, t)}</p>}
          {create.data && <p className="mt-4 text-sm text-panel-green">{t("createdServer", { name: create.data.server.name })}</p>}
          <p className="mt-4 text-xs text-slate-500">{t("currentStep", { step: selectedTitle })}</p>
        </div>
      </div>
    </Card>
  );
}

function GameStep({
  games,
  isLoading,
  selectedGameKey,
  onSelectGame
}: {
  games: GameCatalogEntry[];
  isLoading: boolean;
  selectedGameKey: string;
  onSelectGame: (game: GameCatalogEntry) => void;
}) {
  const { t } = useI18n();
  const orderedGames = games.length > 0 ? games : [{ key: "terraria", name: "Terraria", description: "", status: "available", providers: [] }];
  return (
    <div>
      <h2 className="text-lg font-semibold">{t("chooseGame")}</h2>
      <p className="mt-1 text-sm text-slate-400">{t("chooseGameDescription")}</p>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        {orderedGames.map((game) => {
          const isSelected = game.key === selectedGameKey;
          const isAvailable = game.status === "available";
          const isUnsupported = game.status === "unsupported";
          const hasReadyProvider = game.providers.some((provider) => isRuntimeImageReady(provider.runtimeImage));
          return (
            <button
              key={game.key}
              type="button"
              disabled={!isAvailable}
              onClick={() => onSelectGame(game)}
              className={cn(
                "relative rounded-lg border p-4 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
                isSelected ? "border-panel-green bg-panel-green/10" : "border-panel-line bg-slate-950/40",
                isAvailable ? "hover:border-panel-green/70 hover:bg-slate-900/55" : "cursor-not-allowed opacity-75"
              )}
            >
              <div className="flex items-start gap-3">
                <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-md border", isSelected ? "border-panel-green bg-panel-green/15 text-panel-green" : "border-panel-line text-slate-400")}>
                  <Gamepad2 aria-hidden="true" />
                </span>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <p className="font-medium text-white">{gameDisplayName(game.key, game.name, t)}</p>
                    <span className={cn("rounded px-2 py-0.5 text-xs", isAvailable ? "bg-panel-green/15 text-panel-green" : "bg-slate-800 text-slate-400")}>
                      {isAvailable ? t("gameAvailable") : isUnsupported ? t("gameUnsupported") : t("gamePlanned")}
                    </span>
                  </div>
                  <p className="mt-1 text-sm leading-6 text-slate-400">{gameDescription(game.key, game.description || t("terrariaGameDescription"), t)}</p>
                  {game.providers.length > 0 && (
                    <p className="mt-3 text-xs text-slate-500">
                      {t("providerCount", { count: game.providers.length })}
                      {isAvailable ? ` · ${hasReadyProvider ? t("gameLibraryInstalled") : t("gameLibraryNotInstalled")}` : ""}
                    </p>
                  )}
                  {!isAvailable && <p className="mt-3 text-xs text-slate-500">{isUnsupported ? t("unsupportedGameHint") : t("plannedGameHint")}</p>}
                </div>
              </div>
              {isSelected && (
                <span className="absolute right-3 top-3 flex size-6 items-center justify-center rounded-full bg-panel-green text-slate-950">
                  <Check aria-hidden="true" className="size-4" />
                </span>
              )}
            </button>
          );
        })}
      </div>
      {isLoading && <p className="mt-3 text-sm text-slate-500">{t("loading")}</p>}
    </div>
  );
}

function ModeStep({
  mode,
  providers,
  selectedProviderKey,
  setMode,
  onSelectProvider
}: {
  mode: "vanilla" | "tmodloader";
  providers: ProviderCatalog[];
  selectedProviderKey: ProviderKey;
  setMode: (mode: "vanilla" | "tmodloader") => void;
  onSelectProvider: (provider: ProviderCatalog) => void;
}) {
  const { t } = useI18n();
  const modeProviders = orderModeProviders(providers);
  if (providers.length > 0) {
    return (
      <div>
          <h2 className="text-lg font-semibold">{t("chooseServerMode")}</h2>
          <div className="mt-4 grid gap-3 md:grid-cols-2">
            {modeProviders.map((provider) => {
              const isSelected = selectedProviderKey === provider.key;
              const isModded = provider.capabilities.mods;
              const displayName = providerDisplayName(provider.key, provider.name, t);
              const displayDescription = providerDescription(provider.key, provider.description, t);
              return (
                <button
                  key={provider.key}
                  type="button"
                  aria-pressed={isSelected}
                  onClick={() => onSelectProvider(provider)}
                  className={cn(
                    "relative rounded-lg border p-4 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
                    isSelected
                      ? "border-panel-green bg-panel-green/10 ring-1 ring-panel-green/40"
                      : "border-panel-line bg-slate-950/40 hover:border-panel-green/70 hover:bg-slate-900/55"
                  )}
                >
                  {isSelected && (
                    <span className="absolute right-3 top-3 flex size-6 items-center justify-center rounded-full bg-panel-green text-slate-950">
                      <Check aria-hidden="true" className="size-4" />
                    </span>
                  )}
                  {isModded ? <Package aria-hidden="true" className="text-panel-green" /> : <Hammer aria-hidden="true" className="text-panel-green" />}
                  <p className="mt-3 pr-8 font-medium">{displayName}</p>
                  <p className="mt-1 text-sm text-slate-400">{displayDescription}</p>
                  <div className="mt-4 flex flex-wrap items-center gap-2">
                    <RuntimeImagePill status={provider.runtimeImage} />
                    {provider.recommended && <span className="inline-flex rounded bg-panel-green/15 px-2 py-1 text-xs text-panel-green">{t("recommended")}</span>}
                  </div>
                </button>
              );
            })}
          </div>
      </div>
    );
  }
  return (
    <div>
        <h2 className="text-lg font-semibold">{t("chooseServerMode")}</h2>
        <div className="mt-4 grid gap-3 md:grid-cols-2">
          <button
            type="button"
            aria-pressed={mode === "vanilla"}
            onClick={() => setMode("vanilla")}
            className={cn(
              "relative rounded-lg border p-4 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
              mode === "vanilla"
                ? "border-panel-green bg-panel-green/10 ring-1 ring-panel-green/40"
                : "border-panel-line bg-slate-950/40 hover:border-panel-green/70 hover:bg-slate-900/55"
            )}
          >
            {mode === "vanilla" && (
              <span className="absolute right-3 top-3 flex size-6 items-center justify-center rounded-full bg-panel-green text-slate-950">
                <Check aria-hidden="true" className="size-4" />
              </span>
            )}
            <Hammer aria-hidden="true" className="text-panel-green" />
            <p className="mt-3 font-medium">{t("vanillaTerraria")}</p>
            <p className="mt-1 text-sm text-slate-400">{t("vanillaTerrariaDescription")}</p>
          </button>
          <button
            type="button"
            aria-pressed={mode === "tmodloader"}
            onClick={() => setMode("tmodloader")}
            className={cn(
              "relative rounded-lg border p-4 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
              mode === "tmodloader"
                ? "border-panel-green bg-panel-green/10 ring-1 ring-panel-green/40"
                : "border-panel-line bg-slate-950/40 hover:border-panel-green/70 hover:bg-slate-900/55"
            )}
          >
            {mode === "tmodloader" && (
              <span className="absolute right-3 top-3 flex size-6 items-center justify-center rounded-full bg-panel-green text-white">
                <Check aria-hidden="true" className="size-4" />
              </span>
            )}
            <Package aria-hidden="true" className="text-panel-green" />
            <p className="mt-3 font-medium">tModLoader</p>
            <p className="mt-1 text-sm text-slate-400">{t("tmodLoaderDescription")}</p>
          </button>
        </div>
    </div>
  );
}

function ConfigPresetPicker({ presets, onSelect }: { presets: ConfigPreset[]; onSelect: (preset: ConfigPreset) => void }) {
  const { t } = useI18n();
  const visiblePresets = presets.slice(0, 4);
  if (visiblePresets.length === 0) return null;
  return (
    <section className="rounded-lg border border-panel-line bg-slate-950/35 p-4">
      <div className="flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/50 text-panel-green">
          <Bookmark aria-hidden="true" className="size-4" />
        </span>
        <div>
          <h2 className="text-sm font-semibold text-slate-100">{t("configurationPresets")}</h2>
          <p className="mt-1 text-xs text-slate-500">{t("gameConfigurationPresetsDescription")}</p>
        </div>
      </div>
      <div className="mt-3 flex gap-2 overflow-x-auto pb-1">
        {visiblePresets.map((preset) => (
          <button
            key={preset.id}
            type="button"
            className="min-w-52 rounded-md border border-panel-line bg-slate-950/50 p-3 text-left transition hover:border-panel-green/50 hover:bg-slate-900/60 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
            onClick={() => onSelect(preset)}
          >
            <p className="truncate text-sm font-medium text-slate-100">{preset.name}</p>
            <p className="mt-1 truncate text-xs text-slate-500">{providerDisplayName(preset.providerKey, preset.providerKey, t)}</p>
          </button>
        ))}
      </div>
    </section>
  );
}

function RuntimeImagePill({ status }: { status?: RuntimeImageStatus }) {
  const { t } = useI18n();
  const tone = runtimeImageTone(status);
  return (
    <span
      className={cn(
        "inline-flex rounded px-2 py-1 text-xs",
        tone === "success" && "bg-panel-green/15 text-panel-green",
        tone === "info" && "bg-sky-500/15 text-sky-300",
        tone === "warning" && "bg-panel-gold/15 text-panel-gold",
        tone === "neutral" && "bg-slate-800 text-slate-400"
      )}
    >
      {t(runtimeImageLabelKey(status))}
    </span>
  );
}

function PresetStep({
  selectedPreset,
  setPreset,
  compact = false
}: {
  selectedPreset: PresetKey;
  setPreset: (preset: PresetKey) => void;
  compact?: boolean;
}) {
  const { t } = useI18n();
  const presetOptions = [...presets, customPreset];
  const renderTag = (tag: PresetTag) => {
    if (tag === "6" || tag === "8" || tag === "12") return t("tagPlayers", { count: tag });
    return t(tag as MessageKey);
  };
  return (
    <section className={cn(compact && "rounded-lg border border-panel-line bg-slate-950/25 p-4")}>
      <h2 className={cn("font-semibold", compact ? "text-sm text-slate-100" : "text-lg")}>{t("choosePreset")}</h2>
      <p className={cn("mt-1 text-slate-400", compact ? "text-xs" : "text-sm")}>{t("presetDescription")}</p>
      <div className={cn("mt-4 grid gap-3", compact ? "sm:grid-cols-2 xl:grid-cols-3" : "md:grid-cols-2")}>
        {presetOptions.map((preset) => {
          const presetKey = preset.key as PresetKey;
          const isSelected = selectedPreset === presetKey;
          const isCustom = presetKey === "custom";
          return (
            <button
              key={preset.key}
              type="button"
              aria-pressed={isSelected}
              onClick={() => setPreset(presetKey)}
              className={cn(
                "relative rounded-lg border text-left transition focus:outline-none focus:ring-2",
                compact ? "p-3" : "p-4",
                isSelected && !isCustom && "border-panel-green bg-panel-green/10 ring-1 ring-panel-green/40 focus:ring-panel-green/50",
                isSelected && isCustom && "border-slate-400 bg-slate-800/60 ring-1 ring-slate-500/50 focus:ring-slate-400/50",
                !isSelected && "border-panel-line bg-slate-950/40 hover:bg-slate-900/55 focus:ring-panel-green/40"
              )}
            >
              {isSelected && (
                <span className={cn("absolute right-3 top-3 flex size-6 items-center justify-center rounded-full", isCustom ? "bg-slate-300 text-slate-950" : "bg-panel-green text-slate-950")}>
                  <Check aria-hidden="true" className="size-4" />
                </span>
              )}
              <p className="pr-8 font-medium">{t(preset.labelKey)}</p>
              {!compact ? <p className="mt-1 text-sm text-slate-400">{t(preset.descriptionKey)}</p> : null}
              <div className={cn("flex flex-wrap gap-2", compact ? "mt-2" : "mt-4")}>
                {preset.tags.map((tag) => (
                  <span key={tag} className="rounded bg-slate-800 px-2 py-1 text-xs text-slate-300">
                    {renderTag(tag)}
                  </span>
                ))}
              </div>
            </button>
          );
        })}
      </div>
    </section>
  );
}

function ConfigStep({
  config,
  gameKey,
  hostPort,
  hostPortMode,
  provider,
  providerConfigPayload,
  validationErrors,
  setConfig,
  setProviderConfigPayload,
  onClearValidationError,
  onCustomize,
  setHostPort,
  setHostPortMode,
  versions,
  version,
  setVersion
}: {
  config: TerrariaConfig;
  gameKey: string;
  hostPort: number;
  hostPortMode: "auto" | "manual";
  provider?: ProviderCatalog;
  providerConfigPayload: ProviderConfigPayload;
  validationErrors: ConfigValidationErrors;
  setConfig: (config: TerrariaConfig) => void;
  setProviderConfigPayload: (payload: ProviderConfigPayload) => void;
  onClearValidationError: (field: string) => void;
  onCustomize: () => void;
  setHostPort: (port: number) => void;
  setHostPortMode: (mode: "auto" | "manual") => void;
  versions: string[];
  version: string;
  setVersion: (version: string) => void;
}) {
  const { t } = useI18n();
  const update = <K extends keyof TerrariaConfig>(key: K, value: TerrariaConfig[K]) => {
    onCustomize();
    onClearValidationError(String(key));
    setConfig({ ...config, [key]: value });
  };
  const supportsModernSeedModes = isTerrariaVersionAtLeast(version, "1.4.5");
  const supportsLegacySecretSeedPicker = provider?.key === "terraria-tmodloader" && !supportsModernSeedModes;
  if (gameKey !== "terraria") {
    const providerFields = provider?.configSchema ?? [];
    return (
      <div>
        <ConfigStepHeader />
        <div className="mt-4 grid gap-5">
          <section className="rounded-lg border border-panel-line bg-slate-950/25 p-4">
            <div className="flex items-start gap-3">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/50 text-panel-green">
                <Gamepad2 aria-hidden="true" className="size-4" />
              </span>
              <div>
                <h3 className="text-sm font-semibold text-slate-100">{t("gameConfiguration")}</h3>
                <p className="mt-1 text-xs leading-5 text-slate-500">{t("gameConfigurationHint")}</p>
              </div>
            </div>
            <ProviderConfigEditor
              fields={providerFields}
              payload={providerConfigPayload}
              providerKey={provider?.key ?? ""}
              errors={validationErrors}
              fieldLabel={(field) => providerFieldLabel(field, t)}
              fieldHelp={(field) => providerFieldHelp(field, t)}
              onChange={(field, nextValue) => {
                onCustomize();
                onClearValidationError(field.name);
                setProviderConfigPayload(updateProviderConfigPayload(providerConfigPayload, field, nextValue));
              }}
              onRestoreDefaults={(fields) => {
                onCustomize();
                setProviderConfigPayload(restoreProviderConfigDefaults(providerConfigPayload, fields));
              }}
            />
          </section>
          <section className="rounded-lg border border-panel-line bg-slate-950/25 p-4">
            <div className="flex items-start gap-3">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/50 text-panel-green">
                <Settings2 aria-hidden="true" className="size-4" />
              </span>
              <div>
                <h3 className="text-sm font-semibold text-slate-100">{t("runtimeConfiguration")}</h3>
                <p className="mt-1 text-xs leading-5 text-slate-500">{t("runtimeConfigurationHint")}</p>
              </div>
            </div>
            <div className="mt-4 grid gap-4 md:grid-cols-2">
              <WizardField label={t("gameVersion")}>
                <WizardSelect label={t("gameVersion")} value={version} onChange={(value) => setVersion(value)}>
                  {versions.map((v) => (
                    <option key={v} value={v}>{v}</option>
                  ))}
                </WizardSelect>
              </WizardField>
              <WizardField label={t("externalPort")}>
                <WizardSelect label={t("externalPort")} value={hostPortMode} onChange={(value) => setHostPortMode(value as "auto" | "manual")}>
                  <option value="auto">{t("automaticPort")}</option>
                  <option value="manual">{t("manualPort")}</option>
                </WizardSelect>
              </WizardField>
              {hostPortMode === "manual" && (
                <WizardField label={t("externalPortValue")}>
                  <Input aria-label={t("externalPortValue")} type="number" min={1024} max={65535} value={hostPort} onChange={(event) => setHostPort(Number(event.target.value))} />
                </WizardField>
              )}
            </div>
          </section>
        </div>
      </div>
    );
  }
  return (
    <div>
      <ConfigStepHeader />
      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <WizardField label={t("serverName")} required error={validationErrors.serverName}>
          <Input
            aria-label={t("serverName")}
            value={config.serverName ?? ""}
            aria-invalid={Boolean(validationErrors.serverName)}
            className={validationErrors.serverName ? "border-red-400/70 focus:border-red-300" : undefined}
            onChange={(event) => update("serverName", event.target.value)}
          />
        </WizardField>
        <WizardField label={t("worldName")} required error={validationErrors.worldName}>
          <Input
            aria-label={t("worldName")}
            value={config.worldName}
            aria-invalid={Boolean(validationErrors.worldName)}
            className={validationErrors.worldName ? "border-red-400/70 focus:border-red-300" : undefined}
            onChange={(event) => update("worldName", event.target.value)}
          />
        </WizardField>
        <WizardField label={t("worldSize")}>
          <WizardSelect label={t("worldSize")} value={config.worldSize} onChange={(value) => update("worldSize", value as TerrariaConfig["worldSize"])}>
            <option value="small">{t("tagSmallWorld")}</option>
            <option value="medium">{t("tagMediumWorld")}</option>
            <option value="large">{t("tagLargeWorld")}</option>
          </WizardSelect>
        </WizardField>
        <WizardField label={t("worldEvil")}>
          <WizardSelect label={t("worldEvil")} value={config.worldEvil} onChange={(value) => update("worldEvil", value as TerrariaConfig["worldEvil"])}>
            <option value="random">{t("tagRandom")}</option>
            <option value="corruption">{t("tagCorruption")}</option>
            <option value="crimson">{t("tagCrimson")}</option>
          </WizardSelect>
        </WizardField>
        <WizardField label={t("difficulty")}>
          <WizardSelect label={t("difficulty")} value={config.difficulty} onChange={(value) => update("difficulty", value as TerrariaConfig["difficulty"])}>
            <option value="journey">{t("tagJourney")}</option>
            <option value="classic">{t("tagClassic")}</option>
            <option value="expert">{t("tagExpert")}</option>
            <option value="master">{t("tagMaster")}</option>
          </WizardSelect>
        </WizardField>
        <WizardField label={t("gameVersion")}>
          <WizardSelect label={t("gameVersion")} value={version} onChange={(value) => setVersion(value)}>
            {versions.map((v) => (
              <option key={v} value={v}>{v}</option>
            ))}
          </WizardSelect>
        </WizardField>
        <WizardField label={t("externalPort")}>
          <WizardSelect label={t("externalPort")} value={hostPortMode} onChange={(value) => setHostPortMode(value as "auto" | "manual")}>
            <option value="auto">{t("automaticPort")}</option>
            <option value="manual">{t("manualPort")}</option>
          </WizardSelect>
        </WizardField>
        {hostPortMode === "manual" && (
          <WizardField label={t("externalPortValue")}>
            <Input aria-label={t("externalPortValue")} type="number" min={1024} max={65535} value={hostPort} onChange={(event) => setHostPort(Number(event.target.value))} />
          </WizardField>
        )}
        <WizardField label={t("maxPlayersInput")} required error={validationErrors.maxPlayers}>
          <Input
            aria-label={t("maxPlayersInput")}
            type="number"
            min={1}
            max={255}
            value={config.maxPlayers}
            aria-invalid={Boolean(validationErrors.maxPlayers)}
            className={validationErrors.maxPlayers ? "border-red-400/70 focus:border-red-300" : undefined}
            onChange={(event) => update("maxPlayers", Number(event.target.value))}
          />
        </WizardField>
        <WizardField label={t("password")}>
          <Input aria-label={t("password")} type="password" value={config.password ?? ""} onChange={(event) => update("password", event.target.value)} />
        </WizardField>
        <WizardField label={t("motd")}>
          <Input aria-label={t("motd")} value={config.motd ?? ""} onChange={(event) => update("motd", event.target.value)} />
        </WizardField>
        <div className="md:col-span-2">
          <WizardField label={t("worldSeed")} help={t("worldSeedHint")}>
            <SeedInput
              config={config}
              supportsLegacySecretSeedPicker={supportsLegacySecretSeedPicker}
              supportsModernSeedModes={supportsModernSeedModes}
              value={config.seed ?? ""}
              placeholder={t("worldSeedPlaceholder")}
              onChange={(value) => update("seed", value)}
              onChangeSeedModes={(specialSeeds, secretSeeds) => {
                onCustomize();
                setConfig({ ...config, specialSeeds, secretSeeds });
              }}
              onSelectLegacySeed={(seed) => {
                onCustomize();
                setConfig({ ...config, seed, specialSeeds: [], secretSeeds: [] });
              }}
            />
          </WizardField>
        </div>
        <div className="overflow-hidden rounded-md border border-panel-line bg-slate-950/35 md:col-span-2 sm:grid sm:grid-cols-2 sm:divide-x sm:divide-panel-line">
          <WizardSwitch label={t("secureMode")} checked={config.secure} onChange={(checked) => update("secure", checked)} />
          <WizardSwitch label={t("autoCreateWorld")} checked={config.autoCreateWorld} onChange={(checked) => update("autoCreateWorld", checked)} />
        </div>
      </div>
    </div>
  );
}

function SeedInput({
  config,
  onChange,
  onChangeSeedModes,
  onSelectLegacySeed,
  placeholder,
  supportsLegacySecretSeedPicker,
  supportsModernSeedModes,
  value
}: {
  config: TerrariaConfig;
  onChange: (value: string) => void;
  onChangeSeedModes: (specialSeeds: string[], secretSeeds: string[]) => void;
  onSelectLegacySeed: (seed: string) => void;
  placeholder: string;
  supportsLegacySecretSeedPicker: boolean;
  supportsModernSeedModes: boolean;
  value: string;
}) {
  const { locale, t } = useI18n();
  const [open, setOpen] = useState(false);
  const showsSeedPicker = supportsModernSeedModes || supportsLegacySecretSeedPicker;
  const selectedSpecialSeeds = config.specialSeeds ?? [];
  const selectedSecretSeeds = config.secretSeeds ?? [];
  const selectedModeCount = selectedSpecialSeeds.length + selectedSecretSeeds.length;
  const legacySpecialSeed = terrariaLegacySpecialWorldSeeds.find((seed) => seed.key === secretSeedKeyFor(value));
  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open]);
  const toggleSeed = (type: "special" | "secret", key: string) => {
    const current = type === "special" ? selectedSpecialSeeds : selectedSecretSeeds;
    const next = current.includes(key) ? current.filter((item) => item !== key) : [...current, key];
    onChangeSeedModes(
      type === "special" ? next : selectedSpecialSeeds,
      type === "secret" ? next : selectedSecretSeeds
    );
  };
  const clearModes = () => onChangeSeedModes([], []);
  const clearLegacySeed = () => onChange("");
  const selectLegacySeed = (key: string) => {
    if (legacySpecialSeed?.key === key) {
      clearLegacySeed();
      return;
    }
    onSelectLegacySeed(key);
  };
  const pickerLabel = supportsModernSeedModes
    ? selectedModeCount > 0 ? t("seedModesSelected", { count: selectedModeCount }) : t("seedModes")
    : legacySpecialSeed ? terrariaSeedDisplayName(legacySpecialSeed, locale) : t("secretSeed");
  const selectedSeedItems = supportsModernSeedModes
    ? [
        ...selectedSpecialSeeds.map((key) => ({ key, type: "special" as const, seed: terrariaSpecialWorldSeeds.find((item) => item.key === key) })),
        ...selectedSecretSeeds.map((key) => ({ key, type: "secret" as const, seed: terrariaSecretWorldSeeds145.find((item) => item.key === key) }))
      ]
    : legacySpecialSeed ? [{ key: legacySpecialSeed.key, type: "legacy" as const, seed: legacySpecialSeed }] : [];
  return (
    <div className="relative space-y-1.5">
      <Input className="w-full" value={value} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} />
      {showsSeedPicker ? (
        <div className="flex min-h-10 flex-wrap items-center gap-1.5 rounded-md border border-panel-line bg-slate-950/60 px-2 py-1.5 focus-within:border-panel-green">
          {selectedSeedItems.map(({ key, seed, type }) => {
            const name = seed ? terrariaSeedDisplayName(seed, locale) : key;
            return (
              <button
                key={`${type}:${key}`}
                type="button"
                aria-label={`${t("delete")} ${name}`}
                className="inline-flex h-7 max-w-full items-center gap-1 rounded border border-panel-green/35 bg-panel-green/12 px-2 text-xs font-medium text-panel-green transition hover:border-panel-green/60 hover:bg-panel-green/18 focus:outline-none focus:ring-2 focus:ring-panel-green/40"
                onClick={() => type === "legacy" ? clearLegacySeed() : toggleSeed(type, key)}
              >
                <span className="max-w-52 truncate">{name}</span>
                <X aria-hidden="true" className="size-3 shrink-0" />
              </button>
            );
          })}
          <button
            type="button"
            aria-expanded={open}
            className={cn(
              "inline-flex h-7 min-w-0 items-center gap-1 rounded px-2 text-xs font-medium transition focus:outline-none focus:ring-2 focus:ring-panel-green/40",
              selectedSeedItems.length > 0
                ? "ml-auto text-slate-400 hover:bg-slate-900 hover:text-slate-200"
                : "flex-1 justify-between text-slate-500 hover:bg-slate-900 hover:text-slate-200"
            )}
            onClick={() => setOpen(true)}
          >
            {selectedSeedItems.length > 0 ? t("edit") : pickerLabel}
            <ChevronDown aria-hidden="true" className="size-3.5" />
          </button>
        </div>
      ) : null}
      {supportsLegacySecretSeedPicker && legacySpecialSeed ? (
        <p className="text-xs leading-5 text-panel-green">
          {t("secretSeedDetected", { name: terrariaSeedDisplayName(legacySpecialSeed, locale) })}
          <span className="text-slate-500"> · {legacySpecialSeed.description}</span>
        </p>
      ) : null}
      {open ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/65 px-4 py-8 backdrop-blur-sm"
          role="presentation"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) {
              setOpen(false);
            }
          }}
        >
          <div
            aria-modal="true"
            className="w-full max-w-5xl rounded-lg border border-panel-line bg-panel-card shadow-[0_18px_56px_rgba(0,0,0,0.45)]"
            role="dialog"
          >
            <div className="flex items-start justify-between gap-4 border-b border-panel-line p-4">
              <div className="min-w-0">
                <h3 className="text-base font-semibold text-white">{supportsModernSeedModes ? t("seedModes") : t("secretSeed")}</h3>
                <p className="mt-1 text-sm leading-5 text-slate-500">{supportsModernSeedModes ? t("seedModesHint145") : t("legacySecretSeedHint")}</p>
              </div>
              <div className="flex shrink-0 items-center gap-2">
                {supportsModernSeedModes && selectedModeCount > 0 ? (
                  <button
                    type="button"
                    className="h-8 rounded-md border border-panel-line px-3 text-xs font-medium text-slate-300 transition hover:border-slate-600 hover:bg-slate-900 hover:text-white"
                    onClick={clearModes}
                  >
                    {t("clearSeedModes")}
                  </button>
                ) : null}
                {supportsLegacySecretSeedPicker && legacySpecialSeed ? (
                  <button
                    type="button"
                    className="h-8 rounded-md border border-panel-line px-3 text-xs font-medium text-slate-300 transition hover:border-slate-600 hover:bg-slate-900 hover:text-white"
                    onClick={clearLegacySeed}
                  >
                    {t("noSecretSeed")}
                  </button>
                ) : null}
                <button
                  type="button"
                  aria-label={t("cancel")}
                  className="flex size-8 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/40"
                  onClick={() => setOpen(false)}
                >
                  <X aria-hidden="true" className="size-4" />
                </button>
              </div>
            </div>
            <div className="max-h-[min(72vh,620px)] overflow-y-auto p-4">
              {supportsModernSeedModes ? (
                <>
                  <SeedModeSection
                    description={t("specialSeedModesDescription")}
                    selected={selectedSpecialSeeds}
                    seeds={terrariaSpecialWorldSeeds}
                    title={t("specialWorldSeeds")}
                    onToggle={(key) => toggleSeed("special", key)}
                  />
                  <SeedModeSection
                    className="mt-5"
                    description={t("secretSeedModesDescription145")}
                    selected={selectedSecretSeeds}
                    seeds={terrariaSecretWorldSeeds145}
                    title={t("secretWorldSeeds145")}
                    onToggle={(key) => toggleSeed("secret", key)}
                  />
                </>
              ) : (
                <SeedModeSection
                  description={t("legacySpecialSeedModesDescription")}
                  selected={legacySpecialSeed ? [legacySpecialSeed.key] : []}
                  seeds={terrariaLegacySpecialWorldSeeds}
                  title={t("specialWorldSeeds")}
                  onToggle={selectLegacySeed}
                />
              )}
            </div>
            <div className="flex items-center justify-end border-t border-panel-line p-4">
              <Button type="button" onClick={() => setOpen(false)}>
                {t("done")}
              </Button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function SeedModeSection({
  className,
  description,
  onToggle,
  seeds,
  selected,
  title
}: {
  className?: string;
  description: string;
  onToggle: (key: string) => void;
  seeds: readonly { key: string; label: string; description: string }[];
  selected: string[];
  title: string;
}) {
  const { locale } = useI18n();
  return (
    <section className={className}>
      <div className="flex flex-col gap-1 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h4 className="text-sm font-semibold text-slate-100">{title}</h4>
          <p className="mt-1 text-xs leading-5 text-slate-500">{description}</p>
        </div>
        <span className="text-xs text-slate-500">{selected.length}/{seeds.length}</span>
      </div>
      <div className="mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
        {seeds.map((seed) => {
          const active = selected.includes(seed.key);
          const displayName = terrariaSeedDisplayName(seed, locale);
          const seedReference = displayName === seed.label ? seed.key : `${seed.label} · ${seed.key}`;
          return (
            <button
              key={seed.key}
              type="button"
              aria-pressed={active}
              className={cn(
                "group flex min-h-20 items-start justify-between gap-3 rounded-md border px-3 py-2 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/40",
                active
                  ? "border-panel-green/55 bg-panel-green/12 text-white"
                  : "border-panel-line bg-slate-950/45 text-slate-300 hover:border-slate-600 hover:bg-slate-900"
              )}
              onClick={() => onToggle(seed.key)}
            >
              <span className="min-w-0">
                <span className="block truncate text-sm font-semibold text-slate-100">{displayName}</span>
                <span className="mt-0.5 block truncate text-xs text-slate-500">{seedReference}</span>
                <span className="mt-1 line-clamp-2 block text-xs leading-5 text-slate-500 group-hover:text-slate-400">{seed.description}</span>
              </span>
              <span
                className={cn(
                  "mt-0.5 flex size-5 shrink-0 items-center justify-center rounded border transition",
                  active ? "border-panel-green bg-panel-green text-slate-950" : "border-slate-700 bg-slate-950"
                )}
              >
                {active ? <Check aria-hidden="true" className="size-3.5" /> : null}
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
}

function ResourcesStep({
  hostCpuCores,
  hostMemoryMb,
  onChange,
  resourceLimits
}: {
  hostCpuCores?: number;
  hostMemoryMb?: number;
  onChange: (limits: ResourceLimits) => void;
  resourceLimits: ResourceLimits;
}) {
  return (
    <div>
      <RuntimeResourceSection hostCpuCores={hostCpuCores} hostMemoryMb={hostMemoryMb} resourceLimits={resourceLimits} onChange={onChange} />
    </div>
  );
}

function ConfigStepHeader() {
  const { t } = useI18n();
  return (
    <div>
      <h2 className="text-lg font-semibold">{t("serverConfig")}</h2>
      <p className="mt-1 text-sm text-slate-500">{t("serverConfigDescription")}</p>
    </div>
  );
}

function RuntimeResourceSection({
  hostCpuCores,
  hostMemoryMb,
  onChange,
  resourceLimits
}: {
  hostCpuCores?: number;
  hostMemoryMb?: number;
  onChange: (limits: ResourceLimits) => void;
  resourceLimits: ResourceLimits;
}) {
  const { t } = useI18n();
  const cpuSliderMax = Math.max(1, hostCpuCores ?? 8, Math.ceil(resourceLimits.cpuLimitCores));
  const memorySliderMax = Math.max(1024, Math.floor((hostMemoryMb ?? 16384) / 1024) * 1024, Math.ceil(resourceLimits.memoryLimitMb / 1024) * 1024);
  return (
    <section className="rounded-lg border border-panel-line bg-slate-950/35 p-4 md:col-span-2">
      <div>
        <h3 className="text-sm font-semibold text-slate-100">{t("runtimeResources")}</h3>
        <p className="mt-1 max-w-2xl text-xs leading-5 text-slate-500">{t("resourceLimitsHint")}</p>
      </div>
      <div className="mt-4 grid gap-3">
        <div className="rounded-md border border-panel-line bg-slate-950/45 p-4">
          <ResourceLimitSlider
            formatValue={(value) => formatCpuResourceLimit(value, t("unlimited"), t("cpuUnit"))}
            label={t("cpuLimit")}
            max={cpuSliderMax}
            step={0.25}
            value={resourceLimits.cpuLimitCores}
            onChange={(value) => onChange({ ...resourceLimits, cpuLimitCores: value })}
          />
        </div>
        <div className="rounded-md border border-panel-line bg-slate-950/45 p-4">
          <ResourceLimitSlider
            formatValue={(value) => formatMemoryResourceLimit(value, t("unlimited"))}
            label={t("memoryLimit")}
            max={memorySliderMax}
            step={1024}
            value={resourceLimits.memoryLimitMb}
            onChange={(value) => onChange({ ...resourceLimits, memoryLimitMb: value })}
          />
        </div>
      </div>
    </section>
  );
}

function WizardField({
  children,
  error,
  help,
  label,
  required
}: {
  children: React.ReactNode;
  error?: string;
  help?: string;
  label: string;
  required?: boolean;
}) {
  const { t } = useI18n();
  return (
    <div className="grid w-full min-w-0 self-start content-start gap-1.5">
      <span className="flex min-w-0 items-center gap-2 text-xs font-medium text-slate-500">
        <span className="truncate">{label}</span>
        {help ? <FieldHelp text={help} /> : null}
        {required && (
          <span className="shrink-0 rounded border border-panel-gold/30 bg-panel-gold/10 px-1.5 py-0.5 text-[10px] font-semibold text-panel-gold">
            {t("requiredField")}
          </span>
        )}
      </span>
      {children}
      {error && <span className="text-xs font-medium text-red-200">{error}</span>}
    </div>
  );
}

function FieldHelp({ text }: { text: string }) {
  return (
    <span className="group/help relative inline-flex shrink-0">
      <button
        aria-label={text}
        className="flex size-4 cursor-help select-none items-center justify-center rounded-full border border-slate-600 bg-slate-950/70 text-[10px] font-bold leading-none text-slate-300 transition hover:border-panel-green/70 hover:text-panel-green focus:border-panel-green focus:text-panel-green focus:outline-none focus:ring-2 focus:ring-panel-green/30"
        type="button"
      >
        ?
      </button>
      <span className="pointer-events-none absolute left-1/2 top-6 z-20 hidden w-64 -translate-x-1/2 rounded-md border border-panel-line bg-slate-950 px-3 py-2 text-xs font-normal leading-5 text-slate-300 shadow-[0_10px_30px_rgba(0,0,0,0.35)] group-hover/help:block group-focus-within/help:block">
        {text}
      </span>
    </span>
  );
}

function WizardSelect({
  children,
  invalid,
  label,
  onChange,
  value
}: {
  children: React.ReactNode;
  invalid?: boolean;
  label: string;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <select
      aria-label={label}
      aria-invalid={invalid}
      className={cn(
        "h-10 w-full rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-panel-green",
        invalid && "border-red-400/70 focus:border-red-300"
      )}
      value={value}
      onChange={(event) => onChange(event.target.value)}
    >
      {children}
    </select>
  );
}

function WizardSwitch({ checked, label, onChange }: { checked: boolean; label: string; onChange: (checked: boolean) => void }) {
  return (
    <label
      className="group flex min-h-12 cursor-pointer items-center justify-between gap-4 px-3.5 py-2.5 text-sm text-slate-300 transition hover:bg-slate-900/60 hover:text-slate-100 focus-within:bg-slate-900/60 focus-within:outline-none focus-within:ring-2 focus-within:ring-inset focus-within:ring-panel-green/40 max-sm:border-b max-sm:border-panel-line last:max-sm:border-b-0"
    >
      <span className="min-w-0 font-medium leading-5">{label}</span>
      <input
        className="sr-only"
        checked={checked}
        role="switch"
        type="checkbox"
        onChange={(event) => onChange(event.target.checked)}
      />
      <span
        aria-hidden="true"
        className={cn(
          "relative h-5 w-9 shrink-0 rounded-full border transition-colors duration-200",
          checked
            ? "border-panel-green bg-panel-green"
            : "border-slate-600 bg-slate-700 group-hover:border-slate-500"
        )}
      >
        <span
          className={cn(
            "absolute left-0.5 top-0.5 size-3.5 rounded-full bg-white transition-transform duration-200",
            checked && "translate-x-4"
          )}
        />
      </span>
    </label>
  );
}

function ModsStep({
  locale,
  supportsMods,
  worldName,
  mods,
  modPacks,
  selectedModPackId,
  selectedModIds,
  onSelectModPack,
  onToggleMod
}: {
  locale: string;
  supportsMods: boolean;
  worldName?: string;
  mods: ModFile[];
  modPacks: ModPack[];
  selectedModPackId: string;
  selectedModIds: string[];
  onSelectModPack: (packId: string) => void;
  onToggleMod: (modId: string) => void;
}) {
  const { t } = useI18n();
  const [search, setSearch] = useState("");
  const searchTerm = search.trim().toLocaleLowerCase(locale);
  const visibleMods = searchTerm
    ? mods.filter((mod) => [modDisplayName(mod, locale), mod.fileName, mod.workshopId, ...(mod.tags ?? [])]
      .some((value) => value?.toLocaleLowerCase(locale).includes(searchTerm)))
    : mods;
  return (
    <div>
      <div className="rounded-lg border border-panel-line bg-slate-950/40 p-4">
        <div className="flex items-center gap-3">
          {worldName ? (
            <>
              <FileArchive aria-hidden="true" className="size-5 shrink-0 text-panel-green" />
              <div className="min-w-0">
                <p className="text-sm text-slate-500">{t("world")}</p>
                <p className="truncate font-medium text-slate-200">{worldName}</p>
              </div>
            </>
          ) : (
            <>
              <Globe aria-hidden="true" className="size-5 shrink-0 text-panel-green" />
              <div>
                <p className="font-medium">{t("autoCreateWorld")}</p>
                <p className="mt-0.5 text-sm text-slate-400">{t("autoCreateWorldHint")}</p>
              </div>
            </>
          )}
        </div>
      </div>

      {!supportsMods ? (
        <div className="mt-6 flex flex-col items-center justify-center rounded-lg border border-dashed border-panel-line bg-slate-950/30 py-12 text-center">
          <Package aria-hidden="true" className="size-8 text-slate-600" />
          <p className="mt-3 text-sm text-slate-400">{t("noModsForProvider")}</p>
        </div>
      ) : (
        <div className="mt-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <h2 className="text-lg font-semibold">{t("selectMods")}</h2>
              <p className="mt-1 text-sm text-slate-400">{t("selectModsHint")}</p>
            </div>
            <div className="flex min-h-8 items-center gap-3">
              <span className={cn("text-sm", selectedModIds.length > 0 ? "text-slate-200" : "text-slate-500")}>
                {t("selectedModsCount", { count: selectedModIds.length })}
              </span>
              {selectedModIds.length > 0 ? (
                <button
                  type="button"
                  className="text-sm font-medium text-slate-400 transition hover:text-white focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/40"
                  onClick={() => onSelectModPack("")}
                >
                  {t("clearSelection")}
                </button>
              ) : null}
            </div>
          </div>
          {modPacks.length > 0 && (
            <div className="mt-5">
              <div>
                <p className="text-sm font-semibold text-slate-200">{t("modPacks")}</p>
                <p className="mt-0.5 text-xs text-slate-500">{t("chooseModPackHint")}</p>
              </div>
              <div className="mt-3 grid gap-2 md:grid-cols-2" role="radiogroup" aria-label={t("modPacks")}>
                {modPacks.map((pack) => {
                  const active = selectedModPackId === pack.id;
                  const previewNames = pack.mods.slice(0, 3).map((mod) => modDisplayName(mod, locale));
                  const preview = pack.description.trim() || [previewNames.join(" · "), pack.modIds.length > 3 ? `+${pack.modIds.length - 3}` : ""].filter(Boolean).join("  ");
                  return (
                    <button
                      key={pack.id}
                      type="button"
                      role="radio"
                      aria-checked={active}
                      onClick={() => {
                        if (!active) onSelectModPack(pack.id);
                      }}
                      className={cn(
                        "flex min-h-16 w-full items-center gap-3 rounded-md border px-3 py-2.5 text-left transition focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/50",
                        active
                          ? "border-panel-green/60 bg-panel-green/10"
                          : "border-panel-line bg-slate-950/35 hover:border-slate-600 hover:bg-slate-900/55"
                      )}
                    >
                      <span
                        aria-hidden="true"
                        className={cn(
                          "flex size-5 shrink-0 items-center justify-center rounded-full border",
                          active ? "border-panel-green bg-panel-green text-slate-950" : "border-slate-600 bg-slate-950"
                        )}
                      >
                        {active ? <Check className="size-3" /> : null}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="flex items-center gap-2">
                          <span className="truncate text-sm font-medium text-slate-100">{pack.name}</span>
                          {active ? <span className="shrink-0 text-xs font-medium text-panel-green">{t("selected")}</span> : null}
                        </span>
                        <span className="mt-0.5 block truncate text-xs text-slate-500">{preview || t("modPacksHint")}</span>
                      </span>
                      <span className="shrink-0 text-xs tabular-nums text-slate-500">{pack.modIds.length}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          )}
          <div className="mt-5 border-t border-panel-line pt-5">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <p className="text-sm font-semibold text-slate-200">{t("modLibrary")}</p>
                <p className="mt-0.5 text-xs text-slate-500">{t("customizeModSelectionHint")}</p>
              </div>
              {mods.length > 0 ? (
                <div className="relative w-full sm:w-72">
                  <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-slate-500" />
                  <Input
                    value={search}
                    placeholder={t("searchMods")}
                    className="pl-9"
                    onChange={(event) => setSearch(event.target.value)}
                  />
                </div>
              ) : null}
            </div>
            <div className="mt-3">
              {mods.length === 0 ? (
                <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-panel-line bg-slate-950/30 py-8 text-center">
                  <Package aria-hidden="true" className="size-8 text-slate-600" />
                  <p className="mt-3 text-sm text-slate-400">{t("noModsInLibrary")}</p>
                  <Link href="/mods" className="mt-3 inline-flex items-center gap-2 text-sm text-panel-green hover:underline">
                    <Package aria-hidden="true" className="size-4" />
                    {t("goToModsPage")}
                  </Link>
                </div>
              ) : visibleMods.length === 0 ? (
                <div className="rounded-md border border-dashed border-panel-line px-4 py-8 text-center text-sm text-slate-500">
                  {t("noMatchingMods")}
                </div>
              ) : (
                <>
                  <div className="grid max-h-[26rem] gap-2 overflow-y-auto pr-1 md:grid-cols-2">
                    {visibleMods.map((mod) => {
                      const active = selectedModIds.includes(mod.id);
                      return (
                        <button
                          key={mod.id}
                          type="button"
                          aria-pressed={active}
                          onClick={() => onToggleMod(mod.id)}
                          className={cn(
                            "flex min-h-14 w-full items-center gap-3 rounded-md border px-3 py-2 text-left transition focus:outline-none focus-visible:ring-2 focus-visible:ring-panel-green/50",
                            active
                              ? "border-panel-green/55 bg-panel-green/10"
                              : "border-panel-line bg-slate-950/35 hover:border-slate-600 hover:bg-slate-900/55"
                          )}
                        >
                          <span className={cn("flex size-5 shrink-0 items-center justify-center rounded border", active ? "border-panel-green bg-panel-green text-slate-950" : "border-slate-600 bg-slate-950")}>
                            {active ? <Check aria-hidden="true" className="size-3" /> : null}
                          </span>
                          <span className="min-w-0 flex-1">
                            <span className="block truncate text-sm font-medium text-slate-100">{modDisplayName(mod, locale)}</span>
                            <span className="mt-0.5 block truncate text-xs text-slate-500">{[mod.modVersion, mod.size].filter(Boolean).join(" · ")}</span>
                          </span>
                        </button>
                      );
                    })}
                  </div>
                  <Link href="/mods" className="mt-3 inline-flex items-center gap-2 text-sm text-panel-green hover:underline">
                    <Package aria-hidden="true" className="size-4" />
                    {t("goToModsPage")}
                  </Link>
                </>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function ReviewStep({
  address,
  configModel,
  gameKey,
  gameName,
  hostPortLabel,
  resourceLimits,
  selectedWorldName,
  selectedModNames,
  saveAsPreset,
  presetName,
  presetSaveError,
  presetSavePending,
  onChangePresetName,
  onToggleSaveAsPreset
}: {
  address?: string;
  configModel: ReviewConfigModel;
  gameKey: string;
  gameName: string;
  hostPortLabel: string;
  resourceLimits: ResourceLimits;
  selectedWorldName?: string;
  selectedModNames: string[];
  saveAsPreset: boolean;
  presetName: string;
  presetSaveError: string;
  presetSavePending: boolean;
  onChangePresetName: (name: string) => void;
  onToggleSaveAsPreset: () => void;
}) {
  const { t } = useI18n();
  const invitePreview = createReviewInvitePreview({
    address,
    gameKey,
    gameName,
    hostPortLabel,
    password: configModel.password,
    serverName: configModel.serverName || gameName
  });
  const joinInstruction = t(reviewJoinInstructionKey(gameKey));
  return (
    <div>
      <h2 className="text-lg font-semibold">{t("review")}</h2>
      <p className="mt-1 text-sm text-slate-500">{t("reviewHint")}</p>
      <Card className="mt-4 p-4">
        <div className="rounded-md border border-panel-line bg-slate-950/60 p-3 text-sm">
          <div className="flex items-center gap-2 font-medium text-slate-100">
            <Gamepad2 aria-hidden="true" className="size-4 text-panel-green" />
            {t("gameConfiguration")}
          </div>
          <div className="mt-3 grid gap-2 md:grid-cols-2">
            {configModel.fields.map((field) => (
              <ReviewConfigItem key={`${field.label}:${field.value}`} label={field.label} value={field.value} />
            ))}
          </div>
        </div>
        <div className="mt-4 rounded-md border border-panel-line bg-slate-950/60 p-3 text-sm">
          <div className="flex items-center gap-2 font-medium text-slate-100">
            <Settings2 aria-hidden="true" className="size-4 text-panel-green" />
            {t("runtimeResources")}
          </div>
          <div className="mt-3 grid gap-2 md:grid-cols-2">
            <ReviewConfigItem label={t("cpuLimit")} value={formatCpuLimitLabel(resourceLimits.cpuLimitCores, t)} />
            <ReviewConfigItem label={t("memoryLimit")} value={formatMemoryLimitLabel(resourceLimits.memoryLimitMb, t)} />
          </div>
        </div>
        <div className="mt-4 rounded-md border border-panel-line bg-slate-950/60 p-3 text-sm">
          <div className="flex items-center gap-2 font-medium text-slate-100">
            <Globe aria-hidden="true" className="size-4 text-panel-green" />
            {t("reviewJoinTitle")}
          </div>
          <p className="mt-2 text-slate-400">{joinInstruction}</p>
          <p className="mt-2 text-xs text-slate-500">{t("reviewJoinHint")}</p>
          <p className="mt-2 overflow-hidden text-ellipsis rounded-md border border-panel-line bg-slate-950 px-3 py-2 font-mono text-xs text-panel-green">{invitePreview}</p>
        </div>
        {selectedWorldName && (
          <div className="mt-4 rounded-md border border-panel-line bg-slate-950/60 p-3 text-sm text-slate-300">
            <p>{t("selectedWorldFile")}: <span className="text-panel-green">{selectedWorldName}</span></p>
          </div>
        )}
        {selectedModNames.length > 0 && (
          <div className="mt-2 rounded-md border border-panel-line bg-slate-950/60 p-3 text-sm text-slate-300">
            <p>{t("selectedModFiles")}: <span className="text-panel-green">{selectedModNames.join(", ")}</span></p>
          </div>
        )}
      </Card>
      <div className="mt-4 overflow-hidden rounded-lg border border-panel-line bg-slate-950/35">
        <button
          type="button"
          role="switch"
          aria-checked={saveAsPreset}
          className="flex w-full items-center gap-3 p-4 text-left transition hover:bg-slate-900/55 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-panel-green/50"
          disabled={presetSavePending}
          onClick={onToggleSaveAsPreset}
        >
          <span className={cn(
            "flex size-9 shrink-0 items-center justify-center rounded-md border",
            saveAsPreset ? "border-panel-green/50 bg-panel-green/10 text-panel-green" : "border-panel-line bg-slate-950/60 text-slate-500"
          )}>
            <Bookmark aria-hidden="true" className="size-4" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block text-sm font-medium text-slate-100">{t("savePresetWithServer")}</span>
            <span className="mt-0.5 block text-xs text-slate-500">{t("savePresetWithServerHint")}</span>
          </span>
          <span
            aria-hidden="true"
            className={cn(
              "flex h-6 w-11 shrink-0 items-center rounded-full p-0.5 transition",
              saveAsPreset ? "justify-end bg-panel-green" : "justify-start bg-slate-700"
            )}
          >
            <span className="size-5 rounded-full bg-white shadow-sm" />
          </span>
        </button>
        {saveAsPreset && (
          <motion.div
            initial={{ opacity: 0, y: -4 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.15 }}
            className="border-t border-panel-line p-4"
          >
            <WizardField label={t("configurationPresetName")}>
              <Input
                aria-label={t("configurationPresetName")}
                value={presetName}
                disabled={presetSavePending}
                placeholder={t("configurationPresetNamePlaceholder")}
                onChange={(event) => onChangePresetName(event.target.value)}
              />
            </WizardField>
            <p className="mt-2 text-xs leading-5 text-slate-500">{t("configurationPresetSaveHint")}</p>
            {presetSaveError && <p className="mt-2 text-xs text-panel-gold">{presetSaveError}</p>}
          </motion.div>
        )}
      </div>
    </div>
  );
}

function ReviewConfigItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/55 px-3 py-2">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 break-words text-sm font-medium leading-5 text-slate-200">{value}</p>
    </div>
  );
}
