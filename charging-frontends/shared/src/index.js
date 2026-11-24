// Shared tenant white-label config loader with cache and default fallback.
const cache = new Map()

const DEFAULT_CONFIG = {
  name: 'OpenCharge',
  logoUrl: '',
  primaryColor: '#1677ff',
  servicePhone: '',
  roamingEnabled: false
}

export async function loadTenantConfig({ appId, domain, apiBase = '' }) {
  const key = appId || domain || 'default'
  if (cache.has(key)) return cache.get(key)
  try {
    const query = appId ? `appId=${encodeURIComponent(appId)}` : `domain=${encodeURIComponent(domain)}`
    const res = await fetch(`${apiBase}/api/tenant/config?${query}`)
    if (!res.ok) throw new Error(`HTTP ${res.status}`)
    const config = await res.json()
    const merged = { ...DEFAULT_CONFIG, ...config }
    cache.set(key, merged)
    return merged
  } catch (err) {
    console.warn('tenant config fallback to defaults', err)
    return DEFAULT_CONFIG
  }
}

export function clearTenantConfigCache(key) {
  if (key) cache.delete(key)
  else cache.clear()
}
