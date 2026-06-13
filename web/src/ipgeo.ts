import { reactive } from 'vue'
import { api } from './api'

// Module-level cache shared across components/route changes, so an IP's
// geolocation is fetched at most once per app session. (The backend also caches
// per IP, protecting the rate-limited upstream across reloads and tabs.)
export interface IpInfo {
  loading: boolean
  text?: string
  error?: boolean
}

const cache = reactive<Record<string, IpInfo>>({})

// Pull the bare IP out of a WireGuard endpoint: "1.2.3.4:51820" or
// "[2001:db8::1]:51820".
export function ipOf(endpoint?: string): string {
  if (!endpoint) return ''
  if (endpoint.startsWith('[')) return endpoint.slice(1, endpoint.indexOf(']'))
  const i = endpoint.lastIndexOf(':')
  return i === -1 ? endpoint : endpoint.slice(0, i)
}

export async function lookupIP(endpoint?: string): Promise<void> {
  const ip = ipOf(endpoint)
  if (!ip) return
  const cur = cache[ip]
  // Skip if a lookup is in flight or already succeeded; only a prior *error*
  // is allowed to retry (the backend throttles repeated upstream calls).
  if (cur && (cur.loading || cur.text)) return
  cache[ip] = { loading: true }
  try {
    const info = await api.ipInfo(ip)
    const text = [info.country, info.city].filter(Boolean).join(' · ')
    cache[ip] = { loading: false, text: text || '未知地区' }
  } catch {
    cache[ip] = { loading: false, error: true }
  }
}

export function endpointTip(endpoint?: string): string {
  const ip = ipOf(endpoint)
  const info = ip ? cache[ip] : undefined
  if (!info || info.loading) return '查询中…'
  if (info.error) return '查询失败'
  return info.text || '未知地区'
}
