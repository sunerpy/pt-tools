<script setup lang="ts">
import {
  type AggregatedStatsResponse,
  type SiteConfig,
  type SiteLoginState,
  sitesApi,
  tasksApi,
  type TaskStatsResponse,
  userInfoApi,
} from "@/api";
import LevelTooltip from "@/components/LevelTooltip.vue";
import PtIcon from "@/components/PtIcon";
import SiteAvatar from "@/components/SiteAvatar.vue";
import PtBreakdown, { type BreakdownRow } from "@/components/ui/PtBreakdown.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtKpiBar from "@/components/ui/PtKpiBar.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { type ReminderTier, useLoginState } from "@/composables/useLoginState";
import { useSiteLevelsStore } from "@/stores/siteLevels";
import {
  formatBytes,
  formatDate,
  formatJoinDuration,
  formatNumber,
  formatRatio,
  formatTime,
  formatTimeAgo,
  getRatioType,
  getSiteBonusName,
  getSiteSeedingBonusName,
} from "@/utils/format";
import { ElMessage } from "element-plus";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { computed, onMounted, onUnmounted, ref } from "vue";

const siteLevelsStore = useSiteLevelsStore();

/**
 * 六态状态机（设计文档 §5）。这一页没有筛选，所以 0 行只会是 empty / error / perm，
 * 不会出现 zero；partial 留给「聚合成功但有站点没同步上」，由 failedSites 决定。
 */
const failedSites = ref(0);
const { loading, state, errorText, run, hasPartialBanner } = useDataState({
  failed: () => failedSites.value,
});
const syncing = ref(false);
const syncingSite = ref<string | null>(null);
const aggregatedStats = ref<AggregatedStatsResponse | null>(null);
/** 全库任务计数，画板 KPI 六格里有三格来自这里 */
const taskStats = ref<TaskStatsResponse | null>(null);
const sitesByName = ref<Record<string, SiteConfig>>({});
const loginStates = ref<Record<string, SiteLoginState>>({});
const isMobile = useIsMobile();
const { effectiveLastActive, daysRemaining, reminderTier, tierLabel } = useLoginState(loginStates);

/** 两条顶部说明各自可关，关掉后本次会话不再出现（不落盘：换页回来仍要提醒） */
const extHintOpen = ref(true);
const riskHintOpen = ref(true);

type PillTone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

/* getRatioType 返回的是 el-tag 的色名，胶囊用语义名，这里换一次 */
const RATIO_TONE: Record<ReturnType<typeof getRatioType>, PillTone> = {
  success: "ok",
  info: "info",
  warning: "warn",
  danger: "dang",
};

function ratioTone(ratio: number): PillTone {
  return RATIO_TONE[getRatioType(ratio)];
}

/*
 * 封禁提醒档位 → 胶囊语义色。useLoginState.tierTagType 给的是 el-tag 的 type，
 * 其中 none 档返回空串（Element 的默认灰），但「正常」应该是绿的，所以这里另立一张表。
 */
const TIER_TONE: Record<ReminderTier, PillTone> = {
  none: "ok",
  "pre-warn": "primary",
  "30d": "primary",
  "14d": "warn",
  "7d": "warn",
  "3d": "dang",
  "1d": "dang",
  "banned-imminent": "dang",
  unknown: "neutral",
};

function tierTone(site: string): PillTone {
  return TIER_TONE[reminderTier(site)];
}

// 定时刷新相关
const REFRESH_INTERVAL = 5 * 60 * 1000; // 5分钟
let refreshTimer: ReturnType<typeof setInterval> | null = null;
const autoRefreshEnabled = ref(true);

/**
 * KPI 条的数据项。结构与 PtKpiBar 的 KpiItem 一致（结构化匹配，不用导出类型）。
 * 聚合接口只给当前值、没有历史序列，所以这里不传 series —— 柱子宁缺勿造。
 */
interface KpiRow {
  label: string;
  value: string;
  icon: string;
  delta?: string;
  deltaTone?: "ok" | "warn" | "dang" | "info" | "primary" | "neutral";
  /** 画板每格右侧 48×22 的柱图。这里画的是按站点的构成，不是时间序列 */
  series?: number[];
}

/**
 * KPI 带 —— 画板 kpibar：**6 格一行、整条 64 高**，格间 1px 竖线。
 *
 * 六格的口径照画板 10 写：站点、活跃任务、今日推送、免费种子、总上传、平均分享率。
 * 前三格来自站点统计接口，「活跃任务 / 今日推送 / 免费种子」来自 `/api/tasks/stats`
 * —— 那三个是任务口径，站点统计接口里没有，为此加了那一个只回四个整数的接口。
 *
 * 柱图（画板每格右侧 48×22）画的是**按站点的构成**，不是时间序列：
 * 聚合接口不存历史，硬造一条趋势线就是编数据。所以只有「按站点能分解」的格子有柱子。
 */
const kpiItems = computed<KpiRow[]>(() => {
  const stats = aggregatedStats.value;
  if (!stats) return [];

  const perSite = stats.perSiteStats ?? [];
  const uploadSeries = perSite
    .map((r) => r.uploaded)
    .sort((a, b) => b - a)
    .slice(0, 8);
  const ratioSeries = perSite
    .map((r) => r.ratio)
    .filter((n) => n > 0)
    .sort((a, b) => b - a)
    .slice(0, 8);
  const tasks = taskStats.value;

  const items: KpiRow[] = [
    { label: "站点数量", value: stats.siteCount.toString(), icon: "globe" },
    {
      label: "活跃任务",
      value: tasks ? tasks.active.toString() : "—",
      icon: "list-checks",
    },
    {
      label: "今日推送",
      value: tasks ? tasks.pushedToday.toString() : "—",
      icon: "send",
    },
    {
      label: "免费种子",
      value: tasks ? tasks.free.toString() : "—",
      icon: "zap",
    },
    {
      label: "总上传量",
      value: formatBytes(stats.totalUploaded),
      icon: "upload",
      series: uploadSeries,
    },
    {
      label: "平均分享率",
      value: formatRatio(stats.averageRatio),
      icon: "gauge",
      series: ratioSeries,
    },
  ];

  // 分享率低于 1 是要动手的信号，挂一枚警示胶囊；健康时不占位
  if (stats.averageRatio < 1) {
    const ratio = items.find((i) => i.label === "平均分享率");
    if (ratio) {
      ratio.delta = "偏低";
      ratio.deltaTone = "warn";
    }
  }

  return items;
});

const siteRows = computed(() => aggregatedStats.value?.perSiteStats ?? []);

/**
 * 画板 10 在 gfoot 之后还有三张分析卡：p-up 548（上传构成）、p-dist 516（等级分布）、
 * p-watch 1080（需要关注的站点）。三张卡的数据全部来自已经拿到的 perSiteStats，
 * 不额外请求接口 —— 聚合接口没有历史序列，所以这里画的是「当前的构成」而不是趋势。
 */

/** p-up：上传量最大的几个站点占了多少 */
const uploadRows = computed<BreakdownRow[]>(() => {
  const rows = [...siteRows.value].sort((a, b) => b.uploaded - a.uploaded).slice(0, 6);
  return rows.map((r) => ({
    key: r.site,
    label: r.site,
    value: formatBytes(r.uploaded),
    weight: r.uploaded,
    tone: "primary" as const,
    hint: `分享率 ${formatRatio(r.ratio)} · 做种 ${r.seeding}`,
  }));
});

const uploadTotal = computed(() => siteRows.value.reduce((n, r) => n + r.uploaded, 0));

/** p-dist：等级分布。等级名是站点各自的说法，按名字归组就是画板那张分布 */
const levelRows = computed<BreakdownRow[]>(() => {
  const buckets = new Map<string, number>();
  for (const r of siteRows.value) {
    const name = r.levelName || r.rank || "未知等级";
    buckets.set(name, (buckets.get(name) ?? 0) + 1);
  }
  return [...buckets.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, n]) => ({
      key: name,
      label: name,
      value: n,
      tone: "info" as const,
    }));
});

/**
 * p-watch：需要动手的站点。
 * 判定口径写在 hint 里，不让用户猜为什么这一条被列出来。
 */
const watchRows = computed<BreakdownRow[]>(() => {
  const out: BreakdownRow[] = [];
  for (const r of siteRows.value) {
    const reasons: string[] = [];
    if (r.ratio > 0 && r.ratio < 1) reasons.push(`分享率 ${formatRatio(r.ratio)} 低于 1`);
    if ((r.unreadMessageCount ?? 0) > 0) reasons.push(`${r.unreadMessageCount} 条未读站内信`);
    if (r.seeding === 0) reasons.push("一个种都没做");
    if (reasons.length === 0) continue;
    out.push({
      key: r.site,
      label: r.site,
      value: reasons.length,
      weight: reasons.length,
      tone: r.ratio > 0 && r.ratio < 1 ? "dang" : "warn",
      hint: reasons.join(" · "),
    });
  }
  return out.sort((a, b) => Number(b.value) - Number(a.value));
});

/*
 * 本页没有摘要行 —— 画板 10 的主区顶上是 KPI 带而不是 head（App.vue 的 KPI_TOP_ROUTES），
 * 那 6 格真实数字本身就是摘要。之前这里往 #pt-head-sub 打了一个 Teleport，
 * 而那个靶子在没有 head 的页面上根本不存在，摘要落了空。
 */

/** 状态块副标题：失败时给真实错误，空态时给下一步动作 */
const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  return "同步任意站点后这里会出现统计";
});

/**
 * 加载数据。
 *
 * 聚合统计是这一页的主数据，拿不到就没有任何东西可显示 —— 它失败走 error/perm。
 * 站点配置和登录态是装饰性的补充（用来给出站点链接和保号提醒），单独失败不该
 * 把整页判死，所以它们各自 catch 掉降级为空；但要计入 failedSites，
 * 页面会挂一条「部分数据没拿到」的提示，而不是假装一切正常。
 */
async function loadData() {
  failedSites.value = 0;
  const agg = await run(() => userInfoApi.getAggregated());
  if (!agg) {
    aggregatedStats.value = null;
    return;
  }
  aggregatedStats.value = agg;

  const [siteMap, states, stats] = await Promise.all([
    sitesApi.list().catch(() => {
      failedSites.value += 1;
      return {} as Record<string, SiteConfig>;
    }),
    sitesApi.listLoginStates().catch(() => {
      failedSites.value += 1;
      return [] as SiteLoginState[];
    }),
    /*
     * 任务计数（画板 KPI 的「活跃任务 / 今日推送 / 免费种子」三格）。
     * 拿不到就让那三格显示 —— 站点统计本身还在，不该因为任务计数失败整页降级。
     */
    tasksApi.stats().catch(() => {
      failedSites.value += 1;
      return null;
    }),
  ]);
  taskStats.value = stats;

  sitesByName.value = siteMap;

  const byName: Record<string, SiteLoginState> = {};
  for (const st of states ?? []) byName[st.site_name] = st;
  loginStates.value = byName;
}

function openSite(site: string) {
  const url =
    sitesByName.value[site]?.web_url ??
    sitesByName.value[site]?.urls?.[0] ??
    loginStates.value[site]?.base_url;
  if (url) window.open(url, "_blank", "noopener");
}

function openAllSites() {
  const rows = aggregatedStats.value?.perSiteStats ?? [];
  const urls = rows
    .map(
      (row) =>
        sitesByName.value[row.site]?.web_url ??
        sitesByName.value[row.site]?.urls?.[0] ??
        loginStates.value[row.site]?.base_url,
    )
    .filter((url): url is string => Boolean(url));

  if (urls.length === 0) {
    ElMessage.info("没有可打开的站点");
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

// 同步所有站点
async function syncAll() {
  syncing.value = true;
  try {
    const result = await userInfoApi.syncAll();
    if (result.failed && result.failed.length > 0) {
      ElMessage.warning(`同步完成: ${result.success.length} 成功, ${result.failed.length} 失败`);
    } else {
      ElMessage.success(`同步完成: ${result.success.length} 个站点`);
    }
    await loadData();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "同步失败");
  } finally {
    syncing.value = false;
  }
}

// 同步单个站点
async function syncSite(siteId: string) {
  syncingSite.value = siteId;
  try {
    await userInfoApi.syncSite(siteId);
    ElMessage.success(`站点 ${siteId} 同步成功`);
    await loadData();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "同步失败");
  } finally {
    syncingSite.value = null;
  }
}

// 清除缓存
async function clearCache() {
  try {
    await userInfoApi.clearCache();
    ElMessage.success("缓存已清除");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "清除缓存失败");
  }
}

// 启动定时刷新
function startAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer);
  }
  refreshTimer = setInterval(() => {
    if (!loading.value && !syncing.value) {
      loadData();
    }
  }, REFRESH_INTERVAL);
}

// 停止定时刷新
function stopAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
}

// 切换自动刷新
function toggleAutoRefresh() {
  autoRefreshEnabled.value = !autoRefreshEnabled.value;
  if (autoRefreshEnabled.value) {
    startAutoRefresh();
    ElMessage.success("已开启自动刷新");
  } else {
    stopAutoRefresh();
    ElMessage.info("已关闭自动刷新");
  }
}

// 检查是否有 H&R 数据
function hasHnR(row: any): boolean {
  return (
    (row.hnrUnsatisfied && row.hnrUnsatisfied > 0) || (row.hnrPreWarning && row.hnrPreWarning > 0)
  );
}

onMounted(() => {
  loadData();
  // 预加载所有站点的等级信息
  siteLevelsStore.loadAll();
  if (autoRefreshEnabled.value) {
    startAutoRefresh();
  }
});

onUnmounted(() => {
  stopAutoRefresh();
});
</script>

<template>
  <div class="dash">
    <!--
      KPI 条 —— 画板 kpibar 328,0 1112×64：主区最顶上的一条全宽白带，
      6 格之间是 1px 竖线。它替代页头出现在总览页，所以不再包在卡片里。
    -->
    <PtKpiBar v-if="kpiItems.length" :items="kpiItems" :cols="6" band />

    <!--
      提示放在顶部带**之后** —— 画板 27（系统设置）就是这个落法：head 0..64，
      提示在 y=80、左右内缩 16。放在带之前会把 KPI 带从主区顶上顶下来，
      而画板里 KPI 带是贴着 y=0 的。
    -->
    <!--
      partial（设计文档 §5）：聚合统计拿到了，但站点配置或登录态没拿到。
      既不该整页报错，也不该假装正常 —— 统计照常显示，这里说清少了什么，
      免得用户以为「打开站点」按钮和保号提醒坏了。
    -->
    <div v-if="hasPartialBanner(siteRows.length)" class="pt-note pt-note--warn dash__note">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>
        部分数据没拿到（{{ failedSites }} 项）：站点链接与保号提醒可能不完整，统计数字不受影响。
      </span>
      <button type="button" class="dash__note-x" aria-label="重新加载" @click="loadData">
        <PtIcon name="refresh-cw" :size="14" />
      </button>
    </div>

    <div v-if="extHintOpen" class="pt-note dash__note">
      <PtIcon name="info" :size="14" class="pt-note__icon" />
      <span>
        推荐安装
        <a href="https://github.com/sunerpy/pt-tools/releases" target="_blank" rel="noopener">
          PT Tools Helper 浏览器扩展
        </a>
        ：自动同步 Cookie、一键采集新站点数据。
        <a
          href="https://github.com/sunerpy/pt-tools/blob/main/docs/guide/request-new-site.md"
          target="_blank"
          rel="noopener">
          了解更多
        </a>
      </span>
      <button type="button" class="dash__note-x" aria-label="关闭提示" @click="extHintOpen = false">
        <PtIcon name="x" :size="14" />
      </button>
    </div>

    <div v-if="riskHintOpen" class="pt-note pt-note--warn dash__note">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>
        活跃时间通过 cookie/API 探测获取，可刷新多数站点的 last_access（最近动向）以保号；
        但少数站点按 last_login（实际登录）或做种活跃度清理，此类站点仍需定期手动登录，
        请勿仅依赖此处数据。
      </span>
      <button
        type="button"
        class="dash__note-x"
        aria-label="关闭提示"
        @click="riskHintOpen = false">
        <PtIcon name="x" :size="14" />
      </button>
    </div>

    <!-- 站点详情表格：画板 grid 是全宽平铺的带，不是圆角描边卡片 -->
    <PtToolbar band>
      <template #right>
        <el-tooltip
          :content="autoRefreshEnabled ? '点击关闭自动刷新 (5分钟)' : '点击开启自动刷新'"
          placement="top">
          <el-button
            size="small"
            :type="autoRefreshEnabled ? 'success' : 'info'"
            @click="toggleAutoRefresh">
            <PtIcon name="timer" :size="14" />
            <span>{{ autoRefreshEnabled ? "自动刷新中" : "自动刷新已关闭" }}</span>
          </el-button>
        </el-tooltip>
        <el-button size="small" @click="clearCache">
          <PtIcon name="trash-2" :size="14" />
          <span>清除缓存</span>
        </el-button>
        <el-button
          size="small"
          data-testid="userinfo-open-all-btn"
          :disabled="!siteRows.length"
          @click="openAllSites">
          <PtIcon name="external-link" :size="14" />
          <span>一键打开站点</span>
        </el-button>
        <el-button size="small" type="info" @click="$router.push('/userinfo/export')">
          <PtIcon name="share-2" :size="14" />
          <span>导出分享</span>
        </el-button>
        <el-button size="small" @click="$router.push('/supported-sites')">
          <PtIcon name="list" :size="14" />
          <span>已支持站点</span>
        </el-button>
        <el-button type="primary" size="small" :loading="syncing" @click="syncAll">
          <PtIcon v-if="!syncing" name="refresh-cw" :size="14" />
          <span>同步全部</span>
        </el-button>
      </template>
    </PtToolbar>

    <div v-loading="loading" class="pt-band--grid">
      <!-- 桌面端表格视图 -->
      <!-- roomy：站点列有 32px 头像 + 未读角标，数据量/魔力列是双行，34px 装不下 -->
      <el-table
        v-if="!isMobile"
        class="pt-grid pt-grid--roomy"
        :data="siteRows"
        style="width: 100%"
        :default-sort="{ prop: 'uploaded', order: 'descending' }"
        highlight-current-row>
        <!-- 站点列：带消息徽章和悬停效果 -->
        <el-table-column
          prop="site"
          label="站点"
          min-width="160"
          sortable
          fixed="left"
          class-name="pt-cell-overflow">
          <template #default="{ row }">
            <div class="site">
              <el-badge
                :value="row.unreadMessageCount"
                :hidden="!row.unreadMessageCount || row.unreadMessageCount === 0"
                :max="99"
                type="danger">
                <button
                  type="button"
                  class="site__av"
                  :aria-label="`同步 ${row.site}`"
                  @click.stop="syncSite(row.site)">
                  <SiteAvatar :site-name="row.site" :site-id="row.site" :size="32" />
                  <PtIcon
                    v-if="syncingSite === row.site"
                    name="loader-circle"
                    :size="14"
                    class="site__spin" />
                </button>
              </el-badge>
              <div class="site__txt">
                <span class="site__name">{{ row.site }}</span>
                <span class="site__user">{{ row.username }}</span>
              </div>
            </div>
          </template>
        </el-table-column>

        <!-- 等级列 -->
        <el-table-column prop="rank" label="等级" min-width="100" align="center">
          <template #default="{ row }">
            <LevelTooltip
              :site-id="row.site"
              :current-level-name="row.levelName || row.rank || '-'"
              :current-level-id="row.levelId" />
          </template>
        </el-table-column>

        <!-- 上传/下载量：双行布局带图标 -->
        <el-table-column
          prop="uploaded"
          label="数据量"
          min-width="140"
          sortable
          align="right"
          class-name="pt-cell-num">
          <template #default="{ row }">
            <div class="io">
              <span class="io__r is-up">
                <PtIcon name="upload" :size="12" />{{ formatBytes(row.uploaded) }}
              </span>
              <span class="io__r is-dn">
                <PtIcon name="download" :size="12" />{{ formatBytes(row.downloaded) }}
              </span>
            </div>
          </template>
        </el-table-column>

        <!-- 真实数据（如果不同） -->
        <el-table-column
          prop="trueUploaded"
          label="真实数据"
          min-width="140"
          sortable
          align="right"
          class-name="pt-cell-num">
          <template #default="{ row }">
            <div v-if="row.trueUploaded && row.trueUploaded !== row.uploaded" class="io">
              <span class="io__r is-up">
                <PtIcon name="upload" :size="12" />{{ formatBytes(row.trueUploaded) }}
              </span>
              <span class="io__r is-dn">
                <PtIcon name="download" :size="12" />{{ formatBytes(row.trueDownloaded ?? 0) }}
              </span>
            </div>
            <span v-else class="nil">-</span>
          </template>
        </el-table-column>

        <!-- 分享率 -->
        <el-table-column prop="ratio" label="分享率" min-width="90" sortable align="center">
          <template #default="{ row }">
            <PtStatusPill :tone="ratioTone(row.ratio)" size="sm">
              {{ formatRatio(row.ratio) }}
            </PtStatusPill>
          </template>
        </el-table-column>

        <!-- 做种数 + H&R -->
        <el-table-column prop="seeding" label="做种" min-width="110" sortable align="center">
          <template #default="{ row }">
            <div class="seed">
              <PtStatusPill tone="ok" size="sm">{{ row.seeding }}</PtStatusPill>
              <div v-if="hasHnR(row)" class="hnr">
                <el-tooltip
                  v-if="row.hnrPreWarning > 0"
                  :content="`H&R 预警: ${row.hnrPreWarning}`">
                  <span class="hnr__i is-warn">
                    <PtIcon name="triangle-alert" :size="12" />{{ row.hnrPreWarning }}
                  </span>
                </el-tooltip>
                <el-tooltip
                  v-if="row.hnrUnsatisfied > 0"
                  :content="`H&R 未满足: ${row.hnrUnsatisfied}`">
                  <span class="hnr__i is-dang">
                    <PtIcon name="circle-x" :size="12" />{{ row.hnrUnsatisfied }}
                  </span>
                </el-tooltip>
              </div>
            </div>
          </template>
        </el-table-column>

        <!-- 做种体积 -->
        <el-table-column
          prop="seederSize"
          label="做种体积"
          min-width="110"
          sortable
          align="right"
          class-name="pt-cell-num">
          <template #default="{ row }">
            <span class="is-ok">{{ formatBytes(row.seederSize ?? 0) }}</span>
          </template>
        </el-table-column>

        <!-- 魔力值 + 做种积分 -->
        <el-table-column
          prop="bonus"
          label="积分"
          min-width="140"
          sortable
          align="right"
          class-name="pt-cell-num">
          <template #default="{ row }">
            <div class="bonus">
              <span class="bonus__r">
                <span class="bonus__v">{{ formatNumber(row.bonus ?? 0) }}</span>
                <span class="bonus__l">{{ getSiteBonusName(row.site) }}</span>
              </span>
              <span
                v-if="row.seedingBonus && row.seedingBonus > 0 && getSiteSeedingBonusName(row.site)"
                class="bonus__r is-seed">
                <span class="bonus__v">{{ formatNumber(row.seedingBonus) }}</span>
                <span class="bonus__l">{{ getSiteSeedingBonusName(row.site) }}</span>
              </span>
            </div>
          </template>
        </el-table-column>

        <!-- 时魔 -->
        <el-table-column
          prop="bonusPerHour"
          label="时魔/h"
          min-width="100"
          sortable
          align="right"
          class-name="pt-cell-num">
          <template #default="{ row }">
            <span class="is-warn">{{ formatNumber(row.bonusPerHour ?? 0) }}</span>
          </template>
        </el-table-column>

        <!-- 注册时间 -->
        <el-table-column
          prop="joinDate"
          label="入站"
          min-width="110"
          sortable
          align="center"
          class-name="pt-cell-muted">
          <template #default="{ row }">
            <el-tooltip v-if="row.joinDate" :content="formatDate(row.joinDate)" placement="top">
              <span class="ts">{{ formatJoinDuration(row.joinDate) }}</span>
            </el-tooltip>
            <span v-else class="nil">-</span>
          </template>
        </el-table-column>

        <!-- 判定活跃 -->
        <el-table-column label="判定活跃" min-width="110" align="center" class-name="pt-cell-muted">
          <template #default="{ row }">
            <span class="ts">{{ formatTimeAgo(effectiveLastActive(row.site)) }}</span>
          </template>
        </el-table-column>

        <!-- 封禁提醒 -->
        <el-table-column min-width="120" align="center">
          <template #header>
            <el-tooltip content="距离站点封禁阈值的剩余天数；负数表示已超过阈值" placement="top">
              <span class="th-help">剩余天数</span>
            </el-tooltip>
          </template>
          <template #default="{ row }">
            <div class="days">
              <span class="days__v">
                {{ daysRemaining(row.site) === null ? "—" : `${daysRemaining(row.site)} 天` }}
              </span>
              <PtStatusPill :tone="tierTone(row.site)" size="sm">
                {{ tierLabel(reminderTier(row.site)) }}
              </PtStatusPill>
            </div>
          </template>
        </el-table-column>

        <!-- 更新时间 -->
        <el-table-column
          prop="lastUpdate"
          label="更新"
          min-width="100"
          sortable
          align="center"
          class-name="pt-cell-muted">
          <template #default="{ row }">
            <el-tooltip :content="formatTime(row.lastUpdate)" placement="top">
              <span class="ts">{{ formatTimeAgo(row.lastUpdate) }}</span>
            </el-tooltip>
          </template>
        </el-table-column>

        <!-- 操作列 -->
        <el-table-column
          label="操作"
          width="150"
          align="center"
          fixed="right"
          class-name="pt-cell-act">
          <template #default="{ row }">
            <div class="acts">
              <el-tooltip
                content="未配置站点地址"
                placement="top"
                :disabled="!!(sitesByName[row.site]?.urls?.[0] || loginStates[row.site]?.base_url)">
                <span>
                  <el-button
                    link
                    type="primary"
                    :disabled="
                      !sitesByName[row.site]?.urls?.[0] && !loginStates[row.site]?.base_url
                    "
                    :data-testid="`userinfo-open-site-${row.site}`"
                    @click="openSite(row.site)">
                    <PtIcon name="external-link" :size="14" /><span>打开</span>
                  </el-button>
                </span>
              </el-tooltip>
              <el-button
                link
                type="primary"
                :loading="syncingSite === row.site"
                @click="syncSite(row.site)">
                <PtIcon v-if="syncingSite !== row.site" name="refresh-cw" :size="14" />
                <span>同步</span>
              </el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty>
          <PtDataState :state="state" :sub="stateSub">
            <template v-if="state === 'error'" #action>
              <el-button size="small" @click="loadData">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
            <template v-else-if="state === 'empty'" #action>
              <el-button type="primary" size="small" :loading="syncing" @click="syncAll">
                <PtIcon v-if="!syncing" name="refresh-cw" :size="14" />
                <span>同步全部</span>
              </el-button>
            </template>
          </PtDataState>
        </template>
      </el-table>

      <!--
        移动端卡片视图：13 列的表格在手机上横向滚动没法用，所以 <768px 换成一站一卡。
        显隐由 v-if 控制而不是 CSS，两套视图不会同时挂在 DOM 上。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!siteRows.length" :state="state" :sub="stateSub">
          <template v-if="state === 'error'" #action>
            <el-button size="small" @click="loadData">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
          <template v-else-if="state === 'empty'" #action>
            <el-button type="primary" size="small" :loading="syncing" @click="syncAll">
              <PtIcon v-if="!syncing" name="refresh-cw" :size="14" />
              <span>同步全部</span>
            </el-button>
          </template>
        </PtDataState>

        <article v-for="row in siteRows" :key="row.site" class="card">
          <header class="card__head">
            <div class="site">
              <el-badge
                :value="row.unreadMessageCount"
                :hidden="!row.unreadMessageCount || row.unreadMessageCount === 0"
                :max="99"
                type="danger">
                <button
                  type="button"
                  class="site__av"
                  :aria-label="`同步 ${row.site}`"
                  @click.stop="syncSite(row.site)">
                  <SiteAvatar :site-name="row.site" :site-id="row.site" :size="40" />
                  <PtIcon
                    v-if="syncingSite === row.site"
                    name="loader-circle"
                    :size="16"
                    class="site__spin" />
                </button>
              </el-badge>
              <div class="site__txt">
                <span class="site__name">{{ row.site }}</span>
                <span class="site__user">{{ row.username }}</span>
              </div>
            </div>
            <LevelTooltip
              :site-id="row.site"
              :current-level-name="row.levelName || row.rank || '-'"
              :current-level-id="row.levelId" />
          </header>

          <div class="card__io">
            <div class="io">
              <span class="io__r is-up">
                <PtIcon name="upload" :size="13" />{{ formatBytes(row.uploaded) }}
              </span>
              <span class="io__r is-dn">
                <PtIcon name="download" :size="13" />{{ formatBytes(row.downloaded) }}
              </span>
            </div>
            <PtStatusPill :tone="ratioTone(row.ratio)">{{ formatRatio(row.ratio) }}</PtStatusPill>
          </div>

          <div class="card__stats">
            <div class="stat">
              <span class="stat__l">
                <PtIcon name="star" :size="11" />{{ getSiteBonusName(row.site) }}
              </span>
              <span class="stat__v is-warn">{{ formatNumber(row.bonus ?? 0) }}</span>
            </div>
            <div class="stat">
              <span class="stat__l"><PtIcon name="timer" :size="11" />时魔/h</span>
              <span class="stat__v is-warn">{{ formatNumber(row.bonusPerHour ?? 0) }}</span>
            </div>
            <div class="stat">
              <span class="stat__l"><PtIcon name="share-2" :size="11" />做种</span>
              <span class="stat__v">
                {{ row.seeding }}
                <PtIcon
                  v-if="(row.hnrUnsatisfied ?? 0) > 0"
                  name="circle-x"
                  :size="12"
                  class="is-dang" />
              </span>
            </div>
            <div class="stat">
              <span class="stat__l"><PtIcon name="hard-drive" :size="11" />体积</span>
              <span class="stat__v is-ok">{{ formatBytes(row.seederSize ?? 0) }}</span>
            </div>
            <div
              v-if="row.seedingBonus && row.seedingBonus > 0 && getSiteSeedingBonusName(row.site)"
              class="stat">
              <span class="stat__l"><PtIcon name="medal" :size="11" />做种积分</span>
              <span class="stat__v is-ok">{{ formatNumber(row.seedingBonus) }}</span>
            </div>
            <div class="stat">
              <span class="stat__l"><PtIcon name="calendar" :size="11" />入站</span>
              <span class="stat__v">{{ formatJoinDuration(row.joinDate ?? 0) }}</span>
            </div>
            <div class="stat">
              <span class="stat__l"><PtIcon name="clock" :size="11" />判定活跃</span>
              <span class="stat__v">{{ formatTimeAgo(effectiveLastActive(row.site)) }}</span>
            </div>
            <div class="stat">
              <span class="stat__l"><PtIcon name="triangle-alert" :size="11" />剩余天数</span>
              <span class="stat__v">
                {{ daysRemaining(row.site) === null ? "—" : `${daysRemaining(row.site)} 天` }}
              </span>
              <PtStatusPill :tone="tierTone(row.site)" size="sm">
                {{ tierLabel(reminderTier(row.site)) }}
              </PtStatusPill>
            </div>
          </div>

          <footer class="card__foot">
            <span class="ts card__ts">
              <PtIcon name="clock" :size="12" />{{ formatTimeAgo(row.lastUpdate) }}
            </span>
            <div class="acts">
              <el-tooltip
                content="未配置站点地址"
                placement="top"
                :disabled="!!(sitesByName[row.site]?.urls?.[0] || loginStates[row.site]?.base_url)">
                <span>
                  <el-button
                    size="small"
                    :disabled="
                      !sitesByName[row.site]?.urls?.[0] && !loginStates[row.site]?.base_url
                    "
                    :data-testid="`userinfo-open-site-${row.site}`"
                    @click="openSite(row.site)">
                    <PtIcon name="external-link" :size="14" /><span>打开站点</span>
                  </el-button>
                </span>
              </el-tooltip>
              <el-button
                type="primary"
                size="small"
                :loading="syncingSite === row.site"
                @click="syncSite(row.site)">
                <PtIcon v-if="syncingSite !== row.site" name="refresh-cw" :size="14" />
                <span>同步</span>
              </el-button>
            </div>
          </footer>
        </article>
      </div>
    </div>

    <!-- 画板 gfoot 34：表格下方的说明带，左侧是统计口径与更新时间 -->
    <div v-if="aggregatedStats" class="pt-band--foot">
      <span>{{ siteRows.length }} 个站点</span>
      <span class="pt-band__spacer" />
      <span>最后更新 {{ formatTime(aggregatedStats.lastUpdate) }}</span>
    </div>

    <!--
      画板 10 的分析卡：p-up 548 / p-dist 516 两栏 + p-watch 1080 通栏。
      三张都由 perSiteStats 现算，没有额外请求。
    -->
    <div v-if="siteRows.length > 0" class="pt-cards pt-cards--2">
      <PtPanel title="上传构成" icon="upload" :count="formatBytes(uploadTotal)">
        <PtBreakdown
          :rows="uploadRows"
          :total="uploadTotal"
          foot="柱长是该站点上传量占全部站点上传量的比例，只列前 6 个。" />
      </PtPanel>

      <PtPanel title="等级分布" icon="award" :count="`${levelRows.length} 种`">
        <PtBreakdown
          :rows="levelRows"
          :total="siteRows.length"
          foot="等级名沿用各站自己的叫法，同名的归成一组。" />
      </PtPanel>

      <PtPanel
        class="pt-cards__full"
        title="需要关注的站点"
        icon="bell-ring"
        :count="watchRows.length > 0 ? `${watchRows.length} 个` : '暂无'">
        <PtBreakdown
          v-if="watchRows.length > 0"
          :rows="watchRows"
          cols
          foot="判定口径：分享率低于 1、有未读站内信、或者一个种都没做。" />
        <p v-else class="dash__ok">所有站点的分享率都在 1 以上、没有未读站内信、也都在做种。</p>
      </PtPanel>
    </div>
  </div>
</template>

<style scoped>
/* 需要关注的站点一个都没有时的正面结论，不摆一张空卡 */
.dash__ok {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

/*
 * 这一页有两套视图：≥768px 走 el-table（皮肤在全局 .pt-grid），<768px 走卡片。
 * 两边刻意复用同一批单元格类名（.site / .io / .bonus / .acts …），
 * 改一处颜色两边一起变，不用维护两份语义色。
 */
/*
 * 画板主区是一串全宽横向带（KPI 条 → 工具栏 → 表格 → 页脚带），带与带之间没有间距，
 * 所以这里不再给 gap；需要内缩的东西（提示、移动端卡片）自己带 16 的留白。
 */
.dash {
  display: flex;
  flex-direction: column;
}

/* 提示不是全宽带：画板里它们在卡片层，左右各内缩 16 */
.dash__note {
  margin: var(--pt-pad) var(--pt-pad) 0;
}

/* 两条可关提示：.pt-note 默认居中对齐，这里是多行文案，图标要贴顶 */
.dash__note {
  align-items: flex-start;
}

.dash__note a {
  font-weight: 600;
  color: var(--pt-p);
  text-decoration: none;
}

.dash__note a:hover {
  text-decoration: underline;
}

.dash__note-x {
  flex-shrink: 0;
  padding: 0;
  margin-left: auto;
  color: var(--pt-t4);
  cursor: pointer;
  background: none;
  border: 0;
}

.dash__note-x:hover {
  color: var(--pt-t2);
}

/* ---- 站点单元格（表格 + 卡片头共用） ---- */
.site {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  min-width: 0;
}

/*
 * 未读消息角标压在头像右上角内侧。
 *
 * Element 默认把角标整个甩到头像外（right 偏移再 translateX(100%)），两位数就有
 * 十几像素探到间距外面，直接盖住站点名；上沿也要多出 9px，在表格里被 .cell 裁掉。
 * 改成贴住头像右上角、向左生长（去掉 translateX，只留右偏移）：
 *   right -2  右沿只探出头像 2px，角标再宽也是往左长进头像里，
 *             永远吃不到和站点名之间那 8px 间距 —— 居中锚定会随位数左右扩，
 *             「99+」那种宽度足以横跨整个 32px 头像；
 *   top 4     上沿落在头像上方 4px，配合 .pt-grid--roomy 的 48px 行高，
 *             距单元格上边框还剩 4px，不会和行线糊在一起；
 *   16/11     Element 默认 18px 高、12px 字，在 32px 头像上显得抢戏，收小一档。
 */
.site :deep(.el-badge__content.is-fixed) {
  top: 4px;
  right: -2px;
  height: 16px;
  padding: 0 4px;
  font-size: var(--pt-fz-label);
  line-height: 16px;
  transform: translateY(-50%);
}

/* 头像本身就是「同步本站」的按钮，所以是真 button，键盘可达 */
.site__av {
  position: relative;
  display: block;
  padding: 0;
  cursor: pointer;
  background: none;
  border: 0;
  border-radius: var(--pt-radius-full);
}

.site__av:hover {
  opacity: 0.75;
}

.site__av:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

/* 同步中的转圈：不再借 el-icon.is-loading，自己转 */
.site__spin {
  position: absolute;
  top: 50%;
  left: 50%;
  color: var(--pt-p);
  transform: translate(-50%, -50%);
  animation: dash-spin 1s linear infinite;
}

@keyframes dash-spin {
  to {
    transform: translate(-50%, -50%) rotate(360deg);
  }
}

.site__txt {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.site__name {
  overflow: hidden;
  font-weight: 600;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.site__user {
  overflow: hidden;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ---- 上传/下载两行 ---- */
.io {
  display: flex;
  flex-direction: column;
  gap: 2px;
  align-items: flex-end;
}

.io__r {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  font-variant-numeric: tabular-nums;
}

.io__r.is-up {
  color: var(--pt-ok);
}

.io__r.is-dn {
  color: var(--pt-dang);
}

/* 缺值统一用最弱的文字色，避免和真实数字抢注意力 */
.nil {
  color: var(--pt-t4);
}

/* ---- 做种数 + H&R ---- */
.seed {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
  justify-content: flex-end;
}

.hnr {
  display: inline-flex;
  gap: 6px;
  align-items: center;
}

.hnr__i {
  display: inline-flex;
  gap: 2px;
  align-items: center;
  font-size: var(--pt-fz-label);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  cursor: help;
}

.is-ok {
  color: var(--pt-ok);
}

.is-warn {
  color: var(--pt-warn);
}

.is-dang {
  color: var(--pt-dang);
}

/* ---- 积分：值在上、单位名在下 ---- */
.bonus {
  display: flex;
  flex-direction: column;
  gap: 2px;
  align-items: flex-end;
}

.bonus__r {
  display: inline-flex;
  gap: 4px;
  align-items: baseline;
}

.bonus__v {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--pt-warn);
}

.bonus__r.is-seed .bonus__v {
  color: var(--pt-ok);
}

.bonus__l {
  font-size: var(--pt-fz-foot);
  color: var(--pt-t4);
}

/* ---- 时间戳与表头提示 ---- */
.ts {
  font-variant-numeric: tabular-nums;
}

.th-help {
  display: inline-flex;
  gap: 3px;
  align-items: center;
  cursor: help;
  border-bottom: 1px dotted var(--pt-t4);
}

.days {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
}

.days__v {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t1);
}

.acts {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
}

/* ---- 移动端卡片 ---- */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-3);
  padding: var(--pt-pad);
}

.card {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-3);
  padding: var(--pt-space-3);
  background: var(--pt-raised);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

.card__head {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  justify-content: space-between;
}

/* 上传/下载 + 分享率一行：卡片里最想先看到的就是这三个数 */
.card__io {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  justify-content: space-between;
  padding-top: var(--pt-space-3);
  border-top: 1px solid var(--pt-border);
}

.card__io .io {
  align-items: flex-start;
  font-size: var(--pt-fz-sm);
}

.card__stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--pt-space-3) var(--pt-space-2);
}

.stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
  align-items: flex-start;
  min-width: 0;
}

.stat__l {
  display: inline-flex;
  gap: 3px;
  align-items: center;
  overflow: hidden;
  font-size: var(--pt-fz-foot);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

.stat__v {
  display: inline-flex;
  gap: 3px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t1);
}

.card__foot {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  justify-content: space-between;
  padding-top: var(--pt-space-3);
  border-top: 1px solid var(--pt-border);
}

.card__ts {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  font-size: var(--pt-fz-foot);
  color: var(--pt-t3);
}

/* 窄屏三列会把「做种积分」这类长标签挤成两行，退成两列 */
@media (max-width: 480px) {
  .card__stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
