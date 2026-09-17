<script setup lang="ts">
import { chatopsApi, type ChatOpBinding, type NotificationConfig } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { onMounted, onUnmounted, ref } from "vue";

const TTL_OPTIONS = [
  { label: "5 分钟", value: 300 },
  { label: "1 小时", value: 3600 },
  { label: "1 天", value: 86400 },
  { label: "30 天", value: 2592000 },
  { label: "永久", value: 0 },
];

const loading = ref(false);
const pendingBindings = ref<ChatOpBinding[]>([]);
const activeBindings = ref<ChatOpBinding[]>([]);
const configs = ref<NotificationConfig[]>([]);

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
  loading.value = true;
  try {
    const [bindingsRes, configsRes] = await Promise.all([
      chatopsApi.bindings.list(),
      chatopsApi.notifications.list(),
    ]);
    pendingBindings.value = bindingsRes.pending || [];
    activeBindings.value = bindingsRes.bindings || [];
    configs.value = configsRes || [];
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "获取绑定列表失败");
  } finally {
    loading.value = false;
  }
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
  <div class="bindings-page">
    <PtToolbar
      standalone
      :note="`待绑定 ${pendingBindings.length} · 已绑定 ${activeBindings.length}`">
      <el-button size="small" :loading="loading" @click="loadData">
        <PtIcon name="refresh-cw" :size="14" /><span>刷新</span>
      </el-button>
      <template #right>
        <el-button type="primary" size="small" @click="openGenerateDialog">
          <PtIcon name="plus" :size="14" /><span>生成绑定码</span>
        </el-button>
      </template>
    </PtToolbar>

    <PtPanel
      v-loading="loading"
      title="待绑定绑定码"
      icon="key-round"
      :count="`${pendingBindings.length} 条`"
      padding="none">
      <el-table :data="pendingBindings" class="pt-grid" row-key="code" style="width: 100%">
        <template #empty>
          <PtDataState state="empty" dense sub="生成一个绑定码，再去聊天客户端里把它发给机器人" />
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
      <el-table :data="activeBindings" class="pt-grid" row-key="id" style="width: 100%">
        <template #empty>
          <PtDataState state="empty" dense sub="还没有用户完成绑定，先生成一个绑定码" />
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

      <template v-if="activeBindings.length > 0" #footer>
        <span class="pt-foot-note">
          管理员身份在通道的凭证里配置（admin_users / admin_qq_users），这里只展示
        </span>
      </template>
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
.bindings-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
}

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
