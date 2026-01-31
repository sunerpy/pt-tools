<script setup lang="ts">
import { type AggregatedStatsResponse, userInfoApi } from "@/api";
import SiteAvatar from "@/components/SiteAvatar.vue";
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
import { computed, onMounted, ref, nextTick } from "vue";
import { useRouter } from "vue-router";

const router = useRouter();

const loading = ref(false);
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
});

const presetThemes = [
  { name: "森林绿", bg: "#134e5e", end: "#71b280" },
  { name: "蓝色海洋", bg: "#2193b0", end: "#6dd5ed" },
  { name: "日落橙", bg: "#ee0979", end: "#ff6a00" },
  { name: "暗夜黑", bg: "#232526", end: "#414345" },
  { name: "玫瑰金", bg: "#f4c4f3", end: "#fc67fa" },
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
    { label: "站点数", value: stats.siteCount.toString(), color: "#a78bfa", icon: "▣" },
  ];
  if (stats.totalSeedingBonus && stats.totalSeedingBonus > 0) {
    items.splice(5, 0, {
      label: "做种积分",
      value: formatNumber(stats.totalSeedingBonus),
      color: "#4ade80",
      icon: "✦",
    });
  }
  return items;
});

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
  loading.value = true;
  try {
    aggregatedStats.value = await userInfoApi.getAggregated();
    if (exportConfig.value.selectedSites.length === 0) {
      exportConfig.value.selectedSites = allSites.value.slice(0, exportConfig.value.maxSitesToShow);
    }
    await preloadSiteLogos();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载数据失败");
  } finally {
    loading.value = false;
  }
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
  const summaryRowHeight = 100;
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

        if (site.username) {
          ctx.font = "10px -apple-system, BlinkMacSystemFont, sans-serif";
          ctx.fillStyle = "rgba(255, 255, 255, 0.5)";
          drawMosaicText(
            ctx,
            `@${site.username}`,
            x + 10 + logoSize + 6,
            y + 34,
            exportConfig.value.blurUsernames,
            10,
          );
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
  <div class="page-container">
    <svg class="svg-filters" xmlns="http://www.w3.org/2000/svg">
      <filter id="mosaic-filter">
        <feFlood x="4" y="4" height="2" width="2" />
        <feComposite width="6" height="6" />
        <feTile result="a" />
        <feComposite in="SourceGraphic" in2="a" operator="in" />
        <feMorphology operator="dilate" radius="3" />
      </filter>
    </svg>

    <div class="export-page">
      <div class="export-preview-section">
        <div class="preview-header">
          <h2>预览</h2>
          <div class="preview-actions">
            <el-button @click="router.back()">
              <el-icon><Back /></el-icon>
              返回
            </el-button>
            <el-button :loading="copying" @click="copyToClipboard">
              <el-icon><CopyDocument /></el-icon>
              复制图片
            </el-button>
            <el-button type="primary" :loading="exporting" @click="exportImage">
              <el-icon><Download /></el-icon>
              下载图片
            </el-button>
          </div>
        </div>

        <div class="preview-scroll-container">
          <div
            v-loading="loading"
            id="export-card"
            class="export-card"
            :style="{
              background: `linear-gradient(135deg, ${exportConfig.backgroundColor}, ${exportConfig.gradientEnd})`,
            }">
            <div class="export-card-header">
              <h1 class="export-title">{{ exportConfig.title }}</h1>
              <p v-if="aggregatedStats" class="export-subtitle">
                {{ aggregatedStats.siteCount }} 个站点 · 更新于
                {{ new Date().toLocaleDateString("zh-CN") }}
              </p>
            </div>

            <div v-if="earliestJoinDate" class="export-user-info">
              <span class="user-info-icon">🎂</span>
              <span class="user-info-item">入站时间: {{ formatDate(earliestJoinDate) }}</span>
              <span class="user-info-divider">·</span>
              <span class="user-info-item">已入站 {{ formatJoinDuration(earliestJoinDate) }}</span>
            </div>

            <div v-if="aggregatedStats" class="export-summary-grid">
              <div v-for="stat in summaryStats" :key="stat.label" class="export-summary-item">
                <div class="summary-value" :style="{ color: stat.color }">{{ stat.value }}</div>
                <div class="summary-label">
                  <span class="summary-icon">{{ stat.icon }}</span>
                  {{ stat.label }}
                </div>
              </div>
            </div>

            <div
              v-if="exportConfig.showSiteDetails && selectedSiteStats.length > 0"
              class="export-sites-section">
              <h3 class="export-section-title">站点详情</h3>
              <div class="export-sites-grid">
                <div v-for="site in selectedSiteStats" :key="site.site" class="export-site-card">
                  <div class="site-card-row site-card-header-row">
                    <div class="site-card-left">
                      <div
                        class="site-avatar-wrapper"
                        :class="{ pixelated: exportConfig.blurLogos }">
                        <SiteAvatar :site-name="site.site" :site-id="site.site" :size="20" />
                      </div>
                      <div class="site-card-info">
                        <span
                          class="site-card-name"
                          :class="{ 'mosaic-text': exportConfig.blurSiteNames }">
                          {{ getMosaicText(site.site, exportConfig.blurSiteNames) }}
                        </span>
                        <span
                          v-if="site.username"
                          class="site-card-username"
                          :class="{ 'mosaic-text': exportConfig.blurUsernames }">
                          @{{ getMosaicText(site.username, exportConfig.blurUsernames) }}
                        </span>
                      </div>
                    </div>
                    <div class="site-card-right">
                      <span class="site-bonus">{{ formatNumber(site.bonus ?? 0) }}</span>
                      <span class="site-bonus-label">{{ getSiteBonusName(site.site) }}</span>
                    </div>
                  </div>
                  <div class="site-card-row site-card-stats-row">
                    <div class="site-card-left">
                      <span class="stat-upload">↑{{ formatBytes(site.uploaded) }}</span>
                      <span class="stat-download">↓{{ formatBytes(site.downloaded) }}</span>
                    </div>
                    <div class="site-card-right">
                      <span class="stat-ratio">R: {{ formatRatio(site.ratio) }}</span>
                      <span v-if="site.bonusPerHour" class="stat-bonus-hour"
                        >{{ formatNumber(site.bonusPerHour) }}/h</span
                      >
                    </div>
                  </div>
                  <div v-if="site.joinDate" class="site-card-row site-card-footer-row">
                    <span class="site-join-time"
                      >{{ formatDate(site.joinDate) }} ·
                      {{ formatJoinDuration(site.joinDate) }}</span
                    >
                  </div>
                </div>
              </div>
            </div>

            <div class="export-footer">
              Generated by pt-tools · {{ new Date().toLocaleString("zh-CN") }}
            </div>
          </div>
        </div>
      </div>

      <div class="export-settings-section">
        <div class="settings-card">
          <h3>导出设置</h3>

          <div class="setting-group">
            <label>标题</label>
            <el-input v-model="exportConfig.title" placeholder="输入标题" />
          </div>

          <div class="setting-group">
            <label>主题颜色</label>
            <div class="theme-presets">
              <div
                v-for="theme in presetThemes"
                :key="theme.name"
                class="theme-preset"
                :style="{ background: `linear-gradient(135deg, ${theme.bg}, ${theme.end})` }"
                :title="theme.name"
                @click="applyTheme(theme)" />
            </div>
            <div class="color-pickers">
              <div class="color-picker-item">
                <span>起始色</span>
                <el-color-picker v-model="exportConfig.backgroundColor" />
              </div>
              <div class="color-picker-item">
                <span>结束色</span>
                <el-color-picker v-model="exportConfig.gradientEnd" />
              </div>
            </div>
          </div>

          <div class="setting-group">
            <label>显示选项</label>
            <el-switch v-model="exportConfig.showSiteDetails" active-text="显示站点详情" />
          </div>

          <div class="setting-group">
            <label>隐私保护</label>
            <el-switch v-model="exportConfig.blurUsernames" active-text="模糊用户名" />
            <el-switch v-model="exportConfig.blurSiteNames" active-text="模糊站点名" />
            <el-switch v-model="exportConfig.blurLogos" active-text="模糊站点图标" />
          </div>

          <div v-if="exportConfig.showSiteDetails" class="setting-group">
            <label>选择站点 ({{ exportConfig.selectedSites.length }}/{{ allSites.length }})</label>
            <el-checkbox-group v-model="exportConfig.selectedSites" class="site-checkbox-group">
              <el-checkbox v-for="site in allSites" :key="site" :value="site" :label="site">
                {{ site }}
              </el-checkbox>
            </el-checkbox-group>
            <div class="site-select-actions">
              <el-button size="small" @click="exportConfig.selectedSites = [...allSites]">
                全选
              </el-button>
              <el-button size="small" @click="exportConfig.selectedSites = []">清空</el-button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";
@import "@/styles/export.css";
</style>
