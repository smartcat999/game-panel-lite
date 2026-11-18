"use client";

import { FormEvent, useEffect, useState } from "react";
import { useConsole } from "../../../console/ConsoleProvider";
import { errorMessage } from "../../../console/lib/session";
import type { KubeList, RegistryUpstream, Tenant } from "../../../console/lib/types";
import { Button, Field, Input, Modal, Switch } from "../ui";

const AUTO_PREWARM_ANNOTATION = "core.gamepanel.io/auto-prewarm-defaults";

export function DownloadProxyModal({
  open,
  onClose,
  onSaved,
}: {
  open: boolean;
  onClose: () => void;
  onSaved?: () => void;
}) {
  const { apiFetch, toast, reload } = useConsole();
  const [upstreams, setUpstreams] = useState<RegistryUpstream[]>([]);
  const [proxyURL, setProxyURL] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [autoPrewarm, setAutoPrewarm] = useState(false);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    setApiKey("");
    void (async () => {
      try {
        const [upRes, adminRes] = await Promise.all([
          apiFetch("/apis/registry.gamepanel.io/v1/upstreams?limit=200"),
          apiFetch("/apis/core.gamepanel.io/v1/tenants/admin"),
        ]);
        if (!upRes.ok) throw new Error(await errorMessage(upRes, "加载代理设置失败"));
        const data = (await upRes.json()) as KubeList<RegistryUpstream>;
        let nextAuto = false;
        if (adminRes.ok) {
          const adminTenant = (await adminRes.json()) as Tenant;
          nextAuto =
            (adminTenant.metadata.annotations?.[AUTO_PREWARM_ANNOTATION] || "").toLowerCase() ===
            "true";
        }
        if (cancelled) return;
        const items = data.items || [];
        setUpstreams(items);
        const existingProxy = items.find((u) => u.proxyURL?.trim())?.proxyURL?.trim() || "";
        setProxyURL(existingProxy);
        setAutoPrewarm(nextAuto);
      } catch (e) {
        if (!cancelled) toast("error", e instanceof Error ? e.message : "加载镜像设置失败");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, apiFetch, toast]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const trimmedProxy = proxyURL.trim();
    if (trimmedProxy && !/^(https?|socks5h?):\/\//i.test(trimmedProxy)) {
      toast("error", "代理地址需以 http://、https:// 或 socks5:// 开头");
      return;
    }
    setSaving(true);
    try {
      const trimmedKey = apiKey.trim();
      await Promise.all(
        upstreams.map(async (u) => {
          const patch: Record<string, unknown> = { proxyURL: trimmedProxy };
          if (u.provider === "steam" && trimmedKey) {
            patch.apiKey = trimmedKey;
          }
          const res = await apiFetch(
            `/apis/registry.gamepanel.io/v1/upstreams/${encodeURIComponent(u.metadata.name)}`,
            {
              method: "PATCH",
              headers: { "Content-Type": "application/merge-patch+json" },
              body: JSON.stringify(patch),
            },
          );
          if (!res.ok) throw new Error(await errorMessage(res, "保存代理设置失败"));
        }),
      );
      const tenantRes = await apiFetch("/apis/core.gamepanel.io/v1/tenants/admin", {
        method: "PATCH",
        headers: { "Content-Type": "application/merge-patch+json" },
        body: JSON.stringify({
          metadata: {
            annotations: {
              [AUTO_PREWARM_ANNOTATION]: autoPrewarm ? "true" : "false",
            },
          },
        }),
      });
      if (!tenantRes.ok) {
        throw new Error(await errorMessage(tenantRes, "保存自动预热开关失败"));
      }
      await reload();
      onSaved?.();
      toast("success", "设置已保存");
      onClose();
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "保存设置失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="镜像设置"
      description="配置上游网络代理、Steam 密钥与节点自动预热"
      footer={
        <>
          <Button type="button" variant="ghost" onClick={onClose}>
            取消
          </Button>
          <Button type="submit" form="download-proxy-form" loading={saving} disabled={loading}>
            保存
          </Button>
        </>
      }
    >
      <form id="download-proxy-form" onSubmit={(e) => void handleSubmit(e)} className="space-y-4">
        <Field label="网络代理（选填）" htmlFor="dl-proxy-url">
          <Input
            id="dl-proxy-url"
            name="proxyURL"
            value={proxyURL}
            onChange={(e) => setProxyURL(e.target.value)}
            disabled={loading}
            placeholder="例如 http://127.0.0.1:7890 或 socks5://127.0.0.1:1080"
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="none"
            spellCheck={false}
          />
        </Field>
        <Field label="Steam API 密钥（选填）" htmlFor="dl-steam-key">
          <Input
            id="dl-steam-key"
            name="apiKey"
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            disabled={loading}
            placeholder="留空保持当前配置不变"
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="none"
            spellCheck={false}
          />
        </Field>
        <div className="flex items-center justify-between gap-4 border-t border-[#EEF0F7] pt-4">
          <div className="min-w-0">
            <div className="text-[14px] font-medium text-[#14161F]">
              节点自动预热
            </div>
            <p className="mt-0.5 text-xs text-[#5B6075]">
              新节点接入或默认版本更新时，自动向节点同步默认游戏包与运行环境
            </p>
          </div>
          <Switch
            checked={autoPrewarm}
            onChange={setAutoPrewarm}
            disabled={loading}
            label="节点自动预热"
          />
        </div>
      </form>
    </Modal>
  );
}
