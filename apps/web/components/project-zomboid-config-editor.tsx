"use client";

import { Search } from "lucide-react";
import { useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { SecretInput } from "@/components/secret-input";
import { Button, Input } from "@/components/ui";
import { useI18n } from "@/lib/i18n";
import { providerConfigValue, type ProviderConfigPayload } from "@/lib/provider-config";
import { applyPZPreset, isPZRuntimeReadOnlyField, isPZSpawnPoint, pzINIStringError, pzMetadata, pzOption, pzPages, pzStartDayMax, updatePZField, type PZScope } from "@/lib/project-zomboid-config";
import type { ProviderConfigField } from "@/lib/types";
import { cn } from "@/lib/utils";

export function ProjectZomboidConfigEditor({ disabled = false, errors = {}, fields, payload, onPayloadChange, surface = "create" }: {
  disabled?: boolean; errors?: Record<string, string>; fields: ProviderConfigField[];
  payload: ProviderConfigPayload; onPayloadChange?: (payload: ProviderConfigPayload) => void; surface?: "create" | "server";
}) {
  const { locale } = useI18n();
  const zh = locale === "zh";
  const [scope, setScope] = useState<PZScope>("server");
  const [pageId, setPageId] = useState("Details");
  const [query, setQuery] = useState("");
  const [selectedPreset, setSelectedPreset] = useState("");
  const [confirmPreset, setConfirmPreset] = useState(false);
  const [inputErrors, setInputErrors] = useState<Record<string, string>>({});
  const pages = pzPages(scope);
  const activePage = pages.find((page) => page.id === pageId) ?? pages[0];
  const presets = pzMetadata.presets.filter((preset) => preset.values && pzMetadata.sandbox.every((option) => Object.hasOwn(preset.values, option.name)));
  const preset = presets.find((item) => item.id === selectedPreset);
  const locked = disabled || !onPayloadChange;
  const search = query.trim().toLocaleLowerCase();
  const matches = (field: ProviderConfigField) => {
    const option = pzOption(field.name);
    return option?.visible && `${field.name} ${option.label} ${option.labelEn} ${option.help} ${option.helpEn}`.toLocaleLowerCase().includes(search);
  };
  const visiblePages = search ? pages.filter((page) => page.fields.some((name) => fields.some((field) => field.name === `${scope}.${name}` && matches(field)))) : activePage ? [activePage] : [];
  const change = (field: ProviderConfigField, value: string | boolean) => {
    const error = pzINIStringError(field.name, value, locale) ?? (field.name === "server.SpawnPoint" && !isPZSpawnPoint(String(value)) ? (zh ? "坐标需要三个非负整数。" : "Coordinates require three non-negative integers.") : undefined);
    setInputErrors((current) => ({ ...current, [field.name]: error ?? "" }));
    if (!error) onPayloadChange?.(updatePZField(payload, field, value));
  };
  const adminField = fields.find((field) => field.name === "adminPassword");

  const renderField = (field: ProviderConfigField) => {
    const option = pzOption(field.name);
    const label = field.name === "adminPassword" ? "adminPassword" : field.name.startsWith("server.") ? option?.name ?? field.label : zh ? option?.label ?? field.label : option?.labelEn ?? field.labelEn ?? field.label;
    const help = field.name === "adminPassword"
      ? zh ? "仅用于首次创建管理员；留空由后台生成，不会覆盖已有账号密码。" : "Initial admin password only. Leave empty to generate; existing accounts are never overwritten."
      : (zh ? option?.help : option?.helpEn) ?? "";
    const value = providerConfigValue(payload, field.name) ?? option?.defaultValue ?? "";
    const fixedPort = ["server.DefaultPort", "server.UDPPort"].includes(field.name);
    const constrained = option?.editor === "maps";
    const runtimeReadOnly = isPZRuntimeReadOnlyField(field.name);
    const error = inputErrors[field.name] || errors[field.name];
    const readOnly = runtimeReadOnly || fixedPort || option?.readOnly || option?.readonly || constrained || (field.name === "adminPassword" && surface === "server");
    const inputDisabled = locked || readOnly;
    const id = `pz-${field.name}`;
    const descriptionId = `${id}-help`;
    const errorId = `${id}-error`;
    const common = { id, disabled: inputDisabled, "aria-invalid": Boolean(error), "aria-describedby": `${descriptionId}${error ? ` ${errorId}` : ""}` };
    const secret = field.type === "password" || /Password$|Token$/.test(field.name);
    return (
      <div key={field.name} className="grid min-w-0 gap-2 border-b border-panel-line py-3 last:border-0 sm:grid-cols-[minmax(0,1fr)_minmax(9rem,14rem)] sm:gap-x-5">
        <div className="min-w-0">
          <label className="break-words text-xs font-medium text-slate-200" htmlFor={id}>{label}</label>
          <p id={descriptionId} className="mt-1 whitespace-pre-line break-words text-xs leading-5 text-slate-400">{help}{runtimeReadOnly ? (zh ? " 当前 runtime 未启用远程管理或自动端口映射，仅发布 UDP 16261/16262。" : " This runtime does not enable remote administration or automatic port mapping; only UDP 16261/16262 are published.") : ""}{fixedPort ? (zh ? " 容器内端口固定；请单独修改主机端口映射。" : " Container port is fixed; edit the host port mapping separately.") : ""}{constrained ? (zh ? " 原版地图固定为 Muldraugh, KY。" : " Vanilla map is fixed to Muldraugh, KY.") : ""}{field.name.startsWith("server.") && option?.type === "text" ? (zh ? " INI 值保持单行；游戏内显示换行请用 <LINE>，字面反斜杠 n 保持原样。" : " Keep INI values on one line. Use <LINE> for display line breaks; literal backslash-n is preserved.") : ""}</p>
        </div>
        <div className="min-w-0 self-start">
          {option?.editor === "spawn-point" ? (
            <div className="grid grid-cols-3 gap-2">
              {(["x", "y", "z"] as const).map((axis, index) => <label key={axis} className="text-xs text-slate-300">
                {axis}
                <Input {...common} id={index === 0 ? id : `${id}-${axis}`} className="mt-1 h-9 px-2 text-xs" aria-label={`SpawnPoint ${axis}`} type="number" min={0} step={1} value={String(value).split(",")[index] ?? "0"} onChange={(event) => {
                  const coordinates = String(value).split(",");
                  coordinates[index] = event.target.value;
                  change(field, coordinates.join(","));
                }} />
              </label>)}
            </div>
          ) : option?.type === "boolean" || field.type === "boolean" ? (
            <button {...common} type="button" role="switch" aria-checked={value === true} aria-label={label} className="flex h-9 w-full items-center justify-between rounded-md border border-panel-line bg-slate-900 px-3 text-xs text-slate-200 outline-none focus-visible:ring-2 focus-visible:ring-panel-green disabled:opacity-50" onClick={() => change(field, value !== true)}>
              {value === true ? (zh ? "开启" : "Enabled") : (zh ? "关闭" : "Disabled")}
              <span aria-hidden="true" className={cn("h-3 w-3 rounded-full", value === true ? "bg-panel-green" : "bg-slate-500")} />
            </button>
          ) : option?.choices ? (
            <select {...common} className="h-9 w-full rounded-md border border-panel-line bg-slate-900 px-2 text-xs text-slate-200 outline-none focus-visible:ring-2 focus-visible:ring-panel-green disabled:opacity-50" value={String(value)} onChange={(event) => change(field, event.target.value)}>
              {option.choices.filter((choice) => field.name !== "sandbox.StartDay" || choice.value <= pzStartDayMax(payload)).map((choice) => <option key={choice.value} value={choice.value}>{zh ? choice.label : choice.labelEn}</option>)}
            </select>
          ) : secret ? (
            <SecretInput {...common} value={String(value)} showLabel={zh ? "显示密码" : "Show password"} hideLabel={zh ? "隐藏密码" : "Hide password"} onChange={(event) => change(field, event.target.value)} />
          ) : option?.type === "text" ? (
            <textarea {...common} className="min-h-24 w-full rounded-md border border-panel-line bg-slate-900 p-2 text-xs text-slate-200 outline-none focus-visible:ring-2 focus-visible:ring-panel-green disabled:opacity-50" value={String(value)} onChange={(event) => change(field, event.target.value)} />
          ) : (
            <Input {...common} className="h-9 text-xs" type={option && ["integer", "double"].includes(option.type) ? "number" : "text"} min={option?.min ?? undefined} max={field.name === "sandbox.StartDay" ? pzStartDayMax(payload) : option?.max ?? undefined} maxLength={option?.maxLength ?? undefined} step={option?.type === "double" ? "any" : 1} value={String(fixedPort ? field.name === "server.DefaultPort" ? 16261 : 16262 : value)} onChange={(event) => change(field, event.target.value)} />
          )}
          {error && <p id={errorId} role="alert" className="mt-1 text-xs text-red-300">{error}</p>}
        </div>
      </div>
    );
  };

  return (
    <div className="mt-3 min-w-0 space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        {(["server", "sandbox"] as const).map((tab) => <Button key={tab} type="button" variant={scope === tab ? "primary" : "secondary"} className="h-8 text-xs" aria-pressed={scope === tab} onClick={() => { setScope(tab); setPageId(pzPages(tab)[0]?.id ?? ""); setQuery(""); }}>{tab === "server" ? (zh ? "服务器选项" : "Server options") : (zh ? "沙盒规则" : "Sandbox rules")}</Button>)}
        <span className="text-xs text-slate-400">Project Zomboid {pzMetadata.source.gameVersion}</span>
      </div>
      <div className="relative">
        <Search aria-hidden="true" className="absolute left-3 top-2.5 size-4 text-slate-400" />
        <Input className="h-9 pl-9 text-xs" aria-label={zh ? "搜索原生字段、说明" : "Search native fields and descriptions"} placeholder={zh ? "搜索原生字段、说明" : "Search native fields and descriptions"} value={query} onChange={(event) => setQuery(event.target.value)} />
      </div>
      <div className="grid min-w-0 gap-4 md:grid-cols-[10rem_minmax(0,1fr)]">
        <nav aria-label={zh ? "配置分类" : "Configuration categories"} className="flex gap-1 overflow-x-auto md:max-h-[36rem] md:flex-col md:overflow-y-auto">
          {pages.map((page) => <button key={page.id} type="button" aria-current={page.id === activePage?.id && !search ? "page" : undefined} className={cn("shrink-0 rounded-md px-3 py-2 text-left text-xs outline-none focus-visible:ring-2 focus-visible:ring-panel-green", page.id === activePage?.id && !search ? "bg-panel-green/10 text-panel-green" : "text-slate-300 hover:bg-slate-800")} onClick={() => { setPageId(page.id); setQuery(""); }}>{zh ? page.label : page.labelEn}</button>)}
        </nav>
        <div className="min-w-0">
          {visiblePages.map((page) => <section key={page.id} aria-label={zh ? page.label : page.labelEn}>
            <h3 className="border-b border-panel-line pb-2 text-sm font-semibold text-slate-100">{zh ? page.label : page.labelEn}</h3>
            {page.editor === "presets" ? <div className="space-y-3 py-3">
              <p className="text-xs leading-5 text-slate-400">{zh ? "新建使用原生默认值。选择预设不会自动应用；应用后替换整个沙盒草稿，保留服务器选项和密码，保存后生效。" : "New servers use native defaults. Selecting a preset does not apply it. Applying replaces the entire sandbox draft and preserves server options and passwords. Save to persist."}</p>
              <label className="block text-xs text-slate-200" htmlFor="pz-preset">{zh ? "官方预设" : "Official preset"}</label>
              <select id="pz-preset" className="h-9 w-full rounded-md border border-panel-line bg-slate-900 px-2 text-sm text-slate-200 disabled:opacity-50" disabled={locked || !presets.length} value={selectedPreset} onChange={(event) => setSelectedPreset(event.target.value)}>
                <option value="">{zh ? "选择预设" : "Select a preset"}</option>
                {presets.map((item) => <option key={item.id} value={item.id}>{zh ? item.label ?? item.zh ?? item.id : item.labelEn ?? item.id}</option>)}
              </select>
              {!presets.length && <p className="text-xs text-slate-400">{zh ? "官方预设参数尚未核验完成，暂不可应用。" : "Official preset values are awaiting verification."}</p>}
              <Button type="button" className="h-8 text-xs" disabled={locked || !preset} onClick={() => setConfirmPreset(true)}>{zh ? "应用预设到草稿" : "Apply preset to draft"}</Button>
            </div> : <>{scope === "server" && page.id === "Admin" && adminField && renderField(adminField)}{page.fields.flatMap((name) => {
              const field = fields.find((item) => item.name === `${scope}.${name}`);
              return field && matches(field) ? [renderField(field)] : [];
            })}{scope === "server" && page.id === "SpawnRegions" && !search && <div className="py-3">
              <p className="text-xs font-medium text-slate-200">{zh ? "官方默认出生区域" : "Official default spawn regions"}</p>
              <p className="mt-1 text-xs leading-5 text-slate-400">{zh ? "使用原版 11 个出生区域及其默认顺序，当前仅供查看。" : "Uses the 11 vanilla spawn regions in their default order. Read-only."}</p>
              <ol className="mt-2 list-inside list-decimal space-y-1 text-xs text-slate-300">{page.defaultRegions?.map((region) => <li key={region.name}>{region.name}</li>)}</ol>
            </div>}</>}
          </section>)}
          {visiblePages.length === 0 && <p className="py-6 text-sm text-slate-400">{zh ? "没有匹配的配置项。" : "No matching options."}</p>}
        </div>
      </div>
      <ConfirmDialog open={confirmPreset} eyebrow="Project Zomboid" title={zh ? "覆盖当前沙盒草稿？" : "Replace the sandbox draft?"} description={zh ? "所有沙盒规则将重置为所选预设。服务器选项和已有密码保持不变，保存前仍可修改。" : "All sandbox rules will be replaced by the selected preset. Server options and existing passwords are preserved. You can still edit before saving."} cancelLabel={zh ? "保留草稿" : "Keep draft"} confirmLabel={zh ? "替换沙盒草稿" : "Replace sandbox draft"} confirmDisabled={locked || !preset} onCancel={() => setConfirmPreset(false)} onConfirm={() => { if (!locked && preset) onPayloadChange?.(applyPZPreset(payload, preset)); setConfirmPreset(false); }} />
    </div>
  );
}
