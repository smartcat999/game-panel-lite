"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Button } from "@/components/ui";
import { uninstallRuntimeImage } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { providerDisplayName } from "@/lib/provider-display";
import { formatRuntimeUninstallError } from "@/lib/runtime-errors";
import type { ProviderCatalog } from "@/lib/types";

export function RuntimeImageUninstall({ provider, disabled = false }: { provider: ProviderCatalog; disabled?: boolean }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const client = useQueryClient();
  const status = provider.runtimeImage;
  const uninstall = useMutation({
    mutationFn: () => uninstallRuntimeImage(provider.key),
    onSuccess: async () => {
      setOpen(false);
      await client.invalidateQueries({ queryKey: ["games"] });
    }
  });
  if (!status || !["ready", "update_available", "failed"].includes(status.status)) return null;
  return (
    <>
      <Button
        type="button"
        variant="ghost"
        className="h-8 whitespace-nowrap px-2 text-xs text-slate-400 hover:text-red-300"
        disabled={disabled || uninstall.isPending}
        onClick={() => { uninstall.reset(); setOpen(true); }}
      >
        <Trash2 aria-hidden="true" className="size-3.5" />
        {t("runtimeUninstall")}
      </Button>
      <ConfirmDialog
        open={open}
        eyebrow={t("versionManagementTitle")}
        title={t("runtimeUninstallTitle")}
        description={t("runtimeUninstallDescription", { name: providerDisplayName(provider.key, provider.name, t) })}
        detail={uninstall.isError ? <p className="text-red-300" role="alert">{formatRuntimeUninstallError(uninstall.error, t)}</p> : undefined}
        cancelLabel={t("cancel")}
        confirmLabel={t(uninstall.isPending ? "runtimeUninstalling" : "runtimeUninstall")}
        busy={uninstall.isPending}
        onCancel={() => setOpen(false)}
        onConfirm={() => uninstall.mutate()}
      />
    </>
  );
}
