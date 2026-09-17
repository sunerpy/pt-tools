<script setup lang="ts">
import { computed } from "vue";
import { useRuntimeStore } from "../../stores/runtime";
import { useVersionStore } from "../../stores/version";
import VersionChecker from "../VersionChecker.vue";
import PtIcon from "../PtIcon";

/**
 * 底部深色状态条（设计稿 G 的 `statusbar`，高 28）。
 * 板 41 的落位表把两件事放在这里：调度器的启停，和版本/更新入口。
 * 其余格子是背景信息，数字全部来自 runtimeStore 的真实拉取。
 */
const runtimeStore = useRuntimeStore();
const versionStore = useVersionStore();

const schedulerIcon = computed(() => {
  if (runtimeStore.schedulerHint === "running") return "circle-check";
  if (runtimeStore.schedulerHint === "stopped") return "circle-pause";
  return "activity";
});

const schedulerColor = computed(() => {
  if (runtimeStore.schedulerHint === "running") return "var(--pt-ok)";
  if (runtimeStore.schedulerHint === "stopped") return "var(--pt-warn)";
  return "var(--pt-chrome-t2)";
});

/** 与旧页脚同一套算法：取浏览器年份与构建年份的较大值，机器时钟偏早时不显示过去的年份 */
const year = computed(() => {
  const browserYear = new Date().getFullYear();
  const buildTime = versionStore.versionInfo?.build_time;
  if (!buildTime || buildTime === "unknown") return browserYear;
  const buildYear = new Date(buildTime).getFullYear();
  return Number.isFinite(buildYear) ? Math.max(browserYear, buildYear) : browserYear;
});
</script>

<template>
  <footer class="pt-status" aria-label="运行状态">
    <el-popover placement="top-start" trigger="click" :width="200" :offset="10">
      <template #reference>
        <button type="button" class="pt-status__cell pt-status__cell--btn">
          <PtIcon :name="schedulerIcon" :size="13" :style="{ color: schedulerColor }" />
          <span>{{ runtimeStore.schedulerText }}</span>
          <PtIcon name="chevron-up" :size="12" class="pt-status__caret" />
        </button>
      </template>
      <div class="pt-status__menu">
        <el-button
          size="small"
          type="danger"
          plain
          :loading="runtimeStore.stopLoading"
          @click="runtimeStore.stopAll()">
          停止所有任务
        </el-button>
        <el-button
          size="small"
          type="primary"
          plain
          :loading="runtimeStore.startLoading"
          @click="runtimeStore.startAll()">
          启动所有任务
        </el-button>
      </div>
    </el-popover>

    <el-tooltip :content="`每 ${runtimeStore.pollMinutes} 分钟自动刷新`" placement="top">
      <span class="pt-status__cell" :class="{ 'is-stale': runtimeStore.stale }">
        <PtIcon name="refresh-cw" :size="13" />
        <span>最后同步 {{ runtimeStore.lastSyncText }}</span>
      </span>
    </el-tooltip>

    <span class="pt-status__cell" :class="{ 'is-stale': runtimeStore.stale }">
      <PtIcon name="download" :size="13" />
      <span>{{ runtimeStore.downloadText }}</span>
    </span>

    <span class="pt-status__cell" :class="{ 'is-stale': runtimeStore.stale }">
      <PtIcon name="upload" :size="13" />
      <span>{{ runtimeStore.uploadText }}</span>
    </span>

    <el-tooltip content="下载器剩余空间合计" placement="top">
      <span class="pt-status__cell" :class="{ 'is-stale': runtimeStore.stale }">
        <PtIcon name="hard-drive" :size="13" />
        <span>{{ runtimeStore.freeSpaceText }}</span>
      </span>
    </el-tooltip>

    <span class="pt-status__spacer" />

    <span class="pt-status__version">
      <VersionChecker compact />
    </span>

    <span class="pt-status__cell pt-status__copy">
      <a href="https://github.com/sunerpy/pt-tools" target="_blank" rel="noopener">pt-tools</a>
      <span>© {{ year }} · PT 助手</span>
    </span>
  </footer>
</template>
