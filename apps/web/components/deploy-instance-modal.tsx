"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { X, Server } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { createGameServerWithResources } from "@/lib/create-server-flow";
import { Button, Input } from "@/components/ui";

interface DeployInstanceModalProps {
  open: boolean;
  onClose: () => void;
}

export function DeployInstanceModal({ open, onClose }: DeployInstanceModalProps) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");

  const [engine, setEngine] = useState<"vanilla" | "tmodloader">("vanilla");
  const [name, setName] = useState("terraria-survival-01");
  const [port, setPort] = useState(7777);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) return;
    const randomSuffix = Math.floor(10 + Math.random() * 90);
    setName(`terraria-${randomSuffix}`);
    setPort(7777);
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
      const providerKey = engine === "tmodloader" ? "terraria-tmodloader" : "terraria-vanilla";
      return createGameServerWithResources({
        name: name.trim(),
        mode: engine,
        providerKey,
        hostPort: port,
        config: {},
        resources: {
          cpuLimitCores: 2,
          memoryLimitMb: 2048
        }
      });
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["game-servers"] });
      onClose();
      if (result?.server?.id) {
        router.push(`/servers/${result.server.id}`);
      }
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : (isZh ? "创建实例失败，请检查配置" : "Failed to create instance");
      setError(msg);
    }
  });

  if (!open) return null;

  return (
    <div
      onClick={onClose}
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 backdrop-blur-xs p-4 animate-in fade-in duration-150"
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md rounded-2xl border border-slate-200/80 bg-white p-5 shadow-2xl space-y-4 animate-in zoom-in-95 duration-150"
      >
        {/* Modal Header */}
        <div className="flex items-center justify-between border-b border-slate-100 pb-3">
          <div className="flex items-center gap-2.5">
            <div className="flex size-7 items-center justify-center rounded-lg bg-emerald-50 text-emerald-600 font-bold border border-emerald-100">
              <Server className="size-4" />
            </div>
            <div>
              <h3 className="text-sm font-bold text-slate-900 leading-tight">
                {isZh ? "新建 Terraria 服务器" : "New Terraria Server"}
              </h3>
            </div>
          </div>
          <button
            onClick={onClose}
            className="flex size-7 items-center justify-center rounded-lg text-slate-400 hover:bg-slate-100 hover:text-slate-700 transition cursor-pointer"
          >
            <X className="size-4" />
          </button>
        </div>

        {/* Form Body */}
        <div className="space-y-3.5 text-xs">
          {/* Engine Selection */}
          <div>
            <label className="block font-semibold text-slate-700 mb-1.5">
              {isZh ? "服务器类型" : "Server Type"}
            </label>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => setEngine("vanilla")}
                className={`p-2.5 rounded-xl border text-left transition flex items-center gap-2 cursor-pointer ${
                  engine === "vanilla"
                    ? "border-emerald-500 bg-emerald-50/50 text-slate-900 shadow-2xs"
                    : "border-slate-200/80 bg-white text-slate-600 hover:bg-slate-50"
                }`}
              >
                <span className={`size-2 rounded-full ${engine === "vanilla" ? "bg-emerald-500" : "bg-slate-300"}`} />
                <div>
                  <div className="font-bold text-xs">Vanilla {isZh ? "原版" : "Server"}</div>
                  <div className="text-[10px] text-slate-400 font-mono">1.4.4.9</div>
                </div>
              </button>

              <button
                type="button"
                onClick={() => setEngine("tmodloader")}
                className={`p-2.5 rounded-xl border text-left transition flex items-center gap-2 cursor-pointer ${
                  engine === "tmodloader"
                    ? "border-purple-500 bg-purple-50/50 text-slate-900 shadow-2xs"
                    : "border-slate-200/80 bg-white text-slate-600 hover:bg-slate-50"
                }`}
              >
                <span className={`size-2 rounded-full ${engine === "tmodloader" ? "bg-purple-500" : "bg-slate-300"}`} />
                <div>
                  <div className="font-bold text-xs">tModLoader</div>
                  <div className="text-[10px] text-slate-400 font-mono">v2024.05</div>
                </div>
              </button>
            </div>
          </div>

          {/* Instance Name */}
          <div>
            <label className="block font-semibold text-slate-700 mb-1">
              {isZh ? "实例名称" : "Server Name"}
            </label>
            <Input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="font-mono text-xs"
            />
          </div>

          {/* Port */}
          <div>
            <label className="block font-semibold text-slate-700 mb-1">
              {isZh ? "服务端口" : "Server Port"}
            </label>
            <Input
              type="number"
              value={port}
              onChange={(e) => setPort(Number(e.target.value))}
              className="font-mono text-xs"
            />
          </div>

          {error && (
            <div className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
              {error}
            </div>
          )}
        </div>

        {/* Modal Footer */}
        <div className="border-t border-slate-100 pt-3 flex items-center justify-end gap-2">
          <Button variant="secondary" onClick={onClose} disabled={deployMutation.isPending} className="h-8 text-xs">
            {isZh ? "取消" : "Cancel"}
          </Button>
          <Button
            onClick={() => deployMutation.mutate()}
            disabled={deployMutation.isPending || !name.trim()}
            className="h-8 text-xs bg-emerald-600 hover:bg-emerald-700 text-white"
          >
            {deployMutation.isPending ? (isZh ? "创建中..." : "Deploying...") : (isZh ? "立即创建" : "Create")}
          </Button>
        </div>
      </div>
    </div>
  );
}
