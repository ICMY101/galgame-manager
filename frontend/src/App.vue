<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Sidebar from './components/Sidebar.vue'
import GameCard from './components/GameCard.vue'
import Icon from './components/Icon.vue'
import DetailDrawer from './components/DetailDrawer.vue'
import ImportModal from './components/ImportModal.vue'
import SettingsModal from './components/SettingsModal.vue'
import { api, STATUS_MAP, formatPlayTime, formatSize, placeholderGradient } from './api.js'

const games = ref([])
const facets = ref({ categories: [], tags: [], statuses: [], total: 0, favorites: 0 })
const settings = ref({ gridSize: 'md', translationTools: [] })
const loading = ref(true)

const filter = ref({ type: 'all' })
const keyword = ref('')
const sort = ref('title')
const view = ref('grid')

const detailId = ref(0)
const showImport = ref(false)
const showSettings = ref(false)
const importPath = ref('')
const toasts = ref([])
const ctx = ref(null)

let searchTimer = null
let toastSeq = 0

const gridMin = computed(() => ({ sm: 148, md: 184, lg: 220 }[settings.value.gridSize] || 184))

const SORT_LABEL = {
  title: '按标题',
  added: '按加入时间',
  rating: '按我的评分',
  playtime: '按游玩时长',
  lastplayed: '按最近游玩',
  developer: '按开发商',
}
const sortLabel = computed(() => SORT_LABEL[sort.value] || '按标题')

const filterTitle = computed(() => {
  const f = filter.value
  if (f.type === 'fav') return '我的收藏'
  if (f.type === 'status') return STATUS_MAP[f.status]?.label || '状态'
  if (f.type === 'category') return f.category
  if (f.type === 'tag') return `# ${f.tag}`
  return '全部游戏'
})

const eyebrow = computed(() => {
  const t = filter.value.type
  if (t === 'fav') return 'Favorites'
  if (t === 'status') return 'Status'
  if (t === 'category') return 'Category'
  if (t === 'tag') return 'Tag'
  return 'Library'
})

// 当前结果集的总占用体积
const totalSize = computed(() => games.value.reduce((s, g) => s + (g.sizeBytes || 0), 0))
const sizeText = computed(() => (totalSize.value > 0 ? formatSize(totalSize.value) : '—'))

const isFiltered = computed(() => filter.value.type !== 'all' || !!keyword.value.trim())

function clearFilter() {
  filter.value = { type: 'all' }
  keyword.value = ''
}

const query = computed(() => {
  const f = filter.value
  return {
    keyword: keyword.value.trim(),
    status: f.type === 'status' ? f.status : '',
    category: f.type === 'category' ? f.category : '',
    tag: f.type === 'tag' ? f.tag : '',
    favorite: f.type === 'fav',
    sort: sort.value,
  }
})

async function loadGames() {
  try {
    games.value = await api.listGames(query.value)
  } catch (e) {
    toast(e.message, 'error')
  }
}

async function loadFacets() {
  try {
    facets.value = await api.facets()
  } catch (e) {
    /* ignore */
  }
}

async function refreshAll() {
  await Promise.all([loadGames(), loadFacets()])
}

function toast(text, type = 'info') {
  const id = ++toastSeq
  toasts.value.push({ id, text, type })
  setTimeout(() => {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }, 3200)
}

function onToast(payload) {
  if (typeof payload === 'string') toast(payload)
  else toast(payload.text, payload.type)
}

watch(keyword, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(loadGames, 180)
})
watch([filter, sort], loadGames, { deep: true })

function setFilter(f) {
  filter.value = f
}

function openDetail(game) {
  detailId.value = game.id
}

async function quickLaunch(game) {
  try {
    await api.launch(game.id)
    toast(`正在启动 ${game.title}`, 'success')
    setTimeout(refreshAll, 2500)
  } catch (e) {
    toast(e.message, 'error')
    openDetail(game)
  }
}

async function toggleFav(game) {
  try {
    await api.setFavorite(game.id, !game.favorite)
    game.favorite = !game.favorite
    loadFacets()
    if (filter.value.type === 'fav') loadGames()
  } catch (e) {
    toast(e.message, 'error')
  }
}

function openContext({ event, game }) {
  ctx.value = { x: event.clientX, y: event.clientY, game }
}

function closeContext() {
  ctx.value = null
}

async function ctxAction(action) {
  const game = ctx.value?.game
  closeContext()
  if (!game) return
  if (action === 'launch') return quickLaunch(game)
  if (action === 'detail') return openDetail(game)
  if (action === 'fav') return toggleFav(game)
  if (action === 'folder') {
    api.openFolder(game.folder).catch((e) => toast(e.message, 'error'))
    return
  }
  if (action === 'remove') {
    if (!confirm(`从库中移除「${game.title}」？\n不会删除硬盘上的游戏文件。`)) return
    try {
      await api.deleteGame(game.id)
      toast('已从库中移除')
      if (detailId.value === game.id) detailId.value = 0
      refreshAll()
    } catch (e) {
      toast(e.message, 'error')
    }
    return
  }
  if (action.startsWith('status:')) {
    const s = action.split(':')[1]
    try {
      await api.setStatus(game.id, s)
      refreshAll()
    } catch (e) {
      toast(e.message, 'error')
    }
  }
}

async function newCategory() {
  const name = prompt('新建分类名称：')
  if (!name || !name.trim()) return
  try {
    await api.categoryAction('add', name.trim())
    await loadFacets()
    toast(`已创建分类「${name.trim()}」`, 'success')
  } catch (e) {
    toast(e.message, 'error')
  }
}

async function categoryMenu({ event, category }) {
  ctx.value = { x: event.clientX, y: event.clientY, category }
}

async function doCategoryAction(action) {
  const name = ctx.value?.category
  closeContext()
  if (!name) return
  if (action === 'rename') {
    const next = prompt('重命名分类：', name)
    if (!next || !next.trim() || next === name) return
    try {
      await api.categoryAction('rename', next.trim(), name)
      if (filter.value.type === 'category' && filter.value.category === name) {
        filter.value = { type: 'category', category: next.trim() }
      }
      refreshAll()
    } catch (e) {
      toast(e.message, 'error')
    }
  } else if (action === 'delete') {
    if (!confirm(`删除分类「${name}」？游戏本身不会被删除。`)) return
    try {
      await api.categoryAction('delete', name)
      if (filter.value.type === 'category' && filter.value.category === name) {
        filter.value = { type: 'all' }
      }
      refreshAll()
    } catch (e) {
      toast(e.message, 'error')
    }
  }
}

async function onImported() {
  await refreshAll()
}

function openImport(path = '') {
  importPath.value = path
  showImport.value = true
}

async function saveSettings(next) {
  settings.value = next
}

async function loadSettings() {
  try {
    settings.value = await api.getSettings()
  } catch {
    /* ignore */
  }
}

function onGlobalClick() {
  if (ctx.value) closeContext()
}
function onKey(e) {
  if (e.key === 'Escape') {
    if (ctx.value) closeContext()
    else if (showImport.value) showImport.value = false
    else if (showSettings.value) showSettings.value = false
    else if (detailId.value) detailId.value = 0
  }
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'f') {
    e.preventDefault()
    document.querySelector('.search input')?.focus()
  }
}

onMounted(async () => {
  await loadSettings()
  await refreshAll()
  loading.value = false
  window.addEventListener('click', onGlobalClick)
  window.addEventListener('keydown', onKey)

  // Wails 文件拖放事件（浏览器模式下不存在）
  if (window.runtime?.EventsOn) {
    window.runtime.EventsOn('folder-dropped', (paths) => {
      if (Array.isArray(paths) && paths.length) openImport(paths[0])
    })
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('click', onGlobalClick)
  window.removeEventListener('keydown', onKey)
  clearTimeout(searchTimer)
})

const ctxStyle = computed(() => {
  if (!ctx.value) return {}
  const w = typeof window !== 'undefined' ? window.innerWidth : 1200
  const h = typeof window !== 'undefined' ? window.innerHeight : 800
  return {
    left: Math.min(ctx.value.x, w - 200) + 'px',
    top: Math.min(ctx.value.y, h - 330) + 'px',
  }
})

// 无封面时按标题生成稳定的渐变色，列表视图用
function phStyle(game) {
  return placeholderGradient(game.title)
}

const emptyState = computed(() => facets.value.total === 0)
const noResult = computed(() => !emptyState.value && games.value.length === 0)
</script>

<template>
  <div class="app">
    <Sidebar
      :facets="facets"
      :filter="filter"
      @change="setFilter"
      @settings="showSettings = true"
      @new-category="newCategory"
      @category-menu="categoryMenu"
    />

    <div class="main">
      <header class="page-head">
        <div class="ph-text">
          <div class="ph-eyebrow">{{ eyebrow }}</div>
          <h1 :title="filterTitle">{{ filterTitle }}</h1>
          <div class="ph-sub">
            <span>{{ games.length }} 款游戏</span>
            <span class="dot" />
            <span>{{ sizeText }}</span>
            <span class="dot" />
            <span>{{ sortLabel }}</span>
            <button v-if="isFiltered" class="btn btn-sm btn-ghost" style="margin-left: 4px" @click="clearFilter">
              <Icon name="x" :size="11" /> 清除筛选
            </button>
          </div>
        </div>

        <div class="ph-actions">
          <div class="search">
            <Icon name="search" :size="15" />
            <input
              v-model="keyword"
              class="input"
              placeholder="搜索标题 / 开发商 / 标签"
            />
            <button v-if="keyword" class="search-clear" @click="keyword = ''">
              <Icon name="x" :size="11" />
            </button>
          </div>

          <select v-model="sort" class="select" style="width: 136px">
            <option value="title">按标题</option>
            <option value="added">按加入时间</option>
            <option value="rating">按我的评分</option>
            <option value="playtime">按游玩时长</option>
            <option value="lastplayed">按最近游玩</option>
            <option value="developer">按开发商</option>
          </select>

          <div class="view-toggle">
            <button :class="{ active: view === 'grid' }" title="网格视图" @click="view = 'grid'">
              <Icon name="grid" :size="14" />
            </button>
            <button :class="{ active: view === 'list' }" title="列表视图" @click="view = 'list'">
              <Icon name="list" :size="15" />
            </button>
          </div>

          <button class="btn btn-primary" @click="openImport()">
            <Icon name="plus" :size="15" />
            导入游戏
          </button>
        </div>
      </header>

      <div class="content">
        <!-- 完全空库 -->
        <div v-if="emptyState && !loading" class="empty">
          <div class="empty-art">
            <Icon name="gamepad" :size="46" :stroke="1.2" style="color: var(--accent-2); opacity: 0.9" />
          </div>
          <h2>把硬盘里的 galgame 收进一个库</h2>
          <p>
            选择一个根目录，程序会自动识别每一款游戏的启动文件和封面，
            并从 Bangumi / VNDB 补全简介、评分与标签。
          </p>
          <div class="empty-actions">
            <button class="btn btn-primary" @click="openImport()">
              <Icon name="folder" :size="15" /> 选择目录导入
            </button>
            <button class="btn" @click="showSettings = true">
              <Icon name="settings" :size="14" /> 设置
            </button>
          </div>
          <div class="empty-hint">
            <span><Icon name="check" :size="13" /> 自动识别启动文件</span>
            <span><Icon name="check" :size="13" /> 自动抓取封面与简介</span>
            <span><Icon name="check" :size="13" /> 统计游玩时长</span>
            <span><Icon name="check" :size="13" /> 分类、标签、评分</span>
          </div>
        </div>

        <!-- 有游戏 -->
        <template v-else>
          <div v-if="noResult" class="empty" style="min-height: 320px">
            <Icon name="search" :size="34" :stroke="1.2" style="color: var(--text-faint)" />
            <h2 style="font-size: 15px; margin-top: 10px">没有匹配的游戏</h2>
            <p>换个关键词，或者清除当前筛选条件。</p>
            <button class="btn" style="margin-top: 14px" @click="filter = { type: 'all' }; keyword = ''">
              查看全部游戏
            </button>
          </div>

          <div
            v-else-if="view === 'grid'"
            class="grid"
            :style="{ '--card-min': gridMin + 'px' }"
          >
            <GameCard
              v-for="g in games"
              :key="g.id"
              :game="g"
              @open="openDetail"
              @launch="quickLaunch"
              @favorite="toggleFav"
              @context="openContext"
            />
          </div>

          <div v-else class="list-view">
            <div class="list-head">
              <span />
              <span>名称</span>
              <span>开发商</span>
              <span>游玩时长</span>
              <span>状态</span>
            </div>
            <div v-for="g in games" :key="g.id" class="list-row" @click="openDetail(g)">
              <div class="thumb" :style="g.coverPath ? {} : phStyle(g)">
                <img v-if="g.coverPath" :src="api.coverUrl(g)" :alt="g.title" />
                <span v-else class="thumb-ph">{{ g.title.slice(0, 2) }}</span>
              </div>
              <div style="min-width: 0">
                <div class="name">{{ g.title }}</div>
                <div class="dev">{{ g.originalTitle || '' }}</div>
              </div>
              <div class="dev">{{ g.developer || '—' }}</div>
              <div class="dev">{{ g.playTime ? formatPlayTime(g.playTime) : '—' }}</div>
              <div>
                <span class="tag" :style="{ color: STATUS_MAP[g.status]?.color }">
                  {{ STATUS_MAP[g.status]?.label }}
                </span>
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- 详情抽屉 -->
    <DetailDrawer
      v-if="detailId"
      :game-id="detailId"
      :categories="facets.categories.map((c) => c.name)"
      :tools="settings.translationTools || []"
      @close="detailId = 0"
      @changed="refreshAll"
      @toast="onToast"
    />

    <!-- 导入 -->
    <ImportModal
      v-if="showImport"
      :initial-path="importPath"
      :settings="settings"
      @close="showImport = false"
      @imported="onImported"
      @toast="onToast"
    />

    <!-- 设置 -->
    <SettingsModal
      v-if="showSettings"
      :settings="settings"
      @close="showSettings = false"
      @saved="saveSettings"
      @toast="onToast"
    />

    <!-- 右键菜单 -->
    <div
      v-if="ctx"
      class="context-menu"
      :style="ctxStyle"
      @click.stop
    >
      <template v-if="ctx.game">
        <button class="mi" @click="ctxAction('launch')"><Icon name="play" :size="13" /> 启动游戏</button>
        <button class="mi" @click="ctxAction('detail')"><Icon name="edit" :size="13" /> 详情 / 编辑</button>
        <button class="mi" @click="ctxAction('fav')">
          <Icon :name="ctx.game.favorite ? 'heartFill' : 'heart'" :size="13" />
          {{ ctx.game.favorite ? '取消收藏' : '收藏' }}
        </button>
        <div class="sep" />
        <div class="mtitle">设置状态</div>
        <button
          v-for="(info, key) in STATUS_MAP"
          :key="key"
          class="mi"
          @click="ctxAction('status:' + key)"
        >
          <i class="badge-dot" :style="{ background: info.color, width: '7px', height: '7px' }" />
          {{ info.label }}
          <Icon v-if="ctx.game.status === key" name="check" :size="12" style="margin-left: auto" />
        </button>
        <div class="sep" />
        <button class="mi" @click="ctxAction('folder')"><Icon name="folder" :size="13" /> 打开所在目录</button>
        <button class="mi danger" @click="ctxAction('remove')"><Icon name="trash" :size="13" /> 从库中移除</button>
      </template>

      <template v-else-if="ctx.category">
        <button class="mi" @click="doCategoryAction('rename')"><Icon name="edit" :size="13" /> 重命名分类</button>
        <button class="mi danger" @click="doCategoryAction('delete')"><Icon name="trash" :size="13" /> 删除分类</button>
      </template>
    </div>

    <!-- 提示 -->
    <div class="toasts">
      <div v-for="t in toasts" :key="t.id" class="toast" :class="t.type">
        <Icon
          :name="t.type === 'error' ? 'alert' : t.type === 'success' ? 'check' : 'sparkles'"
          :size="14"
          :style="{
            color:
              t.type === 'error' ? 'var(--danger)' : t.type === 'success' ? 'var(--success)' : 'var(--accent-2)',
          }"
        />
        {{ t.text }}
      </div>
    </div>
  </div>
</template>
