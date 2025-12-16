<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { qbitApi, type QbitSettings } from '@/api'
import { ElMessage } from 'element-plus'

const loading = ref(false)
const saving = ref(false)

const form = ref<QbitSettings>({
  enabled: false,
  url: '',
  user: '',
  password: ''
})

onMounted(async () => {
  loading.value = true
  try {
    form.value = await qbitApi.get()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
})

async function save() {
  if (form.value.enabled && (!form.value.url || !form.value.user || !form.value.password)) {
    ElMessage.error('启用时 URL、用户名、密码均为必填')
    return
  }

  saving.value = true
  try {
    await qbitApi.save(form.value)
    ElMessage.success('保存成功')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="page-container">
    <el-card v-loading="loading" shadow="never">
      <template #header>
        <div class="card-header">
          <span>qBittorrent 设置</span>
          <el-tag :type="form.enabled ? 'success' : 'info'" size="small">
            {{ form.enabled ? '已启用' : '未启用' }}
          </el-tag>
        </div>
      </template>

      <el-form :model="form" label-width="100px" label-position="right">
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
          <div class="form-tip">启用后将自动推送种子到 qBittorrent</div>
        </el-form-item>

        <el-divider />

        <el-form-item label="URL">
          <el-input
            v-model="form.url"
            placeholder="http://192.168.1.10:8080"
            :disabled="!form.enabled"
          >
            <template #prepend>
              <el-icon><Link /></el-icon>
            </template>
          </el-input>
          <div class="form-tip">qBittorrent Web UI 地址</div>
        </el-form-item>

        <el-form-item label="用户名">
          <el-input v-model="form.user" placeholder="admin" :disabled="!form.enabled">
            <template #prepend>
              <el-icon><User /></el-icon>
            </template>
          </el-input>
        </el-form-item>

        <el-form-item label="密码">
          <el-input
            v-model="form.password"
            type="password"
            show-password
            placeholder="请输入密码"
            :disabled="!form.enabled"
          >
            <template #prepend>
              <el-icon><Lock /></el-icon>
            </template>
          </el-input>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" :loading="saving" @click="save">保存设置</el-button>
        </el-form-item>
      </el-form>
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

.form-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}
</style>
