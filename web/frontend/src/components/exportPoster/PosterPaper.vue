<script setup lang="ts">
/**
 * 素笺：纸底细线、衬线大数字，像一份年度报告。每项指标前一个低饱和的颜色点；逐站是一张对齐的明细表。
 * 大数字用打包进来的 Source Serif 4（只有拉丁字符），中文照旧用系统字体。
 */
import { computed } from "vue";

import type { PosterModel, PosterNumber } from "./model";
import PosterLogo from "./PosterLogo.vue";
import PosterMask from "./PosterMask.vue";

const props = defineProps<{ model: PosterModel; colorway: string }>();

interface Cell {
  c: string;
  k: string;
  v: string;
  u: string;
}

const num = (n: PosterNumber) => ({ v: n.value, u: n.unit });

/** 汇总格：每行 4 格，凑不满一行时补「平均每站上传」；只有 6 格时排成 3 列 */
const cells = computed<Cell[]>(() => {
  const m = props.model;
  const out: Cell[] = [
    { c: "down", k: "下载（计费）", ...num(m.down) },
    { c: "bonus", k: "总魔力", ...num(m.bonus) },
    { c: "bph", k: "时魔", v: m.bph.value, u: `${m.bph.unit}/h` },
  ];
  if (m.seedBonus) out.push({ c: "sb", k: "做种积分", ...num(m.seedBonus) });
  out.push(
    { c: "seed", k: "做种数", v: m.seeding, u: "" },
    { c: "size", k: "做种量", ...num(m.seedSize) },
    { c: "sites", k: "站点", v: String(m.siteCount), u: "个" },
  );
  if (m.joinDur) out.push({ c: "join", k: "入站", v: m.joinDur, u: "" });
  if (out.length === 7) out.push({ c: "up", k: "平均每站上传", ...num(m.avgUp) });
  return out;
});
const cols = computed(() => (cells.value.length % 4 === 0 ? 4 : 3));
</script>

<template>
  <div class="poster poster-paper" :class="`is-${colorway}`">
    <div class="pp-eyebrow">
      <span>PT · PERSONAL REPORT</span><span>{{ model.date }}</span>
    </div>
    <h1 class="pp-title">{{ model.title }}</h1>
    <div class="pp-sub">
      {{ model.siteCount }} 个站点<template v-if="model.join">
        · 自 {{ model.join }} 入站，已经 {{ model.joinDur }}</template
      >
    </div>
    <div class="pp-rule" />

    <div class="pp-hero">
      <div>
        <div class="pp-lab"><span class="pp-dot" style="background: var(--c-up)" />总上传</div>
        <div class="pp-v pp-num">
          {{ model.up.value }}<span class="pp-u">{{ model.up.unit }}</span>
        </div>
        <div class="pp-note">{{ model.siteCount }} 个站点合计</div>
      </div>
      <div>
        <div class="pp-lab">
          <span class="pp-dot" style="background: var(--c-ratio)" />平均分享率
        </div>
        <div class="pp-v pp-num">{{ model.ratio }}</div>
        <div class="pp-note">各站分享率的平均</div>
      </div>
    </div>

    <div class="pp-grid" :style="{ gridTemplateColumns: `repeat(${cols}, 1fr)` }">
      <div
        v-for="(cell, i) in cells"
        :key="cell.k"
        class="pp-cell"
        :class="{ 'is-first': i % cols === 0 }">
        <div class="pp-lab pp-lab--sm">
          <span class="pp-dot" :style="{ background: `var(--c-${cell.c})` }" />{{ cell.k }}
        </div>
        <div class="pp-cell__v pp-num">
          {{ cell.v }}<small v-if="cell.u">{{ cell.u }}</small>
        </div>
      </div>
    </div>

    <div v-if="model.inc" class="pp-inc">
      <span class="pp-inc__tag">{{ model.inc.label }}</span>
      <span
        >上传<b style="color: var(--c-up)">{{ model.inc.up }}</b></span
      >
      <span
        >下载<b style="color: var(--c-down)">{{ model.inc.down }}</b></span
      >
      <span
        >魔力<b style="color: var(--c-bonus)">{{ model.inc.bonus }}</b></span
      >
    </div>

    <template v-if="model.sites.length">
      <div class="pp-sec">
        <h2>逐站明细</h2>
        <span>按上传排序 · 上传 / 分享率 / 魔力</span>
      </div>
      <div v-for="(s, i) in model.sites" :key="s.id" class="pp-row">
        <span class="pp-idx pp-num">{{ String(i + 1).padStart(2, "0") }}</span>
        <PosterLogo
          :logo="s.logo"
          :name="s.name"
          :color="s.color"
          :masked="model.mask.logos"
          :size="20"
          :radius="5" />
        <div class="pp-who">
          <div class="pp-name"><PosterMask :text="s.name" :on="model.mask.names" /></div>
          <div v-if="s.user || s.level" class="pp-meta">
            <PosterMask v-if="s.user" :text="s.user" :on="model.mask.users" prefix="@" />
            <span v-if="s.user && s.level"> · </span>{{ s.level }}
          </div>
        </div>
        <div class="pp-bar"><i :style="{ width: `${(s.upShare * 100).toFixed(1)}%` }" /></div>
        <span class="pp-up pp-num">{{ s.up.text }}</span>
        <span class="pp-ratio pp-num">{{ s.ratio }}</span>
        <span class="pp-bonus pp-num"
          >{{ s.bonus.value }}<small>{{ s.bonus.unit }}{{ s.bonusName }}</small></span
        >
      </div>
    </template>

    <div class="pp-foot">
      <span class="pp-brand">PT-TOOLS</span><span>生成于 {{ model.generated }}</span>
    </div>
  </div>
</template>

<style scoped>
.poster-paper {
  --ink: #1f1b16;
  --ink2: #6f675d;
  --ink3: #a49a8d;
  --rule: #e4ddd2;
  --paper: #f7f4ee;
  --band: #efe8dc;
  --sheen: rgb(255 255 255 / 70%);
  --c-up: #4f7d5c;
  --c-down: #b5573c;
  --c-ratio: #3d5a99;
  --c-bonus: #b38327;
  --c-bph: #c26a2e;
  --c-seed: #2f7f7a;
  --c-size: #4a8f86;
  --c-sites: #5d6673;
  --c-sb: #7d8a3a;
  --c-join: #8a5a83;
  padding: 40px 44px 30px;
  color: var(--ink);
  background: radial-gradient(120% 60% at 100% 0%, var(--sheen), transparent 60%), var(--paper);
  border-radius: 6px;
}

.poster-paper.is-cool {
  --ink: #16202c;
  --ink2: #5d6878;
  --ink3: #98a2ae;
  --rule: #dde3ea;
  --paper: #f4f6f9;
  --band: #e8edf3;
  --c-up: #3f7d63;
  --c-down: #b4533f;
  --c-ratio: #3a5da5;
  --c-bonus: #a47a1f;
  --c-bph: #b8642c;
  --c-seed: #2c7a80;
  --c-size: #3f8790;
  --c-sites: #5a6472;
  --c-sb: #6f7d33;
  --c-join: #7d5785;
}

.poster-paper.is-green {
  --ink: #efe7d6;
  --ink2: rgb(239 231 214 / 72%);
  --ink3: rgb(239 231 214 / 48%);
  --rule: rgb(239 231 214 / 15%);
  --paper: #1e2b25;
  --band: rgb(239 231 214 / 8%);
  --sheen: rgb(255 255 255 / 5%);
  --c-up: #9fd3ad;
  --c-down: #f0a58e;
  --c-ratio: #a9c1f2;
  --c-bonus: #e6c27a;
  --c-bph: #f0a868;
  --c-seed: #8fd3cc;
  --c-size: #9cc9c2;
  --c-sites: #c9cfd6;
  --c-sb: #c8d48f;
  --c-join: #d7aed0;
}

.pp-num {
  font-family:
    "PT Poster Serif", "Source Serif 4", "Noto Serif SC", "Noto Serif CJK SC", "Songti SC", STSong,
    serif;
  font-weight: 500;
  letter-spacing: -0.01em;
}

.pp-eyebrow {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  font-weight: 500;
  letter-spacing: 0.26em;
  color: var(--ink3);
}

.pp-title {
  margin: 22px 0 6px;
  font-size: 30px;
  font-weight: 700;
  line-height: 1.3;
  letter-spacing: 0.01em;
  overflow-wrap: anywhere;
}

.pp-sub {
  font-size: 13px;
  color: var(--ink2);
}

.pp-rule {
  height: 1px;
  margin: 26px 0 22px;
  background: var(--rule);
}

.pp-dot {
  display: inline-block;
  flex: 0 0 auto;
  width: 7px;
  height: 7px;
  margin-right: 7px;
  border-radius: 50%;
}

.pp-lab {
  display: flex;
  align-items: center;
  font-size: 12px;
  color: var(--ink2);
}

.pp-lab--sm {
  font-size: 11px;
}

.pp-hero {
  display: grid;
  grid-template-columns: 1.35fr 1fr;
  align-items: end;
}

.pp-hero > div + div {
  padding-left: 28px;
  border-left: 1px solid var(--rule);
}

.pp-v {
  margin-top: 6px;
  font-size: 60px;
  line-height: 1;
  white-space: nowrap;
}

.pp-u {
  margin-left: 6px;
  font-family:
    "Noto Sans SC",
    -apple-system,
    BlinkMacSystemFont,
    "Segoe UI",
    "PingFang SC",
    "Microsoft YaHei",
    sans-serif;
  font-size: 20px;
  color: var(--ink2);
}

.pp-note {
  margin-top: 10px;
  font-size: 12px;
  color: var(--ink3);
}

.pp-grid {
  display: grid;
  margin-top: 28px;
  border-top: 1px solid var(--rule);
}

.pp-cell {
  min-width: 0;
  padding: 14px 0 14px 14px;
  border-bottom: 1px solid var(--rule);
  border-left: 1px solid var(--rule);
}

.pp-cell.is-first {
  padding-left: 0;
  border-left: 0;
}

.pp-cell__v {
  margin-top: 6px;
  font-size: 21px;
  line-height: 1.1;
  white-space: nowrap;
}

.pp-cell__v small {
  margin-left: 3px;
  font-family:
    "Noto Sans SC",
    -apple-system,
    BlinkMacSystemFont,
    "Segoe UI",
    "PingFang SC",
    "Microsoft YaHei",
    sans-serif;
  font-size: 11px;
  color: var(--ink2);
}

.pp-inc {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 16px;
  align-items: center;
  margin-top: 20px;
  padding: 12px 16px;
  font-size: 12.5px;
  color: var(--ink2);
  background: var(--band);
  border-radius: 10px;
}

.pp-inc__tag {
  padding: 2px 9px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  color: var(--paper);
  background: var(--ink);
  border-radius: 999px;
}

.pp-inc b {
  margin-left: 5px;
  font-weight: 600;
}

.pp-sec {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  margin: 34px 0 6px;
}

.pp-sec h2 {
  margin: 0;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.26em;
  color: var(--ink3);
}

.pp-sec span {
  font-size: 11px;
  color: var(--ink3);
}

.pp-row {
  display: grid;
  grid-template-columns: 26px 30px minmax(0, 1fr) 100px 80px 46px 92px;
  align-items: center;
  min-height: 46px;
  padding: 5px 0;
  border-bottom: 1px solid var(--rule);
}

.pp-idx {
  font-size: 13px;
  color: var(--ink3);
}

.pp-who {
  min-width: 0;
  padding-left: 0;
}

.pp-name {
  font-size: 13.5px;
  font-weight: 600;
}

.pp-meta {
  margin-top: 2px;
  overflow: hidden;
  font-size: 10.5px;
  color: var(--ink3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pp-bar {
  position: relative;
  height: 4px;
  margin: 0 14px 0 10px;
  background: var(--rule);
  border-radius: 2px;
}

.pp-bar > i {
  position: absolute;
  inset: 0 auto 0 0;
  background: var(--c-up);
  border-radius: 2px;
}

.pp-up {
  font-size: 15px;
  text-align: right;
  white-space: nowrap;
}

.pp-ratio {
  font-size: 12px;
  color: var(--c-ratio);
  text-align: right;
}

.pp-bonus {
  font-size: 12px;
  color: var(--c-bonus);
  text-align: right;
  white-space: nowrap;
}

.pp-bonus small {
  margin-left: 2px;
  font-family:
    "Noto Sans SC",
    -apple-system,
    BlinkMacSystemFont,
    "Segoe UI",
    "PingFang SC",
    "Microsoft YaHei",
    sans-serif;
  font-size: 10px;
  color: var(--ink3);
}

.pp-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 28px;
  font-size: 10.5px;
  color: var(--ink3);
}

.pp-brand {
  font-weight: 700;
  letter-spacing: 0.16em;
  color: var(--ink2);
}
</style>
