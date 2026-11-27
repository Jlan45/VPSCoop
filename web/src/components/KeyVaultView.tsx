import React, { useState } from 'react';
import {
  KeyRound,
  Plus,
  Copy,
  Check,
  Download,
  Trash2,
  ShieldCheck,
  Terminal,
  X,
  Sparkles,
  Eye,
  EyeOff,
  Pencil,
} from 'lucide-react';
import { api } from '../api';
import type { SSHKey } from '../types';

interface KeyVaultViewProps {
  keys: SSHKey[];
  onRefresh: () => Promise<void>;
}

export const KeyVaultView: React.FC<KeyVaultViewProps> = ({ keys, onRefresh }) => {
  const [modalOpen, setModalOpen] = useState(false);
  const [editingKey, setEditingKey] = useState<SSHKey | null>(null);

  const [name, setName] = useState('');
  const [algorithm, setAlgorithm] = useState<string>('ed25519');
  const [publicKey, setPublicKey] = useState('');
  const [privateKey, setPrivateKey] = useState('');
  const [passphrase, setPassphrase] = useState('');
  const [comment, setComment] = useState('vpscoop');

  const [showPrivate, setShowPrivate] = useState(false);
  const [showPass, setShowPass] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [saving, setSaving] = useState(false);
  const [loadingSecret, setLoadingSecret] = useState(false);
  const [error, setError] = useState('');

  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [copiedField, setCopiedField] = useState<string | null>(null);

  const openCreateModal = () => {
    setEditingKey(null);
    setName('');
    setAlgorithm('ed25519');
    setPublicKey('');
    setPrivateKey('');
    setPassphrase('');
    setComment('vpscoop');
    setShowPrivate(true);
    setShowPass(false);
    setError('');
    setModalOpen(true);
  };

  const openEditModal = async (k: SSHKey) => {
    setEditingKey(k);
    setName(k.name);
    setAlgorithm(k.algorithm || 'ed25519');
    setPublicKey(k.publicKey);
    setPrivateKey('');
    setPassphrase('');
    setComment(k.comment || '');
    setShowPrivate(false);
    setShowPass(false);
    setError('');
    setModalOpen(true);

    setLoadingSecret(true);
    try {
      const full = await api.getKeySecret(k.id);
      setPrivateKey(full.privateKey || '');
      setPassphrase(full.passphrase || '');
      setPublicKey(full.publicKey || k.publicKey);
    } catch (err: any) {
      setError(err?.message || '读取私钥详情失败');
    } finally {
      setLoadingSecret(false);
    }
  };

  const handleGenerateInline = async () => {
    setError('');
    setGenerating(true);
    try {
      const res = await api.generateKey({
        algorithm,
        comment: comment || 'vpscoop',
        passphrase,
        dryRun: true,
      });
      setPublicKey(res.publicKey);
      setPrivateKey(res.privateKey);
      setShowPrivate(true);
      if (!name.trim()) {
        setName(`${algorithm.toUpperCase()} Key`);
      }
    } catch (err: any) {
      setError(err?.message || '生成密钥对失败');
    } finally {
      setGenerating(false);
    }
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    if (!name.trim()) {
      setError('请输入密钥名称');
      return;
    }
    if (!privateKey.trim()) {
      setError('请粘贴私钥或点击「生成密钥对」');
      return;
    }

    setSaving(true);
    try {
      if (editingKey) {
        await api.updateKey(editingKey.id, {
          name,
          algorithm,
          publicKey,
          privateKey,
          passphrase,
          comment,
        });
      } else {
        await api.importKey({
          name,
          privateKey,
          passphrase,
          comment,
        });
      }
      setModalOpen(false);
      await onRefresh();
    } catch (err: any) {
      setError(err?.message || '保存失败');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (k: SSHKey) => {
    if (!window.confirm(`确定删除密钥 "${k.name}" 吗？关联该密钥的主机将解除绑定。`)) return;
    try {
      await api.deleteKey(k.id);
      await onRefresh();
    } catch (err: any) {
      setError(err?.message || '删除密钥失败');
    }
  };

  const copyText = (text: string, tag: string) => {
    navigator.clipboard.writeText(text);
    setCopiedField(tag);
    window.setTimeout(() => setCopiedField((cur) => (cur === tag ? null : cur)), 1500);
  };

  const copyDeployCmd = (k: SSHKey) => {
    const cmd = `mkdir -p ~/.ssh && chmod 700 ~/.ssh && echo '${k.publicKey}' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys`;
    navigator.clipboard.writeText(cmd);
    setCopiedId(k.id);
    window.setTimeout(() => setCopiedId((cur) => (cur === k.id ? null : cur)), 1500);
  };

  const inputCls =
    'w-full rounded-lg border border-white/[0.08] bg-zinc-900/80 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-500 outline-none transition focus:border-violet-500/60 focus:ring-2 focus:ring-violet-500/15';
  const labelCls = 'mb-1.5 block text-xs font-medium text-zinc-400';

  return (
    <div className="h-full overflow-y-auto px-6 py-6">
      <div className="mx-auto max-w-6xl space-y-6">
        {/* Header */}
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold tracking-tight text-zinc-100">
              密钥管理 (Keychain)
            </h1>
            <p className="mt-1 text-xs text-zinc-400">
              参考 Termius Keychain 设计：统一管理密钥名称、类型、公钥与加密私钥，支持一键生成密钥对
            </p>
          </div>
          <button
            type="button"
            onClick={openCreateModal}
            className="inline-flex items-center gap-1.5 rounded-xl bg-emerald-500 px-4 py-2 text-xs font-semibold text-zinc-950 shadow-lg shadow-emerald-500/15 transition hover:bg-emerald-400"
          >
            <Plus className="h-4 w-4" />
            添加 / 生成密钥
          </button>
        </div>

        {error && !modalOpen && (
          <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-xs text-red-300">
            {error}
          </div>
        )}

        {/* Key List */}
        {keys.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-white/[0.1] bg-[#101013] py-16 text-center">
            <KeyRound className="mb-3 h-8 w-8 text-zinc-600" />
            <div className="text-sm font-medium text-zinc-300">密钥库为空</div>
            <p className="mt-1 text-xs text-zinc-500">
              点击「添加 / 生成密钥」创建 Ed25519 / RSA 密钥对或导入已有私钥
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            {keys.map((k) => (
              <div
                key={k.id}
                className="flex flex-col justify-between rounded-2xl border border-white/[0.08] bg-[#111114] p-5 transition hover:border-white/[0.16]"
              >
                <div className="space-y-3.5">
                  {/* Top Row: Name + Type + Actions */}
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-3">
                      <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-violet-500/15 text-violet-400 ring-1 ring-violet-500/25">
                        <KeyRound className="h-4 w-4" />
                      </div>
                      <div>
                        <div className="flex items-center gap-2">
                          <h3 className="text-sm font-semibold text-zinc-100">{k.name}</h3>
                          <span className="rounded-md border border-violet-500/25 bg-violet-500/10 px-2 py-0.5 font-mono text-[10px] font-semibold uppercase text-violet-300">
                            {k.algorithm}
                          </span>
                          {k.hasPassphrase && (
                            <span className="inline-flex items-center gap-1 rounded-md border border-emerald-500/20 bg-emerald-500/10 px-1.5 py-0.5 text-[10px] text-emerald-300">
                              <ShieldCheck className="h-3 w-3" />
                              Passphrase
                            </span>
                          )}
                        </div>
                        <div className="mt-0.5 font-mono text-[11px] text-zinc-500">
                          {k.fingerprint}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-1">
                      <button
                        type="button"
                        onClick={() => openEditModal(k)}
                        className="rounded-lg p-1.5 text-zinc-400 transition hover:bg-white/[0.06] hover:text-zinc-100"
                        title="查看/编辑公钥与私钥"
                      >
                        <Pencil className="h-4 w-4" />
                      </button>
                      <a
                        href={api.downloadPrivateKeyUrl(k.id)}
                        className="rounded-lg p-1.5 text-zinc-400 transition hover:bg-white/[0.06] hover:text-sky-400"
                        title="下载私钥 (.pem)"
                      >
                        <Download className="h-4 w-4" />
                      </a>
                      <button
                        type="button"
                        onClick={() => handleDelete(k)}
                        className="rounded-lg p-1.5 text-zinc-400 transition hover:bg-red-500/15 hover:text-red-400"
                        title="删除密钥"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                  </div>

                  {/* Public Key Field */}
                  <div className="rounded-xl border border-white/[0.06] bg-[#09090b] p-3">
                    <div className="mb-1 flex items-center justify-between text-[10px] uppercase tracking-wider text-zinc-500">
                      <span>公钥 (Public Key)</span>
                      <button
                        type="button"
                        onClick={() => copyText(k.publicKey, `pub-${k.id}`)}
                        className="inline-flex items-center gap-1 text-zinc-400 hover:text-emerald-400"
                      >
                        {copiedField === `pub-${k.id}` ? (
                          <>
                            <Check className="h-3 w-3 text-emerald-400" />
                            <span className="text-emerald-400">已复制</span>
                          </>
                        ) : (
                          <>
                            <Copy className="h-3 w-3" />
                            <span>复制公钥</span>
                          </>
                        )}
                      </button>
                    </div>
                    <div className="truncate font-mono text-[11px] text-zinc-400">
                      {k.publicKey}
                    </div>
                  </div>

                  {/* Private Key Status Row */}
                  <div className="flex items-center justify-between rounded-xl border border-white/[0.06] bg-[#09090b] px-3 py-2">
                    <div className="flex items-center gap-2 text-[11px]">
                      <span className="text-zinc-500">私钥 (Private Key)：</span>
                      <span className="font-mono text-zinc-400">••••••••••••••••••••</span>
                    </div>
                    <button
                      type="button"
                      onClick={() => openEditModal(k)}
                      className="inline-flex items-center gap-1 text-[11px] text-sky-400 hover:text-sky-300"
                    >
                      <Eye className="h-3 w-3" />
                      查看 / 编辑私钥
                    </button>
                  </div>
                </div>

                {/* Footer: One-click Deploy */}
                <div className="mt-4 flex items-center justify-between border-t border-white/[0.06] pt-3">
                  <span className="text-[11px] text-zinc-500">快速部署到服务器：</span>
                  <button
                    type="button"
                    onClick={() => copyDeployCmd(k)}
                    className="inline-flex items-center gap-1.5 rounded-lg border border-white/[0.08] bg-white/[0.03] px-2.5 py-1 text-[11px] text-zinc-300 transition hover:bg-white/[0.08]"
                  >
                    {copiedId === k.id ? (
                      <>
                        <Check className="h-3 w-3 text-emerald-400" />
                        <span className="text-emerald-300">已复制安装命令</span>
                      </>
                    ) : (
                      <>
                        <Terminal className="h-3 w-3 text-emerald-400" />
                        <span>复制 authorized_keys 命令</span>
                      </>
                    )}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Termius-style Add / Edit / Generate Key Modal */}
      {modalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
          <div className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-white/[0.1] bg-[#111114] shadow-2xl">
            <div className="flex items-center justify-between border-b border-white/[0.08] px-6 py-4">
              <div className="flex items-center gap-2.5">
                <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-violet-500/15 text-violet-400">
                  <KeyRound className="h-4 w-4" />
                </div>
                <div>
                  <h2 className="text-base font-semibold text-zinc-100">
                    {editingKey ? `编辑密钥 · ${editingKey.name}` : '添加或生成 SSH 密钥'}
                  </h2>
                  <p className="text-xs text-zinc-500">
                    管理密钥名称、密钥类型、公钥与私钥
                  </p>
                </div>
              </div>
              <button
                type="button"
                onClick={() => setModalOpen(false)}
                className="rounded-lg p-1.5 text-zinc-400 hover:bg-white/[0.06] hover:text-zinc-200"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            <form onSubmit={handleSave} className="flex-1 space-y-4 overflow-y-auto px-6 py-5">
              {error && (
                <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-xs text-red-300">
                  {error}
                </div>
              )}

              {/* Name & Type */}
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <label className={labelCls}>名称 (Label) *</label>
                  <input
                    required
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="例如：My Production Key"
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className={labelCls}>密钥类型 (Key Type)</label>
                  <div className="flex gap-2">
                    <select
                      value={algorithm}
                      onChange={(e) => setAlgorithm(e.target.value)}
                      className={inputCls}
                    >
                      <option value="ed25519">Ed25519 (推荐)</option>
                      <option value="ecdsa-p256">ECDSA P-256 (NIST P-256)</option>
                      <option value="ecdsa-p384">ECDSA P-384 (NIST P-384)</option>
                      <option value="ecdsa-p521">ECDSA P-521 (NIST P-521)</option>
                      <option value="rsa-2048">RSA 2048</option>
                      <option value="rsa-3072">RSA 3072</option>
                      <option value="rsa-4096">RSA 4096</option>
                    </select>
                    <button
                      type="button"
                      disabled={generating}
                      onClick={handleGenerateInline}
                      className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-emerald-500/30 bg-emerald-500/15 px-3 py-2 text-xs font-medium text-emerald-300 transition hover:bg-emerald-500/25 disabled:opacity-50"
                      title="自动生成新的公钥与私钥"
                    >
                      <Sparkles className="h-3.5 w-3.5" />
                      {generating ? '生成中...' : '生成密钥对'}
                    </button>
                  </div>
                </div>
              </div>

              {/* Public Key */}
              <div>
                <div className="mb-1.5 flex items-center justify-between">
                  <label className="text-xs font-medium text-zinc-400">公钥 (Public Key)</label>
                  {publicKey && (
                    <button
                      type="button"
                      onClick={() => copyText(publicKey, 'modal-pub')}
                      className="inline-flex items-center gap-1 text-[11px] text-zinc-400 hover:text-emerald-400"
                    >
                      {copiedField === 'modal-pub' ? (
                        <>
                          <Check className="h-3 w-3 text-emerald-400" />
                          <span className="text-emerald-400">已复制</span>
                        </>
                      ) : (
                        <>
                          <Copy className="h-3 w-3" />
                          <span>复制公钥</span>
                        </>
                      )}
                    </button>
                  )}
                </div>
                <textarea
                  rows={2}
                  value={publicKey}
                  onChange={(e) => setPublicKey(e.target.value)}
                  placeholder="ssh-ed25519 AAAA... (生成或保存私钥时自动提取)"
                  className={`${inputCls} font-mono text-xs`}
                />
              </div>

              {/* Private Key */}
              <div>
                <div className="mb-1.5 flex items-center justify-between">
                  <label className="text-xs font-medium text-zinc-400">
                    私钥 (Private Key) * {loadingSecret ? '（加载中...）' : ''}
                  </label>
                  <div className="flex items-center gap-3">
                    {privateKey && (
                      <button
                        type="button"
                        onClick={() => copyText(privateKey, 'modal-priv')}
                        className="inline-flex items-center gap-1 text-[11px] text-zinc-400 hover:text-sky-400"
                      >
                        {copiedField === 'modal-priv' ? (
                          <>
                            <Check className="h-3 w-3 text-emerald-400" />
                            <span className="text-emerald-400">已复制</span>
                          </>
                        ) : (
                          <>
                            <Copy className="h-3 w-3" />
                            <span>复制私钥</span>
                          </>
                        )}
                      </button>
                    )}
                    <button
                      type="button"
                      onClick={() => setShowPrivate((v) => !v)}
                      className="inline-flex items-center gap-1 text-[11px] text-zinc-400 hover:text-zinc-200"
                    >
                      {showPrivate ? (
                        <>
                          <EyeOff className="h-3 w-3" />
                          <span>隐藏私钥</span>
                        </>
                      ) : (
                        <>
                          <Eye className="h-3 w-3" />
                          <span>显示私钥</span>
                        </>
                      )}
                    </button>
                  </div>
                </div>
                {showPrivate ? (
                  <textarea
                    required
                    rows={6}
                    value={privateKey}
                    onChange={(e) => setPrivateKey(e.target.value)}
                    placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                    className={`${inputCls} font-mono text-xs`}
                  />
                ) : (
                  <div
                    onClick={() => setShowPrivate(true)}
                    className="flex h-24 cursor-pointer items-center justify-center rounded-lg border border-white/[0.08] bg-zinc-900/60 font-mono text-xs text-zinc-500 transition hover:border-white/[0.15] hover:text-zinc-300"
                  >
                    ••••••••••••••••••••••••••••（点击显示或编辑私钥）
                  </div>
                )}
              </div>

              {/* Passphrase & Comment */}
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <div className="mb-1.5 flex items-center justify-between">
                    <label className="text-xs font-medium text-zinc-400">
                      私钥密码 (Passphrase，可选)
                    </label>
                    <button
                      type="button"
                      onClick={() => setShowPass((v) => !v)}
                      className="text-[11px] text-zinc-500 hover:text-zinc-300"
                    >
                      {showPass ? '隐藏' : '显示'}
                    </button>
                  </div>
                  <input
                    type={showPass ? 'text' : 'password'}
                    value={passphrase}
                    onChange={(e) => setPassphrase(e.target.value)}
                    placeholder="如有加密请填写"
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className={labelCls}>注释 (Comment，可选)</label>
                  <input
                    value={comment}
                    onChange={(e) => setComment(e.target.value)}
                    placeholder="vpscoop"
                    className={`${inputCls} font-mono`}
                  />
                </div>
              </div>
            </form>

            <div className="flex items-center justify-end gap-2.5 border-t border-white/[0.08] bg-zinc-950/60 px-6 py-4">
              <button
                type="button"
                onClick={() => setModalOpen(false)}
                className="rounded-lg border border-white/[0.08] px-4 py-2 text-xs font-medium text-zinc-300 hover:bg-white/[0.05]"
              >
                取消
              </button>
              <button
                type="button"
                disabled={saving}
                onClick={handleSave}
                className="rounded-lg bg-emerald-500 px-4 py-2 text-xs font-semibold text-zinc-950 transition hover:bg-emerald-400 disabled:opacity-50"
              >
                {saving ? '保存中...' : editingKey ? '保存修改' : '保存到密钥库'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
