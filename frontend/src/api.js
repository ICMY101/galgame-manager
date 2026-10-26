// 与 Go 后端通信的 API 封装。所有请求走同源 /api，Wails 与浏览器模式通用。
const BASE = '/api'

async function request(path, { method = 'GET', body, timeout = 60000 } = {}) {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), timeout)
  try {
    const res = await fetch(BASE + path, {
      method,
      headers: body ? { 'Content-Type': 'application/json' } : undefined,
      body: body ? JSON.stringify(body) : undefined,
      signal: controller.signal,
    })
    const text = await res.text()
    let data = null
    if (text) {
      try {
        data = JSON.parse(text)
      } catch {
        data = { error: text }
      }
    }
    if (!res.ok) {
      throw new Error((data && data.error) || `请求失败 (${res.status})`)
    }
    return data
  } catch (err) {
    if (err.name === 'AbortError') throw new Error('请求超时，请稍后重试')
    throw err
  } finally {
    clearTimeout(timer)
  }
}

export const api = {
  // 列表 / 详情
  listGames(query = {}) {
    const params = new URLSearchParams()
    Object.entries(query).forEach(([k, v]) => {
      if (v !== '' && v !== null && v !== undefined && v !== false) {
        params.set(k, v === true ? '1' : v)
      }
    })
    const qs = params.toString()
    return request(`/games${qs ? '?' + qs : ''}`)
  },
  getGame(id) {
    return request(`/games/${id}`)
  },
  updateGame(id, patch) {
    return request(`/games/${id}`, { method: 'POST', body: patch })
  },
  deleteGame(id) {
    return request(`/games/${id}`, { method: 'DELETE' })
  },
  setFavorite(id, value) {
    return request(`/games/${id}/favorite`, { method: 'POST', body: { value } })
  },
  setStatus(id, value) {
    return request(`/games/${id}/status`, { method: 'POST', body: { value } })
  },
  setCategories(id, names) {
    return request(`/games/${id}/categories`, { method: 'POST', body: { names } })
  },
  setCover(id, mode, path = '') {
    return request(`/games/${id}/cover`, { method: 'POST', body: { mode, path } })
  },

  // 聚合
  facets() {
    return request('/facets')
  },
  categories() {
    return request('/categories')
  },
  categoryAction(action, name, oldName = '') {
    return request('/categories', { method: 'POST', body: { action, name, oldName } })
  },

  // 扫描 / 导入
  startScan(root, recursive, fetchMeta) {
    return request('/scan', { method: 'POST', body: { root, recursive, fetchMeta } })
  },
  scanStatus() {
    return request('/scan/status')
  },
  cancelScan() {
    return request('/scan/cancel', { method: 'POST' })
  },
  importGames(items) {
    return request('/import', { method: 'POST', body: { items }, timeout: 120000 })
  },

  // 启动
  launch(id) {
    return request(`/games/${id}/launch`, { method: 'POST' })
  },
  runTool(id, index) {
    return request(`/games/${id}/tool`, { method: 'POST', body: { index } })
  },
  openFolder(path) {
    return request('/open', { method: 'POST', body: { path } })
  },

  // 元数据
  refreshMeta(id, query = '') {
    return request('/metadata/refresh', { method: 'POST', body: { id, query }, timeout: 60000 })
  },
  listExes(folder) {
    return request(`/exes?folder=${encodeURIComponent(folder)}`)
  },

  // 设置
  getSettings() {
    return request('/settings')
  },
  sourceStatus() {
    return request('/sources')
  },
  saveSettings(settings) {
    return request('/settings', { method: 'POST', body: settings })
  },

  // 原生对话框（仅桌面模式可用）
  pickFolder() {
    return request('/dialog/folder', { method: 'POST' })
  },
  pickFile(title, filter) {
    return request('/dialog/file', { method: 'POST', body: { title, filter } })
  },

  coverUrl(game) {
    if (!game || !game.coverPath) return ''
    return `${BASE}/cover/${game.id}?v=${game.coverVersion || 0}`
  },
  previewUrl(path) {
    if (!path) return ''
    return `${BASE}/preview?path=${encodeURIComponent(path)}`
  },
}

export function formatPlayTime(seconds) {
  const s = Math.max(0, Math.floor(seconds || 0))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  if (h > 0) return `${h} 小时 ${m} 分`
  if (m > 0) return `${m} 分钟`
  return s > 0 ? '不到 1 分钟' : '未游玩'
}

export function formatSize(bytes) {
  if (!bytes || bytes <= 0) return ''
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let n = bytes
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(i >= 3 ? 2 : 1)} ${units[i]}`
}

export const STATUS_MAP = {
  unplayed: { label: '未游玩', color: '#8b93a7' },
  playing: { label: '游玩中', color: '#4cc2ff' },
  played: { label: '已通关', color: '#5fd38d' },
  abandoned: { label: '弃坑', color: '#ff6b81' },
}

// 无封面时的占位渐变：按标题取一个稳定色相，但限制在紫→粉的品牌色区间内
export function placeholderGradient(title) {
  const t = title || '?'
  let h = 0
  for (let i = 0; i < t.length; i++) h = (h * 31 + t.charCodeAt(i)) % 1000
  const hue = 244 + (h % 74) // 244 ~ 318
  return {
    background: `linear-gradient(150deg, hsl(${hue}, 46%, 34%) 0%, hsl(${(hue + 26) % 360}, 40%, 18%) 100%)`,
  }
}
