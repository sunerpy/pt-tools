<script setup lang="ts">
import {
  type AggregatedStatsResponse,
  type DeltaSummary,
  userInfoApi,
  type UserInfoRange,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import SiteAvatar from "@/components/SiteAvatar.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import {
  formatBytes,
  formatNumber,
  formatRatio,
  formatDate,
  formatJoinDuration,
  getSiteBonusName,
  getAvatarColor,
} from "@/utils/format";
import { ElMessage } from "element-plus";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { computed, onMounted, ref, nextTick, watch } from "vue";
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
const siteLogos = ref<Map<string, HTMLImageElement>>(new Map());

const exportConfig = ref({
  title: "我的 PT 数据统计",
  showSiteDetails: true,
  backgroundColor: "#134e5e",
  gradientEnd: "#71b280",
  textColor: "#ffffff",
  cardBackground: "rgba(255, 255, 255, 0.12)",
  selectedSites: [] as string[],
  blurUsernames: false,
  blurSiteNames: true,
  blurLogos: true,
  maxSitesToShow: 10,
  /* 周期增量（每日快照算出来的）：今日 / 本周 / 本月 */
  showIncrements: true,
  incrementRange: "7d" as UserInfoRange,
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

const presetThemes = [
  { name: "森林绿", bg: "#134e5e", end: "#71b280" },
  { name: "海岸青", bg: "#0f766e", end: "#22d3ee" },
  { name: "日落橙", bg: "#c2410c", end: "#fb923c" },
  { name: "暗夜黑", bg: "#232526", end: "#414345" },
  { name: "暖金", bg: "#a16207", end: "#fbbf24" },
  { name: "深海蓝", bg: "#0f2027", end: "#2c5364" },
];

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

const earliestJoinDate = computed(() => {
  if (!aggregatedStats.value) return null;
  const dates = aggregatedStats.value.perSiteStats
    .filter((s) => s.joinDate && s.joinDate > 0)
    .map((s) => s.joinDate!);
  if (dates.length === 0) return null;
  return Math.min(...dates);
});

/*
 * 卡片里这些取色是写死的十六进制，不是 token —— 它导出成一张 PNG，
 * 落在别人的聊天窗口里，跟本站当前是亮色还是暗色主题没有关系。
 */
const summaryStats = computed(() => {
  if (!aggregatedStats.value) return [];
  const stats = aggregatedStats.value;
  const items = [
    { label: "总上传", value: formatBytes(stats.totalUploaded), color: "#4ade80", icon: "↑" },
    { label: "总下载", value: formatBytes(stats.totalDownloaded), color: "#f87171", icon: "↓" },
    { label: "分享率", value: formatRatio(stats.averageRatio), color: "#60a5fa", icon: "◎" },
    { label: "总魔力", value: formatNumber(stats.totalBonus), color: "#fbbf24", icon: "★" },
    {
      label: "时魔/h",
      value: formatNumber(stats.totalBonusPerHour ?? 0),
      color: "#fb923c",
      icon: "⏱",
    },
    { label: "做种数", value: stats.totalSeeding.toString(), color: "#34d399", icon: "●" },
    {
      label: "做种量",
      value: formatBytes(stats.totalSeederSize ?? 0),
      color: "#2dd4bf",
      icon: "◆",
    },
    { label: "站点数", value: stats.siteCount.toString(), color: "#14b8a6", icon: "▣" },
  ];
  if (stats.totalSeedingBonus && stats.totalSeedingBonus > 0) {
    items.splice(5, 0, {
      label: "做种积分",
      value: formatNumber(stats.totalSeedingBonus),
      color: "#4ade80",
      icon: "✦",
    });
  }
  const inc = incrementSummary.value;
  if (exportConfig.value.showIncrements && inc && hasIncrements.value) {
    const name = incrementLabel.value;
    items.push(
      {
        label: `${name}上传`,
        value: `+${formatBytes(inc.totalUploaded)}`,
        color: "#4ade80",
        icon: "↗",
      },
      {
        label: `${name}下载`,
        value: `+${formatBytes(inc.totalDownloaded)}`,
        color: "#f87171",
        icon: "↘",
      },
      {
        label: `${name}魔力`,
        value: `+${formatNumber(Math.round(inc.totalBonus))}`,
        color: "#fbbf24",
        icon: "✧",
      },
    );
  }
  return items;
});

const activeThemeName = computed(() => {
  const hit = presetThemes.find((t) => isActiveTheme(t));
  return hit ? hit.name : "自定义";
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

function isActiveTheme(theme: (typeof presetThemes)[0]) {
  return (
    exportConfig.value.backgroundColor.toLowerCase() === theme.bg &&
    exportConfig.value.gradientEnd.toLowerCase() === theme.end
  );
}

function getMosaicText(text: string, blur: boolean): string {
  if (!blur || !text) return text;
  const firstChar = text.charAt(0);
  const blocks = ["▓", "▒", "░", "▓", "▒"];
  let result = firstChar;
  for (let i = 1; i < text.length; i++) {
    result += blocks[i % blocks.length];
  }
  return result;
}

function drawMosaicText(
  ctx: CanvasRenderingContext2D,
  text: string,
  x: number,
  y: number,
  blur: boolean,
  fontSize: number = 14,
) {
  if (!blur || !text) {
    ctx.fillText(text, x, y);
    return;
  }

  const firstChar = text.charAt(0);
  ctx.fillText(firstChar, x, y);

  const firstCharWidth = ctx.measureText(firstChar).width;
  const remainingText = text.slice(1);

  if (remainingText.length === 0) return;

  const originalFill = ctx.fillStyle;
  const blockSize = fontSize * 0.55;
  const blockGap = 1;
  let currentX = x + firstCharWidth + 3;

  for (let i = 0; i < remainingText.length; i++) {
    const rows = 3;
    const cols = 2;
    const cellSize = blockSize / rows;

    for (let row = 0; row < rows; row++) {
      for (let col = 0; col < cols; col++) {
        const seed = (i * 7 + row * 3 + col) % 10;
        const brightness = 0.25 + (seed / 10) * 0.45;

        ctx.globalAlpha = brightness;
        ctx.fillStyle = typeof originalFill === "string" ? originalFill : "#ffffff";

        const cellX = currentX + col * cellSize;
        const cellY = y - blockSize + row * cellSize;

        ctx.fillRect(cellX, cellY, cellSize - 0.5, cellSize - 0.5);
      }
    }

    currentX += blockSize + blockGap;
  }

  ctx.globalAlpha = 1;
  ctx.fillStyle = originalFill;
}

function drawPixelatedImage(
  ctx: CanvasRenderingContext2D,
  img: HTMLImageElement,
  x: number,
  y: number,
  size: number,
  pixelSize: number = 3,
) {
  const tempCanvas = document.createElement("canvas");
  const tempCtx = tempCanvas.getContext("2d");
  if (!tempCtx) return;

  const smallSize = Math.ceil(size / pixelSize);
  tempCanvas.width = smallSize;
  tempCanvas.height = smallSize;

  tempCtx.drawImage(img, 0, 0, smallSize, smallSize);

  ctx.save();
  ctx.imageSmoothingEnabled = false;
  roundRect(ctx, x, y, size, size, 4);
  ctx.clip();
  ctx.drawImage(tempCanvas, 0, 0, smallSize, smallSize, x, y, size, size);
  ctx.restore();
}

function drawSiteLogo(
  ctx: CanvasRenderingContext2D,
  siteId: string,
  x: number,
  y: number,
  size: number,
  pixelate: boolean = false,
) {
  const logo = siteLogos.value.get(siteId.toLowerCase());

  ctx.save();

  if (logo && logo.complete && logo.naturalWidth > 0) {
    if (pixelate) {
      drawPixelatedImage(ctx, logo, x, y, size, 4);
    } else {
      roundRect(ctx, x, y, size, size, 4);
      ctx.clip();
      ctx.fillStyle = "#ffffff";
      ctx.fillRect(x, y, size, size);
      ctx.drawImage(logo, x, y, size, size);
    }
  } else {
    roundRect(ctx, x, y, size, size, 4);
    ctx.clip();

    const color = getAvatarColor(siteId);
    ctx.fillStyle = color;
    ctx.fillRect(x, y, size, size);

    if (pixelate) {
      const blockSize = 4;
      for (let py = 0; py < size; py += blockSize) {
        for (let px = 0; px < size; px += blockSize) {
          const seed = (px * 7 + py * 13) % 20;
          ctx.globalAlpha = 0.3 + (seed / 20) * 0.4;
          ctx.fillStyle = seed % 2 === 0 ? "rgba(255,255,255,0.3)" : "rgba(0,0,0,0.2)";
          ctx.fillRect(x + px, y + py, blockSize - 0.5, blockSize - 0.5);
        }
      }
      ctx.globalAlpha = 1;
    } else {
      ctx.fillStyle = "#ffffff";
      ctx.font = `bold ${size * 0.5}px -apple-system, BlinkMacSystemFont, sans-serif`;
      ctx.textAlign = "center";
      ctx.textBaseline = "middle";
      ctx.fillText(siteId.charAt(0).toUpperCase(), x + size / 2, y + size / 2);
    }
  }

  ctx.restore();
}

async function preloadSiteLogos() {
  const sites = aggregatedStats.value?.perSiteStats || [];
  const loadPromises = sites.map((site) => {
    return new Promise<void>((resolve) => {
      const img = new Image();
      img.crossOrigin = "anonymous";
      img.onload = () => {
        siteLogos.value.set(site.site.toLowerCase(), img);
        resolve();
      };
      img.onerror = () => resolve();
      img.src = `/api/favicon/${site.site.toLowerCase()}`;
    });
  });

  await Promise.all(loadPromises);
}

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
  await preloadSiteLogos();
}

function applyTheme(theme: (typeof presetThemes)[0]) {
  exportConfig.value.backgroundColor = theme.bg;
  exportConfig.value.gradientEnd = theme.end;
}

function createExportCanvas(): HTMLCanvasElement {
  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d");
  if (!ctx) {
    throw new Error("无法创建 Canvas 上下文");
  }

  const scale = 2;
  const width = 640;
  const padding = 28;
  const headerHeight = 90;
  const userInfoHeight = earliestJoinDate.value ? 50 : 0;
  /* 汇总卡每行 4 张、每张 42 高加 8 间距；行数随卡片数走（原来写死 100，卡片超过两行会被裁掉） */
  const summaryPerRow = Math.max(1, Math.min(summaryStats.value.length, 4));
  const summaryRowHeight = Math.max(2, Math.ceil(summaryStats.value.length / summaryPerRow)) * 50;
  const siteCardHeight = 72;
  const sitesCount = selectedSiteStats.value.length;
  const siteRows = Math.ceil(sitesCount / 2);
  const sitesHeight = exportConfig.value.showSiteDetails ? siteRows * siteCardHeight + 36 : 0;
  const footerHeight = 44;
  const height =
    headerHeight + userInfoHeight + summaryRowHeight + sitesHeight + footerHeight + padding * 2;

  canvas.width = width * scale;
  canvas.height = height * scale;
  ctx.scale(scale, scale);

  const gradient = ctx.createLinearGradient(0, 0, width, height);
  gradient.addColorStop(0, exportConfig.value.backgroundColor);
  gradient.addColorStop(1, exportConfig.value.gradientEnd);
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, width, height);

  ctx.fillStyle = exportConfig.value.textColor;
  ctx.textAlign = "center";

  ctx.font = "bold 26px -apple-system, BlinkMacSystemFont, sans-serif";
  ctx.fillText(exportConfig.value.title, width / 2, padding + 36);

  if (aggregatedStats.value) {
    const stats = aggregatedStats.value;

    ctx.font = "13px -apple-system, BlinkMacSystemFont, sans-serif";
    ctx.fillStyle = "rgba(255, 255, 255, 0.75)";
    const subtitle = `${stats.siteCount} 个站点 · 更新于 ${new Date().toLocaleDateString("zh-CN")}`;
    ctx.fillText(subtitle, width / 2, padding + 60);

    const userInfoY = padding + headerHeight;
    if (earliestJoinDate.value) {
      ctx.fillStyle = "rgba(255, 255, 255, 0.1)";
      roundRect(ctx, padding, userInfoY, width - padding * 2, 38, 10);
      ctx.fill();

      ctx.fillStyle = "rgba(255, 255, 255, 0.9)";
      ctx.font = "12px -apple-system, BlinkMacSystemFont, sans-serif";
      ctx.textAlign = "center";
      const joinInfo = `🎂 入站时间: ${formatDate(earliestJoinDate.value)} · 已入站 ${formatJoinDuration(earliestJoinDate.value)}`;
      ctx.fillText(joinInfo, width / 2, userInfoY + 24);
    }

    const summaryY = userInfoY + userInfoHeight + 8;
    const statsPerRow = Math.min(summaryStats.value.length, 4);
    const rows = Math.ceil(summaryStats.value.length / statsPerRow);
    const cardWidth = (width - padding * 2 - (statsPerRow - 1) * 10) / statsPerRow;
    const cardHeight = 42;

    summaryStats.value.forEach((stat, index) => {
      const row = Math.floor(index / statsPerRow);
      const col = index % statsPerRow;
      const x = padding + col * (cardWidth + 10);
      const y = summaryY + row * (cardHeight + 8);

      ctx.fillStyle = "rgba(255, 255, 255, 0.1)";
      roundRect(ctx, x, y, cardWidth, cardHeight, 8);
      ctx.fill();

      ctx.fillStyle = stat.color;
      ctx.font = "bold 15px -apple-system, BlinkMacSystemFont, sans-serif";
      ctx.textAlign = "center";
      ctx.fillText(stat.value, x + cardWidth / 2, y + 18);

      ctx.fillStyle = "rgba(255, 255, 255, 0.6)";
      ctx.font = "10px -apple-system, BlinkMacSystemFont, sans-serif";
      ctx.fillText(`${stat.icon} ${stat.label}`, x + cardWidth / 2, y + 34);
    });

    if (exportConfig.value.showSiteDetails && selectedSiteStats.value.length > 0) {
      const actualSummaryHeight = rows * (cardHeight + 8);
      const sitesStartY = summaryY + actualSummaryHeight + 16;

      ctx.fillStyle = "rgba(255, 255, 255, 0.5)";
      ctx.font = "12px -apple-system, BlinkMacSystemFont, sans-serif";
      ctx.textAlign = "left";
      ctx.fillText("站点详情", padding, sitesStartY);

      const siteCardWidth = (width - padding * 2 - 12) / 2;
      const logoSize = 18;

      selectedSiteStats.value.forEach((site, index) => {
        const row = Math.floor(index / 2);
        const col = index % 2;
        const x = padding + col * (siteCardWidth + 12);
        const y = sitesStartY + 16 + row * siteCardHeight;

        ctx.fillStyle = "rgba(255, 255, 255, 0.08)";
        roundRect(ctx, x, y, siteCardWidth, siteCardHeight - 6, 8);
        ctx.fill();

        drawSiteLogo(ctx, site.site, x + 10, y + 8, logoSize, exportConfig.value.blurLogos);

        ctx.fillStyle = "#ffffff";
        ctx.font = "bold 12px -apple-system, BlinkMacSystemFont, sans-serif";
        ctx.textAlign = "left";
        drawMosaicText(
          ctx,
          site.site,
          x + 10 + logoSize + 6,
          y + 20,
          exportConfig.value.blurSiteNames,
          12,
        );

        // 绘制用户名和等级（在同一行）
        const levelText = site.levelName || site.rank;
        if (site.username || levelText) {
          ctx.font = "10px -apple-system, BlinkMacSystemFont, sans-serif";
          let currentX = x + 10 + logoSize + 6;

          if (site.username) {
            ctx.fillStyle = "rgba(255, 255, 255, 0.5)";
            drawMosaicText(
              ctx,
              `@${site.username}`,
              currentX,
              y + 34,
              exportConfig.value.blurUsernames,
              10,
            );
            currentX +=
              ctx.measureText(`@${getMosaicText(site.username, exportConfig.value.blurUsernames)}`)
                .width + 6;
          }

          if (levelText) {
            ctx.fillStyle = "#22d3ee";
            ctx.fillText(levelText, currentX, y + 34);
          }
        }

        ctx.font = "10px -apple-system, BlinkMacSystemFont, sans-serif";
        ctx.textAlign = "left";

        ctx.fillStyle = "#4ade80";
        ctx.fillText(`↑${formatBytes(site.uploaded)}`, x + 10, y + 50);

        ctx.fillStyle = "#f87171";
        const uploadWidth = ctx.measureText(`↑${formatBytes(site.uploaded)}`).width;
        ctx.fillText(`↓${formatBytes(site.downloaded)}`, x + 10 + uploadWidth + 8, y + 50);

        ctx.fillStyle = "#fbbf24";
        ctx.textAlign = "right";
        ctx.fillText(
          `${formatNumber(site.bonus ?? 0)} ${getSiteBonusName(site.site)}`,
          x + siteCardWidth - 10,
          y + 20,
        );

        if (site.bonusPerHour && site.bonusPerHour > 0) {
          ctx.fillStyle = "#fb923c";
          ctx.fillText(`${formatNumber(site.bonusPerHour)}/h`, x + siteCardWidth - 10, y + 34);
        }

        ctx.fillStyle = "#93c5fd";
        ctx.fillText(`R: ${formatRatio(site.ratio)}`, x + siteCardWidth - 10, y + 50);

        if (site.joinDate) {
          ctx.fillStyle = "rgba(255, 255, 255, 0.4)";
          ctx.textAlign = "left";
          ctx.fillText(
            `${formatDate(site.joinDate)} · ${formatJoinDuration(site.joinDate)}`,
            x + 10,
            y + 62,
          );
        }
      });
    }
  }

  ctx.fillStyle = "rgba(255, 255, 255, 0.4)";
  ctx.font = "10px -apple-system, BlinkMacSystemFont, sans-serif";
  ctx.textAlign = "center";
  ctx.fillText(
    `Generated by pt-tools · ${new Date().toLocaleString("zh-CN")}`,
    width / 2,
    height - 16,
  );

  return canvas;
}

async function exportImage() {
  exporting.value = true;
  await nextTick();

  try {
    const canvas = createExportCanvas();
    const dataUrl = canvas.toDataURL("image/png", 1.0);
    const link = document.createElement("a");
    link.download = `pt-stats-${new Date().toISOString().split("T")[0]}.png`;
    link.href = dataUrl;
    link.click();

    ElMessage.success("图片已导出");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "导出失败");
  } finally {
    exporting.value = false;
  }
}

async function copyToClipboard() {
  copying.value = true;
  await nextTick();

  try {
    // 检查是否为安全上下文（HTTPS 或 localhost）
    if (!window.isSecureContext) {
      ElMessage.warning("HTTP 环境不支持一键复制，请右键复制预览图或使用下载功能");
      return;
    }

    const canvas = createExportCanvas();

    // 方法1: 现代 Clipboard API (需要 HTTPS)
    if (
      navigator.clipboard &&
      typeof navigator.clipboard.write === "function" &&
      typeof ClipboardItem !== "undefined"
    ) {
      const blob = await new Promise<Blob>((resolve, reject) => {
        canvas.toBlob(
          (b) => {
            if (b) resolve(b);
            else reject(new Error("无法生成图片"));
          },
          "image/png",
          1.0,
        );
      });
      await navigator.clipboard.write([new ClipboardItem({ "image/png": blob })]);
      ElMessage.success("图片已复制到剪贴板");
      return;
    }

    // 方法2: 复制 Data URL 文本
    if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
      const dataUrl = canvas.toDataURL("image/png", 1.0);
      await navigator.clipboard.writeText(dataUrl);
      ElMessage.warning("浏览器不支持复制图片，已复制图片 Base64 数据");
      return;
    }

    // 不支持任何剪贴板 API
    ElMessage.warning("浏览器不支持剪贴板操作，请右键复制预览图或使用下载功能");
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

function roundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  width: number,
  height: number,
  radius: number,
) {
  ctx.beginPath();
  ctx.moveTo(x + radius, y);
  ctx.lineTo(x + width - radius, y);
  ctx.quadraticCurveTo(x + width, y, x + width, y + radius);
  ctx.lineTo(x + width, y + height - radius);
  ctx.quadraticCurveTo(x + width, y + height, x + width - radius, y + height);
  ctx.lineTo(x + radius, y + height);
  ctx.quadraticCurveTo(x, y + height, x, y + height - radius);
  ctx.lineTo(x, y + radius);
  ctx.quadraticCurveTo(x, y, x + radius, y);
  ctx.closePath();
}

onMounted(() => {
  loadData();
});
</script>

<template>
  <div class="export-page">
    <!-- 打码用的 SVG 滤镜容器：本身不可见，只供 .pcard__logo.is-pixelated 引用 -->
    <svg class="svg-filters" xmlns="http://www.w3.org/2000/svg">
      <filter id="mosaic-filter">
        <feFlood x="4" y="4" height="2" width="2" />
        <feComposite width="6" height="6" />
        <feTile result="a" />
        <feComposite in="SourceGraphic" in2="a" operator="in" />
        <feMorphology operator="dilate" radius="3" />
      </filter>
    </svg>

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
          v-if="!aggregatedStats"
          :state="state"
          :title="state === 'empty' ? '还没有可导出的统计' : ''"
          :sub="stateSub">
          <template v-if="state === 'error'" #action>
            <el-button @click="loadData">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>

        <!-- 预览用 DOM 复刻画布，不是画布本身：导出走 canvas，两边的排版规则要手动对齐 -->
        <div v-else class="stage">
          <div
            class="poster"
            :style="{
              background: `linear-gradient(135deg, ${exportConfig.backgroundColor}, ${exportConfig.gradientEnd})`,
            }">
            <div class="poster__head">
              <h1 class="poster__title">{{ exportConfig.title }}</h1>
              <p v-if="aggregatedStats" class="poster__sub">
                {{ aggregatedStats.siteCount }} 个站点 · 更新于
                {{ new Date().toLocaleDateString("zh-CN") }}
              </p>
            </div>

            <div v-if="earliestJoinDate" class="poster__join">
              <span class="poster__join-icon">🎂</span>
              <span>入站时间: {{ formatDate(earliestJoinDate) }}</span>
              <span class="poster__join-sep">·</span>
              <span>已入站 {{ formatJoinDuration(earliestJoinDate) }}</span>
            </div>

            <div v-if="aggregatedStats" class="poster__stats">
              <div v-for="stat in summaryStats" :key="stat.label" class="pstat">
                <div class="pstat__v" :style="{ color: stat.color }">{{ stat.value }}</div>
                <div class="pstat__l">
                  <span class="pstat__i">{{ stat.icon }}</span>
                  {{ stat.label }}
                </div>
              </div>
            </div>

            <div
              v-if="exportConfig.showSiteDetails && selectedSiteStats.length > 0"
              class="poster__sites">
              <h3 class="poster__sites-title">站点详情</h3>
              <div class="poster__sites-grid">
                <div v-for="site in selectedSiteStats" :key="site.site" class="pcard">
                  <div class="pcard__row pcard__row--head">
                    <div class="pcard__l">
                      <div class="pcard__logo" :class="{ 'is-pixelated': exportConfig.blurLogos }">
                        <SiteAvatar :site-name="site.site" :site-id="site.site" :size="20" />
                      </div>
                      <div class="pcard__ident">
                        <span
                          class="pcard__name"
                          :class="{ 'is-mosaic': exportConfig.blurSiteNames }">
                          {{ getMosaicText(site.site, exportConfig.blurSiteNames) }}
                        </span>
                        <div class="pcard__meta">
                          <span
                            v-if="site.username"
                            class="pcard__user"
                            :class="{ 'is-mosaic': exportConfig.blurUsernames }">
                            @{{ getMosaicText(site.username, exportConfig.blurUsernames) }}
                          </span>
                          <span v-if="site.levelName || site.rank" class="pcard__level">
                            {{ site.levelName || site.rank }}
                          </span>
                        </div>
                      </div>
                    </div>
                    <div class="pcard__r">
                      <span class="pcard__bonus">{{ formatNumber(site.bonus ?? 0) }}</span>
                      <span class="pcard__bonus-unit">{{ getSiteBonusName(site.site) }}</span>
                    </div>
                  </div>

                  <div class="pcard__row pcard__row--num">
                    <div class="pcard__l">
                      <span class="pcard__up">↑{{ formatBytes(site.uploaded) }}</span>
                      <span class="pcard__down">↓{{ formatBytes(site.downloaded) }}</span>
                    </div>
                    <div class="pcard__r">
                      <span class="pcard__ratio">R: {{ formatRatio(site.ratio) }}</span>
                      <span v-if="site.bonusPerHour" class="pcard__rate">
                        {{ formatNumber(site.bonusPerHour) }}/h
                      </span>
                    </div>
                  </div>

                  <div v-if="site.joinDate" class="pcard__row pcard__row--foot">
                    <span>
                      {{ formatDate(site.joinDate) }} · {{ formatJoinDuration(site.joinDate) }}
                    </span>
                  </div>
                </div>
              </div>
            </div>

            <div class="poster__foot">
              Generated by pt-tools · {{ new Date().toLocaleString("zh-CN") }}
            </div>
          </div>
        </div>
      </PtPanel>

      <PtPanel class="export-side" title="导出设置" icon="settings" padding="none">
        <div class="pt-form settings-form">
          <div class="pt-strip"><PtIcon name="type" :size="14" /><span>标题</span></div>
          <div class="settings-body">
            <el-input v-model="exportConfig.title" placeholder="输入标题" />
            <div class="field-tip">印在图片最上方，导出后就固定在图里了</div>
          </div>

          <div class="pt-strip">
            <PtIcon name="palette" :size="14" /><span>配色</span>
            <span class="pt-strip__end">{{ activeThemeName }}</span>
          </div>
          <div class="settings-body">
            <!-- 预设色块自己就是自己的图例，配上文字标签只会让这一排变成两行 -->
            <div class="swatches">
              <button
                v-for="theme in presetThemes"
                :key="theme.name"
                type="button"
                class="swatch"
                :class="{ 'is-on': isActiveTheme(theme) }"
                :style="{ background: `linear-gradient(135deg, ${theme.bg}, ${theme.end})` }"
                :title="theme.name"
                :aria-label="`使用${theme.name}配色`"
                @click="applyTheme(theme)">
                <PtIcon v-if="isActiveTheme(theme)" name="check" :size="16" />
              </button>
            </div>
            <div class="picks">
              <label class="pick">
                <span>起始色</span>
                <el-color-picker v-model="exportConfig.backgroundColor" />
              </label>
              <label class="pick">
                <span>结束色</span>
                <el-color-picker v-model="exportConfig.gradientEnd" />
              </label>
            </div>
            <div class="field-tip">左上到右下的两色渐变，手调后上面会显示「自定义」</div>
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
              打码只保留首字符，导出的 PNG 里也是真马赛克，不是能还原的模糊
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
                一个都不选时按默认取前 {{ exportConfig.maxSitesToShow }} 个
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

/* 卡片层的内缩与间隔由 .pt-cards--main 给；这一层只用来挂 SVG 滤镜与 Teleport */

/* 滤镜容器：占位为 0，但不能 display:none，否则 Safari 里 url(#…) 引用失效 */
.svg-filters {
  position: absolute;
  width: 0;
  height: 0;
  overflow: hidden;
  pointer-events: none;
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

/* 海报是纯白字压在自选渐变上，得给它一层中性底衬，否则亮色主题里边缘糊在一起 */
.stage {
  display: flex;
  justify-content: center;
  padding: var(--pt-space-4);
  background: var(--pt-hover);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

/*
 * 以下 .poster / .pstat / .pcard 全部写死颜色和像素值，故意不用 token：
 * 它们是 createExportCanvas() 那张 640 宽 PNG 的 DOM 复刻，
 * 必须跟画布逐像素对上，而画布不认识本站的主题变量。
 */
.poster {
  position: relative;
  width: 100%;
  max-width: 640px;
  overflow: hidden;
  padding: 28px;
  color: #ffffff;
  border-radius: 20px;
  box-shadow: 0 20px 45px rgb(2 6 23 / 28%);
}

.poster::before {
  position: absolute;
  top: -50%;
  left: -50%;
  width: 200%;
  height: 200%;
  content: "";
  background: radial-gradient(circle, rgb(255 255 255 / 8%) 0%, transparent 50%);
  pointer-events: none;
}

.poster__head {
  position: relative;
  z-index: 1;
  margin-bottom: 20px;
  text-align: center;
}

.poster__title {
  margin: 0 0 6px;
  font-size: 26px;
  font-weight: 800;
  letter-spacing: -0.02em;
  text-shadow: 0 2px 4px rgb(0 0 0 / 10%);
}

.poster__sub {
  margin: 0;
  font-size: 13px;
  opacity: 0.75;
}

.poster__join {
  position: relative;
  z-index: 1;
  display: flex;
  gap: 6px;
  align-items: center;
  justify-content: center;
  margin-bottom: 16px;
  padding: 10px 16px;
  font-size: 12px;
  background: rgb(255 255 255 / 10%);
  border-radius: 10px;
}

.poster__join-icon {
  font-size: 14px;
}

.poster__join-sep {
  opacity: 0.5;
}

.poster__stats {
  position: relative;
  z-index: 1;
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
  margin-bottom: 20px;
}

.pstat {
  padding: 10px 8px;
  text-align: center;
  background: rgb(255 255 255 / 10%);
  border-radius: 8px;
}

.pstat__v {
  margin-bottom: 2px;
  font-size: 15px;
  font-weight: 700;
}

.pstat__l {
  display: flex;
  gap: 3px;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  opacity: 0.6;
}

.pstat__i {
  font-size: 9px;
}

.poster__sites {
  position: relative;
  z-index: 1;
}

.poster__sites-title {
  margin: 0 0 12px;
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  opacity: 0.5;
}

.poster__sites-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.pcard {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  background: rgb(255 255 255 / 8%);
  border-radius: 8px;
}

.pcard__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.pcard__row--head {
  align-items: flex-start;
}

.pcard__row--num {
  font-size: 10px;
}

.pcard__row--foot {
  font-size: 9px;
  opacity: 0.4;
}

.pcard__l,
.pcard__r {
  display: flex;
  gap: 6px;
  align-items: center;
}

.pcard__r {
  text-align: right;
}

.pcard__ident {
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.pcard__meta {
  display: flex;
  gap: 6px;
  align-items: center;
}

.pcard__name {
  font-size: 12px;
  font-weight: 700;
}

.pcard__user {
  font-size: 10px;
  opacity: 0.5;
}

.pcard__level {
  font-size: 10px;
  font-weight: 500;
  color: #2dd4bf;
}

.pcard__bonus {
  font-size: 12px;
  font-weight: 700;
  color: #fbbf24;
}

.pcard__bonus-unit {
  font-size: 9px;
  opacity: 0.5;
}

.pcard__up {
  color: #4ade80;
}

.pcard__down {
  color: #f87171;
}

.pcard__ratio {
  color: #93c5fd;
}

.pcard__rate {
  font-size: 9px;
  color: #fb923c;
  opacity: 0.7;
}

.pcard__logo {
  display: flex;
  overflow: hidden;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
}

.pcard__logo.is-pixelated {
  filter: url("#mosaic-filter");
  image-rendering: pixelated;
}

/* 文字打码：字符已经在 getMosaicText() 里换成方块，这里只把字距拉开对齐画布 */
.is-mosaic {
  font-family: var(--pt-font-mono);
  letter-spacing: 1px;
}

.poster__foot {
  position: relative;
  z-index: 1;
  margin-top: 20px;
  padding-top: 14px;
  font-size: 10px;
  text-align: center;
  border-top: 1px solid rgb(255 255 255 / 8%);
  opacity: 0.4;
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

.swatches {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
}

.swatch {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  padding: 0;
  color: #ffffff;
  cursor: pointer;
  border: 2px solid transparent;
  border-radius: var(--pt-r-md);
  transition:
    border-color var(--pt-transition-fast),
    transform var(--pt-transition-fast);
}

.swatch:hover {
  transform: translateY(-1px);
  border-color: var(--pt-border);
}

.swatch.is-on {
  border-color: var(--pt-t1);
}

.picks {
  display: flex;
  gap: var(--pt-space-4);
  margin-top: var(--pt-space-3);
}

.pick {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  cursor: pointer;
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
 * .pt-cards 的 auto-fit(320px)，1024 下就成了两等分 —— 预览只剩 440 宽，分享图的 DOM 预览随之折行
 * （「412.6 / TB」），站点卡右列被切。导出的 PNG 是 Canvas 按固定 640 宽画的，不受影响，
 * 但预览不再是所见即所得。单栏时预览有整条主区宽，640 的分享图原样放得下。
 */
@media (max-width: 1180px) {
  .export-cols {
    grid-template-columns: minmax(0, 1fr);
  }

  .export-side {
    position: static;
  }
}

@media (max-width: 540px) {
  .poster {
    padding: 18px;
  }

  .poster__stats {
    grid-template-columns: repeat(2, 1fr);
  }

  .poster__sites-grid {
    grid-template-columns: 1fr;
  }

  /*
   * 单栏的站点卡在 375 宽的手机上只有 ~230：「128.49万」在「万」前折行、「魔力」竖成两行，
   * 等级「Crazy User」也折两行。数值与单位不折，右边不让宽；左边的身份区让出宽度，等级名省略。
   * 只影响手机上的预览 —— 导出的 PNG 是 Canvas 按 640 宽画的。
   */
  .pcard__bonus,
  .pcard__bonus-unit,
  .pcard__user,
  .pcard__level {
    white-space: nowrap;
  }

  .pcard__row--head > .pcard__r {
    flex: 0 0 auto;
  }

  .pcard__l,
  .pcard__ident,
  .pcard__meta {
    min-width: 0;
  }

  .pcard__level {
    overflow: hidden;
    text-overflow: ellipsis;
  }
}
</style>
