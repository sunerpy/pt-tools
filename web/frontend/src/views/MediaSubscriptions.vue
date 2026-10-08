<script setup lang="ts">
/*
 * 订阅（路线图 M11）：订阅列表（进度、最近一次搜索的结果、立即搜索、暂停与恢复、确认、修改、删除），
 * 订阅设置、质量档案与豆瓣想看来源。画板没有这一页，沿用媒体页的样式：页头 + 面板里的表格，手机是行卡。
 */
import {
  type DoubanItem,
  type DoubanSource,
  type DownloaderSetting,
  type NotificationConfig,
  type QualityProfile,
  type SubscribeSettings,
  type Subscription,
  chatopsApi,
  downloadersApi,
  sitesApi,
  subscribeApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
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
import { formatShortDateTime } from "@/utils/format";
import { posterURL } from "@/utils/media";
import {
  SUB_FILTERS,
  profileSummary,
  progressText,
  sourceLabel,
  subName,
  subStatus,
} from "@/utils/subscribe";
import { ElMessage, ElMessageBox } from "element-plus";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";

const isMobile = useIsMobile();
const router = useRouter();
const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;

// ---- 订阅列表 ----
const status = ref("");
const keyword = ref("");
const subs = ref<Subscription[]>([]);
const ds = useDataState({ filtered: () => status.value !== "" || keyword.value.trim() !== "" });
const failedPosters = ref(new Set<number>());

async function loadSubs() {
  const pending = ds.run(() =>
    subscribeApi.list({ status: status.value, q: keyword.value.trim() }),
  );
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  subs.value = got;
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

function refresh() {
  void loadSubs();
  void loadSettings();
  void loadProfiles();
  void loadSources();
}

function openExplore() {
  void router.push("/media/explore");
}

onMounted(() => {
  refresh();
  void loadOptions();
});
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub
      >订阅电影或剧集的一季：定时到各站点搜索，RSS
      里出现时也会下载；剧集补缺的集，洗版时换成更好的版本</PtHeadSub
    >
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="ms-explore" @click="openExplore">
        <PtIcon name="compass" :size="15" /><span>去探索</span>
      </el-button>
      <el-button data-testid="ms-refresh" @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

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
                v-if="row.poster_path && !failedPosters.has(row.id)"
                class="ms-poster"
                :src="posterURL(row.poster_path, 'w92')"
                alt=""
                @error="failedPosters.add(row.id)" />
              <div class="ms-entry__text">
                <div class="ms-strong">{{ subName(row) }}</div>
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
        <el-table-column label="进度" min-width="170">
          <template #default="{ row }">{{ progressText(row.media_type, row.progress) }}</template>
        </el-table-column>
        <el-table-column
          label="最近一次"
          min-width="240"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">
            <div>{{ row.message || "还没搜过" }}</div>
            <div v-if="row.status === 'active' && row.next_search_at" class="ms-sub">
              下次搜索 {{ formatShortDateTime(row.next_search_at) }}
            </div>
          </template>
        </el-table-column>
        <el-table-column label="" width="250" align="right">
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
            <el-button link :data-testid="`ms-edit-${row.id}`" @click="edit(row)">修改</el-button>
            <el-button link type="danger" :data-testid="`ms-del-${row.id}`" @click="removeSub(row)"
              >删除</el-button
            >
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="ms-cards">
        <PtRowCard v-for="s in subs" :key="s.id">
          <template #title>{{ subName(s) }}</template>
          <template #meta>
            <span>{{ progressText(s.media_type, s.progress) }}</span>
            <span
              >{{ sourceLabel(s.source) }} · {{ profileName(s.profile_id)
              }}<template v-if="s.upgrade"> · 洗版</template></span
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

    <PtPanel title="订阅设置" icon="sliders-horizontal">
      <div v-if="settingsFailed" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>设置没读到，点刷新重试。</span>
      </div>
      <el-form class="pt-form ms-form" label-position="top" @submit.prevent>
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
              每个订阅按这个间隔搜一次，另加最多 30 分钟的随机偏移；最少 6 小时，避免给站点添负担
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
            <el-select v-model="sform.default_downloader_id" data-testid="ms-default-downloader">
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
        <el-button
          type="primary"
          :loading="savingSettings"
          data-testid="ms-save-settings"
          @click="saveSettings">
          <PtIcon v-if="!savingSettings" name="save" :size="14" /><span>保存</span>
        </el-button>
      </el-form>
    </PtPanel>

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
            <el-button size="small" type="danger" plain @click="removeProfile(p)">删除</el-button>
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
        <el-table-column label="" width="210" align="right">
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
            <el-button size="small" type="danger" plain @click="removeSource(s)">删除</el-button>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <SubscribeDialog v-model="editDialog" :sub="editing" @saved="loadSubs" />
    <SubscriptionDetailDialog v-model="detailDialog" :id="detailID" />
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
          <PtStatusPill :tone="it.status === 'subscribed' ? 'ok' : 'warn'" size="sm">{{
            it.status === "subscribed" ? "已订阅" : "没找到条目"
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

.ms-form {
  max-width: 860px;
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
