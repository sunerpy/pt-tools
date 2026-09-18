<script setup lang="ts">
import {
  downloaderDirectoriesApi,
  type DownloaderDirectory,
  type DownloaderHealthResponse,
  downloadersApi,
  type DownloaderSetting,
  dynamicSitesApi,
  globalApi,
  type SiteDownloaderSummaryItem,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtBreakdown, { type BreakdownRow } from "@/components/ui/PtBreakdown.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref } from "vue";

/** 手机上三张表都退化成行卡（设计文档 §9），不做横向滚动表格 */
const isMobile = useIsMobile();

const saving = ref(false);
const showDialog = ref(false);
const editMode = ref(false);
const healthTimeoutMs = 5000;

const downloaders = ref<DownloaderSetting[]>([]);
const healthStatus = ref<Record<number, DownloaderHealthResponse>>({});
/**
 * 连通性探测**本身**失败（超时 / 请求没发出去 / 非 2xx）的下载器 id。
 *
 * 和「下载器回报 is_healthy: false」要分开：后者是拿到了结论，前者是没拿到。
 * 这一份就是这张表的 partial 数据源 —— 列表本体拿到了，附带的连通性只探到一部分。
 */
const healthFailedIds = ref<number[]>([]);

// 目录管理相关
const showDirDialog = ref(false);
const currentDownloader = ref<DownloaderSetting | null>(null);
const directories = ref<DownloaderDirectory[]>([]);
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

/*
 * 六态状态机（设计文档 §5），这一页三张表各一份。
 *
 * 以前三处都是「一个 loading ref + 失败弹个 toast」：toast 两秒就没了，表格停在
 * 「还没有下载器」上 —— 用户看到的是「库里是空的」，真相是请求失败了。401/403 还会
 * 被画成普通失败，导致用户一直点重试。错误必须留在页面上，且要分出无权访问。
 *
 * 主表的 partial 来自连通性探测：列表本体成功、附带的健康检查有几台没探到。
 * zero 在这三张表上到不了，因为它们都没有筛选/搜索入口（0 行只能是 empty）。
 */
const { loading, state, errorText, run, hasPartialBanner } = useDataState({
  failed: () => healthFailedIds.value.length,
});

/** empty 用本页文案，error / perm 让 PtDataState 用自己的预设标题 */
const stateTitle = computed(() => (state.value === "empty" ? "还没有下载器" : ""));

const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  return "接上 qBittorrent 或 Transmission，RSS 命中的种子才有地方推";
});

/**
 * 画板 19 在 p-dl（下载器表）之后还有四张卡：
 *   p-dir 548  下载目录（各下载器的预设目录）
 *   p-safe 516 磁盘保护（全局配置，这里只读回显 + 指路）
 *   p-bind 1080 站点绑定（哪些站点推到哪台）
 *   p-log 1080  连通性检查（最近一次探测的结果）
 * 四张的数据都来自已有接口，不新增后端。
 */
const dirSummary = ref<Record<number, DownloaderDirectory[]>>({});
const siteBindings = ref<SiteDownloaderSummaryItem[]>([]);
const diskProtect = ref<{ on: boolean; minGb: number } | null>(null);

async function loadCardData() {
  const [dirs, bindings, global] = await Promise.all([
    downloaderDirectoriesApi.listAll().catch(() => ({}) as Record<number, DownloaderDirectory[]>),
    dynamicSitesApi
      .getDownloaderSummary()
      .catch(() => ({ sites: [] as SiteDownloaderSummaryItem[] })),
    globalApi.get().catch(() => null),
  ]);
  dirSummary.value = dirs ?? {};
  siteBindings.value = Array.isArray(bindings?.sites) ? bindings.sites : [];
  diskProtect.value = global
    ? {
        on: global.cleanup_disk_protect === true,
        minGb: global.cleanup_min_disk_space_gb ?? 0,
      }
    : null;
}

/** p-dir：每台下载器配了几个预设目录 */
const dirRows = computed<BreakdownRow[]>(() =>
  downloaders.value
    .filter((d) => d.id !== undefined)
    .map((d) => {
      const list = dirSummary.value[d.id as number] ?? [];
      return {
        key: String(d.id),
        label: d.name,
        value: `${list.length} 个`,
        weight: list.length,
        tone: list.length > 0 ? ("primary" as const) : ("mute" as const),
        hint: list.length > 0 ? list.map((x) => x.alias || x.path).join(" · ") : "还没配预设目录",
      };
    }),
);

/** p-bind：站点推到哪台下载器；没绑定的走默认 */
const bindRows = computed<BreakdownRow[]>(() => {
  const buckets = new Map<string, string[]>();
  for (const item of siteBindings.value) {
    const name = item.downloader_name || "跟随默认";
    const list = buckets.get(name) ?? [];
    list.push(item.display_name || item.site_name);
    buckets.set(name, list);
  }
  return [...buckets.entries()]
    .sort((a, b) => b[1].length - a[1].length)
    .map(([name, sites]) => ({
      key: name,
      label: name,
      value: `${sites.length} 个站点`,
      weight: sites.length,
      tone: name === "跟随默认" ? ("mute" as const) : ("primary" as const),
      hint: sites.join(" · "),
    }));
});

/** p-log：最近一次连通性检查的结果 */
const healthRows = computed<BreakdownRow[]>(() =>
  downloaders.value
    .filter((d) => d.id !== undefined)
    .map((d) => {
      const id = d.id as number;
      const probeFailed = healthFailedIds.value.includes(id);
      const res = healthStatus.value[id];
      const ok = res?.is_healthy === true;
      return {
        key: String(id),
        label: d.name,
        value: probeFailed ? "未知" : ok ? "正常" : "异常",
        weight: 1,
        tone: probeFailed ? ("mute" as const) : ok ? ("ok" as const) : ("dang" as const),
        hint: probeFailed
          ? "这一次没探到（超时或请求失败），不代表下载器有问题"
          : res?.message || "—",
      };
    }),
);

/** 画板 head 的 sub（11.5/400 t3）：共几个下载器、默认是哪一个 */
const headSub = computed(() => {
  // 失败时不能顺着 length === 0 说「还没有配置下载器」，那是把加载失败说成空库
  if (state.value === "perm") return "无权访问下载器配置";
  if (state.value === "error") return "下载器列表没加载出来";
  if (downloaders.value.length === 0) return "还没有配置下载器";
  const dl = defaultDownloader.value;
  return dl
    ? `共 ${downloaders.value.length} 个 · 默认 ${dl.name}（${getTypeLabel(dl.type)}）`
    : `共 ${downloaders.value.length} 个 · 未指定默认下载器`;
});

const panelCount = computed(() =>
  downloaders.value.length ? `${downloaders.value.length} 个` : "",
);

onMounted(async () => {
  await loadDownloaders();
});

async function loadDownloaders() {
  healthFailedIds.value = [];
  const data = await run(() => downloadersApi.list());
  if (!data) {
    // 失败时清空：留着上一次的数据配一个「加载失败」的状态块更让人误解
    downloaders.value = [];
    healthStatus.value = {};
    ElMessage.error(errorText.value || "加载失败");
    return;
  }
  downloaders.value = data;
  loadHealthStatuses(downloaders.value);
  void loadCardData();
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

/** 记下 / 抹掉「这台的连通性没探到」，partial 提示条按这份清单计数 */
function markHealthProbeFailed(id: number, failed: boolean) {
  const i = healthFailedIds.value.indexOf(id);
  if (failed && i === -1) healthFailedIds.value.push(id);
  else if (!failed && i !== -1) healthFailedIds.value.splice(i, 1);
}

function loadHealthStatuses(list: DownloaderSetting[]) {
  const tasks = list
    .filter((dl) => dl.id && dl.enabled)
    .map((dl) =>
      fetchHealthStatus(dl.id!).then(
        (response) => {
          healthStatus.value[dl.id!] = response;
          markHealthProbeFailed(dl.id!, false);
        },
        (error) => {
          healthStatus.value[dl.id!] = {
            name: dl.name,
            is_healthy: false,
            message: getHealthErrorMessage(error),
          };
          markHealthProbeFailed(dl.id!, true);
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
        markHealthProbeFailed(dl.id, false);
      } catch (error: unknown) {
        healthStatus.value[dl.id] = {
          name: dl.name,
          is_healthy: false,
          message: getHealthErrorMessage(error),
        };
        markHealthProbeFailed(dl.id, true);
      }
    } else {
      // 停用的不再探测，也就不该继续算进「没探到」的计数里
      markHealthProbeFailed(dl.id, false);
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
    markHealthProbeFailed(dl.id, false);
    const status = healthStatus.value[dl.id];
    if (status && status.is_healthy) {
      ElMessage.success("连接正常");
    } else {
      ElMessage.warning(status?.message || "连接异常");
    }
  } catch (e: unknown) {
    const message = getHealthErrorMessage(e);
    healthStatus.value[dl.id] = { name: dl.name, is_healthy: false, message };
    markHealthProbeFailed(dl.id, true);
    ElMessage.error(message);
  }
}

/*
 * 状态胶囊上只写四个字以内的结论，失败原因走 tooltip：
 * 后端的 message 可能是一整条 HTTP 错误，直接铺在单元格里会把这一列撑到 300 宽。
 * （手机行卡上没有 tooltip 可用，那边把 detail 直接排在 meta 里。）
 */
function healthMeta(dl: DownloaderSetting): {
  tone: "ok" | "warn" | "dang" | "neutral";
  text: string;
  detail: string;
} {
  if (!dl.id || !dl.enabled) return { tone: "neutral", text: "未启用", detail: "" };
  const status = healthStatus.value[dl.id];
  if (!status) return { tone: "warn", text: "未检查", detail: "" };
  /*
   * 探测本身失败（超时 / 请求没发出去）时画成红色「异常」是在冤枉机器：
   * 我们并没有拿到「它不健康」这个结论，只是没问到。所以单独一档 warn「未知」，
   * 和上方的 partial 提示条对应，用户看到的原因才不矛盾。
   */
  if (healthFailedIds.value.includes(dl.id)) {
    return { tone: "warn", text: "未知", detail: status.message || "连通性检查没成功" };
  }
  return status.is_healthy
    ? { tone: "ok", text: "正常", detail: "" }
    : { tone: "dang", text: "异常", detail: status.message || "连接异常" };
}

function getTypeLabel(type: string) {
  return downloaderTypes.find((t) => t.value === type)?.label || type;
}

// ============== 目录管理功能 ==============

const {
  loading: loadingDirs,
  state: dirState,
  errorText: dirErrorText,
  run: runDirs,
} = useDataState();

const dirStateSub = computed(() => {
  if (dirState.value === "error" || dirState.value === "perm") return dirErrorText.value;
  return "没有配置目录时，推送走下载器自己的默认保存路径";
});

const dirNote = computed(() => {
  if (dirState.value === "perm") return "无权访问目录配置";
  if (dirState.value === "error") return "目录列表没加载出来";
  return `${directories.value.length} 个目录`;
});

async function openDirDialog(dl: DownloaderSetting) {
  currentDownloader.value = dl;
  showDirDialog.value = true;
  await loadDirectories(dl.id!);
}

async function loadDirectories(downloaderId: number) {
  const data = await runDirs(() => downloaderDirectoriesApi.list(downloaderId));
  if (!data) {
    directories.value = [];
    ElMessage.error(dirErrorText.value || "加载目录失败");
    return;
  }
  directories.value = data;
}

/** 状态块里的「重试」用得到：它不知道当前是哪个下载器 */
function reloadDirectories() {
  if (currentDownloader.value?.id) void loadDirectories(currentDownloader.value.id);
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

const {
  loading: loadingSyncSites,
  state: syncState,
  errorText: syncErrorText,
  run: runSyncSites,
} = useDataState();

const syncStateSub = computed(() => {
  if (syncState.value === "error" || syncState.value === "perm") return syncErrorText.value;
  return "还没有启用的站点";
});

const syncNote = computed(() => {
  if (syncState.value === "perm") return "无权读取站点列表";
  if (syncState.value === "error") return "站点列表没加载出来";
  return `已选 ${selectedSiteIds.value.length} / ${syncSites.value.length}`;
});

async function openSyncDialog(dl: DownloaderSetting) {
  newDefaultDownloader.value = dl;
  showSyncDialog.value = true;
  await loadSyncSites();
}

/*
 * 失败时不再顺手把对话框关掉。默认下载器**已经**切过去了，站点同步是紧接着的第二步；
 * 对话框一闪而过只留个 toast，用户既不知道同步做没做，也没有入口重试。
 */
async function loadSyncSites() {
  const data = await runSyncSites(() => dynamicSitesApi.getDownloaderSummary());
  if (!data) {
    syncSites.value = [];
    selectedSiteIds.value = [];
    ElMessage.error(syncErrorText.value || "加载站点失败");
    return;
  }
  syncSites.value = data.sites;
  selectedSiteIds.value = data.sites.filter((s) => s.downloader_id == null).map((s) => s.site_id);
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
  <!--
    画板 19 是卡片页：head 之后直接是 1080 通栏的 p-dl，没有工具栏带 ——
    刷新与添加按钮在画板上是页头右侧那两枚（32 高），所以送进外壳页头。
    画板另有 p-dir / p-safe / p-bind / p-log 四张卡，对应的是全局设置里的
    下载目录、磁盘保护、站点绑定与日志，本页不重复一份，见设计文档 §5。
  -->
  <div class="downloader-page pt-cards pt-cards--wide">
    <Teleport v-if="headSub" to="#pt-head-sub">{{ headSub }}</Teleport>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button :loading="loading" @click="loadDownloaders">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
      <el-button type="primary" @click="openAddDialog">
        <PtIcon name="plus" :size="15" /><span>添加下载器</span>
      </el-button>
    </Teleport>

    <PtPanel
      v-loading="loading"
      title="下载器"
      icon="hard-drive"
      :count="panelCount"
      padding="none">
      <!--
        partial：列表本体是完整的，只是附带的连通性探测有几台没探到。
        这种情形不能用一整块状态图顶掉表格 —— 那等于把已经拿到的数据也藏起来。
      -->
      <div v-if="hasPartialBanner(downloaders.length)" class="pt-note pt-note--warn partial-note">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>
          有 {{ healthFailedIds.length }} 个下载器的连通性没探到（超时或请求失败），
          它们在表里显示为「未知」而不是「异常」。列表本身是完整的，可以单独点「检查」重试。
        </span>
      </div>

      <el-table
        v-if="!isMobile"
        :data="downloaders"
        class="pt-grid"
        row-key="id"
        style="width: 100%">
        <template #empty>
          <PtDataState :state="state" dense :title="stateTitle" :sub="stateSub">
            <template v-if="state === 'error'" #action>
              <el-button size="small" @click="loadDownloaders">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>
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

      <!--
        移动端行卡（§9）。桌面这张表 7 列，末列还是 fixed 的操作列，手机上横着滚既看不到
        列头也和页面纵向滚动打架。卡上留的是真正要看的：名称 + 类型/地址/失败原因 + 连通性 + 操作。
        「默认」是整张表里的单选，所以放在 lead 上当一枚星标，触控区由 lead 保证 ≥44。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!downloaders.length" :state="state" :title="stateTitle" :sub="stateSub">
          <template v-if="state === 'error'" #action>
            <el-button size="small" @click="loadDownloaders">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>

        <PtRowCard v-for="dl in downloaders" :key="dl.id">
          <template #lead>
            <button
              type="button"
              class="star star--touch"
              :class="{ 'is-on': dl.is_default }"
              :disabled="dl.is_default"
              :aria-label="dl.is_default ? '当前默认下载器' : `把 ${dl.name} 设为默认下载器`"
              @click="setDefault(dl)">
              <PtIcon name="star" :size="18" />
            </button>
          </template>

          <template #title>{{ dl.name }}</template>

          <template #meta>
            <PtTag>{{ getTypeLabel(dl.type) }}</PtTag>
            <code class="url">{{ dl.url }}</code>
            <!-- 手机上没有 tooltip 可用，失败原因直接排在这里，换行也比藏起来好 -->
            <span
              v-if="healthMeta(dl).detail"
              class="card-reason"
              :class="`is-${healthMeta(dl).tone}`">
              <PtIcon name="triangle-alert" :size="11" />{{ healthMeta(dl).detail }}
            </span>
          </template>

          <template #status>
            <PtStatusPill :tone="healthMeta(dl).tone" size="sm">
              {{ healthMeta(dl).text }}
            </PtStatusPill>
          </template>

          <template #actions>
            <!-- 桌面那枚 20 高的开关在手机上按不准，换成和其它操作同宽的按钮 -->
            <el-button size="small" @click="toggleEnabled(dl)">
              <PtIcon :name="dl.enabled ? 'circle-pause' : 'circle-check'" :size="14" />
              <span>{{ dl.enabled ? "停用" : "启用" }}</span>
            </el-button>
            <el-button size="small" :disabled="!dl.enabled" @click="checkHealth(dl)">
              <PtIcon name="activity" :size="14" /><span>检查</span>
            </el-button>
            <el-button size="small" @click="openDirDialog(dl)">
              <PtIcon name="folder" :size="14" /><span>目录</span>
            </el-button>
            <el-button size="small" @click="openEditDialog(dl)">
              <PtIcon name="pencil" :size="14" /><span>编辑</span>
            </el-button>
            <el-button size="small" type="danger" plain @click="deleteDownloader(dl)">
              <PtIcon name="trash-2" :size="14" /><span>删除</span>
            </el-button>
          </template>
        </PtRowCard>
      </div>

      <template v-if="downloaders.length > 0" #footer>
        <span class="pt-foot-note">
          默认下载器用于没有单独绑定下载器的站点；「检查」只探连通性，不改任何配置
        </span>
      </template>
    </PtPanel>

    <!--
      画板 19 在 p-dl 之后的四张卡。数据都来自已有接口：
      目录清单用 /api/downloaders/all-directories，站点绑定用 /api/sites/downloader-summary，
      磁盘保护读全局配置（只回显，改还是去系统设置 —— 同一份配置不做两个写入口），
      连通性用每台的 /health 探测结果。
    -->
    <div v-if="downloaders.length > 0" class="pt-cards pt-cards--2 dl-cards">
      <PtPanel title="下载目录" icon="folder" :count="`${downloaders.length} 台`">
        <PtBreakdown
          :rows="dirRows"
          foot="预设目录用于推送时挑保存路径；一台都没配时推送会用下载器自己的默认目录。点表格里的「目录」可以管理。" />
      </PtPanel>

      <PtPanel title="磁盘保护" icon="shield">
        <ul v-if="diskProtect" class="dl-kv">
          <li class="dl-kv__row">
            <span class="dl-kv__k">保护开关</span>
            <PtStatusPill :tone="diskProtect.on ? 'ok' : 'neutral'" size="sm" dot>
              {{ diskProtect.on ? "已开启" : "未开启" }}
            </PtStatusPill>
          </li>
          <li class="dl-kv__row">
            <span class="dl-kv__k">最低保留空间</span>
            <span class="dl-kv__v">{{ diskProtect.minGb }} GB</span>
          </li>
        </ul>
        <p v-else class="dl-empty">全局配置没取到，这里只能留空。</p>
        <p class="dl-foot">
          推送前会拿「下载器剩余空间 − 未完成体积 − 本进程已预留」和这个阈值比，不够就不推。
          开启时读不到剩余空间会**拒绝推送**而不是放行。这是全局配置，改在
          <el-button link type="primary" @click="$router.push('/global')">系统设置</el-button>。
        </p>
      </PtPanel>

      <PtPanel class="pt-cards__full" title="站点绑定" icon="link">
        <PtBreakdown
          v-if="bindRows.length > 0"
          :rows="bindRows"
          cols
          foot="站点没单独绑定时走默认下载器；RSS 订阅还能再单独绑一台，优先级是 RSS → 站点 → 默认。" />
        <p v-else class="dl-empty">还没有站点，或者站点列表没取到。</p>
      </PtPanel>

      <PtPanel class="pt-cards__full" title="连通性检查" icon="activity">
        <PtBreakdown
          :rows="healthRows"
          cols
          foot="打开页面时自动探一次，每台 5 秒超时。「未知」是这一次没探到（超时或请求没发出去），不等于下载器坏了 —— 可以点表格里的「检查」单独重试。" />
      </PtPanel>
    </div>

    <!-- 添加/编辑对话框 -->
    <el-dialog
      v-model="showDialog"
      class="pt-dialog"
      :title="editMode ? '编辑下载器' : '添加下载器'"
      :width="isMobile ? '94vw' : '520px'"
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
      :width="isMobile ? '94vw' : '720px'"
      align-center>
      <PtToolbar :note="dirNote">
        <template #right>
          <el-button type="primary" size="small" @click="openAddDirDialog">
            <PtIcon name="plus" :size="14" /><span>添加目录</span>
          </el-button>
        </template>
      </PtToolbar>

      <el-table
        v-if="!isMobile"
        v-loading="loadingDirs"
        :data="directories"
        class="pt-grid"
        row-key="id"
        style="width: 100%">
        <template #empty>
          <PtDataState :state="dirState" dense :sub="dirStateSub">
            <template v-if="dirState === 'error'" #action>
              <el-button size="small" @click="reloadDirectories">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>
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

      <!-- 目录表在手机上同样退化成行卡：别名当标题，完整路径排第二行 -->
      <div v-else v-loading="loadingDirs" class="cards cards--dialog">
        <PtDataState v-if="!directories.length" :state="dirState" :sub="dirStateSub">
          <template v-if="dirState === 'error'" #action>
            <el-button size="small" @click="reloadDirectories">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>

        <PtRowCard v-for="dir in directories" :key="dir.id">
          <template #lead>
            <button
              type="button"
              class="star star--touch"
              :class="{ 'is-on': dir.is_default }"
              :disabled="dir.is_default"
              :aria-label="dir.is_default ? '当前默认目录' : '设为默认目录'"
              @click="setDefaultDirectory(dir)">
              <PtIcon name="star" :size="18" />
            </button>
          </template>

          <template #title>{{ dir.alias || dir.path }}</template>

          <template v-if="dir.alias" #meta>
            <code class="url">{{ dir.path }}</code>
          </template>

          <template #actions>
            <el-button size="small" @click="openEditDirDialog(dir)">
              <PtIcon name="pencil" :size="14" /><span>编辑</span>
            </el-button>
            <el-button size="small" type="danger" plain @click="deleteDirectory(dir)">
              <PtIcon name="trash-2" :size="14" /><span>删除</span>
            </el-button>
          </template>
        </PtRowCard>
      </div>
    </el-dialog>

    <!-- 添加/编辑目录对话框 -->
    <el-dialog
      v-model="showAddDirDialog"
      class="pt-dialog"
      :title="editDirMode ? '编辑目录' : '添加目录'"
      :width="isMobile ? '94vw' : '480px'"
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
      :width="isMobile ? '94vw' : '620px'"
      align-center>
      <div class="pt-note sync-note">
        <PtIcon name="info" :size="14" class="pt-note__icon" />
        <span>
          默认下载器已切到「{{ newDefaultDownloader?.name }}」。下面这些站点要不要一并改过来？
          默认勾选的是原本就跟着默认走的站点。
        </span>
      </div>

      <PtToolbar :note="syncNote">
        <el-button size="small" :disabled="!syncSites.length" @click="toggleAllSites">
          <PtIcon name="check-check" :size="14" />
          <span>{{ selectedSiteIds.length === syncSites.length ? "取消全选" : "全选" }}</span>
        </el-button>
      </PtToolbar>

      <el-table
        v-if="!isMobile"
        v-loading="loadingSyncSites"
        :data="syncSites"
        class="pt-grid"
        row-key="site_id"
        max-height="380"
        style="width: 100%">
        <template #empty>
          <PtDataState :state="syncState" dense :sub="syncStateSub">
            <template v-if="syncState === 'error'" #action>
              <el-button size="small" @click="loadSyncSites">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>
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

      <!--
        站点表在手机上同样退化成行卡。这里的多选是 el-checkbox 自己的，
        和桌面共用同一份 selectedSiteIds，所以两种视图切来切去勾选不会丢。
      -->
      <div v-else v-loading="loadingSyncSites" class="cards cards--dialog cards--scroll">
        <PtDataState v-if="!syncSites.length" :state="syncState" :sub="syncStateSub">
          <template v-if="syncState === 'error'" #action>
            <el-button size="small" @click="loadSyncSites">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>

        <PtRowCard v-for="site in syncSites" :key="site.site_id">
          <template #lead>
            <el-checkbox
              :model-value="selectedSiteIds.includes(site.site_id)"
              :aria-label="`选择 ${site.display_name || site.site_name}`"
              @change="(val: boolean) => toggleSiteSelection(site.site_id, val)" />
          </template>

          <template #title>{{ site.display_name || site.site_name }}</template>

          <template #meta>
            <PtTag v-if="site.downloader_name">{{ site.downloader_name }}</PtTag>
            <span v-else class="follow">跟随默认</span>
          </template>
        </PtRowCard>
      </div>

      <template #footer>
        <el-button @click="showSyncDialog = false">跳过</el-button>
        <!-- 站点没加载出来时这枚按钮做不了任何事，别让它看起来还能点 -->
        <el-button
          type="primary"
          :loading="applyingSites"
          :disabled="!selectedSiteIds.length"
          @click="applySitesDownloader">
          应用到 {{ selectedSiteIds.length }} 个站点
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* 画板 19 的四张分析卡 */
.dl-kv {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.dl-kv__row {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  justify-content: space-between;
  font-size: var(--pt-fz-sm);
}

.dl-kv__k {
  color: var(--pt-t3);
}

.dl-kv__v {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  color: var(--pt-t1);
}

.dl-empty {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.dl-foot {
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t4);
}

.dl-foot :deep(.el-button) {
  height: auto;
  padding: 0;
  font-size: inherit;
  vertical-align: baseline;
}

/* 版面（内缩 16、卡间 16）交给 .pt-cards--wide */

.url {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t2);
}

/* 行卡里的地址没有列宽兜着，长 URL 允许在任意位置断行，别把卡片撑破 */
.cards .url {
  overflow-wrap: anywhere;
}

/* partial 提示条挂在表格上方；面板 padding="none"，留白由它自己给 */
.partial-note {
  margin: var(--pt-space-3) var(--pt-space-3) 0;
}

/* 移动端行卡列表：面板 padding="none"，所以留白由这里给 */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-space-3);
}

/* 对话框正文已经有 16 内边距，卡片列表不再叠一层 */
.cards--dialog {
  padding: var(--pt-space-3) 0 0;
}

/* 对应桌面表格的 max-height="380"：站点可能有几十个，别把对话框顶出屏幕 */
.cards--scroll {
  max-height: 55vh;
  overflow-y: auto;
}

/* 失败原因：手机上没有 tooltip，原因直接铺在 meta 里，允许换行，配色跟着胶囊走 */
.card-reason {
  overflow-wrap: anywhere;
}

.card-reason.is-warn {
  color: var(--pt-warn);
}

.card-reason.is-dang {
  color: var(--pt-dang);
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

/* 行卡上的星标要能用拇指按：26 的桌面尺寸撑到 44（§9 的触控下限） */
.star--touch {
  width: var(--pt-m-touch);
  height: var(--pt-m-touch);
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
