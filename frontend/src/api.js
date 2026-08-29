// Backend bilan yagona aloqa nuqtasi. Token localStorage'da saqlanadi.
const BASE = import.meta.env.VITE_API_URL || ''

export const tokenStore = {
  get: () => localStorage.getItem('sahiy_token'),
  set: (t) => localStorage.setItem('sahiy_token', t),
  clear: () => localStorage.removeItem('sahiy_token'),
}

export class ApiError extends Error {
  constructor(message, status) {
    super(message)
    this.status = status
  }
}

async function request(path, { method = 'GET', body } = {}) {
  const headers = { 'Content-Type': 'application/json' }
  const token = tokenStore.get()
  if (token) headers.Authorization = `Bearer ${token}`

  const res = await fetch(BASE + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  if (res.status === 401) {
    tokenStore.clear()
    window.location.hash = '#/login'
    throw new ApiError('Sessiya tugadi — qaytadan kiring', 401)
  }

  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) throw new ApiError(data?.error || `Xato (${res.status})`, res.status)
  return data
}

export const api = {
  login: (login, password) => request('/api/auth/login', { method: 'POST', body: { login, password } }),
  me: () => request('/api/auth/me'),

  stats: () => request('/api/stats'),
  daily: (days = 14) => request(`/api/stats/daily?days=${days}`),
  clients: (days = 30, limit = 20) => request(`/api/stats/clients?days=${days}&limit=${limit}`),

  interactions: (status = '', page = 1, limit = 20) =>
    request(`/api/interactions?status=${status}&page=${page}&limit=${limit}`),
  interaction: (id) => request(`/api/interactions/${id}`),
  patchInteraction: (id, body) => request(`/api/interactions/${id}`, { method: 'PATCH', body }),
  approve: (id) => request(`/api/interactions/${id}/approve`, { method: 'POST' }),
  reject: (id) => request(`/api/interactions/${id}/reject`, { method: 'POST' }),

  promts: () => request('/api/promts'),
  createPromt: (body) => request('/api/promts', { method: 'POST', body }),
  updatePromt: (id, body) => request(`/api/promts/${id}`, { method: 'PUT', body }),
  deletePromt: (id) => request(`/api/promts/${id}`, { method: 'DELETE' }),

  settings: () => request('/api/settings'),
  saveSettings: (body) => request('/api/settings', { method: 'PUT', body }),

  run: (conversation_id, client_id) =>
    request('/api/agent/run', { method: 'POST', body: { conversation_id, client_id } }),
}

// Ko'p ishlatiladigan formatlar.
export const fmt = {
  date: (s) => (s ? new Date(s).toLocaleString('uz-UZ', { dateStyle: 'short', timeStyle: 'short' }) : '—'),
  day: (s) => (s ? new Date(s).toLocaleDateString('uz-UZ', { day: '2-digit', month: '2-digit' }) : '—'),
  num: (n) => (n ?? 0).toLocaleString('uz-UZ'),
  usd: (n) => '$' + (n ?? 0).toFixed(4),
}

export const STATUS = {
  pending: { label: 'Kutmoqda', cls: 'st-pending' },
  sent: { label: 'Avto yuborildi', cls: 'st-sent' },
  approved: { label: 'Tasdiqlandi', cls: 'st-sent' },
  rejected: { label: 'Rad etildi', cls: 'st-rejected' },
  failed: { label: 'Xato', cls: 'st-failed' },
}
