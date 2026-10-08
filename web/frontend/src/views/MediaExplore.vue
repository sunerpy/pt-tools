<script setup lang="ts">
/*
 * 探索（路线图 M11）：TMDB 的本周趋势、热门与搜索，海报网格；每项标出已入库、已订阅，点「订阅」建订阅。
 * 画板没有这一页，沿用媒体页的样式：页头 + 面板，手机是两列。
 */
import { type ExploreItem, type ExplorePage, type MediaKind, subscribeApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import SubscribeDialog, { type SubscribeTarget } from "@/components/media/SubscribeDialog.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { posterURL } from "@/utils/media";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";

type ListKind = "trending" | "popular" | "search";

const isMobile = useIsMobile();
const router = useRouter();
const kind = ref<MediaKind>("movie");
const list = ref<ListKind>("trending");
const keyword = ref("");
const page = ref(1);
const data = ref<ExplorePage | null>(null);
const searched = ref("");
const ds = useDataState({ filtered: () => list.value === "search" });
const failedPosters = ref(new Set<number>());

const items = computed(() => data.value?.items ?? []);
const totalPages = computed(() => data.value?.total_pages ?? 1);

async function load() {
  if (list.value === "search" && !keyword.value.trim()) {
    // 还在飞的趋势、热门请求作废：空跑一次 run，那些请求落地时就是过期的，不会写进列表
    void ds.run(async () => null);
    data.value = null;
    searched.value = "";
    return;
  }
  const q = list.value === "search" ? keyword.value.trim() : "";
  const pending = ds.run(() =>
    subscribeApi.explore({ kind: kind.value, list: list.value, page: page.value, q }),
  );
  const got = await pending;
  if (ds.isStale(pending)) return;
  data.value = got;
  searched.value = q;
}

function switchView() {
  page.value = 1;
  void load();
}

function search() {
  list.value = "search";
  switchView();
}

function goPage(p: number) {
  page.value = p;
  void load();
}

// ---- 订阅 ----
const dialog = ref(false);
const target = ref<SubscribeTarget | null>(null);

function subscribe(it: ExploreItem) {
  target.value = {
    kind: it.media_type,
    tmdb_id: it.id,
    title: it.title,
    year: it.year,
    poster_path: it.poster_path,
  };
  dialog.value = true;
}

function onSaved() {
  void load();
}

function openSubscriptions() {
  void router.push("/media/subscriptions");
}

onMounted(load);
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub>TMDB 的本周趋势、热门与搜索，看到想看的点「订阅」，pt-tools 会帮你找资源</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="ex-refresh" @click="load">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <PtPanel title="探索" icon="compass">
      <div class="ex-bar">
        <el-radio-group v-model="kind" size="small" data-testid="ex-kind" @change="switchView">
          <el-radio-button value="movie">电影</el-radio-button>
          <el-radio-button value="tv">剧集</el-radio-button>
        </el-radio-group>
        <el-radio-group v-model="list" size="small" data-testid="ex-list" @change="switchView">
          <el-radio-button value="trending">本周趋势</el-radio-button>
          <el-radio-button value="popular">热门</el-radio-button>
          <el-radio-button value="search">搜索</el-radio-button>
        </el-radio-group>
        <el-input
          v-model="keyword"
          class="ex-search"
          size="small"
          clearable
          placeholder="电影或剧集的名字，回车搜索"
          data-testid="ex-keyword"
          @keyup.enter="search">
          <template #prefix><PtIcon name="search" :size="14" /></template>
        </el-input>
      </div>

      <div
        v-if="items.length && ds.error.value"
        class="pt-note pt-note--warn"
        data-testid="ex-stale">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>刷新失败：{{ ds.errorText.value }}。下面是上次读到的。</span>
      </div>
      <PtDataState
        v-if="!items.length"
        :state="
          list === 'search' && !searched && !ds.loading.value && !ds.error.value
            ? 'empty'
            : ds.state.value
        "
        :title="
          list === 'search' && !searched
            ? '输入名字搜索'
            : ds.state.value === 'zero'
              ? '没有搜到'
              : ds.state.value === 'empty'
                ? '列表是空的'
                : ''
        "
        :sub="
          ds.errorText.value ||
          (ds.state.value === 'zero' ? '换个名字试试，中文名、英文名都可以' : '')
        "
        data-testid="ex-state" />
      <div v-else class="ex-grid" data-testid="ex-grid">
        <div v-for="it in items" :key="it.id" class="ex-card" :data-testid="`ex-item-${it.id}`">
          <div class="ex-card__poster">
            <img
              v-if="it.poster_path && !failedPosters.has(it.id)"
              :src="posterURL(it.poster_path, 'w342')"
              :alt="it.title"
              loading="lazy"
              @error="failedPosters.add(it.id)" />
            <div v-else class="ex-card__noposter"><PtIcon name="compass" :size="28" /></div>
            <div class="ex-card__badges">
              <PtStatusPill v-if="it.in_library" tone="ok" size="sm">已入库</PtStatusPill>
              <PtStatusPill v-if="it.subscribed" tone="info" size="sm">已订阅</PtStatusPill>
            </div>
          </div>
          <div class="ex-card__body">
            <div class="ex-card__title" :title="it.title">{{ it.title }}</div>
            <div class="ex-card__meta">
              <span v-if="it.year">{{ it.year }}</span>
              <span v-if="it.vote_average" class="ex-card__vote">
                <PtIcon name="star" :size="12" />{{ it.vote_average.toFixed(1) }}
              </span>
            </div>
            <el-button
              v-if="it.subscribed"
              size="small"
              text
              :data-testid="`ex-subscribed-${it.id}`"
              @click="openSubscriptions"
              >查看订阅</el-button
            >
            <el-button
              v-else
              size="small"
              type="primary"
              plain
              :data-testid="`ex-sub-${it.id}`"
              @click="subscribe(it)">
              <PtIcon name="bookmark" :size="13" /><span>订阅</span>
            </el-button>
          </div>
        </div>
      </div>
      <div v-if="items.length && list !== 'search' && totalPages > 1" class="ex-pager">
        <el-pagination
          :current-page="page"
          :page-count="totalPages"
          :pager-count="isMobile ? 5 : 7"
          layout="prev, pager, next"
          size="small"
          data-testid="ex-pager"
          @current-change="goPage" />
      </div>
      <p class="ex-credit">
        电影与剧集的数据来自 TMDB；本产品使用 TMDB API，但未经 TMDB 认可或认证。
      </p>
    </PtPanel>

    <SubscribeDialog v-model="dialog" :target="target" @saved="onSaved" />
  </div>
</template>

<style scoped>
.ex-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}

.ex-search {
  width: 260px;
  max-width: 100%;
}

.ex-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 16px;
}

.ex-card {
  display: flex;
  flex-direction: column;
  min-width: 0;
  border: 1px solid var(--pt-border);
  border-radius: 8px;
  overflow: hidden;
  background: var(--pt-bg-surface);
}

.ex-card__poster {
  position: relative;
  aspect-ratio: 2 / 3;
  background: var(--pt-bg-surface-muted);
}

.ex-card__poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.ex-card__noposter {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  color: var(--pt-t3);
}

.ex-card__badges {
  position: absolute;
  top: 6px;
  left: 6px;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

/* 标记压在海报上：状态色本身是半透明的，垫一层实色底，海报再花也看得清 */
.ex-card__badges :deep(.pt-pill) {
  --ex-tint: color-mix(in srgb, var(--pt-pill-c) 14%, transparent);
  background: linear-gradient(var(--ex-tint), var(--ex-tint)), var(--pt-bg-surface);
  box-shadow: 0 1px 3px rgb(0 0 0 / 0.35);
}

.ex-card__body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 10px 10px;
  min-width: 0;
}

.ex-card__title {
  font-size: 13px;
  font-weight: 600;
  color: var(--pt-t1);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ex-card__meta {
  display: flex;
  gap: 8px;
  font-size: 12px;
  color: var(--pt-t3);
}

.ex-card__vote {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.ex-card__body .el-button {
  align-self: flex-start;
  margin-top: 2px;
}

.ex-pager {
  display: flex;
  justify-content: center;
  margin-top: 16px;
}

.ex-credit {
  margin: 14px 0 0;
  font-size: 12px;
  color: var(--pt-t3);
}

@media (max-width: 767px) {
  .ex-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }

  .ex-search {
    width: 100%;
  }
}
</style>
