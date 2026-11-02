<script setup lang="ts">
import { computed } from "vue";

import type { MediaSearchEpisode, MediaSearchMember, MediaSearchResult } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import {
  coverPill,
  type EntryCard,
  lockMismatch,
  memberLine,
  parsedTags,
  scoreCard,
  shortSize,
  spanText,
} from "@/utils/entrySearch";

/**
 * 手机上的一张结果卡（画板 09，343 宽的行卡）：桌面卡片的内容竖着排——档案分牌与种子名、
 * 解析标签、覆盖结果、版本提醒、首选成员与推送，其余站点收进「另有 N 个站点」。
 */
const props = defineProps<{
  card: EntryCard;
  episodes: MediaSearchEpisode[];
  movie?: MediaSearchResult["movie"];
  lock: { group: string; resolution: string } | null;
  merged: boolean;
  expanded: boolean;
  unfolded: boolean;
}>();

const emit = defineEmits<{ push: [member: MediaSearchMember]; toggle: []; unfold: [] }>();

const best = computed(() => props.card.members[0]!);
const g = computed(() => props.card.group);
const folded = computed(() => g.value.in_library && !props.unfolded);
const score = computed(() => scoreCard(best.value.verdict, g.value.in_library));
const pill = computed(() =>
  coverPill({ ...g.value, verdict: best.value.verdict }, props.episodes, props.movie),
);
const lockText = computed(() => lockMismatch(props.lock, best.value.meta));
const tags = computed(() => parsedTags(best.value.meta));
const others = computed(() => props.card.members.slice(1));
const shown = computed(() => (props.expanded ? props.card.members : [best.value]));
/* 副行：这一组的集（服务端对过集号的，见 spanText；电影没有）与体积，画板 09「第08集 · 4.71 GB」 */
const sub = computed(() =>
  [spanText(g.value.span) || best.value.subtitle, shortSize(best.value.size_bytes)]
    .filter(Boolean)
    .join(" · "),
);

function pushLabel(
  m: MediaSearchMember,
  i: number,
): { label: string; kind: "primary" | "soft" | "plain" } {
  if (!props.card.pass || (m.verdict && m.verdict.reject))
    return { label: "仍然推送", kind: "plain" };
  return i === 0 ? { label: "推送", kind: "primary" } : { label: "推送这个", kind: "soft" };
}
</script>

<template>
  <article
    class="es-mcard"
    :class="{ 'is-folded': folded, 'is-dim': g.in_library }"
    :data-testid="`es-mcard-${card.key}`"
    @click="folded && emit('unfold')">
    <div class="es-mcard__head">
      <div class="es-mscore" :class="`is-${score.tone}`">
        <span class="es-mscore__n">{{ score.value }}</span>
        <span class="es-mscore__u">{{ score.unit }}</span>
      </div>
      <div class="es-mcard__text">
        <p class="es-mcard__title" :title="best.title">{{ best.title }}</p>
        <p class="es-mcard__sub">{{ sub }}</p>
      </div>
    </div>
    <div v-if="tags.length" class="es-mcard__tags">
      <PtTag v-for="t in tags" :key="t.label" :tone="t.tone">{{ t.label }}</PtTag>
    </div>
    <div v-if="pill" class="es-mcard__pill">
      <PtStatusPill :tone="pill.tone" :dot="pill.dot" size="sm">
        <PtIcon v-if="pill.icon" :name="pill.icon" :size="11" />{{ pill.text }}
      </PtStatusPill>
    </div>
    <p v-if="lockText && !folded" class="es-mcard__lock">
      <PtIcon name="triangle-alert" :size="11" />不是锁定的版本（{{ lockText }}）
    </p>
    <p v-if="folded" class="es-mcard__dimnote">这一组的集都已入库，点一下展开</p>
    <template v-else>
      <div class="es-mcard__rule" />
      <div v-for="(m, i) in shown" :key="`${m.site}|${m.torrent_id}`" class="es-mmember">
        <div class="es-mmember__text">
          <span class="es-mmember__site">
            {{ m.site }}
            <PtStatusPill v-if="i === 0 && card.pass" tone="primary" size="sm">首选</PtStatusPill>
          </span>
          <span class="es-mmember__line">{{ memberLine(m) }}</span>
        </div>
        <el-button
          class="es-mmember__push"
          :class="`is-${pushLabel(m, i).kind}`"
          :type="pushLabel(m, i).kind === 'primary' ? 'primary' : 'default'"
          @click.stop="emit('push', m)">
          <PtIcon v-if="pushLabel(m, i).kind === 'primary'" name="download" :size="15" />
          <span>{{ pushLabel(m, i).label }}</span>
        </el-button>
      </div>
      <button
        v-if="merged && others.length"
        type="button"
        class="es-mcard__more"
        @click.stop="emit('toggle')">
        <template v-if="expanded">收起 ▴</template>
        <template v-else
          >另有 {{ others.length }} 个站点：{{ others.map((m) => m.site).join("、") }} ▾</template
        >
      </button>
    </template>
  </article>
</template>

<style scoped>
/* 行卡：343 宽、r8、1px 边、14 内边距（同 PtRowCard 的全站规格） */
.es-mcard {
  display: flex;
  flex-direction: column;
  padding: 14px;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

.es-mcard.is-folded {
  cursor: pointer;
}

.es-mcard__head {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}

.es-mscore {
  display: flex;
  flex: 0 0 auto;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  color: var(--pt-t3);
  background: var(--pt-hover);
  border-radius: var(--pt-r-lg);
}

.es-mscore.is-ok {
  color: var(--pt-ok);
  background: var(--pt-ok-soft);
}

.es-mscore.is-dim {
  background: color-mix(in srgb, var(--pt-t3) 12%, transparent);
}

.es-mscore__n {
  font-size: var(--pt-fz-h2);
  font-weight: 700;
  line-height: 1.2;
}

.es-mscore__u {
  font-size: var(--pt-fz-foot);
}

.es-mcard__text {
  flex: 1 1 auto;
  min-width: 0;
}

/* 种子名 13/700，两行省略 */
.es-mcard__title {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  line-height: 1.4;
  color: var(--pt-t1);
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.es-mcard.is-dim .es-mcard__title {
  color: var(--pt-t3);
}

.es-mcard__sub {
  margin: 3px 0 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.es-mcard__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 10px;
}

.es-mcard__pill {
  display: flex;
  margin-top: 10px;
}

.es-mcard__lock {
  display: flex;
  gap: 5px;
  align-items: center;
  margin: 6px 0 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-warn);
}

.es-mcard__dimnote {
  margin: 8px 0 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.es-mcard__rule {
  height: 1px;
  margin: 12px 0 10px;
  background: var(--pt-border);
}

.es-mmember {
  display: flex;
  gap: 10px;
  align-items: center;
  min-height: 46px;
}

.es-mmember + .es-mmember {
  margin-top: 6px;
}

.es-mmember__text {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.es-mmember__site {
  display: inline-flex;
  gap: 8px;
  align-items: center;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  color: var(--pt-t1);
}

.es-mmember__line {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 推送：主按钮 76×36；不符合档案时 default 92×36「仍然推送」 */
.es-mmember__push {
  flex: 0 0 auto;
  height: 36px;
  min-width: 76px;
}

.es-mmember__push.is-plain,
.es-mmember__push.is-soft {
  min-width: 92px;
}

.es-mmember__push.is-soft {
  color: var(--pt-p);
  background: var(--pt-p-soft);
  border-color: transparent;
}

.es-mcard__more {
  align-self: flex-start;
  min-height: 24px;
  padding: 0;
  margin-top: 4px;
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
