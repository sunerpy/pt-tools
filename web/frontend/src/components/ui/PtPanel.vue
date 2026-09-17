<script setup lang="ts">
import { computed, useSlots } from "vue";
import PtIcon from "../PtIcon";

/**
 * 面板卡片 —— 设计稿 G 的 `panel()`，即 `k.card` 的 G 预设：
 * surface 底 + 1px border + 圆角 8（rLg）+ 一层浅阴影，页头 42 高，标题 13/600。
 *
 * 这是 G 方向上所有内容容器的唯一外框，取代旧的 `.table-card`/`.form-card`。
 * 表格类面板传 `padding="none"`，让网格自己贴边；表单类用默认的 16 内边距。
 */
const props = withDefaults(
  defineProps<{
    title?: string;
    /** 标题左侧的 lucide 图标名 */
    icon?: string;
    /** 标题右侧的计数徽标，0 也会显示，不需要时不传 */
    count?: number | string;
    /** 右上角文字操作；需要更复杂的操作区就用 actions 插槽 */
    action?: string;
    actionIcon?: string;
    /** 正文内边距：md = 16（默认），none = 贴边（表格），lg = 20 */
    padding?: "none" | "md" | "lg";
    /** 关掉阴影，用于嵌套在别的面板里 */
    flat?: boolean;
  }>(),
  { title: "", icon: "", count: undefined, action: "", actionIcon: "", padding: "md", flat: false },
);

const emit = defineEmits<{ action: [] }>();

const slots = useSlots();

const hasHead = computed(
  () => Boolean(props.title) || Boolean(slots.title) || Boolean(slots.actions),
);

const hasCount = computed(() => props.count !== undefined && props.count !== "");
</script>

<template>
  <section class="pt-panel" :class="{ 'is-flat': flat }">
    <header v-if="hasHead" class="pt-panel__head">
      <PtIcon v-if="icon" :name="icon" :size="16" class="pt-panel__icon" />
      <h2 class="pt-panel__title">
        <slot name="title">{{ title }}</slot>
      </h2>
      <span v-if="hasCount" class="pt-panel__count">{{ count }}</span>
      <span class="pt-panel__gap" />
      <slot name="actions">
        <button v-if="action" type="button" class="pt-panel__action" @click="emit('action')">
          <PtIcon v-if="actionIcon" :name="actionIcon" :size="14" />
          <span>{{ action }}</span>
        </button>
      </slot>
    </header>

    <div class="pt-panel__body" :class="`is-pad-${padding}`">
      <slot />
    </div>

    <footer v-if="$slots.footer" class="pt-panel__foot">
      <slot name="footer" />
    </footer>
  </section>
</template>

<style scoped>
.pt-panel {
  display: flex;
  flex-direction: column;
  min-width: 0;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: var(--pt-shadow-sm);
}

.pt-panel.is-flat {
  box-shadow: none;
}

.pt-panel__head {
  display: flex;
  flex: 0 0 auto;
  gap: 9px;
  align-items: center;
  height: 42px;
  padding: 0 var(--pt-pad);
  border-bottom: 1px solid var(--pt-border);
}

.pt-panel__icon {
  flex: 0 0 auto;
  color: var(--pt-t3);
}

.pt-panel__title {
  min-width: 0;
  margin: 0;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pt-panel__count {
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

/* 标题与右侧操作之间的弹性空隙，用元素而不是 margin-left:auto，
   这样 actions 插槽塞进来的多个按钮仍然靠右成组 */
.pt-panel__gap {
  flex: 1 1 auto;
}

.pt-panel__action {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 4px;
  align-items: center;
  height: 26px;
  padding: 0 8px;
  font-family: inherit;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-p);
  background: transparent;
  border: 0;
  border-radius: var(--pt-r-sm);
  cursor: pointer;
  transition: background var(--pt-transition-fast);
}

.pt-panel__action:hover {
  background: var(--pt-p-soft);
}

.pt-panel__body {
  flex: 1 1 auto;
  min-width: 0;
}

.pt-panel__body.is-pad-md {
  padding: var(--pt-pad);
}

.pt-panel__body.is-pad-lg {
  padding: var(--pt-space-5);
}

.pt-panel__foot {
  display: flex;
  flex: 0 0 auto;
  gap: var(--pt-space-2);
  align-items: center;
  flex-wrap: wrap;
  min-height: 34px;
  /* 上下 6：纯文字页脚仍是 34 高（min-height 胜出），塞进 32 高的按钮时长到 44 */
  padding: 6px var(--pt-pad);
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  border-top: 1px solid var(--pt-border);
}

/* 正文贴边时，圆角要靠面板自己裁掉，否则表格首行会盖住左上角。
   有页脚时下方圆角交给页脚，所以只在正文是最后一个孩子时才裁下沿 */
.pt-panel__body.is-pad-none {
  overflow: hidden;
}

.pt-panel__body.is-pad-none:last-child {
  border-radius: 0 0 calc(var(--pt-r-lg) - 1px) calc(var(--pt-r-lg) - 1px);
}
</style>
