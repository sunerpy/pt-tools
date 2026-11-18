<script setup lang="ts">
/*
 * 「自动签到」页的设置：签到时间窗、AI 签到（看图模型与「测试看图」）、签到结果推送。
 * 以前在「全局设置 → 每日签到」里，挪到签到页以后这些设置和每个站点的开关放在一处。
 * 走签到自己的接口（/api/sites/attendance/settings），读不回来时整段禁用，免得用默认值覆盖服务端的设置。
 */
import {
  assistantApi,
  type AttendanceSettings,
  attendanceApi,
  chatopsApi,
  type NotificationConfig,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref } from "vue";

const emit = defineEmits<{ saved: [settings: AttendanceSettings] }>();

const form = ref<
  Required<Omit<AttendanceSettings, "assistant_model">> & { assistant_model?: string }
>({
  window_start: "08:00",
  window_end: "10:00",
  ai_enabled: false,
  vision_model: "",
  assistant_ready: false,
  notify_enabled: true,
  notify_channel_ids: [],
});
const saved = ref<string>("");
const loadFailed = ref(false);
const loading = ref(true);
const saving = ref(false);
const channels = ref<NotificationConfig[]>([]);

/** 读回来的设置补上默认值（旧版后端不回 AI 和推送这几项） */
function normalize(a: AttendanceSettings) {
  return {
    window_start: a.window_start,
    window_end: a.window_end,
    ai_enabled: Boolean(a.ai_enabled),
    vision_model: a.vision_model ?? "",
    assistant_ready: Boolean(a.assistant_ready),
    assistant_model: a.assistant_model,
    notify_enabled: a.notify_enabled ?? true,
    notify_channel_ids: [...(a.notify_channel_ids ?? [])],
  };
}

/** 比较用的指纹：只看会保存的几项 */
function fingerprint(f: typeof form.value): string {
  return JSON.stringify([
    f.window_start,
    f.window_end,
    f.ai_enabled,
    f.vision_model.trim(),
    f.notify_enabled,
    [...f.notify_channel_ids].sort((a, b) => a - b),
  ]);
}

const dirty = computed(() => !loadFailed.value && fingerprint(form.value) !== saved.value);

async function load() {
  loading.value = true;
  try {
    const [att, list] = await Promise.all([
      attendanceApi.getSettings(),
      chatopsApi.notifications.list().catch(() => [] as NotificationConfig[]),
    ]);
    form.value = normalize(att);
    saved.value = fingerprint(form.value);
    channels.value = (list ?? []).filter((c) => c.enabled);
    loadFailed.value = false;
  } catch {
    loadFailed.value = true;
  } finally {
    loading.value = false;
  }
}

async function save() {
  const f = form.value;
  if (!(f.window_start < f.window_end)) {
    ElMessage.error("签到时间窗的开始时间必须早于结束时间");
    return;
  }
  saving.value = true;
  try {
    const res = await attendanceApi.saveSettings({
      window_start: f.window_start,
      window_end: f.window_end,
      ai_enabled: f.ai_enabled,
      vision_model: f.vision_model.trim(),
      notify_enabled: f.notify_enabled,
      notify_channel_ids: [...f.notify_channel_ids],
    });
    // AI 助手是否配好、用哪个模型是只读的两项，回包里没有时沿用读到的
    form.value = {
      ...normalize(res),
      assistant_ready: res.assistant_ready ?? f.assistant_ready,
      assistant_model: res.assistant_model ?? f.assistant_model,
    };
    saved.value = fingerprint(form.value);
    ElMessage.success("签到设置已保存");
    emit("saved", res);
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

/**
 * 「识别验证码用的模型」下拉框：第一次打开时按 AI 助手存着的设置读一次服务商的模型列表
 * （不带 Key：接口地址没变时后端用存着的 Key）。读不到也能手输模型名。
 */
const visionModels = ref<string[]>([]);
const visionModelsLoading = ref(false);
const visionModelsError = ref("");
let visionModelsLoaded = false;

async function loadVisionModels(open: boolean) {
  if (!open || visionModelsLoaded || visionModelsLoading.value) return;
  visionModelsLoading.value = true;
  visionModelsError.value = "";
  try {
    const st = await assistantApi.settings();
    const res = await assistantApi.listModels({
      enabled: st.enabled,
      provider: st.provider,
      base_url: st.base_url,
      model: st.model,
      chatops: st.chatops,
      max_steps: st.max_steps,
      confirm_minutes: st.confirm_minutes,
    });
    visionModels.value = res.models;
    visionModelsLoaded = true;
  } catch (e: unknown) {
    visionModelsError.value = (e as Error).message || "读不到模型列表";
  } finally {
    visionModelsLoading.value = false;
  }
}

const visionPlaceholder = computed(() =>
  form.value.assistant_model ? `与 AI 助手相同（${form.value.assistant_model}）` : "与 AI 助手相同",
);

/** 「测试看图」：现画一张验证码让选中的模型认（还没保存的模型名也能试） */
const visionTesting = ref(false);
const visionResult = ref<{ ok: boolean; text: string } | null>(null);

async function testVision() {
  visionTesting.value = true;
  visionResult.value = null;
  try {
    const r = await attendanceApi.testVision(form.value.vision_model.trim());
    const took = `用了 ${(r.latency_ms / 1000).toFixed(1)} 秒`;
    if (r.ok) {
      visionResult.value = { ok: true, text: `${r.model} 认对了：${r.got}（${took}）` };
    } else if (r.got && r.mismatch <= 2) {
      // 差一两个字：模型能看图，只是这一张认混了；签到时认错会换一张再认
      visionResult.value = {
        ok: false,
        text: `${r.model} 认成了「${r.got}」，正确是「${r.expected}」，差 ${r.mismatch} 个字（${took}）。能看图，多试几次看看；一直认不对就换一个模型`,
      };
    } else {
      visionResult.value = {
        ok: false,
        text: `${r.model} 认成了「${r.got || "没认出来"}」，正确是「${r.expected}」：这个模型可能不能看图，换一个再试`,
      };
    }
  } catch (e: unknown) {
    visionResult.value = { ok: false, text: (e as Error).message || "测试失败" };
  } finally {
    visionTesting.value = false;
  }
}

onMounted(load);

defineExpose({ reload: load });
</script>

<template>
  <el-form
    v-loading="loading"
    class="pt-form"
    label-position="top"
    :data-state="loading ? 'loading' : loadFailed ? 'error' : 'ready'"
    data-testid="attendance-settings"
    @submit.prevent>
    <div v-if="loadFailed" class="pt-note pt-note--warn" data-testid="attendance-load-failed">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>签到设置读取失败，暂时不能修改；刷新页面重试</span>
    </div>

    <div class="field-head">签到时间</div>
    <div class="field-row">
      <el-form-item label="时间窗开始">
        <el-time-select
          v-model="form.window_start"
          :disabled="loadFailed"
          start="00:00"
          step="00:15"
          end="23:45"
          :clearable="false"
          data-testid="attendance-window-start" />
      </el-form-item>
      <el-form-item label="时间窗结束">
        <el-time-select
          v-model="form.window_end"
          :disabled="loadFailed"
          start="00:15"
          step="00:15"
          end="23:45"
          :clearable="false"
          data-testid="attendance-window-end" />
      </el-form-item>
    </div>
    <div class="field-tip att-tip">
      开启了自动签到的站点，每天在这个时间段里随机选一个时刻签到（服务器时区）；失败过 10、30
      分钟各重试一次
    </div>

    <div class="field-head">用 AI 签到</div>
    <el-form-item label="交给 AI 过验证码和答题">
      <el-switch
        v-model="form.ai_enabled"
        :disabled="loadFailed"
        data-testid="attendance-ai-enabled" />
      <div class="field-tip">
        HDSky、OpenCD 签到要识别验证码，CHDBits、52PT 签到要答题：打开后交给 AI
        助手的模型做。验证码图片和题目会发给「AI 助手设置」里的服务商
      </div>
    </el-form-item>
    <el-form-item label="识别验证码用的模型">
      <div class="vision-pick">
        <el-select
          v-model="form.vision_model"
          filterable
          allow-create
          default-first-option
          clearable
          :disabled="loadFailed"
          :loading="visionModelsLoading"
          :placeholder="visionPlaceholder"
          data-testid="attendance-vision-model"
          @visible-change="loadVisionModels">
          <el-option v-for="m in visionModels" :key="m" :label="m" :value="m" />
        </el-select>
        <el-button
          :loading="visionTesting"
          :disabled="loadFailed || !form.assistant_ready"
          data-testid="attendance-vision-test"
          @click="testVision">
          测试看图
        </el-button>
      </div>
      <div class="field-tip">
        要能看图的模型，例如 Gemini、gpt-4o-mini、Llama 4；DeepSeek 不能看图。答题用 AI
        助手的模型<template v-if="visionModelsError"
          >（模型列表：{{ visionModelsError }}）</template
        >
      </div>
      <div
        v-if="visionResult"
        class="field-tip"
        :class="visionResult.ok ? 'vision-ok' : 'vision-bad'"
        data-testid="attendance-vision-result">
        {{ visionResult.text }}
      </div>
    </el-form-item>
    <div
      v-if="form.ai_enabled && !form.assistant_ready"
      class="pt-note pt-note--warn att-ai-note"
      data-testid="attendance-ai-not-ready">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>
        AI 助手还没有启用或没有配好：先在
        <router-link to="/assistant/settings">AI 助手设置</router-link>
        里填好接口地址、API Key 和模型并启用，否则这些站点签到会失败
      </span>
    </div>

    <div class="field-head">签到结果推送</div>
    <el-form-item label="推送签到结果">
      <el-switch
        v-model="form.notify_enabled"
        :disabled="loadFailed"
        data-testid="attendance-notify-enabled" />
      <div class="field-tip">
        每天开启了自动签到的站点都有结果以后，推一条汇总（哪些签上了、哪些失败及原因）；每次签到的结果也都记在下面的签到记录里
      </div>
    </el-form-item>
    <el-form-item label="推送到">
      <el-select
        v-model="form.notify_channel_ids"
        :disabled="loadFailed || !form.notify_enabled"
        multiple
        collapse-tags
        clearable
        placeholder="所有启用的通知通道"
        data-testid="attendance-notify-channels">
        <el-option v-for="c in channels" :key="c.id" :label="c.name" :value="c.id" />
      </el-select>
      <div class="field-tip">
        不选就推到所有启用的通道；没有通道时先去
        <router-link to="/chatops/notifications">消息通知</router-link> 里添加
      </div>
    </el-form-item>

    <div class="att-actions">
      <el-button
        type="primary"
        :loading="saving"
        :disabled="loadFailed || loading || !dirty"
        data-testid="attendance-settings-save"
        @click="save">
        <PtIcon v-if="!saving" name="save" :size="14" /><span>保存签到设置</span>
      </el-button>
      <span v-if="!dirty && !loadFailed && !loading" class="att-saved">已保存</span>
    </div>
  </el-form>
</template>

<style scoped>
/* 时间窗那两格下面共用一句说明：贴着字段行，和下一组拉开 */
.att-tip {
  margin: -8px 0 var(--pt-space-4);
}

/* 识别验证码用的模型：下拉框占满，右边是「测试看图」 */
.vision-pick {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  width: 100%;
}

.vision-pick > .el-select {
  flex: 1;
  min-width: 0;
}

.vision-ok {
  color: var(--pt-ok);
}

.vision-bad {
  color: var(--pt-warn);
}

/* AI 助手没配好的提醒：跟在 AI 两项下面，留出与下一组的距离 */
.att-ai-note {
  margin-bottom: var(--pt-space-4);
}

.att-actions {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
}

.att-saved {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}
</style>
