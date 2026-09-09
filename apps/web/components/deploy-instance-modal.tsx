"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, X, Zap } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { createGameServerWithResources } from "@/lib/create-server-flow";
import { cn } from "@/lib/utils";

interface DeployInstanceModalProps {
  open: boolean;
  onClose: () => void;
}

export function DeployInstanceModal({ open, onClose }: DeployInstanceModalProps) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { locale } = useI18n();
  const isZh = locale === "zh";

  const [engine, setEngine] = useState<"vanilla" | "tmodloader">("vanilla");
  const [name, setName] = useState("terraria-cloud-01");
  const [port, setPort] = useState(7781);
  const [tier, setTier] = useState<"standard" | "pro">("standard");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    const randomSuffix = Math.floor(10 + Math.random() * 90);
    setName(`terraria-cloud-${randomSuffix}`);
    setPort(7780 + Math.floor(Math.random() * 20));
    setError("");
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [open, onClose]);

  const deployMutation = useMutation({
    mutationFn: async () => {
      const mode = engine === "tmodloader" ? "tmodloader" : "vanilla";
      const ramMb = tier === "pro" ? 8192 : 4096;
      return createGameServerWithResources({
        name: name.trim() || "terraria-server",
        mode,
        hostPort: port,
        config: {
          worldName: `${name.trim() || "world"}.wld`,
          motd: "Welcome to GamePanel Lite Server!",
          maxPlayers: 16
        },
        resources: {
          memoryLimitMb: ramMb,
          cpuLimitCores: tier === "pro" ? 4 : 2
        }
      });
    },
    onSuccess: async (created) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["game-servers"] }),
        queryClient.invalidateQueries({ queryKey: ["game-servers-page"] })
      ]);
      onClose();
      if (created?.server?.id) {
        router.push(`/servers/${created.server.id}`);
      }
    },
    onError: (err) => {
      setError(err instanceof Error ? err.message : isZh ? "创建实例失败" : "Failed to deploy instance");
    }
  });

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-slate-900/40 backdrop-blur-xs animate-in fade-in duration-150"
      onClick={onClose}
    >
      <div
        className="w-full max-w-md rounded-2xl border micro-border bg-white p-5 shadow-2xl space-y-4 animate-in fade-in zoom-in-95 duration-150"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b micro-border pb-3">
          <div className="flex items-center gap-2.5">
            <div className="flex size-7 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-600 font-bold">
              <Plus className="size-4 stroke-[2.5]" />
            </div>
            <div>
              <h3 className="text-sm font-bold text-slate-900 leading-tight">
                {isZh ? "新建 Terraria 容器实例" : "Deploy Terraria Container"}
              </h3>
              <p className="text-[11px] text-slate-400">
                {isZh ? "独立容器沙盒隔离 · 自动绑定端口与卷" : "Isolated container sandbox · Independent volume"}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="flex size-7 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 hover:text-slate-700 transition"
          >
            <X className="size-4" />
          </button>
        </div>

        {error && (
          <div className="rounded-lg border border-rose-200 bg-rose-50/70 p-2.5 text-xs text-rose-700 font-medium">
            {error}
          </div>
        )}

        <div className="space-y-3.5 text-xs">
          {/* Engine Selector */}
          <div>
            <label className="block font-semibold text-slate-700 mb-1.5">
              {isZh ? "核心类型" : "Server Core Engine"}
            </label>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => setEngine("vanilla")}
                className={cn(
                  "flex items-center gap-2.5 rounded-xl border p-2.5 text-left transition",
                  engine === "vanilla"
                    ? "border-2 border-emerald-500 bg-emerald-50/40 shadow-xs"
                    : "micro-border hover:bg-slate-50"
                )}
              >
                <span className="size-2 rounded-full bg-emerald-500 shrink-0" />
                <div>
                  <div className="font-bold text-slate-900 text-xs">Vanilla 原版</div>
                  <div className="text-[10px] text-slate-400 font-mono">1.4.4.9 Stable</div>
                </div>
              </button>
              <button
                type="button"
                onClick={() => setEngine("tmodloader")}
                className={cn(
                  "flex items-center gap-2.5 rounded-xl border p-2.5 text-left transition",
                  engine === "tmodloader"
                    ? "border-2 border-purple-500 bg-purple-50/40 shadow-xs"
                    : "micro-border hover:bg-slate-50"
                )}
              >
                <span className="size-2 rounded-full bg-purple-500 shrink-0" />
                <div>
                  <div className="font-bold text-slate-900 text-xs">tModLoader</div>
                  <div className="text-[10px] text-slate-400 font-mono">v2024.05 Modded</div>
                </div>
              </button>
            </div>
          </div>

          {/* Instance Name */}
          <div>
            <label className="block font-semibold text-slate-700 mb-1">
              {isZh ? "实例标识符 (Name)" : "Instance Name"}
            </label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="e.g. terraria-survival-01"
              className="h-8 w-full rounded-lg border micro-border bg-slate-50/50 px-3 text-xs font-mono text-slate-900 focus:border-emerald-500 focus:bg-white focus:outline-hidden transition"
            />
          </div>

          {/* Port and Plan */}
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="block font-semibold text-slate-700 mb-1">
                {isZh ? "分配主机端口" : "Host Port"}
              </label>
              <input
                type="number"
                value={port}
                onChange={(e) => setPort(Number(e.target.value))}
                className="h-8 w-full rounded-lg border micro-border bg-slate-50/50 px-3 text-xs font-mono text-slate-900 focus:border-emerald-500 focus:bg-white focus:outline-hidden transition"
              />
            </div>
            <div>
              <label className="block font-semibold text-slate-700 mb-1">
                {isZh ? "计算资源套餐" : "Compute Tier"}
              </label>
              <select
                value={tier}
                onChange={(e) => setTier(e.target.value as "standard" | "pro")}
                className="h-8 w-full rounded-lg border micro-border bg-slate-50/50 px-2 text-xs text-slate-900 focus:border-emerald-500 focus:bg-white focus:outline-hidden transition"
              >
                <option value="standard">{isZh ? "标准 4GB RAM (2 vCPU)" : "Standard 4GB (2 vCPU)"}</option>
                <option value="pro">{isZh ? "旗舰 8GB RAM (4 vCPU)" : "Pro 8GB (4 vCPU)"}</option>
              </select>
            </div>
          </div>
        </div>

        {/* Footer Actions */}
        <div className="flex items-center justify-between border-t micro-border pt-3">
          <button
            type="button"
            onClick={() => {
              onClose();
              router.push("/servers/new");
            }}
            className="text-[11px] text-slate-400 hover:text-slate-700 hover:underline transition"
          >
            {isZh ? "高级专家向导..." : "Advanced Wizard..."}
          </button>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              className="h-8 rounded-lg border micro-border px-3 text-xs font-medium text-slate-600 hover:bg-slate-50 transition"
            >
              {isZh ? "取消" : "Cancel"}
            </button>
            <button
              type="button"
              disabled={deployMutation.isPending || !name.trim()}
              onClick={() => deployMutation.mutate()}
              className="flex h-8 items-center gap-1.5 rounded-lg bg-emerald-600 px-3.5 text-xs font-bold text-white shadow-xs hover:bg-emerald-500 transition disabled:opacity-50"
            >
              {deployMutation.isPending ? (
                <span>{isZh ? "正在部署..." : "Deploying..."}</span>
              ) : (
                <>
                  <Zap className="size-3.5 fill-current" />
                  <span>{isZh ? "立即创建" : "Deploy Now"}</span>
                </>
              )}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
