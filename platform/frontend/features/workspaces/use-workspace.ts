"use client";

import { useQuery } from "@tanstack/react-query";

import { controlPlane } from "@/lib/control-plane";

export function useWorkspace(workspaceSlug: string) {
  const query = useQuery({ queryKey: ["workspaces"], queryFn: controlPlane.workspaces });
  return { ...query, workspace: query.data?.find((workspace) => workspace.slug === workspaceSlug) };
}
