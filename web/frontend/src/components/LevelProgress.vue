<script setup lang="ts">
import { computed } from 'vue'
import type { SiteLevelRequirement } from '@/api'
import {
  parseISODuration,
  parseISODurationToDays,
  calculateDaysSinceJoin,
  formatNumber
} from '@/utils/format'

const props = defineProps<{
  currentLevelId?: number
  currentLevelName?: string
  nextLevel?: SiteLevelRequirement | null
  unmetRequirements?: Record<string, unknown>
  progressPercent?: number
  joinDate?: number // Unix timestamp in seconds
  uploaded?: number
  downloaded?: number
  ratio?: number
  bonus?: number
  seedingBonus?: number
  uploads?: number
}>()

// 是否已达到最高等级
const isMaxLevel = computed(() => {
  return !props.nextLevel
})

// 计算进度百分比
const progress = computed(() => {
  if (isMaxLevel.value) return 100
  if (props.progressPercent !== undefined) return Math.min(100, Math.max(0, props.progressPercent))

  // 如果没有提供进度，尝试计算
  if (!props.nextLevel) return 0

  let totalProgress = 0
  let requirementCount = 0

  // 注册时间进度
  if (props.nextLevel.interval && props.joinDate) {
    const requiredDays = parseISODurationToDays(props.nextLevel.interval)
    const actualDays = calculateDaysSinceJoin(props.joinDate)
    if (requiredDays > 0) {
      totalProgress += Math.min(100, (actualDays / requiredDays) * 100)
      requirementCount++
    }
  }

  // 下载量进度
  if (props.nextLevel.downloaded && props.downloaded !== undefined) {
    const required = parseSizeToBytes(props.nextLevel.downloaded)
    if (required > 0) {
      totalProgress += Math.min(100, (props.downloaded / required) * 100)
      requirementCount++
    }
  }

  // 分享率进度
  if (props.nextLevel.ratio && props.ratio !== undefined) {
    totalProgress += Math.min(100, (props.ratio / props.nextLevel.ratio) * 100)
    requirementCount++
  }

  // 魔力进度
  if (props.nextLevel.bonus && props.bonus !== undefined) {
    totalProgress += Math.min(100, (props.bonus / props.nextLevel.bonus) * 100)
    requirementCount++
  }

  if (requirementCount === 0) return 0
  return Math.round(totalProgress / requirementCount)
})

// 进度条颜色
const progressColor = computed(() => {
  if (isMaxLevel.value) return '#67c23a'
  if (progress.value >= 80) return '#67c23a'
  if (progress.value >= 50) return '#e6a23c'
  return '#409eff'
})

// 未满足的要求列表
const unmetList = computed(() => {
  const list: string[] = []

  if (!props.nextLevel) return list

  // 注册时间
  if (props.nextLevel.interval && props.joinDate) {
    const requiredDays = parseISODurationToDays(props.nextLevel.interval)
    const actualDays = calculateDaysSinceJoin(props.joinDate)
    if (actualDays < requiredDays) {
      list.push(`注册时间: ${actualDays}天 / ${parseISODuration(props.nextLevel.interval)}`)
    }
  }

  // 下载量
  if (props.nextLevel.downloaded && props.downloaded !== undefined) {
    const required = parseSizeToBytes(props.nextLevel.downloaded)
    if (props.downloaded < required) {
      list.push(`下载量: ${formatBytes(props.downloaded)} / ${props.nextLevel.downloaded}`)
    }
  }

  // 分享率
  if (props.nextLevel.ratio && props.ratio !== undefined) {
    if (props.ratio < props.nextLevel.ratio) {
      list.push(`分享率: ${props.ratio.toFixed(2)} / ${props.nextLevel.ratio}`)
    }
  }

  // 魔力
  if (props.nextLevel.bonus && props.bonus !== undefined) {
    if (props.bonus < props.nextLevel.bonus) {
      list.push(`魔力: ${formatNumber(props.bonus)} / ${formatNumber(props.nextLevel.bonus)}`)
    }
  }

  // 做种积分
  if (props.nextLevel.seedingBonus && props.seedingBonus !== undefined) {
    if (props.seedingBonus < props.nextLevel.seedingBonus) {
      list.push(
        `做种积分: ${formatNumber(props.seedingBonus)} / ${formatNumber(props.nextLevel.seedingBonus)}`
      )
    }
  }

  return list
})

// 解析大小字符串为字节数
function parseSizeToBytes(sizeStr: string | undefined): number {
  if (!sizeStr) return 0
  const match = sizeStr.match(/^([\d.]+)\s*(B|KB|MB|GB|TB|PB)?$/i)
  if (!match || !match[1]) return 0

  const num = parseFloat(match[1])
  const unit = match[2] ? match[2].toUpperCase() : 'B'

  const multipliers: Record<string, number> = {
    B: 1,
    KB: 1024,
    MB: 1024 ** 2,
    GB: 1024 ** 3,
    TB: 1024 ** 4,
    PB: 1024 ** 5
  }

  return num * (multipliers[unit] || 1)
}

// 格式化字节数
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}
</script>

<template>
  <div class="level-progress">
    <template v-if="isMaxLevel">
      <el-tag type="success" size="small" effect="dark">
        <el-icon><Trophy /></el-icon>
        最高等级
      </el-tag>
    </template>

    <template v-else>
      <el-tooltip :disabled="unmetList.length === 0" placement="top">
        <template #content>
          <div class="unmet-tooltip">
            <div class="tooltip-title">升级到 {{ nextLevel?.name }} 还需:</div>
            <div v-for="(item, idx) in unmetList" :key="idx" class="unmet-item">
              {{ item }}
            </div>
          </div>
        </template>

        <div class="progress-wrapper">
          <el-progress
            :percentage="progress"
            :color="progressColor"
            :stroke-width="8"
            :show-text="false"
          />
          <span class="progress-text">
            {{ progress }}%
            <span class="next-level" v-if="nextLevel">→ {{ nextLevel.name }}</span>
          </span>
        </div>
      </el-tooltip>
    </template>
  </div>
</template>

<style scoped>
.level-progress {
  display: inline-flex;
  align-items: center;
  min-width: 120px;
}

.progress-wrapper {
  display: flex;
  flex-direction: column;
  gap: 2px;
  width: 100%;
  cursor: pointer;
}

.progress-wrapper :deep(.el-progress) {
  width: 100%;
}

.progress-text {
  font-size: 11px;
  color: var(--el-text-color-secondary);
  display: flex;
  align-items: center;
  gap: 4px;
}

.next-level {
  color: var(--el-color-primary);
  font-weight: 500;
}

.unmet-tooltip {
  max-width: 250px;
}

.tooltip-title {
  font-weight: 600;
  margin-bottom: 6px;
  color: var(--el-color-warning);
}

.unmet-item {
  font-size: 12px;
  padding: 2px 0;
  color: var(--el-text-color-regular);
}

.unmet-item::before {
  content: '•';
  margin-right: 6px;
  color: var(--el-color-danger);
}
</style>
