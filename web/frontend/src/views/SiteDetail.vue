<script setup lang="ts">
import {
  downloaderDirectoriesApi,
  type DownloaderDirectory,
  downloadersApi,
  type DownloaderSetting,
  type FilterRule,
  filterRulesApi,
  type RSSConfig,
  type SiteConfig,
  sitesApi,
} from "@/api";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, reactive, ref } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();

const siteName = computed(() => route.params.name as string);
const loading = ref(false);
const saving = ref(false);
const addingRss = ref(false);
const rssDialogVisible = ref(false);
const downloaders = ref<DownloaderSetting[]>([]);
const filterRules = ref<FilterRule[]>([]);
const downloaderDirectories = ref<Record<number, DownloaderDirectory[]>>({});

// 新增：是否使用自定义路径
const newRssUseCustomPath = ref(false);
const editRssUseCustomPath = ref(false);

const form = ref<SiteConfig>({
  enabled: false,
  auth_method: "cookie",
  cookie: "",
  api_key: "",
  api_url: "",
  rss: [],
});

// 示例 RSS 配置（不存入数据库，仅用于展示）
const exampleRssConfigs: Record<string, RSSConfig[]> = {
  springsunday: [
    {
      name: "SpringSunday 电视剧",
      url: "https://springxxx.xxx/torrentrss.php?passkey=xxx",
      category: "Tv",
      tag: "SpringSunday",
      interval_minutes: 5,
      is_example: true,
    },
  ],
  hdsky: [
    {
      name: "HDSky 电影",
      url: "https://hdsky.xxx/torrentrss.php?passkey=xxx",
      category: "Mv",
      tag: "HDSKY",
      interval_minutes: 5,
      is_example: true,
    },
  ],
  mteam: [
    {
      name: "M-Team 电视剧",
      url: "https://rss.m-team.xxx/api/rss/xxx",
      category: "Tv",
      tag: "MT",
      interval_minutes: 10,
      is_example: true,
    },
  ],
};

// 获取当前站点的示例 RSS
const exampleRss = computed(() => {
  const name = siteName.value.toLowerCase();
  return exampleRssConfigs[name] || [];
});

// 显示的 RSS 列表（真实数据 + 示例数据）
const displayRssList = computed(() => {
  const realRss = form.value.rss || [];
  // 如果有真实数据，只显示真实数据
  if (realRss.length > 0) {
    return realRss;
  }
  // 如果没有真实数据，显示示例数据
  return exampleRss.value;
});

// 是否显示的是示例数据
const showingExamples = computed(() => {
  return (form.value.rss || []).length === 0 && exampleRss.value.length > 0;
});

const newRss = reactive<RSSConfig>({
  name: "",
  url: "",
  category: "",
  tag: "",
  interval_minutes: 10,
  downloader_id: undefined,
  download_path: "",
  filter_rule_ids: [],
  pause_on_free_end: false,
});

const editRssDialogVisible = ref(false);
const editingRss = reactive<RSSConfig>({
  id: undefined,
  name: "",
  url: "",
  category: "",
  tag: "",
  interval_minutes: 10,
  downloader_id: undefined,
  download_path: "",
  filter_rule_ids: [],
  pause_on_free_end: false,
});
const editingRssIndex = ref(-1);
const updatingRss = ref(false);

onMounted(async () => {
  loading.value = true;
  try {
    // 并行加载站点配置、下载器列表、过滤规则列表和下载器目录
    const [siteData, downloaderList, filterRuleList, directoriesData] = await Promise.all([
      sitesApi.get(siteName.value),
      downloadersApi.list(),
      filterRulesApi.list(),
      downloaderDirectoriesApi.listAll(),
    ]);
    form.value = siteData;
    downloaders.value = downloaderList; // 显示所有下载器，不过滤
    filterRules.value = filterRuleList.filter((r) => r.enabled); // 只显示启用的过滤规则
    downloaderDirectories.value = directoriesData;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
  }
});

// 获取指定下载器的目录列表
function getDirectoriesForDownloader(downloaderId: number | undefined): DownloaderDirectory[] {
  if (!downloaderId) {
    // 如果没有指定下载器，获取默认下载器的目录
    const defaultDownloader = downloaders.value.find((d) => d.is_default && d.enabled);
    if (defaultDownloader?.id) {
      return downloaderDirectories.value[defaultDownloader.id] || [];
    }
    return [];
  }
  return downloaderDirectories.value[downloaderId] || [];
}

// 检查路径是否为预设目录
function isPresetDirectory(path: string, downloaderId: number | undefined): boolean {
  const dirs = getDirectoriesForDownloader(downloaderId);
  return dirs.some((d) => d.path === path);
}

// 获取路径的显示名称（优先显示别名）
function getPathDisplayName(path: string, downloaderId: number | undefined): string {
  const dirs = getDirectoriesForDownloader(downloaderId);
  const dir = dirs.find((d) => d.path === path);
  if (dir) {
    return dir.alias || path;
  }
  // 如果是自定义路径，只显示最后一级目录名
  const parts = path.split("/").filter(Boolean);
  return parts.length > 0 ? (parts[parts.length - 1] as string) : path;
}

async function save() {
  saving.value = true;
  try {
    await sitesApi.save(siteName.value, form.value);
    ElMessage.success("保存成功");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

function openAddRssDialog() {
  Object.assign(newRss, {
    name: "",
    url: "",
    category: "",
    tag: "",
    interval_minutes: 10,
    downloader_id: undefined,
    download_path: "",
    filter_rule_ids: [],
    pause_on_free_end: true,
  });
  newRssUseCustomPath.value = false;
  rssDialogVisible.value = true;
}

async function addRss() {
  if (!newRss.name || !newRss.url) {
    ElMessage.error("名称和链接为必填");
    return;
  }
  if (!newRss.url.startsWith("http://") && !newRss.url.startsWith("https://")) {
    ElMessage.error("链接必须以 http:// 或 https:// 开头");
    return;
  }

  // 检查重复 RSS URL
  const normalizedUrl = newRss.url.trim().toLowerCase();
  const rssList = form.value.rss || [];
  const isDuplicate = rssList.some((r) => r.url.trim().toLowerCase() === normalizedUrl);
  if (isDuplicate) {
    ElMessage.error("该 RSS 链接已存在，请勿重复添加");
    return;
  }

  addingRss.value = true;
  console.log("[RSS] 开始添加 RSS:", newRss.name, newRss.url);
  try {
    if (!form.value.rss) {
      form.value.rss = [];
    }
    form.value.rss.push({
      ...newRss,
      interval_minutes: Math.max(5, Math.min(1440, newRss.interval_minutes || 10)),
      downloader_id: newRss.downloader_id || undefined,
      download_path: newRss.download_path || "",
      filter_rule_ids: newRss.filter_rule_ids || [],
      pause_on_free_end: newRss.pause_on_free_end || false,
    });
    await sitesApi.save(siteName.value, form.value);
    // 重新加载数据以获取数据库中的真实 ID
    const data = await sitesApi.get(siteName.value);
    form.value = {
      ...data,
      rss: data.rss || [],
    };
    ElMessage.success("RSS 添加成功");
    rssDialogVisible.value = false;
  } catch (e: unknown) {
    // 添加失败时，移除刚添加的 RSS
    form.value.rss.pop();
    ElMessage.error((e as Error).message || "添加失败");
  } finally {
    addingRss.value = false;
  }
}

async function deleteRss(index: number) {
  const rss = form.value.rss[index];
  if (!rss) return;

  try {
    await ElMessageBox.confirm(`确定删除 RSS "${rss.name}"？`, "确认删除", {
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      type: "warning",
    });

    console.log("[RSS] 开始删除 RSS:", rss.name, "id:", rss.id);
    if (rss.id) {
      await sitesApi.deleteRss(siteName.value, rss.id);
      console.log("[RSS] 删除 RSS 成功:", rss.name);
      // 重新加载数据以确保数据一致性
      const data = await sitesApi.get(siteName.value);
      form.value = {
        ...data,
        rss: data.rss || [],
      };
    } else {
      // 没有 ID 的 RSS（未保存到数据库），直接从前端列表移除
      console.log("[RSS] RSS 无 ID，仅从前端移除:", rss.name);
      form.value.rss.splice(index, 1);
    }
    ElMessage.success("已删除");
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      console.error("[RSS] 删除 RSS 失败:", e);
      ElMessage.error((e as Error).message || "删除失败");
    }
  }
}

function openEditRssDialog(index: number) {
  const rss = form.value.rss[index];
  if (!rss) return;

  editingRssIndex.value = index;
  Object.assign(editingRss, {
    id: rss.id,
    name: rss.name,
    url: rss.url,
    category: rss.category || "",
    tag: rss.tag || "",
    interval_minutes: rss.interval_minutes || 10,
    downloader_id: rss.downloader_id || undefined,
    download_path: rss.download_path || "",
    filter_rule_ids: rss.filter_rule_ids || [],
    pause_on_free_end: rss.pause_on_free_end || false,
  });
  // 检查当前路径是否为预设目录，如果不是则启用自定义输入
  editRssUseCustomPath.value = rss.download_path
    ? !isPresetDirectory(rss.download_path, rss.downloader_id)
    : false;
  editRssDialogVisible.value = true;
}

async function updateRss() {
  if (!editingRss.name || !editingRss.url) {
    ElMessage.error("名称和链接为必填");
    return;
  }
  if (!editingRss.url.startsWith("http://") && !editingRss.url.startsWith("https://")) {
    ElMessage.error("链接必须以 http:// 或 https:// 开头");
    return;
  }

  const normalizedUrl = editingRss.url.trim().toLowerCase();
  const rssList = form.value.rss || [];
  const isDuplicate = rssList.some(
    (r, idx) => idx !== editingRssIndex.value && r.url.trim().toLowerCase() === normalizedUrl,
  );
  if (isDuplicate) {
    ElMessage.error("该 RSS 链接已存在，请勿重复添加");
    return;
  }

  updatingRss.value = true;
  console.log("[RSS] 开始更新 RSS:", editingRss.name, editingRss.url);

  try {
    // 更新本地数据
    form.value.rss[editingRssIndex.value] = {
      id: editingRss.id,
      name: editingRss.name,
      url: editingRss.url,
      category: editingRss.category,
      tag: editingRss.tag,
      interval_minutes: Math.max(5, Math.min(1440, editingRss.interval_minutes || 10)),
      downloader_id: editingRss.downloader_id || undefined,
      download_path: editingRss.download_path || "",
      filter_rule_ids: editingRss.filter_rule_ids || [],
      pause_on_free_end: editingRss.pause_on_free_end || false,
    };

    // 保存到服务器
    await sitesApi.save(siteName.value, form.value);
    ElMessage.success("RSS 更新成功");
    editRssDialogVisible.value = false;
  } catch (e: unknown) {
    console.error("[RSS] 更新 RSS 失败:", e);
    ElMessage.error((e as Error).message || "更新失败");
  } finally {
    // 无论成功或失败，都重新加载数据以确保数据一致性
    const data = await sitesApi.get(siteName.value);
    form.value = {
      ...data,
      rss: data.rss || [],
    };
    updatingRss.value = false;
  }
}

function goBack() {
  router.push("/sites");
}

function toggleNewRssCustomPath() {
  newRssUseCustomPath.value = !newRssUseCustomPath.value;
  if (!newRssUseCustomPath.value) {
    newRss.download_path = "";
  }
}

function toggleEditRssCustomPath() {
  editRssUseCustomPath.value = !editRssUseCustomPath.value;
  if (!editRssUseCustomPath.value) {
    editingRss.download_path = "";
  }
}

function getRowClassName({ row }: { row: RSSConfig }) {
  return row.is_example ? "example-row" : "";
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
              {{ form.enabled ? "已启用" : "未启用" }}
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
                {{ form.auth_method === "api_key" ? "API Key" : "Cookie" }}
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
            placeholder="从浏览器开发者工具中获取" />
        </el-form-item>

        <el-form-item v-if="form.auth_method === 'api_key'" label="API Key">
          <el-input
            v-model="form.api_key"
            type="password"
            show-password
            placeholder="从 M-Team 个人设置中获取" />
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

      <!-- 示例数据提示 -->
      <el-alert
        v-if="showingExamples"
        title="以下为示例配置，仅供参考"
        type="info"
        :closable="false"
        show-icon
        style="margin-bottom: 16px">
        <template #default>
          示例配置不会被执行，请点击"添加 RSS"按钮添加您自己的 RSS 订阅。
        </template>
      </el-alert>

      <el-table :data="displayRssList" style="width: 100%" :row-class-name="getRowClassName">
        <el-table-column type="index" label="序号" width="60" align="center" />
        <el-table-column label="名称" prop="name" min-width="120">
          <template #default="{ row }">
            <span :class="{ 'example-text': row.is_example }">{{ row.name }}</span>
            <el-tag v-if="row.is_example" type="info" size="small" style="margin-left: 4px">
              示例
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="链接" min-width="250">
          <template #default="{ row }">
            <el-tooltip :content="row.url" placement="top">
              <span class="url-cell" :class="{ 'example-text': row.is_example }">
                {{ row.url }}
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
        <el-table-column label="分类" prop="category" min-width="80">
          <template #default="{ row }">
            <span :class="{ 'example-text': row.is_example }">{{ row.category }}</span>
          </template>
        </el-table-column>
        <el-table-column label="标签" prop="tag" min-width="80">
          <template #default="{ row }">
            <span :class="{ 'example-text': row.is_example }">{{ row.tag }}</span>
          </template>
        </el-table-column>
        <el-table-column label="下载器" min-width="120">
          <template #default="{ row }">
            <template v-if="row.is_example">
              <el-tag type="info" size="small" class="example-text">默认</el-tag>
            </template>
            <template v-else>
              <el-tag v-if="row.downloader_id" type="primary" size="small">
                {{ downloaders.find((d) => d.id === row.downloader_id)?.name || "未知" }}
              </el-tag>
              <el-tag v-else type="info" size="small">默认</el-tag>
            </template>
          </template>
        </el-table-column>
        <el-table-column label="下载路径" min-width="150">
          <template #default="{ row }">
            <template v-if="row.is_example">
              <el-tag type="info" size="small" class="example-text">默认</el-tag>
            </template>
            <template v-else-if="row.download_path">
              <el-tooltip :content="row.download_path" placement="top">
                <el-tag type="success" size="small" class="path-tag">
                  {{ getPathDisplayName(row.download_path, row.downloader_id) }}
                </el-tag>
              </el-tooltip>
            </template>
            <template v-else>
              <el-tag type="info" size="small">默认</el-tag>
            </template>
          </template>
        </el-table-column>
        <el-table-column label="过滤规则" min-width="150">
          <template #default="{ row }">
            <template v-if="row.is_example">
              <el-tag type="info" size="small" class="example-text">无</el-tag>
            </template>
            <template v-else-if="row.filter_rule_ids && row.filter_rule_ids.length > 0">
              <el-tag
                v-for="ruleId in row.filter_rule_ids.slice(0, 2)"
                :key="ruleId"
                type="success"
                size="small"
                style="margin-right: 4px">
                {{ filterRules.find((r) => r.id === ruleId)?.name || `规则${ruleId}` }}
              </el-tag>
              <el-tag v-if="row.filter_rule_ids.length > 2" type="info" size="small">
                +{{ row.filter_rule_ids.length - 2 }}
              </el-tag>
            </template>
            <el-tag v-else type="info" size="small">无</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="间隔(分钟)" prop="interval_minutes" width="100" align="center">
          <template #default="{ row }">
            <span :class="{ 'example-text': row.is_example }">{{ row.interval_minutes }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" min-width="160" align="center">
          <template #default="{ row, $index }">
            <template v-if="row.is_example">
              <el-button type="info" size="small" disabled>示例</el-button>
            </template>
            <template v-else>
              <div class="action-buttons">
                <el-button
                  type="primary"
                  size="small"
                  :icon="'Edit'"
                  @click="openEditRssDialog($index)">
                  编辑
                </el-button>
                <el-button type="danger" size="small" :icon="'Delete'" @click="deleteRss($index)">
                  删除
                </el-button>
              </div>
            </template>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="displayRssList.length === 0" description="暂无 RSS 订阅" />
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
        <el-form-item label="下载器">
          <el-select
            v-model="newRss.downloader_id"
            placeholder="使用默认下载器"
            clearable
            style="width: 100%">
            <el-option
              v-for="dl in downloaders"
              :key="dl.id"
              :label="dl.name + (dl.is_default ? ' (默认)' : '') + (!dl.enabled ? ' (未启用)' : '')"
              :value="dl.id"
              :disabled="!dl.enabled" />
          </el-select>
          <div class="form-tip">
            不选择则使用默认下载器。灰色选项表示下载器未启用，请先在下载器管理中启用。
          </div>
        </el-form-item>
        <el-form-item label="间隔(分钟)">
          <el-input-number v-model="newRss.interval_minutes" :min="5" :max="1440" />
        </el-form-item>
        <el-form-item label="过滤规则">
          <el-select
            v-model="newRss.filter_rule_ids"
            multiple
            placeholder="选择要应用的过滤规则"
            style="width: 100%">
            <el-option
              v-for="rule in filterRules"
              :key="rule.id"
              :label="rule.name"
              :value="rule.id" />
          </el-select>
          <div class="form-tip">选择要应用于此 RSS 订阅的过滤规则，不选择则不进行过滤下载</div>
        </el-form-item>
        <el-form-item label="免费结束暂停">
          <el-switch v-model="newRss.pause_on_free_end" />
          <div class="form-tip">启用后，免费期结束时如果下载未完成，系统将自动暂停任务</div>
        </el-form-item>
        <el-form-item label="下载路径">
          <div class="path-selector">
            <el-select
              v-if="!newRssUseCustomPath"
              v-model="newRss.download_path"
              placeholder="使用下载器默认路径"
              clearable
              style="flex: 1">
              <el-option value="" label="使用下载器默认路径" />
              <el-option
                v-for="dir in getDirectoriesForDownloader(newRss.downloader_id)"
                :key="dir.id"
                :label="`${dir.alias || dir.path}${dir.is_default ? ' (默认)' : ''}`"
                :value="dir.path" />
            </el-select>
            <el-input
              v-else
              v-model="newRss.download_path"
              placeholder="输入自定义路径，如: /downloads/movies"
              style="flex: 1" />
            <el-button
              :type="newRssUseCustomPath ? 'primary' : 'default'"
              :icon="newRssUseCustomPath ? 'Select' : 'Edit'"
              @click="toggleNewRssCustomPath">
              {{ newRssUseCustomPath ? "选择预设" : "自定义" }}
            </el-button>
          </div>
          <div class="form-tip">
            <template v-if="getDirectoriesForDownloader(newRss.downloader_id).length > 0">
              可选择下载器预设的目录，或点击"自定义"手动输入路径
            </template>
            <template v-else>
              当前下载器未设置目录，可点击"自定义"手动输入路径，或留空使用默认路径
            </template>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="rssDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="addingRss" @click="addRss">添加</el-button>
      </template>
    </el-dialog>

    <!-- 编辑 RSS 对话框 -->
    <el-dialog v-model="editRssDialogVisible" title="编辑 RSS 订阅" width="500px">
      <el-form :model="editingRss" label-width="100px">
        <el-form-item label="名称" required>
          <el-input v-model="editingRss.name" placeholder="如：CMCT电视剧" />
        </el-form-item>
        <el-form-item label="链接" required>
          <el-input v-model="editingRss.url" placeholder="https://..." />
        </el-form-item>
        <el-form-item label="分类">
          <el-input v-model="editingRss.category" placeholder="Tv" />
        </el-form-item>
        <el-form-item label="标签">
          <el-input v-model="editingRss.tag" placeholder="CMCT" />
        </el-form-item>
        <el-form-item label="下载器">
          <el-select
            v-model="editingRss.downloader_id"
            placeholder="使用默认下载器"
            clearable
            style="width: 100%">
            <el-option
              v-for="dl in downloaders"
              :key="dl.id"
              :label="dl.name + (dl.is_default ? ' (默认)' : '') + (!dl.enabled ? ' (未启用)' : '')"
              :value="dl.id"
              :disabled="!dl.enabled" />
          </el-select>
          <div class="form-tip">
            不选择则使用默认下载器。灰色选项表示下载器未启用，请先在下载器管理中启用。
          </div>
        </el-form-item>
        <el-form-item label="间隔(分钟)">
          <el-input-number v-model="editingRss.interval_minutes" :min="5" :max="1440" />
        </el-form-item>
        <el-form-item label="过滤规则">
          <el-select
            v-model="editingRss.filter_rule_ids"
            multiple
            placeholder="选择要应用的过滤规则"
            style="width: 100%">
            <el-option
              v-for="rule in filterRules"
              :key="rule.id"
              :label="rule.name"
              :value="rule.id" />
          </el-select>
          <div class="form-tip">选择要应用于此 RSS 订阅的过滤规则，不选择则不进行过滤下载</div>
        </el-form-item>
        <el-form-item label="免费结束暂停">
          <el-switch v-model="editingRss.pause_on_free_end" />
          <div class="form-tip">启用后，免费期结束时如果下载未完成，系统将自动暂停任务</div>
        </el-form-item>
        <el-form-item label="下载路径">
          <div class="path-selector">
            <el-select
              v-if="!editRssUseCustomPath"
              v-model="editingRss.download_path"
              placeholder="使用下载器默认路径"
              clearable
              style="flex: 1">
              <el-option value="" label="使用下载器默认路径" />
              <el-option
                v-for="dir in getDirectoriesForDownloader(editingRss.downloader_id)"
                :key="dir.id"
                :label="`${dir.alias || dir.path}${dir.is_default ? ' (默认)' : ''}`"
                :value="dir.path" />
            </el-select>
            <el-input
              v-else
              v-model="editingRss.download_path"
              placeholder="输入自定义路径，如: /downloads/movies"
              style="flex: 1" />
            <el-button
              :type="editRssUseCustomPath ? 'primary' : 'default'"
              :icon="editRssUseCustomPath ? 'Select' : 'Edit'"
              @click="toggleEditRssCustomPath">
              {{ editRssUseCustomPath ? "选择预设" : "自定义" }}
            </el-button>
          </div>
          <div class="form-tip">
            <template v-if="getDirectoriesForDownloader(editingRss.downloader_id).length > 0">
              可选择下载器预设的目录，或点击"自定义"手动输入路径
            </template>
            <template v-else>
              当前下载器未设置目录，可点击"自定义"手动输入路径，或留空使用默认路径
            </template>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editRssDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="updatingRss" @click="updateRss">保存</el-button>
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
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.action-buttons {
  display: flex;
  justify-content: center;
  gap: 8px;
}

.form-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}

.path-selector {
  display: flex;
  gap: 8px;
  width: 100%;
}

.path-tag {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 示例数据样式 - 使用 CSS 变量适配明暗主题 */
.example-text {
  color: var(--el-text-color-placeholder);
  font-style: italic;
}

:deep(.example-row) {
  background-color: var(--el-fill-color-lighter);
  border-left: 3px dashed var(--el-border-color);
}

:deep(.example-row > td) {
  background-color: var(--el-fill-color-lighter) !important;
}

:deep(.example-row:hover > td) {
  background-color: var(--el-fill-color-light) !important;
}

/* 示例标签样式 */
:deep(.example-row .el-tag--info) {
  background-color: var(--el-fill-color);
  border-color: var(--el-border-color-lighter);
  color: var(--el-text-color-secondary);
}

/* 示例提示框样式增强 */
:deep(.el-alert--info) {
  border: 1px dashed var(--el-color-info-light-5);
}
</style>
