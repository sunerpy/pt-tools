<script setup lang="ts">
/*
 * 探索（路线图 M11）：TMDB 的列表、按条件发现与搜索，海报网格；每项标出已入库、已订阅，点「订阅」建订阅。
 * 番剧日历（媒体中心 M9b）按星期列出 Bangumi 每日放送里在播的动画，点一部时在 TMDB 上找到对应的剧集再打开详情。
 * 画板没有这一页，沿用媒体页的样式：页头 + 面板，手机是两列。
 */
import {
  type BangumiCalendarItem,
  type BangumiDay,
  type ExploreItem,
  type ExploreList,
  type ExplorePage,
  type MediaKind,
  subscribeApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import TmdbKeyNotice from "@/components/media/TmdbKeyNotice.vue";
import SubscribeDialog, { type SubscribeTarget } from "@/components/media/SubscribeDialog.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { useTmdbImageBase } from "@/composables/useTmdbImageBase";
import { posterURL } from "@/utils/media";
import { itemLink } from "@/utils/mediaItem";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";

/** 上面的切换：电影、剧集是 TMDB 的列表，bangumi 是番剧日历 */
type View = MediaKind | "bangumi";

const LISTS: Record<MediaKind, { value: ExploreList; label: string }[]> = {
  movie: [
    { value: "trending", label: "本周趋势" },
    { value: "popular", label: "热门" },
    { value: "now_playing", label: "正在上映" },
    { value: "upcoming", label: "即将上映" },
    { value: "top_rated", label: "高分" },
    { value: "discover", label: "按国家与类型找" },
    { value: "search", label: "搜索" },
  ],
  tv: [
    { value: "trending", label: "本周趋势" },
    { value: "popular", label: "热门" },
    { value: "on_the_air", label: "正在播出" },
    { value: "airing_today", label: "今天播出" },
    { value: "top_rated", label: "高分" },
    { value: "discover", label: "按国家与类型找" },
    { value: "search", label: "搜索" },
  ],
};

const COUNTRIES = [
  { value: "", label: "不限国家" },
  { value: "CN", label: "中国大陆" },
  { value: "HK", label: "中国香港" },
  { value: "TW", label: "中国台湾" },
  { value: "JP", label: "日本" },
  { value: "KR", label: "韩国" },
  { value: "US", label: "美国" },
  { value: "GB", label: "英国" },
];

const isMobile = useIsMobile();
// 「媒体识别」里换了 TMDB 图片地址时，海报从换的地址加载
useTmdbImageBase();
const router = useRouter();
const view = ref<View>("movie");
const kind = computed<MediaKind>(() => (view.value === "bangumi" ? "tv" : view.value));
const list = ref<ExploreList>("trending");
const country = ref("");
const genres = ref("");
const keyword = ref("");
const page = ref(1);
const data = ref<ExplorePage | null>(null);
const searched = ref("");
const ds = useDataState({ filtered: () => list.value === "search" });
const failedPosters = ref(new Set<number>());

const items = computed(() => data.value?.items ?? []);
const totalPages = computed(() => data.value?.total_pages ?? 1);
const lists = computed(() => LISTS[kind.value]);
/** 综艺（真人秀与脱口秀）只有剧集有 */
const genreOptions = computed(() => [
  { value: "", label: "不限类型" },
  { value: "16", label: "动画" },
  { value: "99", label: "纪录片" },
  ...(kind.value === "tv" ? [{ value: "10764|10767", label: "综艺" }] : []),
]);

async function load() {
  if (view.value === "bangumi") {
    void loadBangumi();
    return;
  }
  if (list.value === "search" && !keyword.value.trim()) {
    // 还在飞的趋势、热门请求作废：空跑一次 run，那些请求落地时就是过期的，不会写进列表
    void ds.run(async () => null);
    data.value = null;
    searched.value = "";
    return;
  }
  const q = list.value === "search" ? keyword.value.trim() : "";
  const discover = list.value === "discover";
  const pending = ds.run(() =>
    subscribeApi.explore({
      kind: kind.value,
      list: list.value,
      page: page.value,
      q,
      ...(discover ? { country: country.value, genres: genres.value } : {}),
    }),
  );
  const got = await pending;
  if (ds.isStale(pending)) return;
  data.value = got;
  searched.value = q;
}

function switchView() {
  // 换了电影、剧集以后这一边没有的列表与类型换回默认的
  if (!lists.value.some((l) => l.value === list.value)) list.value = "trending";
  if (!genreOptions.value.some((g) => g.value === genres.value)) genres.value = "";
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

/** 按条目搜索这个条目的资源（剧集默认最新的一季，搜索页上可以换季） */
function searchFor(it: ExploreItem) {
  void router.push({
    path: "/search",
    query: { media_type: it.media_type, tmdb_id: String(it.id) },
  });
}

// ---- 番剧日历 ----
const WEEKDAYS = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"];
/** 今天是星期几（1 是星期一，7 是星期日，按浏览器的时间） */
const today = ((new Date().getDay() + 6) % 7) + 1;
const bgm = useDataState();
const bgmDays = ref<BangumiDay[] | null>(null);
const bgmDay = ref(today);
const failedCovers = ref(new Set<number>());
/** 正在 TMDB 上找的一部 */
const resolving = ref(0);
/** 在 TMDB 上没找到的一部：写明，给一个按名字搜索 */
const missing = ref<{ name: string; reason: string } | null>(null);

const bgmItems = computed(
  () => bgmDays.value?.find((d) => d.weekday === bgmDay.value)?.items ?? [],
);

function dayLabel(n: number) {
  return n === today ? "今天" : WEEKDAYS[n - 1];
}

async function loadBangumi() {
  const pending = bgm.run(() => subscribeApi.bangumiCalendar());
  const got = await pending;
  if (bgm.isStale(pending)) return;
  if (got) bgmDays.value = got.days ?? [];
}

function bgmName(it: BangumiCalendarItem) {
  return it.name_cn || it.name;
}

async function openBangumi(it: BangumiCalendarItem) {
  if (resolving.value) return;
  resolving.value = it.id;
  missing.value = null;
  try {
    const got = await subscribeApi.bangumiTMDB(it.id);
    void router.push(itemLink(got.media_type, got.tmdb_id, "explore"));
  } catch (e) {
    missing.value = { name: bgmName(it), reason: (e as Error)?.message || "TMDB 上没找到" };
  } finally {
    resolving.value = 0;
  }
}

/** 没找到时按名字在 TMDB 上搜剧集 */
function searchMissing() {
  if (!missing.value) return;
  keyword.value = missing.value.name;
  missing.value = null;
  view.value = "tv";
  list.value = "search";
  page.value = 1;
  void load();
}

onMounted(load);
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub
      >TMDB 的趋势、热门、上映与播出的列表，按国家与类型找，或者搜索；还有 Bangumi
      的番剧日历。看到想看的电影点「订阅」、剧集点「追剧」，pt-tools 会帮你找资源</PtHeadSub
    >
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="ex-refresh" @click="load">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>
    <TmdbKeyNotice class="pt-cards__full" usage="探索与订阅" />

    <PtPanel title="探索" icon="compass">
      <div class="ex-bar">
        <el-radio-group v-model="view" size="small" data-testid="ex-kind" @change="switchView">
          <el-radio-button value="movie">电影</el-radio-button>
          <el-radio-button value="tv">剧集</el-radio-button>
          <el-radio-button value="bangumi">番剧日历</el-radio-button>
        </el-radio-group>
        <template v-if="view !== 'bangumi'">
          <el-select
            v-model="list"
            class="ex-list"
            size="small"
            aria-label="列表"
            data-testid="ex-list"
            @change="switchView">
            <el-option v-for="l in lists" :key="l.value" :value="l.value" :label="l.label" />
          </el-select>
          <template v-if="list === 'discover'">
            <el-select
              v-model="country"
              class="ex-filter"
              size="small"
              aria-label="出品国家"
              :empty-values="[null, undefined]"
              data-testid="ex-country"
              @change="switchView">
              <el-option v-for="c in COUNTRIES" :key="c.value" :value="c.value" :label="c.label" />
            </el-select>
            <el-select
              v-model="genres"
              class="ex-filter"
              size="small"
              aria-label="类型"
              :empty-values="[null, undefined]"
              data-testid="ex-genres"
              @change="switchView">
              <el-option
                v-for="g in genreOptions"
                :key="g.value"
                :value="g.value"
                :label="g.label" />
            </el-select>
          </template>
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
        </template>
        <el-radio-group v-else v-model="bgmDay" size="small" class="ex-days" data-testid="ex-days">
          <el-radio-button v-for="n in 7" :key="n" :value="n">{{ dayLabel(n) }}</el-radio-button>
        </el-radio-group>
      </div>

      <template v-if="view === 'bangumi'">
        <div v-if="missing" class="pt-note pt-note--warn ex-note" data-testid="ex-missing">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>「{{ missing.name }}」：{{ missing.reason }}。</span>
          <el-button
            size="small"
            text
            type="primary"
            data-testid="ex-missing-search"
            @click="searchMissing"
            >按名字搜索</el-button
          >
        </div>
        <div
          v-if="bgmItems.length && bgm.error.value"
          class="pt-note pt-note--warn ex-note"
          data-testid="ex-bgm-stale">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>刷新失败：{{ bgm.errorText.value }}。下面是上次读到的。</span>
        </div>
        <PtDataState
          v-if="!bgmItems.length"
          :state="bgm.loading.value || bgm.error.value || !bgmDays ? bgm.state.value : 'empty'"
          :title="bgm.error.value ? '' : '这一天没有在播的番剧'"
          :sub="bgm.errorText.value || '换一天看看'"
          data-testid="ex-bgm-state">
          <template v-if="bgm.state.value === 'error'" #action>
            <el-button size="small" data-testid="ex-bgm-reload" @click="loadBangumi">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>
        <div v-else class="ex-grid" data-testid="ex-bgm-grid">
          <div v-for="it in bgmItems" :key="it.id" class="ex-card" :data-testid="`ex-bgm-${it.id}`">
            <button
              type="button"
              class="ex-card__poster ex-card__btn"
              :aria-label="`在 TMDB 上找 ${bgmName(it)} 的详情`"
              :disabled="resolving !== 0"
              :data-testid="`ex-bgm-open-${it.id}`"
              @click="openBangumi(it)">
              <img
                v-if="it.image && !failedCovers.has(it.id)"
                :src="it.image"
                :alt="bgmName(it)"
                loading="lazy"
                referrerpolicy="no-referrer"
                @error="failedCovers.add(it.id)" />
              <div v-else class="ex-card__noposter"><PtIcon name="compass" :size="28" /></div>
              <div v-if="resolving === it.id" class="ex-card__busy">
                <PtIcon name="loader-circle" :size="20" class="pt-spin" />
              </div>
            </button>
            <div class="ex-card__body">
              <button
                type="button"
                class="ex-card__title ex-card__btn"
                :title="
                  it.name_cn && it.name !== it.name_cn ? `${it.name_cn}（${it.name}）` : it.name
                "
                :disabled="resolving !== 0"
                @click="openBangumi(it)">
                {{ bgmName(it) }}
              </button>
              <div class="ex-card__meta">
                <span v-if="it.air_date">{{ it.air_date }} 开播</span>
                <span v-if="it.score" class="ex-card__vote">
                  <PtIcon name="star" :size="12" />{{ it.score.toFixed(1) }}
                </span>
              </div>
            </div>
          </div>
        </div>
        <p class="ex-credit">
          番剧日历的数据来自 Bangumi 番组计划（bgm.tv）；点一部时到 TMDB 上找对应的剧集。
        </p>
      </template>
      <template v-else>
        <div
          v-if="items.length && ds.error.value"
          class="pt-note pt-note--warn ex-note"
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
                  ? 'TMDB 这一页没有内容'
                  : ''
          "
          :sub="
            ds.errorText.value ||
            (list === 'search' && !searched
              ? '电影或剧集的名字都行，中文名、英文名都可以'
              : ds.state.value === 'zero'
                ? '换个名字试试，中文名、英文名都可以'
                : ds.state.value === 'empty'
                  ? 'TMDB 暂时没有返回内容，可以过一会儿再刷新，或者换成搜索'
                  : '')
          "
          data-testid="ex-state">
          <!--
          趋势、热门是 TMDB 给的只读列表：空了不是「添加第一条记录」能解决的，给一个刷新。
          组件默认的副标题是给增删改列表写的，这里不能漏到页面上。
        -->
          <template
            v-if="ds.state.value === 'error' || (list !== 'search' && ds.state.value === 'empty')"
            #action>
            <el-button size="small" data-testid="ex-reload" @click="load">
              <PtIcon name="refresh-cw" :size="14" />
              <span>{{ ds.state.value === "error" ? "重试" : "刷新" }}</span>
            </el-button>
          </template>
        </PtDataState>
        <div v-else class="ex-grid" data-testid="ex-grid">
          <div v-for="it in items" :key="it.id" class="ex-card" :data-testid="`ex-item-${it.id}`">
            <router-link
              class="ex-card__poster"
              :to="itemLink(it.media_type, it.id, 'explore')"
              :aria-label="`${it.title} 的详情`"
              :data-testid="`ex-open-${it.id}`">
              <img
                v-if="posterURL(it.poster_path, 'w342') && !failedPosters.has(it.id)"
                :src="posterURL(it.poster_path, 'w342')"
                :alt="it.title"
                loading="lazy"
                @error="failedPosters.add(it.id)" />
              <div v-else class="ex-card__noposter"><PtIcon name="compass" :size="28" /></div>
              <div class="ex-card__badges">
                <PtStatusPill v-if="it.in_library" tone="ok" size="sm">已入库</PtStatusPill>
                <PtStatusPill v-if="it.subscribed" tone="info" size="sm">已订阅</PtStatusPill>
              </div>
            </router-link>
            <div class="ex-card__body">
              <router-link
                class="ex-card__title"
                :to="itemLink(it.media_type, it.id, 'explore')"
                :title="it.title"
                >{{ it.title }}</router-link
              >
              <div class="ex-card__meta">
                <span v-if="it.year">{{ it.year }}</span>
                <span v-if="it.vote_average" class="ex-card__vote">
                  <PtIcon name="star" :size="12" />{{ it.vote_average.toFixed(1) }}
                </span>
              </div>
              <div class="ex-card__acts">
                <el-button
                  v-if="it.subscribed"
                  size="small"
                  text
                  :data-testid="`ex-subscribed-${it.id}`"
                  @click="openSubscriptions"
                  >{{ it.media_type === "tv" ? "查看追剧" : "查看订阅" }}</el-button
                >
                <el-button
                  v-else
                  size="small"
                  type="primary"
                  plain
                  :data-testid="`ex-sub-${it.id}`"
                  @click="subscribe(it)">
                  <PtIcon name="bookmark" :size="13" /><span>{{
                    it.media_type === "tv" ? "追剧" : "订阅"
                  }}</span>
                </el-button>
                <!-- 按条目搜：带着这个条目打开种子搜索 -->
                <el-button
                  size="small"
                  text
                  :data-testid="`ex-search-${it.id}`"
                  @click="searchFor(it)">
                  <PtIcon name="search" :size="13" /><span>搜资源</span>
                </el-button>
              </div>
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
      </template>
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

.ex-list {
  width: 150px;
}

.ex-filter {
  width: 120px;
}

/*
 * 一周七天连成一排：放不下时横着滚，不折行（折行以后按钮连不起来）。滚动的容器会裁掉按钮贴边的描边，
 * 留 1px 内边距再用负外边距抵回去，和左边的切换按钮一样高
 */
.ex-days {
  flex-wrap: nowrap;
  max-width: 100%;
  padding: 1px;
  margin: -1px 0;
  overflow-x: auto;
}

/* 番剧日历的封面与名字是按钮（点了先到 TMDB 上找）：去掉按钮自己的样式 */
.ex-card__btn {
  padding: 0;
  border: 0;
  font: inherit;
  text-align: left;
  background: none;
  cursor: pointer;
}

.ex-card__btn:disabled {
  cursor: progress;
}

.ex-card__poster.ex-card__btn {
  width: 100%;
  background: var(--pt-bg-surface-muted);
}

.ex-card__btn:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: -2px;
}

.ex-card__busy {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  background: rgb(0 0 0 / 0.35);
}

/* 列表上方的提示（刷新失败、TMDB 上没找到）和下面的海报隔开 */
.ex-note {
  margin-bottom: 14px;
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
  display: block;
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
  display: block;
  font-size: 13px;
  font-weight: 600;
  color: var(--pt-t1);
  text-decoration: none;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ex-card__title:hover {
  color: var(--pt-p);
}

.ex-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 8px;
  font-size: 12px;
  color: var(--pt-t3);
}

/* 窄的卡片上一项放不下时整项换到下一行，不在字中间断开（例如「2025-01-05 开播」） */
.ex-card__meta > span {
  white-space: nowrap;
}

.ex-card__vote {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.ex-card__acts {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 4px;
  align-items: center;
  margin-top: 2px;
}

.ex-card__acts .el-button {
  margin: 0;
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

  .ex-list,
  .ex-filter {
    flex: 1 1 0;
    min-width: 0;
  }

  .ex-days :deep(.el-radio-button__inner) {
    padding-inline: 9px;
  }
}
</style>
