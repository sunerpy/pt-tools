<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { logsApi, type LogsResponse } from '@/api'
import { ElMessage } from 'element-plus'

const loading = ref(false)
const logs = ref<string[]>([])
const logPath = ref('')
const truncated = ref(false)
const logContainer = ref<HTMLElement | null>(null)
const autoScroll = ref(true)

// ANSI 颜色代码映射
const ansiColors: Record<string, string> = {
  '30': '#000000', // 黑色
  '31': '#e74c3c', // 红色
  '32': '#2ecc71', // 绿色
  '33': '#f39c12', // 黄色
  '34': '#3498db', // 蓝色
  '35': '#9b59b6', // 紫色
  '36': '#1abc9c', // 青色
  '37': '#ecf0f1', // 白色
  '90': '#7f8c8d', // 亮黑色（灰色）
  '91': '#e74c3c', // 亮红色
  '92': '#2ecc71', // 亮绿色
  '93': '#f1c40f', // 亮黄色
  '94': '#3498db', // 亮蓝色
  '95': '#9b59b6', // 亮紫色
  '96': '#1abc9c', // 亮青色
  '97': '#ffffff' // 亮白色
}

// 解析 ANSI 转义序列并转换为 HTML
function parseAnsi(text: string): string {
  // 匹配 ANSI 转义序列
  const ansiRegex = /\u001b\[(\d+)m/g
  let result = ''
  let lastIndex = 0
  let currentColor: string | null = null
  let match: RegExpExecArray | null

  while ((match = ansiRegex.exec(text)) !== null) {
    // 添加匹配前的文本
    if (match.index > lastIndex) {
      const textBefore = text.slice(lastIndex, match.index)
      result += escapeHtml(textBefore)
    }

    const code = match[1]
    if (code === '0') {
      // 重置
      if (currentColor) {
        result += '</span>'
        currentColor = null
      }
    } else if (code && ansiColors[code]) {
      // 关闭之前的颜色
      if (currentColor) {
        result += '</span>'
      }
      currentColor = ansiColors[code]
      result += `<span style="color: ${currentColor}">`
    }

    lastIndex = match.index + match[0].length
  }

  // 添加剩余文本
  if (lastIndex < text.length) {
    result += escapeHtml(text.slice(lastIndex))
  }

  // 关闭未关闭的标签
  if (currentColor) {
    result += '</span>'
  }

  return result
}

// HTML 转义
function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;')
}

// 计算渲染后的日志 HTML
const renderedLogs = computed(() => {
  return logs.value.map(line => parseAnsi(line)).join('\n')
})

onMounted(async () => {
  await loadLogs()
})

async function loadLogs() {
  loading.value = true
  try {
    const data: LogsResponse = await logsApi.get()
    logs.value = data.lines || []
    logPath.value = data.path || ''
    truncated.value = data.truncated || false

    // 自动滚动到底部
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
  <div class="page-container">
    <el-card v-loading="loading" shadow="never" class="log-card">
      <template #header>
        <div class="card-header">
          <div class="header-left">
            <span>日志查看</span>
            <el-tag v-if="truncated" type="warning" size="small" style="margin-left: 8px">
              已截断（最近 5000 行）
            </el-tag>
            <el-tag type="info" size="small" style="margin-left: 8px">{{ logs.length }} 行</el-tag>
          </div>
          <div class="header-right">
            <el-checkbox v-model="autoScroll" label="自动滚动" size="small" />
            <el-button-group>
              <el-button size="small" :icon="'Top'" @click="scrollToTop">顶部</el-button>
              <el-button size="small" :icon="'Bottom'" @click="scrollToBottom">底部</el-button>
            </el-button-group>
            <el-button type="primary" :icon="'Refresh'" :loading="loading" @click="loadLogs">
              刷新
            </el-button>
          </div>
        </div>
      </template>

      <el-alert
        v-if="logPath"
        :title="`日志路径: ${logPath}`"
        type="info"
        :closable="false"
        show-icon
        style="margin-bottom: 16px"
      />

      <div ref="logContainer" class="log-container terminal">
        <pre class="log-content"><code v-html="renderedLogs || '暂无日志'"></code></pre>
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.page-container {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.log-card {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.log-card :deep(.el-card__body) {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 16px;
  font-weight: 600;
}

.header-left {
  display: flex;
  align-items: center;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.log-container {
  flex: 1;
  min-height: 400px;
  max-height: calc(100vh - 300px);
  overflow: auto;
  border-radius: 8px;
}

/* 终端样式 */
.terminal {
  background: #1e1e1e;
  padding: 16px;
  border: 1px solid #333;
}

.log-content {
  margin: 0;
  font-family: 'Consolas', 'Monaco', 'Courier New', 'Menlo', monospace;
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  color: #d4d4d4;
}

.log-content code {
  background: transparent;
  padding: 0;
  font-family: inherit;
}

/* 滚动条样式 */
.terminal::-webkit-scrollbar {
  width: 10px;
  height: 10px;
}

.terminal::-webkit-scrollbar-track {
  background: #2d2d2d;
}

.terminal::-webkit-scrollbar-thumb {
  background: #555;
  border-radius: 5px;
}

.terminal::-webkit-scrollbar-thumb:hover {
  background: #666;
}
</style>
