"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Coins, History, ArrowUpRight, ArrowDownLeft, X } from "lucide-react";
import { getUserCredits } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export function CreditsBadge() {
  const { locale } = useI18n();
  const isZh = locale.startsWith("zh");
  const [modalOpen, setModalOpen] = useState(false);

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

      {modalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="w-full max-w-lg rounded-2xl border border-slate-700 bg-slate-900 p-6 shadow-2xl animate-in fade-in zoom-in-95">
            <div className="flex items-center justify-between border-b border-slate-800 pb-4">
              <div className="flex items-center gap-2">
                <div className="p-2 rounded-xl bg-amber-500/20 text-amber-400">
                  <Coins className="size-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-white">
                    {isZh ? "账户额度与账单流水" : "Credits & Transaction History"}
                  </h3>
                  <p className="text-xs text-slate-400">
                    {data?.organizationName ? `${isZh ? "租户空间: " : "Workspace: "}${data.organizationName}` : ""}
                  </p>
                </div>
              </div>
              <button
                type="button"
                onClick={() => setModalOpen(false)}
                className="rounded-lg p-1.5 text-slate-400 hover:bg-slate-800 hover:text-white"
              >
                <X className="size-4" />
              </button>
            </div>

            {/* Balance banner */}
            <div className="my-5 flex items-center justify-between rounded-xl border border-amber-500/30 bg-gradient-to-r from-amber-500/10 to-amber-600/10 p-4">
              <div>
                <span className="text-xs text-slate-400 font-medium">
                  {isZh ? "可用点券余额" : "Available Balance"}
                </span>
                <div className="text-2xl font-black text-amber-300 font-mono mt-0.5">
                  {credits} <span className="text-xs text-amber-400/80 font-normal">{isZh ? "点券" : "pts"}</span>
                </div>
              </div>
              <div className="text-right">
                <span className="inline-block px-2.5 py-1 rounded-full text-[11px] font-medium bg-panel-green/20 text-panel-green border border-panel-green/30">
                  {isZh ? "创建实例抵扣" : "Instance Creation Ready"}
                </span>
                <p className="text-[11px] text-slate-400 mt-1">
                  {isZh ? "标准实例: 10点券/个" : "10 pts / instance"}
                </p>
              </div>
            </div>

            {/* Transactions History */}
            <div>
              <div className="flex items-center gap-1.5 text-xs font-semibold text-slate-300 mb-2">
                <History className="size-3.5 text-slate-400" />
                <span>{isZh ? "收支明细" : "Recent Transactions"}</span>
              </div>

              <div className="max-h-60 overflow-y-auto space-y-1.5 pr-1">
                {(!data?.transactions || data.transactions.length === 0) ? (
                  <div className="text-center py-6 text-xs text-slate-500 border border-dashed border-slate-800 rounded-xl">
                    {isZh ? "暂无流水明细" : "No transaction records found"}
                  </div>
                ) : (
                  data.transactions.map((tx) => {
                    const isPlus = tx.amount > 0;
                    return (
                      <div
                        key={tx.id}
                        className="flex items-center justify-between p-2.5 rounded-lg border border-slate-800 bg-slate-950/60 text-xs"
                      >
                        <div className="flex items-center gap-2">
                          <div className={cn("p-1 rounded-md", isPlus ? "bg-panel-green/20 text-panel-green" : "bg-rose-500/20 text-rose-400")}>
                            {isPlus ? <ArrowDownLeft className="size-3.5" /> : <ArrowUpRight className="size-3.5" />}
                          </div>
                          <div>
                            <div className="font-medium text-slate-200">{tx.description}</div>
                            <div className="text-[10px] text-slate-500 font-mono">
                              {new Date(tx.createdAt).toLocaleString(isZh ? "zh-CN" : "en-US")}
                            </div>
                          </div>
                        </div>
                        <div className="text-right">
                          <span className={cn("font-bold font-mono", isPlus ? "text-panel-green" : "text-rose-400")}>
                            {isPlus ? `+${tx.amount}` : tx.amount}
                          </span>
                          <div className="text-[10px] text-slate-500 font-mono">
                            {isZh ? "结余: " : "Bal: "}{tx.balanceAfter}
                          </div>
                        </div>
                      </div>
                    );
                  })
                )}
              </div>
            </div>

            <div className="mt-5 flex justify-end border-t border-slate-800 pt-4">
              <button
                type="button"
                onClick={() => setModalOpen(false)}
                className="px-4 py-1.5 rounded-lg border border-slate-700 bg-slate-800 text-xs text-slate-200 hover:bg-slate-700 transition"
              >
                {isZh ? "关闭" : "Close"}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
