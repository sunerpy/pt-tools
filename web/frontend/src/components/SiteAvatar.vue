<script setup lang="ts">
import { getAvatarColor } from "@/utils/format";
import { computed, ref } from "vue";

const props = withDefaults(
  defineProps<{
    siteName: string;
    siteId: string;
    size?: number;
    noFetch?: boolean;
  }>(),
  {
    size: 32,
    noFetch: false,
  },
);

const imageError = ref(false);
/*
 * 图标真的画出来之前一直显示首字母：之前有图标地址时只渲染 <img>，而图标要走 /api/favicon
 * 取（首次会去站点抓），加载完成前那一块是近白的空方块 —— 用户统计与站点列表里常见一排白块。
 * 现在字母常驻在下层，<img> 叠在上面，load 成功才淡入。
 */
const imageLoaded = ref(false);

const faviconUrl = computed(() => {
  if (imageError.value) return null;
  const lower = props.siteId.toLowerCase();
  const suffix = props.noFetch ? "?nofetch=1" : "";
  return `/api/favicon/${lower}${suffix}`;
});

const avatarColor = computed(() => getAvatarColor(props.siteName));

const avatarLetter = computed(() => {
  return props.siteName.charAt(0).toUpperCase();
});

function handleImageError() {
  imageError.value = true;
  imageLoaded.value = false;
}

/*
 * 后端取不到站点图标时不会报错，而是回一张 1×1 的透明占位 PNG（响应头
 * X-Favicon-Placeholder: 1）。<img> 读不到响应头，只能按尺寸认：不认的话
 * 这张图会被拉满整块，首字母兜底永远不出现 —— 已支持站点那一页几十张卡片
 * 全走 nofetch，于是全是一色的空方块。
 */
function handleImageLoad(e: Event) {
  const img = e.target as HTMLImageElement;
  if (img.naturalWidth <= 1 || img.naturalHeight <= 1) {
    imageError.value = true;
    return;
  }
  imageLoaded.value = true;
}
</script>

<template>
  <div
    class="site-avatar"
    :class="{ 'has-image': imageLoaded && !imageError, 'is-fallback': !imageLoaded || imageError }"
    :style="{
      width: size + 'px',
      height: size + 'px',
      minWidth: size + 'px',
    }">
    <span
      class="avatar-letter"
      :style="{
        fontSize: size * 0.5 + 'px',
        '--avatar-base': avatarColor,
      }">
      {{ avatarLetter }}
    </span>
    <img
      v-if="faviconUrl && !imageError"
      :src="faviconUrl"
      :alt="siteName"
      class="avatar-image"
      :class="{ 'is-loaded': imageLoaded }"
      @error="handleImageError"
      @load="handleImageLoad" />
  </div>
</template>
