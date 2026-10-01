<script setup lang="ts">
import { computed } from "vue";
import PtIcon from "../PtIcon";

/**
 * 六种数据状态 —— 设计文档 §5 的状态表。六种必须区分开，因为「没有数据」
 * 和「筛选后没有匹配」对用户的下一步动作完全不同：前者要去添加，后者要去放宽条件。
 *
 * | key     | 图标           | 标题         |
 * |---------|----------------|--------------|
 * | loading | loader-circle  | 加载中       |
 * | empty   | inbox          | 还没有数据   |
 * | zero    | search         | 没有匹配结果 |
 * | error   | circle-alert   | 加载失败     |
 * | partial | triangle-alert | 部分数据失败 |
 * | perm    | lock           | 无权访问     |
 *
 * 副标题只是通用兜底，稿上的「添加第一个站点开始」「12 / 14 成功」这类
 * 都是页面特有文案，由调用方传 `sub` 覆盖。
 */
type StateKey = "loading" | "empty" | "zero" | "error" | "partial" | "perm";

const props = withDefaults(
  defineProps<{
    state: StateKey;
    title?: string;
    sub?: string;
    /** 表格内联用 dense，图标和留白都收一档 */
    dense?: boolean;
  }>(),
  { title: "", sub: "", dense: false },
);

const PRESETS: Record<StateKey, { icon: string; title: string; sub: string; tone: string }> = {
  loading: { icon: "loader-circle", title: "加载中", sub: "正在获取数据", tone: "t3" },
  empty: { icon: "inbox", title: "还没有数据", sub: "添加第一条记录后这里会显示内容", tone: "t3" },
  zero: { icon: "search", title: "没有匹配结果", sub: "试试放宽筛选条件", tone: "t3" },
  error: { icon: "circle-alert", title: "加载失败", sub: "请检查网络后重试", tone: "dang" },
  partial: { icon: "triangle-alert", title: "部分数据失败", sub: "已展示可用部分", tone: "warn" },
  perm: { icon: "lock", title: "无权访问", sub: "需要管理员权限", tone: "t3" },
};

const preset = computed(() => PRESETS[props.state] ?? PRESETS.empty);
</script>

<template>
  <div class="pt-state" :class="{ 'is-dense': dense }" role="status" aria-live="polite">
    <span class="pt-state__ring" :class="`is-${preset.tone}`" aria-hidden="true">
      <PtIcon
        :name="preset.icon"
        :size="dense ? 20 : 28"
        :class="{ 'is-spin': state === 'loading' }" />
    </span>
    <p class="pt-state__title">
      <slot name="title">{{ title || preset.title }}</slot>
    </p>
    <p class="pt-state__sub">
      <slot name="sub">{{ sub || preset.sub }}</slot>
    </p>
    <div v-if="$slots.action" class="pt-state__action">
      <slot name="action" />
    </div>
  </div>
</template>

<style scoped>
.pt-state {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: center;
  justify-content: center;
  padding: var(--pt-space-10) var(--pt-space-4);
  text-align: center;
}

.pt-state.is-dense {
  padding: var(--pt-space-6) var(--pt-space-4);
}

/* 图标托在一枚圆底上（稿上是 isz*1.9 的方板配 isz*0.95 圆角，即正圆） */
.pt-state__ring {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 53px;
  height: 53px;
  margin-bottom: var(--pt-space-3);
  color: var(--pt-t3);
  background: var(--pt-hover);
  border-radius: 50%;
}

.is-dense .pt-state__ring {
  width: 38px;
  height: 38px;
  margin-bottom: var(--pt-space-2);
}

.pt-state__ring.is-dang {
  color: var(--pt-dang);
  background: var(--pt-dang-soft);
}

.pt-state__ring.is-warn {
  color: var(--pt-warn);
  background: var(--pt-warn-soft);
}

/* 稿上是 14/600；字号阶梯里没有 14，取相邻的 16（区块标题），
   表格内联的 dense 场合再收回 13，免得比表头还抢眼 */
.pt-state__title {
  margin: 0;
  font-size: var(--pt-fz-h2);
  font-weight: 600;
  color: var(--pt-t1);
}

.is-dense .pt-state__title {
  font-size: var(--pt-fz-body);
}

.pt-state__sub {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.pt-state__action {
  display: flex;
  gap: var(--pt-space-2);
  margin-top: var(--pt-space-3);
}

.is-spin {
  animation: pt-state-spin 900ms linear infinite;
}

@keyframes pt-state-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .is-spin {
    animation: none;
  }
}
</style>
