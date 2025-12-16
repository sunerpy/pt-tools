<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { globalApi, type GlobalSettings } from '@/api'
import { ElMessage } from 'element-plus'

const loading = ref(false)
const saving = ref(false)
const showWarning = ref(false)

const form = ref<GlobalSettings>({
  default_interval_minutes: 10,
  download_dir: '',
  download_limit_enabled: false,
  download_speed_limit: 20,
  torrent_size_gb: 200,
  auto_start: false
})

onMounted(async () => {
  loading.value = true
  try {
    const data = await globalApi.get()
    form.value = {
      ...data,
      default_interval_minutes: Math.max(5, data.default_interval_minutes || 10)
    }
    showWarning.value = !form.value.download_dir
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
})

async function save() {
  if (!form.value.download_dir) {
    ElMessage.error('下载目录不能为空')
    showWarning.value = true
    return
  }

  saving.value = true
  try {
    await globalApi.save({
      ...form.value,
      default_interval_minutes: Math.max(5, form.value.default_interval_minutes)
    })
    ElMessage.success('保存成功')
    showWarning.value = false
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="page-container">
    <el-alert
      v-if="showWarning"
      title="警告：未设置下载目录，后台任务不会启动，请先设置并保存"
      type="warning"
      show-icon
      :closable="false"
      style="margin-bottom: 20px"
    />

    <el-card v-loading="loading" shadow="never">
      <template #header>
        <div class="card-header">
          <span>全局设置</span>
        </div>
      </template>

      <el-form :model="form" label-width="140px" label-position="right">
        <el-form-item label="默认间隔(分钟)">
          <el-input-number v-model="form.default_interval_minutes" :min="5" :max="1440" :step="1" />
          <div class="form-tip">RSS 检查的默认时间间隔，最小 5 分钟</div>
        </el-form-item>

        <el-form-item label="种子下载目录">
          <el-input v-model="form.download_dir" placeholder="保存 .torrent 种子文件的目录" />
          <div class="form-tip">
            绝对路径将直接使用；相对路径会拼接为
            <code>~/.pt-tools/&lt;输入值&gt;</code>
            并自动创建目录
          </div>
          <div class="form-tip">
            此目录用于保存已下载的
            <code>.torrent</code>
            种子文件；并非 qBittorrent 中文件数据的保存路径
          </div>
        </el-form-item>

        <el-divider />

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="启用限速">
              <el-switch v-model="form.download_limit_enabled" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="下载限速(MB/s)">
              <el-input-number
                v-model="form.download_speed_limit"
                :min="1"
                :max="1000"
                :disabled="!form.download_limit_enabled"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="最大种子大小(GB)">
              <el-input-number v-model="form.torrent_size_gb" :min="1" :max="10000" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="自动启动任务">
              <el-switch v-model="form.auto_start" />
              <div class="form-tip">启动时自动开始 RSS 检查任务</div>
            </el-form-item>
          </el-col>
        </el-row>

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
  font-size: 16px;
  font-weight: 600;
}

.form-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
  line-height: 1.5;
}

.form-tip code {
  background: var(--el-fill-color-light);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: monospace;
}
</style>
