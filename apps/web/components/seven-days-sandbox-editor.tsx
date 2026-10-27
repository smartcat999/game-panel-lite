"use client";

import { useEffect, useRef, useState } from "react";
import { Search, X } from "lucide-react";
import { Button, Input } from "@/components/ui";
import { useI18n } from "@/lib/i18n";
import { decodeSandboxCode, sandboxCategories, sandboxDisabledBy, sandboxOptions, sandboxValue, updateSandboxOption } from "@/lib/seven-days-sandbox";
import { cn } from "@/lib/utils";

export function SevenDaysSandboxEditor({ code, disabled, onChange }: { code: string; disabled: boolean; onChange: (code: string) => void }) {
  const { locale } = useI18n();
  const zh = locale === "zh";
  const dialog = useRef<HTMLDialogElement>(null);
  const [draft, setDraft] = useState(code);
  const [category, setCategory] = useState("General");
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  let valid = true;
  try { decodeSandboxCode(code); } catch { valid = false; }
  useEffect(() => {
    if (!open) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    dialog.current?.showModal();
    return () => { document.body.style.overflow = previous; };
  }, [open]);
  const close = () => { dialog.current?.close(); setOpen(false); };
  const options = sandboxOptions.filter((o) => o.category === category && `${o.label.zh} ${o.label.en} ${o.key}`.toLowerCase().includes(query.trim().toLowerCase()));
  return <>
    <button type="button" className="shrink-0 rounded-md border border-slate-700 px-2.5 py-2 text-xs text-panel-green hover:bg-slate-800 disabled:opacity-40" disabled={disabled || !valid} onClick={() => { setDraft(code); setQuery(""); setOpen(true); }}>{zh ? "编辑沙盒选项" : "Edit sandbox options"}</button>
    {!valid ? <p className="mt-1 text-xs text-panel-gold">{zh ? "可视化编辑支持 A 版本规则代码；请检查粘贴的代码。" : "Visual editing requires a valid version A code."}</p> : null}
    {open ? <dialog ref={dialog} aria-labelledby="seven-sandbox-title" onCancel={close} onClose={() => setOpen(false)} className="fixed inset-0 m-auto h-[min(80dvh,760px)] max-h-[calc(100dvh-2rem)] w-[calc(100vw-2rem)] max-w-5xl overflow-hidden rounded-xl border border-panel-line bg-panel-card p-0 text-slate-100 backdrop:bg-slate-950/80">
      <div className="flex h-full flex-col">
        <header className="flex shrink-0 items-center justify-between gap-3 border-b border-panel-line px-4 py-3">
          <h2 id="seven-sandbox-title" className="font-semibold">{zh ? "沙盒选项" : "Sandbox options"} <span className="ml-2 text-xs font-normal text-slate-500">v3.3 · 165</span></h2>
          <button type="button" aria-label={zh ? "关闭" : "Close"} onClick={close} className="rounded p-1.5 text-slate-400 hover:text-white"><X className="size-4" /></button>
        </header>
        <div className="shrink-0 border-b border-panel-line px-4 py-3">
          <nav aria-label={zh ? "沙盒分类" : "Sandbox categories"} className="flex flex-wrap gap-1">
            {sandboxCategories.map((c) => <button type="button" key={c.key} aria-pressed={category === c.key} className={cn("rounded-md px-3 py-2 text-xs font-medium", category === c.key ? "bg-slate-800 text-panel-green" : "text-slate-400 hover:text-white")} onClick={() => setCategory(c.key)}>{zh ? c.zh : c.en}</button>)}
          </nav>
          <label className="relative mt-2 block"><Search className="absolute left-2.5 top-2.5 size-3.5 text-slate-500" /><Input autoFocus aria-label={zh ? "搜索当前分类" : "Search this category"} placeholder={zh ? "搜索当前分类…" : "Search this category…"} value={query} onChange={(e) => setQuery(e.target.value)} className="h-9 pl-8 text-xs" /></label>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-4">
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {options.map((o) => {
              const value = sandboxValue(draft, o);
              const blockedBy = sandboxDisabledBy(draft, o);
              return <label key={o.id} className="min-w-0 rounded-lg border border-panel-line bg-slate-950/50 p-2.5">
                <span className="mb-2 block text-xs font-medium">{zh ? o.label.zh : o.label.en}</span>
                <select aria-label={zh ? o.label.zh : o.label.en} disabled={Boolean(blockedBy)} value={value} onChange={(e) => setDraft(updateSandboxOption(draft, o, Number(e.target.value)))} className="h-9 w-full rounded-md border border-slate-700 bg-slate-900 px-2 text-xs outline-none focus:border-panel-green disabled:opacity-40">
                  {value >= o.choices.length ? <option value={value}>{zh ? "未知选值" : "Unknown value"} ({value})</option> : null}
                  {o.choices.map((choice, index) => <option key={index} value={index}>{zh ? choice.zh : choice.en}</option>)}
                </select>
                {blockedBy ? <span className="mt-1 block text-[10px] text-slate-500">{zh ? `受“${blockedBy.label.zh}”限制` : `Disabled by ${blockedBy.label.en}`}</span> : null}
              </label>;
            })}
          </div>
          {!options.length ? <p className="py-10 text-center text-sm text-slate-500">{zh ? "没有匹配的选项" : "No matching options"}</p> : null}
        </div>
        <footer className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t border-panel-line px-4 py-3">
          <span className="text-xs text-slate-500">{zh ? "应用后需保存配置，重启服务器生效。" : "Save configuration and restart the server to apply."}</span>
          <div className="flex gap-2"><Button variant="secondary" onClick={close}>{zh ? "取消" : "Cancel"}</Button><Button disabled={disabled || draft === code} onClick={() => { onChange(draft); close(); }}>{zh ? "应用规则" : "Apply rules"}</Button></div>
        </footer>
      </div>
    </dialog> : null}
  </>;
}
