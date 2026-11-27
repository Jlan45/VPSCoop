import React, { useEffect, useState } from 'react';
import {
  KeyRound,
  X,
  ShieldCheck,
  ShieldAlert,
  CheckCircle2,
  XCircle,
  Syringe,
  SearchCheck,
} from 'lucide-react';
import { api } from '../api';
import type { Host, KeyCheckResult, SSHKey } from '../types';

interface KeyInjectModalProps {
  open: boolean;
  host: Host | null;
  savedKeys: SSHKey[];
  onClose: () => void;
  onUpdated: () => Promise<void>;
}

export const KeyInjectModal: React.FC<KeyInjectModalProps> = ({
  open,
  host,
  savedKeys,
  onClose,
  onUpdated,
}) => {
  const [selectedKeyId, setSelectedKeyId] = useState('');
  const [switchHostAuth, setSwitchHostAuth] = useState(true);
  const [checking, setChecking] = useState(false);
  const [injecting, setInjecting] = useState(false);
  const [result, setResult] = useState<KeyCheckResult | null>(null);
  const [switchedNotice, setSwitchedNotice] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open || !host) return;
    setError('');
    setResult(null);
    setSwitchedNotice(false);
    setSelectedKeyId(host.keyId || savedKeys[0]?.id || '');
  }, [open, host, savedKeys]);

  if (!open || !host) return null;

  const handleCheck = async () => {
    if (!selectedKeyId) return;
    setError('');
    setSwitchedNotice(false);
    setChecking(true);
    try {
      const res = await api.checkHostKey(host.id, selectedKeyId);
      setResult(res);
    } catch (err: any) {
      setError(err?.message || '检测失败');
    } finally {
      setChecking(false);
    }
  };

  const handleInject = async () => {
    if (!selectedKeyId) return;
    setError('');
    setSwitchedNotice(false);
    setInjecting(true);
    try {
      const res = await api.injectHostKey(host.id, selectedKeyId, switchHostAuth);
      setResult(res.result);
      setSwitchedNotice(res.switched);
      if (res.switched) {
        await onUpdated();
      }
    } catch (err: any) {
      setError(err?.message || '自动注入失败');
    } finally {
      setInjecting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="w-full max-w-lg overflow-hidden rounded-2xl border border-white/[0.1] bg-[#111114] shadow-2xl">
        <div className="flex items-center justify-between border-b border-white/[0.08] px-6 py-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-violet-500/15 text-violet-400">
              <KeyRound className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-zinc-100">
                密钥自动注入与检测 · {host.name}
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

        <div className="space-y-4 p-6">
          {savedKeys.length === 0 ? (
            <div className="rounded-xl border border-dashed border-white/[0.1] bg-zinc-900/40 p-5 text-center text-xs text-zinc-400">
              密钥库中还没有任何密钥，请先前往左侧「密钥管理」生成或导入一个密钥对。
            </div>
          ) : (
            <>
              <div>
                <label className="mb-1.5 block text-xs font-medium text-zinc-400">
                  选择密钥库中的密钥
                </label>
                <select
                  value={selectedKeyId}
                  onChange={(e) => {
                    setSelectedKeyId(e.target.value);
                    setResult(null);
                    setSwitchedNotice(false);
                  }}
                  className="w-full rounded-lg border border-white/[0.08] bg-zinc-900/80 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-violet-500/60"
                >
                  {savedKeys.map((k) => (
                    <option key={k.id} value={k.id}>
                      {k.name} ({k.algorithm} · {k.fingerprint.slice(0, 18)}...)
                    </option>
                  ))}
                </select>
              </div>

              <label className="flex cursor-pointer items-center gap-2 text-xs text-zinc-300">
                <input
                  type="checkbox"
                  checked={switchHostAuth}
                  onChange={(e) => setSwitchHostAuth(e.target.checked)}
                  className="h-3.5 w-3.5 rounded border-white/20 bg-zinc-900 accent-emerald-500"
                />
                <span>注入并验证成功后，自动将此主机的登录方式切换为该密钥</span>
              </label>

              {error && (
                <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-xs text-red-300">
                  {error}
                </div>
              )}

              {result && (
                <div className="space-y-2 rounded-xl border border-white/[0.08] bg-[#09090b] p-4 text-xs">
                  <div className="flex items-center justify-between">
                    <span className="text-zinc-400">远程 authorized_keys：</span>
                    {result.inAuthorizedKeys ? (
                      <span className="inline-flex items-center gap-1 text-emerald-400">
                        <CheckCircle2 className="h-3.5 w-3.5" />
                        已写入公钥
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 text-amber-400">
                        <ShieldAlert className="h-3.5 w-3.5" />
                        未检测到公钥
                      </span>
                    )}
                  </div>

                  <div className="flex items-center justify-between">
                    <span className="text-zinc-400">密钥免密登录测试：</span>
                    {result.authWorks ? (
                      <span className="inline-flex items-center gap-1 font-mono text-emerald-400">
                        <ShieldCheck className="h-3.5 w-3.5" />
                        认证成功 ({result.latencyMs} ms)
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 text-red-400">
                        <XCircle className="h-3.5 w-3.5" />
                        认证失败
                      </span>
                    )}
                  </div>

                  {result.error && (
                    <div className="mt-1 rounded bg-red-500/10 px-2.5 py-1.5 font-mono text-[11px] text-red-300">
                      {result.error}
                    </div>
                  )}

                  {switchedNotice && (
                    <div className="mt-1 rounded bg-emerald-500/10 px-2.5 py-1.5 text-[11px] text-emerald-300">
                      已自动将主机切换为使用「{result.keyName}」密钥登录。
                    </div>
                  )}
                </div>
              )}

              <div className="flex items-center justify-end gap-2.5 pt-2">
                <button
                  type="button"
                  disabled={checking || injecting || !selectedKeyId}
                  onClick={handleCheck}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-white/[0.08] bg-white/[0.04] px-3.5 py-2 text-xs font-medium text-zinc-200 transition hover:bg-white/[0.08] disabled:opacity-50"
                >
                  <SearchCheck className="h-3.5 w-3.5 text-sky-400" />
                  {checking ? '检测中...' : '检测密钥状态'}
                </button>
                <button
                  type="button"
                  disabled={checking || injecting || !selectedKeyId}
                  onClick={handleInject}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-500 px-4 py-2 text-xs font-semibold text-zinc-950 transition hover:bg-emerald-400 disabled:opacity-50"
                >
                  <Syringe className="h-3.5 w-3.5" />
                  {injecting ? '注入并验证中...' : '一键自动注入'}
                </button>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
};
