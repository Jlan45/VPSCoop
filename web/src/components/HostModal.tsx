import React, { useEffect, useState } from 'react';
import { X, Server, KeyRound, CreditCard, FileText, Plus, Trash2, ArrowUpDown } from 'lucide-react';
import type { AuthType, BillingCycle, Currency, Host, HostEndpoint, HostInput, SSHKey } from '../types';

interface HostModalProps {
  open: boolean;
  host: Host | null;
  savedKeys: SSHKey[];
  onClose: () => void;
  onSave: (input: HostInput, existingId?: string) => Promise<void>;
}

const PROVIDER_PRESETS = [
  'RackNerd',
  'BandwagonHost',
  'CloudCone',
  'Hetzner',
  'Vultr',
  'DigitalOcean',
  'Aliyun',
  'Tencent Cloud',
  'Oracle Cloud',
  'DMIT',
];

const emptyForm = (): HostInput => ({
  name: '',
  groupName: 'Default',
  provider: '',
  region: '',
  host: '',
  port: 22,
  endpoints: [],
  username: 'root',
  authType: 'password',
  keyId: '',
  password: '',
  privateKey: '',
  passphrase: '',
  cost: 0,
  currency: 'USD',
  billingCycle: 'annual',
  expiresAt: '',
  purchasedAt: '',
  specs: '',
  tags: [],
  notes: '',
});

export const HostModal: React.FC<HostModalProps> = ({ open, host, savedKeys, onClose, onSave }) => {
  const [form, setForm] = useState<HostInput>(emptyForm());
  const [tagsText, setTagsText] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    setError('');
    if (host) {
      setForm({
        name: host.name,
        groupName: host.groupName || 'Default',
        provider: host.provider,
        region: host.region,
        host: host.host,
        port: host.port || 22,
        endpoints: host.endpoints || [],
        username: host.username || 'root',
        authType: host.authType || 'password',
        keyId: host.keyId || '',
        password: '',
        privateKey: '',
        passphrase: '',
        cost: host.cost || 0,
        currency: host.currency || 'USD',
        billingCycle: host.billingCycle || 'monthly',
        expiresAt: host.expiresAt || '',
        purchasedAt: host.purchasedAt || '',
        specs: host.specs || '',
        tags: host.tags || [],
        notes: host.notes || '',
      });
      setTagsText((host.tags || []).join(', '));
    } else {
      setForm(emptyForm());
      setTagsText('');
    }
  }, [open, host]);

  if (!open) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setSaving(true);
    try {
      const tags = tagsText
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean);
      await onSave({ ...form, tags }, host?.id);
      onClose();
    } catch (err: any) {
      setError(err?.message || '保存失败');
    } finally {
      setSaving(false);
    }
  };

  const inputCls =
    'w-full rounded-lg border border-white/[0.08] bg-zinc-900/80 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-500 outline-none transition focus:border-emerald-500/60 focus:ring-2 focus:ring-emerald-500/15';
  const labelCls = 'mb-1.5 block text-xs font-medium text-zinc-400';

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-white/[0.1] bg-[#111114] shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-white/[0.08] px-6 py-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-400">
              <Server className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-base font-semibold text-zinc-100">
                {host ? `编辑主机 · ${host.name}` : '添加新 VPS 主机'}
              </h2>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg p-1.5 text-zinc-400 transition hover:bg-white/[0.06] hover:text-zinc-200"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* Form Body */}
        <form onSubmit={handleSubmit} className="flex-1 space-y-6 overflow-y-auto px-6 py-5">
          {error && (
            <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-xs text-red-300">
              {error}
            </div>
          )}

          {/* Section 1: Basic & SSH */}
          <section className="space-y-4">
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-zinc-400">
              <KeyRound className="h-3.5 w-3.5 text-emerald-400" />
              <span>连接信息</span>
            </div>

            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
              <div>
                <label className={labelCls}>主机名称 *</label>
                <input
                  required
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="例如：LA-CN2-GIA-01"
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>分组</label>
                <input
                  value={form.groupName}
                  onChange={(e) => setForm({ ...form, groupName: e.target.value })}
                  placeholder="例如：Production / Proxy / Lab"
                  className={inputCls}
                />
              </div>
            </div>

            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-4">
              <div className="sm:col-span-2">
                <label className={labelCls}>IP 地址 / 域名 *</label>
                <input
                  required
                  value={form.host}
                  onChange={(e) => setForm({ ...form, host: e.target.value })}
                  placeholder="192.0.2.10 或 vps.example.com"
                  className={`${inputCls} font-mono`}
                />
              </div>
              <div>
                <label className={labelCls}>端口</label>
                <input
                  type="number"
                  min={1}
                  max={65535}
                  value={form.port}
                  onChange={(e) => setForm({ ...form, port: Number(e.target.value) || 22 })}
                  className={`${inputCls} font-mono`}
                />
              </div>
              <div>
                <label className={labelCls}>用户名</label>
                <input
                  value={form.username}
                  onChange={(e) => setForm({ ...form, username: e.target.value })}
                  placeholder="root"
                  className={`${inputCls} font-mono`}
                />
              </div>
            </div>

            {/* Extra Endpoints */}
            <div className="rounded-xl border border-white/[0.06] bg-zinc-950/50 p-3.5 space-y-2.5">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-zinc-300">
                  备用连接入口 / 多 IP（IPv6、中转、DDNS、内网）
                </span>
                <button
                  type="button"
                  onClick={() =>
                    setForm({
                      ...form,
                      endpoints: [
                        ...(form.endpoints || []),
                        { id: crypto.randomUUID(), label: '', host: '', port: 22 },
                      ],
                    })
                  }
                  className="inline-flex items-center gap-1 rounded-md border border-white/[0.08] bg-white/[0.04] px-2 py-1 text-[11px] text-zinc-200 hover:bg-white/[0.08]"
                >
                  <Plus className="h-3 w-3 text-emerald-400" />
                  添加入口
                </button>
              </div>
              {(form.endpoints || []).length > 0 && (
                <div className="space-y-2">
                  {(form.endpoints || []).map((ep, idx) => (
                    <div key={ep.id || idx} className="grid grid-cols-12 items-center gap-2">
                      <input
                        value={ep.label}
                        onChange={(e) => {
                          const next = [...(form.endpoints || [])];
                          next[idx] = { ...ep, label: e.target.value };
                          setForm({ ...form, endpoints: next });
                        }}
                        placeholder="标签 (如 IPv6 / 中转)"
                        className={`${inputCls} col-span-3 py-1.5 text-xs`}
                      />
                      <input
                        value={ep.host}
                        onChange={(e) => {
                          const next = [...(form.endpoints || [])];
                          next[idx] = { ...ep, host: e.target.value };
                          setForm({ ...form, endpoints: next });
                        }}
                        placeholder="IP / 域名"
                        className={`${inputCls} col-span-5 py-1.5 font-mono text-xs`}
                      />
                      <input
                        type="number"
                        min={1}
                        max={65535}
                        value={ep.port}
                        onChange={(e) => {
                          const next = [...(form.endpoints || [])];
                          next[idx] = { ...ep, port: Number(e.target.value) || 22 };
                          setForm({ ...form, endpoints: next });
                        }}
                        className={`${inputCls} col-span-2 py-1.5 font-mono text-xs`}
                      />
                      <div className="col-span-2 flex items-center justify-end gap-1">
                        <button
                          type="button"
                          onClick={() => {
                            const next = [...(form.endpoints || [])];
                            const oldPrimary: HostEndpoint = {
                              id: ep.id || crypto.randomUUID(),
                              label: ep.label || '备用',
                              host: form.host,
                              port: form.port,
                            };
                            next[idx] = oldPrimary;
                            setForm({
                              ...form,
                              host: ep.host,
                              port: ep.port,
                              endpoints: next,
                            });
                          }}
                          className="rounded p-1.5 text-zinc-400 hover:bg-white/[0.06] hover:text-sky-400"
                          title="设为默认主入口"
                        >
                          <ArrowUpDown className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => {
                            const next = (form.endpoints || []).filter((_, i) => i !== idx);
                            setForm({ ...form, endpoints: next });
                          }}
                          className="rounded p-1.5 text-zinc-400 hover:bg-red-500/15 hover:text-red-400"
                          title="移除"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div>
              <label className={labelCls}>认证方式</label>
              <div className="inline-flex rounded-lg border border-white/[0.08] bg-zinc-900/70 p-1">
                {(['password', 'key'] as AuthType[]).map((type) => (
                  <button
                    key={type}
                    type="button"
                    onClick={() => setForm({ ...form, authType: type })}
                    className={`rounded-md px-3.5 py-1.5 text-xs font-medium transition ${
                      form.authType === type
                        ? 'bg-emerald-500/20 text-emerald-300'
                        : 'text-zinc-400 hover:text-zinc-200'
                    }`}
                  >
                    {type === 'password' ? '密码认证' : 'SSH 私钥'}
                  </button>
                ))}
              </div>
            </div>

            {form.authType === 'password' ? (
              <div>
                <label className={labelCls}>
                  SSH 密码 {host?.hasPassword ? '（留空则保留当前已加密密码）' : ''}
                </label>
                <input
                  type="password"
                  value={form.password}
                  onChange={(e) => setForm({ ...form, password: e.target.value })}
                  placeholder={host?.hasPassword ? '••••••••••••' : '输入 SSH 登录密码'}
                  className={inputCls}
                />
              </div>
            ) : (
              <div className="space-y-3">
                <div>
                  <label className={labelCls}>从密钥库选择 (Key Vault)</label>
                  <select
                    value={form.keyId || ''}
                    onChange={(e) => setForm({ ...form, keyId: e.target.value })}
                    className={inputCls}
                  >
                    <option value="">-- 手动填写下方私钥 --</option>
                    {savedKeys.map((k) => (
                      <option key={k.id} value={k.id}>
                        {k.name} ({k.algorithm} · {k.fingerprint.slice(0, 18)}...)
                      </option>
                    ))}
                  </select>
                </div>
                {!form.keyId && (
                  <>
                    <div>
                      <label className={labelCls}>
                        私钥内容 (PEM / OpenSSH){' '}
                        {host?.hasPrivateKey ? '（留空则保留当前私钥）' : ''}
                      </label>
                      <textarea
                        rows={4}
                        value={form.privateKey}
                        onChange={(e) => setForm({ ...form, privateKey: e.target.value })}
                        placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"
                        className={`${inputCls} font-mono text-xs`}
                      />
                    </div>
                    <div>
                      <label className={labelCls}>私钥密码 (Passphrase，可选)</label>
                      <input
                        type="password"
                        value={form.passphrase}
                        onChange={(e) => setForm({ ...form, passphrase: e.target.value })}
                        placeholder="如果私钥设置了密码请填写"
                        className={inputCls}
                      />
                    </div>
                  </>
                )}
              </div>
            )}
          </section>

          {/* Section 2: Provider & Billing */}
          <section className="space-y-4 border-t border-white/[0.08] pt-5">
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-zinc-400">
              <CreditCard className="h-3.5 w-3.5 text-sky-400" />
              <span>购买商与费用信息</span>
            </div>

            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
              <div>
                <label className={labelCls}>购买商 / 厂商</label>
                <input
                  list="provider-presets"
                  value={form.provider}
                  onChange={(e) => setForm({ ...form, provider: e.target.value })}
                  placeholder="例如：RackNerd / BandwagonHost / Hetzner"
                  className={inputCls}
                />
                <datalist id="provider-presets">
                  {PROVIDER_PRESETS.map((p) => (
                    <option key={p} value={p} />
                  ))}
                </datalist>
              </div>
              <div>
                <label className={labelCls}>机房 / 地区</label>
                <input
                  value={form.region}
                  onChange={(e) => setForm({ ...form, region: e.target.value })}
                  placeholder="例如：Los Angeles DC02 / Tokyo"
                  className={inputCls}
                />
              </div>
            </div>

            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
              <div>
                <label className={labelCls}>花费金额</label>
                <input
                  type="number"
                  step="0.01"
                  min={0}
                  value={form.cost}
                  onChange={(e) => setForm({ ...form, cost: parseFloat(e.target.value) || 0 })}
                  className={`${inputCls} font-mono`}
                />
              </div>
              <div>
                <label className={labelCls}>货币</label>
                <select
                  value={form.currency}
                  onChange={(e) => setForm({ ...form, currency: e.target.value as Currency })}
                  className={inputCls}
                >
                  <option value="USD">USD ($)</option>
                  <option value="CNY">CNY (¥)</option>
                  <option value="EUR">EUR (€)</option>
                  <option value="GBP">GBP (£)</option>
                  <option value="JPY">JPY (¥)</option>
                </select>
              </div>
              <div>
                <label className={labelCls}>计费周期</label>
                <select
                  value={form.billingCycle}
                  onChange={(e) =>
                    setForm({ ...form, billingCycle: e.target.value as BillingCycle })
                  }
                  className={inputCls}
                >
                  <option value="monthly">月付 (Monthly)</option>
                  <option value="quarterly">季付 (Quarterly)</option>
                  <option value="semiannual">半年付 (Semi-annual)</option>
                  <option value="annual">年付 (Annual)</option>
                  <option value="biennial">两年付 (Biennial)</option>
                  <option value="triennial">三年付 (Triennial)</option>
                  <option value="onetime">一次性 (One-time)</option>
                </select>
              </div>
            </div>

            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
              <div>
                <label className={labelCls}>到期时间</label>
                <input
                  type="date"
                  value={form.expiresAt}
                  onChange={(e) => setForm({ ...form, expiresAt: e.target.value })}
                  className={`${inputCls} font-mono`}
                />
              </div>
              <div>
                <label className={labelCls}>购买时间</label>
                <input
                  type="date"
                  value={form.purchasedAt}
                  onChange={(e) => setForm({ ...form, purchasedAt: e.target.value })}
                  className={`${inputCls} font-mono`}
                />
              </div>
            </div>
          </section>

          {/* Section 3: Specs & Notes */}
          <section className="space-y-4 border-t border-white/[0.08] pt-5">
            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-zinc-400">
              <FileText className="h-3.5 w-3.5 text-violet-400" />
              <span>配置与备注</span>
            </div>

            <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
              <div>
                <label className={labelCls}>硬件配置</label>
                <input
                  value={form.specs}
                  onChange={(e) => setForm({ ...form, specs: e.target.value })}
                  placeholder="例如：2 vCPU · 2GB RAM · 40GB SSD"
                  className={inputCls}
                />
              </div>
              <div>
                <label className={labelCls}>标签（逗号分隔）</label>
                <input
                  value={tagsText}
                  onChange={(e) => setTagsText(e.target.value)}
                  placeholder="cn2, docker, backup"
                  className={inputCls}
                />
              </div>
            </div>

            <div>
              <label className={labelCls}>备注</label>
              <textarea
                rows={2}
                value={form.notes}
                onChange={(e) => setForm({ ...form, notes: e.target.value })}
                placeholder="记录面板地址、续费优惠码、用途说明等..."
                className={inputCls}
              />
            </div>
          </section>
        </form>

        {/* Footer */}
        <div className="flex items-center justify-end gap-2.5 border-t border-white/[0.08] bg-zinc-950/60 px-6 py-4">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-white/[0.08] px-4 py-2 text-xs font-medium text-zinc-300 transition hover:bg-white/[0.05]"
          >
            取消
          </button>
          <button
            type="button"
            disabled={saving}
            onClick={handleSubmit}
            className="rounded-lg bg-emerald-500 px-4 py-2 text-xs font-semibold text-zinc-950 transition hover:bg-emerald-400 disabled:opacity-50"
          >
            {saving ? '保存中...' : host ? '保存修改' : '添加主机'}
          </button>
        </div>
      </div>
    </div>
  );
};
