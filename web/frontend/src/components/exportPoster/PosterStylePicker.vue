<script setup lang="ts">
/**
 * 导出页的「样式」选择：五个样式各一张小缩略图（用几块色块示意版式，不是真的海报），下面是选中样式的配色。
 */
import { computed } from "vue";

import PtIcon from "@/components/PtIcon";

import { POSTER_STYLES, posterColorway, posterStyle, type PosterStyleId } from "./styles";

const props = defineProps<{ styleId: PosterStyleId; colorway: string }>();
const emit = defineEmits<{
  "update:styleId": [PosterStyleId];
  "update:colorway": [string];
}>();

const current = computed(() => posterStyle(props.styleId));
const currentColorway = computed(() => posterColorway(current.value, props.colorway).id);
</script>

<template>
  <div class="psp">
    <div class="psp-thumbs" role="radiogroup" aria-label="海报样式">
      <button
        v-for="s in POSTER_STYLES"
        :key="s.id"
        type="button"
        role="radio"
        class="psp-thumb"
        :class="{ 'is-on': s.id === styleId }"
        :aria-checked="s.id === styleId"
        :title="s.hint"
        :data-testid="`poster-style-${s.id}`"
        @click="emit('update:styleId', s.id)">
        <span class="psp-art" :class="`psp-art--${s.id}`" aria-hidden="true">
          <template v-if="s.id === 'bento'">
            <i class="a1"><b /><b /><b /><b /></i><i class="a2" /><i class="a3" /><i class="a4" />
          </template>
          <template v-else-if="s.id === 'paper'">
            <i class="a1">8.04</i><i class="a2" /><i class="a3" /><i class="a4" />
          </template>
          <template v-else-if="s.id === 'night'"
            ><i class="a1" /><i class="a2" /><i class="a3"
          /></template>
          <template v-else-if="s.id === 'wrapped'"
            ><i class="a1">186</i><i class="a2" /><i class="a3"
          /></template>
          <template v-else
            ><i class="a1"><b /></i
          ></template>
        </span>
        <span class="psp-name">{{ s.name }}</span>
        <PtIcon v-if="s.id === styleId" name="check" :size="12" class="psp-check" />
      </button>
    </div>
    <div class="psp-hint">{{ current.hint }}</div>
    <div class="psp-ways" role="radiogroup" aria-label="配色">
      <button
        v-for="c in current.colorways"
        :key="c.id"
        type="button"
        role="radio"
        class="psp-way"
        :class="{ 'is-on': c.id === currentColorway }"
        :aria-checked="c.id === currentColorway"
        :data-testid="`poster-colorway-${c.id}`"
        @click="emit('update:colorway', c.id)">
        <span
          class="psp-dot"
          :style="{
            background: `linear-gradient(135deg, ${c.swatch[0]} 50%, ${c.swatch[1]} 50%)`,
          }" />
        {{ c.name }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.psp {
  display: flex;
  flex-direction: column;
}

.psp-thumbs {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: var(--pt-space-2);
}

.psp-thumb {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: center;
  min-width: 0;
  padding: 4px 4px 6px;
  font: inherit;
  color: var(--pt-t2);
  cursor: pointer;
  background: transparent;
  border: 1.5px solid var(--pt-border);
  border-radius: var(--pt-r-md);
  transition:
    border-color var(--pt-transition-fast),
    transform var(--pt-transition-fast);
}

.psp-thumb:hover {
  border-color: var(--pt-t3);
  transform: translateY(-1px);
}

.psp-thumb:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.psp-thumb.is-on {
  color: var(--pt-t1);
  border-color: var(--pt-p);
}

.psp-check {
  position: absolute;
  top: 6px;
  right: 6px;
  padding: 1px;
  color: var(--pt-on-p);
  background: var(--pt-p);
  border-radius: 50%;
}

.psp-name {
  font-size: var(--pt-fz-sm);
  line-height: 1.2;
}

.psp-hint {
  margin-top: var(--pt-space-2);
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.psp-ways {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
  margin-top: var(--pt-space-3);
}

.psp-way {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  padding: 4px 10px 4px 6px;
  font: inherit;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  cursor: pointer;
  background: transparent;
  border: 1px solid var(--pt-border);
  border-radius: 999px;
}

.psp-way:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.psp-way.is-on {
  color: var(--pt-t1);
  background: var(--pt-p-soft);
  border-color: var(--pt-p);
}

.psp-dot {
  width: 14px;
  height: 14px;
  border: 1px solid rgb(0 0 0 / 12%);
  border-radius: 50%;
}

/* ── 缩略图：几块色块示意版式，颜色是各样式的默认配色（图片内容，不跟主题） ── */
.psp-art {
  position: relative;
  display: block;
  width: 100%;
  aspect-ratio: 3 / 4;
  overflow: hidden;
  border-radius: 6px;
}

.psp-art i {
  position: absolute;
  display: block;
  font-style: normal;
}

.psp-art--bento {
  background: #eef1f5;
}

.psp-art--bento i {
  background: #fff;
  border-radius: 4px;
}

.psp-art--bento .a1 {
  inset: 10% 8% 52% 8%;
  display: flex;
  gap: 2px;
  align-items: flex-end;
  padding: 4px;
}

.psp-art--bento .a1 b {
  flex: 1;
  background: #a6f4c5;
  border-radius: 2px 2px 1px 1px;
}

.psp-art--bento .a1 b:nth-child(1) {
  height: 90%;
  background: #12b76a;
}

.psp-art--bento .a1 b:nth-child(2) {
  height: 62%;
}

.psp-art--bento .a1 b:nth-child(3) {
  height: 44%;
}

.psp-art--bento .a1 b:nth-child(4) {
  height: 28%;
}

.psp-art--bento .a2 {
  inset: 52% 54% 26% 8%;
  border-top: 3px solid #2e90fa;
}

.psp-art--bento .a3 {
  inset: 52% 8% 26% 50%;
  border-top: 3px solid #f79009;
}

.psp-art--bento .a4 {
  inset: 78% 8% 8% 8%;
}

.psp-art--paper {
  background: #f7f4ee;
  border: 1px solid #e4ddd2;
}

.psp-art--paper .a1 {
  top: 16%;
  left: 12%;
  font-family: "PT Poster Serif", "Source Serif 4", Georgia, serif;
  font-size: 15px;
  line-height: 1;
  color: #1f1b16;
}

.psp-art--paper .a2,
.psp-art--paper .a3,
.psp-art--paper .a4 {
  left: 12%;
  right: 12%;
  height: 1px;
  background: #d6ccbc;
}

.psp-art--paper .a2 {
  top: 48%;
}

.psp-art--paper .a3 {
  top: 64%;
}

.psp-art--paper .a4 {
  top: 80%;
}

.psp-art--night {
  background:
    radial-gradient(70% 50% at 0% 0%, rgb(45 212 191 / 35%), transparent 70%),
    radial-gradient(70% 50% at 100% 100%, rgb(139 92 246 / 40%), transparent 70%), #0a1020;
}

.psp-art--night .a1 {
  top: 14%;
  left: 50%;
  width: 46%;
  aspect-ratio: 1;
  background: conic-gradient(#34d399 0 300deg, #fb7185 300deg 360deg);
  border-radius: 50%;
  transform: translateX(-50%);
  mask: radial-gradient(closest-side, transparent 64%, #000 66%);
}

.psp-art--night .a2,
.psp-art--night .a3 {
  left: 12%;
  right: 12%;
  height: 12%;
  background: rgb(255 255 255 / 8%);
  border: 1px solid rgb(255 255 255 / 12%);
  border-radius: 4px;
}

.psp-art--night .a2 {
  top: 58%;
}

.psp-art--night .a3 {
  top: 75%;
}

.psp-art--wrapped {
  background: linear-gradient(160deg, #6d28d9 0%, #be185d 50%, #ea580c 100%);
}

.psp-art--wrapped .a1 {
  top: 22%;
  left: 12%;
  font-size: 17px;
  font-weight: 900;
  line-height: 1;
  color: #fff;
  letter-spacing: -0.04em;
}

.psp-art--wrapped .a2 {
  top: 54%;
  left: 12%;
  right: 30%;
  height: 4px;
  background: rgb(255 255 255 / 70%);
  border-radius: 2px;
}

.psp-art--wrapped .a3 {
  left: 12%;
  right: 12%;
  bottom: 10%;
  height: 20%;
  background: rgb(255 255 255 / 92%);
  border-radius: 4px;
}

.psp-art--receipt {
  background: #20232a;
}

.psp-art--receipt .a1 {
  inset: 10% 18%;
  background: repeating-linear-gradient(180deg, #fbfaf6 0 7px, #d9d9de 7px 8px) #fbfaf6;
  border-radius: 1px;
}

.psp-art--receipt .a1 b {
  position: absolute;
  top: 10%;
  right: -10%;
  width: 46%;
  aspect-ratio: 1;
  border: 2px solid #d33a2c;
  border-radius: 50%;
  transform: rotate(-14deg);
}
</style>
