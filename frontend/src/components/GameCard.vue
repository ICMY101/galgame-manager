<script setup>
import { computed, ref } from 'vue'
import Icon from './Icon.vue'
import { api, STATUS_MAP, formatPlayTime, placeholderGradient } from '../api.js'

const props = defineProps({
  game: { type: Object, required: true },
})
const emit = defineEmits(['open', 'launch', 'favorite', 'context'])

const imgFailed = ref(false)

const cover = computed(() => (imgFailed.value ? '' : api.coverUrl(props.game)))
const status = computed(() => STATUS_MAP[props.game.status] || STATUS_MAP.unplayed)

// 无封面时按标题生成稳定的渐变色占位
const placeholderStyle = computed(() => placeholderGradient(props.game.title))

const initials = computed(() => {
  const t = (props.game.title || '?').trim()
  return t.length <= 8 ? t : t.slice(0, 8)
})

const metaLine = computed(() => {
  const g = props.game
  const bits = []
  if (g.communityRating > 0) bits.push({ star: true, text: g.communityRating.toFixed(1) })
  if (g.rating > 0) bits.push({ star: true, text: `我的 ${g.rating.toFixed(1)}` })
  if (g.playTime > 0) bits.push({ text: formatPlayTime(g.playTime) })
  if (!bits.length && g.developer) bits.push({ text: g.developer })
  return bits
})

function onFav(e) {
  e.stopPropagation()
  emit('favorite', props.game)
}
</script>

<template>
  <div
    class="card"
    @click="emit('open', game)"
    @contextmenu.prevent.stop="emit('context', { event: $event, game })"
  >
    <div class="cover-wrap">
      <img
        v-if="cover"
        :src="cover"
        :alt="game.title"
        loading="lazy"
        @error="imgFailed = true"
      />
      <div v-else class="cover-ph" :style="placeholderStyle">{{ initials }}</div>

      <div class="badge-row">
        <span v-if="game.status !== 'unplayed'" class="badge">
          <i class="badge-dot" :style="{ background: status.color }" />
          {{ status.label }}
        </span>
      </div>

      <button
        class="fav-btn"
        :class="{ on: game.favorite }"
        :title="game.favorite ? '取消收藏' : '收藏'"
        @click="onFav"
      >
        <Icon :name="game.favorite ? 'heartFill' : 'heart'" :size="14" />
      </button>

      <div class="cover-overlay">
        <button
          class="play-btn"
          :class="{ disabled: !game.exePath }"
          @click.stop="game.exePath ? emit('launch', game) : emit('open', game)"
        >
          <Icon name="play" :size="12" />
          {{ game.exePath ? '启动游戏' : '设置启动文件' }}
        </button>
      </div>
    </div>

    <div class="card-body">
      <div class="card-title" :title="game.title">{{ game.title }}</div>
      <div class="card-meta">
        <template v-for="(bit, i) in metaLine" :key="i">
          <span v-if="i > 0" class="sep">·</span>
          <span :class="{ star: bit.star }">
            <Icon v-if="bit.star" name="star" :size="10" />
            {{ bit.text }}
          </span>
        </template>
      </div>
    </div>
  </div>
</template>
