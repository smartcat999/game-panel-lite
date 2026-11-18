"use client";

import { ReactNode } from "react";
import FloatingToolbox from "../components/console/FloatingToolbox";
import TopBar from "../components/console/TopBar";
import { ConsoleProvider, useConsole } from "./ConsoleProvider";

function Gate({ children }: { children: ReactNode }) {
  const { ready } = useConsole();
  if (!ready) return <div className="min-h-[100dvh] bg-[#F5F7FD]" />;
  return (
    <div className="flex min-h-[100dvh] flex-col bg-[#F5F7FD] text-[#14161F] antialiased">
      <TopBar />
      <main className="mx-auto w-full max-w-[1320px] flex-1 px-4 pb-24 pt-8 sm:px-6 sm:pt-10">{children}</main>
      <FloatingToolbox />
    </div>
  );
}

export default function ConsoleLayout({ children }: { children: ReactNode }) {
  return (
    <ConsoleProvider>
      <Gate>{children}</Gate>
    </ConsoleProvider>
  );
}
