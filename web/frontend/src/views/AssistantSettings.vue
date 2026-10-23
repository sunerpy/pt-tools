<script setup lang="ts">
/*
 * AI 助手设置（路线图 M19）：服务商、接口地址、模型、API Key（只写不读）、代理，ChatOps 开关与几项上限。
 * 任何 OpenAI 兼容的接口都能接；选了预设会填好地址与常用模型，模型也可以手填，或者「获取模型列表」从服务商读
 * （M20：参照 Voltip，预设写明免费额度、申请 Key 的页面、隐私与要不要代理）。
 * 换了接口地址又没填新 Key 时，后端会清掉存着的 Key（Key 不会发到保存它时以外的地址），这里提前写明。
 * 语音输入（Voltip）：填了 Voltip 本机服务的地址与访问令牌，对话页才有麦克风；令牌同 API Key，只写不读、换地址就清掉。
 * 画板没有这一页，沿用 qB 兼容入口页的形状：左边设置、右边说明。
 */
import {
  assistantApi,
  type AssistantSettings,
  type AssistantSettingsInput,
  type AssistantTestResult,
  type AssistantVoiceTest,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { ASSISTANT_PROVIDERS, providerOf } from "@/utils/assistant";
import { voiceUnavailable } from "@/utils/voiceRecorder";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref, watch } from "vue";

const ds = useDataState();
const saved = ref<AssistantSettings | null>(null);
const form = ref({
  enabled: false,
  provider: "deepseek",
  base_url: "",
  model: "",
  proxy_url: "",
  chatops: false,
  max_steps: 8,
  confirm_minutes: 10,
  retention_days: 30,
  context_window: 0,
  auto_compact: true,
  compact_percent: 80,
  voice_base_url: "",
  voice_model: "",
  voice_language: "zh",
});
/** 新填的 API Key；空串表示不改 */
const newKey = ref("");
/** 点了「清除」：保存时把 Key 清掉 */
const clearKey = ref(false);
const saving = ref(false);
const testing = ref(false);
const tested = ref<{ ok: true; res: AssistantTestResult } | { ok: false; msg: string } | null>(
  null,
);
const listing = ref(false);
/** 从服务商读到的模型（换服务商时清空） */
const fetched = ref<string[]>([]);
/** 每发一次「获取模型列表」或换服务商加一：晚到的回应对不上就丢掉 */
let listSeq = 0;
const listed = ref<{ ok: true; total: number; count: number } | { ok: false; msg: string } | null>(
  null,
);
/** 新填的 Voltip 访问令牌；空串表示不改 */
const newVoiceToken = ref("");
/** 点了「清除」：保存时把令牌清掉 */
const clearVoiceToken = ref(false);
const voiceTesting = ref(false);
const voiceTested = ref<{ ok: true; res: AssistantVoiceTest } | { ok: false; msg: string } | null>(
  null,
);
/** 测试时 Voltip 列出的处理方式（换地址时清空） */
const voiceModels = ref<AssistantVoiceTest["models"]>([]);
/** 浏览器给不了麦克风的原因（不是 HTTPS 时） */
const micBlocked = voiceUnavailable();
const VOICE_LANGUAGES = [
  { value: "zh", label: "中文（zh）" },
  { value: "en", label: "英语（en）" },
  { value: "auto", label: "自动判断（auto）" },
];

const provider = computed(() => providerOf(form.value.provider));
/** 存着 Key，而且没有点「清除」 */
const hasKey = computed(() => !!saved.value?.has_api_key && !clearKey.value);
/** 预设的常用模型在前，读到的在后，去重 */
const modelOptions = computed(() => [...new Set([...provider.value.models, ...fetched.value])]);
/** 换了接口地址又没填新 Key：保存时存着的 Key 会被清掉 */
const keyWillDrop = computed(
  () =>
    !!saved.value?.has_api_key &&
    !newKey.value.trim() &&
    !clearKey.value &&
    form.value.base_url.trim().replace(/\/+$/, "") !== saved.value.base_url,
);
/** 存着 Voltip 的令牌，而且没有点「清除」 */
const hasVoiceToken = computed(() => !!saved.value?.has_voice_token && !clearVoiceToken.value);
/** 换了 Voltip 的地址又没填新令牌：保存时存着的令牌会被清掉 */
const voiceTokenWillDrop = computed(
  () =>
    !!saved.value?.has_voice_token &&
    !newVoiceToken.value.trim() &&
    !clearVoiceToken.value &&
    form.value.voice_base_url.trim().replace(/\/+$/, "") !== saved.value.voice_base_url,
);
/** 模型下拉框：Voltip 列出的处理方式，加上已经填的 */
const voiceModelOptions = computed(() => {
  const list = [...voiceModels.value];
  // 下拉框点了清除时值是 undefined
  const cur = (form.value.voice_model ?? "").trim();
  if (cur && !list.some((m) => m.id === cur)) list.unshift({ id: cur, name: cur });
  return list;
});

function fill(s: AssistantSettings) {
  saved.value = s;
  form.value = {
    enabled: s.enabled,
    provider: s.provider || (s.base_url ? "custom" : "deepseek"),
    base_url: s.base_url,
    model: s.model,
    proxy_url: s.proxy_url,
    chatops: s.chatops,
    max_steps: s.max_steps,
    confirm_minutes: s.confirm_minutes,
    retention_days: s.retention_days,
    context_window: s.context_window ?? 0,
    auto_compact: s.auto_compact ?? true,
    compact_percent: s.compact_percent || 80,
    voice_base_url: s.voice_base_url ?? "",
    voice_model: s.voice_model ?? "",
    // 还没配过语音输入：语言先填中文（不填时 Voltip 按它自己的设置或场景）
    voice_language: s.voice_language || (s.voice_base_url ? "" : "zh"),
  };
  if (!s.base_url) applyProvider(form.value.provider);
  newKey.value = "";
  clearKey.value = false;
  newVoiceToken.value = "";
  clearVoiceToken.value = false;
}

async function load() {
  const pending = ds.run(() => assistantApi.settings());
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  fill(got);
}

/** 选了预设：填地址与第一个常用模型（「其他」不动已经填的） */
function applyProvider(key: string) {
  const p = providerOf(key);
  listSeq++;
  listing.value = false;
  fetched.value = [];
  listed.value = null;
  if (p.key === "custom") return;
  form.value.base_url = p.baseURL;
  form.value.model = p.models[0] ?? "";
}

/** 要提交的设置。测试大模型、读模型列表时不带语音输入的几项（语音那几项填错了不该挡住它们） */
function input(withVoice = true): AssistantSettingsInput {
  const { voice_base_url, voice_model, voice_language, ...rest } = form.value;
  const out: AssistantSettingsInput = {
    ...rest,
    base_url: form.value.base_url.trim(),
    model: form.value.model.trim(),
  };
  if (newKey.value.trim()) out.api_key = newKey.value.trim();
  else if (clearKey.value) out.api_key = "";
  if (withVoice) {
    out.voice_base_url = voice_base_url.trim();
    out.voice_model = (voice_model ?? "").trim();
    out.voice_language = voice_language ?? "";
    if (newVoiceToken.value.trim()) out.voice_token = newVoiceToken.value.trim();
    else if (clearVoiceToken.value) out.voice_token = "";
  }
  return out;
}

function errText(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback;
}

async function save() {
  saving.value = true;
  try {
    fill(await assistantApi.saveSettings(input()));
    ElMessage.success("已保存");
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}

async function test() {
  testing.value = true;
  tested.value = null;
  try {
    tested.value = { ok: true, res: await assistantApi.testSettings(input(false)) };
  } catch (e) {
    tested.value = { ok: false, msg: errText(e, "连接失败") };
  } finally {
    testing.value = false;
  }
}

// 改了接口地址：读到的列表是旧地址的，丢掉；还没回来的也作废
watch(
  () => form.value.base_url,
  () => {
    listSeq++;
    listing.value = false;
    fetched.value = [];
    listed.value = null;
  },
);

async function listModels() {
  const seq = ++listSeq;
  listing.value = true;
  listed.value = null;
  try {
    const res = await assistantApi.listModels(input(false));
    if (seq !== listSeq) return;
    fetched.value = res.models;
    listed.value = { ok: true, total: res.total, count: res.models.length };
  } catch (e) {
    if (seq !== listSeq) return;
    listed.value = { ok: false, msg: errText(e, "读不到模型列表") };
  } finally {
    if (seq === listSeq) listing.value = false;
  }
}

async function testVoice() {
  voiceTesting.value = true;
  voiceTested.value = null;
  try {
    const res = await assistantApi.voiceTest(input());
    voiceModels.value = res.models;
    voiceTested.value = { ok: true, res };
  } catch (e) {
    voiceTested.value = { ok: false, msg: errText(e, "连不上 Voltip") };
  } finally {
    voiceTesting.value = false;
  }
}

// 改了 Voltip 的地址：测出来的处理方式是旧地址的，丢掉
watch(
  () => form.value.voice_base_url,
  () => {
    voiceModels.value = [];
    voiceTested.value = null;
  },
);

onMounted(load);
</script>

<template>
  <div class="pt-cards pt-cards--main">
    <PtHeadSub
      >接一个 OpenAI 兼容的大模型，让 AI 助手在网页、App 与通知渠道里回答问题、帮你操作</PtHeadSub
    >

    <PtDataState
      v-if="!saved"
      class="pt-cards__full"
      :state="ds.state.value"
      :sub="ds.errorText.value"
      data-testid="ass-state" />

    <template v-else>
      <!-- 这张表单比一屏长（服务商、上限、压缩、语音……）：保存与测试贴在可视区底部 -->
      <PtPanel title="设置" icon="sparkles" sticky-foot data-testid="ass-settings">
        <el-form class="pt-form" label-position="top" @submit.prevent>
          <el-form-item label="启用 AI 助手">
            <el-switch v-model="form.enabled" data-testid="ass-enabled" />
          </el-form-item>
          <el-form-item label="服务商">
            <el-select v-model="form.provider" data-testid="ass-provider" @change="applyProvider">
              <el-option
                v-for="p in ASSISTANT_PROVIDERS"
                :key="p.key"
                :value="p.key"
                :label="p.label" />
            </el-select>
            <div v-if="provider.hint" class="ass-tip">{{ provider.hint }}</div>
            <div
              v-if="provider.free || provider.privacy || provider.needsProxy || provider.consoleURL"
              class="ass-provider"
              data-testid="ass-provider-info">
              <div v-if="provider.free" class="ass-tip ass-tip--row" data-testid="ass-free">
                <PtStatusPill tone="ok" size="sm">免费额度</PtStatusPill>
                <span>{{ provider.free }}额度会调整，以服务商的页面为准。</span>
              </div>
              <div v-if="provider.privacy" class="ass-tip" data-testid="ass-privacy">
                {{ provider.privacy }}
              </div>
              <div
                v-if="provider.needsProxy"
                class="ass-tip ass-tip--warn"
                data-testid="ass-proxy-hint">
                从中国大陆访问一般要代理：在下面的「代理」里填一个能访问它的地址。
              </div>
              <a
                v-if="provider.consoleURL"
                class="ass-link"
                :href="provider.consoleURL"
                target="_blank"
                rel="noopener noreferrer"
                data-testid="ass-console"
                >去申请 API Key <PtIcon name="external-link" :size="12"
              /></a>
            </div>
          </el-form-item>
          <el-form-item label="接口地址">
            <el-input
              v-model="form.base_url"
              placeholder="https://api.example.com/v1（到 /chat/completions 之前）"
              data-testid="ass-base-url" />
          </el-form-item>
          <el-form-item label="模型">
            <div class="ass-modelrow">
              <el-select
                v-model="form.model"
                class="ass-modelrow__select"
                filterable
                allow-create
                default-first-option
                placeholder="选一个，或者直接输入模型名"
                data-testid="ass-model">
                <el-option v-for="m in modelOptions" :key="m" :value="m" :label="m" />
              </el-select>
              <el-button :loading="listing" data-testid="ass-list-models" @click="listModels">
                <PtIcon name="cloud-download" :size="14" /><span>获取模型列表</span>
              </el-button>
            </div>
            <div
              v-if="listed"
              class="ass-tip"
              :class="{ 'ass-tip--warn': !listed.ok }"
              data-testid="ass-listed">
              <template v-if="listed.ok"
                >服务商列出 {{ listed.total }} 个模型，去掉嵌入、语音、图像这类以后
                {{ listed.count }}
                个放进了下拉框。按名字筛的，不保证每个都能用：要支持工具调用（function
                calling），选好以后点「测试连接」看看。</template
              >
              <template v-else>{{ listed.msg }}</template>
            </div>
          </el-form-item>
          <el-form-item label="API Key">
            <el-input
              v-model="newKey"
              type="password"
              show-password
              autocomplete="off"
              :placeholder="
                hasKey
                  ? '已保存（不显示）；要换就填新的'
                  : provider.noKey
                    ? '不需要的话留空'
                    : '填服务商给的 API Key'
              "
              data-testid="ass-key" />
            <div class="ass-keyrow">
              <PtStatusPill :tone="hasKey ? 'ok' : 'neutral'" size="sm">{{
                hasKey ? "已保存 Key" : "没有 Key"
              }}</PtStatusPill>
              <el-button
                v-if="hasKey"
                link
                type="danger"
                data-testid="ass-key-clear"
                @click="clearKey = true"
                >清除</el-button
              >
              <el-button v-if="clearKey" link data-testid="ass-key-keep" @click="clearKey = false"
                >不清除</el-button
              >
            </div>
            <div v-if="keyWillDrop" class="ass-tip ass-tip--warn" data-testid="ass-key-drop">
              换了接口地址：保存时会清掉现在的 API Key，要用 Key 就重新填。
            </div>
          </el-form-item>
          <el-form-item label="代理">
            <el-input
              v-model="form.proxy_url"
              placeholder="http://127.0.0.1:7890 或 socks5://…，留空用环境变量里的代理"
              data-testid="ass-proxy" />
          </el-form-item>
          <h3 class="ass-sub">入口与上限</h3>
          <el-form-item label="通知渠道（ChatOps）">
            <el-switch v-model="form.chatops" data-testid="ass-chatops" />
            <div class="ass-tip">
              打开以后，绑定过的通知渠道里不是命令的消息交给助手回答；只有管理员绑定能做改动类操作。
            </div>
          </el-form-item>
          <el-form-item label="确认等待（分钟）">
            <el-input-number
              v-model="form.confirm_minutes"
              :min="1"
              :max="60"
              data-testid="ass-confirm" />
            <div class="ass-tip">改动类操作等你确认的时间，过了就不执行。</div>
          </el-form-item>
          <el-form-item label="每次回答最多调用几轮工具">
            <el-input-number v-model="form.max_steps" :min="1" :max="20" data-testid="ass-steps" />
          </el-form-item>
          <el-form-item label="会话保留（天）">
            <el-input-number
              v-model="form.retention_days"
              :min="0"
              :max="3650"
              data-testid="ass-retention" />
            <div class="ass-tip">更新时间早于这么多天的会话每天清理一次；0 表示不清理。</div>
          </el-form-item>
          <h3 class="ass-sub">上下文与压缩</h3>
          <el-form-item label="上下文窗口（token）">
            <el-input-number
              v-model="form.context_window"
              :min="0"
              :max="10000000"
              :step="8192"
              :step-strictly="false"
              data-testid="ass-window" />
            <div class="ass-tip">
              0 表示自动：按服务商模型列表报的长度，没报的按模型名估计，最多按 128K
              算（窗口很大的模型填满再压缩太费 token）。填小一点会更早压缩、每次请求更省。
            </div>
          </el-form-item>
          <el-form-item label="自动压缩">
            <el-switch v-model="form.auto_compact" data-testid="ass-compact" />
            <div class="ass-tip">
              上下文用到窗口的这个比例时，开始回答以前先让模型把前面的对话写成摘要；对话页上照旧能看到全部消息。关掉以后太长时从最早的消息丢起。
            </div>
          </el-form-item>
          <el-form-item v-if="form.auto_compact" label="压缩阈值（%）">
            <el-input-number
              v-model="form.compact_percent"
              :min="50"
              :max="95"
              :step="5"
              data-testid="ass-compact-percent" />
          </el-form-item>
          <h3 class="ass-sub">语音输入（Voltip）</h3>
          <div class="ass-tip ass-tip--lead">
            填了 Voltip
            本机服务的地址与访问令牌，对话页的输入框旁边会多一个麦克风：说完一段，文字填进输入框，改好再发。
          </div>
          <el-form-item label="Voltip 的地址">
            <el-input
              v-model="form.voice_base_url"
              placeholder="http://127.0.0.1:47840/v1（Voltip「设置 → 本机服务」里的接口地址）"
              data-testid="ass-voice-url" />
            <div class="ass-tip">
              Voltip 默认只接受同一台电脑的连接：pt-tools 跑在别的机器或者 Docker
              里时，看文档「语音输入」一节。留空就不用语音输入。
            </div>
          </el-form-item>
          <el-form-item label="访问令牌">
            <el-input
              v-model="newVoiceToken"
              type="password"
              show-password
              autocomplete="off"
              :placeholder="
                hasVoiceToken
                  ? '已保存（不显示）；要换就填新的'
                  : 'Voltip 里「复制令牌」，或者 voltip-server --print-token'
              "
              data-testid="ass-voice-token" />
            <div class="ass-keyrow">
              <PtStatusPill :tone="hasVoiceToken ? 'ok' : 'neutral'" size="sm">{{
                hasVoiceToken ? "已保存令牌" : "没有令牌"
              }}</PtStatusPill>
              <el-button
                v-if="hasVoiceToken"
                link
                type="danger"
                data-testid="ass-voice-token-clear"
                @click="clearVoiceToken = true"
                >清除</el-button
              >
              <el-button
                v-if="clearVoiceToken"
                link
                data-testid="ass-voice-token-keep"
                @click="clearVoiceToken = false"
                >不清除</el-button
              >
            </div>
            <div
              v-if="voiceTokenWillDrop"
              class="ass-tip ass-tip--warn"
              data-testid="ass-voice-token-drop">
              换了 Voltip 的地址：保存时会清掉现在的访问令牌，要用就重新填。
            </div>
          </el-form-item>
          <el-form-item label="处理方式（模型）">
            <el-select
              v-model="form.voice_model"
              filterable
              allow-create
              default-first-option
              clearable
              placeholder="voltip：Voltip 里设的默认处理方式"
              data-testid="ass-voice-model">
              <el-option v-for="m in voiceModelOptions" :key="m.id" :value="m.id" :label="m.name">
                <span>{{ m.name }}</span>
                <span v-if="m.name !== m.id" class="ass-opt-id">{{ m.id }}</span>
              </el-option>
            </el-select>
            <div class="ass-tip">
              点「测试语音输入」读出 Voltip 里的预设与场景，比如「不使用 AI
              润色」（voltip:raw）；留空用 Voltip 的默认处理方式。
            </div>
          </el-form-item>
          <el-form-item label="识别语言">
            <el-select
              v-model="form.voice_language"
              clearable
              placeholder="不指定：按 Voltip 的设置或场景"
              data-testid="ass-voice-lang">
              <el-option
                v-for="l in VOICE_LANGUAGES"
                :key="l.value"
                :value="l.value"
                :label="l.label" />
            </el-select>
          </el-form-item>
          <div class="ass-voicetest">
            <el-button :loading="voiceTesting" data-testid="ass-voice-test" @click="testVoice">
              <PtIcon name="mic" :size="14" /><span>测试语音输入</span>
            </el-button>
          </div>
          <div v-if="voiceTested" class="ass-tested" data-testid="ass-voice-tested">
            <template v-if="voiceTested.ok">
              <PtStatusPill tone="ok" dot size="sm">连接正常</PtStatusPill>
              <span
                >Voltip 列出 {{ voiceTested.res.models.length }} 种处理方式，用时
                {{ voiceTested.res.latency_ms }} 毫秒。保存以后对话页上就有麦克风。</span
              >
            </template>
            <template v-else>
              <PtStatusPill tone="dang" dot size="sm">没有连上</PtStatusPill>
              <span>{{ voiceTested.msg }}</span>
            </template>
          </div>
          <div v-if="micBlocked" class="ass-tip ass-tip--warn" data-testid="ass-voice-insecure">
            {{ micBlocked }}。不然对话页上的麦克风用不了。
          </div>
        </el-form>
        <div v-if="tested" class="ass-tested" data-testid="ass-tested">
          <template v-if="tested.ok">
            <PtStatusPill tone="ok" dot size="sm">连接正常</PtStatusPill>
            <span
              >{{ tested.res.model }} 回答「{{ tested.res.reply || "（空）" }}」，用时
              {{ tested.res.latency_ms }} 毫秒</span
            >
          </template>
          <template v-else>
            <PtStatusPill tone="dang" dot size="sm">没有连上</PtStatusPill>
            <span>{{ tested.msg }}</span>
          </template>
        </div>
        <template #footer>
          <el-button :loading="testing" data-testid="ass-test" @click="test">
            <PtIcon name="activity" :size="14" /><span>测试连接</span>
          </el-button>
          <el-button type="primary" :loading="saving" data-testid="ass-save" @click="save">
            <PtIcon name="save" :size="14" /><span>保存</span>
          </el-button>
        </template>
      </PtPanel>

      <PtPanel title="安全" icon="shield-check" class="ass-aside" data-testid="ass-safety">
        <ul class="ass-list">
          <li>
            对话内容与工具查到的结果（站点数据、种子标题、下载器状态）会发给所选的模型服务商。
          </li>
          <li>
            改动类操作（暂停、删除、推送种子，管理订阅，整理入库，修改配置）每次都要你在对话里点「确认执行」，卡片上写明改以前、改以后与可能的影响；AI
            自己不能确认。这些操作记在「操作审计」里，通道是「AI 助手」。
          </li>
          <li>
            AI 改配置时读不到密钥（密码、Cookie、令牌只显示有没有设置）；登录密码、API
            令牌、远程访问与这一页的设置不能经 AI 改。
          </li>
          <li>
            种子下载地址里的 passkey、Cookie
            与密码不会交给模型；你贴的下载地址会先换成「站点与种子编号」。
          </li>
          <li>API Key 加密保存，只能改、不能读出来。</li>
          <li>
            语音输入的录音经 pt-tools 发给你填的 Voltip 服务，不发给大模型服务商；Voltip
            的访问令牌同样加密保存、只能改不能读。
          </li>
        </ul>
      </PtPanel>
    </template>
  </div>
</template>

<style scoped>
/* el-form-item 的内容区是横排的：说明文字占满一行，落在控件下面 */
.ass-tip {
  flex-basis: 100%;
  margin: 6px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.ass-sub {
  margin: 8px 0 12px;
  padding-top: 12px;
  border-top: 1px solid var(--pt-border);
  font-size: 13px;
  font-weight: 600;
  color: var(--pt-t1);
}

.ass-tip--warn {
  color: var(--pt-warn);
}

/* 小节开头的一句说明：不在表单项里，自己留出下面的空 */
.ass-tip--lead {
  margin: -4px 0 12px;
}

.ass-voicetest {
  margin-bottom: 8px;
}

.ass-opt-id {
  margin-left: 8px;
  font-size: 12px;
  color: var(--pt-t3);
}

.ass-tip--row {
  display: flex;
  gap: 6px;
  align-items: baseline;
}

.ass-provider {
  display: flex;
  flex-basis: 100%;
  flex-direction: column;
  gap: 2px;
}

.ass-link {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  width: fit-content;
  margin-top: 6px;
  font-size: 12px;
  color: var(--pt-p);
  text-decoration: none;
}

.ass-link:hover {
  text-decoration: underline;
}

.ass-modelrow {
  display: flex;
  flex-basis: 100%;
  gap: 8px;
  align-items: center;
}

.ass-modelrow__select {
  flex: 1;
  min-width: 0;
}

.ass-keyrow {
  display: flex;
  flex-basis: 100%;
  gap: 8px;
  align-items: center;
  margin-top: 6px;
}

.ass-tested {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  margin-top: 4px;
  font-size: 13px;
  color: var(--pt-t1);
  word-break: break-all;
}

/* 右栏窄卡自己顶对齐：以前被左边那张一屏多高的表单拉到整页高，下面大半是空白 */
.ass-aside {
  align-self: start;
}

.ass-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.ass-list li {
  position: relative;
  padding-left: 14px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--pt-t1);
}

.ass-list li::before {
  position: absolute;
  top: 0.62em;
  left: 2px;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--pt-t3);
  content: "";
}
</style>
