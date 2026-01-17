<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { tasksApi, type TaskItem, type TaskListResponse } from '@/api'
import { ElMessage } from 'element-plus'

const loading = ref(false)
const tasks = ref<TaskItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)

const filters = ref({
  q: '',
  site: '',
  downloaded: false,
  pushed: false,
  expired: false
})

// 站点列表（从任务中提取）
const siteOptions = computed(() => {
  const sites = new Set<string>()
  tasks.value.forEach(t => {
    if (t.siteName) sites.add(t.siteName)
  })
  return Array.from(sites)
})

onMounted(async () => {
  await loadTasks()
})

async function loadTasks() {
  loading.value = true
  try {
    const params = new URLSearchParams()
    params.set('page', page.value.toString())
    params.set('page_size', pageSize.value.toString())
    if (filters.value.q) params.set('q', filters.value.q)
    if (filters.value.site) params.set('site', filters.value.site)
    if (filters.value.downloaded) params.set('downloaded', '1')
    if (filters.value.pushed) params.set('pushed', '1')
    if (filters.value.expired) params.set('expired', '1')

    const data: TaskListResponse = await tasksApi.list(params)
    tasks.value = data.items || []
    total.value = data.total || 0
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  loadTasks()
}

function clearFilters() {
  filters.value = { q: '', site: '', downloaded: false, pushed: false, expired: false }
  page.value = 1
  loadTasks()
}

function handlePageChange(newPage: number) {
  page.value = newPage
  loadTasks()
}

function handleSizeChange(newSize: number) {
  pageSize.value = newSize
  page.value = 1
  loadTasks()
}

function formatTime(timeStr: string): string {
  if (!timeStr || timeStr === '0001-01-01T00:00:00Z') return '-'
  try {
    return new Date(timeStr).toLocaleString('zh-CN')
  } catch {
    return timeStr
  }
}

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let unitIndex = 0
  let size = bytes
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex++
  }
  return `${size.toFixed(2)} ${units[unitIndex]}`
}

function getProgressColor(progress: number) {
  if (progress < 30) return '#F56C6C'
  if (progress < 70) return '#E6A23C'
  return '#67C23A'
}

function getDownloadedSize(task: TaskItem): string {
  const downloaded = task.torrentSize * (task.progress / 100)
  return formatSize(downloaded)
}

function formatProgress(progress: number): string {
  return `${progress.toFixed(1)}%`
}

function getStatusType(task: TaskItem): 'success' | 'warning' | 'danger' | 'info' {
  if (task.lastError === '种子已从下载器中删除') return 'info'
  if (task.isExpired) return 'danger'
  if (task.isPushed) return 'success'
  if (task.isDownloaded) return 'warning'
  return 'info'
}

function getStatusText(task: TaskItem): string {
  if (task.lastError === '种子已从下载器中删除') return '已删除'
  if (task.isExpired) return '已过期'
  if (task.isPushed) return '已推送'
  if (task.isDownloaded) return '已下载'
  return '无需处理'
}

function getDiscountTag(task: TaskItem): {
  text: string
  type: 'success' | 'warning' | 'danger' | 'info'
} {
  const level = (task.freeLevel || '').toUpperCase()

  switch (level) {
    case '2XFREE':
    case '_2X_FREE':
      return { text: '2xFree', type: 'success' }
    case 'FREE':
      return { text: 'Free', type: 'success' }
    case 'PERCENT_50':
    case '50%':
      return { text: '50%', type: 'warning' }
    case 'PERCENT_30':
    case '30%':
      return { text: '30%', type: 'warning' }
    case 'PERCENT_70':
    case '70%':
      return { text: '70%', type: 'warning' }
    case '2XUP':
    case '_2X_UP':
      return { text: '2xUp', type: 'info' }
    case '2X50':
    case '_2X_PERCENT_50':
      return { text: '2x50%', type: 'warning' }
    case 'NONE':
    case '':
      return { text: '普通', type: 'info' }
    default:
      if (task.isFree) {
        return { text: 'Free', type: 'success' }
      }
      if (level && level !== 'NONE') {
        return { text: level, type: 'warning' }
      }
      return { text: '普通', type: 'info' }
  }
}
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h1 class="page-title">任务列表</h1>
        <p class="page-subtitle">查看和管理系统后台任务执行记录</p>
      </div>
    </div>

    <!-- 筛选工具栏 -->
    <div class="common-card search-card">
      <div class="common-card-body">
        <el-form :inline="true" :model="filters" class="filter-form">
          <el-form-item label="搜索">
            <el-input
              v-model="filters.q"
              placeholder="标题/Hash"
              clearable
              style="width: 240px"
              @keyup.enter="applyFilters"
            >
              <template #prefix>
                <el-icon><Search /></el-icon>
              </template>
            </el-input>
          </el-form-item>
          <el-form-item label="站点">
            <el-select v-model="filters.site" placeholder="全部站点" clearable style="width: 160px">
              <el-option label="全部站点" value="" />
              <el-option v-for="site in siteOptions" :key="site" :label="site" :value="site" />
            </el-select>
          </el-form-item>
          <el-form-item>
            <el-checkbox v-model="filters.downloaded">已下载</el-checkbox>
            <el-checkbox v-model="filters.pushed">已推送</el-checkbox>
            <el-checkbox v-model="filters.expired">已过期</el-checkbox>
          </el-form-item>
          <el-form-item>
            <el-button type="primary" :loading="loading" @click="applyFilters">筛选</el-button>
            <el-button @click="clearFilters">重置</el-button>
          </el-form-item>
        </el-form>
      </div>
    </div>

    <!-- 任务列表 -->
    <div class="table-card" v-loading="loading">
      <div class="table-card-header">
        <div class="table-card-header-title">
          <span>任务列表</span>
          <el-tag type="info" size="small" effect="plain" style="margin-left: 8px">
            共 {{ total }} 条
          </el-tag>
        </div>
        <div class="table-card-header-actions">
          <el-button type="primary" size="small" plain @click="loadTasks">
            <el-icon class="mr-1"><Refresh /></el-icon>
            刷新
          </el-button>
        </div>
      </div>

      <div class="table-wrapper">
        <el-table
          :data="tasks"
          style="width: 100%"
          class="pt-table"
          :header-cell-style="{ background: 'var(--pt-bg-secondary)', fontWeight: 600 }"
        >
          <el-table-column label="站点" prop="siteName" width="120" align="center">
            <template #default="{ row }">
              <el-tag size="small" type="primary" effect="light">{{ row.siteName || '-' }}</el-tag>
            </template>
          </el-table-column>

          <el-table-column label="优惠" width="90" align="center">
            <template #default="{ row }">
              <el-tag :type="getDiscountTag(row).type" size="small" effect="dark">
                {{ getDiscountTag(row).text }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column label="标题" min-width="300">
            <template #default="{ row }">
              <div class="title-cell">
                <div class="title-main">
                  <span class="title-text">{{ row.title || '-' }}</span>
                </div>
                <div v-if="row.category || row.tag" class="title-meta">
                  <el-tag v-if="row.category" size="small" type="info" effect="plain">
                    {{ row.category }}
                  </el-tag>
                  <el-tag v-if="row.tag" size="small" effect="plain">{{ row.tag }}</el-tag>
                </div>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="Hash" width="140">
            <template #default="{ row }">
              <template v-if="row.torrentHash">
                <el-tooltip :content="row.torrentHash" placement="top">
                  <code class="hash-cell">{{ row.torrentHash.slice(0, 8) }}...</code>
                </el-tooltip>
              </template>
              <span v-else class="text-tertiary">-</span>
            </template>
          </el-table-column>

          <el-table-column label="大小" width="100" align="right">
            <template #default="{ row }">
              <span class="size-text">{{ formatSize(row.torrentSize) }}</span>
            </template>
          </el-table-column>

          <el-table-column label="进度" width="180">
            <template #default="{ row }">
              <div v-if="row.torrentSize > 0" class="progress-container">
                <el-progress
                  :percentage="Math.round(row.progress)"
                  :stroke-width="8"
                  :show-text="false"
                  :color="getProgressColor(row.progress)"
                />
                <div class="progress-info">
                  <span class="progress-detail">
                    {{ getDownloadedSize(row) }} / {{ formatSize(row.torrentSize) }}
                  </span>
                  <span class="progress-pct">{{ formatProgress(row.progress) }}</span>
                </div>
              </div>
              <span v-else class="text-tertiary">-</span>
            </template>
          </el-table-column>

          <el-table-column label="免费结束" width="160">
            <template #default="{ row }">
              <span :class="row.isExpired ? 'text-danger' : 'text-secondary'">
                {{ formatTime(row.freeEndTime) }}
              </span>
            </template>
          </el-table-column>

          <el-table-column label="最后检查" width="160">
            <template #default="{ row }">
              <span class="text-secondary">{{ formatTime(row.lastCheckTime) }}</span>
            </template>
          </el-table-column>

          <el-table-column label="推送时间" width="160">
            <template #default="{ row }">
              <span v-if="row.isPushed" class="text-success">
                {{ formatTime(row.pushTime) }}
              </span>
              <span v-else class="text-tertiary">-</span>
            </template>
          </el-table-column>

          <el-table-column label="状态" width="100" align="center" fixed="right">
            <template #default="{ row }">
              <el-tag :type="getStatusType(row)" size="small" effect="dark">
                {{ getStatusText(row) }}
              </el-tag>
            </template>
          </el-table-column>
        </el-table>

        <!-- 分页 -->
        <div v-if="total > 0" class="pagination-container">
          <el-pagination
            v-model:current-page="page"
            v-model:page-size="pageSize"
            :page-sizes="[10, 20, 50, 100]"
            :total="total"
            layout="total, sizes, prev, pager, next, jumper"
            @size-change="handleSizeChange"
            @current-change="handlePageChange"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@import '@/styles/common-page.css';
@import '@/styles/table-page.css';

.search-card {
  margin-bottom: var(--pt-space-6);
}

.filter-form {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.title-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 4px 0;
}

.title-text {
  font-weight: 600;
  color: var(--pt-text-primary);
  line-height: 1.4;
  /* Allow multi-line */
  white-space: normal;
  word-break: break-word;
}

.title-meta {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.hash-cell {
  font-family: var(--pt-font-mono);
  font-size: 12px;
  background: var(--pt-bg-tertiary);
  padding: 2px 6px;
  border-radius: 4px;
  color: var(--pt-text-secondary);
  cursor: pointer;
  border: 1px solid var(--pt-border-color);
}

.text-danger {
  color: var(--pt-color-danger);
}
.text-success {
  color: var(--pt-color-success);
}
.text-secondary {
  color: var(--pt-text-secondary);
}
.text-tertiary {
  color: var(--pt-text-tertiary);
}

.mr-1 {
  margin-right: 4px;
}

.size-text {
  font-family: var(--pt-font-mono);
  font-size: 12px;
  color: var(--pt-text-secondary);
}

.progress-container {
  padding: 2px 0;
}

.progress-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 4px;
  font-size: 11px;
}

.progress-detail {
  color: var(--pt-text-secondary);
  font-family: var(--pt-font-mono);
  font-variant-numeric: tabular-nums;
}

.progress-pct {
  color: var(--pt-text-primary);
  font-weight: 600;
  font-family: var(--pt-font-mono);
}
</style>
