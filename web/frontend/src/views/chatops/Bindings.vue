<script setup lang="ts">
import { chatopsApi, type ChatOpBinding, type NotificationConfig } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, onUnmounted, ref } from "vue";

const TTL_OPTIONS = [
  { label: "5 分钟", value: 300 },
  { label: "1 小时", value: 3600 },
  { label: "1 天", value: 86400 },
  { label: "30 天", value: 2592000 },
  { label: "永久", value: 0 },
];

const isMobile = useIsMobile();

/** 画板 head 的 sub（11.5/400 t3）：待绑定与已绑定各几条 */
const headSub = computed(
  () => `待绑定 ${pendingBindings.value.length} · 已绑定 ${activeBindings.value.length}`,
);
const pendingBindings = ref<ChatOpBinding[]>([]);
const activeBindings = ref<ChatOpBinding[]>([]);
const configs = ref<NotificationConfig[]>([]);

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
const { loading, state, errorText, run, hasPartialBanner } = useDataState({
  failed: () => configsFailed.value,
});

const PARTIAL_SUB = "绑定列表已就绪，但渠道配置没取到，通道名称只能显示编号";

/** 状态块副标题：失败给真实错误，部分失败说清缺了什么，空态给下一步动作 */
function subFor(emptySub: string) {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  if (state.value === "partial") return PARTIAL_SUB;
  return emptySub;
}

const pendingSub = computed(() => subFor("生成一个绑定码，再去聊天客户端里把它发给机器人"));
const activeSub = computed(() => subFor("还没有用户完成绑定，先生成一个绑定码"));

/** 有绑定数据但渠道配置没拿到：在内容上方挂一条提示，而不是假装一切正常 */
const showPartialNote = computed(() =>
  hasPartialBanner(pendingBindings.value.length + activeBindings.value.length),
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
  const data = await run(async () => {
    const [bindingsRes, configsRes] = await Promise.all([
      chatopsApi.bindings.list(),
      // 渠道配置只用来把 conf_id 显示成通道名、以及填发码下拉，单独失败不该把整页判死
      chatopsApi.notifications.list().catch(() => {
        configsFailed.value += 1;
        return [] as NotificationConfig[];
      }),
    ]);
    configs.value = configsRes || [];
    return bindingsRes;
  });

  if (!data) {
    // 失败时清空：留着上一次的数据配一个「加载失败」的状态块更让人误解
    pendingBindings.value = [];
    activeBindings.value = [];
    return;
  }
  pendingBindings.value = data.pending || [];
  activeBindings.value = data.bindings || [];
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
  return new Date(dateStr).toLocaleString("zh-CN", { hour12: false });
}

function maskUserId(userId?: string) {
  if (!userId) return "-";
  if (userId.length <= 6) return userId;
  return userId.slice(0, 2) + "***" + userId.slice(-4);
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

async function handleDelete(id: number) {
  try {
    await ElMessageBox.confirm("确定要撤销该绑定吗？撤销后该用户需要重新拿码绑定。", "撤销绑定", {
      type: "warning",
      confirmButtonText: "确定撤销",
      cancelButtonText: "取消",
    });
    await chatopsApi.bindings.delete(id);
    ElMessage.success("绑定已撤销");
    loadData();
  } catch (_e) {
    /* user cancelled */
  }
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
  -->
  <div class="bindings-page pt-cards pt-cards--lead">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button :loading="loading" @click="loadData">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
      <el-button type="primary" @click="openGenerateDialog">
        <PtIcon name="plus" :size="15" /><span>生成绑定码</span>
      </el-button>
    </Teleport>

    <!-- 部分失败（§5 partial）：绑定拿到了、渠道配置没拿到，说清缺的是什么而不是静默降级 -->
    <div v-if="showPartialNote" class="pt-note pt-note--warn">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>
        渠道配置没取到：通道名称只能显示为编号，发码对话框里的通道下拉也会是空的。
        <el-button link type="primary" @click="loadData">重试</el-button>
      </span>
    </div>

    <PtPanel
      v-loading="loading"
      title="待绑定绑定码"
      icon="key-round"
      :count="`${pendingBindings.length} 条`"
      padding="none">
      <el-table
        v-if="!isMobile"
        :data="pendingBindings"
        class="pt-grid"
        row-key="code"
        style="width: 100%">
        <template #empty>
          <PtDataState :state="state" dense :sub="pendingSub">
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
      </el-table>

      <!--
        移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
        卡上留的是真正要做的事：把码复制走。所以码本身当标题，
        倒计时进 status（红字表示快过期了），复制做成一个撑满的主操作 —— 表格里那个
        16px 的链接图标在手机上根本点不准。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!pendingBindings.length" :state="state" :sub="pendingSub">
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
          </template>
        </PtRowCard>
      </div>

      <template v-if="pendingBindings.length > 0" #footer>
        <span class="pt-foot-note">绑定码一次有效，过期或用掉后会从这里消失</span>
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
          <PtDataState :state="state" dense :sub="activeSub">
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

        <el-table-column label="操作" width="150" fixed="right" class-name="pt-cell-act">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="handleToggleLang(row)">
              <PtIcon name="globe" :size="14" /><span>语言</span>
            </el-button>
            <el-button link type="danger" size="small" @click="handleDelete(row.id)">
              <PtIcon name="trash-2" :size="14" /><span>撤销</span>
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!--
        移动端行卡。管理员那一栏在卡上不能沿用表格的「是 / 否」—— 卡片没有列头，
        脱离表头的「否」读不出是在说什么，所以换成自解释的「管理员 / 普通」。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!activeBindings.length" :state="state" :sub="activeSub">
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
          管理员身份在通道的凭证里配置（admin_users / admin_qq_users），这里只展示
        </span>
      </template>
    </PtPanel>

    <!--
      画板 p-note 612：绑定流程说明。内容按代码核过 ——
      绑定码是 8 位、只存 bcrypt 哈希（models.BotToken.CodeOrTokenHash）、有 UsedAt 与
      ExpiresAt 两个状态；兑换走的是 ChatOps 的 `/bind <绑定码>` 命令
      （internal/chatops/commands/bind.go）。
    -->
    <PtPanel title="绑定是怎么走的" icon="info">
      <ol class="bind-note">
        <li>在这一页点「生成绑定码」，选好通道，拿到一个 8 位的码。</li>
        <li>
          在对应的聊天里发 <code>/bind &lt;绑定码&gt;</code>。机器人拿这个码换绑定关系，
          成功后这条记录从「待绑定」挪到「已绑定」。
        </li>
        <li>码只存哈希，页面上那一份关掉就再也看不到 —— 丢了就重新生成一个，旧的到期自动失效。</li>
        <li>
          绑定之后这个聊天账号才能发命令。管理员身份是另一回事 —— 它在通道的凭证里配置 （<code
            >admin_users</code
          >
          / <code>admin_qq_users</code>），上面那张表只做展示， 绑上不等于是管理员。
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
  border-radius: 3px;
}

/* 栏宽 612、内缩 16、卡间 16 都由 .pt-cards--lead 给 */

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
  color: var(--pt-t4);
  text-decoration: line-through;
}

/* 移动端行卡列表：面板 padding="none"，所以留白由这里给 */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-space-3);
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
