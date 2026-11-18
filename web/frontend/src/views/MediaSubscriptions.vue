<script setup lang="ts">
/*
 * 追剧与订阅（路线图 M11，追剧见 state 里的方案）：订阅列表（进度、每一集的格子、定时与版本锁、最近一次搜索的结果、
 * 立即搜索、暂停与恢复、确认、修改、删除），订阅设置、质量档案、豆瓣想看来源与拉黑的种子。画板没有这一页，沿用媒体页的样式：
 * 页头 + 面板里的表格，手机是行卡。
 */
import {
  type DoubanItem,
  type DoubanSource,
  type DownloaderSetting,
  type NotificationConfig,
  type QualityProfile,
  type SubscribeSettings,
  type Subscription,
  type TorrentBlock,
  chatopsApi,
  downloadersApi,
  sitesApi,
  subscribeApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import TmdbKeyNotice from "@/components/media/TmdbKeyNotice.vue";
import DoubanSourceDialog from "@/components/media/DoubanSourceDialog.vue";
import QualityProfileDialog from "@/components/media/QualityProfileDialog.vue";
import SubscribeDialog from "@/components/media/SubscribeDialog.vue";
import SubscriptionDetailDialog from "@/components/media/SubscriptionDetailDialog.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { useTmdbImageBase } from "@/composables/useTmdbImageBase";
import { formatShortDateTime } from "@/utils/format";
import { posterURL } from "@/utils/media";
import { itemLink } from "@/utils/mediaItem";
import {
  SUB_FILTERS,
  doubanItemState,
  episodeState,
  isFollow,
  lockText,
  profileSummary,
  progressText,
  scheduleText,
  sourceLabel,
  subName,
  subStatus,
} from "@/utils/subscribe";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";

const isMobile = useIsMobile();
// 「媒体识别」里换了 TMDB 图片地址时，海报从换的地址加载
useTmdbImageBase();
const router = useRouter();
const route = useRoute();
const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;

// ---- 订阅列表 ----
/** 首页的「等版本 N」带着 ?state=waiting_version 进来：只看在找的、有集在等锁住的版本的订阅 */
const waitingOnly = ref(route.query.state === "waiting_version");
const status = ref(waitingOnly.value ? "active" : "");
const keyword = ref("");
const loadedSubs = ref<Subscription[]>([]);
const subs = computed(() =>
  waitingOnly.value
    ? loadedSubs.value.filter((s) => (s.progress?.waiting ?? 0) > 0)
    : loadedSubs.value,
);
const ds = useDataState({
  filtered: () => status.value !== "" || keyword.value.trim() !== "" || waitingOnly.value,
});
const failedPosters = ref(new Set<number>());

async function loadSubs() {
  const pending = ds.run(() =>
    subscribeApi.list({ status: status.value, q: keyword.value.trim() }),
  );
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  loadedSubs.value = got;
}

const searching = ref<number | null>(null);
async function searchNow(s: Subscription) {
  searching.value = s.id;
  try {
    const res = await subscribeApi.search(s.id);
    if (res.message.startsWith("下载了")) ElMessage.success(res.message);
    else ElMessage.info(res.message);
    await loadSubs();
  } catch (e) {
    ElMessage.error(errText(e, "搜索失败"));
  } finally {
    searching.value = null;
  }
}

async function setStatus(s: Subscription, next: "active" | "paused") {
  try {
    await subscribeApi.setStatus(s.id, next);
    ElMessage.success(
      next === "paused" ? "已暂停" : s.status === "pending" ? "已确认，开始找资源" : "已恢复",
    );
    await loadSubs();
  } catch (e) {
    ElMessage.error(errText(e, "操作失败"));
  }
}

async function removeSub(s: Subscription) {
  try {
    await ElMessageBox.confirm(
      `删除订阅「${subName(s)}」？下载器里的种子与库里的文件不动。`,
      "删除订阅",
      {
        type: "warning",
        confirmButtonText: "删除",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }
  try {
    await subscribeApi.remove(s.id);
    ElMessage.success("已删除订阅");
    await loadSubs();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}

const editDialog = ref(false);
const editing = ref<Subscription | null>(null);
function edit(s: Subscription) {
  editing.value = s;
  editDialog.value = true;
}

const detailDialog = ref(false);
const detailID = ref<number | null>(null);
function showDetail(s: Subscription) {
  detailID.value = s.id;
  detailDialog.value = true;
}

// 每一集一个小格子：季太长时不画（行会太高），详情里有完整的
const MAX_STRIP = 60;
function strip(s: Subscription): string[] {
  const states = s.media_type === "tv" ? (s.progress?.states ?? []) : [];
  return states.length <= MAX_STRIP ? states : [];
}

/** 追剧的定时与版本锁写成一句话（电影与升级以前的订阅为空）。 */
function followText(s: Subscription): string {
  if (s.media_type !== "tv") return "";
  return [isFollow(s) ? scheduleText(s.schedule) : "", lockText(s)].filter(Boolean).join(" · ");
}

// ---- 设置 ----
const settings = ref<SubscribeSettings | null>(null);
const sform = ref<SubscribeSettings>({
  enabled: false,
  search_interval_hours: 12,
  search_skip_sites: [],
  default_profile_id: 0,
  default_downloader_id: 0,
  notify_channels: [],
  upgrade_old: "keep",
  today_enabled: false,
  today_time: "09:00",
  today_channels: [],
  subdir_enabled: false,
  subdir_template: "{title} ({year})",
  ai_review_enabled: false,
  ai_review_scope: "name",
  unlock_after_days: 0,
});
const settingsFailed = ref(false);
const savingSettings = ref(false);
const downloaders = ref<DownloaderSetting[]>([]);
const channels = ref<NotificationConfig[]>([]);
const siteNames = ref<string[]>([]);

async function loadSettings() {
  try {
    const s = await subscribeApi.settings();
    if (s) {
      settings.value = s;
      sform.value = {
        ...s,
        search_skip_sites: [...s.search_skip_sites],
        notify_channels: [...s.notify_channels],
        today_channels: [...(s.today_channels ?? [])],
      };
    }
    settingsFailed.value = false;
  } catch {
    settingsFailed.value = true;
  }
}

async function loadOptions() {
  const [dls, chs, ss] = await Promise.allSettled([
    downloadersApi.list(),
    chatopsApi.notifications.list(),
    sitesApi.list(),
  ]);
  if (dls.status === "fulfilled") downloaders.value = dls.value ?? [];
  if (chs.status === "fulfilled") channels.value = chs.value ?? [];
  if (ss.status === "fulfilled") {
    siteNames.value = Object.entries(ss.value ?? {})
      .filter(([, c]) => c.enabled)
      .map(([n]) => n)
      .sort();
  }
}

async function saveSettings() {
  if (!settings.value) return;
  savingSettings.value = true;
  try {
    const s = await subscribeApi.saveSettings(sform.value);
    settings.value = s;
    ElMessage.success("已保存订阅设置");
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    savingSettings.value = false;
  }
}

// ---- 质量档案 ----
const profiles = ref<QualityProfile[]>([]);
const options = ref({
  resolutions: [] as string[],
  sources: [] as string[],
  codecs: [] as string[],
});
const profilesDS = useDataState();
const profileDialog = ref(false);
const editingProfile = ref<QualityProfile | null>(null);

async function loadProfiles() {
  const pending = profilesDS.run(() => subscribeApi.profiles());
  const got = await pending;
  if (profilesDS.isStale(pending) || !got) return;
  profiles.value = got.items;
  options.value = { resolutions: got.resolutions, sources: got.sources, codecs: got.codecs };
}

function openProfile(p: QualityProfile | null = null) {
  editingProfile.value = p;
  profileDialog.value = true;
}

async function removeProfile(p: QualityProfile) {
  try {
    await ElMessageBox.confirm(`删除质量档案「${p.name}」？`, "删除质量档案", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  try {
    await subscribeApi.deleteProfile(p.id);
    ElMessage.success("已删除质量档案");
    await loadProfiles();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}

const profileName = (id: number) =>
  id === 0 ? "默认" : (profiles.value.find((p) => p.id === id)?.name ?? `档案 ${id}`);

// ---- 豆瓣想看 ----
const sources = ref<DoubanSource[]>([]);
const sourcesDS = useDataState();
const doubanDialog = ref(false);
const editingSource = ref<DoubanSource | null>(null);
const fetching = ref<number | null>(null);
const itemsDialog = ref(false);
const items = ref<DoubanItem[]>([]);
const itemsOf = ref<DoubanSource | null>(null);

async function loadSources() {
  const pending = sourcesDS.run(() => subscribeApi.doubanSources());
  const got = await pending;
  if (sourcesDS.isStale(pending) || !got) return;
  sources.value = got;
}

function openSource(s: DoubanSource | null = null) {
  editingSource.value = s;
  doubanDialog.value = true;
}

async function fetchSource(s: DoubanSource) {
  fetching.value = s.id;
  try {
    const res = await subscribeApi.fetchDouban(s.id);
    ElMessage.success(res.created ? `建了 ${res.created} 个订阅` : "没有新的想看");
    await Promise.all([loadSources(), loadSubs()]);
  } catch (e) {
    ElMessage.error(errText(e, "拉取失败"));
    await loadSources();
  } finally {
    fetching.value = null;
  }
}

async function showItems(s: DoubanSource) {
  itemsOf.value = s;
  items.value = [];
  itemsDialog.value = true;
  try {
    items.value = (await subscribeApi.doubanItems(s.id)) ?? [];
  } catch (e) {
    ElMessage.error(errText(e, "读取失败"));
  }
}

async function removeSource(s: DoubanSource) {
  try {
    await ElMessageBox.confirm(
      `删除豆瓣来源「${s.name || s.user_id}」？已经建的订阅留着。`,
      "删除豆瓣来源",
      {
        type: "warning",
        confirmButtonText: "删除",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }
  try {
    await subscribeApi.deleteDouban(s.id);
    ElMessage.success("已删除豆瓣来源");
    await loadSources();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}

// ---- 拉黑的种子 ----
const blocks = ref<TorrentBlock[]>([]);
const blocksDS = useDataState();
const unblocking = ref<number | null>(null);

async function loadBlocks() {
  const pending = blocksDS.run(() => subscribeApi.blocks());
  const got = await pending;
  if (blocksDS.isStale(pending) || !got) return;
  blocks.value = got.items ?? [];
}

/** 作用于哪个条目：对所有条目的（关键字搜索里拉黑的）写「所有条目」 */
function blockScope(b: TorrentBlock) {
  if (!b.tmdb_id) return "所有条目";
  return b.media_title || `${b.media_type === "tv" ? "剧集" : "电影"} ${b.tmdb_id}`;
}

async function unblock(b: TorrentBlock) {
  unblocking.value = b.id;
  try {
    await subscribeApi.removeBlock(b.id);
    ElMessage.success("已移出黑名单");
    await loadBlocks();
  } catch (e) {
    ElMessage.error(errText(e, "移出黑名单失败"));
  } finally {
    unblocking.value = null;
  }
}

/** 订阅详情里改了东西（跳过、换版本、拉黑……）：列表与拉黑的种子都重读 */
function onDetailChanged() {
  void loadSubs();
  void loadBlocks();
}

function refresh() {
  void loadSubs();
  void loadSettings();
  void loadProfiles();
  void loadSources();
  void loadBlocks();
}

function openExplore() {
  void router.push("/media/explore");
}

// 进度、下次搜索与结果说明在后台每分钟更新（整理入库以后最多一分钟台账就是新的）：页面开着时每分钟静默刷新一次列表，
// 切到别的标签页、开着修改或详情弹窗、上一次还没回来时这一拍不刷
const AUTO_REFRESH_MS = 60_000;
let autoTimer: ReturnType<typeof setInterval> | undefined;

onMounted(() => {
  refresh();
  void loadOptions();
  autoTimer = setInterval(() => {
    if (document.hidden || editDialog.value || detailDialog.value || ds.loading.value) return;
    void loadSubs();
  }, AUTO_REFRESH_MS);
});

onBeforeUnmount(() => clearInterval(autoTimer));
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub
      >追剧集、订阅电影：按播出时间或定时到各站点搜索，RSS
      里出现时也会下载；剧集按集补齐、只要同一个版本，洗版时换成更好的版本</PtHeadSub
    >
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="ms-explore" @click="openExplore">
        <PtIcon name="compass" :size="15" /><span>去探索</span>
      </el-button>
      <el-button data-testid="ms-refresh" @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>
    <TmdbKeyNotice class="pt-cards__full" usage="订阅的搜索、补集与洗版" />

    <PtPanel title="订阅" icon="bookmark" :count="subs.length">
      <div
        v-if="settings && !settings.enabled"
        class="pt-note pt-note--warn"
        data-testid="ms-disabled">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>订阅的总开关关着：不会搜索、不接 RSS、不拉豆瓣。在下面的「订阅设置」里打开。</span>
      </div>
      <div class="ms-bar">
        <el-radio-group v-model="status" size="small" data-testid="ms-status" @change="loadSubs">
          <el-radio-button v-for="f in SUB_FILTERS" :key="f.value || 'all'" :value="f.value">{{
            f.label
          }}</el-radio-button>
        </el-radio-group>
        <el-check-tag
          :checked="waitingOnly"
          data-testid="ms-waiting"
          @change="waitingOnly = !waitingOnly"
          >等版本</el-check-tag
        >
        <el-input
          v-model="keyword"
          class="ms-search"
          size="small"
          clearable
          placeholder="名字"
          data-testid="ms-keyword"
          @keyup.enter="loadSubs"
          @clear="loadSubs">
          <template #prefix><PtIcon name="search" :size="14" /></template>
        </el-input>
      </div>
      <div
        v-if="subs.length && ds.error.value"
        class="pt-note pt-note--warn"
        data-testid="ms-stale">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>刷新失败：{{ ds.errorText.value }}。下面是上次读到的。</span>
      </div>
      <PtDataState
        v-if="!subs.length"
        :state="ds.state.value"
        :title="
          ds.state.value === 'empty'
            ? '还没有订阅'
            : ds.state.value === 'zero'
              ? '没有对得上的订阅'
              : ''
        "
        :sub="
          ds.errorText.value ||
          (ds.state.value === 'empty' ? '去「探索」页挑想看的，或者在下面加豆瓣想看来源' : '')
        "
        data-testid="ms-state" />
      <el-table
        v-else-if="!isMobile"
        :data="subs"
        row-key="id"
        class="pt-grid"
        data-testid="ms-table">
        <el-table-column label="条目" min-width="240">
          <template #default="{ row }">
            <div class="ms-entry">
              <img
                v-if="posterURL(row.poster_path, 'w92') && !failedPosters.has(row.id)"
                class="ms-poster"
                :src="posterURL(row.poster_path, 'w92')"
                alt=""
                @error="failedPosters.add(row.id)" />
              <div class="ms-entry__text">
                <router-link
                  class="ms-strong ms-link"
                  :to="itemLink(row.media_type, row.tmdb_id, 'subscriptions', row.season)"
                  :data-testid="`ms-item-${row.id}`"
                  >{{ subName(row) }}</router-link
                >
                <div class="ms-sub">
                  {{ sourceLabel(row.source) }} · {{ profileName(row.profile_id)
                  }}<template v-if="row.upgrade"> · 洗版</template
                  ><template v-if="row.sites?.length"> · {{ row.sites.join("、") }}</template>
                </div>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="96">
          <template #default="{ row }">
            <PtStatusPill :tone="subStatus(row.status).tone" size="sm">{{
              subStatus(row.status).label
            }}</PtStatusPill>
          </template>
        </el-table-column>
        <el-table-column label="进度" min-width="200">
          <template #default="{ row }">
            <div>{{ progressText(row.media_type, row.progress) }}</div>
            <div
              v-if="strip(row).length"
              class="ms-strip"
              :data-testid="`ms-strip-${row.id}`"
              aria-hidden="true">
              <i
                v-for="(st, i) in strip(row)"
                :key="i"
                class="ms-strip__cell"
                :class="`ms-strip__cell--${st}`"
                :title="`第 ${i + 1} 集 · ${episodeState(st).label}`" />
            </div>
          </template>
        </el-table-column>
        <!--
          「最近一次」以前整格单行截断：下载到的种子名只剩前半截（实测可见 246 / 实际 487），
          下次搜索、追更这几行也跟着挤成一行。现在结果最多两行，完整内容在悬停提示里。
        -->
        <el-table-column label="最近一次" min-width="280">
          <template #default="{ row }">
            <el-tooltip
              :content="row.message"
              :disabled="!row.message"
              placement="top"
              :show-after="300">
              <div class="ms-msg">{{ row.message || "还没搜过" }}</div>
            </el-tooltip>
            <div v-if="row.status === 'active' && row.next_search_at" class="ms-sub">
              下次搜索 {{ formatShortDateTime(row.next_search_at) }}
            </div>
            <div v-if="followText(row)" class="ms-sub" :data-testid="`ms-follow-${row.id}`">
              {{ followText(row) }}
            </div>
          </template>
        </el-table-column>
        <!--
          操作列以前是 300 宽的无标题列，五个文字按钮并排（立即搜索 / 暂停 / 详情 / 修改 / 删除），
          把「最近一次」挤窄。常用的留在外面，修改、删除收进「更多」。
        -->
        <el-table-column label="操作" width="232" align="right" class-name="ms-acts">
          <template #default="{ row }">
            <el-button
              v-if="row.status === 'active'"
              link
              :loading="searching === row.id"
              :data-testid="`ms-search-${row.id}`"
              @click="searchNow(row)"
              >立即搜索</el-button
            >
            <el-button
              v-if="row.status === 'pending'"
              link
              type="primary"
              :data-testid="`ms-confirm-${row.id}`"
              @click="setStatus(row, 'active')"
              >确认</el-button
            >
            <el-button
              v-else-if="row.status === 'paused'"
              link
              :data-testid="`ms-resume-${row.id}`"
              @click="setStatus(row, 'active')"
              >恢复</el-button
            >
            <el-button
              v-else-if="row.status === 'active'"
              link
              :data-testid="`ms-pause-${row.id}`"
              @click="setStatus(row, 'paused')"
              >暂停</el-button
            >
            <el-button link :data-testid="`ms-detail-${row.id}`" @click="showDetail(row)"
              >详情</el-button
            >
            <el-dropdown
              class="ms-more"
              trigger="click"
              @command="(c: unknown) => (c === 'edit' ? edit(row) : removeSub(row))">
              <el-button link :aria-label="`${subName(row)} 的更多操作`">
                <PtIcon name="ellipsis" :size="16" />
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="edit" :data-testid="`ms-edit-${row.id}`">
                    <PtIcon name="pencil" :size="14" class="ms-dd-ico" /><span>修改</span>
                  </el-dropdown-item>
                  <el-dropdown-item
                    command="del"
                    divided
                    class="ms-dd-danger"
                    :data-testid="`ms-del-${row.id}`">
                    <PtIcon name="trash-2" :size="14" class="ms-dd-ico" /><span>删除</span>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="ms-cards">
        <PtRowCard v-for="s in subs" :key="s.id">
          <template #title>
            <router-link
              class="ms-link"
              :to="itemLink(s.media_type, s.tmdb_id, 'subscriptions', s.season)"
              >{{ subName(s) }}</router-link
            >
          </template>
          <template #meta>
            <span>{{ progressText(s.media_type, s.progress) }}</span>
            <span v-if="strip(s).length" class="ms-strip ms-full" aria-hidden="true">
              <i
                v-for="(st, i) in strip(s)"
                :key="i"
                class="ms-strip__cell"
                :class="`ms-strip__cell--${st}`" />
            </span>
            <span
              >{{ sourceLabel(s.source) }} · {{ profileName(s.profile_id)
              }}<template v-if="s.upgrade"> · 洗版</template></span
            >
            <span v-if="followText(s)" class="ms-full">{{ followText(s) }}</span>
            <span v-if="s.status === 'active' && s.next_search_at"
              >下次搜索 {{ formatShortDateTime(s.next_search_at) }}</span
            >
            <span class="ms-full">{{ s.message || "还没搜过" }}</span>
          </template>
          <template #status>
            <PtStatusPill dot :tone="subStatus(s.status).tone" size="sm">{{
              subStatus(s.status).label
            }}</PtStatusPill>
          </template>
          <template #actions>
            <el-button
              v-if="s.status === 'active'"
              size="small"
              :loading="searching === s.id"
              @click="searchNow(s)"
              >搜索</el-button
            >
            <el-button
              v-if="s.status === 'pending'"
              size="small"
              type="primary"
              plain
              @click="setStatus(s, 'active')"
              >确认</el-button
            >
            <el-button
              v-else-if="s.status === 'paused'"
              size="small"
              @click="setStatus(s, 'active')"
              >恢复</el-button
            >
            <el-button
              v-else-if="s.status === 'active'"
              size="small"
              @click="setStatus(s, 'paused')"
              >暂停</el-button
            >
            <el-button size="small" @click="showDetail(s)">详情</el-button>
            <el-button size="small" @click="edit(s)">修改</el-button>
            <el-button size="small" type="danger" plain @click="removeSub(s)">删除</el-button>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <div class="pt-cols">
      <div class="pt-col">
        <PtPanel title="订阅设置" icon="sliders-horizontal">
          <div v-if="settingsFailed" class="pt-note pt-note--warn">
            <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
            <span v-if="settings">设置刷新失败，下面是上次读到的。</span>
            <span v-else>设置没读到，读到以前不能修改，点刷新重试。</span>
          </div>
          <!-- 读到设置以前表单是默认值：不能保存，避免盖掉真正的设置 -->
          <el-form
            class="pt-form ms-form"
            label-position="top"
            :disabled="!settings"
            @submit.prevent>
            <el-form-item label="订阅">
              <div class="ms-switch">
                <el-switch v-model="sform.enabled" data-testid="ms-enabled" />
                <span>打开以后定时搜索、对 RSS 里的种子、拉豆瓣想看</span>
              </div>
            </el-form-item>
            <div class="ms-row">
              <el-form-item label="主动搜索的间隔（小时）">
                <el-input-number
                  v-model="sform.search_interval_hours"
                  :min="6"
                  :max="168"
                  controls-position="right"
                  data-testid="ms-interval" />
                <div class="field-tip">
                  每个订阅按这个间隔搜一次，另加最多 30 分钟的随机偏移；最少 6
                  小时，避免给站点添负担
                </div>
              </el-form-item>
              <el-form-item label="不参与主动搜索的站点">
                <el-select
                  v-model="sform.search_skip_sites"
                  multiple
                  filterable
                  clearable
                  placeholder="都参与"
                  data-testid="ms-skip-sites">
                  <el-option v-for="s in siteNames" :key="s" :value="s" :label="s" />
                </el-select>
                <div class="field-tip">这些站点只看 RSS 里的种子</div>
              </el-form-item>
            </div>
            <div class="ms-row">
              <el-form-item label="默认质量档案">
                <el-select v-model="sform.default_profile_id" data-testid="ms-default-profile">
                  <el-option :value="0" label="不限（按分辨率与来源从好到差挑）" />
                  <el-option v-for="p in profiles" :key="p.id" :value="p.id" :label="p.name" />
                </el-select>
              </el-form-item>
              <el-form-item label="默认下载器">
                <el-select
                  v-model="sform.default_downloader_id"
                  data-testid="ms-default-downloader">
                  <el-option :value="0" label="pt-tools 的默认下载器" />
                  <el-option
                    v-for="d in downloaders"
                    :key="d.id"
                    :value="d.id as number"
                    :label="d.name" />
                </el-select>
              </el-form-item>
            </div>
            <div class="ms-row">
              <el-form-item label="通知">
                <el-select
                  v-model="sform.notify_channels"
                  multiple
                  clearable
                  placeholder="不通知"
                  data-testid="ms-notify">
                  <el-option v-for="c in channels" :key="c.id" :label="c.name" :value="c.id" />
                </el-select>
                <div class="field-tip">下载了订阅的资源、豆瓣想看连续拉取失败时通知</div>
              </el-form-item>
              <el-form-item label="洗版以后的旧种子">
                <el-select v-model="sform.upgrade_old" data-testid="ms-upgrade-old">
                  <el-option value="keep" label="继续做种" />
                  <el-option value="delete" label="连数据删掉（有 H&R 要求的留着）" />
                </el-select>
              </el-form-item>
            </div>
            <!-- 追剧的版本锁久等不到锁住的版本时：等满几天改用别的版本（0 是一直等） -->
            <el-form-item label="版本锁等满几天改用别的版本">
              <el-input-number
                v-model="sform.unlock_after_days"
                :min="0"
                :max="90"
                :step="1"
                step-strictly
                controls-position="right"
                data-testid="ms-unlock-after-days" />
              <div class="field-tip">
                追剧开着版本锁时，一集等锁住的版本满这么多天（开始等以后至少搜过一次），就像点了「这一集换别的版本」一样改用别的版本，并马上搜一次；0
                是一直等
              </div>
            </el-form-item>
            <!-- 媒体中心 M3：每天推送今天播出的集（同订阅日历的今天） -->
            <el-form-item label="今日更新">
              <div class="ms-switch">
                <el-switch v-model="sform.today_enabled" data-testid="ms-today" />
                <span>每天推送今天播出的集与上映的电影，没有时不推</span>
              </div>
            </el-form-item>
            <div v-if="sform.today_enabled" class="ms-row">
              <el-form-item label="推送时刻">
                <el-time-select
                  v-model="sform.today_time"
                  start="00:00"
                  step="00:15"
                  end="23:45"
                  :clearable="false"
                  data-testid="ms-today-time" />
                <div class="field-tip">服务器时区；订阅的总开关关着时不推</div>
              </el-form-item>
              <el-form-item label="推送到">
                <el-select
                  v-model="sform.today_channels"
                  multiple
                  clearable
                  placeholder="同上面的通知"
                  data-testid="ms-today-channels">
                  <el-option v-for="c in channels" :key="c.id" :label="c.name" :value="c.id" />
                </el-select>
                <div class="field-tip">不选时推送到上面「通知」选的通道</div>
              </el-form-item>
            </div>
            <!-- 媒体中心 M9b：订阅推送的种子放进按条目起名的子目录 -->
            <el-form-item label="下载时按条目建子目录">
              <div class="ms-switch">
                <el-switch v-model="sform.subdir_enabled" data-testid="ms-subdir" />
                <span>订阅推送的种子放进保存路径下的一级子目录，例如「最后生还者 (2023)」</span>
              </div>
              <div class="field-tip">
                订阅没填保存路径时建在下载器的默认保存路径下，下载器照这个目录放（qBittorrent
                这几个种子不用自动管理）。自动整理限了「保存路径」时，范围要包含这些子目录
              </div>
            </el-form-item>
            <el-form-item v-if="sform.subdir_enabled" label="子目录名">
              <el-input
                v-model="sform.subdir_template"
                maxlength="255"
                placeholder="{title} ({year})"
                data-testid="ms-subdir-template" />
              <div class="field-tip">
                可以用
                {title}（标题）、{original_title}（原名）、{year}（年份，剧集是首播年份）、{season}（季）、{tmdb_id}；名字里不能用的字符会换掉，空着是
                {title} ({year})
              </div>
            </el-form-item>
            <!-- 媒体中心 M9b：推送前让 AI 助手的模型复核候选是不是要的条目 -->
            <el-form-item label="下载前让 AI 复核">
              <div class="ms-switch">
                <el-switch v-model="sform.ai_review_enabled" data-testid="ms-ai-review" />
                <span
                  >推送订阅找到的种子以前，问 AI
                  助手的模型是不是这部片（剧集还要是这一季），说不是的不推</span
                >
              </div>
              <div class="field-tip">
                要先在
                <router-link to="/assistant/settings" data-testid="ms-ai-review-link"
                  >AI 助手设置</router-link
                >
                里配好并启用。每个种子要等模型回答，会用掉 token；每次搜索最多问 10 个，同一个种子
                24 小时内不重复问。模型出错时照常推送，写进运行记录
              </div>
            </el-form-item>
            <el-form-item v-if="sform.ai_review_enabled" label="复核哪些">
              <el-select v-model="sform.ai_review_scope" data-testid="ms-ai-review-scope">
                <el-option value="name" label="只复核按名字对上的（按 IMDb 编号对上的不问）" />
                <el-option value="all" label="全部" />
              </el-select>
            </el-form-item>
            <el-button
              type="primary"
              :loading="savingSettings"
              :disabled="!settings"
              data-testid="ms-save-settings"
              @click="saveSettings">
              <PtIcon v-if="!savingSettings" name="save" :size="14" /><span>保存</span>
            </el-button>
          </el-form>
        </PtPanel>
      </div>

      <div class="pt-col">
        <PtPanel
          title="质量档案"
          icon="layers"
          :count="profiles.length"
          action="添加"
          action-icon="plus"
          @action="openProfile()">
          <PtDataState
            v-if="!profiles.length"
            :state="profilesDS.state.value"
            :title="profilesDS.state.value === 'empty' ? '还没有质量档案' : ''"
            :sub="
              profilesDS.errorText.value ||
              (profilesDS.state.value === 'empty' ? '不建时按分辨率与来源从好到差挑' : '')
            "
            data-testid="ms-profiles-state" />
          <el-table
            v-else-if="!isMobile"
            :data="profiles"
            row-key="id"
            class="pt-grid"
            data-testid="ms-profiles">
            <el-table-column label="名称" min-width="120" prop="name" class-name="pt-cell-strong" />
            <el-table-column label="要求" min-width="320">
              <template #default="{ row }">{{ profileSummary(row) }}</template>
            </el-table-column>
            <el-table-column label="" width="120" align="right">
              <template #default="{ row }">
                <el-button link :data-testid="`ms-profile-edit-${row.id}`" @click="openProfile(row)"
                  >修改</el-button
                >
                <el-button
                  link
                  type="danger"
                  :data-testid="`ms-profile-del-${row.id}`"
                  @click="removeProfile(row)"
                  >删除</el-button
                >
              </template>
            </el-table-column>
          </el-table>
          <div v-else class="ms-cards">
            <PtRowCard v-for="p in profiles" :key="p.id">
              <template #title>{{ p.name }}</template>
              <template #meta
                ><span class="ms-full">{{ profileSummary(p) }}</span></template
              >
              <template #actions>
                <el-button size="small" @click="openProfile(p)">修改</el-button>
                <el-button size="small" type="danger" plain @click="removeProfile(p)"
                  >删除</el-button
                >
              </template>
            </PtRowCard>
          </div>
        </PtPanel>

        <PtPanel
          title="豆瓣想看"
          icon="heart"
          :count="sources.length"
          action="添加"
          action-icon="plus"
          @action="openSource()">
          <p class="ms-tip">
            每 6 小时拉一次豆瓣用户公开的「收藏」RSS，给「想看」的电影与剧集建订阅；条目从 RSS
            里消失时不删已经建的订阅。
          </p>
          <PtDataState
            v-if="!sources.length"
            :state="sourcesDS.state.value"
            :title="sourcesDS.state.value === 'empty' ? '还没有豆瓣来源' : ''"
            :sub="sourcesDS.errorText.value"
            data-testid="ms-douban-state" />
          <el-table
            v-else-if="!isMobile"
            :data="sources"
            row-key="id"
            class="pt-grid"
            data-testid="ms-douban">
            <el-table-column label="豆瓣用户" min-width="140">
              <template #default="{ row }">
                <div class="ms-strong">{{ row.name || row.user_id }}</div>
                <div v-if="row.name" class="ms-sub">{{ row.user_id }}</div>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="110">
              <template #default="{ row }">
                <PtStatusPill v-if="!row.enabled" tone="neutral" size="sm">停用</PtStatusPill>
                <PtStatusPill v-else-if="row.abnormal" tone="dang" size="sm">异常</PtStatusPill>
                <PtStatusPill v-else tone="ok" size="sm">正常</PtStatusPill>
              </template>
            </el-table-column>
            <el-table-column label="建过的订阅" min-width="150">
              <template #default="{ row }">
                {{ row.subscribed || 0 }} 个<template v-if="row.unmatched">
                  · {{ row.unmatched }} 个没找到条目</template
                >
              </template>
            </el-table-column>
            <el-table-column
              label="最近一次"
              min-width="220"
              class-name="pt-cell-1line"
              show-overflow-tooltip>
              <template #default="{ row }">
                <div v-if="row.last_error" class="ms-error">{{ row.last_error }}</div>
                <div v-else>
                  {{ row.last_fetch_at ? formatShortDateTime(row.last_fetch_at) : "还没拉过" }}
                </div>
                <div v-if="row.enabled && row.next_fetch_at" class="ms-sub">
                  下次 {{ formatShortDateTime(row.next_fetch_at) }}
                </div>
              </template>
            </el-table-column>
            <el-table-column label="" width="260" align="right">
              <template #default="{ row }">
                <el-button
                  link
                  :loading="fetching === row.id"
                  :data-testid="`ms-douban-fetch-${row.id}`"
                  @click="fetchSource(row)"
                  >立即拉取</el-button
                >
                <el-button link :data-testid="`ms-douban-items-${row.id}`" @click="showItems(row)"
                  >条目</el-button
                >
                <el-button link :data-testid="`ms-douban-edit-${row.id}`" @click="openSource(row)"
                  >修改</el-button
                >
                <el-button
                  link
                  type="danger"
                  :data-testid="`ms-douban-del-${row.id}`"
                  @click="removeSource(row)"
                  >删除</el-button
                >
              </template>
            </el-table-column>
          </el-table>
          <div v-else class="ms-cards">
            <PtRowCard v-for="s in sources" :key="s.id">
              <template #title>{{ s.name || s.user_id }}</template>
              <template #meta>
                <span
                  >{{ s.subscribed || 0 }} 个订阅<template v-if="s.unmatched">
                    · {{ s.unmatched }} 个没找到</template
                  ></span
                >
                <span v-if="s.last_error" class="ms-full ms-error">{{ s.last_error }}</span>
              </template>
              <template #status>
                <PtStatusPill v-if="!s.enabled" dot tone="neutral" size="sm">停用</PtStatusPill>
                <PtStatusPill v-else-if="s.abnormal" dot tone="dang" size="sm">异常</PtStatusPill>
                <PtStatusPill v-else dot tone="ok" size="sm">正常</PtStatusPill>
              </template>
              <template #actions>
                <el-button size="small" :loading="fetching === s.id" @click="fetchSource(s)"
                  >拉取</el-button
                >
                <el-button size="small" @click="showItems(s)">条目</el-button>
                <el-button size="small" @click="openSource(s)">修改</el-button>
                <el-button size="small" type="danger" plain @click="removeSource(s)"
                  >删除</el-button
                >
              </template>
            </PtRowCard>
          </div>
        </PtPanel>

        <PtPanel title="拉黑的种子" icon="eye-off" :count="blocks.length">
          <p class="ms-tip">
            在按条目搜索、关键字搜索或订阅详情里点了「不要」的种子：订阅不再选它们。移出黑名单以后订阅又会选。
          </p>
          <PtDataState
            v-if="!blocks.length"
            :state="blocksDS.state.value"
            :title="blocksDS.state.value === 'empty' ? '没有拉黑的种子' : ''"
            :sub="blocksDS.errorText.value"
            data-testid="ms-blocks-state" />
          <el-table
            v-else-if="!isMobile"
            :data="blocks"
            row-key="id"
            class="pt-grid"
            data-testid="ms-blocks">
            <el-table-column label="种子" min-width="260" class-name="pt-cell-1line">
              <template #default="{ row }">
                <div class="ms-strong" :title="row.title">{{ row.title || row.torrent_id }}</div>
                <div class="ms-sub">{{ row.site_name }} · {{ row.torrent_id }}</div>
              </template>
            </el-table-column>
            <el-table-column label="作用于" min-width="150">
              <template #default="{ row }">{{ blockScope(row) }}</template>
            </el-table-column>
            <el-table-column label="原因" min-width="140">
              <template #default="{ row }">
                <span :class="{ 'ms-sub': !row.reason }">{{ row.reason || "没写" }}</span>
              </template>
            </el-table-column>
            <el-table-column label="时间" width="120" class-name="pt-cell-muted">
              <template #default="{ row }">{{ formatShortDateTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column label="" width="80" align="right">
              <template #default="{ row }">
                <el-button
                  link
                  :loading="unblocking === row.id"
                  :data-testid="`ms-block-del-${row.id}`"
                  @click="unblock(row)"
                  >移出</el-button
                >
              </template>
            </el-table-column>
          </el-table>
          <div v-else class="ms-cards">
            <PtRowCard v-for="b in blocks" :key="b.id">
              <template #title>{{ b.title || b.torrent_id }}</template>
              <template #meta>
                <span>{{ b.site_name }}</span>
                <span>{{ blockScope(b) }}</span>
                <span v-if="b.reason" class="ms-full">{{ b.reason }}</span>
                <span>{{ formatShortDateTime(b.created_at) }}</span>
              </template>
              <template #actions>
                <el-button size="small" :loading="unblocking === b.id" @click="unblock(b)"
                  >移出黑名单</el-button
                >
              </template>
            </PtRowCard>
          </div>
        </PtPanel>
      </div>
    </div>

    <SubscribeDialog v-model="editDialog" :sub="editing" @saved="loadSubs" />
    <SubscriptionDetailDialog v-model="detailDialog" :id="detailID" @changed="onDetailChanged" />
    <QualityProfileDialog
      v-model="profileDialog"
      :profile="editingProfile"
      :resolutions="options.resolutions"
      :sources="options.sources"
      :codecs="options.codecs"
      @saved="loadProfiles" />
    <DoubanSourceDialog
      v-model="doubanDialog"
      :source="editingSource"
      :profiles="profiles"
      @saved="loadSources" />
    <el-dialog
      v-model="itemsDialog"
      class="pt-dialog"
      :title="`豆瓣想看：${itemsOf?.name || itemsOf?.user_id || ''}`"
      :width="isMobile ? '94%' : '560px'"
      append-to-body
      align-center
      data-testid="douban-items">
      <p v-if="!items.length" class="ms-sub">还没有见过的条目</p>
      <div v-else class="ms-items">
        <div v-for="it in items" :key="it.id" class="ms-item">
          <span class="ms-item__title"
            >{{ it.title }}<template v-if="it.year"> ({{ it.year }})</template></span
          >
          <PtStatusPill :tone="doubanItemState(it.status).tone" size="sm">{{
            doubanItemState(it.status).label
          }}</PtStatusPill>
        </div>
      </div>
      <template #footer>
        <el-button @click="itemsDialog = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.ms-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-bottom: 14px;
}

.ms-search {
  width: 220px;
  max-width: 100%;
}

.ms-entry {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.ms-poster {
  width: 32px;
  height: 48px;
  flex-shrink: 0;
  object-fit: cover;
  border-radius: 3px;
  background: var(--pt-bg-surface-muted);
}

.ms-entry__text {
  min-width: 0;
}

.ms-strong {
  font-weight: 600;
  color: var(--pt-t1);
}

.ms-sub {
  font-size: 12px;
  color: var(--pt-t3);
}

/* 最近一次的结果：最多两行；种子名没有空格，要允许在任意处断行 */
.ms-msg {
  display: -webkit-box;
  overflow: hidden;
  overflow-wrap: anywhere;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

/* 操作排成一行：窄了「更多」会被挤到第二行（td 是 el-table 渲染的，没有本组件的 scoped 属性，要 :deep） */
:deep(td.ms-acts .cell) {
  white-space: nowrap;
}

/* 「更多」与左边的文字按钮同一节奏（link 按钮之间是 12 的间距） */
.ms-more {
  margin-left: 12px;
  vertical-align: middle;
}

/* 下拉项：图标与文字的间距；删除用危险色。菜单 teleport 到 body，scoped 的属性选择器照样生效 */
.ms-dd-ico {
  margin-right: 6px;
}

.ms-dd-danger {
  color: var(--pt-dang);
}

.ms-error {
  color: var(--pt-dang);
}

.ms-cards {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.ms-full {
  flex-basis: 100%;
  word-break: break-all;
}

/* 每一集一个小格子，颜色同追剧详情里的格子 */
.ms-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
  max-width: 240px;
  margin-top: 4px;
}

.ms-strip__cell {
  --c: var(--pt-t3);

  width: 7px;
  height: 7px;
  border-radius: 2px;
  background: var(--c);
}

.ms-strip__cell--library {
  --c: var(--pt-ok);
}

.ms-strip__cell--downloading {
  --c: var(--pt-info);
}

.ms-strip__cell--missing {
  --c: var(--pt-warn);
}

.ms-strip__cell--waiting_version {
  --c: var(--pt-p);
}

.ms-strip__cell--upcoming {
  background: color-mix(in srgb, var(--pt-t3) 30%, transparent);
}

.ms-strip__cell--skipped {
  border: 1px dashed var(--pt-t4);
  background: transparent;
}

.ms-form {
  max-width: 860px;
}

/* 条目名点进条目详情：样式不变，悬停时下划线 */
.ms-link {
  color: inherit;
  text-decoration: none;
}

.ms-link:hover {
  color: var(--pt-p);
  text-decoration: underline;
}

.ms-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 0 20px;
}

.ms-switch {
  display: flex;
  align-items: center;
  gap: 8px;
  line-height: 1.4;
  font-size: 13px;
  color: var(--pt-t2);
}

.ms-tip {
  margin: 0 0 12px;
  font-size: 12px;
  color: var(--pt-t3);
}

.ms-items {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ms-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  font-size: 13px;
  color: var(--pt-t1);
}

.ms-item__title {
  min-width: 0;
  word-break: break-all;
}

@media (max-width: 767px) {
  .ms-search {
    width: 100%;
  }
}
</style>
