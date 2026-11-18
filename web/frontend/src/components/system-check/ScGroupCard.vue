<script setup lang="ts">
/*
 * 「全部检查项」里的一张分组卡（首页 NAS 概览 H1）：卡头是组的图标（按整组最坏的状态着色）、组名、项数与状态胶囊，
 * 下面每项一行：状态图标、标题、现在的样子（换行写全，不截断）。有失败或提醒的组，卡边带一点那个状态的颜色。
 */
import type { HealthItem } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { type CheckGroup, healthStatus } from "@/utils/systemCheck";
import { computed } from "vue";

const props = defineProps<{
  /** 整组：卡头的状态按整组算 */
  group: CheckGroup;
  /** 按筛选留下、要列出来的项 */
  items: HealthItem[];
  index: number;
}>();

const st = computed(() => healthStatus(props.group.worst));
const isBad = computed(() => props.group.worst === "fail" || props.group.worst === "warn");
const badCount = computed(
  () => props.group.items.filter((it) => it.status === props.group.worst).length,
);
const pill = computed(() =>
  isBad.value ? `${badCount.value} 项${st.value.label}` : st.value.label,
);
</script>

<template>
  <article
    :id="`sc-g-${group.key}`"
    class="sc-card sc-rv sc-lift sc-grp"
    :class="[`is-${st.tone}`, { 'is-bad': isBad }]"
    :style="{ '--sc-i': index }"
    :data-testid="`sc-group-${group.key}`">
    <div class="sc-grp__head">
      <span class="sc-grp__icon"><PtIcon :name="group.icon" :size="16" /></span>
      <div class="sc-grp__name">
        <div class="sc-grp__title">{{ group.label }}</div>
        <div class="sc-grp__n">{{ group.items.length }} 项</div>
      </div>
      <PtStatusPill :tone="st.tone" dot class="sc-grp__state">{{ pill }}</PtStatusPill>
    </div>
    <div
      v-for="it in items"
      :key="it.key"
      class="sc-row"
      :class="`st-${it.status}`"
      :data-testid="`sc-item-${it.key}`">
      <PtIcon
        :name="healthStatus(it.status).icon"
        :size="15"
        class="sc-row__icon"
        :class="`is-${healthStatus(it.status).tone}`"
        :aria-label="healthStatus(it.status).label" />
      <div class="sc-row__text">
        <div class="sc-row__title">{{ it.title }}</div>
        <div v-if="it.detail" class="sc-row__detail">{{ it.detail }}</div>
      </div>
    </div>
  </article>
</template>

<style scoped>
.sc-grp {
  overflow: hidden;
}

.sc-grp.is-bad.is-dang {
  border-color: color-mix(in srgb, var(--pt-dang) 45%, var(--pt-border));
}

.sc-grp.is-bad.is-warn {
  border-color: color-mix(in srgb, var(--pt-warn) 45%, var(--pt-border));
}

.sc-grp__head {
  display: flex;
  gap: 10px;
  align-items: center;
  min-height: 52px;
  padding: 8px 14px;
  border-bottom: 1px solid var(--pt-border);
}

.sc-grp__icon {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: var(--pt-r-md);
}

.sc-grp.is-ok .sc-grp__icon {
  color: var(--pt-ok);
  background: var(--pt-ok-soft);
}

.sc-grp.is-warn .sc-grp__icon {
  color: var(--pt-warn);
  background: var(--pt-warn-soft);
}

.sc-grp.is-dang .sc-grp__icon {
  color: var(--pt-dang);
  background: var(--pt-dang-soft);
}

.sc-grp.is-neutral .sc-grp__icon {
  color: var(--pt-t2);
  background: var(--pt-hover);
}

.sc-grp__name {
  flex: 1 1 auto;
  min-width: 0;
}

.sc-grp__title {
  font-size: var(--pt-fz-body-lg);
  font-weight: 600;
  color: var(--pt-t1);
}

.sc-grp__n {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.sc-grp__state {
  flex: 0 0 auto;
}

.sc-row {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 9px 14px;
}

.sc-row + .sc-row {
  border-top: 1px solid var(--pt-border);
}

.sc-row__icon {
  flex: 0 0 auto;
  margin-top: 2px;
}

.sc-row__icon.is-ok {
  color: var(--pt-ok);
}

.sc-row__icon.is-warn {
  color: var(--pt-warn);
}

.sc-row__icon.is-dang {
  color: var(--pt-dang);
}

.sc-row__icon.is-neutral {
  color: var(--pt-t3);
}

.sc-row__text {
  flex: 1 1 auto;
  min-width: 0;
}

.sc-row__title {
  font-size: var(--pt-fz-body);
  font-weight: 500;
  color: var(--pt-t1);
  overflow-wrap: anywhere;
}

.sc-row__detail {
  margin-top: 2px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  overflow-wrap: anywhere;
}

.sc-row.st-skip .sc-row__title {
  color: var(--pt-t3);
}
</style>
