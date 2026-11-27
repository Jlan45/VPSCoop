import React, { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Server,
  Plus,
  Search,
  Terminal as TerminalIcon,
  FolderOpen,
  CreditCard,
  Copy,
  Check,
  Pencil,
  Trash2,
  Activity,
  X,
  AlertTriangle,
  Sparkles,
  Globe,
  Cpu,
  KeyRound,
  Receipt,
  Sun,
  Moon,
  Upload,
  Network,
  ShieldCheck,
  Lock,
  Bot,
  Route,
  LayoutGrid,
  List,
  Menu,
} from 'lucide-react';
import { api } from './api';
import type { Currency, Host, HostEndpoint, HostInput, SSHKey, WorkspaceTab } from './types';
import { CURRENCY_SYMBOLS, formatCost, getExpiryInfo, toMonthlyEquivalent } from './utils/format';
import { SetupScreen } from './components/SetupScreen';

const HostModal = lazy(() => import('./components/HostModal').then((m) => ({ default: m.HostModal })));
const TerminalView = lazy(() => import('./components/TerminalView').then((m) => ({ default: m.TerminalView })));
const ScpFileManager = lazy(() => import('./components/ScpFileManager').then((m) => ({ default: m.ScpFileManager })));
const BillingView = lazy(() => import('./components/BillingView').then((m) => ({ default: m.BillingView })));
const KeyVaultView = lazy(() => import('./components/KeyVaultView').then((m) => ({ default: m.KeyVaultView })));
const KeyInjectModal = lazy(() => import('./components/KeyInjectModal').then((m) => ({ default: m.KeyInjectModal })));
const InvoiceModal = lazy(() => import('./components/InvoiceModal').then((m) => ({ default: m.InvoiceModal })));
const ImportModal = lazy(() => import('./components/ImportModal').then((m) => ({ default: m.ImportModal })));
const TailscaleModal = lazy(() => import('./components/TailscaleModal').then((m) => ({ default: m.TailscaleModal })));
const BackupModal = lazy(() => import('./components/BackupModal').then((m) => ({ default: m.BackupModal })));
const McpModal = lazy(() => import('./components/McpModal').then((m) => ({ default: m.McpModal })));
const NetworkModal = lazy(() => import('./components/NetworkModal').then((m) => ({ default: m.NetworkModal })));

type MainSection = 'hosts' | 'keys' | 'billing';

export default function App() {
  const [authState, setAuthState] = useState<{
    initialized: boolean;
    unlocked: boolean;
    authenticated: boolean;
    version?: string;
  } | null>(null);
  const [hosts, setHosts] = useState<Host[]>([]);
  const [savedKeys, setSavedKeys] = useState<SSHKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const [section, setSection] = useState<MainSection>('hosts');
  const [tabs, setTabs] = useState<WorkspaceTab[]>([]);
  const [activeTabId, setActiveTabId] = useState<string>('main');
  const tabsLoadedRef = useRef(false);

  const [search, setSearch] = useState('');
  const [selectedGroup, setSelectedGroup] = useState<string>('ALL');
  const [onlyExpiring, setOnlyExpiring] = useState(false);

  const [modalOpen, setModalOpen] = useState(false);
  const [editingHost, setEditingHost] = useState<Host | null>(null);
  const [injectHost, setInjectHost] = useState<Host | null>(null);
  const [invoiceHost, setInvoiceHost] = useState<Host | null>(null);
  const [tailscaleHost, setTailscaleHost] = useState<Host | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [backupOpen, setBackupOpen] = useState(false);
  const [mcpOpen, setMcpOpen] = useState(false);
  const [networkHost, setNetworkHost] = useState<Host | null>(null);
  const [mcpCopied, setMcpCopied] = useState(false);

  const handleQuickCopyMcp = async () => {
    try {
      const res = await api.getMCPToken();
      const snippet = JSON.stringify(
        {
          mcpServers: {
            vpscoop: {
              url: `${window.location.origin}/mcp`,
              headers: {
                Authorization: `Bearer ${res.token}`,
              },
            },
          },
        },
        null,
        2
      );
      await navigator.clipboard.writeText(snippet);
      setMcpCopied(true);
      window.setTimeout(() => setMcpCopied(false), 1800);
    } catch {
      setMcpOpen(true);
    }
  };

  const [theme, setTheme] = useState<'dark' | 'light'>(() => {
    const saved = window.localStorage.getItem('vpscoop-theme');
    return saved === 'light' ? 'light' : 'dark';
  });

  useEffect(() => {
    const root = document.documentElement;
    if (theme === 'light') {
      root.classList.remove('dark');
      root.classList.add('light');
    } else {
      root.classList.remove('light');
      root.classList.add('dark');
    }
    window.localStorage.setItem('vpscoop-theme', theme);
  }, [theme]);

  const toggleTheme = () => setTheme((t) => (t === 'dark' ? 'light' : 'dark'));

  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [testingId, setTestingId] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<Record<string, { ok: boolean; msg: string }>>({});
  const [viewMode, setViewMode] = useState<'grid' | 'list'>(() => {
    return window.localStorage.getItem('vpscoop-view') === 'list' ? 'list' : 'grid';
  });
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    window.localStorage.setItem('vpscoop-view', viewMode);
  }, [viewMode]);

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setSection('hosts');
        setActiveTabId('main');
        window.setTimeout(() => searchInputRef.current?.focus(), 20);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);

  const fetchHosts = useCallback(async () => {
    setLoading(true);
    try {
      const list = await api.listHosts();
      setHosts(list);
      setError('');
    } catch (err: any) {
      setError(err?.message || '加载主机列表失败');
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchKeys = useCallback(async () => {
    try {
      const list = await api.listKeys();
      setSavedKeys(list);
    } catch {
      // non-fatal
    }
  }, []);

  const fetchWorkspaceTabs = useCallback(async () => {
    try {
      const res = await api.getWorkspaceTabs();
      if (Array.isArray(res.tabs)) {
        setTabs(res.tabs);
      }
      if (res.activeTabId) {
        setActiveTabId(res.activeTabId);
      }
    } catch {
      // non-fatal
    } finally {
      tabsLoadedRef.current = true;
    }
  }, []);

  useEffect(() => {
    if (!tabsLoadedRef.current) return;
    const timer = window.setTimeout(() => {
      api.saveWorkspaceTabs(tabs, activeTabId).catch(() => {});
    }, 200);
    return () => window.clearTimeout(timer);
  }, [tabs, activeTabId]);

  useEffect(() => {
    api
      .authStatus()
      .then((res) => {
        setAuthState(res);
        if (res.initialized && res.unlocked && res.authenticated) {
          fetchHosts();
          fetchKeys();
          fetchWorkspaceTabs();
        }
      })
      .catch(() => {
        setAuthState({ initialized: true, unlocked: false, authenticated: false });
      });
  }, [fetchHosts, fetchKeys, fetchWorkspaceTabs]);

  const groups = useMemo(() => {
    const set = new Set<string>();
    hosts.forEach((h) => {
      if (h.groupName) set.add(h.groupName);
    });
    return ['ALL', ...Array.from(set).sort()];
  }, [hosts]);

  const expiringCount = useMemo(() => {
    return hosts.filter((h) => {
      const info = getExpiryInfo(h.expiresAt);
      return info.status === 'expired' || info.status === 'urgent' || info.status === 'soon';
    }).length;
  }, [hosts]);

  const monthlySpendSummary = useMemo(() => {
    const sums: Record<string, number> = {};
    hosts.forEach((h) => {
      if (!h.cost) return;
      const m = toMonthlyEquivalent(h.cost, h.billingCycle);
      sums[h.currency] = (sums[h.currency] || 0) + m;
    });
    const entries = Object.entries(sums);
    if (entries.length === 0) return '—';
    return entries
      .map(([cur, val]) => `${CURRENCY_SYMBOLS[cur as Currency] || ''}${val.toFixed(1)}`)
      .join(' + ');
  }, [hosts]);

  const filteredHosts = useMemo(() => {
    const q = search.trim().toLowerCase();
    return hosts.filter((h) => {
      if (selectedGroup !== 'ALL' && h.groupName !== selectedGroup) return false;
      if (onlyExpiring) {
        const info = getExpiryInfo(h.expiresAt);
        if (info.status !== 'expired' && info.status !== 'urgent' && info.status !== 'soon') {
          return false;
        }
      }
      if (!q) return true;
      return (
        h.name.toLowerCase().includes(q) ||
        h.host.toLowerCase().includes(q) ||
        h.provider.toLowerCase().includes(q) ||
        h.region.toLowerCase().includes(q) ||
        h.groupName.toLowerCase().includes(q) ||
        h.tags.some((t) => t.toLowerCase().includes(q))
      );
    });
  }, [hosts, search, selectedGroup, onlyExpiring]);

  const openSshTab = (host: Host, endpoint?: HostEndpoint) => {
    const id = `ssh-${host.id}-${endpoint?.id || 'primary'}-${Date.now()}`;
    const epLabel = endpoint ? endpoint.label || endpoint.host : undefined;
    setTabs((prev) => [
      ...prev,
      {
        id,
        kind: 'ssh',
        hostId: host.id,
        endpointId: endpoint?.id,
        endpointLabel: epLabel,
        title: epLabel ? `${host.name} (${epLabel})` : host.name,
        subtitle: `${host.username}@${endpoint ? endpoint.host : host.host}`,
      },
    ]);
    setActiveTabId(id);
  };

  const openScpTab = (host: Host, endpoint?: HostEndpoint) => {
    const existing = tabs.find(
      (t) => t.kind === 'scp' && t.hostId === host.id && (t.endpointId || '') === (endpoint?.id || '')
    );
    if (existing) {
      setActiveTabId(existing.id);
      return;
    }
    const id = `scp-${host.id}-${endpoint?.id || 'primary'}-${Date.now()}`;
    const epLabel = endpoint ? endpoint.label || endpoint.host : undefined;
    setTabs((prev) => [
      ...prev,
      {
        id,
        kind: 'scp',
        hostId: host.id,
        endpointId: endpoint?.id,
        endpointLabel: epLabel,
        title: epLabel ? `${host.name} (${epLabel}) · SCP` : `${host.name} · SCP`,
        subtitle: endpoint ? endpoint.host : host.host,
      },
    ]);
    setActiveTabId(id);
  };

  const closeTab = (tabId: string, e: React.MouseEvent) => {
    e.stopPropagation();
    setTabs((prev) => {
      const next = prev.filter((t) => t.id !== tabId);
      if (activeTabId === tabId) {
        setActiveTabId(next.length > 0 ? next[next.length - 1].id : 'main');
      }
      return next;
    });
  };

  const handleSaveHost = async (input: HostInput, existingId?: string) => {
    if (existingId) {
      await api.updateHost(existingId, input);
    } else {
      await api.createHost(input);
    }
    await fetchHosts();
  };

  const handleDeleteHost = async (host: Host) => {
    if (!window.confirm(`确定删除主机 "${host.name}" (${host.host}) 吗？`)) return;
    try {
      await api.deleteHost(host.id);
      setTabs((prev) => prev.filter((t) => t.hostId !== host.id));
      await fetchHosts();
    } catch (err: any) {
      setError(err?.message || '删除失败');
    }
  };

  const handleTestHost = async (host: Host) => {
    setTestingId(host.id);
    try {
      const res = await api.testHost(host.id);
      setTestResult((prev) => ({
        ...prev,
        [host.id]: { ok: true, msg: `${res.latencyMs} ms` },
      }));
      if (res.specs && !host.specs) {
        setHosts((prev) =>
          prev.map((item) => (item.id === host.id ? { ...item, specs: res.specs! } : item))
        );
      }
    } catch (err: any) {
      setTestResult((prev) => ({
        ...prev,
        [host.id]: { ok: false, msg: err?.message || '连接失败' },
      }));
    } finally {
      setTestingId(null);
    }
  };

  const handleCopyIp = (host: Host) => {
    navigator.clipboard.writeText(host.host);
    setCopiedId(host.id);
    window.setTimeout(() => setCopiedId((cur) => (cur === host.id ? null : cur)), 1500);
  };

  const hostMap = useMemo(() => {
    const map = new Map<string, Host>();
    hosts.forEach((h) => map.set(h.id, h));
    return map;
  }, [hosts]);

  if (authState === null) {
    return (
      <div className="flex h-screen w-screen items-center justify-center bg-[#09090b] text-xs text-zinc-500">
        正在检查系统状态...
      </div>
    );
  }

  if (!authState.initialized || !authState.unlocked || !authState.authenticated) {
    return (
      <SetupScreen
        mode={!authState.initialized ? 'setup' : 'unlock'}
        version={authState.version}
        onComplete={() => {
          setAuthState({
            initialized: true,
            unlocked: true,
            authenticated: true,
            version: authState.version,
          });
          fetchHosts();
          fetchKeys();
          fetchWorkspaceTabs();
        }}
      />
    );
  }

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-[#09090b] text-zinc-100">
      {/* Left Sidebar (Desktop + Mobile Drawer) */}
      {mobileMenuOpen && (
        <div
          onClick={() => setMobileMenuOpen(false)}
          className="fixed inset-0 z-40 bg-black/60 backdrop-blur-xs md:hidden"
        />
      )}
      <aside
        className={`${
          mobileMenuOpen ? 'fixed inset-y-0 left-0 z-50 flex' : 'hidden md:flex'
        } w-64 shrink-0 flex-col border-r border-white/[0.08] bg-[#0c0c0f]`}
      >
        {/* Brand */}
        <div className="flex items-center justify-between border-b border-white/[0.08] px-4 py-3.5">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-gradient-to-br from-emerald-400/20 to-sky-500/20 ring-1 ring-emerald-500/30">
              <Sparkles className="h-4 w-4 text-emerald-400" />
            </div>
            <div>
              <div className="flex items-center gap-1.5">
                <span className="text-sm font-semibold tracking-tight text-zinc-100">VPSCoop</span>
                {authState.version && (
                  <span className="rounded-md border border-white/[0.08] bg-white/[0.04] px-1.5 py-0.5 font-mono text-[10px] text-zinc-400">
                    {authState.version}
                  </span>
                )}
              </div>
              <div className="text-[10px] text-zinc-500">SSH · SCP · Vault</div>
            </div>
          </div>
          <div className="flex items-center gap-1.5">
            <button
              type="button"
              onClick={toggleTheme}
              className="flex h-7 w-7 items-center justify-center rounded-lg border border-white/[0.08] bg-white/[0.04] text-zinc-300 transition hover:bg-white/[0.08]"
              title={theme === 'dark' ? '切换到浅色模式' : '切换到深色模式'}
            >
              {theme === 'dark' ? (
                <Sun className="h-3.5 w-3.5 text-amber-400" />
              ) : (
                <Moon className="h-3.5 w-3.5 text-indigo-500" />
              )}
            </button>
            <button
              type="button"
              onClick={() => {
                setEditingHost(null);
                setModalOpen(true);
              }}
              className="flex h-7 w-7 items-center justify-center rounded-lg border border-white/[0.08] bg-white/[0.04] text-zinc-200 transition hover:border-emerald-500/40 hover:bg-emerald-500/15 hover:text-emerald-300"
              title="添加 VPS 主机"
            >
              <Plus className="h-4 w-4" />
            </button>
          </div>
        </div>

        {/* Nav */}
        <div className="space-y-1 p-3">
          <button
            type="button"
            onClick={() => {
              setSection('hosts');
              setActiveTabId('main');
            }}
            className={`flex w-full items-center justify-between rounded-lg px-3 py-2 text-xs font-medium transition ${
              activeTabId === 'main' && section === 'hosts'
                ? 'bg-white/[0.08] text-zinc-100'
                : 'text-zinc-400 hover:bg-white/[0.04] hover:text-zinc-200'
            }`}
          >
            <span className="flex items-center gap-2.5">
              <Server className="h-4 w-4 text-emerald-400" />
              主机库 (Vault)
            </span>
            <span className="font-mono text-[11px] text-zinc-500">{hosts.length}</span>
          </button>

          <button
            type="button"
            onClick={() => {
              setSection('keys');
              setActiveTabId('main');
            }}
            className={`flex w-full items-center justify-between rounded-lg px-3 py-2 text-xs font-medium transition ${
              activeTabId === 'main' && section === 'keys'
                ? 'bg-white/[0.08] text-zinc-100'
                : 'text-zinc-400 hover:bg-white/[0.04] hover:text-zinc-200'
            }`}
          >
            <span className="flex items-center gap-2.5">
              <KeyRound className="h-4 w-4 text-violet-400" />
              密钥管理 (Keys)
            </span>
            <span className="font-mono text-[11px] text-zinc-500">{savedKeys.length}</span>
          </button>

          <button
            type="button"
            onClick={() => {
              setSection('billing');
              setActiveTabId('main');
            }}
            className={`flex w-full items-center justify-between rounded-lg px-3 py-2 text-xs font-medium transition ${
              activeTabId === 'main' && section === 'billing'
                ? 'bg-white/[0.08] text-zinc-100'
                : 'text-zinc-400 hover:bg-white/[0.04] hover:text-zinc-200'
            }`}
          >
            <span className="flex items-center gap-2.5">
              <CreditCard className="h-4 w-4 text-sky-400" />
              账单与到期
            </span>
            {expiringCount > 0 && (
              <span className="rounded-full bg-amber-500/20 px-1.5 py-0.5 font-mono text-[10px] text-amber-300">
                {expiringCount}
              </span>
            )}
          </button>
          <button
            type="button"
            onClick={() => setBackupOpen(true)}
            className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-medium text-zinc-400 transition hover:bg-white/[0.04] hover:text-zinc-200"
          >
            <ShieldCheck className="h-4 w-4 text-emerald-400" />
            加密备份 / 恢复
          </button>

          <button
            type="button"
            onClick={() => setMcpOpen(true)}
            className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-medium text-zinc-400 transition hover:bg-white/[0.04] hover:text-zinc-200"
          >
            <Bot className="h-4 w-4 text-sky-400" />
            HTTP MCP 接口
          </button>

          <button
            type="button"
            onClick={async () => {
              await api.authLock();
              setTabs([]);
              setActiveTabId('main');
              setAuthState({ initialized: true, unlocked: false, authenticated: false });
            }}
            className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-xs font-medium text-zinc-400 transition hover:bg-white/[0.04] hover:text-amber-300"
          >
            <Lock className="h-4 w-4 text-amber-400" />
            锁定面板
          </button>
        </div>

        {/* Quick Connect List */}
        <div className="flex min-h-0 flex-1 flex-col border-t border-white/[0.06] px-3 pt-3">
          <div className="mb-2 px-2 text-[10px] font-semibold uppercase tracking-wider text-zinc-500">
            快速连接 SSH
          </div>
          <div className="flex-1 space-y-0.5 overflow-y-auto pb-3">
            {hosts.map((h) => (
              <button
                key={h.id}
                type="button"
                onClick={() => openSshTab(h)}
                className="group flex w-full items-center justify-between rounded-lg px-2.5 py-1.5 text-left text-xs text-zinc-400 transition hover:bg-white/[0.05] hover:text-zinc-100"
              >
                <div className="min-w-0 flex-1">
                  <div className="truncate font-medium text-zinc-200">{h.name}</div>
                  <div className="truncate font-mono text-[10px] text-zinc-500">{h.host}</div>
                </div>
                <TerminalIcon className="h-3.5 w-3.5 shrink-0 text-emerald-400 opacity-0 transition group-hover:opacity-100" />
              </button>
            ))}
          </div>
        </div>

        {/* Expiry Alert Footer */}
        {expiringCount > 0 && (
          <div className="border-t border-white/[0.08] p-3">
            <button
              type="button"
              onClick={() => {
                setSection('hosts');
                setOnlyExpiring(true);
                setActiveTabId('main');
              }}
              className="flex w-full items-center gap-2.5 rounded-xl border border-amber-500/25 bg-amber-500/10 px-3 py-2 text-left text-xs text-amber-200 transition hover:bg-amber-500/15"
            >
              <AlertTriangle className="h-4 w-4 shrink-0 text-amber-400" />
              <div>
                <div className="font-medium">{expiringCount} 台主机即将到期</div>
                <div className="text-[10px] text-amber-300/70">点击筛选查看</div>
              </div>
            </button>
          </div>
        )}
      </aside>

      {/* Main Content Area */}
      <div className="flex min-w-0 flex-1 flex-col">
        {/* Top Tab Bar (Termius-style) */}
        <header className="flex h-11 shrink-0 items-center justify-between gap-2 border-b border-white/[0.08] bg-[#0c0c0f] px-3">
          <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
            <button
              type="button"
              onClick={() => setMobileMenuOpen(true)}
              className="mr-1 rounded-lg p-1.5 text-zinc-400 hover:bg-white/[0.06] hover:text-zinc-200 md:hidden"
              title="打开菜单"
            >
              <Menu className="h-4 w-4" />
            </button>
            <button
              type="button"
              onClick={() => setActiveTabId('main')}
              className={`inline-flex shrink-0 items-center gap-2 rounded-lg px-3 py-1.5 text-xs font-medium transition ${
                activeTabId === 'main'
                  ? 'bg-white/[0.08] text-zinc-100'
                  : 'text-zinc-400 hover:bg-white/[0.04] hover:text-zinc-200'
              }`}
            >
              {section === 'hosts' ? (
                <>
                  <Server className="h-3.5 w-3.5 text-emerald-400" />
                  <span>主机库</span>
                </>
              ) : section === 'keys' ? (
                <>
                  <KeyRound className="h-3.5 w-3.5 text-violet-400" />
                  <span>密钥管理</span>
                </>
              ) : (
                <>
                  <CreditCard className="h-3.5 w-3.5 text-sky-400" />
                  <span>账单与到期</span>
                </>
              )}
            </button>

            {tabs.map((tab) => {
              const isActive = activeTabId === tab.id;
              return (
                <div
                  key={tab.id}
                  onClick={() => setActiveTabId(tab.id)}
                  className={`group inline-flex shrink-0 cursor-pointer items-center gap-2 rounded-lg border px-3 py-1.5 text-xs transition ${
                    isActive
                      ? 'border-white/[0.1] bg-[#16161b] text-zinc-100'
                      : 'border-transparent text-zinc-400 hover:bg-white/[0.04] hover:text-zinc-200'
                  }`}
                >
                  {tab.kind === 'ssh' ? (
                    <TerminalIcon className="h-3.5 w-3.5 text-emerald-400" />
                  ) : (
                    <FolderOpen className="h-3.5 w-3.5 text-sky-400" />
                  )}
                  <span className="max-w-[140px] truncate font-medium">{tab.title}</span>
                  <button
                    type="button"
                    onClick={(e) => closeTab(tab.id, e)}
                    className="rounded p-0.5 text-zinc-500 hover:bg-white/[0.1] hover:text-zinc-200"
                  >
                    <X className="h-3 w-3" />
                  </button>
                </div>
              );
            })}
          </div>

          <div className="flex shrink-0 items-center gap-1">
            <button
              type="button"
              onClick={handleQuickCopyMcp}
              className="inline-flex items-center gap-1.5 rounded-lg border border-sky-500/25 bg-sky-500/10 px-2.5 py-1 text-[11px] font-medium text-sky-300 transition hover:bg-sky-500/20"
              title="一键复制随机派生的 HTTP MCP 接入配置"
            >
              {mcpCopied ? (
                <>
                  <Check className="h-3.5 w-3.5 text-emerald-400" />
                  <span className="text-emerald-300">已复制 MCP 配置</span>
                </>
              ) : (
                <>
                  <Bot className="h-3.5 w-3.5" />
                  <span>接入 MCP</span>
                </>
              )}
            </button>
            <button
              type="button"
              onClick={() => setMcpOpen(true)}
              className="rounded-lg border border-white/[0.08] bg-white/[0.03] px-2 py-1 text-[11px] text-zinc-400 transition hover:bg-white/[0.08] hover:text-zinc-200"
              title="查看或重置 MCP Token"
            >
              详情
            </button>
          </div>
        </header>

        {/* Workspace Viewport */}
        <main className="relative min-h-0 flex-1">
          {/* Main Vault / Billing View */}
          <div className={activeTabId === 'main' ? 'h-full' : 'hidden'}>
            <Suspense fallback={<div className="flex h-full items-center justify-center text-xs text-zinc-500">加载中...</div>}>
            {section === 'billing' ? (
              <BillingView
                hosts={hosts}
                onEditHost={(h) => {
                  setEditingHost(h);
                  setModalOpen(true);
                }}
                onOpenSsh={openSshTab}
                onOpenInvoices={(h) => setInvoiceHost(h)}
              />
            ) : section === 'keys' ? (
              <KeyVaultView keys={savedKeys} onRefresh={fetchKeys} />
            ) : (
              <div className="h-full overflow-y-auto px-6 py-6">
                <div className="mx-auto max-w-6xl space-y-6">
                  {/* Top Header & Actions */}
                  <div className="flex flex-wrap items-center justify-between gap-4">
                    <div>
                      <h1 className="text-xl font-semibold tracking-tight text-zinc-100">
                        我的 VPS 主机库
                      </h1>
                    </div>
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        onClick={() => setImportOpen(true)}
                        className="inline-flex items-center gap-1.5 rounded-xl border border-white/[0.08] bg-white/[0.03] px-3.5 py-2 text-xs font-medium text-zinc-200 transition hover:bg-white/[0.07]"
                      >
                        <Upload className="h-3.5 w-3.5 text-sky-400" />
                        导入 (Termius)
                      </button>
                      <button
                        type="button"
                        onClick={() => setSection('billing')}
                        className="inline-flex items-center gap-1.5 rounded-xl border border-white/[0.08] bg-white/[0.03] px-3.5 py-2 text-xs font-medium text-zinc-200 transition hover:bg-white/[0.07] md:hidden"
                      >
                        <CreditCard className="h-3.5 w-3.5 text-sky-400" />
                        账单
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          setEditingHost(null);
                          setModalOpen(true);
                        }}
                        className="inline-flex items-center gap-1.5 rounded-xl bg-emerald-500 px-4 py-2 text-xs font-semibold text-zinc-950 shadow-lg shadow-emerald-500/15 transition hover:bg-emerald-400"
                      >
                        <Plus className="h-4 w-4" />
                        添加 VPS
                      </button>
                    </div>
                  </div>

                  {/* KPI Summary Strip */}
                  <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                    <div className="rounded-xl border border-white/[0.08] bg-[#111114] px-4 py-3">
                      <div className="text-[11px] text-zinc-500">纳管主机</div>
                      <div className="mt-1 font-mono text-lg font-semibold text-zinc-100">{hosts.length}</div>
                    </div>
                    <div className="rounded-xl border border-white/[0.08] bg-[#111114] px-4 py-3">
                      <div className="text-[11px] text-zinc-500">活跃标签会话</div>
                      <div className="mt-1 font-mono text-lg font-semibold text-emerald-400">{tabs.length}</div>
                    </div>
                    <button
                      type="button"
                      onClick={() => setOnlyExpiring((v) => !v)}
                      className="rounded-xl border border-white/[0.08] bg-[#111114] px-4 py-3 text-left transition hover:border-amber-500/30"
                    >
                      <div className="text-[11px] text-zinc-500">30天内到期</div>
                      <div className={`mt-1 font-mono text-lg font-semibold ${expiringCount > 0 ? 'text-amber-400' : 'text-zinc-300'}`}>
                        {expiringCount}
                      </div>
                    </button>
                    <button
                      type="button"
                      onClick={() => setSection('billing')}
                      className="rounded-xl border border-white/[0.08] bg-[#111114] px-4 py-3 text-left transition hover:border-sky-500/30"
                    >
                      <div className="text-[11px] text-zinc-500">折算月费</div>
                      <div className="mt-1 truncate font-mono text-lg font-semibold text-sky-400">
                        {monthlySpendSummary}
                      </div>
                    </button>
                  </div>

                  {/* Search & Filters */}
                  <div className="flex flex-wrap items-center gap-3">
                    <div className="relative min-w-[240px] flex-1">
                      <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-zinc-500" />
                      <input
                        ref={searchInputRef}
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                        placeholder="搜索主机、IP、购买商、标签... (Ctrl+K)"
                        className="w-full rounded-xl border border-white/[0.08] bg-[#111114] py-2 pl-9 pr-3 text-xs text-zinc-100 placeholder-zinc-500 outline-none transition focus:border-emerald-500/50"
                      />
                    </div>

                    <div className="flex flex-wrap items-center gap-1.5">
                      {groups.map((g) => (
                        <button
                          key={g}
                          type="button"
                          onClick={() => setSelectedGroup(g)}
                          className={`rounded-lg px-3 py-1.5 text-xs font-medium transition ${
                            selectedGroup === g
                              ? 'bg-white/[0.1] text-zinc-100'
                              : 'border border-white/[0.06] bg-white/[0.02] text-zinc-400 hover:text-zinc-200'
                          }`}
                        >
                          {g === 'ALL' ? '全部' : g}
                        </button>
                      ))}
                      <button
                        type="button"
                        onClick={() => setOnlyExpiring((v) => !v)}
                        className={`inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-medium transition ${
                          onlyExpiring
                            ? 'border border-amber-500/40 bg-amber-500/15 text-amber-300'
                            : 'border border-white/[0.06] bg-white/[0.02] text-zinc-400 hover:text-zinc-200'
                        }`}
                      >
                        <AlertTriangle className="h-3.5 w-3.5" />
                        即将到期
                      </button>

                      <div className="ml-1 inline-flex rounded-lg border border-white/[0.08] bg-white/[0.02] p-0.5">
                        <button
                          type="button"
                          onClick={() => setViewMode('grid')}
                          className={`rounded-md p-1.5 transition ${
                            viewMode === 'grid' ? 'bg-white/[0.1] text-zinc-100' : 'text-zinc-500 hover:text-zinc-300'
                          }`}
                          title="卡片视图"
                        >
                          <LayoutGrid className="h-3.5 w-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => setViewMode('list')}
                          className={`rounded-md p-1.5 transition ${
                            viewMode === 'list' ? 'bg-white/[0.1] text-zinc-100' : 'text-zinc-500 hover:text-zinc-300'
                          }`}
                          title="紧凑列表视图"
                        >
                          <List className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                  </div>

                  {error && (
                    <div className="rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-xs text-red-300">
                      {error}
                    </div>
                  )}

                  {/* Host Grid */}
                  {loading ? (
                    <div className="py-16 text-center text-xs text-zinc-500">正在加载主机...</div>
                  ) : filteredHosts.length === 0 ? (
                    <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-white/[0.1] bg-[#101013] py-16 text-center">
                      <Server className="mb-3 h-8 w-8 text-zinc-600" />
                      <div className="text-sm font-medium text-zinc-300">
                        {hosts.length === 0 ? '还没有添加任何 VPS' : '没有匹配的主机'}
                      </div>
                      <p className="mt-1 text-xs text-zinc-500">
                        {hosts.length === 0
                          ? '点击右上角「添加 VPS」录入你的第一台服务器'
                          : '尝试调整搜索关键词或筛选条件'}
                      </p>
                    </div>
                  ) : viewMode === 'list' ? (
                    <div className="overflow-hidden rounded-2xl border border-white/[0.08] bg-[#111114]">
                      <table className="w-full text-left text-xs">
                        <thead className="border-b border-white/[0.08] bg-white/[0.02] text-[11px] uppercase tracking-wider text-zinc-500">
                          <tr>
                            <th className="px-4 py-3 font-medium">主机 / IP</th>
                            <th className="hidden px-3 py-3 font-medium sm:table-cell">购买商 / 地区</th>
                            <th className="hidden px-3 py-3 font-medium lg:table-cell">配置 / 标签</th>
                            <th className="hidden px-3 py-3 font-medium md:table-cell">费用 / 到期</th>
                            <th className="px-4 py-3 text-right font-medium">操作</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-white/[0.05]">
                          {filteredHosts.map((h) => {
                            const exp = getExpiryInfo(h.expiresAt);
                            return (
                              <tr key={h.id} className="transition hover:bg-white/[0.02]">
                                <td className="px-4 py-3">
                                  <div className="flex items-center gap-2 font-semibold text-zinc-100">
                                    <span>{h.name}</span>
                                    {h.groupName && (
                                      <span className="rounded border border-white/[0.08] bg-white/[0.04] px-1.5 py-0.5 text-[10px] font-normal text-zinc-400">
                                        {h.groupName}
                                      </span>
                                    )}
                                  </div>
                                  <div className="mt-0.5 font-mono text-[11px] text-zinc-500">
                                    {h.username}@{h.host}:{h.port}
                                  </div>
                                </td>
                                <td className="hidden px-3 py-3 text-zinc-300 sm:table-cell">
                                  <div>{h.provider || '—'}</div>
                                  {h.region && <div className="text-[11px] text-zinc-500">{h.region}</div>}
                                </td>
                                <td className="hidden px-3 py-3 lg:table-cell">
                                  {h.specs && <div className="max-w-[220px] truncate text-[11px] text-zinc-400">{h.specs}</div>}
                                  {h.tags.length > 0 && (
                                    <div className="mt-1 flex flex-wrap gap-1">
                                      {h.tags.slice(0, 4).map((t) => (
                                        <span key={t} className="rounded bg-zinc-800/80 px-1.5 py-0.5 text-[10px] text-zinc-400">
                                          #{t}
                                        </span>
                                      ))}
                                    </div>
                                  )}
                                </td>
                                <td className="hidden px-3 py-3 md:table-cell">
                                  {h.cost > 0 && (
                                    <div className="font-mono text-zinc-200">{formatCost(h.cost, h.currency, h.billingCycle)}</div>
                                  )}
                                  {h.expiresAt && (
                                    <div className="text-[11px] text-zinc-500">{exp.label}</div>
                                  )}
                                </td>
                                <td className="px-4 py-3 text-right">
                                  <div className="inline-flex items-center gap-1">
                                    <button
                                      type="button"
                                      onClick={() => openSshTab(h)}
                                      className="inline-flex items-center gap-1 rounded-md bg-emerald-500/15 px-2.5 py-1 text-[11px] font-medium text-emerald-300 hover:bg-emerald-500/25"
                                    >
                                      <TerminalIcon className="h-3 w-3" />
                                      SSH
                                    </button>
                                    <button
                                      type="button"
                                      onClick={() => openScpTab(h)}
                                      className="inline-flex items-center gap-1 rounded-md bg-sky-500/10 px-2.5 py-1 text-[11px] font-medium text-sky-300 hover:bg-sky-500/20"
                                    >
                                      <FolderOpen className="h-3 w-3" />
                                      SCP
                                    </button>
                                    <button
                                      type="button"
                                      onClick={() => setNetworkHost(h)}
                                      className="rounded p-1.5 text-zinc-400 hover:bg-white/[0.06] hover:text-emerald-400"
                                      title="IP 与链路探测"
                                    >
                                      <Route className="h-3.5 w-3.5" />
                                    </button>
                                    <button
                                      type="button"
                                      onClick={() => {
                                        setEditingHost(h);
                                        setModalOpen(true);
                                      }}
                                      className="rounded p-1.5 text-zinc-400 hover:bg-white/[0.06] hover:text-zinc-200"
                                      title="编辑"
                                    >
                                      <Pencil className="h-3.5 w-3.5" />
                                    </button>
                                  </div>
                                </td>
                              </tr>
                            );
                          })}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-3">
                      {filteredHosts.map((h) => {
                        const exp = getExpiryInfo(h.expiresAt);
                        const expCls = {
                          none: 'border-white/[0.06] bg-white/[0.03] text-zinc-500',
                          healthy: 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300',
                          soon: 'border-amber-500/25 bg-amber-500/10 text-amber-300',
                          urgent: 'border-red-500/30 bg-red-500/15 text-red-300',
                          expired: 'border-red-500/40 bg-red-500/20 text-red-200',
                        }[exp.status];

                        const tRes = testResult[h.id];

                        return (
                          <div
                            key={h.id}
                            className="group flex flex-col justify-between rounded-2xl border border-white/[0.08] bg-[#111114] p-4 transition hover:border-white/[0.16] hover:bg-[#141418]"
                          >
                            <div className="space-y-3">
                              {/* Card Top */}
                              <div className="flex items-start justify-between gap-2">
                                <div className="min-w-0">
                                  <div className="flex items-center gap-2">
                                    <h3 className="truncate text-sm font-semibold text-zinc-100">
                                      {h.name}
                                    </h3>
                                    {h.groupName && (
                                      <span className="shrink-0 rounded-md border border-white/[0.08] bg-white/[0.04] px-1.5 py-0.5 text-[10px] text-zinc-400">
                                        {h.groupName}
                                      </span>
                                    )}
                                  </div>

                                  <button
                                    type="button"
                                    onClick={() => handleCopyIp(h)}
                                    className="mt-1 inline-flex items-center gap-1.5 font-mono text-xs text-zinc-400 transition hover:text-zinc-200"
                                    title="点击复制 IP"
                                  >
                                    <span>
                                      {h.username}@{h.host}:{h.port}
                                    </span>
                                    {copiedId === h.id ? (
                                      <Check className="h-3 w-3 text-emerald-400" />
                                    ) : (
                                      <Copy className="h-3 w-3 opacity-50" />
                                    )}
                                  </button>
                                </div>

                                <div className="flex shrink-0 items-center gap-1">
                                  <button
                                    type="button"
                                    onClick={() => setNetworkHost(h)}
                                    className="rounded-lg p-1.5 text-zinc-500 transition hover:bg-white/[0.06] hover:text-emerald-400"
                                    title="IP 与三网链路探测 (CN2/9929/CMIN2)"
                                  >
                                    <Route className="h-3.5 w-3.5" />
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => setTailscaleHost(h)}
                                    className="rounded-lg p-1.5 text-zinc-500 transition hover:bg-white/[0.06] hover:text-sky-400"
                                    title="Tailscale 自动组网与 IP 同步"
                                  >
                                    <Network className="h-3.5 w-3.5" />
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => setInvoiceHost(h)}
                                    className="rounded-lg p-1.5 text-zinc-500 transition hover:bg-white/[0.06] hover:text-sky-400"
                                    title="Invoice 账单凭证"
                                  >
                                    <Receipt className="h-3.5 w-3.5" />
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => setInjectHost(h)}
                                    className="rounded-lg p-1.5 text-zinc-500 transition hover:bg-white/[0.06] hover:text-violet-400"
                                    title="密钥自动注入与检测"
                                  >
                                    <KeyRound className="h-3.5 w-3.5" />
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => handleTestHost(h)}
                                    disabled={testingId === h.id}
                                    className="rounded-lg p-1.5 text-zinc-500 transition hover:bg-white/[0.06] hover:text-zinc-200"
                                    title="测试 SSH 连通性"
                                  >
                                    <Activity
                                      className={`h-3.5 w-3.5 ${
                                        testingId === h.id ? 'animate-pulse text-emerald-400' : ''
                                      }`}
                                    />
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => {
                                      setEditingHost(h);
                                      setModalOpen(true);
                                    }}
                                    className="rounded-lg p-1.5 text-zinc-500 transition hover:bg-white/[0.06] hover:text-zinc-200"
                                    title="编辑"
                                  >
                                    <Pencil className="h-3.5 w-3.5" />
                                  </button>
                                  <button
                                    type="button"
                                    onClick={() => handleDeleteHost(h)}
                                    className="rounded-lg p-1.5 text-zinc-500 transition hover:bg-red-500/15 hover:text-red-400"
                                    title="删除"
                                  >
                                    <Trash2 className="h-3.5 w-3.5" />
                                  </button>
                                </div>
                              </div>

                              {/* Meta Badges */}
                              <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
                                {h.provider && (
                                  <span className="inline-flex items-center gap-1 rounded-md border border-white/[0.08] bg-white/[0.03] px-2 py-0.5 text-zinc-300">
                                    <Globe className="h-3 w-3 text-sky-400" />
                                    {h.provider}
                                    {h.region ? ` · ${h.region}` : ''}
                                  </span>
                                )}
                                {h.cost > 0 && (
                                  <span className="rounded-md border border-white/[0.08] bg-white/[0.03] px-2 py-0.5 font-mono text-zinc-300">
                                    {formatCost(h.cost, h.currency, h.billingCycle)}
                                  </span>
                                )}
                                {h.expiresAt && (
                                  <span className={`rounded-md border px-2 py-0.5 ${expCls}`}>
                                    {exp.label} ({h.expiresAt})
                                  </span>
                                )}
                                {tRes && (
                                  <span
                                    className={`rounded-md border px-2 py-0.5 font-mono ${
                                      tRes.ok
                                        ? 'border-emerald-500/25 bg-emerald-500/10 text-emerald-300'
                                        : 'border-red-500/25 bg-red-500/10 text-red-300'
                                    }`}
                                  >
                                    {tRes.msg}
                                  </span>
                                )}
                              </div>

                              {h.specs && (
                                <div className="flex items-center gap-1.5 text-[11px] text-zinc-400">
                                  <Cpu className="h-3 w-3 shrink-0 text-zinc-500" />
                                  <span className="truncate">{h.specs}</span>
                                </div>
                              )}

                              {h.endpoints && h.endpoints.length > 0 && (
                                <div className="flex flex-wrap items-center gap-1.5">
                                  <span className="text-[10px] text-zinc-500">备用入口:</span>
                                  {h.endpoints.map((ep) => (
                                    <button
                                      key={ep.id}
                                      type="button"
                                      onClick={() => openSshTab(h, ep)}
                                      className="inline-flex items-center gap-1 rounded-md border border-emerald-500/20 bg-emerald-500/10 px-2 py-0.5 font-mono text-[10px] text-emerald-300 transition hover:bg-emerald-500/20"
                                      title={`使用 ${ep.host}:${ep.port} 连接 SSH`}
                                    >
                                      <span>{ep.label || ep.host}</span>
                                      <span className="text-emerald-400/70">:{ep.port}</span>
                                    </button>
                                  ))}
                                </div>
                              )}

                              {h.tags.length > 0 && (
                                <div className="flex flex-wrap gap-1">
                                  {h.tags.map((tag) => (
                                    <span
                                      key={tag}
                                      className="rounded bg-zinc-800/80 px-1.5 py-0.5 text-[10px] text-zinc-400"
                                    >
                                      #{tag}
                                    </span>
                                  ))}
                                </div>
                              )}
                            </div>

                            {/* Action Buttons */}
                            <div className="mt-4 grid grid-cols-2 gap-2 border-t border-white/[0.06] pt-3">
                              <button
                                type="button"
                                onClick={() => openSshTab(h)}
                                className="inline-flex items-center justify-center gap-1.5 rounded-xl bg-emerald-500/15 py-2 text-xs font-medium text-emerald-300 ring-1 ring-emerald-500/25 transition hover:bg-emerald-500/25"
                              >
                                <TerminalIcon className="h-3.5 w-3.5" />
                                SSH 终端
                              </button>
                              <button
                                type="button"
                                onClick={() => openScpTab(h)}
                                className="inline-flex items-center justify-center gap-1.5 rounded-xl bg-sky-500/10 py-2 text-xs font-medium text-sky-300 ring-1 ring-sky-500/20 transition hover:bg-sky-500/20"
                              >
                                <FolderOpen className="h-3.5 w-3.5" />
                                SCP 文件
                              </button>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              </div>
            )}
            </Suspense>
          </div>

          {/* Mounted SSH & SCP Tabs (kept alive while switching tabs) */}
          {tabs.map((tab) => {
            const host = hostMap.get(tab.hostId);
            if (!host) return null;
            const isActive = activeTabId === tab.id;
            return (
              <div key={tab.id} className={isActive ? 'h-full' : 'hidden'}>
                <Suspense fallback={<div className="flex h-full items-center justify-center text-xs text-zinc-500">正在加载工作区...</div>}>
                  {tab.kind === 'ssh' ? (
                    <TerminalView
                      host={host}
                      endpointId={tab.endpointId}
                      endpointLabel={tab.endpointLabel}
                      active={isActive}
                      themeMode={theme}
                      onOpenScp={(h) => openScpTab(h)}
                    />
                  ) : (
                    <ScpFileManager
                      host={host}
                      endpointId={tab.endpointId}
                      onOpenSsh={(h) => openSshTab(h)}
                    />
                  )}
                </Suspense>
              </div>
            );
          })}
        </main>
      </div>

      <Suspense fallback={null}>
        <HostModal
          open={modalOpen}
          host={editingHost}
          savedKeys={savedKeys}
          onClose={() => setModalOpen(false)}
          onSave={handleSaveHost}
        />

        <KeyInjectModal
          open={Boolean(injectHost)}
          host={injectHost}
          savedKeys={savedKeys}
          onClose={() => setInjectHost(null)}
          onUpdated={fetchHosts}
        />

        <InvoiceModal
          open={Boolean(invoiceHost)}
          host={invoiceHost}
          onClose={() => setInvoiceHost(null)}
        />

        <ImportModal
          open={importOpen}
          savedKeys={savedKeys}
          onClose={() => setImportOpen(false)}
          onImported={fetchHosts}
        />

        <TailscaleModal
          open={Boolean(tailscaleHost)}
          host={tailscaleHost}
          onClose={() => setTailscaleHost(null)}
          onUpdated={fetchHosts}
        />

        <BackupModal
          open={backupOpen}
          onClose={() => setBackupOpen(false)}
          onRestored={async () => {
            await fetchHosts();
            await fetchKeys();
          }}
        />

        <McpModal open={mcpOpen} onClose={() => setMcpOpen(false)} />

        <NetworkModal
          open={Boolean(networkHost)}
          host={networkHost}
          onClose={() => setNetworkHost(null)}
          onUpdated={fetchHosts}
        />
      </Suspense>
    </div>
  );
}
