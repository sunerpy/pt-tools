<script setup lang="ts">
import { type NotificationConfig, chatopsApi } from "@/api";
import {
  ChatDotRound,
  ChatLineSquare,
  ChatSquare,
  Connection,
  Link,
  Plus,
  Refresh,
} from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref } from "vue";
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
  { value: "telegram", label: "Telegram", icon: ChatDotRound },
  { value: "qq_onebot", label: "QQ (OneBot)", icon: ChatSquare },
  { value: "webhook", label: "Webhook", icon: Link },
  { value: "wecom_webhook", label: "WeCom Webhook", icon: Connection },
];

// 通道类型 → Element Plus tag type 映射
const channelTagType: Record<string, "" | "success" | "warning" | "info" | "danger"> = {
  telegram: "info",
  qq_onebot: "success",
  webhook: "danger",
  wecom_webhook: "warning",
};

const enabledCount = computed(() => notifications.value.filter((n) => n.enabled).length);
const disabledCount = computed(() => notifications.value.length - enabledCount.value);
const typeBreakdown = computed(() => {
  const map: Record<string, number> = {};
  notifications.value.forEach((n) => {
    map[n.channel_type] = (map[n.channel_type] || 0) + 1;
  });
  return map;
});

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
  return opt ? opt.icon : ChatLineSquare;
}

function getChannelLabel(type: string) {
  const opt = channelTypeOptions.find((o) => o.value === type);
  return opt ? opt.label : type;
}

function getChannelTagType(type: string) {
  return channelTagType[type] || "info";
}
</script>

<template>
  <div class="page-container chatops-notifications-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">通知通道</h1>
        <p class="page-subtitle">
          管理与即时通讯软件的连接，接收系统通知并通过聊天界面控制 pt-tools。
        </p>
      </div>
      <div class="page-actions">
        <el-button size="default" :loading="loading" @click="loadNotifications">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
        <el-button
          type="primary"
          size="default"
          @click="openAddDialog"
          data-testid="add-channel-btn">
          <el-icon><Plus /></el-icon>
          添加通道
        </el-button>
      </div>
    </div>

    <!-- 汇总信息 -->
    <div v-if="notifications.length > 0" class="summary-bar">
      <el-tag size="default" type="info" effect="plain" round>
        共 {{ notifications.length }} 个通道
      </el-tag>
      <el-tag v-if="enabledCount > 0" size="default" type="success" effect="plain" round>
        {{ enabledCount }} 个启用中
      </el-tag>
      <el-tag v-if="disabledCount > 0" size="default" type="info" effect="plain" round>
        {{ disabledCount }} 个已停用
      </el-tag>
      <el-divider direction="vertical" />
      <el-tag
        v-for="(count, type) in typeBreakdown"
        :key="type"
        size="default"
        :type="getChannelTagType(String(type))"
        effect="light"
        round>
        {{ getChannelLabel(String(type)) }} · {{ count }}
      </el-tag>
    </div>

    <!-- 加载骨架 -->
    <el-skeleton
      v-if="loading && notifications.length === 0"
      :rows="6"
      animated
      class="loading-skeleton" />

    <!-- 通道卡片网格 -->
    <div v-else-if="notifications.length > 0" class="channels-grid">
      <article
        v-for="item in notifications"
        :key="item.id"
        class="channel-card"
        :class="{ 'is-disabled': !item.enabled }"
        :data-testid="`channel-card-${item.name}`">
        <div class="channel-card-accent" :data-channel="item.channel_type"></div>

        <div class="channel-card-header">
          <div class="channel-brand">
            <span class="channel-icon" :data-channel="item.channel_type">
              <el-icon><component :is="getChannelIcon(item.channel_type)" /></el-icon>
            </span>
            <el-tag
              :type="getChannelTagType(item.channel_type)"
              size="small"
              effect="light"
              round>
              {{ getChannelLabel(item.channel_type) }}
            </el-tag>
          </div>
          <el-switch
            v-model="item.enabled"
            inline-prompt
            active-text="启用"
            inactive-text="停用"
            @change="handleToggle(item)" />
        </div>

        <div class="channel-card-body">
          <h3 class="channel-name" :title="item.name">{{ item.name }}</h3>
          <div class="channel-meta">
            <el-tag size="small" type="info" effect="plain"> ID #{{ item.id }} </el-tag>
            <span class="status-pill" :class="item.enabled ? 'is-active' : 'is-idle'">
              <span class="status-dot"></span>
              {{ item.enabled ? "运行中" : "已停用" }}
            </span>
          </div>
        </div>

        <div class="channel-card-footer">
          <el-button-group>
            <el-button size="small" @click="handleEdit(item)"> 详情 </el-button>
            <el-button size="small" :disabled="!item.enabled" @click="handleTest(item)">
              测试
            </el-button>
          </el-button-group>
          <el-button size="small" type="danger" plain @click="handleDelete(item)"> 删除 </el-button>
        </div>
      </article>
    </div>

    <!-- 空态 -->
    <div v-else-if="!loading" class="empty-state">
      <el-empty :image-size="100" description="">
        <template #description>
          <h3 class="empty-title">尚未配置任何通知通道</h3>
          <p class="empty-desc">
            添加 Telegram / QQ / Webhook / 企业微信 通道，让 pt-tools 主动推送任务结果与告警。
          </p>
        </template>
        <el-button type="primary" size="default" @click="openAddDialog">
          <el-icon><Plus /></el-icon>
          添加你的第一个通知通道
        </el-button>
      </el-empty>
    </div>

    <!-- 新建通道对话框 -->
    <el-dialog
      v-model="addDialogVisible"
      title="添加通知通道"
      width="520px"
      class="chatops-add-dialog">
      <el-form label-position="top" @submit.prevent>
        <el-form-item label="通道类型">
          <el-radio-group v-model="newChannel.channel_type" class="channel-type-group">
            <el-radio-button
              v-for="opt in channelTypeOptions"
              :key="opt.value"
              :label="opt.value"
              :data-testid="`channel-type-${opt.value}`">
              <el-icon><component :is="opt.icon" /></el-icon>
              {{ opt.label }}
            </el-radio-button>
          </el-radio-group>
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
            :loading="submitting"
            data-testid="save-btn"
            @click="handleCreate">
            确定
          </el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";

.chatops-notifications-page {
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

.summary-bar :deep(.el-divider--vertical) {
  height: 16px;
  margin: 0 var(--pt-space-1);
}

.loading-skeleton {
  padding: var(--pt-space-6);
  background: var(--pt-bg-surface-raised);
  border: 1px solid var(--pt-border-color);
  border-radius: var(--pt-radius-xl);
}

/* 通道网格 */
.channels-grid {
  display: grid;
  grid-template-columns: repeat(1, 1fr);
  gap: var(--pt-space-4);
}

@media (min-width: 640px) {
  .channels-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (min-width: 1280px) {
  .channels-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}

@media (min-width: 1700px) {
  .channels-grid {
    grid-template-columns: repeat(4, 1fr);
  }
}

/* 通道卡片 */
.channel-card {
  position: relative;
  display: flex;
  flex-direction: column;
  padding: var(--pt-space-5);
  padding-top: calc(var(--pt-space-5) + 4px);
  background: var(--pt-bg-surface-raised);
  border: 1px solid var(--pt-border-color);
  border-radius: var(--pt-radius-xl);
  box-shadow: var(--pt-shadow-sm);
  transition:
    box-shadow var(--pt-transition-normal),
    border-color var(--pt-transition-normal),
    transform var(--pt-transition-normal);
  overflow: hidden;
}

.channel-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--pt-shadow-lg);
  border-color: color-mix(in srgb, var(--pt-color-primary) 30%, var(--pt-border-color));
}

.channel-card.is-disabled {
  opacity: 0.78;
}

.channel-card-accent {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: var(--pt-color-primary);
  opacity: 0.85;
}

.channel-card-accent[data-channel="telegram"] {
  background: linear-gradient(90deg, #2aabee, #229ed9);
}

.channel-card-accent[data-channel="qq_onebot"] {
  background: linear-gradient(90deg, #10b981, #059669);
}

.channel-card-accent[data-channel="wecom_webhook"] {
  background: linear-gradient(90deg, #f59e0b, #d97706);
}

.channel-card-accent[data-channel="webhook"] {
  background: linear-gradient(90deg, #ef4444, #dc2626);
}

.channel-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--pt-space-4);
}

.channel-brand {
  display: flex;
  align-items: center;
  gap: var(--pt-space-2);
}

.channel-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: var(--pt-radius-md);
  background: var(--pt-bg-accent-soft);
  color: var(--pt-color-primary);
  font-size: 18px;
  flex-shrink: 0;
}

.channel-icon[data-channel="telegram"] {
  background: color-mix(in srgb, #2aabee 14%, transparent);
  color: #2aabee;
}

.channel-icon[data-channel="qq_onebot"] {
  background: color-mix(in srgb, #10b981 14%, transparent);
  color: #10b981;
}

.channel-icon[data-channel="wecom_webhook"] {
  background: color-mix(in srgb, #f59e0b 14%, transparent);
  color: #d97706;
}

.channel-icon[data-channel="webhook"] {
  background: color-mix(in srgb, #ef4444 14%, transparent);
  color: #ef4444;
}

.channel-card-body {
  flex: 1;
  margin-bottom: var(--pt-space-4);
  min-height: 60px;
}

.channel-name {
  margin: 0 0 var(--pt-space-2);
  font-size: var(--pt-text-lg);
  font-weight: 700;
  color: var(--pt-text-primary);
  letter-spacing: -0.01em;
  line-height: 1.3;
  word-break: break-word;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.channel-meta {
  display: flex;
  align-items: center;
  gap: var(--pt-space-2);
  flex-wrap: wrap;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: var(--pt-text-xs);
  font-weight: 600;
  color: var(--pt-text-secondary);
}

.status-pill .status-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--pt-color-neutral-300);
}

.status-pill.is-active {
  color: var(--pt-color-success);
}

.status-pill.is-active .status-dot {
  background: var(--pt-color-success);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--pt-color-success) 22%, transparent);
  animation: pulse-dot 2s ease-in-out infinite;
}

@keyframes pulse-dot {
  0%,
  100% {
    transform: scale(1);
    opacity: 1;
  }
  50% {
    transform: scale(1.18);
    opacity: 0.78;
  }
}

.channel-card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pt-space-2);
  padding-top: var(--pt-space-3);
  border-top: 1px solid var(--pt-border-color);
}

/* 空态 */
.empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--pt-space-8) var(--pt-space-4);
  background: var(--pt-bg-surface-raised);
  border: 1px dashed var(--pt-border-color);
  border-radius: var(--pt-radius-xl);
}

.empty-title {
  margin: 0 0 var(--pt-space-2);
  font-size: var(--pt-text-lg);
  font-weight: 700;
  color: var(--pt-text-primary);
}

.empty-desc {
  margin: 0 auto var(--pt-space-4);
  max-width: 420px;
  font-size: var(--pt-text-sm);
  color: var(--pt-text-secondary);
  line-height: 1.6;
}

/* 对话框：通道类型 radio button group */
.chatops-add-dialog :deep(.channel-type-group) {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
}

.chatops-add-dialog :deep(.channel-type-group .el-radio-button) {
  margin: 0;
}

.chatops-add-dialog :deep(.channel-type-group .el-radio-button__inner) {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border-radius: var(--pt-radius-md) !important;
  border-left-width: 1px !important;
}

.chatops-add-dialog :deep(.channel-type-group .el-radio-button:not(:first-child)) {
  margin-left: 0;
}

/* 暗色模式微调 */
html.dark .channel-card {
  background: var(--pt-bg-surface);
}

html.dark .summary-bar {
  background: var(--pt-bg-surface);
}

/* 移动端 */
@media (max-width: 640px) {
  .chatops-notifications-page {
    padding: var(--pt-space-3);
  }

  .channel-card-footer {
    flex-direction: column;
    align-items: stretch;
  }

  .channel-card-footer .el-button-group {
    display: flex;
  }

  .channel-card-footer .el-button-group .el-button {
    flex: 1;
  }
}
</style>
