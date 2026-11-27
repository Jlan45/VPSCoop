import React, { useEffect, useState } from 'react';
import { Globe, X, RefreshCw, Route } from 'lucide-react';
import { api } from '../api';
import type { Host, NetworkReport } from '../types';

interface NetworkModalProps {
  open: boolean;
  host: Host | null;
  onClose: () => void;
  onUpdated: () => Promise<void>;
}

export const NetworkModal: React.FC<NetworkModalProps> = ({
  open,
  host,
  onClose,
  onUpdated,
}) => {
  const [loading, setLoading] = useState(false);
  const [report, setReport] = useState<NetworkReport | null>(null);
  const [error, setError] = useState('');

  const runProbe = async (targetHost: Host) => {
    setLoading(true);
    setError('');
    try {
      const res = await api.networkTest(targetHost.id);
      setReport(res);
      if (!targetHost.region && res.country) {
        await onUpdated();
      }
    } catch (err: any) {
      setError(err?.message || '网络探测失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!open || !host) return;
    setReport(null);
    runProbe(host);
  }, [open, host]);

  if (!open || !host) return null;

  const routeBadgeCls = (lineType: string) => {
    if (lineType.includes('CN2 GIA') || lineType.includes('9929') || lineType.includes('CMIN2')) {
      return 'border-emerald-500/30 bg-emerald-500/15 text-emerald-300';
    }
    if (lineType.includes('CN2 GT') || lineType.includes('4837')) {
      return 'border-sky-500/30 bg-sky-500/15 text-sky-300';
    }
    return 'border-white/[0.08] bg-white/[0.04] text-zinc-300';
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-white/[0.1] bg-[#111114] shadow-2xl">
        <div className="flex items-center justify-between border-b border-white/[0.08] px-6 py-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-400">
              <Route className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-zinc-100">
                IP 与三网链路探测 · {host.name}
              </h2>
              <p className="font-mono text-[11px] text-zinc-500">
                {host.username}@{host.host}:{host.port}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-1.5">
            <button
              type="button"
              disabled={loading}
              onClick={() => runProbe(host)}
              className="inline-flex items-center gap-1 rounded-lg border border-white/[0.08] bg-white/[0.03] px-2.5 py-1.5 text-xs text-zinc-300 hover:bg-white/[0.08] disabled:opacity-50"
            >
              <RefreshCw className={`h-3.5 w-3.5 text-emerald-400 ${loading ? 'animate-spin' : ''}`} />
              重新探测
            </button>
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg p-1.5 text-zinc-400 hover:bg-white/[0.06] hover:text-zinc-200"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>

        <div className="flex-1 space-y-5 overflow-y-auto p-6">
          {error && (
            <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-xs text-red-300">
              {error}
            </div>
          )}

          {loading && !report ? (
            <div className="py-12 text-center text-xs text-zinc-400">
              正在通过 SSH 探测出网 IP、地区 ASN 与电信/联通/移动三网链路（约 3~8 秒）...
            </div>
          ) : (
            report && (
              <>
                {/* Outbound IP & Geo */}
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <div className="rounded-xl border border-white/[0.08] bg-[#0b0b0e] p-4">
                    <div className="mb-2 flex items-center gap-1.5 text-[11px] uppercase tracking-wider text-zinc-500">
                      <Globe className="h-3.5 w-3.5 text-sky-400" />
                      <span>出网 IP 信息</span>
                    </div>
                    <div className="space-y-1.5 text-xs">
                      <div className="flex justify-between gap-2">
                        <span className="text-zinc-500">IPv4：</span>
                        <span className="font-mono text-zinc-200">{report.ipv4 || '—'}</span>
                      </div>
                      <div className="flex justify-between gap-2">
                        <span className="text-zinc-500">IPv6：</span>
                        <span className="truncate font-mono text-zinc-200">
                          {report.ipv6 || '未检测到'}
                        </span>
                      </div>
                    </div>
                  </div>

                  <div className="rounded-xl border border-white/[0.08] bg-[#0b0b0e] p-4">
                    <div className="mb-2 text-[11px] uppercase tracking-wider text-zinc-500">
                      归属地与 ASN
                    </div>
                    <div className="space-y-1.5 text-xs">
                      <div className="flex justify-between gap-2">
                        <span className="text-zinc-500">地区：</span>
                        <span className="text-zinc-200">
                          {[report.city, report.region, report.country].filter(Boolean).join(', ') || '—'}
                        </span>
                      </div>
                      <div className="flex justify-between gap-2">
                        <span className="text-zinc-500">ISP / ASN：</span>
                        <span className="truncate text-zinc-200">
                          {report.asn || report.isp || '—'}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                {/* 3-Carrier Routes */}
                <div className="space-y-3">
                  <div className="text-xs font-semibold text-zinc-300">
                    三网回程 / 出网链路（电信 · 联通 · 移动）
                  </div>
                  <div className="space-y-2.5">
                    {(report.routes || []).map((r) => (
                      <div
                        key={r.carrier}
                        className="rounded-xl border border-white/[0.08] bg-[#0b0b0e] p-3.5"
                      >
                        <div className="flex flex-wrap items-center justify-between gap-2">
                          <div className="flex items-center gap-2.5">
                            <span className="text-xs font-semibold text-zinc-100">
                              {r.carrier}
                            </span>
                            <span className="font-mono text-[11px] text-zinc-500">
                              ({r.target})
                            </span>
                          </div>
                          <div className="flex items-center gap-2">
                            {r.latencyMs ? (
                              <span className="font-mono text-[11px] text-zinc-400">
                                {r.latencyMs.toFixed(1)} ms
                              </span>
                            ) : null}
                            <span
                              className={`rounded-md border px-2 py-0.5 text-[11px] font-medium ${routeBadgeCls(
                                r.lineType
                              )}`}
                            >
                              {r.lineType}
                            </span>
                          </div>
                        </div>

                        {r.hops && r.hops.length > 0 && (
                          <div className="mt-2 font-mono text-[10px] leading-relaxed text-zinc-500">
                            {r.hops.join(' → ')}
                          </div>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              </>
            )
          )}
        </div>
      </div>
    </div>
  );
};
