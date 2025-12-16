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

// 获取状态标签类型
function getStatusType(task: TaskItem): 'success' | 'warning' | 'danger' | 'info' {
  if (task.isExpired) return 'danger'
  if (task.isPushed) return 'success'
  if (task.isDownloaded) return 'warning'
  return 'info'
}

function getStatusText(task: TaskItem): string {
  if (task.isExpired) return '已过期'
  if (task.isPushed) return '已推送'
  if (task.isDownloaded) return '已下载'
  return '无需处理'
}
</script>

<template>
  <div class="page-container">
    <!-- 筛选工具栏 -->
    <el-card shadow="never" class="filter-card">
      <el-form :inline="true" :model="filters" class="filter-form">
        <el-form-item label="搜索">
          <el-input
            v-model="filters.q"
            placeholder="标题/Hash"
            clearable
            style="width: 200px"
            @keyup.enter="applyFilters"
          >
            <template #prefix>
              <el-icon><Search /></el-icon>
            </template>
          </el-input>
        </el-form-item>
        <el-form-item label="站点">
          <el-select v-model="filters.site" placeholder="全部站点" clearable style="width: 140px">
            <el-option label="全部站点" value="" />
            <el-option v-for="site in siteOptions" :key="site" :label="site" :value="site" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="filters.downloaded">已下载</el-checkbox>
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="filters.pushed">已推送</el-checkbox>
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="filters.expired">已过期</el-checkbox>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :icon="'Search'" @click="applyFilters">筛选</el-button>
          <el-button :icon="'Refresh'" @click="clearFilters">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 任务列表 -->
    <el-card v-loading="loading" shadow="never" class="task-card">
      <template #header>
        <div class="card-header">
          <div class="header-left">
            <span>任务列表</span>
            <el-tag type="info" size="small" style="margin-left: 8px">共 {{ total }} 条</el-tag>
          </div>
          <el-button type="primary" :icon="'Refresh'" :loading="loading" @click="loadTasks">
            刷新
          </el-button>
        </div>
      </template>

      <!-- 表格 -->
      <el-table
        :data="tasks"
        style="width: 100%"
        stripe
        border
        :header-cell-style="{ background: 'var(--el-fill-color-light)', fontWeight: 600 }"
      >
        <el-table-column label="站点" prop="siteName" width="100" align="center">
          <template #default="{ row }">
            <el-tag size="small" type="primary">{{ row.siteName || '-' }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column label="标题" min-width="280">
          <template #default="{ row }">
            <div class="title-cell">
              <el-tooltip :content="row.title" placement="top" :show-after="500">
                <span class="title-text">{{ row.title || '-' }}</span>
              </el-tooltip>
              <div v-if="row.category || row.tag" class="title-meta">
                <el-tag v-if="row.category" size="small" type="info">{{ row.category }}</el-tag>
                <el-tag v-if="row.tag" size="small">{{ row.tag }}</el-tag>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="Hash" width="140">
          <template #default="{ row }">
            <template v-if="row.torrentHash">
              <el-tooltip :content="row.torrentHash" placement="top">
                <code class="hash-cell">{{ row.torrentHash.slice(0, 10) }}...</code>
              </el-tooltip>
            </template>
            <span v-else>-</span>
          </template>
        </el-table-column>

        <el-table-column label="免费结束" width="160">
          <template #default="{ row }">
            <span :class="{ 'text-danger': row.isExpired }">
              {{ formatTime(row.freeEndTime) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="最后检查" width="160">
          <template #default="{ row }">
            {{ formatTime(row.lastCheckTime) }}
          </template>
        </el-table-column>

        <el-table-column label="推送时间" width="160">
          <template #default="{ row }">
            <span v-if="row.isPushed" class="text-success">
              {{ formatTime(row.pushTime) }}
            </span>
            <span v-else>-</span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row)" size="small">
              {{ getStatusText(row) }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <div class="pagination-container">
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
    </el-card>
  </div>
</template>

<style scoped>
.page-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.filter-card :deep(.el-card__body) {
  padding-bottom: 2px;
}

.filter-form {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
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

.title-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.title-text {
  display: block;
  max-width: 280px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}

.title-meta {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.hash-cell {
  font-family: 'Consolas', 'Monaco', monospace;
  font-size: 12px;
  background: var(--el-fill-color-light);
  padding: 2px 6px;
  border-radius: 4px;
  cursor: pointer;
}

.text-danger {
  color: var(--el-color-danger);
}

.text-success {
  color: var(--el-color-success);
}

.pagination-container {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}

/* 暗色模式适配 */
html.dark .hash-cell {
  background: var(--el-fill-color-darker);
}
</style>
