<script setup lang="ts">
import { chatopsApi, type NotificationConfig } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { isLoopbackListenAddr } from "@/utils/listenAddr";
import { OUTBOUND_CHANNELS, missingRequired, outboundChannel } from "@/utils/notifyChannels";
import type { FormInstance, FormRules } from "element-plus";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

/* 图标与色相与通道列表页保持同一套：同一个通道在两个页面必须长一个样 */
const CHANNEL_META: Record<string, { label: string; icon: string; color: string }> = {
  telegram: { label: "Telegram", icon: "send", color: "var(--pt-info)" },
  qq_onebot: { label: "QQ (OneBot)", icon: "message-square", color: "var(--pt-ok)" },
  webhook: { label: "Webhook", icon: "link", color: "var(--pt-p)" },
  wecom_webhook: { label: "WeCom Webhook", icon: "message-circle", color: "var(--pt-warn)" },
  ...Object.fromEntries(
    OUTBOUND_CHANNELS.map((c) => [c.type, { label: c.label, icon: c.icon, color: c.color }]),
  ),
};

const route = useRoute();
const router = useRouter();
/* ≤768 时外壳隐藏页头，页头动作得原地落回页面（Teleport 的 disabled） */
const isMobile = useIsMobile();

const id = computed(() => Number(route.params.id));

const loading = ref(false);
const saving = ref(false);
const testing = ref(false);
/** 当前路由上的这条通道读回来过一次。没读回来（首次加载中、刚切到别的通道）时 conf 不是它的配置 */
const loaded = ref(false);
/** 保存与测试都按 conf.id 发出：没读回来或正在读时一律不放行 */
const locked = computed(() => !loaded.value || loading.value);
/** 每次加载换一个序号：切走之后才回来的旧响应作废 */
let loadSeq = 0;

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
  /*
   * 之前这张卡列的是「保存成功 / 连接测试成功 / 连接测试失败 / 凭证留空则保持不变」—— 这页根本不弹前三条，
   * 第四条还是错的：凭证字段会回显当前值、按原样提交，清空再保存就是用空串覆盖。现在逐条对着下面的 ElMessage，
   * 带 ${…} 的那条用「…」代替插值；toastCopy.test.ts 核对两边一条不多、一条不少。
   */
  { kind: "成功", tone: "ok", text: "已保存基本信息", when: "改完名称或静默时段并保存" },
  {
    kind: "成功",
    tone: "ok",
    text: "已保存凭证",
    when: "改完凭证并保存 —— 清空的字段会按空值保存",
  },
  { kind: "成功", tone: "ok", text: "测试消息发送成功", when: "「发送测试消息」送达" },
  {
    kind: "提示",
    tone: "warn",
    text: "已触发测试，但返回结果异常",
    when: "测试请求发出了，但返回的不是预期结构",
  },
  {
    kind: "失败",
    tone: "dang",
    text: "测试消息发送失败：…",
    when: "测试请求失败 —— 冒号后是后端给的原因，停留 8 秒、可手动关闭",
  },
  {
    kind: "失败",
    tone: "dang",
    text: "保存失败",
    when: "保存请求失败 —— 后端有原话时直接显示原话",
  },
  {
    kind: "失败",
    tone: "dang",
    text: "加载详情失败",
    when: "打开这一页时读不到通道详情 —— 后端有原话时直接显示原话",
  },
  { kind: "失败", tone: "dang", text: "无效的通道 ID", when: "地址里的通道 ID 不是有效的数字" },
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

/**
 * 画板 37 的 p-map：四种 channel_type 各自的显示名与唯一必填项。
 *
 * 取值不是抄的：类型字符串是各适配器 `RegisterChannel` 的注册名
 * （internal/notify/adapter/*），必填项与下面的 `credRules` 同源 —— 两处要一起改。
 */
const channelTypeMap = [
  { type: "telegram", label: "Telegram", must: "bot_token" },
  { type: "qq_onebot", label: "QQ (OneBot)", must: "listen_addr" },
  { type: "webhook", label: "Webhook", must: "endpoint_url" },
  { type: "wecom_webhook", label: "WeCom Webhook", must: "webhook_key" },
  ...OUTBOUND_CHANNELS.map((c) => ({
    type: c.type,
    label: c.label,
    must: c.fields.find((f) => f.required)?.key ?? "",
  })),
];

/* 只出站通道的字段按名字绑定：conf 始终是同一个 reactive 对象（resetConf 只删键、loadDetail 用 Object.assign） */
const confText = conf as unknown as Record<string, string | undefined>;
const confNum = conf as unknown as Record<string, number | undefined>;

const credRules = computed<FormRules>(() => {
  switch (conf.channel_type) {
    case "telegram":
      return {
        bot_token: [{ required: true, message: "请填写 Bot Token", trigger: "blur" }],
      };
    case "qq_onebot":
      return {
        listen_addr: [{ required: true, message: "请填写监听地址", trigger: "blur" }],
        access_token: [
          {
            /* 后端在监听非本机地址且 token 为空时拒绝启动通道：反向 WS 的 user_id 由连接方自报，
               没有 token，能连上端口的人就能冒充管理员发命令 */
            validator: (_rule: unknown, value: string, callback: (error?: Error) => void) => {
              if (!value?.trim() && !isLoopbackListenAddr(conf.listen_addr || "")) {
                callback(new Error("监听地址不是本机地址时必须填写 Access Token"));
              } else {
                callback();
              }
            },
            trigger: "blur",
          },
        ],
      };
    case "webhook":
      return {
        endpoint_url: [{ required: true, message: "请填写 Endpoint URL", trigger: "blur" }],
      };
    case "wecom_webhook":
      return {
        webhook_key: [{ required: true, message: "请填写 Webhook Key", trigger: "blur" }],
      };
    default: {
      const rules: FormRules = {};
      for (const f of outboundChannel(conf.channel_type)?.fields ?? []) {
        if (f.required)
          rules[f.key] = [{ required: true, message: `请填写${f.label}`, trigger: "blur" }];
      }
      return rules;
    }
  }
});

onMounted(async () => {
  await loadDetail();
});

/*
 * 只改路由参数（/chatops/notifications/1 → /2）时 Vue Router 复用这个组件，onMounted 不会再跑。
 * 不重新加载的话页面上还是 1 号，保存按 conf.id 发给 1 号；只 Object.assign 新数据也不够，
 * 2 号没配的字段会留着 1 号的值，保存凭证时一并写进 2 号。所以先清空再加载。
 */
watch(id, (next, prev) => {
  if (next === prev) return;
  resetConf();
  void loadDetail();
});

function resetConf() {
  loaded.value = false;
  const sink = conf as unknown as Record<string, unknown>;
  for (const key of Object.keys(sink)) delete sink[key];
  Object.assign(conf, { id: 0, channel_type: "telegram", name: "", enabled: true });
  tgForm.admin_users_text = "";
  tgForm.allowed_users_text = "";
  tgForm.default_chat_id_text = "";
  testResult.value = null;
}

async function loadDetail() {
  const seq = ++loadSeq;
  if (!id.value) {
    loading.value = false;
    ElMessage.error("无效的通道 ID");
    return;
  }
  loading.value = true;
  try {
    const data = await chatopsApi.notifications.get(id.value);
    if (seq !== loadSeq) return;
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
    loaded.value = true;
  } catch (e: unknown) {
    if (seq !== loadSeq) return;
    ElMessage.error((e as Error).message || "加载详情失败");
  } finally {
    if (seq === loadSeq) loading.value = false;
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
  if (locked.value || !formRef.value) return;
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
  if (locked.value || !credFormRef.value) return;
  const valid = await credFormRef.value.validate().catch(() => false);
  if (!valid) return;
  // 只出站的通道再按字段定义查一遍必填项（与新建对话框同一份规则）
  const missing = missingRequired(conf.channel_type, conf as unknown as Record<string, unknown>);
  if (missing) {
    ElMessage.warning(`请填写${missing.label}`);
    return;
  }

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
      default: {
        // 只出站的通道：字段来自 utils/notifyChannels，整份发出（没填的发空值，后端按空处理）
        const sink = payload as unknown as Record<string, unknown>;
        for (const f of outboundChannel(conf.channel_type)?.fields ?? []) {
          sink[f.key] = f.kind === "number" ? (confNum[f.key] ?? 0) : (confText[f.key] ?? "");
        }
      }
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
  if (locked.value) return;
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
    是页头右侧的动作。宽屏下卡宽停在 1080（.pt-cards--form）。
  -->
  <div v-loading="loading" class="notify-detail-page pt-cards pt-cards--wide pt-cards--form">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button @click="goBack">
        <PtIcon name="arrow-left" :size="15" /><span>通道列表</span>
      </el-button>
      <!-- 开关直接落库：详情页顶上的这一枚是「上线 / 下线」按钮，不是待保存的表单项 -->
      <label class="ctl">
        <el-switch
          v-model="conf.enabled"
          :loading="saving"
          :disabled="locked"
          data-testid="enable-switch"
          @change="handleSaveBasic" />
        <span>启用通道</span>
      </label>
    </Teleport>

    <!-- 画板 hero 1080×116：通道身份 —— 色块图标 + 名字 + 类型 + 运行状态 -->
    <!--
      data-card 是给验收用的身份标签：这块是卡片层的一员，但它没有 PtPanel 的标题，
      验收就没法认出它、删掉也不会红（review 实测这一点）。凡是卡片层里没有标题的块
      都要带上它，值用画板上的图层名。
    -->
    <section class="hero" data-card="hero">
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
          :disabled="locked"
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
              <!--
                原来写「留空表示允许所有人」「发非管理员命令」，两句都与代码不符：
                adapter/telegram/inbound.go 的 permitted 只放行两栏里出现的 user_id，两栏都空就谁都不放；
                管理员命令看的是绑定上的 PtAdmin，而 ConsumeCode 给每个新绑定都写 true。
              -->
              <div class="field-tip">
                这里和下面「管理员用户」里的 user_id
                才能与机器人对话；两栏都留空时，任何人发来的消息都会被拒绝
              </div>
            </el-form-item>
            <el-form-item label="管理员用户（admin_users）">
              <el-input
                v-model="tgForm.admin_users_text"
                type="textarea"
                :rows="2"
                placeholder="逗号分隔的 Telegram user_id，例如：123456789" />
              <div class="field-tip">
                与「允许用户」一样放行发消息；管理命令（<code>/pause</code> <code>/delete</code>
                等）目前对每个完成绑定的账号开放
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
              <el-form-item label="Access Token" prop="access_token">
                <el-input
                  v-model="conf.access_token"
                  type="password"
                  show-password
                  placeholder="OneBot access_token" />
                <div class="field-tip">
                  与 OneBot 实现里配置的 token 一致；监听地址不是 127.0.0.1 / localhost 时必填
                </div>
              </el-form-item>
            </div>

            <div class="field-head">谁可以用这个机器人</div>

            <el-form-item label="管理员 QQ（admin_qq_users）">
              <el-input
                v-model="conf.admin_qq_users"
                type="textarea"
                :rows="2"
                placeholder="逗号分隔的 QQ 号" />
              <!--
                两栏都是对话白名单：adapter/qq/inbound.go 的 isUserAllowed 只放行出现在其中一栏的号码，
                其余消息直接忽略。它们不决定管理员权限（看绑定上的 PtAdmin）；另外
                app/notification_service.go 的 qqTestChatID 把测试消息发给这一栏的第一个号码。
              -->
              <div class="field-tip">
                这里和下面「允许 QQ」里的号码才能与机器人对话；测试消息发给这里的第一个号码
              </div>
            </el-form-item>
            <el-form-item label="允许 QQ（allowed_qq_users）">
              <el-input
                v-model="conf.allowed_qq_users"
                type="textarea"
                :rows="2"
                placeholder="逗号分隔的 QQ 号" />
              <div class="field-tip">
                同样能与机器人对话；两栏都留空时，任何人发来的消息都会被忽略。管理命令目前对每个完成绑定的账号开放
              </div>
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

          <!-- 只出站的通道（Bark、Server 酱、ntfy、钉钉、飞书）：字段来自 utils/notifyChannels -->
          <template v-if="outboundChannel(conf.channel_type)">
            <el-form-item
              v-for="f in outboundChannel(conf.channel_type)!.fields"
              :key="f.key"
              :label="f.label"
              :prop="f.key">
              <el-select
                v-if="f.kind === 'select'"
                v-model="confText[f.key]"
                :placeholder="f.options?.[0]?.label"
                clearable
                :data-testid="`cred-${f.key}`">
                <el-option
                  v-for="o in f.options"
                  :key="o.value"
                  :label="o.label"
                  :value="o.value" />
              </el-select>
              <el-input-number
                v-else-if="f.kind === 'number'"
                v-model="confNum[f.key]"
                :min="f.min"
                :max="f.max"
                controls-position="right"
                :data-testid="`cred-${f.key}`" />
              <el-input
                v-else
                v-model="confText[f.key]"
                :type="f.kind === 'password' ? 'password' : 'text'"
                :show-password="f.kind === 'password'"
                :placeholder="f.placeholder"
                :data-testid="`cred-${f.key}`" />
              <div v-if="f.tip" class="field-tip">{{ f.tip }}</div>
            </el-form-item>
          </template>
        </div>
      </el-form>

      <template #footer>
        <span class="pt-foot-note">保存后会立即用新凭证重连通道</span>
        <el-button
          type="primary"
          :loading="saving"
          :disabled="locked"
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
            :disabled="locked"
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

    <!--
      画板 37（通道详情 · 凭证 tab）的两张说明卡：p-sec 700「密钥与安全」、
      p-map 364「channel_type 映射」。每句话都能在代码里核对：
        · 加密在 internal/crypto（AES-256-GCM），密钥来自 base64 的
          PT_TOOLS_SECRET_KEY 或 ~/.pt-tools/secret.key 里的十六进制文本；
        · 列表 DTO 整体脱敏 ConfigJSON，只有这个鉴权后的详情页会解密回显；
        · 映射表的四个取值来自各适配器的注册名（telegram / qq_onebot /
          webhook / wecom_webhook），必填项来自本文件的 credRules。
      画板那一版把四种类型做成 tab 并列展示，那是规格页的写法：一条已存在的通道类型不可改
      （页头那句「类型不可修改，换类型请删除后重建」），所以这里只画当前类型的表单，
      四种类型的差异改由这张映射卡交代。
    -->
    <!-- 画板 37 把这两张并排：p-sec 700 + p-map 364，所以套一层 .pt-cards--main -->
    <div class="pt-cards pt-cards--main">
      <PtPanel title="密钥与安全" icon="shield-check">
        <ul class="sec">
          <li>
            Bot Token、Access Token、HMAC Secret、Webhook Key 以及代理凭据都以
            <strong>AES-256-GCM</strong> 加密后落库，不存明文。
          </li>
          <li>
            列表接口整体脱敏 <code>config_json</code>，只有这一页（鉴权之后）会解密回显；
            日志与审计不输出明文。
          </li>
          <li>
            密钥取自 <code>PT_TOOLS_SECRET_KEY</code>（base64）或
            <code>~/.pt-tools/secret.key</code>（十六进制文本）。密钥缺失或轮换之后，
            旧凭证解不开，需要重新保存一次。
          </li>
        </ul>
      </PtPanel>

      <PtPanel title="channel_type 映射" icon="list-checks">
        <ul class="cmap">
          <li v-for="row in channelTypeMap" :key="row.type" class="cmap__row">
            <code>{{ row.type }}</code>
            <span class="cmap__label">{{ row.label }}</span>
            <span class="cmap__must">必填 {{ row.must }}</span>
          </li>
        </ul>
      </PtPanel>
    </div>
  </div>
</template>

<style scoped>
/* 提示文案卡：一行一条，左胶囊右场景 */
.msg-card {
  max-width: 700px;
}

/* 画板 37 的 p-sec 700 / p-map 364：栏宽由外面那层 .pt-cards--main 给 */
.sec {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding-left: var(--pt-space-4);
  font-size: var(--pt-fz-sm);
  line-height: 1.7;
  color: var(--pt-t2);
}

.cmap {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.cmap__row {
  display: grid;
  grid-template-columns: minmax(0, auto) minmax(0, 1fr);
  gap: 2px var(--pt-space-2);
  align-items: baseline;
  font-size: var(--pt-fz-label);
}

.cmap__row code {
  font-size: var(--pt-fz-label);
  color: var(--pt-t1);
}

.cmap__label {
  color: var(--pt-t2);
}

/* 必填项另起一行，靠左对齐到类型名下方 */
.cmap__must {
  grid-column: 1 / -1;
  color: var(--pt-t3);
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

/* 画板 23 的 hero name 是 19/700。之前写的 --pt-fz-h3 从来没定义过，整条声明失效，回落成 13 号 */
.hero__name {
  margin: 0;
  overflow: hidden;
  font-size: var(--pt-fz-h1);
  font-weight: 700;
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
