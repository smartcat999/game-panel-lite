import Link from "next/link";
import { Card } from "@/components/ui";

export default function NotFoundPage() {
  return (
    <Card className="p-6">
      <h1 className="text-xl font-semibold text-white">Page not found</h1>
      <p className="mt-2 text-sm text-slate-400">The requested GamePanel Lite page does not exist.</p>
      <Link className="mt-4 inline-flex text-sm font-medium text-panel-green hover:underline" href="/dashboard">
        Back to dashboard
      </Link>
    </Card>
  );
}
