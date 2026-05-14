<script setup lang="ts">
import { type NotificationConfig, chatopsApi } from "@/api";
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
      <h1 class="hero-title">通知通道</h1>
      <p class="hero-subtitle">
        管理与即时通讯软件的连接，接收系统通知并通过聊天界面控制 pt-tools。
      </p>
      <div class="hero-actions">
        <el-button type="primary" size="large" @click="openAddDialog" data-testid="add-channel-btn">
          <el-icon><Plus /></el-icon>
          添加通道
        </el-button>
      </div>
    </div>

    <el-skeleton v-if="loading && notifications.length === 0" :rows="6" animated class="mt-4" />

    <div v-else-if="notifications.length > 0" class="cards-grid">
      <el-card
        v-for="item in notifications"
        :key="item.id"
        class="channel-card"
        :data-testid="`channel-card-${item.name}`"
        shadow="hover">
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
      </el-card>
    </div>

    <el-empty v-else description="暂无通知通道" class="mt-8">
      <el-button type="primary" @click="openAddDialog">添加通道</el-button>
    </el-empty>

    <!-- 添加通道弹窗 -->
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

        <!-- 根据类型显示不同凭证输入 -->
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

/* Delta UI style Hero */
.hero-block {
  position: relative;
  padding: 40px 24px;
  margin-bottom: 32px;
  border-radius: var(--pt-radius-xl, 16px);
  background:
    radial-gradient(
      ellipse at top right,
      color-mix(in oklab, var(--pt-color-primary) 15%, transparent),
      transparent 60%
    ),
    linear-gradient(to right, rgb(128 128 128 / 8%) 1px, transparent 1px) 0 0 / 32px 32px,
    linear-gradient(to bottom, rgb(128 128 128 / 8%) 1px, transparent 1px) 0 0 / 32px 32px,
    var(--pt-bg-surface);
  border: 1px solid var(--pt-border-color);
  overflow: hidden;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.hero-title {
  font-size: 32px;
  font-weight: 700;
  margin: 0;
  color: var(--pt-text-primary);
  letter-spacing: -0.02em;
}

.hero-subtitle {
  font-size: 16px;
  color: var(--pt-text-secondary);
  margin: 0;
  max-width: 600px;
  line-height: 1.6;
}

.hero-actions {
  margin-top: 8px;
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

/* Glassmorphism Card */
.channel-card {
  border-radius: var(--pt-radius-lg, 12px);
  background: color-mix(in oklab, var(--pt-bg-surface) 70%, transparent);
  backdrop-filter: blur(8px);
  border: 1px solid var(--pt-border-color);
  box-shadow: var(--pt-shadow-sm);
  transition:
    transform 200ms ease,
    box-shadow 200ms ease;
  display: flex;
  flex-direction: column;
}

.channel-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--pt-shadow-md);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.channel-brand {
  display: flex;
  align-items: center;
  gap: 8px;
}

.brand-icon {
  font-size: 20px;
  color: var(--pt-color-primary);
  background: color-mix(in oklab, var(--pt-color-primary) 10%, transparent);
  padding: 8px;
  border-radius: 8px;
}

.brand-name {
  font-weight: 600;
  color: var(--pt-text-secondary);
  font-size: 14px;
}

.card-body {
  flex: 1;
  margin-bottom: 24px;
}

.channel-name {
  font-size: 18px;
  font-weight: 600;
  margin: 0 0 8px 0;
  color: var(--pt-text-primary);
}

.channel-status {
  font-size: 13px;
  color: var(--pt-text-secondary);
  margin: 0;
  display: flex;
  align-items: center;
  gap: 6px;
}

.channel-status::before {
  content: "";
  display: block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--pt-color-neutral-400);
}

.channel-status.is-active::before {
  background: var(--pt-color-success);
  box-shadow: 0 0 8px color-mix(in oklab, var(--pt-color-success) 50%, transparent);
}

.card-footer {
  display: flex;
  gap: 8px;
  padding-top: 16px;
  border-top: 1px solid color-mix(in oklab, var(--pt-border-color) 50%, transparent);
}

.spacer {
  flex: 1;
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
