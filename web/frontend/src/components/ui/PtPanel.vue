<script setup lang="ts">
import { computed, useSlots } from "vue";
import PtIcon from "../PtIcon";

/**
 * 面板卡片 —— 设计稿 G 的 `panel()`，即 `k.card` 的 G 预设：
 * surface 底 + 1px border + 圆角 8（rLg）+ 一层浅阴影，卡头 29 高，标题 13/600。
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

/*
 * 画板「卡片 p-*」只给卡头 29 高，且 head-rule 正好落在 29：全局 box-sizing 是
 * border-box，所以 height:29 + border-bottom:1 之后内容区剩 28，发丝线压在第 29 行。
 * 28 的内容区靠 align-items:center 分配上下空隙，所以不给纵向内边距：实测 16 的图标
 * 上下各 6，13/600 的标题行盒 17（13px × normal 行高）上下各 5.5，18 的计数徽标各 5。
 * 16 的图标是卡头里最高的固定尺寸元素，它那 6 就是这里能有的最大余量，所以下面
 * 把 actions 塞进来的控件压到 22 —— 那是还剩得下上下各 3 的上限。
 */
.pt-panel__head {
  display: flex;
  flex: 0 0 auto;
  gap: 9px;
  align-items: center;
  height: 29px;
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

/*
 * 画板把卡头右侧动作定为 12/500 的强调色文字链，不是胶囊按钮：26 高的胶囊在 28 的
 * 内容区里上下只剩 1px，hover 底会贴住发丝线。所以这里不给高度、不给 hover 底，
 * 悬停时用下划线做反馈（只靠变色对弱视用户不够），键盘焦点仍走焦点环。
 */
.pt-panel__action {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 4px;
  align-items: center;
  padding: 0;
  font-family: inherit;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-p);
  background: transparent;
  border: 0;
  cursor: pointer;
  transition: color var(--pt-transition-fast);
}

.pt-panel__action:hover {
  text-decoration: underline;
}

.pt-panel__action:focus-visible {
  border-radius: var(--pt-r-sm);
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

/*
 * actions 插槽塞进来的按钮统一压到 22（28 的内容区上下各留 3）。Element 的 small
 * 按钮是 24 高，只剩 2px 就顶满了；各页面自己写的图标钮多是 26，只剩 1px，hover
 * 底会贴住 29 的发丝线。压在 PtPanel 这一处，二十多个 view 就不必各写一遍。
 * 排除 .pt-panel__action：它已经是不带高度的文字链。
 * padding 只给 el-button：图标钮靠固定宽居中，横向内边距会把图标挤出去。
 */
.pt-panel__head :deep(.el-button),
.pt-panel__head :deep(button:not(.pt-panel__action)) {
  height: 22px;
  min-height: 22px;
  margin: 0;
  font-size: var(--pt-fz-sm);
}

.pt-panel__head :deep(.el-button) {
  padding: 0 8px;
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
