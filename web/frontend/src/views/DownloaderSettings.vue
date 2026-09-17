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
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
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

const toolbarNote = computed(() => {
  if (downloaders.value.length === 0) return "还没有配置下载器";
  const dl = defaultDownloader.value;
  return dl
    ? `共 ${downloaders.value.length} 个 · 默认 ${dl.name}（${getTypeLabel(dl.type)}）`
    : `共 ${downloaders.value.length} 个 · 未指定默认下载器`;
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
    await ElMessageBox.confirm(
      `确定删除下载器 "${dl.name}"？绑定到它的站点会回退到默认下载器。`,
      "删除下载器",
      {
        confirmButtonText: "确定删除",
        cancelButtonText: "取消",
        type: "warning",
      },
    );
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

/*
 * 状态胶囊上只写四个字以内的结论，失败原因走 tooltip：
 * 后端的 message 可能是一整条 HTTP 错误，直接铺在单元格里会把这一列撑到 300 宽。
 */
function healthMeta(dl: DownloaderSetting): {
  tone: "ok" | "warn" | "dang" | "neutral";
  text: string;
  detail: string;
} {
  if (!dl.id || !dl.enabled) return { tone: "neutral", text: "未启用", detail: "" };
  const status = healthStatus.value[dl.id];
  if (!status) return { tone: "warn", text: "未检查", detail: "" };
  return status.is_healthy
    ? { tone: "ok", text: "正常", detail: "" }
    : { tone: "dang", text: "异常", detail: status.message || "连接异常" };
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
    await ElMessageBox.confirm(`确定删除目录 "${dir.alias || dir.path}"？`, "删除目录", {
      confirmButtonText: "确定删除",
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

function toggleSiteSelection(siteId: number, checked: boolean) {
  if (checked) {
    if (!selectedSiteIds.value.includes(siteId)) selectedSiteIds.value.push(siteId);
  } else {
    selectedSiteIds.value = selectedSiteIds.value.filter((id) => id !== siteId);
  }
}
</script>

<template>
  <div class="downloader-page">
    <PtToolbar standalone :note="toolbarNote">
      <el-button size="small" :loading="loading" @click="loadDownloaders">
        <PtIcon name="refresh-cw" :size="14" /><span>刷新</span>
      </el-button>
      <template #right>
        <el-button type="primary" size="small" @click="openAddDialog">
          <PtIcon name="plus" :size="14" /><span>添加下载器</span>
        </el-button>
      </template>
    </PtToolbar>

    <PtPanel
      v-loading="loading"
      title="下载器"
      icon="hard-drive"
      :count="`${downloaders.length} 个`"
      padding="none">
      <el-table :data="downloaders" class="pt-grid" row-key="id" style="width: 100%">
        <template #empty>
          <PtDataState
            state="empty"
            dense
            title="还没有下载器"
            sub="接上 qBittorrent 或 Transmission，RSS 命中的种子才有地方推" />
        </template>

        <!-- 「谁是默认」在整张表里只能有一个，做成一列星标比每行一枚按钮更像单选 -->
        <el-table-column label="默认" width="66" align="center">
          <template #default="{ row }">
            <el-tooltip
              :content="row.is_default ? '当前默认下载器' : '设为默认下载器'"
              placement="top"
              :show-after="300">
              <button
                type="button"
                class="star"
                :class="{ 'is-on': row.is_default }"
                :disabled="row.is_default"
                :aria-label="row.is_default ? '当前默认下载器' : `把 ${row.name} 设为默认下载器`"
                @click="setDefault(row)">
                <PtIcon name="star" :size="15" />
              </button>
            </el-tooltip>
          </template>
        </el-table-column>

        <el-table-column
          prop="name"
          label="名称"
          min-width="140"
          show-overflow-tooltip
          class-name="pt-cell-strong" />

        <el-table-column prop="type" label="类型" width="130">
          <template #default="{ row }">
            <PtTag>{{ getTypeLabel(row.type) }}</PtTag>
          </template>
        </el-table-column>

        <el-table-column
          prop="url"
          label="地址"
          min-width="200"
          show-overflow-tooltip
          class-name="pt-cell-muted">
          <template #default="{ row }">
            <code class="url">{{ row.url }}</code>
          </template>
        </el-table-column>

        <el-table-column label="连通性" width="110">
          <template #default="{ row }">
            <el-tooltip
              v-if="healthMeta(row).detail"
              :content="healthMeta(row).detail"
              placement="top"
              :show-after="300">
              <span class="pill-wrap">
                <PtStatusPill :tone="healthMeta(row).tone" size="sm">
                  {{ healthMeta(row).text }}
                </PtStatusPill>
              </span>
            </el-tooltip>
            <PtStatusPill v-else :tone="healthMeta(row).tone" size="sm">
              {{ healthMeta(row).text }}
            </PtStatusPill>
          </template>
        </el-table-column>

        <el-table-column label="启用" width="78" align="center">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" size="small" @change="toggleEnabled(row)" />
          </template>
        </el-table-column>

        <el-table-column label="操作" width="250" fixed="right" class-name="pt-cell-act">
          <template #default="{ row }">
            <el-button
              link
              type="primary"
              size="small"
              :disabled="!row.enabled"
              @click="checkHealth(row)">
              <PtIcon name="activity" :size="14" /><span>检查</span>
            </el-button>
            <el-button link type="primary" size="small" @click="openDirDialog(row)">
              <PtIcon name="folder" :size="14" /><span>目录</span>
            </el-button>
            <el-button link type="primary" size="small" @click="openEditDialog(row)">
              <PtIcon name="pencil" :size="14" /><span>编辑</span>
            </el-button>
            <el-button link type="danger" size="small" @click="deleteDownloader(row)">
              <PtIcon name="trash-2" :size="14" /><span>删除</span>
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <template v-if="downloaders.length > 0" #footer>
        <span class="pt-foot-note">
          默认下载器用于没有单独绑定下载器的站点；「检查」只探连通性，不改任何配置
        </span>
      </template>
    </PtPanel>

    <!-- 添加/编辑对话框 -->
    <el-dialog
      v-model="showDialog"
      class="pt-dialog"
      :title="editMode ? '编辑下载器' : '添加下载器'"
      width="520px"
      align-center>
      <el-form :model="form" class="pt-form" label-position="top" @submit.prevent>
        <div class="field-row">
          <el-form-item label="名称" required>
            <el-input v-model="form.name" placeholder="例如：主下载器" :disabled="editMode" />
            <div class="field-tip">{{ editMode ? "名称建好后不可改" : "只用于在列表里区分" }}</div>
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
        </div>

        <el-form-item label="地址" required>
          <el-input
            v-model="form.url"
            :placeholder="
              form.type === 'qbittorrent' ? 'http://192.168.1.10:8080' : 'http://192.168.1.10:9091'
            " />
          <div class="field-tip">
            {{ form.type === "qbittorrent" ? "qBittorrent Web UI 地址" : "Transmission RPC 地址" }}
          </div>
        </el-form-item>

        <div class="field-row">
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
        </div>

        <div class="field-head">行为</div>

        <div class="opts">
          <label class="opt">
            <el-switch v-model="form.is_default" size="small" />
            <span class="opt__body">
              <span class="opt__name">设为默认</span>
              <span class="opt__tip">没有单独绑定下载器的站点都推到这里</span>
            </span>
          </label>

          <label class="opt">
            <el-switch v-model="form.enabled" size="small" />
            <span class="opt__body">
              <span class="opt__name">启用</span>
              <span class="opt__tip">停用后不参与推送，也不做连通性检查</span>
            </span>
          </label>

          <label class="opt">
            <el-switch v-model="form.auto_start" size="small" />
            <span class="opt__body">
              <span class="opt__name">自动开始</span>
              <span class="opt__tip">关掉的话，种子以暂停状态添加，要手动开始</span>
            </span>
          </label>
        </div>
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
      class="pt-dialog"
      :title="`保存目录 · ${currentDownloader?.name || ''}`"
      width="720px"
      align-center>
      <PtToolbar :note="`${directories.length} 个目录`">
        <template #right>
          <el-button type="primary" size="small" @click="openAddDirDialog">
            <PtIcon name="plus" :size="14" /><span>添加目录</span>
          </el-button>
        </template>
      </PtToolbar>

      <el-table
        v-loading="loadingDirs"
        :data="directories"
        class="pt-grid"
        row-key="id"
        style="width: 100%">
        <template #empty>
          <PtDataState state="empty" dense sub="没有配置目录时，推送走下载器自己的默认保存路径" />
        </template>

        <el-table-column label="默认" width="66" align="center">
          <template #default="{ row }">
            <el-tooltip
              :content="row.is_default ? '当前默认目录' : '设为默认目录'"
              placement="top"
              :show-after="300">
              <button
                type="button"
                class="star"
                :class="{ 'is-on': row.is_default }"
                :disabled="row.is_default"
                :aria-label="row.is_default ? '当前默认目录' : '设为默认目录'"
                @click="setDefaultDirectory(row)">
                <PtIcon name="star" :size="15" />
              </button>
            </el-tooltip>
          </template>
        </el-table-column>

        <el-table-column label="别名" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">{{ row.alias || "-" }}</template>
        </el-table-column>

        <el-table-column
          label="路径"
          min-width="260"
          show-overflow-tooltip
          class-name="pt-cell-strong">
          <template #default="{ row }">
            <code class="url">{{ row.path }}</code>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="140" fixed="right" class-name="pt-cell-act">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openEditDirDialog(row)">
              <PtIcon name="pencil" :size="14" /><span>编辑</span>
            </el-button>
            <el-button link type="danger" size="small" @click="deleteDirectory(row)">
              <PtIcon name="trash-2" :size="14" /><span>删除</span>
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <!-- 添加/编辑目录对话框 -->
    <el-dialog
      v-model="showAddDirDialog"
      class="pt-dialog"
      :title="editDirMode ? '编辑目录' : '添加目录'"
      width="480px"
      align-center
      append-to-body>
      <el-form :model="dirForm" class="pt-form" label-position="top" @submit.prevent>
        <el-form-item label="路径" required>
          <el-input v-model="dirForm.path" placeholder="/downloads/movies" />
          <div class="field-tip">下载器容器内看到的路径，不是本机路径</div>
        </el-form-item>

        <el-form-item label="别名">
          <el-input v-model="dirForm.alias" placeholder="电影目录" />
          <div class="field-tip">推送时下拉框里显示的名字，留空就显示路径</div>
        </el-form-item>

        <label class="opt">
          <el-switch v-model="dirForm.is_default" size="small" />
          <span class="opt__body">
            <span class="opt__name">设为默认目录</span>
            <span class="opt__tip">推送时自动选中它</span>
          </span>
        </label>
      </el-form>

      <template #footer>
        <el-button @click="showAddDirDialog = false">取消</el-button>
        <el-button type="primary" :loading="savingDir" @click="saveDirectory">
          {{ editDirMode ? "保存" : "添加" }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 站点下载器同步对话框 -->
    <el-dialog
      v-model="showSyncDialog"
      class="pt-dialog"
      title="同步站点下载器"
      width="620px"
      align-center>
      <div class="pt-note sync-note">
        <PtIcon name="info" :size="14" class="pt-note__icon" />
        <span>
          默认下载器已切到「{{ newDefaultDownloader?.name }}」。下面这些站点要不要一并改过来？
          默认勾选的是原本就跟着默认走的站点。
        </span>
      </div>

      <PtToolbar :note="`已选 ${selectedSiteIds.length} / ${syncSites.length}`">
        <el-button size="small" @click="toggleAllSites">
          <PtIcon name="check-check" :size="14" />
          <span>{{ selectedSiteIds.length === syncSites.length ? "取消全选" : "全选" }}</span>
        </el-button>
      </PtToolbar>

      <el-table
        v-loading="loadingSyncSites"
        :data="syncSites"
        class="pt-grid"
        row-key="site_id"
        max-height="380"
        style="width: 100%">
        <template #empty>
          <PtDataState state="empty" dense sub="还没有启用的站点" />
        </template>

        <el-table-column width="50" align="center">
          <template #default="{ row }">
            <el-checkbox
              :model-value="selectedSiteIds.includes(row.site_id)"
              :aria-label="`选择 ${row.display_name || row.site_name}`"
              @change="(val: boolean) => toggleSiteSelection(row.site_id, val)" />
          </template>
        </el-table-column>

        <el-table-column label="站点" min-width="160" class-name="pt-cell-strong">
          <template #default="{ row }">{{ row.display_name || row.site_name }}</template>
        </el-table-column>

        <el-table-column label="当前下载器" min-width="150">
          <template #default="{ row }">
            <PtTag v-if="row.downloader_name">{{ row.downloader_name }}</PtTag>
            <span v-else class="follow">跟随默认</span>
          </template>
        </el-table-column>
      </el-table>

      <template #footer>
        <el-button @click="showSyncDialog = false">跳过</el-button>
        <el-button type="primary" :loading="applyingSites" @click="applySitesDownloader">
          应用到 {{ selectedSiteIds.length }} 个站点
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.downloader-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
}

.url {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t2);
}

/* el-tooltip 要一个能挂事件的元素，胶囊本身是组件根，包一层 span 最省事 */
.pill-wrap {
  display: inline-flex;
}

/*
 * 星标是这张表里唯一的图标按钮：默认态描边灰、选中态实心 warn。
 * 选中后 disabled，因为「取消默认」不是一个合法操作 —— 只能把默认给别人。
 */
.star {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  color: var(--pt-t4);
  cursor: pointer;
  background: none;
  border: 0;
  border-radius: var(--pt-r-sm);
  transition:
    color var(--pt-transition-fast),
    background var(--pt-transition-fast);
}

.star:hover:not(:disabled) {
  color: var(--pt-warn);
  background: var(--pt-hover);
}

.star.is-on {
  color: var(--pt-warn);
  cursor: default;
}

.follow {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.sync-note {
  margin-bottom: var(--pt-space-3);
}

/*
 * 开关行：开关在左，右边是名字 + 一行说明。
 * 不用 el-form-item 的 label，因为这三项的说明比标题长得多，
 * 压在标签下方会让「设为默认」和它的解释隔着一整个控件。
 */
.opts {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-3);
}

.opt {
  display: flex;
  gap: var(--pt-space-3);
  align-items: flex-start;
  cursor: pointer;
}

.opt__body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.opt__name {
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-t1);
}

.opt__tip {
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-body);
  color: var(--pt-t3);
}
</style>
