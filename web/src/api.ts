import type { FileEntry, Host, HostInput, Invoice, KeyCheckResult, NetworkReport, SSHKey, TailscaleStatus } from './types';

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  const text = await res.text();
  let data: any = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    // non-json response
  }
  if (!res.ok) {
    throw new Error(data?.error || text || `Request failed (${res.status})`);
  }
  return data as T;
}

const epParam = (endpointId?: string) =>
  endpointId ? `&endpointId=${encodeURIComponent(endpointId)}` : '';

export const api = {
  authStatus: () =>
    request<{ initialized: boolean; unlocked: boolean; authenticated: boolean; version?: string }>(
      '/api/auth/status'
    ),

  authSetup: (passphrase: string) =>
    request<{ ok: boolean; initialized: boolean; unlocked: boolean; authenticated: boolean }>(
      '/api/auth/setup',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ passphrase }),
      }
    ),

  authUnlock: (passphrase: string) =>
    request<{ ok: boolean; initialized: boolean; unlocked: boolean; authenticated: boolean }>(
      '/api/auth/unlock',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ passphrase }),
      }
    ),

  authLock: () =>
    request<{ ok: boolean; unlocked: boolean; authenticated: boolean }>('/api/auth/lock', {
      method: 'POST',
    }),

  authLogout: () =>
    request<{ ok: boolean; authenticated: boolean }>('/api/auth/logout', {
      method: 'POST',
    }),

  listHosts: () => request<{ hosts: Host[] }>('/api/hosts').then((r) => r.hosts),

  createHost: (input: HostInput) =>
    request<Host>('/api/hosts', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }),

  updateHost: (id: string, input: HostInput) =>
    request<Host>(`/api/hosts/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }),

  deleteHost: (id: string) =>
    request<{ ok: boolean }>(`/api/hosts/${id}`, { method: 'DELETE' }),

  testHost: (id: string, endpointId?: string) =>
    request<{ ok: boolean; latencyMs: number; specs?: string }>(
      `/api/hosts/${id}/test${endpointId ? `?endpointId=${encodeURIComponent(endpointId)}` : ''}`,
      { method: 'POST' }
    ),

  networkTest: (id: string, endpointId?: string) =>
    request<NetworkReport>(
      `/api/hosts/${id}/network-test${endpointId ? `?endpointId=${encodeURIComponent(endpointId)}` : ''}`,
      { method: 'POST' }
    ),

  scpList: (hostId: string, path: string, endpointId?: string) =>
    request<{ path: string; entries: FileEntry[] }>(
      `/api/scp/${hostId}/list?path=${encodeURIComponent(path)}${epParam(endpointId)}`
    ),

  scpDownloadUrl: (hostId: string, path: string, endpointId?: string) =>
    `/api/scp/${hostId}/download?path=${encodeURIComponent(path)}${epParam(endpointId)}`,

  scpUpload: async (hostId: string, dir: string, file: File, endpointId?: string) => {
    const form = new FormData();
    form.append('dir', dir);
    form.append('file', file);
    return request<{ ok: boolean }>(
      `/api/scp/${hostId}/upload${endpointId ? `?endpointId=${encodeURIComponent(endpointId)}` : ''}`,
      {
        method: 'POST',
        body: form,
      }
    );
  },

  scpMkdir: (hostId: string, path: string, endpointId?: string) =>
    request<{ ok: boolean }>(
      `/api/scp/${hostId}/mkdir${endpointId ? `?endpointId=${encodeURIComponent(endpointId)}` : ''}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path }),
      }
    ),

  scpRename: (hostId: string, oldPath: string, newPath: string, endpointId?: string) =>
    request<{ ok: boolean }>(
      `/api/scp/${hostId}/rename${endpointId ? `?endpointId=${encodeURIComponent(endpointId)}` : ''}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ oldPath, newPath }),
      }
    ),

  scpDelete: (hostId: string, path: string, endpointId?: string) =>
    request<{ ok: boolean }>(
      `/api/scp/${hostId}/delete${endpointId ? `?endpointId=${encodeURIComponent(endpointId)}` : ''}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path }),
      }
    ),

  scpRead: (hostId: string, path: string, endpointId?: string) =>
    request<{ content: string }>(
      `/api/scp/${hostId}/read?path=${encodeURIComponent(path)}${epParam(endpointId)}`
    ),

  scpWrite: (hostId: string, dir: string, filename: string, content: string, endpointId?: string) =>
    request<{ ok: boolean }>(
      `/api/scp/${hostId}/write${endpointId ? `?endpointId=${encodeURIComponent(endpointId)}` : ''}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ dir, filename, content }),
      }
    ),

  listKeys: () => request<{ keys: SSHKey[] }>('/api/keys').then((r) => r.keys),

  generateKey: (input: {
    name?: string;
    algorithm: string;
    comment: string;
    passphrase?: string;
    dryRun?: boolean;
  }) =>
    request<any>('/api/keys/generate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }),

  getKeySecret: (id: string) =>
    request<SSHKey & { privateKey: string; passphrase: string }>(`/api/keys/${id}/secret`),

  updateKey: (
    id: string,
    input: {
      name: string;
      algorithm: string;
      publicKey: string;
      privateKey: string;
      passphrase?: string;
      comment?: string;
    }
  ) =>
    request<SSHKey>(`/api/keys/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }),

  importKey: (input: {
    name: string;
    privateKey: string;
    passphrase?: string;
    comment?: string;
  }) =>
    request<SSHKey>('/api/keys', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }),

  deleteKey: (id: string) =>
    request<{ ok: boolean }>(`/api/keys/${id}`, { method: 'DELETE' }),

  downloadPrivateKeyUrl: (id: string) => `/api/keys/${id}/private`,

  checkHostKey: (hostId: string, keyId: string) =>
    request<KeyCheckResult>(`/api/hosts/${hostId}/key-check`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ keyId }),
    }),

  injectHostKey: (hostId: string, keyId: string, switchHostAuth = true) =>
    request<{ result: KeyCheckResult; switched: boolean }>(`/api/hosts/${hostId}/key-inject`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ keyId, switchHostAuth }),
    }),

  listInvoices: (hostId: string) =>
    request<{ invoices: Invoice[] }>(`/api/hosts/${hostId}/invoices`).then((r) => r.invoices),

  uploadInvoice: async (
    hostId: string,
    payload: {
      file: File;
      title?: string;
      amount?: string;
      currency?: string;
      billedAt?: string;
      notes?: string;
    }
  ) => {
    const form = new FormData();
    form.append('file', payload.file);
    if (payload.title) form.append('title', payload.title);
    if (payload.amount) form.append('amount', payload.amount);
    if (payload.currency) form.append('currency', payload.currency);
    if (payload.billedAt) form.append('billedAt', payload.billedAt);
    if (payload.notes) form.append('notes', payload.notes);
    return request<Invoice>(`/api/hosts/${hostId}/invoices`, {
      method: 'POST',
      body: form,
    });
  },

  invoiceFileUrl: (invoiceId: string, download = false) =>
    `/api/invoices/${invoiceId}/file${download ? '?download=1' : ''}`,

  deleteInvoice: (invoiceId: string) =>
    request<{ ok: boolean }>(`/api/invoices/${invoiceId}`, { method: 'DELETE' }),

  importHosts: (payload: {
    format?: string;
    content: string;
    keyId?: string;
    dryRun?: boolean;
  }) =>
    request<any>('/api/hosts/import', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }),

  getTailscaleSetting: () =>
    request<{ hasAuthKey: boolean }>('/api/settings/tailscale'),

  saveTailscaleSetting: (authKey: string) =>
    request<{ ok: boolean }>('/api/settings/tailscale', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ authKey }),
    }),

  checkTailscale: (hostId: string) =>
    request<{ status: TailscaleStatus; synced: boolean }>(
      `/api/hosts/${hostId}/tailscale/check`,
      { method: 'POST' }
    ),

  injectTailscale: (
    hostId: string,
    payload: {
      authKey?: string;
      saveAuthKey?: boolean;
      hostname?: string;
      enableSSH?: boolean;
    }
  ) =>
    request<{ status: TailscaleStatus; synced: boolean }>(
      `/api/hosts/${hostId}/tailscale/inject`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      }
    ),

  exportEncryptedBackup: async (passphrase: string) => {
    const res = await fetch('/api/backup/export', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ passphrase }),
    });
    if (!res.ok) {
      const text = await res.text();
      let msg = text;
      try {
        msg = JSON.parse(text)?.error || text;
      } catch {}
      throw new Error(msg || `Export failed (${res.status})`);
    }
    const blob = await res.blob();
    const disp = res.headers.get('Content-Disposition') || '';
    const match = disp.match(/filename="([^"]+)"/);
    const filename = match?.[1] || 'vpscoop-backup.vpscoop';
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  },

  importEncryptedBackup: (passphrase: string, content: string) =>
    request<{
      hostsImported: number;
      hostsSkipped: number;
      keysImported: number;
      keysSkipped: number;
      invoicesImported: number;
    }>('/api/backup/import', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ passphrase, content }),
    }),

  getWebDAVConfig: () =>
    request<{
      url: string;
      username: string;
      hasPassword: boolean;
      hasPassphrase: boolean;
      autoSync: boolean;
    }>('/api/cloud/webdav'),

  saveWebDAVConfig: (cfg: {
    url: string;
    username: string;
    password?: string;
    passphrase?: string;
    autoSync?: boolean;
  }) =>
    request<{ ok: boolean }>('/api/cloud/webdav', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cfg),
    }),

  testWebDAV: (cfg?: { url?: string; username?: string; password?: string }) =>
    request<{ ok: boolean }>('/api/cloud/webdav/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(cfg || {}),
    }),

  listWebDAVFiles: () =>
    request<{ files: { name: string; size: number; lastModified: string }[] }>(
      '/api/cloud/webdav/files'
    ).then((r) => r.files),

  pushWebDAV: (passphrase?: string, keepSnapshot = true) =>
    request<{ ok: boolean; latest: string; snapshot: string; bytes: number }>(
      '/api/cloud/webdav/push',
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ passphrase: passphrase || '', keepSnapshot }),
      }
    ),

  pullWebDAV: (filename: string, passphrase?: string) =>
    request<{
      hostsImported: number;
      hostsSkipped: number;
      keysImported: number;
      keysSkipped: number;
      invoicesImported: number;
    }>('/api/cloud/webdav/pull', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ filename, passphrase: passphrase || '' }),
    }),

  getMCPToken: () =>
    request<{ token: string; fromEnv: boolean }>('/api/settings/mcp'),

  rotateMCPToken: () =>
    request<{ token: string }>('/api/settings/mcp/rotate', {
      method: 'POST',
    }),

  getWorkspaceTabs: () =>
    request<{ tabs: any[]; activeTabId: string }>('/api/workspace/tabs'),

  saveWorkspaceTabs: (tabs: any[], activeTabId: string) =>
    request<{ ok: boolean }>('/api/workspace/tabs', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ tabs, activeTabId }),
    }),

  closeSshSession: (sessionId: string) =>
    request<{ ok: boolean }>(`/api/ssh/sessions/${encodeURIComponent(sessionId)}`, {
      method: 'DELETE',
    }),

  checkUpgrade: () =>
    request<{
      currentVersion: string;
      latestVersion: string;
      hasUpdate: boolean;
      releaseName: string;
      releaseNotes: string;
      publishedAt: string;
      htmlUrl: string;
      assetName: string;
      downloadUrl: string;
      assetSize: number;
      canSelfUpdate: boolean;
      reason?: string;
      execPath: string;
      repo: string;
    }>('/api/system/upgrade/check'),

  applyUpgrade: (force = false) =>
    request<{ ok: boolean; version: string; restarting: boolean }>('/api/system/upgrade/apply', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ force }),
    }),
};
