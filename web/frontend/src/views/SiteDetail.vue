<script setup lang="ts">
import {
  chatopsApi,
  downloaderDirectoriesApi,
  type DownloaderDirectory,
  downloadersApi,
  type DownloaderSetting,
  type FilterRule,
  filterRulesApi,
  type NotificationConfig,
  type RSSConfig,
  type SiteConfig,
  sitesApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
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
const availableConfs = ref<NotificationConfig[]>([]);
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
  passkey: "",
  upload_limit_kbs: 0,
  download_limit_kbs: 0,
  seeding_capacity_gb: 0,
  rss: [],
});
const savedCookieHidden = computed(() => form.value.has_cookie === true && !form.value.cookie);

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

// API Key 输入框占位符（根据站点显示不同提示）
const apiKeyPlaceholder = computed(() => {
  const name = siteName.value.toLowerCase();
  if (name === "hddolby") {
    return "从站点 RSS 订阅页面获取 RSS Key";
  }
  return "从 M-Team 个人设置中获取";
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
  filter_mode: "",
  notify_mode: "",
  notify_conf_ids: "[]",
  max_notifications_per_hour: 100,
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
  filter_mode: "",
  notify_mode: "",
  notify_conf_ids: "[]",
  max_notifications_per_hour: 100,
});
const editingRssIndex = ref(-1);
const updatingRss = ref(false);

function parseConfIDs(raw: string | undefined): number[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((n) => typeof n === "number") : [];
  } catch {
    return [];
  }
}

const newRssConfIDs = computed<number[]>({
  get: () => parseConfIDs(newRss.notify_conf_ids),
  set: (v) => {
    newRss.notify_conf_ids = JSON.stringify(v || []);
  },
});

const editingRssConfIDs = computed<number[]>({
  get: () => parseConfIDs(editingRss.notify_conf_ids),
  set: (v) => {
    editingRss.notify_conf_ids = JSON.stringify(v || []);
  },
});

onMounted(async () => {
  loading.value = true;
  try {
    // 并行加载站点配置、下载器列表、过滤规则列表、下载器目录、通知通道列表
    const [siteData, downloaderList, filterRuleList, directoriesData, confList] = await Promise.all(
      [
        sitesApi.get(siteName.value),
        downloadersApi.list(),
        filterRulesApi.list(),
        downloaderDirectoriesApi.listAll(),
        chatopsApi.notifications.list().catch(() => [] as NotificationConfig[]),
      ],
    );
    form.value = siteData;
    downloaders.value = downloaderList; // 显示所有下载器，不过滤
    filterRules.value = filterRuleList.filter((r) => r.enabled); // 只显示启用的过滤规则
    downloaderDirectories.value = directoriesData;
    availableConfs.value = (confList || []).filter((c) => c.enabled);
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
    // 根据认证方式清空互斥字段，避免后端校验失败
    const payload = { ...form.value };
    if (payload.auth_method === "api_key") {
      payload.cookie = "";
    } else if (payload.auth_method === "cookie") {
      payload.api_key = "";
    }
    // cookie_and_api_key: keep both fields
    await sitesApi.save(siteName.value, payload);
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
    filter_mode: "",
    notify_mode: "",
    notify_conf_ids: "[]",
    max_notifications_per_hour: 100,
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
      filter_mode: newRss.filter_mode || "",
      notify_mode: newRss.notify_mode || "",
      notify_conf_ids: newRss.notify_conf_ids || "[]",
      max_notifications_per_hour: newRss.max_notifications_per_hour ?? 100,
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
    filter_mode: rss.filter_mode || "",
    notify_mode: rss.notify_mode || "",
    notify_conf_ids: rss.notify_conf_ids || "[]",
    max_notifications_per_hour: rss.max_notifications_per_hour ?? 100,
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
      filter_mode: editingRss.filter_mode || "",
      notify_mode: editingRss.notify_mode || "",
      notify_conf_ids: editingRss.notify_conf_ids || "[]",
      max_notifications_per_hour: editingRss.max_notifications_per_hour ?? 100,
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

const authMethodLabel = computed(() => {
  switch (form.value.auth_method) {
    case "api_key":
      return "API Key";
    case "cookie_and_api_key":
      return "Cookie + API Key";
    case "passkey":
      return "Passkey";
    default:
      return "Cookie";
  }
});

/* 工具条上的一行摘要：认证方式 + RSS 条数，省掉两个只放一个数字的面板 */
const toolbarNote = computed(() => {
  const count = (form.value.rss || []).length;
  return `认证 ${authMethodLabel.value} · RSS ${count} 条`;
});

function downloaderNameOf(id: number | undefined): string {
  if (!id) return "默认";
  return downloaders.value.find((d) => d.id === id)?.name || "未知";
}

function ruleNameOf(id: number): string {
  return filterRules.value.find((r) => r.id === id)?.name || `规则 ${id}`;
}
</script>

<template>
  <div class="site-detail-page">
    <PtToolbar standalone :note="toolbarNote">
      <el-button size="small" @click="goBack">
        <PtIcon name="arrow-left" :size="14" /><span>站点列表</span>
      </el-button>
      <span class="site-id">{{ siteName }}</span>
      <PtStatusPill :tone="form.enabled ? 'ok' : 'neutral'" dot size="sm">
        {{ form.enabled ? "已启用" : "未启用" }}
      </PtStatusPill>

      <template #right>
        <el-button type="primary" size="small" :loading="saving" @click="save">
          <PtIcon name="save" :size="14" /><span>保存配置</span>
        </el-button>
      </template>
    </PtToolbar>

    <div v-if="form.unavailable" class="pt-note pt-note--warn">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>{{ form.unavailable_reason || "该站点暂时不可用" }}</span>
    </div>

    <PtPanel v-loading="loading" title="站点配置" icon="sliders-horizontal" padding="none">
      <el-form :model="form" label-position="top" class="pt-form settings-form">
        <div class="pt-strip">
          <PtIcon name="globe" :size="13" />
          <span>基本信息</span>
          <span class="pt-strip__end">{{ authMethodLabel }}</span>
        </div>
        <div class="settings-body">
          <el-form-item label="启用站点">
            <el-tooltip
              :content="form.unavailable ? form.unavailable_reason : ''"
              :disabled="!form.unavailable"
              placement="top">
              <span class="sw">
                <el-switch v-model="form.enabled" :disabled="form.unavailable" />
              </span>
            </el-tooltip>
            <div class="field-tip">停用后该站点的 RSS 任务与登录探测都会跳过</div>
          </el-form-item>

          <el-form-item v-if="form.urls && form.urls.length > 0" label="站点地址">
            <div class="urls">
              <a v-for="url in form.urls" :key="url" :href="url" target="_blank" rel="noopener">
                <PtIcon name="external-link" :size="12" />
                <span>{{ url }}</span>
              </a>
            </div>
            <div class="field-tip">地址由内置站点定义提供，切换镜像请更新站点定义</div>
          </el-form-item>
        </div>

        <div class="pt-strip">
          <PtIcon name="key-round" :size="13" />
          <span>认证凭据</span>
        </div>
        <div class="settings-body">
          <el-form-item v-if="form.auth_method === 'cookie'" label="Cookie">
            <div v-if="savedCookieHidden" class="pt-note pt-note--ok cred-note">
              <PtIcon name="shield" :size="14" class="pt-note__icon" />
              <span>Cookie 已保存，出于安全原因不会回显；留空保存不会覆盖已存的值</span>
            </div>
            <el-input
              v-model="form.cookie"
              type="textarea"
              :rows="3"
              placeholder="从浏览器开发者工具中获取；已保存 Cookie 时可留空" />
          </el-form-item>

          <template v-if="form.auth_method === 'api_key'">
            <el-form-item label="API Key">
              <el-input
                v-model="form.api_key"
                type="password"
                show-password
                :placeholder="apiKeyPlaceholder" />
              <div v-if="siteName.toLowerCase() === 'hddolby'" class="field-tip">
                前往站点「<a
                  href="https://www.hddolby.com/getrss.php"
                  target="_blank"
                  rel="noopener">
                  控制面板 - RSS 订阅 </a
                >」页面，复制「本次加密密钥（RssKEY）」
              </div>
            </el-form-item>
            <el-form-item label="API URL">
              <el-input :model-value="form.api_url" disabled />
              <div class="field-tip">由站点定义提供，不可修改</div>
            </el-form-item>
          </template>

          <template v-if="form.auth_method === 'cookie_and_api_key'">
            <el-form-item label="Cookie">
              <div v-if="savedCookieHidden" class="pt-note pt-note--ok cred-note">
                <PtIcon name="shield" :size="14" class="pt-note__icon" />
                <span>Cookie 已保存，出于安全原因不会回显；留空保存不会覆盖已存的值</span>
              </div>
              <el-input
                v-model="form.cookie"
                type="textarea"
                :rows="3"
                placeholder="从浏览器开发者工具中获取（用于获取时魔等信息）；已保存 Cookie 时可留空" />
            </el-form-item>
            <el-form-item label="API Key / RSS Key">
              <el-input
                v-model="form.api_key"
                type="password"
                show-password
                :placeholder="apiKeyPlaceholder" />
              <div v-if="siteName.toLowerCase() === 'hddolby'" class="field-tip">
                前往站点「<a
                  href="https://www.hddolby.com/getrss.php"
                  target="_blank"
                  rel="noopener">
                  控制面板 - RSS 订阅 </a
                >」页面，复制「本次加密密钥（RssKEY）」
              </div>
            </el-form-item>
            <el-form-item label="API URL">
              <el-input :model-value="form.api_url" disabled />
              <div class="field-tip">由站点定义提供，不可修改</div>
            </el-form-item>
          </template>

          <el-form-item v-if="form.auth_method === 'passkey'" label="Passkey">
            <el-input
              v-model="form.passkey"
              type="password"
              show-password
              placeholder="从站点个人设置中获取 Passkey" />
            <div class="field-tip">Passkey 用于 RSS 订阅认证，从站点个人设置页面获取</div>
          </el-form-item>
        </div>

        <div class="pt-strip">
          <PtIcon name="gauge" :size="13" />
          <span>限速与容量</span>
          <span class="pt-strip__end">0 = 不限制</span>
        </div>
        <div class="settings-body">
          <div class="field-row">
            <el-form-item label="上传限速（KB/s）">
              <el-input-number
                v-model="form.upload_limit_kbs"
                :min="0"
                :max="1048576"
                :step="128"
                controls-position="right"
                style="width: 100%" />
              <div class="field-tip">推送到下载器的每个种子都会套上这个上传上限</div>
            </el-form-item>
            <el-form-item label="下载限速（KB/s）">
              <el-input-number
                v-model="form.download_limit_kbs"
                :min="0"
                :max="1048576"
                :step="128"
                controls-position="right"
                style="width: 100%" />
              <div class="field-tip">同上，0 表示沿用下载器的全局设置</div>
            </el-form-item>
          </div>
          <el-form-item label="刷流容量上限（GB）">
            <el-input-number
              v-model="form.seeding_capacity_gb"
              :min="0"
              :step="10"
              controls-position="right"
              style="width: 100%" />
            <div class="field-tip">该站点做种总量到顶后就不再推新种，避免把盘塞满</div>
          </el-form-item>
        </div>
      </el-form>
    </PtPanel>

    <PtPanel title="RSS 订阅" icon="rss" :count="`${(form.rss || []).length} 条`">
      <template #actions>
        <el-button type="primary" size="small" @click="openAddRssDialog">
          <PtIcon name="plus" :size="14" /><span>添加 RSS</span>
        </el-button>
      </template>

      <div class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>
          没挂过滤规则时，RSS 订阅<strong>只会自动下载免费种子</strong>。
          单纯刷流不用配规则；要追剧或抓非免费资源，才需要建规则并关掉「仅免费」。
        </span>
      </div>

      <div v-if="showingExamples" class="pt-note">
        <PtIcon name="info" :size="14" class="pt-note__icon" />
        <span>下面是示例配置，只作参考、不会被执行。点「添加 RSS」建自己的订阅。</span>
      </div>

      <div v-if="displayRssList.length > 0" class="rss-grid">
        <article
          v-for="(row, $index) in displayRssList"
          :key="row.id || `${row.url}-${$index}`"
          class="rss"
          :class="{ 'is-example': row.is_example }">
          <header class="rss__head">
            <span class="rss__name">{{ row.name }}</span>
            <PtTag v-if="row.is_example">示例</PtTag>
            <span class="rss__int">
              <PtIcon name="timer" :size="12" />
              {{ row.interval_minutes }} 分钟
            </span>
          </header>

          <el-tooltip :content="row.url" placement="top">
            <p class="rss__url">{{ row.url }}</p>
          </el-tooltip>

          <div v-if="row.category || row.tag || row.pause_on_free_end" class="rss__tags">
            <PtTag v-if="row.category">{{ row.category }}</PtTag>
            <PtTag v-if="row.tag">{{ row.tag }}</PtTag>
            <PtStatusPill v-if="row.pause_on_free_end" tone="warn" size="sm">
              免费结束暂停
            </PtStatusPill>
          </div>

          <dl class="rss__facts">
            <div class="fact">
              <dt>下载器</dt>
              <dd>{{ row.is_example ? "默认" : downloaderNameOf(row.downloader_id) }}</dd>
            </div>
            <div class="fact">
              <dt>下载路径</dt>
              <dd>
                <el-tooltip
                  v-if="!row.is_example && row.download_path"
                  :content="row.download_path"
                  placement="top">
                  <span>{{ getPathDisplayName(row.download_path, row.downloader_id) }}</span>
                </el-tooltip>
                <span v-else>默认</span>
              </dd>
            </div>
            <div class="fact fact--wide">
              <dt>过滤规则</dt>
              <dd>
                <template
                  v-if="!row.is_example && row.filter_rule_ids && row.filter_rule_ids.length > 0">
                  <span class="rules">
                    <PtTag v-for="ruleId in row.filter_rule_ids.slice(0, 3)" :key="ruleId">
                      {{ ruleNameOf(ruleId) }}
                    </PtTag>
                    <PtTag v-if="row.filter_rule_ids.length > 3">
                      +{{ row.filter_rule_ids.length - 3 }}
                    </PtTag>
                  </span>
                </template>
                <span v-else-if="row.is_example">无</span>
                <span v-else class="only-free">仅免费</span>
              </dd>
            </div>
          </dl>

          <footer class="rss__acts">
            <span v-if="row.is_example" class="rss__hint">示例配置不可编辑</span>
            <template v-else>
              <el-button link type="primary" size="small" @click="openEditRssDialog($index)">
                <PtIcon name="pencil" :size="14" /><span>编辑</span>
              </el-button>
              <el-button link type="danger" size="small" @click="deleteRss($index)">
                <PtIcon name="trash-2" :size="14" /><span>删除</span>
              </el-button>
            </template>
          </footer>
        </article>
      </div>

      <PtDataState
        v-else
        state="empty"
        title="还没有 RSS 订阅"
        sub="订阅是自动下载的入口，先加一条 RSS 链接" />
    </PtPanel>

    <el-dialog
      v-model="rssDialogVisible"
      class="pt-dialog"
      title="添加 RSS 订阅"
      width="580px"
      align-center>
      <el-form :model="newRss" label-position="top" class="pt-form" @submit.prevent>
        <div class="field-head">基础</div>
        <div class="field-row">
          <el-form-item label="名称" required>
            <el-input v-model="newRss.name" placeholder="如：CMCT 电视剧" />
          </el-form-item>
          <el-form-item label="检查间隔（分钟）">
            <el-input-number
              v-model="newRss.interval_minutes"
              :min="5"
              :max="1440"
              controls-position="right"
              style="width: 100%" />
          </el-form-item>
        </div>
        <el-form-item label="链接" required>
          <el-input v-model="newRss.url" placeholder="https://..." />
        </el-form-item>
        <div class="field-row">
          <el-form-item label="分类">
            <el-input v-model="newRss.category" placeholder="Tv" />
          </el-form-item>
          <el-form-item label="标签">
            <el-input v-model="newRss.tag" placeholder="CMCT" />
          </el-form-item>
        </div>

        <div class="field-head">下载去向</div>
        <el-form-item label="下载器">
          <el-select
            v-model="newRss.downloader_id"
            placeholder="使用默认下载器"
            clearable
            style="width: 100%">
            <el-option
              v-for="dl in downloaders"
              :key="dl.id"
              :label="
                dl.name + (dl.is_default ? '（默认）' : '') + (!dl.enabled ? '（未启用）' : '')
              "
              :value="dl.id"
              :disabled="!dl.enabled" />
          </el-select>
          <div class="field-tip">留空用默认下载器；灰色项表示该下载器未启用</div>
        </el-form-item>
        <el-form-item label="下载路径">
          <div class="path-pick">
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
                :label="`${dir.alias || dir.path}${dir.is_default ? '（默认）' : ''}`"
                :value="dir.path" />
            </el-select>
            <el-input
              v-else
              v-model="newRss.download_path"
              placeholder="如 /downloads/movies"
              style="flex: 1" />
            <el-button
              :type="newRssUseCustomPath ? 'primary' : 'default'"
              @click="toggleNewRssCustomPath">
              <PtIcon :name="newRssUseCustomPath ? 'check' : 'pencil'" :size="14" />
              <span>{{ newRssUseCustomPath ? "选预设" : "自定义" }}</span>
            </el-button>
          </div>
          <div class="field-tip">
            <template v-if="getDirectoriesForDownloader(newRss.downloader_id).length > 0">
              可以挑下载器里预设的目录，也可以切到「自定义」手输
            </template>
            <template v-else>当前下载器没设目录，切「自定义」手输，或留空用默认路径</template>
          </div>
        </el-form-item>

        <div class="field-head">过滤与暂停</div>
        <el-form-item label="过滤规则">
          <el-select
            v-model="newRss.filter_rule_ids"
            multiple
            placeholder="不选则不做规则过滤"
            style="width: 100%">
            <el-option
              v-for="rule in filterRules"
              :key="rule.id"
              :label="rule.name"
              :value="rule.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="下载模式">
          <el-radio-group v-model="newRss.filter_mode" size="small">
            <el-radio-button value="">跟随全局</el-radio-button>
            <el-radio-button value="auto_free">智能（推荐）</el-radio-button>
            <el-radio-button value="filter_only">仅过滤规则</el-radio-button>
            <el-radio-button value="free_only">仅免费</el-radio-button>
          </el-radio-group>
          <div class="field-tip">
            <strong>智能</strong>：没规则就只下免费种，有规则就只下命中的。全局大小上限始终生效。
          </div>
        </el-form-item>
        <el-form-item label="免费结束暂停">
          <span class="sw"><el-switch v-model="newRss.pause_on_free_end" /></span>
          <div class="field-tip">免费期结束时若还没下完，自动暂停任务</div>
        </el-form-item>

        <div class="field-head">通知</div>
        <el-form-item label="通知模式">
          <el-radio-group v-model="newRss.notify_mode" size="small">
            <el-radio-button value="">不通知</el-radio-button>
            <el-radio-button value="all">全部新种（简略）</el-radio-button>
            <el-radio-button value="filtered">只通知匹配的（详细）</el-radio-button>
            <el-radio-button value="both">都通知 + 匹配的给详细</el-radio-button>
          </el-radio-group>
          <div class="field-tip">
            「简略」只用 RSS 标题和链接，不额外拉站点详情；「详细」拉详情后按
            <code>filter_rules.purpose IN ('notify','both')</code> 匹配，命中才发；
            两路都开时同一种子会合并成详细版，不会重复通知。
          </div>
        </el-form-item>
        <template v-if="newRss.notify_mode">
          <el-form-item label="通知通道">
            <el-select
              v-model="newRssConfIDs"
              multiple
              collapse-tags
              collapse-tags-tooltip
              style="width: 100%"
              placeholder="选择推送的 ChatOps 通道（空 = 不通知）">
              <el-option
                v-for="c in availableConfs"
                :key="c.id"
                :label="`${c.name}（${c.channel_type}）`"
                :value="c.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="每小时最多">
            <el-input-number
              v-model="newRss.max_notifications_per_hour"
              :min="0"
              :max="10000"
              controls-position="right"
              style="width: 100%" />
            <div class="field-tip">超出后余下的通知标记为 throttled 直接丢弃；0 = 不限制</div>
          </el-form-item>
        </template>
      </el-form>

      <template #footer>
        <el-button @click="rssDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="addingRss" @click="addRss">添加</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="editRssDialogVisible"
      class="pt-dialog"
      title="编辑 RSS 订阅"
      width="580px"
      align-center>
      <el-form :model="editingRss" label-position="top" class="pt-form" @submit.prevent>
        <div class="field-head">基础</div>
        <div class="field-row">
          <el-form-item label="名称" required>
            <el-input v-model="editingRss.name" placeholder="如：CMCT 电视剧" />
          </el-form-item>
          <el-form-item label="检查间隔（分钟）">
            <el-input-number
              v-model="editingRss.interval_minutes"
              :min="5"
              :max="1440"
              controls-position="right"
              style="width: 100%" />
          </el-form-item>
        </div>
        <el-form-item label="链接" required>
          <el-input v-model="editingRss.url" placeholder="https://..." />
        </el-form-item>
        <div class="field-row">
          <el-form-item label="分类">
            <el-input v-model="editingRss.category" placeholder="Tv" />
          </el-form-item>
          <el-form-item label="标签">
            <el-input v-model="editingRss.tag" placeholder="CMCT" />
          </el-form-item>
        </div>

        <div class="field-head">下载去向</div>
        <el-form-item label="下载器">
          <el-select
            v-model="editingRss.downloader_id"
            placeholder="使用默认下载器"
            clearable
            style="width: 100%">
            <el-option
              v-for="dl in downloaders"
              :key="dl.id"
              :label="
                dl.name + (dl.is_default ? '（默认）' : '') + (!dl.enabled ? '（未启用）' : '')
              "
              :value="dl.id"
              :disabled="!dl.enabled" />
          </el-select>
          <div class="field-tip">留空用默认下载器；灰色项表示该下载器未启用</div>
        </el-form-item>
        <el-form-item label="下载路径">
          <div class="path-pick">
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
                :label="`${dir.alias || dir.path}${dir.is_default ? '（默认）' : ''}`"
                :value="dir.path" />
            </el-select>
            <el-input
              v-else
              v-model="editingRss.download_path"
              placeholder="如 /downloads/movies"
              style="flex: 1" />
            <el-button
              :type="editRssUseCustomPath ? 'primary' : 'default'"
              @click="toggleEditRssCustomPath">
              <PtIcon :name="editRssUseCustomPath ? 'check' : 'pencil'" :size="14" />
              <span>{{ editRssUseCustomPath ? "选预设" : "自定义" }}</span>
            </el-button>
          </div>
          <div class="field-tip">
            <template v-if="getDirectoriesForDownloader(editingRss.downloader_id).length > 0">
              可以挑下载器里预设的目录，也可以切到「自定义」手输
            </template>
            <template v-else>当前下载器没设目录，切「自定义」手输，或留空用默认路径</template>
          </div>
        </el-form-item>

        <div class="field-head">过滤与暂停</div>
        <el-form-item label="过滤规则">
          <el-select
            v-model="editingRss.filter_rule_ids"
            multiple
            placeholder="不选则不做规则过滤"
            style="width: 100%">
            <el-option
              v-for="rule in filterRules"
              :key="rule.id"
              :label="rule.name"
              :value="rule.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="下载模式">
          <el-radio-group v-model="editingRss.filter_mode" size="small">
            <el-radio-button value="">跟随全局</el-radio-button>
            <el-radio-button value="auto_free">智能（推荐）</el-radio-button>
            <el-radio-button value="filter_only">仅过滤规则</el-radio-button>
            <el-radio-button value="free_only">仅免费</el-radio-button>
          </el-radio-group>
          <div class="field-tip">
            <strong>智能</strong>：没规则就只下免费种，有规则就只下命中的。全局大小上限始终生效。
          </div>
        </el-form-item>
        <el-form-item label="免费结束暂停">
          <span class="sw"><el-switch v-model="editingRss.pause_on_free_end" /></span>
          <div class="field-tip">免费期结束时若还没下完，自动暂停任务</div>
        </el-form-item>

        <div class="field-head">通知</div>
        <el-form-item label="通知模式">
          <el-radio-group v-model="editingRss.notify_mode" size="small">
            <el-radio-button value="">不通知</el-radio-button>
            <el-radio-button value="all">全部新种（简略）</el-radio-button>
            <el-radio-button value="filtered">只通知匹配的（详细）</el-radio-button>
            <el-radio-button value="both">都通知 + 匹配的给详细</el-radio-button>
          </el-radio-group>
          <div class="field-tip">
            「简略」只用 RSS 标题和链接，不额外拉站点详情；「详细」拉详情后按
            <code>filter_rules.purpose IN ('notify','both')</code> 匹配，命中才发；
            两路都开时同一种子会合并成详细版，不会重复通知。
          </div>
        </el-form-item>
        <template v-if="editingRss.notify_mode">
          <el-form-item label="通知通道">
            <el-select
              v-model="editingRssConfIDs"
              multiple
              collapse-tags
              collapse-tags-tooltip
              style="width: 100%"
              placeholder="选择推送的 ChatOps 通道（空 = 不通知）">
              <el-option
                v-for="c in availableConfs"
                :key="c.id"
                :label="`${c.name}（${c.channel_type}）`"
                :value="c.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="每小时最多">
            <el-input-number
              v-model="editingRss.max_notifications_per_hour"
              :min="0"
              :max="10000"
              controls-position="right"
              style="width: 100%" />
            <div class="field-tip">超出后余下的通知标记为 throttled 直接丢弃；0 = 不限制</div>
          </el-form-item>
        </template>
      </el-form>

      <template #footer>
        <el-button @click="editRssDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="updatingRss" @click="updateRss">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.site-detail-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
}

/* 站点 id 在定义里是小写，首字母大写才像个名字 */
.site-id {
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t1);
  text-transform: capitalize;
}

/* 第一条区块条紧贴面板页头，两条发丝线会叠成 2px */
.settings-form > .pt-strip:first-child {
  border-top: 0;
}

.settings-body {
  padding: var(--pt-pad) var(--pt-pad) 0;
}

/* el-tooltip 要一个能接事件的宿主，disabled 的开关自己不派发 mouseenter */
.sw {
  display: inline-flex;
}

.cred-note {
  margin-bottom: var(--pt-space-2);
}

.urls {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.urls a {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-sm);
  color: var(--pt-p);
  text-decoration: none;
}

.urls a:hover {
  text-decoration: underline;
}

/* 自定义路径的输入框和「切换来源」按钮必须同一行，不然读起来像两个独立字段 */
.path-pick {
  display: flex;
  gap: var(--pt-space-2);
}

/*
 * RSS 用卡片而不是表格：一条订阅要同时交代链接、去向、规则和间隔，
 * 摊成表格会有一半列常年是「默认」，卡片能把这些折进两行事实里。
 */
.rss-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: var(--pt-space-3);
}

.rss {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-space-3);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
  transition: border-color var(--pt-transition-fast);
}

.rss:hover {
  border-color: var(--pt-p);
}

/* 示例卡整体压暗并换成虚线框：一眼看出它不参与调度 */
.rss.is-example {
  border-style: dashed;
  opacity: 0.72;
}

.rss.is-example:hover {
  border-color: var(--pt-border);
}

.rss__head {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
}

.rss__name {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rss__int {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  margin-left: auto;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}

.rss__url {
  margin: 0;
  overflow: hidden;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rss__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.rss__facts {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--pt-space-2);
  margin: 0;
  padding-top: var(--pt-space-2);
  border-top: 1px solid var(--pt-border);
}

.fact--wide {
  grid-column: 1 / -1;
}

.fact dt {
  margin-bottom: 2px;
  font-size: var(--pt-fz-label);
  color: var(--pt-t4);
}

.fact dd {
  margin: 0;
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rules {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

/* 「仅免费」是默认落点而不是用户选的规则，用 warn 色提示它是兜底行为 */
.only-free {
  color: var(--pt-warn);
}

.rss__acts {
  display: flex;
  gap: var(--pt-space-2);
  justify-content: flex-end;
  padding-top: var(--pt-space-2);
  border-top: 1px solid var(--pt-border);
}

.rss__hint {
  font-size: var(--pt-fz-label);
  color: var(--pt-t4);
}
</style>
