<script lang="ts">
/* 同一页面里可能同时有几张海报（例如测试里）：环形图渐变的 id 按实例编号，各自唯一 */
let ringSeq = 0;
</script>

<script setup lang="ts">
/**
 * 夜幕：深色底上两团柔光、极淡的网格，内容在玻璃卡片里。环形图画的是「上传与计费下载」的比例
 * （红的那一段是下载），环中间是平均分享率；逐站每行下面一条绿红比例条，条长是上传。
 * 玻璃不用 backdrop-filter：导出时画进 SVG 的内容不认它，预览与 PNG 会对不上。
 */
import { computed } from "vue";

import PtIcon from "@/components/PtIcon";

import type { PosterModel } from "./model";
import PosterLogo from "./PosterLogo.vue";
import PosterMask from "./PosterMask.vue";

const props = defineProps<{ model: PosterModel; colorway: string }>();

const gradId = `pn-ring-${++ringSeq}`;

const R = 64;
const C = 2 * Math.PI * R;
const upPct = computed(() => Math.round(props.model.upFrac * 100));
/** 上传那段弧：两端是圆头，减掉一点让红色的下载段露出来；几乎全是上传时画满一圈 */
const arc = computed(() => {
  const f = props.model.upFrac;
  if (f >= 0.995) return { len: C, down: false };
  if (f <= 0.005) return { len: 0, down: true };
  return { len: Math.max(0, C * f - 8), down: true };
});

interface Tile {
  c: string;
  icon: string;
  k: string;
  v: string;
  u: string;
}

/** 六个小格：按顺序取有的，没有做种积分、入站日期时往后补 */
const tiles = computed<Tile[]>(() => {
  const m = props.model;
  const all: (Tile | null)[] = [
    { c: "bph", icon: "clock", k: "时魔", v: m.bph.value, u: `${m.bph.unit}/h` },
    { c: "seed", icon: "leaf", k: "做种数", v: m.seeding, u: "" },
    { c: "size", icon: "hard-drive", k: "做种量", v: m.seedSize.value, u: m.seedSize.unit },
    m.seedBonus
      ? { c: "sb", icon: "sparkles", k: "做种积分", v: m.seedBonus.value, u: m.seedBonus.unit }
      : null,
    m.join ? { c: "join", icon: "calendar", k: "最早入站", v: m.join, u: "" } : null,
    { c: "sites", icon: "layers", k: "站点", v: String(m.siteCount), u: "个" },
    { c: "up", icon: "upload", k: "平均每站上传", v: m.avgUp.value, u: m.avgUp.unit },
  ];
  return all.filter((t): t is Tile => t !== null).slice(0, 6);
});
</script>

<template>
  <div class="poster poster-night" :class="`is-${colorway}`">
    <div class="pn-grid" />
    <div class="pn-top">
      <span class="pn-pill"><i />PT-TOOLS</span><span>{{ model.date }}</span>
    </div>
    <h1 class="pn-title">{{ model.title }}</h1>
    <div class="pn-sub">
      {{ model.siteCount }} 个站点<template v-if="model.join">
        · 入站 {{ model.joinDur }}（{{ model.join }} 起）</template
      >
    </div>

    <div class="pn-hero pn-glass">
      <div class="pn-ring">
        <svg viewBox="0 0 150 150" width="150" height="150" aria-hidden="true">
          <defs>
            <linearGradient :id="gradId" x1="0" y1="0" x2="1" y2="1">
              <stop offset="0" class="pn-g1" />
              <stop offset="1" class="pn-g2" />
            </linearGradient>
          </defs>
          <circle cx="75" cy="75" :r="R" fill="none" class="pn-ring__track" stroke-width="12" />
          <circle
            v-if="arc.down"
            cx="75"
            cy="75"
            :r="R"
            fill="none"
            class="pn-ring__down"
            stroke-width="12" />
          <circle
            v-if="arc.len > 0"
            cx="75"
            cy="75"
            :r="R"
            fill="none"
            :stroke="`url(#${gradId})`"
            stroke-width="12"
            :stroke-dasharray="`${arc.len.toFixed(1)} ${C.toFixed(1)}`"
            :stroke-linecap="arc.down ? 'round' : 'butt'" />
        </svg>
        <div class="pn-ring__c">
          <b>{{ model.ratio }}</b
          ><span>平均分享率</span>
        </div>
      </div>
      <div class="pn-big">
        <div class="pn-big__lab">总上传</div>
        <div class="pn-big__v" style="color: var(--c-up)">
          {{ model.up.value }}<small>{{ model.up.unit }}</small>
        </div>
        <div class="pn-big__row">
          <div>
            下载（计费）<b style="color: var(--c-down)">{{ model.down.text }}</b>
          </div>
          <div>
            总魔力<b style="color: var(--c-bonus)">{{ model.bonus.text }}</b>
          </div>
        </div>
        <div class="pn-legend">
          <span><i style="background: var(--c-up)" />上传 {{ upPct }}%</span>
          <span><i style="background: var(--c-down)" />计费下载 {{ 100 - upPct }}%</span>
        </div>
      </div>
    </div>

    <div class="pn-tiles">
      <div v-for="t in tiles" :key="t.k" class="pn-tile pn-glass">
        <span
          class="pn-ico"
          :style="{
            color: `var(--c-${t.c})`,
            background: `color-mix(in srgb, var(--c-${t.c}) 16%, transparent)`,
          }">
          <PtIcon :name="t.icon" :size="17" />
        </span>
        <div class="pn-tile__body">
          <div class="pn-tile__k">{{ t.k }}</div>
          <div class="pn-tile__v" :style="{ color: `var(--c-${t.c})` }">
            {{ t.v }}<small v-if="t.u">{{ t.u }}</small>
          </div>
        </div>
      </div>
    </div>

    <div v-if="model.inc" class="pn-inc pn-glass">
      <span class="pn-inc__k">{{ model.inc.label }}</span>
      <span class="pn-chip" style="background: color-mix(in srgb, var(--c-up) 14%, transparent)">
        上传<b style="color: var(--c-up)">{{ model.inc.up }}</b>
      </span>
      <span class="pn-chip" style="background: color-mix(in srgb, var(--c-down) 14%, transparent)">
        下载<b style="color: var(--c-down)">{{ model.inc.down }}</b>
      </span>
      <span class="pn-chip" style="background: color-mix(in srgb, var(--c-bonus) 14%, transparent)">
        魔力<b style="color: var(--c-bonus)">{{ model.inc.bonus }}</b>
      </span>
    </div>

    <div v-if="model.sites.length" class="pn-sites pn-glass">
      <div class="pn-sites__hd">
        <span>逐站明细 · 条长是上传，绿红是上传与计费下载</span><span>上传 · 分享率 · 魔力</span>
      </div>
      <div v-for="s in model.sites" :key="s.id" class="pn-row">
        <PosterLogo
          :logo="s.logo"
          :name="s.name"
          :color="s.color"
          :masked="model.mask.logos"
          :size="26"
          :radius="8" />
        <div class="pn-row__who">
          <div class="pn-row__nm">
            <PosterMask :text="s.name" :on="model.mask.names" />
            <span v-if="s.user || s.level" class="pn-lv">
              <PosterMask v-if="s.user" :text="s.user" :on="model.mask.users" prefix="@" />
              <span v-if="s.user && s.level"> · </span>{{ s.level }}
            </span>
          </div>
          <div class="pn-ud" :style="{ width: `${(s.upShare * 100).toFixed(1)}%` }">
            <i :style="{ flex: s.upFrac, background: 'var(--c-up)' }" />
            <i :style="{ flex: 1 - s.upFrac, background: 'var(--c-down)' }" />
          </div>
        </div>
        <span class="pn-row__v" style="color: var(--c-up)">{{ s.up.text }}</span>
        <span class="pn-row__v" style="color: var(--c-ratio)">{{ s.ratio }}</span>
        <span class="pn-row__v" style="color: var(--c-bonus)">
          {{ s.bonus.value }}<small>{{ s.bonus.unit }}{{ s.bonusName }}</small>
        </span>
      </div>
    </div>

    <div class="pn-foot">Generated by pt-tools · {{ model.generated }}</div>
  </div>
</template>

<style scoped>
.poster-night {
  --t1: #f4f7fb;
  --t2: rgb(226 232 240 / 66%);
  --t3: rgb(226 232 240 / 42%);
  --glass: rgb(255 255 255 / 5.5%);
  --edge: rgb(255 255 255 / 10%);
  --g1: #34d399;
  --g2: #38bdf8;
  --c-up: #34d399;
  --c-down: #fb7185;
  --c-ratio: #38bdf8;
  --c-bonus: #fbbf24;
  --c-bph: #fb923c;
  --c-seed: #2dd4bf;
  --c-size: #22d3ee;
  --c-sb: #a3e635;
  --c-join: #a78bfa;
  --c-sites: #818cf8;
  padding: 34px 32px 26px;
  color: var(--t1);
  background:
    radial-gradient(60% 40% at 0% 0%, rgb(45 212 191 / 22%), transparent 70%),
    radial-gradient(55% 38% at 100% 100%, rgb(139 92 246 / 26%), transparent 70%),
    radial-gradient(40% 30% at 100% 18%, rgb(56 189 248 / 12%), transparent 70%),
    linear-gradient(180deg, #0a1020, #070b16);
  border-radius: 24px;
}

.poster-night.is-amber {
  --g1: #60a5fa;
  --g2: #fbbf24;
  background:
    radial-gradient(60% 40% at 0% 0%, rgb(59 130 246 / 26%), transparent 70%),
    radial-gradient(55% 38% at 100% 100%, rgb(249 115 22 / 24%), transparent 70%),
    radial-gradient(40% 30% at 100% 18%, rgb(56 189 248 / 10%), transparent 70%),
    linear-gradient(180deg, #0b1022, #080b17);
}

.poster-night.is-black {
  --glass: rgb(255 255 255 / 4.5%);
  --edge: rgb(255 255 255 / 9%);
  --g1: #f4f4f5;
  --g2: #a1a1aa;
  background:
    radial-gradient(60% 40% at 0% 0%, rgb(255 255 255 / 7%), transparent 70%),
    radial-gradient(55% 38% at 100% 100%, rgb(255 255 255 / 5%), transparent 70%),
    linear-gradient(180deg, #0a0a0a, #030303);
}

/* 极淡的网格：只在暗底上看得出质感，不抢数字 */
.pn-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgb(255 255 255 / 3%) 1px, transparent 1px),
    linear-gradient(90deg, rgb(255 255 255 / 3%) 1px, transparent 1px);
  background-size: 32px 32px;
  mask-image: linear-gradient(180deg, #000, transparent 55%);
  pointer-events: none;
}

.poster-night > *:not(.pn-grid) {
  position: relative;
}

.pn-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 11px;
  color: var(--t3);
}

.pn-pill {
  display: inline-flex;
  gap: 7px;
  align-items: center;
  padding: 4px 10px;
  font-weight: 700;
  letter-spacing: 0.14em;
  color: var(--t2);
  background: var(--glass);
  border: 1px solid var(--edge);
  border-radius: 999px;
}

.pn-pill > i {
  width: 6px;
  height: 6px;
  background: var(--c-seed);
  border-radius: 50%;
  box-shadow: 0 0 8px var(--c-seed);
}

.pn-title {
  margin: 22px 0 6px;
  font-size: 28px;
  font-weight: 700;
  line-height: 1.3;
  letter-spacing: 0.01em;
  overflow-wrap: anywhere;
}

.pn-sub {
  font-size: 13px;
  color: var(--t2);
}

.pn-glass {
  background: var(--glass);
  border: 1px solid var(--edge);
  border-radius: 18px;
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 6%);
}

.pn-hero {
  display: grid;
  grid-template-columns: 150px 1fr;
  gap: 26px;
  align-items: center;
  margin-top: 24px;
  padding: 22px 24px;
}

.pn-ring {
  position: relative;
  width: 150px;
  height: 150px;
}

.pn-ring > svg {
  position: absolute;
  inset: 0;
  transform: rotate(-90deg);
}

.pn-g1 {
  stop-color: var(--g1);
}

.pn-g2 {
  stop-color: var(--g2);
}

.pn-ring__track {
  stroke: rgb(255 255 255 / 7%);
}

.pn-ring__down {
  stroke: var(--c-down);
}

.pn-ring__c {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.pn-ring__c > b {
  font-size: 34px;
  font-weight: 700;
  line-height: 1;
  letter-spacing: -0.02em;
  color: var(--c-ratio);
}

.pn-ring__c > span {
  margin-top: 6px;
  font-size: 11px;
  color: var(--t2);
}

.pn-big {
  min-width: 0;
}

.pn-big__lab {
  font-size: 12px;
  color: var(--t2);
}

.pn-big__v {
  margin-top: 4px;
  font-size: 44px;
  font-weight: 700;
  line-height: 1.05;
  letter-spacing: -0.02em;
  white-space: nowrap;
}

.pn-big__v small {
  margin-left: 6px;
  font-size: 18px;
  font-weight: 600;
  color: var(--t2);
}

.pn-big__row {
  display: flex;
  gap: 18px;
  margin-top: 14px;
  padding-top: 14px;
  font-size: 12px;
  color: var(--t2);
  border-top: 1px solid var(--edge);
}

.pn-big__row b {
  display: block;
  margin-top: 3px;
  font-size: 18px;
  font-weight: 700;
  white-space: nowrap;
}

.pn-legend {
  display: flex;
  gap: 14px;
  margin-top: 10px;
  font-size: 10.5px;
  color: var(--t3);
}

.pn-legend > span {
  display: inline-flex;
  gap: 5px;
  align-items: center;
}

.pn-legend i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

.pn-tiles {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  margin-top: 12px;
}

.pn-tile {
  display: flex;
  gap: 12px;
  align-items: center;
  min-width: 0;
  padding: 14px;
}

.pn-ico {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 34px;
  height: 34px;
  border-radius: 11px;
}

.pn-tile__body {
  min-width: 0;
}

.pn-tile__k {
  font-size: 11px;
  color: var(--t3);
}

.pn-tile__v {
  margin-top: 2px;
  font-size: 18px;
  font-weight: 700;
  white-space: nowrap;
}

.pn-tile__v small {
  margin-left: 2px;
  font-size: 11px;
  font-weight: 500;
  color: var(--t2);
}

.pn-inc {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  margin-top: 12px;
  padding: 10px 12px;
  font-size: 12px;
  color: var(--t2);
}

.pn-inc__k {
  margin-right: 4px;
  font-weight: 600;
  color: var(--t1);
}

.pn-chip {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  padding: 5px 10px;
  border-radius: 999px;
}

.pn-chip b {
  font-weight: 700;
}

.pn-sites {
  margin-top: 12px;
  padding: 8px 18px 6px;
}

.pn-sites__hd {
  display: flex;
  justify-content: space-between;
  padding: 8px 0 10px;
  font-size: 11px;
  color: var(--t3);
  border-bottom: 1px solid var(--edge);
}

.pn-row {
  display: grid;
  grid-template-columns: 26px minmax(0, 1fr) 84px 52px 96px;
  gap: 12px;
  align-items: center;
  padding: 11px 0;
  border-bottom: 1px solid rgb(255 255 255 / 6%);
}

.pn-row:last-child {
  border-bottom: 0;
}

.pn-row__who {
  min-width: 0;
}

.pn-row__nm {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
  font-size: 13.5px;
  font-weight: 600;
}

.pn-lv {
  min-width: 0;
  padding: 1px 7px;
  overflow: hidden;
  font-size: 10px;
  font-weight: 500;
  color: var(--t2);
  text-overflow: ellipsis;
  white-space: nowrap;
  border: 1px solid var(--edge);
  border-radius: 999px;
}

.pn-ud {
  display: flex;
  min-width: 4px;
  height: 3px;
  margin-top: 7px;
  overflow: hidden;
  border-radius: 2px;
}

.pn-ud > i {
  display: block;
}

.pn-row__v {
  font-size: 13px;
  font-weight: 700;
  text-align: right;
  white-space: nowrap;
}

.pn-row__v small {
  margin-left: 2px;
  font-size: 10px;
  font-weight: 500;
  color: var(--t3);
}

.pn-foot {
  margin-top: 20px;
  font-size: 10.5px;
  letter-spacing: 0.04em;
  color: var(--t3);
  text-align: center;
}
</style>
