<script setup>
import { computed, ref, watch } from 'vue'
import Icon from './Icon.vue'
import { api, STATUS_MAP, formatPlayTime, formatSize, placeholderGradient } from '../api.js'

const props = defineProps({
  gameId: { type: Number, default: 0 },
  categories: { type: Array, default: () => [] },
  tools: { type: Array, default: () => [] },
})
const emit = defineEmits(['close', 'changed', 'toast', 'launch'])

const detail = ref(null)
const loading = ref(true)
const saving = ref(false)
const refreshing = ref(false)
const descExpanded = ref(false)
const edit = ref(null)
const catInput = ref('')
const showCatInput = ref(false)
const errorMsg = ref('')

const game = computed(() => (detail.value ? detail.value.game : null))
const cover = computed(() => (game.value ? api.coverUrl(game.value) : ''))
const statusInfo = computed(() => STATUS_MAP[game.value?.status] || STATUS_MAP.unplayed)

const placeholderStyle = computed(() => placeholderGradient(game.value?.title))

async function load() {
  if (!props.gameId) return
  loading.value = true
  errorMsg.value = ''
  try {
    const d = await api.getGame(props.gameId)
    detail.value = d
    edit.value = {
      title: d.game.title,
      exePath: d.game.exePath,
      launchArgs: d.game.launchArgs,
      windowMode: d.game.windowMode,
      notes: d.game.notes,
      status: d.game.status,
      tags: d.game.tags,
      rating: d.game.rating,
    }
  } catch (e) {
    errorMsg.value = e.message
  } finally {
    loading.value = false
  }
}

watch(() => props.gameId, load, { immediate: true })

const dirty = computed(() => {
  if (!game.value || !edit.value) return false
  const g = game.value
  return (
    edit.value.title !== g.title ||
    edit.value.exePath !== g.exePath ||
    edit.value.launchArgs !== g.launchArgs ||
    edit.value.windowMode !== g.windowMode ||
    edit.value.notes !== g.notes ||
    edit.value.status !== g.status ||
    edit.value.tags !== g.tags ||
    edit.value.rating !== g.rating
  )
})

async function save() {
  if (!game.value) return
  saving.value = true
  try {
    await api.updateGame(game.value.id, { ...game.value, ...edit.value })
    await load()
    emit('changed')
    emit('toast', { type: 'success', text: '已保存' })
  } catch (e) {
    emit('toast', { type: 'error', text: e.message })
  } finally {
    saving.value = false
  }
}

async function toggleFav() {
  try {
    await api.setFavorite(game.value.id, !game.value.favorite)
    game.value.favorite = !game.value.favorite
    emit('changed')
  } catch (e) {
    emit('toast', { type: 'error', text: e.message })
  }
}

async function setStatus(s) {
  edit.value.status = s
  await save()
}

async function setRating(v) {
  edit.value.rating = edit.value.rating === v ? 0 : v
  await save()
}

async function doLaunch() {
  try {
    await api.launch(game.value.id)
    emit('toast', { type: 'success', text: `正在启动 ${game.value.title}` })
    emit('changed')
    setTimeout(load, 2500)
  } catch (e) {
    emit('toast', { type: 'error', text: e.message })
  }
}

async function refreshMeta() {
  refreshing.value = true
  try {
    const res = await api.refreshMeta(game.value.id, game.value.title)
    await load()
    emit('changed')
    emit('toast', {
      type: 'success',
      text: `已从 ${res.source === 'vndb' ? 'VNDB' : 'Bangumi'} 更新元数据`,
    })
  } catch (e) {
    emit('toast', { type: 'error', text: e.message })
  } finally {
    refreshing.value = false
  }
}

async function pickExe() {
  try {
    const res = await api.pickFile('选择启动文件', '可执行文件|*.exe')
    if (res.path) {
      edit.value.exePath = res.path
      await save()
    }
  } catch (e) {
    emit('toast', { type: 'error', text: e.message })
  }
}

async function pickCover() {
  try {
    const res = await api.pickFile('选择封面图片', '图片|*.jpg;*.jpeg;*.png;*.webp;*.bmp')
    if (res.path) {
      await api.setCover(game.value.id, 'file', res.path)
      await load()
      emit('changed')
      emit('toast', { type: 'success', text: '封面已更新' })
    }
  } catch (e) {
    emit('toast', { type: 'error', text: e.message })
  }
}

async function coverFromExe() {
  try {
    await api.setCover(game.value.id, 'exe')
    await load()
    emit('changed')
    emit('toast', { type: 'success', text: '已从启动文件提取图标' })
  } catch (e) {
    emit('toast', { type: 'error', text: e.message })
  }
}

async function addCategory() {
  const name = catInput.value.trim()
  if (!name) return
  const list = [...(detail.value.categories || [])]
  if (!list.includes(name)) list.push(name)
  await api.setCategories(game.value.id, list)
  detail.value.categories = list
  catInput.value = ''
  showCatInput.value = false
  emit('changed')
}

async function removeCategory(name) {
  const list = (detail.value.categories || []).filter((c) => c !== name)
  await api.setCategories(game.value.id, list)
  detail.value.categories = list
  emit('changed')
}

function openFolder() {
  api.openFolder(game.value.folder).catch((e) => emit('toast', { type: 'error', text: e.message }))
}

function runTool(i) {
  api.runTool(game.value.id, i).catch((e) => emit('toast', { type: 'error', text: e.message }))
}

const tagList = computed(() => (edit.value?.tags ? edit.value.tags.split(',').filter(Boolean) : []))
const availableCats = computed(() =>
  props.categories.filter((c) => !(detail.value?.categories || []).includes(c))
)
</script>

<template>
  <teleport to="body">
    <div class="drawer-mask" @click="emit('close')" />
    <aside class="drawer">
      <button class="detail-close" @click="emit('close')"><Icon name="x" :size="16" /></button>

      <div v-if="loading" style="padding: 60px 24px; text-align: center; color: var(--text-faint)">
        <Icon name="loader" :size="22" class="spin" />
      </div>

      <div v-else-if="errorMsg" style="padding: 60px 24px; text-align: center; color: var(--text-dim)">
        {{ errorMsg }}
      </div>

      <template v-else-if="game">
        <div class="drawer-scroll">
          <div class="detail-hero">
            <div v-if="cover" class="bg" :style="{ backgroundImage: `url(${cover})` }" />
            <div class="fade" />
            <div class="inner">
              <div class="detail-cover">
                <img v-if="cover" :src="cover" :alt="game.title" />
                <div v-else class="cover-ph" :style="placeholderStyle" style="font-size: 12px">
                  {{ game.title.slice(0, 8) }}
                </div>
              </div>
              <div class="detail-titles">
                <h2>{{ game.title }}</h2>
                <div class="orig">{{ game.originalTitle || game.developer || '　' }}</div>
              </div>
            </div>
          </div>

          <div class="detail-actions">
            <button class="btn btn-primary" :disabled="!game.exePath" @click="doLaunch">
              <Icon name="play" :size="13" />
              启动游戏
            </button>
            <button class="btn btn-icon" :title="game.favorite ? '取消收藏' : '收藏'" @click="toggleFav">
              <Icon :name="game.favorite ? 'heartFill' : 'heart'" :size="15" :style="game.favorite ? 'color:var(--warn)' : ''" />
            </button>
            <button class="btn btn-icon" title="打开目录" @click="openFolder">
              <Icon name="folder" :size="15" />
            </button>
            <button class="btn btn-icon" title="刷新元数据" :disabled="refreshing" @click="refreshMeta">
              <Icon name="refresh" :size="15" :class="{ spin: refreshing }" />
            </button>
          </div>

          <div class="detail-body">
            <div class="stat-row">
              <div class="stat">
                <div class="k">游玩时长</div>
                <div class="v">{{ formatPlayTime(game.playTime) }}</div>
              </div>
              <div class="stat">
                <div class="k">启动次数</div>
                <div class="v">{{ game.playCount }} 次</div>
              </div>
              <div class="stat">
                <div class="k">最近游玩</div>
                <div class="v" style="font-size: 12px">{{ game.lastPlayed || '—' }}</div>
              </div>
            </div>

            <!-- 我的评分 -->
            <div class="section" style="margin-top: 4px">
              <div class="section-title">
                <span>我的评分</span>
                <span class="line" />
                <span style="color: var(--warn); font-weight: 700; letter-spacing: 0">
                  {{ edit.rating > 0 ? edit.rating.toFixed(0) + ' 分' : '未评分' }}
                </span>
              </div>
              <div class="stars">
                <svg
                  v-for="i in 10"
                  :key="i"
                  class="s"
                  :class="{ on: edit.rating >= i }"
                  viewBox="0 0 24 24"
                  fill="currentColor"
                  @click="setRating(i)"
                >
                  <path d="M12 3.5l2.7 5.6 6.1.9-4.4 4.3 1 6.1-5.4-2.9-5.4 2.9 1-6.1L3.2 10l6.1-.9L12 3.5z" />
                </svg>
              </div>
              <div v-if="game.communityRating > 0" style="font-size: 11.5px; color: var(--text-faint); margin-top: 6px">
                社区评分 {{ game.communityRating.toFixed(1) }} / 10 · 来源
                {{ game.source === 'vndb' ? 'VNDB' : game.source === 'bangumi' ? 'Bangumi' : '网络' }}
              </div>
            </div>

            <!-- 状态 -->
            <div class="section">
              <div class="section-title"><span>游玩状态</span><span class="line" /></div>
              <div style="display: flex; gap: 6px; flex-wrap: wrap">
                <button
                  v-for="(info, key) in STATUS_MAP"
                  :key="key"
                  class="btn btn-sm"
                  :style="
                    edit.status === key
                      ? `background:${info.color}22;border-color:${info.color}66;color:${info.color}`
                      : ''
                  "
                  @click="setStatus(key)"
                >
                  {{ info.label }}
                </button>
              </div>
            </div>

            <!-- 分类 -->
            <div class="section">
              <div class="section-title">
                <span>分类</span>
                <span class="line" />
                <button class="link" @click="showCatInput = !showCatInput">+ 添加</button>
              </div>
              <div class="tag-cloud">
                <span v-for="c in detail.categories" :key="c" class="tag tag-accent">
                  {{ c }}
                  <button style="margin-left: 4px; opacity: 0.6" @click="removeCategory(c)">
                    <Icon name="x" :size="10" />
                  </button>
                </span>
                <span v-if="!detail.categories.length" style="font-size: 12px; color: var(--text-faint)">
                  还没有分类
                </span>
              </div>
              <div v-if="showCatInput" style="margin-top: 8px; display: flex; gap: 6px">
                <input
                  v-model="catInput"
                  class="input"
                  placeholder="输入分类名后回车"
                  style="height: 30px; font-size: 12.5px"
                  @keyup.enter="addCategory"
                />
                <button class="btn btn-sm" @click="addCategory">添加</button>
              </div>
              <div v-if="availableCats.length" style="margin-top: 8px; display: flex; gap: 5px; flex-wrap: wrap">
                <button
                  v-for="c in availableCats.slice(0, 8)"
                  :key="c"
                  class="tag"
                  style="cursor: pointer"
                  @click="catInput = c; addCategory()"
                >
                  + {{ c }}
                </button>
              </div>
            </div>

            <!-- 标签 -->
            <div class="section" v-if="tagList.length">
              <div class="section-title"><span>标签</span><span class="line" /></div>
              <div class="tag-cloud">
                <span v-for="t in tagList" :key="t" class="tag">{{ t }}</span>
              </div>
            </div>

            <!-- 简介 -->
            <div class="section" v-if="game.description">
              <div class="section-title"><span>简介</span><span class="line" /></div>
              <div class="desc" :class="{ clamped: !descExpanded }">{{ game.description }}</div>
              <button class="link" style="margin-top: 6px" @click="descExpanded = !descExpanded">
                {{ descExpanded ? '收起' : '展开全部' }}
              </button>
            </div>

            <!-- 编辑 -->
            <div class="section">
              <div class="section-title"><span>启动设置</span><span class="line" /></div>

              <div class="edit-row">
                <label>标题</label>
                <input v-model="edit.title" class="input" />
              </div>

              <div class="edit-row">
                <label>启动文件</label>
                <div style="display: flex; gap: 6px; min-width: 0">
                  <input v-model="edit.exePath" class="input" style="min-width: 0" :title="edit.exePath" />
                  <button class="btn btn-sm" @click="pickExe">浏览</button>
                </div>
              </div>

              <div v-if="detail.exeOptions.length > 1" style="margin: -4px 0 10px 84px; display: flex; gap: 5px; flex-wrap: wrap">
                <button
                  v-for="e in detail.exeOptions.slice(0, 6)"
                  :key="e"
                  class="tag"
                  style="cursor: pointer"
                  :style="e === edit.exePath ? 'background:var(--accent-soft);color:var(--accent-2)' : ''"
                  :title="e"
                  @click="edit.exePath = e; save()"
                >
                  {{ e.split('\\').pop() }}
                </button>
              </div>

              <div class="edit-row">
                <label>启动参数</label>
                <input v-model="edit.launchArgs" class="input" placeholder="例如 -windowed" />
              </div>

              <div class="edit-row">
                <label>窗口模式</label>
                <select v-model="edit.windowMode" class="select">
                  <option value="default">跟随游戏默认</option>
                  <option value="windowed">窗口化</option>
                  <option value="fullscreen">全屏</option>
                  <option value="borderless">无边框</option>
                </select>
              </div>

              <div class="edit-row">
                <label>备注</label>
                <input v-model="edit.notes" class="input" placeholder="通关路线、汉化补丁等" />
              </div>
            </div>

            <!-- 翻译工具 -->
            <div class="section" v-if="tools.length">
              <div class="section-title"><span>翻译工具</span><span class="line" /></div>
              <div style="display: flex; gap: 6px; flex-wrap: wrap">
                <button v-for="(t, i) in tools" :key="i" class="btn btn-sm" @click="runTool(i)">
                  <Icon name="external" :size="12" />
                  {{ t.name }}
                </button>
              </div>
            </div>

            <!-- 路径信息 -->
            <div class="section">
              <div class="section-title"><span>文件</span><span class="line" /></div>
              <div class="path-box" style="margin-bottom: 6px">{{ game.folder }}</div>
              <div style="display: flex; gap: 10px; align-items: center; font-size: 11.5px; color: var(--text-faint)">
                <span v-if="game.sizeBytes">占用 {{ formatSize(game.sizeBytes) }}</span>
                <span v-if="game.dateAdded">加入于 {{ game.dateAdded }}</span>
                <span v-if="!detail.exists" style="color: var(--danger)">目录已不存在</span>
              </div>
            </div>

            <!-- 封面操作 -->
            <div class="section">
              <div class="section-title"><span>封面</span><span class="line" /></div>
              <div style="display: flex; gap: 6px; flex-wrap: wrap">
                <button class="btn btn-sm" @click="pickCover"><Icon name="image" :size="12" /> 选择图片</button>
                <button class="btn btn-sm" @click="coverFromExe"><Icon name="gamepad" :size="12" /> 取启动文件图标</button>
              </div>
            </div>
          </div>
        </div>

        <div v-if="dirty" class="modal-foot" style="border-top: 1px solid var(--border-soft)">
          <span style="font-size: 12px; color: var(--text-faint)">有未保存的修改</span>
          <div style="flex: 1" />
          <button class="btn btn-sm" @click="load">放弃</button>
          <button class="btn btn-primary btn-sm" :disabled="saving" @click="save">
            {{ saving ? '保存中…' : '保存修改' }}
          </button>
        </div>
      </template>
    </aside>
  </teleport>
</template>
