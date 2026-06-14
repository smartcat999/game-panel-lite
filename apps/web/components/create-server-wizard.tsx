"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { motion } from "framer-motion";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronLeft, ChevronRight, FileArchive, FileUp, Gamepad2, Hammer, Package, Settings2, X } from "lucide-react";
import Image from "next/image";
import { useMemo, useState } from "react";
import { Button, Card, Input } from "@/components/ui";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { createServer, importWorld, previewTerrariaConfig, uploadMod } from "@/lib/api";
import { getTerrariaPreset, type TerrariaConfig } from "@gamepanel-lite/shared";

const stepKeys = ["stepGame", "stepMode", "stepPreset", "stepConfig", "stepWorldMods", "stepReview"] as const;
const presets = [
  { key: "friends-casual", labelKey: "presetFriendsCasual", descriptionKey: "presetFriendsCasualDescription", tags: ["tagClassic", "tagMediumWorld", "8"] },
  { key: "building-world", labelKey: "presetBuildingWorld", descriptionKey: "presetBuildingWorldDescription", tags: ["tagClassic", "tagLargeWorld", "8"] },
  { key: "expert-adventure", labelKey: "presetExpertAdventure", descriptionKey: "presetExpertAdventureDescription", tags: ["tagExpert", "tagMediumWorld", "8"] },
  { key: "modded-starter", labelKey: "presetModdedStarter", descriptionKey: "presetModdedStarterDescription", tags: ["tModLoader", "tagMediumWorld", "10"] },
  { key: "master-challenge", labelKey: "presetMasterChallenge", descriptionKey: "presetMasterChallengeDescription", tags: ["tagMaster", "tagMediumWorld", "6"] }
] as const;

type PresetKey = (typeof presets)[number]["key"];

export function CreateServerWizard() {
  const { t } = useI18n();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [step, setStep] = useState(2);
  const [mode, setMode] = useState<"vanilla" | "tmodloader">("tmodloader");
  const [selectedPreset, setSelectedPreset] = useState<PresetKey>("modded-starter");
  const [config, setConfig] = useState<TerrariaConfig>(getTerrariaPreset("modded-starter").config);
  const [worldFile, setWorldFile] = useState<File | null>(null);
  const [modFiles, setModFiles] = useState<File[]>([]);
  const worldFileName = worldFile?.name ?? "";
  const modFileNames = modFiles.map((file) => file.name);
  const fallbackStepKey: (typeof stepKeys)[number] = "stepReview";
  const currentStepKey = stepKeys[step] ?? fallbackStepKey;
  const nextStepKey = stepKeys[Math.min(stepKeys.length - 1, step + 1)] ?? fallbackStepKey;
  const selectedTitle = useMemo(() => t(currentStepKey), [currentStepKey, t]);
  const create = useMutation({
    mutationFn: async () => {
      const server = await createServer({
        name: config.serverName || "Terraria Server",
        providerKey: mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla",
        config
      });
      if (worldFile) {
        await importWorld(worldFile, server.id);
      }
      if (mode === "tmodloader" && modFiles.length > 0) {
        await Promise.all(modFiles.map((file) => uploadMod(server.id, file)));
      }
      return server;
    },
    onSuccess: async (server) => {
      await queryClient.invalidateQueries({ queryKey: ["servers"] });
      await queryClient.invalidateQueries({ queryKey: ["worlds"] });
      await queryClient.invalidateQueries({ queryKey: ["mods", server.id] });
      queryClient.setQueryData(["server", server.id], server);
      router.push(`/servers/${server.id}`);
    }
  });
  const chooseMode = (nextMode: "vanilla" | "tmodloader") => {
    const nextPreset = nextMode === "tmodloader" ? "modded-starter" : "friends-casual";
    setMode(nextMode);
    setSelectedPreset(nextPreset);
    setConfig(getTerrariaPreset(nextPreset).config);
    if (nextMode === "vanilla") {
      setModFiles([]);
    }
  };
  const choosePreset = (preset: PresetKey) => {
    setSelectedPreset(preset);
    setConfig(getTerrariaPreset(preset).config);
  };

  return (
    <Card className="overflow-hidden">
      <div className="grid min-h-[640px] lg:grid-cols-[280px_1fr]">
        <aside className="hidden border-r border-panel-line bg-[linear-gradient(180deg,#111827,#07111b)] p-6 lg:flex lg:flex-col lg:justify-end">
          <div className="overflow-hidden rounded-lg border border-panel-line bg-slate-950 shadow-[0_0_0_1px_rgba(123,217,120,0.08)]">
            <Image
              src="/images/terraria-official-cover.jpg"
              alt={t("terrariaCoverAlt")}
              width={1200}
              height={1800}
              className="h-72 w-full object-cover"
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
            {step === 3 && <ConfigStep config={config} setConfig={setConfig} />}
            {step === 4 && (
              <WorldModsStep
                mode={mode}
                worldFileName={worldFileName}
                modFileNames={modFileNames}
                setWorldFile={setWorldFile}
                setModFiles={setModFiles}
              />
            )}
            {step === 5 && <ReviewStep mode={mode} config={config} worldFileName={worldFileName} modFileNames={modFileNames} />}
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
          {create.data && <p className="mt-4 text-sm text-panel-green">{t("createdServer", { name: create.data.name })}</p>}
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
    if (tag === "6" || tag === "8" || tag === "10") return t("tagPlayers", { count: tag });
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

function ConfigStep({ config, setConfig }: { config: TerrariaConfig; setConfig: (config: TerrariaConfig) => void }) {
  const { t } = useI18n();
  const preview = useMutation({
    mutationFn: () => previewTerrariaConfig(config)
  });
  const update = <K extends keyof TerrariaConfig>(key: K, value: TerrariaConfig[K]) => setConfig({ ...config, [key]: value });
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
        <WizardField label={t("difficulty")}>
          <WizardSelect value={config.difficulty} onChange={(value) => update("difficulty", value as TerrariaConfig["difficulty"])}>
            <option value="journey">{t("tagJourney")}</option>
            <option value="classic">{t("tagClassic")}</option>
            <option value="expert">{t("tagExpert")}</option>
            <option value="master">{t("tagMaster")}</option>
          </WizardSelect>
        </WizardField>
        <WizardField label={t("port")}>
          <Input type="number" min={1024} max={65535} value={config.port} onChange={(event) => update("port", Number(event.target.value))} />
        </WizardField>
        <WizardField label={t("maxPlayersInput")}>
          <Input type="number" min={1} max={255} value={config.maxPlayers} onChange={(event) => update("maxPlayers", Number(event.target.value))} />
        </WizardField>
        <WizardField label={t("motd")}>
          <Input value={config.motd ?? ""} onChange={(event) => update("motd", event.target.value)} />
        </WizardField>
        <WizardField label={t("password")}>
          <Input value={config.password ?? ""} onChange={(event) => update("password", event.target.value)} />
        </WizardField>
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
  worldFileName,
  modFileNames,
  setWorldFile,
  setModFiles
}: {
  mode: "vanilla" | "tmodloader";
  worldFileName: string;
  modFileNames: string[];
  setWorldFile: (file: File | null) => void;
  setModFiles: (files: File[]) => void;
}) {
  const { t } = useI18n();
  return (
    <div>
      <h2 className="text-lg font-semibold">{mode === "tmodloader" ? t("worldAndMods") : t("worldOnly")}</h2>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <label className="cursor-pointer rounded-lg border border-panel-line bg-slate-950/40 p-4 text-left transition hover:border-panel-green focus-within:border-panel-green">
          <input
            className="sr-only"
            type="file"
            accept=".wld"
            onChange={(event) => setWorldFile(event.target.files?.[0] ?? null)}
          />
          <span className="flex items-center gap-3 font-medium text-white">
            <FileUp aria-hidden="true" className="text-panel-green" />
            {t("importWldFile")}
          </span>
          <span className={cn("mt-3 block truncate text-sm", worldFileName ? "text-panel-green" : "text-slate-400")}>
            {worldFileName ? t("selectedFile", { name: worldFileName }) : t("clickToChooseFile")}
          </span>
        </label>
        {mode === "tmodloader" && (
          <label className="cursor-pointer rounded-lg border border-panel-purple bg-slate-950/40 p-4 text-left transition hover:border-panel-purple/80 focus-within:border-panel-purple">
            <input
              className="sr-only"
              type="file"
              accept=".tmod,.txt,.json"
              multiple
              onChange={(event) => setModFiles(Array.from(event.target.files ?? []))}
            />
            <span className="flex items-center gap-3 font-medium text-white">
              <FileArchive aria-hidden="true" className="text-panel-purple" />
              {t("uploadModFiles")}
            </span>
            <span className={cn("mt-3 block truncate text-sm", modFileNames.length ? "text-panel-purple" : "text-slate-400")}>
              {modFileNames.length ? t("selectedFiles", { count: modFileNames.length }) : t("clickToChooseFile")}
            </span>
            {modFileNames.length > 0 && (
              <span className="mt-2 block truncate text-xs text-slate-500">{modFileNames.join(", ")}</span>
            )}
          </label>
        )}
      </div>
      {(worldFileName || modFileNames.length > 0) && (
        <p className="mt-3 text-xs text-slate-500">{t("wizardUploadDeferredNote")}</p>
      )}
    </div>
  );
}

function ReviewStep({
  mode,
  config,
  worldFileName,
  modFileNames
}: {
  mode: "vanilla" | "tmodloader";
  config: TerrariaConfig;
  worldFileName: string;
  modFileNames: string[];
}) {
  const { t } = useI18n();
  return (
    <div>
      <h2 className="text-lg font-semibold">{t("review")}</h2>
      <Card className="mt-4 p-4">
        <div className="flex items-center gap-3"><Settings2 aria-hidden="true" /> {t("reviewSummary", { mode: mode === "tmodloader" ? "tModLoader" : t("modeVanilla"), port: config.port })}</div>
        <p className="mt-3 text-sm text-slate-400">{t("reviewWorldPlayers", { world: config.worldName, players: config.maxPlayers })}</p>
        {(worldFileName || modFileNames.length > 0) && (
          <div className="mt-4 rounded-md border border-panel-line bg-slate-950/60 p-3 text-sm text-slate-300">
            {worldFileName && <p>{t("selectedWorldFile")}: <span className="text-panel-green">{worldFileName}</span></p>}
            {modFileNames.length > 0 && <p className="mt-1">{t("selectedModFiles")}: <span className="text-panel-purple">{modFileNames.join(", ")}</span></p>}
          </div>
        )}
      </Card>
    </div>
  );
}
