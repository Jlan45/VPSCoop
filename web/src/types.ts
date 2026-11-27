export type AuthType = 'password' | 'key';
export type Currency = 'USD' | 'CNY' | 'EUR' | 'GBP' | 'JPY';
export type BillingCycle =
  | 'monthly'
  | 'quarterly'
  | 'semiannual'
  | 'annual'
  | 'biennial'
  | 'triennial'
  | 'onetime';

export interface HostEndpoint {
  id: string;
  label: string;
  host: string;
  port: number;
}

export interface Host {
  id: string;
  name: string;
  groupName: string;
  provider: string;
  region: string;
  host: string;
  port: number;
  endpoints: HostEndpoint[];
  username: string;
  authType: AuthType;
  keyId: string;
  hasPassword: boolean;
  hasPrivateKey: boolean;
  cost: number;
  currency: Currency;
  billingCycle: BillingCycle;
  expiresAt: string;
  purchasedAt: string;
  specs: string;
  tags: string[];
  notes: string;
  lastConnectedAt: string;
  createdAt: string;
  updatedAt: string;
}

export interface HostInput {
  name: string;
  groupName: string;
  provider: string;
  region: string;
  host: string;
  port: number;
  endpoints?: HostEndpoint[];
  username: string;
  authType: AuthType;
  keyId?: string;
  password?: string;
  privateKey?: string;
  passphrase?: string;
  cost: number;
  currency: Currency;
  billingCycle: BillingCycle;
  expiresAt: string;
  purchasedAt: string;
  specs: string;
  tags: string[];
  notes: string;
}

export interface FileEntry {
  name: string;
  path: string;
  isDir: boolean;
  isSymlink: boolean;
  size: number;
  mode: string;
  modTime: string;
  owner: string;
  group: string;
}

export interface WorkspaceTab {
  id: string;
  kind: 'ssh' | 'scp';
  hostId: string;
  endpointId?: string;
  endpointLabel?: string;
  title: string;
  subtitle: string;
}

export interface SSHKey {
  id: string;
  name: string;
  algorithm: string;
  publicKey: string;
  fingerprint: string;
  comment: string;
  hasPassphrase: boolean;
  createdAt: string;
}

export interface KeyCheckResult {
  keyId: string;
  keyName: string;
  inAuthorizedKeys: boolean;
  authWorks: boolean;
  latencyMs?: number;
  error?: string;
}

export interface Invoice {
  id: string;
  hostId: string;
  title: string;
  filename: string;
  mimeType: string;
  size: number;
  amount: number;
  currency: string;
  billedAt: string;
  notes: string;
  createdAt: string;
}

export interface TailscaleStatus {
  installed: boolean;
  running: boolean;
  tailscaleIp: string;
  hostname: string;
  output?: string;
}
