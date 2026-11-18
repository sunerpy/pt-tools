<script setup lang="ts">
import { ElMessage } from "element-plus";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";

import {
  freeWatchApi,
  type MediaKind,
  type MediaSearchMember,
  type MediaSearchResult,
  type QualityProfile,
  subscribeApi,
  type TorrentBlock,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import {
  blockedLine,
  buildCards,
  type EntryCard,
  ENTRY_SORTS,
  type EntrySort,
  episodeNote,
  spanText,
  summaryText,
} from "@/utils/entrySearch";
import { itemLink } from "@/utils/mediaItem";
import {
  applyFacets,
  buildFacets,
  type FacetKey,
  type FacetRecord,
  type FacetSelection,
  MAIN_FACETS,
  MOBILE_FACETS,
  MORE_FACETS,
  selectedCount,
  toggleFacet,
} from "@/utils/searchFacets";

import BlockDialog, { type BlockChoice } from "./BlockDialog.vue";
import EntryGroupCard from "./EntryGroupCard.vue";
import EntryGroupCardMobile from "./EntryGroupCardMobile.vue";
import EntryPoster from "./EntryPoster.vue";
import EntryRail from "./EntryRail.vue";
import EpisodeGrid from "./EpisodeGrid.vue";
import FacetPills from "./FacetPills.vue";

/**
 * 按条目搜索（画板 06 桌面、09 手机、10 手机筛选面板）：调 POST /api/media/search（只读），
 * 左栏是条目、季、每集格子、档案与带计数的筛选，右边是分组卡片。筛选、排序、合并都在前端按返回的全部结果算。
 * 推送交给页面现有的推送弹窗（带上条目）；「等免费」也交给它（等免费模式）。
 * 「不要」在这里拉黑这一组的成员（只对这个条目），拉黑的默认不显示，最后一行写条数，点了显示出来。
 */
export interface EntryRef {
  media_type: MediaKind;
  tmdb_id: number;
  /** 剧集的季；不填时是最新的一季 */
  season?: number;
}

const props = defineProps<{
  entry: EntryRef;
  /** 输入框里再写的字：按标题筛，不重新搜 */
  text: string;
  /** 合并同一资源 */
  merge: boolean;
}>();

const emit = defineEmits<{
  push: [
    member: MediaSearchMember,
    media: { media_type: MediaKind; tmdb_id: number; season: number },
  ];
  watch: [
    member: MediaSearchMember,
    media: { media_type: MediaKind; tmdb_id: number; season: number },
  ];
  season: [n: number];
  loaded: [item: MediaSearchResult["item"]];
  keyword: [keyword: string];
  busy: [loading: boolean];
}>();

const isMobile = useIsMobile();

/** 视口 ≥ 1280 时左栏常驻，窄了收进右侧抽屉（画板 06 的布局规则） */
const wide = ref(true);
let wideQuery: MediaQueryList | null = null;
const onWide = (e: MediaQueryListEvent) => (wide.value = e.matches);
onMounted(() => {
  if (typeof window.matchMedia !== "function") return;
  wideQuery = window.matchMedia("(min-width: 1280px)");
  wide.value = wideQuery.matches;
  wideQuery.addEventListener("change", onWide);
});
onBeforeUnmount(() => wideQuery?.removeEventListener("change", onWide));

const result = ref<MediaSearchResult | null>(null);
const loading = ref(false);
const error = ref("");
const profiles = ref<QualityProfile[]>([]);
/** 只影响这一次搜索：0 跟订阅（或默认档案），-1 不按档案 */
const profileId = ref(0);
const defaultProfileName = ref("跟订阅或默认");
const includeUnmatched = ref(false);
const sel = ref<FacetSelection>({});
const sort = ref<EntrySort>("score");
const episode = ref<number | undefined>(undefined);
const expanded = ref(new Set<string>());
const unfolded = ref(new Set<string>());
const showMore = ref(false);
const drawer = ref(false);
/** 手机筛选面板：面板里的选择点「看 N 条结果」才生效，直接关掉不生效 */
const panel = ref(false);
const draft = ref<FacetSelection>({});
/** 显示拉黑的（最后一行「已拉黑 N 条」点了打开） */
const showBlocked = ref(false);
/** 能等免费的站点（小写）；读不到时都当作不能 */
const watchSites = ref(new Set<string>());

let seq = 0;

async function loadWatchSites() {
  try {
    watchSites.value = new Set(
      ((await freeWatchApi.sites()).sites ?? []).map((s) => s.toLowerCase()),
    );
  } catch {
    watchSites.value = new Set();
  }
}

async function loadProfiles() {
  try {
    profiles.value = (await subscribeApi.profiles()).items ?? [];
  } catch {
    profiles.value = [];
  }
}

/** 搜一次。返回以前又搜了一次（换了季、档案）时丢掉旧的结果 */
async function run() {
  const my = ++seq;
  loading.value = true;
  error.value = "";
  emit("busy", true);
  try {
    const res = await subscribeApi.searchMedia({
      media_type: props.entry.media_type,
      tmdb_id: props.entry.tmdb_id,
      season: props.entry.season ?? 0,
      profile_id: profileId.value,
      include_unmatched: includeUnmatched.value,
    });
    if (my !== seq) return;
    result.value = res;
    if (profileId.value === 0) {
      defaultProfileName.value = res.profile ? res.profile.name || "按分辨率与来源" : "不按档案";
    }
    if (!res.profile && sort.value === "score") sort.value = "seeders";
    emit("loaded", res.item);
  } catch (e) {
    if (my !== seq) return;
    error.value = (e as Error).message || "搜索失败";
  } finally {
    if (my === seq) {
      loading.value = false;
      emit("busy", false);
    }
  }
}

defineExpose({ run });

onMounted(() => {
  void loadProfiles();
  void loadWatchSites();
  void run();
});

/* 换了条目或季：筛选、点的那一集都清掉 */
watch(
  () => [props.entry.media_type, props.entry.tmdb_id, props.entry.season],
  (now, before) => {
    if (now[0] !== before[0] || now[1] !== before[1]) {
      sel.value = {};
      profileId.value = 0;
      includeUnmatched.value = false;
      sort.value = "score";
      showBlocked.value = false;
    }
    episode.value = undefined;
    expanded.value = new Set();
    unfolded.value = new Set();
    void run();
  },
);

function setProfile(id: number) {
  if (id === profileId.value) return;
  profileId.value = id;
  if (id !== -1 && sort.value === "seeders" && !result.value?.profile) sort.value = "score";
  void run();
}

function showUnmatched() {
  includeUnmatched.value = true;
  void run();
}

/** 卡片里的成员连同它所在的组，供筛选计数用（计数是成员的条数）；拉黑的不显示时也不计 */
const records = computed<FacetRecord[]>(() =>
  (result.value?.groups ?? []).flatMap((g) =>
    g.members
      .filter((m) => showBlocked.value || !m.block)
      .map((m) => ({
        meta: m.meta,
        free: m.free,
        hr: m.hr,
        discount: m.discount,
        site: m.site,
        episodes: spanText(g.span),
      })),
  ),
);

const facetGroups = computed(() => buildFacets(records.value, sel.value, MAIN_FACETS));
const moreGroups = computed(() => buildFacets(records.value, sel.value, MORE_FACETS));
const panelGroups = computed(() =>
  buildFacets(records.value, draft.value, MOBILE_FACETS, { half: true }),
);
const panelCount = computed(() => applyFacets(records.value, draft.value).length);
const chosen = computed(() => selectedCount(sel.value) + (episode.value ? 1 : 0));

const cards = computed(() =>
  result.value
    ? buildCards(result.value, {
        merge: props.merge,
        sort: sort.value,
        sel: sel.value,
        text: props.text,
        episode: episode.value,
        showBlocked: showBlocked.value,
      })
    : [],
);

const summary = computed(() =>
  result.value ? summaryText(result.value, cards.value, props.merge, isMobile.value) : "",
);
/*
 * 手机的排序下拉按选中的字定宽（画板 09「档案分」那枚 71 宽）：字 12 号，加左 10、右 8 的内边距与箭头（14 加间距 6）。
 * 照画板的 71 给，三个字会被省略成「档…」，所以多给几像素
 */
const sortWidth = computed(
  () => `${(ENTRY_SORTS.find((o) => o.value === sort.value)?.label.length ?? 3) * 12 + 46}px`,
);
const siteErrors = computed(() => (result.value?.errors ?? []).filter((e) => e.site));
const lock = computed(() => result.value?.subscription?.lock ?? null);
const seasonLabel = computed(() => {
  const it = result.value?.item;
  if (!it) return "";
  return it.media_type === "tv" && it.season ? `${it.title} 第 ${it.season} 季` : it.title;
});

function toggle(key: FacetKey, value: string) {
  sel.value = toggleFacet(sel.value, key, value);
}

function pickEpisode(n: number) {
  episode.value = episode.value === n ? undefined : n;
}

function clearFilters() {
  sel.value = {};
  episode.value = undefined;
}

function toggleExpanded(key: string) {
  const next = new Set(expanded.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  expanded.value = next;
}

function unfold(key: string) {
  unfolded.value = new Set(unfolded.value).add(key);
}

function media() {
  const it = result.value?.item;
  return it ? { media_type: it.media_type, tmdb_id: it.tmdb_id, season: it.season ?? 0 } : null;
}

function push(m: MediaSearchMember) {
  const md = media();
  if (md) emit("push", m, md);
}

/** 卡上能等免费的成员：没拉黑、站点读得了种子详情；首选在前 */
function watchable(c: EntryCard): MediaSearchMember | undefined {
  return c.members.find((m) => !m.block && watchSites.value.has(m.site.toLowerCase()));
}

function watchFree(c: EntryCard) {
  const m = watchable(c);
  const md = media();
  if (m && md) emit("watch", m, md);
}

/* ---- 不要（黑名单） ---- */

const blockOpen = ref(false);
const blockBusy = ref(false);
const blockCard = ref<EntryCard | null>(null);
/** 这次拉黑的成员：卡上还没拉黑的（同一资源在几个站点上的种子一起拉黑） */
const blockTargets = computed(() => blockCard.value?.members.filter((m) => !m.block) ?? []);
/** 订阅有了才能把制作组加进标题排除词；名字按首选的解析结果写 */
const blockGroup = computed(() =>
  result.value?.subscription ? blockTargets.value[0]?.meta.group || undefined : undefined,
);
/** 拉黑记下的条目名（订阅页的「拉黑的种子」里写作用于哪个条目） */
const mediaTitle = computed(() => {
  const it = result.value?.item;
  if (!it) return "";
  return it.media_type === "movie" && it.year ? `${it.title} (${it.year})` : seasonLabel.value;
});

function openBlock(c: EntryCard) {
  blockCard.value = c;
  blockOpen.value = true;
}

const blockKey = (site: string, id: string) => `${site.toLowerCase()}|${id}`;

/**
 * 按服务端的黑名单重新标一遍结果里的成员：对这个条目的优先，其次是对所有条目的（同服务端 blocksFor）。
 * 拉黑与移出以后都走这里，免得本地猜错（比如移出了对这个条目的，对所有条目的那条还在）。
 */
async function refreshBlocks() {
  const res = result.value;
  if (!res) return;
  let list: TorrentBlock[];
  try {
    list = (await subscribeApi.blocks()).items ?? [];
  } catch (e) {
    ElMessage.error((e as Error).message || "读取黑名单失败");
    return;
  }
  const it = res.item;
  const season = it.media_type === "tv" ? (it.season ?? 0) : 0;
  const byKey = new Map<string, TorrentBlock>();
  for (const b of list) {
    const mine = b.media_type === it.media_type && b.tmdb_id === it.tmdb_id && b.season === season;
    if (!mine && b.tmdb_id !== 0) continue;
    const k = blockKey(b.site_name, b.torrent_id);
    const old = byKey.get(k);
    if (!old || (old.tmdb_id === 0 && mine)) byKey.set(k, b);
  }
  let n = 0;
  for (const g of res.groups) {
    for (const m of g.members) {
      const b = byKey.get(blockKey(m.site, m.torrent_id));
      m.block = b
        ? {
            id: b.id,
            ...(b.reason ? { reason: b.reason } : {}),
            ...(b.tmdb_id ? {} : { all: true }),
          }
        : undefined;
      if (b) n++;
    }
    g.blocked = g.members.every((m) => Boolean(m.block));
  }
  res.blocked_count = n;
}

async function confirmBlock(choice: BlockChoice) {
  const it = result.value?.item;
  const items = blockTargets.value.map((m) => ({
    site: m.site,
    torrent_id: m.torrent_id,
    title: m.title,
  }));
  if (!it || !items.length) {
    blockOpen.value = false;
    return;
  }
  blockBusy.value = true;
  try {
    const res = await subscribeApi.addBlocks({
      media_type: it.media_type,
      tmdb_id: it.tmdb_id,
      season: it.media_type === "tv" ? (it.season ?? 0) : 0,
      media_title: mediaTitle.value,
      reason: choice.reason || undefined,
      items,
      exclude_group: choice.excludeGroup || undefined,
    });
    blockOpen.value = false;
    ElMessage.success(
      res.excluded_group
        ? `已拉黑，制作组 ${res.excluded_group} 也加进了订阅的标题排除词`
        : "已拉黑，订阅以后不选它",
    );
    await refreshBlocks();
  } catch (e) {
    ElMessage.error((e as Error).message || "拉黑失败");
  } finally {
    blockBusy.value = false;
  }
}

async function unblock(members: MediaSearchMember[]) {
  const ids = [...new Set(members.map((m) => m.block?.id ?? 0).filter((id) => id > 0))];
  if (!ids.length) return;
  try {
    for (const id of ids) await subscribeApi.removeBlock(id);
    ElMessage.success("已移出黑名单");
  } catch (e) {
    ElMessage.error((e as Error).message || "移出黑名单失败");
  }
  await refreshBlocks();
}

const blocked = computed(() => (result.value ? blockedLine(result.value) : null));

function openPanel() {
  draft.value = { ...sel.value };
  panel.value = true;
}

function applyPanel() {
  sel.value = { ...draft.value };
  panel.value = false;
}

function keywordSearch() {
  emit("keyword", result.value?.keywords[0] ?? result.value?.item.title ?? "");
}

const seasons = computed(() =>
  Array.from({ length: Math.max(result.value?.item.seasons ?? 0, 0) }, (_, i) => i + 1),
);

const profileOptions = computed(() => [
  { value: 0, label: `档案：${defaultProfileName.value}` },
  ...profiles.value.map((p) => ({ value: p.id, label: `档案：${p.name}` })),
  { value: -1, label: "不按档案" },
]);
</script>

<template>
  <div class="es" :class="{ 'is-wide': wide && !isMobile, 'is-mobile': isMobile }" data-testid="es">
    <!-- 第一次还没搜完：整块等 -->
    <PtDataState
      v-if="!result && loading"
      state="loading"
      title="正在各站点上搜这个条目"
      sub="站点多时要十几秒到一分钟" />
    <PtDataState v-else-if="!result && error" state="error" :sub="error">
      <template #action>
        <el-button size="small" @click="run">
          <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
        </el-button>
      </template>
    </PtDataState>

    <template v-else-if="result">
      <!-- 手机（画板 09）：控件行、条目卡，然后是概况与结果卡 -->
      <template v-if="isMobile">
        <div class="es-mctl">
          <el-select
            v-if="result.item.media_type === 'tv' && seasons.length > 1"
            class="es-mctl__season"
            :model-value="result.item.season"
            aria-label="季"
            @update:model-value="emit('season', $event as number)">
            <el-option v-for="n in seasons" :key="n" :label="`第 ${n} 季`" :value="n" />
          </el-select>
          <el-select
            class="es-mctl__profile"
            :model-value="profileId"
            aria-label="质量档案"
            @update:model-value="setProfile($event as number)">
            <el-option
              v-for="o in profileOptions"
              :key="o.value"
              :label="o.label"
              :value="o.value" />
          </el-select>
          <button
            type="button"
            class="es-mctl__filter"
            :class="{ 'is-on': chosen > 0 }"
            data-testid="es-mfilter"
            @click="openPanel">
            <PtIcon name="list-filter" :size="15" />
            <span>{{ chosen > 0 ? `筛选 · ${chosen}` : "筛选" }}</span>
          </button>
        </div>
        <section class="es-mitem">
          <EntryPoster :path="result.item.poster_path" :width="40" :height="60" :radius="3" />
          <div class="es-mitem__text">
            <p class="es-mitem__title">
              <router-link
                class="es-mitem__link"
                :to="
                  itemLink(
                    result.item.media_type,
                    result.item.tmdb_id,
                    'search',
                    result.item.season,
                  )
                "
                data-testid="es-mdetail"
                >{{ seasonLabel }}</router-link
              >
            </p>
            <p class="es-mitem__sub">
              {{
                [
                  result.subscription
                    ? result.item.media_type === "tv"
                      ? "追剧中"
                      : "订阅中"
                    : "",
                  result.subscription?.lock
                    ? `版本锁 ${[result.subscription.lock.group, result.subscription.lock.resolution].filter(Boolean).join(" ")}`
                    : "",
                ]
                  .filter(Boolean)
                  .join(" · ") || episodeNote(result.episodes)
              }}
            </p>
            <EpisodeGrid
              v-if="result.episodes.length"
              mobile
              :episodes="result.episodes"
              :selected="episode"
              @pick="pickEpisode" />
          </div>
        </section>
      </template>

      <!-- 桌面窄屏（< 1280）：左栏收成一行，筛选在右侧抽屉里 -->
      <div v-else-if="!wide" class="es-narrow">
        <EntryPoster :path="result.item.poster_path" :width="28" :height="42" :radius="3" />
        <div class="es-narrow__text">
          <p class="es-narrow__title">{{ seasonLabel }}</p>
          <p class="es-narrow__note">{{ episodeNote(result.episodes) }}</p>
        </div>
        <el-button data-testid="es-drawer-btn" @click="drawer = true">
          <PtIcon name="list-filter" :size="15" />
          <span>{{ chosen > 0 ? `筛选 · ${chosen}` : "筛选" }}</span>
        </el-button>
      </div>

      <div class="es-body">
        <aside v-if="wide && !isMobile" class="es-body__rail">
          <EntryRail
            :result="result"
            :profiles="profiles"
            :profile-id="profileId"
            :default-profile-name="defaultProfileName"
            :facets="facetGroups"
            :more-facets="moreGroups"
            :sel="sel"
            :show-more="showMore"
            :episode="episode"
            @season="emit('season', $event)"
            @profile="setProfile"
            @toggle="toggle"
            @more="showMore = !showMore"
            @episode="pickEpisode" />
        </aside>

        <div v-loading="loading" class="es-results">
          <div class="es-sum">
            <span class="es-sum__text" data-testid="es-summary">{{ summary }}</span>
            <el-tooltip
              v-if="siteErrors.length"
              placement="bottom"
              :trigger="isMobile ? 'click' : 'hover'">
              <template #content>
                <p v-for="e in siteErrors" :key="e.site" class="es-sum__err">
                  {{ e.site }}：{{ e.error }}
                </p>
              </template>
              <span class="es-sum__warn" data-testid="es-site-errors"
                >{{ siteErrors.length }} 个站点没回应</span
              >
            </el-tooltip>
            <button v-if="episode" type="button" class="es-sum__chip" @click="episode = undefined">
              只看 E{{ String(episode).padStart(2, "0") }} <PtIcon name="x" :size="11" />
            </button>
            <span class="es-sum__gap" />
            <template v-if="!isMobile">
              <span class="es-sum__label">排序</span>
              <button
                v-for="o in ENTRY_SORTS"
                :key="o.value"
                type="button"
                class="es-sort"
                :class="{ 'is-on': sort === o.value }"
                :aria-pressed="sort === o.value"
                :disabled="o.value === 'score' && !result.profile"
                :data-testid="`es-sort-${o.value}`"
                @click="sort = o.value">
                {{ o.label }}
              </button>
            </template>
            <el-select
              v-else
              v-model="sort"
              class="es-sum__msort"
              :style="{ width: sortWidth }"
              aria-label="排序">
              <el-option
                v-for="o in ENTRY_SORTS"
                :key="o.value"
                :label="o.label"
                :value="o.value"
                :disabled="o.value === 'score' && !result.profile" />
            </el-select>
          </div>

          <p v-if="result.groups.length === 0" class="es-empty" data-testid="es-empty">
            这几个站点都没有搜到「{{ seasonLabel }}」
            <button type="button" class="es-link" @click="keywordSearch">换成关键字搜</button>
          </p>
          <p v-else-if="cards.length === 0" class="es-empty" data-testid="es-filtered-empty">
            筛选后没有结果
            <button type="button" class="es-link" @click="clearFilters">清空筛选</button>
          </p>

          <div class="es-cards">
            <template v-if="isMobile">
              <EntryGroupCardMobile
                v-for="c in cards"
                :key="c.key"
                :card="c"
                :episodes="result.episodes"
                :movie="result.movie"
                :lock="lock"
                :merged="merge"
                :expanded="expanded.has(c.key)"
                :unfolded="unfolded.has(c.key)"
                :can-watch="Boolean(watchable(c))"
                @push="push"
                @toggle="toggleExpanded(c.key)"
                @unfold="unfold(c.key)"
                @watch="watchFree(c)"
                @block="openBlock(c)"
                @unblock="unblock" />
            </template>
            <template v-else>
              <EntryGroupCard
                v-for="c in cards"
                :key="c.key"
                :card="c"
                :episodes="result.episodes"
                :movie="result.movie"
                :lock="lock"
                :merged="merge"
                :expanded="expanded.has(c.key)"
                :unfolded="unfolded.has(c.key)"
                :can-watch="Boolean(watchable(c))"
                @push="push"
                @toggle="toggleExpanded(c.key)"
                @unfold="unfold(c.key)"
                @watch="watchFree(c)"
                @block="openBlock(c)"
                @unblock="unblock" />
            </template>
          </div>

          <!-- 最后一行（画板 06）：已拉黑的与没对上的，两段之间「 · 」；文字链点了把它们显示出来 -->
          <p
            v-if="blocked || result.unmatched_count > 0"
            class="es-last"
            data-testid="es-unmatched-line">
            <PtIcon name="eye-off" :size="14" />
            <span>
              <template v-if="blocked">
                <button
                  type="button"
                  class="es-link es-link--muted"
                  :aria-pressed="showBlocked"
                  data-testid="es-show-blocked"
                  @click="showBlocked = !showBlocked">
                  {{ blocked.count }}</button
                >{{ showBlocked ? "，灰显在上面（再点一次不显示）" : blocked.detail
                }}<template v-if="result.unmatched_count > 0">{{
                  !showBlocked && blocked.detail ? "· " : " · "
                }}</template>
              </template>
              <template v-if="result.unmatched_count > 0">
                对不上这个条目的
                <button
                  v-if="!result.unmatched?.length"
                  type="button"
                  class="es-link es-link--muted"
                  data-testid="es-show-unmatched"
                  @click="showUnmatched">
                  {{ result.unmatched_count }} 条没显示
                </button>
                <template v-else>{{ result.unmatched_count }} 条，列在下面</template>
              </template>
            </span>
          </p>
          <PtPanel
            v-if="result.unmatched?.length"
            title="对不上这个条目的结果"
            icon="eye-off"
            :count="result.unmatched.length"
            padding="none"
            class="es-unmatched">
            <ul class="es-unmatched__list">
              <li
                v-for="m in result.unmatched"
                :key="`${m.site}|${m.torrent_id}`"
                class="es-unmatched__row">
                <a
                  v-if="m.url"
                  :href="m.url"
                  target="_blank"
                  rel="noopener"
                  class="es-unmatched__title"
                  >{{ m.title }}</a
                >
                <span v-else class="es-unmatched__title">{{ m.title }}</span>
                <span class="es-unmatched__meta">{{ m.site }} · {{ m.reason }}</span>
              </li>
            </ul>
          </PtPanel>
        </div>
      </div>
    </template>

    <!-- 不要：拉黑这一组还没拉黑的成员，只对这个条目（这一季） -->
    <BlockDialog
      v-model="blockOpen"
      :torrent-title="blockTargets[0]?.title ?? ''"
      :sites="blockTargets.map((m) => m.site)"
      :scope="`只对「${mediaTitle}」生效：订阅以后不选它，这里默认不显示。关键字搜索不受影响。`"
      :exclude-group="blockGroup"
      :busy="blockBusy"
      @confirm="confirmBlock" />

    <!-- 桌面窄屏的筛选抽屉：内容同左栏 -->
    <el-drawer
      v-model="drawer"
      direction="rtl"
      size="296px"
      title="筛选"
      class="es-drawer"
      append-to-body>
      <EntryRail
        v-if="result"
        :result="result"
        :profiles="profiles"
        :profile-id="profileId"
        :default-profile-name="defaultProfileName"
        :facets="facetGroups"
        :more-facets="moreGroups"
        :sel="sel"
        :show-more="showMore"
        :episode="episode"
        @season="emit('season', $event)"
        @profile="setProfile"
        @toggle="toggle"
        @more="showMore = !showMore"
        @episode="pickEpisode" />
    </el-drawer>

    <!-- 手机筛选面板（画板 10）：从底部弹出，点「看 N 条结果」才生效 -->
    <el-drawer
      v-model="panel"
      direction="btt"
      size="692px"
      :with-header="false"
      class="es-mpanel"
      append-to-body
      data-testid="es-mpanel">
      <div class="es-mpanel__grip" aria-hidden="true" />
      <div class="es-mpanel__head">
        <span class="es-mpanel__title">筛选</span>
        <button type="button" class="es-link" @click="draft = {}">重置</button>
      </div>
      <div class="es-mpanel__body">
        <FacetPills
          :groups="panelGroups"
          :sel="draft"
          @toggle="(k, v) => (draft = toggleFacet(draft, k, v))" />
      </div>
      <div class="es-mpanel__foot">
        <el-button class="es-mpanel__clear" @click="draft = {}">清空</el-button>
        <el-button
          type="primary"
          class="es-mpanel__apply"
          data-testid="es-mpanel-apply"
          @click="applyPanel">
          看 {{ panelCount }} 条结果
        </el-button>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.es {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
}

/*
 * 左栏 256 宽，白底，右边 1px 分隔线；自己滚动。结果区从左栏右边 16 起，到主区右边 16 止。
 * 搜索带（64 高）贴在滚动区顶上，左栏贴在它下面，高度是滚动区减去搜索带与底部状态条
 */
.es-body {
  display: flex;
  flex: 1 1 auto;
  align-items: flex-start;
  min-width: 0;
}

.es-body__rail {
  position: sticky;
  top: 64px;
  flex: 0 0 256px;
  width: 256px;
  height: calc(100vh - 64px - var(--pt-status-h));
  overflow-y: auto;
  background: var(--pt-surface);
  border-right: 1px solid var(--pt-border);
}

.es-results {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
  padding: 16px;
}

/* 手机：外壳的内容列已经左右各内缩 16（画板 09 的 x 16、宽 343），这里不再加 */
.es.is-mobile .es-results {
  padding: 16px 0 0;
}

/* 概况与排序（画板 06 sum y 80 高 24，卡片从 y 112 起） */
.es-sum {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  min-height: 24px;
  margin-bottom: 8px;
}

.es-sum__text {
  font-size: var(--pt-fz-body);
  font-weight: 500;
  color: var(--pt-t1);
}

.es-sum__warn {
  font-size: var(--pt-fz-label);
  color: var(--pt-warn);
  cursor: help;
}

.es-sum__err {
  margin: 0;
}

.es-sum__chip {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  height: 22px;
  padding: 0 8px;
  font: inherit;
  font-size: var(--pt-fz-label);
  color: var(--pt-p);
  cursor: pointer;
  background: var(--pt-p-soft);
  border: 0;
  border-radius: 11px;
}

.es-sum__gap {
  flex: 1 1 auto;
}

.es-sum__label {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

/* 排序：h24 r12、左右 10；选中主色 10% 底加主色 500 字 */
.es-sort {
  height: 24px;
  padding: 0 10px;
  font: inherit;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  cursor: pointer;
  background: transparent;
  border: 0;
  border-radius: 12px;
}

.es-sort.is-on {
  font-weight: 500;
  color: var(--pt-p);
  background: var(--pt-p-soft);
}

.es-sort:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

/* 手机的排序：h26 r13 的胶囊、1px 边、12 t2，右对齐（画板 09） */
.es-sum__msort {
  --el-component-size: 26px;
}

.es-sum__msort :deep(.el-select__wrapper) {
  min-height: 26px;
  padding: 0 8px 0 10px;
  font-size: var(--pt-fz-sm);
  border-radius: 13px;
}

/*
 * 手机上「N 个站点没回应」单独一行放在概况下面（画板 09 的概况一行只有字和排序）；
 * 点它看各站的原因（手机没有悬停）
 */
.es.is-mobile .es-sum__text {
  order: 0;
}

.es.is-mobile .es-sum__chip {
  order: 1;
}

.es.is-mobile .es-sum__gap {
  order: 2;
}

.es.is-mobile .es-sum__msort {
  order: 3;
}

.es.is-mobile .es-sum__warn {
  flex-basis: 100%;
  order: 4;
  font-size: var(--pt-fz-label);
}

.es-cards {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.es.is-mobile .es-cards {
  gap: 8px;
}

.es-empty {
  margin: 16px 0 24px;
  font-size: var(--pt-fz-body);
  color: var(--pt-t3);
  text-align: center;
}

.es-link {
  padding: 0;
  margin-left: 6px;
  font: inherit;
  font-weight: 500;
  color: var(--pt-p);
  cursor: pointer;
  background: transparent;
  border: 0;
}

.es-link:hover {
  text-decoration: underline;
}

.es-link--muted {
  margin: 0;
  color: inherit;
  text-decoration: underline;
}

/* 最后一行：eye-off 14 加 12 t3 */
.es-last {
  display: flex;
  gap: 8px;
  align-items: center;
  min-height: 34px;
  margin: 12px 0 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.es-unmatched {
  margin-top: 8px;
}

.es-unmatched__list {
  padding: 0;
  margin: 0;
  list-style: none;
}

.es-unmatched__row {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 8px 16px;
  border-top: 1px solid var(--pt-border);
}

.es-unmatched__row:first-child {
  border-top: 0;
}

.es-unmatched__title {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  color: var(--pt-t2);
  text-decoration: none;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.es-unmatched__meta {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 桌面窄屏：条目一行 + 「筛选 · N」 */
.es-narrow {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 12px 16px;
  background: var(--pt-surface);
  border-bottom: 1px solid var(--pt-border);
}

.es-narrow__text {
  flex: 1 1 auto;
  min-width: 0;
}

.es-narrow__title {
  margin: 0;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  color: var(--pt-t1);
}

.es-narrow__note {
  margin: 2px 0 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 手机：控件行 h36（季 92、档案 136、筛选 99），条目卡 343 宽的行卡；左右的 16 是外壳内容列给的 */
.es-mctl {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 8px 0 0;
}

.es-mctl__season {
  flex: 0 0 92px;
}

.es-mctl__profile {
  flex: 1 1 136px;
  min-width: 0;
}

.es-mctl :deep(.el-select__wrapper) {
  min-height: 36px;
  font-size: var(--pt-fz-body);
}

.es-mctl__filter {
  display: inline-flex;
  flex: 0 0 99px;
  gap: 6px;
  align-items: center;
  justify-content: center;
  height: 36px;
  font: inherit;
  font-size: var(--pt-fz-body);
  color: var(--pt-t2);
  cursor: pointer;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.es-mctl__filter.is-on {
  font-weight: 500;
  color: var(--pt-p);
  background: var(--pt-p-soft);
  border-color: transparent;
}

/*
 * 条目卡（画板 09，343×118）：海报 14,14；标题 66,12；说明 66,31；格子从 66,56 起，两行到 105，下边 13。
 * 数都按卡的外沿量，CSS 的 1px 边框占在外面，所以内边距各减 1
 */
.es-mitem {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  padding: 11px 13px 12px;
  margin: 12px 0 0;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

.es-mitem > :first-child {
  margin-top: 2px;
}

.es-mitem__text {
  flex: 1 1 auto;
  min-width: 0;
}

.es-mitem__title {
  margin: 0;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  line-height: 1.4;
  color: var(--pt-t1);
}

.es-mitem__link {
  color: inherit;
  text-decoration: none;
}

.es-mitem__link:hover {
  color: var(--pt-p);
}

.es-mitem__sub {
  margin: 1px 0 9px;
  font-size: var(--pt-fz-label);
  line-height: 1.4;
  color: var(--pt-t3);
}

/* 手机筛选面板：上面两个角 r16；把手 36×4；底部「清空」100、「看 N 条结果」235 */
.es-mpanel__grip {
  width: 36px;
  height: 4px;
  margin: 8px auto 0;
  background: var(--pt-border-strong);
  border-radius: 2px;
}

.es-mpanel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 48px;
  padding: 0 16px;
  border-bottom: 1px solid var(--pt-border);
}

.es-mpanel__title {
  font-size: var(--pt-fz-h2);
  font-weight: 700;
  color: var(--pt-t1);
}

.es-mpanel__body {
  flex: 1 1 auto;
  padding: 16px;
  overflow-y: auto;
}

.es-mpanel__foot {
  display: flex;
  gap: 8px;
  padding: 16px;
  border-top: 1px solid var(--pt-border);
}

.es-mpanel__foot .el-button {
  height: 44px;
  margin: 0;
}

.es-mpanel__clear {
  flex: 0 0 100px;
}

.es-mpanel__apply {
  flex: 1 1 auto;
}
</style>

<style>
/* el-drawer 挂在 body 上，scoped 样式够不到 */
.es-mpanel.el-drawer {
  max-height: calc(100vh - 120px);
  border-radius: 16px 16px 0 0;
}

.es-mpanel .el-drawer__body {
  display: flex;
  flex-direction: column;
  padding: 0;
}

.es-drawer .el-drawer__body {
  padding: 0;
}
</style>
