<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { Delete, Edit, FolderAdd, Refresh, VideoPlay } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import type { MediaLibraryConfig } from "@/api";
import { useScraperStore } from "@/stores/scraper";

const router = useRouter();
const store = useScraperStore();

onMounted(() => {
  store.fetchLibraries();
});

const showCreate = ref(false);
const showEdit = ref(false);
const editing = ref<MediaLibraryConfig | null>(null);

const form = reactive<{
  name: string;
  type: "movie" | "tv" | "mixed";
  path: string;
  provider_ids: string;
  nfo_dialect: string;
  auto_scrape: boolean;
}>({
  name: "",
  type: "mixed",
  path: "",
  provider_ids: "tmdb,douban",
  nfo_dialect: "universal",
  auto_scrape: false,
});

function resetForm() {
  form.name = "";
  form.type = "mixed";
  form.path = "";
  form.provider_ids = "tmdb,douban";
  form.nfo_dialect = "universal";
  form.auto_scrape = false;
}

async function submitCreate() {
  try {
    await store.createLibrary({ ...form });
    ElMessage.success("媒体库已添加");
    showCreate.value = false;
    resetForm();
  } catch (e: unknown) {
    ElMessage.error(`添加失败: ${(e as Error).message}`);
  }
}

function openEdit(lib: MediaLibraryConfig) {
  editing.value = lib;
  form.name = lib.name;
  form.type = lib.type;
  form.path = lib.path;
  form.provider_ids = lib.provider_ids;
  form.nfo_dialect = lib.nfo_dialect;
  form.auto_scrape = lib.auto_scrape;
  showEdit.value = true;
}

async function submitEdit() {
  if (!editing.value) return;
  try {
    await store.updateLibrary(editing.value.id, { ...form });
    ElMessage.success("已更新");
    showEdit.value = false;
    editing.value = null;
  } catch (e: unknown) {
    ElMessage.error(`更新失败: ${(e as Error).message}`);
  }
}

async function removeLib(lib: MediaLibraryConfig) {
  try {
    await ElMessageBox.confirm(
      `确定删除媒体库「${lib.name}」？关联的刮削任务和结果将一并删除。`,
      "删除媒体库",
      { type: "warning", confirmButtonText: "删除", cancelButtonText: "取消" },
    );
    await store.deleteLibrary(lib.id);
    ElMessage.success("已删除");
  } catch (err) {
    if (err !== "cancel") {
      ElMessage.error(`删除失败: ${(err as Error).message}`);
    }
  }
}

async function scan(lib: MediaLibraryConfig) {
  try {
    await store.triggerScrape({
      library_id: lib.id,
      media_path: lib.path,
      type: lib.type === "tv" ? "tv" : "movie",
    });
    router.push("/scraper/tasks");
  } catch (e: unknown) {
    ElMessage.error(`触发扫描失败: ${(e as Error).message}`);
  }
}

const libTypeTag = computed(() => (t: string) => {
  if (t === "movie") return { label: "电影", type: "" as const };
  if (t === "tv") return { label: "剧集", type: "success" as const };
  return { label: "混合", type: "info" as const };
});
</script>

<template>
  <div class="library-list-wrap">
    <header class="page-header">
      <div>
        <h2>媒体库</h2>
        <p class="sub">管理目录与刮削规则</p>
      </div>
      <el-button type="primary" :icon="FolderAdd" @click="showCreate = true">
        新增媒体库
      </el-button>
    </header>

    <el-empty v-if="store.libraries.length === 0" description="暂无媒体库">
      <el-button type="primary" @click="showCreate = true">添加第一个</el-button>
    </el-empty>

    <div v-else class="grid">
      <article v-for="lib in store.libraries" :key="lib.id" class="lib-card">
        <header>
          <h3>{{ lib.name }}</h3>
          <el-tag size="small" :type="libTypeTag(lib.type).type">
            {{ libTypeTag(lib.type).label }}
          </el-tag>
        </header>
        <p class="path">{{ lib.path }}</p>
        <div class="meta">
          <el-tag
            v-for="p in (lib.provider_ids ?? '').split(',').filter(Boolean)"
            :key="p"
            size="small"
            effect="plain">
            {{ p }}
          </el-tag>
          <el-tag v-if="lib.auto_scrape" size="small" type="warning" effect="dark">自动</el-tag>
        </div>
        <footer>
          <el-button size="small" :icon="VideoPlay" @click="scan(lib)">扫描</el-button>
          <el-button size="small" :icon="Edit" @click="openEdit(lib)">编辑</el-button>
          <el-button size="small" :icon="Delete" type="danger" plain @click="removeLib(lib)">
            删除
          </el-button>
        </footer>
      </article>
    </div>

    <el-dialog v-model="showCreate" title="新增媒体库" width="520">
      <el-form label-width="110">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="类型">
          <el-select v-model="form.type" style="width: 100%">
            <el-option label="电影" value="movie" />
            <el-option label="剧集" value="tv" />
            <el-option label="混合" value="mixed" />
          </el-select>
        </el-form-item>
        <el-form-item label="路径">
          <el-input v-model="form.path" placeholder="/media/movies" />
        </el-form-item>
        <el-form-item label="数据源">
          <el-input v-model="form.provider_ids" placeholder="tmdb,douban,llm" />
        </el-form-item>
        <el-form-item label="NFO 格式">
          <el-select v-model="form.nfo_dialect" style="width: 100%">
            <el-option label="Universal（三端通吃）" value="universal" />
            <el-option label="Kodi" value="kodi" />
            <el-option label="Jellyfin" value="jellyfin" />
            <el-option label="Emby" value="emby" />
          </el-select>
        </el-form-item>
        <el-form-item label="自动刮削">
          <el-switch v-model="form.auto_scrape" />
          <span class="hint">订阅 TorrentCompleted 自动触发</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="submitCreate">确认</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="showEdit" title="编辑媒体库" width="520">
      <el-form label-width="110">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="路径"><el-input v-model="form.path" /></el-form-item>
        <el-form-item label="数据源"><el-input v-model="form.provider_ids" /></el-form-item>
        <el-form-item label="NFO 格式">
          <el-select v-model="form.nfo_dialect" style="width: 100%">
            <el-option label="Universal" value="universal" />
            <el-option label="Kodi" value="kodi" />
            <el-option label="Jellyfin" value="jellyfin" />
            <el-option label="Emby" value="emby" />
          </el-select>
        </el-form-item>
        <el-form-item label="自动刮削"><el-switch v-model="form.auto_scrape" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEdit = false">取消</el-button>
        <el-button type="primary" @click="submitEdit">保存</el-button>
      </template>
    </el-dialog>

    <div v-if="store.loading" class="loading">
      <el-icon class="rotating"><Refresh /></el-icon> 加载中...
    </div>
  </div>
</template>

<style scoped>
.library-list-wrap {
  padding: 24px;
  container-type: inline-size;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 0;
}
.page-header .sub {
  color: var(--pt-text-secondary, #78716c);
  margin: 4px 0 0;
  font-size: 0.88rem;
}

.grid {
  display: grid;
  gap: 18px;
  grid-template-columns: repeat(1, 1fr);
}

@container (width >= 680px) {
  .grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
@container (width >= 1080px) {
  .grid {
    grid-template-columns: repeat(3, 1fr);
  }
}
@container (width >= 1460px) {
  .grid {
    grid-template-columns: repeat(4, 1fr);
  }
}

.lib-card {
  padding: 20px;
  border-radius: 18px;
  background: var(--pt-bg-surface-raised, #fff);
  border: 1px solid var(--pt-border-color, rgb(28 25 23 / 6%));
  box-shadow: 0 1px 2px rgb(28 25 23 / 4%);
  display: flex;
  flex-direction: column;
  gap: 12px;
  transition:
    transform 200ms,
    box-shadow 200ms;
}

.lib-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 24px -12px rgb(28 25 23 / 12%);
}

.lib-card header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.lib-card h3 {
  margin: 0;
  font-size: 1.05rem;
}

.lib-card .path {
  margin: 0;
  color: var(--pt-text-secondary, #78716c);
  font-size: 0.82rem;
  word-break: break-all;
}

.lib-card .meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.lib-card footer {
  display: flex;
  gap: 8px;
  border-top: 1px solid rgb(28 25 23 / 5%);
  padding-top: 12px;
}
.loading {
  text-align: center;
  padding: 24px;
  color: var(--pt-text-secondary, #78716c);
}
.hint {
  margin-left: 12px;
  color: var(--pt-text-tertiary, #a8a29e);
  font-size: 0.8rem;
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
