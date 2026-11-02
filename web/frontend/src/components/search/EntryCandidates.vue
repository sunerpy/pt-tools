<script setup lang="ts">
import { computed } from "vue";

import type { ExploreItem } from "@/api";
import PtIcon from "@/components/PtIcon";
import { CLASSIFY_COUNTRIES, TMDB_GENRES } from "@/utils/media";

import EntryPoster from "./EntryPoster.vue";

/**
 * 打字时的 TMDB 候选（画板 05 的片段，B 方向挂在 640 宽的输入框下）：剧集、电影各前 5 个，
 * 选了就按条目搜；底下一行说明直接回车仍按关键字搜。高亮哪一个由输入框的上下键决定（active）。
 */
const props = defineProps<{
  tv: ExploreItem[];
  movies: ExploreItem[];
  keyword: string;
  /** 键盘高亮的候选（剧集在前、电影在后连着数），-1 是没有 */
  active: number;
  loading: boolean;
}>();

const emit = defineEmits<{ pick: [item: ExploreItem]; hover: [index: number]; keyword: [] }>();

const groups = computed(() => {
  const out: { label: string; items: { it: ExploreItem; index: number }[] }[] = [];
  if (props.tv.length)
    out.push({ label: "剧集", items: props.tv.map((it, i) => ({ it, index: i })) });
  if (props.movies.length) {
    out.push({
      label: "电影",
      items: props.movies.map((it, i) => ({ it, index: props.tv.length + i })),
    });
  }
  return out;
});

const countryName = (code: string) =>
  CLASSIFY_COUNTRIES.find((c) => c.value === code)?.label ?? code;

/** 说明行：剧集「2025 · 中国大陆」，电影「2023 · 剧情 / 犯罪」 */
function subOf(it: ExploreItem): string {
  const parts: string[] = [];
  if (it.year) parts.push(String(it.year));
  if (it.media_type === "tv") {
    if (it.countries?.length) parts.push(it.countries.slice(0, 2).map(countryName).join("、"));
  } else {
    const names = (it.genre_ids ?? [])
      .map((id) => TMDB_GENRES.movie.find((g) => g.id === id)?.name)
      .filter(Boolean)
      .slice(0, 2);
    if (names.length) parts.push(names.join(" / "));
  }
  return parts.join(" · ");
}

function tagOf(it: ExploreItem): { text: string; tone: "primary" | "ok" } | null {
  if (it.subscribed) return { text: it.media_type === "tv" ? "追剧中" : "已订阅", tone: "primary" };
  if (it.in_library) return { text: "库里有", tone: "ok" };
  return null;
}
</script>

<template>
  <div class="es-cands" role="listbox" aria-label="TMDB 上的条目" data-testid="es-candidates">
    <p v-if="loading && groups.length === 0" class="es-cands__empty">正在找 TMDB 上的条目…</p>
    <p v-else-if="groups.length === 0" class="es-cands__empty">TMDB 上没有找到「{{ keyword }}」</p>
    <section v-for="g in groups" :key="g.label" class="es-cands__group">
      <h3 class="es-cands__label">{{ g.label }}</h3>
      <button
        v-for="{ it, index } in g.items"
        :key="`${it.media_type}-${it.id}`"
        type="button"
        role="option"
        class="es-cands__row"
        :class="{ 'is-active': index === active }"
        :aria-selected="index === active"
        :data-testid="`es-cand-${it.media_type}-${it.id}`"
        @mouseenter="emit('hover', index)"
        @mousedown.prevent
        @click="emit('pick', it)">
        <EntryPoster :path="it.poster_path" size="w92" :width="24" :height="36" :radius="2" />
        <span class="es-cands__text">
          <span class="es-cands__title">{{ it.title }}</span>
          <span v-if="subOf(it)" class="es-cands__sub">{{ subOf(it) }}</span>
        </span>
        <span v-if="tagOf(it)" class="es-cands__tag" :class="`is-${tagOf(it)!.tone}`">
          {{ tagOf(it)!.text }}
        </span>
      </button>
    </section>
    <button type="button" class="es-cands__foot" @mousedown.prevent @click="emit('keyword')">
      <PtIcon name="arrow-up-down" :size="14" />
      <span>回车：按关键字「{{ keyword }}」搜（和现在一样）</span>
    </button>
  </div>
</template>

<style scoped>
/* 画板 05 dropdown：白底、1px 边、r8、0 8 24 的阴影；分组名 11/700 t3，候选 48 高 */
.es-cands {
  display: flex;
  flex-direction: column;
  padding: 10px 0 0;
  overflow: hidden;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: var(--pt-shadow-lg);
}

.es-cands__group + .es-cands__group {
  margin-top: 6px;
}

.es-cands__label {
  margin: 0 0 4px;
  padding: 0 14px;
  font-size: var(--pt-fz-label);
  font-weight: 700;
  line-height: var(--pt-lh-row);
  color: var(--pt-t3);
}

.es-cands__row {
  display: flex;
  gap: 10px;
  align-items: center;
  width: calc(100% - 12px);
  height: 48px;
  padding: 0 8px;
  margin: 0 6px;
  font: inherit;
  color: inherit;
  text-align: left;
  cursor: pointer;
  background: transparent;
  border: 0;
  border-radius: var(--pt-r-md);
}

.es-cands__row.is-active,
.es-cands__row:hover {
  background: var(--pt-hover);
}

.es-cands__text {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.es-cands__title {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  line-height: 1.4;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.es-cands__sub {
  overflow: hidden;
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-row);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 状态：h18 r4、14% 底加同色 11/500 */
.es-cands__tag {
  flex: 0 0 auto;
  padding: 1px 6px;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  line-height: 16px;
  border-radius: var(--pt-r-sm);
}

.es-cands__tag.is-primary {
  color: var(--pt-p);
  background: color-mix(in srgb, var(--pt-p) 14%, transparent);
}

.es-cands__tag.is-ok {
  color: var(--pt-ok);
  background: color-mix(in srgb, var(--pt-ok) 14%, transparent);
}

.es-cands__empty {
  margin: 4px 0 10px;
  padding: 0 14px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.es-cands__foot {
  display: flex;
  gap: 6px;
  align-items: center;
  height: 40px;
  padding: 0 14px;
  margin-top: 8px;
  font: inherit;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  text-align: left;
  cursor: pointer;
  background: transparent;
  border: 0;
  border-top: 1px solid var(--pt-border);
}

.es-cands__foot:hover {
  color: var(--pt-t1);
}
</style>
