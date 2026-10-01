<script setup lang="ts">
import { computed } from "vue";
import { useIsMobile } from "../../composables/useIsMobile";

/**
 * 页头摘要行的落点 —— 画板每一页的 `sub`（「37 个任务 · 12 下载中 · ↓93.8 MB/s」这类）。
 *
 * 为什么各页不直接 Teleport 到桌面页头里的那个 id：
 * 那个靶子长在桌面页头里，而 ≤768 时整条桌面页头是 `display:none`，
 * 于是手机上摘要**一条都看不见**，而画板 30–35 的 topbar 每一块都有这一行。
 * 移动端的靶子在 MobileChrome 的标题列里（`#pt-mhead-sub`），是另一个 DOM 节点，
 * 同一个 id 不能出现两次，所以按视口选靶子。
 *
 * 靶子随断点切换是有意的：`to` 是响应式的，跨过 768 时 Vue 会重新解析并把内容搬过去。
 * 各页自己写这个判断的话，十六个页面就要各写一遍。
 */
const isMobile = useIsMobile();

const target = computed(() => (isMobile.value ? "#pt-mhead-sub" : "#pt-head-sub"));
</script>

<template>
  <Teleport :to="target">
    <slot />
  </Teleport>
</template>
