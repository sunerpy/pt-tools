<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { BridgeError, notify, request } from "./bridge";
import { mobileApi, type MobileOptions, type SubscriptionForm } from "./api";
import { useWords } from "./i18n";
const props = defineProps<{ id: number; canWrite: boolean }>();
const t = useWords();
const query = new URLSearchParams(location.search);
const loading = ref(true);
const saving = ref(false);
const error = ref("");
const forbidden = ref(false);
const options = ref<MobileOptions>();
const original = ref("");
const title = ref("");
const form = ref<SubscriptionForm>({
  media_type: query.get("media_type") || "tv",
  tmdb_id: Number(query.get("tmdb_id") || 0),
  season: Number(query.get("season") || 0),
  profile_id: 0,
  sites: [],
  downloader_id: 0,
  category: "",
  tags: "",
  save_path: "",
  schedule: { kind: "airtime", interval_minutes: 360, cron: "" },
  lock_version: true,
  start_episode: 1,
  follow_next_season: false,
  library_id: 0,
  upgrade: false,
  include_keywords: "",
  exclude_keywords: "",
});
const tv = computed(() => form.value.media_type === "tv");
watch(
  form,
  () => {
    if (!loading.value) notify("dirty", { value: JSON.stringify(form.value) !== original.value });
  },
  { deep: true },
);
async function load() {
  loading.value = true;
  error.value = "";
  forbidden.value = false;
  try {
    const [opts, current] = await Promise.all([
      mobileApi.options(),
      props.id ? mobileApi.subscription(props.id) : Promise.resolve(null),
    ]);
    options.value = opts;
    if (current) {
      title.value = current.title;
      for (const key of Object.keys(form.value) as (keyof SubscriptionForm)[]) {
        if (current[key] !== undefined && current[key] !== null)
          Object.assign(form.value, { [key]: current[key] });
      }
    }
    original.value = JSON.stringify(form.value);
    notify("dirty", { value: false });
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
    forbidden.value = e instanceof BridgeError && [401, 403].includes(e.status);
  } finally {
    loading.value = false;
  }
}
async function save() {
  if (saving.value || !props.canWrite) return;
  error.value = "";
  if (!props.id && (!Number.isInteger(form.value.tmdb_id) || form.value.tmdb_id <= 0)) {
    error.value = t("请填写有效的 TMDB 编号", "Enter a valid TMDB ID");
    return;
  }
  saving.value = true;
  try {
    const { media_type, tmdb_id, season, ...editable } = form.value;
    const payload = props.id ? editable : { media_type, tmdb_id, season, ...editable };
    await request(
      `/api/app/v1/subscriptions${props.id ? `/${props.id}` : ""}`,
      props.id ? "PUT" : "POST",
      payload,
    );
    notify("dirty", { value: false });
    notify("close", { changed: true });
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    saving.value = false;
  }
}
onMounted(() => void load());
</script>
<template>
  <div v-if="loading" role="status" class="mobile-state">{{ t("正在读取…", "Loading…") }}</div>
  <template v-else>
    <div v-if="error" role="alert" class="mobile-error">{{ error }}</div>
    <el-button v-if="!options && !forbidden" @click="load">{{ t("重试", "Retry") }}</el-button>
    <el-form
      v-if="options"
      label-position="top"
      :disabled="!canWrite || saving"
      @submit.prevent="save">
      <p v-if="!canWrite" role="status">
        {{ t("这台设备只有只读权限", "This device is read-only") }}
      </p>
      <p v-if="options.errors && Object.keys(options.errors).length" class="mobile-note">
        {{
          t(
            "部分选项未能加载，保存前请重试",
            "Some options could not be loaded. Retry before saving.",
          )
        }}
      </p>
      <section class="mobile-section">
        <h2>{{ title || t("新增追剧订阅", "New subscription") }}</h2>
        <template v-if="!id">
          <el-form-item :label="t('类型', 'Type')"
            ><el-select v-model="form.media_type"
              ><el-option value="tv" :label="t('剧集', 'TV')" /><el-option
                value="movie"
                :label="t('电影', 'Movie')" /></el-select
          ></el-form-item>
          <el-form-item label="TMDB ID"
            ><el-input-number v-model="form.tmdb_id" :min="1" :precision="0"
          /></el-form-item>
          <el-form-item v-if="tv" :label="t('季（0 表示最新一季）', 'Season (0 = latest)')"
            ><el-input-number v-model="form.season" :min="0" :precision="0"
          /></el-form-item>
        </template>
        <el-form-item :label="t('质量档案', 'Quality profile')"
          ><el-select v-model="form.profile_id"
            ><el-option :value="0" :label="t('默认', 'Default')" /><el-option
              v-for="p in options.profiles"
              :key="p.id"
              :value="p.id"
              :label="p.name" /></el-select
        ></el-form-item>
        <el-form-item :label="t('站点（不选则搜索全部）', 'Sites (empty = all)')"
          ><el-select v-model="form.sites" multiple
            ><el-option
              v-for="s in options.sites"
              :key="s.name"
              :value="s.name"
              :label="s.display_name || s.name" /></el-select
        ></el-form-item>
        <el-form-item :label="t('下载器', 'Downloader')"
          ><el-select v-model="form.downloader_id"
            ><el-option :value="0" :label="t('默认', 'Default')" /><el-option
              v-for="d in options.downloaders"
              :key="d.id"
              :value="d.id"
              :label="d.name" /></el-select
        ></el-form-item>
        <el-form-item :label="t('媒体库', 'Library')"
          ><el-select v-model="form.library_id"
            ><el-option :value="0" :label="t('默认', 'Default')" /><el-option
              v-for="l in options.libraries"
              :key="l.id"
              :value="l.id"
              :label="l.name" /></el-select
        ></el-form-item>
      </section>
      <section class="mobile-section">
        <h2>{{ t("搜索与追更", "Search and follow") }}</h2>
        <el-form-item v-if="tv" :label="t('起始集', 'Start episode')"
          ><el-input-number v-model="form.start_episode" :min="1" :precision="0"
        /></el-form-item>
        <el-form-item :label="t('标题包含（每行一个）', 'Title includes (one per line)')"
          ><el-input v-model="form.include_keywords" type="textarea"
        /></el-form-item>
        <el-form-item :label="t('标题排除（每行一个）', 'Title excludes (one per line)')"
          ><el-input v-model="form.exclude_keywords" type="textarea"
        /></el-form-item>
        <template v-if="tv">
          <el-form-item :label="t('搜索定时', 'Search schedule')"
            ><el-select v-model="form.schedule.kind"
              ><el-option value="" :label="t('跟随全局间隔', 'Global interval')" /><el-option
                value="daily"
                :label="t('每天', 'Daily')" /><el-option
                value="every_days"
                :label="t('每隔几天', 'Every N days')" /><el-option
                value="weekly"
                :label="t('每周', 'Weekly')" /><el-option
                value="monthly"
                :label="t('每月', 'Monthly')" /><el-option
                value="airtime"
                :label="t('按播出时间', 'Airtime')" /><el-option
                value="interval"
                :label="t('固定间隔', 'Interval')" /><el-option
                value="cron"
                label="Cron" /></el-select
          ></el-form-item>
          <el-form-item
            v-if="
              ['airtime', 'daily', 'every_days', 'weekly', 'monthly'].includes(form.schedule.kind)
            "
            :label="t('搜索时间（HH:mm）', 'Search time (HH:mm)')"
            ><el-input v-model="form.schedule.at" placeholder="20:00"
          /></el-form-item>
          <el-form-item
            v-if="form.schedule.kind === 'every_days'"
            :label="t('每隔几天', 'Every N days')"
            ><el-input-number v-model="form.schedule.every" :min="1" :precision="0"
          /></el-form-item>
          <el-form-item
            v-if="['weekly', 'monthly'].includes(form.schedule.kind)"
            :label="
              t(
                form.schedule.kind === 'weekly' ? '星期（0 是周日）' : '每月日期',
                form.schedule.kind === 'weekly' ? 'Weekdays (0 = Sunday)' : 'Days of month',
              )
            "
            ><el-select v-model="form.schedule.days" multiple
              ><el-option
                v-for="n in form.schedule.kind === 'weekly' ? 7 : 31"
                :key="n"
                :value="form.schedule.kind === 'weekly' ? n - 1 : n"
                :label="String(form.schedule.kind === 'weekly' ? n - 1 : n)" /></el-select
          ></el-form-item>
          <el-form-item
            v-if="form.schedule.kind === 'interval'"
            :label="t('间隔（分钟，30–10080）', 'Interval (minutes, 30–10080)')"
            ><el-input-number
              v-model="form.schedule.interval_minutes"
              :min="30"
              :max="10080"
              :precision="0"
          /></el-form-item>
          <el-form-item v-if="form.schedule.kind === 'cron'" label="Cron"
            ><el-input v-model="form.schedule.cron" placeholder="0 20 * * *"
          /></el-form-item>
          <el-checkbox v-model="form.lock_version">{{
            t("锁定版本", "Lock release version")
          }}</el-checkbox>
          <el-checkbox v-model="form.follow_next_season">{{
            t("自动追下一季", "Follow next season")
          }}</el-checkbox>
        </template>
        <el-checkbox v-model="form.upgrade">{{
          t("允许升级版本", "Allow quality upgrades")
        }}</el-checkbox>
      </section>
      <el-collapse
        ><el-collapse-item :title="t('下载选项', 'Download options')" name="download">
          <el-form-item :label="t('保存目录', 'Save path')"
            ><el-input v-model="form.save_path"
          /></el-form-item>
          <el-form-item :label="t('分类', 'Category')"
            ><el-input v-model="form.category"
          /></el-form-item>
          <el-form-item :label="t('标签（逗号分隔）', 'Tags (comma separated)')"
            ><el-input v-model="form.tags"
          /></el-form-item> </el-collapse-item
      ></el-collapse>
      <div v-if="canWrite" class="mobile-actions">
        <el-button type="primary" native-type="submit" :loading="saving">{{
          t("保存", "Save")
        }}</el-button>
      </div>
    </el-form>
  </template>
</template>
