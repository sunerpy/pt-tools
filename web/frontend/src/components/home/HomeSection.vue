<script setup lang="ts">
/*
 * 首页的区块标题（画板 02 的「今天播出」「继续观看」「最近入库」，07 的手机区块标题）：16/700 的标题、计数、身份 chip，
 * 右侧主色文字链（right 插槽放在它左边，NAS 概览用来写更新时间）。标题行 21 高，标题顶到内容顶 32。
 */
import PtIcon from "@/components/PtIcon";
import PtTag from "@/components/ui/PtTag.vue";
import type { RouteLocationRaw } from "vue-router";

defineProps<{
  title: string;
  /** 计数（「5 集」「今天 7」） */
  count?: string;
  /** 身份 chip（继续观看写「Jellyfin · admin」） */
  tag?: string;
  /** 右侧文字链：to 站内跳转，href 新标签页打开 */
  action?: string;
  to?: RouteLocationRaw;
  href?: string;
}>();
</script>

<template>
  <section class="hm-sec">
    <div class="hm-sec__head">
      <h2 class="hm-sec__title">{{ title }}</h2>
      <span v-if="count" class="hm-sec__count">{{ count }}</span>
      <PtTag v-if="tag" class="hm-sec__tag">{{ tag }}</PtTag>
      <span class="hm-sec__gap" />
      <slot name="right" />
      <a
        v-if="action && href"
        :href="href"
        target="_blank"
        rel="noopener noreferrer"
        class="hm-sec__link">
        {{ action }}<PtIcon name="chevron-right" :size="14" />
      </a>
      <router-link v-else-if="action && to" :to="to" class="hm-sec__link">
        {{ action }}<PtIcon name="chevron-right" :size="14" />
      </router-link>
    </div>
    <slot />
  </section>
</template>

<style scoped>
.hm-sec {
  min-width: 0;
}

.hm-sec__head {
  display: flex;
  gap: 8px;
  align-items: center;
  height: 21px;
  margin-bottom: 11px;
}

.hm-sec__title {
  margin: 0;
  font-size: var(--pt-fz-h2);
  font-weight: 700;
  line-height: 21px;
  color: var(--pt-t1);
  white-space: nowrap;
}

.hm-sec__count {
  flex: 0 0 auto;
  height: 18px;
  padding: 0 6px;
  font-size: var(--pt-fz-label);
  font-variant-numeric: tabular-nums;
  line-height: 18px;
  color: var(--pt-t3);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

.hm-sec__tag {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hm-sec__gap {
  flex: 1 1 auto;
}

.hm-sec__link {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 2px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-p);
  text-decoration: none;
}

.hm-sec__link:hover {
  text-decoration: underline;
}

.hm-sec__link:focus-visible {
  border-radius: var(--pt-r-sm);
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}
</style>
