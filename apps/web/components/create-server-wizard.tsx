"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronLeft, ChevronRight, FileArchive, Gamepad2, Globe, Hammer, Package, Settings2, X } from "lucide-react";
import Image from "next/image";
import { useMemo, useState } from "react";
import { Button, Card, Input } from "@/components/ui";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { getTerrariaVersions, listGlobalMods, listWorlds, previewTerrariaConfig } from "@/lib/api";
import { createTerrariaServerWithWorld } from "@/lib/create-server-flow";
import { getTerrariaPreset, secretSeedKeyFor, terrariaSecretSeeds, type TerrariaConfig } from "@gamepanel-lite/shared";

const stepKeys = ["stepGame", "stepMode", "stepPreset", "stepConfig", "stepWorldMods", "stepReview"] as const;
const presets = [
  { key: "friends-casual", labelKey: "presetFriendsCasual", descriptionKey: "presetFriendsCasualDescription", tags: ["tagClassic", "tagMediumWorld", "8"] },
  { key: "building-world", labelKey: "presetBuildingWorld", descriptionKey: "presetBuildingWorldDescription", tags: ["tagClassic", "tagLargeWorld", "12"] },
  { key: "expert-adventure", labelKey: "presetExpertAdventure", descriptionKey: "presetExpertAdventureDescription", tags: ["tagExpert", "tagLargeWorld", "8"] },
  { key: "modded-starter", labelKey: "presetModdedStarter", descriptionKey: "presetModdedStarterDescription", tags: ["tModLoader", "tagMediumWorld", "8"] },
  { key: "master-challenge", labelKey: "presetMasterChallenge", descriptionKey: "presetMasterChallengeDescription", tags: ["tagMaster", "tagLargeWorld", "6"] }
] as const;

type PresetKey = (typeof presets)[number]["key"];

export function CreateServerWizard() {
  const { t } = useI18n();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [step, setStep] = useState(0);
  const [mode, setMode] = useState<"vanilla" | "tmodloader">("tmodloader");
  const [selectedPreset, setSelectedPreset] = useState<PresetKey>("modded-starter");
  const [config, setConfig] = useState<TerrariaConfig>(getTerrariaPreset("modded-starter").config);
  const [version, setVersion] = useState("");
  const [selectedWorldId, setSelectedWorldId] = useState("");
  const [selectedModIds, setSelectedModIds] = useState<string[]>([]);
  const versionsQuery = useQuery({ queryKey: ["terraria-versions"], queryFn: getTerrariaVersions, staleTime: 5 * 60 * 1000 });
  const worldsQuery = useQuery({ queryKey: ["worlds"], queryFn: listWorlds, retry: false });
  const modsQuery = useQuery({ queryKey: ["global-mods"], queryFn: listGlobalMods, retry: false });
  const providerKey = mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla";
  const availableVersions = versionsQuery.data?.[providerKey] ?? [];
  const selectedVersion = availableVersions.includes(version) ? version : availableVersions[0] || "";
  const availableWorlds = worldsQuery.data ?? [];
  const selectedWorld = availableWorlds.find((w) => w.id === selectedWorldId);
  const availableMods = modsQuery.data ?? [];
  const selectedModNames = availableMods.filter((m) => selectedModIds.includes(m.id)).map((m) => m.fileName);
  const fallbackStepKey: (typeof stepKeys)[number] = "stepReview";
  const currentStepKey = stepKeys[step] ?? fallbackStepKey;
  const nextStepKey = stepKeys[Math.min(stepKeys.length - 1, step + 1)] ?? fallbackStepKey;
  const selectedTitle = useMemo(() => t(currentStepKey), [currentStepKey, t]);
  const create = useMutation({
    mutationFn: () => createTerrariaServerWithWorld({ config, mode, worldId: selectedWorldId || undefined, modIds: selectedModIds, version: selectedVersion }),
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
  };
  const choosePreset = (preset: PresetKey) => {
    setSelectedPreset(preset);
    setConfig(getTerrariaPreset(preset).config);
  };

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
            {step === 3 && <ConfigStep config={config} setConfig={setConfig} versions={availableVersions} version={selectedVersion} setVersion={setVersion} />}
            {step === 4 && (
              <WorldModsStep
                mode={mode}
                worlds={availableWorlds}
                selectedWorldId={selectedWorldId}
                onSelectWorld={setSelectedWorldId}
                mods={availableMods}
                selectedModIds={selectedModIds}
                onToggleMod={(modId) => setSelectedModIds((current) => current.includes(modId) ? current.filter((id) => id !== modId) : [...current, modId])}
              />
            )}
            {step === 5 && <ReviewStep mode={mode} config={config} version={selectedVersion} selectedWorldName={selectedWorld?.name} selectedModNames={selectedModNames} />}
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
            "relative rounded-lg border p-4 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-purple/50",
            mode === "tmodloader"
              ? "border-panel-purple bg-panel-purple/10 ring-1 ring-panel-purple/40"
              : "border-panel-line bg-slate-950/40 hover:border-panel-purple/70 hover:bg-slate-900/55"
          )}
        >
          {mode === "tmodloader" && (
            <span className="absolute right-3 top-3 flex size-6 items-center justify-center rounded-full bg-panel-purple text-white">
              <Check aria-hidden="true" className="size-4" />
            </span>
          )}
          <Package aria-hidden="true" className="text-panel-purple" />
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
  const renderTag = (tag: (typeof presets)[number]["tags"][number]) => {
    if (tag === "tModLoader") return tag;
    if (tag === "6" || tag === "8" || tag === "12") return t("tagPlayers", { count: tag });
    return t(tag);
  };
  return (
    <div>
      <h2 className="text-lg font-semibold">{t("choosePreset")}</h2>
      <p className="mt-1 text-sm text-slate-400">{t("presetDescription")}</p>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        {presets.filter((preset) => mode === "tmodloader" || preset.key !== "modded-starter").map((preset) => {
          const presetKey = preset.key as PresetKey;
          const isSelected = selectedPreset === presetKey;
          const isModded = presetKey === "modded-starter";
          return (
          <button
            key={preset.key}
            type="button"
            aria-pressed={isSelected}
            onClick={() => setPreset(presetKey)}
            className={cn(
              "relative rounded-lg border p-4 text-left transition focus:outline-none focus:ring-2",
              isSelected && !isModded && "border-panel-green bg-panel-green/10 ring-1 ring-panel-green/40 focus:ring-panel-green/50",
              isSelected && isModded && "border-panel-purple bg-panel-purple/10 ring-1 ring-panel-purple/40 focus:ring-panel-purple/50",
              !isSelected && "border-panel-line bg-slate-950/40 hover:bg-slate-900/55 focus:ring-panel-green/40"
            )}
          >
            {isSelected && (
              <span className={cn("absolute right-3 top-3 flex size-6 items-center justify-center rounded-full", isModded ? "bg-panel-purple text-white" : "bg-panel-green text-slate-950")}>
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

function ConfigStep({ config, setConfig, versions, version, setVersion }: { config: TerrariaConfig; setConfig: (config: TerrariaConfig) => void; versions: string[]; version: string; setVersion: (version: string) => void }) {
  const { t } = useI18n();
  const preview = useMutation({
    mutationFn: () => previewTerrariaConfig(config)
  });
  const update = <K extends keyof TerrariaConfig>(key: K, value: TerrariaConfig[K]) => setConfig({ ...config, [key]: value });
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
        <WizardField label={t("port")}>
          <Input type="number" min={1024} max={65535} value={config.port} onChange={(event) => update("port", Number(event.target.value))} />
        </WizardField>
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

function WorldModsStep({
  mode,
  worlds,
  selectedWorldId,
  onSelectWorld,
  mods,
  selectedModIds,
  onToggleMod
}: {
  mode: "vanilla" | "tmodloader";
  worlds: { id: string; name: string; modified: string; bytes: string }[];
  selectedWorldId: string;
  onSelectWorld: (id: string) => void;
  mods: { id: string; fileName: string; size: string }[];
  selectedModIds: string[];
  onToggleMod: (modId: string) => void;
}) {
  const { t } = useI18n();
  return (
    <div>
      <h2 className="text-lg font-semibold">{t("selectWorld")}</h2>
      <p className="mt-1 text-sm text-slate-400">{t("selectWorldHint")}</p>
      <div className="mt-4 space-y-3">
        <button
          type="button"
          onClick={() => onSelectWorld("")}
          className={cn(
            "flex w-full items-center gap-3 rounded-lg border p-4 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
            selectedWorldId === ""
              ? "border-panel-green bg-panel-green/10 ring-1 ring-panel-green/40"
              : "border-panel-line bg-slate-950/40 hover:bg-slate-900/55"
          )}
        >
          <span className={cn("flex size-5 items-center justify-center rounded-full border", selectedWorldId === "" ? "border-panel-green bg-panel-green" : "border-slate-600")}>
            {selectedWorldId === "" && <Check aria-hidden="true" className="size-3 text-slate-950" />}
          </span>
          <Globe aria-hidden="true" className="text-panel-green" />
          <div>
            <p className="font-medium">{t("autoCreateWorld")}</p>
            <p className="mt-0.5 text-sm text-slate-400">{t("autoCreateWorldHint")}</p>
          </div>
        </button>
        {worlds.length > 0 && (
          <div className="space-y-2">
            <p className="px-1 text-xs font-medium text-slate-500">{t("importedWorlds")}</p>
            {worlds.map((world) => (
              <button
                key={world.id}
                type="button"
                onClick={() => onSelectWorld(world.id)}
                className={cn(
                  "flex w-full items-center gap-3 rounded-lg border p-4 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-green/50",
                  selectedWorldId === world.id
                    ? "border-panel-green bg-panel-green/10 ring-1 ring-panel-green/40"
                    : "border-panel-line bg-slate-950/40 hover:bg-slate-900/55"
                )}
              >
                <span className={cn("flex size-5 shrink-0 items-center justify-center rounded-full border", selectedWorldId === world.id ? "border-panel-green bg-panel-green" : "border-slate-600")}>
                  {selectedWorldId === world.id && <Check aria-hidden="true" className="size-3 text-slate-950" />}
                </span>
                <div className="min-w-0">
                  <p className="truncate font-medium">{world.name}</p>
                  <p className="mt-0.5 text-sm text-slate-400">{world.modified} · {world.bytes}</p>
                </div>
              </button>
            ))}
          </div>
        )}
        <Link href="/worlds" className="inline-flex items-center gap-2 text-sm text-panel-green hover:underline">
          <FileArchive aria-hidden="true" className="size-4" />
          {t("goToWorldsPage")}
        </Link>
      </div>
      {mode === "tmodloader" && (
        <div className="mt-6 border-t border-panel-line pt-6">
          <h2 className="text-lg font-semibold">{t("selectMods")}</h2>
          <p className="mt-1 text-sm text-slate-400">{t("selectModsHint")}</p>
          <div className="mt-4 space-y-2">
            {mods.length === 0 ? (
              <Link href="/mods" className="inline-flex items-center gap-2 text-sm text-panel-purple hover:underline">
                <Package aria-hidden="true" className="size-4" />
                {t("goToModsPage")}
              </Link>
            ) : (
              <>
                {mods.map((mod) => (
                  <button
                    key={mod.id}
                    type="button"
                    onClick={() => onToggleMod(mod.id)}
                    className={cn(
                      "flex w-full items-center gap-3 rounded-lg border p-3 text-left transition focus:outline-none focus:ring-2 focus:ring-panel-purple/50",
                      selectedModIds.includes(mod.id)
                        ? "border-panel-purple bg-panel-purple/10 ring-1 ring-panel-purple/40"
                        : "border-panel-line bg-slate-950/40 hover:bg-slate-900/55"
                    )}
                  >
                    <span className={cn("flex size-5 shrink-0 items-center justify-center rounded border", selectedModIds.includes(mod.id) ? "border-panel-purple bg-panel-purple text-white" : "border-slate-600")}>
                      {selectedModIds.includes(mod.id) && <Check aria-hidden="true" className="size-3" />}
                    </span>
                    <div className="min-w-0">
                      <p className="truncate text-sm font-medium">{mod.fileName}</p>
                      <p className="mt-0.5 text-xs text-slate-500">{mod.size}</p>
                    </div>
                  </button>
                ))}
                <Link href="/mods" className="inline-flex items-center gap-2 pt-1 text-sm text-panel-purple hover:underline">
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
  version,
  selectedWorldName,
  selectedModNames
}: {
  mode: "vanilla" | "tmodloader";
  config: TerrariaConfig;
  version: string;
  selectedWorldName?: string;
  selectedModNames: string[];
}) {
  const { t } = useI18n();
  return (
    <div>
      <h2 className="text-lg font-semibold">{t("review")}</h2>
      <Card className="mt-4 p-4">
        <div className="flex items-center gap-3"><Settings2 aria-hidden="true" /> {t("reviewSummary", { mode: mode === "tmodloader" ? "tModLoader" : t("modeVanilla"), port: config.port })}</div>
        <p className="mt-3 text-sm text-slate-400">{t("reviewWorldPlayers", { world: config.worldName, players: config.maxPlayers })}</p>
        {version && <p className="mt-2 text-sm text-slate-400">{t("gameVersion")}: <span className="text-slate-200">{version}</span></p>}
        {selectedWorldName && (
          <div className="mt-4 rounded-md border border-panel-line bg-slate-950/60 p-3 text-sm text-slate-300">
            <p>{t("selectedWorldFile")}: <span className="text-panel-green">{selectedWorldName}</span></p>
          </div>
        )}
        {mode === "tmodloader" && selectedModNames.length > 0 && (
          <div className="mt-2 rounded-md border border-panel-line bg-slate-950/60 p-3 text-sm text-slate-300">
            <p>{t("selectedModFiles")}: <span className="text-panel-purple">{selectedModNames.join(", ")}</span></p>
          </div>
        )}
      </Card>
    </div>
  );
}
