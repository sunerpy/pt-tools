<script setup lang="ts">
import {
  type ChatOpBinding,
  type ChatOpsBindingChannel,
  type ChatOpsChannelUser,
  chatopsApi,
  type NotificationConfig,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { activityText, maskUserId, notifyViaText, roleTexts } from "@/utils/bindingChannels";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, onUnmounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";

const TTL_OPTIONS = [
  { label: "5 分钟", value: 300 },
  { label: "1 小时", value: 3600 },
  { label: "1 天", value: 86400 },
  { label: "30 天", value: 2592000 },
  { label: "永久", value: 0 },
];

const isMobile = useIsMobile();
const router = useRouter();

/** 画板 head 的 sub（11.5/400 t3）：几个通道、已绑定几个；有没用掉的绑定码时再写待绑定 */
const headSub = computed(() => {
  const parts = [`${channels.value.length} 个通道`, `已绑定 ${activeBindings.value.length}`];
  if (pendingBindings.value.length > 0) parts.push(`待绑定 ${pendingBindings.value.length}`);
  return parts.join(" · ");
});
const pendingBindings = ref<ChatOpBinding[]>([]);
const activeBindings = ref<ChatOpBinding[]>([]);
const configs = ref<NotificationConfig[]>([]);
/**
 * 能收命令的通道（Telegram、QQ）：机器人在不在线、通知发给谁、名单里每个账号绑定了没有。
 * 2026-10-28 用户以为「机器人接上了就是绑定了」—— 机器人在线、通知照常发，账号却发过 /unbind，
 * 页面只列绑定行时三件事混在一起看不出来（见 utils/bindingChannels.ts）。
 */
const channels = ref<ChatOpsBindingChannel[]>([]);

/**
 * 六态状态机（设计文档 §5）。
 *
 * 以前这里只有一个 loading ref，失败时弹个 toast 就完事 —— 两秒后 toast 消失，
 * 两张表都停在 empty 上，用户看到的是「还没有人绑定」，而真相是请求失败了。
 *
 * 这一页没有筛选框，所以 0 行只会是 empty / error / perm / partial，不会出现 zero。
 * partial 留给「绑定列表拿到了、渠道配置没拿到」：通道名只能退化成 `#3`，
 * 发码对话框的通道下拉也会是空的 —— 这件事必须写在页面上，光让通道名变成编号没人看得懂。
 */
const configsFailed = ref(0);
/** 通道一栏没取到（绑定列表取到了）：也按 partial 说清楚 */
const channelsFailed = ref(0);
const { loading, state, errorText, run, isStale, hasPartialBanner } = useDataState({
  failed: () => configsFailed.value + channelsFailed.value,
});

const PARTIAL_SUB = "绑定列表已就绪，但渠道配置没取到，通道名称只能显示编号";

/** 状态块副标题：失败给真实错误，部分失败说清缺了什么，空态给下一步动作 */
function subFor(emptySub: string) {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  if (state.value === "partial") return PARTIAL_SUB;
  return emptySub;
}

const pendingSub = computed(() => subFor("生成一个绑定码，再去聊天客户端里把它发给机器人"));
const activeSub = computed(() =>
  subFor("还没有账号完成绑定：在上面的通道里点「绑定」，或者生成一个绑定码"),
);
const channelsSub = computed(() =>
  subFor("先在「消息通知」里加一个 Telegram 或 QQ 通道，再回来绑定账号"),
);

/** 有绑定数据但渠道配置没拿到：在内容上方挂一条提示，而不是假装一切正常 */
const showPartialNote = computed(() =>
  hasPartialBanner(
    pendingBindings.value.length + activeBindings.value.length + channels.value.length,
  ),
);

const generateDialogVisible = ref(false);
const selectedConfId = ref<number | null>(null);
const selectedTTL = ref<number>(300);
const generatedCode = ref<string | null>(null);
const generatedExpiresAt = ref<string | null>(null);
const generating = ref(false);

/* 秒级心跳只为倒计时那一列：绑定码 5 分钟就过期，分钟级刷新看不出快到点了 */
const now = ref(Date.now());
let timer: ReturnType<typeof setInterval>;

onMounted(() => {
  loadData();
  timer = setInterval(() => {
    now.value = Date.now();
  }, 1000);
});

onUnmounted(() => {
  if (timer) clearInterval(timer);
});

async function loadData() {
  configsFailed.value = 0;
  channelsFailed.value = 0;
  /*
   * 渠道配置随绑定列表一起交回来，确认这一份没过期之后才写：写在 run 里面的话，
   * 被顶掉的旧请求照样会改通道名，它的渠道配置失败还会把新页面标成 partial。
   */
  const pending = run(async () => {
    let confsFailed = false;
    let chFailed = false;
    const [bindingsRes, configsRes, channelsRes] = await Promise.all([
      chatopsApi.bindings.list(),
      // 渠道配置只用来把 conf_id 显示成通道名、以及填发码下拉，单独失败不该把整页判死
      chatopsApi.notifications.list().catch(() => {
        confsFailed = true;
        return [] as NotificationConfig[];
      }),
      chatopsApi.bindings.channels().catch(() => {
        chFailed = true;
        return { channels: [] as ChatOpsBindingChannel[] };
      }),
    ]);
    return {
      bindingsRes,
      configsRes: configsRes || [],
      confsFailed,
      channelsRes: channelsRes?.channels ?? [],
      chFailed,
    };
  });
  const data = await pending;
  if (isStale(pending)) return;

  if (!data) {
    // 失败时清空：留着上一次的数据配一个「加载失败」的状态块更让人误解
    pendingBindings.value = [];
    activeBindings.value = [];
    channels.value = [];
    return;
  }
  configs.value = data.configsRes;
  configsFailed.value = data.confsFailed ? 1 : 0;
  channels.value = data.channelsRes;
  channelsFailed.value = data.chFailed ? 1 : 0;
  pendingBindings.value = data.bindingsRes.pending || [];
  activeBindings.value = data.bindingsRes.bindings || [];
}

function getCountdown(expiresAt?: string) {
  if (!expiresAt) return "永久";
  const end = new Date(expiresAt).getTime();
  const diff = end - now.value;
  if (diff <= 0) return "已过期";

  const m = Math.floor(diff / 60000);
  const s = Math.floor((diff % 60000) / 1000);
  return `${m} 分 ${s.toString().padStart(2, "0")} 秒`;
}

/* 不到一分钟标红：这一列是「还来不来得及去客户端发码」，红字比动画更好读 */
function getCountdownClass(expiresAt?: string) {
  if (!expiresAt) return "cd cd--forever";
  const text = getCountdown(expiresAt);
  if (text === "已过期") return "cd cd--expired";
  const diff = new Date(expiresAt).getTime() - now.value;
  if (diff < 60000) return "cd cd--urgent";
  return "cd cd--live";
}

function formatDate(dateStr?: string) {
  if (!dateStr) return "-";
  const d = new Date(dateStr);
  // 后端没有这个时间时回 Go 的零值 0001-01-01：以前显示成「1/1/1 08:05:43」
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return "-";
  return d.toLocaleString("zh-CN", { hour12: false });
}

/* ------------------------------------------------------------------ 通道一栏 */

const RUNTIME_LABEL: Record<string, { label: string; tone: "ok" | "warn" | "dang" | "neutral" }> = {
  connected: { label: "已连接", tone: "ok" },
  running: { label: "运行中", tone: "ok" },
  error: { label: "异常", tone: "dang" },
  disabled: { label: "已停用", tone: "neutral" },
};

function runtimeOf(ch: ChatOpsBindingChannel) {
  const key = ch.runtime_state || (ch.enabled ? "" : "disabled");
  return (
    RUNTIME_LABEL[key] ?? { label: ch.enabled ? "已启用" : "已停用", tone: "neutral" as const }
  );
}

const CHANNEL_NAMES: Record<string, string> = { telegram: "Telegram", qq_onebot: "QQ" };

/** 通知（每日战报、签到、追剧…）发给谁：「默认 Chat ID 12***7890」；没有收件人时写原因 */
function notifyLine(ch: ChatOpsBindingChannel): string {
  if (ch.notify_target) {
    return `${notifyViaText(ch.notify_via)} ${maskUserId(ch.notify_target)}`.trim();
  }
  return ch.notify_error || "";
}

const binding = reactive<Record<string, boolean>>({});

/*
 * 网页上直接绑定名单里的账号：效果与绑定码相同（能执行全部命令，含管理员命令），后端只认名单里的账号，
 * 并记一条审计（命令 bind，参数 via=web）。确认框写清楚这一点 —— 这是给一个聊天账号发权限。
 */
async function handleBindDirect(ch: ChatOpsBindingChannel, user: ChatOpsChannelUser) {
  try {
    await ElMessageBox.confirm(
      `绑定 ${CHANNEL_NAMES[ch.channel_type] ?? ch.channel_type} 账号 ${maskUserId(user.user_id)}？绑定后它能在「${ch.name}」里执行全部命令，包括暂停、删除种子这些管理员命令。`,
      "绑定账号",
      { type: "warning", confirmButtonText: "绑定", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  const key = `${ch.conf_id}:${user.user_id}`;
  binding[key] = true;
  try {
    await chatopsApi.bindings.bindDirect(ch.conf_id, user.user_id);
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "绑定失败");
    return;
  } finally {
    binding[key] = false;
  }
  ElMessage.success("已绑定，现在可以在聊天里发命令了");
  loadData();
}

function openChannel(ch: ChatOpsBindingChannel) {
  void router.push(`/chatops/notifications/${ch.conf_id}`);
}

async function handleGenerateCode() {
  if (!selectedConfId.value) {
    ElMessage.warning("请选择关联的渠道配置");
    return;
  }
  generating.value = true;
  try {
    const res = await chatopsApi.bindings.generateCode(
      selectedConfId.value,
      undefined,
      selectedTTL.value,
    );
    generatedCode.value = res.code;
    generatedExpiresAt.value = res.expires_at ?? null;
    loadData();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "生成失败");
  } finally {
    generating.value = false;
  }
}

function openGenerateDialog() {
  selectedConfId.value = configs.value.length > 0 ? configs.value[0].id : null;
  selectedTTL.value = 300;
  generatedCode.value = null;
  generatedExpiresAt.value = null;
  generateDialogVisible.value = true;
}

async function copyToClipboard(text: string) {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      ElMessage.success("已复制");
      return;
    }
  } catch {
    // fall through to legacy path
  }
  // Legacy fallback for HTTP / non-secure contexts
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  textarea.style.left = "-9999px";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  try {
    const ok = document.execCommand("copy");
    if (ok) {
      ElMessage.success("已复制");
    } else {
      ElMessage.warning("复制失败，请手动复制");
    }
  } catch {
    ElMessage.warning("复制失败，请手动复制");
  } finally {
    document.body.removeChild(textarea);
  }
}

function handleCloseGenerateDialog() {
  generateDialogVisible.value = false;
  generatedCode.value = null;
  generatedExpiresAt.value = null;
}

/*
 * 确认框与删除请求分成两段 try：原来包在同一个 try 里，catch 一律当成「用户取消」吞掉，
 * 接口真的报错（404 / 500）时页面一声不吭，绑定还在，用户却以为已经撤销了。
 * 确认框被取消或关掉时 reject 的是动作名（'cancel' / 'close'），只有这一段是静默的。
 */
async function handleDelete(id: number) {
  try {
    await ElMessageBox.confirm("确定要撤销该绑定吗？撤销后该用户需要重新拿码绑定。", "撤销绑定", {
      type: "warning",
      confirmButtonText: "确定撤销",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  try {
    await chatopsApi.bindings.delete(id);
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "撤销绑定失败");
    return;
  }
  ElMessage.success("绑定已撤销");
  loadData();
}

/*
 * 作废「待绑定」里一个没用掉的绑定码。永久的码只有用掉才会消失，在上面的通道栏直接绑定也不消耗它
 * （2026-10-29 用户：「为什么我绑定了之后」待绑定里还挂着 10-21 生成、一直没用的永久码）。
 */
async function handleRevokeCode(row: ChatOpBinding) {
  try {
    await ElMessageBox.confirm(
      `作废绑定码 ${row.code}？作废后不能再用它绑定，已经绑定的账号不受影响。`,
      "作废绑定码",
      { type: "warning", confirmButtonText: "作废", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    await chatopsApi.bindings.revokeCode(row.id);
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "作废绑定码失败");
    return;
  }
  ElMessage.success("绑定码已作废");
  loadData();
}

async function handleToggleLang(row: ChatOpBinding) {
  const newLang = row.reply_lang === "zh" ? "en" : "zh";
  try {
    await chatopsApi.bindings.update(row.id, { reply_lang: newLang });
    ElMessage.success(`已切换回复语言至 ${newLang === "zh" ? "中文" : "English"}`);
    row.reply_lang = newLang;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "切换语言失败");
  }
}

function getConfNameByConfId(confId?: number) {
  if (!confId) return "-";
  const conf = configs.value.find((c) => c.id === confId);
  return conf ? conf.name : `#${confId}`;
}
</script>

<template>
  <!--
    画板 24：head 64 之后是靠左的一栏 612 宽的卡（p-pending / p-active / p-note），
    右边那片空白上画的是发码弹窗的规格，不是页面内容。刷新与生成绑定码是页头动作。
    落地偏离画板：两张表的列宽合计 840 / 946，612 的卡里只能横滚，「回复语言 / 管理员 / 最后活跃」
    挤在右边看不到，列头「回复语言」还被固定的操作列盖住一半，所以表格放在主栏。
    宽屏（视口 ≥1600）说明挪到右边 400 宽的侧栏（.pt-cards--aside）；以前卡宽停在 1080，
    2048 的屏上右边空出 680（用户反馈）。窄屏照旧一栏、说明在表格下面。
  -->
  <div class="bindings-page pt-cards pt-cards--wide pt-cards--aside">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button :loading="loading" @click="loadData">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
      <el-button type="primary" @click="openGenerateDialog">
        <PtIcon name="plus" :size="15" /><span>生成绑定码</span>
      </el-button>
    </Teleport>

    <div class="pt-cards pt-cards--wide">
      <!-- 部分失败（§5 partial）：绑定拿到了、渠道配置没拿到，说清缺的是什么而不是静默降级 -->
      <div v-if="showPartialNote" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>
          <template v-if="configsFailed">
            渠道配置没取到：通道名称只能显示为编号，发码对话框里的通道下拉也会是空的。
          </template>
          <template v-if="channelsFailed">
            通道名单没取到：上面的「通道」一栏是空的，绑定列表本身是完整的。
          </template>
          <el-button link type="primary" @click="loadData">重试</el-button>
        </span>
      </div>

      <!--
        通道一栏（2026-10-28 用户：「TG 机器人已经和 pt-tools 绑定了，但是却没有显示」）：
        机器人在不在线、通知发给谁、名单里每个账号绑定了没有，分开写。名单里没绑定的账号可以直接点「绑定」。
      -->
      <PtPanel
        v-loading="loading"
        title="通道"
        icon="bot"
        :count="`${channels.length} 个`"
        padding="none"
        data-testid="binding-channels">
        <PtDataState
          v-if="!channels.length"
          :state="state"
          dense
          :title="state === 'empty' ? '还没有能收命令的通道' : ''"
          :sub="channelsSub" />
        <section v-for="ch in channels" :key="ch.conf_id" class="ch" data-testid="binding-channel">
          <header class="ch__head">
            <span class="ch__name">{{ ch.name }}</span>
            <PtTag>{{ CHANNEL_NAMES[ch.channel_type] ?? ch.channel_type }}</PtTag>
            <PtStatusPill dot :tone="runtimeOf(ch).tone" size="sm">
              {{ runtimeOf(ch).label }}
            </PtStatusPill>
            <span class="ch__spacer" />
            <el-button link type="primary" size="small" @click="openChannel(ch)">
              <PtIcon name="settings" :size="14" /><span>通道设置</span>
            </el-button>
          </header>
          <p v-if="ch.config_error" class="ch__line is-warn">
            <PtIcon name="triangle-alert" :size="13" />
            <span>通道配置读不出来：{{ ch.config_error }}</span>
          </p>
          <p v-else class="ch__line" :class="{ 'is-warn': !ch.notify_target }">
            <PtIcon :name="ch.notify_target ? 'bell' : 'triangle-alert'" :size="13" />
            <span v-if="ch.notify_target">
              每日战报、签到、追剧等通知发给{{ notifyLine(ch) }}，不用绑定也收得到
            </span>
            <span v-else>通知发不出去：{{ notifyLine(ch) }}</span>
          </p>

          <ul v-if="ch.users.length" class="ch__users">
            <li
              v-for="u in ch.users"
              :key="u.user_id"
              class="ch__user"
              data-testid="binding-channel-user">
              <code class="ch__uid">{{ maskUserId(u.user_id) }}</code>
              <span class="ch__roles">
                <PtTag v-for="r in roleTexts(u.roles)" :key="r">{{ r }}</PtTag>
                <span v-if="!u.roles.length" class="ch__warn">不在名单里，发的消息会被丢掉</span>
              </span>
              <PtStatusPill :tone="u.bound ? 'ok' : 'warn'" size="sm">
                {{ u.bound ? "已绑定" : "未绑定" }}
              </PtStatusPill>
              <span class="ch__last">
                {{ activityText(u.last) || "还没有和机器人说过话" }}
              </span>
              <el-button
                v-if="!u.bound && u.roles.length"
                size="small"
                type="primary"
                :loading="binding[`${ch.conf_id}:${u.user_id}`]"
                :aria-label="`绑定 ${maskUserId(u.user_id)}`"
                @click="handleBindDirect(ch, u)">
                <PtIcon name="user-plus" :size="14" /><span>绑定</span>
              </el-button>
            </li>
          </ul>
          <p v-else class="ch__line is-warn">
            <PtIcon name="triangle-alert" :size="13" />
            <span>名单是空的：谁发给机器人的消息都会被丢掉。在通道设置里填管理员或允许名单。</span>
          </p>
        </section>
        <template v-if="channels.length > 0" #footer>
          <span class="pt-foot-note">
            命令要绑定才能用；名单里的账号可以直接点「绑定」，效果和绑定码一样。名单在通道设置里改
          </span>
        </template>
      </PtPanel>

      <!-- 待绑定：只在有没用掉的绑定码时出现（以前一直占着一块空面板，用户说从没用过） -->
      <PtPanel
        v-if="pendingBindings.length > 0"
        v-loading="loading"
        title="待绑定"
        icon="key-round"
        :count="`${pendingBindings.length} 个绑定码`"
        padding="none">
        <el-table
          v-if="!isMobile"
          :data="pendingBindings"
          class="pt-grid"
          row-key="code"
          style="width: 100%">
          <template #empty>
            <PtDataState
              :state="state"
              dense
              :title="state === 'empty' ? '没有待使用的绑定码' : ''"
              :sub="pendingSub">
              <template v-if="state === 'error'" #action>
                <el-button size="small" @click="loadData">
                  <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
                </el-button>
              </template>
            </PtDataState>
          </template>

          <el-table-column prop="code" label="绑定码" width="220" class-name="pt-cell-strong">
            <template #default="{ row }">
              <span class="code-cell">
                <code class="code">{{ row.code }}</code>
                <el-button
                  link
                  type="primary"
                  size="small"
                  aria-label="复制绑定码"
                  @click="copyToClipboard(row.code)">
                  <PtIcon name="copy" :size="14" />
                </el-button>
              </span>
            </template>
          </el-table-column>

          <el-table-column label="关联通道" width="170">
            <template #default="{ row }">
              <PtTag>{{ getConfNameByConfId(row.conf_id) }}</PtTag>
            </template>
          </el-table-column>

          <el-table-column prop="label" label="备注" min-width="140" class-name="pt-cell-muted">
            <template #default="{ row }">{{ row.label || "-" }}</template>
          </el-table-column>

          <el-table-column label="创建时间" width="170" class-name="pt-cell-muted">
            <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
          </el-table-column>

          <el-table-column
            label="剩余有效时间"
            min-width="140"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num">
            <template #default="{ row }">
              <span :class="getCountdownClass(row.expires_at)">
                {{ getCountdown(row.expires_at) }}
              </span>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="90" class-name="pt-cell-act">
            <template #default="{ row }">
              <el-tooltip content="作废绑定码" placement="top">
                <el-button
                  link
                  type="danger"
                  size="small"
                  :aria-label="`作废绑定码 ${row.code}`"
                  @click="handleRevokeCode(row)">
                  <PtIcon name="trash-2" :size="15" />
                </el-button>
              </el-tooltip>
            </template>
          </el-table-column>
        </el-table>

        <!--
        移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
        卡上留的是真正要做的事：把码复制走。所以码本身当标题，
        倒计时进 status（红字表示快过期了），复制做成一个撑满的主操作 —— 表格里那个
        16px 的链接图标在手机上根本点不准。
      -->
        <div v-else class="cards">
          <PtDataState
            v-if="!pendingBindings.length"
            :state="state"
            :title="state === 'empty' ? '没有待使用的绑定码' : ''"
            :sub="pendingSub">
            <template v-if="state === 'error'" #action>
              <el-button size="small" @click="loadData">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>

          <PtRowCard v-for="row in pendingBindings" :key="row.code ?? row.id">
            <template #title>
              <code class="code">{{ row.code }}</code>
            </template>

            <template #meta>
              <PtTag>{{ getConfNameByConfId(row.conf_id) }}</PtTag>
              <span v-if="row.label">{{ row.label }}</span>
              <span v-if="row.created_at">
                <PtIcon name="clock" :size="11" />
                {{ formatDate(row.created_at) }}
              </span>
            </template>

            <template #status>
              <span :class="getCountdownClass(row.expires_at)">
                {{ getCountdown(row.expires_at) }}
              </span>
            </template>

            <template #actions>
              <el-button size="small" @click="copyToClipboard(row.code ?? '')">
                <PtIcon name="copy" :size="14" /><span>复制绑定码</span>
              </el-button>
              <el-button
                size="small"
                type="danger"
                plain
                :aria-label="`作废绑定码 ${row.code}`"
                @click="handleRevokeCode(row)">
                <PtIcon name="trash-2" :size="14" /><span>作废</span>
              </el-button>
            </template>
          </PtRowCard>
        </div>

        <template v-if="pendingBindings.length > 0" #footer>
          <span class="pt-foot-note"
            >绑定码只能用一次，用掉或过期后从这里消失；永久的码不用了就作废。在上面的通道栏直接绑定不用绑定码，也不会用掉这里的码。</span
          >
        </template>
      </PtPanel>

      <PtPanel
        v-loading="loading"
        title="已绑定用户"
        icon="user-check"
        :count="`${activeBindings.length} 个`"
        padding="none">
        <el-table
          v-if="!isMobile"
          :data="activeBindings"
          class="pt-grid"
          row-key="id"
          style="width: 100%">
          <template #empty>
            <PtDataState
              :state="state"
              dense
              :title="state === 'empty' ? '还没有已绑定的用户' : ''"
              :sub="activeSub">
              <template v-if="state === 'error'" #action>
                <el-button size="small" @click="loadData">
                  <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
                </el-button>
              </template>
            </PtDataState>
          </template>

          <el-table-column prop="channel_type" label="渠道" width="120">
            <template #default="{ row }">
              <PtTag>{{ row.channel_type }}</PtTag>
            </template>
          </el-table-column>

          <el-table-column label="渠道用户 ID" width="160" class-name="pt-cell-strong">
            <template #default="{ row }">
              <code class="uid">{{ maskUserId(row.channel_user_id) }}</code>
            </template>
          </el-table-column>

          <el-table-column prop="label" label="备注" min-width="140" class-name="pt-cell-muted">
            <template #default="{ row }">{{ row.label || "-" }}</template>
          </el-table-column>

          <el-table-column prop="reply_lang" label="回复语言" width="110">
            <template #default="{ row }">
              <PtTag>{{ row.reply_lang === "zh" ? "中文" : "English" }}</PtTag>
            </template>
          </el-table-column>

          <el-table-column prop="admin" label="管理员" width="96">
            <template #default="{ row }">
              <PtStatusPill :tone="row.admin ? 'warn' : 'neutral'" size="sm">
                {{ row.admin ? "是" : "否" }}
              </PtStatusPill>
            </template>
          </el-table-column>

          <el-table-column label="最后活跃" width="170" class-name="pt-cell-muted">
            <template #default="{ row }">{{ formatDate(row.last_active) }}</template>
          </el-table-column>

          <!-- 操作列只放图标（与站点列表 / 暂停任务一致），aria-label 带上被操作的用户 -->
          <el-table-column label="操作" width="84" fixed="right" class-name="pt-cell-act">
            <template #default="{ row }">
              <el-tooltip content="切换回复语言" placement="top">
                <el-button
                  link
                  type="primary"
                  size="small"
                  :aria-label="`切换 ${maskUserId(row.channel_user_id)} 的回复语言`"
                  @click="handleToggleLang(row)">
                  <PtIcon name="globe" :size="15" />
                </el-button>
              </el-tooltip>
              <el-tooltip content="撤销绑定" placement="top">
                <el-button
                  link
                  type="danger"
                  size="small"
                  :aria-label="`撤销 ${maskUserId(row.channel_user_id)} 的绑定`"
                  @click="handleDelete(row.id)">
                  <PtIcon name="trash-2" :size="15" />
                </el-button>
              </el-tooltip>
            </template>
          </el-table-column>
        </el-table>

        <!--
        移动端行卡。管理员那一栏在卡上不能沿用表格的「是 / 否」—— 卡片没有列头，
        脱离表头的「否」读不出是在说什么，所以换成自解释的「管理员 / 普通」。
      -->
        <div v-else class="cards">
          <PtDataState
            v-if="!activeBindings.length"
            :state="state"
            :title="state === 'empty' ? '还没有已绑定的用户' : ''"
            :sub="activeSub">
            <template v-if="state === 'error'" #action>
              <el-button size="small" @click="loadData">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>

          <PtRowCard v-for="row in activeBindings" :key="row.id">
            <template #title>
              <code class="uid uid--card">{{ maskUserId(row.channel_user_id) }}</code>
            </template>

            <template #meta>
              <PtTag>{{ row.channel_type }}</PtTag>
              <PtTag>{{ row.reply_lang === "zh" ? "中文" : "English" }}</PtTag>
              <span v-if="row.label">{{ row.label }}</span>
              <span v-if="row.last_active">
                <PtIcon name="clock" :size="11" />
                {{ formatDate(row.last_active) }}
              </span>
            </template>

            <template #status>
              <PtStatusPill :tone="row.admin ? 'warn' : 'neutral'" size="sm">
                {{ row.admin ? "管理员" : "普通" }}
              </PtStatusPill>
            </template>

            <template #actions>
              <el-button size="small" @click="handleToggleLang(row)">
                <PtIcon name="globe" :size="14" /><span>切换语言</span>
              </el-button>
              <el-button size="small" type="danger" plain @click="handleDelete(row.id)">
                <PtIcon name="trash-2" :size="14" /><span>撤销</span>
              </el-button>
            </template>
          </PtRowCard>
        </div>

        <template v-if="activeBindings.length > 0" #footer>
          <span class="pt-foot-note">
            绑定时一律写入管理员身份，这一列只做展示；不再需要的绑定请撤销
          </span>
        </template>
      </PtPanel>
    </div>

    <!--
      画板 p-note 612：绑定流程说明。内容按代码核过 ——
      绑定码是 8 位、只存 bcrypt 哈希（models.BotToken.CodeOrTokenHash）、有 UsedAt 与
      ExpiresAt 两个状态；兑换走的是 ChatOps 的 `/bind <绑定码>` 命令
      （internal/chatops/commands/bind.go）。
    -->
    <PtPanel title="绑定是怎么走的" icon="info">
      <ol class="bind-note">
        <li>
          通道名单里的账号可以直接在上面的「通道」里点「绑定」；不在名单里的账号先在通道设置里加进名单。
        </li>
        <li>也可以点「生成绑定码」，选好通道，拿到一个 8 位的码。</li>
        <li>
          在对应的聊天里发 <code>/bind &lt;绑定码&gt;</code>。机器人拿这个码换绑定关系，
          成功后这条记录从「待绑定」挪到「已绑定」。在聊天里发
          <code>/unbind</code> 会解除自己的绑定。
        </li>
        <!--
          之前这里写的是「码只存哈希，关掉就再也看不到」—— 两句都不成立：绑定码按原文存
          （internal/app/binding_service.go 的 CodeOrTokenHash 字段存的就是码本身），下面「待绑定」表里一直能看到、能复制。
          一句不成立的安全声明比不写更糟，这里只写实际行为。
        -->
        <li>
          码在到期或用掉之前，都能在「待绑定」列表里看到和复制；用过或到期后自动失效。永久的码不用了点「作废」，
          直接绑定不会用掉它。
        </li>
        <!--
          之前这里写「管理员身份在通道凭证里配置，绑上不等于是管理员」—— 与代码相反：
          internal/app/binding_service.go 的 ConsumeCode 给每个新绑定写 PtAdmin: true，
          MessageChain 判管理员命令只看这个字段。通道凭证里的两栏名单只决定谁能和机器人说话：
          Telegram 见 adapter/telegram/inbound.go 的 permitted（不在名单里回一条拒绝），
          QQ 见 adapter/qq/inbound.go 的 isUserAllowed（不在名单里直接忽略）。
        -->
        <li>
          绑定之后这个聊天账号才能发命令，而且目前<strong>每个完成绑定的账号都拥有管理员权限</strong>，
          可以暂停、删除种子和管理 RSS 订阅。绑定码只发给信任的人，不再需要的绑定及时撤销。
        </li>
        <li>
          通道凭证里的两栏名单（Telegram 的 <code>admin_users</code> /
          <code>allowed_users</code>，QQ 的 <code>admin_qq_users</code> /
          <code>allowed_qq_users</code>）只决定谁能和机器人说话，
          两栏都空时谁都用不了；它们不决定谁是管理员。
        </li>
      </ol>
    </PtPanel>

    <el-dialog
      v-model="generateDialogVisible"
      class="pt-dialog"
      title="生成绑定码"
      width="440px"
      align-center
      :before-close="handleCloseGenerateDialog">
      <template v-if="!generatedCode">
        <el-form class="pt-form" label-position="top" @submit.prevent>
          <el-form-item label="关联通道">
            <el-select v-model="selectedConfId" placeholder="请选择通道" style="width: 100%">
              <el-option
                v-for="conf in configs"
                :key="conf.id"
                :label="`${conf.name}（${conf.channel_type}）`"
                :value="conf.id" />
            </el-select>
            <div class="field-tip">绑定码只能在这个通道里使用</div>
          </el-form-item>
          <el-form-item label="有效期">
            <el-select v-model="selectedTTL" placeholder="选择有效期" style="width: 100%">
              <el-option
                v-for="opt in TTL_OPTIONS"
                :key="opt.value"
                :label="opt.label"
                :value="opt.value" />
            </el-select>
            <div class="field-tip">越短越安全；「永久」适合自建的私有机器人</div>
          </el-form-item>
        </el-form>
      </template>

      <div v-else class="issued">
        <p class="issued__desc">在聊天客户端里把下面这串码发给机器人即可完成绑定：</p>
        <div class="issued__code">{{ generatedCode }}</div>
        <p class="issued__hint">
          {{
            generatedExpiresAt
              ? `过期时间：${formatDate(generatedExpiresAt)}`
              : "永久有效，无过期时间"
          }}
        </p>
        <el-button type="primary" @click="copyToClipboard(generatedCode)">
          <PtIcon name="copy" :size="14" /><span>复制绑定码</span>
        </el-button>
      </div>

      <template #footer>
        <template v-if="!generatedCode">
          <el-button @click="handleCloseGenerateDialog">取消</el-button>
          <el-button type="primary" :loading="generating" @click="handleGenerateCode">
            生成
          </el-button>
        </template>
        <el-button v-else @click="handleCloseGenerateDialog">完成</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* 通道一栏：每个通道一段，段之间一条细线 */
.ch {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-pad);
}

.ch + .ch {
  border-top: 1px solid var(--pt-border);
}

.ch__head {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
  align-items: center;
}

.ch__name {
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t1);
}

.ch__spacer {
  flex: 1;
}

.ch__line {
  display: flex;
  gap: 6px;
  align-items: flex-start;
  margin: 0;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.ch__line .pt-icon {
  flex: 0 0 auto;
  margin-top: 3px;
}

.ch__line.is-warn {
  color: var(--pt-warn);
}

.ch__users {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

/* 一行：ID、身份、绑定状态、最近一次往来、操作；窄屏折行 */
.ch__user {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2) var(--pt-space-3);
  align-items: center;
  min-height: 44px;
  padding: 6px var(--pt-space-3);
}

.ch__user + .ch__user {
  border-top: 1px solid var(--pt-border);
}

.ch__uid {
  min-width: 92px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-sm);
  color: var(--pt-t1);
}

.ch__roles {
  display: inline-flex;
  gap: 4px;
  align-items: center;
}

.ch__warn {
  font-size: var(--pt-fz-label);
  color: var(--pt-warn);
}

.ch__last {
  flex: 1;
  min-width: 160px;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 绑定流程说明卡：有序步骤 */
.bind-note {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding-left: 20px;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.bind-note code {
  padding: 1px 5px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

/* 内缩 16、卡间 16 由 .pt-cards--wide 给，宽屏的主栏 + 侧栏由 .pt-cards--aside 给 */

/* 码 + 复制按钮是一个整体，别让复制按钮掉到第二行 */
.code-cell {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
}

.code {
  padding: 2px 8px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-body);
  font-weight: 600;
  letter-spacing: 0.08em;
  color: var(--pt-p);
  background: var(--pt-p-soft);
  border-radius: var(--pt-r-sm);
}

.uid {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t2);
}

/* 行卡标题位上的用户 ID：等宽体保留，但要用正文字号和主文色，
   不能沿用表格里那个 11 号次要色 —— 在卡上它是标题，不是次要信息 */
.uid--card {
  font-size: var(--pt-fz-body);
  color: var(--pt-t1);
}

.cd {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-sm);
  font-variant-numeric: tabular-nums;
}

.cd--live {
  color: var(--pt-t1);
}

.cd--urgent {
  font-weight: 600;
  color: var(--pt-dang);
}

.cd--forever {
  color: var(--pt-ok);
}

.cd--expired {
  color: var(--pt-t3);
  text-decoration: line-through;
}

/* 移动端行卡列表：面板 padding="none"，所以留白由这里给 */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-pad) 0;
}

/* 发码结果：一屏里只有这串码值得看，所以给它整块居中和最大的字号 */
.issued {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-3);
  align-items: center;
  text-align: center;
}

.issued__desc {
  margin: 0;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.issued__code {
  width: 100%;
  padding: var(--pt-space-4);
  font-family: var(--pt-font-mono);
  font-size: 30px;
  font-weight: 700;
  letter-spacing: 0.18em;
  color: var(--pt-p);
  word-break: break-all;
  background: var(--pt-p-soft);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.issued__hint {
  margin: 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}
</style>
