<script setup lang="ts">
import { getAvatarColor } from "@/utils/format";
import { computed, ref } from "vue";

const props = withDefaults(
  defineProps<{
    siteName: string;
    siteId: string;
    size?: number;
  }>(),
  {
    size: 32,
  },
);

const imageError = ref(false);

// 使用后端缓存 API 获取站点图标
// 后端会自动缓存图标到本地，避免每次都从远程请求
const faviconUrl = computed(() => {
  if (imageError.value) return null;
  const lower = props.siteId.toLowerCase();
  // 使用后端 favicon 缓存 API
  return `/api/favicon/${lower}`;
});

const avatarColor = computed(() => getAvatarColor(props.siteName));

const avatarLetter = computed(() => {
  return props.siteName.charAt(0).toUpperCase();
});

function handleImageError() {
  imageError.value = true;
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
      @error="handleImageError" />
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
