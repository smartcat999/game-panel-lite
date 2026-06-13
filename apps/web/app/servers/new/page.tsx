import { AppShell } from "@/components/app-shell";
import { CreateServerWizard } from "@/components/create-server-wizard";

export default function NewServerPage() {
  return (
    <AppShell>
      <CreateServerWizard />
    </AppShell>
  );
}
