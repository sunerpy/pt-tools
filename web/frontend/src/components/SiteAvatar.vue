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
  }
}
</script>

<template>
  <div
    class="site-avatar"
    :class="{ 'has-image': !!faviconUrl && !imageError, 'is-fallback': !faviconUrl || imageError }"
    :style="{
      width: size + 'px',
      height: size + 'px',
      minWidth: size + 'px',
    }">
    <img
      v-if="faviconUrl && !imageError"
      :src="faviconUrl"
      :alt="siteName"
      class="avatar-image"
      @error="handleImageError"
      @load="handleImageLoad" />
    <span
      v-else
      class="avatar-letter"
      :style="{
        fontSize: size * 0.5 + 'px',
        '--avatar-base': avatarColor,
      }">
      {{ avatarLetter }}
    </span>
  </div>
</template>
