import React, { useEffect, useRef, useState } from 'react';
import {
  ShieldCheck,
  Download,
  Upload,
  X,
  CheckCircle2,
  Lock,
  Cloud,
  RefreshCw,
  CloudUpload,
  CloudDownload,
} from 'lucide-react';
import { api } from '../api';
import { formatBytes } from '../utils/format';

interface BackupModalProps {
  open: boolean;
  onClose: () => void;
  onRestored: () => Promise<void>;
}

export const BackupModal: React.FC<BackupModalProps> = ({
  open,
  onClose,
  onRestored,
}) => {
  const [tab, setTab] = useState<'export' | 'import' | 'webdav'>('export');
  const [passphrase, setPassphrase] = useState('');
  const [confirmPass, setConfirmPass] = useState('');
  const [fileContent, setFileContent] = useState('');
  const [fileName, setFileName] = useState('');

  // WebDAV state
  const [davUrl, setDavUrl] = useState('');
  const [davUser, setDavUser] = useState('');
  const [davPass, setDavPass] = useState('');
  const [davE2E, setDavE2E] = useState('');
  const [hasDavPass, setHasDavPass] = useState(false);
  const [hasDavE2E, setHasDavE2E] = useState(false);
  const [davFiles, setDavFiles] = useState<{ name: string; size: number; lastModified: string }[]>([]);
  const [davNotice, setDavNotice] = useState('');

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [exportDone, setExportDone] = useState(false);
  const [importSummary, setImportSummary] = useState<{
    hostsImported: number;
    hostsSkipped: number;
    keysImported: number;
    keysSkipped: number;
    invoicesImported: number;
  } | null>(null);

  const fileRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!open) return;
    setError('');
    setDavNotice('');
    api
      .getWebDAVConfig()
      .then((cfg) => {
        setDavUrl(cfg.url || '');
        setDavUser(cfg.username || '');
        setHasDavPass(cfg.hasPassword);
        setHasDavE2E(cfg.hasPassphrase);
      })
      .catch(() => {});
  }, [open]);

  if (!open) return null;

  const handleExport = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setExportDone(false);
    if (!passphrase.trim()) {
      setError('请输入用于加密备份文件的密钥/密码');
      return;
    }
    if (passphrase !== confirmPass) {
      setError('两次输入的密钥不一致');
      return;
    }
    setBusy(true);
    try {
      await api.exportEncryptedBackup(passphrase);
      setExportDone(true);
    } catch (err: any) {
      setError(err?.message || '导出失败');
    } finally {
      setBusy(false);
    }
  };

  const handleFilePick = async (file: File | undefined) => {
    if (!file) return;
    setFileName(file.name);
    setFileContent(await file.text());
    setImportSummary(null);
    setError('');
  };

  const handleImport = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setImportSummary(null);
    if (!fileContent.trim()) {
      setError('请选择 .vpscoop 加密备份文件');
      return;
    }
    if (!passphrase.trim()) {
      setError('请输入导出时设置的解密密钥');
      return;
    }
    setBusy(true);
    try {
      const res = await api.importEncryptedBackup(passphrase, fileContent);
      setImportSummary(res);
      await onRestored();
    } catch (err: any) {
      setError(err?.message || '解密或恢复失败');
    } finally {
      setBusy(false);
    }
  };

  const handleSaveWebDAV = async () => {
    setError('');
    setDavNotice('');
    setBusy(true);
    try {
      await api.saveWebDAVConfig({
        url: davUrl,
        username: davUser,
        password: davPass,
        passphrase: davE2E,
      });
      if (davPass) setHasDavPass(true);
      if (davE2E) setHasDavE2E(true);
      setDavPass('');
      setDavE2E('');
      setDavNotice('WebDAV 云盘配置已加密保存');
    } catch (err: any) {
      setError(err?.message || '保存配置失败');
    } finally {
      setBusy(false);
    }
  };

  const handleTestAndListWebDAV = async () => {
    setError('');
    setDavNotice('');
    setBusy(true);
    try {
      await api.saveWebDAVConfig({
        url: davUrl,
        username: davUser,
        password: davPass,
        passphrase: davE2E,
      });
      if (davPass) setHasDavPass(true);
      if (davE2E) setHasDavE2E(true);
      await api.testWebDAV({ url: davUrl, username: davUser, password: davPass });
      const files = await api.listWebDAVFiles();
      setDavFiles(files);
      setDavNotice(`连接成功！云盘目录中共找到 ${files.length} 个备份文件`);
    } catch (err: any) {
      setError(err?.message || '连接 WebDAV 失败');
    } finally {
      setBusy(false);
    }
  };

  const handlePushWebDAV = async () => {
    setError('');
    setDavNotice('');
    setBusy(true);
    try {
      if (davUrl || davUser || davPass || davE2E) {
        await api.saveWebDAVConfig({
          url: davUrl,
          username: davUser,
          password: davPass,
          passphrase: davE2E,
        });
      }
      const res = await api.pushWebDAV(davE2E, true);
      const files = await api.listWebDAVFiles();
      setDavFiles(files);
      setDavNotice(
        `已加密上传至云盘：${res.latest}（${formatBytes(res.bytes)}），并保留历史快照 ${res.snapshot}`
      );
    } catch (err: any) {
      setError(err?.message || '上传同步失败');
    } finally {
      setBusy(false);
    }
  };

  const handlePullWebDAV = async (filename: string) => {
    setError('');
    setDavNotice('');
    setBusy(true);
    try {
      const res = await api.pullWebDAV(filename, davE2E);
      setDavNotice(
        `已从 ${filename} 恢复：主机 +${res.hostsImported}（跳过 ${res.hostsSkipped}），密钥 +${res.keysImported}（跳过 ${res.keysSkipped}），Invoice +${res.invoicesImported}`
      );
      await onRestored();
    } catch (err: any) {
      setError(err?.message || '拉取或解密失败');
    } finally {
      setBusy(false);
    }
  };

  const inputCls =
    'w-full rounded-lg border border-white/[0.08] bg-zinc-900/80 px-3 py-2 text-xs text-zinc-100 placeholder-zinc-500 outline-none transition focus:border-emerald-500/60';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="flex max-h-[90vh] w-full max-w-xl flex-col overflow-hidden rounded-2xl border border-white/[0.1] bg-[#111114] shadow-2xl">
        <div className="flex items-center justify-between border-b border-white/[0.08] px-6 py-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-400">
              <ShieldCheck className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-zinc-100">
                全量加密备份与云盘同步
              </h2>
              <p className="text-[11px] text-zinc-500">
                支持本地 .vpscoop 加密导出/恢复，以及 WebDAV 云盘跨端同步
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

        {/* Tabs */}
        <div className="flex border-b border-white/[0.08] px-6 pt-3">
          <button
            type="button"
            onClick={() => {
              setTab('export');
              setError('');
              setExportDone(false);
            }}
            className={`border-b-2 px-4 pb-2.5 text-xs font-medium transition ${
              tab === 'export'
                ? 'border-emerald-400 text-emerald-300'
                : 'border-transparent text-zinc-400 hover:text-zinc-200'
            }`}
          >
            本地加密导出
          </button>
          <button
            type="button"
            onClick={() => {
              setTab('import');
              setError('');
              setImportSummary(null);
            }}
            className={`border-b-2 px-4 pb-2.5 text-xs font-medium transition ${
              tab === 'import'
                ? 'border-sky-400 text-sky-300'
                : 'border-transparent text-zinc-400 hover:text-zinc-200'
            }`}
          >
            本地解密恢复
          </button>
          <button
            type="button"
            onClick={() => {
              setTab('webdav');
              setError('');
              setDavNotice('');
            }}
            className={`inline-flex items-center gap-1.5 border-b-2 px-4 pb-2.5 text-xs font-medium transition ${
              tab === 'webdav'
                ? 'border-violet-400 text-violet-300'
                : 'border-transparent text-zinc-400 hover:text-zinc-200'
            }`}
          >
            <Cloud className="h-3.5 w-3.5" />
            云盘同步 (WebDAV)
          </button>
        </div>

        <div className="flex-1 overflow-y-auto p-6">
          {error && (
            <div className="mb-4 rounded-lg border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-xs text-red-300">
              {error}
            </div>
          )}

          {tab === 'export' ? (
            <form onSubmit={handleExport} className="space-y-4">
              <div className="rounded-xl border border-white/[0.08] bg-[#0b0b0e] p-3.5 text-[11px] leading-relaxed text-zinc-400">
                导出的 <code className="text-zinc-200">.vpscoop</code>{' '}
                文件包含所有主机的登录凭据、SSH 私钥、Invoice 附件与 Tailscale Auth Key，均使用你输入的密钥（PBKDF2-SHA256 + AES-256-GCM）加密。
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium text-zinc-400">
                  设置加密密钥 (Passphrase) *
                </label>
                <div className="relative">
                  <Lock className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-zinc-500" />
                  <input
                    type="password"
                    required
                    value={passphrase}
                    onChange={(e) => setPassphrase(e.target.value)}
                    placeholder="输入一个强密码用于加密备份"
                    className={`${inputCls} pl-8`}
                  />
                </div>
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium text-zinc-400">
                  确认加密密钥 *
                </label>
                <input
                  type="password"
                  required
                  value={confirmPass}
                  onChange={(e) => setConfirmPass(e.target.value)}
                  placeholder="再次输入密钥"
                  className={inputCls}
                />
              </div>

              {exportDone && (
                <div className="flex items-center gap-2 rounded-xl border border-emerald-500/30 bg-emerald-500/10 px-3.5 py-2.5 text-xs text-emerald-300">
                  <CheckCircle2 className="h-4 w-4 shrink-0" />
                  <span>加密备份文件已生成并开始下载，请妥善保管密钥！</span>
                </div>
              )}

              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={onClose}
                  className="rounded-lg border border-white/[0.08] px-4 py-2 text-xs text-zinc-300 hover:bg-white/[0.05]"
                >
                  关闭
                </button>
                <button
                  type="submit"
                  disabled={busy}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-500 px-4 py-2 text-xs font-semibold text-zinc-950 hover:bg-emerald-400 disabled:opacity-50"
                >
                  <Download className="h-3.5 w-3.5" />
                  {busy ? '正在加密导出...' : '加密导出全量备份'}
                </button>
              </div>
            </form>
          ) : tab === 'import' ? (
            <form onSubmit={handleImport} className="space-y-4">
              <div>
                <label className="mb-1.5 block text-xs font-medium text-zinc-400">
                  选择加密备份文件 (.vpscoop) *
                </label>
                <label className="flex cursor-pointer items-center justify-center gap-2 rounded-xl border border-dashed border-white/[0.15] bg-[#0b0b0e] px-4 py-5 text-xs text-zinc-300 transition hover:border-sky-400/50 hover:bg-sky-500/5">
                  <Upload className="h-4 w-4 text-sky-400" />
                  <span className="truncate">
                    {fileName || '点击选择 .vpscoop 备份文件'}
                  </span>
                  <input
                    ref={fileRef}
                    type="file"
                    accept=".vpscoop,.json"
                    className="hidden"
                    onChange={(e) => handleFilePick(e.target.files?.[0])}
                  />
                </label>
              </div>

              <div>
                <label className="mb-1.5 block text-xs font-medium text-zinc-400">
                  解密密钥 (Passphrase) *
                </label>
                <div className="relative">
                  <Lock className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-zinc-500" />
                  <input
                    type="password"
                    required
                    value={passphrase}
                    onChange={(e) => setPassphrase(e.target.value)}
                    placeholder="输入导出时设置的密钥"
                    className={`${inputCls} pl-8`}
                  />
                </div>
              </div>

              {importSummary && (
                <div className="space-y-1 rounded-xl border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-xs text-emerald-300">
                  <div className="flex items-center gap-2 font-semibold">
                    <CheckCircle2 className="h-4 w-4 shrink-0" />
                    <span>解密恢复完成</span>
                  </div>
                  <div className="text-[11px] opacity-90">
                    主机：导入 {importSummary.hostsImported} 台（跳过重复 {importSummary.hostsSkipped} 台）
                    · 密钥：导入 {importSummary.keysImported} 个（跳过 {importSummary.keysSkipped} 个）
                    · Invoice：恢复 {importSummary.invoicesImported} 份
                  </div>
                </div>
              )}

              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={onClose}
                  className="rounded-lg border border-white/[0.08] px-4 py-2 text-xs text-zinc-300 hover:bg-white/[0.05]"
                >
                  关闭
                </button>
                <button
                  type="submit"
                  disabled={busy}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-sky-500 px-4 py-2 text-xs font-semibold text-zinc-950 hover:bg-sky-400 disabled:opacity-50"
                >
                  <Upload className="h-3.5 w-3.5" />
                  {busy ? '正在解密恢复...' : '解密并恢复数据'}
                </button>
              </div>
            </form>
          ) : (
            <div className="space-y-4">
              <div className="rounded-xl border border-white/[0.08] bg-[#0b0b0e] p-3 text-[11px] leading-relaxed text-zinc-400">
                支持 <strong className="text-zinc-200">坚果云 (Nutstore)</strong>、
                <strong className="text-zinc-200">Nextcloud</strong>、
                <strong className="text-zinc-200">AList（阿里云盘/OneDrive/百度网盘）</strong>、
                <strong className="text-zinc-200">群晖 NAS</strong> 等所有标准 WebDAV 服务。上传前会在本地完成端到端加密，云盘无法读取明文。
              </div>

              <div className="space-y-3">
                <div>
                  <label className="mb-1 block text-xs font-medium text-zinc-400">
                    WebDAV 目录地址 *
                  </label>
                  <input
                    value={davUrl}
                    onChange={(e) => setDavUrl(e.target.value)}
                    placeholder="例如：https://dav.jianguoyun.com/dav/VPSCoop/"
                    className={`${inputCls} font-mono`}
                  />
                </div>

                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  <div>
                    <label className="mb-1 block text-xs font-medium text-zinc-400">
                      WebDAV 用户名
                    </label>
                    <input
                      value={davUser}
                      onChange={(e) => setDavUser(e.target.value)}
                      placeholder="邮箱或账号"
                      className={inputCls}
                    />
                  </div>
                  <div>
                    <label className="mb-1 block text-xs font-medium text-zinc-400">
                      应用密码 {hasDavPass ? '（留空保留已保存密码）' : ''}
                    </label>
                    <input
                      type="password"
                      value={davPass}
                      onChange={(e) => setDavPass(e.target.value)}
                      placeholder={hasDavPass ? '••••••••••••' : 'WebDAV 专用密码'}
                      className={inputCls}
                    />
                  </div>
                </div>

                <div>
                  <label className="mb-1 block text-xs font-medium text-zinc-400">
                    端到端加密密钥 {hasDavE2E ? '（留空使用已保存密钥）' : '*'}
                  </label>
                  <input
                    type="password"
                    value={davE2E}
                    onChange={(e) => setDavE2E(e.target.value)}
                    placeholder={
                      hasDavE2E
                        ? '••••••••••••（多端需保持一致）'
                        : '用于加密/解密云盘上的 .vpscoop 文件'
                    }
                    className={inputCls}
                  />
                </div>
              </div>

              {davNotice && (
                <div className="flex items-center gap-2 rounded-xl border border-emerald-500/30 bg-emerald-500/10 px-3.5 py-2.5 text-xs text-emerald-300">
                  <CheckCircle2 className="h-4 w-4 shrink-0" />
                  <span>{davNotice}</span>
                </div>
              )}

              <div className="flex flex-wrap items-center justify-between gap-2 pt-1">
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    disabled={busy}
                    onClick={handleSaveWebDAV}
                    className="rounded-lg border border-white/[0.08] bg-white/[0.03] px-3 py-1.5 text-xs text-zinc-200 hover:bg-white/[0.08] disabled:opacity-50"
                  >
                    保存配置
                  </button>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={handleTestAndListWebDAV}
                    className="inline-flex items-center gap-1 rounded-lg border border-white/[0.08] bg-white/[0.03] px-3 py-1.5 text-xs text-zinc-200 hover:bg-white/[0.08] disabled:opacity-50"
                  >
                    <RefreshCw className={`h-3.5 w-3.5 text-sky-400 ${busy ? 'animate-spin' : ''}`} />
                    连接并刷新列表
                  </button>
                </div>

                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => handlePullWebDAV('vpscoop-latest.vpscoop')}
                    className="inline-flex items-center gap-1.5 rounded-lg border border-sky-500/30 bg-sky-500/15 px-3 py-1.5 text-xs font-medium text-sky-300 hover:bg-sky-500/25 disabled:opacity-50"
                  >
                    <CloudDownload className="h-3.5 w-3.5" />
                    拉取最新恢复
                  </button>
                  <button
                    type="button"
                    disabled={busy}
                    onClick={handlePushWebDAV}
                    className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-500 px-3.5 py-1.5 text-xs font-semibold text-zinc-950 hover:bg-emerald-400 disabled:opacity-50"
                  >
                    <CloudUpload className="h-3.5 w-3.5" />
                    立即加密同步
                  </button>
                </div>
              </div>

              {davFiles.length > 0 && (
                <div className="divide-y divide-white/[0.06] overflow-hidden rounded-xl border border-white/[0.08] bg-[#0b0b0e]">
                  {davFiles.map((f) => (
                    <div
                      key={f.name}
                      className="flex items-center justify-between px-3.5 py-2.5 text-xs"
                    >
                      <div className="min-w-0">
                        <div className="truncate font-mono text-zinc-200">{f.name}</div>
                        <div className="text-[10px] text-zinc-500">
                          {formatBytes(f.size)}
                          {f.lastModified ? ` · ${f.lastModified}` : ''}
                        </div>
                      </div>
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => handlePullWebDAV(f.name)}
                        className="shrink-0 rounded-md border border-white/[0.08] bg-white/[0.04] px-2.5 py-1 text-[11px] text-sky-300 hover:bg-white/[0.08]"
                      >
                        从此快照恢复
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
};
