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
  /* 放不下就换行，不许互相压住（见下面那段注释）；放得下时仍是一行 */
  flex-wrap: wrap;
  border-radius: 0;
}

/*
 * 带形态：左组吃掉剩余宽度，右组整块不收缩。
 *
 * 放得下时两组同在一行，带高 40（28 高的控件 + 上下各 6）；放不下时右组整块换到第二行靠右，
 * 左组自己也能折行。之前这里钉的是 nowrap，理由是「控件宽度自己会收缩，不会溢出」——
 * 那只在 1376 下成立：1024 下 /sites 的筛选（分段 + 搜索 + 3 枚 chip + 2 个下拉）加右组
 * 三枚图标钮要 842，左组只有 790，控件直接互相压住、列设置钮被挤没；/logs 的搜索框被压成
 * 一个图标、「自动刷新」和间隔下拉叠在一起。发布前那次 1280 / 1024 / 768 扫描抓出来的。
 *
 * 当初要防的是另一件事：1376 下左组明明放得下却在组内折成两行、把带顶到 65 ——
 * 那时左组没有 flex-grow，只拿到内容宽度的一部分。现在左组 flex: 1 1 auto 拿满剩余宽度，
 * 放得下就不会折；折行只在真的放不下时发生。
 */
@media (min-width: 769px) {
  .pt-toolbar.is-band {
    padding-block: 6px;
  }

  .pt-toolbar.is-band .pt-toolbar__side:first-child {
    flex: 1 1 auto;
  }

  .pt-toolbar.is-band .pt-toolbar__side--end {
    flex: 0 0 auto;
    margin-left: auto;
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
