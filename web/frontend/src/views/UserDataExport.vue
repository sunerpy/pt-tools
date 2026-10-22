<script setup lang="ts">
import {
  type AggregatedStatsResponse,
  type DeltaSummary,
  userInfoApi,
  type UserInfoRange,
} from "@/api";
import ExportPoster from "@/components/exportPoster/ExportPoster.vue";
import { buildPosterModel, type PosterLogoData } from "@/components/exportPoster/model";
import {
  ensurePosterFonts,
  loadPosterLogos,
  posterToBlob,
} from "@/components/exportPoster/posterImage";
import PosterStylePicker from "@/components/exportPoster/PosterStylePicker.vue";
import {
  DEFAULT_POSTER_STYLE,
  posterColorway,
  posterStyle,
  type PosterStyleId,
} from "@/components/exportPoster/styles";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { useResizeObserver, useStorage } from "@vueuse/core";
import { ElMessage } from "element-plus";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from "vue";
import { useRouter } from "vue-router";

const router = useRouter();
/* ≤768 时外壳隐藏页头，页头动作得原地落回页面（Teleport 的 disabled） */
const isMobile = useIsMobile();

/**
 * 六态（设计文档 §5）：以前拉取失败只弹 toast，页面随后画成「还没有可导出的统计」。
 * 这一页没有筛选，也只有一个数据源，所以只会出现 loading / empty / error / perm。
 */
const { loading, state, errorText, run, isStale } = useDataState();

/** 状态块副标题：失败给真实错误，空态给下一步动作 */
const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  if (state.value === "loading") return "正在读取统计数据";
  return "先到数据面板刷新一次各站点数据，再回来出图";
});
const exporting = ref(false);
const copying = ref(false);
const aggregatedStats = ref<AggregatedStatsResponse | null>(null);
/** 各站点的图标（data URL 与打码色块），键是小写的站点编号 */
const logos = shallowRef<Map<string, PosterLogoData>>(new Map());

const exportConfig = ref({
  title: "我的 PT 数据统计",
  showSiteDetails: true,
  selectedSites: [] as string[],
  blurUsernames: false,
  blurSiteNames: true,
  blurLogos: true,
  maxSitesToShow: 10,
  /* 周期增量（每日快照算出来的）：今日 / 本周 / 本月 */
  showIncrements: true,
  incrementRange: "7d" as UserInfoRange,
});

/*
 * 样式与配色记在浏览器里（每个样式记住自己上次用的配色），下次打开导出页还是这一套。
 * 打码开关不记：每次都从默认的「站点名与图标打码」开始，免得上次关掉的打码这次被忘了。
 */
const posterPrefs = useStorage<{ style: PosterStyleId; colorways: Record<string, string> }>(
  "pt-tools:export-poster",
  { style: DEFAULT_POSTER_STYLE, colorways: {} },
  undefined,
  { mergeDefaults: true },
);
const styleId = computed<PosterStyleId>({
  get: () => posterStyle(posterPrefs.value.style).id,
  set: (v) => (posterPrefs.value = { ...posterPrefs.value, style: v }),
});
const colorway = computed<string>({
  get: () =>
    posterColorway(posterStyle(styleId.value), posterPrefs.value.colorways[styleId.value]).id,
  set: (v) =>
    (posterPrefs.value = {
      ...posterPrefs.value,
      colorways: { ...posterPrefs.value.colorways, [styleId.value]: v },
    }),
});
const styleNote = computed(() => {
  const s = posterStyle(styleId.value);
  return `${s.name} · ${posterColorway(s, colorway.value).name}`;
});

const INCREMENT_RANGES: { label: string; value: UserInfoRange }[] = [
  { label: "今日", value: "today" },
  { label: "本周", value: "7d" },
  { label: "本月", value: "30d" },
];
const incrementLabel = computed(
  () => INCREMENT_RANGES.find((r) => r.value === exportConfig.value.incrementRange)?.label ?? "",
);
/** 所选周期的增量；拿不到或还没有可比快照时不往海报上放 */
const incrementSummary = ref<DeltaSummary | null>(null);
let incrementSeq = 0;
async function loadIncrements() {
  const seq = ++incrementSeq;
  const sum = await userInfoApi.getSummary(exportConfig.value.incrementRange).catch(() => null);
  if (seq === incrementSeq) incrementSummary.value = sum;
}
watch(
  () => exportConfig.value.incrementRange,
  () => void loadIncrements(),
);
const hasIncrements = computed(() =>
  (incrementSummary.value?.sites ?? []).some((d) => d.hasBaseline),
);

const allSites = computed(() => {
  if (!aggregatedStats.value) return [];
  return aggregatedStats.value.perSiteStats.map((s) => s.site);
});

const selectedSiteStats = computed(() => {
  if (!aggregatedStats.value) return [];
  const sites = exportConfig.value.selectedSites;
  if (sites.length === 0) {
    return aggregatedStats.value.perSiteStats.slice(0, exportConfig.value.maxSitesToShow);
  }
  return aggregatedStats.value.perSiteStats.filter((s) => sites.includes(s.site));
});

/* 打码开关摘要挂在区块条右端：三个开关折起来时也能一眼看出隐私档位 */
const maskNote = computed(() => {
  const on = [
    exportConfig.value.blurUsernames ? "用户名" : "",
    exportConfig.value.blurSiteNames ? "站点名" : "",
    exportConfig.value.blurLogos ? "图标" : "",
  ].filter(Boolean);
  return on.length === 0 ? "全部明文" : `已打码：${on.join(" / ")}`;
});

/** 海报上的「生成于」：每 30 秒走一次，导出前再取一次当前时间 */
const now = ref(new Date());
const clock = window.setInterval(() => (now.value = new Date()), 30_000);
onBeforeUnmount(() => window.clearInterval(clock));

const posterModel = computed(() => {
  const stats = aggregatedStats.value;
  if (!stats) return null;
  const cfg = exportConfig.value;
  return buildPosterModel({
    stats,
    sites: selectedSiteStats.value,
    showSites: cfg.showSiteDetails,
    increments: cfg.showIncrements && hasIncrements.value ? incrementSummary.value : null,
    incrementLabel: incrementLabel.value,
    title: cfg.title,
    mask: { names: cfg.blurSiteNames, users: cfg.blurUsernames, logos: cfg.blurLogos },
    logos: logos.value,
    now: now.value,
  });
});

/*
 * 预览就是导出的那张图：海报固定 640 宽，放不下时整体按比例缩小（transform 不改它自己的布局尺寸，
 * 导出时量的是节点自己的宽高，不受缩放影响）。
 */
const POSTER_WIDTH = 640;
const stage = ref<HTMLElement | null>(null);
const posterBox = ref<HTMLElement | null>(null);
const stageWidth = ref(POSTER_WIDTH);
const posterHeight = ref(0);
useResizeObserver(stage, (entries) => {
  const w = entries[0]?.contentRect.width;
  if (w) stageWidth.value = w;
});
useResizeObserver(posterBox, (entries) => {
  const h = entries[0]?.contentRect.height;
  if (h) posterHeight.value = h;
});
const fit = computed(() => Math.min(1, stageWidth.value / POSTER_WIDTH));
const fitBox = computed(() => ({
  width: `${Math.floor(POSTER_WIDTH * fit.value)}px`,
  height: posterHeight.value ? `${Math.ceil(posterHeight.value * fit.value)}px` : undefined,
}));
const fitScale = computed(() => (fit.value < 1 ? { transform: `scale(${fit.value})` } : {}));

const poster = ref<InstanceType<typeof ExportPoster> | null>(null);

async function loadData() {
  const pending = run(() => userInfoApi.getAggregated());
  const data = await pending;
  if (isStale(pending)) return;
  if (!data) {
    aggregatedStats.value = null;
    return;
  }
  aggregatedStats.value = data;
  void loadIncrements();
  if (exportConfig.value.selectedSites.length === 0) {
    exportConfig.value.selectedSites = allSites.value.slice(0, exportConfig.value.maxSitesToShow);
  }
  logos.value = await loadPosterLogos(allSites.value);
}

/** 把当前预览画成 PNG：先把「生成于」换成现在，等这一帧画好再取节点 */
async function renderPng(): Promise<Blob> {
  now.value = new Date();
  await nextTick();
  const el = poster.value?.posterEl();
  if (!el) throw new Error("海报还没画出来");
  return posterToBlob(el, poster.value!.fonts());
}

async function exportImage() {
  exporting.value = true;
  try {
    const blob = await renderPng();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.download = `pt-stats-${new Date().toISOString().split("T")[0]}.png`;
    link.href = url;
    link.click();
    // 下载是异步开始的：过一会儿再释放这个地址（只是回收内存，不是在等下载完成）
    window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
    ElMessage.success("图片已导出");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "导出失败");
  } finally {
    exporting.value = false;
  }
}

async function copyToClipboard() {
  // 检查是否为安全上下文（HTTPS 或 localhost）
  if (!window.isSecureContext) {
    ElMessage.warning("HTTP 环境不支持一键复制，请右键复制预览图或使用下载功能");
    return;
  }
  if (
    !navigator.clipboard ||
    typeof navigator.clipboard.write !== "function" ||
    typeof ClipboardItem === "undefined"
  ) {
    ElMessage.warning("浏览器不支持复制图片，请使用下载功能");
    return;
  }
  copying.value = true;
  try {
    /*
     * 图片要画一会儿：把「画好的图」以 Promise 交给 ClipboardItem、马上调用 write，
     * Safari 要求 write 在点击的同一轮里调用；不认 Promise 的浏览器再等图画好重来一次。
     */
    const pending = renderPng();
    pending.catch(() => {});
    try {
      await navigator.clipboard.write([new ClipboardItem({ "image/png": pending })]);
    } catch (e) {
      if (!(e instanceof TypeError)) throw e;
      await navigator.clipboard.write([new ClipboardItem({ "image/png": await pending })]);
    }
    ElMessage.success("图片已复制到剪贴板");
  } catch (e: unknown) {
    const error = e as Error;
    if (error.name === "NotAllowedError") {
      ElMessage.warning("请授予剪贴板访问权限");
    } else {
      ElMessage.error("复制失败，请右键复制预览图或使用下载功能");
      console.error("Clipboard error:", error);
    }
  } finally {
    copying.value = false;
  }
}

onMounted(() => {
  void ensurePosterFonts();
  loadData();
});
</script>

<template>
  <div class="export-page">
    <!--
      画板 11 是两栏卡片页：p-prev 700（预览）+ p-set 364（导出设置），
      head 之后直接进卡片层，没有工具栏带 —— 返回与两枚导出按钮在画板上是页头动作。
    -->
    <PtHeadSub>
      {{
        aggregatedStats
          ? `${aggregatedStats.siteCount} 个站点 · ${selectedSiteStats.length} 个入图`
          : "正在读取统计数据"
      }}
    </PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button @click="router.back()">
        <PtIcon name="arrow-left" :size="15" /><span>返回</span>
      </el-button>
      <el-button :loading="copying" :disabled="!aggregatedStats" @click="copyToClipboard">
        <PtIcon name="copy" :size="15" /><span>复制图片</span>
      </el-button>
      <el-button
        type="primary"
        :loading="exporting"
        :disabled="!aggregatedStats"
        @click="exportImage">
        <PtIcon name="download" :size="15" /><span>下载图片</span>
      </el-button>
    </Teleport>

    <div class="export-cols pt-cards pt-cards--main">
      <PtPanel
        v-loading="loading"
        title="预览"
        icon="eye"
        :count="exportConfig.showSiteDetails ? `${selectedSiteStats.length} 个站点入图` : '仅汇总'">
        <PtDataState
          v-if="!posterModel"
          :state="state"
          :title="state === 'empty' ? '还没有可导出的统计' : ''"
          :sub="stateSub">
          <template v-if="state === 'error'" #action>
            <el-button @click="loadData">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>

        <!-- 预览就是导出的那份 DOM：「下载图片」「复制图片」把这个节点画成 PNG -->
        <div v-else ref="stage" class="stage">
          <div class="stage__fit" :style="fitBox">
            <div ref="posterBox" class="stage__poster" :style="fitScale">
              <ExportPoster
                ref="poster"
                :model="posterModel"
                :style-id="styleId"
                :colorway="colorway" />
            </div>
          </div>
        </div>
      </PtPanel>

      <PtPanel class="export-side" title="导出设置" icon="settings" padding="none">
        <div class="pt-form settings-form">
          <div class="pt-strip">
            <PtIcon name="palette" :size="14" /><span>样式</span>
            <span class="pt-strip__end">{{ styleNote }}</span>
          </div>
          <div class="settings-body">
            <PosterStylePicker v-model:style-id="styleId" v-model:colorway="colorway" />
            <div class="field-tip">样式与配色记在这个浏览器里，下次打开还是这一套</div>
          </div>

          <div class="pt-strip"><PtIcon name="type" :size="14" /><span>标题</span></div>
          <div class="settings-body">
            <el-input v-model="exportConfig.title" placeholder="输入标题" />
            <div class="field-tip">印在图片最上方，导出后就固定在图里了</div>
          </div>

          <div class="pt-strip">
            <PtIcon name="shield" :size="14" /><span>内容与打码</span>
            <span class="pt-strip__end">{{ maskNote }}</span>
          </div>
          <div class="settings-body">
            <div class="toggles">
              <label class="tg">
                <el-switch v-model="exportConfig.showSiteDetails" size="small" />
                <span>附上逐站明细</span>
              </label>
              <label class="tg">
                <el-switch v-model="exportConfig.blurUsernames" size="small" />
                <span>打码用户名</span>
              </label>
              <label class="tg">
                <el-switch v-model="exportConfig.blurSiteNames" size="small" />
                <span>打码站点名</span>
              </label>
              <label class="tg">
                <el-switch v-model="exportConfig.blurLogos" size="small" />
                <span>打码站点图标</span>
              </label>
              <label class="tg">
                <el-switch
                  v-model="exportConfig.showIncrements"
                  size="small"
                  data-testid="export-increments" />
                <span>附上周期增量</span>
              </label>
              <el-select
                v-if="exportConfig.showIncrements"
                v-model="exportConfig.incrementRange"
                size="small"
                class="tg-range"
                data-testid="export-increment-range">
                <el-option
                  v-for="r in INCREMENT_RANGES"
                  :key="r.value"
                  :label="`周期: ${r.label}`"
                  :value="r.value" />
              </el-select>
            </div>
            <div class="field-tip">
              名字打码只留首字，后面换成遮盖条；图标换成同色的色块。原文不在图里，没法还原
            </div>
          </div>

          <template v-if="exportConfig.showSiteDetails">
            <div class="pt-strip">
              <PtIcon name="list-checks" :size="14" /><span>入图站点</span>
              <span class="pt-strip__end">
                {{ exportConfig.selectedSites.length }} / {{ allSites.length }}
              </span>
            </div>
            <div class="settings-body">
              <el-checkbox-group v-model="exportConfig.selectedSites" class="sites">
                <el-checkbox v-for="site in allSites" :key="site" :value="site" :label="site">
                  {{ site }}
                </el-checkbox>
              </el-checkbox-group>
              <div class="sites__acts">
                <el-button size="small" @click="exportConfig.selectedSites = [...allSites]">
                  全选
                </el-button>
                <el-button size="small" @click="exportConfig.selectedSites = []">清空</el-button>
              </div>
              <div class="field-tip">
                一个都不选时按默认取前 {{ exportConfig.maxSitesToShow }} 个；图上按上传从多到少排
              </div>
            </div>
          </template>
        </div>
      </PtPanel>
    </div>
  </div>
</template>

<style scoped>
/* 周期下拉跟在「附上周期增量」开关后面，与开关同一行高 */
.tg-range {
  width: 120px;
}

/* 栏宽（700 / 364）与间隔由 .pt-cards--main 给；这里只让两栏顶对齐 */
.export-cols {
  align-items: start;
}

/* 设置栏跟随滚动：改一个开关就想立刻看预览，不该先滚回去 */
.export-side {
  position: sticky;
  top: var(--pt-space-4);
}

/* ── 预览舞台 ─────────────────────────────────────────────── */

/*
 * 海报压在一层中性底衬上，亮色主题里浅色海报的边缘才分得清。内边距 12：700 宽的预览卡里正好留出
 * 640，桌面上海报按原大显示、不缩放。
 */
.stage {
  display: flex;
  justify-content: center;
  padding: var(--pt-space-3);
  background: var(--pt-hover);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

/* 缩放后的占位：宽高按缩放后的大小给，下面的内容不会被原大的海报撑开 */
.stage__fit {
  flex: 0 0 auto;
  overflow: hidden;
}

.stage__poster {
  width: 640px;
  transform-origin: top left;
}

/* ── 导出设置 ─────────────────────────────────────────────── */

.settings-form > .pt-strip:first-child {
  border-top: 0;
}

/* 这里的正文不走 el-form-item，所以四边都要自己给内边距 */
.settings-body {
  display: flex;
  flex-direction: column;
  padding: var(--pt-pad);
}

/* 开关 + 文字算一个整体控件，点文字也能切；label 天然带这个行为 */
.toggles {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
}

.tg {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  cursor: pointer;
  user-select: none;
}

/* 站点可以有几十个，勾选区自己滚，别把整块设置面板撑成一屏 */
.sites {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 208px;
  overflow-y: auto;
  padding: var(--pt-space-2);
  background: var(--pt-hover);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-sm);
}

.sites__acts {
  display: flex;
  gap: var(--pt-space-2);
  margin-top: var(--pt-space-2);
}

/*
 * 1181 以下单栏，并取消右栏的吸顶。
 *
 * 这一页不能靠 .pt-cards--main 退栏：那个变体只在 ≥1181 给 700 / 364 的比例，1181 以下落回
 * .pt-cards 的 auto-fit(320px)，1024 下就成了两等分 —— 预览只剩 440 宽，海报要缩到七成。
 * 单栏时预览有整条主区宽，640 的海报原样放得下。
 */
@media (max-width: 1180px) {
  .export-cols {
    grid-template-columns: minmax(0, 1fr);
  }

  .export-side {
    position: static;
  }
}
</style>
