<script setup lang="ts">
import { chatopsApi, type NotificationConfig } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
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
const notifications = ref<NotificationConfig[]>([]);

/**
 * 六态（设计文档 §5）：以前加载失败只弹一个 toast，列表随后画成「还没有通知通道」，
 * 用户会以为通道被清空了，去重新配一遍。这一页没有筛选，所以不会出现 zero。
 */
const { loading, state, errorText, run } = useDataState();

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
  await loadNotifications();
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
  <div class="notifications-page">
    <PtToolbar standalone :note="`已配置 ${notifications.length} 个通道`">
      <template #right>
        <el-button :loading="loading" @click="loadNotifications">
          <PtIcon name="refresh-cw" :size="14" /><span>刷新</span>
        </el-button>
        <el-button type="primary" data-testid="add-channel-btn" @click="openAddDialog">
          <PtIcon name="plus" :size="14" /><span>添加通道</span>
        </el-button>
      </template>
    </PtToolbar>

    <PtDataState
      v-if="notifications.length === 0"
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

    <div v-else class="ch-grid">
      <article
        v-for="item in notifications"
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
          <PtStatusPill :tone="item.enabled ? 'ok' : 'neutral'" size="sm">
            {{ item.enabled ? "运行中" : "已停用" }}
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
.notifications-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
}

/* 280 下限：通道名 + 开关一行，再窄开关就会被挤到第二行 */
.ch-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: var(--pt-space-4);
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
.ch-card__icon {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  color: var(--ch-c);
  background: color-mix(in srgb, var(--ch-c) 12%, transparent);
  border-radius: var(--pt-r-md);
}

.ch-card__name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  font-size: var(--pt-fz-h2);
  font-weight: 600;
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
