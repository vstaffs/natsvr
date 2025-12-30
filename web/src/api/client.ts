const API_BASE = '/api'

export interface Agent {
  id: string
  name: string
  ip: string
  online: boolean
  lastSeen: string
  activeTunnels: number
  txBytes: number
  rxBytes: number
}

export interface ForwardRule {
  id: string
  name: string
  type: 'local' | 'remote' | 'p2p' | 'cloud-self'
  protocol: 'tcp' | 'udp'
  sourceAgentId?: string
  listenPort: number
  targetAgentId?: string
  targetHost: string
  targetPort: number
  enabled: boolean
  rateLimit: number     // bytes per second, 0 = unlimited
  trafficLimit: number  // max total bytes, 0 = unlimited
  trafficUsed: number   // current traffic used
  createdAt: string
}

export interface Stats {
  txBytes: number
  rxBytes: number
  txSpeed: number
  rxSpeed: number
  onlineCount: number
  totalRules: number
}

export interface Token {
  id: string
  name: string
  token: string
  usageCount: number
  createdAt: string
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  })
  
  if (!response.ok) {
    const error = await response.text()
    throw new Error(error || `HTTP ${response.status}`)
  }
  
  return response.json()
}

export const api = {
  // Stats
  getStats: () => request<Stats>('/stats'),
  
  // Agents
  getAgents: () => request<Agent[]>('/agents'),
  getAgent: (id: string) => request<Agent>(`/agents/${id}`),
  
  // Forward Rules
  getForwardRules: () => request<ForwardRule[]>('/forward-rules'),
  createForwardRule: (rule: Omit<ForwardRule, 'id' | 'enabled' | 'createdAt' | 'trafficUsed'>) =>
    request<ForwardRule>('/forward-rules', {
      method: 'POST',
      body: JSON.stringify(rule),
    }),
  updateForwardRule: (id: string, updates: Partial<ForwardRule>) =>
    request<ForwardRule>(`/forward-rules/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(updates),
    }),
  deleteForwardRule: (id: string) =>
    request<void>(`/forward-rules/${id}`, { method: 'DELETE' }),
  
  // Tokens
  getTokens: () => request<Token[]>('/tokens'),
  createToken: (name: string) =>
    request<Token>('/tokens', {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
  deleteToken: (id: string) =>
    request<void>(`/tokens/${id}`, { method: 'DELETE' }),
}

