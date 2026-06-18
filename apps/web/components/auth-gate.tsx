"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Gamepad2, LockKeyhole, ShieldCheck } from "lucide-react";
import { usePathname } from "next/navigation";
import { useState, type FormEvent, type ReactNode } from "react";
import { Button, Card, Input } from "@/components/ui";
import { getAuthBootstrap, loginAdmin, setupAdmin } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export function AuthGate({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  if (pathname.startsWith("/share/")) {
    return children;
  }
  return <ProtectedAuthGate>{children}</ProtectedAuthGate>;
}

function ProtectedAuthGate({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  const queryClient = useQueryClient();
  const authQuery = useQuery({ queryKey: ["auth-bootstrap"], queryFn: getAuthBootstrap, retry: false });

  const refreshAuth = async () => {
    await queryClient.invalidateQueries({ queryKey: ["auth-bootstrap"] });
  };

  if (authQuery.isLoading) {
    return <AuthFrame title={t("authLoading")} description={t("authLoadingDescription")} />;
  }

  if (authQuery.isError) {
    return (
      <AuthFrame title={t("authApiUnavailable")} description={t("authApiUnavailableDescription")}>
        <Button variant="secondary" onClick={() => authQuery.refetch()}>
          {t("retry")}
        </Button>
      </AuthFrame>
    );
  }

  if (!authQuery.data?.initialized) {
    return <AuthForm mode="setup" onSuccess={refreshAuth} />;
  }

  if (!authQuery.data.account) {
    return <AuthForm mode="login" onSuccess={refreshAuth} />;
  }

  return children;
}

function AuthFrame({ title, description, children }: { title: string; description: string; children?: ReactNode }) {
  return (
    <main className="flex min-h-screen items-center justify-center bg-panel-bg px-4 text-slate-100">
      <Card className="w-full max-w-md p-6">
        <div className="flex items-start gap-4">
          <span className="flex size-11 shrink-0 items-center justify-center rounded-md bg-panel-green text-slate-950">
            <Gamepad2 aria-hidden="true" />
          </span>
          <div>
            <p className="text-sm font-semibold text-panel-green">GamePanel Lite</p>
            <h1 className="mt-2 text-2xl font-semibold text-white">{title}</h1>
            <p className="mt-2 text-sm leading-6 text-slate-400">{description}</p>
            {children ? <div className="mt-5">{children}</div> : null}
          </div>
        </div>
      </Card>
    </main>
  );
}

function AuthForm({ mode, onSuccess }: { mode: "setup" | "login"; onSuccess: () => Promise<void> }) {
  const { t } = useI18n();
  const [username, setUsername] = useState(mode === "setup" ? "admin" : "");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const isSetup = mode === "setup";
  const mutation = useMutation({
    mutationFn: () => (isSetup ? setupAdmin(username, password) : loginAdmin(username, password)),
    onSuccess: async () => {
      setError("");
      await onSuccess();
    },
    onError: (err) => setError(err instanceof Error ? err.message : t("authFailed"))
  });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError("");
    mutation.mutate();
  };

  return (
    <main className="flex min-h-screen items-center justify-center bg-panel-bg px-4 text-slate-100">
      <Card className="w-full max-w-md p-6">
        <div className="flex items-center gap-3">
          <span className="flex size-11 items-center justify-center rounded-md bg-panel-green text-slate-950">
            {isSetup ? <ShieldCheck aria-hidden="true" /> : <LockKeyhole aria-hidden="true" />}
          </span>
          <div>
            <p className="text-sm font-semibold text-panel-green">GamePanel Lite</p>
            <h1 className="text-2xl font-semibold text-white">{isSetup ? t("setupAdminTitle") : t("loginTitle")}</h1>
          </div>
        </div>
        <p className="mt-4 text-sm leading-6 text-slate-400">{isSetup ? t("setupAdminDescription") : t("loginDescription")}</p>
        <form className="mt-6 space-y-4" onSubmit={submit}>
          <label className="block">
            <span className="text-xs font-medium text-slate-400">{t("username")}</span>
            <Input className="mt-2 w-full" value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" />
          </label>
          <label className="block">
            <span className="text-xs font-medium text-slate-400">{t("password")}</span>
            <Input
              className="mt-2 w-full"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete={isSetup ? "new-password" : "current-password"}
            />
          </label>
          {error ? <p className="rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-200">{error}</p> : null}
          <Button className="h-11 w-full" type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? t("authSubmitting") : isSetup ? t("setupAdminAction") : t("loginAction")}
          </Button>
        </form>
      </Card>
    </main>
  );
}
