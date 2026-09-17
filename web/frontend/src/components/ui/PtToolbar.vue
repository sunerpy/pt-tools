<script setup lang="ts">
/**
 * 页内工具条 —— 设计稿 G 的 `bar()`：40 高、surface 底、下沿一条发丝线。
 * 左侧放分段选择器 / 搜索框 / 筛选 chip，右侧放图标按钮组和一句说明文字。
 *
 * 移动端不再固定 40 高：筛选控件一多就必须换行，压在 40 里会被裁掉。
 */
withDefaults(
  defineProps<{
    note?: string;
    /**
     * 独立成块：卡片网格页的筛选条不在面板里，只有下沿一条发丝线会像一条断线，
     * 所以补齐四边与圆角。默认 false = 贴在 PtPanel 页头下方。
     */
    standalone?: boolean;
  }>(),
  { note: "", standalone: false },
);
</script>

<template>
  <div class="pt-toolbar" :class="{ 'is-standalone': standalone }">
    <div class="pt-toolbar__side">
      <slot />
    </div>
    <div class="pt-toolbar__side pt-toolbar__side--end">
      <span v-if="note || $slots.note" class="pt-toolbar__note">
        <slot name="note">{{ note }}</slot>
      </span>
      <slot name="right" />
    </div>
  </div>
</template>

<style scoped>
.pt-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2) var(--pt-space-3);
  align-items: center;
  justify-content: space-between;
  min-height: var(--pt-toolbar-h);
  padding: 0 var(--pt-pad);
  background: var(--pt-surface);
  border-bottom: 1px solid var(--pt-border);
}

.pt-toolbar.is-standalone {
  padding: var(--pt-space-2) var(--pt-pad);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

.pt-toolbar__side {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
  align-items: center;
  min-width: 0;
}

.pt-toolbar__side--end {
  justify-content: flex-end;
}

.pt-toolbar__note {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

@media (max-width: 768px) {
  .pt-toolbar {
    gap: var(--pt-space-2);
    padding: var(--pt-space-2) var(--pt-space-3);
  }

  /* 手机上左右两组各占一行，右组也左对齐，免得说明文字被挤成竖排 */
  .pt-toolbar__side {
    flex: 1 1 100%;
  }

  .pt-toolbar__side--end {
    justify-content: flex-start;
  }
}
</style>
