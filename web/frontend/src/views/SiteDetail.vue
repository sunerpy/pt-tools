<script setup lang="ts">
import { ref, onMounted, computed, reactive } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { sitesApi, type SiteConfig, type RSSConfig } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'

const route = useRoute()
const router = useRouter()

const siteName = computed(() => route.params.name as string)
const loading = ref(false)
const saving = ref(false)
const addingRss = ref(false)
const rssDialogVisible = ref(false)

const form = ref<SiteConfig>({
  enabled: false,
  auth_method: 'cookie',
  cookie: '',
  api_key: '',
  api_url: '',
  rss: []
})

const newRss = reactive<RSSConfig>({
  name: '',
  url: '',
  category: '',
  tag: '',
  interval_minutes: 10
})

onMounted(async () => {
  loading.value = true
  try {
    form.value = await sitesApi.get(siteName.value)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
})

async function save() {
  saving.value = true
  try {
    await sitesApi.save(siteName.value, form.value)
    ElMessage.success('保存成功')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}

function openAddRssDialog() {
  Object.assign(newRss, { name: '', url: '', category: '', tag: '', interval_minutes: 10 })
  rssDialogVisible.value = true
}

async function addRss() {
  if (!newRss.name || !newRss.url) {
    ElMessage.error('名称和链接为必填')
    return
  }
  if (!newRss.url.startsWith('http://') && !newRss.url.startsWith('https://')) {
    ElMessage.error('链接必须以 http:// 或 https:// 开头')
    return
  }

  // 检查重复 RSS URL
  const normalizedUrl = newRss.url.trim().toLowerCase()
  const isDuplicate = form.value.rss.some(r => r.url.trim().toLowerCase() === normalizedUrl)
  if (isDuplicate) {
    ElMessage.error('该 RSS 链接已存在，请勿重复添加')
    return
  }

  addingRss.value = true
  console.log('[RSS] 开始添加 RSS:', newRss.name, newRss.url)
  try {
    form.value.rss.push({
      ...newRss,
      interval_minutes: Math.max(5, Math.min(1440, newRss.interval_minutes || 10))
    })
    await sitesApi.save(siteName.value, form.value)
    // 重新加载数据以获取数据库中的真实 ID
    form.value = await sitesApi.get(siteName.value)
    ElMessage.success('RSS 添加成功')
    rssDialogVisible.value = false
  } catch (e: unknown) {
    // 添加失败时，移除刚添加的 RSS
    form.value.rss.pop()
    ElMessage.error((e as Error).message || '添加失败')
  } finally {
    addingRss.value = false
  }
}

async function deleteRss(index: number) {
  const rss = form.value.rss[index]
  if (!rss) return

  try {
    await ElMessageBox.confirm(`确定删除 RSS "${rss.name}"？`, '确认删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })

    console.log('[RSS] 开始删除 RSS:', rss.name, 'id:', rss.id)
    if (rss.id) {
      await sitesApi.deleteRss(siteName.value, rss.id)
      console.log('[RSS] 删除 RSS 成功:', rss.name)
      // 重新加载数据以确保数据一致性
      form.value = await sitesApi.get(siteName.value)
    } else {
      // 没有 ID 的 RSS（未保存到数据库），直接从前端列表移除
      console.log('[RSS] RSS 无 ID，仅从前端移除:', rss.name)
      form.value.rss.splice(index, 1)
    }
    ElMessage.success('已删除')
  } catch (e: unknown) {
    if ((e as string) !== 'cancel') {
      console.error('[RSS] 删除 RSS 失败:', e)
      ElMessage.error((e as Error).message || '删除失败')
    }
  }
}

function goBack() {
  router.push('/sites')
}
</script>

<template>
  <div class="page-container">
    <!-- 站点配置 -->
    <el-card v-loading="loading" shadow="never">
      <template #header>
        <div class="card-header">
          <div class="header-left">
            <el-button :icon="'ArrowLeft'" text @click="goBack" />
            <span>站点设置 - {{ siteName }}</span>
            <el-tag :type="form.enabled ? 'success' : 'info'" size="small" style="margin-left: 8px">
              {{ form.enabled ? '已启用' : '未启用' }}
            </el-tag>
          </div>
          <el-button type="primary" :loading="saving" @click="save">保存配置</el-button>
        </div>
      </template>

      <el-form :model="form" label-width="100px" label-position="right">
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="启用">
              <el-switch v-model="form.enabled" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="认证方式">
              <el-tag type="warning">
                {{ form.auth_method === 'api_key' ? 'API Key' : 'Cookie' }}
              </el-tag>
            </el-form-item>
          </el-col>
        </el-row>

        <el-divider />

        <el-form-item v-if="form.auth_method === 'cookie'" label="Cookie">
          <el-input
            v-model="form.cookie"
            type="textarea"
            :rows="3"
            placeholder="从浏览器开发者工具中获取"
          />
        </el-form-item>

        <el-form-item v-if="form.auth_method === 'api_key'" label="API Key">
          <el-input
            v-model="form.api_key"
            type="password"
            show-password
            placeholder="从 M-Team 个人设置中获取"
          />
        </el-form-item>

        <el-form-item v-if="form.auth_method === 'api_key'" label="API URL">
          <el-input :model-value="form.api_url" disabled />
        </el-form-item>
      </el-form>
    </el-card>

    <!-- RSS 订阅列表 -->
    <el-card shadow="never" style="margin-top: 16px">
      <template #header>
        <div class="card-header">
          <span>RSS 订阅</span>
          <el-button type="primary" :icon="'Plus'" @click="openAddRssDialog">添加 RSS</el-button>
        </div>
      </template>

      <el-table :data="form.rss" style="width: 100%">
        <el-table-column label="名称" prop="name" width="150" />
        <el-table-column label="链接" min-width="200">
          <template #default="{ row }">
            <el-tooltip :content="row.url" placement="top">
              <span class="url-cell">{{ row.url }}</span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="分类" prop="category" width="100" />
        <el-table-column label="标签" prop="tag" width="100" />
        <el-table-column label="间隔(分钟)" prop="interval_minutes" width="100" />
        <el-table-column label="操作" width="100">
          <template #default="{ $index }">
            <el-button type="danger" size="small" :icon="'Delete'" @click="deleteRss($index)">
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="!form.rss.length" description="暂无 RSS 订阅" />
    </el-card>

    <!-- 添加 RSS 对话框 -->
    <el-dialog v-model="rssDialogVisible" title="添加 RSS 订阅" width="500px">
      <el-form :model="newRss" label-width="100px">
        <el-form-item label="名称" required>
          <el-input v-model="newRss.name" placeholder="如：CMCT电视剧" />
        </el-form-item>
        <el-form-item label="链接" required>
          <el-input v-model="newRss.url" placeholder="https://..." />
        </el-form-item>
        <el-form-item label="分类">
          <el-input v-model="newRss.category" placeholder="Tv" />
        </el-form-item>
        <el-form-item label="标签">
          <el-input v-model="newRss.tag" placeholder="CMCT" />
        </el-form-item>
        <el-form-item label="间隔(分钟)">
          <el-input-number v-model="newRss.interval_minutes" :min="5" :max="1440" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="rssDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="addingRss" @click="addRss">添加</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-container {
  width: 100%;
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
  gap: 8px;
}

.url-cell {
  display: block;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
