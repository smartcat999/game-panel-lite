import type { ReactNode } from "react";

export function ConsolePageHeader({
  title,
  action
}: {
  title: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex min-h-11 items-center justify-between gap-3 rounded-xl border bg-white px-3.5 py-2 micro-border subtle-elevation">
      <h1 className="text-xs font-bold leading-none text-slate-900">{title}</h1>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}
