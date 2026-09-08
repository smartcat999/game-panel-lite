"use client";

import { useState, useEffect } from "react";
import { createPortal } from "react-dom";
import { useQuery } from "@tanstack/react-query";
import { Coins, History, ArrowUpRight, ArrowDownLeft, X, Receipt } from "lucide-react";
import { getUserCredits } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export function CreditsBadge() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const [modalOpen, setModalOpen] = useState(false);
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
  }, []);

  const { data, isLoading } = useQuery({
    queryKey: ["user-credits"],
    queryFn: () => getUserCredits(),
    refetchInterval: 15000,
    staleTime: 5000,
  });

  const credits = data?.credits ?? 0;

  return (
    <>
      <button
        type="button"
        onClick={() => setModalOpen(true)}
        className="flex items-center gap-1.5 rounded-lg border border-amber-500/30 bg-amber-500/10 px-2.5 py-1.5 text-xs font-semibold text-amber-300 hover:bg-amber-500/20 hover:border-amber-500/50 transition focus:outline-none focus:ring-1 focus:ring-amber-500/50"
        title={isZh ? "查看点券余额与账单流水" : "View Credits & Transactions"}
      >
        <Coins className="size-3.5 text-amber-400 shrink-0" />
        <span className="font-mono">
          {isLoading ? "..." : `${credits} ${isZh ? "点券" : "Credits"}`}
        </span>
      </button>

      {mounted && modalOpen && createPortal(
        <div
          className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 animate-in fade-in duration-150"
          onClick={(e) => {
            if (e.target === e.currentTarget) setModalOpen(false);
          }}
        >
          <div className="w-full max-w-md rounded-2xl border border-slate-700 bg-[#0d131f] p-5 sm:p-6 shadow-2xl shadow-black/90 animate-in zoom-in-95 duration-150 flex flex-col max-h-[90vh]">
            {/* Modal Header */}
            <div className="flex items-center justify-between border-b border-slate-800/80 pb-4 shrink-0">
              <div className="flex items-center gap-3">
                <div className="flex size-9 items-center justify-center rounded-xl bg-amber-500/15 text-amber-400 border border-amber-500/30">
                  <Coins className="size-5" />
                </div>
                <div>
                  <h3 className="text-sm font-bold text-white tracking-tight">
                    {isZh ? "账户额度与账单流水" : "Credits & Transaction History"}
                  </h3>
                  <p className="text-[11px] text-slate-400 font-mono mt-0.5">
                    {data?.organizationName ? `${isZh ? "工作空间: " : "Workspace: "}${data.organizationName}` : (isZh ? "个人账户空间" : "Personal Workspace")}
                  </p>
                </div>
              </div>
              <button
                type="button"
                onClick={() => setModalOpen(false)}
                className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-white transition"
              >
                <X className="size-4" />
              </button>
            </div>

            {/* Scrollable Body */}
            <div className="overflow-y-auto py-4 space-y-4 pr-0.5 flex-1">
              {/* Balance banner */}
              <div className="flex items-center justify-between rounded-xl border border-amber-500/30 bg-gradient-to-br from-amber-500/15 via-amber-950/20 to-slate-900/50 p-4 shadow-inner">
                <div>
                  <span className="text-[11px] text-slate-400 font-medium">
                    {isZh ? "可用点券余额" : "Available Balance"}
                  </span>
                  <div className="text-2xl font-black text-amber-300 font-mono mt-0.5 tracking-tight">
                    {credits} <span className="text-xs text-amber-400/80 font-normal">{isZh ? "点券" : "pts"}</span>
                  </div>
                </div>
                <div className="text-right">
                  <span className="inline-block px-2.5 py-0.5 rounded-full text-[10px] font-medium bg-panel-green/20 text-panel-green border border-panel-green/30">
                    {isZh ? "就绪可用" : "Ready"}
                  </span>
                  <p className="text-[10px] text-slate-400 font-mono mt-1">
                    {isZh ? "创建实例抵扣点券" : "Per instance deduction"}
                  </p>
                </div>
              </div>

              {/* Transactions History */}
              <div className="space-y-2">
                <div className="flex items-center justify-between text-xs font-semibold text-slate-300 px-0.5">
                  <div className="flex items-center gap-1.5">
                    <History className="size-3.5 text-slate-400" />
                    <span>{isZh ? "收支明细" : "Recent Transactions"}</span>
                  </div>
                  {data?.transactions && data.transactions.length > 0 && (
                    <span className="text-[10px] text-slate-500 font-mono">
                      {data.transactions.length} {isZh ? "笔记录" : "records"}
                    </span>
                  )}
                </div>

                <div className="max-h-56 overflow-y-auto space-y-1.5 pr-0.5">
                  {(!data?.transactions || data.transactions.length === 0) ? (
                    <div className="flex flex-col items-center justify-center py-8 px-4 text-center rounded-xl border border-dashed border-slate-800 bg-slate-950/40">
                      <div className="flex size-10 items-center justify-center rounded-xl bg-slate-900 text-slate-500 border border-slate-800 mb-2">
                        <Receipt className="size-5" />
                      </div>
                      <p className="text-xs font-medium text-slate-400">
                        {isZh ? "暂无账单流水记录" : "No transaction records found"}
                      </p>
                      <p className="text-[10px] text-slate-500 mt-0.5">
                        {isZh ? "后续创建或续期服务器的抵扣明细将展示在此处" : "Instance creation & renewal transactions will appear here"}
                      </p>
                    </div>
                  ) : (
                    data.transactions.map((tx) => {
                      const isPlus = tx.amount > 0;
                      return (
                        <div
                          key={tx.id}
                          className="flex items-center justify-between p-2.5 rounded-lg border border-slate-800 bg-slate-950/80 hover:border-slate-700 text-xs transition"
                        >
                          <div className="flex items-center gap-2.5 min-w-0">
                            <div className={cn("flex size-7 shrink-0 items-center justify-center rounded-md", isPlus ? "bg-panel-green/20 text-panel-green" : "bg-rose-500/20 text-rose-400")}>
                              {isPlus ? <ArrowDownLeft className="size-3.5" /> : <ArrowUpRight className="size-3.5" />}
                            </div>
                            <div className="min-w-0">
                              <div className="font-medium text-slate-200 truncate">{tx.description}</div>
                              <div className="text-[10px] text-slate-500 font-mono">
                                {new Date(tx.createdAt).toLocaleString(isZh ? "zh-CN" : "en-US")}
                              </div>
                            </div>
                          </div>
                          <div className="text-right shrink-0 font-mono">
                            <span className={cn("font-bold text-xs", isPlus ? "text-panel-green" : "text-rose-400")}>
                              {isPlus ? `+${tx.amount}` : tx.amount}
                            </span>
                            <div className="text-[10px] text-slate-500">
                              {isZh ? "结余: " : "Bal: "}{tx.balanceAfter}
                            </div>
                          </div>
                        </div>
                      );
                    })
                  )}
                </div>
              </div>
            </div>

            {/* Modal Footer */}
            <div className="pt-3 border-t border-slate-800/80 flex justify-end shrink-0">
              <button
                type="button"
                onClick={() => setModalOpen(false)}
                className="px-4 py-1.5 rounded-lg border border-slate-700 bg-slate-800 text-xs font-medium text-slate-200 hover:bg-slate-700 hover:text-white transition"
              >
                {isZh ? "关闭" : "Close"}
              </button>
            </div>
          </div>
        </div>,
        document.body
      )}
    </>
  );
}
