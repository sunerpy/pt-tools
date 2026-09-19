<script setup lang="ts">
import { ApiError, type SiteConfig, type SiteLoginState, chatopsApi, sitesApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import SiteAvatar from "@/components/SiteAvatar.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";

import { formatTimeAgo } from "@/utils/format";
import { isProbeSuccess, probeStatusLabel, probeStatusSeverity } from "@/utils/probeStatus";
import { type DataStateKey, useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { useLoginState } from "@/composables/useLoginState";
import type { ReminderTier } from "@/composables/useLoginState";

const router = useRouter();

const isMobile = useIsMobile();
const sites = ref<Record<string, SiteConfig>>({});
const loginStates = ref<Record<string, SiteLoginState>>({});
const probing = reactive<Record<string, boolean>>({});
const testingReminder = reactive<Record<string, boolean>>({});
const bulkProbing = ref(false);
const updatingMode = reactive<Record<string, boolean>>({});

/* 风险提示只在本次会话里可关：它讲的是探测机制的固有局限，不是一条会过期的通知 */
const riskHintOpen = ref(true);

const {
  loginState,
  effectiveLastActive,
  lastAccess,
  daysRemaining,
  reminderTier,
  probeModeOf,
  tierTagType,
  tierLabel,
  daysCellClass,
} = useLoginState(loginStates);

const viewMode = ref<"enabled" | "all">("enabled");
/* 画板 bar-64 上的搜索框（220×28）：只筛本页表格，按站点名与域名匹配 */
const searchKeyword = ref("");
const addDialogVisible = ref(false);
const addSearch = ref("");
const enablingInDialog = reactive<Record<string, boolean>>({});

/** 登录状态接口是否单独失败了：站点清单拿到了但它没拿到，就是 partial 而不是 error */
const loginStatesFailed = ref(false);

/**
 * 六态状态机（设计文档 §5）。
 *
 * 以前这里只有一个 loading ref，失败时弹个 toast 就完事 —— toast 两秒后消失，
 * 表格停在「还没有启用任何站点」，用户看到的是「库里是空的」，而真相是请求失败了。
 * 401/403 也被画成普通失败，用户会一直点刷新。
 *
 * 这一页的 partial 是真实存在的：站点清单和登录状态是两个接口，登录状态挂了但清单
 * 拿到了，表格照常渲染，只是「判定活跃 / 剩余天数 / 探测模式」三列没有依据。
 */
const { loading, state, errorText, run, hasPartialBanner } = useDataState({
  failed: () => (loginStatesFailed.value ? 1 : 0),
});

/**
 * 任何一项筛选在生效，0 行的含义就是「筛掉了」而不是「库里没有」—— empty 与 zero 的分界。
 * 漏掉状态 / 认证 / 探测这三项的话，筛到 0 行时页面会说「还没有启用任何站点」，那是假话。
 */
const isFiltered = computed(
  () =>
    searchKeyword.value.trim() !== "" ||
    statusFilter.value.size > 0 ||
    authFilter.value !== "" ||
    probeFilter.value !== "",
);

/**
 * 空态仍按视图模式分成两种文案（「已启用」空 = 去新增，「全部」空 = 内置清单本身没内容），
 * 所以这里只接管 useDataState 判出的非空态，空态自己按 viewMode 与搜索词决定。
 */
const tableState = computed<DataStateKey>(() => {
  const s = state.value;
  if (s === "loading" || s === "error" || s === "perm" || s === "partial") return s;
  if (isFiltered.value) return "zero";
  return viewMode.value === "enabled" ? "empty" : "zero";
});

/** error / perm / partial 用 PtDataState 的预设标题，传空串即可回落 */
const stateTitle = computed(() => {
  if (isFiltered.value && tableState.value === "zero") return "没有匹配的站点";
  if (tableState.value === "empty") return "还没有启用任何站点";
  if (tableState.value === "zero") return "没有可显示的站点";
  return "";
});

/** 状态块的副标题：失败时给真实错误，空态时给下一步动作 */
const stateSub = computed(() => {
  switch (tableState.value) {
    case "error":
    case "perm":
      return errorText.value;
    case "partial":
      return "站点清单是空的，登录状态也没取到，先重试一次";
    case "empty":
      return "从「新增站点」里挑一个开始，启用后才会参与 RSS 与统计";
    default:
      return isFiltered.value
        ? "换个站点名或域名再搜，或切到「全部」看未启用的站点"
        : "站点清单来自内置定义，装上浏览器扩展可以帮助适配新站";
  }
});

onMounted(async () => {
  await loadSites();
});

async function loadSites() {
  loginStatesFailed.value = false;

  const data = await run(async () => {
    // allSettled 而不是 all：登录状态单独挂掉时站点清单还能用，不该整页变成 error
    const [siteRes, stateRes] = await Promise.allSettled([
      sitesApi.list(),
      sitesApi.listLoginStates(),
    ]);
    // 站点清单是主数据，它失败就没有「部分可用」可言，抛出去让状态机判 error / perm
    if (siteRes.status === "rejected") throw siteRes.reason;
    loginStatesFailed.value = stateRes.status === "rejected";
    return {
      siteMap: siteRes.value,
      states: stateRes.status === "fulfilled" ? stateRes.value : [],
    };
  });

  if (!data) {
    // 失败时清空：留着上一次的数据配一个「加载失败」的状态块更让人误解
    sites.value = {};
    loginStates.value = {};
    ElMessage.error(errorText.value || "加载失败");
    return;
  }

  sites.value = data.siteMap;
  const byName: Record<string, SiteLoginState> = {};
  for (const st of data.states ?? []) {
    byName[st.site_name] = st;
  }
  loginStates.value = byName;
}

async function toggleEnabled(name: string) {
  const site = sites.value[name];
  if (!site) return;
  site.enabled = !site.enabled;
  try {
    await sitesApi.save(name, site);
    ElMessage.success("已保存");
  } catch (e: unknown) {
    site.enabled = !site.enabled;
    ElMessage.error((e as Error).message || "保存失败");
  }
}

async function deleteSite(name: string) {
  if (sites.value[name]?.is_builtin) {
    ElMessage.warning("预置站点不可删除");
    return;
  }

  try {
    await ElMessageBox.confirm(`确定删除站点 "${name}"？`, "确认删除", {
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      type: "warning",
    });
    await sitesApi.delete(name);
    ElMessage.success("已删除");
    await loadSites();
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "删除失败");
    }
  }
}

async function probeSite(name: string) {
  if (probing[name]) return;
  probing[name] = true;
  try {
    const res = await sitesApi.probeNow(name);
    // 注意：后端 res.ok 恒为 true（仅表示探测已执行、未发生锁冲突），
    // 判定探测是否成功必须以 last_probe_status === "OK" 为准。
    const status = res.last_probe_status;
    const label = probeStatusLabel(status);
    // 后端 last_probe_error 为人类可读的失败原因（如 Cookie 未失效提示）；
    // 仅在存在非空原因、且原因未重复状态码时展示，避免冗余。
    const reason = res.last_probe_error?.trim();
    if (isProbeSuccess(status)) {
      ElMessage.success(`探测完成：${label}`);
    } else {
      const severity = probeStatusSeverity(status);
      if (severity === "info") {
        // NOT_APPLICABLE 等：探测已完成但不适用，若有原因一并展示。
        ElMessage.info(reason ? `探测完成：${label} — ${reason}` : `探测完成：${label}`);
      } else {
        // warning/error：探测未通过，原因可能较长，改用可关闭、长驻留的提示。
        const message = reason ? `探测未通过：${label} — ${reason}` : `探测未通过：${label}`;
        ElMessage({
          type: severity,
          message,
          showClose: true,
          duration: severity === "error" ? 0 : 6000,
        });
      }
    }
    // 无论成功与否都刷新表格，使状态列反映最新的 last_probe_status / last_probe_at。
    await loadSites();
  } catch (e: unknown) {
    if (e instanceof ApiError && e.status === 409) {
      ElMessage.warning("探测进行中，请稍候");
    } else {
      ElMessage.error((e as Error).message || "探测失败");
    }
  } finally {
    probing[name] = false;
  }
}

async function sendTestReminder(name: string) {
  if (testingReminder[name]) return;
  testingReminder[name] = true;
  try {
    await sitesApi.testReminder(name);
    ElMessage.success("测试提醒已发送，请检查通知通道（TG/QQ 等）");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "发送失败");
  } finally {
    testingReminder[name] = false;
  }
}

async function probeAllEnabled() {
  if (bulkProbing.value) return;
  const names = allEntries.value.filter(([, site]) => site.enabled).map(([name]) => name);
  if (names.length === 0) {
    ElMessage.info("暂无已启用站点");
    return;
  }

  bulkProbing.value = true;
  let cursor = 0;
  let success = 0;
  let failed = 0;
  let skipped = 0;

  async function worker() {
    for (;;) {
      const index = cursor++;
      if (index >= names.length) return;
      const name = names[index];
      if (probing[name]) {
        skipped++;
        continue;
      }

      probing[name] = true;
      try {
        const res = await sitesApi.probeNow(name);
        // res.ok 恒为 true，仅以 last_probe_status === "OK" 判定真正成功；
        // 其余状态（如 PARSE_ERROR，HTTP 200 但探测失败）计为失败。
        if (isProbeSuccess(res.last_probe_status)) success++;
        else failed++;
      } catch (e: unknown) {
        if (e instanceof ApiError && e.status === 409) skipped++;
        else failed++;
      } finally {
        probing[name] = false;
      }
    }
  }

  try {
    const concurrency = Math.min(3, names.length);
    await Promise.all(Array.from({ length: concurrency }, () => worker()));
    const parts = [`成功 ${success}`];
    if (skipped > 0) parts.push(`跳过 ${skipped}`);
    if (failed > 0) parts.push(`失败 ${failed}`);
    const message = `批量探测完成：${parts.join("，")}`;
    if (failed > 0) ElMessage.warning(message);
    else ElMessage.success(message);
    await loadSites();
  } finally {
    bulkProbing.value = false;
  }
}

function openSite(name: string) {
  const url =
    sites.value[name]?.web_url ?? sites.value[name]?.urls?.[0] ?? loginState(name)?.base_url;
  if (url) window.open(url, "_blank", "noopener");
}

function openAllEnabled() {
  const urls = allEntries.value
    .filter(([, s]) => s.enabled)
    .map(
      ([name]) =>
        sites.value[name]?.web_url ?? sites.value[name]?.urls?.[0] ?? loginState(name)?.base_url,
    )
    .filter((url): url is string => Boolean(url));

  if (urls.length === 0) {
    ElMessage.info("没有可打开的已启用站点");
    return;
  }

  if (urls.length === 1) {
    window.open(urls[0], "_blank");
    return;
  }

  let opened = 0;
  let blocked = 0;
  for (const url of urls) {
    const w = window.open(url, "_blank");
    if (w === null || typeof w === "undefined") {
      blocked++;
    } else {
      opened++;
    }
  }

  if (blocked > 0) {
    ElMessage({
      type: "warning",
      duration: 8000,
      showClose: true,
      message: `已打开 ${opened} 个站点，浏览器拦截了其余 ${blocked} 个。请在浏览器地址栏允许本站“弹出式窗口”后重试，或使用每行的“打开站点”按钮逐个打开。`,
    });
  } else {
    ElMessage.success(`已打开 ${opened} 个站点`);
  }
}

function manageSite(name: string) {
  router.push(`/sites/${name}`);
}

/**
 * 行卡「更多」菜单里的操作。
 *
 * 桌面操作列是六个图标按钮。手机上按 §9 只留**一个主操作**（整卡可点 → 站点详情），
 * 六个操作全部折进这个菜单：摊在卡上时一张卡 490 高，屏上只剩一张半。
 * 折进来不等于丢掉，六个一个不少。
 */
async function onCardCommand(cmd: { act: string; name: string }) {
  if (cmd.act === "toggle") return toggleEnabled(cmd.name);
  if (cmd.act === "open") return openSite(cmd.name);
  if (cmd.act === "probe") return probeSite(cmd.name);
  if (cmd.act === "reminder") await sendTestReminder(cmd.name);
  else if (cmd.act === "login-config") await openConfigDialog(cmd.name);
  else if (cmd.act === "delete") await deleteSite(cmd.name);
}

/** 窄屏「批量」菜单的两项 —— 与桌面那两枚按钮是同一个动作，不是另一套逻辑 */
function onBulkCommand(cmd: "probe" | "open") {
  if (cmd === "probe") return probeAllEnabled();
  return openAllEnabled();
}

function getRssCount(site: SiteConfig): number {
  return site.rss?.length || 0;
}

/* 与 openSite 用同一套取址顺序，否则会出现「按钮可点但打不开」 */
function siteUrlOf(name: string): string | undefined {
  return sites.value[name]?.web_url ?? sites.value[name]?.urls?.[0] ?? loginState(name)?.base_url;
}

const allEntries = computed(() => Object.entries(sites.value));

const enabledEntries = computed(() => allEntries.value.filter(([, s]) => s.enabled));

const enabledCount = computed(() => enabledEntries.value.length);

/** 搜索按站点名与域名匹配，取址字段和 openSite 保持一致 */
function matchesSearch(name: string, site: SiteConfig, q: string): boolean {
  if (name.toLowerCase().includes(q)) return true;
  if (site.web_url?.toLowerCase().includes(q)) return true;
  return Boolean(site.urls?.some((u) => u.toLowerCase().includes(q)));
}

/*
 * 画板 12 的 bar-64 上除了分段与搜索，还有两枚筛选 chip（「认证: 全部」「探测: 全部」），
 * 以及画板 30 那排带计数的状态 chip（正常 / 异常 / 已禁用）。
 * 三组都落在这里 —— 它们此前一个都没有，而画板把它们画在了同一条带上。
 *
 * 状态的判定口径（都用本页已经有的两份数据，没有新接口）：
 *   已禁用  !site.enabled
 *   异常    启用中，且「站点暂不可用」或最近一次探测不是 OK（含从未探测成功过）
 *   正常    启用中且不异常
 * 画板把状态画成四档分段（全部/正常/异常/已禁用），这里做成**可选的 chip**，
 * 因为两边的「全部」不是一回事：画板那张图是 14 个已配置站点（12+2=14，已禁用另算 1），
 * 而这个产品的 `/api/sites` 回的是**全部 66 个内置定义**（新装时一个都没启用）。
 * 把「全部」改成 66 会让默认视图变成 66 行，所以保留原来的「已启用 / 全部」作视图范围，
 * 状态与认证、探测一起做成叠加筛选。
 */
type SiteStatus = "ok" | "bad" | "off";

function statusOf(name: string, site: SiteConfig): SiteStatus {
  if (!site.enabled) return "off";
  if (site.unavailable) return "bad";
  const st = loginState(name)?.last_probe_status;
  /* 从未探测过不算异常：那是「还不知道」，不是「坏了」 */
  if (st !== undefined && st !== "" && !isProbeSuccess(st)) return "bad";
  return "ok";
}

/* 探测模式的中文名：导出与筛选 chip 共用一份 */
const PROBE_LABEL: Record<string, string> = { auto: "自动", manual: "手动", disabled: "不探测" };

/*
 * 画板 12 的 bar-64 右端两枚 28×28 图标钮：`bi-columns-3`（列设置）与 `bi-file-down`（导出）。
 *
 * 列设置存进 localStorage：这张表有十一列，谁关心哪几列是长期偏好，不该每次进来重设。
 * 键里带 v1，将来列集合变了可以整批失效。
 */
const COLS_KEY = "pt-tools-sites-cols-v1";

const OPTIONAL_COLS = [
  { key: "auth", label: "认证" },
  { key: "rss", label: "RSS" },
  { key: "active", label: "判定活跃" },
  { key: "days", label: "剩余天数" },
  { key: "siteActive", label: "站点活跃" },
  { key: "probe", label: "探测" },
] as const;

type OptionalCol = (typeof OPTIONAL_COLS)[number]["key"];

/*
 * 默认藏掉最边缘的两列，让默认视图接近画板 12 的列集合（# / 站点 / 状态 / 认证 /
 * RSS / 探测 / 操作）。十一列全开时「探测」那一列会被固定在右侧的「操作」压掉一半 ——
 * 实测如此。要看这两列在列设置里勾回来即可，偏好存本地。
 */
const DEFAULT_HIDDEN: OptionalCol[] = ["active", "siteActive"];

function loadHiddenCols(): Set<OptionalCol> {
  try {
    const raw = window.localStorage.getItem(COLS_KEY);
    if (!raw) return new Set(DEFAULT_HIDDEN);
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return new Set(DEFAULT_HIDDEN);
    const known = new Set(OPTIONAL_COLS.map((c) => c.key as string));
    return new Set(parsed.filter((k): k is OptionalCol => known.has(k)));
  } catch {
    /* 隐私模式下 localStorage 会抛；坏 JSON 当成没设置过 */
    return new Set(DEFAULT_HIDDEN);
  }
}

const hiddenCols = ref<Set<OptionalCol>>(loadHiddenCols());

function toggleCol(key: OptionalCol) {
  const next = new Set(hiddenCols.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  hiddenCols.value = next;
  try {
    window.localStorage.setItem(COLS_KEY, JSON.stringify([...next]));
  } catch {
    /* 存不下就只在本次会话里生效，不影响功能 */
  }
}

const colShown = (key: OptionalCol) => !hiddenCols.value.has(key);

/**
 * 导出当前表格（画板 bi-file-down）。
 *
 * 导的是**这一刻表格里的那批行**，不是整库：工具栏上那些筛选就是用来选这批行的，
 * 导出跟着筛选走才对得上用户看到的东西。字段用 CSV，Excel 与 sed 都能读。
 */
function exportCsv() {
  const head = ["站点", "状态", "认证方式", "RSS 订阅数", "剩余天数", "探测模式", "已启用"];
  const lines = [head.join(",")];
  for (const [name, site] of visibleEntries.value) {
    const days = daysRemaining(name);
    const cells = [
      name,
      STATUS_LABEL[statusOf(name, site)],
      authMethodLabel(site.auth_method),
      String(getRssCount(site)),
      days === null ? "" : String(days),
      PROBE_LABEL[probeModeOf(name)] ?? probeModeOf(name),
      site.enabled ? "是" : "否",
    ];
    /* 站点名里可能有逗号或引号，按 CSV 规则转义 */
    lines.push(cells.map((c) => `"${c.replace(/"/g, '""')}"`).join(","));
  }
  const blob = new Blob([`\uFEFF${lines.join("\n")}`], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `pt-tools-sites-${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
  ElMessage.success(`已导出 ${visibleEntries.value.length} 个站点`);
}

/** 勾中的状态。空集合 = 不筛（而不是全都不显示） */
const statusFilter = ref<Set<SiteStatus>>(new Set());
/** 认证方式筛选，空串 = 全部（画板 chip-0「认证: 全部」） */
const authFilter = ref("");
/** 探测方式筛选，空串 = 全部（画板 chip-1「探测: 全部」） */
const probeFilter = ref("");

const statusCounts = computed(() => {
  const counts: Record<SiteStatus, number> = { ok: 0, bad: 0, off: 0 };
  for (const [name, site] of allEntries.value) counts[statusOf(name, site)] += 1;
  return counts;
});

const STATUS_LABEL: Record<SiteStatus, string> = { ok: "正常", bad: "异常", off: "已禁用" };

/** 状态列与工具栏 chip 共用同一套语义色 */
function statusTone(k: SiteStatus): "ok" | "dang" | "neutral" {
  if (k === "ok") return "ok";
  if (k === "bad") return "dang";
  return "neutral";
}

const statusChips = computed(() =>
  (["ok", "bad", "off"] as const).map((k) => ({
    key: k,
    label: `${STATUS_LABEL[k]} ${statusCounts.value[k]}`,
    active: statusFilter.value.has(k),
  })),
);

function toggleStatus(k: SiteStatus) {
  const next = new Set(statusFilter.value);
  if (next.has(k)) next.delete(k);
  else next.add(k);
  statusFilter.value = next;
}

/** 认证方式的可选项，只列**本页真的出现过**的，不列产品支持但一个站点都没用的 */
const authOptions = computed(() => {
  const seen = new Set<string>();
  for (const [, site] of allEntries.value) {
    if (site.auth_method) seen.add(site.auth_method);
  }
  return [...seen].map((v) => ({ value: v, label: authMethodLabel(v) }));
});

const probeOptions = computed(() => {
  const seen = new Set<string>();
  for (const [name] of allEntries.value) seen.add(probeModeOf(name));
  return [...seen].map((v) => ({ value: v, label: PROBE_LABEL[v] ?? v }));
});

const visibleEntries = computed(() => {
  let base = viewMode.value === "all" ? allEntries.value : enabledEntries.value;
  if (statusFilter.value.size > 0) {
    base = base.filter(([name, site]) => statusFilter.value.has(statusOf(name, site)));
  }
  if (authFilter.value) {
    base = base.filter(([, site]) => site.auth_method === authFilter.value);
  }
  if (probeFilter.value) {
    base = base.filter(([name]) => probeModeOf(name) === probeFilter.value);
  }
  const q = searchKeyword.value.trim().toLowerCase();
  if (!q) return base;
  return base.filter(([name, site]) => matchesSearch(name, site, q));
});

const disabledEntries = computed(() => allEntries.value.filter(([, s]) => !s.enabled));

/* 分段控件上带计数：切过去之前就知道「全部」比「已启用」多出多少 */
const viewOptions = computed(() => [
  { label: `已启用 ${enabledCount.value}`, value: "enabled" },
  { label: `全部 ${allEntries.value.length}`, value: "all" },
]);

/* ---------------------------------------------------------------------------
 * 页头摘要与表格下方的四张分析卡
 *
 * 数字全部由本页已经加载的两份数据（站点清单 + 登录状态）算出来，没有再拉接口，
 * 也没有造占位数。登录状态接口单独失败时，靠它的两张卡没有依据，直接不出现。
 * ------------------------------------------------------------------------- */

/** 需要动手保号的档位：14 天以内才算「要管」，30 天档只是提前知会 */
const ATTENTION_TIERS = new Set<ReminderTier>(["14d", "7d", "3d", "1d", "banned-imminent"]);

const unavailableCount = computed(() => allEntries.value.filter(([, s]) => s.unavailable).length);

const enabledRssCount = computed(() =>
  enabledEntries.value.reduce((sum, [, s]) => sum + getRssCount(s), 0),
);

const enabledProbeOk = computed(
  () =>
    enabledEntries.value.filter(([name]) =>
      isProbeSuccess(loginStates.value[name]?.last_probe_status),
    ).length,
);

const enabledAttention = computed(
  () => enabledEntries.value.filter(([name]) => ATTENTION_TIERS.has(reminderTier(name))).length,
);

/**
 * 画板 head 的 sub —— 标题下面那行实时摘要（11.5/400 t3），
 * 稿上每页都是真实数字，只有页面自己算得出来，所以 Teleport 进外壳页头。
 * 口径固定看「已启用」那一档，不随分段和搜索抖动；没有站点时返回空串。
 */
const headSub = computed(() => {
  const total = allEntries.value.length;
  if (total === 0) return "";
  const parts = [
    `已启用 ${enabledCount.value} / 共 ${total} 个站点`,
    `${enabledRssCount.value} 条 RSS`,
  ];
  if (!loginStatesFailed.value) {
    parts.push(`探测正常 ${enabledProbeOk.value}`);
    if (enabledAttention.value > 0) parts.push(`${enabledAttention.value} 个需保号`);
  }
  if (unavailableCount.value > 0) parts.push(`${unavailableCount.value} 个暂不可用`);
  return parts.join(" · ");
});

interface MiniRow {
  key: string;
  label: string;
  /** 语义色，对应 .mini__bar 的 is-* */
  tone: "ok" | "warn" | "dang" | "info" | "mute";
  n: number;
}

/** 分析卡的口径 = 表格里当前这批行，卡上的数字和表格永远一致 */
const analysisTotal = computed(() => visibleEntries.value.length);

function barWidth(n: number): string {
  const total = analysisTotal.value;
  if (total <= 0 || n <= 0) return "0%";
  return `${Math.round((n / total) * 100)}%`;
}

/**
 * p-health 探测健康：按 last_probe_status 的严重级别分桶。
 * 「不适用」（NOT_APPLICABLE）与从未探测归到一起 —— 两者都不构成健康与否的判据。
 */
const probeHealth = computed<MiniRow[]>(() => {
  let ok = 0;
  let warn = 0;
  let err = 0;
  let none = 0;
  for (const [name] of visibleEntries.value) {
    const status = loginStates.value[name]?.last_probe_status;
    if (!status) {
      none += 1;
      continue;
    }
    const severity = probeStatusSeverity(status);
    if (severity === "success") ok += 1;
    else if (severity === "info") none += 1;
    else if (severity === "warning") warn += 1;
    else err += 1;
  }
  return [
    { key: "ok", label: "探测正常", tone: "ok", n: ok },
    { key: "warn", label: "会话 / 密钥告警", tone: "warn", n: warn },
    { key: "err", label: "探测失败", tone: "dang", n: err },
    { key: "none", label: "未探测 / 不适用", tone: "mute", n: none },
  ];
});

/** p-rss RSS 订阅：条数是总量，其余三行是站点数，所以只有站点数那几行画柱 */
const rssStats = computed(() => {
  let total = 0;
  let withRss = 0;
  let withoutEnabled = 0;
  let withoutDisabled = 0;
  for (const [, site] of visibleEntries.value) {
    const n = getRssCount(site);
    total += n;
    if (n > 0) withRss += 1;
    else if (site.enabled) withoutEnabled += 1;
    else withoutDisabled += 1;
  }
  const rows: MiniRow[] = [
    { key: "with", label: "已配置订阅", tone: "ok", n: withRss },
    { key: "miss", label: "已启用但没有订阅", tone: "warn", n: withoutEnabled },
  ];
  if (withoutDisabled > 0) {
    rows.push({ key: "off", label: "未启用且没有订阅", tone: "mute", n: withoutDisabled });
  }
  return { total, rows };
});

/**
 * p-ev 保号提醒：按封禁提醒档位分桶。
 * 画板只给了这张卡的位置和尺寸，没给内容；本页唯一有依据的「事件」就是保号档位。
 */
const TIER_GROUPS: { key: string; label: string; tone: MiniRow["tone"]; tiers: ReminderTier[] }[] =
  [
    { key: "ok", label: "正常", tone: "ok", tiers: ["none"] },
    { key: "soon", label: "30 天内", tone: "info", tiers: ["pre-warn", "30d"] },
    { key: "warn", label: "14 / 7 天内", tone: "warn", tiers: ["14d", "7d"] },
    {
      key: "crit",
      label: "3 天内 / 即将封禁",
      tone: "dang",
      tiers: ["3d", "1d", "banned-imminent"],
    },
    { key: "unknown", label: "未知", tone: "mute", tiers: ["unknown"] },
  ];

const tierStats = computed<MiniRow[]>(() =>
  TIER_GROUPS.map((g) => ({
    key: g.key,
    label: g.label,
    tone: g.tone,
    n: visibleEntries.value.filter(([name]) => g.tiers.includes(reminderTier(name))).length,
  })),
);

const tierAttention = computed(
  () => visibleEntries.value.filter(([name]) => ATTENTION_TIERS.has(reminderTier(name))).length,
);

/** p-auth 认证方式：按 auth_method 归并，多的排前面 */
const authStats = computed<MiniRow[]>(() => {
  const counts = new Map<string, number>();
  for (const [, site] of visibleEntries.value) {
    const label = authMethodLabel(site.auth_method);
    counts.set(label, (counts.get(label) ?? 0) + 1);
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([label, n]) => ({ key: label, label, tone: "info" as const, n }));
});

const addCandidates = computed(() => {
  const q = addSearch.value.trim().toLowerCase();
  if (!q) return disabledEntries.value;
  return disabledEntries.value.filter(([name, s]) => {
    if (name.toLowerCase().includes(q)) return true;
    if (s.urls?.some((u) => u.toLowerCase().includes(q))) return true;
    return false;
  });
});

function openAddDialog() {
  addSearch.value = "";
  addDialogVisible.value = true;
}

async function enableSiteFromDialog(name: string) {
  const site = sites.value[name];
  if (!site || enablingInDialog[name]) return;
  enablingInDialog[name] = true;
  const snapshot = site.enabled;
  site.enabled = true;
  try {
    await sitesApi.save(name, site);
    ElMessage.success(`已启用 ${name}`);
  } catch (e: unknown) {
    site.enabled = snapshot;
    ElMessage.error((e as Error).message || "启用失败");
  } finally {
    enablingInDialog[name] = false;
  }
}

function configureFromDialog(name: string) {
  addDialogVisible.value = false;
  router.push(`/sites/${name}`);
}

function authMethodLabel(method?: string): string {
  switch (method) {
    case "api_key":
      return "API Key";
    case "cookie_and_api_key":
      return "Cookie + API";
    case "passkey":
      return "Passkey";
    default:
      return "Cookie";
  }
}

/* 保号档位是状态而不是分类，所以走 PtStatusPill；这里把 el-tag 的类型名折到胶囊的语气上 */
function tierTone(tier: ReminderTier): "ok" | "warn" | "dang" | "info" | "neutral" {
  switch (tierTagType(tier)) {
    case "danger":
      return "dang";
    case "warning":
      return "warn";
    case "primary":
      return "info";
    case "info":
      return "neutral";
    default:
      return "ok";
  }
}

async function changeProbeMode(name: string, mode: "auto" | "manual" | "disabled") {
  const st = loginStates.value[name];
  const previous = probeModeOf(name);
  if (previous === mode) return;
  if (updatingMode[name]) return;
  updatingMode[name] = true;
  if (st) st.probe_mode = mode;
  try {
    await sitesApi.updateProbeMode(name, mode);
    ElMessage.success("探测模式已更新");
  } catch (e: unknown) {
    if (st) st.probe_mode = previous;
    ElMessage.error((e as Error).message || "更新失败");
  } finally {
    updatingMode[name] = false;
  }
}

interface LoginConfigForm {
  ban_threshold_days: number;
  remind_before_days: number;
  reminder_cron: string;
  notification_channel_ids: number[];
  probe_mode: "auto" | "manual" | "disabled";
}

const configDialogVisible = ref(false);
const configSaving = ref(false);
const configSiteName = ref("");
const notifyChannels = ref<{ id: number; name: string }[]>([]);
const configForm = reactive<LoginConfigForm>({
  ban_threshold_days: 30,
  remind_before_days: 10,
  reminder_cron: "0 10,22 * * *",
  notification_channel_ids: [],
  probe_mode: "auto",
});

async function loadNotifyChannels() {
  if (notifyChannels.value.length > 0) return;
  try {
    const list = await chatopsApi.notifications.list();
    notifyChannels.value = list.map((c) => ({ id: c.id, name: c.name }));
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载通知通道失败");
  }
}

async function openConfigDialog(name: string) {
  configSiteName.value = name;
  const st = loginStates.value[name];
  configForm.ban_threshold_days = st?.ban_threshold_days ?? 30;
  configForm.remind_before_days = st?.remind_before_days ?? 10;
  configForm.reminder_cron = st?.reminder_cron || "0 10,22 * * *";
  configForm.notification_channel_ids = [...(st?.notification_channel_ids ?? [])];
  configForm.probe_mode = probeModeOf(name);
  configDialogVisible.value = true;
  await loadNotifyChannels();
}

async function saveLoginConfig() {
  const name = configSiteName.value;
  if (!name) return;
  configSaving.value = true;
  try {
    await sitesApi.updateLoginConfig(name, {
      ban_threshold_days: configForm.ban_threshold_days,
      remind_before_days: configForm.remind_before_days,
      reminder_cron: configForm.reminder_cron,
      notification_channel_ids: configForm.notification_channel_ids,
      probe_mode: configForm.probe_mode,
    });
    const st = loginStates.value[name];
    if (st) {
      st.ban_threshold_days = configForm.ban_threshold_days;
      st.remind_before_days = configForm.remind_before_days;
      st.reminder_cron = configForm.reminder_cron;
      st.notification_channel_ids = [...configForm.notification_channel_ids];
      st.probe_mode = configForm.probe_mode;
    }
    ElMessage.success("保号配置已更新");
    configDialogVisible.value = false;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "更新失败");
  } finally {
    configSaving.value = false;
  }
}
</script>

<template>
  <div class="sites-page">
    <!-- 画板 head 的 sub：标题下面那行实时摘要，由本页把真实数字送进外壳页头 -->
    <PtHeadSub v-if="headSub">{{ headSub }}</PtHeadSub>

    <!--
      两条提示都不是带：画板 27 的落法是提示跟在顶部带（这里是外壳页头）之后、
      左右各内缩 16。它们不属于画板构成，关掉之后页面就与画板一致。

      partial（§5）：站点清单拿到了但登录状态没拿到。有数据可看时不该用一整块状态图
      顶掉表格 —— 那等于把已经拿到的也藏了，所以挂一条提示，表格照常渲染。
    -->
    <div
      v-if="hasPartialBanner(visibleEntries.length)"
      class="pt-note pt-note--warn page-note"
      data-testid="sites-partial-note">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span class="page-note__text">
        站点清单已加载，但登录状态接口没有返回：「判定活跃」「剩余天数」「探测模式」暂时没有依据。
      </span>
      <el-button link type="primary" size="small" @click="loadSites">重试</el-button>
    </div>

    <div v-if="riskHintOpen" class="pt-note pt-note--warn page-note">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span class="page-note__text">
        活跃时间来自 cookie/API 探测，能刷新多数站点的 last_access（最近动向）用于保号；
        但少数站点按
        last_login（实际登录）或做种活跃度清理，这类站点仍需定期手动登录，别只看这里的数字。
      </span>
      <button
        type="button"
        class="page-note__x"
        aria-label="关闭提示"
        @click="riskHintOpen = false">
        <PtIcon name="x" :size="14" />
      </button>
    </div>

    <!--
      画板 12 的主区：bar-64（40 高工具栏带）+ grid（438 高表格带）都是全宽平铺的带，
      不是圆角描边卡片，所以表格不再包在 PtPanel 里。
    -->
    <PtToolbar band>
      <el-segmented
        v-model="viewMode"
        class="pt-seg"
        :options="viewOptions"
        data-testid="site-view-toggle" />
      <el-input
        v-model="searchKeyword"
        class="bar-search"
        placeholder="搜索：站点名称 / 域名"
        clearable
        data-testid="site-search">
        <template #prefix>
          <PtIcon name="search" :size="15" />
        </template>
      </el-input>

      <!--
        画板 30 那排带计数的状态 chip（正常 / 异常 / 已禁用）。可叠加：
        「异常 + 已禁用」是「需要我处理的」这个真实组合，做成互斥就把它删掉了。
      -->
      <el-button
        v-for="c in statusChips"
        :key="c.key"
        class="bar-chip__btn"
        :type="c.active ? 'primary' : 'default'"
        :plain="c.active"
        :aria-pressed="c.active"
        :data-testid="`site-status-chip-${c.key}`"
        @click="toggleStatus(c.key)">
        <span>{{ c.label }}</span>
      </el-button>

      <!-- 画板 12 的 chip-0「认证: 全部」/ chip-1「探测: 全部」：点开选一项 -->
      <el-select
        v-model="authFilter"
        class="bar-chip"
        size="small"
        placeholder="认证: 全部"
        clearable
        data-testid="site-auth-filter">
        <el-option label="认证: 全部" value="" />
        <el-option
          v-for="o in authOptions"
          :key="o.value"
          :label="`认证: ${o.label}`"
          :value="o.value" />
      </el-select>
      <el-select
        v-model="probeFilter"
        class="bar-chip"
        size="small"
        placeholder="探测: 全部"
        clearable
        data-testid="site-probe-filter">
        <el-option label="探测: 全部" value="" />
        <el-option
          v-for="o in probeOptions"
          :key="o.value"
          :label="`探测: ${o.label}`"
          :value="o.value" />
      </el-select>

      <template #right>
        <span v-if="isFiltered" class="pt-band__note">筛出 {{ visibleEntries.length }} 个</span>

        <!-- 画板 bar-64 右端的 bi-columns-3：列设置（偏好存本地） -->
        <el-popover placement="bottom-end" trigger="click" :width="180">
          <template #reference>
            <button
              type="button"
              class="pt-band__iconbtn"
              aria-label="列设置"
              data-testid="sites-cols-btn">
              <PtIcon name="columns-3" :size="15" />
            </button>
          </template>
          <div class="cols">
            <label v-for="c in OPTIONAL_COLS" :key="c.key" class="cols__row">
              <el-checkbox :model-value="colShown(c.key)" @change="toggleCol(c.key)" />
              <span>{{ c.label }}</span>
            </label>
          </div>
        </el-popover>

        <!-- 画板 bar-64 右端的 bi-file-down：导出当前筛选出的这批行 -->
        <el-tooltip content="按当前筛选导出 CSV" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            aria-label="导出"
            data-testid="sites-export-btn"
            @click="exportCsv">
            <PtIcon name="file-down" :size="15" />
          </button>
        </el-tooltip>

        <el-tooltip content="重新拉取站点清单与登录状态" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            aria-label="刷新"
            :disabled="loading"
            data-testid="sites-refresh-btn"
            @click="loadSites">
            <PtIcon
              :name="loading ? 'loader-circle' : 'refresh-cw'"
              :size="15"
              :class="{ 'pt-spin': loading }" />
          </button>
        </el-tooltip>

        <!--
          画板 head 右侧动作（y=16、高 32、右端对齐）：两枚次按钮 + 一枚主按钮，
          主操作一律上页头，工具栏只留筛选类控件。

          但 ≤768px 时外壳把 .pt-head 整条 display:none 了，飞进去的按钮会跟着消失，
          「新增站点」在手机上就没有入口了。所以窄屏把 Teleport 关掉，
          三枚按钮就地留在工具栏里 —— 这也正是它们原来的位置。
        -->
        <Teleport to="#pt-head-acts" :disabled="isMobile">
          <!--
            桌面：三枚按钮平铺在页头动作区（画板 head 的右侧动作位）。
            手机：画板 30 的顶栏与筛选行上**没有画这三个入口**，而平铺三枚 44 高的按钮会
            各占一行，把筛选区顶到 240 高，第一张行卡被推到屏幕下半。
            所以窄屏只留「新增站点」这一枚主操作，另两枚收进一个「批量」菜单 ——
            入口一个都没少（用户验收退回过的是「重复」，不是「少」），高度从 240 降到一行。
          -->
          <el-dropdown v-if="isMobile" trigger="click" @command="onBulkCommand">
            <el-button size="small" data-testid="site-bulk-menu">
              <PtIcon name="ellipsis" :size="15" /><span>批量</span>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item
                  command="probe"
                  :disabled="loading || enabledCount === 0 || bulkProbing"
                  data-testid="probe-all-enabled-button">
                  <PtIcon name="activity" :size="14" class="dd-ico" /><span>探测已启用</span>
                </el-dropdown-item>
                <el-dropdown-item
                  command="open"
                  :disabled="enabledCount === 0"
                  data-testid="open-all-sites-btn">
                  <PtIcon name="external-link" :size="14" class="dd-ico" /><span>打开已启用</span>
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>

          <template v-else>
            <el-tooltip
              content="对所有已启用站点执行一次登录状态探测，最多 3 个并发"
              placement="bottom">
              <el-button
                size="small"
                :loading="bulkProbing"
                :disabled="loading || enabledCount === 0"
                data-testid="probe-all-enabled-button"
                @click="probeAllEnabled">
                <PtIcon name="activity" :size="15" /><span>探测已启用</span>
              </el-button>
            </el-tooltip>
            <el-button
              size="small"
              :disabled="enabledCount === 0"
              data-testid="open-all-sites-btn"
              @click="openAllEnabled">
              <PtIcon name="external-link" :size="15" /><span>打开已启用</span>
            </el-button>
          </template>

          <el-button
            type="primary"
            size="small"
            data-testid="add-site-button"
            @click="openAddDialog">
            <PtIcon name="plus" :size="15" /><span>新增站点</span>
          </el-button>
        </Teleport>
      </template>
    </PtToolbar>

    <div v-loading="loading" class="pt-band--grid">
      <el-table
        v-if="!isMobile"
        :data="visibleEntries"
        :row-key="(row: [string, SiteConfig]) => row[0]"
        class="pt-grid"
        style="width: 100%">
        <template #empty>
          <PtDataState :state="tableState" dense :title="stateTitle" :sub="stateSub">
            <template v-if="tableState === 'error' || tableState === 'partial'" #action>
              <el-button size="small" @click="loadSites">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
            <!-- 筛出 0 行时该做的是放宽条件，不是去新增一个已经存在的站点 -->
            <template v-else-if="isFiltered" #action>
              <el-button size="small" @click="searchKeyword = ''">
                <PtIcon name="x" :size="14" /><span>清空搜索</span>
              </el-button>
            </template>
            <template v-else-if="tableState === 'empty'" #action>
              <el-button type="primary" size="small" @click="openAddDialog">
                <PtIcon name="plus" :size="14" /><span>新增站点</span>
              </el-button>
            </template>
          </PtDataState>
        </template>

        <!--
          画板 12 的 th-0「#」：行号。只用来「第几行」地对话，不参与排序 ——
          排序换了顺序，行号就该跟着重排，所以取的是渲染下标而不是数据里的序号。
        -->
        <el-table-column label="#" width="56" align="center" class-name="pt-cell-muted">
          <template #default="{ $index }">{{ $index + 1 }}</template>
        </el-table-column>

        <el-table-column label="站点" min-width="170" class-name="pt-cell-strong">
          <template #default="{ row }">
            <span class="site">
              <SiteAvatar :site-id="row[0]" :site-name="row[0]" :size="22" :no-fetch="true" />
              <span class="site__name">{{ row[0] }}</span>
            </span>
          </template>
        </el-table-column>

        <!--
          画板 12 的 th-2「状态」：把「暂不可用」那枚胶囊从站点列独立出来，
          并把它扩成三态（正常 / 异常 / 已禁用）—— 和工具栏那排状态 chip 同一套口径
          （statusOf），否则筛选与显示会各说一套。
        -->
        <el-table-column label="状态" width="96" align="center">
          <template #default="{ row }">
            <PtStatusPill :tone="statusTone(statusOf(row[0], row[1]))" size="sm">
              {{ STATUS_LABEL[statusOf(row[0], row[1])] }}
            </PtStatusPill>
          </template>
        </el-table-column>

        <el-table-column v-if="colShown('auth')" label="认证" width="118">
          <template #default="{ row }">
            <PtTag>{{ authMethodLabel(row[1].auth_method) }}</PtTag>
          </template>
        </el-table-column>

        <el-table-column
          v-if="colShown('rss')"
          label="RSS"
          width="78"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">
            <span class="rss" :class="{ 'is-zero': getRssCount(row[1]) === 0 }">
              <PtIcon name="rss" :size="13" />
              {{ getRssCount(row[1]) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column v-if="colShown('active')" min-width="104" class-name="pt-cell-muted">
          <template #header>
            <el-tooltip
              content="用于封禁提醒判定的有效活跃时间，优先使用站点返回的 last_access；不是网页登录时间"
              placement="top">
              <span class="th-help">判定活跃 <PtIcon name="info" :size="12" /></span>
            </el-tooltip>
          </template>
          <template #default="{ row }">
            <span :data-testid="`last-login-cell-${row[0]}`" class="ts">
              {{ formatTimeAgo(effectiveLastActive(row[0])) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column v-if="colShown('days')" min-width="118">
          <template #header>
            <el-tooltip content="距离站点封禁阈值的剩余天数；负数表示已超过阈值" placement="top">
              <span class="th-help">剩余天数 <PtIcon name="info" :size="12" /></span>
            </el-tooltip>
          </template>
          <template #default="{ row }">
            <span class="days">
              <span :data-testid="`days-remaining-cell-${row[0]}`" :class="daysCellClass(row[0])">
                {{ daysRemaining(row[0]) === null ? "—" : `${daysRemaining(row[0])} 天` }}
              </span>
              <PtStatusPill :tone="tierTone(reminderTier(row[0]))" size="sm">
                {{ tierLabel(reminderTier(row[0])) }}
              </PtStatusPill>
            </span>
          </template>
        </el-table-column>

        <el-table-column v-if="colShown('siteActive')" min-width="104" class-name="pt-cell-muted">
          <template #header>
            <el-tooltip
              content="站点/API 返回的原始 last_access 或 lastBrowse 时间"
              placement="top">
              <span class="th-help">站点活跃 <PtIcon name="info" :size="12" /></span>
            </el-tooltip>
          </template>
          <template #default="{ row }">
            <span :data-testid="`last-access-cell-${row[0]}`" class="ts">
              {{ formatTimeAgo(lastAccess(row[0])) }}
            </span>
          </template>
        </el-table-column>

        <!-- 「自动」那个下拉实测要 133，104 会把它裁掉 -->
        <el-table-column v-if="colShown('probe')" label="探测" width="140">
          <template #default="{ row }">
            <el-select
              :model-value="probeModeOf(row[0])"
              size="small"
              :disabled="updatingMode[row[0]]"
              :data-testid="`probe-mode-select-${row[0]}`"
              class="mode-sel"
              @change="(value: 'auto' | 'manual' | 'disabled') => changeProbeMode(row[0], value)">
              <el-option label="自动" value="auto" />
              <el-option label="手动" value="manual" />
              <el-option label="禁用" value="disabled" />
            </el-select>
          </template>
        </el-table-column>

        <el-table-column label="启用" width="72" align="center">
          <template #default="{ row }">
            <el-tooltip
              :content="
                row[1].unavailable
                  ? row[1].unavailable_reason || '该站点暂不可用'
                  : row[1].enabled
                    ? '点一下停用'
                    : '点一下启用'
              "
              placement="top">
              <span class="sw">
                <el-switch
                  :model-value="row[1].enabled"
                  size="small"
                  :disabled="row[1].unavailable"
                  @change="toggleEnabled(row[0])" />
              </span>
            </el-tooltip>
          </template>
        </el-table-column>

        <!--
          一行有六个动作，写上文字就要 400 宽，把前面几列挤成两行。
          这里只留图标 + tooltip，图标顺序按使用频率排：先看站点、再探测，删除放最后。
        -->
        <el-table-column label="操作" width="212" fixed="right" class-name="pt-cell-act">
          <template #default="{ row }">
            <el-tooltip
              :content="siteUrlOf(row[0]) ? '打开站点' : '未配置站点地址'"
              placement="top">
              <span>
                <el-button
                  link
                  type="primary"
                  size="small"
                  aria-label="打开站点"
                  :disabled="!siteUrlOf(row[0])"
                  :data-testid="`open-site-btn-${row[0]}`"
                  @click="openSite(row[0])">
                  <PtIcon name="external-link" :size="15" />
                </el-button>
              </span>
            </el-tooltip>

            <el-tooltip content="立即探测登录状态" placement="top">
              <span>
                <el-button
                  link
                  type="primary"
                  size="small"
                  aria-label="立即探测"
                  :loading="probing[row[0]]"
                  :disabled="!row[1].enabled || probing[row[0]]"
                  :data-testid="`probe-button-${row[0]}`"
                  @click="probeSite(row[0])">
                  <PtIcon name="activity" :size="15" />
                </el-button>
              </span>
            </el-tooltip>

            <el-tooltip content="发一条测试提醒到通知通道" placement="top">
              <span>
                <el-button
                  link
                  type="primary"
                  size="small"
                  aria-label="测试提醒"
                  :loading="testingReminder[row[0]]"
                  :disabled="!row[1].enabled || testingReminder[row[0]]"
                  :data-testid="`test-reminder-btn-${row[0]}`"
                  @click="sendTestReminder(row[0])">
                  <PtIcon name="bell-ring" :size="15" />
                </el-button>
              </span>
            </el-tooltip>

            <el-tooltip content="站点配置与 RSS 订阅" placement="top">
              <el-button
                link
                type="primary"
                size="small"
                aria-label="站点配置"
                @click="manageSite(row[0])">
                <PtIcon name="sliders-horizontal" :size="15" />
              </el-button>
            </el-tooltip>

            <el-tooltip content="保号配置：封号阈值、提醒时间、通知通道" placement="top">
              <el-button
                link
                type="primary"
                size="small"
                aria-label="保号配置"
                :data-testid="`login-config-btn-${row[0]}`"
                @click="openConfigDialog(row[0])">
                <PtIcon name="shield" :size="15" />
              </el-button>
            </el-tooltip>

            <el-tooltip
              :content="row[1].is_builtin ? '预置站点不可删除' : '删除站点'"
              placement="top">
              <span>
                <el-button
                  link
                  type="danger"
                  size="small"
                  aria-label="删除站点"
                  :disabled="row[1].is_builtin"
                  @click="deleteSite(row[0])">
                  <PtIcon name="trash-2" :size="15" />
                </el-button>
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>

      <!--
        移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
        这张表桌面有 9 列、操作列里还有六个按钮，手机上横着滚既看不到列头，
        也和页面本身的纵向滚动打架。卡上留真正要看的：站点名 + 认证/RSS/活跃/剩余天数
        + 保号档位，操作收进底部一排 44 高的按钮。
      -->
      <div v-else class="cards">
        <PtDataState
          v-if="!visibleEntries.length"
          :state="tableState"
          :title="stateTitle"
          :sub="stateSub">
          <template v-if="tableState === 'error' || tableState === 'partial'" #action>
            <el-button size="small" @click="loadSites">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
          <template v-else-if="isFiltered" #action>
            <el-button size="small" @click="searchKeyword = ''">
              <PtIcon name="x" :size="14" /><span>清空搜索</span>
            </el-button>
          </template>
          <template v-else-if="tableState === 'empty'" #action>
            <el-button type="primary" size="small" @click="openAddDialog">
              <PtIcon name="plus" :size="14" /><span>新增站点</span>
            </el-button>
          </template>
        </PtDataState>

        <!--
          整卡可点 = 进站点详情（画板 32）。画板 30 的行卡上**一个按钮都没有**，
          §9 也写的是「两行文本 + 一条进度 + 一个主操作」——
          之前这里摊了四个按钮加一个「更多」，一张卡从画板的 88 涨到 490，
          屏上只剩一张半卡。所以主操作交给整卡，其余全部折进「更多」。
        -->
        <PtRowCard
          v-for="[name, site] in visibleEntries"
          :key="name"
          interactive
          :data-testid="`site-card-${name}`"
          @click="manageSite(name)">
          <template #lead>
            <SiteAvatar :site-id="name" :site-name="name" :size="28" :no-fetch="true" />
          </template>

          <template #title>
            <span class="card-name">{{ name }}</span>
          </template>

          <template #meta>
            <PtTag>{{ authMethodLabel(site.auth_method) }}</PtTag>
            <span class="rss" :class="{ 'is-zero': getRssCount(site) === 0 }">
              <PtIcon name="rss" :size="11" />
              {{ getRssCount(site) }} 条 RSS
            </span>
            <span :data-testid="`last-login-cell-${name}`" class="ts">
              <PtIcon name="clock" :size="11" />
              活跃 {{ formatTimeAgo(effectiveLastActive(name)) }}
            </span>
            <span :data-testid="`days-remaining-cell-${name}`" :class="daysCellClass(name)">
              {{ daysRemaining(name) === null ? "剩余 —" : `剩余 ${daysRemaining(name)} 天` }}
            </span>
          </template>

          <!-- 暂不可用时它比保号档位更要紧：站点用不了，档位也就没有意义 -->
          <template #status>
            <PtStatusPill v-if="site.unavailable" tone="dang" size="sm">暂不可用</PtStatusPill>
            <PtStatusPill v-else :tone="tierTone(reminderTier(name))" size="sm">
              {{ tierLabel(reminderTier(name)) }}
            </PtStatusPill>
          </template>

          <template #actions>
            <el-dropdown class="card-more" trigger="click" @command="onCardCommand">
              <el-button size="small" :data-testid="`site-more-btn-${name}`" @click.stop>
                <PtIcon name="ellipsis" :size="14" /><span>更多</span>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item
                    :command="{ act: 'toggle', name }"
                    :disabled="site.unavailable"
                    :data-testid="`site-toggle-btn-${name}`">
                    <PtIcon
                      :name="site.enabled ? 'circle-pause' : 'circle-check'"
                      :size="14"
                      class="dd-ico" />
                    <span>{{ site.enabled ? "停用" : "启用" }}</span>
                  </el-dropdown-item>
                  <el-dropdown-item
                    :command="{ act: 'open', name }"
                    :disabled="!siteUrlOf(name)"
                    :data-testid="`open-site-btn-${name}`">
                    <PtIcon name="external-link" :size="14" class="dd-ico" /><span>打开</span>
                  </el-dropdown-item>
                  <el-dropdown-item
                    :command="{ act: 'probe', name }"
                    :disabled="!site.enabled || probing[name]"
                    :data-testid="`probe-button-${name}`">
                    <PtIcon name="activity" :size="14" class="dd-ico" /><span>探测</span>
                  </el-dropdown-item>
                  <el-dropdown-item
                    :command="{ act: 'reminder', name }"
                    :disabled="!site.enabled || testingReminder[name]">
                    <PtIcon name="bell-ring" :size="14" class="dd-ico" /><span>测试提醒</span>
                  </el-dropdown-item>
                  <el-dropdown-item :command="{ act: 'login-config', name }">
                    <PtIcon name="shield" :size="14" class="dd-ico" /><span>保号配置</span>
                  </el-dropdown-item>
                  <el-dropdown-item
                    divided
                    :command="{ act: 'delete', name }"
                    :disabled="site.is_builtin">
                    <PtIcon name="trash-2" :size="14" class="dd-ico" /><span>删除站点</span>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </PtRowCard>
      </div>
    </div>

    <!--
      画板 gfoot 34：左侧是计数口径，右侧本该是分页（200×30），
      但这一页不分页 —— 站点数量是几十的量级，一次全给完，所以右侧改放读数提醒。
    -->
    <div v-if="visibleEntries.length > 0" class="pt-band--foot">
      <span>
        已启用 {{ enabledCount }} · 全部 {{ allEntries.length }} · 当前显示
        {{ visibleEntries.length }} 个
      </span>
      <span class="pt-band__spacer" />
      <span class="foot-caveat">
        「判定活跃」是保号判据，「站点活跃」是站点原始返回值，两者不一致时以前者为准
      </span>
    </div>

    <!--
      画板 12 表格下方的卡片层：p-health / p-rss / p-ev 三张并排（364/340/344）
      + p-auth 通栏（1080）。四张卡的数字全部由本页已加载的数据算出，口径是表格里
      当前这批行，所以卡上的数和表格永远对得上；登录状态接口失败时前两张没有依据，
      直接不出现，不留空壳。
    -->
    <!--
      画板 12 的分析卡是三栏 364 / 340 / 344 加一张 1080 通栏（p-auth），
      不是等分 —— .pt-cards 的默认 auto-fit 会把三张都排成 349。
    -->
    <div v-if="visibleEntries.length > 0" class="pt-cards pt-cards--3">
      <PtPanel
        v-if="!loginStatesFailed"
        title="探测健康"
        icon="activity"
        :count="`${analysisTotal} 个`">
        <ul class="mini">
          <li v-for="row in probeHealth" :key="row.key" class="mini__row">
            <span class="mini__k">{{ row.label }}</span>
            <span class="mini__v">{{ row.n }}</span>
            <span class="mini__bar" :class="`is-${row.tone}`" aria-hidden="true">
              <span class="mini__fill" :style="{ width: barWidth(row.n) }" />
            </span>
          </li>
        </ul>
      </PtPanel>

      <PtPanel title="RSS 订阅" icon="rss" :count="`${rssStats.total} 条`">
        <ul class="mini">
          <li v-for="row in rssStats.rows" :key="row.key" class="mini__row">
            <span class="mini__k">{{ row.label }}</span>
            <span class="mini__v">{{ row.n }}</span>
            <span class="mini__bar" :class="`is-${row.tone}`" aria-hidden="true">
              <span class="mini__fill" :style="{ width: barWidth(row.n) }" />
            </span>
          </li>
        </ul>
        <p class="mini__foot">
          订阅总条数 {{ rssStats.total }} 条，只有已启用站点的订阅会参与 RSS 任务
        </p>
      </PtPanel>

      <PtPanel
        v-if="!loginStatesFailed"
        title="保号提醒"
        icon="bell-ring"
        :count="tierAttention > 0 ? `需关注 ${tierAttention} 个` : '暂无需关注'">
        <ul class="mini">
          <li v-for="row in tierStats" :key="row.key" class="mini__row">
            <span class="mini__k">{{ row.label }}</span>
            <span class="mini__v">{{ row.n }}</span>
            <span class="mini__bar" :class="`is-${row.tone}`" aria-hidden="true">
              <span class="mini__fill" :style="{ width: barWidth(row.n) }" />
            </span>
          </li>
        </ul>
      </PtPanel>

      <PtPanel class="pt-cards__full" title="认证方式" icon="shield" :count="`${analysisTotal} 个`">
        <ul class="mini mini--cols">
          <li v-for="row in authStats" :key="row.key" class="mini__row">
            <span class="mini__k">{{ row.label }}</span>
            <span class="mini__v">{{ row.n }}</span>
            <span class="mini__bar" :class="`is-${row.tone}`" aria-hidden="true">
              <span class="mini__fill" :style="{ width: barWidth(row.n) }" />
            </span>
          </li>
        </ul>
        <p class="mini__foot">
          认证方式来自站点定义：Cookie 走浏览器扩展同步，API Key / Passkey 需要在站点配置里填。
        </p>
      </PtPanel>
    </div>

    <el-dialog
      v-model="addDialogVisible"
      class="pt-dialog"
      title="添加站点"
      width="640px"
      align-center
      data-testid="add-site-dialog"
      append-to-body>
      <el-input
        v-model="addSearch"
        placeholder="搜索：站点名称 / 域名"
        clearable
        data-testid="add-site-search"
        class="cand-search">
        <template #prefix>
          <PtIcon name="search" :size="14" />
        </template>
      </el-input>

      <el-scrollbar max-height="420px">
        <div v-if="addCandidates.length > 0" class="cands">
          <div v-for="[name, site] in addCandidates" :key="name" class="cand">
            <SiteAvatar :site-id="name" :site-name="name" :size="34" :no-fetch="true" />
            <span class="cand__meta">
              <span class="cand__name">{{ name }}</span>
              <span class="cand__tags">
                <PtTag>{{ authMethodLabel(site.auth_method) }}</PtTag>
                <PtStatusPill v-if="site.unavailable" tone="dang" size="sm">暂不可用</PtStatusPill>
              </span>
            </span>
            <span class="cand__acts">
              <el-tooltip
                :disabled="!site.unavailable"
                :content="site.unavailable_reason || '该站点暂不可用'"
                placement="top">
                <span>
                  <el-button
                    type="primary"
                    size="small"
                    :disabled="site.unavailable"
                    :loading="enablingInDialog[name]"
                    :data-testid="`enable-site-btn-${name}`"
                    @click="enableSiteFromDialog(name)">
                    启用
                  </el-button>
                </span>
              </el-tooltip>
              <el-button size="small" @click="configureFromDialog(name)">配置</el-button>
            </span>
          </div>
        </div>
        <PtDataState
          v-else
          :state="addSearch ? 'zero' : 'empty'"
          :title="addSearch ? '没有匹配的站点' : '支持的站点都已启用'"
          :sub="addSearch ? '换个名字或域名再搜' : '需要新站点的话，往下看提交入口'" />
      </el-scrollbar>

      <template #footer>
        <span class="pt-foot-note cand-hint">
          需要适配新站点？安装
          <a href="https://github.com/sunerpy/pt-tools/releases" target="_blank" rel="noopener">
            浏览器扩展
          </a>
          采集数据后按
          <a
            href="https://github.com/sunerpy/pt-tools/blob/main/docs/guide/request-new-site.md"
            target="_blank"
            rel="noopener">
            指南
          </a>
          提交 Issue
        </span>
        <el-button @click="addDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="configDialogVisible"
      class="pt-dialog"
      :title="`保号配置 · ${configSiteName}`"
      width="520px"
      align-center
      data-testid="login-config-dialog"
      append-to-body>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <div class="field-row">
          <el-form-item label="封号判定天数">
            <el-input-number
              v-model="configForm.ban_threshold_days"
              :min="1"
              :max="365"
              controls-position="right"
              style="width: 100%"
              data-testid="login-config-ban-threshold" />
            <div class="field-tip">不活跃超过这个天数，站点就可能判定封号</div>
          </el-form-item>

          <el-form-item label="提前提醒天数">
            <el-input-number
              v-model="configForm.remind_before_days"
              :min="1"
              :max="365"
              controls-position="right"
              style="width: 100%"
              data-testid="login-config-remind-before" />
            <div class="field-tip">距封号还剩多少天开始提醒</div>
          </el-form-item>
        </div>

        <el-form-item>
          <template #label>
            <el-tooltip placement="top">
              <template #content>
                标准 5 字段 cron：分 时 日 月 周。<br />
                <code>0 10,22 * * *</code> = 每天 10:00 与 22:00 各提醒一次。<br />
                示例：<code>0 9 * * *</code> 每天 9 点；<code>0 */6 * * *</code> 每 6 小时；
                <code>30 8 * * 1</code> 每周一 8:30。
              </template>
              <span class="th-help">提醒 cron <PtIcon name="info" :size="12" /></span>
            </el-tooltip>
          </template>
          <el-input
            v-model="configForm.reminder_cron"
            placeholder="0 10,22 * * *"
            data-testid="login-config-cron" />
          <div class="field-tip">5 字段（分 时 日 月 周），留空按默认 0 10,22 * * * 走</div>
        </el-form-item>

        <el-form-item label="通知通道">
          <el-select
            v-model="configForm.notification_channel_ids"
            multiple
            clearable
            placeholder="留空 = 发送到所有已启用通知通道"
            style="width: 100%"
            data-testid="login-config-channels">
            <el-option v-for="ch in notifyChannels" :key="ch.id" :label="ch.name" :value="ch.id" />
          </el-select>
          <div class="field-tip">
            选了就只发选中的通道，而且通道本身也得是启用状态；留空则发给所有已启用通道
          </div>
        </el-form-item>

        <el-form-item label="探测模式">
          <el-select
            v-model="configForm.probe_mode"
            style="width: 100%"
            data-testid="login-config-probe-mode">
            <el-option label="自动" value="auto" />
            <el-option label="手动" value="manual" />
            <el-option label="禁用" value="disabled" />
          </el-select>
          <div class="field-tip">自动 = 跟随定时任务；手动 = 只在点「立即探测」时执行</div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="configDialogVisible = false">取消</el-button>
        <el-button
          type="primary"
          :loading="configSaving"
          data-testid="login-config-save"
          @click="saveLoginConfig">
          保存
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/*
 * 主区是一串全宽横向带（工具栏 → 表格 → 页脚带 → 卡片区），带与带之间没有间距，
 * 所以这里不给 gap；要内缩的东西（提示、卡片区、移动端行卡）自己带 16 的留白。
 */
.sites-page {
  display: flex;
  flex-direction: column;
}

/* 提示不是带：画板里它在卡片层，左右各内缩 16，贴在页头之后 */
.page-note {
  align-items: flex-start;
  margin: var(--pt-pad) var(--pt-pad) 0;
}

.page-note__text {
  flex: 1 1 auto;
}

.page-note .el-button {
  flex: 0 0 auto;
}

.page-note__x {
  flex-shrink: 0;
  padding: 0;
  color: var(--pt-t4);
  cursor: pointer;
  background: none;
  border: 0;
}

.page-note__x:hover {
  color: var(--pt-t2);
}

/* 画板 bar-64 的搜索框：220×28，hover 底 + 1px 边 + r=4 */
.bar-search {
  flex: 0 0 auto;
  width: 220px;
}

.bar-search :deep(.el-input__wrapper) {
  height: 28px;
  min-height: 28px;
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

.bar-search :deep(.el-input__inner) {
  font-size: var(--pt-fz-sm);
}

/* 带上的图标钮共享件没给 disabled 态，刷新中要看得出点不动 */
.pt-band__iconbtn:disabled {
  color: var(--pt-t4);
  cursor: default;
}

/* gfoot 右侧的读数提醒：窄屏时让它换行而不是把左侧计数挤没 */
.foot-caveat {
  min-width: 0;
  text-align: right;
}

@media (max-width: 768px) {
  /* 搜索框在手机上占满工具栏那一行，220 固定宽会顶出去 */
  .bar-search {
    flex: 1 1 100%;
    width: auto;
  }

  /* 34 高的页脚带放不下两段文字，允许长高并左对齐 */
  .pt-band--foot {
    flex-wrap: wrap;
    padding: 6px var(--pt-space-3);
  }

  .foot-caveat {
    text-align: left;
  }
}

.site {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
  min-width: 0;
}

/* 站点名在定义文件里是小写 id，首字母大写才像个名字 */
.site__name {
  overflow: hidden;
  text-overflow: ellipsis;
  text-transform: capitalize;
  white-space: nowrap;
}

.rss {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  color: var(--pt-t1);
}

.rss.is-zero {
  color: var(--pt-t4);
}

/* 带 tooltip 的表头：虚线下划线提示「这里有解释」，比只放个图标更好点中 */
.th-help {
  display: inline-flex;
  gap: 3px;
  align-items: center;
  cursor: help;
  border-bottom: 1px dotted var(--pt-t4);
}

.ts {
  font-variant-numeric: tabular-nums;
}

.days {
  display: inline-flex;
  flex-direction: column;
  gap: 3px;
  align-items: flex-start;
}

.days-remaining-value {
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-t1);
  font-variant-numeric: tabular-nums;
}

.days-remaining--warn {
  color: var(--pt-warn);
}

.days-remaining--critical {
  font-weight: 700;
  color: var(--pt-dang);
}

.mode-sel {
  width: 88px;
}

/* el-tooltip 要一个能接事件的宿主，disabled 的开关和按钮自己不派发 mouseenter */
.sw {
  display: inline-flex;
}

.cand-search {
  margin-bottom: var(--pt-space-3);
}

.cands {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
}

.cand {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  padding: var(--pt-space-2) var(--pt-space-3);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
  transition: border-color var(--pt-transition-fast);
}

.cand:hover {
  border-color: var(--pt-p);
}

.cand__meta {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.cand__name {
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t1);
  text-transform: capitalize;
}

.cand__tags {
  display: flex;
  gap: 6px;
  align-items: center;
}

.cand__acts {
  display: flex;
  flex-shrink: 0;
  gap: var(--pt-space-2);
}

/* 底部提示要占满左侧，按钮靠右：pt-foot-note 自带 margin-right:auto */
.cand-hint {
  text-align: left;
}

.cand-hint a {
  color: var(--pt-p);
  text-decoration: none;
}

.cand-hint a:hover {
  text-decoration: underline;
}

/* 移动端行卡列表：表格带本身贴边，留白由这里给（同样是 16） */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-pad);
}

/* ---- 表格下方的分析卡 ---- */

/* p-auth 是通栏（画板 1080 宽），在自适应网格里横跨整行 */

/*
 * 卡内的小分布列表：标签 + 计数一行，下面一根 5 高的占比柱（画板令牌里进度条 = 5）。
 * 分母是表格当前的行数，所以柱长表达的是「这批站点里占多少」，不是绝对量。
 */
.mini {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 0;
  margin: 0;
  list-style: none;
}

/* 通栏那张卡宽度富余，认证方式横着铺 */
.mini--cols {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 10px var(--pt-pad);
}

.mini__row {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 4px var(--pt-space-2);
  align-items: center;
  min-width: 0;
}

.mini__k {
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mini__v {
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t1);
}

.mini__bar {
  grid-column: 1 / -1;
  height: 5px;
  overflow: hidden;
  background: var(--pt-hover);
  border-radius: 999px;
}

.mini__fill {
  display: block;
  height: 100%;
  background: var(--mini-c, var(--pt-p));
  border-radius: inherit;
}

.mini__bar.is-ok {
  --mini-c: var(--pt-ok);
}

.mini__bar.is-warn {
  --mini-c: var(--pt-warn);
}

.mini__bar.is-dang {
  --mini-c: var(--pt-dang);
}

.mini__bar.is-info {
  --mini-c: var(--pt-info);
}

.mini__bar.is-mute {
  --mini-c: var(--pt-t4);
}

/* 卡片脚注：说明口径，10/400 t4（画板脚注字号） */
.mini__foot {
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t4);
}

/* 列设置面板：一行一个勾选 */
.cols {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.cols__row {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  font-size: var(--pt-fz-sm);
  cursor: pointer;
}

/* 画板 chip-*：24 高、11px —— 与任务列表那排筛选 chip 同一套写法 */
.bar-chip__btn {
  height: 24px;
  padding: 0 10px;
  margin: 0;
  font-size: var(--pt-fz-label);
}

/* 画板 chip-0 / chip-1 是 72×24 的下拉 chip */
.bar-chip {
  flex: 0 0 auto;
  width: 132px;
}

.bar-chip :deep(.el-select__wrapper) {
  min-height: 24px;
  padding: 0 8px;
  font-size: var(--pt-fz-label);
}

/* 与表格里的 .site__name 一致：定义文件里是小写 id，首字母大写才像个名字 */
.card-name {
  text-transform: capitalize;
}

/*
 * 「更多」是 el-dropdown 包了一层按钮，所以容器要自己声明高度。
 *
 * 不再撑满一行：卡上现在只剩这一个按钮（主操作是整卡可点），撑满会让它看着像
 * 这张卡的主操作，而它装的全是低频动作。靠右放一个 44 高的小按钮就够了。
 */
.pt-rowcard__actions .card-more {
  display: flex;
  flex: 0 0 auto;
  min-height: var(--pt-m-touch);
  margin-left: auto;
}

.card-more :deep(.el-button) {
  width: 100%;
  min-height: var(--pt-m-touch);
}

/* 下拉项的图标与文字间距。菜单被 teleport 到 body，但 scoped 是属性选择器，照样生效 */
.dd-ico {
  margin-right: 6px;
}
</style>
