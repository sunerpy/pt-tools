<script setup lang="ts">
import { type NotificationConfig, chatopsApi } from "@/api";
import { ArrowLeft, ChatDotRound, ChatSquare, Connection, Link } from "@element-plus/icons-vue";
import type { FormInstance, FormRules } from "element-plus";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, reactive, ref } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();

const id = computed(() => Number(route.params.id));

const loading = ref(false);
const saving = ref(false);
const testing = ref(false);
const activeTab = ref<"basic" | "credentials" | "test">("basic");

const formRef = ref<FormInstance>();
const credFormRef = ref<FormInstance>();

const conf = reactive<NotificationConfig>({
  id: 0,
  channel_type: "telegram",
  name: "",
  enabled: true,
});

interface TestResult {
  success: boolean;
  message: string;
  at: string;
}
const testResult = ref<TestResult | null>(null);
const testMessage = ref<string>("pt-tools 测试消息");

const channelTypeMeta: Record<
  string,
  { label: string; icon: unknown; tagType: "primary" | "success" | "warning" | "info" | "danger" }
> = {
  telegram: { label: "Telegram", icon: ChatDotRound, tagType: "primary" },
  qq_onebot: { label: "QQ (OneBot)", icon: ChatSquare, tagType: "warning" },
  webhook: { label: "Webhook", icon: Link, tagType: "info" },
  wecom_webhook: { label: "WeCom Webhook", icon: Connection, tagType: "success" },
};

const currentMeta = computed(() => channelTypeMeta[conf.channel_type] || channelTypeMeta.telegram);

const basicRules: FormRules = {
  name: [
    { required: true, message: "请填写通道名称", trigger: "blur" },
    { min: 1, max: 64, message: "名称长度需在 1~64 字符之间", trigger: "blur" },
  ],
};

const credRules = computed<FormRules>(() => {
  switch (conf.channel_type) {
    case "telegram":
      return {
        bot_token: [{ required: true, message: "请填写 Bot Token", trigger: "blur" }],
      };
    case "qq_onebot":
      return {
        listen_addr: [{ required: true, message: "请填写监听地址", trigger: "blur" }],
      };
    case "webhook":
      return {
        endpoint_url: [{ required: true, message: "请填写 Endpoint URL", trigger: "blur" }],
      };
    case "wecom_webhook":
      return {
        webhook_key: [{ required: true, message: "请填写 Webhook Key", trigger: "blur" }],
      };
    default:
      return {};
  }
});

onMounted(async () => {
  await loadDetail();
});

async function loadDetail() {
  if (!id.value) {
    ElMessage.error("无效的通道 ID");
    return;
  }
  loading.value = true;
  try {
    const data = await chatopsApi.notifications.get(id.value);
    Object.assign(conf, data);
    if (conf.channel_type === "qq_onebot") {
      conf.admin_qq_users = qqListToText(
        (data as unknown as Record<string, unknown>).admin_qq_users,
      );
      conf.allowed_qq_users = qqListToText(
        (data as unknown as Record<string, unknown>).allowed_qq_users,
      );
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载详情失败");
  } finally {
    loading.value = false;
  }
}

function parseQQList(raw: unknown): number[] {
  if (Array.isArray(raw)) {
    return raw.map((x) => Number(x)).filter((n) => Number.isFinite(n) && n > 0);
  }
  if (typeof raw !== "string" || raw.trim() === "") return [];
  return raw
    .split(/[,;\s\n]+/)
    .map((x) => x.trim())
    .filter(Boolean)
    .map((x) => Number(x))
    .filter((n) => Number.isFinite(n) && n > 0);
}

function qqListToText(raw: unknown): string {
  if (Array.isArray(raw)) {
    return raw.filter((x) => x !== null && x !== undefined && x !== "").join(",");
  }
  if (typeof raw === "string") return raw;
  return "";
}

async function handleSaveBasic() {
  if (!formRef.value) return;
  const valid = await formRef.value.validate().catch(() => false);
  if (!valid) return;

  saving.value = true;
  try {
    await chatopsApi.notifications.update(conf.id, {
      name: conf.name,
      enabled: conf.enabled,
      quiet_hours_start: conf.quiet_hours_start || "",
      quiet_hours_end: conf.quiet_hours_end || "",
    });
    ElMessage.success("已保存基本信息");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

async function handleSaveCredentials() {
  if (!credFormRef.value) return;
  const valid = await credFormRef.value.validate().catch(() => false);
  if (!valid) return;

  saving.value = true;
  try {
    const payload: Partial<NotificationConfig> = {};
    switch (conf.channel_type) {
      case "telegram":
        payload.bot_token = conf.bot_token;
        payload.allowed_users = conf.allowed_users;
        payload.admin_users = conf.admin_users;
        payload.default_chat_id = conf.default_chat_id;
        payload.proxy_url = conf.proxy_url;
        break;
      case "qq_onebot":
        payload.listen_addr = conf.listen_addr;
        payload.access_token = conf.access_token;
        payload.admin_qq_users = parseQQList(conf.admin_qq_users) as unknown as string;
        payload.allowed_qq_users = parseQQList(conf.allowed_qq_users) as unknown as string;
        break;
      case "webhook":
        payload.endpoint_url = conf.endpoint_url;
        payload.hmac_secret = conf.hmac_secret;
        payload.headers = conf.headers;
        break;
      case "wecom_webhook":
        payload.webhook_key = conf.webhook_key;
        break;
    }
    await chatopsApi.notifications.update(conf.id, payload);
    ElMessage.success("已保存凭证");
    await loadDetail();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

async function handleTest() {
  if (!conf.enabled) {
    try {
      await ElMessageBox.confirm(
        "通道当前为停用状态，可能无法成功推送测试消息。是否仍要继续？",
        "提示",
        {
          type: "warning",
          confirmButtonText: "继续测试",
          cancelButtonText: "取消",
        },
      );
    } catch {
      return;
    }
  }
  testing.value = true;
  testResult.value = null;
  try {
    const res = await chatopsApi.notifications.test(conf.id);
    testResult.value = {
      success: !!res?.success,
      message: res?.success ? "测试消息发送成功" : "测试消息可能未送达，请检查日志",
      at: new Date().toLocaleString(),
    };
    if (res?.success) {
      ElMessage.success("测试消息已发送");
    } else {
      ElMessage.warning("已触发测试，但返回结果异常");
    }
  } catch (e: unknown) {
    testResult.value = {
      success: false,
      message: (e as Error).message || "测试失败",
      at: new Date().toLocaleString(),
    };
    ElMessage.error((e as Error).message || "测试失败");
  } finally {
    testing.value = false;
  }
}

function goBack() {
  router.push("/chatops/notifications");
}
</script>

<template>
  <div class="page-container" v-loading="loading">
    <!-- Hero / 头部 -->
    <div class="hero-block">
      <div class="hero-back">
        <el-button text @click="goBack">
          <el-icon><ArrowLeft /></el-icon>
          返回通道列表
        </el-button>
      </div>
      <div class="hero-main">
        <div class="hero-brand">
          <el-icon class="brand-icon"><component :is="currentMeta.icon" /></el-icon>
          <div class="brand-text">
            <div class="brand-type">
              <el-tag :type="currentMeta.tagType" size="small">{{ currentMeta.label }}</el-tag>
              <el-tag v-if="conf.enabled" type="success" size="small" effect="plain">运行中</el-tag>
              <el-tag v-else type="info" size="small" effect="plain">已停用</el-tag>
            </div>
            <h1 class="brand-name">{{ conf.name || "未命名通道" }}</h1>
            <p class="brand-id">ID: {{ conf.id || "-" }}</p>
          </div>
        </div>
        <div class="hero-actions">
          <span class="enable-label">启用</span>
          <el-switch
            v-model="conf.enabled"
            :loading="saving"
            data-testid="enable-switch"
            @change="handleSaveBasic" />
        </div>
      </div>
    </div>

    <!-- Tabs -->
    <el-card class="content-card" shadow="never">
      <el-tabs v-model="activeTab" class="detail-tabs">
        <!-- 基本信息 -->
        <el-tab-pane label="基本信息" name="basic">
          <el-form
            ref="formRef"
            :model="conf"
            :rules="basicRules"
            label-position="top"
            class="form-grid"
            @submit.prevent>
            <el-form-item label="通道名称" prop="name">
              <el-input
                v-model="conf.name"
                maxlength="64"
                show-word-limit
                placeholder="例如：我的 Telegram 机器人"
                data-testid="name-input" />
            </el-form-item>
            <el-form-item label="通道类型">
              <el-input :model-value="currentMeta.label" disabled />
              <div class="form-hint">通道类型不可修改，如需更换请删除后重建。</div>
            </el-form-item>
            <el-form-item label="启用状态">
              <el-switch
                v-model="conf.enabled"
                active-text="启用"
                inactive-text="停用"
                inline-prompt />
            </el-form-item>
            <el-form-item label="静默时段">
              <el-time-picker
                v-model="conf.quiet_hours_start"
                format="HH:mm"
                value-format="HH:mm"
                placeholder="开始 (HH:MM)"
                clearable
                style="width: 130px" />
              <span style="margin: 0 8px">→</span>
              <el-time-picker
                v-model="conf.quiet_hours_end"
                format="HH:mm"
                value-format="HH:mm"
                placeholder="结束 (HH:MM)"
                clearable
                style="width: 130px" />
              <div class="form-hint">
                在此时段内通道不会主动发送通知，待静默结束后由 retry worker 投递。支持跨午夜（如 22:00 → 08:00）。
              </div>
            </el-form-item>
            <div class="form-actions">
              <el-button
                type="primary"
                :loading="saving"
                data-testid="save-basic-btn"
                @click="handleSaveBasic">
                保存基本信息
              </el-button>
            </div>
          </el-form>
        </el-tab-pane>

        <!-- 凭证 -->
        <el-tab-pane label="凭证" name="credentials">
          <el-alert
            type="warning"
            :closable="false"
            class="cred-alert"
            title="敏感凭证仅在保存时上传，回显已脱敏。请妥善保管，不要将凭证截图分享给第三方。" />
          <el-form
            ref="credFormRef"
            :model="conf"
            :rules="credRules"
            label-position="top"
            class="form-grid"
            @submit.prevent>
            <!-- Telegram -->
            <template v-if="conf.channel_type === 'telegram'">
              <el-form-item label="Bot Token" prop="bot_token">
                <el-input
                  v-model="conf.bot_token"
                  type="password"
                  show-password
                  placeholder="123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
                  data-testid="bot-token-input" />
              </el-form-item>
              <el-form-item label="允许用户 (allowed_users)">
                <el-input
                  v-model="conf.allowed_users"
                  type="textarea"
                  :rows="2"
                  placeholder="逗号分隔的 Telegram user_id 列表，留空表示允许所有" />
                <div class="form-hint">仅这些 Telegram 用户可与机器人交互。</div>
              </el-form-item>
              <el-form-item label="管理员用户 (admin_users)">
                <el-input
                  v-model="conf.admin_users"
                  type="textarea"
                  :rows="2"
                  placeholder="逗号分隔的 Telegram user_id 列表" />
                <div class="form-hint">管理员可执行受限指令。</div>
              </el-form-item>
              <el-form-item label="默认 Chat ID">
                <el-input v-model="conf.default_chat_id" placeholder="主动推送时使用的 chat_id" />
              </el-form-item>
              <el-form-item label="代理 URL（可选）" prop="proxy_url">
                <el-input
                  v-model="conf.proxy_url"
                  placeholder="http://127.0.0.1:1080 或 socks5://user:pass@host:1080"
                  clearable />
                <div class="form-hint">
                  留空则使用系统环境变量 HTTPS_PROXY / HTTP_PROXY；填写后该通道单独走此代理。
                </div>
              </el-form-item>
            </template>

            <!-- QQ OneBot -->
            <template v-if="conf.channel_type === 'qq_onebot'">
              <el-form-item label="监听地址 (listen_addr)" prop="listen_addr">
                <el-input v-model="conf.listen_addr" placeholder="0.0.0.0:8081" />
                <div class="form-hint">OneBot 反向 WebSocket 监听地址。</div>
              </el-form-item>
              <el-form-item label="Access Token">
                <el-input
                  v-model="conf.access_token"
                  type="password"
                  show-password
                  placeholder="OneBot access_token" />
              </el-form-item>
              <el-form-item label="管理员 QQ (admin_qq_users)">
                <el-input
                  v-model="conf.admin_qq_users"
                  type="textarea"
                  :rows="2"
                  placeholder="逗号分隔的 QQ 号列表" />
              </el-form-item>
              <el-form-item label="允许 QQ (allowed_qq_users)">
                <el-input
                  v-model="conf.allowed_qq_users"
                  type="textarea"
                  :rows="2"
                  placeholder="逗号分隔的 QQ 号列表，留空表示允许所有" />
              </el-form-item>
            </template>

            <!-- Webhook -->
            <template v-if="conf.channel_type === 'webhook'">
              <el-form-item label="Endpoint URL" prop="endpoint_url">
                <el-input v-model="conf.endpoint_url" placeholder="https://example.com/notify" />
              </el-form-item>
              <el-form-item label="HMAC Secret">
                <el-input
                  v-model="conf.hmac_secret"
                  type="password"
                  show-password
                  placeholder="用于签名的密钥（可选）" />
              </el-form-item>
              <el-form-item label="自定义 Headers (JSON)">
                <el-input
                  v-model="conf.headers"
                  type="textarea"
                  :rows="3"
                  placeholder='{"Authorization": "Bearer xxx"}' />
                <div class="form-hint">JSON 对象格式，将随请求一起发送。</div>
              </el-form-item>
            </template>

            <!-- WeCom -->
            <template v-if="conf.channel_type === 'wecom_webhook'">
              <el-form-item label="Webhook Key" prop="webhook_key">
                <el-input
                  v-model="conf.webhook_key"
                  type="password"
                  show-password
                  placeholder="企业微信群机器人 webhook key" />
                <div class="form-hint">来自企业微信群机器人 URL `?key=` 之后的部分。</div>
              </el-form-item>
            </template>

            <div class="form-actions">
              <el-button
                type="primary"
                :loading="saving"
                data-testid="save-cred-btn"
                @click="handleSaveCredentials">
                保存凭证
              </el-button>
            </div>
          </el-form>
        </el-tab-pane>

        <!-- 测试 -->
        <el-tab-pane label="测试" name="test">
          <div class="test-pane">
            <p class="test-desc">
              点击下方按钮立即向 <strong>{{ conf.name || "当前通道" }}</strong> 发送一条测试消息，
              用于验证凭证与连通性。
            </p>
            <el-form label-position="top" @submit.prevent>
              <el-form-item label="测试消息内容（仅展示用，由后端生成）">
                <el-input v-model="testMessage" :disabled="true" />
              </el-form-item>
            </el-form>
            <div class="form-actions">
              <el-button
                type="primary"
                size="large"
                :loading="testing"
                data-testid="run-test-btn"
                @click="handleTest">
                发送测试消息
              </el-button>
            </div>

            <transition name="fade">
              <div
                v-if="testResult"
                class="test-result"
                :class="{ 'is-success': testResult.success, 'is-error': !testResult.success }">
                <div class="result-header">
                  <el-tag :type="testResult.success ? 'success' : 'danger'" effect="dark">
                    {{ testResult.success ? "成功" : "失败" }}
                  </el-tag>
                  <span class="result-time">{{ testResult.at }}</span>
                </div>
                <div class="result-body">{{ testResult.message }}</div>
              </div>
            </transition>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<style scoped>
.page-container {
  padding: 16px 24px 32px;
}

/* Hero */
.hero-block {
  position: relative;
  padding: 24px;
  margin-bottom: 24px;
  border-radius: var(--pt-radius-xl, 16px);
  background:
    radial-gradient(
      ellipse at top right,
      color-mix(in oklab, var(--pt-color-primary) 15%, transparent),
      transparent 60%
    ),
    linear-gradient(to right, rgb(128 128 128 / 8%) 1px, transparent 1px) 0 0 / 32px 32px,
    linear-gradient(to bottom, rgb(128 128 128 / 8%) 1px, transparent 1px) 0 0 / 32px 32px,
    var(--pt-bg-surface);
  border: 1px solid var(--pt-border-color);
  overflow: hidden;
}

.hero-back {
  margin-bottom: 12px;
}

.hero-main {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  flex-wrap: wrap;
}

.hero-brand {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
  flex: 1;
}

.brand-icon {
  font-size: 28px;
  color: var(--pt-color-primary);
  background: color-mix(in oklab, var(--pt-color-primary) 12%, transparent);
  padding: 14px;
  border-radius: 12px;
  flex-shrink: 0;
}

.brand-text {
  min-width: 0;
}

.brand-type {
  display: flex;
  gap: 6px;
  margin-bottom: 6px;
  flex-wrap: wrap;
}

.brand-name {
  font-size: 24px;
  font-weight: 700;
  margin: 0;
  color: var(--pt-text-primary);
  letter-spacing: -0.01em;
  word-break: break-word;
}

.brand-id {
  font-size: 12px;
  color: var(--pt-text-secondary);
  margin: 4px 0 0 0;
  font-family: var(--el-font-family-monospace, ui-monospace, monospace);
}

.hero-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.enable-label {
  font-size: 14px;
  color: var(--pt-text-secondary);
}

/* Content card */
.content-card {
  border-radius: var(--pt-radius-lg, 12px);
  background: color-mix(in oklab, var(--pt-bg-surface) 70%, transparent);
  backdrop-filter: blur(8px);
  border: 1px solid var(--pt-border-color);
  box-shadow: var(--pt-shadow-sm);
}

.detail-tabs :deep(.el-tabs__item.is-active) {
  color: var(--pt-color-primary);
}

.detail-tabs :deep(.el-tabs__active-bar) {
  background-color: var(--pt-color-primary);
}

.form-grid {
  max-width: 640px;
}

.form-hint {
  font-size: 12px;
  color: var(--pt-text-secondary);
  margin-top: 4px;
  line-height: 1.4;
}

.form-actions {
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid color-mix(in oklab, var(--pt-border-color) 50%, transparent);
}

.cred-alert {
  margin-bottom: 20px;
}

/* Test pane */
.test-pane {
  max-width: 640px;
}

.test-desc {
  font-size: 14px;
  color: var(--pt-text-secondary);
  line-height: 1.6;
  margin: 0 0 20px 0;
}

.test-result {
  margin-top: 24px;
  padding: 16px;
  border-radius: var(--pt-radius-md, 10px);
  border: 1px solid var(--pt-border-color);
  background: var(--pt-bg-surface);
}

.test-result.is-success {
  border-color: color-mix(in oklab, var(--pt-color-success) 40%, var(--pt-border-color));
  background: color-mix(in oklab, var(--pt-color-success) 5%, var(--pt-bg-surface));
}

.test-result.is-error {
  border-color: color-mix(in oklab, var(--pt-color-danger) 40%, var(--pt-border-color));
  background: color-mix(in oklab, var(--pt-color-danger) 5%, var(--pt-bg-surface));
}

.result-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 8px;
}

.result-time {
  font-size: 12px;
  color: var(--pt-text-secondary);
  font-family: var(--el-font-family-monospace, ui-monospace, monospace);
}

.result-body {
  font-size: 14px;
  color: var(--pt-text-primary);
  line-height: 1.5;
  word-break: break-word;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 200ms ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
