import React, { useEffect, useRef, useState } from 'react';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { RotateCw, Trash2, FolderOpen } from 'lucide-react';
import type { Host } from '../types';

interface TerminalViewProps {
  host: Host;
  endpointId?: string;
  endpointLabel?: string;
  active: boolean;
  themeMode?: 'dark' | 'light';
  onOpenScp: (host: Host) => void;
}

const DARK_TERM_THEME = {
  background: '#09090b',
  foreground: '#e4e4e7',
  cursor: '#34d399',
  cursorAccent: '#09090b',
  selectionBackground: 'rgba(52, 211, 153, 0.25)',
  black: '#18181b',
  red: '#f87171',
  green: '#34d399',
  yellow: '#fbbf24',
  blue: '#60a5fa',
  magenta: '#c084fc',
  cyan: '#22d3ee',
  white: '#e4e4e7',
  brightBlack: '#52525b',
  brightRed: '#fca5a5',
  brightGreen: '#6ee7b7',
  brightYellow: '#fde68a',
  brightBlue: '#93c5fd',
  brightMagenta: '#d8b4fe',
  brightCyan: '#67e8f9',
  brightWhite: '#ffffff',
};

const LIGHT_TERM_THEME = {
  background: '#fafafa',
  foreground: '#18181b',
  cursor: '#059669',
  cursorAccent: '#fafafa',
  selectionBackground: 'rgba(16, 185, 129, 0.22)',
  black: '#18181b',
  red: '#dc2626',
  green: '#059669',
  yellow: '#d97706',
  blue: '#2563eb',
  magenta: '#9333ea',
  cyan: '#0891b2',
  white: '#e4e4e7',
  brightBlack: '#71717a',
  brightRed: '#ef4444',
  brightGreen: '#10b981',
  brightYellow: '#f59e0b',
  brightBlue: '#3b82f6',
  brightMagenta: '#a855f7',
  brightCyan: '#06b6d4',
  brightWhite: '#ffffff',
};

export const TerminalView: React.FC<TerminalViewProps> = ({
  host,
  endpointId,
  endpointLabel,
  active,
  themeMode = 'dark',
  onOpenScp,
}) => {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const [status, setStatus] = useState<'connecting' | 'connected' | 'disconnected'>('connecting');
  const [connectSeq, setConnectSeq] = useState(0);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13.5,
      lineHeight: 1.3,
      fontFamily: "'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, monospace",
      theme: themeMode === 'light' ? LIGHT_TERM_THEME : DARK_TERM_THEME,
      allowProposedApi: true,
    });

    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(webLinksAddon);
    term.open(el);
    fitAddon.fit();

    termRef.current = term;
    fitRef.current = fitAddon;

    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const epQuery = endpointId ? `&endpointId=${encodeURIComponent(endpointId)}` : '';
    const wsUrl = `${proto}//${window.location.host}/api/ws/ssh/${host.id}?cols=${term.cols}&rows=${term.rows}${epQuery}`;
    const ws = new WebSocket(wsUrl);
    ws.binaryType = 'arraybuffer';
    wsRef.current = ws;
    setStatus('connecting');

    ws.onopen = () => {
      setStatus('connected');
      fitAddon.fit();
      ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
    };

    ws.onmessage = (ev) => {
      if (typeof ev.data === 'string') {
        term.write(ev.data);
      } else if (ev.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(ev.data));
      }
    };

    ws.onclose = () => {
      setStatus('disconnected');
    };

    ws.onerror = () => {
      setStatus('disconnected');
    };

    const dataDisp = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'input', data }));
      }
    });

    const resizeObserver = new ResizeObserver(() => {
      if (!fitRef.current || !termRef.current) return;
      fitRef.current.fit();
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(
          JSON.stringify({
            type: 'resize',
            cols: termRef.current.cols,
            rows: termRef.current.rows,
          })
        );
      }
    });
    resizeObserver.observe(el);

    const pingInterval = window.setInterval(() => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'ping' }));
      }
    }, 25000);

    return () => {
      window.clearInterval(pingInterval);
      resizeObserver.disconnect();
      dataDisp.dispose();
      ws.close();
      term.dispose();
    };
  }, [host.id, endpointId, connectSeq]);

  useEffect(() => {
    if (active && fitRef.current && termRef.current) {
      const timer = window.setTimeout(() => {
        fitRef.current?.fit();
        termRef.current?.focus();
        if (wsRef.current?.readyState === WebSocket.OPEN && termRef.current) {
          wsRef.current.send(
            JSON.stringify({
              type: 'resize',
              cols: termRef.current.cols,
              rows: termRef.current.rows,
            })
          );
        }
      }, 40);
      return () => window.clearTimeout(timer);
    }
  }, [active]);

  useEffect(() => {
    if (termRef.current) {
      termRef.current.options.theme =
        themeMode === 'light' ? LIGHT_TERM_THEME : DARK_TERM_THEME;
    }
  }, [themeMode]);

  const statusBadge = {
    connecting: { dot: 'bg-amber-400 animate-pulse', text: '连接中...' },
    connected: { dot: 'bg-emerald-400', text: '已连接' },
    disconnected: { dot: 'bg-red-400', text: '已断开' },
  }[status];

  return (
    <div className="flex h-full flex-col bg-[#09090b]">
      {/* Terminal Toolbar */}
      <div className="flex items-center justify-between border-b border-white/[0.08] bg-[#0e0e11] px-4 py-2">
        <div className="flex items-center gap-3 text-xs">
          <span className="inline-flex items-center gap-1.5 rounded-full border border-white/[0.08] bg-white/[0.03] px-2.5 py-1 text-zinc-300">
            <span className={`h-2 w-2 rounded-full ${statusBadge.dot}`} />
            {statusBadge.text}
          </span>
          <span className="font-mono text-zinc-400">
            {host.username}@{host.host}:{host.port}
          </span>
          {endpointLabel && (
            <span className="rounded bg-emerald-500/15 px-2 py-0.5 text-[11px] text-emerald-300">
              {endpointLabel}
            </span>
          )}
          {host.provider && (
            <span className="hidden rounded bg-white/[0.05] px-2 py-0.5 text-[11px] text-zinc-400 sm:inline">
              {host.provider}
            </span>
          )}
        </div>

        <div className="flex items-center gap-1.5">
          <button
            type="button"
            onClick={() => onOpenScp(host)}
            className="inline-flex items-center gap-1.5 rounded-md border border-white/[0.08] bg-white/[0.03] px-2.5 py-1 text-xs text-zinc-300 transition hover:bg-white/[0.08]"
          >
            <FolderOpen className="h-3.5 w-3.5 text-sky-400" />
            SCP 文件
          </button>
          <button
            type="button"
            onClick={() => termRef.current?.clear()}
            className="inline-flex items-center gap-1 rounded-md border border-white/[0.08] bg-white/[0.03] px-2.5 py-1 text-xs text-zinc-300 transition hover:bg-white/[0.08]"
            title="清屏"
          >
            <Trash2 className="h-3.5 w-3.5" />
            清屏
          </button>
          <button
            type="button"
            onClick={() => setConnectSeq((s) => s + 1)}
            className="inline-flex items-center gap-1 rounded-md border border-white/[0.08] bg-white/[0.03] px-2.5 py-1 text-xs text-zinc-300 transition hover:bg-white/[0.08]"
            title="重新连接"
          >
            <RotateCw className="h-3.5 w-3.5 text-emerald-400" />
            重连
          </button>
        </div>
      </div>

      {/* xterm Mount */}
      <div ref={containerRef} className="min-h-0 flex-1 overflow-hidden" />
    </div>
  );
};
