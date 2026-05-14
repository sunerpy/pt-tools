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
