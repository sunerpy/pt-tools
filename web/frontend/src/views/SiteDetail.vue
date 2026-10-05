<script setup lang="ts">
import {
  attendanceApi,
  chatopsApi,
  downloaderDirectoriesApi,
  type DownloaderDirectory,
  downloadersApi,
  type DownloaderSetting,
  type FilterRule,
  filterRulesApi,
  type NotificationConfig,
  type RSSConfig,
  type SiteAttendance,
  type SiteConfig,
  type SiteLoginState,
  sitesApi,
  type TaskItem,
  tasksApi,
  type UserInfoResponse,
  userInfoApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { useDataState } from "@/composables/useDataState";
import { isProbeSuccess, probeStatusLabel } from "@/utils/probeStatus";
import { attendanceView } from "@/utils/attendanceStatus";
import { formatShortDateTime } from "@/utils/format";
import { useIsMobile } from "@/composables/useIsMobile";
import { computed, onMounted, reactive, ref } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();
/* ≤768 时外壳隐藏页头，页头动作得原地落回页面（Teleport 的 disabled） */
const isMobile = useIsMobile();

const siteName = computed(() => route.params.name as string);
/**
 * 六态（设计文档 §5）。这一页比列表页更要紧：加载失败时 form 停在默认值上，
 * 而「保存配置」会把这份默认值 PUT 回去，等于用空配置覆盖掉真实站点配置。
 * 所以失败时不渲染表单、也不允许保存。
 */
const { loading, state, errorText, run } = useDataState();
/** 加载失败（含无权限）时锁住表单与保存 */
const loadFailed = computed(() => state.value === "error" || state.value === "perm");
/**
 * 详情至少成功读回来过一次。在那之前 form 是默认空表单 —— 首次加载中、失败后点重试又在加载中，
 * 都不是 error / perm，只看 loadFailed 的话写操作照样放行，一点就用空配置覆盖站点、删光订阅。
 */
const loaded = ref(false);
/** 三处写操作（保存配置、添加 RSS、编辑 RSS）共用的锁：没加载成功过、正在加载、加载失败 */
const writeLocked = computed(() => !loaded.value || loading.value || loadFailed.value);
const saving = ref(false);
const addingRss = ref(false);
const rssDialogVisible = ref(false);
const downloaders = ref<DownloaderSetting[]>([]);
const filterRules = ref<FilterRule[]>([]);
/**
 * 全量规则（含停用的），只用于把 filter_rule_ids 翻成名字。
 * filterRules 里只留启用的（编辑弹窗的下拉不该给出停用项），拿它查名会把
 * 「关联着但已停用」的规则显示成「已删除」—— 两件事的处理方式完全不同。
 */
const allFilterRules = ref<FilterRule[]>([]);
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

onMounted(loadDetail);

async function loadDetail() {
  // 并行加载站点配置、下载器列表、过滤规则列表、下载器目录、通知通道列表
  const data = await run(() =>
    Promise.all([
      sitesApi.get(siteName.value),
      downloadersApi.list(),
      filterRulesApi.list(),
      downloaderDirectoriesApi.listAll(),
      chatopsApi.notifications.list().catch(() => [] as NotificationConfig[]),
    ]),
  );
  if (!data) return; // 失败：form 保持原样但被 writeLocked 锁住，绝不能拿默认值去覆盖
  const [siteData, downloaderList, filterRuleList, directoriesData, confList] = data;
  form.value = siteData;
  loaded.value = true;
  downloaders.value = downloaderList; // 显示所有下载器，不过滤
  allFilterRules.value = filterRuleList;
  filterRules.value = filterRuleList.filter((r) => r.enabled); // 下拉只给启用的
  downloaderDirectories.value = directoriesData;
  availableConfs.value = (confList || []).filter((c) => c.enabled);
  void loadSideCards();
}

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

/*
 * 这一页的三处写操作（保存配置、添加 RSS、编辑 RSS）都是把**整份** form 交给后端，
 * 而后端 UpsertSiteWithRSS 会先删光该站点的全部 RSS 再按提交内容重建。
 * 详情没加载成功时 form 是默认值 —— 这时放行任何一处写操作，都会用空列表把真实订阅整批删掉。
 * 之前只锁了「保存配置」一个按钮，「添加 RSS」照样能点。所以守卫下沉到函数里，三处共用；
 * 按钮也照同一个 writeLocked 禁用，守卫是按钮之外的最后一道。
 * 「没加载成功」包括还在加载：首次加载中、失败后重试中，form 同样是默认值。
 */
function blockedByLoadState(): boolean {
  if (!writeLocked.value) return false;
  ElMessage.warning(
    loading.value
      ? "站点详情还在加载，等加载完成再修改，避免用空表单覆盖现有配置"
      : "站点详情没有加载成功，先重试加载再修改，避免覆盖现有配置",
  );
  return true;
}

async function save() {
  if (blockedByLoadState()) return;
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
  if (blockedByLoadState()) return;
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
  if (blockedByLoadState()) return;
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

/**
 * 画板 head 88 的 sub（11.5/400 t3）。
 * 画板写的是「pterclub.com · Power User · 分享率 5.46 · 上传 14.2 TB · 做种 141」——
 * 那几项统计属于用户数据接口，不在站点配置里；这页能说清的是域名、认证方式与订阅条数。
 */
const headSub = computed(() => {
  const parts: string[] = [];
  const host = (form.value.urls || [])[0];
  if (host) {
    try {
      parts.push(new URL(host).host);
    } catch {
      parts.push(host);
    }
  }
  parts.push(`认证 ${authMethodLabel.value}`);
  parts.push(`RSS ${(form.value.rss || []).length} 条`);
  return parts.join(" · ");
});

/**
 * 画板 tabs 的六个分区。左栏内容随分区切换，右栏四张卡常驻
 * （画板 13 的激活分区是「RSS 订阅」，右栏那几张照样在）。
 *
 * 六项的内容都有真实来源，没有占位分区：
 *   概览     站点配置表单（基本信息 + 限速与容量）
 *   RSS 订阅 这个站点的订阅列表
 *   任务     /api/tasks?site=<name>
 *   推送记录 /api/tasks?site=<name>&pushed=1
 *
 * 「最近推送」与「任务」两张卡在 **RSS 分区下也显示** —— 画板 13 的激活分区是
 * RSS 订阅，而左栏那一列同时摆着 p-rss / p-push / p-tasks 三张。它们各自还有一个
 * 同名分区，点过去就是只看这一张。
 *   过滤规则 这个站点各条 RSS 关联到的规则（form.rss[].filter_rule_ids × filterRules）
 *   凭据     认证凭据表单（右栏那张只读摘要在这个分区下让位，不做两份同样的表单）
 */
const tab = ref<"overview" | "rss" | "tasks" | "push" | "rules" | "cred">("overview");

const TABS = computed(() => [
  { key: "overview" as const, label: "概览", n: 0 },
  { key: "rss" as const, label: "RSS 订阅", n: (form.value.rss || []).length },
  { key: "tasks" as const, label: "任务", n: siteTasks.value.length },
  { key: "push" as const, label: "推送记录", n: sitePushed.value.length },
  { key: "rules" as const, label: "过滤规则", n: siteRuleRows.value.length },
  { key: "cred" as const, label: "凭据", n: 0 },
]);

/**
 * 这个站点实际用到的过滤规则：从各条 RSS 的 filter_rule_ids 汇总，
 * 顺带记下是哪条订阅在用 —— 光列规则名看不出「这一站为什么会命中它」。
 */
const siteRuleRows = computed(() => {
  const used = new Map<number, string[]>();
  for (const rss of form.value.rss || []) {
    for (const id of rss.filter_rule_ids || []) {
      const list = used.get(id) ?? [];
      list.push(rss.name || `RSS #${rss.id ?? "?"}`);
      used.set(id, list);
    }
  }
  return [...used.entries()]
    .map(([id, sources]) => ({
      id,
      rule: allFilterRules.value.find((r) => r.id === id),
      sources,
    }))
    .sort((a, b) => (a.rule?.priority ?? 9999) - (b.rule?.priority ?? 9999));
});

/**
 * 画板 13 右栏是四张常驻卡（p-cred 凭据 / p-keep 保号规则 / p-stat 统计 / p-danger
 * 危险操作），左栏在 RSS 分区下是三张（p-rss 订阅 / p-push 最近推送 / p-tasks 任务）。
 * 这里补齐除凭据之外的五张，数据全部来自已有接口：
 *   保号规则  /api/sites/<name>/login-state（列表接口里筛这一站）
 *   统计      /api/v2/userinfo/sites/<name>
 *   任务/推送 /api/tasks?site=<name>[&pushed=1]
 *   危险操作  /api/sites?name=<name>（删除）
 */
/** probe_mode 的取值来自 SiteLoginState，三档：auto / manual / disabled */
const PROBE_LABEL: Record<string, string> = {
  auto: "自动探测",
  manual: "只手动探测",
  disabled: "不探测",
};

/** 判定活跃的依据，与站点列表的说明一致 */
const ACTIVE_SOURCE_LABEL: Record<string, string> = {
  last_access: "站点最近访问",
  api_last_login: "站点最近登录",
  cookie_last_login: "站点最近登录",
  last_login: "站点最近登录",
  last_visit: "浏览器访问",
  none: "-",
  unknown: "-",
};

const loginState = ref<SiteLoginState | null>(null);
const siteAttendance = ref<SiteAttendance | null>(null);
const attendanceShown = computed(() => attendanceView(siteAttendance.value ?? undefined));
/** 卡片里只放「状态（时间）」，整句提示放进 title，长句不会把行标签挤成两行 */
const attendanceToday = computed(() => {
  const a = siteAttendance.value;
  const v = attendanceShown.value;
  if (!a) return v.label;
  const when =
    a.status === "pending"
      ? (a.next_attempt_at ?? a.scheduled_at)
      : a.status === ""
        ? undefined
        : a.last_attempt_at;
  return when ? `${v.label}（${formatShortDateTime(when * 1000)}）` : v.label;
});
const signing = ref(false);
const siteStats = ref<UserInfoResponse | null>(null);
const siteTasks = ref<TaskItem[]>([]);
const sitePushed = ref<TaskItem[]>([]);
const deleting = ref(false);

async function loadSideCards() {
  const name = siteName.value;
  const [states, attendanceList, stats, tasks, pushed] = await Promise.all([
    sitesApi.listLoginStates().catch(() => [] as SiteLoginState[]),
    attendanceApi.list().catch(() => [] as SiteAttendance[]),
    userInfoApi.getSite(name).catch(() => null),
    tasksApi
      .list(new URLSearchParams({ site: name, page: "1", page_size: "20" }))
      .catch(() => null),
    tasksApi
      .list(new URLSearchParams({ site: name, pushed: "1", page: "1", page_size: "20" }))
      .catch(() => null),
  ]);
  loginState.value = (states ?? []).find((st) => st.site_name === name) ?? null;
  siteAttendance.value = (attendanceList ?? []).find((a) => a.site_name === name) ?? null;
  siteStats.value = stats;
  siteTasks.value = tasks?.items ?? [];
  sitePushed.value = pushed?.items ?? [];
}

/** 立即签到一次；结果写回卡片，不必重新加载整页 */
async function signNow() {
  if (signing.value) return;
  signing.value = true;
  try {
    const res = await attendanceApi.signNow(siteName.value);
    siteAttendance.value = res;
    const v = attendanceView(res);
    if (res.status === "signed" || res.status === "already") ElMessage.success(v.detail);
    else ElMessage.warning(`签到未成功：${res.last_error || v.detail}`);
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "签到失败");
  } finally {
    signing.value = false;
  }
}

/**
 * 内置站点不可删 —— 判定用接口回的 is_builtin（服务端由站点定义注册表给出）。
 * 服务端 `ConfigStore.DeleteSite` 也会自己拦一道；这里拦是为了不让按钮可点，
 * 让用户点下去再吃一个报错比一开始就禁用要糟。
 * 站点列表页原本就有这道拦截，详情页这个新入口当初漏了。
 */
const isBuiltinSite = computed(() => form.value.is_builtin === true);

/** 危险操作：删掉这个站点的配置。删完回列表页，别停在一个已经不存在的详情上 */
async function deleteSite() {
  if (isBuiltinSite.value) {
    ElMessage.warning("预置站点不可删除");
    return;
  }
  try {
    await ElMessageBox.confirm(
      `删除站点「${siteName.value}」的配置？它的 RSS 订阅、Cookie 与限速设置会一起删掉，` +
        `已经下载的种子不受影响。`,
      "确认删除",
      { type: "warning", confirmButtonText: "删除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  deleting.value = true;
  try {
    await sitesApi.delete(siteName.value);
    ElMessage.success("已删除");
    router.push("/sites");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "删除失败");
  } finally {
    deleting.value = false;
  }
}

function formatWhen(unixSeconds?: number): string {
  if (typeof unixSeconds !== "number" || !Number.isFinite(unixSeconds) || unixSeconds <= 0) {
    return "-";
  }
  const d = new Date(unixSeconds * 1000);
  /* 拿到非法值时印「-」而不是 Invalid Date —— 后者看着像功能坏了 */
  return Number.isNaN(d.getTime()) ? "-" : d.toLocaleString("zh-CN", { hour12: false });
}

function formatTB(bytes?: number): string {
  if (!bytes) return "-";
  const tb = bytes / 1024 ** 4;
  if (tb >= 1) return `${tb.toFixed(2)} TB`;
  return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
}

function downloaderNameOf(id: number | undefined): string {
  if (!id) return "默认";
  return downloaders.value.find((d) => d.id === id)?.name || "未知";
}

function ruleNameOf(id: number): string {
  return filterRules.value.find((r) => r.id === id)?.name || `规则 ${id}`;
}
</script>

<template>
  <!--
    画板 13：head 88（面包屑 + 标题 + 状态 pill + 架构 tag + 摘要，外壳给）
    → tabs 40（分区带）→ 700 + 16 + 364 的两列卡片。
    画板的 6 个分区里，任务 / 推送记录 / 过滤规则 在本项目是独立路由（/tasks、
    /filter-rules），这一页只给有本地内容的两个分区，见设计文档 §5 的偏离记录。
  -->
  <div class="site-detail-page">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <PtStatusPill :tone="form.enabled ? 'ok' : 'neutral'" dot size="sm">
        {{ form.enabled ? "已启用" : "未启用" }}
      </PtStatusPill>
      <el-button @click="goBack">
        <PtIcon name="arrow-left" :size="15" /><span>站点列表</span>
      </el-button>
      <!-- 没加载成功（含还在加载）就不给保存：那会把默认值写回去，覆盖掉真实配置 -->
      <el-button type="primary" :loading="saving" :disabled="writeLocked" @click="save">
        <PtIcon name="save" :size="15" /><span>保存配置</span>
      </el-button>
    </Teleport>

    <!-- 画板 tabs 344,88 1080×40：激活项 13/600 + 2px 指示条 -->
    <nav class="pt-band--tabs" aria-label="站点详情分区">
      <button
        v-for="t in TABS"
        :key="t.key"
        type="button"
        class="pt-band__tab"
        :class="{ 'is-on': tab === t.key }"
        :aria-current="tab === t.key ? 'page' : undefined"
        @click="tab = t.key">
        <span>{{ t.label }}</span>
        <span v-if="t.n" class="pt-band__tab-n">{{ t.n }}</span>
      </button>
    </nav>

    <!--
      画板 13 的卡片层是 700 + 16 + 364 两栏，两栏各自纵向堆卡。
      必须分成两个容器：直接把 7 张卡丢进两栏栅格，右栏每张卡会去跟左栏那张高卡对行，
      中间留出大片空白（实测右栏第 2 张卡被推到 680 以下）。
    -->
    <div class="pt-cards pt-cards--main">
      <div class="sd-col">
        <!-- 画板 bn 344,144 700×58：登录状态失效一类的当前告警，压在左栏顶上 -->
        <div v-if="form.unavailable" class="pt-note pt-note--warn site-bn" data-card="bn">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>{{ form.unavailable_reason || "该站点暂时不可用" }}</span>
        </div>

        <PtPanel
          v-show="tab === 'overview'"
          v-loading="loading"
          title="站点配置"
          icon="sliders-horizontal"
          padding="none">
          <PtDataState v-if="loadFailed" :state="state" :sub="errorText">
            <template v-if="state === 'error'" #action>
              <el-button size="small" @click="loadDetail">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>

          <el-form v-else :model="form" label-position="top" class="pt-form settings-form">
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

        <PtPanel
          v-show="tab === 'rss'"
          class="rss-card"
          title="RSS 订阅"
          icon="rss"
          :count="`${(form.rss || []).length} 条`">
          <template #actions>
            <el-button
              type="primary"
              size="small"
              :disabled="writeLocked"
              @click="openAddRssDialog">
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
                      v-if="
                        !row.is_example && row.filter_rule_ids && row.filter_rule_ids.length > 0
                      ">
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
                  <el-button
                    link
                    type="primary"
                    size="small"
                    :disabled="writeLocked"
                    @click="openEditRssDialog($index)">
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

        <!-- 画板 p-push 700「最近推送」：这个站点已推送的任务 -->
        <PtPanel
          v-show="tab === 'rss' || tab === 'push'"
          class="rss-card"
          title="最近推送"
          icon="send"
          :count="`${sitePushed.length} 条`">
          <ul v-if="sitePushed.length > 0" class="sd-list">
            <li v-for="t in sitePushed.slice(0, 6)" :key="t.id" class="sd-list__row">
              <span class="sd-list__t">{{ t.title }}</span>
              <!-- 这张卡叫「最近推送」，时间就该是推送时间；之前显示的是任务的创建时间 -->
              <span class="sd-list__m">
                {{ (t.pushTime || t.createdAt)?.slice(0, 16).replace("T", " ") }}
              </span>
            </li>
          </ul>
          <p v-else class="sd-empty">这个站点还没有推送成功的任务。</p>
          <p class="sd-foot">口径是 <code>/api/tasks?site=…&amp;pushed=1</code> 的前 20 条。</p>
        </PtPanel>

        <!-- 画板 p-tasks 700「任务」：这个站点的全部 RSS 任务 -->
        <PtPanel
          v-show="tab === 'rss' || tab === 'tasks'"
          class="rss-card"
          title="任务"
          icon="list-checks"
          :count="`${siteTasks.length} 个`">
          <ul v-if="siteTasks.length > 0" class="sd-list">
            <li v-for="t in siteTasks.slice(0, 6)" :key="t.id" class="sd-list__row">
              <PtStatusPill :tone="t.isPushed ? 'ok' : t.isExpired ? 'neutral' : 'warn'" size="sm">
                {{ t.isPushed ? "已推送" : t.isExpired ? "已过期" : "待处理" }}
              </PtStatusPill>
              <span class="sd-list__t">{{ t.title }}</span>
            </li>
          </ul>
          <p v-else class="sd-empty">这个站点还没有任务记录。</p>
          <p class="sd-foot">
            完整列表、批量删除在
            <el-button link type="primary" @click="router.push('/tasks')">任务列表</el-button>
            里。
          </p>
        </PtPanel>

        <!-- 画板分区「过滤规则」：这个站点的订阅实际关联到哪些规则 -->
        <PtPanel
          v-show="tab === 'rules'"
          class="rss-card"
          title="过滤规则"
          icon="list-filter"
          :count="`${siteRuleRows.length} 条`">
          <ul v-if="siteRuleRows.length > 0" class="sd-list">
            <li v-for="row in siteRuleRows" :key="row.id" class="sd-list__row">
              <PtStatusPill :tone="row.rule?.enabled ? 'ok' : 'neutral'" size="sm">
                {{ row.rule?.enabled ? "启用" : "停用" }}
              </PtStatusPill>
              <span class="sd-list__t">
                {{ row.rule?.name ?? `规则 #${row.id}（已删除）` }}
              </span>
              <span class="sd-list__m">{{ row.sources.join("、") }}</span>
            </li>
          </ul>
          <p v-else class="sd-empty">
            这个站点的订阅没有关联过滤规则 —— 它们按「免费种子自动下载」走。
          </p>
          <p class="sd-foot">
            关联在每条 RSS 的编辑弹窗里改；规则本身在
            <el-button link type="primary" @click="router.push('/filter-rules')">
              过滤规则
            </el-button>
            页维护。一旦某条 RSS 关联了规则，它就只下命中的种子。
          </p>
        </PtPanel>

        <!-- 画板分区「凭据」：可编辑的那一份认证凭据表单，700 宽 -->
        <PtPanel v-show="tab === 'cred'" class="rss-card" title="认证凭据" icon="key-round">
          <el-form :model="form" label-position="top" class="pt-form">
            <div class="cred-kv">
              <span class="cred-kv__k">认证方式</span>
              <span class="cred-kv__v">{{ authMethodLabel }}</span>
            </div>
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
          </el-form>
          <!--
            之前这里笼统地写「留空保存不会覆盖已存的值」—— 只对 Cookie 成立（core/config_store.go 里未携带 cookie 就保留已存的）。
            API Key 与 Passkey 会回显当前值，清空再保存会被后端以「不能为空」拒掉，并不是「保持不变」。
          -->
          <p class="sd-foot">
            改完要点页头的「保存配置」才会落库。Cookie
            不回显：留空保存会原样保留已存的那份（也照样算通过非空校验）。 API Key 与 Passkey
            会回显当前值，清空后保存会被拒绝 —— 这两项不能为空。要换就直接填新值；
            不想让这个站点参与任务，关掉「启用站点」。
          </p>
        </PtPanel>
      </div>

      <div class="sd-col">
        <!--
        画板 p-cred 1060,144 364×210「站点凭据」—— 凭据是这一页风险最高的一块，
        画板把它单独放在右栏而不是混在配置表里。它在两个分区下都在（画板 13 的
        激活分区是 RSS 订阅，右栏照样是这张卡）。
      -->
        <!--
          右栏这张是**只读摘要**：认证方式与各凭据存了没。
          可编辑的那份在「凭据」分区的左栏 —— 同一份表单不做两个副本，
          否则两处各填一半、保存哪一份都说不清。
        -->
        <PtPanel v-show="tab !== 'cred'" class="cred-card" title="站点凭据" icon="key-round">
          <ul class="sd-kv">
            <li class="sd-kv__row">
              <span class="sd-kv__k">认证方式</span>
              <span class="sd-kv__v">{{ authMethodLabel }}</span>
            </li>
            <li class="sd-kv__row">
              <span class="sd-kv__k">Cookie</span>
              <span class="sd-kv__v">{{ form.has_cookie ? "已保存" : "未设置" }}</span>
            </li>
            <li v-if="form.api_url" class="sd-kv__row">
              <span class="sd-kv__k">API 地址</span>
              <span class="sd-kv__v sd-kv__v--wrap">{{ form.api_url }}</span>
            </li>
          </ul>
          <p class="sd-foot">
            凭据在本机库里以 AES-GCM 加密存放，页面不回显已存的值。要改就去
            <el-button link type="primary" @click="tab = 'cred'">凭据分区</el-button>。
          </p>
        </PtPanel>

        <!-- 画板 p-keep 364「保号规则」：探测模式与提醒阈值，来自 login-state -->
        <PtPanel class="cred-card" title="保号规则" icon="bell-ring">
          <ul v-if="loginState" class="sd-kv">
            <li class="sd-kv__row">
              <span class="sd-kv__k">探测模式</span>
              <span class="sd-kv__v">{{
                PROBE_LABEL[loginState.probe_mode] ?? loginState.probe_mode
              }}</span>
            </li>
            <li class="sd-kv__row">
              <span class="sd-kv__k">封号阈值</span>
              <span class="sd-kv__v">
                {{
                  loginState.ban_threshold_days ? `${loginState.ban_threshold_days} 天不活跃` : "-"
                }}
              </span>
            </li>
            <li class="sd-kv__row">
              <span class="sd-kv__k">提前提醒</span>
              <span class="sd-kv__v">
                {{ loginState.remind_before_days ? `${loginState.remind_before_days} 天` : "-" }}
              </span>
            </li>
            <li class="sd-kv__row">
              <span class="sd-kv__k">剩余天数</span>
              <span class="sd-kv__v" :class="{ 'is-warn': (loginState.days_remaining ?? 99) <= 7 }">
                {{
                  loginState.days_remaining === undefined ? "-" : `${loginState.days_remaining} 天`
                }}
              </span>
            </li>
            <li class="sd-kv__row">
              <span class="sd-kv__k">最近探测</span>
              <span class="sd-kv__v">{{ formatWhen(loginState.last_probe_at) }}</span>
            </li>
            <li v-if="loginState.last_probe_status" class="sd-kv__row">
              <span class="sd-kv__k">探测结果</span>
              <span
                class="sd-kv__v"
                :class="{ 'is-warn': !isProbeSuccess(loginState.last_probe_status) }"
                data-testid="sd-probe-status">
                {{ probeStatusLabel(loginState.last_probe_status) }}
              </span>
            </li>
            <li class="sd-kv__row">
              <span class="sd-kv__k">判定依据</span>
              <span class="sd-kv__v" data-testid="sd-active-source">
                {{ ACTIVE_SOURCE_LABEL[loginState.effective_source ?? "none"] ?? "-" }}
              </span>
            </li>
            <li v-if="loginState.probe_mode === 'auto'" class="sd-kv__row">
              <span class="sd-kv__k">下次探测</span>
              <span class="sd-kv__v" data-testid="sd-next-probe">{{
                formatWhen(loginState.next_probe_at)
              }}</span>
            </li>
            <li v-if="(loginState.first_failure_at ?? 0) > 0" class="sd-kv__row">
              <span class="sd-kv__k">连续失败自</span>
              <span class="sd-kv__v is-warn" data-testid="sd-failing-since">{{
                formatWhen(loginState.first_failure_at)
              }}</span>
            </li>
            <li v-if="(loginState.access_stale_since ?? 0) > 0" class="sd-kv__row">
              <span class="sd-kv__k">访问状态</span>
              <span class="sd-kv__v is-warn" data-testid="sd-access-stale">
                访问未生效，需要手动登录
              </span>
            </li>
            <li v-if="siteAttendance" class="sd-kv__row">
              <span class="sd-kv__k">每日签到</span>
              <span class="sd-kv__v" data-testid="sd-attendance-enabled">
                {{
                  !siteAttendance.supported
                    ? "不支持"
                    : siteAttendance.attendance_enabled
                      ? "已开启"
                      : "未开启"
                }}
              </span>
            </li>
            <li v-if="siteAttendance" class="sd-kv__row">
              <span class="sd-kv__k">今日签到</span>
              <span
                class="sd-kv__v"
                :class="{
                  'is-warn': attendanceShown.tone === 'dang' || attendanceShown.tone === 'warn',
                }"
                :title="attendanceShown.detail"
                data-testid="sd-attendance-today">
                {{ attendanceToday }}
              </span>
            </li>
          </ul>
          <p v-else class="sd-empty">还没有这个站点的探测记录。</p>
          <p class="sd-foot">
            阈值与提醒在站点列表页按站点配置；这里只回显，避免同一份配置两个写入口。
            <el-button
              v-if="attendanceShown.canSign"
              link
              type="primary"
              :loading="signing"
              data-testid="sd-attend-now"
              @click="signNow">
              立即签到
            </el-button>
          </p>
        </PtPanel>

        <!-- 画板 p-stat 364「统计」：这个站点的用户数据 -->
        <PtPanel class="cred-card" title="站点统计" icon="gauge">
          <ul v-if="siteStats" class="sd-kv">
            <li class="sd-kv__row">
              <span class="sd-kv__k">等级</span>
              <span class="sd-kv__v">{{ siteStats.levelName || siteStats.rank || "-" }}</span>
            </li>
            <li class="sd-kv__row">
              <span class="sd-kv__k">上传 / 下载</span>
              <span class="sd-kv__v">
                {{ formatTB(siteStats.uploaded) }} / {{ formatTB(siteStats.downloaded) }}
              </span>
            </li>
            <li class="sd-kv__row">
              <span class="sd-kv__k">分享率</span>
              <span
                class="sd-kv__v"
                :class="{ 'is-warn': (siteStats.ratio ?? 0) > 0 && (siteStats.ratio ?? 0) < 1 }">
                {{ (siteStats.ratio ?? 0).toFixed(2) }}
              </span>
            </li>
            <li class="sd-kv__row">
              <span class="sd-kv__k">做种</span>
              <span class="sd-kv__v">{{ siteStats.seeding }}</span>
            </li>
            <li class="sd-kv__row">
              <span class="sd-kv__k">更新于</span>
              <span class="sd-kv__v">{{ formatWhen(siteStats.lastUpdate) }}</span>
            </li>
          </ul>
          <p v-else class="sd-empty">还没有同步过这个站点的用户数据。</p>
        </PtPanel>

        <!-- 画板 p-danger 364「危险操作」 -->
        <PtPanel class="cred-card" title="危险操作" icon="triangle-alert">
          <p class="sd-danger__p">
            删除这个站点的配置：它的 RSS 订阅、Cookie / API Key 与限速设置会一起删掉。
            <strong>已经下载的种子和下载器里的任务不受影响。</strong>
          </p>
          <p v-if="isBuiltinSite" class="sd-danger__p">
            这是内置站点（站点定义由程序自带），<strong>不能删除</strong>。
            不想让它参与任务，把「基本信息」里的「启用站点」关掉就行。
          </p>
          <el-tooltip :disabled="!isBuiltinSite" content="预置站点不可删除" placement="top">
            <span class="sd-danger__btn">
              <el-button
                type="danger"
                plain
                :disabled="isBuiltinSite"
                :loading="deleting"
                @click="deleteSite">
                <PtIcon name="trash-2" :size="15" /><span>删除站点配置</span>
              </el-button>
            </span>
          </el-tooltip>
        </PtPanel>
      </div>
    </div>

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
        <el-button type="primary" :loading="addingRss" :disabled="writeLocked" @click="addRss">
          添加
        </el-button>
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
        <el-button type="primary" :loading="updatingRss" :disabled="writeLocked" @click="updateRss">
          保存
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* 带与卡片层各自管留白，这一层只负责纵向堆叠 */
.site-detail-page {
  display: flex;
  flex-direction: column;
}

/*
 * 画板 bn 与 RSS 卡都在左栏（700），凭据卡在右栏（364）。
 * bn 是横幅不是卡，跨满两栏；两张主卡各占一栏，靠 grid-column 显式指定 ——
 * 分区切换时 v-show 会让其中一张不占格，不指定的话另一张会跳到左栏。
 */
.site-bn {
  grid-column: 1 / -1;
}

/*
 * 卡的宽度由所在栏容器给。这里刻意**不**写 align-self: start ——
 * 在 flex 列里那句的意思是「按内容收缩」（交叉轴是横向），
 * 内容短的那张卡（站点统计）会缩成 217 宽而不是跟着栏走 364。
 */

/* 两栏各自纵向堆卡 */
.sd-col {
  display: flex;
  flex-direction: column;
  gap: var(--pt-pad);
  align-self: start;
  min-width: 0;
}

/* 画板 13 右栏三张卡与左栏两张列表卡的共用样式 */
.sd-kv {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.sd-kv__row {
  display: flex;
  gap: var(--pt-space-3);
  align-items: baseline;
  justify-content: space-between;
  font-size: var(--pt-fz-sm);
}

.sd-kv__k {
  color: var(--pt-t3);
}

.sd-kv__v {
  font-variant-numeric: tabular-nums;
  font-weight: 500;
  color: var(--pt-t1);
  text-align: right;
}

/* API 地址这类长值允许折行，别把卡顶宽 */
.sd-kv__v--wrap {
  text-align: left;
  word-break: break-all;
}

.sd-kv__v.is-warn {
  color: var(--pt-warn);
}

.sd-list {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.sd-list__row {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  min-width: 0;
  font-size: var(--pt-fz-sm);
}

.sd-list__t {
  flex: 1;
  overflow: hidden;
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sd-list__m {
  flex: 0 0 auto;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.sd-empty {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

/* disabled 的按钮自己不派发 mouseenter，el-tooltip 需要一个能接事件的宿主 */
.sd-danger__btn {
  display: inline-flex;
}

.sd-danger__p {
  margin: 0 0 var(--pt-space-3);
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.sd-foot {
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t3);
}

.sd-foot code {
  padding: 1px 4px;
  font-family: var(--pt-font-mono);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

.sd-foot :deep(.el-button) {
  height: auto;
  padding: 0;
  font-size: inherit;
  vertical-align: baseline;
}

/* 凭据卡顶上的「认证方式」是只读事实，不用做成表单项 */
.cred-kv {
  display: flex;
  gap: var(--pt-space-3);
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: var(--pt-space-4);
  padding-bottom: var(--pt-space-3);
  font-size: var(--pt-fz-sm);
  border-bottom: 1px solid var(--pt-border);
}

.cred-kv__k {
  color: var(--pt-t3);
}

.cred-kv__v {
  font-weight: 500;
  color: var(--pt-t1);
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

/*
 * 示例卡换成虚线框、标题旁挂「示例」标签：一眼看出它不参与调度。
 * 之前还整体压 opacity 0.72，卡里的 t2 / t3 字合成后只有 2.9–4.8:1，承载信息的字不能这样压。
 */
.rss.is-example {
  border-style: dashed;
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
  color: var(--pt-t3);
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
  color: var(--pt-t3);
}
</style>
