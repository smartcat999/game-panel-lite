export default function Loading() {
  return (
    <div className="min-h-[640px] animate-pulse rounded-lg border border-panel-line bg-panel-card p-6">
      <div className="h-7 w-48 rounded bg-slate-800" />
      <div className="mt-7 grid grid-cols-6 gap-3">
        {Array.from({ length: 6 }).map((_, index) => (
          <div key={index} className="flex flex-col items-center gap-2">
            <div className="size-8 rounded-full border border-panel-line bg-slate-800" />
            <div className="h-3 w-12 rounded bg-slate-800" />
          </div>
        ))}
      </div>
      <div className="mt-8 space-y-4">
        <div className="h-6 w-32 rounded bg-slate-800" />
        <div className="grid gap-3 md:grid-cols-2">
          {Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className="h-24 rounded-lg border border-panel-line bg-slate-950/40" />
          ))}
        </div>
      </div>
    </div>
  );
}
