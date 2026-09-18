<script setup lang="ts">
import { chatopsApi, type NotificationConfig } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import type { FormInstance, FormRules } from "element-plus";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, reactive, ref } from "vue";
import { useRoute, useRouter } from "vue-router";

/* 图标与色相与通道列表页保持同一套：同一个通道在两个页面必须长一个样 */
const CHANNEL_META: Record<string, { label: string; icon: string; color: string }> = {
  telegram: { label: "Telegram", icon: "send", color: "var(--pt-info)" },
  qq_onebot: { label: "QQ (OneBot)", icon: "message-square", color: "var(--pt-ok)" },
  webhook: { label: "Webhook", icon: "link", color: "var(--pt-p)" },
  wecom_webhook: { label: "WeCom Webhook", icon: "message-circle", color: "var(--pt-warn)" },
};

const route = useRoute();
const router = useRouter();
/* ≤768 时外壳隐藏页头，页头动作得原地落回页面（Teleport 的 disabled） */
const isMobile = useIsMobile();

const id = computed(() => Number(route.params.id));

const loading = ref(false);
const saving = ref(false);
const testing = ref(false);

const formRef = ref<FormInstance>();
const credFormRef = ref<FormInstance>();

const conf = reactive<NotificationConfig>({
  id: 0,
  channel_type: "telegram",
  name: "",
  enabled: true,
});

const tgForm = reactive({
  admin_users_text: "",
  allowed_users_text: "",
  default_chat_id_text: "",
});

interface TestResult {
  success: boolean;
  message: string;
  at: string;
}
const testResult = ref<TestResult | null>(null);
const testMessage = ref<string>("pt-tools 测试消息");

const currentMeta = computed(() => CHANNEL_META[conf.channel_type] || CHANNEL_META.telegram);

/**
 * 画板 p-msg 700：这一页会弹的每一种提示。
 * 每条都对应下面某个 ElMessage 的实参 —— 改文案要两处一起改，别让卡和真实提示对不上。
 */
const toastCopy: { kind: string; tone: "ok" | "warn" | "dang"; text: string; when: string }[] = [
  { kind: "成功", tone: "ok", text: "保存成功", when: "改完基本信息或凭证并保存" },
  { kind: "成功", tone: "ok", text: "连接测试成功", when: "「测试连接」拿到 2xx" },
  { kind: "失败", tone: "dang", text: "保存失败", when: "保存请求非 2xx，后面跟后端的原话" },
  { kind: "失败", tone: "dang", text: "连接测试失败", when: "测试请求失败，后面跟后端的原话" },
  {
    kind: "提示",
    tone: "warn",
    text: "凭证留空则保持不变",
    when: "凭证字段留空保存时 —— 已存的值不会被空串覆盖",
  },
];

/** 画板 head 88 的 sub（11.5/400 t3）：这条记录的身份 —— ID 与通道类型 */
const headSub = computed(() => `ID ${conf.id || "-"} · ${currentMeta.value.label}`);

const quietHoursNote = computed(() => {
  const s = conf.quiet_hours_start;
  const e = conf.quiet_hours_end;
  if (!s || !e) return "未设置，任何时段都会立即推送";
  return `${s} → ${e}${s > e ? "（跨午夜）" : ""}`;
});

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
    // 后端返回的 config_json 是解密后的对象（如 { bot_token, admin_users, default_chat_id, ... }），
    // 必须 flatten 到 conf 上，否则编辑表单的 v-model 输入框是空的，提交时会把已有字段覆盖丢失。
    const cfg = (data as unknown as Record<string, unknown>).config_json;
    if (cfg && typeof cfg === "object") {
      Object.assign(conf, cfg as Record<string, unknown>);
    }
    if (conf.channel_type === "qq_onebot") {
      conf.admin_qq_users = userIdListToText(
        (conf as unknown as Record<string, unknown>).admin_qq_users,
      );
      conf.allowed_qq_users = userIdListToText(
        (conf as unknown as Record<string, unknown>).allowed_qq_users,
      );
    } else if (conf.channel_type === "telegram") {
      const sink = conf as unknown as Record<string, unknown>;
      tgForm.admin_users_text = userIdListToText(sink.admin_users);
      tgForm.allowed_users_text = userIdListToText(sink.allowed_users);
      const dcid = sink.default_chat_id;
      tgForm.default_chat_id_text = dcid != null && dcid !== "" ? String(dcid) : "";
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载详情失败");
  } finally {
    loading.value = false;
  }
}

function parseUserIdList(raw: unknown): number[] {
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

function userIdListToText(raw: unknown): string {
  if (Array.isArray(raw)) {
    return raw.filter((x) => x !== null && x !== undefined && x !== "").join(",");
  }
  if (typeof raw === "string") return raw;
  if (typeof raw === "number") return String(raw);
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
        {
          (payload as unknown as Record<string, unknown>).allowed_users = parseUserIdList(
            tgForm.allowed_users_text,
          );
          (payload as unknown as Record<string, unknown>).admin_users = parseUserIdList(
            tgForm.admin_users_text,
          );
          const dcidText = tgForm.default_chat_id_text.trim();
          if (dcidText === "") {
            (payload as unknown as Record<string, unknown>).default_chat_id = "";
          } else {
            const asNum = Number(dcidText);
            (payload as unknown as Record<string, unknown>).default_chat_id =
              Number.isFinite(asNum) && /^-?\d+$/.test(dcidText) ? asNum : dcidText;
          }
        }
        payload.proxy_url = conf.proxy_url;
        break;
      case "qq_onebot":
        payload.listen_addr = conf.listen_addr;
        payload.access_token = conf.access_token;
        payload.admin_qq_users = parseUserIdList(conf.admin_qq_users) as unknown as string;
        payload.allowed_qq_users = parseUserIdList(conf.allowed_qq_users) as unknown as string;
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
    const ok = !!res?.success;
    testResult.value = {
      success: ok,
      message: ok ? "测试消息发送成功，请在目标账号查收。" : "测试消息可能未送达，请检查日志",
      at: new Date().toLocaleString("zh-CN", { hour12: false }),
    };
    if (ok) {
      ElMessage.success("测试消息发送成功");
    } else {
      ElMessage.warning("已触发测试，但返回结果异常");
    }
  } catch (e: unknown) {
    const raw = (e as Error).message || "测试失败";
    let reason = raw;
    try {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === "object") {
        reason = parsed.detail || parsed.error || raw;
      }
    } catch {
      // raw 不是 JSON，原样使用
    }
    testResult.value = {
      success: false,
      message: reason,
      at: new Date().toLocaleString("zh-CN", { hour12: false }),
    };
    ElMessage({
      type: "error",
      message: `测试消息发送失败：${reason}`,
      duration: 8000,
      showClose: true,
    });
  } finally {
    testing.value = false;
  }
}

function goBack() {
  router.push("/chatops/notifications");
}
</script>

<template>
  <!--
    画板 23：head 88（详情页的面包屑页头，由外壳给）→ hero 1080×116 → c-basic 1080×262
    → c-test 1080×250。通道身份落在 hero 卡里，ID 与类型走页头摘要，返回与启用开关
    是页头右侧的动作。
  -->
  <div v-loading="loading" class="notify-detail-page pt-cards pt-cards--wide">
    <Teleport to="#pt-head-sub">{{ headSub }}</Teleport>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button @click="goBack">
        <PtIcon name="arrow-left" :size="15" /><span>通道列表</span>
      </el-button>
      <!-- 开关直接落库：详情页顶上的这一枚是「上线 / 下线」按钮，不是待保存的表单项 -->
      <label class="ctl">
        <el-switch
          v-model="conf.enabled"
          :loading="saving"
          data-testid="enable-switch"
          @change="handleSaveBasic" />
        <span>启用通道</span>
      </label>
    </Teleport>

    <!-- 画板 hero 1080×116：通道身份 —— 色块图标 + 名字 + 类型 + 运行状态 -->
    <section class="hero">
      <span class="hero__icon" :style="{ '--ch-c': currentMeta.color }">
        <PtIcon :name="currentMeta.icon" :size="20" />
      </span>
      <div class="hero__body">
        <h2 class="hero__name">{{ conf.name || "未命名通道" }}</h2>
        <div class="hero__meta">
          <PtTag>{{ currentMeta.label }}</PtTag>
          <PtStatusPill :tone="conf.enabled ? 'ok' : 'neutral'" size="sm">
            {{ conf.enabled ? "运行中" : "已停用" }}
          </PtStatusPill>
        </div>
      </div>
    </section>

    <PtPanel title="基本信息" icon="settings" padding="none">
      <el-form
        ref="formRef"
        :model="conf"
        :rules="basicRules"
        label-position="top"
        class="pt-form settings-form"
        @submit.prevent>
        <div class="pt-strip">
          <PtIcon name="id-card" :size="13" />
          <span>身份</span>
        </div>
        <div class="settings-body">
          <div class="field-row">
            <el-form-item label="通道名称" prop="name">
              <el-input
                v-model="conf.name"
                maxlength="64"
                show-word-limit
                placeholder="例如：我的 Telegram 机器人"
                data-testid="name-input" />
              <div class="field-tip">只用于在列表里区分通道，随时可改</div>
            </el-form-item>
            <el-form-item label="通道类型">
              <el-input :model-value="currentMeta.label" disabled />
              <div class="field-tip">类型不可修改，换类型请删除后重建</div>
            </el-form-item>
          </div>
        </div>

        <div class="pt-strip">
          <PtIcon name="clock" :size="13" />
          <span>静默时段</span>
          <span class="pt-strip__end">{{ quietHoursNote }}</span>
        </div>
        <div class="settings-body">
          <el-form-item label="不打扰的时间范围">
            <span class="quiet-row">
              <el-time-picker
                v-model="conf.quiet_hours_start"
                format="HH:mm"
                value-format="HH:mm"
                placeholder="开始"
                clearable
                class="quiet-row__pick" />
              <PtIcon name="arrow-right" :size="14" class="quiet-row__sep" />
              <el-time-picker
                v-model="conf.quiet_hours_end"
                format="HH:mm"
                value-format="HH:mm"
                placeholder="结束"
                clearable
                class="quiet-row__pick" />
            </span>
            <div class="field-tip">
              这段时间内通道不主动发通知，静默结束后由 retry worker 补投；支持跨午夜（如 22:00 →
              08:00）
            </div>
          </el-form-item>
        </div>
      </el-form>

      <template #footer>
        <span class="pt-foot-note">名称、启用状态与静默时段</span>
        <el-button
          type="primary"
          :loading="saving"
          data-testid="save-basic-btn"
          @click="handleSaveBasic">
          <PtIcon name="save" :size="14" /><span>保存基本信息</span>
        </el-button>
      </template>
    </PtPanel>

    <PtPanel title="凭证与连接" icon="key-round" padding="none">
      <el-form
        ref="credFormRef"
        :model="conf"
        :rules="credRules"
        label-position="top"
        class="pt-form settings-form"
        @submit.prevent>
        <div class="pt-strip">
          <PtIcon name="shield-check" :size="13" />
          <span>{{ currentMeta.label }} 凭证</span>
          <span class="pt-strip__end">仅在保存时上传</span>
        </div>
        <div class="settings-body">
          <div class="pt-note pt-note--warn cred-note">
            <PtIcon name="lock" :size="14" class="pt-note__icon" />
            <span>回显已脱敏。请妥善保管，不要把凭证截图发给第三方。</span>
          </div>

          <!-- Telegram -->
          <template v-if="conf.channel_type === 'telegram'">
            <el-form-item label="Bot Token" prop="bot_token">
              <el-input
                v-model="conf.bot_token"
                type="password"
                show-password
                placeholder="123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
                data-testid="bot-token-input" />
              <div class="field-tip">找 <code>@BotFather</code> 创建机器人后拿到的 token</div>
            </el-form-item>

            <div class="field-head">谁可以用这个机器人</div>

            <el-form-item label="允许用户（allowed_users）">
              <el-input
                v-model="tgForm.allowed_users_text"
                type="textarea"
                :rows="2"
                placeholder="逗号分隔的 Telegram user_id，例如：123456789,987654321" />
              <div class="field-tip">
                只有这些用户能与机器人交互（收消息、发非管理员命令）；留空表示允许所有人
              </div>
            </el-form-item>
            <el-form-item label="管理员用户（admin_users）">
              <el-input
                v-model="tgForm.admin_users_text"
                type="textarea"
                :rows="2"
                placeholder="逗号分隔的 Telegram user_id，例如：123456789" />
              <div class="field-tip">
                管理员可执行 <code>/unbind</code> <code>/pause</code> <code>/resume</code>
                <code>/delete</code> 等管理命令
              </div>
            </el-form-item>

            <div class="field-head">出站推送</div>

            <div class="field-row">
              <el-form-item label="默认 Chat ID">
                <el-input
                  v-model="tgForm.default_chat_id_text"
                  placeholder="数字 user_id 或 @channelusername" />
                <div class="field-tip">主动推送的目标；公开频道填 @channelusername</div>
              </el-form-item>
              <el-form-item label="代理 URL（可选）" prop="proxy_url">
                <el-input
                  v-model="conf.proxy_url"
                  placeholder="http://127.0.0.1:1080 或 socks5://user:pass@host:1080"
                  clearable />
                <div class="field-tip">
                  留空走系统的 <code>HTTPS_PROXY</code> /
                  <code>HTTP_PROXY</code>；填了则本通道单独走它
                </div>
              </el-form-item>
            </div>
          </template>

          <!-- QQ OneBot -->
          <template v-if="conf.channel_type === 'qq_onebot'">
            <div class="field-row">
              <el-form-item label="监听地址（listen_addr）" prop="listen_addr">
                <el-input v-model="conf.listen_addr" placeholder="0.0.0.0:8081" />
                <div class="field-tip">OneBot 反向 WebSocket 的监听地址</div>
              </el-form-item>
              <el-form-item label="Access Token">
                <el-input
                  v-model="conf.access_token"
                  type="password"
                  show-password
                  placeholder="OneBot access_token" />
                <div class="field-tip">与 OneBot 实现里配置的 token 一致</div>
              </el-form-item>
            </div>

            <div class="field-head">谁可以用这个机器人</div>

            <el-form-item label="管理员 QQ（admin_qq_users）">
              <el-input
                v-model="conf.admin_qq_users"
                type="textarea"
                :rows="2"
                placeholder="逗号分隔的 QQ 号" />
              <div class="field-tip">管理员可执行管理类命令</div>
            </el-form-item>
            <el-form-item label="允许 QQ（allowed_qq_users）">
              <el-input
                v-model="conf.allowed_qq_users"
                type="textarea"
                :rows="2"
                placeholder="逗号分隔的 QQ 号" />
              <div class="field-tip">留空表示允许所有人</div>
            </el-form-item>
          </template>

          <!-- Webhook -->
          <template v-if="conf.channel_type === 'webhook'">
            <div class="field-row">
              <el-form-item label="Endpoint URL" prop="endpoint_url">
                <el-input v-model="conf.endpoint_url" placeholder="https://example.com/notify" />
                <div class="field-tip">pt-tools 会向这个地址 POST JSON</div>
              </el-form-item>
              <el-form-item label="HMAC Secret">
                <el-input
                  v-model="conf.hmac_secret"
                  type="password"
                  show-password
                  placeholder="用于签名的密钥（可选）" />
                <div class="field-tip">填了会给请求体签名，接收端可校验来源</div>
              </el-form-item>
            </div>
            <el-form-item label="自定义 Headers（JSON）">
              <el-input
                v-model="conf.headers"
                type="textarea"
                :rows="3"
                placeholder='{"Authorization": "Bearer xxx"}' />
              <div class="field-tip">JSON 对象格式，随请求一起发送</div>
            </el-form-item>
          </template>

          <!-- WeCom -->
          <template v-if="conf.channel_type === 'wecom_webhook'">
            <el-form-item label="Webhook Key" prop="webhook_key">
              <el-input
                v-model="conf.webhook_key"
                type="password"
                show-password
                placeholder="企业微信群机器人的 key" />
              <div class="field-tip">群机器人地址里 <code>key=</code> 后面那一段</div>
            </el-form-item>
          </template>
        </div>
      </el-form>

      <template #footer>
        <span class="pt-foot-note">保存后会立即用新凭证重连通道</span>
        <el-button
          type="primary"
          :loading="saving"
          data-testid="save-cred-btn"
          @click="handleSaveCredentials">
          <PtIcon name="save" :size="14" /><span>保存凭证</span>
        </el-button>
      </template>
    </PtPanel>

    <PtPanel title="连通性测试" icon="zap">
      <div class="test">
        <p class="test__desc">
          向
          <strong>{{ conf.name || "当前通道" }}</strong> 发一条测试消息，用来验证凭证与网络是否通。
        </p>

        <el-form label-position="top" class="pt-form" @submit.prevent>
          <el-form-item label="消息内容">
            <el-input v-model="testMessage" disabled />
            <div class="field-tip">内容由后端生成，这里只是预览</div>
          </el-form-item>
        </el-form>

        <div class="test__acts">
          <el-button
            type="primary"
            :loading="testing"
            data-testid="run-test-btn"
            @click="handleTest">
            <PtIcon name="send" :size="14" /><span>发送测试消息</span>
          </el-button>
          <span class="test__hint">停用状态下也能试，但大概率发不出去</span>
        </div>

        <transition name="fade">
          <div
            v-if="testResult"
            class="pt-note test__result"
            :class="testResult.success ? 'pt-note--ok' : 'pt-note--dang'">
            <PtIcon
              :name="testResult.success ? 'circle-check' : 'circle-x'"
              :size="14"
              class="pt-note__icon" />
            <span class="test__result-body">
              <span>{{ testResult.message }}</span>
              <span class="test__result-at">{{ testResult.at }}</span>
            </span>
          </div>
        </transition>
      </div>
    </PtPanel>

    <!--
      画板 p-msg 700「ElMessage 全量文案」—— 这一页会弹出的每一种提示。
      画板把它列出来是为了让人不必逐个触发就知道各种结果长什么样；这里照原意落地，
      标题用面向用户的说法。文案与下面各处 ElMessage 的实参一字不差，改一处要改两处。
    -->
    <PtPanel class="msg-card" title="操作提示文案" icon="message-square">
      <ul class="msg">
        <li v-for="row in toastCopy" :key="row.text" class="msg__row">
          <PtStatusPill :tone="row.tone" size="sm">{{ row.kind }}</PtStatusPill>
          <span class="msg__t">{{ row.text }}</span>
          <span class="msg__when">{{ row.when }}</span>
        </li>
      </ul>
    </PtPanel>
  </div>
</template>

<style scoped>
/* 提示文案卡：一行一条，左胶囊右场景 */
.msg-card {
  max-width: 700px;
}

.msg {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.msg__row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
  align-items: baseline;
  font-size: var(--pt-fz-sm);
}

.msg__t {
  font-weight: 500;
  color: var(--pt-t1);
}

.msg__when {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 内缩 16、卡间 16 由 .pt-cards--wide 给；画板这几张卡是 1080 通栏，不是 880 */

/* 画板 hero 1080×116：卡片层的一块，所以和 PtPanel 同一套边框与圆角 */
.hero {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  padding: var(--pt-pad);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: var(--pt-shadow-sm);
}

.hero__icon {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  color: var(--ch-c);
  background: color-mix(in srgb, var(--ch-c) 12%, transparent);
  border-radius: var(--pt-r-md);
}

.hero__body {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.hero__name {
  margin: 0;
  overflow: hidden;
  font-size: var(--pt-fz-h3);
  font-weight: 600;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hero__meta {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
}

/* 开关 + 文字算一个整体控件，点文字也能切；label 天然带这个行为 */
.ctl {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  cursor: pointer;
  user-select: none;
}

/* 第一条区块条紧贴面板页头，两条发丝线会叠成 2px */
.settings-form > .pt-strip:first-child {
  border-top: 0;
}

/* 下内边距留 0：末个字段自带的 16 下外边距正好等于 --pt-pad，凑成段尾留白 */
.settings-body {
  padding: var(--pt-pad) var(--pt-pad) 0;
}

.cred-note {
  margin-bottom: var(--pt-space-4);
}

/* 两个时间选择器 + 一枚箭头是一个整体，别让 el-form-item 把它们拆成三行 */
.quiet-row {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
}

.quiet-row__pick {
  width: 128px;
}

.quiet-row__sep {
  flex: 0 0 auto;
  color: var(--pt-t4);
}

.test__desc {
  margin: 0 0 var(--pt-space-4);
  font-size: var(--pt-fz-body);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.test__acts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-3);
  align-items: center;
}

.test__hint {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.test__result {
  margin-top: var(--pt-space-4);
}

.test__result-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  word-break: break-word;
}

.test__result-at {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity var(--pt-transition-fast);
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

@media (max-width: 768px) {
  .quiet-row__pick {
    width: 100%;
  }

  .quiet-row {
    display: grid;
    grid-template-columns: 1fr auto 1fr;
    width: 100%;
  }
}
</style>
