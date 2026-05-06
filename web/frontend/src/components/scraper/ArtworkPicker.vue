<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ElMessage } from "element-plus";
import { scraperApi, type MediaArtwork } from "@/api";

const props = defineProps<{
  modelValue: boolean;
  tmdbId: number;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  selected: [artwork: MediaArtwork, kind: ArtworkKind];
}>();

type ArtworkKind = "poster" | "fanart" | "banner";

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit("update:modelValue", v),
});

const activeTab = ref<ArtworkKind>("poster");
const langFilter = ref<string>("");
const loading = ref(false);
const allArtworks = ref<Record<ArtworkKind, MediaArtwork[]>>({
  poster: [],
  fanart: [],
  banner: [],
});
const selected = ref<Record<ArtworkKind, MediaArtwork | null>>({
  poster: null,
  fanart: null,
  banner: null,
});

watch(
  () => [props.modelValue, props.tmdbId],
  async ([v]) => {
    if (!v || !props.tmdbId) return;
    await loadAll();
  },
  { immediate: true },
);

async function loadAll() {
  loading.value = true;
  try {
    for (const kind of ["poster", "fanart", "banner"] as ArtworkKind[]) {
      const params = new URLSearchParams();
      params.set("tmdb_id", String(props.tmdbId));
      params.set("type", kind);
      try {
        const arts = await scraperApi.getArtworks(params);
        allArtworks.value[kind] = arts ?? [];
      } catch {
        allArtworks.value[kind] = [];
      }
    }
  } finally {
    loading.value = false;
  }
}

const visibleArts = computed(() => {
  const list = allArtworks.value[activeTab.value];
  if (!langFilter.value) return list;
  return list.filter((a) => (a.language ?? "") === langFilter.value);
});

const availableLangs = computed(() => {
  const s = new Set<string>();
  for (const a of allArtworks.value[activeTab.value]) {
    if (a.language) s.add(a.language);
  }
  return Array.from(s).sort();
});

function pick(a: MediaArtwork) {
  selected.value[activeTab.value] = a;
}

function isSelected(a: MediaArtwork): boolean {
  const sel = selected.value[activeTab.value];
  return sel?.url === a.url;
}

function confirmAll() {
  let picked = 0;
  for (const kind of ["poster", "fanart", "banner"] as ArtworkKind[]) {
    const sel = selected.value[kind];
    if (sel) {
      emit("selected", sel, kind);
      picked++;
    }
  }
  if (picked === 0) {
    ElMessage.warning("请至少选择一张图片");
    return;
  }
  ElMessage.success(`已选择 ${picked} 张`);
  visible.value = false;
}
</script>

<template>
  <el-dialog v-model="visible" title="选择艺术图" width="720" align-center append-to-body>
    <div v-if="loading" class="loading">加载候选图中...</div>
    <template v-else>
      <el-tabs v-model="activeTab" type="border-card">
        <el-tab-pane label="海报" name="poster" />
        <el-tab-pane label="背景图" name="fanart" />
        <el-tab-pane label="横幅" name="banner" />
      </el-tabs>

      <el-form inline class="filter-bar">
        <el-form-item label="语言">
          <el-select v-model="langFilter" clearable style="width: 120px">
            <el-option label="全部" value="" />
            <el-option v-for="l in availableLangs" :key="l" :label="l" :value="l" />
          </el-select>
        </el-form-item>
        <span v-if="selected[activeTab]" class="current">
          当前已选: {{ selected[activeTab]?.language || "—" }}
        </span>
      </el-form>

      <el-empty v-if="visibleArts.length === 0" description="无候选图片" />
      <div v-else class="grid" :class="{ fanart: activeTab !== 'poster' }">
        <div
          v-for="(a, i) in visibleArts"
          :key="a.url + i"
          class="art"
          :class="{ selected: isSelected(a) }"
          @click="pick(a)">
          <img :src="a.url" :alt="a.language ?? ''" loading="lazy" />
          <div class="overlay">
            <span class="lang">{{ a.language ?? "?" }}</span>
            <el-icon v-if="isSelected(a)" class="check"><Check /></el-icon>
          </div>
        </div>
      </div>
    </template>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" @click="confirmAll">应用选择</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.loading {
  padding: 48px;
  text-align: center;
  color: var(--pt-text-secondary, #78716c);
}

.filter-bar {
  margin: 12px 0;
  display: flex;
  align-items: center;
  gap: 12px;
}

.current {
  color: var(--pt-text-secondary, #78716c);
  font-size: 0.88rem;
  margin-left: auto;
}

.grid {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
  max-height: 480px;
  overflow-y: auto;
}

.grid.fanart {
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
}

.art {
  position: relative;
  border: 2px solid transparent;
  border-radius: 10px;
  overflow: hidden;
  cursor: pointer;
  transition:
    transform 150ms,
    border 150ms;
  aspect-ratio: 2 / 3;
  background: var(--pt-bg-surface-muted, #e7e5e4);
}

.grid.fanart .art {
  aspect-ratio: 16 / 9;
}

.art:hover {
  transform: translateY(-2px);
}
.art.selected {
  border-color: var(--pt-color-primary, #18a058);
}
.art img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.overlay {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  padding: 4px 8px;
  background: linear-gradient(transparent, rgba(0, 0, 0, 0.7));
  color: #fff;
  font-size: 0.75rem;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.check {
  background: var(--pt-color-primary, #18a058);
  color: #fff;
  border-radius: 999px;
  padding: 4px;
}
</style>
