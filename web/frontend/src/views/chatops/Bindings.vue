<template>
  <div class="bindings-page">
    <div class="hero-block">
      <div class="hero-content">
        <span class="hero-eyebrow">CHATOPS · BINDINGS</span>
        <h1 class="hero-title">绑定管理</h1>
        <p class="hero-subtitle">
          为聊天客户端用户颁发一次性绑定码，将通知通道用户与 pt-tools 账户关联。
        </p>
        <div class="hero-actions">
          <el-button type="primary" size="large" @click="openGenerateDialog">
            生成绑定码
          </el-button>
          <span class="hero-meta">
            待绑定 {{ pendingBindings.length }} · 已绑定 {{ activeBindings.length }}
          </span>
        </div>
      </div>
    </div>

    <section class="glass-card">
      <header class="card-section-header">
        <div class="title-block">
          <h2 class="section-title">待绑定 Code</h2>
          <p class="section-desc">展示当前有效期内未被使用的绑定码</p>
        </div>
        <el-tag round type="warning" effect="plain" size="default">
          {{ pendingBindings.length }} 条待激活
        </el-tag>
      </header>

      <el-table
        :data="pendingBindings"
        v-loading="loading"
        class="bindings-table"
        :empty-text="loading ? '加载中...' : '暂无待绑定 Code'">
        <el-table-column prop="code" label="绑定码" width="220">
          <template #default="{ row }">
            <div class="code-cell">
              <span class="bind-code">{{ row.code }}</span>
              <el-button
                link
                type="primary"
                :icon="CopyDocument"
                @click="copyToClipboard(row.code)"
                aria-label="复制绑定码"></el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="关联渠道" width="180">
          <template #default="{ row }">
            <el-tag round size="small" effect="plain">
              {{ getConfNameByConfId(row.conf_id) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="label" label="备注" min-width="140">
          <template #default="{ row }">
            <span class="meta-text">{{ row.label || "-" }}</span>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">
            <span class="meta-text">{{ formatDate(row.created_at) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="剩余有效时间" min-width="140">
          <template #default="{ row }">
            <span :class="getCountdownClass(row.expires_at)">
              {{ getCountdown(row.expires_at) }}
            </span>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <section class="glass-card">
      <header class="card-section-header">
        <div class="title-block">
          <h2 class="section-title">已绑定列表</h2>
          <p class="section-desc">已激活的聊天客户端用户绑定</p>
        </div>
        <el-tag round type="success" effect="plain" size="default">
          {{ activeBindings.length }} 个已绑定
        </el-tag>
      </header>

      <el-table
        :data="activeBindings"
        v-loading="loading"
        class="bindings-table"
        :empty-text="loading ? '加载中...' : '暂无绑定，先生成绑定码'">
        <el-table-column prop="channel_type" label="渠道类型" width="120">
          <template #default="{ row }">
            <el-tag round size="small">{{ row.channel_type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="渠道用户ID" width="180">
          <template #default="{ row }">
            <span class="mono-text">{{ maskUserId(row.channel_user_id) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="label" label="备注" min-width="140">
          <template #default="{ row }">
            <span class="meta-text">{{ row.label || "-" }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="reply_lang" label="回复语言" width="110">
          <template #default="{ row }">
            <el-tag round size="small" :type="row.reply_lang === 'zh' ? 'success' : 'info'">
              {{ row.reply_lang === "zh" ? "中文" : "English" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="admin" label="管理员" width="90">
          <template #default="{ row }">
            <el-tag round size="small" :type="row.admin ? 'danger' : 'info'" effect="plain">
              {{ row.admin ? "是" : "否" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="最后活跃" width="180">
          <template #default="{ row }">
            <span class="meta-text">{{ formatDate(row.last_active) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="180" fixed="right" align="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="handleToggleLang(row)">切换语言</el-button>
            <el-button link type="danger" @click="handleDelete(row.id)">撤销</el-button>
          </template>
        </el-table-column>
      </el-table>
    </section>

    <el-dialog
      v-model="generateDialogVisible"
      title="生成绑定码"
      width="440px"
      :before-close="handleCloseGenerateDialog">
      <div v-if="!generatedCode">
        <p class="dialog-desc">选择要绑定的渠道配置，将生成 6 位有效期 5 分钟的绑定码。</p>
        <el-select v-model="selectedConfId" placeholder="请选择配置" class="w-full">
          <el-option
            v-for="conf in configs"
            :key="conf.id"
            :label="conf.name + ' (' + conf.channel_type + ')'"
            :value="conf.id" />
        </el-select>
        <div class="dialog-actions">
          <el-button @click="handleCloseGenerateDialog">取消</el-button>
          <el-button type="primary" :loading="generating" @click="handleGenerateCode">
            生成
          </el-button>
        </div>
      </div>
      <div v-else class="code-display">
        <p class="dialog-desc">请在 Chat 客户端中发送以下绑定码完成绑定：</p>
        <div class="code-bubble">
          <span class="big-code">{{ generatedCode }}</span>
        </div>
        <el-button type="primary" :icon="CopyDocument" @click="copyToClipboard(generatedCode)">
          复制绑定码
        </el-button>
        <p class="expiry-hint">过期时间：{{ formatDate(generatedExpiresAt || undefined) }}</p>
        <div class="dialog-actions">
          <el-button @click="handleCloseGenerateDialog">完成</el-button>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
import { chatopsApi, type ChatOpBinding, type NotificationConfig } from "@/api";
import { ElMessage, ElMessageBox } from "element-plus";
import { CopyDocument } from "@element-plus/icons-vue";

const loading = ref(false);
const pendingBindings = ref<ChatOpBinding[]>([]);
const activeBindings = ref<ChatOpBinding[]>([]);
const configs = ref<NotificationConfig[]>([]);

const generateDialogVisible = ref(false);
const selectedConfId = ref<number | null>(null);
const generatedCode = ref<string | null>(null);
const generatedExpiresAt = ref<string | null>(null);
const generating = ref(false);

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
  if (!expiresAt) return "-";
  const end = new Date(expiresAt).getTime();
  const diff = end - now.value;
  if (diff <= 0) return "已过期";

  const m = Math.floor(diff / 60000);
  const s = Math.floor((diff % 60000) / 1000);
  return `${m} 分 ${s.toString().padStart(2, "0")} 秒`;
}

function getCountdownClass(expiresAt?: string) {
  const text = getCountdown(expiresAt);
  if (text === "已过期") return "countdown countdown--expired";
  if (text === "-") return "countdown";
  const diff = new Date(expiresAt!).getTime() - now.value;
  if (diff < 60000) return "countdown countdown--urgent";
  return "countdown countdown--active";
}

function formatDate(dateStr?: string) {
  if (!dateStr) return "-";
  const d = new Date(dateStr);
  return d.toLocaleString();
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
    const res = await chatopsApi.bindings.generateCode(selectedConfId.value);
    generatedCode.value = res.code;
    generatedExpiresAt.value = res.expires_at;
    loadData();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "生成失败");
  } finally {
    generating.value = false;
  }
}

function openGenerateDialog() {
  selectedConfId.value = configs.value.length > 0 ? configs.value[0].id : null;
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
    await ElMessageBox.confirm("确定要撤销该绑定吗？", "警告", {
      type: "warning",
      confirmButtonText: "确定",
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
    ElMessage.success(`已切换语言至 ${newLang}`);
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

<style scoped>
.bindings-page {
  padding: 16px 24px 32px;
  background-color: var(--pt-bg-base);
  color: var(--pt-text-primary);
  min-height: 100%;
}

.hero-block {
  position: relative;
  padding: 44px 32px;
  margin-bottom: 28px;
  border-radius: 22px;
  background:
    radial-gradient(
      ellipse at top right,
      color-mix(in oklab, var(--pt-color-primary) 16%, transparent),
      transparent 60%
    ),
    linear-gradient(
        to right,
        color-mix(in oklab, var(--pt-text-primary) 6%, transparent) 1px,
        transparent 1px
      )
      0 0 / 32px 32px,
    linear-gradient(
        to bottom,
        color-mix(in oklab, var(--pt-text-primary) 6%, transparent) 1px,
        transparent 1px
      )
      0 0 / 32px 32px,
    var(--pt-bg-surface);
  border: 1px solid var(--pt-border-color);
  overflow: hidden;
  box-shadow:
    0 1px 2px rgb(28 25 23 / 4%),
    0 8px 24px -12px rgb(28 25 23 / 8%);
}

.hero-content {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 700px;
}

.hero-eyebrow {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.18em;
  color: var(--pt-color-primary);
  text-transform: uppercase;
}

.hero-title {
  font-size: 36px;
  font-weight: 700;
  margin: 0;
  letter-spacing: -0.03em;
  line-height: 1.1;
  background: linear-gradient(135deg, var(--pt-text-primary) 25%, var(--pt-color-primary) 100%);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  color: transparent;
}

.hero-subtitle {
  font-size: 15px;
  color: var(--pt-text-secondary);
  margin: 0;
  max-width: 580px;
  line-height: 1.65;
}

.hero-actions {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-top: 12px;
  flex-wrap: wrap;
}

.hero-meta {
  font-size: 13px;
  color: var(--pt-text-secondary);
}

.glass-card {
  margin-bottom: 24px;
  padding: 24px;
  border-radius: 18px;
  background: color-mix(in oklab, var(--pt-bg-surface) 78%, transparent);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid var(--pt-border-color);
  box-shadow: 0 1px 2px rgb(28 25 23 / 4%);
  transition: box-shadow 200ms ease;
}

.glass-card:hover {
  box-shadow:
    0 1px 2px rgb(28 25 23 / 4%),
    0 8px 20px -14px rgb(28 25 23 / 10%);
}

.card-section-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
  padding-bottom: 14px;
  border-bottom: 1px solid color-mix(in oklab, var(--pt-border-color) 60%, transparent);
}

.title-block {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.section-title {
  font-size: 17px;
  font-weight: 600;
  margin: 0;
  color: var(--pt-text-primary);
  letter-spacing: -0.01em;
}

.section-desc {
  font-size: 13px;
  color: var(--pt-text-secondary);
  margin: 0;
}

.bindings-table :deep(.el-table) {
  background: transparent;
  --el-table-row-hover-bg-color: color-mix(in oklab, var(--pt-color-primary) 4%, transparent);
}

.bindings-table :deep(.el-table tr) {
  background: transparent;
  transition: background 200ms ease;
}

.bindings-table :deep(.el-table th.el-table__cell) {
  background: color-mix(in oklab, var(--pt-text-primary) 4%, transparent);
  color: var(--pt-text-secondary);
  font-weight: 500;
  font-size: 12.5px;
  letter-spacing: 0.02em;
  text-transform: uppercase;
}

.code-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}

.bind-code {
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  font-size: 17px;
  font-weight: 600;
  letter-spacing: 0.06em;
  color: var(--pt-color-primary);
  background: color-mix(in oklab, var(--pt-color-primary) 10%, transparent);
  padding: 5px 12px;
  border-radius: 8px;
}

.mono-text {
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  font-size: 13px;
  color: var(--pt-text-secondary);
}

.meta-text {
  font-size: 13px;
  color: var(--pt-text-secondary);
}

.countdown {
  font-variant-numeric: tabular-nums;
  font-size: 13px;
  font-weight: 500;
}

.countdown--active {
  color: var(--pt-color-primary);
}

.countdown--urgent {
  color: #ef4444;
  font-weight: 600;
  animation: blink 1.5s ease-in-out infinite;
}

.countdown--expired {
  color: var(--pt-text-secondary);
  text-decoration: line-through;
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

.dialog-desc {
  font-size: 13px;
  color: var(--pt-text-secondary);
  line-height: 1.65;
  margin: 0 0 16px;
}

.dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 24px;
}

.code-display {
  text-align: center;
  padding: 8px 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
}

.code-bubble {
  width: 100%;
  padding: 28px;
  border-radius: 16px;
  background:
    radial-gradient(
      ellipse at center,
      color-mix(in oklab, var(--pt-color-primary) 12%, transparent),
      transparent 70%
    ),
    color-mix(in oklab, var(--pt-color-primary) 6%, transparent);
  border: 1px solid color-mix(in oklab, var(--pt-color-primary) 24%, transparent);
}

.big-code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 36px;
  font-weight: 700;
  letter-spacing: 0.18em;
  color: var(--pt-color-primary);
}

.expiry-hint {
  font-size: 12px;
  color: var(--pt-text-secondary);
  margin: 0;
}

.w-full {
  width: 100%;
}
</style>
