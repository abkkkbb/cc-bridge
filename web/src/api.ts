const BASE = ''

let authToken = ''

export function setAuth(token: string) {
  authToken = token
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(BASE + path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${authToken}`,
    },
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || res.statusText)
  }
  return res.json()
}

export interface Account {
  id: number
  name: string
  email: string
  status: string
  token: string
  proxy_url: string
  device_id: string
  concurrency: number
  priority: number
  rate_limited_at?: string
  rate_limit_reset_at?: string
  created_at: string
  updated_at: string
}

export interface UsageStats {
  account_id: number
  account_name: string
  total_requests: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_read: number
  total_cache_creation: number
}

export interface Dashboard {
  accounts: { total: number; active: number; error: number; disabled: number }
  usage_24h: { requests: number; input_tokens: number; output_tokens: number }
}

export const api = {
  listAccounts: () => request<Account[]>('GET', '/admin/accounts'),
  createAccount: (a: Partial<Account>) => request<Account>('POST', '/admin/accounts', a),
  updateAccount: (id: number, a: Partial<Account>) => request<Account>('PUT', `/admin/accounts/${id}`, a),
  deleteAccount: (id: number) => request<void>('DELETE', `/admin/accounts/${id}`),
  testAccount: (id: number) => request<{ status: string; message?: string }>('POST', `/admin/accounts/${id}/test`),
  getUsage: (hours = 24) => request<UsageStats[]>('GET', `/admin/usage?hours=${hours}`),
  getDashboard: () => request<Dashboard>('GET', '/admin/dashboard'),
}
