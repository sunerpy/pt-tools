<script setup lang="ts">
import type { SiteLevelRequirement } from "@/api";
import { useSiteLevelsStore } from "@/stores/siteLevels";
import { formatNumber, parseISODuration } from "@/utils/format";
import { computed, onMounted } from "vue";

const props = defineProps<{
  siteId: string;
  currentLevelName: string;
  currentLevelId?: number;
}>();

const siteLevelsStore = useSiteLevelsStore();

// 使用store中的数据
const levels = computed(() => siteLevelsStore.getLevels(props.siteId));
const loading = computed(() => siteLevelsStore.loading);
const error = computed(() => siteLevelsStore.error);

// 组件挂载时确保等级数据已加载
onMounted(() => {
  if (!siteLevelsStore.loaded) {
    siteLevelsStore.loadAll();
  }
});

// 检查是否是当前等级
function isCurrentLevel(level: SiteLevelRequirement): boolean {
  // 优先通过 ID 匹配
  if (props.currentLevelId && props.currentLevelId > 0 && level.id === props.currentLevelId) {
    return true;
  }
  // 通过名称匹配
  if (props.currentLevelName && props.currentLevelName !== "-") {
    // 标准化名称：去除空格、转小写
    const normalize = (s: string) => s.toLowerCase().replace(/\s+/g, "").trim();
    const currentName = normalize(props.currentLevelName);
    const levelName = normalize(level.name);

    // 精确匹配
    if (currentName === levelName) {
      return true;
    }

    // 检查别名
    if (level.nameAka) {
      for (const aka of level.nameAka) {
        if (currentName === normalize(aka)) {
          return true;
        }
      }
    }
  }
  return false;
}

// 格式化等级要求
function formatRequirement(level: SiteLevelRequirement): string[] {
  const reqs: string[] = [];

  if (level.interval) {
    reqs.push(`注册 ${parseISODuration(level.interval)}`);
  }
  if (level.downloaded) {
    reqs.push(`下载 ${level.downloaded}`);
  }
  if (level.uploaded) {
    reqs.push(`上传 ${level.uploaded}`);
  }
  if (level.ratio && level.ratio > 0) {
    reqs.push(`分享率 ${level.ratio}`);
  }
  if (level.bonus && level.bonus > 0) {
    reqs.push(`魔力 ${formatNumber(level.bonus)}`);
  }
  if (level.seedingBonus && level.seedingBonus > 0) {
    reqs.push(`做种积分 ${formatNumber(level.seedingBonus)}`);
  }
  if (level.uploads && level.uploads > 0) {
    reqs.push(`发布 ${level.uploads} 个`);
  }
  if (level.seeding && level.seeding > 0) {
    reqs.push(`做种 ${level.seeding} 个`);
  }
  if (level.seedingSize) {
    reqs.push(`做种体积 ${level.seedingSize}`);
  }

  return reqs;
}

// 格式化替代要求
function formatAlternatives(level: SiteLevelRequirement): string[][] {
  if (!level.alternative || level.alternative.length === 0) {
    return [];
  }

  return level.alternative.map((alt) => {
    const reqs: string[] = [];
    if (alt.seedingBonus && alt.seedingBonus > 0) {
      reqs.push(`做种积分 ${formatNumber(alt.seedingBonus)}`);
    }
    if (alt.uploads && alt.uploads > 0) {
      reqs.push(`发布 ${alt.uploads} 个`);
    }
    if (alt.bonus && alt.bonus > 0) {
      reqs.push(`魔力 ${formatNumber(alt.bonus)}`);
    }
    if (alt.downloaded) {
      reqs.push(`下载 ${alt.downloaded}`);
    }
    if (alt.ratio && alt.ratio > 0) {
      reqs.push(`分享率 ${alt.ratio}`);
    }
    return reqs;
  });
}

// 获取等级组类型标签
function getGroupTypeLabel(groupType?: string): string {
  switch (groupType) {
    case "vip":
      return "VIP";
    case "manager":
      return "管理";
    default:
      return "";
  }
}

// 过滤只显示普通用户等级
const userLevels = computed(() => {
  return levels.value.filter((l) => !l.groupType || l.groupType === "user");
});

// 特殊等级（VIP、管理等）
const specialLevels = computed(() => {
  return levels.value.filter((l) => l.groupType && l.groupType !== "user");
});

// 重新加载等级数据
function reloadLevels() {
  siteLevelsStore.reset();
  siteLevelsStore.loadAll();
}
</script>

<template>
  <el-popover placement="bottom" :width="420" trigger="hover" popper-class="level-tooltip-popper">
    <template #reference>
      <el-tag size="small" type="info" class="level-tag">
        {{ currentLevelName || "-" }}
      </el-tag>
    </template>

    <div class="level-tooltip-content">
      <div v-if="loading" class="loading-state">
        <el-icon class="is-loading"><Loading /></el-icon>
        <span>加载中...</span>
      </div>

      <div v-else-if="error" class="error-state">
        <el-icon><WarningFilled /></el-icon>
        <span>{{ error }}</span>
        <el-button size="small" @click="reloadLevels">重试</el-button>
      </div>

      <div v-else-if="levels.length === 0" class="empty-state">
        <span>暂无等级信息</span>
      </div>

      <div v-else class="levels-list">
        <div class="levels-header">
          <span>用户等级</span>
        </div>

        <!-- 普通用户等级 -->
        <div
          v-for="level in userLevels"
          :key="level.id"
          class="level-item"
          :class="{ 'is-current': isCurrentLevel(level) }">
          <div class="level-header">
            <span class="level-name">
              {{ level.name }}
              <el-tag v-if="isCurrentLevel(level)" size="small" type="success" effect="dark">
                当前
              </el-tag>
            </span>
            <span class="level-id">Lv.{{ level.id }}</span>
          </div>

          <div class="level-requirements" v-if="formatRequirement(level).length > 0">
            <span
              v-for="(req, idx) in formatRequirement(level)"
              :key="idx"
              class="requirement-item">
              {{ req }}
            </span>
          </div>

          <!-- 替代要求 -->
          <div v-if="formatAlternatives(level).length > 0" class="level-alternatives">
            <div
              v-for="(altReqs, altIdx) in formatAlternatives(level)"
              :key="altIdx"
              class="alternative-group">
              <span class="or-separator" v-if="altIdx > 0 || formatRequirement(level).length > 0">
                或
              </span>
              <span v-for="(req, reqIdx) in altReqs" :key="reqIdx" class="requirement-item alt">
                {{ req }}
              </span>
            </div>
          </div>

          <!-- 特权描述 -->
          <div v-if="level.privilege" class="level-privilege">
            <el-icon><Star /></el-icon>
            <span>{{ level.privilege }}</span>
          </div>
        </div>

        <!-- 特殊等级 -->
        <template v-if="specialLevels.length > 0">
          <div class="levels-header special">
            <span>特殊等级</span>
          </div>
          <div
            v-for="level in specialLevels"
            :key="level.id"
            class="level-item special"
            :class="{ 'is-current': isCurrentLevel(level) }">
            <div class="level-header">
              <span class="level-name">
                {{ level.name }}
                <el-tag size="small" :type="level.groupType === 'vip' ? 'warning' : 'danger'">
                  {{ getGroupTypeLabel(level.groupType) }}
                </el-tag>
                <el-tag v-if="isCurrentLevel(level)" size="small" type="success" effect="dark">
                  当前
                </el-tag>
              </span>
            </div>
            <div v-if="level.privilege" class="level-privilege">
              <el-icon><Star /></el-icon>
              <span>{{ level.privilege }}</span>
            </div>
          </div>
        </template>
      </div>
    </div>
  </el-popover>
</template>
