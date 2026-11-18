import { redirect } from "next/navigation";

export default function AdminIndexPage() {
  redirect("/console/admin/nodes");
}
