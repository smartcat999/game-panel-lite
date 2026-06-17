"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronLeft, ChevronRight, FileArchive, Gamepad2, Globe, Hammer, Package, Settings2, X } from "lucide-react";
import Image from "next/image";
import { useEffect, useMemo, useState } from "react";
import { Button, Card, Input } from "@/components/ui";
import { useI18n, type MessageKey } from "@/lib/i18n";
import { modDisplayName } from "@/lib/mod-display";
import { cn } from "@/lib/utils";
import { getTerrariaVersions, listGlobalMods, listModPacks, listWorlds, previewTerrariaConfig } from "@/lib/api";
import { defaultCreateServerConfig, defaultCreateServerMode, defaultCreateServerPreset } from "@/lib/create-server-defaults";
import { createTerrariaServerWithWorld } from "@/lib/create-server-flow";
import { getTerrariaPreset, secretSeedKeyFor, terrariaInternalPort, terrariaSecretSeeds, type TerrariaConfig } from "@gamepanel-lite/shared";
import type { ModFile, ModPack, ResourceLimits } from "@/lib/types";

const stepKeys = ["stepGame", "stepMode", "stepPreset", "stepConfig", "stepMods", "stepReview"] as const;
const presets = [
  { key: "friends-casual", labelKey: "presetFriendsCasual", descriptionKey: "presetFriendsCasualDescription", tags: ["tagClassic", "tagMediumWorld", "8"] },
  { key: "building-world", labelKey: "presetBuildingWorld", descriptionKey: "presetBuildingWorldDescription", tags: ["tagClassic", "tagLargeWorld", "12"] },
  { key: "expert-adventure", labelKey: "presetExpertAdventure", descriptionKey: "presetExpertAdventureDescription", tags: ["tagExpert", "tagLargeWorld", "8"] },
  { key: "modded-starter", labelKey: "presetModdedStarter", descriptionKey: "presetModdedStarterDescription", tags: ["tModLoader", "tagMediumWorld", "8"] },
  { key: "master-challenge", labelKey: "presetMasterChallenge", descriptionKey: "presetMasterChallengeDescription", tags: ["tagMaster", "tagLargeWorld", "6"] }
] as const;
const customPreset = { key: "custom", labelKey: "presetCustom", descriptionKey: "presetCustomDescription", tags: ["tagCustom"] } as const;

type BuiltInPresetKey = (typeof presets)[number]["key"];
type PresetKey = BuiltInPresetKey | typeof customPreset.key;
type PresetTag = (typeof presets)[number]["tags"][number] | (typeof customPreset)["tags"][number];

const cpuLimitOptions = [0, 0.5, 1, 2, 4] as const;
const memoryLimitOptions = [0, 1024, 2048, 4096, 8192] as const;

function formatCpuLimitLabel(value: number, t: (key: MessageKey, values?: Record<string, string | number>) => string) {
  return value > 0 ? t("cpuCoresValue", { cores: value }) : t("unlimited");
}

function formatMemoryLimitLabel(value: number, t: (key: MessageKey, values?: Record<string, string | number>) => string) {
  return value > 0 ? t("memoryGbValue", { gb: value / 1024 }) : t("unlimited");
}

export function CreateServerWizard() {
  const { locale, t } = useI18n();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [step, setStep] = useState(0);
  const [mode, setMode] = useState<"vanilla" | "tmodloader">(defaultCreateServerMode);
  const [selectedPreset, setSelectedPreset] = useState<PresetKey>(defaultCreateServerPreset);
  const [config, setConfig] = useState<TerrariaConfig>(defaultCreateServerConfig);
  const [hostPortMode, setHostPortMode] = useState<"auto" | "manual">("auto");
  const [hostPort, setHostPort] = useState(terrariaInternalPort);
  const [resourceLimits, setResourceLimits] = useState<ResourceLimits>({ cpuLimitCores: 0, memoryLimitMb: 0 });
  const [version, setVersion] = useState("");
  const [selectedWorldId, setSelectedWorldId] = useState("");
  const [appliedWorldConfigId, setAppliedWorldConfigId] = useState("");
  const [selectedModIds, setSelectedModIds] = useState<string[]>([]);
  const [selectedModPackId, setSelectedModPackId] = useState("");
  const versionsQuery = useQuery({ queryKey: ["terraria-versions"], queryFn: getTerrariaVersions, staleTime: 5 * 60 * 1000 });
  const worldsQuery = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, retry: false });
  const modsQuery = useQuery({ queryKey: ["global-mods"], queryFn: listGlobalMods, retry: false });
  const modPacksQuery = useQuery({ queryKey: ["mod-packs"], queryFn: listModPacks, retry: false });
  const providerKey = mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla";
  const availableVersions = versionsQuery.data?.[providerKey] ?? [];
  const selectedVersion = availableVersions.includes(version) ? version : availableVersions[0] || "";
  const allWorlds = worldsQuery.data ?? [];
  const selectedWorld = allWorlds.find((w) => w.id === selectedWorldId);
  const availableMods = modsQuery.data ?? [];
  const modPacks = modPacksQuery.data ?? [];
  const selectedModNames = availableMods.filter((m) => selectedModIds.includes(m.id)).map((m) => modDisplayName(m, locale));
  const fallbackStepKey: (typeof stepKeys)[number] = "stepReview";
  const currentStepKey = stepKeys[step] ?? fallbackStepKey;
  const nextStepKey = stepKeys[Math.min(stepKeys.length - 1, step + 1)] ?? fallbackStepKey;
  const selectedTitle = useMemo(() => t(currentStepKey), [currentStepKey, t]);
  const create = useMutation({
    mutationFn: () => createTerrariaServerWithWorld({
      config: { ...config, port: terrariaInternalPort },
      hostPort: hostPortMode === "manual" ? hostPort : undefined,
      mode,
      resources: resourceLimits,
      worldId: selectedWorldId || undefined,
      modIds: selectedModIds,
      version: selectedVersion
    }),
    onSuccess: async ({ server }) => {
      await queryClient.invalidateQueries({ queryKey: ["servers"] });
      await queryClient.invalidateQueries({ queryKey: ["worlds"] });
      await queryClient.invalidateQueries({ queryKey: ["backups"] });
      await queryClient.invalidateQueries({ queryKey: ["mods", server.id] });
      await queryClient.invalidateQueries({ queryKey: ["server", server.id] });
      queryClient.setQueryData(["server", server.id], server);
      router.push(`/servers/${server.id}`);
    }
  });
  const chooseMode = (nextMode: "vanilla" | "tmodloader") => {
    const nextPreset = nextMode === "tmodloader" ? "modded-starter" : "friends-casual";
    setMode(nextMode);
    setSelectedPreset(nextPreset);
    setConfig(getTerrariaPreset(nextPreset).config);
    setSelectedWorldId("");
    setAppliedWorldConfigId("");
    setSelectedModIds([]);
    setSelectedModPackId("");
  };
  const choosePreset = (preset: PresetKey) => {
    setSelectedPreset(preset);
    if (preset === "custom") return;
    setConfig(getTerrariaPreset(preset).config);
  };
  const chooseModPack = (packId: string) => {
    setSelectedModPackId(packId);
    const pack = modPacks.find((item) => item.id === packId);
    setSelectedModIds(pack?.modIds ?? []);
  };

  useEffect(() => {
    if (typeof window === "undefined" || selectedWorldId) return;
    const worldId = new URLSearchParams(window.location.search).get("worldId");
    if (!worldId) return;
    setSelectedWorldId(worldId);
  }, [selectedWorldId]);

  useEffect(() => {
    if (!selectedWorld || appliedWorldConfigId === selectedWorld.id) return;
    const nextMode = selectedWorld.providerKey === "terraria-tmodloader" ? "tmodloader" : "vanilla";
    const nextPreset = nextMode === "tmodloader" ? "modded-starter" : "friends-casual";
    const presetConfig = getTerrariaPreset(nextPreset).config;
    setMode(nextMode);
    setSelectedPreset("custom");
    setConfig(selectedWorld.config ? { ...presetConfig, ...selectedWorld.config } : { ...presetConfig, worldName: selectedWorld.name });
    setAppliedWorldConfigId(selectedWorld.id);
    setStep(3);
  }, [appliedWorldConfigId, selectedWorld]);

  return (
    <Card className="overflow-hidden">
      <div className="grid min-h-[640px] lg:grid-cols-[280px_1fr]">
        <aside className="hidden border-r border-panel-line bg-[linear-gradient(180deg,#111827,#07111b)] p-6 lg:block">
          <div className="overflow-hidden rounded-lg border border-panel-line bg-slate-950 shadow-[0_0_0_1px_rgba(123,217,120,0.08)]">
            <Image
              src="/images/terraria-official-cover.jpg"
              alt={t("terrariaCoverAlt")}
              width={1200}
              height={1800}
              className="aspect-[2/3] w-full object-cover"
              priority
            />
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
            {stepKeys.map((labelKey, index) => (
              <button key={labelKey} className="flex flex-col items-center gap-2 text-xs text-slate-400" onClick={() => setStep(index)}>
                <span className={cn("flex size-8 items-center justify-center rounded-full border border-panel-line", index <= step && "border-panel-green bg-panel-green text-slate-950")}>
                  {index < step ? <Check aria-hidden="true" /> : index + 1}
                </span>
                {t(labelKey)}
              </button>
            ))}
          </div>
          <motion.div key={step} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.18 }} className="mt-8">
            {step === 0 && <Choice title={t("chooseGame")} icon={<Gamepad2 />} options={["Terraria"]} />}
            {step === 1 && <ModeStep mode={mode} setMode={chooseMode} />}
            {step === 2 && <PresetStep mode={mode} selectedPreset={selectedPreset} setPreset={choosePreset} />}
            {step === 3 && (
              <ConfigStep
                config={config}
                hostPort={hostPort}
                hostPortMode={hostPortMode}
                setConfig={setConfig}
                onCustomize={() => setSelectedPreset("custom")}
                setHostPort={setHostPort}
                setHostPortMode={setHostPortMode}
                resourceLimits={resourceLimits}
                setResourceLimits={setResourceLimits}
                versions={availableVersions}
                version={selectedVersion}
                setVersion={setVersion}
              />
            )}
            {step === 4 && (
              <ModsStep
                locale={locale}
                mode={mode}
                worldName={selectedWorld?.name}
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
            {step === 5 && (
              <ReviewStep
                mode={mode}
                config={config}
                hostPortLabel={hostPortMode === "manual" ? String(hostPort) : t("automaticPort")}
                version={selectedVersion}
                resourceLimits={resourceLimits}
                selectedWorldName={selectedWorld?.name}
                selectedModNames={selectedModNames}
              />
            )}
          </motion.div>
          <div className="mt-8 flex justify-between">
            <Button variant="secondary" disabled={step === 0} onClick={() => setStep((value) => Math.max(0, value - 1))}>
              <ChevronLeft aria-hidden="true" />
              {t("back")}
            </Button>
            <Button onClick={() => step === stepKeys.length - 1 ? create.mutate() : setStep((value) => Math.min(stepKeys.length - 1, value + 1))} disabled={create.isPending}>
              {step === stepKeys.length - 1 ? create.isPending ? t("creating") : t("createServerLower") : t("nextStep", { step: t(nextStepKey) })}
              <ChevronRight aria-hidden="true" />
            </Button>
          </div>
          {create.isError && <p className="mt-4 text-sm text-red-200">{create.error.message}</p>}
          {create.data && <p className="mt-4 text-sm text-panel-green">{t("createdServer", { name: create.data.server.name })}</p>}
          <p className="mt-4 text-xs text-slate-500">{t("currentStep", { step: selectedTitle })}</p>
        </div>
      </div>
    </Card>
  );
}

function Choice({ title, icon, options }: { title: string; icon: React.ReactNode; options: string[] }) {
  return (
    <div>
      <h2 className="text-lg font-semibold">{title}</h2>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        {options.map((option) => (
          <Card key={option} className="border-panel-green p-4">
            <div className="flex items-center gap-3 text-panel-green">{icon}{option}</div>
          </Card>
        ))}
      </div>
    </div>
  );
}

function ModeStep({ mode, setMode }: { mode: "vanilla" | "tmodloader"; setMode: (mode: "vanilla" | "tmodloader") => void }) {
  const { t } = useI18n();
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

function PresetStep({
  mode,
  selectedPreset,
  setPreset
}: {
  mode: "vanilla" | "tmodloader";
  selectedPreset: PresetKey;
  setPreset: (preset: PresetKey) => void;
}) {
  const { t } = useI18n();
  const presetOptions = [customPreset, ...presets].filter((preset) => mode === "tmodloader" || preset.key !== "modded-starter");
  const renderTag = (tag: PresetTag) => {
    if (tag === "tModLoader") return tag;
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
          const isModded = presetKey === "modded-starter";
          const isCustom = presetKey === "custom";
          return (
          <button
            key={preset.key}
            type="button"
            aria-pressed={isSelected}
            onClick={() => setPreset(presetKey)}
            className={cn(
              "relative rounded-lg border p-4 text-left transition focus:outline-none focus:ring-2",
              isSelected && !isModded && !isCustom && "border-panel-green bg-panel-green/10 ring-1 ring-panel-green/40 focus:ring-panel-green/50",
              isSelected && isModded && "border-panel-green bg-panel-green/10 ring-1 ring-panel-green/40 focus:ring-panel-green/50",
              isSelected && isCustom && "border-slate-400 bg-slate-800/60 ring-1 ring-slate-500/50 focus:ring-slate-400/50",
              !isSelected && "border-panel-line bg-slate-950/40 hover:bg-slate-900/55 focus:ring-panel-green/40"
            )}
          >
            {isSelected && (
              <span className={cn("absolute right-3 top-3 flex size-6 items-center justify-center rounded-full", isModded ? "bg-panel-green text-white" : isCustom ? "bg-slate-300 text-slate-950" : "bg-panel-green text-slate-950")}>
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
  hostPort,
  hostPortMode,
  resourceLimits,
  setConfig,
  onCustomize,
  setHostPort,
  setHostPortMode,
  setResourceLimits,
  versions,
  version,
  setVersion
}: {
  config: TerrariaConfig;
  hostPort: number;
  hostPortMode: "auto" | "manual";
  resourceLimits: ResourceLimits;
  setConfig: (config: TerrariaConfig) => void;
  onCustomize: () => void;
  setHostPort: (port: number) => void;
  setHostPortMode: (mode: "auto" | "manual") => void;
  setResourceLimits: (limits: ResourceLimits) => void;
  versions: string[];
  version: string;
  setVersion: (version: string) => void;
}) {
  const { t } = useI18n();
  const preview = useMutation({
    mutationFn: () => previewTerrariaConfig(config)
  });
  const update = <K extends keyof TerrariaConfig>(key: K, value: TerrariaConfig[K]) => {
    onCustomize();
    setConfig({ ...config, [key]: value });
  };
  const updateResources = (limits: ResourceLimits) => {
    onCustomize();
    setResourceLimits(limits);
  };
  const secretSeed = secretSeedKeyFor(config.seed);
  return (
    <div>
      <h2 className="text-lg font-semibold">{t("serverConfig")}</h2>
      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <WizardField label={t("serverName")}>
          <Input value={config.serverName ?? ""} onChange={(event) => update("serverName", event.target.value)} />
        </WizardField>
        <WizardField label={t("worldName")}>
          <Input value={config.worldName} onChange={(event) => update("worldName", event.target.value)} />
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
        <WizardField label={t("maxPlayersInput")}>
          <Input type="number" min={1} max={255} value={config.maxPlayers} onChange={(event) => update("maxPlayers", Number(event.target.value))} />
        </WizardField>
        <WizardField label={t("password")}>
          <Input value={config.password ?? ""} onChange={(event) => update("password", event.target.value)} />
        </WizardField>
        <WizardField label={t("motd")}>
          <Input value={config.motd ?? ""} onChange={(event) => update("motd", event.target.value)} />
        </WizardField>
        <div className="grid gap-3 rounded-md border border-panel-line bg-slate-950/40 p-3 md:col-span-2">
          <WizardField label={t("secretSeed")}>
            <WizardSelect value={secretSeed} onChange={(value) => update("seed", value)}>
              <option value="">{t("noSecretSeed")}</option>
              {terrariaSecretSeeds.map((seed) => (
                <option key={seed.key} value={seed.key}>{seed.label} — {seed.description}</option>
              ))}
            </WizardSelect>
          </WizardField>
          <WizardField label={t("customSeed")}>
            <Input
              value={secretSeed ? "" : (config.seed ?? "")}
              placeholder={secretSeed ? t("customSeedDisabledHint") : t("customSeedPlaceholder")}
              onChange={(event) => update("seed", event.target.value)}
              disabled={Boolean(secretSeed)}
            />
          </WizardField>
        </div>
        <div className="grid gap-3 rounded-md border border-panel-line bg-slate-950/40 p-3 md:col-span-2">
          <WizardCheckbox label={t("secureMode")} checked={config.secure} onChange={(checked) => update("secure", checked)} />
          <WizardCheckbox label={t("autoCreateWorld")} checked={config.autoCreateWorld} onChange={(checked) => update("autoCreateWorld", checked)} />
        </div>
        <details className="rounded-md border border-panel-line bg-slate-950/40 p-3 md:col-span-2">
          <summary className="cursor-pointer select-none text-sm font-semibold text-slate-200 outline-none transition hover:text-panel-green focus:text-panel-green">
            {t("advancedRuntimeResources")}
          </summary>
          <p className="mt-2 text-xs text-slate-500">{t("resourceLimitsHint")}</p>
          <div className="mt-3 grid gap-3 md:grid-cols-2">
            <WizardField label={t("cpuLimit")}>
              <WizardSelect value={String(resourceLimits.cpuLimitCores)} onChange={(value) => updateResources({ ...resourceLimits, cpuLimitCores: Number(value) })}>
                {cpuLimitOptions.map((value) => (
                  <option key={value} value={value}>{formatCpuLimitLabel(value, t)}</option>
                ))}
              </WizardSelect>
            </WizardField>
            <WizardField label={t("memoryLimit")}>
              <WizardSelect value={String(resourceLimits.memoryLimitMb)} onChange={(value) => updateResources({ ...resourceLimits, memoryLimitMb: Number(value) })}>
                {memoryLimitOptions.map((value) => (
                  <option key={value} value={value}>{formatMemoryLimitLabel(value, t)}</option>
                ))}
              </WizardSelect>
            </WizardField>
          </div>
        </details>
      </div>
      <div className="mt-4 flex items-center gap-3">
        <Button variant="secondary" onClick={() => preview.mutate()} disabled={preview.isPending}>
          {preview.isPending ? t("rendering") : t("previewServerConfig")}
        </Button>
        {preview.isError && <p className="text-sm text-red-200">{preview.error.message}</p>}
      </div>
      {preview.data && (
        <pre className="mt-4 overflow-auto rounded-md border border-panel-line bg-slate-950 p-4 text-xs leading-6 text-slate-300">
          {preview.data}
        </pre>
      )}
    </div>
  );
}

function WizardField({ children, label }: { children: React.ReactNode; label: string }) {
  return (
    <label className="grid gap-1.5">
      <span className="text-xs font-medium text-slate-500">{label}</span>
      {children}
    </label>
  );
}

function WizardSelect({ children, onChange, value }: { children: React.ReactNode; onChange: (value: string) => void; value: string }) {
  return (
    <select
      className="h-10 rounded-md border border-panel-line bg-slate-950/60 px-3 text-sm text-slate-100 outline-none focus:border-panel-green"
      value={value}
      onChange={(event) => onChange(event.target.value)}
    >
      {children}
    </select>
  );
}

function WizardCheckbox({ checked, label, onChange }: { checked: boolean; label: string; onChange: (checked: boolean) => void }) {
  return (
    <label className="flex items-center justify-between gap-3 text-sm text-slate-300">
      <span>{label}</span>
      <input
        className="size-4 accent-panel-green"
        checked={checked}
        type="checkbox"
        onChange={(event) => onChange(event.target.checked)}
      />
    </label>
  );
}

function ModsStep({
  locale,
  mode,
  worldName,
  mods,
  modPacks,
  selectedModPackId,
  selectedModIds,
  onSelectModPack,
  onToggleMod
}: {
  locale: string;
  mode: "vanilla" | "tmodloader";
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

      {mode !== "tmodloader" ? (
        <div className="mt-6 flex flex-col items-center justify-center rounded-lg border border-dashed border-panel-line bg-slate-950/30 py-12 text-center">
          <Package aria-hidden="true" className="size-8 text-slate-600" />
          <p className="mt-3 text-sm text-slate-400">{t("noModsForVanilla")}</p>
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
  mode,
  config,
  hostPortLabel,
  resourceLimits,
  version,
  selectedWorldName,
  selectedModNames
}: {
  mode: "vanilla" | "tmodloader";
  config: TerrariaConfig;
  hostPortLabel: string;
  resourceLimits: ResourceLimits;
  version: string;
  selectedWorldName?: string;
  selectedModNames: string[];
}) {
  const { t } = useI18n();
  return (
    <div>
      <h2 className="text-lg font-semibold">{t("review")}</h2>
      <Card className="mt-4 p-4">
        <div className="flex items-center gap-3"><Settings2 aria-hidden="true" /> {t("reviewSummary", { mode: mode === "tmodloader" ? "tModLoader" : t("modeVanilla"), port: hostPortLabel })}</div>
        <p className="mt-3 text-sm text-slate-400">{t("reviewWorldPlayers", { world: config.worldName, players: config.maxPlayers })}</p>
        <p className="mt-2 text-sm text-slate-400">
          {t("resourceLimits")}: <span className="text-slate-200">{formatCpuLimitLabel(resourceLimits.cpuLimitCores, t)} · {formatMemoryLimitLabel(resourceLimits.memoryLimitMb, t)}</span>
        </p>
        {version && <p className="mt-2 text-sm text-slate-400">{t("gameVersion")}: <span className="text-slate-200">{version}</span></p>}
        {selectedWorldName && (
          <div className="mt-4 rounded-md border border-panel-line bg-slate-950/60 p-3 text-sm text-slate-300">
            <p>{t("selectedWorldFile")}: <span className="text-panel-green">{selectedWorldName}</span></p>
          </div>
        )}
        {mode === "tmodloader" && selectedModNames.length > 0 && (
          <div className="mt-2 rounded-md border border-panel-line bg-slate-950/60 p-3 text-sm text-slate-300">
            <p>{t("selectedModFiles")}: <span className="text-panel-green">{selectedModNames.join(", ")}</span></p>
          </div>
        )}
      </Card>
    </div>
  );
}
