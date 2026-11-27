import React, { useRef, useState } from 'react';
import { Upload, X, FileCode2, CheckCircle2 } from 'lucide-react';
import { api } from '../api';
import type { SSHKey } from '../types';

interface ImportModalProps {
  open: boolean;
  savedKeys: SSHKey[];
  onClose: () => void;
  onImported: () => Promise<void>;
}

export const ImportModal: React.FC<ImportModalProps> = ({
  open,
  savedKeys,
  onClose,
  onImported,
}) => {
  const [format, setFormat] = useState<'auto' | 'ssh_config' | 'csv' | 'json'>('auto');
  const [content, setContent] = useState('');
  const [keyId, setKeyId] = useState('');
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState('');
  const [summary, setSummary] = useState<{
    format: string;
    total: number;
    imported: number;
    skipped: number;
  } | null>(null);
  const fileRef = useRef<HTMLInputElement | null>(null);

  if (!open) return null;

  const handleFile = async (file: File | undefined) => {
    if (!file) return;
    const text = await file.text();
    setContent(text);
    setSummary(null);
    setError('');
  };

  const handleImport = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!content.trim()) {
      setError('请粘贴或上传 Termius 导出的 SSH Config / CSV / JSON 内容');
      return;
    }
    setError('');
    setImporting(true);
    try {
      const res = await api.importHosts({ format, content, keyId });
      setSummary(res);
      await onImported();
    } catch (err: any) {
      setError(err?.message || '导入失败');
    } finally {
      setImporting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-white/[0.1] bg-[#111114] shadow-2xl">
        <div className="flex items-center justify-between border-b border-white/[0.08] px-6 py-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-400">
              <FileCode2 className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-zinc-100">
                从 Termius / SSH Config 一键迁移
              </h2>
              <p className="text-[11px] text-zinc-500">
                支持 Termius 导出的 SSH Config、CSV、JSON 或本机 ~/.ssh/config
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

        <form onSubmit={handleImport} className="flex-1 space-y-4 overflow-y-auto px-6 py-5">
          {/* Quick Guide */}
          <div className="rounded-xl border border-white/[0.08] bg-[#0b0b0e] p-3.5 text-[11px] leading-relaxed text-zinc-400">
            <div className="mb-1 font-semibold text-zinc-200">Termius 迁移步骤：</div>
            <div>
              1. 在 Termius 桌面端打开 <code className="text-zinc-300">Settings → Data</code>（或使用{' '}
              <code className="text-zinc-300">termius export-ssh-config</code>），导出为{' '}
              <strong className="text-zinc-200">SSH Config / CSV / JSON</strong>。
            </div>
            <div>2. 将导出的文件拖入/上传到下方，或直接全选粘贴内容，系统会自动识别格式并去重导入。</div>
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label className="mb-1.5 block text-xs font-medium text-zinc-400">格式识别</label>
              <select
                value={format}
                onChange={(e) => setFormat(e.target.value as any)}
                className="w-full rounded-lg border border-white/[0.08] bg-zinc-900/80 px-3 py-2 text-xs text-zinc-100 outline-none focus:border-emerald-500/60"
              >
                <option value="auto">自动识别 (SSH Config / CSV / JSON)</option>
                <option value="ssh_config">OpenSSH Config (~/.ssh/config)</option>
                <option value="csv">CSV 表格</option>
                <option value="json">JSON</option>
              </select>
            </div>

            <div>
              <label className="mb-1.5 block text-xs font-medium text-zinc-400">
                默认关联密钥（可选，用于私钥主机）
              </label>
              <select
                value={keyId}
                onChange={(e) => setKeyId(e.target.value)}
                className="w-full rounded-lg border border-white/[0.08] bg-zinc-900/80 px-3 py-2 text-xs text-zinc-100 outline-none focus:border-emerald-500/60"
              >
                <option value="">不自动关联</option>
                {savedKeys.map((k) => (
                  <option key={k.id} value={k.id}>
                    {k.name} ({k.algorithm})
                  </option>
                ))}
              </select>
            </div>
          </div>

          <div>
            <div className="mb-1.5 flex items-center justify-between">
              <label className="text-xs font-medium text-zinc-400">导入内容</label>
              <label className="inline-flex cursor-pointer items-center gap-1.5 text-[11px] text-sky-400 hover:text-sky-300">
                <Upload className="h-3 w-3" />
                <span>从文件读取</span>
                <input
                  ref={fileRef}
                  type="file"
                  accept=".conf,.config,.txt,.csv,.json"
                  className="hidden"
                  onChange={(e) => handleFile(e.target.files?.[0])}
                />
              </label>
            </div>
            <textarea
              rows={9}
              value={content}
              onDragOver={(e) => e.preventDefault()}
              onDrop={async (e) => {
                e.preventDefault();
                await handleFile(e.dataTransfer.files?.[0]);
              }}
              onChange={(e) => {
                setContent(e.target.value);
                setSummary(null);
              }}
              placeholder={`Host la-cn2\n  HostName 192.0.2.10\n  User root\n  Port 22\n\n或直接拖拽 ~/.ssh/config、CSV、JSON 文件到此处...`}
              className="w-full rounded-xl border border-white/[0.08] bg-[#09090b] p-3 font-mono text-xs text-zinc-200 placeholder-zinc-600 outline-none focus:border-emerald-500/60"
            />
          </div>

          {error && (
            <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-xs text-red-300">
              {error}
            </div>
          )}

          {summary && (
            <div className="flex items-center gap-2.5 rounded-xl border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-xs text-emerald-300">
              <CheckCircle2 className="h-4 w-4 shrink-0" />
              <span>
                识别格式 <strong>{summary.format}</strong>：共解析 {summary.total} 台，成功导入{' '}
                <strong>{summary.imported}</strong> 台，跳过重复 {summary.skipped} 台。
              </span>
            </div>
          )}
        </form>

        <div className="flex items-center justify-end gap-2.5 border-t border-white/[0.08] bg-zinc-950/60 px-6 py-4">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-white/[0.08] px-4 py-2 text-xs font-medium text-zinc-300 hover:bg-white/[0.05]"
          >
            关闭
          </button>
          <button
            type="button"
            disabled={importing}
            onClick={handleImport}
            className="rounded-lg bg-emerald-500 px-4 py-2 text-xs font-semibold text-zinc-950 transition hover:bg-emerald-400 disabled:opacity-50"
          >
            {importing ? '导入中...' : '开始一键导入'}
          </button>
        </div>
      </div>
    </div>
  );
};
