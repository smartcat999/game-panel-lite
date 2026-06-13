"use client";

import { motion } from "framer-motion";
import { useMutation } from "@tanstack/react-query";
import { Check, ChevronLeft, ChevronRight, Gamepad2, Hammer, Package, Settings2 } from "lucide-react";
import { useMemo, useState } from "react";
import { Button, Card, Input } from "@/components/ui";
import { cn } from "@/lib/utils";
import { previewTerrariaConfig } from "@/lib/api";
import { createServer } from "@/lib/api";
import { getTerrariaPreset, type TerrariaConfig } from "@gamepanel-lite/shared";

const steps = ["Game", "Mode", "Preset", "Config", "World / Mods", "Review"];
const presets = [
  { key: "friends-casual", label: "Friends Casual", description: "Perfect for casual play with friends.", tags: ["Classic", "Medium World", "8 Players"] },
  { key: "building-world", label: "Building World", description: "Great for building and creativity.", tags: ["Classic", "Large World", "8 Players"] },
  { key: "expert-adventure", label: "Expert Adventure", description: "For experienced players looking for a challenge.", tags: ["Expert", "Medium World", "8 Players"] },
  { key: "modded-starter", label: "Modded Starter", description: "Start your modded adventure.", tags: ["tModLoader", "Medium World", "10 Players"] },
  { key: "master-challenge", label: "Master Challenge", description: "The ultimate challenge for veteran players.", tags: ["Master", "Medium World", "6 Players"] }
] as const;

type PresetKey = (typeof presets)[number]["key"];

export function CreateServerWizard() {
  const [step, setStep] = useState(2);
  const [mode, setMode] = useState<"vanilla" | "tmodloader">("tmodloader");
  const [config, setConfig] = useState<TerrariaConfig>(getTerrariaPreset("modded-starter").config);
  const selectedTitle = useMemo(() => steps[step], [step]);
  const create = useMutation({
    mutationFn: () => createServer({
      name: config.serverName || "Terraria Server",
      providerKey: mode === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla",
      config
    })
  });
  const chooseMode = (nextMode: "vanilla" | "tmodloader") => {
    setMode(nextMode);
    setConfig(getTerrariaPreset(nextMode === "tmodloader" ? "modded-starter" : "friends-casual").config);
  };

  return (
    <Card className="overflow-hidden">
      <div className="grid min-h-[640px] lg:grid-cols-[280px_1fr]">
        <aside className="hidden border-r border-panel-line bg-[linear-gradient(180deg,#111827,#07111b)] p-6 lg:flex lg:flex-col lg:justify-end">
          <div className="h-72 rounded-lg bg-[linear-gradient(180deg,#17365d,#102217_60%,#3a2818)]" />
        </aside>
        <div className="p-6">
          <h1 className="text-2xl font-semibold">Create Terraria Server</h1>
          <div className="mt-7 grid grid-cols-3 gap-3 md:grid-cols-6">
            {steps.map((label, index) => (
              <button key={label} className="flex flex-col items-center gap-2 text-xs text-slate-400" onClick={() => setStep(index)}>
                <span className={cn("flex size-8 items-center justify-center rounded-full border border-panel-line", index <= step && "border-panel-green bg-panel-green text-slate-950")}>
                  {index < step ? <Check aria-hidden="true" /> : index + 1}
                </span>
                {label}
              </button>
            ))}
          </div>
          <motion.div key={step} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.18 }} className="mt-8">
            {step === 0 && <Choice title="Choose a Game" icon={<Gamepad2 />} options={["Terraria"]} />}
            {step === 1 && <ModeStep mode={mode} setMode={chooseMode} />}
            {step === 2 && <PresetStep mode={mode} setConfig={setConfig} />}
            {step === 3 && <ConfigStep config={config} setConfig={setConfig} />}
            {step === 4 && <WorldModsStep mode={mode} />}
            {step === 5 && <ReviewStep mode={mode} config={config} />}
          </motion.div>
          <div className="mt-8 flex justify-between">
            <Button variant="secondary" disabled={step === 0} onClick={() => setStep((value) => Math.max(0, value - 1))}>
              <ChevronLeft aria-hidden="true" />
              Back
            </Button>
            <Button onClick={() => step === steps.length - 1 ? create.mutate() : setStep((value) => Math.min(steps.length - 1, value + 1))} disabled={create.isPending}>
              {step === steps.length - 1 ? create.isPending ? "Creating..." : "Create server" : `Next: ${steps[Math.min(steps.length - 1, step + 1)]}`}
              <ChevronRight aria-hidden="true" />
            </Button>
          </div>
          {create.isError && <p className="mt-4 text-sm text-red-200">{create.error.message}</p>}
          {create.data && <p className="mt-4 text-sm text-panel-green">Created {create.data.name}. Open Servers to manage it.</p>}
          <p className="mt-4 text-xs text-slate-500">Current step: {selectedTitle}</p>
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
  return (
    <div>
      <h2 className="text-lg font-semibold">Choose server mode</h2>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <button onClick={() => setMode("vanilla")} className={cn("rounded-lg border border-panel-line bg-slate-950/40 p-4 text-left", mode === "vanilla" && "border-panel-green")}>
          <Hammer aria-hidden="true" className="text-panel-green" />
          <p className="mt-3 font-medium">Vanilla Terraria</p>
          <p className="mt-1 text-sm text-slate-400">Official server flow with clean world setup.</p>
        </button>
        <button onClick={() => setMode("tmodloader")} className={cn("rounded-lg border border-panel-line bg-slate-950/40 p-4 text-left", mode === "tmodloader" && "border-panel-purple")}>
          <Package aria-hidden="true" className="text-panel-purple" />
          <p className="mt-3 font-medium">tModLoader</p>
          <p className="mt-1 text-sm text-slate-400">Modded Terraria with uploads enabled.</p>
        </button>
      </div>
    </div>
  );
}

function PresetStep({ mode, setConfig }: { mode: "vanilla" | "tmodloader"; setConfig: (config: TerrariaConfig) => void }) {
  return (
    <div>
      <h2 className="text-lg font-semibold">Choose a Preset</h2>
      <p className="mt-1 text-sm text-slate-400">Start with a template and customize it later.</p>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        {presets.filter((preset) => mode === "tmodloader" || preset.key !== "modded-starter").map((preset) => (
          <button key={preset.key} onClick={() => setConfig(getTerrariaPreset(preset.key as PresetKey).config)} className={cn("rounded-lg border border-panel-line bg-slate-950/40 p-4 text-left hover:border-panel-green", preset.key === "modded-starter" && "border-panel-green")}>
            <p className="font-medium">{preset.label}</p>
            <p className="mt-1 text-sm text-slate-400">{preset.description}</p>
            <div className="mt-4 flex flex-wrap gap-2">
              {preset.tags.map((tag) => <span key={tag} className="rounded bg-slate-800 px-2 py-1 text-xs text-slate-300">{tag}</span>)}
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

function ConfigStep({ config, setConfig }: { config: TerrariaConfig; setConfig: (config: TerrariaConfig) => void }) {
  const preview = useMutation({
    mutationFn: () => previewTerrariaConfig(config)
  });
  const update = <K extends keyof TerrariaConfig>(key: K, value: TerrariaConfig[K]) => setConfig({ ...config, [key]: value });
  return (
    <div>
      <h2 className="text-lg font-semibold">Server config</h2>
      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <Input placeholder="Server name" value={config.serverName ?? ""} onChange={(event) => update("serverName", event.target.value)} />
        <Input placeholder="World name" value={config.worldName} onChange={(event) => update("worldName", event.target.value)} />
        <Input placeholder="Port" value={config.port} onChange={(event) => update("port", Number(event.target.value))} />
        <Input placeholder="Max players" value={config.maxPlayers} onChange={(event) => update("maxPlayers", Number(event.target.value))} />
        <Input placeholder="MOTD" value={config.motd ?? ""} onChange={(event) => update("motd", event.target.value)} />
        <Input placeholder="Password" value={config.password ?? ""} onChange={(event) => update("password", event.target.value)} />
      </div>
      <div className="mt-4 flex items-center gap-3">
        <Button variant="secondary" onClick={() => preview.mutate()} disabled={preview.isPending}>
          {preview.isPending ? "Rendering..." : "Preview serverconfig.txt"}
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

function WorldModsStep({ mode }: { mode: "vanilla" | "tmodloader" }) {
  return (
    <div>
      <h2 className="text-lg font-semibold">World {mode === "tmodloader" ? "and mods" : ""}</h2>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        <Card className="p-4">Import `.wld` world file</Card>
        {mode === "tmodloader" && <Card className="border-panel-purple p-4">Upload `.tmod`, `install.txt`, or `enabled.json`</Card>}
      </div>
    </div>
  );
}

function ReviewStep({ mode, config }: { mode: "vanilla" | "tmodloader"; config: TerrariaConfig }) {
  return (
    <div>
      <h2 className="text-lg font-semibold">Review</h2>
      <Card className="mt-4 p-4">
        <div className="flex items-center gap-3"><Settings2 aria-hidden="true" /> Terraria {mode === "tmodloader" ? "tModLoader" : "Vanilla"} server on port {config.port}</div>
        <p className="mt-3 text-sm text-slate-400">World: {config.worldName} · Players: {config.maxPlayers}</p>
      </Card>
    </div>
  );
}
