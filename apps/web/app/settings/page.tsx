"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Save, Check } from "lucide-react";
import { useState, useEffect } from "react";
import { useI18n } from "@/lib/i18n";
import { getSettings, updatePublicHost } from "@/lib/api";
import { Button, Input } from "@/components/ui";
import { ConsolePageHeader } from "@/components/console-page-header";

export default function SettingsPage() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const queryClient = useQueryClient();

  const [host, setHost] = useState("");
  const [savedSuccess, setSavedSuccess] = useState(false);

  const settingsQuery = useQuery({
    queryKey: ["settings"],
    queryFn: getSettings,
    retry: false
  });

  useEffect(() => {
    if (settingsQuery.data?.publicHost) {
      setHost(settingsQuery.data.publicHost);
    }
  }, [settingsQuery.data?.publicHost]);

  const saveMutation = useMutation({
    mutationFn: (newHost: string) => updatePublicHost(newHost),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["settings"] });
      setSavedSuccess(true);
      setTimeout(() => setSavedSuccess(false), 2000);
    }
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    saveMutation.mutate(host.trim());
  };

  return (
    <div className="space-y-3">
      <ConsolePageHeader title={isZh ? "系统设置" : "Settings"} />

      <form onSubmit={handleSubmit} className="max-w-2xl space-y-4 rounded-xl border bg-white p-5 micro-border subtle-elevation">
        <div>
          <h3 className="text-xs font-bold text-slate-900 uppercase tracking-wider">
            {isZh ? "公网连接配置" : "Public Endpoint"}
          </h3>
          <p className="text-[11px] text-slate-400 mt-0.5">
            {isZh ? "玩家连接服务器时显示的公网 IP 或域名" : "Public IP or domain shown to players for connection."}
          </p>
        </div>

        <div className="space-y-3 text-xs">
          <div>
            <label className="block font-semibold text-slate-700 mb-1">
              {isZh ? "公网主机地址 (Public Host / IP)" : "Public Host / IP"}
            </label>
            <Input
              value={host}
              onChange={(e) => setHost(e.target.value)}
              placeholder="e.g. 192.168.2.4 or play.example.com"
              className="font-mono text-xs"
            />
          </div>
        </div>

        <div className="pt-3 border-t border-slate-100 flex items-center justify-between">
          {savedSuccess ? (
            <span className="text-xs font-semibold text-emerald-600 flex items-center gap-1">
              <Check className="size-3.5" />
              <span>{isZh ? "设置已保存" : "Saved successfully"}</span>
            </span>
          ) : <span />}

          <Button type="submit" disabled={saveMutation.isPending} className="h-8 px-4 bg-slate-900 hover:bg-slate-800 text-white text-xs">
            <Save className="size-3.5" />
            <span>{saveMutation.isPending ? (isZh ? "保存中..." : "Saving...") : (isZh ? "保存设置" : "Save Settings")}</span>
          </Button>
        </div>
      </form>
    </div>
  );
}
