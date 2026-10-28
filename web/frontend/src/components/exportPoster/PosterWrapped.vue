<script setup lang="ts">
/**
 * 回顾：像年度回顾一样讲故事，一句话一个大数字；渐变底，上传最多的三个站上领奖台（台子高低就是名次），
 * 其余的列在下面。入图的站点不到三个时不画领奖台，直接列表。
 */
import { computed } from "vue";
import { usePosterWords } from "./locale";

import type { PosterModel, PosterSite } from "./model";
import PosterLogo from "./PosterLogo.vue";
import PosterMask from "./PosterMask.vue";

const props = defineProps<{ model: PosterModel; colorway: string }>();
const t = usePosterWords();

/** 领奖台的摆法：左第二、中第一、右第三 */
const podium = computed<{ s: PosterSite; rank: number; h: number }[]>(() => {
  const [a, b, c] = props.model.sites;
  if (!a || !b || !c) return [];
  return [
    { s: b, rank: 2, h: 64 },
    { s: a, rank: 1, h: 92 },
    { s: c, rank: 3, h: 46 },
  ];
});
const rest = computed(() =>
  podium.value.length
    ? props.model.sites.slice(3).map((s, i) => ({ s, rank: i + 4 }))
    : props.model.sites.map((s, i) => ({ s, rank: i + 1 })),
);
</script>

<template>
  <div class="poster poster-wrapped" :class="`is-${colorway}`">
    <div class="pw-top">
      <span>PT-TOOLS</span><span>{{ model.date }}</span>
    </div>
    <div class="pw-lead">
      {{ model.title }} ·
      <template v-if="model.joinDur">{{ t("入站 ") }}{{ model.joinDur }}，</template
      >{{ t("在\n      ") }}{{ model.siteCount }}{{ t(" 个站点一共上传了\n    ") }}
    </div>
    <div class="pw-mega">
      {{ model.up.value }}<small>{{ model.up.unit }}</small>
    </div>
    <div class="pw-say">
      {{ t("\n      平均分享率 ") }}<b style="color: var(--c-ratio)">{{ model.ratio }}</b
      >{{ t("，计费下载 ") }}<b style="color: var(--c-down)">{{ model.down.text }}</b
      >。
      <template v-if="model.join">
        <br />{{ t("最早的账号从 ") }}<b>{{ model.join }}</b
        >{{ t(" 开始，到今天 ") }}<b>{{ model.joinDur }}</b
        >。
      </template>
    </div>

    <div class="pw-trio">
      <div>
        <div class="pw-trio__k">{{ t("总魔力") }}</div>
        <div class="pw-trio__n" style="color: var(--c-bonus)">
          {{ model.bonus.value }}<small>{{ model.bonus.unit }}</small>
        </div>
        <div class="pw-trio__h">{{ t("每小时 +") }}{{ model.bph.text }}</div>
      </div>
      <div>
        <div class="pw-trio__k">{{ t("正在做种") }}</div>
        <div class="pw-trio__n">
          {{ model.seeding }}<small>{{ t("个") }}</small>
        </div>
        <div class="pw-trio__h">{{ t("共 ") }}{{ model.seedSize.text }}</div>
      </div>
      <div v-if="model.seedBonus">
        <div class="pw-trio__k">{{ t("做种积分") }}</div>
        <div class="pw-trio__n">
          {{ model.seedBonus.value }}<small>{{ model.seedBonus.unit }}</small>
        </div>
        <div class="pw-trio__h">{{ model.siteCount }}{{ t(" 个站点合计") }}</div>
      </div>
      <div v-else>
        <div class="pw-trio__k">{{ t("平均每站上传") }}</div>
        <div class="pw-trio__n">
          {{ model.avgUp.value }}<small>{{ model.avgUp.unit }}</small>
        </div>
        <div class="pw-trio__h">{{ t("按 ") }}{{ model.siteCount }}{{ t(" 个站点平均") }}</div>
      </div>
    </div>

    <div v-if="model.inc" class="pw-week">
      <span>
        <span class="pw-week__k">{{ model.inc.label }}</span
        >{{ t("又上传了 ") }}<b>{{ model.inc.up }}</b>
      </span>
      <span class="pw-week__r"
        >{{ t("下载 ") }}{{ model.inc.down }}{{ t(" · 魔力 ") }}{{ model.inc.bonus }}</span
      >
    </div>

    <template v-if="model.sites.length">
      <h2 class="pw-h2">{{ t("上传最多的站") }}</h2>
      <div v-if="podium.length" class="pw-podium">
        <div v-for="p in podium" :key="p.s.id" class="pw-pd">
          <PosterLogo
            :logo="p.s.logo"
            :name="p.s.name"
            :color="p.s.color"
            :masked="model.mask.logos"
            :size="p.rank === 1 ? 40 : 32"
            :radius="11" />
          <span class="pw-pd__nm"><PosterMask :text="p.s.name" :on="model.mask.names" /></span>
          <span class="pw-pd__up">{{ p.s.up.text }}</span>
          <span class="pw-pd__lv">{{ t("分享率 ") }}{{ p.s.ratio }}</span>
          <span v-if="p.s.user || p.s.level" class="pw-pd__lv">
            <PosterMask v-if="p.s.user" :text="p.s.user" :on="model.mask.users" prefix="@" />
            <span v-if="p.s.user && p.s.level"> · </span>{{ p.s.level }}
          </span>
          <div class="pw-pd__block" :style="{ height: `${p.h}px` }">{{ p.rank }}</div>
        </div>
      </div>
      <div v-if="rest.length" class="pw-rest" :class="{ 'is-alone': !podium.length }">
        <div v-for="r in rest" :key="r.s.id" class="pw-rr">
          <span class="pw-rr__i">{{ r.rank }}</span>
          <PosterLogo
            :logo="r.s.logo"
            :name="r.s.name"
            :color="r.s.color"
            :masked="model.mask.logos"
            :size="22"
            :radius="7" />
          <span class="pw-rr__who">
            <span class="pw-rr__nm"><PosterMask :text="r.s.name" :on="model.mask.names" /></span>
            <span v-if="r.s.user" class="pw-rr__user">
              <PosterMask :text="r.s.user" :on="model.mask.users" prefix="@" />
            </span>
          </span>
          <span class="pw-rr__v" style="color: var(--c-up)">{{ r.s.up.text }}</span>
          <span class="pw-rr__v" style="color: var(--c-ratio)">{{ r.s.ratio }}</span>
          <span class="pw-rr__v" style="color: var(--c-bonus)">
            {{ r.s.bonus.value }}<small>{{ r.s.bonus.unit }}{{ r.s.bonusName }}</small>
          </span>
        </div>
      </div>
    </template>

    <div class="pw-foot"><b>PT-TOOLS</b>{{ t(" · 生成于 ") }}{{ model.generated }}</div>
  </div>
</template>

<style scoped>
.poster-wrapped {
  --t1: #fff;
  --t2: rgb(255 255 255 / 78%);
  --t3: rgb(255 255 255 / 56%);
  --card: rgb(255 255 255 / 12%);
  --edge: rgb(255 255 255 / 18%);
  --week-k: #9d174d;
  --week-b: #6d28d9;
  --week-t: #5b4f66;
  --week-r: #7c6f86;
  --c-up: #b9f6ca;
  --c-down: #ffd3d8;
  --c-ratio: #c7e5ff;
  --c-bonus: #ffe8a3;
  padding: 36px 34px 28px;
  color: var(--t1);
  background:
    radial-gradient(70% 45% at 85% 8%, rgb(255 196 120 / 55%), transparent 70%),
    radial-gradient(60% 40% at 0% 60%, rgb(236 72 153 / 45%), transparent 70%),
    radial-gradient(70% 40% at 100% 100%, rgb(99 102 241 / 55%), transparent 70%),
    linear-gradient(160deg, #6d28d9 0%, #be185d 48%, #ea580c 100%);
  border-radius: 28px;
}

.poster-wrapped.is-ocean {
  --week-k: #0e7490;
  --week-b: #1e40af;
  --week-t: #3f5a66;
  --week-r: #5f7882;
  background:
    radial-gradient(70% 45% at 85% 8%, rgb(56 189 248 / 50%), transparent 70%),
    radial-gradient(60% 40% at 0% 60%, rgb(16 185 129 / 40%), transparent 70%),
    radial-gradient(70% 40% at 100% 100%, rgb(99 102 241 / 45%), transparent 70%),
    linear-gradient(160deg, #1e3a8a 0%, #0e7490 50%, #059669 100%);
}

.poster-wrapped.is-berry {
  --week-k: #be185d;
  --week-b: #7c3aed;
  --week-t: #5b4f66;
  --week-r: #7c6f86;
  background:
    radial-gradient(70% 45% at 85% 8%, rgb(244 114 182 / 55%), transparent 70%),
    radial-gradient(60% 40% at 0% 60%, rgb(168 85 247 / 45%), transparent 70%),
    radial-gradient(70% 40% at 100% 100%, rgb(99 102 241 / 50%), transparent 70%),
    linear-gradient(160deg, #be185d 0%, #9333ea 55%, #4f46e5 100%);
}

.pw-top {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.18em;
  color: var(--t3);
}

.pw-lead {
  margin-top: 30px;
  font-size: 15px;
  color: var(--t2);
  overflow-wrap: anywhere;
}

.pw-mega {
  margin-top: 6px;
  font-size: 88px;
  font-weight: 900;
  line-height: 0.95;
  letter-spacing: -0.045em;
  white-space: nowrap;
}

.pw-mega small {
  margin-left: 8px;
  font-size: 30px;
  font-weight: 800;
  letter-spacing: 0;
  color: var(--t2);
}

.pw-say {
  margin-top: 14px;
  font-size: 15px;
  line-height: 1.7;
  color: var(--t2);
}

.pw-say b {
  font-weight: 800;
  color: var(--t1);
}

.pw-trio {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  margin-top: 26px;
}

.pw-trio > div {
  min-width: 0;
  padding: 14px 14px 13px;
  background: var(--card);
  border: 1px solid var(--edge);
  border-radius: 18px;
}

.pw-trio__k {
  font-size: 12px;
  color: var(--t2);
}

.pw-trio__n {
  margin-top: 6px;
  font-size: 26px;
  font-weight: 900;
  letter-spacing: -0.02em;
  white-space: nowrap;
}

.pw-trio__n small {
  margin-left: 3px;
  font-size: 12px;
  font-weight: 700;
  color: var(--t2);
}

.pw-trio__h {
  margin-top: 4px;
  font-size: 11px;
  color: var(--t3);
}

.pw-week {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 12px;
  align-items: baseline;
  justify-content: space-between;
  margin-top: 12px;
  padding: 14px 18px;
  font-size: 14px;
  color: var(--week-t);
  background: rgb(255 255 255 / 92%);
  border-radius: 18px;
}

.pw-week__k {
  margin-right: 6px;
  font-weight: 800;
  color: var(--week-k);
}

.pw-week b {
  font-size: 22px;
  font-weight: 900;
  color: var(--week-b);
}

.pw-week__r {
  font-size: 12px;
  color: var(--week-r);
}

.pw-h2 {
  margin: 30px 0 14px;
  font-size: 15px;
  font-weight: 700;
  color: var(--t2);
}

/* 领奖台：中间第一、左第二、右第三，台子高低就是名次 */
.pw-podium {
  display: grid;
  grid-template-columns: 1fr 1.12fr 1fr;
  gap: 10px;
  align-items: end;
}

.pw-pd {
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: center;
  min-width: 0;
  text-align: center;
}

.pw-pd__nm {
  font-size: 14px;
  font-weight: 800;
}

.pw-pd__up {
  font-size: 17px;
  font-weight: 900;
  color: var(--c-up);
  white-space: nowrap;
}

.pw-pd__lv {
  max-width: 100%;
  overflow: hidden;
  font-size: 10.5px;
  color: var(--t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pw-pd__block {
  display: flex;
  align-items: flex-start;
  justify-content: center;
  width: 100%;
  margin-top: 4px;
  padding-top: 10px;
  font-size: 28px;
  font-weight: 900;
  color: rgb(255 255 255 / 85%);
  background: linear-gradient(180deg, rgb(255 255 255 / 26%), rgb(255 255 255 / 8%));
  border: 1px solid var(--edge);
  border-bottom: 0;
  border-radius: 14px 14px 0 0;
}

.pw-rest {
  padding: 4px 16px;
  background: rgb(0 0 0 / 14%);
  border: 1px solid var(--edge);
  border-radius: 0 0 18px 18px;
}

.pw-rest.is-alone {
  border-radius: 18px;
}

.pw-rr {
  display: grid;
  grid-template-columns: 22px 26px minmax(0, 1fr) 86px 50px 96px;
  gap: 8px;
  align-items: center;
  padding: 10px 0;
  font-size: 13px;
  border-bottom: 1px solid rgb(255 255 255 / 10%);
}

.pw-rr:last-child {
  border-bottom: 0;
}

.pw-rr__i {
  font-weight: 800;
  color: var(--t3);
}

.pw-rr__who {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.pw-rr__nm {
  font-weight: 700;
}

.pw-rr__user {
  overflow: hidden;
  font-size: 10.5px;
  color: var(--t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pw-rr__v {
  font-weight: 800;
  text-align: right;
  white-space: nowrap;
}

.pw-rr__v small {
  margin-left: 2px;
  font-size: 10px;
  font-weight: 500;
  color: var(--t3);
}

.pw-foot {
  margin-top: 22px;
  font-size: 10.5px;
  color: var(--t3);
  text-align: center;
}

.pw-foot b {
  font-weight: 800;
  letter-spacing: 0.12em;
  color: var(--t2);
}
</style>
