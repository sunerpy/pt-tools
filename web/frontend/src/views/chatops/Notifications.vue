<script setup lang="ts">
import { chatopsApi, type NotificationConfig, type RSSNotificationLog } from "@/api";
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
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";

/*
 * 通道类型的图标与色相：四个通道在卡片网格里必须一眼分得开，
 * 但只用语义色（info / ok / warn / primary），不引入这一页专属的调色板 ——
 * 四套主题切换时自定义色号会失配。
 */
const channelTypeOptions = [
  { value: "telegram", label: "Telegram", icon: "send", color: "var(--pt-info)" },
  { value: "qq_onebot", label: "QQ (OneBot)", icon: "message-square", color: "var(--pt-ok)" },
  { value: "webhook", label: "Webhook", icon: "link", color: "var(--pt-p)" },
  {
    value: "wecom_webhook",
    label: "WeCom Webhook",
    icon: "message-circle",
    color: "var(--pt-warn)",
  },
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
  webhook: "Webhook",
  wecom_webhook: "WeCom Webhook",
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
const { loading, state, errorText, run } = useDataState();

/**
 * 画板 22 在四张通道卡之后还有三张：
 *   p-policy 548 投递策略（安静时段按通道配置，就在通道自己的字段里）
 *   p-stat 516  最近投递统计（按通道分组，数据来自 RSS 通知日志）
 *   p-recent 1080 最近的通知记录
 * 后两张读 `/api/chatops/rss-notifications` 的一页，不新增后端。
 */
const recentLogs = ref<RSSNotificationLog[]>([]);

async function loadRecent() {
  try {
    const params = new URLSearchParams({ page: "1", page_size: "50" });
    const res = await chatopsApi.rssNotifications.list(params);
    recentLogs.value = res.items ?? [];
  } catch {
    /* 统计是附带信息，取不到就让卡里说明，不影响通道列表 */
    recentLogs.value = [];
  }
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

/** p-stat：这 50 条里每个通道投了多少、成了多少 */
const statRows = computed<BreakdownRow[]>(() => {
  const buckets = new Map<number, { total: number; sent: number }>();
  for (const log of recentLogs.value) {
    const cur = buckets.get(log.notification_conf_id) ?? { total: 0, sent: 0 };
    cur.total += 1;
    if (log.result === "sent") cur.sent += 1;
    buckets.set(log.notification_conf_id, cur);
  }
  return [...buckets.entries()]
    .sort((a, b) => b[1].total - a[1].total)
    .map(([id, agg]) => {
      const conf = notifications.value.find((n) => n.id === id);
      return {
        key: String(id),
        label: conf ? conf.name : `#${id}`,
        value: `${agg.sent} / ${agg.total}`,
        weight: agg.total,
        tone: agg.sent === agg.total ? ("ok" as const) : ("warn" as const),
        hint: agg.sent === agg.total ? "全部送达" : `${agg.total - agg.sent} 条没送达`,
      };
    });
});

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
  return "接上 Telegram / QQ / Webhook / 企业微信，就能收到任务结果与告警，也能反过来发命令";
});

const addDialogVisible = ref(false);
const submitting = ref(false);

const newChannel = ref<Partial<NotificationConfig>>({
  channel_type: "telegram",
  name: "",
  enabled: true,
  bot_token: "",
  endpoint_url: "",
  webhook_key: "",
});

onMounted(async () => {
  await Promise.all([loadNotifications(), loadRecent()]);
});

async function loadNotifications() {
  const data = await run(() => chatopsApi.notifications.list());
  if (!data) {
    // 失败时清空：留着旧列表配一个「加载失败」更让人误解
    notifications.value = [];
    return;
  }
  notifications.value = data;
}

function openAddDialog() {
  newChannel.value = {
    channel_type: "telegram",
    name: "",
    enabled: true,
    bot_token: "",
    endpoint_url: "",
    webhook_key: "",
  };
  addDialogVisible.value = true;
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

  submitting.value = true;
  try {
    await chatopsApi.notifications.create(newChannel.value as Omit<NotificationConfig, "id">);
    ElMessage.success("添加成功");
    addDialogVisible.value = false;
    await loadNotifications();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "添加失败");
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
    window.setTimeout(() => {
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
        / p-recent 1080（最近记录）。统计与记录读 RSS 通知日志的一页，不新增后端。
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
          安静时段里的通知记为 <code>suppressed</code>，不算失败也不补发； 每条 RSS
          还能单独设「每小时最多几条」，超出的记为 <code>throttled</code>。
        </p>
      </PtPanel>

      <PtPanel
        title="最近投递统计"
        icon="chart-pie"
        :count="recentLogs.length > 0 ? `最近 ${recentLogs.length} 条` : '暂无'">
        <PtBreakdown
          v-if="statRows.length > 0"
          :rows="statRows"
          foot="口径是「已送达 / 总条数」，取的是 RSS 通知日志最近 50 条，不是全量历史。" />
        <p v-else class="pol__empty">还没有投递记录，或者通知日志没取到。</p>
      </PtPanel>

      <PtPanel
        class="pt-cards__full"
        title="最近的通知"
        icon="history"
        :count="`${recentLogs.length} 条`">
        <!--
          画板 22 的 p-recent 是一张**带表头的小表**（时间 / 通道 / 事件 / 站点 / 结果 /
          延迟 / 重试，正文 12.5、只有「结果」是胶囊），不是一排 chip。
          落地此前是无列名的 chip 串：一行里三枚色块，而且看不出哪个字段是什么。
          「延迟」那列没有对应数据（RSS 通知日志不记投递耗时），所以不画 —— 不造数。
        -->
        <!--
          窄屏降级成列表，不做横向滚动表格 —— 这是设计文档 §9 的四方向共用规则，
          画板 33 给这一块的形态也正是「圆点 6 + 双行文本 + 右侧时间 + 分隔线」、节奏 54。
          上一版在所有视口都渲染这张七列表（单元格还都 nowrap），375 下右边两列直接被裁掉：
          外层是 overflow hidden，滚都滚不出来。一次评审查出这点，判得对。
        -->
        <ul v-if="isMobile && recentLogs.length > 0" class="recm">
          <li v-for="log in recentLogs.slice(0, 8)" :key="log.id" class="recm__row">
            <span
              class="recm__dot"
              :class="`is-${log.result === 'sent' ? 'ok' : log.result === 'failed' ? 'dang' : 'neutral'}`"
              aria-hidden="true" />
            <span class="recm__body">
              <span class="recm__l1">
                {{ log.site_name || "未知站点" }}
                <span class="recm__tid">{{ log.torrent_id }}</span>
              </span>
              <span class="recm__l2">
                {{ channelNameOf(log.notification_conf_id) }} ·
                {{ log.notify_kind === "filtered" ? "仅匹配的" : "全部新种" }} · {{ log.result
                }}<template v-if="(log.attempts ?? 0) > 1"> · 第 {{ log.attempts }} 次</template>
              </span>
            </span>
            <span class="recm__when">{{ formatWhen(log.created_at) }}</span>
          </li>
        </ul>

        <table v-else-if="recentLogs.length > 0" class="rec">
          <thead>
            <tr>
              <th>时间</th>
              <th>通道</th>
              <th>类型</th>
              <th>站点</th>
              <th>种子 ID</th>
              <th>结果</th>
              <th class="rec__num">尝试</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="log in recentLogs.slice(0, 8)" :key="log.id">
              <td class="rec__when">{{ formatWhen(log.created_at) }}</td>
              <td>{{ channelNameOf(log.notification_conf_id) }}</td>
              <td>{{ log.notify_kind === "filtered" ? "仅匹配的" : "全部新种" }}</td>
              <td>{{ log.site_name || "未知站点" }}</td>
              <td>{{ log.torrent_id }}</td>
              <td>
                <PtStatusPill
                  :tone="
                    log.result === 'sent' ? 'ok' : log.result === 'failed' ? 'dang' : 'neutral'
                  "
                  size="sm">
                  {{ log.result }}
                </PtStatusPill>
              </td>
              <td class="rec__num">{{ log.attempts ?? 0 }}</td>
            </tr>
          </tbody>
        </table>
        <p v-else class="pol__empty">还没有通知记录。</p>
        <p class="pol__foot">
          完整清单、重试与取消在
          <el-button link type="primary" @click="router.push('/chatops/rss-notifications')">
            RSS 通知日志
          </el-button>
          里。
        </p>
      </PtPanel>
    </div>

    <el-dialog
      v-model="addDialogVisible"
      class="pt-dialog"
      title="添加通知通道"
      width="520px"
      align-center>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <el-form-item label="通道类型">
          <el-select v-model="newChannel.channel_type" style="width: 100%">
            <el-option
              v-for="opt in channelTypeOptions"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
              :data-testid="`channel-type-${opt.value}`" />
          </el-select>
        </el-form-item>

        <el-form-item label="通道名称" required>
          <el-input
            v-model="newChannel.name"
            placeholder="例如：我的 Telegram 机器人"
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

        <div v-if="newChannel.channel_type === 'qq_onebot'" class="pt-note">
          <PtIcon name="info" :size="14" class="pt-note__icon" />
          <span>OneBot 的连接地址与 token 在通道创建后，到「设置」里补全。</span>
        </div>
      </el-form>

      <template #footer>
        <el-button @click="addDialogVisible = false">取消</el-button>
        <el-button
          type="primary"
          :loading="submitting"
          data-testid="save-btn"
          @click="handleCreate">
          创建通道
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
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
  color: var(--pt-t4);
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
  color: var(--pt-t4);
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

/* 最近的通知：一行一条，站点 + 种子 ID + 通道 + 时间 */
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

.recm__tid {
  font-weight: 400;
  color: var(--pt-t3);
}

.recm__l2 {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
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

/* 停用的通道压暗，但操作按钮照常可点：删掉它也是一种操作 */
.ch-card.is-off .ch-card__icon,
.ch-card.is-off .ch-card__name {
  opacity: 0.6;
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
