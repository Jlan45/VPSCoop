import React, { useCallback, useEffect, useState } from 'react';
import {
  ArrowUpCircle,
  CheckCircle2,
  ExternalLink,
  Loader2,
  RefreshCw,
  AlertTriangle,
  X,
} from 'lucide-react';
import { api } from '../api';

export interface UpgradeInfo {
  currentVersion: string;
  latestVersion: string;
  hasUpdate: boolean;
  releaseName: string;
  releaseNotes: string;
  publishedAt: string;
  htmlUrl: string;
  assetName: string;
  downloadUrl: string;
  assetSize: number;
  canSelfUpdate: boolean;
  reason?: string;
  execPath: string;
  repo: string;
}

interface UpgradeModalProps {
  open: boolean;
  initialInfo?: UpgradeInfo | null;
  onClose: () => void;
  onInfoUpdated?: (info: UpgradeInfo) => void;
}

function formatBytes(bytes: number): string {
  if (!bytes) return '-';
  const mb = bytes / (1024 * 1024);
  return `${mb.toFixed(1)} MB`;
}

export const UpgradeModal: React.FC<UpgradeModalProps> = ({
  open,
  initialInfo,
  onClose,
  onInfoUpdated,
}) => {
  const [info, setInfo] = useState<UpgradeInfo | null>(initialInfo || null);
  const [checking, setChecking] = useState(false);
  const [applying, setApplying] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [error, setError] = useState('');

  const check = useCallback(async () => {
    setChecking(true);
    setError('');
    try {
      const res = await api.checkUpgrade();
      setInfo(res);
      onInfoUpdated?.(res);
    } catch (err: any) {
      setError(err?.message || '检查更新失败');
    } finally {
      setChecking(false);
    }
  }, [onInfoUpdated]);

  useEffect(() => {
    if (!open) return;
    if (initialInfo) {
      setInfo(initialInfo);
    }
    check();
  }, [open, initialInfo, check]);

  const waitForRestart = useCallback((targetVersion: string) => {
    setRestarting(true);
    let attempts = 0;
    const poll = async () => {
      attempts += 1;
      try {
        const res = await fetch('/api/health', { cache: 'no-store' });
        if (res.ok) {
          const data = await res.json();
          if (data?.status === 'ok') {
            window.location.reload();
            return;
          }
        }
      } catch {
        // server restarting
      }
      if (attempts < 40) {
        window.setTimeout(poll, 1500);
      } else {
        setRestarting(false);
        setApplying(false);
        setError(`服务重连超时，请手动刷新页面确认是否已升级至 ${targetVersion}`);
      }
    };
    window.setTimeout(poll, 1500);
  }, []);

  const handleApply = async (force = false) => {
    setApplying(true);
    setError('');
    try {
      const res = await api.applyUpgrade(force);
      waitForRestart(res.version);
    } catch (err: any) {
      setApplying(false);
      setError(err?.message || '升级失败');
    }
  };

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-2xl border border-white/[0.1] bg-[#111115] shadow-2xl">
        <div className="flex items-center justify-between border-b border-white/[0.08] px-5 py-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-emerald-500/15 ring-1 ring-emerald-500/30">
              <ArrowUpCircle className="h-4 w-4 text-emerald-400" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-zinc-100">系统版本与在线升级</h3>
              <p className="text-[11px] text-zinc-500">从 GitHub Releases 检查并应用二进制更新</p>
            </div>
          </div>
          <button
            type="button"
            disabled={applying || restarting}
            onClick={onClose}
            className="rounded-lg p-1.5 text-zinc-400 transition hover:bg-white/[0.06] hover:text-zinc-200 disabled:opacity-40"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="space-y-4 p-5">
          {error && (
            <div className="flex items-start gap-2 rounded-xl border border-rose-500/25 bg-rose-500/10 px-3.5 py-2.5 text-xs text-rose-200">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-rose-400" />
              <span className="break-all">{error}</span>
            </div>
          )}

          {restarting ? (
            <div className="flex flex-col items-center justify-center gap-3 rounded-xl border border-emerald-500/20 bg-emerald-500/5 py-8 text-center">
              <Loader2 className="h-7 w-7 animate-spin text-emerald-400" />
              <div>
                <div className="text-sm font-medium text-emerald-200">升级包已就绪，正在重启服务...</div>
                <div className="mt-1 text-xs text-zinc-400">服务恢复后将自动刷新页面</div>
              </div>
            </div>
          ) : (
            <>
              <div className="grid grid-cols-2 gap-3">
                <div className="rounded-xl border border-white/[0.08] bg-white/[0.02] p-3">
                  <div className="text-[11px] text-zinc-500">当前版本</div>
                  <div className="mt-1 font-mono text-base font-semibold text-zinc-100">
                    {info?.currentVersion || '-'}
                  </div>
                </div>
                <div className="rounded-xl border border-white/[0.08] bg-white/[0.02] p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-[11px] text-zinc-500">最新版本</span>
                    {info?.hasUpdate ? (
                      <span className="rounded-full bg-emerald-500/20 px-2 py-0.5 text-[10px] font-medium text-emerald-300">
                        可升级
                      </span>
                    ) : info ? (
                      <span className="inline-flex items-center gap-1 text-[10px] text-zinc-400">
                        <CheckCircle2 className="h-3 w-3 text-emerald-400" />
                        已是最新
                      </span>
                    ) : null}
                  </div>
                  <div className="mt-1 font-mono text-base font-semibold text-emerald-400">
                    {checking && !info ? '检查中...' : info?.latestVersion || '-'}
                  </div>
                </div>
              </div>

              {info && (
                <div className="space-y-2 rounded-xl border border-white/[0.07] bg-black/30 p-3 text-xs text-zinc-400">
                  <div className="flex items-center justify-between gap-2">
                    <span>构建产物</span>
                    <span className="font-mono text-zinc-200">
                      {info.assetName} ({formatBytes(info.assetSize)})
                    </span>
                  </div>
                  <div className="flex items-center justify-between gap-2">
                    <span>程序路径</span>
                    <span className="truncate font-mono text-zinc-300" title={info.execPath}>
                      {info.execPath || '-'}
                    </span>
                  </div>
                  {info.htmlUrl && (
                    <div className="flex items-center justify-between gap-2">
                      <span>发布页面</span>
                      <a
                        href={info.htmlUrl}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex items-center gap-1 text-emerald-400 hover:underline"
                      >
                        GitHub Release
                        <ExternalLink className="h-3 w-3" />
                      </a>
                    </div>
                  )}
                </div>
              )}

              {info?.releaseNotes && (
                <div>
                  <div className="mb-1.5 text-[11px] font-medium text-zinc-400">更新日志</div>
                  <pre className="max-h-40 overflow-y-auto whitespace-pre-wrap rounded-xl border border-white/[0.07] bg-black/40 p-3 font-mono text-[11px] leading-relaxed text-zinc-300">
                    {info.releaseNotes}
                  </pre>
                </div>
              )}

              {info && !info.canSelfUpdate && info.reason && (
                <div className="flex items-start gap-2 rounded-xl border border-amber-500/25 bg-amber-500/10 px-3.5 py-2.5 text-xs text-amber-200">
                  <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-400" />
                  <span>{info.reason}</span>
                </div>
              )}
            </>
          )}
        </div>

        <div className="flex items-center justify-between border-t border-white/[0.08] px-5 py-3.5">
          <button
            type="button"
            disabled={checking || applying || restarting}
            onClick={check}
            className="inline-flex items-center gap-1.5 rounded-xl border border-white/[0.08] bg-white/[0.04] px-3 py-2 text-xs font-medium text-zinc-300 transition hover:bg-white/[0.08] disabled:opacity-40"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${checking ? 'animate-spin' : ''}`} />
            重新检查
          </button>

          <div className="flex items-center gap-2">
            {info && !info.hasUpdate && info.canSelfUpdate && !restarting && (
              <button
                type="button"
                disabled={applying || checking}
                onClick={() => handleApply(true)}
                className="rounded-xl border border-white/[0.08] bg-white/[0.04] px-3 py-2 text-xs font-medium text-zinc-300 transition hover:bg-white/[0.08] disabled:opacity-40"
              >
                {applying ? '正在重装...' : '强制重装'}
              </button>
            )}
            {info?.hasUpdate && !restarting && (
              <button
                type="button"
                disabled={!info.canSelfUpdate || applying || checking}
                onClick={() => handleApply(false)}
                className="inline-flex items-center gap-1.5 rounded-xl bg-emerald-500 px-4 py-2 text-xs font-semibold text-zinc-950 transition hover:bg-emerald-400 disabled:cursor-not-allowed disabled:opacity-40"
              >
                {applying ? (
                  <>
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    下载并升级中...
                  </>
                ) : (
                  <>
                    <ArrowUpCircle className="h-3.5 w-3.5" />
                    立即升级并重启
                  </>
                )}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
