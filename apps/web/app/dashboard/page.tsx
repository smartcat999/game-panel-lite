export default function DashboardPage() {
  return (
    <main className="min-h-screen bg-panel-bg p-8 text-slate-50">
      <section className="mx-auto max-w-6xl">
        <h1 className="text-2xl font-semibold">Dashboard</h1>
        <p className="mt-2 text-sm text-slate-400">Manage your Terraria servers in one place.</p>
        <div className="mt-6 grid gap-4 md:grid-cols-4">
          {["Running Servers", "Online Players", "Latest Backup", "Storage Used"].map((label) => (
            <div key={label} className="rounded-lg border border-panel-line bg-panel-card p-5">
              <p className="text-sm text-slate-400">{label}</p>
              <p className="mt-3 text-2xl font-semibold">0</p>
            </div>
          ))}
        </div>
      </section>
    </main>
  );
}
