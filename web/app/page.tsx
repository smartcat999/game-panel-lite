"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import LandingPage from "./components/landing/LandingPage";
import { RESOURCE_PLANS, appToPreset, type GamePreset } from "./console/lib/presets";
import { readToken } from "./console/lib/session";
import type { AppItem } from "./console/lib/types";

export default function HomePage() {
  const router = useRouter();
  const [authed, setAuthed] = useState(false);
  const [games, setGames] = useState<GamePreset[]>([]);

  useEffect(() => {
    setAuthed(!!readToken());
    let cancelled = false;
    fetch("/apis/core.gamepanel.io/v1/apps", { cache: "no-store" })
      .then((res) => (res.ok ? (res.json() as Promise<{ items?: AppItem[] }>) : null))
      .then((list) => {
        if (cancelled || !list?.items?.length) return;
        const published = list.items.filter((a) => a.published !== false).map(appToPreset);
        if (published.length > 0) {
          setGames(published);
        }
      })
      .catch(() => {
        // Ignore fetch errors when API is unreachable
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const deployHref = (gameId?: string, planId?: string) => {
    const params = new URLSearchParams();
    if (gameId) params.set("game", gameId);
    if (planId) params.set("plan", planId);
    const qs = params.toString();
    return `/console/new${qs ? `?${qs}` : ""}`;
  };

  return (
    <LandingPage
      isAuthed={authed}
      games={games}
      plans={RESOURCE_PLANS}
      onLogin={() => router.push("/login")}
      onEnterConsole={() => router.push("/console")}
      onDeploy={(gameId, planId) => {
        const target = deployHref(gameId, planId);
        router.push(authed ? target : `/login?next=${encodeURIComponent(target)}`);
      }}
    />
  );
}
