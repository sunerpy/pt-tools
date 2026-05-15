<script setup lang="ts">
import { type NotificationConfig, chatopsApi } from "@/api";
import { ChatDotRound, Plus } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";

const router = useRouter();
const loading = ref(false);
const notifications = ref<NotificationConfig[]>([]);

// Dialog for adding a new channel
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

const channelTypeOptions = [
  { value: "telegram", label: "Telegram", icon: "ChatDotRound" },
  { value: "qq_onebot", label: "QQ (OneBot)", icon: "ChatSquare" },
  { value: "webhook", label: "Webhook", icon: "Link" },
  { value: "wecom_webhook", label: "WeCom Webhook", icon: "Connection" },
];

onMounted(async () => {
  await loadNotifications();
});

async function loadNotifications() {
  loading.value = true;
  try {
    const data = await chatopsApi.notifications.list();
    notifications.value = data || [];
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
  }
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
  } catch (e: unknown) {
    row.enabled = !row.enabled; // revert
    ElMessage.error((e as Error).message || "操作失败");
  }
}

function handleEdit(row: NotificationConfig) {
  // Navigation to details page
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

function getChannelIcon(type: string) {
  const opt = channelTypeOptions.find((o) => o.value === type);
  return opt ? opt.icon : "ChatDotRound";
}

function getChannelLabel(type: string) {
  const opt = channelTypeOptions.find((o) => o.value === type);
  return opt ? opt.label : type;
}
</script>

<template>
  <div class="page-container">
    <div class="hero-block">
      <div class="hero-content">
        <span class="hero-eyebrow">CHATOPS · NOTIFICATIONS</span>
        <h1 class="hero-title">通知通道</h1>
        <p class="hero-subtitle">
          管理与即时通讯软件的连接，接收系统通知并通过聊天界面控制 pt-tools。
        </p>
        <div class="hero-actions">
          <el-button
            type="primary"
            size="large"
            @click="openAddDialog"
            data-testid="add-channel-btn">
            <el-icon><Plus /></el-icon>
            添加通道
          </el-button>
          <span class="hero-meta">已配置 {{ notifications.length }} 个通道</span>
        </div>
      </div>
    </div>

    <el-skeleton v-if="loading && notifications.length === 0" :rows="6" animated class="mt-4" />

    <div v-else-if="notifications.length > 0" class="cards-grid">
      <article
        v-for="item in notifications"
        :key="item.id"
        class="channel-card"
        :data-testid="`channel-card-${item.name}`">
        <div class="card-accent" :data-channel="item.channel_type"></div>
        <div class="card-header">
          <div class="channel-brand">
            <el-icon class="brand-icon"
              ><component :is="getChannelIcon(item.channel_type)"
            /></el-icon>
            <span class="brand-name">{{ getChannelLabel(item.channel_type) }}</span>
          </div>
          <el-switch v-model="item.enabled" @change="handleToggle(item)" />
        </div>

        <div class="card-body">
          <h3 class="channel-name">{{ item.name }}</h3>
          <p class="channel-status" :class="{ 'is-active': item.enabled }">
            {{ item.enabled ? "运行中" : "已停用" }}
          </p>
        </div>

        <div class="card-footer">
          <el-button size="small" @click="handleEdit(item)">设置</el-button>
          <el-button size="small" @click="handleTest(item)" :disabled="!item.enabled"
            >测试</el-button
          >
          <div class="spacer"></div>
          <el-button size="small" type="danger" plain @click="handleDelete(item)">删除</el-button>
        </div>
      </article>
    </div>

    <div v-else class="empty-state">
      <div class="empty-icon">
        <el-icon><ChatDotRound /></el-icon>
      </div>
      <h3 class="empty-title">尚未配置任何通知通道</h3>
      <p class="empty-desc">
        添加 Telegram / QQ / Webhook / 企业微信 通道，让 pt-tools 主动推送任务结果与告警。
      </p>
      <el-button type="primary" size="large" @click="openAddDialog">
        <el-icon><Plus /></el-icon>
        添加第一个通道
      </el-button>
    </div>

    <el-dialog v-model="addDialogVisible" title="添加通知通道" width="500px">
      <el-form label-position="top" @submit.prevent>
        <el-form-item label="通道类型">
          <el-select v-model="newChannel.channel_type" class="w-full">
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
        </el-form-item>

        <template v-if="newChannel.channel_type === 'telegram'">
          <el-form-item label="Bot Token" required>
            <el-input
              v-model="newChannel.bot_token"
              type="password"
              show-password
              placeholder="123456789:ABCdefGHIjklMNOpqrsTUVwxyz"
              data-testid="bot-token-input" />
          </el-form-item>
        </template>

        <template v-if="newChannel.channel_type === 'webhook'">
          <el-form-item label="Endpoint URL" required>
            <el-input v-model="newChannel.endpoint_url" placeholder="https://..." />
          </el-form-item>
        </template>

        <template v-if="newChannel.channel_type === 'wecom_webhook'">
          <el-form-item label="Webhook Key" required>
            <el-input v-model="newChannel.webhook_key" placeholder="企业微信群机器人的 key" />
          </el-form-item>
        </template>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="addDialogVisible = false">取消</el-button>
          <el-button
            type="primary"
            @click="handleCreate"
            :loading="submitting"
            data-testid="save-btn"
            >确定</el-button
          >
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-container {
  padding: 16px 24px 32px;
}

.hero-block {
  position: relative;
  padding: 48px 32px;
  margin-bottom: 32px;
  border-radius: 22px;
  background:
    radial-gradient(
      ellipse at top right,
      color-mix(in oklab, var(--pt-color-primary) 18%, transparent),
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
  max-width: 720px;
}

.hero-eyebrow {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.18em;
  color: var(--pt-color-primary);
  text-transform: uppercase;
}

.hero-title {
  font-size: 40px;
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
  font-size: 16px;
  color: var(--pt-text-secondary);
  margin: 0;
  max-width: 600px;
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

.cards-grid {
  display: grid;
  grid-template-columns: repeat(1, 1fr);
  gap: 20px;
}

@media (min-width: 768px) {
  .cards-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1024px) {
  .cards-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}

.channel-card {
  position: relative;
  border-radius: 18px;
  background: color-mix(in oklab, var(--pt-bg-surface) 78%, transparent);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid var(--pt-border-color);
  box-shadow: 0 1px 2px rgb(28 25 23 / 4%);
  transition:
    transform 200ms cubic-bezier(0.16, 1, 0.3, 1),
    box-shadow 200ms cubic-bezier(0.16, 1, 0.3, 1),
    border-color 200ms ease;
  display: flex;
  flex-direction: column;
  padding: 22px;
  overflow: hidden;
}

.channel-card:hover {
  transform: translateY(-3px);
  box-shadow:
    0 1px 2px rgb(28 25 23 / 4%),
    0 12px 32px -16px rgb(28 25 23 / 14%);
  border-color: color-mix(in oklab, var(--pt-color-primary) 25%, var(--pt-border-color));
}

.card-accent {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: linear-gradient(
    90deg,
    var(--pt-color-primary) 0%,
    color-mix(in oklab, var(--pt-color-primary) 40%, transparent) 100%
  );
  opacity: 0.85;
}

.card-accent[data-channel="telegram"] {
  background: linear-gradient(90deg, #2aabee 0%, color-mix(in oklab, #2aabee 30%, transparent));
}

.card-accent[data-channel="qq_onebot"] {
  background: linear-gradient(90deg, #12b7f5 0%, color-mix(in oklab, #12b7f5 30%, transparent));
}

.card-accent[data-channel="wecom_webhook"] {
  background: linear-gradient(90deg, #07c160 0%, color-mix(in oklab, #07c160 30%, transparent));
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.channel-brand {
  display: flex;
  align-items: center;
  gap: 10px;
}

.brand-icon {
  font-size: 20px;
  color: var(--pt-color-primary);
  background: color-mix(in oklab, var(--pt-color-primary) 12%, transparent);
  padding: 9px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.brand-name {
  font-weight: 600;
  color: var(--pt-text-secondary);
  font-size: 13px;
  letter-spacing: 0.02em;
}

.card-body {
  flex: 1;
  margin-bottom: 24px;
}

.channel-name {
  font-size: 19px;
  font-weight: 600;
  margin: 0 0 8px 0;
  color: var(--pt-text-primary);
  letter-spacing: -0.01em;
}

.channel-status {
  font-size: 13px;
  color: var(--pt-text-secondary);
  margin: 0;
  display: flex;
  align-items: center;
  gap: 8px;
}

.channel-status::before {
  content: "";
  display: block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: color-mix(in oklab, var(--pt-text-secondary) 40%, transparent);
}

.channel-status.is-active::before {
  background: var(--pt-color-success, #16a34a);
  box-shadow: 0 0 10px color-mix(in oklab, var(--pt-color-success, #16a34a) 50%, transparent);
  animation: pulse-dot 2s ease-in-out infinite;
}

@keyframes pulse-dot {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.7;
    transform: scale(1.18);
  }
}

.card-footer {
  display: flex;
  gap: 8px;
  padding-top: 16px;
  border-top: 1px solid color-mix(in oklab, var(--pt-border-color) 60%, transparent);
}

.spacer {
  flex: 1;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
  padding: 64px 24px;
  margin: 24px auto;
  max-width: 520px;
  text-align: center;
  border-radius: 22px;
  background: color-mix(in oklab, var(--pt-bg-surface) 70%, transparent);
  border: 1px dashed var(--pt-border-color);
}

.empty-icon {
  display: grid;
  place-items: center;
  width: 72px;
  height: 72px;
  border-radius: 999px;
  background: color-mix(in oklab, var(--pt-color-primary) 10%, transparent);
  color: var(--pt-color-primary);
  font-size: 32px;
}

.empty-title {
  font-size: 20px;
  font-weight: 600;
  margin: 0;
  color: var(--pt-text-primary);
}

.empty-desc {
  font-size: 14px;
  color: var(--pt-text-secondary);
  margin: 0 0 8px;
  line-height: 1.65;
  max-width: 400px;
}

.mt-4 {
  margin-top: 16px;
}
.mt-8 {
  margin-top: 32px;
}
.w-full {
  width: 100%;
}
</style>
