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
    /**
     * 带形态 —— 画板 bar-64：主区里一条 40 高的全宽白带，只有下沿一条线，
     * 左右不内缩到卡片里。列表页用这个，`standalone` 是卡片网格页那种独立块。
     */
    band?: boolean;
  }>(),
  { note: "", standalone: false, band: false },
);
</script>

<template>
  <!--
    带形态同时挂上全局的 .pt-band--toolbar：画板 bar-64 里的分段、搜索框、下拉、
    按钮统统是 28 高（Element 默认 32，塞进 40 高的带里会把带顶满）。那套压高度的
    规则在 atoms.css 里，只认这个类名 —— 不挂上去的话每个页面都得自己 :deep 一遍。
  -->
  <div
    class="pt-toolbar"
    :class="{ 'is-standalone': standalone, 'is-band': band, 'pt-band--toolbar': band }">
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
/* 带形态：画板里工具栏就是主区的一条全宽带，两端不留白、不描边、不圆角 */
.pt-toolbar.is-band {
  flex-wrap: nowrap;
  border-radius: 0;
}

/*
 * 带形态里左组也不换行，并且吃掉剩余宽度。
 * 只给容器 nowrap 不够：两个 side 自己还是 wrap，而 space-between 下左组只拿到
 * 内容宽度，筛选控件一多（搜索框 320 + 下拉 180 + 一枚按钮）就在组内折成两行，
 * 40 高的带被顶到 65（已支持站点页实测）。控件宽度自己会收缩，不会溢出。
 */
@media (min-width: 769px) {
  .pt-toolbar.is-band .pt-toolbar__side {
    flex-wrap: nowrap;
  }

  .pt-toolbar.is-band .pt-toolbar__side:first-child {
    flex: 1 1 auto;
  }
}

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
