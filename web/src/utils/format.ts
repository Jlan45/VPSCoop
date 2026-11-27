import type { BillingCycle, Currency } from '../types';

export const CURRENCY_SYMBOLS: Record<Currency, string> = {
  USD: '$',
  CNY: '¥',
  EUR: '€',
  GBP: '£',
  JPY: '¥',
};

export const BILLING_LABELS: Record<BillingCycle, string> = {
  monthly: '/ mo',
  quarterly: '/ qtr',
  semiannual: '/ 6mo',
  annual: '/ yr',
  biennial: '/ 2yr',
  triennial: '/ 3yr',
  onetime: 'one-time',
};

export const BILLING_LABELS_ZH: Record<BillingCycle, string> = {
  monthly: '月付',
  quarterly: '季付',
  semiannual: '半年付',
  annual: '年付',
  biennial: '两年付',
  triennial: '三年付',
  onetime: '一次性',
};

export function formatCost(cost: number, currency: Currency, cycle: BillingCycle): string {
  const sym = CURRENCY_SYMBOLS[currency] || '$';
  const num = Number.isInteger(cost) ? cost.toString() : cost.toFixed(2);
  return `${sym}${num} ${BILLING_LABELS[cycle] || ''}`.trim();
}

export function toMonthlyEquivalent(cost: number, cycle: BillingCycle): number {
  switch (cycle) {
    case 'monthly':
      return cost;
    case 'quarterly':
      return cost / 3;
    case 'semiannual':
      return cost / 6;
    case 'annual':
      return cost / 12;
    case 'biennial':
      return cost / 24;
    case 'triennial':
      return cost / 36;
    case 'onetime':
      return 0;
  }
}

export interface ExpiryInfo {
  daysLeft: number | null;
  status: 'none' | 'expired' | 'urgent' | 'soon' | 'healthy';
  label: string;
}

export function getExpiryInfo(expiresAt: string): ExpiryInfo {
  if (!expiresAt) {
    return { daysLeft: null, status: 'none', label: '未设置到期时间' };
  }
  const target = new Date(expiresAt + 'T23:59:59');
  if (Number.isNaN(target.getTime())) {
    return { daysLeft: null, status: 'none', label: expiresAt };
  }
  const now = new Date();
  const diffMs = target.getTime() - now.getTime();
  const daysLeft = Math.ceil(diffMs / (1000 * 60 * 60 * 24));

  if (daysLeft < 0) {
    return { daysLeft, status: 'expired', label: `已过期 ${Math.abs(daysLeft)} 天` };
  }
  if (daysLeft === 0) {
    return { daysLeft: 0, status: 'urgent', label: '今天到期' };
  }
  if (daysLeft <= 7) {
    return { daysLeft, status: 'urgent', label: `剩余 ${daysLeft} 天` };
  }
  if (daysLeft <= 30) {
    return { daysLeft, status: 'soon', label: `剩余 ${daysLeft} 天` };
  }
  return { daysLeft, status: 'healthy', label: `剩余 ${daysLeft} 天` };
}

export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  const val = bytes / Math.pow(k, i);
  return `${val >= 10 || i === 0 ? val.toFixed(0) : val.toFixed(1)} ${sizes[i]}`;
}
