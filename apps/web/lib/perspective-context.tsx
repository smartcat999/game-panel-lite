"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { usePermissions } from "./permissions";

export type PerspectiveRole = "user" | "admin" | "super";

interface PerspectiveContextValue {
  perspective: PerspectiveRole;
  setPerspective: (role: PerspectiveRole) => void;
  isMemberView: boolean;
  isWorkspaceAdminView: boolean;
  isSuperAdminView: boolean;
}

const PerspectiveContext = createContext<PerspectiveContextValue | null>(null);

export function PerspectiveProvider({ children }: { children: ReactNode }) {
  const { role: actualRole } = usePermissions();
  const [perspective, setPerspectiveState] = useState<PerspectiveRole>("admin");

  useEffect(() => {
    const saved = window.localStorage.getItem("gamepanel.perspective") as PerspectiveRole | null;
    if (saved && ["user", "admin", "super"].includes(saved)) {
      setPerspectiveState(saved);
    } else if (actualRole === "viewer") {
      setPerspectiveState("user");
    } else {
      setPerspectiveState("admin");
    }
  }, [actualRole]);

  const setPerspective = (role: PerspectiveRole) => {
    setPerspectiveState(role);
    window.localStorage.setItem("gamepanel.perspective", role);
  };

  return (
    <PerspectiveContext.Provider
      value={{
        perspective,
        setPerspective,
        isMemberView: perspective === "user",
        isWorkspaceAdminView: perspective === "admin",
        isSuperAdminView: perspective === "super"
      }}
    >
      {children}
    </PerspectiveContext.Provider>
  );
}

export function usePerspective() {
  const ctx = useContext(PerspectiveContext);
  if (!ctx) {
    return {
      perspective: "admin" as PerspectiveRole,
      setPerspective: () => {},
      isMemberView: false,
      isWorkspaceAdminView: true,
      isSuperAdminView: false
    };
  }
  return ctx;
}
