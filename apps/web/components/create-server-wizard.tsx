"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bookmark, Check, ChevronDown, ChevronLeft, ChevronRight, FileArchive, Gamepad2, Globe, Hammer, Package, Settings2, X } from "lucide-react";
import Image from "next/image";
import { useEffect, useMemo, useState } from "react";
import { Button, Card, Input } from "@/components/ui";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { modDisplayName } from "@/lib/mod-display";
import { showWorldAndBackupFeatures } from "@/lib/feature-flags";
import { getGameArt } from "@/lib/game-art";
import { gameDescription, gameDisplayName } from "@/lib/game-display";
import { providerDescription, providerDisplayName } from "@/lib/provider-display";
import { formatCreateServerError } from "@/lib/runtime-errors";
import { cn } from "@/lib/utils";
import { createConfigPreset, getGameVersions, listConfigPresets, listGames, listGlobalMods, listModPacks, listWorlds } from "@/lib/api";
import { defaultCreateServerConfig, defaultCreateServerMode, defaultCreateServerPreset } from "@/lib/create-server-defaults";
import { createGameServerWithResources } from "@/lib/create-server-flow";
import { createReviewInvitePreview, reviewJoinInstructionKey } from "@/lib/create-server-review";
import { filterModResources } from "@/lib/mod-filters";
import { createDefaultProviderConfigPayload, providerConfigValue, updateProviderConfigPayload, type ProviderConfigPayload } from "@/lib/provider-config";
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
  game: "stepGame",
  mode: "stepMode",
  preset: "stepPreset",
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

type BuiltInPresetKey = (typeof presets)[number]["key"];
type PresetKey = BuiltInPresetKey | typeof customPreset.key;
type PresetTag = (typeof presets)[number]["tags"][number] | (typeof customPreset)["tags"][number];

const cpuLimitOptions = [0, 0.5, 1, 2, 4] as const;
const memoryLimitOptions = [0, 1024, 2048, 4096, 8192] as const;
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

function createGameDefaultNames(gameName: string) {
  const suffix = createNameSuffix();
  return {
    clusterName: appendNameSuffix(`${gameName} Cluster`, suffix),
    saveName: appendNameSuffix(`${gameName} Save`, suffix),
    serverName: appendNameSuffix(`${gameName} Server`, suffix),
    worldName: appendNameSuffix(`${gameName} World`, suffix)
  };
}

function createProviderDefaultOverrides(names: ReturnType<typeof createGameDefaultNames>) {
  return {
    clusterName: names.clusterName,
    saveName: names.saveName,
    serverName: names.serverName,
    worldName: names.worldName
  };
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
    return option?.label ?? String(value ?? "");
  }
  return String(value ?? "");
}

function createReviewConfigModel({
  config,
  gameKey,
  gameName,
  hostPortLabel,
  provider,
  providerConfigPayload,
  t,
  version
}: {
  config: TerrariaConfig;
  gameKey: string;
  gameName: string;
  hostPortLabel: string;
  provider?: ProviderCatalog;
  providerConfigPayload: ProviderConfigPayload;
  t: (key: MessageKey, values?: Record<string, string | number>) => string;
  version: string;
}): ReviewConfigModel {
  if (gameKey === "terraria") {
    const secretSeed = secretSeedKeyFor(config.seed);
    const selectedSeedModeCount = terrariaSeedModeCodes(config).length;
    const seedLabel = secretSeed
      ? `${terrariaSecretSeeds.find((seed) => seed.key === secretSeed)?.label ?? secretSeed} · ${secretSeed}`
      : config.seed?.trim() || t("tagRandom");
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
        ...(selectedSeedModeCount > 0 ? [{ label: t("seedModes"), value: t("seedModesSummary", { special: config.specialSeeds?.length ?? 0, secret: config.secretSeeds?.length ?? 0 }) }] : []),
        { label: t("maxPlayersInput"), value: String(config.maxPlayers) },
        { label: t("password"), value: config.password ? t("enabled") : t("none") },
        { label: t("secureMode"), value: config.secure ? t("enabled") : t("disabled") },
        { label: t("autoCreateWorld"), value: config.autoCreateWorld ? t("enabled") : t("disabled") },
        ...(version ? [{ label: t("gameVersion"), value: version }] : []),
        { label: t("externalPort"), value: hostPortLabel }
      ]
    };
  }

  const fields = (provider?.configSchema ?? [])
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
  const [presetName, setPresetName] = useState("");
  const [presetDialogOpen, setPresetDialogOpen] = useState(false);
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
  const games = gamesQuery.data ?? [];
  const selectedGame = games.find((game) => game.key === selectedGameKey) ?? games[0] ?? games.find((game) => game.key === "terraria");
  const selectedGameArt = getGameArt(selectedGame?.coverImage ?? selectedGame?.key ?? selectedGameKey);
  const SelectedGameIcon = selectedGameArt.icon;
  const selectedProvider = selectedGame?.providers.find((provider) => provider.key === selectedProviderKey) ?? selectedGame?.providers.find((provider) => provider.recommended) ?? selectedGame?.providers[0];
  const providerKey = selectedProvider?.key ?? selectedProviderKey;
  const stepIds: StepId[] = useMemo(() => [
    "game",
    "mode",
    ...(selectedGameKey === "terraria" ? ["preset" as const] : []),
    "config",
    "resources",
    ...(selectedProvider?.capabilities.mods ? ["mods" as const] : []),
    "review"
  ], [selectedGameKey, selectedProvider?.capabilities.mods]);
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
  const canContinueCurrentStep = currentStepId === "game"
    ? selectedGame?.status === "available" && selectedGameHasReadyProvider
    : currentStepId === "mode"
      ? selectedProviderReady
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
      modPackId: selectedModPackId || undefined
    }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["config-presets"] });
    }
  });
  const openPresetDialog = () => {
    saveConfigPreset.reset();
    const defaultPresetSource = selectedGameKey === "terraria"
      ? config.serverName
      : providerServerName(providerConfigPayload, "");
    setPresetName(`${defaultPresetSource || gameDisplayName(selectedGame?.key ?? selectedGameKey, selectedGame?.name ?? selectedGameKey, t)} ${t("configurationPreset")}`);
    setPresetDialogOpen(true);
  };
  const closePresetDialog = () => {
    if (saveConfigPreset.isPending) return;
    saveConfigPreset.reset();
    setPresetDialogOpen(false);
  };
  const submitPreset = () => {
    const name = presetName.trim();
    if (!name) return;
    saveConfigPreset.mutate(name);
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
      const names = createGameDefaultNames(game.name);
      setSelectedPreset("custom");
      setConfig(editableTerrariaConfig({
        ...defaultCreateServerConfig,
        serverName: names.serverName,
        worldName: names.worldName,
        maxPlayers: 8,
        password: "",
        motd: ""
      }));
      setProviderConfigPayload(createDefaultProviderConfigPayload(nextProvider, createProviderDefaultOverrides(names)));
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
      const names = createGameDefaultNames(selectedGame?.name ?? provider.name);
      setSelectedPreset("custom");
      setProviderConfigPayload(createDefaultProviderConfigPayload(provider, createProviderDefaultOverrides(names)));
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
    const pack = allModPacks.find((item) => item.id === preset.modPackId);
    setSelectedModIds(pack?.modIds ?? []);
    setStep(preset.gameKey === "terraria" ? 3 : 2);
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
    setStep(1);
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
    setStep(3);
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
          <div className="mt-7 grid grid-cols-3 gap-3 md:grid-cols-6">
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
            {currentStepId === "game" && (
              <GameStep
                games={games}
                isLoading={gamesQuery.isLoading}
                selectedGameKey={selectedGameKey}
                onSelectGame={chooseGame}
              />
            )}
            {currentStepId === "mode" && (
              <ModeStep
                configPresets={gameConfigPresets}
                mode={mode}
                providers={selectedGame?.providers ?? []}
                selectedProviderKey={providerKey}
                setMode={chooseMode}
                onSelectConfigPreset={applyConfigPreset}
                onSelectProvider={chooseProvider}
              />
            )}
            {currentStepId === "preset" && <PresetStep selectedPreset={selectedPreset} setPreset={choosePreset} />}
            {currentStepId === "config" && (
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
            )}
            {currentStepId === "resources" && (
              <ResourcesStep
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
                configModel={createReviewConfigModel({
                  config,
                  gameKey: selectedGameKey,
                  gameName: selectedGame ? gameDisplayName(selectedGame.key, selectedGame.name, t) : t("gameNameTerraria"),
                  hostPortLabel: hostPortMode === "manual" ? String(hostPort) : t("automaticPort"),
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
                presetDialogOpen={presetDialogOpen}
                presetName={presetName}
                presetSaveError={saveConfigPreset.error instanceof Error ? saveConfigPreset.error.message : ""}
                presetSavePending={saveConfigPreset.isPending}
                presetSaveSuccess={saveConfigPreset.isSuccess}
                onChangePresetName={setPresetName}
                onClosePreset={closePresetDialog}
                onOpenPreset={openPresetDialog}
                onSavePreset={submitPreset}
              />
            )}
          </motion.div>
          <div className="mt-8 flex justify-between">
            <Button variant="secondary" disabled={step === 0} onClick={() => setStep((value) => Math.max(0, value - 1))}>
              <ChevronLeft aria-hidden="true" />
              {t("back")}
            </Button>
            <Button
              onClick={() => {
                if ((currentStepId === "config" || currentStepId === "review") && !validateCurrentConfig()) return;
                if (step === stepIds.length - 1) {
                  create.mutate();
                  return;
                }
                setStep((value) => Math.min(stepIds.length - 1, value + 1));
              }}
              disabled={create.isPending || !canContinueCurrentStep || (step === stepIds.length - 1 && !canCreateSelectedProvider)}
            >
              {step === stepIds.length - 1 ? create.isPending ? t("creating") : t("createServerLower") : t("nextStep", { step: t(nextStepKey) })}
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
  configPresets,
  mode,
  providers,
  selectedProviderKey,
  setMode,
  onSelectConfigPreset,
  onSelectProvider
}: {
  configPresets: ConfigPreset[];
  mode: "vanilla" | "tmodloader";
  providers: ProviderCatalog[];
  selectedProviderKey: ProviderKey;
  setMode: (mode: "vanilla" | "tmodloader") => void;
  onSelectConfigPreset: (preset: ConfigPreset) => void;
  onSelectProvider: (provider: ProviderCatalog) => void;
}) {
  const { t } = useI18n();
  const visibleConfigPresets = configPresets.slice(0, 4);
  const modeProviders = orderModeProviders(providers);
  const configPresetSection = visibleConfigPresets.length > 0 && (
    <div className="rounded-lg border border-panel-line bg-slate-950/35 p-4">
      <div className="flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-panel-line bg-slate-950/50 text-panel-green">
          <Bookmark aria-hidden="true" className="size-4" />
        </span>
        <div>
          <h2 className="text-lg font-semibold">{t("configurationPresets")}</h2>
          <p className="mt-1 text-sm text-slate-400">{t("gameConfigurationPresetsDescription")}</p>
        </div>
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        {visibleConfigPresets.map((preset) => (
          <button
            key={preset.id}
            type="button"
            className="rounded-md border border-panel-line bg-slate-950/50 p-3 text-left transition hover:border-panel-green/50 hover:bg-slate-900/60 focus:outline-none focus:ring-2 focus:ring-panel-green/50"
            onClick={() => onSelectConfigPreset(preset)}
          >
            <p className="font-medium text-slate-100">{preset.name}</p>
            <p className="mt-1 text-xs text-slate-500">
              {providerDisplayName(preset.providerKey, preset.providerKey, t)}
              {preset.version ? ` · ${preset.version}` : ""}
            </p>
          </button>
        ))}
      </div>
    </div>
  );
  if (providers.length > 0) {
    return (
      <div className="space-y-6">
        {configPresetSection}
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
      </div>
    );
  }
  return (
    <div className="space-y-6">
      {configPresetSection}
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
    </div>
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
  setPreset
}: {
  selectedPreset: PresetKey;
  setPreset: (preset: PresetKey) => void;
}) {
  const { t } = useI18n();
  const presetOptions = [...presets, customPreset];
  const renderTag = (tag: PresetTag) => {
    if (tag === "6" || tag === "8" || tag === "12") return t("tagPlayers", { count: tag });
    return t(tag as MessageKey);
  };
  return (
    <div>
      <h2 className="text-lg font-semibold">{t("choosePreset")}</h2>
      <p className="mt-1 text-sm text-slate-400">{t("presetDescription")}</p>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
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
                "relative rounded-lg border p-4 text-left transition focus:outline-none focus:ring-2",
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
              <p className="mt-1 text-sm text-slate-400">{t(preset.descriptionKey)}</p>
              <div className="mt-4 flex flex-wrap gap-2">
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
    </div>
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
            {provider?.key === "dont-starve-together" ? (
              <DSTProviderConfigSections
                fields={providerFields}
                payload={providerConfigPayload}
                validationErrors={validationErrors}
                onCustomize={onCustomize}
                onClearValidationError={onClearValidationError}
                setProviderConfigPayload={setProviderConfigPayload}
              />
            ) : (
              <ProviderSchemaFieldsGrid
                fields={providerFields}
                payload={providerConfigPayload}
                validationErrors={validationErrors}
                onCustomize={onCustomize}
                onClearValidationError={onClearValidationError}
                setProviderConfigPayload={setProviderConfigPayload}
              />
            )}
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
                <WizardSelect value={version} onChange={(value) => setVersion(value)}>
                  {versions.map((v) => (
                    <option key={v} value={v}>{v}</option>
                  ))}
                </WizardSelect>
              </WizardField>
              <WizardField label={t("externalPort")}>
                <WizardSelect value={hostPortMode} onChange={(value) => setHostPortMode(value as "auto" | "manual")}>
                  <option value="auto">{t("automaticPort")}</option>
                  <option value="manual">{t("manualPort")}</option>
                </WizardSelect>
              </WizardField>
              {hostPortMode === "manual" && (
                <WizardField label={t("externalPortValue")}>
                  <Input type="number" min={1024} max={65535} value={hostPort} onChange={(event) => setHostPort(Number(event.target.value))} />
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
            value={config.serverName ?? ""}
            aria-invalid={Boolean(validationErrors.serverName)}
            className={validationErrors.serverName ? "border-red-400/70 focus:border-red-300" : undefined}
            onChange={(event) => update("serverName", event.target.value)}
          />
        </WizardField>
        <WizardField label={t("worldName")} required error={validationErrors.worldName}>
          <Input
            value={config.worldName}
            aria-invalid={Boolean(validationErrors.worldName)}
            className={validationErrors.worldName ? "border-red-400/70 focus:border-red-300" : undefined}
            onChange={(event) => update("worldName", event.target.value)}
          />
        </WizardField>
        <WizardField label={t("worldSize")}>
          <WizardSelect value={config.worldSize} onChange={(value) => update("worldSize", value as TerrariaConfig["worldSize"])}>
            <option value="small">{t("tagSmallWorld")}</option>
            <option value="medium">{t("tagMediumWorld")}</option>
            <option value="large">{t("tagLargeWorld")}</option>
          </WizardSelect>
        </WizardField>
        <WizardField label={t("worldEvil")}>
          <WizardSelect value={config.worldEvil} onChange={(value) => update("worldEvil", value as TerrariaConfig["worldEvil"])}>
            <option value="random">{t("tagRandom")}</option>
            <option value="corruption">{t("tagCorruption")}</option>
            <option value="crimson">{t("tagCrimson")}</option>
          </WizardSelect>
        </WizardField>
        <WizardField label={t("difficulty")}>
          <WizardSelect value={config.difficulty} onChange={(value) => update("difficulty", value as TerrariaConfig["difficulty"])}>
            <option value="journey">{t("tagJourney")}</option>
            <option value="classic">{t("tagClassic")}</option>
            <option value="expert">{t("tagExpert")}</option>
            <option value="master">{t("tagMaster")}</option>
          </WizardSelect>
        </WizardField>
        <WizardField label={t("gameVersion")}>
          <WizardSelect value={version} onChange={(value) => setVersion(value)}>
            {versions.map((v) => (
              <option key={v} value={v}>{v}</option>
            ))}
          </WizardSelect>
        </WizardField>
        <WizardField label={t("externalPort")}>
          <WizardSelect value={hostPortMode} onChange={(value) => setHostPortMode(value as "auto" | "manual")}>
            <option value="auto">{t("automaticPort")}</option>
            <option value="manual">{t("manualPort")}</option>
          </WizardSelect>
        </WizardField>
        {hostPortMode === "manual" && (
          <WizardField label={t("externalPortValue")}>
            <Input type="number" min={1024} max={65535} value={hostPort} onChange={(event) => setHostPort(Number(event.target.value))} />
          </WizardField>
        )}
        <WizardField label={t("maxPlayersInput")} required error={validationErrors.maxPlayers}>
          <Input
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
          <Input value={config.password ?? ""} onChange={(event) => update("password", event.target.value)} />
        </WizardField>
        <WizardField label={t("motd")}>
          <Input value={config.motd ?? ""} onChange={(event) => update("motd", event.target.value)} />
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
            />
          </WizardField>
        </div>
        <div className="grid gap-3 md:col-span-2 sm:grid-cols-2">
          <WizardCheckbox label={t("secureMode")} checked={config.secure} onChange={(checked) => update("secure", checked)} />
          <WizardCheckbox label={t("autoCreateWorld")} checked={config.autoCreateWorld} onChange={(checked) => update("autoCreateWorld", checked)} />
        </div>
      </div>
    </div>
  );
}

function SeedInput({
  config,
  onChange,
  onChangeSeedModes,
  placeholder,
  supportsLegacySecretSeedPicker,
  supportsModernSeedModes,
  value
}: {
  config: TerrariaConfig;
  onChange: (value: string) => void;
  onChangeSeedModes: (specialSeeds: string[], secretSeeds: string[]) => void;
  placeholder: string;
  supportsLegacySecretSeedPicker: boolean;
  supportsModernSeedModes: boolean;
  value: string;
}) {
  const { t } = useI18n();
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
  const clearLegacySeed = () => {
    onChange("");
    onChangeSeedModes([], []);
  };
  const selectLegacySeed = (key: string) => {
    onChange(key);
    onChangeSeedModes([], []);
    setOpen(false);
  };
  const pickerLabel = supportsModernSeedModes
    ? selectedModeCount > 0 ? t("seedModesSelected", { count: selectedModeCount }) : t("seedModes")
    : legacySpecialSeed ? legacySpecialSeed.label : t("secretSeed");
  return (
    <div className="relative space-y-1.5">
      <div className="relative">
        <Input
          value={value}
          placeholder={placeholder}
          className={showsSeedPicker ? "pr-36" : undefined}
          onChange={(event) => onChange(event.target.value)}
        />
        {showsSeedPicker ? (
          <button
            type="button"
            aria-expanded={open}
            className={cn(
              "absolute right-1.5 top-1/2 inline-flex h-7 -translate-y-1/2 items-center gap-1 rounded-md border px-2 text-xs font-medium transition focus:outline-none focus:ring-2 focus:ring-panel-green/40",
              selectedModeCount > 0 || Boolean(legacySpecialSeed)
                ? "border-panel-green/50 bg-panel-green/15 text-panel-green"
                : "border-panel-line bg-slate-950/70 text-slate-400 hover:border-slate-600 hover:bg-slate-900 hover:text-slate-200"
            )}
            onClick={() => setOpen(true)}
          >
            {pickerLabel}
            <ChevronDown aria-hidden="true" className="size-3.5" />
          </button>
        ) : null}
      </div>
      {supportsLegacySecretSeedPicker && legacySpecialSeed ? (
        <p className="text-xs leading-5 text-panel-green">
          {t("secretSeedDetected", { name: legacySpecialSeed.label })}
          <span className="text-slate-500"> · {legacySpecialSeed.description}</span>
        </p>
      ) : null}
      {supportsModernSeedModes && selectedModeCount > 0 ? (
        <p className="text-xs leading-5 text-panel-green">
          {t("seedModesSummary", { special: selectedSpecialSeeds.length, secret: selectedSecretSeeds.length })}
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
          return (
            <button
              key={seed.key}
              type="button"
              className={cn(
                "group flex min-h-20 items-start justify-between gap-3 rounded-md border px-3 py-2 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/40",
                active
                  ? "border-panel-green/55 bg-panel-green/12 text-white"
                  : "border-panel-line bg-slate-950/45 text-slate-300 hover:border-slate-600 hover:bg-slate-900"
              )}
              onClick={() => onToggle(seed.key)}
            >
              <span className="min-w-0">
                <span className="block truncate text-sm font-semibold text-slate-100">{seed.label}</span>
                <span className="mt-0.5 block truncate text-xs text-slate-500">{seed.key}</span>
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
  onChange,
  resourceLimits
}: {
  onChange: (limits: ResourceLimits) => void;
  resourceLimits: ResourceLimits;
}) {
  return (
    <div>
      <RuntimeResourceSection resourceLimits={resourceLimits} onChange={onChange} />
    </div>
  );
}

function ProviderSchemaField({
  error,
  field,
  label,
  onChange,
  value
}: {
  error?: string;
  field: ProviderConfigField;
  label: string;
  onChange: (value: string | boolean) => void;
  value: unknown;
}) {
  const { t } = useI18n();
  const help = providerFieldHelp(field, t);
  if (field.type === "boolean") {
    return (
      <div className="grid w-full min-w-0 self-start content-start gap-1.5">
        <span className="flex min-w-0 items-center gap-2 text-xs font-medium text-slate-500">
          <span className="truncate">{label}</span>
          {field.required && <RequiredFieldBadge />}
        </span>
        <WizardCheckbox label={help || label} checked={Boolean(value)} onChange={onChange} invalid={Boolean(error)} />
        {error && <span className="text-xs font-medium text-red-200">{error}</span>}
      </div>
    );
  }

  return (
    <WizardField label={label} required={field.required} error={error}>
                {field.type === "select" ? (
                  <WizardSelect value={String(value ?? "")} onChange={onChange} invalid={Boolean(error)}>
                    {(field.options ?? []).map((option) => (
                      <option key={option.value} value={option.value}>{option.label}</option>
                    ))}
                  </WizardSelect>
                ) : (
                  <Input
                    type={field.type === "password" ? "password" : field.type === "number" ? "number" : "text"}
                    value={field.type === "number" ? Number(value ?? 0) : String(value ?? "")}
                    aria-invalid={Boolean(error)}
                    className={error ? "border-red-400/70 focus:border-red-300" : undefined}
                    onChange={(event) => onChange(event.target.value)}
                  />
                )}
                {help && <span className="text-xs text-slate-500">{help}</span>}
    </WizardField>
  );
}

function ProviderSchemaFieldsGrid({
  fields,
  onClearValidationError,
  onCustomize,
  payload,
  setProviderConfigPayload,
  validationErrors
}: {
  fields: ProviderConfigField[];
  onClearValidationError: (field: string) => void;
  onCustomize: () => void;
  payload: ProviderConfigPayload;
  setProviderConfigPayload: (payload: ProviderConfigPayload) => void;
  validationErrors: ConfigValidationErrors;
}) {
  const { t } = useI18n();
  return (
    <div className="mt-4 grid gap-4 md:grid-cols-2">
      {fields.map((field) => (
        <ProviderSchemaField
          key={field.name}
          error={validationErrors[field.name]}
          field={field}
          label={providerFieldLabel(field, t)}
          value={providerConfigValue(payload, field.name)}
          onChange={(nextValue) => {
            onCustomize();
            onClearValidationError(field.name);
            setProviderConfigPayload(updateProviderConfigPayload(payload, field, nextValue));
          }}
        />
      ))}
    </div>
  );
}

const dstConfigSections = [
  { title: "Identity and Access", prefix: "identity.", summary: ["identity.visibility"] },
  { title: "Gameplay", prefix: "gameplay.", summary: ["gameplay.gameMode", "gameplay.maxPlayers"] },
  { title: "World Generation", prefix: "world.", summary: ["world.preset"] },
  { title: "Caves", prefix: "caves.", summary: ["caves.enabled"] }
] as const;

function DSTProviderConfigSections(props: {
  fields: ProviderConfigField[];
  onClearValidationError: (field: string) => void;
  onCustomize: () => void;
  payload: ProviderConfigPayload;
  setProviderConfigPayload: (payload: ProviderConfigPayload) => void;
  validationErrors: ConfigValidationErrors;
}) {
  return (
    <div className="mt-4 grid gap-3">
      {dstConfigSections.map((section) => {
        const fields = props.fields.filter((field) => field.name.startsWith(section.prefix));
        if (fields.length === 0) return null;
        return (
          <section key={section.title} className="rounded-md border border-panel-line bg-slate-950/40 p-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h4 className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-400">{section.title}</h4>
              <div className="flex flex-wrap gap-1.5">
                {section.summary.map((path) => {
                  const value = providerConfigValue(props.payload, path);
                  return (
                    <span key={path} className="rounded border border-panel-line bg-slate-900 px-2 py-1 text-[11px] text-slate-400">
                      {String(value ?? "default")}
                    </span>
                  );
                })}
              </div>
            </div>
            <ProviderSchemaFieldsGrid {...props} fields={fields} />
          </section>
        );
      })}
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
  onChange,
  resourceLimits
}: {
  onChange: (limits: ResourceLimits) => void;
  resourceLimits: ResourceLimits;
}) {
  const { t } = useI18n();
  return (
    <section className="rounded-lg border border-panel-line bg-slate-950/35 p-4 md:col-span-2">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h3 className="text-sm font-semibold text-slate-100">{t("runtimeResources")}</h3>
          <p className="mt-1 max-w-2xl text-xs leading-5 text-slate-500">{t("resourceLimitsHint")}</p>
        </div>
        <div className="shrink-0 rounded-md border border-panel-line bg-slate-950/60 px-3 py-2 text-xs font-medium text-slate-300">
          {formatCpuLimitLabel(resourceLimits.cpuLimitCores, t)} · {formatMemoryLimitLabel(resourceLimits.memoryLimitMb, t)}
        </div>
      </div>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <WizardField label={t("cpuLimit")}>
          <WizardSelect value={String(resourceLimits.cpuLimitCores)} onChange={(value) => onChange({ ...resourceLimits, cpuLimitCores: Number(value) })}>
            {cpuLimitOptions.map((value) => (
              <option key={value} value={value}>{formatCpuLimitLabel(value, t)}</option>
            ))}
          </WizardSelect>
        </WizardField>
        <WizardField label={t("memoryLimit")}>
          <WizardSelect value={String(resourceLimits.memoryLimitMb)} onChange={(value) => onChange({ ...resourceLimits, memoryLimitMb: Number(value) })}>
            {memoryLimitOptions.map((value) => (
              <option key={value} value={value}>{formatMemoryLimitLabel(value, t)}</option>
            ))}
          </WizardSelect>
        </WizardField>
      </div>
    </section>
  );
}

function PresetSaveDialog({
  error,
  name,
  onChangeName,
  onClose,
  onSave,
  open,
  pending,
  success
}: {
  error: string;
  name: string;
  onChangeName: (name: string) => void;
  onClose: () => void;
  onSave: () => void;
  open: boolean;
  pending: boolean;
  success: boolean;
}) {
  const { t } = useI18n();
  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !pending) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [onClose, open, pending]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/75 px-4 py-8 backdrop-blur-sm"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget && !pending) onClose();
      }}
    >
      <div
        aria-describedby="preset-save-dialog-description"
        aria-labelledby="preset-save-dialog-title"
        aria-modal="true"
        className="w-full max-w-lg rounded-lg border border-panel-line bg-panel-card shadow-[0_16px_48px_rgba(0,0,0,0.4)]"
        role="dialog"
      >
        <div className="flex items-start justify-between gap-4 border-b border-panel-line p-5">
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-white" id="preset-save-dialog-title">{t("saveConfigurationPreset")}</h2>
            <p className="mt-1 text-sm leading-6 text-slate-500" id="preset-save-dialog-description">{t("configurationPresetSaveHint")}</p>
          </div>
          <button
            aria-label={t("cancel")}
            className="flex size-8 shrink-0 items-center justify-center rounded-md text-slate-400 transition hover:bg-slate-800 hover:text-white focus:outline-none focus:ring-2 focus:ring-panel-green/50 disabled:cursor-not-allowed disabled:opacity-50"
            disabled={pending}
            onClick={onClose}
            type="button"
          >
            <X aria-hidden="true" className="size-4" />
          </button>
        </div>
        <div className="p-5">
          <WizardField label={t("configurationPresetName")}>
            <Input
              autoFocus
              value={name}
              placeholder={t("configurationPresetNamePlaceholder")}
              onChange={(event) => onChangeName(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !pending && name.trim().length > 0) {
                  onSave();
                }
              }}
            />
          </WizardField>
          {success && <p className="mt-3 text-xs text-panel-green">{t("configurationPresetSaved")}</p>}
          {error && <p className="mt-3 text-xs text-panel-gold">{error}</p>}
          <div className="mt-5 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button variant="secondary" onClick={onClose} disabled={pending}>{t("cancel")}</Button>
            <Button variant="primary" onClick={onSave} disabled={pending || name.trim().length === 0}>
              <Bookmark aria-hidden="true" />
              {pending ? t("saving") : t("saveConfigurationPreset")}
            </Button>
          </div>
        </div>
      </div>
    </div>
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

function RequiredFieldBadge() {
  const { t } = useI18n();
  return (
    <span className="shrink-0 rounded border border-panel-gold/30 bg-panel-gold/10 px-1.5 py-0.5 text-[10px] font-semibold text-panel-gold">
      {t("requiredField")}
    </span>
  );
}

function WizardSelect({
  children,
  invalid,
  onChange,
  value
}: {
  children: React.ReactNode;
  invalid?: boolean;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <select
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

function WizardCheckbox({ checked, invalid, label, onChange }: { checked: boolean; invalid?: boolean; label: string; onChange: (checked: boolean) => void }) {
  return (
    <label
      className={cn(
        "grid min-h-11 cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-3 rounded-md border px-3 py-2 text-sm transition",
        checked
          ? "border-panel-green/45 bg-panel-green/10 text-slate-100"
          : "border-panel-line bg-slate-950/40 text-slate-400 hover:border-slate-600 hover:text-slate-200",
        invalid && "border-red-400/70 bg-red-950/20 text-red-100"
      )}
    >
      <span className="min-w-0 break-words font-medium leading-5">{label}</span>
      <input
        className="sr-only"
        checked={checked}
        type="checkbox"
        onChange={(event) => onChange(event.target.checked)}
      />
      <span
        aria-hidden="true"
        className={cn(
          "flex size-5 shrink-0 items-center justify-center rounded border",
          checked ? "border-panel-green bg-panel-green text-slate-950" : "border-panel-line bg-slate-950 text-transparent",
          invalid && !checked ? "border-red-300/70 bg-red-950/20" : null
        )}
      >
        <Check className="size-3.5" />
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
          <h2 className="text-lg font-semibold">{t("selectMods")}</h2>
          <p className="mt-1 text-sm text-slate-400">{t("selectModsHint")}</p>
          {modPacks.length > 0 && (
            <div className="mt-4">
              <p className="text-xs font-medium text-slate-500">{t("modPacks")}</p>
              <div className="mt-2 grid gap-2">
                {modPacks.map((pack) => (
                  <button
                    key={pack.id}
                    type="button"
                    onClick={() => onSelectModPack(selectedModPackId === pack.id ? "" : pack.id)}
                    className={cn(
                      "flex w-full items-center justify-between gap-3 rounded-lg border p-3 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
                      selectedModPackId === pack.id
                        ? "border-panel-green bg-panel-green/10 ring-1 ring-panel-green/40"
                        : "border-panel-line bg-slate-950/40 hover:bg-slate-900/55"
                    )}
                  >
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{pack.name}</p>
                      <p className="mt-0.5 truncate text-xs text-slate-500">{pack.description || pack.mods.map((mod) => modDisplayName(mod, locale)).join(", ")}</p>
                    </div>
                    <span className="shrink-0 rounded bg-panel-green/15 px-2 py-1 text-xs text-panel-green">{pack.mods.length}</span>
                  </button>
                ))}
              </div>
            </div>
          )}
          <div className="mt-4 space-y-2">
            {mods.length === 0 ? (
              <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-panel-line bg-slate-950/30 py-8 text-center">
                <Package aria-hidden="true" className="size-8 text-slate-600" />
                <p className="mt-3 text-sm text-slate-400">{t("noModsInLibrary")}</p>
                <Link href="/mods" className="mt-3 inline-flex items-center gap-2 text-sm text-panel-green hover:underline">
                  <Package aria-hidden="true" className="size-4" />
                  {t("goToModsPage")}
                </Link>
              </div>
            ) : (
              <>
                {mods.map((mod) => (
                  <button
                    key={mod.id}
                    type="button"
                    onClick={() => onToggleMod(mod.id)}
                    className={cn(
                      "flex w-full items-center gap-3 rounded-lg border p-3 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
                      selectedModIds.includes(mod.id)
                        ? "border-panel-green bg-panel-green/10 ring-1 ring-panel-green/40"
                        : "border-panel-line bg-slate-950/40 hover:bg-slate-900/55"
                    )}
                  >
                    <span className={cn("flex size-5 shrink-0 items-center justify-center rounded border", selectedModIds.includes(mod.id) ? "border-panel-green bg-panel-green text-white" : "border-slate-600")}>
                      {selectedModIds.includes(mod.id) && <Check aria-hidden="true" className="size-3" />}
                    </span>
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{modDisplayName(mod, locale)}</p>
                      <p className="mt-0.5 text-xs text-slate-500">{mod.size}</p>
                    </div>
                  </button>
                ))}
                <Link href="/mods" className="inline-flex items-center gap-2 pt-1 text-sm text-panel-green hover:underline">
                  <Package aria-hidden="true" className="size-4" />
                  {t("goToModsPage")}
                </Link>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function ReviewStep({
  configModel,
  gameKey,
  gameName,
  hostPortLabel,
  resourceLimits,
  selectedWorldName,
  selectedModNames,
  presetDialogOpen,
  presetName,
  presetSaveError,
  presetSavePending,
  presetSaveSuccess,
  onChangePresetName,
  onClosePreset,
  onOpenPreset,
  onSavePreset
}: {
  configModel: ReviewConfigModel;
  gameKey: string;
  gameName: string;
  hostPortLabel: string;
  resourceLimits: ResourceLimits;
  selectedWorldName?: string;
  selectedModNames: string[];
  presetDialogOpen: boolean;
  presetName: string;
  presetSaveError: string;
  presetSavePending: boolean;
  presetSaveSuccess: boolean;
  onChangePresetName: (name: string) => void;
  onClosePreset: () => void;
  onOpenPreset: () => void;
  onSavePreset: () => void;
}) {
  const { t } = useI18n();
  const invitePreview = createReviewInvitePreview({
    gameKey,
    gameName,
    hostPortLabel,
    password: configModel.password,
    serverName: configModel.serverName || gameName
  });
  const joinInstruction = t(reviewJoinInstructionKey(gameKey));
  return (
    <div>
      <PresetSaveDialog
        error={presetSaveError}
        name={presetName}
        open={presetDialogOpen}
        pending={presetSavePending}
        success={presetSaveSuccess}
        onChangeName={onChangePresetName}
        onClose={onClosePreset}
        onSave={onSavePreset}
      />
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h2 className="text-lg font-semibold">{t("review")}</h2>
          <p className="mt-1 text-sm text-slate-500">{t("configurationPresetSaveHint")}</p>
        </div>
        <Button
          type="button"
          variant="secondary"
          aria-expanded={presetDialogOpen}
          aria-haspopup="dialog"
          onClick={onOpenPreset}
        >
          <Bookmark aria-hidden="true" />
          {t("saveConfigurationPreset")}
        </Button>
      </div>
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
    </div>
  );
}

function ReviewConfigItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-panel-line bg-slate-950/55 px-3 py-2">
      <p className="text-xs text-slate-500">{label}</p>
      <p className="mt-1 truncate text-sm font-medium text-slate-200">{value}</p>
    </div>
  );
}
