"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Save } from "lucide-react";
import { useEffect, useState } from "react";
import { ConsolePageHeader } from "@/components/console-page-header";
import { Button, Input } from "@/components/ui";
import { getSettings, updatePublicHost } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export default function SettingsPage() {
  const { locale } = useI18n();
  const isZh = locale === "zh";
  const queryClient = useQueryClient();
  const [host, setHost] = useState("");
  const [hostSaved, setHostSaved] = useState(false);

  const settingsQuery = useQuery({ queryKey: ["settings"], queryFn: getSettings, retry: false });

  useEffect(() => {
    if (settingsQuery.data?.publicHost) setHost(settingsQuery.data.publicHost);
  }, [settingsQuery.data?.publicHost]);

  const hostMutation = useMutation({
    mutationFn: updatePublicHost,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
      setHostSaved(true);
      window.setTimeout(() => setHostSaved(false), 2000);
    }
  });

  return (
    <div className="space-y-3">
      <ConsolePageHeader title={isZh ? "空间设置" : "Workspace settings"} />

      <form onSubmit={(event) => { event.preventDefault(); hostMutation.mutate(host.trim()); }} className="max-w-2xl space-y-4 rounded-xl border bg-white p-5 micro-border subtle-elevation">
        <div>
          <h2 className="text-xs font-bold uppercase tracking-wider text-slate-900">{isZh ? "公网连接" : "Public endpoint"}</h2>
          <p className="mt-0.5 text-[11px] text-slate-500">{isZh ? "玩家连接实例时显示的公网地址。" : "The public address shown to players when they connect."}</p>
        </div>
        <label className="block text-xs font-semibold text-slate-700">
          {isZh ? "公网主机地址" : "Public host"}
          <Input value={host} onChange={(event) => setHost(event.target.value)} placeholder={isZh ? "例如：play.example.com" : "For example: play.example.com"} className="mt-1 font-mono text-xs" />
        </label>
        <div className="flex items-center justify-between border-t border-slate-100 pt-3">
          {hostSaved ? <span className="flex items-center gap-1 text-xs font-semibold text-emerald-600"><Check className="size-3.5" />{isZh ? "设置已保存" : "Settings saved"}</span> : <span />}
          <Button type="submit" disabled={hostMutation.isPending} className="h-8 bg-slate-900 px-4 text-xs text-white hover:bg-slate-800">
            <Save className="size-3.5" />{hostMutation.isPending ? (isZh ? "保存中…" : "Saving…") : (isZh ? "保存" : "Save")}
          </Button>
        </div>
      </form>
    </div>
  );
}
