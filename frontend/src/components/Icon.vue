<script setup>
// 轻量内联图标集，避免引入图标库依赖。
const props = defineProps({
  name: { type: String, required: true },
  size: { type: [Number, String], default: 16 },
  stroke: { type: [Number, String], default: 1.8 },
})

const paths = {
  search: '<circle cx="11" cy="11" r="7"/><path d="M20 20l-3.5-3.5"/>',
  x: '<path d="M18 6L6 18M6 6l12 12"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  play: '<path d="M7 4.5v15l13-7.5-13-7.5z" fill="currentColor" stroke="none"/>',
  star: '<path d="M12 3.5l2.7 5.6 6.1.9-4.4 4.3 1 6.1-5.4-2.9-5.4 2.9 1-6.1L3.2 10l6.1-.9L12 3.5z" fill="currentColor" stroke="none"/>',
  starOutline: '<path d="M12 3.5l2.7 5.6 6.1.9-4.4 4.3 1 6.1-5.4-2.9-5.4 2.9 1-6.1L3.2 10l6.1-.9L12 3.5z"/>',
  heart: '<path d="M12 20.3l-1.3-1.2C5.9 14.7 3 12.1 3 8.9 3 6.4 5 4.4 7.5 4.4c1.5 0 2.9.7 3.8 1.8l.7.9.7-.9c.9-1.1 2.3-1.8 3.8-1.8C19 4.4 21 6.4 21 8.9c0 3.2-2.9 5.8-7.7 10.2L12 20.3z"/>',
  heartFill: '<path d="M12 20.3l-1.3-1.2C5.9 14.7 3 12.1 3 8.9 3 6.4 5 4.4 7.5 4.4c1.5 0 2.9.7 3.8 1.8l.7.9.7-.9c.9-1.1 2.3-1.8 3.8-1.8C19 4.4 21 6.4 21 8.9c0 3.2-2.9 5.8-7.7 10.2L12 20.3z" fill="currentColor" stroke="none"/>',
  folder: '<path d="M3 7.5A2 2 0 015 5.5h4l2 2.5h8a2 2 0 012 2v8a2 2 0 01-2 2H5a2 2 0 01-2-2v-10.5z"/>',
  settings: '<circle cx="12" cy="12" r="3.2"/><path d="M19.4 15a1.6 1.6 0 00.3 1.8l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.6 1.6 0 00-1.8-.3 1.6 1.6 0 00-1 1.5V21a2 2 0 11-4 0v-.1A1.6 1.6 0 008 19.4a1.6 1.6 0 00-1.8.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.6 1.6 0 00.3-1.8 1.6 1.6 0 00-1.5-1H2a2 2 0 110-4h.1A1.6 1.6 0 004.6 8a1.6 1.6 0 00-.3-1.8l-.1-.1a2 2 0 112.8-2.8l.1.1a1.6 1.6 0 001.8.3H9a1.6 1.6 0 001-1.5V2a2 2 0 114 0v.1a1.6 1.6 0 001 1.5 1.6 1.6 0 001.8-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.6 1.6 0 00-.3 1.8V9a1.6 1.6 0 001.5 1H22a2 2 0 110 4h-.1a1.6 1.6 0 00-1.5 1z"/>',
  grid: '<rect x="3.5" y="3.5" width="7" height="7" rx="1.6"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.6"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.6"/><rect x="13.5" y="13.5" width="7" height="7" rx="1.6"/>',
  list: '<path d="M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01"/>',
  refresh: '<path d="M20 11a8 8 0 10-2.3 6.3M20 5v6h-6"/>',
  trash: '<path d="M4 7h16M9.5 7V5.2A1.2 1.2 0 0110.7 4h2.6a1.2 1.2 0 011.2 1.2V7M6.5 7l.8 12.1a1.5 1.5 0 001.5 1.4h6.4a1.5 1.5 0 001.5-1.4L17.5 7"/>',
  edit: '<path d="M4 20h4l10.5-10.5a2.1 2.1 0 00-3-3L5 17v3z"/>',
  image: '<rect x="3" y="4.5" width="18" height="15" rx="2.2"/><circle cx="8.5" cy="9.5" r="1.6"/><path d="M21 15.5l-4.5-4.2L7 19"/>',
  external: '<path d="M14 4h6v6M20 4l-8.5 8.5M18 14v5a1.8 1.8 0 01-1.8 1.8H5.8A1.8 1.8 0 014 19V8.8A1.8 1.8 0 015.8 7h5"/>',
  check: '<path d="M4.5 12.5l5 5 10-10.5"/>',
  chevronDown: '<path d="M6 9.5l6 6 6-6"/>',
  chevronRight: '<path d="M9.5 6l6 6-6 6"/>',
  gamepad: '<rect x="2.5" y="7.5" width="19" height="10" rx="4.5"/><path d="M7 11v3M5.5 12.5h3M15.5 11.5h.01M17.8 13.8h.01"/>',
  book: '<path d="M4 5.5A2 2 0 016 3.5h13v17H6a2 2 0 00-2 2v-17z"/><path d="M4 19.5a2 2 0 012-2h13"/>',
  tag: '<path d="M20.5 12.7l-7.8 7.8a1.8 1.8 0 01-2.5 0l-7-7a1.8 1.8 0 01-.5-1.3V4.5A1.5 1.5 0 014.2 3h7.7c.5 0 .9.2 1.3.5l7.3 7.3a1.8 1.8 0 010 2.5z"/><circle cx="7.8" cy="7.8" r="1.4"/>',
  layers: '<path d="M12 3l9 5-9 5-9-5 9-5z"/><path d="M3 12.5l9 5 9-5M3 17l9 5 9-5"/>',
  clock: '<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/>',
  download: '<path d="M12 4v11M7.5 10.5L12 15l4.5-4.5M4.5 19.5h15"/>',
  alert: '<path d="M12 8.5v5M12 17h.01"/><circle cx="12" cy="12" r="8.5"/>',
  sparkles: '<path d="M12 3.5l1.6 4.4 4.4 1.6-4.4 1.6L12 15.5l-1.6-4.4L6 9.5l4.4-1.6L12 3.5z"/><path d="M18.5 15l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8.8-2.2z"/>',
  loader: '<path d="M12 3.5v4M12 16.5v4M3.5 12h4M16.5 12h4M6 6l2.8 2.8M15.2 15.2L18 18M18 6l-2.8 2.8M8.8 15.2L6 18"/>',
  filter: '<path d="M4 5.5h16l-6.2 7.3V19l-3.6 1.8v-8L4 5.5z"/>',
  eye: '<path d="M2.5 12S6 5.8 12 5.8 21.5 12 21.5 12 18 18.2 12 18.2 2.5 12 2.5 12z"/><circle cx="12" cy="12" r="2.8"/>',
  home: '<path d="M3.5 10.5L12 3.5l8.5 7M5.5 9.5V20h13V9.5"/>',
}
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    :stroke-width="stroke"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    v-html="paths[name] || ''"
  />
</template>
