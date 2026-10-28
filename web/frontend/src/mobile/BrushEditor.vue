<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import type { BrushTaskConfig } from "@/api";
import { mobileApi, type MobileOptions } from "./api";
import { BridgeError, notify, request } from "./bridge";
import { useWords } from "./i18n";
const props = defineProps<{ id: number; canWrite: boolean }>();
const t = useWords();
const loading = ref(true);
const saving = ref(false);
const error = ref("");
const forbidden = ref(false);
const original = ref("");
const options = ref<MobileOptions>();
interface Preview {
  summary: string;
  changes: { label: string; before?: string; after?: string }[];
  impact: string[];
}
const preview = ref<Preview>();
const form = ref<BrushTaskConfig>({
  name: "",
  enabled: false,
  site_name: "",
  downloader_id: 0,
  save_path: "",
  category: "",
  tags: "",
  interval_min: 30,
  section: "normal",
  discounts: "FREE",
  min_free_remain_min: 0,
  min_size_gb: 0,
  max_size_gb: 0,
  max_seeders: 0,
  min_leechers: 0,
  max_publish_age_min: 0,
  exclude_hr: true,
  include_keywords: "",
  exclude_keywords: "",
  max_downloading: 3,
  max_total_size_gb: 0,
  max_daily_download_gb: 0,
  remove_seed_time_h: 0,
  remove_ratio: 0,
  remove_low_speed_kbs: 0,
  remove_low_speed_window_min: 30,
  remove_inactive_h: 0,
  remove_free_expired_incomplete: false,
  remove_with_data: false,
});
const numericFields: {
  key: keyof BrushTaskConfig;
  zh: string;
  en: string;
  min: number;
  max?: number;
  integer?: boolean;
}[] = [
  {
    key: "min_free_remain_min",
    zh: "最短剩余免费时间（分钟）",
    en: "Minimum free time remaining (minutes)",
    min: 0,
    integer: true,
  },
  { key: "min_size_gb", zh: "最小体积（GB）", en: "Minimum size (GB)", min: 0 },
  {
    key: "max_size_gb",
    zh: "最大体积（GB，0 不限）",
    en: "Maximum size (GB, 0 = unlimited)",
    min: 0,
  },
  {
    key: "max_seeders",
    zh: "最多做种人数（0 不限）",
    en: "Maximum seeders (0 = unlimited)",
    min: 0,
    integer: true,
  },
  { key: "min_leechers", zh: "最少下载人数", en: "Minimum leechers", min: 0, integer: true },
  {
    key: "max_publish_age_min",
    zh: "最大发布时长（分钟，0 不限）",
    en: "Maximum age (minutes, 0 = unlimited)",
    min: 0,
    integer: true,
  },
  {
    key: "max_total_size_gb",
    zh: "在刷体积上限（GB，0 不限）",
    en: "Active size limit (GB, 0 = unlimited)",
    min: 0,
  },
  {
    key: "max_daily_download_gb",
    zh: "每日下载上限（GB，0 不限）",
    en: "Daily download limit (GB, 0 = unlimited)",
    min: 0,
  },
  {
    key: "remove_seed_time_h",
    zh: "做种多久后删除（小时，0 关闭）",
    en: "Remove after seeding (hours, 0 = disabled)",
    min: 0,
  },
  {
    key: "remove_ratio",
    zh: "达到分享率删除（0 关闭）",
    en: "Remove at ratio (0 = disabled)",
    min: 0,
  },
  {
    key: "remove_low_speed_kbs",
    zh: "低于上传速度删除（KB/s，0 关闭）",
    en: "Remove below upload speed (KB/s, 0 = disabled)",
    min: 0,
  },
  {
    key: "remove_low_speed_window_min",
    zh: "低速观察窗口（分钟）",
    en: "Low-speed window (minutes)",
    min: 5,
    max: 1440,
    integer: true,
  },
  {
    key: "remove_inactive_h",
    zh: "无活动多久后删除（小时，0 关闭）",
    en: "Remove after inactivity (hours, 0 = disabled)",
    min: 0,
  },
];
watch(
  form,
  () => {
    preview.value = undefined;
    if (!loading.value) notify("dirty", { value: JSON.stringify(form.value) !== original.value });
  },
  { deep: true },
);
function numberValue(key: keyof BrushTaskConfig): number {
  return Number(form.value[key]);
}
function setNumber(key: keyof BrushTaskConfig, value: number | undefined) {
  Object.assign(form.value, { [key]: value ?? 0 });
}
async function load() {
  loading.value = true;
  error.value = "";
  forbidden.value = false;
  try {
    const [opts, current] = await Promise.all([
      mobileApi.options(),
      props.id
        ? request<BrushTaskConfig>(`/api/app/v1/brush/tasks/${props.id}`)
        : Promise.resolve(null),
    ]);
    options.value = opts;
    if (current)
      for (const key of Object.keys(form.value) as (keyof BrushTaskConfig)[]) {
        if (current[key] !== undefined) Object.assign(form.value, { [key]: current[key] });
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
  if (!form.value.name.trim() || !form.value.site_name || !form.value.downloader_id) {
    error.value = t(
      "请填写任务名并选择站点、下载器",
      "Enter a task name and select a site and downloader",
    );
    return;
  }
  saving.value = true;
  try {
    if (!preview.value) {
      preview.value = await request<Preview>("/api/app/v1/brush/tasks/preview", "POST", {
        op: props.id ? "update" : "add",
        ...(props.id ? { id: String(props.id) } : {}),
        values: form.value,
      });
      return;
    }
    await request(
      `/api/app/v1/brush/tasks${props.id ? `/${props.id}` : ""}`,
      props.id ? "PUT" : "POST",
      form.value,
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
      <p v-if="!canWrite">{{ t("这台设备只有只读权限", "This device is read-only") }}</p>
      <p v-if="options.errors && Object.keys(options.errors).length" class="mobile-note">
        {{
          t(
            "部分选项未能加载，保存前请重试",
            "Some options could not be loaded. Retry before saving.",
          )
        }}
      </p>
      <section class="mobile-section">
        <h2>
          {{ t(id ? "编辑刷流任务" : "新增刷流任务", id ? "Edit brush task" : "New brush task") }}
        </h2>
        <el-form-item :label="t('任务名', 'Task name')"
          ><el-input v-model="form.name" :maxlength="64"
        /></el-form-item>
        <el-form-item :label="t('站点', 'Site')"
          ><el-select v-model="form.site_name"
            ><el-option
              v-for="s in options.sites"
              :key="s.name"
              :value="s.name"
              :label="s.display_name || s.name" /></el-select
        ></el-form-item>
        <el-form-item :label="t('下载器', 'Downloader')"
          ><el-select v-model="form.downloader_id"
            ><el-option
              v-for="d in options.downloaders"
              :key="d.id"
              :value="d.id"
              :label="d.name" /></el-select
        ></el-form-item>
        <el-form-item :label="t('检查间隔（分钟）', 'Check interval (minutes)')"
          ><el-input-number v-model="form.interval_min" :min="5" :max="1440" :precision="0"
        /></el-form-item>
        <el-form-item :label="t('同时下载数', 'Concurrent downloads')"
          ><el-input-number v-model="form.max_downloading" :min="1" :max="50" :precision="0"
        /></el-form-item>
        <el-checkbox v-model="form.enabled">{{
          t("保存后启用任务", "Enable task after saving")
        }}</el-checkbox>
      </section>
      <section class="mobile-section">
        <h2>{{ t("选种条件", "Selection rules") }}</h2>
        <el-form-item :label="t('优惠类型（逗号分隔）', 'Discounts (comma separated)')"
          ><el-input v-model="form.discounts" placeholder="FREE"
        /></el-form-item>
        <el-form-item
          :label="t('M-Team 分区（其他站点忽略）', 'M-Team section (ignored for other sites)')"
          ><el-select v-model="form.section"
            ><el-option value="normal" :label="t('综合', 'Normal')" /><el-option
              value="adult"
              :label="t('成人', 'Adult')" /><el-option
              value="all"
              :label="t('全部', 'All')" /></el-select
        ></el-form-item>
        <el-form-item :label="t('标题包含', 'Title includes')"
          ><el-input v-model="form.include_keywords" type="textarea" :maxlength="1000"
        /></el-form-item>
        <el-form-item :label="t('标题排除', 'Title excludes')"
          ><el-input v-model="form.exclude_keywords" type="textarea" :maxlength="1000"
        /></el-form-item>
        <el-checkbox v-model="form.exclude_hr">{{ t("排除 H&R", "Exclude H&R") }}</el-checkbox>
      </section>
      <el-collapse>
        <el-collapse-item
          :title="t('体积、人数、限额与删种规则', 'Size, peers, limits and removal rules')"
          name="rules">
          <el-form-item
            v-for="field in numericFields"
            :key="field.key"
            :label="t(field.zh, field.en)"
            ><el-input-number
              :model-value="numberValue(field.key)"
              :min="field.min"
              :max="field.max"
              :precision="field.integer ? 0 : 2"
              @update:model-value="setNumber(field.key, $event)"
          /></el-form-item>
          <el-checkbox v-model="form.remove_free_expired_incomplete">{{
            t("免费结束时删除未完成种子", "Remove incomplete torrents when free period ends")
          }}</el-checkbox>
          <el-checkbox v-model="form.remove_with_data">{{
            t("删种时连数据一起删除", "Delete data when removing torrents")
          }}</el-checkbox>
          <p v-if="form.remove_with_data" role="alert" class="mobile-note">
            {{
              t(
                "启用后，符合删种规则的文件也会被删除。请确认规则。",
                "Matching torrent data will also be deleted. Review removal rules carefully.",
              )
            }}
          </p>
        </el-collapse-item>
        <el-collapse-item :title="t('下载选项', 'Download options')" name="download">
          <el-form-item :label="t('保存目录', 'Save path')"
            ><el-input v-model="form.save_path"
          /></el-form-item>
          <el-form-item :label="t('分类', 'Category')"
            ><el-input v-model="form.category"
          /></el-form-item>
          <el-form-item :label="t('标签', 'Tags')"><el-input v-model="form.tags" /></el-form-item>
        </el-collapse-item>
      </el-collapse>
      <section v-if="preview" class="mobile-section" role="status">
        <h2>{{ t("确认改动", "Review changes") }}</h2>
        <p>{{ preview.summary }}</p>
        <p v-for="(change, index) in preview.changes" :key="index">
          {{ change.label }}: {{ change.before }} → {{ change.after }}
        </p>
        <p v-for="impact in preview.impact" :key="impact" class="mobile-note">{{ impact }}</p>
      </section>
      <div v-if="canWrite" class="mobile-actions">
        <el-button type="primary" native-type="submit" :loading="saving">{{
          preview ? t("确认保存", "Confirm save") : t("预览改动", "Review changes")
        }}</el-button>
      </div>
    </el-form>
  </template>
</template>
