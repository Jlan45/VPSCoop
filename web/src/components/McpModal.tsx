import React, { useEffect, useState } from 'react';
import { Bot, X, Copy, Check, RotateCw, Eye, EyeOff } from 'lucide-react';
import { api } from '../api';

interface McpModalProps {
  open: boolean;
  onClose: () => void;
}

export const McpModal: React.FC<McpModalProps> = ({ open, onClose }) => {
  const [token, setToken] = useState('');
  const [fromEnv, setFromEnv] = useState(false);
  const [showToken, setShowToken] = useState(false);
  const [rotating, setRotating] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    setError('');
    api
      .getMCPToken()
      .then((res) => {
        setToken(res.token);
        setFromEnv(res.fromEnv);
      })
      .catch((err: any) => setError(err?.message || '获取 MCP Token 失败'));
  }, [open]);

  if (!open) return null;

  const mcpUrl = `${window.location.origin}/mcp`;
  const configSnippet = JSON.stringify(
    {
      mcpServers: {
        vpscoop: {
          url: mcpUrl,
          headers: {
            Authorization: `Bearer ${token || '<YOUR_TOKEN>'}`,
          },
        },
      },
    },
    null,
    2
  );

  const copy = (text: string, key: string) => {
    navigator.clipboard.writeText(text);
    setCopied(key);
    window.setTimeout(() => setCopied((c) => (c === key ? null : c)), 1500);
  };

  const handleRotate = async () => {
    if (!window.confirm('重置 Token 后，旧的 MCP 客户端配置将失效，确定继续？')) return;
    setRotating(true);
    try {
      const res = await api.rotateMCPToken();
      setToken(res.token);
    } catch (err: any) {
      setError(err?.message || '重置失败');
    } finally {
      setRotating(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm">
      <div className="w-full max-w-lg overflow-hidden rounded-2xl border border-white/[0.1] bg-[#111114] shadow-2xl">
        <div className="flex items-center justify-between border-b border-white/[0.08] px-6 py-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-sky-500/15 text-sky-400">
              <Bot className="h-4 w-4" />
            </div>
            <div>
              <h2 className="text-sm font-semibold text-zinc-100">
                HTTP MCP Server (Streamable HTTP)
              </h2>
              <p className="text-[11px] text-zinc-500">
                基于官方 Go MCP SDK · 需携带 Authorization Bearer 头鉴权
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
          {error && (
            <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-3.5 py-2.5 text-xs text-red-300">
              {error}
            </div>
          )}

          <div>
            <div className="mb-1.5 flex items-center justify-between text-xs">
              <span className="font-medium text-zinc-400">MCP Endpoint URL</span>
              <button
                type="button"
                onClick={() => copy(mcpUrl, 'url')}
                className="inline-flex items-center gap-1 text-[11px] text-zinc-400 hover:text-emerald-400"
              >
                {copied === 'url' ? (
                  <>
                    <Check className="h-3 w-3 text-emerald-400" />
                    <span className="text-emerald-400">已复制</span>
                  </>
                ) : (
                  <>
                    <Copy className="h-3 w-3" />
                    <span>复制地址</span>
                  </>
                )}
              </button>
            </div>
            <div className="rounded-lg border border-white/[0.08] bg-[#09090b] px-3 py-2 font-mono text-xs text-zinc-200">
              {mcpUrl}
            </div>
          </div>

          <div>
            <div className="mb-1.5 flex items-center justify-between text-xs">
              <span className="font-medium text-zinc-400">
                Authorization Bearer Token {fromEnv ? '（来自环境变量）' : ''}
              </span>
              <div className="flex items-center gap-3">
                <button
                  type="button"
                  onClick={() => setShowToken((v) => !v)}
                  className="inline-flex items-center gap-1 text-[11px] text-zinc-400 hover:text-zinc-200"
                >
                  {showToken ? <EyeOff className="h-3 w-3" /> : <Eye className="h-3 w-3" />}
                  <span>{showToken ? '隐藏' : '显示'}</span>
                </button>
                <button
                  type="button"
                  onClick={() => copy(token, 'tok')}
                  className="inline-flex items-center gap-1 text-[11px] text-zinc-400 hover:text-emerald-400"
                >
                  {copied === 'tok' ? (
                    <>
                      <Check className="h-3 w-3 text-emerald-400" />
                      <span className="text-emerald-400">已复制</span>
                    </>
                  ) : (
                    <>
                      <Copy className="h-3 w-3" />
                      <span>复制 Token</span>
                    </>
                  )}
                </button>
                {!fromEnv && (
                  <button
                    type="button"
                    disabled={rotating}
                    onClick={handleRotate}
                    className="inline-flex items-center gap-1 text-[11px] text-amber-400 hover:text-amber-300"
                  >
                    <RotateCw className={`h-3 w-3 ${rotating ? 'animate-spin' : ''}`} />
                    <span>重置</span>
                  </button>
                )}
              </div>
            </div>
            <div className="rounded-lg border border-white/[0.08] bg-[#09090b] px-3 py-2 font-mono text-xs text-zinc-200">
              {showToken ? token : '••••••••••••••••••••••••••••••••'}
            </div>
          </div>

          <div>
            <div className="mb-1.5 flex items-center justify-between text-xs">
              <span className="font-medium text-zinc-400">客户端配置示例</span>
              <button
                type="button"
                onClick={() => copy(configSnippet, 'cfg')}
                className="inline-flex items-center gap-1 text-[11px] text-zinc-400 hover:text-emerald-400"
              >
                {copied === 'cfg' ? (
                  <>
                    <Check className="h-3 w-3 text-emerald-400" />
                    <span className="text-emerald-400">已复制配置</span>
                  </>
                ) : (
                  <>
                    <Copy className="h-3 w-3" />
                    <span>复制 JSON</span>
                  </>
                )}
              </button>
            </div>
            <pre className="overflow-x-auto rounded-xl border border-white/[0.08] bg-[#09090b] p-3 font-mono text-[11px] leading-relaxed text-zinc-300">
              {configSnippet}
            </pre>
          </div>
        </div>
      </div>
    </div>
  );
};
