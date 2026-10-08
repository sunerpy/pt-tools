<script setup lang="ts">
/*
 * 订阅弹窗：新建（从探索页点「订阅」）与修改订阅。剧集要选季（不填订最新一季）；可以选质量档案、只在哪些站点找、
 * 推送到哪个下载器、分类、标签、保存目录，打开洗版。条目与季建好以后不能改。
 */
import {
  type DownloaderSetting,
  type MediaKind,
  type QualityProfile,
  type Subscription,
  type SubscriptionInput,
  downloadersApi,
  sitesApi,
  subscribeApi,
} from "@/api";
import { useIsMobile } from "@/composables/useIsMobile";
import { posterURL } from "@/utils/media";
import { ElMessage } from "element-plus";
import { computed, ref, watch } from "vue";

export interface SubscribeTarget {
  kind: MediaKind;
  tmdb_id: number;
  title: string;
  year?: number;
  poster_path?: string;
}

const props = defineProps<{
  modelValue: boolean;
  /** 新建时的条目 */
  target?: SubscribeTarget | null;
  /** 修改时的订阅 */
  sub?: Subscription | null;
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
const kind = computed<MediaKind>(() => props.sub?.media_type ?? props.target?.kind ?? "movie");
const title = computed(() => props.sub?.title ?? props.target?.title ?? "");
const year = computed(() => props.sub?.year ?? props.target?.year ?? 0);
const poster = computed(() => props.sub?.poster_path ?? props.target?.poster_path ?? "");
const posterFailed = ref(false);

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

async function loadOptions() {
  const [ps, dls, ss] = await Promise.allSettled([
    subscribeApi.profiles(),
    downloadersApi.list(),
    sitesApi.list(),
  ]);
  if (ps.status === "fulfilled") profiles.value = ps.value?.items ?? [];
  if (dls.status === "fulfilled") downloaders.value = dls.value ?? [];
  if (ss.status === "fulfilled") {
    sites.value = Object.entries(ss.value ?? {})
      .filter(([, c]) => c.enabled)
      .map(([name]) => name)
      .sort();
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return;
    posterFailed.value = false;
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
    season.value = s?.media_type === "tv" ? s.season : undefined;
    void loadOptions();
  },
  { immediate: true },
);

async function save() {
  saving.value = true;
  try {
    let saved: Subscription;
    if (props.sub) {
      saved = await subscribeApi.update(props.sub.id, form.value);
      ElMessage.success("已保存订阅");
    } else if (props.target) {
      saved = await subscribeApi.create({
        ...form.value,
        media_type: props.target.kind,
        tmdb_id: props.target.tmdb_id,
        season: kind.value === "tv" ? (season.value ?? 0) : 0,
      });
      ElMessage.success(`已订阅：${saved.title}`);
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
    :title="editing ? '修改订阅' : '订阅'"
    :width="isMobile ? '94%' : '560px'"
    append-to-body
    align-center
    data-testid="subscribe-dialog">
    <div class="sd">
      <div class="sd__head">
        <img
          v-if="poster && !posterFailed"
          class="sd__poster"
          :src="posterURL(poster, 'w92')"
          alt=""
          @error="posterFailed = true" />
        <div class="sd__title">
          <div class="sd__name" data-testid="sd-name">
            {{ title }}<template v-if="year"> ({{ year }})</template>
          </div>
          <div class="sd__sub">{{ kind === "tv" ? "剧集" : "电影" }}</div>
        </div>
      </div>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <el-form-item v-if="kind === 'tv'" label="季">
          <el-input-number
            v-model="season"
            :min="1"
            :max="200"
            :disabled="editing"
            controls-position="right"
            placeholder="最新一季"
            data-testid="sd-season" />
          <div class="field-tip">不填时订最新一季；建好以后不能改，要订别的季再建一个</div>
        </el-form-item>
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
        editing ? "保存" : "订阅"
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

.sd__row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
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
</style>
