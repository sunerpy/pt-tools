<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import { scraperApi, type ScrapeSearchResult } from "@/api";

const props = defineProps<{
  modelValue: boolean;
  initialQuery?: string;
  initialYear?: number;
  libraryId?: number;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  matched: [result: ScrapeSearchResult];
}>();

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit("update:modelValue", v),
});

const query = ref(props.initialQuery ?? "");
const year = ref<number | undefined>(props.initialYear);
const provider = ref<"both" | "tmdb" | "douban">("both");
const loading = ref(false);
const results = ref<ScrapeSearchResult[]>([]);
const activeIdx = ref(0);

watch(
  () => [props.initialQuery, props.initialYear],
  ([q, y]) => {
    query.value = (q as string) ?? "";
    year.value = y as number | undefined;
  },
);

let debounceTimer: number | null = null;
watch(query, () => {
  if (!query.value || query.value.length < 2) {
    results.value = [];
    return;
  }
  if (debounceTimer) window.clearTimeout(debounceTimer);
  debounceTimer = window.setTimeout(doSearch, 300);
});

async function doSearch() {
  if (!query.value || !props.libraryId) return;
  loading.value = true;
  try {
    const providerParam = provider.value === "both" ? undefined : provider.value;
    const res = await scraperApi.searchMedia({
      library_id: props.libraryId,
      query: query.value,
      media_type: providerParam,
    });
    results.value = res ?? [];
    activeIdx.value = 0;
  } catch (e: unknown) {
    ElMessage.error(`搜索失败: ${(e as Error).message}`);
    results.value = [];
  } finally {
    loading.value = false;
  }
}

function choose(i: number) {
  if (i < 0 || i >= results.value.length) return;
  const pick = results.value[i];
  emit("matched", pick);
  visible.value = false;
}

function onKey(e: KeyboardEvent) {
  if (results.value.length === 0) return;
  if (e.key === "ArrowDown") {
    e.preventDefault();
    activeIdx.value = Math.min(activeIdx.value + 1, results.value.length - 1);
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    activeIdx.value = Math.max(activeIdx.value - 1, 0);
  } else if (e.key === "Enter") {
    e.preventDefault();
    choose(activeIdx.value);
  }
}

watch(visible, async (v) => {
  if (v) {
    await nextTick();
    if (query.value) doSearch();
  }
});
</script>

<template>
  <el-dialog
    v-model="visible"
    title="手动匹配"
    width="680"
    align-center
    append-to-body
    @keydown="onKey">
    <el-form inline>
      <el-form-item label="标题">
        <el-input v-model="query" placeholder="输入电影/剧集名" style="width: 280px" autofocus />
      </el-form-item>
      <el-form-item label="年份">
        <el-input-number v-model="year" :min="1900" :max="2100" style="width: 120px" />
      </el-form-item>
      <el-form-item label="数据源">
        <el-select v-model="provider" style="width: 120px">
          <el-option label="全部" value="both" />
          <el-option label="TMDB" value="tmdb" />
          <el-option label="豆瓣" value="douban" />
        </el-select>
      </el-form-item>
      <el-button type="primary" :loading="loading" @click="doSearch">搜索</el-button>
    </el-form>

    <div v-if="loading" class="match-loading">搜索中...</div>
    <el-empty v-else-if="results.length === 0" description="暂无结果" />
    <div v-else class="match-results">
      <article
        v-for="(r, i) in results"
        :key="r.media_key + i"
        class="match-item"
        :class="{ active: i === activeIdx }"
        @click="choose(i)"
        @mouseenter="activeIdx = i">
        <img
          v-if="r.selected_candidate?.['poster_path' as keyof typeof r.selected_candidate]"
          class="poster"
          src=""
          alt="" />
        <div v-else class="poster placeholder">无图</div>
        <div class="meta">
          <div class="title">
            {{ r.selected_candidate?.title ?? r.media_key }}
            <span v-if="r.selected_candidate?.year" class="year"
              >({{ r.selected_candidate.year }})</span
            >
          </div>
          <div v-if="r.selected_candidate?.original_title" class="original">
            {{ r.selected_candidate.original_title }}
          </div>
          <div v-if="r.selected_candidate?.description" class="desc">
            {{ r.selected_candidate.description }}
          </div>
          <div class="tags">
            <el-tag size="small" effect="dark">{{ r.media_type }}</el-tag>
            <el-tag
              v-for="g in r.selected_candidate?.genres?.slice(0, 3) ?? []"
              :key="g"
              size="small">
              {{ g }}
            </el-tag>
          </div>
        </div>
      </article>
    </div>

    <template #footer>
      <span class="hint">使用 ↑↓ 选择，Enter 确认</span>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :disabled="results.length === 0" @click="choose(activeIdx)">
        使用选中项
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.match-loading {
  padding: 48px;
  text-align: center;
  color: var(--pt-text-secondary, #78716c);
}

.match-results {
  max-height: 520px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.match-item {
  display: flex;
  gap: 12px;
  padding: 12px;
  border: 1px solid var(--pt-border-color, rgb(28 25 23 / 6%));
  border-radius: 12px;
  cursor: pointer;
  transition:
    border 150ms,
    background 150ms,
    transform 150ms;
}

.match-item:hover,
.match-item.active {
  border-color: color-mix(in srgb, var(--pt-color-primary, #18a058) 40%, transparent);
  background: color-mix(in srgb, var(--pt-color-primary, #18a058) 4%, transparent);
  transform: translateX(2px);
}

.poster {
  width: 60px;
  height: 90px;
  object-fit: cover;
  border-radius: 6px;
  background: var(--pt-bg-surface-muted, #e7e5e4);
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--pt-text-tertiary, #a8a29e);
  font-size: 0.75rem;
}

.meta {
  flex: 1;
  min-width: 0;
}
.title {
  font-weight: 600;
  font-size: 1rem;
}
.year {
  color: var(--pt-text-tertiary, #a8a29e);
  font-weight: 400;
  margin-left: 6px;
}
.original {
  color: var(--pt-text-secondary, #78716c);
  font-size: 0.82rem;
  margin-top: 2px;
}
.desc {
  color: var(--pt-text-secondary, #78716c);
  font-size: 0.85rem;
  margin-top: 6px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.tags {
  display: flex;
  gap: 4px;
  margin-top: 8px;
}
.hint {
  color: var(--pt-text-tertiary, #a8a29e);
  font-size: 0.8rem;
  margin-right: auto;
}
</style>
