<script setup lang="ts">
import { ref, onMounted, nextTick, shallowRef } from 'vue'
import { logsApi, type LogsResponse } from '@/api'
import { ElMessage } from 'element-plus'
import { Refresh, Top, Bottom } from '@element-plus/icons-vue'

const loading = ref(false)
const logs = shallowRef<string[]>([])
const logPath = ref('')
const truncated = ref(false)
const logContainer = ref<HTMLElement | null>(null)
const autoScroll = ref(true)
const renderedHtml = shallowRef('')

function escapeHtml(text: string): string {
  return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function getLevelClass(level: string): string {
  switch (level?.toLowerCase()) {
    case 'debug':
      return 'log-debug'
    case 'info':
      return 'log-info'
    case 'warn':
    case 'warning':
      return 'log-warn'
    case 'error':
      return 'log-error'
    case 'fatal':
    case 'panic':
      return 'log-fatal'
    default:
      return 'json-string'
  }
}

function formatValue(value: unknown, key?: string): string {
  if (value === null) {
    return '<span class="json-null">null</span>'
  }
  if (typeof value === 'boolean') {
    return `<span class="json-boolean">${value}</span>`
  }
  if (typeof value === 'number') {
    return `<span class="json-number">${value}</span>`
  }
  if (typeof value === 'string') {
    const escaped = escapeHtml(value)
    if (key === 'level') {
      return `"<span class="${getLevelClass(value)}">${escaped}</span>"`
    }
    if (key === 'time') {
      return `"<span class="json-time">${escaped}</span>"`
    }
    if (key === 'msg') {
      return `"<span class="json-msg">${escaped}</span>"`
    }
    return `"<span class="json-string">${escaped}</span>"`
  }
  if (Array.isArray(value)) {
    const items = value.map(v => formatValue(v)).join('<span class="json-punct">,</span> ')
    return `<span class="json-punct">[</span>${items}<span class="json-punct">]</span>`
  }
  if (typeof value === 'object') {
    return formatObject(value as Record<string, unknown>)
  }
  return escapeHtml(String(value))
}

function formatObject(obj: Record<string, unknown>): string {
  const entries = Object.entries(obj)
  if (entries.length === 0) {
    return '<span class="json-punct">{}</span>'
  }
  const parts = entries.map(([k, v]) => {
    return `"<span class="json-key">${escapeHtml(k)}</span>": ${formatValue(v, k)}`
  })
  return `<span class="json-punct">{</span>${parts.join('<span class="json-punct">,</span> ')}<span class="json-punct">}</span>`
}

function highlightLine(line: string): string {
  if (!line.trim()) return ''
  try {
    const obj = JSON.parse(line)
    return formatObject(obj)
  } catch {
    return escapeHtml(line)
  }
}

function processLogs(lines: string[]): string {
  return lines.map(highlightLine).join('\n')
}

onMounted(async () => {
  await loadLogs()
})

async function loadLogs() {
  loading.value = true
  try {
    const data: LogsResponse = await logsApi.get()
    const lines = data.lines || []
    logs.value = lines
    logPath.value = data.path || ''
    truncated.value = data.truncated || false

    await nextTick()
    renderedHtml.value = processLogs(lines)

    if (autoScroll.value) {
      await nextTick()
      scrollToBottom()
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

function scrollToBottom() {
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
}

function scrollToTop() {
  if (logContainer.value) {
    logContainer.value.scrollTop = 0
  }
}
</script>

<template>
  <div class="page-container log-viewer-page">
    <div class="page-header">
      <div class="header-left">
        <h1 class="page-title">日志查看</h1>
        <div class="page-subtitle" style="display: flex; gap: 8px; align-items: center">
          <el-tag
            v-if="truncated"
            type="warning"
            size="small"
            effect="plain"
            class="status-badge status-badge--warning"
          >
            已截断（最近 5000 行）
          </el-tag>
          <el-tag type="info" size="small" effect="plain" class="status-badge status-badge--info">
            {{ logs.length }} 行
          </el-tag>
          <span v-if="logPath" class="log-path-text">{{ logPath }}</span>
        </div>
      </div>
      <div class="page-actions">
        <el-checkbox v-model="autoScroll" label="自动滚动" size="default" />
        <el-button-group>
          <el-button :icon="Top" @click="scrollToTop">顶部</el-button>
          <el-button :icon="Bottom" @click="scrollToBottom">底部</el-button>
        </el-button-group>
        <el-button type="primary" :icon="Refresh" :loading="loading" @click="loadLogs">
          刷新
        </el-button>
      </div>
    </div>

    <div class="log-card">
      <div ref="logContainer" class="log-container">
        <pre class="log-content"><code v-html="renderedHtml || '暂无日志'"></code></pre>
      </div>
    </div>
  </div>
</template>

<style scoped>
.log-viewer-page {
  height: calc(100vh - 120px);
  display: flex;
  flex-direction: column;
}

.log-card {
  flex: 1;
  min-height: 0;
  background: var(--pt-bg-surface);
  border: 1px solid var(--pt-border-color);
  border-radius: var(--pt-radius-xl);
  box-shadow: var(--pt-shadow-sm);
  overflow: hidden;
}

.log-path-text {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: var(--pt-text-xs);
  color: var(--pt-text-tertiary);
  margin-left: var(--pt-space-2);
}

.log-container {
  height: 100%;
  overflow: auto;
  padding: 16px 20px;
  background-color: #fafbfc;
  color: #24292f;
}

.log-content {
  margin: 0;
  font-family: 'JetBrains Mono', 'Fira Code', 'SF Mono', 'Cascadia Code', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.8;
  white-space: pre-wrap;
  word-break: break-all;
}

.log-content code {
  background: transparent;
  padding: 0;
  font-family: inherit;
  border: none;
  color: inherit;
}

.log-container::-webkit-scrollbar {
  width: 10px;
  height: 10px;
}

.log-container::-webkit-scrollbar-track {
  background: #f6f8fa;
}

.log-container::-webkit-scrollbar-thumb {
  background-color: #d0d7de;
  border-radius: 5px;
  border: 2px solid #f6f8fa;
}

.log-container::-webkit-scrollbar-thumb:hover {
  background-color: #afb8c1;
}

@media (max-width: 768px) {
  .log-viewer-page {
    height: auto;
  }
  .log-container {
    height: 60vh;
  }
  .log-content {
    font-size: 12px;
  }
}
</style>

<style>
/* Light theme - JSON syntax */
.json-key {
  color: #0550ae;
}
.json-string {
  color: #0a3069;
}
.json-number {
  color: #0550ae;
}
.json-boolean {
  color: #cf222e;
}
.json-null {
  color: #6e7781;
  font-style: italic;
}
.json-punct {
  color: #6e7781;
}
.json-time {
  color: #8250df;
}
.json-msg {
  color: #24292f;
}

/* Light theme - Log levels */
.log-debug {
  color: #6e7781;
}
.log-info {
  color: #1a7f37;
  font-weight: 600;
}
.log-warn {
  color: #9a6700;
  font-weight: 600;
}
.log-error {
  color: #cf222e;
  font-weight: 600;
}
.log-fatal {
  color: #fff;
  background: #cf222e;
  padding: 1px 6px;
  border-radius: 3px;
  font-weight: 700;
}

/* Dark theme - Container */
html.dark .log-container {
  background-color: #1a1b26;
  color: #a9b1d6;
}

/* Dark theme - JSON syntax (Tokyo Night Storm palette) */
html.dark .json-key {
  color: #73daca;
}
html.dark .json-string {
  color: #9ece6a;
}
html.dark .json-number {
  color: #ff9e64;
}
html.dark .json-boolean {
  color: #f7768e;
}
html.dark .json-null {
  color: #565f89;
  font-style: italic;
}
html.dark .json-punct {
  color: #565f89;
}
html.dark .json-time {
  color: #7dcfff;
}
html.dark .json-msg {
  color: #c0caf5;
}

/* Dark theme - Log levels */
html.dark .log-debug {
  color: #565f89;
}
html.dark .log-info {
  color: #9ece6a;
  font-weight: 600;
}
html.dark .log-warn {
  color: #e0af68;
  font-weight: 600;
}
html.dark .log-error {
  color: #f7768e;
  font-weight: 600;
}
html.dark .log-fatal {
  color: #1a1b26;
  background: #f7768e;
}

/* Dark theme - Scrollbar */
html.dark .log-container::-webkit-scrollbar-track {
  background: #16161e;
}
html.dark .log-container::-webkit-scrollbar-thumb {
  background-color: #3b4261;
  border-color: #16161e;
}
html.dark .log-container::-webkit-scrollbar-thumb:hover {
  background-color: #565f89;
}
</style>
