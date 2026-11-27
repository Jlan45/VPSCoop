import React, { useCallback, useEffect, useRef, useState } from 'react';
import {
  Folder,
  FileText,
  Link2,
  ArrowUp,
  Home,
  RefreshCw,
  Upload,
  FolderPlus,
  Download,
  Pencil,
  Trash2,
  Eye,
  Terminal as TerminalIcon,
  Search,
  X,
  Save,
} from 'lucide-react';
import { api } from '../api';
import type { FileEntry, Host } from '../types';
import { formatBytes } from '../utils/format';

interface ScpFileManagerProps {
  host: Host;
  endpointId?: string;
  initialPath?: string;
  compact?: boolean;
  onOpenSsh?: (host: Host) => void;
}

export const ScpFileManager: React.FC<ScpFileManagerProps> = ({
  host,
  endpointId,
  initialPath,
  compact = false,
  onOpenSsh,
}) => {
  const [currentPath, setCurrentPath] = useState('~');
  const [pathInput, setPathInput] = useState('~');
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState('');
  const [filter, setFilter] = useState('');
  const [dragOver, setDragOver] = useState(false);
  const [uploadStatus, setUploadStatus] = useState('');
  const dragCounterRef = useRef(0);

  // Preview/Edit modal state
  const [editingFile, setEditingFile] = useState<{
    entry: FileEntry;
    content: string;
    saving: boolean;
  } | null>(null);

  const fileInputRef = useRef<HTMLInputElement | null>(null);

  const loadDir = useCallback(
    async (targetPath: string) => {
      setLoading(true);
      setError('');
      try {
        const res = await api.scpList(host.id, targetPath, endpointId);
        const sorted = [...res.entries].sort((a, b) => {
          if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
          return a.name.localeCompare(b.name);
        });
        setEntries(sorted);
        setCurrentPath(res.path);
        setPathInput(res.path);
      } catch (err: any) {
        setError(err?.message || '无法读取远程目录');
      } finally {
        setLoading(false);
      }
    },
    [host.id, endpointId]
  );

  useEffect(() => {
    loadDir(initialPath || '~');
  }, [loadDir, initialPath]);

  const handleGoUp = () => {
    if (currentPath === '/' || currentPath === '') return;
    const parts = currentPath.split('/').filter(Boolean);
    parts.pop();
    loadDir('/' + parts.join('/'));
  };

  const handleUploadFiles = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    const list = Array.from(files);
    setUploading(true);
    setError('');
    try {
      for (let i = 0; i < list.length; i++) {
        setUploadStatus(`正在上传 (${i + 1}/${list.length}): ${list[i].name}`);
        await api.scpUpload(host.id, currentPath, list[i], endpointId);
      }
      await loadDir(currentPath);
    } catch (err: any) {
      setError(err?.message || 'SCP 上传失败');
    } finally {
      setUploading(false);
      setUploadStatus('');
      if (fileInputRef.current) fileInputRef.current.value = '';
    }
  };

  const handleMkdir = async () => {
    const name = window.prompt('新建文件夹名称：');
    if (!name || !name.trim()) return;
    const target = currentPath === '/' ? `/${name.trim()}` : `${currentPath}/${name.trim()}`;
    try {
      await api.scpMkdir(host.id, target, endpointId);
      await loadDir(currentPath);
    } catch (err: any) {
      setError(err?.message || '创建文件夹失败');
    }
  };

  const handleRename = async (entry: FileEntry) => {
    const nextName = window.prompt('重命名为：', entry.name);
    if (!nextName || !nextName.trim() || nextName.trim() === entry.name) return;
    const newPath =
      currentPath === '/' ? `/${nextName.trim()}` : `${currentPath}/${nextName.trim()}`;
    try {
      await api.scpRename(host.id, entry.path, newPath, endpointId);
      await loadDir(currentPath);
    } catch (err: any) {
      setError(err?.message || '重命名失败');
    }
  };

  const handleDelete = async (entry: FileEntry) => {
    const ok = window.confirm(`确定要永久删除 ${entry.isDir ? '文件夹' : '文件'} "${entry.name}" 吗？`);
    if (!ok) return;
    try {
      await api.scpDelete(host.id, entry.path, endpointId);
      await loadDir(currentPath);
    } catch (err: any) {
      setError(err?.message || '删除失败');
    }
  };

  const handlePreview = async (entry: FileEntry) => {
    setError('');
    try {
      const res = await api.scpRead(host.id, entry.path, endpointId);
      setEditingFile({ entry, content: res.content, saving: false });
    } catch (err: any) {
      setError(err?.message || '无法预览该文件');
    }
  };

  const handleSavePreview = async () => {
    if (!editingFile) return;
    setEditingFile({ ...editingFile, saving: true });
    try {
      await api.scpWrite(host.id, currentPath, editingFile.entry.name, editingFile.content, endpointId);
      setEditingFile(null);
      await loadDir(currentPath);
    } catch (err: any) {
      setError(err?.message || '保存文件失败');
      setEditingFile((prev) => (prev ? { ...prev, saving: false } : null));
    }
  };

  const filteredEntries = entries.filter((e) =>
    filter ? e.name.toLowerCase().includes(filter.toLowerCase()) : true
  );

  const breadcrumbs = currentPath.split('/').filter(Boolean);

  return (
    <div
      className="relative flex h-full flex-col bg-[#09090b]"
      onDragEnter={(e) => {
        e.preventDefault();
        e.stopPropagation();
        dragCounterRef.current += 1;
        setDragOver(true);
      }}
      onDragOver={(e) => {
        e.preventDefault();
        e.stopPropagation();
      }}
      onDragLeave={(e) => {
        e.preventDefault();
        e.stopPropagation();
        dragCounterRef.current -= 1;
        if (dragCounterRef.current <= 0) {
          dragCounterRef.current = 0;
          setDragOver(false);
        }
      }}
      onDrop={(e) => {
        e.preventDefault();
        e.stopPropagation();
        dragCounterRef.current = 0;
        setDragOver(false);
        handleUploadFiles(e.dataTransfer.files);
      }}
    >
      {/* Top Toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-white/[0.08] bg-[#0e0e11] px-4 py-2.5">
        <div className="flex min-w-0 flex-1 items-center gap-1.5">
          <button
            type="button"
            onClick={() => loadDir('~')}
            className="rounded-md border border-white/[0.08] bg-white/[0.03] p-1.5 text-zinc-300 transition hover:bg-white/[0.08]"
            title="主目录 (~)"
          >
            <Home className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={handleGoUp}
            disabled={currentPath === '/'}
            className="rounded-md border border-white/[0.08] bg-white/[0.03] p-1.5 text-zinc-300 transition hover:bg-white/[0.08] disabled:opacity-40"
            title="上级目录"
          >
            <ArrowUp className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={() => loadDir(currentPath)}
            className="rounded-md border border-white/[0.08] bg-white/[0.03] p-1.5 text-zinc-300 transition hover:bg-white/[0.08]"
            title="刷新"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>

          <form
            onSubmit={(e) => {
              e.preventDefault();
              loadDir(pathInput);
            }}
            className="ml-1 flex min-w-0 flex-1 items-center"
          >
            <input
              value={pathInput}
              onChange={(e) => setPathInput(e.target.value)}
              className="w-full rounded-md border border-white/[0.08] bg-zinc-900/90 px-2.5 py-1 font-mono text-xs text-zinc-200 outline-none focus:border-sky-500/60"
              placeholder="/path/to/dir"
            />
          </form>
        </div>

        <div className="flex items-center gap-2">
          <div className="relative hidden sm:block">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-zinc-500" />
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="过滤文件..."
              className="w-36 rounded-md border border-white/[0.08] bg-zinc-900/70 py-1 pl-8 pr-2.5 text-xs text-zinc-200 placeholder-zinc-500 outline-none focus:border-sky-500/60"
            />
          </div>

          <button
            type="button"
            onClick={handleMkdir}
            className="inline-flex items-center gap-1.5 rounded-md border border-white/[0.08] bg-white/[0.03] px-2.5 py-1.5 text-xs text-zinc-200 transition hover:bg-white/[0.08]"
          >
            <FolderPlus className="h-3.5 w-3.5 text-amber-400" />
            新建文件夹
          </button>

          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(e) => handleUploadFiles(e.target.files)}
          />
          <button
            type="button"
            disabled={uploading}
            onClick={() => fileInputRef.current?.click()}
            className="inline-flex items-center gap-1.5 rounded-md bg-sky-500 px-3 py-1.5 text-xs font-semibold text-zinc-950 transition hover:bg-sky-400 disabled:opacity-50"
          >
            <Upload className="h-3.5 w-3.5" />
            {uploading ? uploadStatus || 'SCP 上传中...' : 'SCP 上传（支持拖拽）'}
          </button>

          {!compact && onOpenSsh && (
            <button
              type="button"
              onClick={() => onOpenSsh(host)}
              className="inline-flex items-center gap-1.5 rounded-md border border-white/[0.08] bg-white/[0.03] px-2.5 py-1.5 text-xs text-zinc-200 transition hover:bg-white/[0.08]"
            >
              <TerminalIcon className="h-3.5 w-3.5 text-emerald-400" />
              SSH
            </button>
          )}
        </div>
      </div>

      {/* Breadcrumbs */}
      <div className="flex items-center gap-1 overflow-x-auto border-b border-white/[0.05] bg-[#0b0b0e] px-4 py-1.5 font-mono text-[11px] text-zinc-400">
        <button
          type="button"
          onClick={() => loadDir('/')}
          className="rounded px-1.5 py-0.5 hover:bg-white/[0.06] hover:text-zinc-200"
        >
          /
        </button>
        {breadcrumbs.map((seg, idx) => {
          const target = '/' + breadcrumbs.slice(0, idx + 1).join('/');
          return (
            <React.Fragment key={target}>
              <span className="text-zinc-600">/</span>
              <button
                type="button"
                onClick={() => loadDir(target)}
                className="rounded px-1.5 py-0.5 hover:bg-white/[0.06] hover:text-zinc-200"
              >
                {seg}
              </button>
            </React.Fragment>
          );
        })}
      </div>

      {error && (
        <div className="mx-4 mt-3 flex items-center justify-between rounded-lg border border-red-500/30 bg-red-500/10 px-3.5 py-2 text-xs text-red-300">
          <span>{error}</span>
          <button type="button" onClick={() => setError('')}>
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      )}

      {/* File Table */}
      <div className="min-h-0 flex-1 overflow-auto px-4 py-3">
        <div className="overflow-hidden rounded-xl border border-white/[0.08] bg-[#101014]">
          <table className="w-full text-left text-xs">
            <thead className="border-b border-white/[0.08] bg-white/[0.02] text-[11px] uppercase tracking-wider text-zinc-500">
              <tr>
                <th className="px-4 py-2.5 font-medium">名称</th>
                <th className="hidden px-3 py-2.5 font-medium sm:table-cell">权限</th>
                <th className="px-3 py-2.5 text-right font-medium">大小</th>
                <th className="hidden px-3 py-2.5 font-medium md:table-cell">修改时间</th>
                <th className="px-4 py-2.5 text-right font-medium">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.05]">
              {loading && entries.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-10 text-center text-zinc-500">
                    正在通过 SSH/SCP 读取目录...
                  </td>
                </tr>
              ) : filteredEntries.length === 0 ? (
                <tr>
                  <td colSpan={5} className="px-4 py-10 text-center text-zinc-500">
                    目录为空
                  </td>
                </tr>
              ) : (
                filteredEntries.map((entry) => (
                  <tr
                    key={entry.path}
                    className="group transition hover:bg-white/[0.03]"
                  >
                    <td className="px-4 py-2.5">
                      {entry.isDir ? (
                        <button
                          type="button"
                          onClick={() => loadDir(entry.path)}
                          className="flex items-center gap-2.5 font-medium text-zinc-100 hover:text-sky-400"
                        >
                          <Folder className="h-4 w-4 shrink-0 text-amber-400" />
                          <span className="truncate">{entry.name}</span>
                        </button>
                      ) : (
                        <div className="flex items-center gap-2.5 text-zinc-300">
                          {entry.isSymlink ? (
                            <Link2 className="h-4 w-4 shrink-0 text-violet-400" />
                          ) : (
                            <FileText className="h-4 w-4 shrink-0 text-zinc-500" />
                          )}
                          <span className="truncate">{entry.name}</span>
                        </div>
                      )}
                    </td>
                    <td className="hidden px-3 py-2.5 font-mono text-[11px] text-zinc-500 sm:table-cell">
                      {entry.mode}
                    </td>
                    <td className="px-3 py-2.5 text-right font-mono text-zinc-400">
                      {entry.isDir ? '—' : formatBytes(entry.size)}
                    </td>
                    <td className="hidden px-3 py-2.5 font-mono text-[11px] text-zinc-500 md:table-cell">
                      {entry.modTime}
                    </td>
                    <td className="px-4 py-2.5 text-right">
                      <div className="inline-flex items-center gap-1 opacity-80 transition group-hover:opacity-100">
                        {!entry.isDir && (
                          <>
                            <button
                              type="button"
                              onClick={() => handlePreview(entry)}
                              className="rounded p-1 text-zinc-400 hover:bg-white/[0.08] hover:text-zinc-100"
                              title="查看/编辑文本 (SCP)"
                            >
                              <Eye className="h-3.5 w-3.5" />
                            </button>
                            <a
                              href={api.scpDownloadUrl(host.id, entry.path, endpointId)}
                              className="rounded p-1 text-zinc-400 hover:bg-white/[0.08] hover:text-sky-400"
                              title="SCP 下载"
                            >
                              <Download className="h-3.5 w-3.5" />
                            </a>
                          </>
                        )}
                        <button
                          type="button"
                          onClick={() => handleRename(entry)}
                          className="rounded p-1 text-zinc-400 hover:bg-white/[0.08] hover:text-zinc-100"
                          title="重命名"
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => handleDelete(entry)}
                          className="rounded p-1 text-zinc-400 hover:bg-red-500/15 hover:text-red-400"
                          title="删除"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>

        {/* Visible Drag & Drop Upload Strip */}
        <div
          onClick={() => !uploading && fileInputRef.current?.click()}
          className="mt-3 flex cursor-pointer items-center justify-center gap-2 rounded-xl border border-dashed border-white/[0.1] bg-white/[0.02] px-4 py-3 text-xs text-zinc-400 transition hover:border-sky-400/50 hover:bg-sky-500/5 hover:text-zinc-200"
        >
          <Upload className="h-3.5 w-3.5 text-sky-400" />
          <span>
            {uploading
              ? uploadStatus || '正在通过 SCP 上传...'
              : `拖拽本地文件到页面任意位置，或点击此处通过 SCP 上传至 ${currentPath}`}
          </span>
        </div>
      </div>

      {/* Drag & Drop Overlay */}
      {dragOver && (
        <div className="pointer-events-none absolute inset-3 z-20 flex items-center justify-center rounded-2xl border-2 border-dashed border-sky-400/70 bg-sky-500/10 backdrop-blur-xs">
          <div className="rounded-xl bg-zinc-950/90 px-6 py-4 text-center shadow-xl">
            <Upload className="mx-auto mb-2 h-6 w-6 text-sky-400" />
            <p className="text-sm font-medium text-zinc-100">松开以上传文件到 {currentPath}</p>
            <p className="text-xs text-zinc-400">将通过 SCP 协议传输</p>
          </div>
        </div>
      )}

      {/* Text File Editor Modal */}
      {editingFile && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4 backdrop-blur-sm">
          <div className="flex h-[80vh] w-full max-w-4xl flex-col overflow-hidden rounded-2xl border border-white/[0.1] bg-[#111114] shadow-2xl">
            <div className="flex items-center justify-between border-b border-white/[0.08] px-5 py-3">
              <div>
                <h3 className="font-mono text-sm font-semibold text-zinc-100">
                  {editingFile.entry.path}
                </h3>
                <p className="text-[11px] text-zinc-500">通过 SCP 读取与保存文本文件</p>
              </div>
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  disabled={editingFile.saving}
                  onClick={handleSavePreview}
                  className="inline-flex items-center gap-1.5 rounded-lg bg-emerald-500 px-3 py-1.5 text-xs font-semibold text-zinc-950 transition hover:bg-emerald-400 disabled:opacity-50"
                >
                  <Save className="h-3.5 w-3.5" />
                  {editingFile.saving ? '保存中...' : 'SCP 保存'}
                </button>
                <button
                  type="button"
                  onClick={() => setEditingFile(null)}
                  className="rounded-lg p-1.5 text-zinc-400 hover:bg-white/[0.06] hover:text-zinc-200"
                >
                  <X className="h-4 w-4" />
                </button>
              </div>
            </div>
            <textarea
              value={editingFile.content}
              onChange={(e) =>
                setEditingFile({ ...editingFile, content: e.target.value })
              }
              spellCheck={false}
              className="flex-1 resize-none bg-[#09090b] p-4 font-mono text-xs leading-relaxed text-zinc-200 outline-none"
            />
          </div>
        </div>
      )}
    </div>
  );
};
