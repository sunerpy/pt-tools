<script setup lang="ts">
/*
 * 下载器路径输入框：点开就列出这些下载器在「下载器设置」里预设的目录，选一个就行；列表里没有的路径直接输入。
 * 2026-10-26 用户：「下载器设置了下载目录，就可以直接选择而不是必须要输入」。
 */
import { type PathOption, useDownloaderPaths } from "@/composables/useDownloaderPaths";
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    /** 列哪些下载器的预设目录；不给或是 0 时用 pt-tools 的默认下载器 */
    downloaderIds?: number | number[] | null;
    placeholder?: string;
  }>(),
  { downloaderIds: null, placeholder: "" },
);
const model = defineModel<string>({ default: "" });

const { pathOptionsFor } = useDownloaderPaths();
const options = computed(() => pathOptionsFor(props.downloaderIds));
const many = computed(() => new Set(options.value.map((o) => o.downloaderId)).size > 1);

// 空着、或者就是某个预设目录时列出全部（换一个目录不用先清空）；输入别的路径时按别名与路径筛
function suggest(query: string, cb: (items: PathOption[]) => void) {
  const q = (query ?? "").trim().toLowerCase();
  const all = options.value;
  if (!q || all.some((o) => o.path.toLowerCase() === q || o.alias.toLowerCase() === q)) {
    cb(all);
    return;
  }
  cb(all.filter((o) => o.path.toLowerCase().includes(q) || o.alias.toLowerCase().includes(q)));
}
</script>

<template>
  <el-autocomplete
    v-model="model"
    :fetch-suggestions="suggest"
    value-key="path"
    clearable
    :debounce="0"
    :placeholder="placeholder"
    class="pt-path-input"
    popper-class="pt-path-input__popper">
    <template #default="{ item }">
      <div class="pt-path-opt" :title="item.path">
        <span class="pt-path-opt__name">
          <span>{{ item.alias || item.path }}</span>
          <span v-if="item.isDefault" class="pt-path-opt__tag">默认</span>
          <span v-if="many" class="pt-path-opt__dl">{{ item.downloader }}</span>
        </span>
        <span v-if="item.alias && item.alias !== item.path" class="pt-path-opt__path">{{
          item.path
        }}</span>
      </div>
    </template>
  </el-autocomplete>
</template>

<style scoped>
.pt-path-input {
  width: 100%;
}

/* 有别名的目录：左边别名，右边灰色写真实路径，免得只看到别名不知道落在哪（与 RSS 的目录下拉一致） */
.pt-path-opt {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  justify-content: space-between;
  min-width: 0;
}

.pt-path-opt__name {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  min-width: 0;
  overflow: hidden;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pt-path-opt__tag,
.pt-path-opt__dl {
  flex: none;
  padding: 0 var(--pt-space-1);
  font-size: var(--pt-text-xs);
  line-height: 18px;
  color: var(--pt-t3);
  background: var(--pt-bg-surface-muted);
  border-radius: var(--pt-radius-sm);
}

.pt-path-opt__path {
  overflow: hidden;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-text-xs);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
