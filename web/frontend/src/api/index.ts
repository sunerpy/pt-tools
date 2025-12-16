const BASE_URL = ''

interface ApiOptions extends RequestInit {
  body?: string
}

class ApiError extends Error {
  constructor(
    public status: number,
    message: string
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(path: string, options: ApiOptions = {}): Promise<T> {
  const response = await fetch(`${BASE_URL}${path}`, {
    credentials: 'same-origin',
    headers: {
      'Content-Type': 'application/json',
      ...options.headers
    },
    ...options
  })

  const contentType = response.headers.get('content-type') || ''
  const isJSON = contentType.includes('application/json')

  if (!response.ok) {
    const msg = await response.text()
    throw new ApiError(response.status, msg || `HTTP ${response.status}`)
  }

  if (isJSON) {
    return response.json() as Promise<T>
  }
  return response.text() as unknown as T
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, data?: unknown) =>
    request<T>(path, {
      method: 'POST',
      body: data ? JSON.stringify(data) : undefined
    }),
  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' })
}

// API 类型定义
export interface GlobalSettings {
  default_interval_minutes: number
  download_dir: string
  download_limit_enabled: boolean
  download_speed_limit: number
  torrent_size_gb: number
  auto_start: boolean
}

export interface QbitSettings {
  enabled: boolean
  url: string
  user: string
  password: string
}

export interface RSSConfig {
  id?: string
  name: string
  url: string
  category: string
  tag: string
  interval_minutes: number
}

export interface SiteConfig {
  enabled: boolean
  auth_method: string
  cookie: string
  api_key: string
  api_url: string
  rss: RSSConfig[]
}

export interface TaskItem {
  siteName: string
  title: string
  category: string
  tag: string
  torrentHash: string
  isFree: boolean
  freeLevel: string
  freeEndTime: string
  isDownloaded: boolean
  isPushed: boolean
  createdAt: string
  lastCheckTime: string
  isExpired: boolean
  retryCount: number
  lastError: string
  pushTime: string
}

export interface TaskListResponse {
  items: TaskItem[]
  total: number
  page: number
  page_size: number
}

export interface LogsResponse {
  lines: string[]
  path: string
  truncated: boolean
}

// API 方法
export const globalApi = {
  get: () => api.get<GlobalSettings>('/api/global'),
  save: (data: GlobalSettings) => api.post<void>('/api/global', data)
}

export const qbitApi = {
  get: () => api.get<QbitSettings>('/api/qbit'),
  save: (data: QbitSettings) => api.post<void>('/api/qbit', data)
}

export const sitesApi = {
  list: () => api.get<Record<string, SiteConfig>>('/api/sites'),
  get: (name: string) => api.get<SiteConfig>(`/api/sites/${name}`),
  save: (name: string, data: SiteConfig) => api.post<void>(`/api/sites/${name}`, data),
  delete: (name: string) => api.delete<void>(`/api/sites?name=${encodeURIComponent(name)}`),
  deleteRss: (name: string, id: string) =>
    api.delete<void>(`/api/sites/${name}?id=${encodeURIComponent(id)}`)
}

export const tasksApi = {
  list: (params: URLSearchParams) => api.get<TaskListResponse>(`/api/tasks?${params.toString()}`)
}

export const logsApi = {
  get: () => api.get<LogsResponse>('/api/logs')
}

export const controlApi = {
  stop: () => api.post<void>('/api/control/stop'),
  start: () => api.post<void>('/api/control/start')
}

export const passwordApi = {
  change: (data: { username: string; old: string; new: string }) =>
    api.post<void>('/api/password', data)
}
