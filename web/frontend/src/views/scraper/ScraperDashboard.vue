<script setup lang="ts">
import { computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { FolderOpened, List, Setting } from "@element-plus/icons-vue";
import { useScraperStore } from "@/stores/scraper";

const router = useRouter();
const store = useScraperStore();

onMounted(async () => {
  await Promise.allSettled([store.fetchLibraries(), store.fetchTasks({ limit: 10 })]);
});

const stats = computed(() => ({
  libraries: store.libraries.length,
  total: store.tasks.length,
  running: store.runningTasks.length,
  failed: store.failedTasks.length,
}));

const recent = computed(() => store.tasks.slice(0, 10));

function go(path: string) {
  router.push(path);
}

function stateType(s: string) {
  switch (s) {
    case "success":
      return "success";
    case "failed":
      return "danger";
    case "running":
      return "warning";
    case "pending":
    case "retrying":
      return "info";
    default:
      return "";
  }
}
</script>

<template>
  <div class="dashboard-wrap">
    <section class="hero">
      <div class="hero-inner">
        <h1 class="hero-title">媒体刮削</h1>
        <p class="hero-sub">TMDB · 豆瓣 · LLM 三源融合，自动写入 NFO 并触发 Jellyfin/Emby 刷新</p>
        <div class="hero-actions">
          <el-button type="primary" size="large" @click="go('/scraper/libraries')">
            <el-icon><FolderOpened /></el-icon>媒体库
          </el-button>
          <el-button size="large" @click="go('/scraper/tasks')">
            <el-icon><List /></el-icon>任务
          </el-button>
          <el-button size="large" @click="go('/scraper/settings')">
            <el-icon><Setting /></el-icon>设置
          </el-button>
        </div>
      </div>
    </section>

    <section class="stats">
      <div class="stat-card">
        <span class="stat-label">媒体库</span>
        <span class="stat-value">{{ stats.libraries }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">任务总数</span>
        <span class="stat-value">{{ stats.total }}</span>
      </div>
      <div class="stat-card warn">
        <span class="stat-label">运行中</span>
        <span class="stat-value">{{ stats.running }}</span>
      </div>
      <div class="stat-card danger">
        <span class="stat-label">失败</span>
        <span class="stat-value">{{ stats.failed }}</span>
      </div>
    </section>

    <section class="recent">
      <h3>最近活动</h3>
      <el-empty v-if="recent.length === 0" description="暂无任务" />
      <div v-else class="timeline">
        <div v-for="t in recent" :key="t.id" class="timeline-item">
          <el-tag :type="stateType(t.state)" size="small">{{ t.state }}</el-tag>
          <span class="path">{{ t.media_path }}</span>
          <span class="time">{{ t.created_at?.slice(0, 19) }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.dashboard-wrap {
  padding: 24px;
  container-type: inline-size;
}

.hero {
  border-radius: 22px;
  padding: 48px 32px;
  text-align: center;
  background-image:
    linear-gradient(to right, rgb(28 25 23 / 5%) 1px, transparent 1px),
    linear-gradient(to bottom, rgb(28 25 23 / 5%) 1px, transparent 1px);
  background-size: 32px 32px;
  margin-bottom: 24px;
  border: 1px solid var(--pt-border-color, rgb(28 25 23 / 6%));
}

.hero-title {
  font-family: ui-serif, "Songti SC", "STZhongsong", "Noto Serif CJK SC", Georgia, serif;
  font-size: 2.75rem;
  font-weight: 700;
  letter-spacing: -0.03em;
  margin: 0;
  background: linear-gradient(
    135deg,
    var(--pt-text-primary, #1c1917) 25%,
    var(--pt-color-primary, #18a058) 100%
  );
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.hero-sub {
  color: var(--pt-text-secondary, #78716c);
  margin: 16px 0 28px;
  font-size: 1.02rem;
}

.hero-actions {
  display: flex;
  gap: 12px;
  justify-content: center;
  flex-wrap: wrap;
}

.stats {
  display: grid;
  grid-template-columns: repeat(1, 1fr);
  gap: 16px;
  margin-bottom: 24px;
}

@container (width >= 640px) {
  .stats {
    grid-template-columns: repeat(2, 1fr);
  }
}
@container (width >= 960px) {
  .stats {
    grid-template-columns: repeat(4, 1fr);
  }
}

.stat-card {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 20px;
  border-radius: 18px;
  background: var(--pt-bg-surface-raised, #fff);
  border: 1px solid var(--pt-border-color, rgb(28 25 23 / 6%));
  box-shadow: 0 1px 2px rgb(28 25 23 / 4%);
  transition:
    transform 200ms,
    box-shadow 200ms;
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 24px -12px rgb(28 25 23 / 12%);
}

.stat-card.warn {
  border-color: color-mix(in srgb, var(--pt-color-warning, #e6a23c) 45%, transparent);
}

.stat-card.danger {
  border-color: color-mix(in srgb, var(--pt-color-danger, #f56c6c) 45%, transparent);
}

.stat-label {
  font-size: 0.85rem;
  color: var(--pt-text-secondary, #78716c);
}
.stat-value {
  font-size: 2rem;
  font-weight: 700;
}

.recent h3 {
  margin: 0 0 12px;
  font-size: 1.05rem;
}

.timeline {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.timeline-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 14px;
  border: 1px solid var(--pt-border-color, rgb(28 25 23 / 6%));
  border-radius: 10px;
  background: var(--pt-bg-surface-raised, #fff);
  font-size: 0.88rem;
}

.timeline-item .path {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.timeline-item .time {
  color: var(--pt-text-tertiary, #a8a29e);
  font-variant-numeric: tabular-nums;
}
</style>
