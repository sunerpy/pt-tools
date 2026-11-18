<script setup lang="ts">
import {
  ApiError,
  chatopsApi,
  type FeishuRegisterStart,
  type FeishuRegisterStatus,
  type NotificationConfig,
  type NotifyStatsView,
  type RecentNotification,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtBreakdown, { type BreakdownRow } from "@/components/ui/PtBreakdown.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { NOTIFY_SOURCE_LABELS } from "@/utils/notifySource";

import {
  OUTBOUND_CHANNELS,
  apiErrorDetail,
  outboundChannel,
  outboundFieldKeys,
  outboundProblem,
} from "@/utils/notifyChannels";

/*
 * 通道类型的图标与色相：四个通道在卡片网格里必须一眼分得开，
 * 但只用语义色（info / ok / warn / primary），不引入这一页专属的调色板 ——
 * 四套主题切换时自定义色号会失配。
 */
const channelTypeOptions = [
  { value: "telegram", label: "Telegram", icon: "send", color: "var(--pt-info)" },
  { value: "qq_onebot", label: "QQ (OneBot)", icon: "message-square", color: "var(--pt-ok)" },
  {
    value: "feishu_app",
    label: "飞书应用机器人",
    icon: "messages-square",
    color: "var(--pt-warn)",
  },
  { value: "webhook", label: "Webhook", icon: "link", color: "var(--pt-p)" },
  {
    value: "wecom_webhook",
    label: "WeCom Webhook",
    icon: "message-circle",
    color: "var(--pt-warn)",
  },
  ...OUTBOUND_CHANNELS.map((c) => ({
    value: c.type,
    label: c.label,
    icon: c.icon,
    color: c.color,
  })),
];

const router = useRouter();
/* ≤768 时外壳隐藏页头，页头动作得原地落回页面（Teleport 的 disabled） */
const isMobile = useIsMobile();
const notifications = ref<NotificationConfig[]>([]);

/*
 * 画板 22 的 bar-64 的三组控件。
 *
 * seg 是画板的四档。「已连接 / 异常」读的是 DTO 上的 runtime_state ——
 * 启动时逐条 Init，成功的留着实例、失败的只记日志跳过（cmd.initEnabledChannels），
 * 所以「启用了却没有实例在跑」就是异常。它不落库，重启即重算。
 */
const STATUS_SEG = [
  { label: "全部", value: "" },
  { label: "已连接", value: "connected" },
  { label: "异常", value: "error" },
  { label: "已停用", value: "off" },
] as const;

const statusSeg = ref<"" | "connected" | "error" | "off">("");

/*
 * 通道热重载是异步的（events.ConfigChanged → cmd.reloadChatOpsChannels 全量重建），
 * 改完开关立刻重取会拿到重建前的运行态。这个等待时长只影响「多久看到新状态」，
 * 拿不到也不会显示错的东西 —— 上面会先把这一行的 runtime_state 清掉。
 */
const reloadSettleMs = 1200;
/* 开关之后那一次延迟重取的定时器：离开页面时要清掉，否则会向已卸载的组件写状态 */
let settleTimer: number | undefined;
onBeforeUnmount(() => window.clearTimeout(settleTimer));

/** 后端没给 runtime_state 时退回到 enabled —— 问不到不等于坏了 */
function runtimeOf(n: NotificationConfig): "connected" | "running" | "error" | "disabled" | "" {
  if (n.runtime_state) return n.runtime_state;
  return n.enabled ? "" : "disabled";
}

/*
 * 「运行中」与「已连接」分开写，不是啰嗦：四个适配器的 Healthy() 都只代表构造/启动成功
 * （QQ 绑上端口就 true，而 NapCat 没握手时发送会明确失败），只有 QQ 能判断对端真的接上。
 * 把在跑的一律写成「已连接」会在界面上说一件不存在的事 —— 那是评审查出来的缺陷。
 */
const RUNTIME_LABEL: Record<string, string> = {
  connected: "已连接",
  running: "运行中",
  error: "异常",
  disabled: "已停用",
};
const typeFilter = ref("");

const CHANNEL_LABELS: Record<string, string> = {
  telegram: "Telegram",
  qq_onebot: "QQ (OneBot)",
  feishu_app: "飞书应用机器人",
  webhook: "Webhook",
  wecom_webhook: "WeCom Webhook",
  ...Object.fromEntries(OUTBOUND_CHANNELS.map((c) => [c.type, c.label])),
};

/** 画板右端两枚视图钮（bi-layout-grid / bi-rows-3），偏好存本地 */
const CH_VIEW_KEY = "pt-tools-channels-view-v1";

function loadChView(): "grid" | "rows" {
  try {
    return window.localStorage.getItem(CH_VIEW_KEY) === "rows" ? "rows" : "grid";
  } catch {
    return "grid";
  }
}

const viewMode = ref<"grid" | "rows">(loadChView());

function setView(v: "grid" | "rows") {
  viewMode.value = v;
  try {
    window.localStorage.setItem(CH_VIEW_KEY, v);
  } catch {
    /* 存不下就只在本次会话里生效 */
  }
}

/**
 * 六态（设计文档 §5）：以前加载失败只弹一个 toast，列表随后画成「还没有通知通道」，
 * 用户会以为通道被清空了，去重新配一遍。这一页没有筛选，所以不会出现 zero。
 */
const { loading, state, errorText, run, isStale } = useDataState();

/**
 * 画板 22 在四张通道卡之后还有三张：
 *   p-policy 548 投递策略（安静时段按通道配置，就在通道自己的字段里）
 *   p-stat 516  最近投递统计（按通道分组，读 `/api/chatops/notifications/stats`：近 7 天的 RSS 通知
 *               与后台监控通知合在一起算。以前只数 RSS 通知日志最近 50 条，签到、战报这些通知全不在里面）
 *   p-recent 1080 最近的通知记录（读 `/api/chatops/notifications/recent`：RSS 通知与后台通知合在一起的
 *               最近 8 条。以前读 RSS 通知日志的一页，后台通知不在里面）
 */
const recent = ref<RecentNotification[]>([]);
const recentFailed = ref(false);

async function loadRecent() {
  try {
    const res = await chatopsApi.notifications.recent(8);
    recent.value = res.items ?? [];
    recentFailed.value = false;
  } catch {
    /* 最近记录是附带信息，取不到就让卡里说明，不影响通道列表 */
    recent.value = [];
    recentFailed.value = true;
  }
}

/** 结果的叫法与胶囊色，和「最近投递统计」的小字同一套说法 */
const RESULT_VIEW: Record<
  RecentNotification["result"],
  { label: string; tone: "ok" | "dang" | "warn" | "neutral" }
> = {
  sent: { label: "已发出", tone: "ok" },
  failed: { label: "失败", tone: "dang" },
  pending: { label: "等待发送", tone: "neutral" },
  canceled: { label: "取消或被取代", tone: "neutral" },
  throttled: { label: "超出每小时上限", tone: "warn" },
};

function resultOf(r: string) {
  return RESULT_VIEW[r as RecentNotification["result"]] ?? { label: r, tone: "neutral" as const };
}

/** 来源：RSS 再写是「仅匹配的」还是「全部新种」 */
function sourceOf(n: RecentNotification): string {
  const base = SOURCE_LABELS[n.source] ?? n.source;
  if (n.source !== "rss") return base;
  return `${base} · ${n.kind === "filtered" ? "仅匹配的" : "全部新种"}`;
}

/** 内容：通知的内置标题（不含消息模板的改动）；RSS 前面加站点，没存下标题的写种子编号 */
function contentOf(n: RecentNotification): string {
  if (n.source !== "rss") return n.title || SOURCE_LABELS[n.source] || n.source;
  const what = n.title || `种子 ${n.torrent_id ?? ""}`.trim();
  return n.site ? `${n.site} · ${what}` : what;
}

const policyRows = computed(() =>
  notifications.value.map((n) => {
    const start = n.quiet_hours_start ?? "";
    const end = n.quiet_hours_end ?? "";
    const on = Boolean(start && end);
    return {
      id: n.id,
      name: n.name,
      enabled: n.enabled,
      quiet: on ? `${start} – ${end}` : "未设置",
      crossesMidnight: on && start > end,
    };
  }),
);

const stats = ref<NotifyStatsView | null>(null);

async function loadStats() {
  try {
    stats.value = await chatopsApi.notifications.stats();
  } catch {
    /* 统计是附带信息，取不到就让卡里说明，不影响通道列表 */
    stats.value = null;
  }
}

/** 通知来源的叫法：RSS 以外都是后台监控发的（与操作审计里的叫法同一份） */
const SOURCE_LABELS = NOTIFY_SOURCE_LABELS;

/**
 * p-stat：每个通道近 7 天发了多少；小字写来源，再写没发出的是失败、在等、取消还是超出上限，
 * 最后写 RSS 通知转进重发队列以后的结果（「已发出」里含这些，补发没成的要单独看得见）
 */
const statRows = computed<BreakdownRow[]>(() =>
  (stats.value?.channels ?? []).map((ch) => {
    const conf = notifications.value.find((n) => n.id === ch.conf_id);
    const parts: string[] = [];
    if (ch.sources.length > 0) {
      parts.push(
        ch.sources
          .map((src) => `${SOURCE_LABELS[src.source] ?? src.source} ${src.total}`)
          .join(" · "),
      );
    }
    const left: string[] = [];
    if (ch.failed > 0) left.push(`${ch.failed} 条失败`);
    if (ch.pending > 0) left.push(`${ch.pending} 条等待发送`);
    if (ch.canceled > 0) left.push(`${ch.canceled} 条取消或被取代`);
    if (ch.throttled > 0) left.push(`${ch.throttled} 条超出每小时上限`);
    const rest = ch.total - ch.sent - ch.failed - ch.pending - ch.canceled - ch.throttled;
    if (rest > 0) left.push(`${rest} 条没发出`);
    if (ch.total > 0) parts.push(left.length > 0 ? left.join("、") : "全部发出");
    const q = ch.requeued;
    if (q.total > 0) {
      const done: string[] = [];
      if (q.sent > 0) done.push(`${q.sent} 条已补发`);
      if (q.pending > 0) done.push(`${q.pending} 条重试中`);
      if (q.dead > 0) done.push(`${q.dead} 条放弃`);
      parts.push(
        `${q.total} 条实时没发出、转入重发${done.length > 0 ? `：${done.join("、")}` : ""}`,
      );
    }
    return {
      key: String(ch.conf_id),
      label: conf ? conf.name : `已删除的通道 #${ch.conf_id}`,
      value: `${ch.sent} / ${ch.total}`,
      weight: ch.total,
      tone: ch.failed > 0 || q.dead > 0 ? ("warn" as const) : ("ok" as const),
      hint: parts.join("；"),
    };
  }),
);

const statFoot = computed(
  () =>
    `口径是「已发出 / 总条数」，近 ${stats.value?.days ?? 7} 天：RSS 通知和保号、签到、战报、失效种子、订阅与入库、远程访问这些后台通知都算，` +
    "通道测试和命令回复不算。RSS 通知实时发不出去会转入重发队列，也算已发出，补发的结果写在小字里。",
);

/** 画板 head 的 sub（11.5/400 t3）：共几个通道、几个启用、都是什么类型 */
/** 表里这一批通道 = 状态筛 + 类型筛（两条都来自画板 bar-64） */
const shownChannels = computed(() =>
  notifications.value.filter((n) => {
    if (statusSeg.value === "off" && n.enabled) return false;
    if (statusSeg.value === "connected" && runtimeOf(n) !== "connected") return false;
    if (statusSeg.value === "error" && runtimeOf(n) !== "error") return false;
    if (typeFilter.value && n.channel_type !== typeFilter.value) return false;
    return true;
  }),
);

const headSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return "通知通道没加载出来";
  if (notifications.value.length === 0) return "";
  const on = notifications.value.filter((n) => n.enabled).length;
  const kinds = [...new Set(notifications.value.map((n) => getChannelLabel(n.channel_type)))];
  return `${notifications.value.length} 个通道 · ${on} 个启用 · ${kinds.join(" / ")}`;
});

/** 状态块副标题：失败给真实错误，空态给下一步动作 */
const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  if (state.value === "loading") return "正在读取通知通道";
  return "接上 Telegram / QQ / 飞书 / Webhook / 企业微信，就能收到任务结果与告警，也能反过来发命令";
});

const addDialogVisible = ref(false);
const submitting = ref(false);

/**
 * 新建表单的初值：只有通用字段与 Telegram / Webhook / 企业微信 / 飞书应用机器人的必填项，只出站通道的字段按需写入。
 * 飞书的平台默认 feishu：Element Plus 2.14.3 的选择框不认 null。
 */
function blankChannel(type = "telegram", name = ""): Partial<NotificationConfig> {
  return {
    channel_type: type,
    name,
    enabled: true,
    bot_token: "",
    endpoint_url: "",
    webhook_key: "",
    app_id: "",
    app_secret: "",
    domain: "feishu",
  };
}

const newChannel = ref<Partial<NotificationConfig>>(blankChannel());

/*
 * 飞书应用机器人的两种建法：扫码一键创建（默认）在扫码人的企业里建好应用，后端直接建通道并把扫码人绑定；
 * 手动填写 App ID 与 App Secret 和以前一样。
 */
const FEISHU_MODES = [
  { label: "扫码一键创建（推荐）", value: "scan" },
  { label: "填写 App ID 与 App Secret", value: "manual" },
];
const FEISHU_REG_DEFAULT_NAME = "飞书应用机器人";
const feishuMode = ref<"scan" | "manual">("scan");
const isFeishuScan = computed(
  () => newChannel.value.channel_type === "feishu_app" && feishuMode.value === "scan",
);

/*
 * 换类型时清掉上一个类型填的字段（只留名称）：钉钉和飞书都有 webhook_url，留着会带着错的地址创建。
 * 选飞书应用机器人时名称没填就给默认的「飞书应用机器人」，换走时把这个默认名去掉。
 */
watch(
  () => newChannel.value.channel_type,
  (type, prev) => {
    if (type === prev) return;
    let name = newChannel.value.name ?? "";
    if (type === "feishu_app" && !name) name = FEISHU_REG_DEFAULT_NAME;
    else if (prev === "feishu_app" && name === FEISHU_REG_DEFAULT_NAME) name = "";
    newChannel.value = blankChannel(type, name);
  },
);

/*
 * 扫码一键创建的这一次。编号只放在组件的内存里，不进地址栏与浏览器存储：页面刷新或关掉对话框以后
 * 不接着原来的二维码（后端那一个留在等扫码，直到过期或被新开的取代），要重新点「生成二维码」。
 */
const reg = ref<FeishuRegisterStart | null>(null);
const regStatus = ref<FeishuRegisterStatus | null>(null);
/** 查状态回 404：被别处新开的取代，或者结束太久已经清掉 */
const regGone = ref(false);
const regStarting = ref(false);
/** 这次对话框里生成过几次二维码：第二次起写明之前的已经作废 */
const regRounds = ref(0);
/** 关对话框、离开页面时加一：在路上的「开始」回来时发现换了一轮，就把它取消 */
let regSession = 0;
let regTimer: ReturnType<typeof setInterval> | undefined;
let regPolling = false;
const REG_POLL_MS = 2000;

const REG_STATES: Record<string, { label: string; tone: "ok" | "warn" | "dang" | "info" }> = {
  pending: { label: "等待扫码与确认", tone: "info" },
  saving: { label: "正在保存通道", tone: "info" },
  done: { label: "已建好", tone: "ok" },
  denied: { label: "已在飞书里取消", tone: "warn" },
  expired: { label: "已过期", tone: "warn" },
  canceled: { label: "已取消", tone: "warn" },
  failed: { label: "没有建成", tone: "dang" },
  gone: { label: "已作废", tone: "dang" },
};

const regState = computed(() => (regGone.value ? "gone" : (regStatus.value?.state ?? "pending")));
/** 还在等飞书或正在保存：这时不换类型、不改名称（名称在开始时就发出去了），点遮罩也不关对话框 */
const regBusy = computed(
  () => !!reg.value && (regState.value === "pending" || regState.value === "saving"),
);
/** 结束了但没建成：按钮换成「重新生成」 */
const regEnded = computed(() => !!reg.value && !regBusy.value);
const regReason = computed(() => {
  if (!reg.value || regBusy.value) return "";
  if (regGone.value) return "这个一键创建已经被新的取代或过期";
  return regStatus.value?.message ?? "";
});
const regQrSrc = computed(() =>
  reg.value ? `data:image/svg+xml;charset=utf-8,${encodeURIComponent(reg.value.qr_svg)}` : "",
);
/** 「N 分钟内有效」：按开始时给的过期时间算一次 */
const regMinutes = computed(() => {
  if (!reg.value) return 0;
  const ms = new Date(reg.value.expires_at).getTime() - Date.now();
  return Number.isFinite(ms) ? Math.max(1, Math.round(ms / 60_000)) : 10;
});

function stopRegPoll() {
  if (regTimer) clearInterval(regTimer);
  regTimer = undefined;
}

async function startFeishuRegister() {
  const name = (newChannel.value.name ?? "").trim() || FEISHU_REG_DEFAULT_NAME;
  const session = regSession;
  stopRegPoll();
  regStarting.value = true;
  try {
    const started = await chatopsApi.feishuRegister.start(name);
    if (session !== regSession) {
      // 等二维码的时候对话框关了：没人扫得到它，取消
      await chatopsApi.feishuRegister.cancel(started.id).catch(() => undefined);
      return;
    }
    reg.value = started;
    regStatus.value = { id: started.id, state: started.state, expires_at: started.expires_at };
    regGone.value = false;
    regRounds.value += 1;
    regTimer = setInterval(() => void pollRegister(), REG_POLL_MS);
  } catch (e: unknown) {
    ElMessage.error(apiErrorDetail(e, "生成二维码失败"));
  } finally {
    regStarting.value = false;
  }
}

/** 每 2 秒按编号问一次（上一次还没回来就跳过） */
async function pollRegister() {
  const cur = reg.value;
  if (!cur || regPolling) return;
  regPolling = true;
  try {
    const st = await chatopsApi.feishuRegister.status(cur.id);
    if (reg.value?.id !== cur.id) return; // 关掉了，或者已经换成新的一个
    regStatus.value = st;
    if (st.state === "done") {
      stopRegPoll();
      await finishRegister(st);
    } else if (st.state !== "pending" && st.state !== "saving") {
      stopRegPoll();
    }
  } catch (e: unknown) {
    if (reg.value?.id !== cur.id) return;
    if (e instanceof ApiError && e.status === 404) {
      regGone.value = true;
      stopRegPoll();
    }
    // 别的错误（网络抖动）：下一次再问
  } finally {
    regPolling = false;
  }
}

/** 建好了：提示、关掉对话框、刷新列表并打开这个通道的详情页 */
async function finishRegister(st: FeishuRegisterStatus) {
  if (st.message) {
    // 带提示时是绑定没成之类：原文写明要去绑定页手动绑定，多留一会儿
    ElMessage({ type: "warning", message: st.message, duration: 10_000, showClose: true });
  } else {
    ElMessage.success("已经建好通道，正在连接飞书");
  }
  // 先清掉：关对话框时不取消已经建好的这一个
  reg.value = null;
  regStatus.value = null;
  addDialogVisible.value = false;
  await loadNotifications();
  if (st.conf_id) router.push(`/chatops/notifications/${st.conf_id}`);
}

/**
 * 不再看这一个（关掉对话框、离开页面、换走扫码模式）：停止轮询；还在等的按编号取消
 * （saving 以后取消不起作用，后端照常建完）。notify 为真时写明二维码作废。
 */
async function dropRegister(notify: boolean) {
  regSession += 1;
  stopRegPoll();
  const cur = reg.value;
  const busy = regBusy.value;
  const pending = regState.value === "pending";
  reg.value = null;
  regStatus.value = null;
  regGone.value = false;
  regRounds.value = 0;
  if (!cur || !busy) return;
  try {
    await chatopsApi.feishuRegister.cancel(cur.id);
  } catch {
    /* 十分钟以后自己过期 */
  }
  if (notify && pending) {
    ElMessage.info("二维码已经作废，不要再扫；已经扫了的话，请到飞书开发者后台删掉多出来的应用");
  }
}

watch(isFeishuScan, (on) => {
  if (!on && reg.value) void dropRegister(true);
});

onBeforeUnmount(() => void dropRegister(false));

onMounted(async () => {
  await Promise.all([loadNotifications(), loadRecent(), loadStats()]);
});

async function loadNotifications() {
  const pending = run(() => chatopsApi.notifications.list());
  const data = await pending;
  if (isStale(pending)) return;
  if (!data) {
    // 失败时清空：留着旧列表配一个「加载失败」更让人误解
    notifications.value = [];
    return;
  }
  notifications.value = data;
}

/** 只出站通道的字段不在 newChannel 的初值里，按字段名读写 */
function newField(key: string): unknown {
  return (newChannel.value as Record<string, unknown>)[key];
}
function setNewField(key: string, v: unknown) {
  (newChannel.value as Record<string, unknown>)[key] = v;
}

function openAddDialog() {
  if (reg.value) void dropRegister(true);
  newChannel.value = blankChannel();
  feishuMode.value = "scan";
  addDialogVisible.value = true;
}

/** 各类型新建时要发的字段：只发当前类型的，换过类型也不会把别的类型的值带进配置 */
const CREATE_FIELDS: Record<string, (keyof NotificationConfig)[]> = {
  telegram: ["bot_token"],
  webhook: ["endpoint_url"],
  wecom_webhook: ["webhook_key"],
  qq_onebot: [],
  feishu_app: ["app_id", "app_secret", "domain"],
};

function createBody(): Omit<NotificationConfig, "id"> {
  const c = newChannel.value as Record<string, unknown>;
  const type = newChannel.value.channel_type ?? "";
  const body: Record<string, unknown> = {
    channel_type: type,
    name: newChannel.value.name,
    enabled: newChannel.value.enabled,
  };
  const keys = outboundChannel(type) ? outboundFieldKeys(type) : (CREATE_FIELDS[type] ?? []);
  for (const k of keys) {
    const v = typeof c[k] === "string" ? (c[k] as string).trim() : c[k];
    if (v !== undefined && v !== "") body[k] = v;
  }
  return body as unknown as Omit<NotificationConfig, "id">;
}

async function handleCreate() {
  if (!newChannel.value.name) {
    ElMessage.warning("请填写通道名称");
    return;
  }

  if (newChannel.value.channel_type === "telegram" && !newChannel.value.bot_token) {
    ElMessage.warning("Telegram 通道需填写 Bot Token");
    return;
  }
  if (
    newChannel.value.channel_type === "feishu_app" &&
    (!newChannel.value.app_id?.trim() || !newChannel.value.app_secret?.trim())
  ) {
    ElMessage.warning("飞书应用机器人通道需填写 App ID 与 App Secret");
    return;
  }
  const problem = outboundProblem(
    newChannel.value.channel_type,
    newChannel.value as Record<string, unknown>,
  );
  if (problem) {
    ElMessage.warning(problem);
    return;
  }

  submitting.value = true;
  try {
    await chatopsApi.notifications.create(createBody());
    ElMessage.success("添加成功");
    addDialogVisible.value = false;
    await loadNotifications();
  } catch (e: unknown) {
    // 配置没通过后端检查时，detail 写明哪一项不对
    ElMessage.error(apiErrorDetail(e, "添加失败"));
  } finally {
    submitting.value = false;
  }
}

async function handleToggle(row: NotificationConfig) {
  try {
    await chatopsApi.notifications.update(row.id, { enabled: row.enabled });
    ElMessage.success(`${row.enabled ? "已启用" : "已停用"} ${row.name}`);
    /*
     * 运行态必须跟着走一遍，否则界面会拿旧值继续说话：停掉一个「已连接」的通道之后
     * 胶囊仍写「已连接」，启用一个停用的通道之后仍写「已停用」，直到手工刷新。
     *
     * 先就地清掉这一行的 runtime_state（胶囊退回按开关说话，不再声称连通性），
     * 再等热重载落地后重取一次 —— 通道是由 events.ConfigChanged（source=notification）
     * 异步全量重建的（cmd.reloadChatOpsChannels），立刻重取会拿到重建前的状态。
     */
    row.runtime_state = undefined;
    window.clearTimeout(settleTimer);
    settleTimer = window.setTimeout(() => {
      void loadNotifications();
    }, reloadSettleMs);
  } catch (e: unknown) {
    row.enabled = !row.enabled; // 回滚开关，避免界面和后端不一致
    ElMessage.error((e as Error).message || "操作失败");
  }
}

function handleEdit(row: NotificationConfig) {
  router.push(`/chatops/notifications/${row.id}`);
}

async function handleTest(row: NotificationConfig) {
  try {
    await chatopsApi.notifications.test(row.id);
    ElMessage.success("测试消息已触发");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "测试失败");
  }
}

async function handleDelete(row: NotificationConfig) {
  try {
    await ElMessageBox.confirm(
      `确定要删除通知通道 "${row.name}" 吗？此操作不可恢复。`,
      "删除通道",
      {
        type: "warning",
        confirmButtonText: "确定删除",
        cancelButtonText: "取消",
      },
    );

    await chatopsApi.notifications.delete(row.id);
    ElMessage.success("已删除");
    await loadNotifications();
  } catch (e) {
    if (e !== "cancel") {
      ElMessage.error((e as Error).message || "删除失败");
    }
  }
}

function channelMeta(type: string) {
  return channelTypeOptions.find((o) => o.value === type);
}

function channelNameOf(id: number): string {
  const conf = notifications.value.find((n) => n.id === id);
  return conf ? conf.name : `#${id}`;
}

function formatWhen(s?: string): string {
  if (!s) return "-";
  try {
    return new Date(s).toLocaleString("zh-CN", { hour12: false });
  } catch {
    return s;
  }
}

function getChannelIcon(type: string) {
  return channelMeta(type)?.icon || "bell";
}

function getChannelColor(type: string) {
  return channelMeta(type)?.color || "var(--pt-t3)";
}

function getChannelLabel(type: string) {
  return channelMeta(type)?.label || type;
}
</script>

<template>
  <!--
    画板 22：head 64 → bar-64（40）→ nt0..nt3 四张通道卡（548 / 516 两栏）。
    刷新与添加是页头右侧那两枚 32 高的按钮；工具栏带在画板上留给通道筛选，
    本项目通道数量个位数，不需要筛选，所以带上只挂一行覆盖率说明。
  -->
  <div class="notifications-page">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button :loading="loading" @click="loadNotifications">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
      <el-button type="primary" data-testid="add-channel-btn" @click="openAddDialog">
        <PtIcon name="plus" :size="15" /><span>添加通道</span>
      </el-button>
    </Teleport>

    <!--
      画板 22 的 bar-64：seg（全部/已连接/异常/已停用）+ chip「类型: 全部」+ 右端两枚视图钮。
      这条带此前**整条没有**（注释里写的是「通道数量个位数，不需要筛选」）——
      但类型筛选与两种视图是画板画的，通道多起来（四个以上）就用得上。

      「已连接 / 异常」两档读 DTO 上的 runtime_state。这一条曾按「后端没有这个信号」
      记成偏离 —— 其实有：启动时逐条 Init，成功的留着实例在 map 里，失败的只记日志
      然后跳过，所以「启用了却不在 map 里」正是异常的样子。
    -->
    <PtToolbar v-if="notifications.length > 0" band>
      <el-segmented
        v-model="statusSeg"
        class="pt-seg"
        :options="STATUS_SEG"
        :props="{ label: 'label', value: 'value' }"
        data-testid="channels-status-seg" />
      <el-select
        v-model="typeFilter"
        class="ch-chip"
        size="small"
        placeholder="类型: 全部"
        clearable
        data-testid="channels-type-filter">
        <el-option label="类型: 全部" value="" />
        <el-option
          v-for="(label, value) in CHANNEL_LABELS"
          :key="value"
          :label="`类型: ${label}`"
          :value="value" />
      </el-select>

      <template #right>
        <el-tooltip content="卡片视图" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            :class="{ 'is-active': viewMode === 'grid' }"
            aria-label="卡片视图"
            :aria-pressed="viewMode === 'grid'"
            data-testid="channels-view-grid"
            @click="setView('grid')">
            <PtIcon name="layout-grid" :size="15" />
          </button>
        </el-tooltip>
        <el-tooltip content="紧凑列表" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            :class="{ 'is-active': viewMode === 'rows' }"
            aria-label="紧凑列表"
            :aria-pressed="viewMode === 'rows'"
            data-testid="channels-view-rows"
            @click="setView('rows')">
            <PtIcon name="rows-3" :size="15" />
          </button>
        </el-tooltip>
      </template>

      <template #note>{{ shownChannels.length }} / {{ notifications.length }} 个通道</template>
    </PtToolbar>

    <PtDataState
      v-if="notifications.length === 0"
      class="ch-state"
      :state="state"
      :title="state === 'empty' ? '还没有通知通道' : ''"
      :sub="stateSub">
      <template v-if="state === 'error'" #action>
        <el-button @click="loadNotifications">
          <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
        </el-button>
      </template>
      <template v-else-if="state === 'empty'" #action>
        <el-button type="primary" @click="openAddDialog">
          <PtIcon name="plus" :size="14" /><span>添加第一个通道</span>
        </el-button>
      </template>
    </PtDataState>

    <div
      v-else
      class="ch-grid pt-cards"
      :class="viewMode === 'rows' ? 'pt-cards--wide is-rows' : 'pt-cards--2'">
      <article
        v-for="item in shownChannels"
        :key="item.id"
        class="ch-card"
        :class="{ 'is-off': !item.enabled }"
        :data-testid="`channel-card-${item.name}`">
        <header class="ch-card__head">
          <span class="ch-card__icon" :style="{ '--ch-c': getChannelColor(item.channel_type) }">
            <PtIcon :name="getChannelIcon(item.channel_type)" :size="16" />
          </span>
          <span class="ch-card__name">{{ item.name }}</span>
          <el-switch v-model="item.enabled" size="small" @change="handleToggle(item)" />
        </header>

        <div class="ch-card__meta">
          <PtTag>{{ getChannelLabel(item.channel_type) }}</PtTag>
          <!--
            状态胶囊说的是**实时运行态**，不是「配置里开着没开」：
            启用了却没起来的那条必须显示异常，写「运行中」是一句假话。
          -->
          <PtStatusPill
            :tone="
              runtimeOf(item) === 'connected'
                ? 'ok'
                : runtimeOf(item) === 'error'
                  ? 'dang'
                  : 'neutral'
            "
            size="sm">
            {{ RUNTIME_LABEL[runtimeOf(item)] ?? (item.enabled ? "运行中" : "已停用") }}
          </PtStatusPill>
        </div>

        <footer class="ch-card__acts">
          <el-button size="small" @click="handleEdit(item)">
            <PtIcon name="settings" :size="14" /><span>设置</span>
          </el-button>
          <el-button size="small" :disabled="!item.enabled" @click="handleTest(item)">
            <PtIcon name="send" :size="14" /><span>发测试消息</span>
          </el-button>
          <el-button
            link
            type="danger"
            size="small"
            class="ch-card__del"
            @click="handleDelete(item)">
            <PtIcon name="trash-2" :size="14" /><span>删除</span>
          </el-button>
        </footer>
      </article>

      <!--
        画板 22 的后三张卡：p-policy 548（投递策略）/ p-stat 516（最近投递统计）
        / p-recent 1080（最近记录）。统计读近 7 天的投递统计，记录读最近 8 条，两者都含 RSS 与后台监控通知。
      -->
      <PtPanel title="投递策略" icon="moon" :count="`${policyRows.length} 个通道`">
        <ul class="pol">
          <li v-for="row in policyRows" :key="row.id" class="pol__row">
            <span class="pol__k">{{ row.name }}</span>
            <span class="pol__v" :class="{ 'is-off': row.quiet === '未设置' }">{{
              row.quiet
            }}</span>
            <span v-if="row.crossesMidnight" class="pol__tag">跨午夜</span>
            <span v-if="!row.enabled" class="pol__tag pol__tag--off">已停用</span>
          </li>
        </ul>
        <p class="pol__foot">
          安静时段里的通知先挂起（<code>pending</code>），到时段结束再发；每条 RSS
          还能单独设「每小时最多几条」，超出的记为 <code>throttled</code>、不再补发。
        </p>
      </PtPanel>

      <PtPanel
        title="最近投递统计"
        icon="chart-pie"
        :count="stats ? `近 ${stats.days} 天` : '暂无'"
        data-testid="notify-stats">
        <PtBreakdown v-if="statRows.length > 0" :rows="statRows" :foot="statFoot" />
        <p v-else class="pol__empty">
          {{ stats ? `近 ${stats.days} 天没有投递记录。` : "投递统计没取到。" }}
        </p>
      </PtPanel>

      <PtPanel
        class="pt-cards__full"
        title="最近的通知"
        icon="history"
        :count="`${recent.length} 条`"
        data-testid="notify-recent">
        <!--
          画板 22 的 p-recent 是一张**带表头的小表**（时间 / 通道 / 事件 / 站点 / 结果 /
          延迟 / 重试，正文 12.5、只有「结果」是胶囊），不是一排 chip。
          落地此前是无列名的 chip 串：一行里三枚色块，而且看不出哪个字段是什么。
          「延迟」那列没有对应数据（通知日志不记投递耗时），所以不画 —— 不造数。
          后台通知没有站点与种子，「事件 / 站点」两列落成「来源 / 内容」：内容是通知的内置标题
          （投递以前存下的；配了消息模板时实际收到的标题按模板，几条 RSS 合并发出时这里仍逐条列出）。
        -->
        <!--
          窄屏降级成列表，不做横向滚动表格 —— 这是设计文档 §9 的四方向共用规则，
          画板 33 给这一块的形态也正是「圆点 6 + 双行文本 + 右侧时间 + 分隔线」、节奏 54。
          上一版在所有视口都渲染这张七列表（单元格还都 nowrap），375 下右边两列直接被裁掉：
          外层是 overflow hidden，滚都滚不出来。一次评审查出这点，判得对。
        -->
        <ul v-if="isMobile && recent.length > 0" class="recm">
          <li v-for="n in recent" :key="`${n.source}-${n.log_id}`" class="recm__row">
            <span class="recm__dot" :class="`is-${resultOf(n.result).tone}`" aria-hidden="true" />
            <span class="recm__body">
              <span class="recm__l1">{{ contentOf(n) }}</span>
              <span class="recm__l2">
                {{ resultOf(n.result).label }} · {{ channelNameOf(n.conf_id) }} · {{ sourceOf(n)
                }}<template v-if="n.attempts > 1"> · 第 {{ n.attempts }} 次</template>
              </span>
              <span v-if="n.error && n.result !== 'sent'" class="recm__err">{{ n.error }}</span>
            </span>
            <span class="recm__when">{{ formatWhen(n.created_at) }}</span>
          </li>
        </ul>

        <table v-else-if="recent.length > 0" class="rec">
          <thead>
            <tr>
              <th>时间</th>
              <th>通道</th>
              <th>来源</th>
              <th>内容</th>
              <th>结果</th>
              <th class="rec__num">尝试</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="n in recent" :key="`${n.source}-${n.log_id}`">
              <td class="rec__when">{{ formatWhen(n.created_at) }}</td>
              <td>{{ channelNameOf(n.conf_id) }}</td>
              <td>{{ sourceOf(n) }}</td>
              <td>
                <span class="rec__what" :title="contentOf(n)">{{ contentOf(n) }}</span>
              </td>
              <td>
                <!-- 没发出去的写上原因（最近一次失败）：悬停看全文，手机列表里直接铺一行 -->
                <el-tooltip
                  :content="n.error"
                  placement="top"
                  :disabled="!n.error || n.result === 'sent'">
                  <PtStatusPill
                    :tone="resultOf(n.result).tone"
                    size="sm"
                    :title="n.error && n.result !== 'sent' ? n.error : undefined">
                    {{ resultOf(n.result).label }}
                  </PtStatusPill>
                </el-tooltip>
              </td>
              <td class="rec__num">{{ n.attempts }}</td>
            </tr>
          </tbody>
        </table>
        <p v-else class="pol__empty">
          {{ recentFailed ? "最近的通知没取到。" : "还没有通知记录。" }}
        </p>
        <p class="pol__foot">
          RSS 通知的完整清单、重试与取消在
          <el-button link type="primary" @click="router.push('/chatops/rss-notifications')">
            RSS 通知日志
          </el-button>
          里；后台通知发送失败会在 1、5、30
          分钟后各重试一次。「内容」是通知的内置标题：配了消息模板时实际收到的标题按模板，几条 RSS
          通知合并成一条发出时这里仍逐条列出。
        </p>
      </PtPanel>
    </div>

    <!-- 扫码一键创建在等扫码时，点遮罩不关对话框：关掉会让二维码作废 -->
    <el-dialog
      v-model="addDialogVisible"
      class="pt-dialog"
      title="添加通知通道"
      :width="isMobile ? '94%' : '520px'"
      align-center
      :close-on-click-modal="!regBusy"
      data-testid="add-channel-dialog"
      @closed="dropRegister(true)">
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <el-form-item label="通道类型">
          <el-select v-model="newChannel.channel_type" style="width: 100%" :disabled="regBusy">
            <el-option
              v-for="opt in channelTypeOptions"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
              :data-testid="`channel-type-${opt.value}`" />
          </el-select>
        </el-form-item>

        <el-form-item v-if="newChannel.channel_type === 'feishu_app'" label="创建方式">
          <el-segmented
            v-model="feishuMode"
            class="pt-seg fsmode"
            :options="FEISHU_MODES"
            :props="{ label: 'label', value: 'value' }"
            :disabled="regBusy"
            :block="isMobile"
            data-testid="feishu-mode" />
        </el-form-item>

        <el-form-item label="通道名称" required>
          <el-input
            v-model="newChannel.name"
            placeholder="例如：我的 Telegram 机器人"
            :disabled="regBusy"
            data-testid="name-input" />
          <div class="field-tip">只用于在列表里区分通道，随时可改</div>
        </el-form-item>

        <div class="field-rule"></div>

        <el-form-item v-if="newChannel.channel_type === 'telegram'" label="Bot Token" required>
          <el-input
            v-model="newChannel.bot_token"
            type="password"
            show-password
            placeholder="123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
            data-testid="bot-token-input" />
          <div class="field-tip">找 <code>@BotFather</code> 创建机器人后拿到的 token</div>
        </el-form-item>

        <el-form-item v-if="newChannel.channel_type === 'webhook'" label="Endpoint URL" required>
          <el-input v-model="newChannel.endpoint_url" placeholder="https://..." />
          <div class="field-tip">pt-tools 会向这个地址 POST JSON</div>
        </el-form-item>

        <el-form-item
          v-if="newChannel.channel_type === 'wecom_webhook'"
          label="Webhook Key"
          required>
          <el-input v-model="newChannel.webhook_key" placeholder="企业微信群机器人的 key" />
          <div class="field-tip">群机器人地址里 <code>key=</code> 后面那一段</div>
        </el-form-item>

        <!-- 扫码一键创建：二维码、在浏览器里打开、有效期与状态；链接与二维码只在开始时给一次 -->
        <template v-if="isFeishuScan">
          <div v-if="reg" class="fsreg" data-testid="feishu-reg-qr">
            <img
              class="fsreg__qr"
              :class="{ 'fsreg__qr--used': !regBusy }"
              :src="regQrSrc"
              alt="一键创建飞书应用机器人的二维码，用飞书扫描"
              width="240"
              height="240" />
            <div class="fsreg__state">
              <PtStatusPill :tone="REG_STATES[regState]!.tone" dot data-testid="feishu-reg-state">{{
                REG_STATES[regState]!.label
              }}</PtStatusPill>
              <span v-if="regBusy" class="fsreg__sub">二维码 {{ regMinutes }} 分钟内有效</span>
            </div>
            <p v-if="regReason" class="fsreg__reason" data-testid="feishu-reg-reason">
              {{ regReason }}
            </p>
            <a
              v-if="regBusy"
              class="fsreg__open"
              :href="reg.url"
              target="_blank"
              rel="noopener noreferrer"
              data-testid="feishu-reg-open">
              <PtIcon name="external-link" :size="14" /><span>在浏览器里打开</span>
            </a>
            <div v-if="regRounds > 1 || regGone" class="pt-note pt-note--warn fsreg__void">
              <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
              <span>之前的二维码已经作废，不要再扫；扫了的话请到开发者后台删掉多出来的应用。</span>
            </div>
          </div>
          <div class="pt-note" data-testid="feishu-reg-intro">
            <PtIcon name="info" :size="14" class="pt-note__icon" />
            <span>
              用要接 pt-tools
              的那个企业的飞书账号扫码；应用名在飞书的确认页上可以改；建好以后不用发布版本；
              扫码的人自动成为管理员并直接绑定；凭证和别的通道一样，只在通道详情页里看得到。步骤见
              <a
                href="https://github.com/sunerpy/pt-tools/blob/main/docs/guide/chatops-feishu.md#扫码一键创建推荐"
                target="_blank"
                rel="noopener noreferrer"
                >飞书应用机器人文档</a
              >。
            </span>
          </div>
        </template>

        <template v-if="newChannel.channel_type === 'feishu_app' && feishuMode === 'manual'">
          <el-form-item label="App ID" required>
            <el-input v-model="newChannel.app_id" placeholder="cli_…" data-testid="new-app_id" />
            <div class="field-tip">开发者后台「凭证与基础信息」里的 App ID</div>
          </el-form-item>
          <el-form-item label="App Secret" required>
            <el-input
              v-model="newChannel.app_secret"
              type="password"
              show-password
              data-testid="new-app_secret" />
          </el-form-item>
          <el-form-item label="平台">
            <el-select v-model="newChannel.domain" style="width: 100%" data-testid="new-domain">
              <el-option label="飞书" value="feishu" data-testid="domain-feishu" />
              <el-option label="Lark 国际版" value="lark" data-testid="domain-lark" />
            </el-select>
          </el-form-item>
        </template>

        <!-- 只出站的通道：新建时就填全部字段，自建服务器的地址与鉴权不用等创建后再补 -->
        <template v-for="f in outboundChannel(newChannel.channel_type)?.fields ?? []" :key="f.key">
          <el-form-item :label="f.label" :required="f.required">
            <el-switch
              v-if="f.kind === 'switch'"
              :model-value="newField(f.key) === true"
              :data-testid="`new-${f.key}`"
              @update:model-value="
                (v: string | number | boolean) => setNewField(f.key, v === true)
              " />
            <el-select
              v-else-if="f.kind === 'select'"
              :model-value="newField(f.key) as string | undefined"
              :placeholder="f.options?.[0]?.label"
              clearable
              style="width: 100%"
              :data-testid="`new-${f.key}`"
              @update:model-value="(v: string) => setNewField(f.key, v)">
              <el-option v-for="o in f.options" :key="o.value" :label="o.label" :value="o.value" />
            </el-select>
            <el-input-number
              v-else-if="f.kind === 'number'"
              :model-value="newField(f.key) as number | undefined"
              :min="f.min"
              :max="f.max"
              controls-position="right"
              :data-testid="`new-${f.key}`"
              @update:model-value="(v: number | undefined) => setNewField(f.key, v ?? 0)" />
            <el-input
              v-else
              :model-value="String(newField(f.key) ?? '')"
              :type="f.kind === 'password' ? 'password' : 'text'"
              :show-password="f.kind === 'password'"
              :placeholder="f.placeholder"
              :data-testid="`new-${f.key}`"
              @update:model-value="(v: string) => setNewField(f.key, v)" />
            <div v-if="f.tip" class="field-tip">{{ f.tip }}</div>
          </el-form-item>
        </template>

        <div v-if="newChannel.channel_type === 'qq_onebot'" class="pt-note">
          <PtIcon name="info" :size="14" class="pt-note__icon" />
          <span>OneBot 的连接地址与 token 在通道创建后，到「设置」里补全。</span>
        </div>
        <div
          v-if="newChannel.channel_type === 'feishu_app' && feishuMode === 'manual'"
          class="pt-note">
          <PtIcon name="info" :size="14" class="pt-note__icon" />
          <span>
            创建以后，等通道列表里显示「已连接」，到飞书开发者后台把事件订阅方式改成「使用长连接接收事件」、
            添加「接收消息 v2.0」并发布版本；然后私聊机器人发一条消息，它会回你的
            open_id，填进通道详情的「管理员 open_id」。步骤见
            <a
              href="https://github.com/sunerpy/pt-tools/blob/main/docs/guide/chatops-feishu.md"
              target="_blank"
              rel="noopener noreferrer"
              >飞书应用机器人文档</a
            >第 5–7 节。
          </span>
        </div>
      </el-form>

      <template #footer>
        <template v-if="isFeishuScan">
          <el-button data-testid="feishu-reg-close" @click="addDialogVisible = false">{{
            reg ? "关闭" : "取消"
          }}</el-button>
          <el-button
            v-if="!regBusy"
            type="primary"
            :loading="regStarting"
            data-testid="feishu-reg-start"
            @click="startFeishuRegister">
            {{ regEnded ? "重新生成" : "生成二维码" }}
          </el-button>
        </template>
        <template v-else>
          <el-button @click="addDialogVisible = false">取消</el-button>
          <el-button
            type="primary"
            :loading="submitting"
            data-testid="save-btn"
            @click="handleCreate">
            创建通道
          </el-button>
        </template>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/*
 * 创建方式的分段：等扫码时停用，选中那档仍是设计系统的「纸」（Element 默认换成主色浅底）；
 * 手机上两档占满一行，长的那档折成两行，不在控件里横着滚、也不被截掉。
 */
.fsmode {
  --el-segmented-item-selected-disabled-bg-color: var(--pt-surface);
}

@media (max-width: 768px) {
  .fsmode :deep(.el-segmented__item) {
    padding: 4px 8px;
  }

  .fsmode :deep(.el-segmented__item-label) {
    white-space: normal;
  }
}

/*
 * 扫码一键创建：二维码居中（与远程访问的配对二维码同一个写法，白底、结束以后模糊掉），
 * 下面一行状态与有效期，一行原因，「在浏览器里打开」。
 */
.fsreg {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-3);
  align-items: center;
  margin-bottom: var(--pt-space-3);
}

.fsreg__qr {
  width: 240px;
  max-width: 100%;
  height: auto;
  aspect-ratio: 1;
  background: #fff;
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.fsreg__qr--used {
  filter: blur(6px) grayscale(1);
}

.fsreg__state {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
  align-items: center;
  justify-content: center;
}

.fsreg__sub {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.fsreg__reason {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  text-align: center;
  overflow-wrap: anywhere;
}

.fsreg__open {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-p);
  text-decoration: none;
}

.fsreg__open:hover {
  text-decoration: underline;
}

/* 作废提示占满一行，和下面的说明对齐 */
.fsreg__void {
  align-self: stretch;
}

/* 画板 22 的 chip-0「类型: 全部」与紧凑列表视图 */
.ch-chip {
  flex: 0 0 auto;
  width: 170px;
}

.ch-chip :deep(.el-select__wrapper) {
  min-height: 24px;
  padding: 0 8px;
  font-size: var(--pt-fz-label);
}

.ch-grid.is-rows .ch-card__acts {
  padding-top: var(--pt-space-2);
}

/* 投递策略卡：一行一个通道 */
.pol {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.pol__row {
  display: flex;
  gap: var(--pt-space-2);
  align-items: baseline;
  font-size: var(--pt-fz-sm);
}

.pol__k {
  flex: 1;
  overflow: hidden;
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pol__v {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: var(--pt-t1);
}

.pol__v.is-off {
  font-weight: 400;
  color: var(--pt-t3);
}

.pol__tag {
  padding: 0 5px;
  font-size: var(--pt-fz-foot);
  color: var(--pt-warn);
  background: color-mix(in srgb, var(--pt-warn) 14%, transparent);
  border-radius: var(--pt-r-sm);
}

.pol__tag--off {
  color: var(--pt-t3);
  background: var(--pt-hover);
}

.pol__empty {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.pol__foot {
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t3);
}

.pol__foot code {
  padding: 1px 4px;
  font-family: var(--pt-font-mono);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

.pol__foot :deep(.el-button) {
  height: auto;
  padding: 0;
  font-size: inherit;
  vertical-align: baseline;
}

/* 最近的通知：一行一条，时间 + 通道 + 来源 + 内容 + 结果 */
/*
 * 最近投递的小表 —— 画板 22 的 p-recent：表头 11/600 #7C8695 + 1px 行线，
 * 正文 12.5，时间列 500/t1（它是这一行的身份），其余 400/t2，只有「结果」是胶囊。
 * 卡内的小表比带里的主表轻一档：没有列竖线，也不铺表头底色。
 */
.rec {
  width: 100%;
  border-collapse: collapse;
  font-variant-numeric: tabular-nums;
}

.rec th {
  padding: 0 var(--pt-space-2) 6px 0;
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t3);
  text-align: left;
  white-space: nowrap;
  border-bottom: 1px solid var(--pt-border);
}

.rec td {
  padding: 6px var(--pt-space-2) 6px 0;
  font-size: 12.5px;
  font-weight: 400;
  color: var(--pt-t2);
  white-space: nowrap;
  border-bottom: 1px solid var(--pt-border);
}

.rec tbody tr:last-child td {
  border-bottom: 0;
}

/* 时间是这一行的身份，按画板的 td-0-0 走 500 + t1 */
.rec td.rec__when {
  font-weight: 500;
  color: var(--pt-t1);
}

.rec .rec__num {
  text-align: right;
}

/* 内容是种子名、消息标题，长短不一：截断成一行，鼠标停上去看全名 */
.rec__what {
  display: inline-block;
  max-width: 28em;
  overflow: hidden;
  text-overflow: ellipsis;
  vertical-align: bottom;
}

/*
 * 窄屏的同一块数据：画板 33 的「最近投递」是四行「圆点 6 + 双行文本 + 右侧时间 +
 * 329×1 分隔线」，节奏 54。圆点承载结果的语义色 —— 一行里不再需要胶囊。
 */
.recm {
  margin: 0;
  padding: 0;
  list-style: none;
}

.recm__row {
  display: flex;
  gap: var(--pt-space-2);
  align-items: flex-start;
  min-height: 54px;
  padding: 8px 0;
  border-bottom: 1px solid var(--pt-border);
}

.recm__row:last-child {
  border-bottom: 0;
}

.recm__dot {
  flex: 0 0 auto;
  width: 6px;
  height: 6px;
  margin-top: 6px;
  background: var(--pt-t4);
  border-radius: var(--pt-radius-full);
}

.recm__dot.is-ok {
  background: var(--pt-ok);
}

.recm__dot.is-dang {
  background: var(--pt-dang);
}

.recm__dot.is-warn {
  background: var(--pt-warn);
}

.recm__body {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.recm__l1 {
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recm__l2 {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 没发出去的原因：长原因可以断行 */
.recm__err {
  font-size: var(--pt-fz-label);
  color: var(--pt-dang);
  overflow-wrap: anywhere;
}

.recm__when {
  flex: 0 0 auto;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 带与卡片层各自管自己的留白，这一层只负责纵向堆叠 */
.notifications-page {
  display: flex;
  flex-direction: column;
}

/*
 * 通道卡栅格 —— 栏宽与间隔由 .pt-cards--2 给（画板 nt0..nt3 是 548 / 516 两栏）。
 * 280 那条 auto-fill 下限留在窄屏：.pt-cards 的基础规则本身就是 minmax(320px, 1fr)。
 */

/* 空态不在卡里，自己内缩 16 对齐卡片层 */
.ch-state {
  padding: var(--pt-pad);
}

.ch-card {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-3);
  padding: var(--pt-pad);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: var(--pt-shadow-sm);
  transition:
    border-color var(--pt-transition-fast),
    box-shadow var(--pt-transition-fast);
}

.ch-card:hover {
  border-color: var(--pt-p);
  box-shadow: var(--pt-shadow-md);
}

/*
 * 停用的通道压暗，但操作按钮照常可点：删掉它也是一种操作。
 * 图标照旧压 opacity；名称是承载信息的字，改成降一档字色（t3 ≥ 4.5）—— opacity 0.6 下 halo 浅色只剩 4.51。
 */
.ch-card.is-off .ch-card__icon {
  opacity: 0.6;
}

.ch-card.is-off .ch-card__name {
  color: var(--pt-t3);
}

.ch-card__head {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
}

/* 图标底色由 --ch-c 兑 12% 出来，四个通道的身份就靠这一枚色块 */
/*
 * 图标盘：画板 22 的 nt0 里那块 `ring` 是 **40×40、圆角 12、12% 语义色底**。
 * 落地此前是 30×30 / r6 —— 小一号又方一档，卡头的重心压不住 14/700 的标题。
 * 12 不在 4/6/8 那套里，但它是画板给这块盘定的值（盘越大圆角越大，是同一套比例）。
 */
.ch-card__icon {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  color: var(--ch-c);
  background: color-mix(in srgb, var(--ch-c) 12%, transparent);
  border-radius: 12px;
}

/* 通道名：画板 nt0 的 `n` 是 14/700（比区块标题小一号、但更重） */
.ch-card__name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  font-size: var(--pt-fz-body-lg);
  font-weight: 700;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ch-card__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
}

/* 操作组贴卡片底部，卡片再高也对齐 */
.ch-card__acts {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  margin-top: auto;
  padding-top: var(--pt-space-3);
  border-top: 1px solid var(--pt-border);
}

.ch-card__del {
  margin-left: auto;
}

@media (max-width: 768px) {
  .ch-grid {
    grid-template-columns: 1fr;
  }
}
</style>
