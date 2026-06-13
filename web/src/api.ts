// Thin fetch wrapper for the panel API. Same-origin in production (served by
// the Go binary); proxied to :8000 by vite during `npm run dev`.

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    ...init,
  })
  if (!res.ok) {
    // Prefer the backend's (Chinese) error message; fall back to a Chinese
    // generic with the status code rather than the English statusText.
    let msg = `请求失败（${res.status}）`
    try {
      const body = await res.json()
      if (body?.error) msg = body.error
    } catch {
      // ignore non-JSON error bodies
    }
    throw new Error(msg)
  }
  return res.json() as Promise<T>
}

export interface Overview {
  interface: string
  configured: boolean
  live: boolean
  address: string[] | null
  listenPort: number
  clientsTotal: number
  online: number
  rxBytes: number
  txBytes: number
  lastUpdated: string
}

export interface Client {
  name: string
  publicKey: string
  allowedIPs: string[] | null
  subnets: string[] | null
  endpoint: string
  lastHandshake: string | null
  rxBytes: number
  txBytes: number
  uploadTotal: number
  downloadTotal: number
  downloadLimit: number
  expiresAt: string | null
  blocked: boolean
  blockReason: string
  online: boolean
}

export interface ClientsResult {
  clients: Client[]
  configured: boolean
  live: boolean
}

export interface AddResult {
  name: string
  publicKey: string
  address: string
  privateKey: string
  configText: string
  qrCode: string
  reloaded: boolean
  reloadError?: string
}

export interface WriteResult {
  reloaded: boolean
  reloadError?: string
}

export interface ClientConfigView {
  name: string
  address: string
  configText: string
  qrCode: string
  hasPrivateKey: boolean
}

export interface Pool {
  cidr: string
  network: string
  nextFree: string
}

export interface SystemInfo {
  hostname: string
  os: string
  kernel: string
  arch: string
  uptime: number
  load1: number
  cpuCount: number
  memTotal: number
  memUsed: number
  ipv4Forwarding: boolean
  wgVersion: string
  wgModuleLoaded: boolean
  interface: string
  wgRunning: boolean
}

export interface Settings {
  address: string[]
  listenPort: number
  mtu: number
  endpointHost: string
  configured: boolean
  live: boolean
  autostart: boolean
}

export interface SettingsResult {
  applied: boolean
  restarted: boolean
  applyError?: string
}

export const api = {
  login: (username: string, password: string) =>
    request<{ username: string }>('/api/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    }),
  logout: () => request<{ status: string }>('/api/logout', { method: 'POST' }),
  me: () => request<{ username: string }>('/api/me'),
  overview: () => request<Overview>('/api/overview'),
  system: () => request<SystemInfo>('/api/system'),
  versionInfo: () =>
    request<{ current: string; latest?: string; updateAvailable: boolean }>('/api/version'),
  selfUpdate: () => request<{ status: string; version: string }>('/api/update', { method: 'POST' }),
  enableIPForward: () =>
    request<{ ipv4Forwarding: boolean }>('/api/system/ip-forward', { method: 'POST' }),
  interfaceControl: (action: 'up' | 'down' | 'restart') =>
    request<{ interface: string; running: boolean }>('/api/interface', {
      method: 'POST',
      body: JSON.stringify({ action }),
    }),
  setAutostart: (enabled: boolean) =>
    request<{ autostart: boolean }>('/api/interface/autostart', {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
  clients: () => request<ClientsResult>('/api/clients'),
  createClient: (name: string, address?: string, subnets?: string[]) =>
    request<AddResult>('/api/clients', {
      method: 'POST',
      body: JSON.stringify({ name, address: address ?? '', subnets: subnets ?? [] }),
    }),
  setClientSubnets: (publicKey: string, subnets: string[]) =>
    request<WriteResult>('/api/clients/subnets', {
      method: 'POST',
      body: JSON.stringify({ publicKey, subnets }),
    }),
  setClientLimit: (publicKey: string, downloadLimit: number, expiresAt: string | null) =>
    request<WriteResult>('/api/clients/limit', {
      method: 'POST',
      body: JSON.stringify({ publicKey, downloadLimit, expiresAt }),
    }),
  resetClientUsage: (publicKey: string) =>
    request<WriteResult>('/api/clients/usage/reset', {
      method: 'POST',
      body: JSON.stringify({ publicKey }),
    }),
  network: () => request<{ pools: Pool[] }>('/api/network'),
  detectPublicIP: () => request<{ ip: string }>('/api/public-ip'),
  ipInfo: (ip: string) =>
    request<{ country: string; city: string }>('/api/ip-info?ip=' + encodeURIComponent(ip)),
  getSettings: () => request<Settings>('/api/settings'),
  updateSettings: (s: Partial<Settings>) =>
    request<SettingsResult>('/api/settings', { method: 'POST', body: JSON.stringify(s) }),
  changePassword: (currentPassword: string, username: string, newPassword: string) =>
    request<{ status: string }>('/api/account/password', {
      method: 'POST',
      body: JSON.stringify({ currentPassword, username, newPassword }),
    }),
  deleteClient: (publicKey: string) =>
    request<WriteResult>('/api/clients/delete', {
      method: 'POST',
      body: JSON.stringify({ publicKey }),
    }),
  renameClient: (publicKey: string, name: string) =>
    request<WriteResult>('/api/clients/rename', {
      method: 'POST',
      body: JSON.stringify({ publicKey, name }),
    }),
  clientConfig: (publicKey: string) =>
    request<ClientConfigView>('/api/clients/config', {
      method: 'POST',
      body: JSON.stringify({ publicKey }),
    }),
}

// Shared formatting helpers.
export function fmtBytes(n: number): string {
  if (!n || n <= 0 || !Number.isFinite(n)) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  // Clamp the unit index: values in (0,1) give a negative index (→ units[-1]
  // === undefined, printing "512.0 undefined"); huge values overflow the array.
  const i = Math.min(units.length - 1, Math.max(0, Math.floor(Math.log(n) / Math.log(1024))))
  return `${(n / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

export function fmtSince(iso: string | null): string {
  if (!iso) return '从未'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return '从未'
  const s = Math.max(0, Math.floor((Date.now() - then) / 1000))
  if (s < 60) return `${s} 秒前`
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前`
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前`
  return `${Math.floor(s / 86400)} 天前`
}
