<script setup lang="ts">
import { type AggregatedStatsResponse, userInfoApi } from "@/api";
import LevelTooltip from "@/components/LevelTooltip.vue";
import SiteAvatar from "@/components/SiteAvatar.vue";
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
import { computed, onMounted, onUnmounted, ref } from "vue";

const siteLevelsStore = useSiteLevelsStore();

const loading = ref(false);
const syncing = ref(false);
const syncingSite = ref<string | null>(null);
const aggregatedStats = ref<AggregatedStatsResponse | null>(null);
const isMobile = ref(window.innerWidth < 768);

// 定时刷新相关
const REFRESH_INTERVAL = 5 * 60 * 1000; // 5分钟
let refreshTimer: ReturnType<typeof setInterval> | null = null;
const autoRefreshEnabled = ref(true);

// 监听窗口大小变化
function handleResize() {
  isMobile.value = window.innerWidth < 768;
}

// 计算统计卡片数据
const statsCards = computed(() => {
  if (!aggregatedStats.value) return [];
  const stats = aggregatedStats.value;
  const cards = [
    {
      title: "总上传量",
      value: formatBytes(stats.totalUploaded),
      icon: "Upload",
      type: "success",
      className: "stat-upload",
    },
    {
      title: "总下载量",
      value: formatBytes(stats.totalDownloaded),
      icon: "Download",
      type: "info",
      className: "stat-download",
    },
    {
      title: "平均分享率",
      value: formatRatio(stats.averageRatio),
      icon: "DataAnalysis",
      type: stats.averageRatio >= 1 ? "success" : "warning",
      className: "stat-ratio",
    },
    {
      title: "做种数",
      value: stats.totalSeeding.toString(),
      icon: "Connection",
      type: "success",
      className: "stat-seeding",
    },
    {
      title: "下载中",
      value: stats.totalLeeching.toString(),
      icon: "Loading",
      type: "info",
      className: "stat-leeching",
    },
    {
      title: "总魔力值",
      value: formatNumber(stats.totalBonus),
      icon: "Star",
      type: "warning",
      className: "stat-bonus",
    },
    {
      title: "总时魔/h",
      value: formatNumber(stats.totalBonusPerHour ?? 0),
      icon: "Timer",
      type: "warning",
      className: "stat-bonus-hour",
    },
  ];

  // 只有当存在做种积分时才显示
  if (stats.totalSeedingBonus && stats.totalSeedingBonus > 0) {
    cards.push({
      title: "总做种积分",
      value: formatNumber(stats.totalSeedingBonus),
      icon: "Medal",
      type: "success",
      className: "stat-seeding-bonus",
    });
  }

  cards.push(
    {
      title: "做种总量",
      value: formatBytes(stats.totalSeederSize ?? 0),
      icon: "Upload",
      type: "success",
      className: "stat-seeding-size",
    },
    {
      title: "站点数量",
      value: stats.siteCount.toString(),
      icon: "OfficeBuilding",
      type: "info",
      className: "stat-sites",
    },
  );

  return cards;
});

// 加载数据
async function loadData() {
  loading.value = true;
  try {
    aggregatedStats.value = await userInfoApi.getAggregated();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
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
  window.addEventListener("resize", handleResize);
});

onUnmounted(() => {
  stopAutoRefresh();
  window.removeEventListener("resize", handleResize);
});
</script>

<template>
  <div class="page-container">
    <el-alert type="info" show-icon :closable="true" class="extension-banner">
      <template #title>
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
      </template>
    </el-alert>

    <!-- 统计卡片 -->
    <div class="dashboard-stats-row">
      <div
        v-for="card in statsCards"
        :key="card.title"
        class="dashboard-stat-card"
        :class="[card.type, card.className]">
        <div class="dashboard-stat-icon">
          <el-icon><component :is="card.icon" /></el-icon>
        </div>
        <div class="dashboard-stat-info">
          <div class="dashboard-stat-value">{{ card.value }}</div>
          <div class="dashboard-stat-title">{{ card.title }}</div>
        </div>
      </div>
    </div>

    <!-- 站点详情表格 -->
    <div v-loading="loading" class="dashboard-table-card">
      <div class="dashboard-card-header">
        <h3>站点统计详情</h3>
        <div class="dashboard-header-actions">
          <el-tooltip
            :content="autoRefreshEnabled ? '点击关闭自动刷新 (5分钟)' : '点击开启自动刷新'"
            placement="top">
            <el-button
              size="small"
              :type="autoRefreshEnabled ? 'success' : 'info'"
              @click="toggleAutoRefresh">
              <el-icon><Timer /></el-icon>
              {{ autoRefreshEnabled ? "自动刷新中" : "自动刷新已关闭" }}
            </el-button>
          </el-tooltip>
          <el-button size="small" @click="clearCache">清除缓存</el-button>
          <el-button size="small" type="info" @click="$router.push('/userinfo/export')">
            <el-icon><Share /></el-icon>
            导出分享
          </el-button>
          <el-button type="primary" size="small" :loading="syncing" @click="syncAll">
            <el-icon><Refresh /></el-icon>
            同步全部
          </el-button>
        </div>
      </div>

      <div class="dashboard-card-body">
        <!-- 桌面端表格视图 -->
        <el-table
          v-if="!isMobile"
          :data="aggregatedStats?.perSiteStats || []"
          style="width: 100%"
          :default-sort="{ prop: 'uploaded', order: 'descending' }"
          stripe
          highlight-current-row>
          <!-- 站点列：带消息徽章和悬停效果 -->
          <el-table-column prop="site" label="站点" min-width="160" sortable fixed="left">
            <template #default="{ row }">
              <div class="site-cell">
                <el-badge
                  :value="row.unreadMessageCount"
                  :hidden="!row.unreadMessageCount || row.unreadMessageCount === 0"
                  :max="99"
                  type="danger">
                  <div class="site-avatar-wrapper" @click.stop="syncSite(row.site)">
                    <SiteAvatar :site-name="row.site" :site-id="row.site" :size="32" />
                    <el-icon v-if="syncingSite === row.site" class="sync-icon is-loading">
                      <Loading />
                    </el-icon>
                  </div>
                </el-badge>
                <div class="site-info">
                  <span class="site-name">{{ row.site }}</span>
                  <span class="user-name">{{ row.username }}</span>
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
          <el-table-column prop="uploaded" label="数据量" min-width="140" sortable align="right">
            <template #default="{ row }">
              <div class="data-cell">
                <div class="data-row upload">
                  <el-icon><Top /></el-icon>
                  <span>{{ formatBytes(row.uploaded) }}</span>
                </div>
                <div class="data-row download">
                  <el-icon><Bottom /></el-icon>
                  <span>{{ formatBytes(row.downloaded) }}</span>
                </div>
              </div>
            </template>
          </el-table-column>

          <!-- 真实数据（如果不同） -->
          <el-table-column
            prop="trueUploaded"
            label="真实数据"
            min-width="140"
            sortable
            align="right">
            <template #default="{ row }">
              <div v-if="row.trueUploaded && row.trueUploaded !== row.uploaded" class="data-cell">
                <div class="data-row upload">
                  <el-icon><Top /></el-icon>
                  <span>{{ formatBytes(row.trueUploaded) }}</span>
                </div>
                <div class="data-row download">
                  <el-icon><Bottom /></el-icon>
                  <span>{{ formatBytes(row.trueDownloaded ?? 0) }}</span>
                </div>
              </div>
              <span v-else class="no-data">-</span>
            </template>
          </el-table-column>

          <!-- 分享率 -->
          <el-table-column prop="ratio" label="分享率" min-width="90" sortable align="center">
            <template #default="{ row }">
              <el-tag :type="getRatioType(row.ratio)" size="small" effect="plain">
                {{ formatRatio(row.ratio) }}
              </el-tag>
            </template>
          </el-table-column>

          <!-- 做种数 + H&R -->
          <el-table-column prop="seeding" label="做种" min-width="110" sortable align="center">
            <template #default="{ row }">
              <div class="seeding-cell">
                <div class="seeding-count">
                  <el-tag type="success" size="small" effect="plain">{{ row.seeding }}</el-tag>
                </div>
                <div v-if="hasHnR(row)" class="hnr-info">
                  <el-tooltip
                    v-if="row.hnrPreWarning > 0"
                    :content="`H&R 预警: ${row.hnrPreWarning}`">
                    <span class="hnr-item warning">
                      <el-icon><Warning /></el-icon>
                      <span>{{ row.hnrPreWarning }}</span>
                    </span>
                  </el-tooltip>
                  <el-tooltip
                    v-if="row.hnrUnsatisfied > 0"
                    :content="`H&R 未满足: ${row.hnrUnsatisfied}`">
                    <span class="hnr-item danger">
                      <el-icon><CircleClose /></el-icon>
                      <span>{{ row.hnrUnsatisfied }}</span>
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
            align="right">
            <template #default="{ row }">
              <span class="seeding-size">{{ formatBytes(row.seederSize ?? 0) }}</span>
            </template>
          </el-table-column>

          <!-- 魔力值 + 做种积分 -->
          <el-table-column prop="bonus" label="积分" min-width="140" sortable align="right">
            <template #default="{ row }">
              <div class="bonus-cell">
                <el-tooltip :content="getSiteBonusName(row.site)" placement="left">
                  <div class="bonus-row">
                    <span class="value">{{ formatNumber(row.bonus ?? 0) }}</span>
                    <span class="label">{{ getSiteBonusName(row.site) }}</span>
                  </div>
                </el-tooltip>
                <el-tooltip
                  v-if="
                    row.seedingBonus && row.seedingBonus > 0 && getSiteSeedingBonusName(row.site)
                  "
                  :content="getSiteSeedingBonusName(row.site) || '做种积分'"
                  placement="left">
                  <div class="bonus-row seeding">
                    <span class="value">{{ formatNumber(row.seedingBonus) }}</span>
                    <span class="label">{{ getSiteSeedingBonusName(row.site) }}</span>
                  </div>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>

          <!-- 时魔 -->
          <el-table-column
            prop="bonusPerHour"
            label="时魔/h"
            min-width="100"
            sortable
            align="right">
            <template #default="{ row }">
              <span class="bonus-per-hour">{{ formatNumber(row.bonusPerHour ?? 0) }}</span>
            </template>
          </el-table-column>

          <!-- 注册时间 -->
          <el-table-column prop="joinDate" label="入站" min-width="110" sortable align="center">
            <template #default="{ row }">
              <el-tooltip v-if="row.joinDate" :content="formatDate(row.joinDate)" placement="top">
                <span class="join-time">{{ formatJoinDuration(row.joinDate) }}</span>
              </el-tooltip>
              <span v-else class="no-data">-</span>
            </template>
          </el-table-column>

          <!-- 更新时间 -->
          <el-table-column prop="lastUpdate" label="更新" min-width="100" sortable align="center">
            <template #default="{ row }">
              <el-tooltip :content="formatTime(row.lastUpdate)" placement="top">
                <span class="update-time">{{ formatTimeAgo(row.lastUpdate) }}</span>
              </el-tooltip>
            </template>
          </el-table-column>

          <!-- 操作列 -->
          <el-table-column label="操作" width="80" align="center" fixed="right">
            <template #default="{ row }">
              <el-button
                type="primary"
                size="small"
                circle
                :loading="syncingSite === row.site"
                @click="syncSite(row.site)">
                <el-icon><Refresh /></el-icon>
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 移动端卡片视图 -->
      <div v-if="isMobile" class="mobile-cards">
        <div
          v-for="row in aggregatedStats?.perSiteStats || []"
          :key="row.site"
          class="mobile-site-card">
          <!-- 卡片头部：站点信息 -->
          <div class="mobile-card-header">
            <div class="site-info-wrapper">
              <el-badge
                :value="row.unreadMessageCount"
                :hidden="!row.unreadMessageCount || row.unreadMessageCount === 0"
                :max="99"
                type="danger">
                <div class="site-avatar-wrapper" @click.stop="syncSite(row.site)">
                  <SiteAvatar :site-name="row.site" :site-id="row.site" :size="40" />
                  <el-icon v-if="syncingSite === row.site" class="sync-icon is-loading">
                    <Loading />
                  </el-icon>
                </div>
              </el-badge>
              <div class="site-details">
                <span class="site-name">{{ row.site }}</span>
                <span class="user-name">{{ row.username }}</span>
              </div>
            </div>
            <LevelTooltip
              :site-id="row.site"
              :current-level-name="row.levelName || row.rank || '-'"
              :current-level-id="row.levelId" />
          </div>

          <!-- 主要数据：上传下载和分享率 -->
          <div class="mobile-card-main">
            <div class="data-group">
              <div class="data-item upload">
                <el-icon><Top /></el-icon>
                <span class="value">{{ formatBytes(row.uploaded) }}</span>
              </div>
              <div class="data-item download">
                <el-icon><Bottom /></el-icon>
                <span class="value">{{ formatBytes(row.downloaded) }}</span>
              </div>
            </div>
            <div class="ratio-display">
              <el-tag :type="getRatioType(row.ratio)" effect="dark" size="large">
                {{ formatRatio(row.ratio) }}
              </el-tag>
            </div>
          </div>

          <!-- 次要数据网格 -->
          <div class="mobile-card-stats">
            <div class="stat-item">
              <span class="label">
                <el-icon><Star /></el-icon>
                {{ getSiteBonusName(row.site) }}
              </span>
              <span class="value bonus">{{ formatNumber(row.bonus ?? 0) }}</span>
            </div>
            <div class="stat-item">
              <span class="label">
                <el-icon><Timer /></el-icon>
                时魔/h
              </span>
              <span class="value bonus">{{ formatNumber(row.bonusPerHour ?? 0) }}</span>
            </div>
            <div class="stat-item">
              <span class="label">
                <el-icon><Connection /></el-icon>
                做种
              </span>
              <span class="value">
                {{ row.seeding }}
                <template v-if="hasHnR(row)">
                  <el-icon v-if="(row.hnrUnsatisfied ?? 0) > 0" color="#f56c6c">
                    <CircleClose />
                  </el-icon>
                </template>
              </span>
            </div>
            <div class="stat-item">
              <span class="label">
                <el-icon><Upload /></el-icon>
                体积
              </span>
              <span class="value seeding">{{ formatBytes(row.seederSize ?? 0) }}</span>
            </div>
            <div
              v-if="row.seedingBonus && row.seedingBonus > 0 && getSiteSeedingBonusName(row.site)"
              class="stat-item">
              <span class="label">
                <el-icon><Medal /></el-icon>
                做种积分
              </span>
              <span class="value seeding">{{ formatNumber(row.seedingBonus) }}</span>
            </div>
            <div class="stat-item">
              <span class="label">
                <el-icon><Calendar /></el-icon>
                入站
              </span>
              <span class="value">{{ formatJoinDuration(row.joinDate ?? 0) }}</span>
            </div>
          </div>

          <!-- 卡片底部：操作和更新时间 -->
          <div class="mobile-card-footer">
            <span class="update-info">
              <el-icon><Clock /></el-icon>
              {{ formatTimeAgo(row.lastUpdate) }}
            </span>
            <el-button
              type="primary"
              size="small"
              round
              :loading="syncingSite === row.site"
              @click="syncSite(row.site)">
              <el-icon><Refresh /></el-icon>
              同步
            </el-button>
          </div>
        </div>
      </div>

      <!-- 最后更新时间 -->
      <div v-if="aggregatedStats" class="last-update">
        最后更新: {{ formatTime(aggregatedStats.lastUpdate) }}
      </div>
    </div>
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";
@import "@/styles/dashboard.css";

/* Component specific overrides */
.extension-banner {
  margin-bottom: 16px;
}

.extension-banner a {
  color: var(--el-color-primary);
  font-weight: 600;
  text-decoration: none;
}

.extension-banner a:hover {
  text-decoration: underline;
}

.dashboard-card-body {
  padding: var(--pt-space-5);
}

.sync-icon {
  position: absolute;
  right: -2px;
  bottom: -2px;
  background: var(--pt-bg-surface);
  border-radius: 50%;
  font-size: 14px;
  color: var(--pt-color-primary);
  box-shadow: var(--pt-shadow-sm);
}

.no-data {
  color: var(--pt-text-tertiary);
}

/* Seeding cell styles */
.seeding-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.hnr-info {
  display: flex;
  gap: 6px;
}

.hnr-item {
  display: flex;
  align-items: center;
  gap: 2px;
  font-size: 12px;
}

.hnr-item.warning {
  color: var(--pt-color-warning);
}
.hnr-item.danger {
  color: var(--pt-color-danger);
}

.seeding-size {
  color: var(--pt-color-success);
  font-weight: 600;
}

/* Bonus row specialized styles */
.bonus-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
}

.bonus-row.seeding .value,
.bonus-row.seeding .label {
  color: var(--pt-color-success);
}

.bonus-per-hour {
  color: var(--pt-color-warning);
  font-weight: 600;
}

/* Time styles */
.join-time,
.update-time {
  font-size: var(--pt-text-xs);
  color: var(--pt-text-secondary);
}

.last-update {
  padding: var(--pt-space-4) var(--pt-space-5);
  text-align: right;
  font-size: var(--pt-text-xs);
  color: var(--pt-text-tertiary);
  border-top: 1px solid var(--pt-border-color);
}

/* Mobile specific styling not in dashboard.css */
.site-info-wrapper {
  display: flex;
  align-items: center;
  gap: var(--pt-space-3);
}

.mobile-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--pt-space-4);
}

.mobile-card-header .site-details {
  display: flex;
  flex-direction: column;
}

.mobile-card-header .site-name {
  font-weight: 700;
  font-size: var(--pt-text-base);
  color: var(--pt-text-primary);
}

.mobile-card-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--pt-space-3) 0;
  border-top: 1px solid var(--pt-border-color);
  border-bottom: 1px solid var(--pt-border-color);
  margin-bottom: var(--pt-space-2);
}

.data-group {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
}

.data-item {
  display: flex;
  align-items: center;
  gap: var(--pt-space-2);
}

.data-item.upload .value {
  color: var(--pt-color-success);
}
.data-item.download .value {
  color: var(--pt-color-danger);
}

.data-item .value {
  font-size: var(--pt-text-base);
  font-weight: 700;
}

.ratio-display .el-tag {
  font-weight: 700;
  font-size: var(--pt-text-lg);
  padding: 0 var(--pt-space-4);
}

.mobile-card-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: var(--pt-space-2);
  padding: var(--pt-space-3);
  border: 1px solid var(--pt-border-color);
  border-radius: var(--pt-radius-lg);
  background: color-mix(in srgb, var(--pt-bg-secondary) 75%, var(--pt-color-primary-50));
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  min-height: 48px;
}

.stat-item .label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  color: var(--pt-text-secondary);
  text-transform: uppercase;
}

.stat-item .value {
  font-size: var(--pt-text-sm);
  font-weight: 700;
  color: var(--pt-text-primary);
}

.stat-item .value.bonus {
  color: var(--pt-color-warning);
}
.stat-item .value.seeding {
  color: var(--pt-color-success);
}

.mobile-card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: var(--pt-space-3);
  padding-top: var(--pt-space-3);
  border-top: 1px solid var(--pt-border-color);
}

.update-info {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: var(--pt-text-xs);
  color: var(--pt-text-tertiary);
}

html.dark .mobile-card-stats {
  background: color-mix(in srgb, var(--pt-bg-tertiary) 88%, var(--pt-color-primary-900));
}

@media (max-width: 480px) {
  .mobile-card-stats {
    grid-template-columns: repeat(2, 1fr);
  }
}
</style>
