<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { Delete, Refresh, View } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { useScraperStore } from "@/stores/scraper";
import type { ScrapeTask } from "@/api";

const store = useScraperStore();
const filter = ref<string>("");
const activeTask = ref<ScrapeTask | null>(null);

onMounted(async () => {
  await store.fetchTasks({ limit: 200 });
  store.startTaskPolling(2000);
});

onBeforeUnmount(() => {
  store.stopTaskPolling();
});

const filtered = computed(() => {
  if (!filter.value) return store.tasks;
  return store.tasks.filter((t) => t.state === filter.value);
});

function stateType(s: string) {
  switch (s) {
    case "success":
      return "success";
    case "failed":
      return "danger";
    case "running":
      return "warning";
    case "retrying":
      return "warning";
    default:
      return "info";
  }
}

async function remove(row: ScrapeTask) {
  try {
    await ElMessageBox.confirm(`确定取消/删除任务 #${row.id}？`, "确认", { type: "warning" });
    await store.cancelTask(row.id);
    ElMessage.success("已删除");
  } catch (err) {
    if (err !== "cancel") ElMessage.error(`删除失败: ${(err as Error).message}`);
  }
}

async function retry(row: ScrapeTask) {
  try {
    await store.triggerScrape({
      library_id: row.library_id,
      media_path: row.media_path,
      type: row.task_type === "tv" ? "tv" : "movie",
    });
    ElMessage.success("已重新提交");
  } catch (e: unknown) {
    ElMessage.error(`重试失败: ${(e as Error).message}`);
  }
}
</script>

<template>
  <div class="tasks-wrap">
    <header>
      <h2>刮削任务</h2>
      <div class="filters">
        <el-select v-model="filter" placeholder="全部状态" clearable style="width: 140px">
          <el-option label="待处理" value="pending" />
          <el-option label="运行中" value="running" />
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
          <el-option label="重试中" value="retrying" />
        </el-select>
        <el-button :icon="Refresh" @click="store.fetchTasks({ limit: 200 })">刷新</el-button>
      </div>
    </header>

    <el-empty v-if="filtered.length === 0" description="暂无任务" />

    <el-table
      v-else
      :data="filtered"
      stripe
      height="calc(100vh - 260px)"
      @row-click="(row: ScrapeTask) => (activeTask = row)">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="task_type" label="类型" width="80" />
      <el-table-column prop="media_path" label="路径" show-overflow-tooltip min-width="240" />
      <el-table-column prop="state" label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="stateType(row.state)" size="small">{{ row.state }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="current_stage" label="阶段" width="130" />
      <el-table-column prop="progress" label="进度" width="140">
        <template #default="{ row }">
          <el-progress :percentage="Math.round(row.progress)" :stroke-width="8" />
        </template>
      </el-table-column>
      <el-table-column prop="started_at" label="开始时间" width="160">
        <template #default="{ row }">
          {{ row.started_at?.slice(0, 19) || "-" }}
        </template>
      </el-table-column>
      <el-table-column label="操作" width="140" fixed="right">
        <template #default="{ row }">
          <el-button size="small" text :icon="View" @click.stop="activeTask = row">详情</el-button>
          <el-button
            v-if="row.state === 'failed'"
            size="small"
            text
            type="warning"
            @click.stop="retry(row)">
            重试
          </el-button>
          <el-button size="small" text type="danger" :icon="Delete" @click.stop="remove(row)">
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-drawer
      v-model="activeTask"
      :title="`任务 #${activeTask?.id ?? ''}`"
      size="560"
      :before-close="
        (done: () => void) => {
          activeTask = null;
          done();
        }
      ">
      <div v-if="activeTask" class="detail">
        <p><b>路径：</b>{{ activeTask.media_path }}</p>
        <p>
          <b>状态：</b>
          <el-tag :type="stateType(activeTask.state)" size="small">{{ activeTask.state }}</el-tag>
        </p>
        <p><b>阶段：</b>{{ activeTask.current_stage || "-" }}</p>
        <p><b>重试：</b>{{ activeTask.retry_count }} / {{ activeTask.max_retries }}</p>
        <p v-if="activeTask.last_error">
          <b>错误：</b>
          <code class="err">{{ activeTask.last_error }}</code>
        </p>
        <p v-if="activeTask.started_at"><b>开始：</b>{{ activeTask.started_at }}</p>
        <p v-if="activeTask.completed_at"><b>完成：</b>{{ activeTask.completed_at }}</p>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.tasks-wrap {
  padding: 24px;
}

.tasks-wrap header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.tasks-wrap h2 {
  margin: 0;
}
.filters {
  display: flex;
  gap: 8px;
}

.detail p {
  margin: 10px 0;
}
.err {
  display: block;
  margin-top: 6px;
  padding: 10px 12px;
  background: color-mix(in srgb, var(--pt-color-danger, #f56c6c) 8%, transparent);
  border-radius: 8px;
  font-size: 0.82rem;
  word-break: break-all;
  white-space: pre-wrap;
}
</style>
