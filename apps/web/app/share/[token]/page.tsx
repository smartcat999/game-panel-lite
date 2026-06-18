"use client";

import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Copy, Gamepad2, LockKeyhole, Server, Users } from "lucide-react";
import { useState, type ReactNode } from "react";
import { ServerStatusBadge } from "@/components/server-badges";
import { Button, Card } from "@/components/ui";
import { getPublicServerShare } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export default function SharedServerPage() {
  const { t } = useI18n();
  const params = useParams<{ token: string }>();
  const token = params.token;
  const [copied, setCopied] = useState("");
  const query = useQuery({ queryKey: ["public-server-share", token], queryFn: () => getPublicServerShare(token), retry: false });
  const server = query.data;

  const copy = async (label: string, value: string) => {
    await navigator.clipboard.writeText(value);
    setCopied(label);
    window.setTimeout(() => setCopied(""), 1500);
  };

  if (query.isLoading) {
    return <ShareFrame><Card className="p-6 text-sm text-slate-400">{t("loading")}</Card></ShareFrame>;
  }

  if (query.isError || !server) {
    return (
      <ShareFrame>
        <Card className="p-6">
          <div className="flex items-start gap-3">
            <span className="flex size-10 shrink-0 items-center justify-center rounded-md border border-panel-gold/30 bg-panel-gold/10 text-panel-gold">
              <LockKeyhole aria-hidden="true" className="size-5" />
            </span>
            <div>
              <h1 className="text-xl font-semibold text-white">{t("sharePageUnavailable")}</h1>
              <p className="mt-2 text-sm leading-6 text-slate-400">{t("sharePageUnavailableDescription")}</p>
            </div>
          </div>
        </Card>
      </ShareFrame>
    );
  }

  const joinAddress = `${server.joinInfo.address}:${server.joinInfo.port}`;

  return (
    <ShareFrame>
      <Card className="overflow-hidden">
        <div className="border-b border-panel-line p-6">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold text-white">{server.name}</h1>
            <ServerStatusBadge status={server.status} />
          </div>
          <div className="mt-4 flex flex-wrap gap-2 text-sm text-slate-300">
            <span className="inline-flex items-center gap-2 rounded-md border border-panel-line bg-slate-950/45 px-3 py-2">
              <Users aria-hidden="true" className="size-4 text-slate-500" />
              {server.players} / {server.maxPlayers}
            </span>
            <span className="inline-flex items-center gap-2 rounded-md border border-panel-line bg-slate-950/45 px-3 py-2">
              <Server aria-hidden="true" className="size-4 text-slate-500" />
              {joinAddress}
            </span>
          </div>
        </div>

        <div className="space-y-4 p-6">
          <ShareCopyRow label={t("ipAddress")} value={server.joinInfo.address} copied={copied} onCopy={copy} />
          <ShareCopyRow label={t("port")} value={String(server.joinInfo.port)} copied={copied} onCopy={copy} />
          {server.joinInfo.password ? <ShareCopyRow label={t("password")} value={server.joinInfo.password} copied={copied} onCopy={copy} /> : null}
          <Button className="h-11 w-full" onClick={() => void copy("Invite", server.joinInfo.inviteText)}>
            <Copy aria-hidden="true" />
            {copied === "Invite" ? t("copied") : t("copyInviteText")}
          </Button>
          {server.joinInfo.instructions?.length ? (
            <div className="rounded-md border border-panel-line bg-slate-950/35 p-4 text-sm leading-6 text-slate-400">
              {server.joinInfo.instructions.map((line) => <p key={line}>{line}</p>)}
            </div>
          ) : null}
        </div>
      </Card>
    </ShareFrame>
  );
}

function ShareFrame({ children }: { children: ReactNode }) {
  return (
    <main className="min-h-screen bg-panel-bg px-4 py-10 text-slate-100">
      <div className="mx-auto w-full max-w-2xl">
        <div className="mb-6 flex items-center gap-3">
          <span className="flex size-10 items-center justify-center rounded-md bg-panel-green text-slate-950">
            <Gamepad2 aria-hidden="true" />
          </span>
          <div>
            <p className="font-semibold text-white">GamePanel Lite</p>
            <p className="text-xs text-slate-500">Server invite</p>
          </div>
        </div>
        {children}
      </div>
    </main>
  );
}

function ShareCopyRow({ copied, label, onCopy, value }: { copied: string; label: string; onCopy: (label: string, value: string) => void; value: string }) {
  const { t } = useI18n();
  return (
    <div className="flex items-center justify-between gap-3 rounded-md border border-panel-line bg-slate-950/35 px-3 py-3">
      <div className="min-w-0">
        <p className="text-xs text-slate-500">{label}</p>
        <p className="truncate text-sm font-semibold text-white">{value}</p>
      </div>
      <Button className="shrink-0" variant="secondary" onClick={() => onCopy(label, value)}>
        <Copy aria-hidden="true" className="size-4" />
        {copied === label ? t("copied") : t("copy")}
      </Button>
    </div>
  );
}
