import { AppShell } from "@/components/app-shell";
import { PlatformTablePage } from "@/features/prototype/platform-pages";

export default function PlatformInstancesPage() { return <AppShell area="platform"><PlatformTablePage resource="instances" /></AppShell>; }
