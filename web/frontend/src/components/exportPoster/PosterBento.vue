<script setup lang="ts">
/**
 * 便当：大小不一的圆角格子拼成一张图。上传格里是逐站的上传柱（同一色相的竖向渐变、最高的那根满色，
 * 与统计页指标条的规矩一样），每项指标一个颜色；没有的指标（做种积分、周期增量、入站日期）对应的格子收起，
 * 其余的补位。
 */
import { computed } from "vue";

import PtIcon from "@/components/PtIcon";

import type { PosterModel } from "./model";
import PosterLogo from "./PosterLogo.vue";
import PosterMask from "./PosterMask.vue";

const props = defineProps<{ model: PosterModel; colorway: string }>();

/** 上传柱：最多 12 根，太多了一根根细得看不出 */
const cols = computed(() =>
  props.model.sites.slice(0, 12).map((s) => Math.max(8, s.upShare * 100)),
);
const upPct = computed(() => Math.round(props.model.upFrac * 100));

/** 「账号」格里的小项：凑成偶数，两列排得齐 */
const mini = computed(() => {
  const m = props.model;
  const items: { k: string; v: string; c: string }[] = [
    { k: "站点", v: `${m.siteCount} 个`, c: "sites" },
  ];
  if (m.join) items.push({ k: "最早入站", v: m.join, c: "join" });
  if (m.seedBonus) items.push({ k: "做种积分", v: m.seedBonus.text, c: "sb" });
  items.push({ k: "平均每站上传", v: m.avgUp.text, c: "up" });
  if (items.length % 2 === 1) items.push({ k: "做种量", v: m.seedSize.text, c: "size" });
  return items;
});
</script>

<template>
  <div class="poster poster-bento" :class="`is-${colorway}`">
    <div class="pb-head">
      <div>
        <h1 class="pb-title">{{ model.title }}</h1>
        <div class="pb-sub">{{ model.siteCount }} 个站点 · 更新于 {{ model.date }}</div>
      </div>
      <span v-if="model.joinDur" class="pb-badge">
        <PtIcon name="calendar" :size="14" />已入站 {{ model.joinDur }}
      </span>
    </div>

    <div class="pb-grid">
      <div class="pb-tile pb-s4 pb-hero">
        <div class="pb-lab">
          <PtIcon name="upload" :size="15" class="pb-ic" style="color: var(--c-up)" />总上传
        </div>
        <div class="pb-v" style="color: var(--c-up)">
          {{ model.up.value }}<small>{{ model.up.unit }}</small>
        </div>
        <div v-if="cols.length > 1" class="pb-cols">
          <i v-for="(h, i) in cols" :key="i" :style="{ height: `${h.toFixed(1)}%` }" />
        </div>
        <div v-else class="pb-hint">{{ model.siteCount }} 个站点合计</div>
      </div>

      <div class="pb-tile pb-s2">
        <div class="pb-lab">
          <PtIcon name="gauge" :size="15" class="pb-ic" style="color: var(--c-ratio)" />平均分享率
        </div>
        <div class="pb-v pb-v--ratio" style="color: var(--c-ratio)">{{ model.ratio }}</div>
        <div class="pb-split">
          <i :style="{ width: `${upPct}%`, background: 'var(--c-up)' }" />
          <i :style="{ flex: 1, background: 'var(--c-down)' }" />
        </div>
        <div class="pb-legend">
          <span>上传 {{ upPct }}%</span><span>下载 {{ 100 - upPct }}%</span>
        </div>
      </div>

      <div class="pb-tile pb-s2">
        <div class="pb-lab">
          <PtIcon
            name="download"
            :size="15"
            class="pb-ic"
            style="color: var(--c-down)" />下载（计费）
        </div>
        <div class="pb-v" style="color: var(--c-down)">
          {{ model.down.value }}<small>{{ model.down.unit }}</small>
        </div>
        <div class="pb-hint">按免费、折扣折算以后</div>
      </div>

      <div class="pb-tile pb-s2">
        <div class="pb-lab">
          <PtIcon name="coins" :size="15" class="pb-ic" style="color: var(--c-bonus)" />总魔力
        </div>
        <div class="pb-v" style="color: var(--c-bonus)">
          {{ model.bonus.value }}<small>{{ model.bonus.unit }}</small>
        </div>
        <div class="pb-hint">
          时魔 <b style="color: var(--c-bph)">{{ model.bph.text }}</b
          >/h
        </div>
      </div>

      <div class="pb-tile pb-s2">
        <div class="pb-lab">
          <PtIcon name="leaf" :size="15" class="pb-ic" style="color: var(--c-seed)" />做种
        </div>
        <div class="pb-v" style="color: var(--c-seed)">{{ model.seeding }}<small>个</small></div>
        <div class="pb-hint">
          共 <b style="color: var(--c-size)">{{ model.seedSize.text }}</b>
        </div>
      </div>

      <div v-if="model.inc" class="pb-tile pb-s3">
        <div class="pb-lab">
          <PtIcon name="trending-up" :size="15" class="pb-ic" style="color: var(--c-sb)" />
          {{ model.inc.label }}增量
        </div>
        <div class="pb-inc">
          <div>
            <span>上传</span><b style="color: var(--c-up)">{{ model.inc.up }}</b>
          </div>
          <div>
            <span>下载</span><b style="color: var(--c-down)">{{ model.inc.down }}</b>
          </div>
          <div>
            <span>魔力</span><b style="color: var(--c-bonus)">{{ model.inc.bonus }}</b>
          </div>
        </div>
      </div>

      <div class="pb-tile" :class="model.inc ? 'pb-s3' : 'pb-s6'">
        <div class="pb-lab">
          <PtIcon name="layers" :size="15" class="pb-ic" style="color: var(--c-join)" />账号
        </div>
        <div class="pb-mini" :class="{ 'is-wide': !model.inc }">
          <div v-for="it in mini" :key="it.k">
            <div class="pb-mini__k">{{ it.k }}</div>
            <div class="pb-mini__v" :style="{ color: `var(--c-${it.c})` }">{{ it.v }}</div>
          </div>
        </div>
      </div>

      <div v-if="model.sites.length" class="pb-tile pb-s6">
        <div class="pb-lab">逐站明细<span class="pb-lab__end">按上传排序</span></div>
        <div class="pb-sites">
          <div v-for="s in model.sites" :key="s.id" class="pb-site" :style="{ '--hue': s.color }">
            <div class="pb-site__top">
              <PosterLogo
                :logo="s.logo"
                :name="s.name"
                :color="s.color"
                :masked="model.mask.logos"
                :size="22"
                :radius="7" />
              <span class="pb-site__nm"><PosterMask :text="s.name" :on="model.mask.names" /></span>
              <span class="pb-site__meta">
                <PosterMask v-if="s.user" :text="s.user" :on="model.mask.users" prefix="@" />
                <span v-if="s.user && s.level"> · </span>{{ s.level }}
              </span>
            </div>
            <div class="pb-site__nums">
              <div>
                <div class="pb-site__k">上传</div>
                <div class="pb-site__n" style="color: var(--c-up)">{{ s.up.text }}</div>
              </div>
              <div>
                <div class="pb-site__k">分享率</div>
                <div class="pb-site__n" style="color: var(--c-ratio)">{{ s.ratio }}</div>
              </div>
              <div>
                <div class="pb-site__k">{{ s.bonusName }}</div>
                <div class="pb-site__n" style="color: var(--c-bonus)">
                  {{ s.bonus.value }}<small>{{ s.bonus.unit }}</small>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="pb-foot">
      <b>PT-TOOLS</b><span>生成于 {{ model.generated }}</span>
    </div>
  </div>
</template>

<style scoped>
.poster-bento {
  --t1: #101828;
  --t2: #475467;
  --t3: #98a2b3;
  --tile: #fff;
  --site: #f9fafb;
  --site-edge: #f2f4f7;
  --track: #f2f4f7;
  --tint: 13%;
  --c-up: #12b76a;
  --c-down: #f04438;
  --c-ratio: #2e90fa;
  --c-bonus: #f79009;
  --c-bph: #ef6820;
  --c-seed: #15b79e;
  --c-size: #06aed4;
  --c-sb: #66c61c;
  --c-join: #875bf7;
  --c-sites: #6172f3;
  padding: 28px 24px 22px;
  color: var(--t1);
  background:
    radial-gradient(70% 50% at 0% 0%, #e9f4ff 0%, transparent 70%),
    radial-gradient(60% 50% at 100% 100%, #f3ecff 0%, transparent 70%), #f2f4f7;
  border-radius: 26px;
}

.poster-bento.is-dark {
  --t1: #f2f4f7;
  --t2: #a3abb8;
  --t3: #6f7787;
  --tile: #1a1d24;
  --site: #20242c;
  --site-edge: #2a2f38;
  --track: #2a2f38;
  --tint: 20%;
  --c-up: #32d583;
  --c-down: #f97066;
  --c-ratio: #53b1fd;
  --c-bonus: #fdb022;
  --c-bph: #f38744;
  --c-seed: #2ed3b7;
  --c-size: #22ccee;
  --c-sb: #85e13a;
  --c-join: #9b8afb;
  --c-sites: #8098f9;
  background:
    radial-gradient(70% 50% at 0% 0%, rgb(46 144 250 / 14%) 0%, transparent 70%),
    radial-gradient(60% 50% at 100% 100%, rgb(135 91 247 / 14%) 0%, transparent 70%), #0f1115;
}

.pb-head {
  display: flex;
  gap: 16px;
  align-items: flex-end;
  justify-content: space-between;
  padding: 4px 6px 18px;
}

.pb-title {
  margin: 0;
  font-size: 27px;
  font-weight: 800;
  line-height: 1.25;
  letter-spacing: -0.01em;
  overflow-wrap: anywhere;
}

.pb-sub {
  margin-top: 6px;
  font-size: 12.5px;
  color: var(--t2);
}

.pb-badge {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 6px;
  align-items: center;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 600;
  color: var(--c-join);
  white-space: nowrap;
  background: var(--tile);
  border-radius: 999px;
  box-shadow: 0 1px 2px rgb(16 24 40 / 6%);
}

.pb-grid {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 12px;
}

.pb-tile {
  min-width: 0;
  padding: 16px 18px;
  background: var(--tile);
  border-radius: 20px;
  box-shadow:
    0 1px 2px rgb(16 24 40 / 5%),
    0 6px 18px rgb(16 24 40 / 5%);
}

.pb-s2 {
  grid-column: span 2;
}

.pb-s3 {
  grid-column: span 3;
}

.pb-s4 {
  grid-column: span 4;
}

.pb-s6 {
  grid-column: span 6;
}

.pb-lab {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: 12px;
  font-weight: 500;
  color: var(--t2);
}

.pb-ic {
  flex: 0 0 auto;
}

.pb-lab__end {
  margin-left: auto;
  font-weight: 400;
  color: var(--t3);
}

.pb-v {
  margin-top: 8px;
  font-size: 26px;
  font-weight: 800;
  line-height: 1.05;
  letter-spacing: -0.02em;
  white-space: nowrap;
}

.pb-v small {
  margin-left: 4px;
  font-size: 13px;
  font-weight: 600;
  color: var(--t3);
}

.pb-v--ratio {
  font-size: 40px;
}

.pb-hero .pb-v {
  font-size: 46px;
}

.pb-hero .pb-v small {
  font-size: 18px;
}

.pb-hint {
  margin-top: 6px;
  font-size: 11.5px;
  color: var(--t3);
}

.pb-hint b {
  font-weight: 700;
}

.pb-cols {
  display: flex;
  gap: 7px;
  align-items: flex-end;
  height: 74px;
  margin-top: 14px;
}

.pb-cols > i {
  flex: 1;
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--c-up) 55%, var(--tile)),
    color-mix(in srgb, var(--c-up) 18%, var(--tile))
  );
  border-radius: 7px 7px 3px 3px;
}

.pb-cols > i:first-child {
  background: linear-gradient(
    180deg,
    var(--c-up),
    color-mix(in srgb, var(--c-up) 55%, var(--tile))
  );
}

.pb-split {
  display: flex;
  height: 8px;
  margin-top: 14px;
  overflow: hidden;
  background: var(--track);
  border-radius: 999px;
}

.pb-split > i {
  display: block;
}

.pb-legend {
  display: flex;
  justify-content: space-between;
  margin-top: 7px;
  font-size: 11px;
  color: var(--t3);
}

.pb-inc {
  display: grid;
  gap: 8px;
  margin-top: 12px;
}

.pb-inc > div {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  font-size: 12px;
  color: var(--t2);
}

.pb-inc b {
  font-size: 16px;
  font-weight: 700;
  white-space: nowrap;
}

.pb-mini {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 10px;
  margin-top: 12px;
}

.pb-mini.is-wide {
  grid-template-columns: repeat(4, 1fr);
}

.pb-mini__k {
  font-size: 11px;
  color: var(--t3);
}

.pb-mini__v {
  margin-top: 2px;
  font-size: 16px;
  font-weight: 700;
  white-space: nowrap;
}

.pb-sites {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-top: 12px;
}

/* 逐站卡：左上角淡淡一抹站点自己的颜色 */
.pb-site {
  min-width: 0;
  padding: 12px 14px;
  overflow: hidden;
  background:
    radial-gradient(
      120% 90% at 0% 0%,
      color-mix(in srgb, var(--hue) var(--tint), transparent),
      transparent 55%
    ),
    var(--site);
  border: 1px solid var(--site-edge);
  border-radius: 14px;
}

.pb-site__top {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.pb-site__nm {
  flex: 0 0 auto;
  font-size: 13px;
  font-weight: 700;
}

.pb-site__meta {
  min-width: 0;
  margin-left: auto;
  overflow: hidden;
  font-size: 10.5px;
  color: var(--t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pb-site__nums {
  display: grid;
  grid-template-columns: 1.2fr 0.8fr 1.1fr;
  margin-top: 10px;
}

.pb-site__k {
  font-size: 10px;
  color: var(--t3);
}

.pb-site__n {
  margin-top: 1px;
  font-size: 13px;
  font-weight: 700;
  white-space: nowrap;
}

.pb-site__n small {
  margin-left: 2px;
  font-size: 10px;
  font-weight: 500;
  color: var(--t3);
}

.pb-foot {
  display: flex;
  justify-content: space-between;
  padding: 16px 6px 0;
  font-size: 10.5px;
  color: var(--t3);
}

.pb-foot b {
  font-weight: 800;
  letter-spacing: 0.08em;
  color: var(--t2);
}
</style>
