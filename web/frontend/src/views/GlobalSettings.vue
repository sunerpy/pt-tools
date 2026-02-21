<script setup lang="ts">
import { globalApi, type GlobalSettings } from "@/api";
import { Folder, Setting, Timer } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { onMounted, ref } from "vue";

const loading = ref(false);
const saving = ref(false);
const showWarning = ref(false);

const form = ref<GlobalSettings>({
  default_interval_minutes: 10,
  download_dir: "",
  download_limit_enabled: false,
  download_speed_limit: 20,
  torrent_size_gb: 200,
  min_free_minutes: 30,
  auto_start: false,
  auto_delete_on_free_end: false,
});

onMounted(async () => {
  loading.value = true;
  try {
    const data = await globalApi.get();
    form.value = {
      ...data,
      default_interval_minutes: Math.max(5, data.default_interval_minutes || 10),
    };
    showWarning.value = !form.value.download_dir;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
  }
});

async function save() {
  if (!form.value.download_dir) {
    ElMessage.error("下载目录不能为空");
    showWarning.value = true;
    return;
  }

  saving.value = true;
  try {
    await globalApi.save({
      ...form.value,
      default_interval_minutes: Math.max(5, form.value.default_interval_minutes),
    });
    ElMessage.success("保存成功");
    showWarning.value = false;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <div class="page-container global-settings-page">
    <div class="page-header">
      <div class="page-title-group">
        <h2 class="page-title">系统设置</h2>
        <p class="page-subtitle">配置程序运行的全局参数和默认行为</p>
      </div>
      <div class="page-actions">
        <el-button type="primary" :loading="saving" @click="save">
          <el-icon><Setting /></el-icon>
          保存设置
        </el-button>
      </div>
    </div>

    <el-alert
      v-if="showWarning"
      title="警告：未设置下载目录，后台任务不会启动，请先设置并保存"
      type="warning"
      show-icon
      :closable="false"
      class="settings-warning" />

    <el-card v-loading="loading" shadow="never" class="common-card global-settings-card">
      <template #header>
        <div class="form-card-header">
          <h3>
            <el-icon><Setting /></el-icon>
            全局配置
          </h3>
        </div>
      </template>

      <el-form :model="form" label-width="160px" label-position="top" class="settings-form">
        <!-- 基础运行设置 -->
        <div class="form-section settings-section">
          <div class="form-section-title">
            <el-icon><Timer /></el-icon>
            运行频率
          </div>
          <el-form-item label="默认间隔(分钟)">
            <el-input-number
              v-model="form.default_interval_minutes"
              :min="5"
              :max="1440"
              :step="1" />
            <div class="form-tip">所有 RSS 任务的默认检查时间间隔，最小 5 分钟</div>
          </el-form-item>
        </div>

        <!-- 目录设置 -->
        <div class="form-section settings-section">
          <div class="form-section-title">
            <el-icon><Folder /></el-icon>
            存储路径
          </div>
          <el-form-item label="种子下载目录">
            <el-input v-model="form.download_dir" placeholder="保存 .torrent 种子文件的目录" />
            <div class="form-tip">
              绝对路径将直接使用；相对路径会拼接为
              <code>~/.pt-tools/&lt;输入值&gt;</code>
              并自动创建目录
            </div>
            <div class="form-tip">
              注意：此目录仅用于备份下载的
              <code>.torrent</code>
              文件，不是下载器的数据保存路径
            </div>
          </el-form-item>
        </div>

        <!-- 限制与自动化 -->
        <div class="form-section settings-section">
          <div class="form-section-title">
            <el-icon><Speedometer /></el-icon>
            下载策略与限制
          </div>
          <el-row :gutter="40">
            <el-col :md="12" :sm="24">
              <el-form-item label="最大种子大小(GB)">
                <el-input-number
                  v-model="form.torrent_size_gb"
                  :min="1"
                  :max="10000"
                  class="w-full" />
                <div class="form-tip">超过此大小的种子将被自动忽略，防止磁盘撑爆</div>
              </el-form-item>
            </el-col>
            <el-col :md="12" :sm="24">
              <el-form-item label="自动启动任务">
                <el-switch v-model="form.auto_start" />
                <div class="form-tip">程序启动时立即开启已启用的 RSS 检查任务</div>
              </el-form-item>
            </el-col>
          </el-row>

          <el-row :gutter="40" class="strategy-row">
            <el-col :md="12" :sm="24">
              <el-form-item label="启用下载限速判断">
                <el-switch v-model="form.download_limit_enabled" />
                <div class="form-tip">用于评估种子是否能在免费期内下载完成</div>
              </el-form-item>
            </el-col>
            <el-col :md="12" :sm="24">
              <el-form-item label="预估下载速度(MB/s)">
                <el-input-number
                  v-model="form.download_speed_limit"
                  :min="1"
                  :max="1000"
                  :disabled="!form.download_limit_enabled"
                  class="w-full" />
                <div class="form-tip">根据您的网络环境填写实际平均下载速度</div>
              </el-form-item>
            </el-col>
          </el-row>

          <el-row :gutter="40">
            <el-col :md="12" :sm="24">
              <el-form-item label="最短免费时间(分钟)">
                <el-input-number
                  v-model="form.min_free_minutes"
                  :min="0"
                  :max="1440"
                  :step="5"
                  class="w-full" />
                <div class="form-tip">免费剩余时间少于此值的种子将被跳过，0 表示不限制</div>
              </el-form-item>
            </el-col>
          </el-row>
        </div>

        <!-- 免费结束管理 -->
        <div class="form-section settings-section">
          <div class="form-section-title">
            <el-icon><Timer /></el-icon>
            免费结束管理
          </div>
          <el-form-item label="免费结束自动删除">
            <el-switch v-model="form.auto_delete_on_free_end" />
            <div class="form-tip">
              开启后，免费期结束时未下载完成的种子将自动从下载器中删除（包括数据文件），无需手动操作
            </div>
            <el-alert
              v-if="form.auto_delete_on_free_end"
              title="注意：开启后将自动删除免费期结束时未完成的种子及其数据文件，此操作不可恢复"
              type="warning"
              show-icon
              :closable="false"
              class="mt-2" />
            <div class="form-tip">
              关闭时，免费期结束的未完成种子仅暂停，可在「暂停任务管理」页面手动恢复或删除
            </div>
          </el-form-item>
        </div>
      </el-form>

      <div class="form-actions">
        <el-button type="primary" :loading="saving" @click="save" size="large">
          保存所有设置
        </el-button>
      </div>
    </el-card>
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";
@import "@/styles/form-page.css";
@import "@/styles/global-settings-page.css";
</style>
