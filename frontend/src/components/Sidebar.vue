<script setup>
import { computed, ref } from 'vue'
import Icon from './Icon.vue'

const props = defineProps({
  facets: { type: Object, default: () => ({ categories: [], tags: [], statuses: [], total: 0, favorites: 0 }) },
  filter: { type: Object, required: true },
})
const emit = defineEmits(['change', 'settings', 'new-category', 'category-menu'])

const STATUS_ORDER = [
  { key: 'unplayed', label: '未游玩', color: '#8b93a7' },
  { key: 'playing', label: '游玩中', color: '#4cc2ff' },
  { key: 'played', label: '已通关', color: '#5fd38d' },
  { key: 'abandoned', label: '弃坑', color: '#ff6b81' },
]

const showAllTags = ref(false)

const statusCounts = computed(() => {
  const map = {}
  for (const s of props.facets.statuses || []) map[s.name] = s.count
  return map
})

const visibleTags = computed(() => {
  const tags = props.facets.tags || []
  return showAllTags.value ? tags : tags.slice(0, 10)
})

const isActive = (f) => {
  const cur = props.filter
  if (f.type === 'all') return cur.type === 'all'
  if (f.type === 'fav') return cur.type === 'fav'
  if (f.type === 'status') return cur.type === 'status' && cur.status === f.status
  if (f.type === 'category') return cur.type === 'category' && cur.category === f.name
  if (f.type === 'tag') return cur.type === 'tag' && cur.tag === f.name
  return false
}

function pick(f) {
  emit('change', f)
}
</script>

<template>
  <aside class="sidebar">
    <div class="brand">
      <div class="brand-logo">
        <Icon name="sparkles" :size="19" :stroke="1.9" />
      </div>
      <div class="brand-text">
        <span class="brand-title">Galgame</span>
        <span class="brand-sub">Library</span>
      </div>
    </div>

    <div class="side-stats">
      <button
        class="stat-card"
        :class="{ active: isActive({ type: 'all' }) }"
        @click="pick({ type: 'all' })"
      >
        <span class="n">{{ facets.total }}</span>
        <span class="l">全部游戏</span>
      </button>
      <button
        class="stat-card"
        :class="{ active: isActive({ type: 'fav' }) }"
        @click="pick({ type: 'fav' })"
      >
        <span class="n">{{ facets.favorites }}</span>
        <span class="l">我的收藏</span>
      </button>
    </div>

    <nav class="nav">
      <div class="nav-group">
        <div class="nav-label"><span>游玩状态</span></div>
        <button
          v-for="s in STATUS_ORDER"
          :key="s.key"
          class="nav-item"
          :class="{ active: isActive({ type: 'status', status: s.key }) }"
          @click="pick({ type: 'status', status: s.key })"
        >
          <i
            class="badge-dot"
            :style="{
              background: s.color,
              boxShadow: `0 0 8px ${s.color}99`,
              width: '7px',
              height: '7px',
              marginLeft: '4px',
              marginRight: '1px',
            }"
          />
          <span>{{ s.label }}</span>
          <span class="count">{{ statusCounts[s.key] || 0 }}</span>
        </button>
      </div>

      <div class="nav-group">
        <div class="nav-label">
          <span>分类</span>
          <button title="新建分类" @click="emit('new-category')"><Icon name="plus" :size="13" /></button>
        </div>
        <div v-if="!facets.categories.length" class="nav-empty">还没有分类，点上面的 + 新建</div>
        <button
          v-for="c in facets.categories"
          :key="c.name"
          class="nav-item"
          :class="{ active: isActive({ type: 'category', name: c.name }) }"
          @click="pick({ type: 'category', category: c.name })"
          @contextmenu.prevent="emit('category-menu', { event: $event, category: c.name })"
        >
          <Icon class="ico" name="layers" :size="15" />
          <span>{{ c.name }}</span>
          <span class="count">{{ c.count }}</span>
        </button>
      </div>

      <div v-if="facets.tags.length" class="nav-group">
        <div class="nav-label"><span>标签</span></div>
        <button
          v-for="t in visibleTags"
          :key="t.name"
          class="nav-item"
          :class="{ active: isActive({ type: 'tag', name: t.name }) }"
          @click="pick({ type: 'tag', tag: t.name })"
        >
          <Icon class="ico" name="tag" :size="14" />
          <span>{{ t.name }}</span>
          <span class="count">{{ t.count }}</span>
        </button>
        <button
          v-if="facets.tags.length > 10"
          class="nav-item"
          style="color: var(--text-faint); font-size: 12px"
          @click="showAllTags = !showAllTags"
        >
          <Icon class="ico" :name="showAllTags ? 'chevronDown' : 'chevronRight'" :size="13" />
          <span>{{ showAllTags ? '收起' : `还有 ${facets.tags.length - 10} 个` }}</span>
        </button>
      </div>
    </nav>

    <div class="sidebar-footer">
      <button class="btn btn-ghost" @click="emit('settings')">
        <Icon name="settings" :size="15" />
        设置
      </button>
      <div class="side-hint">本地资料库 · {{ facets.total }} 款已入库</div>
    </div>
  </aside>
</template>
