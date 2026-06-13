"use client";

import { useQuery } from "@tanstack/react-query";
import { ShieldCheck } from "lucide-react";
import { AppShell } from "@/components/app-shell";
import { PageHeader } from "@/components/page-header";
import { Card } from "@/components/ui";
import { getDockerStatus } from "@/lib/api";

export default function SettingsPage() {
  const docker = useQuery({ queryKey: ["docker-status"], queryFn: getDockerStatus, retry: false });
  return (
    <AppShell>
      <PageHeader title="Settings" description="Runtime and local panel settings." />
      <div className="grid gap-4 md:grid-cols-2">
        <Card className="p-5">
          <div className="flex items-center gap-3 text-panel-green"><ShieldCheck aria-hidden="true" /><h2 className="font-semibold text-white">Docker Runtime</h2></div>
          <p className="mt-3 text-sm text-slate-400">
            {docker.data ? docker.data.message : docker.isError ? "API unavailable. Start the Go backend to check Docker." : "Checking Docker runtime..."}
          </p>
          {docker.data && <p className={docker.data.available ? "mt-2 text-sm text-panel-green" : "mt-2 text-sm text-panel-gold"}>{docker.data.available ? "Available" : "Unavailable"}</p>}
        </Card>
        <Card className="p-5">
          <h2 className="font-semibold">Data Directories</h2>
          <p className="mt-3 text-sm text-slate-400">Each server instance will use an isolated data directory under the configured GamePanel data root.</p>
        </Card>
      </div>
    </AppShell>
  );
}
