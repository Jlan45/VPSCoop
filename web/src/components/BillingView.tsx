import React from 'react';
import { CalendarClock, CreditCard, Receipt, Server, Terminal as TerminalIcon } from 'lucide-react';
import type { Currency, Host } from '../types';
import {
  BILLING_LABELS_ZH,
  CURRENCY_SYMBOLS,
  formatCost,
  getExpiryInfo,
  toMonthlyEquivalent,
} from '../utils/format';

interface BillingViewProps {
  hosts: Host[];
  onEditHost: (host: Host) => void;
  onOpenSsh: (host: Host) => void;
  onOpenInvoices: (host: Host) => void;
}

export const BillingView: React.FC<BillingViewProps> = ({
  hosts,
  onEditHost,
  onOpenSsh,
  onOpenInvoices,
}) => {
  const monthlyByCurrency = hosts.reduce<Record<string, number>>((acc, h) => {
    if (!h.cost) return acc;
    const m = toMonthlyEquivalent(h.cost, h.billingCycle);
    acc[h.currency] = (acc[h.currency] || 0) + m;
    return acc;
  }, {});

  const byProvider = hosts.reduce<Record<string, Host[]>>((acc, h) => {
    const key = h.provider || '未填写购买商';
    acc[key] = acc[key] || [];
    acc[key].push(h);
    return acc;
  }, {});

  const sortedByExpiry = [...hosts].sort((a, b) => {
    if (!a.expiresAt && !b.expiresAt) return 0;
    if (!a.expiresAt) return 1;
    if (!b.expiresAt) return -1;
    return a.expiresAt.localeCompare(b.expiresAt);
  });

  return (
    <div className="h-full overflow-y-auto px-6 py-6">
      <div className="mx-auto max-w-6xl space-y-6">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-zinc-100">账单与到期总览</h1>
        </div>

        {/* Currency Summary Cards */}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div className="rounded-2xl border border-white/[0.08] bg-[#111114] p-5">
            <div className="flex items-center justify-between text-xs text-zinc-400">
              <span>折算月度支出</span>
              <CreditCard className="h-4 w-4 text-emerald-400" />
            </div>
            <div className="mt-3 space-y-1">
              {Object.keys(monthlyByCurrency).length === 0 ? (
                <div className="text-lg font-semibold text-zinc-500">暂无费用记录</div>
              ) : (
                Object.entries(monthlyByCurrency).map(([cur, val]) => (
                  <div key={cur} className="flex items-baseline gap-2">
                    <span className="font-mono text-2xl font-semibold text-zinc-100">
                      {CURRENCY_SYMBOLS[cur as Currency] || ''}
                      {val.toFixed(2)}
                    </span>
                    <span className="text-xs text-zinc-500">{cur} / 月</span>
                  </div>
                ))
              )}
            </div>
          </div>

          <div className="rounded-2xl border border-white/[0.08] bg-[#111114] p-5">
            <div className="flex items-center justify-between text-xs text-zinc-400">
              <span>折算年度支出</span>
              <CalendarClock className="h-4 w-4 text-sky-400" />
            </div>
            <div className="mt-3 space-y-1">
              {Object.keys(monthlyByCurrency).length === 0 ? (
                <div className="text-lg font-semibold text-zinc-500">暂无费用记录</div>
              ) : (
                Object.entries(monthlyByCurrency).map(([cur, val]) => (
                  <div key={cur} className="flex items-baseline gap-2">
                    <span className="font-mono text-2xl font-semibold text-zinc-100">
                      {CURRENCY_SYMBOLS[cur as Currency] || ''}
                      {(val * 12).toFixed(2)}
                    </span>
                    <span className="text-xs text-zinc-500">{cur} / 年</span>
                  </div>
                ))
              )}
            </div>
          </div>

          <div className="rounded-2xl border border-white/[0.08] bg-[#111114] p-5">
            <div className="flex items-center justify-between text-xs text-zinc-400">
              <span>购买商分布</span>
              <Server className="h-4 w-4 text-violet-400" />
            </div>
            <div className="mt-3 flex flex-wrap gap-1.5">
              {Object.entries(byProvider).map(([provider, list]) => (
                <span
                  key={provider}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-white/[0.08] bg-white/[0.03] px-2.5 py-1 text-xs text-zinc-300"
                >
                  <span>{provider}</span>
                  <span className="font-mono text-zinc-500">{list.length}</span>
                </span>
              ))}
            </div>
          </div>
        </div>

        {/* Expiry Table */}
        <div className="overflow-hidden rounded-2xl border border-white/[0.08] bg-[#111114]">
          <div className="border-b border-white/[0.08] px-5 py-3.5">
            <h2 className="text-sm font-semibold text-zinc-100">到期时间排序</h2>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-white/[0.08] bg-white/[0.02] text-[11px] uppercase tracking-wider text-zinc-500">
                <tr>
                  <th className="px-5 py-3 font-medium">主机名称</th>
                  <th className="px-4 py-3 font-medium">购买商</th>
                  <th className="px-4 py-3 font-medium">费用 / 周期</th>
                  <th className="px-4 py-3 font-medium">到期日期</th>
                  <th className="px-4 py-3 font-medium">状态</th>
                  <th className="px-5 py-3 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-white/[0.05]">
                {sortedByExpiry.map((h) => {
                  const exp = getExpiryInfo(h.expiresAt);
                  const badgeCls = {
                    none: 'bg-zinc-800 text-zinc-400',
                    healthy: 'bg-emerald-500/15 text-emerald-300 border border-emerald-500/20',
                    soon: 'bg-amber-500/15 text-amber-300 border border-amber-500/20',
                    urgent: 'bg-red-500/15 text-red-300 border border-red-500/25',
                    expired: 'bg-red-500/20 text-red-200 border border-red-500/30',
                  }[exp.status];

                  return (
                    <tr key={h.id} className="transition hover:bg-white/[0.02]">
                      <td className="px-5 py-3">
                        <div className="font-medium text-zinc-100">{h.name}</div>
                        <div className="font-mono text-[11px] text-zinc-500">{h.host}</div>
                      </td>
                      <td className="px-4 py-3 text-zinc-300">
                        {h.provider || <span className="text-zinc-600">—</span>}
                      </td>
                      <td className="px-4 py-3 font-mono text-zinc-200">
                        {h.cost > 0 ? (
                          <span>
                            {formatCost(h.cost, h.currency, h.billingCycle)}{' '}
                            <span className="text-[11px] text-zinc-500">
                              ({BILLING_LABELS_ZH[h.billingCycle]})
                            </span>
                          </span>
                        ) : (
                          <span className="text-zinc-600">—</span>
                        )}
                      </td>
                      <td className="px-4 py-3 font-mono text-zinc-300">
                        {h.expiresAt || <span className="text-zinc-600">—</span>}
                      </td>
                      <td className="px-4 py-3">
                        <span className={`inline-flex rounded-full px-2.5 py-0.5 text-[11px] ${badgeCls}`}>
                          {exp.label}
                        </span>
                      </td>
                      <td className="px-5 py-3 text-right">
                        <div className="inline-flex items-center gap-1.5">
                          <button
                            type="button"
                            onClick={() => onOpenInvoices(h)}
                            className="inline-flex items-center gap-1 rounded-md border border-white/[0.08] bg-white/[0.03] px-2 py-1 text-[11px] text-zinc-300 hover:bg-white/[0.08]"
                          >
                            <Receipt className="h-3 w-3 text-sky-400" />
                            Invoice
                          </button>
                          <button
                            type="button"
                            onClick={() => onOpenSsh(h)}
                            className="inline-flex items-center gap-1 rounded-md border border-white/[0.08] bg-white/[0.03] px-2 py-1 text-[11px] text-zinc-300 hover:bg-white/[0.08]"
                          >
                            <TerminalIcon className="h-3 w-3 text-emerald-400" />
                            SSH
                          </button>
                          <button
                            type="button"
                            onClick={() => onEditHost(h)}
                            className="rounded-md border border-white/[0.08] bg-white/[0.03] px-2 py-1 text-[11px] text-zinc-300 hover:bg-white/[0.08]"
                          >
                            编辑
                          </button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
};
