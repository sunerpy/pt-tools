<script setup lang="ts">
/**
 * 小票：深色桌面上一张锯齿边的热敏小票，等宽字、点线引导，平均分享率盖一个红章，底下一条条形码
 * （只是装饰）。等宽字用打包进来的 JetBrains Mono（只有拉丁字符），中文照旧用系统字体。
 */
import { computed } from "vue";
import { usePosterWords } from "./locale";

import type { PosterModel } from "./model";
import PosterLogo from "./PosterLogo.vue";
import PosterMask from "./PosterMask.vue";

const props = defineProps<{ model: PosterModel; colorway: string }>();
const t = usePosterWords();

const PERIOD_EN: Record<string, string> = {
  今日: "TODAY",
  本周: "THIS WEEK",
  本月: "THIS MONTH",
};
const incEn = computed(() => PERIOD_EN[props.model.inc?.label ?? ""] ?? "");
</script>

<template>
  <div class="poster poster-receipt" :class="`is-${colorway}`">
    <div class="pr-shadow">
      <div class="pr-ticket">
        <div class="pr-c">
          <div class="pr-brand">PT-TOOLS</div>
          <div class="pr-ttl">{{ model.title }}</div>
          <div class="pr-meta">No.{{ model.serial }} · {{ model.generated }}</div>
        </div>
        <div class="pr-stamp">
          <b>{{ model.ratio }}</b
          ><span>AVG RATIO</span>
        </div>

        <div class="pr-cut" />
        <div class="pr-li">
          <span class="pr-k">{{ t("站点") }}</span
          ><span class="pr-dots" /><span class="pr-v">{{ model.siteCount }}{{ t(" 个") }}</span>
        </div>
        <div v-if="model.join" class="pr-li">
          <span class="pr-k">{{ t("入站") }}</span
          ><span class="pr-dots" />
          <span class="pr-v">{{ model.joinDur }}（{{ model.join }}{{ t(" 起）") }}</span>
        </div>

        <div class="pr-cut" />
        <div class="pr-li is-big">
          <span class="pr-k">{{ t("总上传") }}</span
          ><span class="pr-dots" />
          <span class="pr-v" style="color: var(--c-up)">{{ model.up.text }}</span>
        </div>
        <div class="pr-li is-big">
          <span class="pr-k">{{ t("下载（计费）") }}</span
          ><span class="pr-dots" />
          <span class="pr-v" style="color: var(--c-down)">{{ model.down.text }}</span>
        </div>
        <div class="pr-li is-big">
          <span class="pr-k">{{ t("平均分享率") }}</span
          ><span class="pr-dots" />
          <span class="pr-v" style="color: var(--c-ratio)">{{ model.ratio }}</span>
        </div>

        <div class="pr-cut" />
        <div class="pr-li">
          <span class="pr-k">{{ t("总魔力") }}</span
          ><span class="pr-dots" />
          <span class="pr-v" style="color: var(--c-bonus)">{{ model.bonus.text }}</span>
        </div>
        <div class="pr-li">
          <span class="pr-k">{{ t("时魔") }}</span
          ><span class="pr-dots" />
          <span class="pr-v" style="color: var(--c-bonus)">{{ model.bph.text }}/h</span>
        </div>
        <div v-if="model.seedBonus" class="pr-li">
          <span class="pr-k">{{ t("做种积分") }}</span
          ><span class="pr-dots" />
          <span class="pr-v">{{ model.seedBonus.text }}</span>
        </div>
        <div class="pr-li">
          <span class="pr-k">{{ t("做种") }}</span
          ><span class="pr-dots" />
          <span class="pr-v">{{ model.seeding }}{{ t(" 个 / ") }}{{ model.seedSize.text }}</span>
        </div>

        <template v-if="model.inc">
          <div class="pr-cut" />
          <div class="pr-sec">
            {{ model.inc.label }}<template v-if="incEn"> · {{ incEn }}</template>
          </div>
          <div class="pr-li">
            <span class="pr-k">{{ t("上传") }}</span
            ><span class="pr-dots" />
            <span class="pr-v" style="color: var(--c-up)">{{ model.inc.up }}</span>
          </div>
          <div class="pr-li">
            <span class="pr-k">{{ t("下载") }}</span
            ><span class="pr-dots" />
            <span class="pr-v" style="color: var(--c-down)">{{ model.inc.down }}</span>
          </div>
          <div class="pr-li">
            <span class="pr-k">{{ t("魔力") }}</span
            ><span class="pr-dots" />
            <span class="pr-v" style="color: var(--c-bonus)">{{ model.inc.bonus }}</span>
          </div>
        </template>

        <template v-if="model.sites.length">
          <div class="pr-dbl" />
          <div class="pr-sec">{{ t("逐站明细 · ") }}{{ model.sites.length }} ITEMS</div>
          <div v-for="(s, i) in model.sites" :key="s.id" class="pr-site">
            <div class="pr-site__l1">
              <span class="pr-site__i">{{ String(i + 1).padStart(2, "0") }}</span>
              <PosterLogo
                :logo="s.logo"
                :name="s.name"
                :color="s.color"
                :masked="model.mask.logos"
                :size="16"
                :radius="4" />
              <span class="pr-site__n"><PosterMask :text="s.name" :on="model.mask.names" /></span>
              <span v-if="s.user || s.level" class="pr-site__lv">
                <PosterMask v-if="s.user" :text="s.user" :on="model.mask.users" prefix="@" />
                <span v-if="s.user && s.level"> · </span>{{ s.level }}
              </span>
            </div>
            <div class="pr-site__l2">
              <span
                >↑ <b style="color: var(--c-up)">{{ s.up.text }}</b></span
              >
              <span
                >↓ <b style="color: var(--c-down)">{{ s.down.text }}</b></span
              >
              <span
                >R <b style="color: var(--c-ratio)">{{ s.ratio }}</b></span
              >
              <span
                >{{ s.bonusName }} <b style="color: var(--c-bonus)">{{ s.bonus.text }}</b></span
              >
            </div>
          </div>
        </template>

        <div class="pr-dbl" />
        <div class="pr-barcode" />
        <div class="pr-c pr-thanks">{{ t("感谢做种 · THANK YOU FOR SEEDING") }}</div>
      </div>
    </div>
    <div class="pr-foot">Generated by pt-tools</div>
  </div>
</template>

<style scoped>
.poster-receipt {
  --paper: #fbfaf6;
  --ink: #1d1d1f;
  --ink2: #5f5f66;
  --ink3: #9a9aa2;
  --dots: #c4c4ca;
  --cut: #cfcfd4;
  --line: #e4e4e8;
  --stamp: #d33a2c;
  --c-up: #0f8a4b;
  --c-down: #c2352b;
  --c-ratio: #1d5fc4;
  --c-bonus: #b06e00;
  --mono:
    "PT Poster Mono", "JetBrains Mono", "Noto Sans Mono", ui-monospace, SFMono-Regular, Menlo,
    Consolas, "Noto Sans Mono CJK SC", monospace;
  padding: 34px 54px 40px;
  background: radial-gradient(80% 50% at 50% 0%, rgb(255 255 255 / 7%), transparent 70%), #20232a;
  border-radius: 22px;
}

.poster-receipt.is-kraft {
  --paper: #e3cfa9;
  --ink: #3b2a17;
  --ink2: #6b543a;
  --ink3: #8f7758;
  --dots: #b39a74;
  --cut: #c2a77e;
  --line: #d3bc95;
  --stamp: #9b2c1f;
  --c-up: #2f6b3a;
  --c-down: #9b2c1f;
  --c-ratio: #2b4f8a;
  --c-bonus: #8a5a00;
  background: radial-gradient(80% 50% at 50% 0%, rgb(255 236 200 / 8%), transparent 70%), #2a2118;
}

/* 阴影放在外层：锯齿边是 mask 切出来的，阴影要是画在同一个元素上会被一起切掉 */
.pr-shadow {
  filter: drop-shadow(0 14px 22px rgb(0 0 0 / 35%));
}

.pr-ticket {
  position: relative;
  padding: 30px 30px 26px;
  font-family: var(--mono);
  font-size: 12.5px;
  color: var(--ink);
  background: var(--paper);
  /* 上下两条锯齿边 */
  -webkit-mask:
    conic-gradient(from -45deg at bottom, #0000, #000 1deg 89deg, #0000 90deg) bottom / 14px 51%
      repeat-x,
    conic-gradient(from 135deg at top, #0000, #000 1deg 89deg, #0000 90deg) top / 14px 51% repeat-x;
  mask:
    conic-gradient(from -45deg at bottom, #0000, #000 1deg 89deg, #0000 90deg) bottom / 14px 51%
      repeat-x,
    conic-gradient(from 135deg at top, #0000, #000 1deg 89deg, #0000 90deg) top / 14px 51% repeat-x;
}

.pr-c {
  text-align: center;
}

.pr-brand {
  font-size: 22px;
  font-weight: 700;
  letter-spacing: 0.32em;
}

.pr-ttl {
  margin: 8px 110px 0;
  font-family:
    "Noto Sans SC",
    -apple-system,
    BlinkMacSystemFont,
    "Segoe UI",
    "PingFang SC",
    "Microsoft YaHei",
    sans-serif;
  font-size: 15px;
  font-weight: 700;
  overflow-wrap: anywhere;
}

.pr-meta {
  margin-top: 6px;
  font-size: 11px;
  color: var(--ink2);
}

.pr-cut {
  margin: 16px 0;
  border-top: 1.5px dashed var(--cut);
}

.pr-dbl {
  margin: 14px 0;
  border-top: 3px double var(--ink);
}

.pr-li {
  display: flex;
  gap: 6px;
  align-items: baseline;
  padding: 4px 0;
}

.pr-k {
  flex: 0 0 auto;
  font-family:
    "Noto Sans SC",
    -apple-system,
    BlinkMacSystemFont,
    "Segoe UI",
    "PingFang SC",
    "Microsoft YaHei",
    var(--mono);
  font-size: 12.5px;
}

.pr-dots {
  flex: 1;
  min-width: 12px;
  border-bottom: 1.5px dotted var(--dots);
  transform: translateY(-3px);
}

.pr-v {
  font-weight: 700;
  white-space: nowrap;
}

.pr-li.is-big .pr-k {
  font-size: 14px;
  font-weight: 700;
}

.pr-li.is-big .pr-v {
  font-size: 18px;
}

.pr-sec {
  margin: 2px 0 8px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.2em;
  color: var(--ink2);
}

.pr-site {
  padding: 7px 0;
  border-bottom: 1px dashed var(--line);
}

.pr-site:last-child {
  border-bottom: 0;
}

.pr-site__l1 {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.pr-site__i {
  color: var(--ink3);
}

.pr-site__n {
  flex: 0 0 auto;
  font-weight: 700;
}

.pr-site__lv {
  min-width: 0;
  margin-left: auto;
  overflow: hidden;
  font-size: 10.5px;
  color: var(--ink3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pr-site__l2 {
  display: grid;
  grid-template-columns: 1.15fr 1fr 0.7fr 1.2fr;
  margin-top: 4px;
  padding-left: 26px;
  font-size: 11.5px;
  color: var(--ink2);
  white-space: nowrap;
}

.pr-site__l2 b {
  font-weight: 700;
}

.pr-stamp {
  position: absolute;
  top: 14px;
  right: 14px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 96px;
  height: 96px;
  color: color-mix(in srgb, var(--stamp) 86%, transparent);
  border: 3px solid currentColor;
  border-radius: 50%;
  transform: rotate(-14deg);
}

.pr-stamp::before {
  position: absolute;
  inset: 4px;
  content: "";
  border: 1px solid currentColor;
  border-radius: 50%;
}

.pr-stamp > b {
  font-size: 24px;
  font-weight: 700;
  line-height: 1;
}

.pr-stamp > span {
  margin-top: 4px;
  font-size: 8.5px;
  font-weight: 700;
  letter-spacing: 0.16em;
}

.pr-barcode {
  height: 46px;
  margin: 18px 10px 8px;
  background: repeating-linear-gradient(
    90deg,
    var(--ink) 0 2px,
    transparent 2px 4px,
    var(--ink) 4px 5px,
    transparent 5px 8px,
    var(--ink) 8px 11px,
    transparent 11px 12px,
    var(--ink) 12px 13px,
    transparent 13px 16px
  );
}

.pr-thanks {
  font-size: 11px;
  letter-spacing: 0.18em;
  color: var(--ink2);
}

.pr-foot {
  margin-top: 18px;
  font-size: 10.5px;
  color: rgb(255 255 255 / 42%);
  text-align: center;
}
</style>
