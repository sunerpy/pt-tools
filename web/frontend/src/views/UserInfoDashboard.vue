<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { userInfoApi, type AggregatedStatsResponse } from '@/api'
import { ElMessage } from 'element-plus'
import SiteAvatar from '@/components/SiteAvatar.vue'
import LevelTooltip from '@/components/LevelTooltip.vue'
import { useSiteLevelsStore } from '@/stores/siteLevels'
import {
  formatBytes,
  formatNumber,
  formatRatio,
  formatTime,
  formatDate,
  formatTimeAgo,
  formatJoinDuration,
  getRatioType,
  getSiteBonusName,
  getSiteSeedingBonusName
} from '@/utils/format'

const siteLevelsStore = useSiteLevelsStore()

const loading = ref(false)
const syncing = ref(false)
const syncingSite = ref<string | null>(null)
const aggregatedStats = ref<AggregatedStatsResponse | null>(null)
const isMobile = ref(window.innerWidth < 768)

// 定时刷新相关
const REFRESH_INTERVAL = 5 * 60 * 1000 // 5分钟
let refreshTimer: ReturnType<typeof setInterval> | null = null
const autoRefreshEnabled = ref(true)

// 监听窗口大小变化
function handleResize() {
  isMobile.value = window.innerWidth < 768
}

// 计算统计卡片数据
const statsCards = computed(() => {
  if (!aggregatedStats.value) return []
  const stats = aggregatedStats.value
  const cards = [
    {
      title: '总上传量',
      value: formatBytes(stats.totalUploaded),
      icon: 'Upload',
      color: '#67c23a'
    },
    {
      title: '总下载量',
      value: formatBytes(stats.totalDownloaded),
      icon: 'Download',
      color: '#409eff'
    },
    {
      title: '平均分享率',
      value: formatRatio(stats.averageRatio),
      icon: 'DataAnalysis',
      color: stats.averageRatio >= 1 ? '#67c23a' : '#e6a23c'
    },
    {
      title: '做种数',
      value: stats.totalSeeding.toString(),
      icon: 'Connection',
      color: '#67c23a'
    },
    {
      title: '下载中',
      value: stats.totalLeeching.toString(),
      icon: 'Loading',
      color: '#409eff'
    },
    {
      title: '总魔力值',
      value: formatNumber(stats.totalBonus),
      icon: 'Star',
      color: '#e6a23c'
    },
    {
      title: '总时魔/h',
      value: formatNumber(stats.totalBonusPerHour ?? 0),
      icon: 'Timer',
      color: '#e6a23c'
    }
  ]

  // 只有当存在做种积分时才显示
  if (stats.totalSeedingBonus && stats.totalSeedingBonus > 0) {
    cards.push({
      title: '总做种积分',
      value: formatNumber(stats.totalSeedingBonus),
      icon: 'Medal',
      color: '#67c23a'
    })
  }

  cards.push(
    {
      title: '做种总量',
      value: formatBytes(stats.totalSeederSize ?? 0),
      icon: 'Upload',
      color: '#67c23a'
    },
    {
      title: '站点数量',
      value: stats.siteCount.toString(),
      icon: 'OfficeBuilding',
      color: '#909399'
    }
  )

  return cards
})

// 加载数据
async function loadData() {
  loading.value = true
  try {
    aggregatedStats.value = await userInfoApi.getAggregated()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

// 同步所有站点
async function syncAll() {
  syncing.value = true
  try {
    const result = await userInfoApi.syncAll()
    if (result.failed && result.failed.length > 0) {
      ElMessage.warning(`同步完成: ${result.success.length} 成功, ${result.failed.length} 失败`)
    } else {
      ElMessage.success(`同步完成: ${result.success.length} 个站点`)
    }
    await loadData()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '同步失败')
  } finally {
    syncing.value = false
  }
}

// 同步单个站点
async function syncSite(siteId: string) {
  syncingSite.value = siteId
  try {
    await userInfoApi.syncSite(siteId)
    ElMessage.success(`站点 ${siteId} 同步成功`)
    await loadData()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '同步失败')
  } finally {
    syncingSite.value = null
  }
}

// 清除缓存
async function clearCache() {
  try {
    await userInfoApi.clearCache()
    ElMessage.success('缓存已清除')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '清除缓存失败')
  }
}

// 启动定时刷新
function startAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
  refreshTimer = setInterval(() => {
    if (!loading.value && !syncing.value) {
      loadData()
    }
  }, REFRESH_INTERVAL)
}

// 停止定时刷新
function stopAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
}

// 切换自动刷新
function toggleAutoRefresh() {
  autoRefreshEnabled.value = !autoRefreshEnabled.value
  if (autoRefreshEnabled.value) {
    startAutoRefresh()
    ElMessage.success('已开启自动刷新')
  } else {
    stopAutoRefresh()
    ElMessage.info('已关闭自动刷新')
  }
}

// 检查是否有 H&R 数据
function hasHnR(row: any): boolean {
  return (
    (row.hnrUnsatisfied && row.hnrUnsatisfied > 0) || (row.hnrPreWarning && row.hnrPreWarning > 0)
  )
}

onMounted(() => {
  loadData()
  // 预加载所有站点的等级信息
  siteLevelsStore.loadAll()
  if (autoRefreshEnabled.value) {
    startAutoRefresh()
  }
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  stopAutoRefresh()
  window.removeEventListener('resize', handleResize)
})
</script>

<template>
  <div class="page-container">
    <!-- 统计卡片 -->
    <el-row :gutter="16" class="stats-row">
      <el-col v-for="card in statsCards" :key="card.title" :xs="12" :sm="8" :md="6" :lg="4" :xl="3">
        <el-card shadow="hover" class="stat-card">
          <div class="stat-content">
            <div
              class="stat-icon"
              :style="{ backgroundColor: card.color + '20', color: card.color }"
            >
              <el-icon :size="24"><component :is="card.icon" /></el-icon>
            </div>
            <div class="stat-info">
              <div class="stat-value">{{ card.value }}</div>
              <div class="stat-title">{{ card.title }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 站点详情表格 -->
    <el-card v-loading="loading" shadow="never" class="table-card">
      <template #header>
        <div class="card-header">
          <span>站点统计详情</span>
          <div class="header-actions">
            <el-tooltip
              :content="autoRefreshEnabled ? '点击关闭自动刷新 (5分钟)' : '点击开启自动刷新'"
              placement="top"
            >
              <el-button
                size="small"
                :type="autoRefreshEnabled ? 'success' : 'info'"
                @click="toggleAutoRefresh"
              >
                <el-icon><Timer /></el-icon>
                {{ autoRefreshEnabled ? '自动刷新中' : '自动刷新已关闭' }}
              </el-button>
            </el-tooltip>
            <el-button size="small" @click="clearCache">清除缓存</el-button>
            <el-button type="primary" size="small" :loading="syncing" @click="syncAll">
              <el-icon><Refresh /></el-icon>
              同步全部
            </el-button>
          </div>
        </div>
      </template>

      <!-- 桌面端表格视图 -->
      <el-table
        v-if="!isMobile"
        :data="aggregatedStats?.perSiteStats || []"
        style="width: 100%"
        :default-sort="{ prop: 'uploaded', order: 'descending' }"
        stripe
        highlight-current-row
      >
        <!-- 站点列：带消息徽章和悬停效果 -->
        <el-table-column prop="site" label="站点" min-width="140" sortable fixed="left">
          <template #default="{ row }">
            <div class="site-cell">
              <el-badge
                :value="row.unreadMessageCount"
                :hidden="!row.unreadMessageCount || row.unreadMessageCount === 0"
                :max="99"
                type="danger"
              >
                <div class="site-avatar-wrapper" @click.stop="syncSite(row.site)">
                  <SiteAvatar :site-name="row.site" :site-id="row.site" :size="32" />
                  <el-icon v-if="syncingSite === row.site" class="sync-icon is-loading">
                    <Loading />
                  </el-icon>
                </div>
              </el-badge>
              <div class="site-info">
                <span class="site-name">{{ row.site }}</span>
                <span class="username">{{ row.username }}</span>
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
              :current-level-id="row.levelId"
            />
          </template>
        </el-table-column>

        <!-- 上传/下载量：双行布局带图标 -->
        <el-table-column prop="uploaded" label="数据量" min-width="130" sortable align="right">
          <template #default="{ row }">
            <div class="data-cell">
              <div class="data-row upload">
                <el-icon class="data-icon" color="#67c23a"><Top /></el-icon>
                <span class="data-value">{{ formatBytes(row.uploaded) }}</span>
              </div>
              <div class="data-row download">
                <el-icon class="data-icon" color="#409eff"><Bottom /></el-icon>
                <span class="data-value">{{ formatBytes(row.downloaded) }}</span>
              </div>
            </div>
          </template>
        </el-table-column>

        <!-- 真实数据（如果不同） -->
        <el-table-column
          prop="trueUploaded"
          label="真实数据"
          min-width="130"
          sortable
          align="right"
        >
          <template #default="{ row }">
            <div v-if="row.trueUploaded && row.trueUploaded !== row.uploaded" class="data-cell">
              <div class="data-row upload">
                <el-icon class="data-icon" color="#67c23a"><Top /></el-icon>
                <span class="data-value">{{ formatBytes(row.trueUploaded) }}</span>
              </div>
              <div class="data-row download">
                <el-icon class="data-icon" color="#409eff"><Bottom /></el-icon>
                <span class="data-value">{{ formatBytes(row.trueDownloaded ?? 0) }}</span>
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
        <el-table-column prop="seeding" label="做种" min-width="100" sortable align="center">
          <template #default="{ row }">
            <div class="seeding-cell">
              <div class="seeding-count">
                <el-tag type="success" size="small" effect="plain">{{ row.seeding }}</el-tag>
              </div>
              <div v-if="hasHnR(row)" class="hnr-info">
                <el-tooltip
                  v-if="row.hnrPreWarning > 0"
                  :content="`H&R 预警: ${row.hnrPreWarning}`"
                >
                  <span class="hnr-item warning">
                    <el-icon><Warning /></el-icon>
                    <span>{{ row.hnrPreWarning }}</span>
                  </span>
                </el-tooltip>
                <el-tooltip
                  v-if="row.hnrUnsatisfied > 0"
                  :content="`H&R 未满足: ${row.hnrUnsatisfied}`"
                >
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
        <el-table-column prop="seederSize" label="做种体积" min-width="100" sortable align="right">
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
                  <el-icon class="bonus-icon" color="#e6a23c"><Star /></el-icon>
                  <span class="bonus-value">{{ formatNumber(row.bonus ?? 0) }}</span>
                  <span class="bonus-unit">{{ getSiteBonusName(row.site) }}</span>
                </div>
              </el-tooltip>
              <el-tooltip
                v-if="row.seedingBonus && row.seedingBonus > 0 && getSiteSeedingBonusName(row.site)"
                :content="getSiteSeedingBonusName(row.site) || '做种积分'"
                placement="left"
              >
                <div class="bonus-row seeding-bonus">
                  <el-icon class="bonus-icon" color="#67c23a"><Medal /></el-icon>
                  <span class="bonus-value">{{ formatNumber(row.seedingBonus) }}</span>
                  <span class="bonus-unit seeding">{{ getSiteSeedingBonusName(row.site) }}</span>
                </div>
              </el-tooltip>
            </div>
          </template>
        </el-table-column>

        <!-- 时魔 -->
        <el-table-column prop="bonusPerHour" label="时魔/h" min-width="90" sortable align="right">
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
              @click="syncSite(row.site)"
            >
              <el-icon><Refresh /></el-icon>
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 移动端卡片视图 -->
      <div v-else class="mobile-cards">
        <el-card
          v-for="row in aggregatedStats?.perSiteStats || []"
          :key="row.site"
          class="mobile-site-card"
          shadow="hover"
        >
          <!-- 卡片头部：站点信息 -->
          <div class="mobile-card-header">
            <div class="site-info">
              <el-badge
                :value="row.unreadMessageCount"
                :hidden="!row.unreadMessageCount || row.unreadMessageCount === 0"
                :max="99"
                type="danger"
              >
                <div class="site-avatar-wrapper" @click.stop="syncSite(row.site)">
                  <SiteAvatar :site-name="row.site" :site-id="row.site" :size="40" />
                  <el-icon v-if="syncingSite === row.site" class="sync-icon is-loading">
                    <Loading />
                  </el-icon>
                </div>
              </el-badge>
              <div class="site-details">
                <span class="site-name">{{ row.site }}</span>
                <span class="username">{{ row.username }}</span>
              </div>
            </div>
            <LevelTooltip
              :site-id="row.site"
              :current-level-name="row.levelName || row.rank || '-'"
              :current-level-id="row.levelId"
            />
          </div>

          <!-- 主要数据：上传下载和分享率 -->
          <div class="mobile-card-main">
            <div class="data-group">
              <div class="data-item upload">
                <el-icon color="#67c23a"><Top /></el-icon>
                <span class="value">{{ formatBytes(row.uploaded) }}</span>
              </div>
              <div class="data-item download">
                <el-icon color="#409eff"><Bottom /></el-icon>
                <span class="value">{{ formatBytes(row.downloaded) }}</span>
              </div>
            </div>
            <div class="ratio-display">
              <el-tag :type="getRatioType(row.ratio)" effect="dark">
                {{ formatRatio(row.ratio) }}
              </el-tag>
            </div>
          </div>

          <!-- 次要数据网格 -->
          <div class="mobile-card-stats">
            <div class="stat-item">
              <span class="label">
                <el-icon color="#e6a23c"><Star /></el-icon>
                {{ getSiteBonusName(row.site) }}
              </span>
              <span class="value bonus">{{ formatNumber(row.bonus ?? 0) }}</span>
            </div>
            <div class="stat-item">
              <span class="label">
                <el-icon color="#e6a23c"><Timer /></el-icon>
                时魔/h
              </span>
              <span class="value bonus">{{ formatNumber(row.bonusPerHour ?? 0) }}</span>
            </div>
            <div class="stat-item">
              <span class="label">
                <el-icon color="#67c23a"><Connection /></el-icon>
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
                <el-icon color="#67c23a"><Upload /></el-icon>
                做种体积
              </span>
              <span class="value seeding">{{ formatBytes(row.seederSize ?? 0) }}</span>
            </div>
            <div
              v-if="row.seedingBonus && row.seedingBonus > 0 && getSiteSeedingBonusName(row.site)"
              class="stat-item"
            >
              <span class="label">
                <el-icon color="#67c23a"><Medal /></el-icon>
                {{ getSiteSeedingBonusName(row.site) }}
              </span>
              <span class="value seeding">{{ formatNumber(row.seedingBonus) }}</span>
            </div>
            <div class="stat-item">
              <span class="label">
                <el-icon color="#909399"><Calendar /></el-icon>
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
              @click="syncSite(row.site)"
            >
              <el-icon><Refresh /></el-icon>
              同步
            </el-button>
          </div>
        </el-card>
      </div>

      <!-- 最后更新时间 -->
      <div v-if="aggregatedStats" class="last-update">
        最后更新: {{ formatTime(aggregatedStats.lastUpdate) }}
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.page-container {
  width: 100%;
}

.stats-row {
  margin-bottom: 20px;
}

.stat-card {
  margin-bottom: 16px;
}

.stat-content {
  display: flex;
  align-items: center;
  gap: 12px;
}

.stat-icon {
  width: 48px;
  height: 48px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.stat-info {
  flex: 1;
}

.stat-value {
  font-size: 20px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.stat-title {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}

.table-card {
  margin-top: 16px;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 16px;
  font-weight: 600;
  flex-wrap: wrap;
  gap: 8px;
}

.header-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

/* 站点单元格样式 */
.site-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 0;
}

/* 确保角标能正常显示 */
.site-cell :deep(.el-badge) {
  display: inline-flex;
}

.site-cell :deep(.el-badge__content) {
  top: 0;
  right: 14px;
  transform: translateY(-50%) translateX(100%);
}

.site-avatar-wrapper {
  position: relative;
  cursor: pointer;
  border-radius: 50%;
  padding: 4px;
  transition: all 0.2s ease;
}

.site-avatar-wrapper:hover {
  background: rgba(64, 158, 255, 0.1);
  transform: scale(1.05);
}

.sync-icon {
  position: absolute;
  right: -2px;
  bottom: -2px;
  background: white;
  border-radius: 50%;
  font-size: 14px;
  color: var(--el-color-primary);
}

.site-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.site-name {
  font-weight: 600;
  font-size: 14px;
  color: var(--el-text-color-primary);
}

.username {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

/* 数据单元格样式 */
.data-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.data-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
}

.data-icon {
  font-size: 14px;
}

.data-value {
  font-size: 13px;
  font-weight: 500;
}

.data-row.upload .data-value {
  color: #67c23a;
}

.data-row.download .data-value {
  color: #409eff;
}

.no-data {
  color: var(--el-text-color-placeholder);
}

/* 做种单元格样式 */
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
  color: #e6a23c;
}

.hnr-item.danger {
  color: #f56c6c;
}

.seeding-size {
  color: #67c23a;
  font-weight: 500;
}

/* 积分单元格样式 */
.bonus-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.bonus-row {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
}

.bonus-icon {
  font-size: 14px;
}

.bonus-value {
  font-size: 13px;
  font-weight: 500;
  color: #e6a23c;
}

.bonus-unit {
  font-size: 11px;
  color: var(--el-text-color-secondary);
  margin-left: 2px;
}

.bonus-unit.seeding {
  color: #67c23a;
}

.bonus-row.seeding-bonus .bonus-value {
  color: #67c23a;
}

.bonus-per-hour {
  color: #e6a23c;
  font-weight: 500;
}

/* 时间样式 */
.join-time,
.update-time {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  cursor: default;
}

.last-update {
  margin-top: 16px;
  text-align: right;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

/* 移动端卡片样式 */
.mobile-cards {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.mobile-site-card {
  border-radius: 12px;
  overflow: hidden;
}

.mobile-site-card :deep(.el-card__body) {
  padding: 16px;
}

.mobile-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 16px;
}

.mobile-card-header .site-info {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-direction: row;
}

.mobile-card-header .site-details {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.mobile-card-header .site-name {
  font-weight: 600;
  font-size: 16px;
}

.mobile-card-header .username {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

/* 主要数据区域 */
.mobile-card-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 0;
  border-top: 1px solid var(--el-border-color-lighter);
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.data-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.data-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.data-item .value {
  font-size: 15px;
  font-weight: 600;
}

.data-item.upload .value {
  color: #67c23a;
}

.data-item.download .value {
  color: #409eff;
}

.ratio-display {
  display: flex;
  align-items: center;
}

.ratio-display .el-tag {
  font-size: 16px;
  padding: 8px 16px;
  height: auto;
}

/* 次要数据网格 */
.mobile-card-stats {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
  padding: 12px 0;
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.stat-item .label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--el-text-color-secondary);
}

.stat-item .value {
  font-size: 14px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 4px;
}

.stat-item .value.bonus {
  color: #e6a23c;
}

.stat-item .value.seeding {
  color: #67c23a;
}

/* 卡片底部 */
.mobile-card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--el-border-color-lighter);
}

.update-info {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

/* 响应式调整 */
@media (max-width: 768px) {
  .card-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .header-actions {
    width: 100%;
    justify-content: flex-end;
  }

  .stat-card {
    margin-bottom: 8px;
  }

  .stat-content {
    gap: 8px;
  }

  .stat-icon {
    width: 40px;
    height: 40px;
  }

  .stat-value {
    font-size: 16px;
  }

  .mobile-card-stats {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 480px) {
  .mobile-card-stats {
    grid-template-columns: repeat(2, 1fr);
  }

  .stat-item .value {
    font-size: 13px;
  }
}
</style>
