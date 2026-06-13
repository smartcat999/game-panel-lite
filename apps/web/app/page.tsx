import Link from "next/link";

export default function HomePage() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-panel-bg p-8">
      <div className="max-w-xl rounded-lg border border-panel-line bg-panel-card p-8">
        <p className="text-sm text-panel-green">GamePanel Lite</p>
        <h1 className="mt-3 text-3xl font-semibold">Terraria servers, ready for V1.</h1>
        <p className="mt-3 text-sm leading-6 text-slate-300">
          The foundation is wired for a Go API, SQLite storage, Docker runtime adapters, and a polished dark dashboard.
        </p>
        <Link className="mt-6 inline-flex rounded-md bg-panel-green px-4 py-2 text-sm font-medium text-slate-950" href="/dashboard">
          Open dashboard
        </Link>
      </div>
    </main>
  );
}
