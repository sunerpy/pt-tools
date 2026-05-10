<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ArrowLeft, Delete, Edit, MagicStick, Search, VideoPlay } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import MatchDialog from "@/components/scraper/MatchDialog.vue";
import LLMChatPanel from "@/components/scraper/LLMChatPanel.vue";
import { useScraperStore } from "@/stores/scraper";
import type { MediaLibraryConfig, NFOResult, ScrapeSearchResult } from "@/api";

const route = useRoute();
const router = useRouter();
const store = useScraperStore();

const lib = ref<MediaLibraryConfig | null>(null);
const showMatch = ref(false);
const showLLM = ref(false);

onMounted(async () => {
  const id = Number(route.params.id);
  if (!Number.isFinite(id) || id <= 0) {
    router.push("/scraper/libraries");
    return;
  }
  try {
    lib.value = await store.fetchLibrary(id);
    await store.fetchTasks({ library_id: id, limit: 100 });
  } catch (e: unknown) {
    ElMessage.error(`加载失败: ${(e as Error).message}`);
    router.push("/scraper/libraries");
  }
});

const tasksOfLib = computed(() => store.tasks.filter((t) => t.library_id === lib.value?.id));

function stateType(s: string) {
  switch (s) {
    case "success":
      return "success";
    case "failed":
      return "danger";
    case "running":
      return "warning";
    default:
      return "info";
  }
}

async function scan() {
  if (!lib.value) return;
  try {
    await store.triggerScrape({
      library_id: lib.value.id,
      media_path: lib.value.path,
      type: lib.value.type === "tv" ? "tv" : "movie",
    });
    ElMessage.success("任务已提交");
  } catch (e: unknown) {
    ElMessage.error(`触发失败: ${(e as Error).message}`);
  }
}

async function remove() {
  if (!lib.value) return;
  try {
    await ElMessageBox.confirm("确定删除此媒体库？", "确认", { type: "warning" });
    await store.deleteLibrary(lib.value.id);
    ElMessage.success("已删除");
    router.push("/scraper/libraries");
  } catch (err) {
    if (err !== "cancel") ElMessage.error(`删除失败: ${(err as Error).message}`);
  }
}

function onMatched(result: ScrapeSearchResult) {
  ElMessage.success(`已匹配: ${result.selected_candidate?.title ?? result.media_key}`);
  // 实际应用：将 match 结果传递给 scrape service 重刮
}

function onLLMSaved(nfo: NFOResult) {
  ElMessage.success(`LLM 元数据已保存: ${nfo.title}`);
  // 实际应用：持久化到 scrape_results 表
}

function openMatchDialog() {
  showMatch.value = true;
}

function openLLMPanel() {
  showLLM.value = true;
}
</script>

<template>
  <div v-if="lib" class="detail-wrap">
    <nav class="breadcrumb">
      <el-button text :icon="ArrowLeft" @click="router.push('/scraper/libraries')">
        媒体库
      </el-button>
      <span class="sep">/</span>
      <span>{{ lib.name }}</span>
    </nav>

    <section class="info-card">
      <header>
        <div>
          <h2>{{ lib.name }}</h2>
          <p class="path">{{ lib.path }}</p>
        </div>
        <div class="actions">
          <el-button :icon="VideoPlay" @click="scan">扫描</el-button>
          <el-button :icon="Search" @click="openMatchDialog">手动匹配</el-button>
          <el-button :icon="MagicStick" @click="openLLMPanel">AI 刮削</el-button>
          <el-button :icon="Edit" @click="router.push(`/scraper/libraries`)">编辑</el-button>
          <el-button :icon="Delete" type="danger" plain @click="remove">删除</el-button>
        </div>
      </header>
      <div class="meta">
        <span><b>类型：</b>{{ lib.type }}</span>
        <span><b>NFO：</b>{{ lib.nfo_dialect }}</span>
        <span><b>数据源：</b>{{ lib.provider_ids }}</span>
        <span v-if="lib.last_scan_at"><b>最近扫描：</b>{{ lib.last_scan_at.slice(0, 19) }}</span>
      </div>
    </section>

    <MatchDialog v-model="showMatch" :library-id="lib.id" @matched="onMatched" />
    <LLMChatPanel v-model="showLLM" :library-id="lib.id" @saved="onLLMSaved" />

    <section class="tasks-section">
      <h3>本库任务（{{ tasksOfLib.length }}）</h3>
      <el-empty v-if="tasksOfLib.length === 0" description="暂无任务" />
      <el-table v-else :data="tasksOfLib" size="small" stripe>
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="media_path" label="路径" show-overflow-tooltip />
        <el-table-column prop="state" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="stateType(row.state)" size="small">{{ row.state }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="current_stage" label="阶段" width="120" />
        <el-table-column prop="progress" label="进度" width="100">
          <template #default="{ row }">
            <el-progress :percentage="Math.round(row.progress)" :stroke-width="8" />
          </template>
        </el-table-column>
      </el-table>
    </section>
  </div>
  <div v-else class="loading">
    <el-icon class="rotating"><VideoPlay /></el-icon> 加载中...
  </div>
</template>

<style scoped>
.detail-wrap {
  padding: 24px;
}

.breadcrumb {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  color: var(--pt-text-secondary, #78716c);
}
.sep {
  color: var(--pt-text-tertiary, #a8a29e);
}

.info-card {
  background: var(--pt-bg-surface-raised, #fff);
  border: 1px solid var(--pt-border-color, rgb(28 25 23 / 6%));
  border-radius: 18px;
  padding: 20px;
  margin-bottom: 24px;
  box-shadow: 0 1px 2px rgb(28 25 23 / 4%);
}

.info-card header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 12px;
}
.info-card h2 {
  margin: 0;
}
.info-card .path {
  color: var(--pt-text-secondary, #78716c);
  font-size: 0.85rem;
  margin: 4px 0 0;
  word-break: break-all;
}
.info-card .actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  font-size: 0.88rem;
  color: var(--pt-text-secondary);
}
.meta b {
  color: var(--pt-text-primary, #1c1917);
}

.tasks-section h3 {
  font-size: 1.02rem;
}
.loading {
  text-align: center;
  padding: 48px;
}
.rotating {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
