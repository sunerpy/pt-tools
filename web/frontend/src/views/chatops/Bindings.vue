<script setup lang="ts">
import { type ChatOpBinding, type NotificationConfig, chatopsApi } from "@/api";
import {
  Check,
  ChatDotRound,
  ChatSquare,
  CircleCheck,
  Clock,
  Connection,
  CopyDocument,
  Key,
  Link,
  Plus,
  Refresh,
} from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, onUnmounted, ref } from "vue";

const loading = ref(false);
const pendingBindings = ref<ChatOpBinding[]>([]);
const activeBindings = ref<ChatOpBinding[]>([]);
const configs = ref<NotificationConfig[]>([]);

const generateDialogVisible = ref(false);
const selectedConfId = ref<number | null>(null);
const selectedTTL = ref<number>(300);
const remarkText = ref<string>("");

const ttlOptions = [
  { label: "5 分钟", value: 300 },
  { label: "1 小时", value: 3600 },
  { label: "1 天", value: 86400 },
  { label: "30 天", value: 2592000 },
  { label: "永久", value: 0 },
];

const generatedCode = ref<string | null>(null);
const generatedExpiresAt = ref<string | null>(null);
const generating = ref(false);

const now = ref(Date.now());
let timer: ReturnType<typeof setInterval>;

// 通道类型 → Element Plus tag type 映射
const channelTagType: Record<string, "" | "success" | "warning" | "info" | "danger"> = {
  telegram: "info",
  qq_onebot: "success",
  qq: "success",
  webhook: "danger",
  wecom_webhook: "warning",
  wecom: "warning",
};

const channelLabelMap: Record<string, string> = {
  telegram: "Telegram",
  qq_onebot: "QQ",
  qq: "QQ",
  webhook: "Webhook",
  wecom_webhook: "WeCom",
  wecom: "WeCom",
};

const channelIconMap: Record<string, unknown> = {
  telegram: ChatDotRound,
  qq_onebot: ChatSquare,
  qq: ChatSquare,
  webhook: Link,
  wecom_webhook: Connection,
  wecom: Connection,
};

const adminCount = computed(() => activeBindings.value.filter((b) => b.admin).length);

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

  const totalSec = Math.floor(diff / 1000);
  const d = Math.floor(totalSec / 86400);
  const h = Math.floor((totalSec % 86400) / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;

  if (d > 0) return `${d} 天 ${h} 小时`;
  if (h > 0) return `${h} 时 ${m.toString().padStart(2, "0")} 分`;
  return `${m} 分 ${s.toString().padStart(2, "0")} 秒`;
}

function getCountdownClass(expiresAt?: string) {
  if (!expiresAt) return "countdown countdown--permanent";
  const text = getCountdown(expiresAt);
  if (text === "已过期") return "countdown countdown--expired";
  const diff = new Date(expiresAt).getTime() - now.value;
  if (diff < 60_000) return "countdown countdown--urgent";
  if (diff < 5 * 60_000) return "countdown countdown--soon";
  return "countdown countdown--active";
}

function formatDate(dateStr?: string) {
  if (!dateStr) return "-";
  const d = new Date(dateStr);
  return d.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

function formatTimeAgo(dateStr?: string) {
  if (!dateStr) return "-";
  const ts = new Date(dateStr).getTime();
  if (!ts) return "-";
  const diffMs = now.value - ts;
  if (diffMs < 0) return formatDate(dateStr);
  const sec = Math.floor(diffMs / 1000);
  if (sec < 60) return "刚刚";
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min} 分钟前`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour} 小时前`;
  const day = Math.floor(hour / 24);
  if (day < 30) return `${day} 天前`;
  return formatDate(dateStr);
}

function maskUserId(userId?: string) {
  if (!userId) return "-";
  if (userId.length <= 6) return userId;
  return userId.slice(0, 2) + "***" + userId.slice(-4);
}

function getConfNameByConfId(confId?: number) {
  if (!confId) return "-";
  const conf = configs.value.find((c) => c.id === confId);
  return conf ? conf.name : `#${confId}`;
}

function getConfChannelType(confId?: number): string {
  if (!confId) return "";
  const conf = configs.value.find((c) => c.id === confId);
  return conf ? conf.channel_type : "";
}

function getChannelTagType(type: string) {
  return channelTagType[type] || "info";
}

function getChannelLabel(type: string) {
  return channelLabelMap[type] || type;
}

function getChannelIcon(type: string) {
  return channelIconMap[type] || ChatDotRound;
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
      remarkText.value ? remarkText.value : undefined,
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
  if (configs.value.length === 0) {
    ElMessage.warning("请先创建至少一个通知通道");
    return;
  }
  selectedConfId.value = configs.value[0].id;
  selectedTTL.value = 300;
  remarkText.value = "";
  generatedCode.value = null;
  generatedExpiresAt.value = null;
  generateDialogVisible.value = true;
}

// 复制到剪贴板，保留 HTTP 非安全上下文回退（document.execCommand）
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

async function handleDelete(id: number, label?: string) {
  try {
    await ElMessageBox.confirm(
      label
        ? `确定要撤销绑定「${label}」吗？该用户将无法再通过聊天客户端控制 pt-tools。`
        : "确定要撤销该绑定吗？",
      "撤销绑定",
      {
        type: "warning",
        confirmButtonText: "确定撤销",
        cancelButtonText: "取消",
      },
    );
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
    ElMessage.success(`已切换语言至 ${newLang === "zh" ? "中文" : "English"}`);
    row.reply_lang = newLang;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "切换语言失败");
  }
}
</script>

<template>
  <div class="page-container chatops-bindings-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">ChatOps 绑定</h1>
        <p class="page-subtitle">
          为聊天客户端用户颁发一次性绑定码，将通知通道用户与 pt-tools 账户关联。
        </p>
      </div>
      <div class="page-actions">
        <el-button size="default" :loading="loading" @click="loadData">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
        <el-button type="primary" size="default" @click="openGenerateDialog">
          <el-icon><Plus /></el-icon>
          生成绑定码
        </el-button>
      </div>
    </div>

    <!-- 汇总信息 -->
    <div class="summary-bar">
      <el-tag size="default" type="warning" effect="plain" round>
        <el-icon><Clock /></el-icon>
        待绑定 {{ pendingBindings.length }}
      </el-tag>
      <el-tag size="default" type="success" effect="plain" round>
        <el-icon><CircleCheck /></el-icon>
        已绑定 {{ activeBindings.length }}
      </el-tag>
      <el-tag v-if="adminCount > 0" size="default" type="danger" effect="plain" round>
        <el-icon><Key /></el-icon>
        管理员 {{ adminCount }}
      </el-tag>
    </div>

    <!-- 待绑定 Code 卡片 -->
    <div class="table-card section-card" v-loading="loading">
      <div class="table-card-header">
        <div class="table-card-header-title">
          <el-icon class="header-icon"><Clock /></el-icon>
          <span>待绑定 Code</span>
          <el-tag type="warning" size="small" effect="plain" round class="count-tag">
            {{ pendingBindings.length }}
          </el-tag>
        </div>
        <div class="table-card-header-meta">在有效期内尚未被聊天客户端激活的绑定码</div>
      </div>

      <div class="table-wrapper">
        <el-table
          :data="pendingBindings"
          class="pt-table bindings-table"
          :header-cell-style="{ background: 'var(--pt-bg-secondary)', fontWeight: 600 }"
          :empty-text="loading ? '加载中...' : '暂无待绑定 Code'">
          <el-table-column label="绑定码" min-width="220">
            <template #default="{ row }">
              <div class="code-cell">
                <code class="bind-code">{{ row.code }}</code>
                <el-tooltip content="复制绑定码" placement="top">
                  <el-button
                    text
                    type="primary"
                    size="small"
                    :icon="CopyDocument"
                    @click="copyToClipboard(row.code)"
                    aria-label="复制绑定码" />
                </el-tooltip>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="关联通道" min-width="180">
            <template #default="{ row }">
              <div class="conf-cell">
                <el-tag
                  :type="getChannelTagType(getConfChannelType(row.conf_id))"
                  size="small"
                  effect="light"
                  round>
                  <el-icon class="tag-icon">
                    <component :is="getChannelIcon(getConfChannelType(row.conf_id))" />
                  </el-icon>
                  {{ getChannelLabel(getConfChannelType(row.conf_id)) }}
                </el-tag>
                <span class="conf-name">{{ getConfNameByConfId(row.conf_id) }}</span>
              </div>
            </template>
          </el-table-column>

          <el-table-column prop="label" label="备注" min-width="140">
            <template #default="{ row }">
              <span class="meta-text">{{ row.label || "-" }}</span>
            </template>
          </el-table-column>

          <el-table-column label="创建时间" width="180">
            <template #default="{ row }">
              <el-tooltip :content="formatDate(row.created_at)" placement="top">
                <span class="meta-text">{{ formatTimeAgo(row.created_at) }}</span>
              </el-tooltip>
            </template>
          </el-table-column>

          <el-table-column label="剩余有效时间" min-width="160">
            <template #default="{ row }">
              <span :class="getCountdownClass(row.expires_at)">
                {{ getCountdown(row.expires_at) }}
              </span>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <!-- 已绑定列表卡片 -->
    <div class="table-card section-card" v-loading="loading">
      <div class="table-card-header">
        <div class="table-card-header-title">
          <el-icon class="header-icon"><CircleCheck /></el-icon>
          <span>已绑定列表</span>
          <el-tag type="success" size="small" effect="plain" round class="count-tag">
            {{ activeBindings.length }}
          </el-tag>
        </div>
        <div class="table-card-header-meta">已通过绑定码激活的聊天客户端用户</div>
      </div>

      <div class="table-wrapper">
        <el-table
          :data="activeBindings"
          class="pt-table bindings-table"
          :header-cell-style="{ background: 'var(--pt-bg-secondary)', fontWeight: 600 }"
          :empty-text="loading ? '加载中...' : '暂无绑定，先生成绑定码'">
          <el-table-column label="通道" width="160">
            <template #default="{ row }">
              <el-tag
                :type="getChannelTagType(row.channel_type)"
                size="small"
                effect="light"
                round>
                <el-icon class="tag-icon">
                  <component :is="getChannelIcon(row.channel_type)" />
                </el-icon>
                {{ getChannelLabel(row.channel_type) }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column label="渠道用户ID" min-width="160">
            <template #default="{ row }">
              <el-tooltip :content="row.channel_user_id || '-'" placement="top">
                <code class="user-id-mono">{{ maskUserId(row.channel_user_id) }}</code>
              </el-tooltip>
            </template>
          </el-table-column>

          <el-table-column prop="label" label="备注" min-width="140">
            <template #default="{ row }">
              <span class="meta-text">{{ row.label || "-" }}</span>
            </template>
          </el-table-column>

          <el-table-column label="回复语言" width="120" align="center">
            <template #default="{ row }">
              <el-tag
                size="small"
                effect="light"
                round
                :type="row.reply_lang === 'zh' ? 'success' : 'info'">
                {{ row.reply_lang === "zh" ? "中文" : "English" }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column label="管理员" width="100" align="center">
            <template #default="{ row }">
              <el-tag
                v-if="row.admin"
                size="small"
                effect="dark"
                round
                type="warning">
                <el-icon class="tag-icon"><Key /></el-icon>
                Admin
              </el-tag>
              <span v-else class="text-tertiary">—</span>
            </template>
          </el-table-column>

          <el-table-column label="最后活跃" width="160">
            <template #default="{ row }">
              <el-tooltip :content="formatDate(row.last_active)" placement="top">
                <span class="meta-text">{{ formatTimeAgo(row.last_active) }}</span>
              </el-tooltip>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="180" align="right" fixed="right">
            <template #default="{ row }">
              <el-button
                text
                bg
                size="small"
                @click="handleToggleLang(row)">
                切换语言
              </el-button>
              <el-button
                text
                bg
                type="danger"
                size="small"
                @click="handleDelete(row.id, row.label || row.channel_user_id)">
                撤销
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>

    <!-- 生成绑定码对话框 -->
    <el-dialog
      v-model="generateDialogVisible"
      :title="generatedCode ? '绑定码已生成' : '生成绑定码'"
      width="480px"
      class="chatops-generate-dialog"
      :before-close="handleCloseGenerateDialog">
      <!-- 步骤 1: 生成 -->
      <div v-if="!generatedCode">
        <p class="dialog-desc">选择要绑定的通道、有效期与备注信息。</p>

        <el-form label-position="top">
          <el-form-item label="通道">
            <el-select
              v-model="selectedConfId"
              placeholder="请选择通道"
              size="default"
              style="width: 100%">
              <el-option
                v-for="conf in configs"
                :key="conf.id"
                :label="`${conf.name} · ${getChannelLabel(conf.channel_type)}`"
                :value="conf.id">
                <span class="select-option">
                  <el-tag
                    :type="getChannelTagType(conf.channel_type)"
                    size="small"
                    effect="light"
                    round>
                    {{ getChannelLabel(conf.channel_type) }}
                  </el-tag>
                  <span>{{ conf.name }}</span>
                </span>
              </el-option>
            </el-select>
          </el-form-item>

          <el-form-item label="有效期">
            <el-radio-group v-model="selectedTTL" class="ttl-radio-group">
              <el-radio-button
                v-for="opt in ttlOptions"
                :key="opt.value"
                :label="opt.value">
                {{ opt.label }}
              </el-radio-button>
            </el-radio-group>
          </el-form-item>

          <el-form-item label="备注 (可选)">
            <el-input
              v-model="remarkText"
              placeholder="例如：张三的 Telegram"
              maxlength="64"
              show-word-limit />
          </el-form-item>
        </el-form>
      </div>

      <!-- 步骤 2: 生成结果 -->
      <div v-else class="code-result">
        <div class="code-success-icon">
          <el-icon><CircleCheck /></el-icon>
        </div>
        <p class="dialog-desc">
          请在你的 Chat 客户端中给绑定的 bot 发送
          <code class="inline-cmd">/bind {{ generatedCode }}</code>
          完成绑定。
        </p>

        <div class="code-bubble">
          <div class="code-bubble-label">绑定码</div>
          <div class="code-bubble-row">
            <span class="big-code">{{ generatedCode }}</span>
            <el-button
              type="primary"
              :icon="CopyDocument"
              circle
              @click="copyToClipboard(generatedCode || '')"
              aria-label="复制绑定码" />
          </div>
        </div>

        <div class="expiry-row">
          <el-tag
            v-if="generatedExpiresAt"
            :type="getCountdownClass(generatedExpiresAt).includes('urgent') ? 'danger' : 'warning'"
            size="small"
            effect="plain"
            round>
            <el-icon><Clock /></el-icon>
            剩余 {{ getCountdown(generatedExpiresAt) }}
          </el-tag>
          <el-tag v-else type="success" size="small" effect="plain" round>
            <el-icon><Check /></el-icon>
            永久有效
          </el-tag>
          <span v-if="generatedExpiresAt" class="expiry-abs">
            过期时间：{{ formatDate(generatedExpiresAt) }}
          </span>
        </div>
      </div>

      <template #footer>
        <span class="dialog-footer">
          <template v-if="!generatedCode">
            <el-button @click="handleCloseGenerateDialog">取消</el-button>
            <el-button type="primary" :loading="generating" @click="handleGenerateCode">
              生成
            </el-button>
          </template>
          <template v-else>
            <el-button
              :icon="CopyDocument"
              @click="copyToClipboard(generatedCode || '')">
              复制绑定码
            </el-button>
            <el-button type="primary" @click="handleCloseGenerateDialog"> 完成 </el-button>
          </template>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";
@import "@/styles/table-page.css";

.chatops-bindings-page {
  padding: var(--pt-space-4) var(--pt-space-5) var(--pt-space-8);
}

/* 汇总条 */
.summary-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--pt-space-2);
  padding: var(--pt-space-3) var(--pt-space-4);
  margin-bottom: var(--pt-space-5);
  background: var(--pt-bg-surface-raised);
  border: 1px solid var(--pt-border-color);
  border-radius: var(--pt-radius-lg);
  box-shadow: var(--pt-shadow-sm);
}

.summary-bar :deep(.el-tag) {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

/* 卡片 */
.section-card {
  margin-bottom: var(--pt-space-5);
}

.section-card:last-of-type {
  margin-bottom: 0;
}

.table-card-header {
  flex-direction: column;
  align-items: flex-start;
  gap: var(--pt-space-1);
}

.table-card-header-title {
  display: flex;
  align-items: center;
  gap: var(--pt-space-2);
  font-size: var(--pt-text-lg);
  font-weight: 700;
  color: var(--pt-text-primary);
}

.header-icon {
  color: var(--pt-color-primary);
  font-size: 18px;
}

.count-tag {
  margin-left: var(--pt-space-1);
}

.table-card-header-meta {
  font-size: var(--pt-text-xs);
  color: var(--pt-text-secondary);
  font-weight: 400;
  margin-left: 26px;
}

/* 表格通用 */
.bindings-table :deep(.el-table) {
  --el-table-row-hover-bg-color: color-mix(
    in srgb,
    var(--pt-color-primary) 5%,
    var(--pt-bg-surface-muted)
  );
}

/* 单元格 */
.code-cell {
  display: flex;
  align-items: center;
  gap: var(--pt-space-2);
}

.bind-code {
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  font-size: var(--pt-text-base);
  font-weight: 600;
  letter-spacing: 0.06em;
  color: var(--pt-color-primary);
  background: var(--pt-bg-accent-soft);
  padding: 4px 10px;
  border-radius: var(--pt-radius-md);
  border: 1px solid color-mix(in srgb, var(--pt-color-primary) 18%, var(--pt-border-color));
}

.conf-cell {
  display: flex;
  align-items: center;
  gap: var(--pt-space-2);
  min-width: 0;
}

.conf-name {
  font-size: var(--pt-text-sm);
  color: var(--pt-text-primary);
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.user-id-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: var(--pt-text-sm);
  color: var(--pt-text-secondary);
  background: var(--pt-bg-secondary);
  padding: 2px 8px;
  border-radius: var(--pt-radius-sm);
}

.meta-text {
  font-size: var(--pt-text-sm);
  color: var(--pt-text-secondary);
}

.text-tertiary {
  color: var(--pt-text-tertiary);
}

.tag-icon {
  margin-right: 2px;
  vertical-align: -1px;
}

/* 倒计时 */
.countdown {
  font-variant-numeric: tabular-nums;
  font-size: var(--pt-text-sm);
  font-weight: 600;
}

.countdown--active {
  color: var(--pt-text-primary);
}

.countdown--soon {
  color: var(--pt-color-warning);
}

.countdown--urgent {
  color: var(--pt-color-danger);
  animation: blink 1.5s ease-in-out infinite;
}

.countdown--expired {
  color: var(--pt-text-tertiary);
  text-decoration: line-through;
}

.countdown--permanent {
  color: var(--pt-color-success);
}

@keyframes blink {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.55;
  }
}

/* ===== 对话框 ===== */
.chatops-generate-dialog .dialog-desc {
  font-size: var(--pt-text-sm);
  color: var(--pt-text-secondary);
  line-height: 1.65;
  margin: 0 0 var(--pt-space-4);
}

.chatops-generate-dialog .inline-cmd {
  display: inline-block;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: var(--pt-text-sm);
  font-weight: 600;
  color: var(--pt-color-primary);
  background: var(--pt-bg-accent-soft);
  padding: 2px 8px;
  border-radius: var(--pt-radius-sm);
  border: 1px solid color-mix(in srgb, var(--pt-color-primary) 18%, var(--pt-border-color));
}

.chatops-generate-dialog :deep(.ttl-radio-group) {
  display: flex;
  flex-wrap: wrap;
  gap: 0;
}

.chatops-generate-dialog :deep(.ttl-radio-group .el-radio-button) {
  margin: 0;
}

.select-option {
  display: inline-flex;
  align-items: center;
  gap: var(--pt-space-2);
}

/* 生成结果 */
.code-result {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: var(--pt-space-3);
}

.code-success-icon {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  margin: 0 auto var(--pt-space-2);
  border-radius: 999px;
  background: color-mix(in srgb, var(--pt-color-success) 14%, transparent);
  color: var(--pt-color-success);
  font-size: 28px;
}

.code-bubble {
  padding: var(--pt-space-4) var(--pt-space-5);
  border-radius: var(--pt-radius-lg);
  background:
    radial-gradient(
      ellipse at top right,
      color-mix(in srgb, var(--pt-color-primary) 12%, transparent),
      transparent 70%
    ),
    var(--pt-bg-accent-soft);
  border: 1px solid color-mix(in srgb, var(--pt-color-primary) 24%, var(--pt-border-color));
}

.code-bubble-label {
  font-size: var(--pt-text-xs);
  color: var(--pt-text-secondary);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  margin-bottom: var(--pt-space-2);
}

.code-bubble-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pt-space-3);
}

.big-code {
  flex: 1;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 28px;
  font-weight: 700;
  letter-spacing: 0.18em;
  color: var(--pt-color-primary);
  word-break: break-all;
}

.expiry-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
}

.expiry-row :deep(.el-tag) {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.expiry-abs {
  font-size: var(--pt-text-xs);
  color: var(--pt-text-tertiary);
}

/* 暗色模式 */
html.dark .summary-bar,
html.dark .table-card {
  background: var(--pt-bg-surface);
}

html.dark .bind-code,
html.dark .chatops-generate-dialog .inline-cmd {
  background: color-mix(in srgb, var(--pt-color-primary) 14%, var(--pt-bg-surface));
}

/* 移动端 */
@media (max-width: 640px) {
  .chatops-bindings-page {
    padding: var(--pt-space-3);
  }

  .table-card-header {
    padding: var(--pt-space-3) var(--pt-space-4);
  }

  .big-code {
    font-size: 22px;
    letter-spacing: 0.12em;
  }
}
</style>
