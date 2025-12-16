<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { sitesApi, type SiteConfig } from '@/api'
import { ElMessage, ElMessageBox } from 'element-plus'

const router = useRouter()

const loading = ref(false)
const sites = ref<Record<string, SiteConfig>>({})

onMounted(async () => {
  await loadSites()
})

async function loadSites() {
  loading.value = true
  try {
    sites.value = await sitesApi.list()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function toggleEnabled(name: string) {
  const site = sites.value[name]
  if (!site) return
  site.enabled = !site.enabled
  try {
    await sitesApi.save(name, site)
    ElMessage.success('已保存')
  } catch (e: unknown) {
    site.enabled = !site.enabled
    ElMessage.error((e as Error).message || '保存失败')
  }
}

async function deleteSite(name: string) {
  if (['cmct', 'hdsky', 'mteam'].includes(name.toLowerCase())) {
    ElMessage.warning('预置站点不可删除')
    return
  }

  try {
    await ElMessageBox.confirm(`确定删除站点 "${name}"？`, '确认删除', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await sitesApi.delete(name)
    ElMessage.success('已删除')
    await loadSites()
  } catch (e: unknown) {
    if ((e as string) !== 'cancel') {
      ElMessage.error((e as Error).message || '删除失败')
    }
  }
}

async function addSite() {
  try {
    const { value: name } = await ElMessageBox.prompt('请输入站点标识', '新增站点', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      inputPlaceholder: 'cmct / hdsky / mteam 或自定义',
      inputValidator: val => {
        if (!val || !val.trim()) return '站点标识不能为空'
        if (sites.value[val.toLowerCase()]) return '站点已存在'
        return true
      }
    })

    if (!name) return

    const lower = name.toLowerCase()
    const payload: SiteConfig = {
      enabled: false,
      rss: [],
      auth_method: lower === 'mteam' ? 'api_key' : 'cookie',
      cookie: '',
      api_key: '',
      api_url: lower === 'mteam' ? 'https://api.m-team.cc/api' : ''
    }

    await sitesApi.save(lower, payload)
    ElMessage.success('已新增站点')
    await loadSites()
  } catch (e: unknown) {
    if ((e as string) !== 'cancel') {
      ElMessage.error((e as Error).message || '新增失败')
    }
  }
}

function manageSite(name: string) {
  router.push(`/sites/${name}`)
}

function getRssCount(site: SiteConfig): number {
  return site.rss?.length || 0
}
</script>

<template>
  <div class="page-container">
    <el-card v-loading="loading" shadow="never">
      <template #header>
        <div class="card-header">
          <span>站点与 RSS</span>
          <el-button type="primary" :icon="'Plus'" @click="addSite">新增站点</el-button>
        </div>
      </template>

      <el-table :data="Object.entries(sites)" style="width: 100%">
        <el-table-column label="站点" width="150">
          <template #default="{ row }">
            <div class="site-name">
              <el-icon><Connection /></el-icon>
              <span>{{ row[0] }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row[1].enabled ? 'success' : 'info'" size="small">
              {{ row[1].enabled ? '已启用' : '未启用' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="认证方式" width="120">
          <template #default="{ row }">
            <el-tag type="warning" size="small" effect="plain">
              {{ row[1].auth_method === 'api_key' ? 'API Key' : 'Cookie' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="RSS 数量" width="100">
          <template #default="{ row }">
            <el-badge
              :value="getRssCount(row[1])"
              :type="getRssCount(row[1]) > 0 ? 'primary' : 'info'"
            />
          </template>
        </el-table-column>

        <el-table-column label="操作" min-width="200">
          <template #default="{ row }">
            <el-space>
              <el-switch
                :model-value="row[1].enabled"
                size="small"
                @change="toggleEnabled(row[0])"
              />
              <el-button type="primary" size="small" @click="manageSite(row[0])">管理</el-button>
              <el-button
                type="danger"
                size="small"
                :disabled="['cmct', 'hdsky', 'mteam'].includes(row[0].toLowerCase())"
                @click="deleteSite(row[0])"
              >
                删除
              </el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>
    </el-card>
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

.site-name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
}
</style>
