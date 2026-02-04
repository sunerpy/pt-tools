<script setup lang="ts">
import {
  downloaderDirectoriesApi,
  type DownloaderDirectory,
  type DownloaderHealthResponse,
  downloadersApi,
  type DownloaderSetting,
  dynamicSitesApi,
  type SiteDownloaderSummaryItem,
} from "@/api";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref } from "vue";

const loading = ref(false);
const saving = ref(false);
const showDialog = ref(false);
const editMode = ref(false);
const healthTimeoutMs = 5000;

const downloaders = ref<DownloaderSetting[]>([]);
const healthStatus = ref<Record<number, DownloaderHealthResponse>>({});

// 目录管理相关
const showDirDialog = ref(false);
const currentDownloader = ref<DownloaderSetting | null>(null);
const directories = ref<DownloaderDirectory[]>([]);
const loadingDirs = ref(false);
const showAddDirDialog = ref(false);
const editDirMode = ref(false);
const savingDir = ref(false);
const dirForm = ref<DownloaderDirectory>({
  downloader_id: 0,
  path: "",
  alias: "",
  is_default: false,
});

const showSyncDialog = ref(false);
const syncSites = ref<SiteDownloaderSummaryItem[]>([]);
const selectedSiteIds = ref<number[]>([]);
const loadingSyncSites = ref(false);
const applyingSites = ref(false);
const newDefaultDownloader = ref<DownloaderSetting | null>(null);

const form = ref<DownloaderSetting>({
  name: "",
  type: "qbittorrent",
  url: "",
  username: "",
  password: "",
  is_default: false,
  enabled: true,
  auto_start: false,
});

const downloaderTypes = [
  { value: "qbittorrent", label: "qBittorrent" },
  { value: "transmission", label: "Transmission" },
];

const defaultDownloader = computed(() => {
  return downloaders.value.find((d) => d.is_default);
});

onMounted(async () => {
  await loadDownloaders();
});

async function loadDownloaders() {
  loading.value = true;
  try {
    downloaders.value = await downloadersApi.list();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
  }
  loadHealthStatuses(downloaders.value);
}

async function fetchHealthStatus(downloaderId: number): Promise<DownloaderHealthResponse> {
  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), healthTimeoutMs);
  try {
    const response = await fetch(`/api/downloaders/${downloaderId}/health`, {
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
      },
      signal: controller.signal,
    });

    if (!response.ok) {
      const msg = await response.text();
      throw new Error(msg || `HTTP ${response.status}`);
    }

    return (await response.json()) as DownloaderHealthResponse;
  } finally {
    window.clearTimeout(timeout);
  }
}

function getHealthErrorMessage(error: unknown) {
  if (error instanceof Error && error.name === "AbortError") {
    return "检查超时";
  }
  return (error as Error)?.message || "检查失败";
}

function loadHealthStatuses(list: DownloaderSetting[]) {
  const tasks = list
    .filter((dl) => dl.id && dl.enabled)
    .map((dl) =>
      fetchHealthStatus(dl.id!).then(
        (response) => {
          healthStatus.value[dl.id!] = response;
        },
        (error) => {
          healthStatus.value[dl.id!] = {
            name: dl.name,
            is_healthy: false,
            message: getHealthErrorMessage(error),
          };
        },
      ),
    );

  void Promise.allSettled(tasks);
}

function openAddDialog() {
  editMode.value = false;
  form.value = {
    name: "",
    type: "qbittorrent",
    url: "",
    username: "",
    password: "",
    is_default: downloaders.value.length === 0,
    enabled: true,
    auto_start: false,
  };
  showDialog.value = true;
}

function openEditDialog(dl: DownloaderSetting) {
  editMode.value = true;
  form.value = { ...dl, password: "" };
  showDialog.value = true;
}

async function saveDownloader() {
  const errors: string[] = [];

  if (!form.value.name?.trim()) {
    errors.push("名称");
  }
  if (!form.value.url?.trim()) {
    errors.push("URL");
  }
  if (!form.value.username?.trim()) {
    errors.push("用户名");
  }
  if (!editMode.value && !form.value.password) {
    errors.push("密码");
  }

  if (errors.length > 0) {
    ElMessage.error(`${errors.join("、")}为必填项`);
    return;
  }

  saving.value = true;
  try {
    if (editMode.value && form.value.id) {
      await downloadersApi.update(form.value.id, form.value);
      ElMessage.success("更新成功");
    } else {
      await downloadersApi.create(form.value);
      ElMessage.success("创建成功");
    }
    showDialog.value = false;
    await loadDownloaders();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

async function deleteDownloader(dl: DownloaderSetting) {
  if (!dl.id) return;

  try {
    await ElMessageBox.confirm(`确定删除下载器 "${dl.name}"？`, "确认删除", {
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      type: "warning",
    });
    await downloadersApi.delete(dl.id);
    ElMessage.success("已删除");
    await loadDownloaders();
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "删除失败");
    }
  }
}

async function toggleEnabled(dl: DownloaderSetting) {
  if (!dl.id) return;
  const newEnabled = !dl.enabled;
  try {
    await downloadersApi.update(dl.id, { ...dl, enabled: newEnabled });
    dl.enabled = newEnabled;
    ElMessage.success("已保存");
    // 刷新健康状态
    if (newEnabled) {
      try {
        healthStatus.value[dl.id] = await fetchHealthStatus(dl.id);
      } catch (error: unknown) {
        healthStatus.value[dl.id] = {
          name: dl.name,
          is_healthy: false,
          message: getHealthErrorMessage(error),
        };
      }
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  }
}

async function setDefault(dl: DownloaderSetting) {
  if (!dl.id || dl.is_default) return;
  try {
    await downloadersApi.setDefault(dl.id);
    ElMessage.success("已设为默认");
    await loadDownloaders();
    openSyncDialog(dl);
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "设置失败");
  }
}

async function checkHealth(dl: DownloaderSetting) {
  if (!dl.id) return;
  try {
    healthStatus.value[dl.id] = await fetchHealthStatus(dl.id);
    const status = healthStatus.value[dl.id];
    if (status && status.is_healthy) {
      ElMessage.success("连接正常");
    } else {
      ElMessage.warning(status?.message || "连接异常");
    }
  } catch (e: unknown) {
    const message = getHealthErrorMessage(e);
    healthStatus.value[dl.id] = { name: dl.name, is_healthy: false, message };
    ElMessage.error(message);
  }
}

function getHealthTag(dl: DownloaderSetting) {
  if (!dl.id || !dl.enabled) return { type: "info" as const, text: "未启用" };
  const status = healthStatus.value[dl.id];
  if (!status) return { type: "info" as const, text: "未检查" };
  return status.is_healthy
    ? { type: "success" as const, text: "正常" }
    : { type: "danger" as const, text: status.message || "异常" };
}

function getTypeLabel(type: string) {
  return downloaderTypes.find((t) => t.value === type)?.label || type;
}

// ============== 目录管理功能 ==============

async function openDirDialog(dl: DownloaderSetting) {
  currentDownloader.value = dl;
  showDirDialog.value = true;
  await loadDirectories(dl.id!);
}

async function loadDirectories(downloaderId: number) {
  loadingDirs.value = true;
  try {
    directories.value = await downloaderDirectoriesApi.list(downloaderId);
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载目录失败");
    directories.value = [];
  } finally {
    loadingDirs.value = false;
  }
}

function openAddDirDialog() {
  if (!currentDownloader.value?.id) return;
  editDirMode.value = false;
  dirForm.value = {
    downloader_id: currentDownloader.value.id,
    path: "",
    alias: "",
    is_default: directories.value.length === 0,
  };
  showAddDirDialog.value = true;
}

function openEditDirDialog(dir: DownloaderDirectory) {
  editDirMode.value = true;
  dirForm.value = { ...dir };
  showAddDirDialog.value = true;
}

async function saveDirectory() {
  if (!dirForm.value.path) {
    ElMessage.error("路径为必填项");
    return;
  }

  savingDir.value = true;
  try {
    if (editDirMode.value && dirForm.value.id) {
      await downloaderDirectoriesApi.update(
        dirForm.value.downloader_id,
        dirForm.value.id,
        dirForm.value,
      );
      ElMessage.success("更新成功");
    } else {
      await downloaderDirectoriesApi.create(currentDownloader.value!.id!, {
        path: dirForm.value.path,
        alias: dirForm.value.alias,
        is_default: dirForm.value.is_default,
      });
      ElMessage.success("添加成功");
    }
    showAddDirDialog.value = false;
    await loadDirectories(currentDownloader.value!.id!);
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    savingDir.value = false;
  }
}

async function deleteDirectory(dir: DownloaderDirectory) {
  if (!dir.id || !currentDownloader.value?.id) return;

  try {
    await ElMessageBox.confirm(`确定删除目录 "${dir.alias || dir.path}"？`, "确认删除", {
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      type: "warning",
    });
    await downloaderDirectoriesApi.delete(currentDownloader.value.id, dir.id);
    ElMessage.success("已删除");
    await loadDirectories(currentDownloader.value.id);
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "删除失败");
    }
  }
}

async function setDefaultDirectory(dir: DownloaderDirectory) {
  if (!dir.id || !currentDownloader.value?.id || dir.is_default) return;
  try {
    await downloaderDirectoriesApi.setDefault(currentDownloader.value.id, dir.id);
    ElMessage.success("已设为默认");
    await loadDirectories(currentDownloader.value.id);
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "设置失败");
  }
}

async function openSyncDialog(dl: DownloaderSetting) {
  newDefaultDownloader.value = dl;
  loadingSyncSites.value = true;
  showSyncDialog.value = true;

  try {
    const resp = await dynamicSitesApi.getDownloaderSummary();
    syncSites.value = resp.sites;
    selectedSiteIds.value = resp.sites.filter((s) => s.downloader_id == null).map((s) => s.site_id);
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载站点失败");
    showSyncDialog.value = false;
  } finally {
    loadingSyncSites.value = false;
  }
}

async function applySitesDownloader() {
  if (selectedSiteIds.value.length === 0) {
    showSyncDialog.value = false;
    return;
  }
  if (!newDefaultDownloader.value?.id) return;

  applyingSites.value = true;
  try {
    const resp = await downloadersApi.applyToSites(
      newDefaultDownloader.value.id,
      selectedSiteIds.value,
    );
    ElMessage.success(`已更新 ${resp.updated_count} 个站点`);
    showSyncDialog.value = false;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "应用失败");
  } finally {
    applyingSites.value = false;
  }
}

function toggleAllSites() {
  if (selectedSiteIds.value.length === syncSites.value.length) {
    selectedSiteIds.value = [];
  } else {
    selectedSiteIds.value = syncSites.value.map((s) => s.site_id);
  }
}
</script>

<template>
  <div class="page-container">
    <el-card v-loading="loading" shadow="never">
      <template #header>
        <div class="card-header">
          <span>下载器管理</span>
          <el-button type="primary" :icon="'Plus'" @click="openAddDialog">添加下载器</el-button>
        </div>
      </template>

      <div v-if="defaultDownloader" class="default-info">
        <el-icon><Star /></el-icon>
        <span>
          默认下载器: {{ defaultDownloader.name }} ({{ getTypeLabel(defaultDownloader.type) }})
        </span>
      </div>

      <el-table :data="downloaders" style="width: 100%" border resizable>
        <el-table-column type="index" label="序号" width="60" align="center" />

        <el-table-column label="名称" min-width="120" resizable show-overflow-tooltip>
          <template #default="{ row }">
            <div class="dl-name">
              <el-icon v-if="row.is_default" color="#E6A23C"><Star /></el-icon>
              <span>{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="类型" min-width="100" align="center" resizable>
          <template #default="{ row }">
            <el-tag type="primary" size="small" effect="plain">
              {{ getTypeLabel(row.type) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="URL" min-width="200" resizable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="url-text">{{ row.url }}</span>
          </template>
        </el-table-column>

        <el-table-column
          label="状态"
          min-width="120"
          align="center"
          resizable
          show-overflow-tooltip>
          <template #default="{ row }">
            <el-tooltip
              v-if="getHealthTag(row).type !== 'success' && healthStatus[row.id]?.message"
              :content="healthStatus[row.id]?.message"
              placement="top"
              :show-after="300">
              <el-tag :type="getHealthTag(row).type" size="small">
                {{ getHealthTag(row).text }}
              </el-tag>
            </el-tooltip>
            <el-tag v-else :type="getHealthTag(row).type" size="small">
              {{ getHealthTag(row).text }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="操作" min-width="300" align="center">
          <template #default="{ row }">
            <el-space>
              <el-switch :model-value="row.enabled" size="small" @change="toggleEnabled(row)" />
              <el-button
                type="info"
                size="small"
                :disabled="!row.enabled"
                @click="checkHealth(row)">
                检查
              </el-button>
              <el-button type="success" size="small" @click="openDirDialog(row)">目录</el-button>
              <el-button
                type="warning"
                size="small"
                :disabled="row.is_default"
                @click="setDefault(row)">
                设为默认
              </el-button>
              <el-button type="primary" size="small" @click="openEditDialog(row)">编辑</el-button>
              <el-button type="danger" size="small" @click="deleteDownloader(row)">删除</el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="downloaders.length === 0" description="暂无下载器，点击上方按钮添加" />
    </el-card>

    <!-- 添加/编辑对话框 -->
    <el-dialog v-model="showDialog" :title="editMode ? '编辑下载器' : '添加下载器'" width="500px">
      <el-form :model="form" label-width="100px" label-position="right">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="例如: 主下载器" :disabled="editMode" />
        </el-form-item>

        <el-form-item label="类型" required>
          <el-select v-model="form.type" style="width: 100%">
            <el-option
              v-for="t in downloaderTypes"
              :key="t.value"
              :label="t.label"
              :value="t.value" />
          </el-select>
        </el-form-item>

        <el-form-item label="URL" required>
          <el-input
            v-model="form.url"
            :placeholder="
              form.type === 'qbittorrent' ? 'http://192.168.1.10:8080' : 'http://192.168.1.10:9091'
            " />
          <div class="form-tip">
            {{ form.type === "qbittorrent" ? "qBittorrent Web UI 地址" : "Transmission RPC 地址" }}
          </div>
        </el-form-item>

        <el-form-item label="用户名" required>
          <el-input v-model="form.username" placeholder="admin" />
        </el-form-item>

        <el-form-item label="密码" :required="!editMode">
          <el-input
            v-model="form.password"
            type="password"
            show-password
            :placeholder="editMode ? '留空保持不变' : '请输入密码'" />
        </el-form-item>

        <el-form-item label="设为默认">
          <el-switch v-model="form.is_default" />
          <div class="form-tip">默认下载器将用于未指定下载器的站点</div>
        </el-form-item>

        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>

        <el-form-item label="自动开始">
          <el-switch v-model="form.auto_start" />
          <div class="form-tip">推送种子后自动开始下载，关闭则以暂停状态添加</div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveDownloader">
          {{ editMode ? "保存" : "添加" }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 目录管理对话框 -->
    <el-dialog
      v-model="showDirDialog"
      :title="`目录管理 - ${currentDownloader?.name || ''}`"
      width="700px">
      <div class="dir-header">
        <el-button type="primary" size="small" @click="openAddDirDialog">添加目录</el-button>
      </div>

      <el-table v-loading="loadingDirs" :data="directories" style="width: 100%" border resizable>
        <el-table-column label="别名" min-width="120" resizable show-overflow-tooltip>
          <template #default="{ row }">
            <div class="dir-alias">
              <el-icon v-if="row.is_default" color="#E6A23C"><Star /></el-icon>
              <span>{{ row.alias || "-" }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="路径" min-width="250" resizable show-overflow-tooltip>
          <template #default="{ row }">
            <span class="url-text">{{ row.path }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" min-width="180" align="center">
          <template #default="{ row }">
            <el-space>
              <el-button
                type="warning"
                size="small"
                :disabled="row.is_default"
                @click="setDefaultDirectory(row)">
                设为默认
              </el-button>
              <el-button type="primary" size="small" @click="openEditDirDialog(row)">
                编辑
              </el-button>
              <el-button type="danger" size="small" @click="deleteDirectory(row)">删除</el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="directories.length === 0 && !loadingDirs" description="暂无目录配置" />
    </el-dialog>

    <!-- 添加/编辑目录对话框 -->
    <el-dialog
      v-model="showAddDirDialog"
      :title="editDirMode ? '编辑目录' : '添加目录'"
      width="500px"
      append-to-body>
      <el-form :model="dirForm" label-width="100px" label-position="right">
        <el-form-item label="路径" required>
          <el-input v-model="dirForm.path" placeholder="/downloads/movies" />
          <div class="form-tip">下载器中的目标保存路径</div>
        </el-form-item>

        <el-form-item label="别名">
          <el-input v-model="dirForm.alias" placeholder="电影目录" />
          <div class="form-tip">便于记忆的名称（可选）</div>
        </el-form-item>

        <el-form-item label="设为默认">
          <el-switch v-model="dirForm.is_default" />
          <div class="form-tip">默认目录将在推送时自动选中</div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showAddDirDialog = false">取消</el-button>
        <el-button type="primary" :loading="savingDir" @click="saveDirectory">
          {{ editDirMode ? "保存" : "添加" }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 站点下载器同步对话框 -->
    <el-dialog v-model="showSyncDialog" title="同步站点下载器" width="600px">
      <div v-if="newDefaultDownloader" class="sync-info">
        是否将以下站点的下载器设置为「{{ newDefaultDownloader.name }}」？
      </div>

      <div class="sync-header">
        <el-button size="small" @click="toggleAllSites">
          {{ selectedSiteIds.length === syncSites.length ? "取消全选" : "全选" }}
        </el-button>
        <span class="sync-count">已选择 {{ selectedSiteIds.length }} / {{ syncSites.length }}</span>
      </div>

      <el-table
        v-loading="loadingSyncSites"
        :data="syncSites"
        style="width: 100%"
        border
        max-height="400">
        <el-table-column width="50" align="center">
          <template #default="{ row }">
            <el-checkbox
              :model-value="selectedSiteIds.includes(row.site_id)"
              @change="
                (val: boolean) => {
                  if (val) {
                    selectedSiteIds.push(row.site_id);
                  } else {
                    selectedSiteIds = selectedSiteIds.filter((id) => id !== row.site_id);
                  }
                }
              " />
          </template>
        </el-table-column>
        <el-table-column label="站点" min-width="120">
          <template #default="{ row }">
            <span>{{ row.display_name || row.site_name }}</span>
          </template>
        </el-table-column>
        <el-table-column label="当前下载器" min-width="120">
          <template #default="{ row }">
            <el-tag v-if="row.downloader_name" size="small">{{ row.downloader_name }}</el-tag>
            <el-tag v-else type="info" size="small">默认</el-tag>
          </template>
        </el-table-column>
      </el-table>

      <template #footer>
        <el-button @click="showSyncDialog = false">跳过</el-button>
        <el-button type="primary" :loading="applyingSites" @click="applySitesDownloader">
          应用 ({{ selectedSiteIds.length }})
        </el-button>
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

.default-info {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px;
  margin-bottom: 16px;
  background: var(--el-color-warning-light-9);
  border-radius: 4px;
  color: var(--el-color-warning);
}

.dl-name {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
}

.url-text {
  font-family: monospace;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.form-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}

.dir-header {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 16px;
}

.dir-alias {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
}

.sync-info {
  padding: 12px;
  margin-bottom: 16px;
  background: var(--el-color-primary-light-9);
  border-radius: 4px;
  color: var(--el-color-primary);
}

.sync-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.sync-count {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}
</style>
