import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  Receipt,
  X,
  Upload,
  FileText,
  Image as ImageIcon,
  ExternalLink,
  Download,
  Trash2,
} from 'lucide-react';
import { api } from '../api';
import type { Currency, Host, Invoice } from '../types';
import { CURRENCY_SYMBOLS, formatBytes } from '../utils/format';

interface InvoiceModalProps {
  open: boolean;
  host: Host | null;
  onClose: () => void;
}

export const InvoiceModal: React.FC<InvoiceModalProps> = ({ open, host, onClose }) => {
  const [invoices, setInvoices] = useState<Invoice[]>([]);
  const [loading, setLoading] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState('');

  const [title, setTitle] = useState('');
  const [amount, setAmount] = useState('');
  const [currency, setCurrency] = useState<Currency>('USD');
  const [billedAt, setBilledAt] = useState('');
  const [notes, setNotes] = useState('');
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const fileRef = useRef<HTMLInputElement | null>(null);

  const loadInvoices = useCallback(async () => {
    if (!host) return;
    setLoading(true);
    try {
      const list = await api.listInvoices(host.id);
      setInvoices(list);
      setError('');
    } catch (err: any) {
      setError(err?.message || '加载 Invoice 失败');
    } finally {
      setLoading(false);
    }
  }, [host]);

  useEffect(() => {
    if (!open || !host) return;
    setTitle('');
    setAmount(host.cost ? String(host.cost) : '');
    setCurrency(host.currency || 'USD');
    setBilledAt(new Date().toISOString().slice(0, 10));
    setNotes('');
    setSelectedFile(null);
    loadInvoices();
  }, [open, host, loadInvoices]);

  if (!open || !host) return null;

  const handleUpload = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedFile) {
      setError('请选择要上传的 Invoice 文件（PDF / 图片等）');
      return;
    }
    setError('');
    setUploading(true);
    try {
      await api.uploadInvoice(host.id, {
        file: selectedFile,
        title: title.trim() || selectedFile.name,
        amount,
        currency,
        billedAt,
        notes,
      });
      setTitle('');
      setNotes('');
      setSelectedFile(null);
      if (fileRef.current) fileRef.current.value = '';
      await loadInvoices();
    } catch (err: any) {
      setError(err?.message || '上传失败');
    } finally {
      setUploading(false);
    }
  };

  const handleDelete = async (inv: Invoice) => {
    if (!window.confirm(`确定删除账单 "${inv.title}" 吗？`)) return;
    try {
      await api.deleteInvoice(inv.id);
      await loadInvoices();
    } catch (err: any) {
      setError(err?.message || '删除失败');
    }
  };

  const inputCls =
    'w-full rounded-lg border border-white/[0.08] bg-zinc-900/80 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 outline-none transition focus:border-sky-500/60';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-white/[0.1] bg-[#111114] shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-white/[0.08] px-6 py-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-sky-500/15 text-sky-400">
              <Receipt className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-zinc-100">
                Invoice 账单凭证 · {host.name}
              </h2>
              <p className="text-[11px] text-zinc-500">
                {host.provider ? `${host.provider} · ` : ''}
                上传并留存 PDF / 截图账单，方便核对续费与报销
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

        <div className="flex-1 space-y-5 overflow-y-auto px-6 py-5">
          {error && (
            <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-xs text-red-300">
              {error}
            </div>
          )}

          {/* Upload Form */}
          <form
            onSubmit={handleUpload}
            className="space-y-3 rounded-xl border border-white/[0.08] bg-[#0b0b0e] p-4"
          >
            <div className="text-xs font-semibold text-zinc-300">上传新 Invoice</div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <input
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder="账单标题（如：2026-11 续费账单，留空用文件名）"
                className={inputCls}
              />
              <div className="grid grid-cols-3 gap-2">
                <input
                  type="number"
                  step="0.01"
                  min={0}
                  value={amount}
                  onChange={(e) => setAmount(e.target.value)}
                  placeholder="金额"
                  className={`${inputCls} col-span-2 font-mono`}
                />
                <select
                  value={currency}
                  onChange={(e) => setCurrency(e.target.value as Currency)}
                  className={inputCls}
                >
                  <option value="USD">USD</option>
                  <option value="CNY">CNY</option>
                  <option value="EUR">EUR</option>
                  <option value="GBP">GBP</option>
                  <option value="JPY">JPY</option>
                </select>
              </div>
            </div>

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <input
                type="date"
                value={billedAt}
                onChange={(e) => setBilledAt(e.target.value)}
                className={`${inputCls} font-mono`}
              />
              <input
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                placeholder="备注（如：订单号 #123456）"
                className={inputCls}
              />
            </div>

            <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
              <label
                onDragOver={(e) => {
                  e.preventDefault();
                  setDragOver(true);
                }}
                onDragLeave={() => setDragOver(false)}
                onDrop={(e) => {
                  e.preventDefault();
                  setDragOver(false);
                  const f = e.dataTransfer.files?.[0];
                  if (f) setSelectedFile(f);
                }}
                className={`inline-flex flex-1 cursor-pointer items-center justify-center gap-2 rounded-lg border border-dashed px-3 py-2.5 text-xs transition ${
                  dragOver
                    ? 'border-sky-400 bg-sky-500/15 text-sky-200'
                    : 'border-white/[0.15] bg-white/[0.02] text-zinc-300 hover:border-sky-400/50 hover:bg-sky-500/5'
                }`}
              >
                <Upload className="h-3.5 w-3.5 text-sky-400" />
                <span className="max-w-[260px] truncate">
                  {selectedFile ? selectedFile.name : '拖拽或点击选择 PDF / 图片 / 账单文件'}
                </span>
                <input
                  ref={fileRef}
                  type="file"
                  accept=".pdf,.png,.jpg,.jpeg,.webp,.txt,.html"
                  className="hidden"
                  onChange={(e) => setSelectedFile(e.target.files?.[0] || null)}
                />
              </label>

              <button
                type="submit"
                disabled={uploading || !selectedFile}
                className="inline-flex items-center gap-1.5 rounded-lg bg-sky-500 px-4 py-2 text-xs font-semibold text-zinc-950 transition hover:bg-sky-400 disabled:opacity-50"
              >
                <Upload className="h-3.5 w-3.5" />
                {uploading ? '上传中...' : '上传并保存'}
              </button>
            </div>
          </form>

          {/* Uploaded Invoices List */}
          <div className="space-y-2.5">
            <div className="text-xs font-semibold text-zinc-400">
              已保存的 Invoice ({invoices.length})
            </div>

            {loading ? (
              <div className="py-8 text-center text-xs text-zinc-500">加载中...</div>
            ) : invoices.length === 0 ? (
              <div className="rounded-xl border border-dashed border-white/[0.08] py-8 text-center text-xs text-zinc-500">
                暂无上传的 Invoice 凭证
              </div>
            ) : (
              <div className="divide-y divide-white/[0.06] overflow-hidden rounded-xl border border-white/[0.08] bg-[#0d0d10]">
                {invoices.map((inv) => {
                  const isImg = inv.mimeType.startsWith('image/');
                  return (
                    <div
                      key={inv.id}
                      className="flex items-center justify-between gap-3 px-4 py-3 transition hover:bg-white/[0.02]"
                    >
                      <div className="flex min-w-0 items-center gap-3">
                        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-white/[0.04] text-sky-400">
                          {isImg ? (
                            <ImageIcon className="h-4 w-4" />
                          ) : (
                            <FileText className="h-4 w-4" />
                          )}
                        </div>
                        <div className="min-w-0">
                          <div className="truncate text-xs font-medium text-zinc-100">
                            {inv.title}
                          </div>
                          <div className="mt-0.5 flex flex-wrap items-center gap-2 font-mono text-[11px] text-zinc-500">
                            {inv.billedAt && <span>{inv.billedAt}</span>}
                            {inv.amount > 0 && (
                              <span className="text-emerald-400">
                                {CURRENCY_SYMBOLS[inv.currency as Currency] || ''}
                                {inv.amount.toFixed(2)} {inv.currency}
                              </span>
                            )}
                            <span>{formatBytes(inv.size)}</span>
                            {inv.notes && (
                              <span className="font-sans text-zinc-400">· {inv.notes}</span>
                            )}
                          </div>
                        </div>
                      </div>

                      <div className="flex shrink-0 items-center gap-1">
                        <a
                          href={api.invoiceFileUrl(inv.id, false)}
                          target="_blank"
                          rel="noreferrer"
                          className="rounded-lg p-1.5 text-zinc-400 transition hover:bg-white/[0.06] hover:text-zinc-100"
                          title="在线预览"
                        >
                          <ExternalLink className="h-3.5 w-3.5" />
                        </a>
                        <a
                          href={api.invoiceFileUrl(inv.id, true)}
                          className="rounded-lg p-1.5 text-zinc-400 transition hover:bg-white/[0.06] hover:text-sky-400"
                          title="下载"
                        >
                          <Download className="h-3.5 w-3.5" />
                        </a>
                        <button
                          type="button"
                          onClick={() => handleDelete(inv)}
                          className="rounded-lg p-1.5 text-zinc-400 transition hover:bg-red-500/15 hover:text-red-400"
                          title="删除"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};
