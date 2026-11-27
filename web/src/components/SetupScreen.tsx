import React, { useState } from 'react';
import { ShieldCheck, Lock, Sparkles, ArrowRight, KeyRound } from 'lucide-react';
import { api } from '../api';

interface SetupScreenProps {
  mode: 'setup' | 'unlock';
  version?: string;
  onComplete: () => void;
}

export const SetupScreen: React.FC<SetupScreenProps> = ({ mode, version, onComplete }) => {
  const [passphrase, setPassphrase] = useState('');
  const [confirm, setConfirm] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  const isSetup = mode === 'setup';

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    if (isSetup) {
      if (passphrase.length < 4) {
        setError('主加密密钥长度至少需要 4 个字符');
        return;
      }
      if (passphrase !== confirm) {
        setError('两次输入的密钥不一致');
        return;
      }
    } else if (!passphrase) {
      setError('请输入主密钥');
      return;
    }

    setSubmitting(true);
    try {
      if (isSetup) {
        await api.authSetup(passphrase);
      } else {
        await api.authUnlock(passphrase);
      }
      onComplete();
    } catch (err: any) {
      setError(err?.message || (isSetup ? '初始化失败' : '解锁失败'));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="flex min-h-screen w-screen items-center justify-center bg-[#09090b] p-4 text-zinc-100">
      <div className="w-full max-w-md overflow-hidden rounded-2xl border border-white/[0.1] bg-[#111114] shadow-2xl">
        <div className="border-b border-white/[0.08] px-6 py-6 text-center">
          <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-400/20 to-sky-500/20 ring-1 ring-emerald-500/30">
            {isSetup ? (
              <Sparkles className="h-6 w-6 text-emerald-400" />
            ) : (
              <KeyRound className="h-6 w-6 text-emerald-400" />
            )}
          </div>
          <h1 className="text-lg font-semibold tracking-tight text-zinc-100">
            {isSetup ? '欢迎使用 VPSCoop' : '解锁 VPSCoop'}
          </h1>
          <p className="mt-1 text-xs text-zinc-400">
            {isSetup
              ? '首次启动 · 设置主加密密钥（Master Key）'
              : '数据库已加密保护 · 请输入主密钥解锁面板'}
          </p>
          {version && (
            <span className="mt-2 inline-block rounded-full border border-white/[0.08] bg-white/[0.04] px-2.5 py-0.5 font-mono text-[10px] text-zinc-400">
              {version}
            </span>
          )}
        </div>

        <form onSubmit={handleSubmit} className="space-y-4 p-6">
          <div className="flex items-start gap-2.5 rounded-xl border border-white/[0.08] bg-[#0b0b0e] p-3.5 text-[11px] leading-relaxed text-zinc-400">
            <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-emerald-400" />
            <span>
              {isSetup ? (
                <>
                  主密钥同时用于<strong className="text-zinc-200">解锁面板</strong>与{' '}
                  <strong className="text-zinc-200">AES-256-GCM 加密数据库</strong>。跨设备迁移时只需拷贝{' '}
                  <code className="text-zinc-200">vpscoop.db</code>，在新设备上用同一主密钥即可直接解锁。
                </>
              ) : (
                <>
                  输入你的主密钥以在内存中派生解密密钥并打开{' '}
                  <code className="text-zinc-200">vpscoop.db</code>。
                </>
              )}
            </span>
          </div>

          {error && (
            <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-xs text-red-300">
              {error}
            </div>
          )}

          <div>
            <label className="mb-1.5 block text-xs font-medium text-zinc-400">
              {isSetup ? '设置主密钥 *' : '主密钥 (Master Key) *'}
            </label>
            <div className="relative">
              <Lock className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-zinc-500" />
              <input
                type="password"
                required
                autoFocus
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
                placeholder={isSetup ? '至少 4 个字符' : '输入主密钥解锁'}
                className="w-full rounded-xl border border-white/[0.08] bg-zinc-900/80 py-2.5 pl-9 pr-3 text-sm text-zinc-100 placeholder-zinc-500 outline-none transition focus:border-emerald-500/60 focus:ring-2 focus:ring-emerald-500/15"
              />
            </div>
          </div>

          {isSetup && (
            <div>
              <label className="mb-1.5 block text-xs font-medium text-zinc-400">
                确认主密钥 *
              </label>
              <input
                type="password"
                required
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                placeholder="再次输入主密钥"
                className="w-full rounded-xl border border-white/[0.08] bg-zinc-900/80 px-3.5 py-2.5 text-sm text-zinc-100 placeholder-zinc-500 outline-none transition focus:border-emerald-500/60 focus:ring-2 focus:ring-emerald-500/15"
              />
            </div>
          )}

          <button
            type="submit"
            disabled={submitting}
            className="inline-flex w-full items-center justify-center gap-2 rounded-xl bg-emerald-500 py-2.5 text-xs font-semibold text-zinc-950 shadow-lg shadow-emerald-500/15 transition hover:bg-emerald-400 disabled:opacity-50"
          >
            <span>
              {submitting
                ? isSetup
                  ? '正在初始化...'
                  : '正在解锁...'
                : isSetup
                  ? '完成初始化并进入面板'
                  : '解锁面板'}
            </span>
            <ArrowRight className="h-4 w-4" />
          </button>
        </form>
      </div>
    </div>
  );
};
