<script setup lang="ts">
/*
 * 订阅弹窗：新建（从探索页点「订阅」「追剧」，或从搜索结果的种子开始追）与修改订阅。剧集要选季（不填追最新一季）；
 * 可以选质量档案、只在哪些站点找、推送到哪个下载器、分类、标签、保存目录，打开洗版。条目与季建好以后不能改。
 * 剧集是追剧：什么时候搜索（默认按播出时间）、版本锁（默认开）、从第几集开始、整理进哪个媒体库、追完以后追下一季。
 */
import {
  type DownloaderSetting,
  type FollowSchedule,
  type MediaKind,
  type MediaLibrary,
  type QualityProfile,
  type Subscription,
  type SubscriptionInput,
  downloadersApi,
  organizeApi,
  sitesApi,
  subscribeApi,
} from "@/api";
import { useIsMobile } from "@/composables/useIsMobile";
import { loadTmdbImageBase } from "@/composables/useTmdbImageBase";
import { posterURL } from "@/utils/media";
import { SCHEDULE_KINDS, WEEKDAY_OPTIONS } from "@/utils/subscribe";
import { ElMessage } from "element-plus";
import { computed, ref, watch } from "vue";

export interface SubscribeTarget {
  kind: MediaKind;
  tmdb_id: number;
  title: string;
  year?: number;
  poster_path?: string;
}

/** 一键追剧选中的种子（站点、种子编号与标题）。 */
export interface FollowTorrent {
  site: string;
  torrent_id: string;
  title: string;
}

const props = defineProps<{
  modelValue: boolean;
  /** 新建时的条目 */
  target?: SubscribeTarget | null;
  /** 修改时的订阅 */
  sub?: Subscription | null;
  /** 从这个种子开始追（只在新建剧集时用） */
  fromTorrent?: FollowTorrent | null;
  /** 识别出的其他候选：认错了可以改选 */
  candidates?: SubscribeTarget[];
  /** 新建剧集时预填的季（从种子标题里认出来的） */
  season?: number;
}>();
const emit = defineEmits<{
  (e: "update:modelValue", v: boolean): void;
  (e: "saved", v: Subscription): void;
}>();

const isMobile = useIsMobile();
const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});
const editing = computed(() => Boolean(props.sub));
// 新建时选中的条目（候选里可以改选）
const chosen = ref<SubscribeTarget | null>(null);
const chosenID = ref(0);
const kind = computed<MediaKind>(() => props.sub?.media_type ?? chosen.value?.kind ?? "movie");
const isTV = computed(() => kind.value === "tv");
const title = computed(() => props.sub?.title ?? chosen.value?.title ?? "");
const year = computed(() => props.sub?.year ?? chosen.value?.year ?? 0);
const poster = computed(() => props.sub?.poster_path ?? chosen.value?.poster_path ?? "");
const posterFailed = ref(false);
const choices = computed(() =>
  (props.candidates ?? []).filter((c) => c.kind === "tv" || !props.fromTorrent),
);

const blank = (): SubscriptionInput => ({
  profile_id: 0,
  sites: [],
  downloader_id: 0,
  category: "",
  tags: "",
  save_path: "",
  upgrade: false,
});
const form = ref<SubscriptionInput>(blank());
const season = ref<number | undefined>(undefined);
const saving = ref(false);
const profiles = ref<QualityProfile[]>([]);
const downloaders = ref<DownloaderSetting[]>([]);
const sites = ref<string[]>([]);
const libraries = ref<MediaLibrary[]>([]);

// ---- 追剧 ----
const sched = ref<FollowSchedule>({ kind: "airtime" });
const lockVersion = ref(true);
const clearLock = ref(false);
const startEpisode = ref<number | undefined>(undefined);
const followNext = ref(false);
const libraryID = ref(0);
const totalEpisodes = ref<number | undefined>(undefined);
const totalBefore = ref<number | undefined>(undefined);
// 修改时现在锁住的版本（还没定、或者点了清掉时为空）
const lockedNow = computed(() =>
  props.sub && !clearLock.value
    ? [props.sub.lock_group, props.sub.lock_resolution].filter(Boolean).join(" ")
    : "",
);
const scheduleTip = computed(() => {
  switch (sched.value.kind) {
    case "airtime":
      return "按 TMDB 上每一集的播出日期：播出后过一会儿开始搜，没找到隔几小时再搜，几天以后还没有就每天搜一次";
    case "":
      return "按订阅设置里的间隔定时搜索";
  }
  return "到点搜一次（前后有几分钟的随机偏移）";
});
const tvLibraries = computed(() => libraries.value.filter((l) => l.kind === "tv"));
const monthDays = Array.from({ length: 31 }, (_, i) => i + 1);

async function loadOptions() {
  const [ps, dls, ss, libs] = await Promise.allSettled([
    subscribeApi.profiles(),
    downloadersApi.list(),
    sitesApi.list(),
    organizeApi.libraries(),
  ]);
  if (ps.status === "fulfilled") profiles.value = ps.value?.items ?? [];
  if (dls.status === "fulfilled") downloaders.value = dls.value ?? [];
  if (ss.status === "fulfilled") {
    sites.value = Object.entries(ss.value ?? {})
      .filter(([, c]) => c.enabled)
      .map(([name]) => name)
      .sort();
  }
  if (libs.status === "fulfilled") libraries.value = libs.value ?? [];
}

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return;
    // 「媒体识别」里换了 TMDB 图片地址时，海报从换的地址加载（打开时才读）
    void loadTmdbImageBase();
    posterFailed.value = false;
    chosen.value = props.target ?? null;
    chosenID.value = props.target?.tmdb_id ?? 0;
    const s = props.sub;
    form.value = s
      ? {
          profile_id: s.profile_id,
          sites: [...(s.sites ?? [])],
          downloader_id: s.downloader_id,
          category: s.category,
          tags: s.tags,
          save_path: s.save_path,
          upgrade: s.upgrade,
        }
      : blank();
    season.value = s?.media_type === "tv" ? s.season : props.season || undefined;
    // 修改时按订阅现在的设置；新建剧集按追剧的默认设置（按播出时间、开着版本锁）
    sched.value = s ? { kind: "", days: [], ...s.schedule } : { kind: "airtime", days: [] };
    if (!sched.value.days) sched.value.days = [];
    lockVersion.value = s ? Boolean(s.lock_version) : true;
    clearLock.value = false;
    startEpisode.value = s?.start_episode && s.start_episode > 1 ? s.start_episode : undefined;
    followNext.value = Boolean(s?.follow_next_season);
    libraryID.value = s?.library_id ?? 0;
    totalEpisodes.value = s?.total_episodes_manual ? s.total_episodes : undefined;
    totalBefore.value = totalEpisodes.value;
    void loadOptions();
  },
  { immediate: true },
);

function pickCandidate(id: number) {
  const c = choices.value.find((x) => x.tmdb_id === id);
  if (c) {
    chosen.value = c;
    posterFailed.value = false;
  }
}

/** 只带这种定时用得到的字段。 */
function scheduleInput(): FollowSchedule {
  const sc = sched.value;
  const at = sc.at || undefined;
  switch (sc.kind) {
    case "airtime":
      return {
        kind: "airtime",
        at,
        air_delay_hours: sc.air_delay_hours || 0,
        air_retry_hours: sc.air_retry_hours || 0,
        air_window_days: sc.air_window_days || 0,
      };
    case "daily":
      return { kind: "daily", at };
    case "every_days":
      return { kind: "every_days", at, every: sc.every || 1 };
    case "weekly":
    case "monthly":
      return { kind: sc.kind, at, days: [...(sc.days ?? [])].sort((a, b) => a - b) };
  }
  return { kind: "" };
}

/** 剧集的追剧设置（电影不带）。 */
function followInput(): Partial<SubscriptionInput> {
  if (!isTV.value) return {};
  const out: Partial<SubscriptionInput> = {
    schedule: scheduleInput(),
    lock_version: lockVersion.value,
    follow_next_season: followNext.value,
    library_id: libraryID.value,
  };
  if (editing.value) {
    // 修改时 0 是整季
    out.start_episode = startEpisode.value ?? 0;
    if (clearLock.value) {
      out.lock_group = "";
      out.lock_resolution = "";
    }
    if (totalEpisodes.value !== totalBefore.value) out.total_episodes = totalEpisodes.value ?? 0;
  } else if (startEpisode.value) {
    // 从种子开始追时不填起始集，服务端按这个种子的集号定
    out.start_episode = startEpisode.value;
  }
  return out;
}

async function save() {
  if (
    isTV.value &&
    (sched.value.kind === "weekly" || sched.value.kind === "monthly") &&
    !sched.value.days?.length
  ) {
    ElMessage.warning(sched.value.kind === "weekly" ? "选一下星期几" : "选一下几号");
    return;
  }
  saving.value = true;
  try {
    let saved: Subscription;
    if (props.sub) {
      saved = await subscribeApi.update(props.sub.id, { ...form.value, ...followInput() });
      ElMessage.success(isTV.value ? "已保存追剧" : "已保存订阅");
    } else if (chosen.value) {
      saved = await subscribeApi.create({
        ...form.value,
        ...followInput(),
        media_type: chosen.value.kind,
        tmdb_id: chosen.value.tmdb_id,
        season: isTV.value ? (season.value ?? 0) : 0,
        ...(isTV.value && props.fromTorrent ? { from_torrent: { ...props.fromTorrent } } : {}),
      });
      if (isTV.value) {
        ElMessage.success(`开始追剧：${saved.title}`);
        // 从种子开始追时，那个种子推没推、为什么，写在订阅的说明里
        if (props.fromTorrent && saved.message) ElMessage.info(saved.message);
      } else {
        ElMessage.success(`已订阅：${saved.title}`);
      }
    } else {
      return;
    }
    emit("saved", saved);
    visible.value = false;
  } catch (e) {
    ElMessage.error((e as Error)?.message || "保存失败");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <el-dialog
    v-model="visible"
    class="pt-dialog"
    :title="editing ? (isTV ? '修改追剧' : '修改订阅') : isTV ? '追剧' : '订阅'"
    :width="isMobile ? '94%' : '600px'"
    append-to-body
    align-center
    data-testid="subscribe-dialog">
    <div class="sd">
      <div class="sd__head">
        <img
          v-if="posterURL(poster, 'w92') && !posterFailed"
          class="sd__poster"
          :src="posterURL(poster, 'w92')"
          alt=""
          @error="posterFailed = true" />
        <div class="sd__title">
          <div class="sd__name" data-testid="sd-name">
            {{ title }}<template v-if="year"> ({{ year }})</template>
          </div>
          <div class="sd__sub">{{ isTV ? "剧集" : "电影" }}</div>
        </div>
      </div>
      <div v-if="!editing && fromTorrent" class="sd__from" data-testid="sd-from">
        <div class="sd__label">从这个种子开始追</div>
        <div class="sd__torrent">{{ fromTorrent.title }}</div>
        <div class="field-tip">
          建好以后马上推送它（不合质量档案、或者这几集已经有了时不推，原因写在追剧的说明里）；之后按下面的设置追后面的集
        </div>
      </div>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <el-form-item v-if="!editing && choices.length > 1" label="认出的条目（认错了可以改选）">
          <el-select v-model="chosenID" data-testid="sd-candidate" @change="pickCandidate">
            <el-option
              v-for="c in choices"
              :key="c.tmdb_id"
              :value="c.tmdb_id"
              :label="`${c.title}${c.year ? ` (${c.year})` : ''}${c.kind === 'tv' ? '' : ' · 电影'}`" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="isTV" label="季">
          <el-input-number
            v-model="season"
            :min="1"
            :max="200"
            :disabled="editing"
            controls-position="right"
            placeholder="最新一季"
            data-testid="sd-season" />
          <div class="field-tip">
            {{
              editing
                ? "季不能改，要追别的季再建一个"
                : "不填时追最新一季；建好以后不能改，要追别的季再建一个"
            }}
          </div>
        </el-form-item>

        <template v-if="isTV">
          <div class="sd__group">追剧</div>
          <el-form-item label="什么时候搜索">
            <el-select
              v-model="sched.kind"
              :empty-values="[null, undefined]"
              data-testid="sd-schedule">
              <el-option
                v-for="k in SCHEDULE_KINDS"
                :key="k.value || 'global'"
                :value="k.value"
                :label="k.label" />
            </el-select>
            <div class="field-tip">{{ scheduleTip }}</div>
          </el-form-item>
          <div v-if="sched.kind" class="sd__row">
            <el-form-item :label="sched.kind === 'airtime' ? '播出时刻' : '几点搜'">
              <el-time-select
                v-model="sched.at"
                start="00:00"
                step="00:15"
                end="23:45"
                placeholder="20:00"
                data-testid="sd-at" />
            </el-form-item>
            <el-form-item v-if="sched.kind === 'every_days'" label="每隔几天">
              <el-input-number
                v-model="sched.every"
                :min="1"
                :max="30"
                controls-position="right"
                placeholder="1"
                data-testid="sd-every" />
            </el-form-item>
          </div>
          <el-form-item v-if="sched.kind === 'weekly'" label="星期几">
            <el-checkbox-group v-model="sched.days" data-testid="sd-weekdays">
              <el-checkbox v-for="d in WEEKDAY_OPTIONS" :key="d.value" :value="d.value">{{
                d.label
              }}</el-checkbox>
            </el-checkbox-group>
          </el-form-item>
          <el-form-item v-if="sched.kind === 'monthly'" label="几号">
            <el-select
              v-model="sched.days"
              multiple
              placeholder="选几号"
              data-testid="sd-monthdays">
              <el-option v-for="d in monthDays" :key="d" :value="d" :label="`${d} 号`" />
            </el-select>
            <div class="field-tip">小月没有的日子按月末</div>
          </el-form-item>
          <div v-if="sched.kind === 'airtime'" class="sd__row">
            <el-form-item label="播出后几小时开始搜">
              <el-input-number
                v-model="sched.air_delay_hours"
                :min="0"
                :max="72"
                controls-position="right"
                placeholder="2"
                data-testid="sd-air-delay" />
            </el-form-item>
            <el-form-item label="没找到隔几小时再搜">
              <el-input-number
                v-model="sched.air_retry_hours"
                :min="0"
                :max="48"
                controls-position="right"
                placeholder="6"
                data-testid="sd-air-retry" />
            </el-form-item>
            <el-form-item label="播出后几天里集中重试">
              <el-input-number
                v-model="sched.air_window_days"
                :min="0"
                :max="60"
                controls-position="right"
                placeholder="7"
                data-testid="sd-air-window" />
            </el-form-item>
          </div>
          <el-form-item label="版本锁">
            <div class="sd__switch">
              <el-switch v-model="lockVersion" data-testid="sd-lock" />
              <span>第一个下载的版本定下以后，只要同一个制作组、同一个分辨率</span>
            </div>
            <div class="field-tip">
              锁住的版本等不到时接着等，不换别的版本；某一集要换，在追剧详情里点那一集「换版本」
            </div>
            <div v-if="lockVersion && lockedNow" class="sd__locked" data-testid="sd-locked">
              <span>现在锁住：{{ lockedNow }}</span>
              <el-button link type="primary" data-testid="sd-clear-lock" @click="clearLock = true"
                >清掉，按下一个下载的版本重新定</el-button
              >
            </div>
          </el-form-item>
          <div class="sd__row">
            <el-form-item label="从第几集开始追">
              <el-input-number
                v-model="startEpisode"
                :min="1"
                :max="2000"
                controls-position="right"
                :placeholder="!editing && fromTorrent ? '这个种子的集' : '第 1 集'"
                data-testid="sd-start" />
            </el-form-item>
            <el-form-item v-if="editing" label="这一季的集数">
              <el-input-number
                v-model="totalEpisodes"
                :min="1"
                :max="2000"
                controls-position="right"
                :placeholder="`按 TMDB（${sub?.total_episodes ?? 0}）`"
                data-testid="sd-total" />
              <div class="field-tip">TMDB 上的集数不对时填；清空改回按 TMDB</div>
            </el-form-item>
          </div>
          <el-form-item label="整理进哪个媒体库">
            <el-select v-model="libraryID" data-testid="sd-library">
              <el-option :value="0" label="按类型自动选" />
              <el-option v-for="l in tvLibraries" :key="l.id" :value="l.id" :label="l.name" />
            </el-select>
          </el-form-item>
          <el-form-item label="追完以后">
            <div class="sd__switch">
              <el-switch v-model="followNext" data-testid="sd-follow-next" />
              <span>接着追下一季（TMDB 上有了就建，设置照这一季）</span>
            </div>
          </el-form-item>
          <div class="sd__group">下载</div>
        </template>

        <el-form-item label="质量档案">
          <el-select v-model="form.profile_id" data-testid="sd-profile">
            <el-option :value="0" label="用设置里的默认档案" />
            <el-option v-for="p in profiles" :key="p.id" :value="p.id" :label="p.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="只在这些站点找（不选时是全部启用的站点）">
          <el-select
            v-model="form.sites"
            multiple
            filterable
            clearable
            placeholder="全部站点"
            data-testid="sd-sites">
            <el-option v-for="s in sites" :key="s" :value="s" :label="s" />
          </el-select>
        </el-form-item>
        <el-form-item label="下载器">
          <el-select v-model="form.downloader_id" data-testid="sd-downloader">
            <el-option :value="0" label="用设置里的默认下载器" />
            <el-option
              v-for="d in downloaders"
              :key="d.id"
              :value="d.id as number"
              :label="d.name" />
          </el-select>
        </el-form-item>
        <div class="sd__row">
          <el-form-item label="分类">
            <el-input v-model="form.category" placeholder="不填" data-testid="sd-category" />
          </el-form-item>
          <el-form-item label="标签">
            <el-input
              v-model="form.tags"
              placeholder="不填，多个用逗号分开"
              data-testid="sd-tags" />
          </el-form-item>
        </div>
        <el-form-item label="保存目录（下载器里看到的）">
          <el-input
            v-model="form.save_path"
            placeholder="不填时用下载器的默认目录"
            data-testid="sd-save-path" />
        </el-form-item>
        <el-form-item label="洗版">
          <div class="sd__switch">
            <el-switch v-model="form.upgrade" data-testid="sd-upgrade" />
            <span>下载过以后接着找分数更高的版本，入库后替换旧版本</span>
          </div>
          <div class="field-tip">
            达到质量档案里排第一的分辨率与来源后停止；剧集只换整季包。档案里没有排分辨率与来源时，下载过一次就停。
          </div>
        </el-form-item>
      </el-form>
    </div>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" data-testid="sd-save" @click="save">{{
        editing ? "保存" : isTV ? "开始追" : "订阅"
      }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.sd {
  display: flex;
  flex-direction: column;
  gap: 14px;
  min-width: 0;
  /* 剧集的表单长：正文自己滚，页头与页脚留在原地 */
  max-height: min(68vh, 680px);
  overflow-y: auto;
  padding-right: 2px;
}

.sd__head {
  display: flex;
  align-items: center;
  gap: 12px;
}

.sd__poster {
  width: 46px;
  height: 69px;
  flex-shrink: 0;
  object-fit: cover;
  border-radius: 4px;
  background: var(--pt-bg-surface-muted);
}

.sd__title {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.sd__name {
  font-size: 14px;
  font-weight: 600;
  color: var(--pt-t1);
  word-break: break-all;
}

.sd__sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.sd__from {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px 12px;
  border: 1px solid var(--pt-border);
  border-radius: 6px;
  background: var(--pt-bg-surface-muted);
}

.sd__label {
  font-size: 12px;
  font-weight: 600;
  color: var(--pt-t2);
}

.sd__torrent {
  font-size: 13px;
  color: var(--pt-t1);
  word-break: break-all;
}

.sd__group {
  margin: 4px 0 10px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--pt-border);
  font-size: 12px;
  font-weight: 600;
  color: var(--pt-t2);
}

.sd__row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 0 14px;
}

.sd__switch {
  display: flex;
  align-items: center;
  gap: 8px;
  line-height: 1.4;
  font-size: 13px;
  color: var(--pt-t2);
}

.sd__locked {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 10px;
  margin-top: 4px;
  font-size: 12px;
  color: var(--pt-t2);
}
</style>
