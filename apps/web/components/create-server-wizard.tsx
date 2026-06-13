"use client";

import { motion } from "framer-motion";
import { Check, ChevronLeft, ChevronRight, Gamepad2, Hammer, Package, Settings2 } from "lucide-react";
import { useMemo, useState } from "react";
import { Button, Card, Input } from "./ui";
import { cn } from "@/lib/utils";

const steps = ["Game", "Mode", "Preset", "Config", "World / Mods", "Review"];
const presets = [
  ["Friends Casual", "Perfect for casual play with friends.", "Classic", "Medium World", "8 Players"],
  ["Building World", "Great for building and creativity.", "Classic", "Large World", "8 Players"],
  ["Expert Adventure", "For experienced players looking for a challenge.", "Expert", "Medium World", "8 Players"],
  ["Modded Starter", "Start your modded adventure.", "tModLoader", "Medium World", "10 Players"],
  ["Master Challenge", "The ultimate challenge for veteran players.", "Master", "Medium World", "6 Players"]
];

export function CreateServerWizard() {
  const [step, setStep] = useState(2);
  const [mode, setMode] = useState<"vanilla" | "tmodloader">("tmodloader");
  const selectedTitle = useMemo(() => steps[step], [step]);

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
            {step === 1 && <ModeStep mode={mode} setMode={setMode} />}
            {step === 2 && <PresetStep mode={mode} />}
            {step === 3 && <ConfigStep />}
            {step === 4 && <WorldModsStep mode={mode} />}
            {step === 5 && <ReviewStep mode={mode} />}
          </motion.div>
          <div className="mt-8 flex justify-between">
            <Button variant="secondary" disabled={step === 0} onClick={() => setStep((value) => Math.max(0, value - 1))}>
              <ChevronLeft aria-hidden="true" />
              Back
            </Button>
            <Button onClick={() => setStep((value) => Math.min(steps.length - 1, value + 1))}>
              {step === steps.length - 1 ? "Create server" : `Next: ${steps[Math.min(steps.length - 1, step + 1)]}`}
              <ChevronRight aria-hidden="true" />
            </Button>
          </div>
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

function PresetStep({ mode }: { mode: "vanilla" | "tmodloader" }) {
  return (
    <div>
      <h2 className="text-lg font-semibold">Choose a Preset</h2>
      <p className="mt-1 text-sm text-slate-400">Start with a template and customize it later.</p>
      <div className="mt-4 grid gap-3 md:grid-cols-2">
        {presets.filter((preset) => mode === "tmodloader" || preset[0] !== "Modded Starter").map((preset) => (
          <button key={preset[0]} className={cn("rounded-lg border border-panel-line bg-slate-950/40 p-4 text-left hover:border-panel-green", preset[0] === "Modded Starter" && "border-panel-green")}>
            <p className="font-medium">{preset[0]}</p>
            <p className="mt-1 text-sm text-slate-400">{preset[1]}</p>
            <div className="mt-4 flex flex-wrap gap-2">
              {preset.slice(2).map((tag) => <span key={tag} className="rounded bg-slate-800 px-2 py-1 text-xs text-slate-300">{tag}</span>)}
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

function ConfigStep() {
  return (
    <div>
      <h2 className="text-lg font-semibold">Server config</h2>
      <div className="mt-4 grid gap-4 md:grid-cols-2">
        <Input placeholder="Server name" defaultValue="Journey Friends" />
        <Input placeholder="World name" defaultValue="Moon Garden" />
        <Input placeholder="Port" defaultValue="7777" />
        <Input placeholder="Max players" defaultValue="8" />
      </div>
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

function ReviewStep({ mode }: { mode: "vanilla" | "tmodloader" }) {
  return (
    <div>
      <h2 className="text-lg font-semibold">Review</h2>
      <Card className="mt-4 p-4">
        <div className="flex items-center gap-3"><Settings2 aria-hidden="true" /> Terraria {mode === "tmodloader" ? "tModLoader" : "Vanilla"} server on port 7777</div>
      </Card>
    </div>
  );
}
