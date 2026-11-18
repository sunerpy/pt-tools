<script setup lang="ts">
/*
 * 「需要处理」里的一张问题卡（首页 NAS 概览 H1）：左边状态色竖条、组名标签、这次新出现的标签、「去处理」链接，
 * 标题、现在的样子、怎么修。失败是危险色，提醒是警告色。
 */
import type { HealthItem } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtTag from "@/components/ui/PtTag.vue";
import { fixRoute, healthStatus } from "@/utils/systemCheck";
import { computed } from "vue";

const props = defineProps<{
  item: HealthItem;
  groupLabel: string;
  /** 这次体检新出现的问题 */
  isNew: boolean;
  /** 第几张，决定依次出现时的延迟 */
  index: number;
}>();

const st = computed(() => healthStatus(props.item.status));
const to = computed(() => fixRoute(props.item));
</script>

<template>
  <article
    class="sc-card sc-rv sc-lift sc-prob"
    :class="[`is-${st.tone}`, { 'sc-new': isNew }]"
    :style="{ '--sc-i': index, '--sc-tone': `var(--pt-${st.tone})` }"
    :data-testid="`sc-prob-${item.key}`">
    <div class="sc-prob__top">
      <PtIcon :name="st.icon" :size="16" class="sc-prob__icon" :aria-label="st.label" />
      <PtTag>{{ groupLabel }}</PtTag>
      <span v-if="isNew" class="sc-prob__new" data-testid="sc-prob-new">新出现</span>
      <router-link v-if="to" :to="to" class="sc-prob__go" data-testid="sc-prob-go">
        去处理<PtIcon name="chevron-right" :size="14" />
      </router-link>
    </div>
    <div class="sc-prob__title">{{ item.title }}</div>
    <div v-if="item.detail" class="sc-prob__detail">{{ item.detail }}</div>
    <div v-if="item.hint" class="sc-prob__hint">
      <PtIcon name="wrench" :size="13" class="sc-prob__hint-icon" /><span>{{ item.hint }}</span>
    </div>
  </article>
</template>

<style scoped>
.sc-prob {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 14px 16px 14px 20px;
  overflow: hidden;
}

.sc-prob::before {
  position: absolute;
  inset: 0 auto 0 0;
  width: 4px;
  content: "";
  background: var(--sc-tone);
}

.sc-prob__top {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.sc-prob__icon {
  flex: 0 0 auto;
}

.sc-prob.is-dang .sc-prob__icon {
  color: var(--pt-dang);
}

.sc-prob.is-warn .sc-prob__icon {
  color: var(--pt-warn);
}

.sc-prob__new {
  height: 20px;
  padding: 0 7px;
  font-size: var(--pt-fz-label);
  font-weight: 700;
  line-height: 20px;
  color: var(--pt-dang);
  white-space: nowrap;
  background: var(--pt-dang-soft);
  border-radius: var(--pt-r-sm);
}

.sc-prob__go {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 2px;
  align-items: center;
  margin-left: auto;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-p);
  text-decoration: none;
}

.sc-prob__go:hover {
  text-decoration: underline;
}

.sc-prob__go:focus-visible {
  border-radius: var(--pt-r-sm);
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.sc-prob__title {
  font-size: var(--pt-fz-body-lg);
  font-weight: 600;
  color: var(--pt-t1);
  overflow-wrap: anywhere;
}

.sc-prob__detail {
  font-size: var(--pt-fz-body);
  color: var(--pt-t2);
  overflow-wrap: anywhere;
}

.sc-prob__hint {
  display: flex;
  gap: 6px;
  align-items: flex-start;
  padding-top: 6px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  overflow-wrap: anywhere;
  border-top: 1px dashed var(--pt-border);
}

.sc-prob__hint-icon {
  flex: 0 0 auto;
  margin-top: 2px;
}
</style>
