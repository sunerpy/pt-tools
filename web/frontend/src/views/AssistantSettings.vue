<script setup lang="ts">
/*
 * AI 助手设置（路线图 M19）：服务商、接口地址、模型、API Key（只写不读）、代理，ChatOps 开关与几项上限。
 * 任何 OpenAI 兼容的接口都能接；选了预设会填好地址与常用模型，模型也可以手填，或者「获取模型列表」从服务商读
 * （M20：参照 Voltip，预设写明免费额度、申请 Key 的页面、隐私与要不要代理）。
 * 换了接口地址又没填新 Key 时，后端会清掉存着的 Key（Key 不会发到保存它时以外的地址），这里提前写明。
 * 画板没有这一页，沿用 qB 兼容入口页的形状：左边设置、右边说明。
 */
import {
  assistantApi,
  type AssistantSettings,
  type AssistantSettingsInput,
  type AssistantTestResult,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { ASSISTANT_PROVIDERS, providerOf } from "@/utils/assistant";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref } from "vue";

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
  };
  if (!s.base_url) applyProvider(form.value.provider);
  newKey.value = "";
  clearKey.value = false;
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

function input(): AssistantSettingsInput {
  const out: AssistantSettingsInput = {
    ...form.value,
    base_url: form.value.base_url.trim(),
    model: form.value.model.trim(),
  };
  if (newKey.value.trim()) out.api_key = newKey.value.trim();
  else if (clearKey.value) out.api_key = "";
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
    tested.value = { ok: true, res: await assistantApi.testSettings(input()) };
  } catch (e) {
    tested.value = { ok: false, msg: errText(e, "连接失败") };
  } finally {
    testing.value = false;
  }
}

async function listModels() {
  const seq = ++listSeq;
  listing.value = true;
  listed.value = null;
  try {
    const res = await assistantApi.listModels(input());
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
      <PtPanel title="设置" icon="sparkles" data-testid="ass-settings">
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

      <PtPanel title="安全" icon="shield-check" data-testid="ass-safety">
        <ul class="ass-list">
          <li>
            对话内容与工具查到的结果（站点数据、种子标题、下载器状态）会发给所选的模型服务商。
          </li>
          <li>
            暂停、继续、删除、推送种子与添加订阅，每次都要你在对话里点「确认执行」；AI
            自己不能确认。这些操作记在「操作审计」里，通道是「AI 助手」。
          </li>
          <li>
            种子下载地址里的 passkey、Cookie
            与密码不会交给模型；你贴的下载地址会先换成「站点与种子编号」。
          </li>
          <li>API Key 加密保存，只能改、不能读出来。</li>
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
