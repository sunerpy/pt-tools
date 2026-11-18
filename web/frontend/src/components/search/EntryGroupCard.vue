<script setup lang="ts">
import { computed } from "vue";

import type { MediaSearchEpisode, MediaSearchMember, MediaSearchResult } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import {
  coverPill,
  discountText,
  type EntryCard,
  hrText,
  lockMismatch,
  parsedTags,
  scoreCard,
  shortSize,
} from "@/utils/entrySearch";
import { formatShortDateTime } from "@/utils/format";

/**
 * 按条目搜索的一张结果卡（画板 06 的 card，888 宽，PtPanel 不要卡头）：一张卡是一个资源，卡里列出各站点。
 * 高度 = 92 + 显示的成员行数 × 36 + 22；默认只显示首选一行，其余收进脚注「另有 N 个站点」。
 * 整组库里都有的卡默认折叠成 108 高，点卡展开。
 */
const props = defineProps<{
  card: EntryCard;
  episodes: MediaSearchEpisode[];
  movie?: MediaSearchResult["movie"];
  lock: { group: string; resolution: string } | null;
  /** 合并同一资源开着（关掉时一个成员一张卡，没有脚注） */
  merged: boolean;
  /** 其余站点展开了 */
  expanded: boolean;
  /** 整组库里都有、用户点开了 */
  unfolded: boolean;
  /** 卡上有能等免费的站点（能读种子详情）；没有时「等免费」点不了 */
  canWatch?: boolean;
}>();

const emit = defineEmits<{
  push: [member: MediaSearchMember];
  toggle: [];
  unfold: [];
  /** 等免费：用卡上第一个能等免费的成员 */
  watch: [];
  /** 不要：拉黑这一组的成员 */
  block: [];
  /** 移出黑名单 */
  unblock: [members: MediaSearchMember[]];
}>();

const best = computed(() => props.card.members[0]!);
const blockedHere = computed(() => props.card.members.filter((m) => m.block));
const g = computed(() => props.card.group);
const folded = computed(() => g.value.in_library && !props.unfolded);
const score = computed(() =>
  scoreCard(best.value.verdict, g.value.in_library || props.card.blocked),
);
const pill = computed(() =>
  coverPill({ ...g.value, verdict: best.value.verdict }, props.episodes, props.movie),
);
const lockText = computed(() => lockMismatch(props.lock, best.value.meta));
const tags = computed(() => parsedTags(best.value.meta));
const others = computed(() => props.card.members.slice(1));
const shown = computed(() => (props.expanded ? props.card.members : [best.value]));

const subline = computed(() => {
  const parts = [best.value.subtitle, shortSize(best.value.size_bytes)];
  if (best.value.uploaded_at) parts.push(formatShortDateTime(best.value.uploaded_at * 1000));
  if (folded.value && props.card.members.length > 1) {
    parts.push(`${best.value.site} 等 ${props.card.members.length} 个站点`);
  }
  return parts.filter(Boolean).join(" · ");
});

/** 推送按钮：过了档案的首选是主按钮「推送」，其余站点「推送这个」，不符合档案的「仍然推送」 */
function pushKind(
  m: MediaSearchMember,
  i: number,
): { label: string; kind: "primary" | "soft" | "plain" } {
  if (!props.card.pass || (m.verdict && m.verdict.reject))
    return { label: "仍然推送", kind: "plain" };
  return i === 0 ? { label: "推送", kind: "primary" } : { label: "推送这个", kind: "soft" };
}

function onCardClick() {
  if (folded.value) emit("unfold");
}
</script>

<template>
  <PtPanel
    padding="none"
    class="es-card"
    :class="{
      'is-folded': folded,
      'is-dim': g.in_library || card.blocked,
      'is-blocked': card.blocked,
    }"
    :data-testid="`es-card-${card.key}`"
    @click="onCardClick">
    <div class="es-card__head">
      <div class="es-score" :class="`is-${score.tone}`" data-testid="es-score">
        <span class="es-score__n">{{ score.value }}</span>
        <span class="es-score__u">{{ score.unit }}</span>
      </div>
      <div class="es-card__main">
        <a
          v-if="best.url"
          :href="best.url"
          target="_blank"
          rel="noopener"
          class="es-card__title"
          :title="best.title"
          @click.stop
          >{{ best.title }}</a
        >
        <span v-else class="es-card__title" :title="best.title">{{ best.title }}</span>
        <span class="es-card__sub" :title="subline">{{ subline }}</span>
        <span v-if="tags.length" class="es-card__tags">
          <PtTag v-for="t in tags" :key="t.label" :tone="t.tone">{{ t.label }}</PtTag>
        </span>
        <p v-if="folded" class="es-card__dimnote">这一组的集都已入库，默认折叠</p>
      </div>
      <div class="es-card__side">
        <PtStatusPill
          v-if="pill"
          :tone="pill.tone"
          :dot="pill.dot"
          size="sm"
          class="es-card__pill"
          :title="pill.text">
          <PtIcon v-if="pill.icon" :name="pill.icon" :size="11" />{{ pill.text }}
        </PtStatusPill>
        <span v-if="lockText && !folded" class="es-card__lock">
          <PtIcon name="triangle-alert" :size="11" />不是锁定的版本（{{ lockText }}）
        </span>
      </div>
    </div>
    <template v-if="!folded">
      <div class="es-card__rule" />
      <ul class="es-card__members">
        <li
          v-for="(m, i) in shown"
          :key="`${m.site}|${m.torrent_id}`"
          class="es-member"
          data-testid="es-member">
          <span class="es-member__site">
            <span class="es-member__name">{{ m.site }}</span>
            <PtStatusPill v-if="m.block" tone="neutral" size="sm" :title="m.block.reason || ''"
              >已拉黑</PtStatusPill
            >
            <PtStatusPill v-else-if="i === 0 && card.pass" tone="primary" size="sm"
              >首选</PtStatusPill
            >
          </span>
          <span class="es-member__seed">做种 {{ m.seeders }}</span>
          <span class="es-member__promo">
            <PtStatusPill :tone="discountText(m).tone" size="sm">{{
              discountText(m).text
            }}</PtStatusPill>
          </span>
          <span class="es-member__hr" :class="{ 'is-warn': hrText(m).warn }">{{
            hrText(m).text
          }}</span>
          <el-button
            v-if="m.block"
            class="es-member__push is-plain"
            :data-testid="`es-unblock-${m.site}-${m.torrent_id}`"
            @click.stop="emit('unblock', [m])">
            <span>移出黑名单</span>
          </el-button>
          <el-button
            v-else
            class="es-member__push"
            :class="`is-${pushKind(m, i).kind}`"
            :type="pushKind(m, i).kind === 'primary' ? 'primary' : 'default'"
            :data-testid="`es-push-${m.site}-${m.torrent_id}`"
            @click.stop="emit('push', m)">
            <PtIcon v-if="pushKind(m, i).kind === 'primary'" name="download" :size="14" />
            <span>{{ pushKind(m, i).label }}</span>
          </el-button>
        </li>
      </ul>
      <div class="es-card__foot">
        <button
          v-if="merged && others.length"
          type="button"
          class="es-card__more"
          data-testid="es-more"
          @click.stop="emit('toggle')">
          <template v-if="expanded">收起 ▴</template>
          <template v-else>
            另有 {{ others.length }} 个站点：{{ others.map((m) => m.site).join("、") }} ▾
          </template>
        </button>
        <span class="es-card__acts">
          <!-- 整张卡都拉黑了：成员行上各有「移出黑名单」，不止一个站点时脚注再给一个全部移出 -->
          <button
            v-if="card.blocked && card.members.length > 1"
            type="button"
            class="es-act"
            data-testid="es-act-unblock"
            @click.stop="emit('unblock', blockedHere)">
            <PtIcon name="rotate-ccw" :size="13" /><span>全部移出黑名单</span>
          </button>
          <template v-else-if="!card.blocked">
            <button
              type="button"
              class="es-act"
              :disabled="!canWatch"
              :title="canWatch ? '免费了再推送' : '这些站点读不了种子详情，不能等免费'"
              data-testid="es-act-watch"
              @click.stop="emit('watch')">
              <PtIcon name="hourglass" :size="13" /><span>等免费</span>
            </button>
            <button
              type="button"
              class="es-act"
              data-testid="es-act-block"
              @click.stop="emit('block')">
              <PtIcon name="eye-off" :size="13" /><span>不要</span>
            </button>
          </template>
        </span>
      </div>
    </template>
  </PtPanel>
</template>

<style scoped>
.es-card.is-folded {
  cursor: pointer;
}

.es-card.is-folded:hover {
  border-color: var(--pt-border-strong);
}

/*
 * 卡头：档案分牌 48 在 16,16；种子名在 80,14；右上角的覆盖结果右对齐到 872。
 * 画板的描边画在卡里面，CSS 的 1px 边框在外面多占一像素：竖向的数都减 1，卡高才和画板一样
 * （卡高 = 92 + 行数 × 36 + 22，折叠 108）
 */
.es-card__head {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  min-height: 83px;
  padding: 13px 16px 0;
}

.es-card.is-folded .es-card__head {
  padding-bottom: 13px;
}

.es-score {
  display: flex;
  flex: 0 0 auto;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  margin-top: 2px;
  color: var(--pt-t3);
  background: var(--pt-hover);
  border-radius: var(--pt-r-lg);
}

.es-score.is-ok {
  color: var(--pt-ok);
  background: var(--pt-ok-soft);
}

.es-score.is-dim {
  color: var(--pt-t3);
  background: color-mix(in srgb, var(--pt-t3) 12%, transparent);
}

/* 数 18/700 落地 --pt-fz-display（20，偏离 ②），单位 10 */
.es-score__n {
  font-size: var(--pt-fz-display);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 1.15;
}

.es-score__u {
  font-size: var(--pt-fz-foot);
  line-height: 1.3;
}

.es-card__main {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
}

.es-card__title {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  line-height: 1.35;
  color: var(--pt-t1);
  text-decoration: none;
  text-overflow: ellipsis;
  white-space: nowrap;
}

a.es-card__title:hover {
  color: var(--pt-p);
}

.es-card.is-dim .es-card__title {
  color: var(--pt-t3);
}

.es-card__sub {
  margin-top: 1px;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-row);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.es-card__tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}

.es-card__dimnote {
  margin: 9px 0 0;
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-row);
  color: var(--pt-t3);
}

.es-card__side {
  display: flex;
  flex: 0 1 auto;
  flex-direction: column;
  gap: 4px;
  align-items: flex-end;
  max-width: 45%;
  padding-top: 1px;
}

.es-card__pill {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
}

.es-card__lock {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  font-size: var(--pt-fz-label);
  color: var(--pt-warn);
  white-space: nowrap;
}

.es-card__rule {
  height: 1px;
  margin: 0 16px;
  background: var(--pt-border);
}

/* 成员行：y 92 起、行高 28、行距 36；站点在 80，做种 260，优惠 340，H&R 440，推送右对齐到 872 */
.es-card__members {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 0 16px 0 80px;
  margin: 7px 0 0;
  list-style: none;
}

.es-member {
  display: grid;
  grid-template-columns: 180px 80px 100px minmax(0, 1fr) auto;
  align-items: center;
  height: 28px;
}

.es-member__site {
  display: inline-flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.es-member__name {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.es-member__seed,
.es-member__hr {
  font-size: var(--pt-fz-body);
  font-variant-numeric: tabular-nums;
  color: var(--pt-t2);
  white-space: nowrap;
}

.es-member__hr {
  color: var(--pt-t3);
}

/* 优惠那一格放一枚状态胶囊：胶囊比格子宽时收住，不撑开网格 */
.es-member__promo {
  display: inline-flex;
  align-items: center;
  min-width: 0;
}

.es-member__hr.is-warn {
  color: var(--pt-warn);
}

.es-member__push {
  height: 28px;
  min-width: 72px;
  padding: 0 12px;
  font-size: var(--pt-fz-sm);
}

.es-member__push.is-soft,
.es-member__push.is-plain {
  min-width: 88px;
}

/* soft：主色 10% 底（落地 --pt-p-soft）+ 主色字，没有描边 */
.es-member__push.is-soft {
  color: var(--pt-p);
  background: var(--pt-p-soft);
  border-color: transparent;
}

.es-member__push.is-soft:hover {
  background: var(--pt-p-soft-strong);
}

/* 脚注一行在成员行下面：字 16 高 + 下边 14（含 1px 边框，同卡头） */
.es-card__foot {
  display: flex;
  align-items: center;
  height: 16px;
  padding: 0 16px 0 80px;
  margin: 0 0 13px;
}

.es-card__more {
  padding: 0;
  font: inherit;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-p);
  cursor: pointer;
  background: transparent;
  border: 0;
}

.es-card__more:hover {
  text-decoration: underline;
}

/* 脚注右边的「等免费」「不要」：图标 13 t3 加 12 t3 字，右对齐，「不要」在最右，两项间 12 */
.es-card__acts {
  display: inline-flex;
  gap: 12px;
  align-items: center;
  margin-left: auto;
}

.es-act {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  height: 16px;
  padding: 0;
  font: inherit;
  font-size: var(--pt-fz-sm);
  line-height: 16px;
  color: var(--pt-t3);
  cursor: pointer;
  background: transparent;
  border: 0;
}

.es-act:hover:not(:disabled) {
  color: var(--pt-p);
}

.es-act:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

/* 拉黑的卡灰显：标题换 t3、档案分用 dim 的颜色（同整组已入库，见 score），成员行留着，「移出黑名单」要点得到 */
</style>
