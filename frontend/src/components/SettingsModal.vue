<script setup>
import { onMounted, ref } from 'vue'
import Icon from './Icon.vue'
import { api } from '../api.js'

const props = defineProps({
  settings: { type: Object, required: true },
})
const emit = defineEmits(['close', 'saved', 'toast'])

const form = ref(JSON.parse(JSON.stringify(props.settings)))
const saving = ref(false)
const sources = ref(null)

onMounted(async () => {
  try {
    sources.value = await api.sourceStatus()
  } catch {
    /* ignore */
  }
})

function addTool() {
  form.value.translationTools.push({ name: '', path: '', args: '{folder}', auto: false })
}
function removeTool(i) {
  form.value.translationTools.splice(i, 1)
}
async function pickToolPath(i) {
  try {
    const res = await api.pickFile('选择工具可执行文件', '可执行文件|*.exe')
    if (res.path) form.value.translationTools[i].path = res.path
  } catch (e) {
    emit('toast', { type: 'error', text: e.message })
  }
}

async function save() {
  saving.value = true
  try {
    await api.saveSettings(form.value)
    emit('saved', form.value)
    emit('toast', { type: 'success', text: '设置已保存' })
    emit('close')
  } catch (e) {
    emit('toast', { type: 'error', text: e.message })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <teleport to="body">
    <div class="overlay" @click.self="emit('close')">
      <div class="modal" style="width: min(700px, calc(100vw - 60px))">
        <div class="modal-head">
          <Icon name="settings" :size="17" />
          <h3>设置</h3>
          <button class="modal-close" @click="emit('close')"><Icon name="x" :size="16" /></button>
        </div>

        <div class="modal-body">
          <div class="settings-section">
            <h4>元数据</h4>
            <p>导入时从网络补全简介、评分、标签与封面。</p>

            <div class="setting-row">
              <div>
                <div class="label">首选数据源</div>
                <div class="hint">Bangumi 中文信息更全，VNDB 英文/罗马音命中率更高</div>
              </div>
              <div class="ctrl">
                <select v-model="form.metadataSource" class="select" style="width: 150px">
                  <option value="bangumi">Bangumi</option>
                  <option value="vndb">VNDB</option>
                </select>
              </div>
            </div>

            <div class="setting-row" v-if="sources">
              <div>
                <div class="label">数据源连通性</div>
                <div class="hint">不可达的源会被自动跳过，回退到另一个源</div>
              </div>
              <div class="ctrl">
                <span class="tag" :style="{ color: sources.bangumi ? 'var(--success)' : 'var(--danger)' }">
                  Bangumi {{ sources.bangumi ? '可用' : '不可达' }}
                </span>
                <span class="tag" :style="{ color: sources.vndb ? 'var(--success)' : 'var(--danger)' }">
                  VNDB {{ sources.vndb ? '可用' : '不可达' }}
                </span>
              </div>
            </div>

            <div class="setting-row">
              <div>
                <div class="label">导入时自动抓取</div>
                <div class="hint">关闭后扫描会更快，可稍后在详情里手动刷新</div>
              </div>
              <div class="ctrl">
                <label class="checkbox"><input v-model="form.autoFetchMeta" type="checkbox" /></label>
              </div>
            </div>

            <div class="setting-row">
              <div>
                <div class="label">默认递归扫描子目录</div>
                <div class="hint">适用于「根目录 / 系列 / 游戏」这种多层结构</div>
              </div>
              <div class="ctrl">
                <label class="checkbox"><input v-model="form.scanRecursive" type="checkbox" /></label>
              </div>
            </div>
          </div>

          <div class="settings-section">
            <h4>界面</h4>
            <p>封面卡片尺寸，也可以随时在顶栏切换。</p>
            <div class="setting-row">
              <div class="label">网格密度</div>
              <div class="ctrl">
                <select v-model="form.gridSize" class="select" style="width: 150px">
                  <option value="sm">紧凑</option>
                  <option value="md">标准</option>
                  <option value="lg">宽松</option>
                </select>
              </div>
            </div>
          </div>

          <div class="settings-section">
            <h4>窗口模式参数</h4>
            <p>启动时按游戏设置的窗口模式追加对应参数，留空则不追加。</p>
            <div class="setting-row">
              <div class="label">窗口化</div>
              <div class="ctrl">
                <input v-model="form.windowFlags.windowed" class="input" style="width: 240px" placeholder="例如 -windowed" />
              </div>
            </div>
            <div class="setting-row">
              <div class="label">全屏</div>
              <div class="ctrl">
                <input v-model="form.windowFlags.fullscreen" class="input" style="width: 240px" placeholder="例如 -fullscreen" />
              </div>
            </div>
            <div class="setting-row">
              <div class="label">无边框</div>
              <div class="ctrl">
                <input v-model="form.windowFlags.borderless" class="input" style="width: 240px" placeholder="例如 -popupwindow" />
              </div>
            </div>
          </div>

          <div class="settings-section">
            <h4>翻译工具</h4>
            <p>
              可配置 Textractor、LunaTranslator、Misaka Translator 等。参数支持占位符
              <code style="color: var(--accent-2)">{folder}</code>、<code style="color: var(--accent-2)">{exe}</code>、<code style="color: var(--accent-2)">{title}</code>。
            </p>

            <div
              v-for="(t, i) in form.translationTools"
              :key="i"
              class="tool-row"
            >
              <input v-model="t.name" class="input" placeholder="名称" />
              <input v-model="t.path" class="input" placeholder="可执行文件路径" />
              <input v-model="t.args" class="input" placeholder="参数" />
              <label class="checkbox" title="启动游戏时自动启动">
                <input v-model="t.auto" type="checkbox" />
              </label>
              <button class="btn btn-sm btn-ghost" @click="removeTool(i)">
                <Icon name="trash" :size="13" />
              </button>
            </div>

            <div style="display: flex; gap: 8px; margin-top: 8px; align-items: center">
              <button class="btn btn-sm" @click="addTool"><Icon name="plus" :size="12" /> 添加工具</button>
              <label class="checkbox" style="margin-left: auto">
                <input v-model="form.autoStartTools" type="checkbox" />
                启动游戏时自动启动标记的工具
              </label>
            </div>
          </div>

          <div class="settings-section" style="margin-bottom: 0">
            <h4>数据位置</h4>
            <p>
              所有数据保存在本机 <code style="color: var(--text-dim)">%APPDATA%\GalgameManager</code>
              目录下，包含 SQLite 数据库与封面缓存，可直接备份。
            </p>
          </div>
        </div>

        <div class="modal-foot">
          <button class="btn" @click="emit('close')">取消</button>
          <div style="flex: 1" />
          <button class="btn btn-primary" :disabled="saving" @click="save">
            {{ saving ? '保存中…' : '保存设置' }}
          </button>
        </div>
      </div>
    </div>
  </teleport>
</template>
