<script setup lang="ts">
import {
  type CleanCategoryResult,
  type CleanResult,
  globalApi,
  type GlobalSettings,
  maintenanceApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref, watch } from "vue";

type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

const isMobile = useIsMobile();

/**
 * 全局配置的六态（设计文档 §5）。这个面板不是表格，但读失败的后果更重：
 * 以前只弹一个 toast，两秒后表单静静停在一整套默认值上，用户点「保存」
 * 就把默认值写回了服务端。所以失败必须留在页面上，并且说清后果。
 */
const {
  loading,
  state: settingsState,
  errorText: settingsErrorText,
  run: runSettings,
  isStale: isSettingsStale,
} = useDataState();

const settingsFailed = computed(
  () => settingsState.value === "error" || settingsState.value === "perm",
);
/** 配置真的读回来过一次。在那之前表单里是前端默认值，不是服务端的删种策略 */
const settingsLoaded = ref(false);
/**
 * 两颗「保存设置」共用的锁，与系统设置页一致：加载中或加载失败时不给保存。
 * save() 会先 GET 现有配置、再用表单整段覆盖删种字段 —— 表单是默认值时，等于把真实策略换成默认值。
 */
const saveLocked = computed(() => loading.value || settingsFailed.value || !settingsLoaded.value);

/* 保存键在这两种状态下都是停用的（saveLocked），提示要说的是「为什么点不了」，而不是「点了会怎样」 */
const settingsAlertText = computed(() => {
  if (settingsState.value === "perm") {
    return "没有权限读取全局配置。下面显示的是默认值，不是服务端的配置，所以保存已停用。";
  }
  return `配置加载失败：${settingsErrorText.value}。下面显示的是默认值，重试成功之前保存已停用，免得用默认值覆盖服务端配置。`;
});

const saving = ref(false);
const scopeTagInput = ref("");
const protectTagInput = ref("");
const selectedPreset = ref("");

const presets = [
  {
    label: "日常刷流（推荐）",
    value: "daily",
    desc: "做种72h 或 分享率2.0 即删",
    config: {
      cleanup_condition_mode: "or",
      cleanup_max_seed_time_h: 72,
      cleanup_min_ratio: 2.0,
      cleanup_max_inactive_h: 0,
      cleanup_slow_seed_time_h: 0,
      cleanup_slow_max_ratio: 0,
      cleanup_del_free_expired: true,
    },
  },
  {
    label: "空间紧张",
    value: "tight",
    desc: "做种48h 或 分享率1.0 或 不活跃24h 或 低效做种",
    config: {
      cleanup_condition_mode: "or",
      cleanup_max_seed_time_h: 48,
      cleanup_min_ratio: 1.0,
      cleanup_max_inactive_h: 24,
      cleanup_slow_seed_time_h: 48,
      cleanup_slow_max_ratio: 0.1,
      cleanup_del_free_expired: true,
    },
  },
  {
    label: "追求高分享率",
    value: "ratio",
    desc: "做种168h 且 分享率3.0 同时满足才删",
    config: {
      cleanup_condition_mode: "and",
      cleanup_max_seed_time_h: 168,
      cleanup_min_ratio: 3.0,
      cleanup_max_inactive_h: 0,
      cleanup_slow_seed_time_h: 0,
      cleanup_slow_max_ratio: 0,
      cleanup_del_free_expired: true,
    },
  },
  {
    label: "仅清理无效种子",
    value: "minimal",
    desc: "只删低效做种和免费到期未完成的",
    config: {
      cleanup_condition_mode: "or",
      cleanup_max_seed_time_h: 0,
      cleanup_min_ratio: 0,
      cleanup_max_inactive_h: 0,
      cleanup_slow_seed_time_h: 72,
      cleanup_slow_max_ratio: 0.05,
      cleanup_del_free_expired: true,
    },
  },
];

function applyPreset(val: string) {
  const preset = presets.find((p) => p.value === val);
  if (!preset) return;
  Object.assign(form.value, preset.config);
}

function detectPreset() {
  const f = form.value;
  for (const p of presets) {
    const cfg = p.config as Record<string, unknown>;
    const matched = Object.keys(cfg).every(
      (k) => (f as unknown as Record<string, unknown>)[k] === cfg[k],
    );
    if (matched) {
      selectedPreset.value = p.value;
      return;
    }
  }
  selectedPreset.value = "";
}

function tagsToArray(s: unknown): string[] {
  if (!s || typeof s !== "string") return [];
  return s
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
}

function addScopeTag() {
  const val = scopeTagInput.value.trim();
  if (val && !form.value.cleanup_scope_tags.includes(val)) {
    form.value.cleanup_scope_tags.push(val);
  }
  scopeTagInput.value = "";
}

function addProtectTag() {
  const val = protectTagInput.value.trim();
  if (val && !form.value.cleanup_protect_tags.includes(val)) {
    form.value.cleanup_protect_tags.push(val);
  }
  protectTagInput.value = "";
}

function removeTag(arr: string[], index: number) {
  arr.splice(index, 1);
}

const form = ref({
  cleanup_enabled: false,
  cleanup_interval_min: 30,
  cleanup_scope: "database",
  cleanup_scope_tags: [] as string[],
  cleanup_remove_data: true,
  cleanup_condition_mode: "or",
  cleanup_max_seed_time_h: 0,
  cleanup_min_ratio: 0,
  cleanup_max_inactive_h: 0,
  cleanup_slow_seed_time_h: 0,
  cleanup_slow_max_ratio: 0,
  cleanup_del_free_expired: true,
  cleanup_disk_protect: true,
  cleanup_min_disk_space_gb: 50,
  cleanup_protect_dl: false,
  cleanup_protect_hr: true,
  cleanup_min_retain_h: 24,
  cleanup_protect_tags: [] as string[],
  peer_ratio_enabled: false,
  peer_ratio_max_sl: 30,
  peer_ratio_interval_min: 10,
  peer_ratio_remove_data: false,
});

const presetFields = computed(() => [
  form.value.cleanup_condition_mode,
  form.value.cleanup_max_seed_time_h,
  form.value.cleanup_min_ratio,
  form.value.cleanup_max_inactive_h,
  form.value.cleanup_slow_seed_time_h,
  form.value.cleanup_slow_max_ratio,
  form.value.cleanup_del_free_expired,
]);

watch(presetFields, () => detectPreset());

onMounted(async () => {
  await loadSettings();
});

async function loadSettings() {
  const pending = runSettings(() => globalApi.get());
  const data = await pending;
  if (isSettingsStale(pending)) return;
  if (!data) {
    // toast 照旧弹，但状态留在页面上的那条提示才是用户两秒后还能看到的东西
    ElMessage.error(settingsErrorText.value || "加载失败");
    return;
  }
  const d = data as unknown as Record<string, unknown>;
  const f = form.value as unknown as Record<string, unknown>;
  Object.keys(f).forEach((key) => {
    if (key in d) {
      f[key] = d[key];
    }
  });
  form.value.cleanup_scope_tags = tagsToArray(d.cleanup_scope_tags as string);
  form.value.cleanup_protect_tags = tagsToArray(d.cleanup_protect_tags as string);
  detectPreset();
  settingsLoaded.value = true;
}

async function save() {
  // 按钮已经按 saveLocked 禁用；这里是按钮之外的最后一道
  if (saveLocked.value) return;
  saving.value = true;
  try {
    const current = await globalApi.get();
    const payload = {
      ...current,
      ...form.value,
      cleanup_scope_tags: form.value.cleanup_scope_tags.join(","),
      cleanup_protect_tags: form.value.cleanup_protect_tags.join(","),
    };
    await globalApi.save(payload as unknown as GlobalSettings);
    ElMessage.success("保存成功");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

const workdirCategoryOptions = [
  { value: "logs", label: "日志文件", desc: "logs/ 目录下的历史日志" },
  { value: "staging", label: "暂存种子文件", desc: "临时下载的 .torrent 暂存文件" },
  { value: "backups", label: "旧配置备份", desc: "自动生成的历史配置备份" },
];

const workdirCategories = ref<string[]>(["logs", "staging", "backups"]);
const keepBackups = ref(5);
const cleanPreview = ref<CleanResult | null>(null);
const cleanResult = ref<CleanResult | null>(null);
/** 用户点过预览 / 清理之后这两块才出现；出现之后就一直负责交代结果，包括失败 */
const previewAsked = ref(false);
const cleanAsked = ref(false);

function categoryLabel(name: string): string {
  return workdirCategoryOptions.find((o) => o.value === name)?.label ?? name;
}

/**
 * 整类被拒绝 —— partial 态的来源。
 *
 * 后端只在整类拒绝清理时把 note 写成「类别 X 拒绝清理：…」（cleaner.go），
 * 其余 note（例如暂存清理的「使用默认保留期 24h」）只是说明，不算失败。
 * 这里按关键字识别：后端文案改了顶多不再显示「部分失败」提示，不会误判成错误。
 */
function isRejected(row: CleanCategoryResult): boolean {
  return (row.note || "").includes("拒绝");
}

function countRejected(res: CleanResult | null): number {
  if (!res) return 0;
  return res.categories.filter(isRejected).length;
}

const previewRows = computed<CleanCategoryResult[]>(() => cleanPreview.value?.categories ?? []);
const cleanRows = computed<CleanCategoryResult[]>(() => cleanResult.value?.categories ?? []);

const previewFailed = computed(() => countRejected(cleanPreview.value));
const cleanFailed = computed(() => countRejected(cleanResult.value));

/** 预览表：没有筛选条件（预览接口一律扫全部三类），所以 0 行就是 empty 而不是 zero */
const {
  loading: previewing,
  state: previewState,
  errorText: previewErrorText,
  run: runPreview,
  isStale: isPreviewStale,
  hasPartialBanner: previewPartial,
} = useDataState({ failed: () => previewFailed.value });

/** 清理结果表：只清勾选的那几类，所以 0 行要分「一个都没勾全」和「真的没东西可删」 */
const {
  loading: cleaning,
  state: cleanState,
  errorText: cleanErrorText,
  run: runClean,
  isStale: isCleanStale,
  hasPartialBanner: cleanPartial,
} = useDataState({
  filtered: () => workdirCategories.value.length < workdirCategoryOptions.length,
  failed: () => cleanFailed.value,
});

/* PtDataState 的默认文案是给「加载列表」写的；这两块一个是扫描一个是删文件，标题要改 */
const previewStateTitle = computed(() => {
  if (previewState.value === "loading") return "正在预览";
  if (previewState.value === "error") return "预览失败";
  if (previewState.value === "empty") return "没有可清理项";
  return "";
});

const previewStateSub = computed(() => {
  if (previewState.value === "error" || previewState.value === "perm") {
    return previewErrorText.value;
  }
  return "日志、暂存种子和旧配置备份目录里都没有可清理的内容";
});

const cleanStateTitle = computed(() => {
  if (cleanState.value === "loading") return "正在清理";
  if (cleanState.value === "error") return "清理失败";
  if (cleanState.value === "empty" || cleanState.value === "zero") return "没有删除任何文件";
  return "";
});

const cleanStateSub = computed(() => {
  if (cleanState.value === "error" || cleanState.value === "perm") return cleanErrorText.value;
  if (cleanState.value === "loading") return "正在删除文件，别关页面";
  if (cleanState.value === "zero") return "勾选的这几项下没有可清理的内容，换一组清理项再试";
  return "勾选的清理项下没有符合条件的文件";
});

/** 拒绝 > 有可删 > 无需清理；预览用 warn（还没删），结果用 ok（已删） */
function previewTone(row: CleanCategoryResult): Tone {
  if (isRejected(row)) return "dang";
  return row.deletedCount > 0 ? "warn" : "neutral";
}

function previewText(row: CleanCategoryResult): string {
  if (isRejected(row)) return "已拒绝";
  return row.deletedCount > 0 ? "可清理" : "无需清理";
}

function cleanTone(row: CleanCategoryResult): Tone {
  if (isRejected(row)) return "dang";
  return row.deletedCount > 0 ? "ok" : "neutral";
}

function cleanText(row: CleanCategoryResult): string {
  if (isRejected(row)) return "已拒绝";
  return row.deletedCount > 0 ? "已清理" : "未清理";
}

/** 行卡那条进度：可释放（已释放）占该目录当前已用的比例 */
function freedPercent(row: CleanCategoryResult): number {
  if (!row.dirUsedBytes || row.dirUsedBytes <= 0) return 0;
  return Math.min(100, Math.round((row.freedBytes / row.dirUsedBytes) * 100));
}

async function loadPreview() {
  previewAsked.value = true;
  // 先清空再请求：这两张表没有面板级 loading 遮罩，留着旧数据的话 loading 态根本看不见
  cleanPreview.value = null;
  const pending = runPreview(() => maintenanceApi.preview());
  const data = await pending;
  // 清理完成后的自动预览撞上手动预览：晚到的旧请求不弹「预览失败」
  if (isPreviewStale(pending)) return;
  if (!data) {
    ElMessage.error(previewErrorText.value || "预览失败");
    return;
  }
  cleanPreview.value = data;
}

async function previewClean() {
  cleanResult.value = null;
  cleanAsked.value = false;
  await loadPreview();
}

async function executeClean() {
  if (workdirCategories.value.length === 0) {
    ElMessage.warning("请至少选择一个清理项");
    return;
  }
  const selectedNames = workdirCategories.value.map(categoryLabel).join("、");
  try {
    await ElMessageBox.confirm(
      `即将清理以下内容：${selectedNames}。数据库、密钥等受保护项不会被删除。此操作不可撤销，确认继续？`,
      "确认清理工作目录",
      {
        confirmButtonText: "立即清理",
        cancelButtonText: "取消",
        type: "warning",
      },
    );
  } catch {
    return;
  }

  cleanAsked.value = true;
  cleanResult.value = null;
  const pending = runClean(() =>
    maintenanceApi.clean({
      categories: workdirCategories.value,
      dryRun: false,
      keepBackups: keepBackups.value,
    }),
  );
  const res = await pending;
  if (isCleanStale(pending)) return;
  if (!res) {
    // 失败时不留旧结果：上一次的「已删 12 项」配一块「清理失败」比什么都不显示更误导
    ElMessage.error(cleanErrorText.value || "清理失败");
    return;
  }
  cleanResult.value = res;
  ElMessage.success(`清理完成，共删除 ${res.totalDeleted} 项，释放 ${res.totalFreedHuman}`);
  await loadPreview();
}
</script>

<template>
  <!--
    画板 21（p-main 344,84 1080×882）与画板 36（p-workdir 1080×570）都是通栏卡：
    head 之后直接进卡片层，左右各内缩 16，卡片之间间隔 16。本页没有表格带。
    宽屏下卡宽停在 1080（.pt-cards--form），卡脚的「保存设置」不会跑到屏幕最右边。
  -->
  <div class="cleanup-page pt-cards pt-cards--wide pt-cards--form">
    <PtPanel v-loading="loading" title="自动删种" icon="trash-2" padding="none">
      <template #actions>
        <PtStatusPill :tone="form.cleanup_enabled ? 'ok' : 'neutral'" size="sm">
          {{ form.cleanup_enabled ? "运行中" : "未启用" }}
        </PtStatusPill>
      </template>

      <!--
        配置读失败时，表单会停在一整套默认值上 —— 长得和「服务端就是这么配的」一模一样。
        perm 不给重试：没权限点多少次都一样。
      -->
      <div
        v-if="settingsFailed"
        class="pt-note settings-alert"
        :class="settingsState === 'perm' ? 'pt-note--warn' : 'pt-note--dang'">
        <PtIcon
          :name="settingsState === 'perm' ? 'lock' : 'circle-alert'"
          :size="14"
          class="pt-note__icon" />
        <span class="settings-alert__text">{{ settingsAlertText }}</span>
        <el-button
          v-if="settingsState === 'error'"
          size="small"
          :loading="loading"
          @click="loadSettings">
          <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
        </el-button>
      </div>

      <el-form :model="form" label-position="top" class="pt-form settings-form">
        <div class="pt-strip">
          <PtIcon name="sliders-horizontal" :size="13" />
          <span>基本设置</span>
        </div>
        <div class="settings-body">
          <el-form-item label="启用自动删种">
            <el-switch v-model="form.cleanup_enabled" />
            <div class="field-tip">开启后按下面的规则定时清理符合条件的种子</div>
          </el-form-item>

          <template v-if="form.cleanup_enabled">
            <div class="field-row">
              <el-form-item label="检查间隔（分钟）">
                <el-input-number
                  v-model="form.cleanup_interval_min"
                  :min="5"
                  :max="1440"
                  :step="5" />
              </el-form-item>
              <el-form-item label="删除时连数据文件一起删">
                <el-switch v-model="form.cleanup_remove_data" />
                <div class="field-tip">关掉则只从下载器移除，磁盘上的文件保留</div>
              </el-form-item>
            </div>

            <el-form-item label="管理范围">
              <el-radio-group v-model="form.cleanup_scope" class="scope-list">
                <div class="scope" :class="{ 'is-on': form.cleanup_scope === 'database' }">
                  <el-radio value="database">仅本应用推送的种子（推荐）</el-radio>
                  <p class="scope__desc">
                    按数据库记录精确匹配，只管理 pt-tools 推送并记录在案的种子
                  </p>
                </div>

                <div class="scope" :class="{ 'is-on': form.cleanup_scope === 'tag' }">
                  <el-radio value="tag">按标签匹配</el-radio>
                  <p class="scope__desc">下载器里带这些标签的种子都会纳入管理</p>
                  <template v-if="form.cleanup_scope === 'tag'">
                    <div class="tagbox">
                      <el-tag
                        v-for="(tag, i) in form.cleanup_scope_tags"
                        :key="tag"
                        closable
                        size="small"
                        @close="removeTag(form.cleanup_scope_tags, i)">
                        {{ tag }}
                      </el-tag>
                      <el-input
                        v-model="scopeTagInput"
                        size="small"
                        class="taginput"
                        placeholder="输入标签后回车"
                        @keyup.enter="addScopeTag()" />
                    </div>
                    <div class="pt-note pt-note--warn scope__note">
                      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
                      <span>你自己手动打了同样标签的种子也会被一起管理。</span>
                    </div>
                  </template>
                </div>

                <div class="scope" :class="{ 'is-on': form.cleanup_scope === 'all' }">
                  <el-radio value="all">下载器中所有种子</el-radio>
                  <p class="scope__desc">不区分来源，下载器里的每一个种子都参与判断</p>
                  <div
                    v-if="form.cleanup_scope === 'all'"
                    class="pt-note pt-note--dang scope__note">
                    <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
                    <span>危险：非本应用添加的种子也会被删，可能误删重要数据。</span>
                  </div>
                </div>
              </el-radio-group>
            </el-form-item>
          </template>
        </div>

        <template v-if="form.cleanup_enabled">
          <div class="pt-strip">
            <PtIcon name="target" :size="13" />
            <span>删除条件</span>
          </div>
          <div class="settings-body">
            <el-form-item label="推荐方案">
              <el-select
                v-model="selectedPreset"
                placeholder="选一个预设，一键填好下面的参数"
                clearable
                style="width: 100%"
                @change="applyPreset">
                <el-option v-for="p in presets" :key="p.value" :label="p.label" :value="p.value">
                  <span class="opt">
                    <span>{{ p.label }}</span>
                    <span class="opt__desc">{{ p.desc }}</span>
                  </span>
                </el-option>
              </el-select>
              <div v-if="selectedPreset" class="field-tip field-tip--ok">
                已选「{{ presets.find((p) => p.value === selectedPreset)?.label }}」，改动下面任一
                参数后这个标记会自动清除
              </div>
              <div v-else class="field-tip">只填充表单，点保存才真正生效</div>
            </el-form-item>

            <el-form-item label="条件关系">
              <el-radio-group v-model="form.cleanup_condition_mode">
                <el-radio-button value="or">任一满足即删（OR）</el-radio-button>
                <el-radio-button value="and">全部满足才删（AND）</el-radio-button>
              </el-radio-group>
              <div v-if="form.cleanup_condition_mode === 'or'" class="field-tip">
                下面任意一个条件命中就删，低效做种也算一个条件
              </div>
              <div v-else class="field-tip">
                下面所有已配置的条件都要同时满足；填 0 或未启用的条件不参与判断
              </div>
            </el-form-item>

            <div class="field-row">
              <el-form-item label="做种时间超过（小时）">
                <el-input-number v-model="form.cleanup_max_seed_time_h" :min="0" :step="1" />
                <div class="field-tip">0 表示不限制</div>
              </el-form-item>
              <el-form-item label="分享率达到">
                <el-input-number
                  v-model="form.cleanup_min_ratio"
                  :min="0"
                  :step="0.1"
                  :precision="2" />
                <div class="field-tip">0 表示不限制</div>
              </el-form-item>
              <el-form-item label="不活跃超过（小时）">
                <el-input-number v-model="form.cleanup_max_inactive_h" :min="0" :step="1" />
                <div class="field-tip">0 表示不限制</div>
              </el-form-item>
            </div>

            <div class="field-row">
              <el-form-item label="低效做种：做种超过（小时）">
                <el-input-number v-model="form.cleanup_slow_seed_time_h" :min="0" :step="1" />
              </el-form-item>
              <el-form-item label="但分享率仍低于">
                <el-input-number
                  v-model="form.cleanup_slow_max_ratio"
                  :min="0"
                  :step="0.01"
                  :precision="2" />
              </el-form-item>
            </div>

            <div
              v-if="form.cleanup_slow_seed_time_h > 0 && form.cleanup_slow_max_ratio > 0"
              class="pt-note slow-note">
              <PtIcon name="info" :size="14" class="pt-note__icon" />
              <span>
                做种超过 {{ form.cleanup_slow_seed_time_h }} 小时、分享率仍低于
                {{ form.cleanup_slow_max_ratio }} 的已完成种子会被删除。
              </span>
            </div>
            <div v-else class="field-tip slow-note">
              低效做种要两个值都填才生效，任一为 0 则不启用这个条件
            </div>

            <el-form-item>
              <el-checkbox v-model="form.cleanup_del_free_expired">
                免费到期仍未下完的种子自动删除
              </el-checkbox>
              <div class="field-tip">
                只对本应用推送、且记录了免费结束时间的种子有效，与上面的管理范围无关
              </div>
            </el-form-item>
          </div>

          <div class="pt-strip">
            <PtIcon name="hard-drive" :size="13" />
            <span>磁盘空间保护</span>
          </div>
          <div class="settings-body">
            <el-form-item>
              <el-checkbox v-model="form.cleanup_disk_protect">启用磁盘空间保护</el-checkbox>
            </el-form-item>
            <el-form-item v-if="form.cleanup_disk_protect" label="最低剩余空间（GB）">
              <el-input-number v-model="form.cleanup_min_disk_space_gb" :min="1" :max="10000" />
              <div class="field-tip">
                低于这个值时：RSS
                停止推送新种子，同时按优先级强制删种腾空间（需开启「删除时连数据文件一起删」，只移除任务不会释放空间）
              </div>
            </el-form-item>
          </div>

          <div class="pt-strip">
            <PtIcon name="shield" :size="13" />
            <span>保护规则</span>
            <span class="pt-strip__end">命中任一条即不删</span>
          </div>
          <div class="settings-body">
            <el-form-item>
              <el-checkbox v-model="form.cleanup_protect_dl">保护下载中的种子</el-checkbox>
              <div class="field-tip">正在下载或校验中的种子不会被删</div>
            </el-form-item>

            <el-form-item>
              <el-checkbox v-model="form.cleanup_protect_hr">保护 H&amp;R 种子</el-checkbox>
              <div class="field-tip">
                没满足站点 H&amp;R 做种时长要求的不删；无 H&amp;R 的站点不受影响
              </div>
            </el-form-item>

            <div class="field-row">
              <el-form-item label="最短保种时间（小时）">
                <el-input-number v-model="form.cleanup_min_retain_h" :min="0" />
                <div class="field-tip">加进下载器还不够这么久的种子不删</div>
              </el-form-item>
              <el-form-item label="保护标签">
                <div class="tagbox">
                  <el-tag
                    v-for="(tag, i) in form.cleanup_protect_tags"
                    :key="tag"
                    closable
                    size="small"
                    @close="removeTag(form.cleanup_protect_tags, i)">
                    {{ tag }}
                  </el-tag>
                  <el-input
                    v-model="protectTagInput"
                    size="small"
                    class="taginput"
                    placeholder="输入标签后回车"
                    @keyup.enter="addProtectTag()" />
                </div>
                <div class="field-tip">带这些标签的种子一律不删</div>
              </el-form-item>
            </div>
          </div>
        </template>
      </el-form>

      <template #footer>
        <span class="pt-foot-note">删除动作不可撤销，改完记得保存</span>
        <el-button type="primary" :loading="saving" :disabled="saveLocked" @click="save">
          <PtIcon name="save" :size="14" /><span>保存设置</span>
        </el-button>
      </template>
    </PtPanel>

    <PtPanel title="做种竞争度监控" icon="users" padding="none">
      <template #actions>
        <el-switch v-model="form.peer_ratio_enabled" size="small" />
      </template>

      <el-form label-position="top" class="pt-form" :disabled="!form.peer_ratio_enabled">
        <div class="settings-body settings-body--lead">
          <div class="field-tip field-tip--lead">
            盯着做种中种子的 Seeder/Leecher 比值，竞争度过高时自动暂停或删除。
          </div>

          <div class="field-row">
            <el-form-item label="S/L 比值上限">
              <el-input-number v-model="form.peer_ratio_max_sl" :min="1" :step="5" :precision="1" />
              <div class="field-tip">比值超过这个数就触发，最小 1</div>
            </el-form-item>
            <el-form-item label="检查间隔（分钟）">
              <el-input-number v-model="form.peer_ratio_interval_min" :min="5" :step="5" />
              <div class="field-tip">从 Tracker 拿 Swarm 级别的做种/下载人数来判断</div>
            </el-form-item>
          </div>

          <el-form-item>
            <el-checkbox v-model="form.peer_ratio_remove_data">直接删除种子及数据</el-checkbox>
            <div class="field-tip">
              开启则超标种子连文件一起删；关闭只暂停，可以在「暂停任务」页手动恢复
            </div>
          </el-form-item>
        </div>
      </el-form>

      <template #footer>
        <span class="pt-foot-note">与自动删种共用同一个保存按钮的配置表</span>
        <el-button type="primary" :loading="saving" :disabled="saveLocked" @click="save">
          <PtIcon name="save" :size="14" /><span>保存设置</span>
        </el-button>
      </template>
    </PtPanel>

    <PtPanel title="清理工作目录" icon="folder" count=".pt-tools" padding="none">
      <div class="settings-body settings-body--lead">
        <div class="field-tip field-tip--lead">
          清掉日志、暂存种子和旧配置备份。数据库、密钥等受保护项永远不会被删。
        </div>

        <el-form label-position="top" class="pt-form">
          <el-form-item label="清理项">
            <el-checkbox-group v-model="workdirCategories" class="cat-group">
              <el-checkbox
                v-for="opt in workdirCategoryOptions"
                :key="opt.value"
                :value="opt.value">
                {{ opt.label }}
                <span class="cat__desc">{{ opt.desc }}</span>
              </el-checkbox>
            </el-checkbox-group>
          </el-form-item>

          <el-form-item label="保留最近备份数量">
            <el-input-number v-model="keepBackups" :min="0" :max="100" :step="1" />
            <!-- 后端 maintenance.Cleaner 把 keep<=0 当成默认 5 份（internal/maintenance/cleaner.go），不是「全清」 -->
            <div class="field-tip">清旧备份时留最近的 N 份，其余删除；填 0 按默认的 5 份保留</div>
          </el-form-item>
        </el-form>

        <div class="clean-acts">
          <el-button :loading="previewing" @click="previewClean">
            <PtIcon name="eye" :size="14" /><span>预览可清理项</span>
          </el-button>
          <el-button type="danger" :loading="cleaning" @click="executeClean">
            <PtIcon name="trash-2" :size="14" /><span>立即清理</span>
          </el-button>
        </div>
      </div>

      <template v-if="previewAsked">
        <div class="pt-strip">
          <PtIcon name="eye" :size="13" />
          <span>预览结果</span>
          <span v-if="cleanPreview" class="pt-strip__end">
            可删 {{ cleanPreview.totalDeleted }} 项 · 可释放 {{ cleanPreview.totalFreedHuman }}
          </span>
        </div>

        <!-- partial：有类别被整类拒绝，但拿到的行照样列出来，不用状态块顶掉表格 -->
        <div v-if="previewPartial(previewRows.length)" class="pt-note pt-note--warn table-note">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>{{ previewFailed }} 个清理项被整类拒绝（原因见「备注」），其余结果照常列出。</span>
        </div>

        <el-table v-if="!isMobile" :data="previewRows" class="pt-grid" style="width: 100%">
          <template #empty>
            <PtDataState
              :state="previewState"
              dense
              :title="previewStateTitle"
              :sub="previewStateSub">
              <template v-if="previewState === 'error'" #action>
                <el-button size="small" @click="loadPreview">
                  <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
                </el-button>
              </template>
            </PtDataState>
          </template>

          <el-table-column label="清理项" min-width="120" class-name="pt-cell-strong">
            <template #default="{ row }">{{ categoryLabel(row.name) }}</template>
          </el-table-column>
          <el-table-column
            prop="dirUsedHuman"
            label="当前已用"
            width="110"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column
            prop="deletedCount"
            label="可删除"
            width="90"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column
            prop="freedHuman"
            label="可释放"
            width="110"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column label="备注" min-width="160" class-name="pt-cell-muted">
            <template #default="{ row }">
              <PtStatusPill v-if="row.note" :tone="isRejected(row) ? 'dang' : 'warn'" size="sm">
                {{ row.note }}
              </PtStatusPill>
              <span v-else-if="row.skippedCount > 0">
                跳过 {{ row.skippedCount }} 项（受保护）
              </span>
              <span v-else>—</span>
            </template>
          </el-table-column>
        </el-table>

        <!--
          移动端行卡（§9）：这张表 5 列里有 3 列是数字，横着滚就没了列头，
          数字也就读不出是「已用」还是「可释放」。卡上给每个数字带上词。
        -->
        <div v-else class="cards">
          <PtDataState
            v-if="!previewRows.length"
            :state="previewState"
            :title="previewStateTitle"
            :sub="previewStateSub">
            <template v-if="previewState === 'error'" #action>
              <el-button size="small" @click="loadPreview">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>

          <PtRowCard v-for="row in previewRows" :key="row.name">
            <template #title>{{ categoryLabel(row.name) }}</template>

            <template #meta>
              <span>可删 {{ row.deletedCount }} 项</span>
              <span v-if="row.skippedCount > 0">跳过 {{ row.skippedCount }} 项（受保护）</span>
              <span
                v-if="row.note"
                class="note-line"
                :class="isRejected(row) ? 'is-dang' : 'is-warn'">
                {{ row.note }}
              </span>
            </template>

            <template #status>
              <PtStatusPill dot :tone="previewTone(row)" size="sm">{{
                previewText(row)
              }}</PtStatusPill>
            </template>

            <template v-if="row.dirUsedBytes > 0" #progress>
              <el-progress
                :percentage="freedPercent(row)"
                :stroke-width="4"
                :show-text="false"
                color="var(--pt-p)" />
              <span class="progress-info">
                <span>可释放 {{ row.freedHuman }} / 已用 {{ row.dirUsedHuman }}</span>
                <span class="progress-pct">{{ freedPercent(row) }}%</span>
              </span>
            </template>
          </PtRowCard>
        </div>

        <div v-if="previewRows.length" class="settings-body settings-body--tail">
          <div class="field-tip">这里只是预览，还没删任何文件；点「立即清理」并确认后才真删。</div>
        </div>
      </template>

      <template v-if="cleanAsked">
        <div class="pt-strip">
          <PtIcon name="circle-check" :size="13" />
          <span>清理结果</span>
          <span v-if="cleanResult" class="pt-strip__end">
            已删 {{ cleanResult.totalDeleted }} 项 · 释放 {{ cleanResult.totalFreedHuman }}
          </span>
        </div>

        <div v-if="cleanPartial(cleanRows.length)" class="pt-note pt-note--warn table-note">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>{{ cleanFailed }} 个清理项被整类拒绝（原因见「备注」），其余已按结果处理。</span>
        </div>

        <el-table v-if="!isMobile" :data="cleanRows" class="pt-grid" style="width: 100%">
          <template #empty>
            <PtDataState :state="cleanState" dense :title="cleanStateTitle" :sub="cleanStateSub">
              <!-- 重试走 executeClean，会重新弹确认框：删文件不能靠一个小按钮直接触发 -->
              <template v-if="cleanState === 'error'" #action>
                <el-button size="small" @click="executeClean">
                  <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
                </el-button>
              </template>
            </PtDataState>
          </template>

          <el-table-column label="清理项" min-width="120" class-name="pt-cell-strong">
            <template #default="{ row }">{{ categoryLabel(row.name) }}</template>
          </el-table-column>
          <el-table-column
            prop="dirUsedHuman"
            label="当前已用"
            width="110"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column
            prop="deletedCount"
            label="已删除"
            width="90"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column
            prop="freedHuman"
            label="释放空间"
            width="110"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column label="备注" min-width="160" class-name="pt-cell-muted">
            <template #default="{ row }">
              <PtStatusPill v-if="row.note" :tone="isRejected(row) ? 'dang' : 'warn'" size="sm">
                {{ row.note }}
              </PtStatusPill>
              <span v-else-if="row.skippedCount > 0">
                跳过 {{ row.skippedCount }} 项（受保护）
              </span>
              <span v-else>—</span>
            </template>
          </el-table-column>
        </el-table>

        <div v-else class="cards">
          <PtDataState
            v-if="!cleanRows.length"
            :state="cleanState"
            :title="cleanStateTitle"
            :sub="cleanStateSub">
            <template v-if="cleanState === 'error'" #action>
              <el-button size="small" @click="executeClean">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>

          <PtRowCard v-for="row in cleanRows" :key="row.name">
            <template #title>{{ categoryLabel(row.name) }}</template>

            <template #meta>
              <span>已删 {{ row.deletedCount }} 项</span>
              <span v-if="row.skippedCount > 0">跳过 {{ row.skippedCount }} 项（受保护）</span>
              <span
                v-if="row.note"
                class="note-line"
                :class="isRejected(row) ? 'is-dang' : 'is-warn'">
                {{ row.note }}
              </span>
            </template>

            <template #status>
              <PtStatusPill dot :tone="cleanTone(row)" size="sm">{{ cleanText(row) }}</PtStatusPill>
            </template>

            <template v-if="row.dirUsedBytes > 0" #progress>
              <el-progress
                :percentage="freedPercent(row)"
                :stroke-width="4"
                :show-text="false"
                color="var(--pt-ok)" />
              <span class="progress-info">
                <span>释放 {{ row.freedHuman }} / 清理前已用 {{ row.dirUsedHuman }}</span>
                <span class="progress-pct">{{ freedPercent(row) }}%</span>
              </span>
            </template>
          </PtRowCard>
        </div>
      </template>
    </PtPanel>
  </div>
</template>

<style scoped>
/*
 * 版面交给 .pt-cards --wide（间隔与内缩都在那里）。原来这里是 max-width 880 的 flex 列，
 * 画板上这几张卡是 1080 通栏，880 会在右边留出 200 的空白。
 */

/* 第一条区块条紧贴面板页头，两条发丝线会叠成 2px */
.settings-form > .pt-strip:first-child {
  border-top: 0;
}

.settings-body {
  padding: var(--pt-pad) var(--pt-pad) 0;
}

/* 面板正文直接以字段开头（没有区块条）时，下沿也要自己收口 */
.settings-body--lead {
  padding-bottom: var(--pt-space-2);
}

.settings-body--tail {
  padding: var(--pt-space-3) var(--pt-pad);
}

.field-tip--lead {
  margin-top: 0;
  margin-bottom: var(--pt-space-4);
}

.field-tip--ok {
  color: var(--pt-ok);
}

/*
 * 管理范围的三选一：每个选项是一整块可点区域，说明和风险提示都跟在选项里。
 * 选中项描边走 primary —— 「按标签」和「所有种子」的风险差得远，
 * 光看一枚小圆点读不出当前选的是哪种范围。
 */
.scope-list {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  width: 100%;
}

.scope {
  display: block;
  padding: var(--pt-space-3);
  background: var(--pt-canvas);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.scope.is-on {
  background: var(--pt-p-soft);
  border-color: var(--pt-p);
}

.scope__desc {
  margin: 2px 0 0 22px;
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-body);
  color: var(--pt-t3);
}

.scope__note {
  margin-top: var(--pt-space-2);
}

/* 标签胶囊和输入框同一行排布，输入框窄一点，不抢走整行 */
.tagbox {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  width: 100%;
}

.taginput {
  width: 150px;
}

.slow-note {
  margin-bottom: var(--pt-space-4);
}

/* 下拉项里的补充说明：主标签之后跟一段更淡的小字 */
.opt {
  display: flex;
  gap: var(--pt-space-2);
  align-items: baseline;
}

.opt__desc {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.cat-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.cat__desc {
  margin-left: 6px;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.clean-acts {
  display: flex;
  gap: var(--pt-space-2);
  padding-bottom: var(--pt-space-2);
}

/* 手机上这两个是本页的主操作，按 §9 的 44 触控高度等分一行 */
@media (max-width: 768px) {
  .clean-acts .el-button {
    flex: 1;
    min-height: var(--pt-m-touch);
    margin: 0;
  }
}

/* 配置读失败的提示：面板 padding="none"，左右留白自己给 */
.settings-alert {
  align-items: center;
  margin: var(--pt-space-3) var(--pt-pad);
}

.settings-alert__text {
  flex: 1;
  min-width: 0;
}

/* 表格/行卡上方的「部分失败」提示 */
.table-note {
  margin: var(--pt-space-3) var(--pt-pad) 0;
}

/* 移动端行卡列表：面板 padding="none"，所以留白由这里给 */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-pad) 0;
}

/* 备注独占一行：整类拒绝的原因带着路径，挤在 meta 的数字之间根本读不出来 */
.note-line {
  flex-basis: 100%;
  line-height: var(--pt-lh-body);
}

.note-line.is-warn {
  color: var(--pt-warn);
}

.note-line.is-dang {
  color: var(--pt-dang);
}

/* 进度条 + 一行说明，和任务列表的行卡保持同一形态 */
.progress-info {
  display: flex;
  gap: var(--pt-space-2);
  justify-content: space-between;
  font-size: var(--pt-fz-foot);
  color: var(--pt-t3);
}

.progress-pct {
  flex: 0 0 auto;
  font-variant-numeric: tabular-nums;
}
</style>
