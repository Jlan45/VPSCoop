import React, { useEffect, useState } from 'react';
import {
  Network,
  X,
  CheckCircle2,
  AlertCircle,
  SearchCheck,
  Zap,
} from 'lucide-react';
import { api } from '../api';
import type { Host, TailscaleStatus } from '../types';

interface TailscaleModalProps {
  open: boolean;
  host: Host | null;
  onClose: () => void;
  onUpdated: () => Promise<void>;
}

export const TailscaleModal: React.FC<TailscaleModalProps> = ({
  open,
  host,
  onClose,
  onUpdated,
}) => {
  const [hasSavedKey, setHasSavedKey] = useState(false);
  const [authKey, setAuthKey] = useState('');
  const [saveAuthKey, setSaveAuthKey] = useState(true);
  const [hostname, setHostname] = useState('');
  const [enableSSH, setEnableSSH] = useState(false);

  const [checking, setChecking] = useState(false);
  const [injecting, setInjecting] = useState(false);
  const [status, setStatus] = useState<TailscaleStatus | null>(null);
  const [synced, setSynced] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open || !host) return;
    setError('');
    setStatus(null);
    setSynced(false);
    setAuthKey('');
    setHostname(
      host.name
        .toLowerCase()
        .replace(/[^a-z0-9-]+/g, '-')
        .replace(/^-+|-+$/g, '') || 'vpscoop-node'
    );
    api
      .getTailscaleSetting()
      .then((r) => setHasSavedKey(r.hasAuthKey))
      .catch(() => setHasSavedKey(false));
  }, [open, host]);

  if (!open || !host) return null;

  const handleCheck = async () => {
    setError('');
    setSynced(false);
    setChecking(true);
    try {
      const res = await api.checkTailscale(host.id);
      setStatus(res.status);
      setSynced(res.synced);
      if (res.synced) {
        await onUpdated();
      }
    } catch (err: any) {
      setError(err?.message || '检测 Tailscale 失败');
    } finally {
      setChecking(false);
    }
  };

  const handleInject = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!authKey.trim() && !hasSavedKey) {
      setError('请输入 Tailscale Auth Key (tskey-auth-...)');
      return;
    }
    setError('');
    setSynced(false);
    setInjecting(true);
    try {
      const res = await api.injectTailscale(host.id, {
        authKey: authKey.trim(),
        saveAuthKey,
        hostname: hostname.trim(),
        enableSSH,
      });
      setStatus(res.status);
      setSynced(res.synced);
      if (saveAuthKey && authKey.trim()) {
        setHasSavedKey(true);
      }
      if (res.synced) {
        await onUpdated();
      }
    } catch (err: any) {
      setError(err?.message || 'Tailscale 自动注入失败');
    } finally {
      setInjecting(false);
    }
  };

  const inputCls =
    'w-full rounded-lg border border-white/[0.08] bg-zinc-900/80 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 outline-none transition focus:border-emerald-500/60';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="w-full max-w-lg overflow-hidden rounded-2xl border border-white/[0.1] bg-[#111114] shadow-2xl">
        <div className="flex items-center justify-between border-b border-white/[0.08] px-6 py-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-sky-500/15 text-sky-400">
              <Network className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-zinc-100">
                Tailscale 自动组网与 IP 同步 · {host.name}
              </h2>
              <p className="font-mono text-[11px] text-zinc-500">
                {host.username}@{host.host}:{host.port}
              </p>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg p-1.5 text-zinc-400 hover:bg-white/[0.06] hover:text-zinc-200"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <form onSubmit={handleInject} className="space-y-4 p-6">
          <div>
            <label className="mb-1.5 block text-xs font-medium text-zinc-400">
              Tailscale Auth Key {hasSavedKey ? '（已保存默认 Auth Key，留空直接使用）' : '*'}
            </label>
            <input
              type="password"
              value={authKey}
              onChange={(e) => setAuthKey(e.target.value)}
              placeholder={
                hasSavedKey
                  ? '••••••••••••（留空使用已保存的 Auth Key）'
                  : 'tskey-auth-...'
              }
              className={`${inputCls} font-mono`}
            />
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label className="mb-1.5 block text-xs font-medium text-zinc-400">
                节点名称 (--hostname)
              </label>
              <input
                value={hostname}
                onChange={(e) => setHostname(e.target.value)}
                placeholder="la-cn2-node"
                className={`${inputCls} font-mono`}
              />
            </div>
            <div className="flex flex-col justify-end gap-2 pb-1">
              <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300">
                <input
                  type="checkbox"
                  checked={saveAuthKey}
                  onChange={(e) => setSaveAuthKey(e.target.checked)}
                  className="h-3.5 w-3.5 rounded accent-emerald-500"
                />
                <span>加密记住此 Auth Key</span>
              </label>
              <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300">
                <input
                  type="checkbox"
                  checked={enableSSH}
                  onChange={(e) => setEnableSSH(e.target.checked)}
                  className="h-3.5 w-3.5 rounded accent-emerald-500"
                />
                <span>同时开启 Tailscale SSH (--ssh)</span>
              </label>
            </div>
          </div>

          {error && (
            <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-xs text-red-300">
              {error}
            </div>
          )}

          {status && (
            <div className="space-y-2 rounded-xl border border-white/[0.08] bg-[#09090b] p-4 text-xs">
              <div className="flex items-center justify-between">
                <span className="text-zinc-400">安装状态：</span>
                {status.installed ? (
                  <span className="inline-flex items-center gap-1 text-emerald-400">
                    <CheckCircle2 className="h-3.5 w-3.5" />
                    已安装
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1 text-amber-400">
                    <AlertCircle className="h-3.5 w-3.5" />
                    未安装
                  </span>
                )}
              </div>
              <div className="flex items-center justify-between">
                <span className="text-zinc-400">Tailscale IPv4：</span>
                <span className="font-mono text-zinc-200">
                  {status.tailscaleIp || '未分配 / 未连接'}
                </span>
              </div>
              {synced && (
                <div className="rounded bg-emerald-500/10 px-2.5 py-1.5 text-[11px] text-emerald-300">
                  已自动将 {status.tailscaleIp} 添加为该主机的「Tailscale」备用登录入口！
                </div>
              )}
            </div>
          )}

          <div className="flex items-center justify-end gap-2.5 pt-2">
            <button
              type="button"
              disabled={checking || injecting}
              onClick={handleCheck}
              className="inline-flex items-center gap-1.5 rounded-lg border border-white/[0.08] bg-white/[0.04] px-3.5 py-2 text-xs font-medium text-zinc-200 transition hover:bg-white/[0.08] disabled:opacity-50"
            >
              <SearchCheck className="h-3.5 w-3.5 text-sky-400" />
              {checking ? '检测中...' : '检测并同步 IP'}
            </button>
            <button
              type="submit"
              disabled={checking || injecting}
              className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-500 px-4 py-2 text-xs font-semibold text-zinc-950 transition hover:bg-emerald-400 disabled:opacity-50"
            >
              <Zap className="h-3.5 w-3.5" />
              {injecting ? '正在安装并组网...' : '一键安装并加入 Tailnet'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
