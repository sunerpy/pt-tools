<script setup lang="ts">
/*
 * 条目详情（媒体中心 M3，画板 04 桌面、08 手机）：一部电影或剧集的资料、你的 NAS 上（库里、做种、订阅）、
 * 季与集（每集的状态、播出日、进度）、这一季的种子（能不能删）、演职员与推荐。只读；动作交给现有的弹窗：
 * 搜资源打开按条目搜索，追剧打开订阅弹窗或追剧详情，重新刮削调整理历史的刮削，手动整理打开整理弹窗。
 * 媒体服务器（M6）：「你的 NAS 上」第 4 行、「在 Jellyfin 播放」、集卡的播放圆。
 * 删除（M7）：大图里的「删除」（手机在「更多」里）、种子行的「删除这个种子」、库里的文件不见了与服务器里删了的「处理」
 * 都打开删除弹窗（画板 03 的片段）；「忽略」把问题改成忽略。
 */
import {
  type ExploreItem,
  type ItemDetail,
  type ItemTorrent,
  type MediaKind,
  type SeasonDetail,
  type SeasonEpisode,
  organizeApi,
  subscribeApi,
} from "@/api";
import MediaDeleteDialog from "@/components/media/MediaDeleteDialog.vue";
import OrganizeDialog, { type OrganizeTarget } from "@/components/media/OrganizeDialog.vue";
import SubscribeDialog, { type SubscribeTarget } from "@/components/media/SubscribeDialog.vue";
import SubscriptionDetailDialog from "@/components/media/SubscriptionDetailDialog.vue";
import PtIcon from "@/components/PtIcon";
import BlockDialog, { type BlockChoice } from "@/components/search/BlockDialog.vue";
import EntryPoster from "@/components/search/EntryPoster.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtProgress from "@/components/ui/PtProgress.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { detailTitle } from "@/composables/useDetailTitle";
import { useIsMobile } from "@/composables/useIsMobile";
import { useTmdbImageBase } from "@/composables/useTmdbImageBase";
import { mediaKindLabel, posterURL, rescrapeOutcome, tmdbPageURL } from "@/utils/media";
import { type DeletePreset, type DeleteTarget, issueSelector } from "@/utils/mediaDelete";
import {
  airDayText,
  collapsedCount,
  collapsedText,
  creditsLine,
  defaultSeason,
  deletableView,
  type EpisodeAction,
  episodeActionLabel,
  episodeActions,
  episodeIssues,
  episodeNote,
  episodePlayURL,
  episodeStatePill,
  infoPills,
  type IssueRef,
  itemFrom,
  itemLink,
  legendCounts,
  nasLines,
  originalLine,
  pickSubscription,
  playTarget,
  protectRulesText,
  searchLink,
  seasonTabText,
  seasonTorrents,
  subscriptionLabel,
  torrentData,
  torrentLine,
  torrentStatePill,
  votesText,
} from "@/utils/mediaItem";
import { useElementSize } from "@vueuse/core";
import { ElMessage } from "element-plus";
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();
const isMobile = useIsMobile();
// 「媒体识别」里换了 TMDB 图片地址时，海报从换的地址加载
useTmdbImageBase();
const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;

const kind = computed<MediaKind>(() => (route.params.type === "tv" ? "tv" : "movie"));
const tmdbID = computed(() => Number(route.params.id) || 0);
const from = computed(() => itemFrom(route.query.from));

const detail = ref<ItemDetail | null>(null);
const loading = ref(false);
const loadError = ref("");
let loadSeq = 0;

/** 手机上点简介展开全文（画板 08：三行省略，点了展开），再点收起；桌面悬停看全文 */
const overviewOpen = ref(false);
function toggleOverview() {
  if (isMobile.value) overviewOpen.value = !overviewOpen.value;
}

async function load() {
  const seq = ++loadSeq;
  loading.value = true;
  loadError.value = "";
  overviewOpen.value = false;
  try {
    const got = await subscribeApi.item(kind.value, tmdbID.value);
    if (seq !== loadSeq) return;
    detail.value = got;
    detailTitle.value = got.item.title;
    if (kind.value === "tv") {
      const wanted = Number(route.query.season);
      const n = defaultSeason(got, Number.isInteger(wanted) ? wanted : 0);
      if (n !== season.value) season.value = n;
      else void loadSeason(n, true);
    }
  } catch (e) {
    if (seq !== loadSeq) return;
    loadError.value = errText(e, "读取条目失败");
  } finally {
    if (seq === loadSeq) loading.value = false;
  }
}

// ---- 季与集 ----
const season = ref(0);
const seasonData = ref<SeasonDetail | null>(null);
const seasonLoading = ref(false);
const seasonError = ref("");
const expanded = ref(false);
let seasonSeq = 0;

async function loadSeason(n: number, force = false) {
  if (kind.value !== "tv" || n < 0) return;
  if (!force && seasonData.value?.season === n && seasonData.value.tmdb_id === tmdbID.value) return;
  const seq = ++seasonSeq;
  seasonLoading.value = true;
  seasonError.value = "";
  expanded.value = false;
  try {
    const got = await subscribeApi.itemSeason(tmdbID.value, n);
    if (seq === seasonSeq) seasonData.value = got;
  } catch (e) {
    if (seq !== seasonSeq) return;
    seasonData.value = null;
    seasonError.value = errText(e, "读取这一季失败");
  } finally {
    if (seq === seasonSeq) seasonLoading.value = false;
  }
}

watch(season, (n) => {
  void loadSeason(n);
  if (kind.value === "tv" && n > 0 && String(n) !== route.query.season) {
    void router.replace({ query: { ...route.query, season: String(n) } });
  }
});

watch(
  () => [kind.value, tmdbID.value] as const,
  () => {
    detail.value = null;
    seasonData.value = null;
    season.value = 0;
    detailTitle.value = "";
    void load();
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  detailTitle.value = "";
});

const info = computed(() => detail.value?.item ?? null);
const episodes = computed<SeasonEpisode[]>(() => seasonData.value?.episodes ?? []);
const sub = computed(() =>
  detail.value ? pickSubscription(detail.value.subscriptions, season.value) : null,
);
const seasonSubscribed = computed(
  () => detail.value?.subscriptions.some((s) => s.season === season.value) ?? false,
);
const pills = computed(() => (info.value ? infoPills(info.value) : []));
const credits = computed(() => (info.value ? creditsLine(info.value) : ""));
const original = computed(() => (info.value ? originalLine(info.value) : ""));
const nas = computed(() => (detail.value ? nasLines(detail.value, sub.value) : []));
/** 「在 Jellyfin 播放」：桌面在动作行里，手机在「你的 NAS 上」白卡底部 */
const play = computed(() => (detail.value ? playTarget(detail.value) : null));
const playLabel = computed(() => (play.value ? `在 ${play.value.name} 播放` : ""));
const legend = computed(() => legendCounts(episodes.value));
const now = ref(new Date());
/** 开头连续已入库、可以收成一行的集数；展开以后这一行换成「收起」 */
const collapsible = computed(() => collapsedCount(episodes.value, isMobile.value));
const collapsed = computed(() => (expanded.value ? 0 : collapsible.value));
const shownEpisodes = computed(() => episodes.value.slice(collapsed.value));

/**
 * 首页的「看版本」带着 ?episode=N 进来：这一季读完以后滚到那一集（折起来的那段里时先展开），只滚一次。
 */
const targetEpisode = Number(route.query.episode) || 0;
let scrolledTo = false;
watch(seasonData, async (d) => {
  if (scrolledTo || targetEpisode <= 0 || !d) return;
  const at = d.episodes.findIndex((e) => e.episode === targetEpisode);
  if (at < 0) return;
  scrolledTo = true;
  if (at < collapsed.value) expanded.value = true;
  await nextTick();
  document
    .querySelector<HTMLElement>(`[data-testid="mi-ep-${targetEpisode}"]`)
    ?.scrollIntoView?.({ block: "center" });
});
/** 还没播的第一集（「播出后 2 小时开始搜」只写在它上面） */
const nextUpcoming = computed(() => episodes.value.find((e) => e.state === "upcoming")?.episode);
const lastEpisode = computed(() => episodes.value.at(-1)?.episode);
const torrents = computed<ItemTorrent[]>(() =>
  detail.value ? seasonTorrents(detail.value.torrents, kind.value, season.value) : [],
);
const torrentsTitle = computed(() => (kind.value === "tv" ? "这一季的种子" : "这部片的种子"));
const torrentsSub = computed(() => protectRulesText(detail.value?.protect_rules ?? []));
const people = computed(() => {
  const i = info.value;
  if (!i) return [];
  const directors = (i.directors ?? []).map((name) => ({ name, role: "导演", profile: "" }));
  const cast = (i.cast ?? []).map((c) => ({
    name: c.name,
    role: c.character ?? "",
    profile: c.profile_path ?? "",
  }));
  return [...directors, ...cast];
});
const castAll = ref(false);
const recs = computed<ExploreItem[]>(() => detail.value?.recommendations ?? []);
const backdrop = computed(() =>
  posterURL(info.value?.backdrop_path, isMobile.value ? "w780" : "w1280"),
);
const backdropFailed = ref(false);
watch(backdrop, () => (backdropFailed.value = false));
const headSub = computed(() => {
  if (!info.value) return "";
  const parts = [mediaKindLabel(kind.value)];
  if (info.value.year) parts.push(String(info.value.year));
  if (sub.value) parts.push(subscriptionLabel(kind.value, sub.value).split(" · ")[0]);
  return parts.join(" · ");
});

/** 主区够宽（≥ 1000）时「你的 NAS 上」放在大图里，否则放到大图下面（同手机的白卡） */
const page = ref<HTMLElement | null>(null);
const { width: pageWidth } = useElementSize(page);
const nasInHero = computed(
  () => !isMobile.value && (pageWidth.value === 0 || pageWidth.value >= 1000),
);
/**
 * 大图的按钮一行（画板 04 量的是 1144 宽的大图）在主区窄时放不下：右边界跟文字列一样，没有「你的 NAS 上」时离大图右边 28，
 * 有时离 300（玻璃卡 248、右边 28、再空 24）。放不下时播放只留图标，重新刮削、删除收进「更多」（同手机）。
 * 这一行不换行，量到的宽就是原宽；收起以后量到的是收起后的宽，所以用收起前最后一次量到的。量不到宽度（还没排版）时不收。
 */
const acts = ref<HTMLElement | null>(null);
const { width: actsWidth } = useElementSize(acts);
const fullActsWidth = ref(0);
const compactActs = computed(() => {
  if (isMobile.value || pageWidth.value === 0) return false;
  return fullActsWidth.value > pageWidth.value - 224 - (nasInHero.value ? 300 : 28);
});
watch(actsWidth, (w) => {
  if (!compactActs.value && w > 0) fullActsWidth.value = w;
});
/**
 * 桌面上演职员、推荐一行放得下几个放几个（画板 04：每格 108 加 8、海报 116 加 12），不露半个；
 * 演职员点「全部 N 位」展开成多行。手机横滑，全部列出。量不到宽度（还没排版）时不截。
 */
function fitCount(pitch: number, gap: number): number {
  return pageWidth.value > 0 ? Math.max(1, Math.floor((pageWidth.value + gap) / pitch)) : Infinity;
}
const shownPeople = computed(() =>
  isMobile.value || castAll.value ? people.value : people.value.slice(0, fitCount(116, 8)),
);
const castFolds = computed(() => pageWidth.value === 0 || people.value.length > fitCount(116, 8));
const shownRecs = computed(() =>
  isMobile.value ? recs.value : recs.value.slice(0, fitCount(128, 12)),
);

// ---- 动作 ----
function searchResources(n?: number) {
  void router.push(searchLink(kind.value, tmdbID.value, n ?? season.value));
}

const subscribeOpen = ref(false);
const subscribeSeason = ref(0);
const subscribeTarget = computed<SubscribeTarget | null>(() =>
  info.value
    ? {
        kind: kind.value,
        tmdb_id: tmdbID.value,
        title: info.value.title,
        year: info.value.year,
        poster_path: info.value.poster_path,
      }
    : null,
);

function subscribe(n: number) {
  subscribeSeason.value = n;
  subscribeOpen.value = true;
}

const subDetailOpen = ref(false);
const subDetailID = ref<number | null>(null);
const statusBusy = ref(false);

async function onSubCommand(cmd: string) {
  const s = sub.value;
  if (cmd === "open" && s) {
    subDetailID.value = s.id;
    subDetailOpen.value = true;
  } else if ((cmd === "pause" || cmd === "resume") && s) {
    statusBusy.value = true;
    try {
      await subscribeApi.setStatus(s.id, cmd === "pause" ? "paused" : "active");
      ElMessage.success(cmd === "pause" ? "已暂停" : "已恢复");
      await load();
    } catch (e) {
      ElMessage.error(errText(e, "操作失败"));
    } finally {
      statusBusy.value = false;
    }
  } else if (cmd === "season") {
    subscribe(season.value);
  }
}

const rescraping = ref(false);
async function rescrape() {
  const id = detail.value?.nas.library.record_id;
  if (!id) return;
  rescraping.value = true;
  try {
    const out = rescrapeOutcome(await organizeApi.rescrape(id));
    ElMessage[out.level](out.text);
  } catch (e) {
    ElMessage.error(errText(e, "重新刮削失败"));
  } finally {
    rescraping.value = false;
  }
}

const organizeOpen = ref(false);
const organizeTarget = ref<OrganizeTarget | null>(null);

async function onMore(cmd: string) {
  if (cmd === "tmdb") {
    window.open(tmdbPageURL(kind.value, tmdbID.value), "_blank", "noopener");
  } else if (cmd === "copy") {
    try {
      await navigator.clipboard.writeText(String(tmdbID.value));
      ElMessage.success(`已复制 TMDB 编号 ${tmdbID.value}`);
    } catch {
      ElMessage.warning(`没能复制，TMDB 编号是 ${tmdbID.value}`);
    }
  } else if (cmd === "organize") {
    organizeTarget.value = { source_path: "", media_type: kind.value, tmdb_id: tmdbID.value };
    organizeOpen.value = true;
  } else if (cmd === "rescrape") {
    await rescrape();
  } else if (cmd === "delete") {
    openDelete({ kind: "item" });
  }
}

function onEpisodeAction(ep: SeasonEpisode, act: EpisodeAction) {
  if (act === "ignore") void ignoreIssues(episodeIssues(ep));
  else if (act === "handle") handleIssues([{ id: 0, season: season.value, episode: ep.episode }]);
  // 等版本看能补这一集的其他版本、缺的去搜：都是按条目搜索这一季
  else searchResources(season.value);
}

function openPlay(url: string) {
  window.open(url, "_blank", "noopener");
}

/** 点已入库的集卡在媒体服务器里播放（卡上的链接与按钮自己处理） */
function onEpisodeCard(ep: SeasonEpisode, e: MouseEvent) {
  const url = episodePlayURL(ep);
  if (!url || (e.target as HTMLElement | null)?.closest("a, button")) return;
  openPlay(url);
}

/** 忽略库里的文件不见了、媒体服务器里删了的问题：文件回来、服务器里又有了以前不再提示 */
const ignoring = ref(false);
async function ignoreIssues(ids: number[]) {
  if (!ids.length || ignoring.value) return;
  ignoring.value = true;
  try {
    await Promise.all(ids.map((id) => organizeApi.dismissIssue(id)));
    ElMessage.success("已忽略");
    await load();
  } catch (e) {
    ElMessage.error(errText(e, "忽略失败"));
  } finally {
    ignoring.value = false;
  }
}

// ---- 种子 ----
const sheetTorrent = ref<ItemTorrent | null>(null);
const sheetOpen = ref(false);
const blockOpen = ref(false);
const blockBusy = ref(false);
const blockTarget = ref<ItemTorrent | null>(null);

function openInDownloader(t: ItemTorrent) {
  sheetOpen.value = false;
  void router.push({
    path: "/downloader-hub",
    query: { downloader_id: String(t.downloader_id), search: t.hash },
  });
}

function askBlock(t: ItemTorrent) {
  sheetOpen.value = false;
  blockTarget.value = t;
  blockOpen.value = true;
}

async function onTorrentCommand(cmd: string, t: ItemTorrent) {
  if (cmd === "open") openInDownloader(t);
  else if (cmd === "delete") deleteTorrent(t);
  else if (cmd === "block") askBlock(t);
}

// ---- 删除（M7） ----
const deleteOpen = ref(false);
const deleteTarget = ref<DeleteTarget | null>(null);
/** 有可删的（库里的文件、种子或订阅）时才给「删除」 */
const canDeleteItem = computed(() => {
  const d = detail.value;
  return Boolean(d && (d.nas.library.files > 0 || d.torrents.length || d.subscriptions.length));
});
const deleteTip = computed(() => {
  if (kind.value !== "tv") return "删除";
  return season.value === 0 ? "删除特别篇" : `删除第 ${season.value} 季`;
});

/** 打开删除弹窗：范围默认是这部电影、正在看的这一季 */
function openDelete(preset: DeletePreset, selector?: DeleteTarget["selector"]) {
  if (!info.value) return;
  sheetOpen.value = false;
  deleteTarget.value = {
    selector: selector ?? {
      media_type: kind.value,
      tmdb_id: tmdbID.value,
      season: kind.value === "tv" ? season.value : 0,
    },
    title: info.value.title,
    preset,
  };
  deleteOpen.value = true;
}

/** 种子行的「删除这个种子」：只勾这一个 */
function deleteTorrent(t: ItemTorrent) {
  openDelete({ kind: "torrent", key: { downloader_id: t.downloader_id, hash: t.hash } });
}

/** 「处理」：打开删除预览，只删有问题的那几集（电影是整部） */
function handleIssues(issues: readonly IssueRef[]) {
  if (!issues.length) return;
  openDelete({ kind: "issues" }, issueSelector(kind.value, tmdbID.value, issues));
}

function openSheet(t: ItemTorrent) {
  sheetTorrent.value = t;
  sheetOpen.value = true;
}

const mediaTitle = computed(() => {
  const i = info.value;
  if (!i) return "";
  const base = i.year ? `${i.title} (${i.year})` : i.title;
  return kind.value === "tv" ? `${base} 第 ${season.value} 季` : base;
});

async function confirmBlock(choice: BlockChoice) {
  const t = blockTarget.value;
  if (!t || !t.torrent_id) return;
  blockBusy.value = true;
  try {
    await subscribeApi.addBlocks({
      media_type: kind.value,
      tmdb_id: tmdbID.value,
      season: kind.value === "tv" ? t.season || season.value : 0,
      media_title: mediaTitle.value,
      reason: choice.reason,
      items: [{ site: t.site, torrent_id: t.torrent_id, title: t.title }],
    });
    ElMessage.success("已拉黑，订阅以后不再选它");
    blockOpen.value = false;
  } catch (e) {
    ElMessage.error(errText(e, "拉黑失败"));
  } finally {
    blockBusy.value = false;
  }
}

function openRec(r: ExploreItem) {
  void router.push(itemLink(r.media_type, r.id, from.value));
}

function stillOf(ep: SeasonEpisode): string {
  return posterURL(ep.still_path, "w300");
}

const failedStills = ref(new Set<number>());
watch(seasonData, () => (failedStills.value = new Set()));
const failedFaces = ref(new Set<string>());
</script>

<template>
  <div ref="page" class="mi" data-testid="mi">
    <PtHeadSub v-if="isMobile && headSub">{{ headSub }}</PtHeadSub>

    <PtDataState
      v-if="!detail"
      :state="loading ? 'loading' : loadError ? 'error' : 'loading'"
      :title="loadError ? '没读到这个条目' : ''"
      :sub="loadError"
      data-testid="mi-state">
      <template v-if="loadError" #action>
        <el-button size="small" data-testid="mi-reload" @click="load">
          <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
        </el-button>
      </template>
    </PtDataState>

    <template v-else-if="info">
      <!-- 大图：背景、海报、名字与资料、动作；桌面主区够宽时右边是「你的 NAS 上」 -->
      <section class="mi-hero" data-testid="mi-hero">
        <img
          v-if="backdrop && !backdropFailed"
          class="mi-hero__bg"
          :src="backdrop"
          alt=""
          @error="backdropFailed = true" />
        <div class="mi-hero__shade" aria-hidden="true" />
        <EntryPoster
          class="mi-hero__poster"
          :path="info.poster_path"
          size="w342"
          :width="isMobile ? 108 : 168"
          :height="isMobile ? 162 : 252"
          :radius="8" />
        <div class="mi-hero__text">
          <h2 class="mi-hero__title" :title="info.title" data-testid="mi-title">
            {{ info.title }}
          </h2>
          <div v-if="original" class="mi-hero__original">{{ original }}</div>
          <div v-if="!isMobile" class="mi-hero__pills" data-testid="mi-pills">
            <span v-for="p in pills" :key="p" class="mi-glass">{{ p }}</span>
          </div>
          <div class="mi-hero__rating" data-testid="mi-rating">
            <template v-if="info.rating">
              <PtIcon name="star" :size="isMobile ? 14 : 16" class="mi-hero__star" />
              <span class="mi-hero__score">{{ info.rating.toFixed(1) }}</span>
              <span class="mi-hero__votes">{{ votesText(info.votes) }}</span>
            </template>
            <a
              v-if="info.imdb_id && !isMobile"
              class="mi-glass mi-glass--link"
              :href="`https://www.imdb.com/title/${info.imdb_id}/`"
              target="_blank"
              rel="noopener"
              data-testid="mi-imdb"
              >IMDb</a
            >
            <a
              v-if="info.tvdb_id && !isMobile"
              class="mi-glass mi-glass--link"
              :href="`https://www.thetvdb.com/dereferrer/series/${info.tvdb_id}`"
              target="_blank"
              rel="noopener"
              data-testid="mi-tvdb"
              >TVDB</a
            >
          </div>
          <div v-if="isMobile" class="mi-hero__pills" data-testid="mi-pills">
            <span v-for="p in pills" :key="p" class="mi-glass">{{ p }}</span>
          </div>
        </div>
        <p
          v-if="info.overview"
          class="mi-hero__overview"
          :class="{ 'is-open': isMobile && overviewOpen }"
          :title="isMobile ? undefined : info.overview"
          :role="isMobile ? 'button' : undefined"
          :tabindex="isMobile ? 0 : undefined"
          :aria-expanded="isMobile ? overviewOpen : undefined"
          data-testid="mi-overview"
          @click="toggleOverview"
          @keydown.enter="toggleOverview">
          {{ info.overview }}
        </p>
        <div v-if="credits" class="mi-hero__credits" :title="credits">{{ credits }}</div>
        <div ref="acts" class="mi-hero__acts" data-testid="mi-acts">
          <button
            type="button"
            class="mi-btn mi-btn--primary"
            data-testid="mi-search"
            @click="searchResources()">
            <PtIcon name="search" :size="15" /><span>搜资源</span>
          </button>
          <el-dropdown
            v-if="sub"
            trigger="click"
            :disabled="statusBusy"
            data-testid="mi-sub-menu"
            @command="onSubCommand">
            <button type="button" class="mi-btn mi-btn--glass" data-testid="mi-sub">
              <PtIcon name="bookmark" :size="15" />
              <span>{{ subscriptionLabel(kind, sub) }}</span>
              <PtIcon name="chevron-down" :size="12" />
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="open" data-testid="mi-sub-open">
                  {{ sub.follow ? "打开追剧详情" : "打开订阅详情" }}
                </el-dropdown-item>
                <el-dropdown-item v-if="sub.status === 'active'" command="pause"
                  >暂停</el-dropdown-item
                >
                <el-dropdown-item v-else-if="sub.status === 'paused'" command="resume"
                  >恢复</el-dropdown-item
                >
                <el-dropdown-item
                  v-if="kind === 'tv' && !seasonSubscribed && season > 0"
                  command="season"
                  data-testid="mi-sub-season">
                  追第 {{ season }} 季
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <button
            v-else
            type="button"
            class="mi-btn mi-btn--glass"
            data-testid="mi-sub"
            @click="subscribe(season)">
            <PtIcon name="bookmark" :size="15" />
            <span>{{ kind === "tv" ? "追剧" : "订阅" }}</span>
          </button>
          <template v-if="!isMobile">
            <el-tooltip v-if="play" :content="playLabel" placement="top" :disabled="!compactActs">
              <button
                type="button"
                class="mi-btn mi-btn--glass"
                :class="{ 'mi-btn--square': compactActs }"
                :aria-label="compactActs ? playLabel : undefined"
                data-testid="mi-play"
                @click="openPlay(play.url)">
                <PtIcon name="play" :size="compactActs ? 16 : 15" />
                <span v-if="!compactActs">{{ playLabel }}</span>
              </button>
            </el-tooltip>
            <el-tooltip
              v-if="!compactActs && detail.nas.library.record_id"
              content="重新刮削"
              placement="top">
              <button
                type="button"
                class="mi-btn mi-btn--glass mi-btn--square"
                aria-label="重新刮削"
                :disabled="rescraping"
                data-testid="mi-rescrape"
                @click="rescrape">
                <PtIcon name="rotate-ccw" :size="16" />
              </button>
            </el-tooltip>
            <el-tooltip v-if="!compactActs && canDeleteItem" :content="deleteTip" placement="top">
              <button
                type="button"
                class="mi-btn mi-btn--glass mi-btn--square"
                :aria-label="deleteTip"
                data-testid="mi-delete"
                @click="openDelete({ kind: 'item' })">
                <PtIcon name="trash-2" :size="16" />
              </button>
            </el-tooltip>
            <el-dropdown trigger="click" data-testid="mi-more-menu" @command="onMore">
              <button
                type="button"
                class="mi-btn mi-btn--glass mi-btn--square"
                aria-label="更多"
                data-testid="mi-more">
                <PtIcon name="ellipsis" :size="16" />
              </button>
              <template #dropdown>
                <el-dropdown-menu>
                  <template v-if="compactActs">
                    <el-dropdown-item
                      v-if="detail.nas.library.record_id"
                      command="rescrape"
                      :disabled="rescraping"
                      data-testid="mi-more-rescrape"
                      >重新刮削</el-dropdown-item
                    >
                    <el-dropdown-item
                      v-if="canDeleteItem"
                      command="delete"
                      data-testid="mi-more-delete"
                      >{{ deleteTip }}</el-dropdown-item
                    >
                  </template>
                  <el-dropdown-item command="tmdb">在 TMDB 打开</el-dropdown-item>
                  <el-dropdown-item command="copy">复制 TMDB 编号</el-dropdown-item>
                  <el-dropdown-item command="organize" data-testid="mi-organize"
                    >手动整理到这个条目</el-dropdown-item
                  >
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </div>
        <!-- 手机：顶栏右边是外壳的搜索、通知、主题，「更多」放在大图右上角 -->
        <el-dropdown v-if="isMobile" class="mi-hero__more" trigger="click" @command="onMore">
          <button
            type="button"
            class="mi-btn mi-btn--glass mi-btn--square"
            aria-label="更多"
            data-testid="mi-more">
            <PtIcon name="ellipsis" :size="16" />
          </button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-if="detail.nas.library.record_id" command="rescrape"
                >重新刮削</el-dropdown-item
              >
              <el-dropdown-item
                v-if="canDeleteItem"
                command="delete"
                data-testid="mi-more-delete"
                >{{ deleteTip }}</el-dropdown-item
              >
              <el-dropdown-item command="tmdb">在 TMDB 打开</el-dropdown-item>
              <el-dropdown-item command="copy">复制 TMDB 编号</el-dropdown-item>
              <el-dropdown-item command="organize">手动整理到这个条目</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <div v-if="nasInHero" class="mi-nas mi-nas--glass" data-testid="mi-nas">
          <div class="mi-nas__title">你的 NAS 上</div>
          <div v-for="l in nas" :key="l.key" class="mi-nas__row" :data-testid="`mi-nas-${l.key}`">
            <PtIcon :name="l.icon" :size="15" class="mi-nas__icon" />
            <div class="mi-nas__body">
              <div class="mi-nas__head" :title="l.title">{{ l.title }}</div>
              <div v-if="l.desc" class="mi-nas__desc" :class="{ 'is-warn': l.warn }">
                <span class="mi-nas__desc-text" :title="l.desc">{{ l.desc }}</span>
                <template v-if="l.issues?.length">
                  ·
                  <button
                    type="button"
                    class="mi-nas__ignore"
                    :data-testid="`mi-issue-handle-${l.key}`"
                    @click="handleIssues(l.issues)">
                    处理
                  </button>
                  ·
                  <button
                    type="button"
                    class="mi-nas__ignore"
                    :disabled="ignoring"
                    :data-testid="`mi-issue-ignore-${l.key}`"
                    @click="ignoreIssues(l.issues.map((i) => i.id))">
                    忽略
                  </button>
                </template>
              </div>
            </div>
          </div>
        </div>
      </section>

      <div v-if="!nasInHero" class="mi-nas mi-nas--card" data-testid="mi-nas">
        <div class="mi-nas__title">你的 NAS 上</div>
        <div v-for="l in nas" :key="l.key" class="mi-nas__row" :data-testid="`mi-nas-${l.key}`">
          <PtIcon
            :name="l.icon"
            :size="15"
            class="mi-nas__icon"
            :class="[`is-${l.key}`, { 'is-warn': l.warn }]" />
          <div class="mi-nas__body">
            <div class="mi-nas__head" :title="l.title">{{ l.title }}</div>
            <div v-if="l.desc" class="mi-nas__desc" :class="{ 'is-warn': l.warn }">
              <span class="mi-nas__desc-text" :title="l.desc">{{ l.desc }}</span>
              <template v-if="l.issues?.length">
                ·
                <button
                  type="button"
                  class="mi-nas__ignore"
                  :data-testid="`mi-issue-handle-${l.key}`"
                  @click="handleIssues(l.issues)">
                  处理
                </button>
                ·
                <button
                  type="button"
                  class="mi-nas__ignore"
                  :disabled="ignoring"
                  :data-testid="`mi-issue-ignore-${l.key}`"
                  @click="ignoreIssues(l.issues.map((i) => i.id))">
                  忽略
                </button>
              </template>
            </div>
          </div>
        </div>
        <!-- 手机的播放放在白卡底部（画板 08）；桌面窄的时候在大图的动作行里 -->
        <button
          v-if="isMobile && play"
          type="button"
          class="mi-nas__play"
          data-testid="mi-play"
          @click="openPlay(play.url)">
          <PtIcon name="play" :size="15" />
          <span>在 {{ play.name }} 播放</span>
        </button>
      </div>

      <div
        v-if="detail.errors.length"
        class="pt-note pt-note--warn mi-errors"
        data-testid="mi-errors">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>有几部分没取到：{{ detail.errors.join("；") }}</span>
        <button type="button" class="mi-link" @click="load">重试</button>
      </div>

      <!-- 季与集 -->
      <section v-if="kind === 'tv'" class="mi-block" data-testid="mi-seasons">
        <div class="mi-seasons__head">
          <h3 class="mi-block__title">季与集</h3>
          <div class="mi-tabs" role="tablist" data-testid="mi-tabs">
            <button
              v-for="s in detail.seasons"
              :key="s.n"
              type="button"
              role="tab"
              class="mi-tab"
              :class="{ 'is-on': s.n === season }"
              :aria-selected="s.n === season"
              :data-testid="`mi-tab-${s.n}`"
              @click="season = s.n">
              <span>{{ seasonTabText(s).name }}</span>
              <span v-if="seasonTabText(s).count">{{ seasonTabText(s).count }}</span>
            </button>
          </div>
          <div v-if="legend.length" class="mi-legend" data-testid="mi-legend">
            <span v-for="l in legend" :key="l.state" class="mi-legend__item">
              <i class="mi-legend__cell" :class="`is-${l.state}`" aria-hidden="true" />
              {{ l.label }} {{ l.count }}
            </span>
          </div>
        </div>
        <PtDataState
          v-if="!episodes.length"
          dense
          :state="seasonLoading ? 'loading' : seasonError ? 'error' : 'empty'"
          :title="seasonError ? '这一季没读到' : seasonLoading ? '' : '这一季还没有集'"
          :sub="seasonError || (seasonLoading ? '' : 'TMDB 上这一季还没列出集')"
          data-testid="mi-season-state">
          <template v-if="seasonError" #action>
            <el-button size="small" @click="loadSeason(season, true)">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>
        <div v-else class="mi-eps" data-testid="mi-eps">
          <div v-if="collapsible" class="mi-collapse" data-testid="mi-collapse">
            <span>{{ collapsedText(episodes, collapsible) }}</span>
            <button
              type="button"
              class="mi-link"
              data-testid="mi-expand"
              @click="expanded = !expanded">
              {{ expanded ? "收起" : "展开"
              }}<PtIcon :name="expanded ? 'chevron-up' : 'chevron-down'" :size="14" />
            </button>
          </div>
          <div
            v-for="ep in shownEpisodes"
            :key="ep.episode"
            class="mi-ep"
            :class="[
              `is-${ep.state}`,
              { 'is-playable': episodePlayURL(ep), 'has-two-acts': episodeActions(ep).length > 1 },
            ]"
            :data-testid="`mi-ep-${ep.episode}`"
            @click="onEpisodeCard(ep, $event)">
            <div class="mi-ep__still">
              <img
                v-if="ep.state !== 'upcoming' && stillOf(ep) && !failedStills.has(ep.episode)"
                :src="stillOf(ep)"
                alt=""
                loading="lazy"
                @error="failedStills.add(ep.episode)" />
              <template v-else-if="ep.state === 'upcoming'">
                <PtIcon name="clock" :size="isMobile ? 18 : 20" class="mi-ep__clock" />
                <span class="mi-ep__when">{{
                  airDayText(ep, now, false).replace(" · 季终", "")
                }}</span>
              </template>
              <PtIcon v-else-if="!episodePlayURL(ep)" name="compass" :size="20" />
              <!-- 已入库、媒体服务器里有这一集：在服务器里播放（点卡片也行） -->
              <a
                v-if="episodePlayURL(ep)"
                class="mi-ep__play"
                :href="episodePlayURL(ep)"
                target="_blank"
                rel="noopener"
                :aria-label="`播放第 ${ep.episode} 集`"
                :data-testid="`mi-ep-play-${ep.episode}`">
                <PtIcon name="play" :size="isMobile ? 12 : 14" />
              </a>
            </div>
            <div class="mi-ep__title" :title="ep.name">
              E{{ String(ep.episode).padStart(2, "0")
              }}<template v-if="ep.name"> · {{ ep.name }}</template>
            </div>
            <div class="mi-ep__air">{{ airDayText(ep, now, ep.episode === lastEpisode) }}</div>
            <div class="mi-ep__state">
              <PtStatusPill :tone="episodeStatePill(ep.state).tone" dot size="sm">
                {{ episodeStatePill(ep.state).label }}
              </PtStatusPill>
              <span
                class="mi-ep__note"
                :class="{ 'is-warn': ep.server_issue || ep.library_issue }"
                :title="episodeNote(ep, seasonData!, now, ep.episode === nextUpcoming)">
                {{ episodeNote(ep, seasonData!, now, ep.episode === nextUpcoming) }}
              </span>
            </div>
            <PtProgress
              v-if="ep.state === 'downloading' && ep.percent !== undefined"
              class="mi-ep__bar"
              :percent="ep.percent"
              tone="info"
              :height="3" />
            <span v-if="episodeActions(ep).length" class="mi-ep__act">
              <template v-for="(a, i) in episodeActions(ep)" :key="a">
                <span v-if="i > 0" class="mi-ep__dot" aria-hidden="true">·</span>
                <button
                  type="button"
                  class="mi-link"
                  :disabled="ignoring && a === 'ignore'"
                  :data-testid="`mi-ep-${a}-${ep.episode}`"
                  @click="onEpisodeAction(ep, a)">
                  {{ episodeActionLabel(a) }}
                </button>
              </template>
            </span>
          </div>
        </div>
      </section>

      <!-- 这一季的种子 -->
      <section class="mi-block" data-testid="mi-torrents">
        <div class="mi-block__head">
          <h3 class="mi-block__title">{{ torrentsTitle }}</h3>
          <span v-if="!isMobile" class="mi-block__sub" data-testid="mi-torrents-sub">{{
            torrentsSub
          }}</span>
        </div>
        <div class="mi-list" :class="{ 'is-mobile': isMobile }">
          <div v-if="!torrents.length" class="mi-list__empty" data-testid="mi-torrents-empty">
            {{ kind === "tv" ? "这一季没有下载过的种子" : "这部片没有下载过的种子" }}
          </div>
          <template v-else-if="!isMobile">
            <div
              v-for="t in torrents"
              :key="`${t.downloader_id}|${t.hash}`"
              class="mi-trow"
              data-testid="mi-trow">
              <span class="mi-trow__site" :title="t.site">{{ t.site || "下载器里" }}</span>
              <span class="mi-trow__title" :title="t.title">{{ torrentLine(t) }}</span>
              <span class="mi-trow__dl"
                ><PtTag :title="t.downloader">{{ t.downloader }}</PtTag></span
              >
              <span>
                <PtStatusPill :tone="torrentStatePill(t.state).tone" dot size="sm">
                  {{ torrentStatePill(t.state).label }}
                </PtStatusPill>
              </span>
              <span class="mi-trow__data">{{ torrentData(t) }}</span>
              <span class="mi-trow__guard">
                <PtStatusPill
                  :tone="deletableView(t).warn ? 'warn' : 'neutral'"
                  size="sm"
                  :title="deletableView(t).title">
                  <PtIcon v-if="deletableView(t).warn" name="shield" :size="11" />{{
                    deletableView(t).text
                  }}
                </PtStatusPill>
              </span>
              <el-dropdown
                trigger="click"
                class="mi-trow__more"
                @command="(c: string) => onTorrentCommand(c, t)">
                <button
                  type="button"
                  class="mi-icon-btn"
                  aria-label="更多"
                  data-testid="mi-trow-more">
                  <PtIcon name="ellipsis" :size="14" />
                </button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="open">在下载器里打开</el-dropdown-item>
                    <el-dropdown-item command="delete" data-testid="mi-trow-delete"
                      >删除这个种子</el-dropdown-item
                    >
                    <el-dropdown-item v-if="t.torrent_id" command="block"
                      >不要这个种子</el-dropdown-item
                    >
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
          <template v-else>
            <button
              v-for="t in torrents"
              :key="`${t.downloader_id}|${t.hash}`"
              type="button"
              class="mi-mrow"
              data-testid="mi-trow"
              @click="openSheet(t)">
              <span class="mi-mrow__top">
                <span class="mi-mrow__site">{{ t.site || "下载器里" }}</span>
                <span class="mi-mrow__title">{{ torrentLine(t, { version: false }) }}</span>
                <PtStatusPill :tone="deletableView(t).warn ? 'warn' : 'neutral'" size="sm">
                  <PtIcon v-if="deletableView(t).warn" name="shield" :size="11" />{{
                    deletableView(t).text
                  }}
                </PtStatusPill>
              </span>
              <span class="mi-mrow__bottom">
                <PtStatusPill :tone="torrentStatePill(t.state).tone" dot size="sm">
                  {{ torrentStatePill(t.state).label }}
                </PtStatusPill>
                <PtTag>{{ t.downloader }}</PtTag>
                <span class="mi-mrow__data">{{ torrentData(t, true) }}</span>
              </span>
            </button>
          </template>
        </div>
      </section>

      <!-- 演职员 -->
      <section v-if="people.length" class="mi-block" data-testid="mi-cast">
        <div class="mi-block__head">
          <h3 class="mi-block__title">演职员</h3>
          <button
            v-if="!isMobile && (castAll || castFolds)"
            type="button"
            class="mi-link mi-block__more"
            data-testid="mi-cast-all"
            @click="castAll = !castAll">
            {{ castAll ? "收起" : `全部 ${people.length} 位` }}
          </button>
        </div>
        <div class="mi-cast" :class="{ 'is-all': castAll, 'is-scroll': isMobile }">
          <div v-for="(p, i) in shownPeople" :key="`${p.name}-${i}`" class="mi-person">
            <span class="mi-person__face">
              <img
                v-if="p.profile && posterURL(p.profile, 'w185') && !failedFaces.has(p.profile)"
                :src="posterURL(p.profile, 'w185')"
                alt=""
                loading="lazy"
                @error="failedFaces.add(p.profile)" />
              <PtIcon v-else name="user" :size="isMobile ? 24 : 28" />
            </span>
            <span class="mi-person__name" :title="p.name">{{ p.name }}</span>
            <span class="mi-person__role" :title="p.role">{{ p.role }}</span>
          </div>
        </div>
      </section>

      <!-- 推荐 -->
      <section v-if="recs.length" class="mi-block" data-testid="mi-recs">
        <div class="mi-block__head">
          <h3 class="mi-block__title">推荐</h3>
          <span v-if="!isMobile" class="mi-block__sub"
            >TMDB 按这部{{ kind === "tv" ? "剧" : "片" }}推荐，标出库里有的和已订阅的</span
          >
        </div>
        <div class="mi-recs" :class="{ 'is-scroll': isMobile }">
          <button
            v-for="r in shownRecs"
            :key="`${r.media_type}-${r.id}`"
            type="button"
            class="mi-rec"
            :data-testid="`mi-rec-${r.id}`"
            @click="openRec(r)">
            <span class="mi-rec__poster">
              <EntryPoster
                :path="r.poster_path"
                size="w342"
                :width="isMobile ? 104 : 116"
                :height="isMobile ? 156 : 174"
                :radius="6" />
              <span
                v-if="r.in_library || r.subscribed"
                class="mi-rec__tag"
                :class="r.in_library ? 'is-ok' : 'is-info'">
                <i aria-hidden="true" />{{ r.in_library ? "已入库" : "已订阅" }}
              </span>
            </span>
            <span class="mi-rec__name" :title="r.title">{{ r.title }}</span>
          </button>
        </div>
      </section>
    </template>

    <SubscribeDialog
      v-model="subscribeOpen"
      :target="subscribeTarget"
      :season="kind === 'tv' ? subscribeSeason : undefined"
      @saved="load" />
    <SubscriptionDetailDialog v-model="subDetailOpen" :id="subDetailID" @changed="load" />
    <OrganizeDialog v-model="organizeOpen" :target="organizeTarget" @done="load" />
    <MediaDeleteDialog v-model="deleteOpen" :target="deleteTarget" @done="load" />
    <BlockDialog
      v-model="blockOpen"
      :torrent-title="blockTarget?.title ?? ''"
      :sites="blockTarget ? [blockTarget.site] : []"
      :scope="`只对「${mediaTitle}」生效：订阅以后不选它。下载器里的种子不动。`"
      :busy="blockBusy"
      @confirm="confirmBlock" />

    <!-- 手机：点一行种子从底部弹出操作 -->
    <el-drawer
      v-model="sheetOpen"
      direction="btt"
      size="auto"
      :with-header="false"
      append-to-body
      class="mi-sheet"
      data-testid="mi-sheet">
      <div v-if="sheetTorrent" class="mi-sheet__body">
        <div class="mi-sheet__title">{{ sheetTorrent.title }}</div>
        <button type="button" class="mi-sheet__item" @click="openInDownloader(sheetTorrent)">
          <PtIcon name="app-window" :size="16" /><span>在下载器里打开</span>
        </button>
        <button
          type="button"
          class="mi-sheet__item"
          data-testid="mi-sheet-delete"
          @click="deleteTorrent(sheetTorrent)">
          <PtIcon name="trash-2" :size="16" /><span>删除这个种子</span>
        </button>
        <button
          v-if="sheetTorrent.torrent_id"
          type="button"
          class="mi-sheet__item"
          @click="askBlock(sheetTorrent)">
          <PtIcon name="eye-off" :size="16" /><span>不要这个种子</span>
        </button>
      </div>
    </el-drawer>
  </div>
</template>

<style scoped>
.mi {
  display: flex;
  flex-direction: column;
  gap: 24px;
  min-width: 0;
  padding: var(--pt-pad) var(--pt-pad) 24px;
}

/* ---- 大图 1144×340 ---- */
.mi-hero {
  position: relative;
  height: 340px;
  overflow: hidden;
  color: #fff;
  background: linear-gradient(135deg, #22384c, #0e1724);
  border-radius: 10px;
}

.mi-hero__bg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.mi-hero__shade {
  position: absolute;
  top: 0;
  left: 0;
  width: min(760px, 100%);
  height: 100%;
  background: linear-gradient(90deg, rgb(11 18 32 / 75%), rgb(11 18 32 / 0%));
}

.mi-hero__poster {
  position: absolute;
  top: 44px;
  left: 28px;
  box-shadow: 0 10px 30px rgb(0 0 0 / 35%);
}

.mi-hero__text {
  position: absolute;
  top: 40px;
  left: 224px;
  right: 300px;
}

.mi-hero__title {
  margin: 0;
  overflow: hidden;
  font-size: 28px;
  font-weight: 700;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mi-hero__original {
  margin-top: 3px;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  line-height: 1.4;
  color: rgb(255 255 255 / 72%);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mi-hero__pills {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  height: 22px;
  margin-top: 11px;
  overflow: hidden;
}

.mi-glass {
  display: inline-flex;
  align-items: center;
  height: 22px;
  padding: 0 8px;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: #fff;
  white-space: nowrap;
  background: rgb(255 255 255 / 14%);
  border-radius: 4px;
}

.mi-glass--link {
  height: 20px;
  text-decoration: none;
}

.mi-glass--link:hover {
  background: rgb(255 255 255 / 22%);
}

.mi-hero__rating {
  display: flex;
  gap: 6px;
  align-items: center;
  height: 24px;
  margin-top: 10px;
}

.mi-hero__star {
  color: #f5b544;
}

.mi-hero__score {
  font-size: var(--pt-fz-display);
  font-weight: 700;
  line-height: 1.2;
}

.mi-hero__votes {
  margin-right: 12px;
  font-size: var(--pt-fz-sm);
  color: rgb(255 255 255 / 72%);
}

.mi-hero__overview {
  position: absolute;
  top: 178px;
  left: 224px;
  display: -webkit-box;
  width: min(600px, calc(100% - 224px - 300px));
  max-height: calc(2 * 1.6em);
  margin: 0;
  overflow: hidden;
  -webkit-line-clamp: 2;
  font-size: var(--pt-fz-body);
  line-height: 1.6;
  color: rgb(255 255 255 / 88%);
  -webkit-box-orient: vertical;
}

.mi-hero__credits {
  position: absolute;
  top: 228px;
  left: 224px;
  right: 300px;
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  line-height: 1.4;
  color: rgb(255 255 255 / 70%);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mi-hero__acts {
  position: absolute;
  top: 264px;
  left: 224px;
  display: flex;
  gap: 8px;
  align-items: center;
}

.mi-btn {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  justify-content: center;
  height: 32px;
  padding: 0 12px;
  font: inherit;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  color: #fff;
  white-space: nowrap;
  cursor: pointer;
  border: 0;
  border-radius: 6px;
}

.mi-btn:disabled {
  cursor: default;
  opacity: 0.6;
}

.mi-btn--primary {
  min-width: 96px;
  background: var(--pt-p-hover);
}

.mi-btn--primary:hover {
  background: var(--pt-p);
}

.mi-btn--glass {
  background: rgb(255 255 255 / 16%);
}

.mi-btn--glass:hover:not(:disabled) {
  background: rgb(255 255 255 / 24%);
}

.mi-btn--square {
  width: 32px;
  padding: 0;
}

/* 「你的 NAS 上」：大图里的玻璃卡 248×252，放不下时（主区 < 1000、手机）是白卡 */
.mi-nas {
  display: flex;
  flex-direction: column;
}

.mi-nas--glass {
  position: absolute;
  top: 44px;
  right: 28px;
  width: 248px;
  height: 252px;
  padding: 14px 16px;
  background: rgb(255 255 255 / 10%);
  border: 1px solid rgb(255 255 255 / 18%);
  border-radius: 10px;
}

.mi-nas--card {
  padding: 12px 14px 14px;
  color: var(--pt-t1);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

.mi-nas__title {
  font-size: var(--pt-fz-label);
  font-weight: 700;
  line-height: 1.3;
}

.mi-nas--glass .mi-nas__title {
  margin-bottom: 11px;
  color: rgb(255 255 255 / 70%);
}

.mi-nas--card .mi-nas__title {
  margin-bottom: 8px;
  color: var(--pt-t3);
}

.mi-nas__row {
  display: flex;
  gap: 9px;
  min-height: 46px;
}

.mi-nas--glass .mi-nas__row {
  min-height: 51px;
}

.mi-nas__icon {
  flex: 0 0 auto;
  margin-top: 2px;
}

.mi-nas--card .mi-nas__icon {
  color: var(--pt-t2);
}

.mi-nas--card .mi-nas__icon.is-library {
  color: var(--pt-ok);
}

.mi-nas--card .mi-nas__icon.is-seeding {
  color: var(--pt-kpi-2);
}

.mi-nas--card .mi-nas__icon.is-subscription {
  color: var(--pt-p);
}

.mi-nas__body {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.mi-nas__head {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 说明一行省略；媒体服务器那一行有问题时后面跟「忽略」（不跟着省略） */
.mi-nas__desc {
  display: flex;
  gap: 4px;
  min-width: 0;
  font-size: var(--pt-fz-label);
  line-height: 1.3;
  white-space: nowrap;
}

.mi-nas__desc-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mi-nas__ignore {
  flex: 0 0 auto;
  padding: 0;
  font: inherit;
  font-weight: 500;
  color: inherit;
  cursor: pointer;
  background: none;
  border: 0;
}

.mi-nas__ignore:hover:not(:disabled) {
  text-decoration: underline;
}

.mi-nas__ignore:disabled {
  cursor: default;
  opacity: 0.6;
}

.mi-nas--glass .mi-nas__desc {
  color: rgb(255 255 255 / 70%);
}

/* 深色大图上的 warn 用评分星的金色（画板 04：#F5B544），白卡上用 --pt-warn */
.mi-nas--glass .mi-nas__desc.is-warn {
  color: #f5b544;
}

/* 四行时最后一行不留行距，卡高 252 放得下 */
.mi-nas--glass .mi-nas__row:last-of-type {
  min-height: 0;
}

.mi-nas--card .mi-nas__desc {
  color: var(--pt-t3);
}

.mi-nas--card .mi-nas__desc.is-warn,
.mi-nas--card .mi-nas__icon.is-warn {
  color: var(--pt-warn);
}

/* 手机白卡底部的「在 Jellyfin 播放」：soft 按钮 315×36，上 8（画板 08） */
.mi-nas__play {
  display: flex;
  gap: 6px;
  align-items: center;
  justify-content: center;
  height: 36px;
  margin: 8px 0 6px;
  padding: 0 12px;
  font: inherit;
  font-size: var(--pt-fz-body);
  color: var(--pt-p);
  cursor: pointer;
  background: var(--pt-p-soft);
  border: 0;
  border-radius: 6px;
}

.mi-nas__play:hover {
  background: var(--pt-p-soft-strong);
}

.mi-errors {
  margin: 0;
}

/* ---- 区块 ---- */
.mi-block {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.mi-block__head {
  display: flex;
  gap: 24px;
  align-items: baseline;
  margin-bottom: 11px;
}

.mi-block__title {
  margin: 0;
  font-size: var(--pt-fz-h2);
  font-weight: 700;
  line-height: 1.3;
  color: var(--pt-t1);
}

.mi-block__sub {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.mi-block__more {
  margin-left: auto;
}

.mi-link {
  display: inline-flex;
  gap: 2px;
  align-items: center;
  padding: 0;
  font: inherit;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-p);
  white-space: nowrap;
  cursor: pointer;
  background: none;
  border: 0;
}

.mi-link:hover {
  text-decoration: underline;
}

/* 季与集：标题、季 tab、图例一行 */
.mi-seasons__head {
  display: flex;
  gap: 24px;
  align-items: center;
  min-height: 26px;
  margin-top: -2px;
  margin-bottom: 12px;
}

.mi-tabs {
  display: flex;
  gap: 8px;
  min-width: 0;
}

.mi-tab {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 8px;
  align-items: center;
  height: 26px;
  padding: 0 12px;
  font: inherit;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  white-space: nowrap;
  cursor: pointer;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: 13px;
}

.mi-tab:hover:not(.is-on) {
  border-color: var(--pt-border-strong);
}

.mi-tab.is-on {
  font-weight: 500;
  color: var(--pt-surface);
  background: var(--pt-t1);
  border-color: var(--pt-t1);
}

.mi-legend {
  display: flex;
  flex: 0 0 auto;
  gap: 16px;
  align-items: center;
  margin-left: auto;
  font-size: var(--pt-fz-label);
  color: var(--pt-t2);
}

.mi-legend__item {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  white-space: nowrap;
}

.mi-legend__cell {
  width: 8px;
  height: 8px;
  border-radius: 2px;
}

.mi-legend__cell.is-library {
  background: var(--pt-ok);
}

.mi-legend__cell.is-downloading {
  background: var(--pt-info);
}

.mi-legend__cell.is-waiting_version {
  background: var(--pt-p);
}

.mi-legend__cell.is-missing {
  background: var(--pt-warn);
}

.mi-legend__cell.is-upcoming {
  background: color-mix(in srgb, var(--pt-t3) 30%, var(--pt-surface));
}

/* 集卡 370×92：三列（1144 宽），列间距 17、行间距 12 */
.mi-eps {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 12px 17px;
}

.mi-collapse {
  display: flex;
  grid-column: 1 / -1;
  align-items: center;
  justify-content: space-between;
  height: 44px;
  padding: 0 14px;
  font-size: var(--pt-fz-body);
  color: var(--pt-t2);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

.mi-ep {
  position: relative;
  height: 92px;
  overflow: hidden;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: 0 1px 2px rgb(27 29 42 / 6%);
}

.mi-ep__still {
  position: absolute;
  top: 9px;
  left: 9px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: 128px;
  height: 72px;
  overflow: hidden;
  /* 画板的时钟是 t4；全站 t4 只给分隔符与禁用态（paletteContrast 钉子），图标落成 t3 */
  color: var(--pt-t3);
  background: var(--pt-hover);
  border-radius: 6px;
}

.mi-ep__still img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.mi-ep.is-playable {
  cursor: pointer;
}

/* 播放圆：剧照正中 32（手机 28），黑 0.4 */
.mi-ep__play {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  margin: auto;
  color: #fff;
  background: rgb(0 0 0 / 40%);
  border-radius: 50%;
}

.mi-ep__play:hover {
  background: rgb(0 0 0 / 55%);
}

.mi-ep__clock {
  margin-top: -6px;
}

.mi-ep__when {
  margin-top: 6px;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: var(--pt-t3);
}

.mi-ep__title {
  position: absolute;
  top: 11px;
  left: 149px;
  right: 72px;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  line-height: 1.4;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mi-ep.is-upcoming .mi-ep__title {
  color: var(--pt-t3);
}

.mi-ep__air {
  position: absolute;
  top: 31px;
  left: 149px;
  font-size: var(--pt-fz-label);
  line-height: 1.3;
  color: var(--pt-t3);
  white-space: nowrap;
}

.mi-ep__state {
  position: absolute;
  top: 57px;
  left: 149px;
  right: 14px;
  display: flex;
  gap: 8px;
  align-items: center;
}

.mi-ep__note {
  min-width: 0;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mi-ep__note.is-warn {
  color: var(--pt-warn);
}

.mi-ep__bar {
  position: absolute;
  top: 83px;
  left: 149px;
  width: 204px;
}

.mi-ep__act {
  position: absolute;
  top: 12px;
  right: 13px;
  display: inline-flex;
  gap: 4px;
  align-items: center;
}

.mi-ep__dot {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

/* 「处理 · 忽略」两个动作时名字多让出一些 */
.mi-ep.has-two-acts .mi-ep__title {
  right: 92px;
}

/* 种子列表：一行 50，列按画板（站点 84、种子伸缩、下载器跟着名字宽、状态 76、数据 224、能不能删 308、更多 20） */
.mi-list {
  display: grid;
  grid-template-columns: 84px minmax(0, 1fr) auto 76px 224px minmax(0, 308px) 20px;
  padding: 0 16px;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: 0 1px 2px rgb(27 29 42 / 6%);
}

.mi-list.is-mobile {
  display: flex;
  flex-direction: column;
  padding: 0 14px;
  box-shadow: none;
}

.mi-list__empty {
  grid-column: 1 / -1;
  padding: 15px 0;
  font-size: var(--pt-fz-body);
  color: var(--pt-t3);
}

.mi-trow {
  display: grid;
  grid-template-columns: subgrid;
  grid-column: 1 / -1;
  align-items: center;
  height: 50px;
  font-size: var(--pt-fz-body);
  border-bottom: 1px solid var(--pt-border);
}

.mi-trow:last-child {
  border-bottom: 0;
}

.mi-trow > span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mi-trow__site {
  font-weight: 700;
  color: var(--pt-t1);
}

.mi-trow__title {
  padding-right: 16px;
  color: var(--pt-t1);
}

.mi-trow__dl {
  padding-right: 15px;
}

.mi-trow__dl :deep(.pt-tag) {
  max-width: 120px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mi-trow__data {
  color: var(--pt-t2);
}

.mi-trow__guard :deep(.pt-pill) {
  gap: 5px;
}

.mi-trow__more {
  justify-self: end;
}

.mi-icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  color: var(--pt-t3);
  cursor: pointer;
  background: none;
  border: 0;
  border-radius: 4px;
}

.mi-icon-btn:hover {
  color: var(--pt-t1);
  background: var(--pt-hover);
}

/* 手机种子行 64 高 */
.mi-mrow {
  display: flex;
  flex-direction: column;
  gap: 8px;
  justify-content: center;
  width: 100%;
  height: 64px;
  padding: 0;
  font: inherit;
  text-align: left;
  cursor: pointer;
  background: none;
  border: 0;
  border-bottom: 1px solid var(--pt-border);
}

.mi-mrow:last-child {
  border-bottom: 0;
}

.mi-mrow__top,
.mi-mrow__bottom {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.mi-mrow__bottom {
  gap: 6px;
}

.mi-mrow__site {
  flex: 0 0 auto;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  color: var(--pt-t1);
}

.mi-mrow__title {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mi-mrow__data {
  min-width: 0;
  margin-left: 2px;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 演职员：每格 108、间距 8，一行放得下几个放几个 */
.mi-cast {
  display: flex;
  flex-wrap: wrap;
  gap: 16px 8px;
  margin-top: 4px;
}

.mi-person {
  display: flex;
  flex: 0 0 108px;
  flex-direction: column;
  align-items: center;
}

.mi-person__face {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 72px;
  height: 72px;
  overflow: hidden;
  color: var(--pt-t3);
  background: var(--pt-hover);
  border-radius: 50%;
}

.mi-person__face img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.mi-person__name,
.mi-person__role {
  max-width: 100%;
  overflow: hidden;
  text-align: center;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mi-person__name {
  margin-top: 8px;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  line-height: 1.4;
  color: var(--pt-t1);
}

.mi-person__role {
  font-size: var(--pt-fz-label);
  line-height: 1.3;
  color: var(--pt-t3);
}

/* 推荐：海报 116×174（2:3，见偏离 ⑤）、间距 12，一行放得下几个放几个 */
.mi-recs {
  display: flex;
  gap: 12px;
}

.mi-rec {
  display: flex;
  flex: 0 0 auto;
  flex-direction: column;
  gap: 6px;
  width: 116px;
  padding: 0;
  font: inherit;
  text-align: left;
  cursor: pointer;
  background: none;
  border: 0;
}

.mi-rec__poster {
  position: relative;
  display: block;
  border-radius: 6px;
}

.mi-rec:hover .mi-rec__poster {
  outline: 1px solid var(--pt-border-strong);
}

.mi-rec__tag {
  position: absolute;
  top: 6px;
  left: 6px;
  display: inline-flex;
  gap: 5px;
  align-items: center;
  height: 18px;
  padding: 0 6px;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  background: #fff;
  border-radius: 4px;
}

.mi-rec__tag i {
  width: 5px;
  height: 5px;
  background: currentcolor;
  border-radius: 50%;
}

.mi-rec__tag.is-ok {
  color: #00773a;
}

.mi-rec__tag.is-info {
  color: #1260d7;
}

.mi-rec__name {
  max-width: 100%;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 500;
  line-height: 1.4;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/*
 * 桌面主区窄（< 1000）：大图右边不放「你的 NAS 上」，文字列伸到大图右边。
 * 只给桌面：手机另有排法，而 :not(:has()) 的特异性比手机那几条高，不圈起来会把手机的简介压成 113 宽、名字压到「更多」底下。
 */
@media (min-width: 769px) {
  .mi-hero:not(:has(.mi-nas--glass)) .mi-hero__text,
  .mi-hero:not(:has(.mi-nas--glass)) .mi-hero__credits {
    right: 28px;
  }

  .mi-hero:not(:has(.mi-nas--glass)) .mi-hero__overview {
    width: min(600px, calc(100% - 224px - 28px));
  }
}

/* 种子列表在窄的主区里给种子那一列让出宽度 */
@media (max-width: 1280px) {
  .mi-list:not(.is-mobile) {
    grid-template-columns: 84px minmax(0, 1fr) auto 76px 180px minmax(0, 170px) 20px;
  }
}

/* ---- 手机（画板 08）：大图 375×348 不留边，块与块 24，行卡 343 ---- */
@media (max-width: 768px) {
  /* 手机外壳的内容列已经左右内缩 16、上面 12：大图用负的外边距贴满屏宽、贴住顶栏 */
  .mi {
    padding: 0;
  }

  .mi-hero {
    height: 348px;
    margin: calc(-1 * var(--pt-space-3)) calc(-1 * var(--pt-space-4)) 0;
    border-radius: 0;
  }

  .mi-hero__shade {
    top: 150px;
    width: 100%;
    height: 198px;
    background: linear-gradient(180deg, rgb(11 18 32 / 0%), rgb(11 18 32 / 70%));
  }

  .mi-hero__poster {
    top: 20px;
    left: 16px;
    box-shadow: 0 8px 24px rgb(0 0 0 / 35%);
  }

  /* 文字列伸到右边 16；只有名字与原名那两行让出右上角的「更多」（32 宽加 8），胶囊在它下面，一行放得下画板的四个 */
  .mi-hero__text {
    top: 20px;
    left: 136px;
    right: 16px;
  }

  .mi-hero__title,
  .mi-hero__original {
    padding-right: 40px;
  }

  .mi-hero__title {
    display: -webkit-box;
    -webkit-line-clamp: 2;
    font-size: var(--pt-fz-title);
    white-space: normal;
    -webkit-box-orient: vertical;
  }

  .mi-hero__original {
    margin-top: 2px;
    font-size: var(--pt-fz-sm);
  }

  .mi-hero__rating {
    height: 20px;
    margin-top: 4px;
  }

  .mi-hero__score {
    font-size: var(--pt-fz-h2);
  }

  .mi-hero__votes {
    font-size: var(--pt-fz-label);
  }

  /* 胶囊两行在大图里 104 起（画板 08：评分那一行下 13） */
  .mi-hero__pills {
    height: auto;
    max-height: 50px;
    margin-top: 13px;
  }

  .mi-hero__overview {
    top: 196px;
    left: 16px;
    width: calc(100% - 32px);
    max-height: calc(3 * 1.6em);
    -webkit-line-clamp: 3;
    cursor: pointer;
  }

  /* 展开：盖在大图上、可以滚动，再点收起；大图的高度不变 */
  .mi-hero__overview.is-open {
    z-index: 3;
    top: 16px;
    bottom: 16px;
    display: block;
    max-height: none;
    padding: 12px 14px;
    overflow-y: auto;
    -webkit-line-clamp: unset;
    background: rgb(11 18 32 / 92%);
    border-radius: var(--pt-r-lg);
  }

  .mi-hero__credits {
    top: 266px;
    left: 16px;
    right: 16px;
  }

  .mi-hero__acts {
    top: 292px;
    left: 16px;
    right: 16px;
  }

  .mi-hero__acts > * {
    flex: 1 1 0;
  }

  .mi-hero__acts .mi-btn {
    width: 100%;
    height: 40px;
  }

  .mi-hero__more {
    position: absolute;
    top: 20px;
    right: 16px;
  }

  /* 「你的 NAS 上」紧跟大图（下 16，画板 08）；三行时 180 高：最后一行下面留 12 + 6 */
  .mi-nas--card {
    margin-top: -8px;
    padding-bottom: 6px;
  }

  /* 标题、横滑 tab、图例各占一行：tab 在标题下 32、图例在 74、收起行在 100（画板 08） */
  .mi-seasons__head {
    flex-wrap: wrap;
    gap: 0;
    margin-top: 0;
    margin-bottom: 12px;
  }

  .mi-seasons__head .mi-block__title {
    flex: 1 0 100%;
  }

  .mi-tabs {
    flex: 1 0 100%;
    margin-top: 11px;
    padding-bottom: 2px;
    overflow-x: auto;
    scrollbar-width: none;
  }

  .mi-tab {
    height: 30px;
    border-radius: 15px;
  }

  .mi-legend {
    margin-top: 10px;
    margin-left: 0;
  }

  .mi-eps {
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
  }

  .mi-ep {
    height: 80px;
  }

  .mi-ep__still {
    top: 7px;
    left: 7px;
    width: 112px;
    height: 63px;
  }

  .mi-ep__play {
    width: 28px;
    height: 28px;
  }

  .mi-ep__title {
    top: 8px;
    left: 131px;
    right: 60px;
  }

  .mi-ep__air {
    top: 28px;
    left: 131px;
  }

  .mi-ep__state {
    top: 51px;
    left: 131px;
    gap: 6px;
  }

  .mi-ep__bar {
    top: 73px;
    left: 131px;
    width: calc(100% - 131px - 14px);
  }

  .mi-ep__act {
    top: 9px;
    right: 13px;
  }

  .mi-cast,
  .mi-recs {
    margin: 0 calc(-1 * var(--pt-space-4));
    padding: 0 var(--pt-space-4);
    overflow-x: auto;
    scrollbar-width: none;
  }

  .mi-cast {
    flex-wrap: nowrap;
    height: 104px;
  }

  .mi-person {
    flex-basis: 76px;
  }

  .mi-person__face {
    width: 64px;
    height: 64px;
  }

  .mi-person__name {
    margin-top: 6px;
  }

  .mi-rec {
    width: 104px;
  }
}
</style>

<style>
/* 手机种子行的操作抽屉 */
.mi-sheet.el-drawer {
  border-radius: 12px 12px 0 0;
}

.mi-sheet__body {
  display: flex;
  flex-direction: column;
  padding-bottom: env(safe-area-inset-bottom);
}

.mi-sheet__title {
  padding: 4px 0 12px;
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mi-sheet__item {
  display: flex;
  gap: 10px;
  align-items: center;
  min-height: var(--pt-m-touch);
  padding: 0;
  font: inherit;
  font-size: var(--pt-fz-body);
  color: var(--pt-t1);
  text-align: left;
  cursor: pointer;
  background: none;
  border: 0;
  border-top: 1px solid var(--pt-border);
}
</style>
