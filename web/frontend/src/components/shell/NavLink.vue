<script setup lang="ts">
import { computed } from "vue";
import { RouterLink, useRouter } from "vue-router";
import type { NavItem } from "../../config/navigation";
import PtIcon from "../PtIcon";

/**
 * 导航列里的一行。
 *
 * external 的项（下载器 Web UI）渲染成真正的 <a target="_blank">，不是
 * router-link + preventDefault —— router-link 自己的 click 处理器排在
 * 外部处理器之前，preventDefault 拦不住它，而且真 <a> 还白拿中键、
 * Ctrl+点击这些浏览器原生行为。
 */
const props = defineProps<{ item: NavItem; active: boolean; badge?: number | null }>();
const emit = defineEmits<{ navigate: [] }>();

const router = useRouter();

const href = computed(() => router.resolve(props.item.path).href);

const bindings = computed(() =>
  props.item.external
    ? { href: href.value, target: "_blank", rel: "noopener" }
    : { to: props.item.path, ariaCurrent: props.active ? "page" : undefined },
);
</script>

<template>
  <component
    :is="item.external ? 'a' : RouterLink"
    class="pt-nav__item"
    :class="{ 'is-active': active }"
    v-bind="bindings"
    @click="emit('navigate')">
    <PtIcon :name="item.icon" :size="15" />
    <span class="pt-nav__item-label">{{ item.label }}</span>
    <span v-if="badge !== null && badge !== undefined" class="pt-nav__badge">{{ badge }}</span>
    <PtIcon v-else-if="item.external" name="external-link" :size="13" class="pt-nav__ext" />
  </component>
</template>
