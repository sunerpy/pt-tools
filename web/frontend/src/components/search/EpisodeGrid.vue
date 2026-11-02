<script setup lang="ts">
import { computed } from "vue";

import type { MediaSearchEpisode } from "@/api";
import { EPISODE_STATE_LABELS, epCode, epList } from "@/utils/entrySearch";

/**
 * 每集格子（画板 06 左栏、09 条目卡）：一格一集，颜色是这一集现在的情况——已入库 ok、下载中 info、
 * 等版本主色、缺 warn、还没播 hover 底加 t3 字。点一格只看包含这一集的结果，再点一次取消。
 * 手机上超过 24 集时只列没入库的集，前面写「E01–E20 已入库」。
 */
const props = defineProps<{
  episodes: MediaSearchEpisode[];
  selected?: number;
  mobile?: boolean;
}>();

const emit = defineEmits<{ pick: [episode: number] }>();

const compact = computed(() => props.mobile && props.episodes.length > 24);
const cells = computed(() =>
  compact.value ? props.episodes.filter((e) => e.state !== "library") : props.episodes,
);
const inLibrary = computed(() =>
  compact.value
    ? epList(props.episodes.filter((e) => e.state === "library").map((e) => e.episode))
    : "",
);

function tip(e: MediaSearchEpisode): string {
  const parts = [`${epCode(e.episode)} ${EPISODE_STATE_LABELS[e.state]}`];
  if (e.site) parts.push(e.site);
  if (e.name) parts.push(e.name);
  if (e.air_date) parts.push(`${e.air_date} 播出`);
  return parts.join(" · ");
}
</script>

<template>
  <div class="es-eps" :class="{ 'is-mobile': mobile }" data-testid="es-episodes">
    <p v-if="inLibrary" class="es-eps__lib">{{ inLibrary }} 已入库</p>
    <div class="es-eps__grid">
      <button
        v-for="e in cells"
        :key="e.episode"
        type="button"
        class="es-eps__cell"
        :class="[`is-${e.state}`, { 'is-on': e.episode === selected }]"
        :title="tip(e)"
        :aria-label="tip(e)"
        :aria-pressed="e.episode === selected"
        :data-testid="`es-ep-${e.episode}`"
        @click="emit('pick', e.episode)">
        {{ epCode(e.episode) }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.es-eps__lib {
  margin: 0 0 6px;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 桌面：每行 6 格，格 34×24 r4、列距 38、行距 30；手机：38×22、列距 43、行距 27 */
.es-eps__grid {
  display: grid;
  grid-template-columns: repeat(6, 34px);
  gap: 6px 4px;
}

.es-eps.is-mobile .es-eps__grid {
  grid-template-columns: repeat(6, 38px);
  gap: 5px;
}

.es-eps__cell {
  height: 24px;
  padding: 0;
  font: inherit;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  color: var(--pt-on-p);
  cursor: pointer;
  background: var(--pt-t3);
  border: 0;
  border-radius: var(--pt-r-sm);
}

.es-eps.is-mobile .es-eps__cell {
  height: 22px;
}

.es-eps__cell.is-library {
  background: var(--pt-ok);
}

.es-eps__cell.is-downloading {
  background: var(--pt-info);
}

.es-eps__cell.is-waiting_version {
  background: var(--pt-p);
}

.es-eps__cell.is-missing {
  background: var(--pt-warn);
}

.es-eps__cell.is-upcoming {
  color: var(--pt-t3);
  background: var(--pt-hover);
}

/* 选中的一集：外面一圈 t1 */
.es-eps__cell.is-on {
  box-shadow:
    0 0 0 2px var(--pt-surface),
    0 0 0 3px var(--pt-t1);
}
</style>
