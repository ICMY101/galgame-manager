<script setup>
import { computed, onBeforeUnmount, ref } from 'vue'
import Icon from './Icon.vue'
import { api, formatSize } from '../api.js'

const props = defineProps({
  initialPath: { type: String, default: '' },
  settings: { type: Object, default: () => ({}) },
})
const emit = defineEmits(['close', 'imported', 'toast'])

const root = ref(props.initialPath || '')
const recursive = ref(props.settings.scanRecursive !== false)
const fetchMeta = ref(props.settings.autoFetchMeta !== false)

const stage = ref('pick') // pick | scanning | done
const progress = ref({ running: false, done: 0, total: 0, current: '', stage: '', items: [], error: '' })
const selected = ref({})
const importing = ref(false)
const dragging = ref(false)
let timer = null

const items = computed(() => progress.value.items || [])
const selectableCount = computed(() => items.value.length)
const selectedCount = computed(() => Object.values(selected.value).filter(Boolean).length)
const percent = computed(() => {
  const s = progress.value
  // 元数据阶段按「已返回结果的条目数」计算，避免进度条提前满格
  if (s.stage === 'metadata' && s.items?.length) {
    const settled = s.items.filter((i) => i.metaStatus && i.metaStatus !== 'pending').length
    return Math.min(90, Math.round((settled / s.items.length) * 90))
  }
  if (s.stage === 'covers') {
    if (!s.coverTotal) return 95
    return 90 + Math.min(9, Math.round((s.coverDone / s.coverTotal) * 9))
  }
  if (s.stage === 'done') return 100
  if (!s.total) return 8
  return Math.min(90, Math.round((s.done / s.total) * 90))
})

const stageText = computed(() => {
  const s = progress.value
  if (s.stage === 'discovering') return '正在遍历目录…'
  if (s.stage === 'scanning') return `正在识别启动文件与封面 ${s.done}/${s.total}`
  if (s.stage === 'metadata') return `正在联网补全信息（${s.current || '…'}）`
  if (s.stage === 'covers') return `正在下载封面 ${s.coverDone || 0}/${s.coverTotal || 0}`
  if (s.stage === 'done') return '扫描完成'
  return ''
})

async function browse() {
  try {
    const res = await api.pickFolder()
    if (res.path) root.value = res.path
  } catch (e) {
    emit('toast', { type: 'error', text: e.message })
  }
}

async function startScan() {
  if (!root.value.trim()) {
    emit('toast', { type: 'error', text: '请先选择或输入游戏根目录' })
    return
  }
  stage.value = 'scanning'
  progress.value = { running: true, done: 0, total: 0, current: '', stage: 'discovering', items: [], error: '' }
  try {
    await api.startScan(root.value.trim(), recursive.value, fetchMeta.value)
    poll()
  } catch (e) {
    stage.value = 'pick'
    emit('toast', { type: 'error', text: e.message })
  }
}

function poll() {
  stopPoll()
  timer = setInterval(async () => {
    try {
      const s = await api.scanStatus()
      progress.value = s
      if (s.items) {
        for (const it of s.items) {
          const key = it.folder
          if (!(key in selected.value)) {
            selected.value[key] = !it.alreadyInLib
          }
        }
      }
      if (!s.running) {
        stopPoll()
        stage.value = 'done'
        if (s.error) emit('toast', { type: 'error', text: s.error })
      }
    } catch (e) {
      stopPoll()
      emit('toast', { type: 'error', text: e.message })
    }
  }, 400)
}

function stopPoll() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

async function cancel() {
  try {
    await api.cancelScan()
  } catch {
    /* ignore */
  }
  stopPoll()
  stage.value = 'done'
}

function toggleAll(v) {
  const next = {}
  items.value.forEach((it) => {
    next[it.folder] = v
  })
  selected.value = next
}

async function doImport() {
  const chosen = items.value.filter((it) => selected.value[it.folder])
  if (!chosen.length) {
    emit('toast', { type: 'error', text: '请至少勾选一款游戏' })
    return
  }
  importing.value = true
  try {
    const payload = chosen.map((it) => ({
      folder: it.folder,
      title: it.title,
      exePath: it.exePath,
      coverPath: it.coverPath,
      coverUrl: it.coverUrl,
      developer: it.developer,
      releaseDate: it.releaseDate,
      description: it.description,
      rating: it.rating,
      tags: it.tags,
      sizeBytes: it.sizeBytes,
      source: it.source,
      sourceId: it.sourceId,
    }))
    const res = await api.importGames(payload)
    emit('imported', res)
    emit('toast', {
      type: 'success',
      text: `导入完成：新增 ${res.imported} 款，更新 ${res.updated} 款`,
    })
    emit('close')
  } catch (e) {
    emit('toast', { type: 'error', text: e.message })
  } finally {
    importing.value = false
  }
}

function onDrop(e) {
  dragging.value = false
  const f = e.dataTransfer?.files?.[0]
  const p = f?.path
  if (p) {
    root.value = p
  } else {
    emit('toast', { type: 'error', text: '请使用「选择目录」按钮指定路径' })
  }
}

function metaBadge(it) {
  if (it.metaStatus === 'ok') return { text: '已匹配', color: 'var(--success)' }
  if (it.metaStatus === 'none') return { text: '未找到', color: 'var(--text-faint)' }
  if (it.metaStatus === 'error') return { text: '失败', color: 'var(--danger)' }
  if (it.metaStatus === 'skipped') return { text: '跳过', color: 'var(--text-faint)' }
  return { text: '查询中', color: 'var(--accent-2)' }
}

onBeforeUnmount(stopPoll)
</script>

<template>
  <teleport to="body">
    <div class="overlay" @click.self="emit('close')">
      <div class="modal">
        <div class="modal-head">
          <Icon name="download" :size="17" />
          <h3>导入游戏</h3>
          <span class="sub">扫描目录，自动识别启动文件与封面</span>
          <button class="modal-close" @click="emit('close')"><Icon name="x" :size="16" /></button>
        </div>

        <div class="modal-body">
          <!-- 选择目录 -->
          <template v-if="stage === 'pick'">
            <div class="field">
              <label class="field-label">游戏根目录</label>
              <div class="field-row">
                <input
                  v-model="root"
                  class="input"
                  placeholder="例如 D:\Galgame 或 \\NAS\galgame"
                  @keyup.enter="startScan"
                />
                <button class="btn" style="flex: none" @click="browse">
                  <Icon name="folder" :size="14" /> 选择目录
                </button>
              </div>
              <div class="field-hint">
                程序会遍历该目录，把每个包含启动程序的子目录识别为一款游戏。
              </div>
            </div>

            <div
              class="dropzone"
              :class="{ dragging }"
              @dragover.prevent="dragging = true"
              @dragleave.prevent="dragging = false"
              @drop.prevent="onDrop"
            >
              <div class="big">把 galgame 文件夹拖到这里</div>
              <p>或者点击上方「选择目录」浏览硬盘</p>
            </div>

            <div style="margin-top: 18px; display: flex; gap: 22px; flex-wrap: wrap">
              <label class="checkbox">
                <input v-model="recursive" type="checkbox" />
                递归扫描多层子目录
              </label>
              <label class="checkbox">
                <input v-model="fetchMeta" type="checkbox" />
                联网补全简介 / 评分 / 标签 / 封面
              </label>
            </div>
          </template>

          <!-- 扫描中 / 结果 -->
          <template v-else>
            <div style="display: flex; align-items: center; gap: 10px; margin-bottom: 10px">
              <span style="font-size: 12.5px; color: var(--text-dim)">{{ stageText }}</span>
              <div style="flex: 1" />
              <span style="font-size: 12px; color: var(--text-faint)">
                已发现 {{ items.length }} 款
              </span>
            </div>

            <div class="progress">
              <div class="progress-bar" :style="{ width: percent + '%' }" />
            </div>

            <div v-if="stage === 'done' && items.length" style="display: flex; align-items: center; gap: 10px; margin: 14px 0 8px">
              <button class="btn btn-sm" @click="toggleAll(true)">全选</button>
              <button class="btn btn-sm" @click="toggleAll(false)">全不选</button>
              <span style="font-size: 12px; color: var(--text-faint)">
                已选 {{ selectedCount }} / {{ selectableCount }}
              </span>
            </div>

            <div v-if="items.length" class="scan-list">
              <div v-for="it in items" :key="it.folder" class="scan-item">
                <label class="checkbox">
                  <input v-model="selected[it.folder]" type="checkbox" />
                </label>
                <div class="cov">
                  <img v-if="it.coverPath" :src="api.previewUrl(it.coverPath)" :alt="it.title" />
                  <Icon v-else-if="it.coverStatus === 'pending'" name="loader" :size="14" class="spin" />
                  <span v-else>{{ it.title.slice(0, 1) }}</span>
                </div>
                <div class="info">
                  <div class="t">{{ it.title }}</div>
                  <div class="p">{{ it.exePath || '未识别到启动文件' }}</div>
                </div>
                <div class="meta">
                  <span v-if="it.alreadyInLib" class="tag">已在库中</span>
                  <span v-else-if="it.sizeBytes" class="tag">{{ formatSize(it.sizeBytes) }}</span>
                  <span class="tag" :style="{ color: metaBadge(it).color }">{{ metaBadge(it).text }}</span>
                </div>
              </div>
            </div>

            <div v-else-if="stage === 'scanning'" style="padding: 40px; text-align: center; color: var(--text-faint)">
              <Icon name="loader" :size="20" class="spin" />
            </div>
          </template>
        </div>

        <div class="modal-foot">
          <template v-if="stage === 'pick'">
            <button class="btn" @click="emit('close')">取消</button>
            <div style="flex: 1" />
            <button class="btn btn-primary" @click="startScan">
              <Icon name="search" :size="14" /> 开始扫描
            </button>
          </template>

          <template v-else-if="stage === 'scanning'">
            <span style="font-size: 12px; color: var(--text-faint)">
              耗时 {{ progress.elapsed ? progress.elapsed.toFixed(0) : 0 }} 秒
            </span>
            <div style="flex: 1" />
            <button class="btn" @click="cancel">停止扫描</button>
          </template>

          <template v-else>
            <button class="btn" @click="stage = 'pick'">
              <Icon name="refresh" :size="13" /> 重新选择
            </button>
            <div style="flex: 1" />
            <button class="btn" @click="emit('close')">取消</button>
            <button class="btn btn-primary" :disabled="importing || !selectedCount" @click="doImport">
              {{ importing ? '导入中…' : `导入所选 (${selectedCount})` }}
            </button>
          </template>
        </div>
      </div>
    </div>
  </teleport>
</template>
