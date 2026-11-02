<script setup lang="ts">
import { computed } from "vue";

import type { MediaSearchResult, QualityProfile } from "@/api";
import { episodeNote } from "@/utils/entrySearch";
import { mediaKindLabel } from "@/utils/media";
import type { FacetGroup, FacetKey, FacetSelection } from "@/utils/searchFacets";

import EntryPoster from "./EntryPoster.vue";
import EpisodeGrid from "./EpisodeGrid.vue";
import FacetChecklist from "./FacetChecklist.vue";

/**
 * 按条目搜索的左栏（画板 06，256 宽）：条目、季、每集格子与一句话、档案、带计数的筛选。
 * 视口窄于 1280 时整块搬进右侧抽屉，内容一样。
 */
const props = defineProps<{
  result: MediaSearchResult;
  profiles: QualityProfile[];
  profileId: number;
  /** profile_id 为 0 时实际用的档案的名字（订阅的或默认的） */
  defaultProfileName: string;
  facets: FacetGroup[];
  moreFacets: FacetGroup[];
  sel: FacetSelection;
  showMore: boolean;
  episode?: number;
}>();

const emit = defineEmits<{
  season: [n: number];
  profile: [id: number];
  toggle: [key: FacetKey, value: string];
  more: [];
  episode: [n: number];
}>();

const item = computed(() => props.result.item);
const isTV = computed(() => item.value.media_type === "tv");

/** 第一行说明：「2025 · 剧集 · 追剧中」 */
const line1 = computed(() => {
  const parts: string[] = [];
  if (item.value.year) parts.push(String(item.value.year));
  parts.push(mediaKindLabel(item.value.media_type));
  const sub = props.result.subscription;
  if (sub) {
    const label: Record<string, string> = {
      active: isTV.value ? "追剧中" : "订阅中",
      paused: "已暂停",
      pending: "待确认",
      done: "已完成",
    };
    parts.push(label[sub.status] ?? sub.status);
  }
  return parts.join(" · ");
});

const lock = computed(() => {
  const l = props.result.subscription?.lock;
  return l ? `版本锁 ${[l.group, l.resolution].filter(Boolean).join(" ")}` : "";
});

const seasons = computed(() =>
  Array.from({ length: Math.max(item.value.seasons ?? 0, 0) }, (_, i) => i + 1),
);
const note = computed(() => episodeNote(props.result.episodes));

const profileOptions = computed(() => [
  { value: 0, label: `档案：${props.defaultProfileName}` },
  ...props.profiles.map((p) => ({ value: p.id, label: `档案：${p.name}` })),
  { value: -1, label: "不按档案" },
]);
</script>

<template>
  <div class="es-rail" data-testid="es-rail">
    <div class="es-rail__item">
      <EntryPoster :path="item.poster_path" size="w185" :width="56" :height="84" />
      <div class="es-rail__text">
        <h2 class="es-rail__title" :title="item.title">{{ item.title }}</h2>
        <p class="es-rail__line">{{ line1 }}</p>
        <p v-if="lock" class="es-rail__line">{{ lock }}</p>
      </div>
    </div>

    <div v-if="isTV && seasons.length > 1" class="es-rail__seasons" role="tablist" aria-label="季">
      <button
        v-for="n in seasons"
        :key="n"
        type="button"
        role="tab"
        class="es-rail__season"
        :class="{ 'is-on': n === item.season }"
        :aria-selected="n === item.season"
        :data-testid="`es-season-${n}`"
        @click="n !== item.season && emit('season', n)">
        第 {{ n }} 季
      </button>
    </div>

    <template v-if="isTV && result.episodes.length">
      <EpisodeGrid
        :episodes="result.episodes"
        :selected="episode"
        @pick="emit('episode', $event)" />
      <p v-if="note" class="es-rail__note" data-testid="es-ep-note">{{ note }}</p>
    </template>

    <el-select
      class="es-rail__profile"
      :model-value="profileId"
      aria-label="质量档案"
      data-testid="es-profile"
      @update:model-value="emit('profile', $event as number)">
      <el-option v-for="o in profileOptions" :key="o.value" :label="o.label" :value="o.value" />
    </el-select>

    <div class="es-rail__rule" />

    <FacetChecklist :groups="facets" :sel="sel" @toggle="(k, v) => emit('toggle', k, v)" />
    <FacetChecklist
      v-if="showMore"
      :groups="moreFacets"
      :sel="sel"
      @toggle="(k, v) => emit('toggle', k, v)" />
    <button
      v-if="moreFacets.length"
      type="button"
      class="es-rail__more"
      data-testid="es-more-facets"
      @click="emit('more')">
      {{ showMore ? "收起更多筛选" : `更多筛选：${moreFacets.map((g) => g.label).join("、")}` }}
    </button>
  </div>
</template>

<style scoped>
.es-rail {
  display: flex;
  flex-direction: column;
  gap: 0;
  padding: 16px;
}

.es-rail__item {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}

.es-rail__text {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
}

/* 条目名 15/700 落地 --pt-fz-body-lg（14，偏离 ②），两行省略 */
.es-rail__title {
  display: -webkit-box;
  margin: 0 0 2px;
  overflow: hidden;
  font-size: var(--pt-fz-body-lg);
  font-weight: 700;
  line-height: 1.3;
  color: var(--pt-t1);
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.es-rail__line {
  margin: 3px 0 0;
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-row);
  color: var(--pt-t3);
}

/* 季：h26 r13、左右 10、12 字；选中 t1 底白字 */
.es-rail__seasons {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 14px;
}

.es-rail__season {
  height: 26px;
  padding: 0 10px;
  font: inherit;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  cursor: pointer;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: 13px;
}

.es-rail__season.is-on {
  font-weight: 500;
  color: var(--pt-surface);
  background: var(--pt-t1);
  border-color: var(--pt-t1);
}

.es-rail :deep(.es-eps) {
  margin-top: 12px;
}

.es-rail__note {
  margin: 6px 0 0;
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-row);
  color: var(--pt-t2);
}

/* 档案：224×30 下拉 */
.es-rail__profile {
  width: 100%;
  margin-top: 12px;

  --el-component-size: 30px;
}

.es-rail__profile :deep(.el-select__wrapper) {
  min-height: 30px;
  font-size: var(--pt-fz-sm);
}

.es-rail__rule {
  height: 1px;
  margin: 14px 0 12px;
  background: var(--pt-border);
}

.es-rail__more {
  align-self: flex-start;
  padding: 0;
  margin-top: 6px;
  font: inherit;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-p);
  text-align: left;
  cursor: pointer;
  background: transparent;
  border: 0;
}
</style>
