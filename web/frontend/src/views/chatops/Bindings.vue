<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
import { chatopsApi, type ChatOpBinding, type NotificationConfig } from "@/api";
import { ElMessage, ElMessageBox } from "element-plus";
import { CopyDocument } from "@element-plus/icons-vue";

const loading = ref(false);
const pendingBindings = ref<ChatOpBinding[]>([]);
const activeBindings = ref<ChatOpBinding[]>([]);
const configs = ref<NotificationConfig[]>([]);

// For code generation dialog
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
  } catch (e: any) {
    ElMessage.error(e.message || "获取绑定列表失败");
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
  return `${m}分${s}秒`;
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
    loadData(); // reload list
  } catch (e: any) {
    ElMessage.error(e.message || "生成失败");
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

function copyToClipboard(text: string) {
  navigator.clipboard
    .writeText(text)
    .then(() => {
      ElMessage.success("已复制到剪贴板");
    })
    .catch(() => {
      ElMessage.error("复制失败，请手动选择复制");
    });
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
  } catch (e) {
    // cancelled
  }
}

async function handleToggleLang(row: ChatOpBinding) {
  const newLang = row.reply_lang === "zh" ? "en" : "zh";
  try {
    await chatopsApi.bindings.update(row.id, { reply_lang: newLang });
    ElMessage.success(`已切换语言至 ${newLang}`);
    row.reply_lang = newLang;
  } catch (e: any) {
    ElMessage.error(e.message || "切换语言失败");
  }
}

function getConfNameByChannelType(channelType: string) {
  const conf = configs.value.find((c) => c.channel_type === channelType);
  return conf ? conf.name : channelType;
}
</script>

<template>
  <div class="chatops-bindings p-4">
    <div class="flex justify-between items-center mb-4">
      <h2 class="text-xl font-bold m-0 text-[var(--pt-text-primary)]">ChatOps 绑定管理</h2>
      <el-button type="primary" @click="openGenerateDialog">生成绑定码</el-button>
    </div>

    <el-card class="mb-6 shadow-sm border-[var(--pt-border-color)]" body-style="padding: 0;">
      <template #header>
        <div class="font-bold">待绑定 Code (有效期内)</div>
      </template>
      <el-table :data="pendingBindings" v-loading="loading" style="width: 100%">
        <el-table-column prop="bind_code" label="绑定码" width="200">
          <template #default="{ row }">
            <div class="flex items-center gap-2">
              <span class="font-mono bg-gray-100 dark:bg-gray-800 px-2 py-1 rounded text-lg">{{
                row.bind_code
              }}</span>
              <el-button
                link
                type="primary"
                :icon="CopyDocument"
                @click="copyToClipboard(row.bind_code)"></el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="关联渠道" width="150">
          <template #default="{ row }">
            <el-tag size="small" type="info">{{
              getConfNameByChannelType(row.channel_type) || row.channel_type
            }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="剩余有效时间">
          <template #default="{ row }">
            <span
              :class="
                getCountdown(row.expires_at) === '已过期'
                  ? 'text-red-500'
                  : 'text-orange-500 font-bold'
              ">
              {{ getCountdown(row.expires_at) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button link type="danger" @click="handleDelete(row.id)">撤销</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card class="shadow-sm border-[var(--pt-border-color)]" body-style="padding: 0;">
      <template #header>
        <div class="font-bold">已绑定列表</div>
      </template>
      <el-table :data="activeBindings" v-loading="loading" style="width: 100%">
        <el-table-column prop="channel_type" label="渠道类型" width="120">
          <template #default="{ row }">
            <el-tag size="small">{{ row.channel_type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="渠道用户ID" width="150">
          <template #default="{ row }">
            <span class="font-mono">{{ maskUserId(row.channel_user_id) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="label" label="用户标识 / 备注" min-width="120">
          <template #default="{ row }">
            {{ row.label || "-" }}
          </template>
        </el-table-column>
        <el-table-column prop="reply_lang" label="回复语言" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="row.reply_lang === 'zh' ? 'success' : ''">
              {{ row.reply_lang === "zh" ? "中文" : "English" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="admin" label="管理员" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="row.admin ? 'danger' : 'info'">{{
              row.admin ? "是" : "否"
            }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="最后活跃时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.last_active) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="handleToggleLang(row)"> 切换语言 </el-button>
            <el-button link type="danger" @click="handleDelete(row.id)">撤销</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="generateDialogVisible"
      title="生成绑定码"
      width="400px"
      :before-close="handleCloseGenerateDialog">
      <div v-if="!generatedCode">
        <div class="mb-4">
          <div class="text-sm text-[var(--pt-text-secondary)] mb-2">选择要绑定的渠道配置：</div>
          <el-select v-model="selectedConfId" placeholder="请选择配置" style="width: 100%">
            <el-option
              v-for="conf in configs"
              :key="conf.id"
              :label="conf.name + ' (' + conf.channel_type + ')'"
              :value="conf.id" />
          </el-select>
        </div>
        <div class="flex justify-end">
          <el-button @click="handleCloseGenerateDialog">取消</el-button>
          <el-button type="primary" :loading="generating" @click="handleGenerateCode"
            >生成</el-button
          >
        </div>
      </div>
      <div v-else class="text-center">
        <div class="text-sm text-[var(--pt-text-secondary)] mb-4">
          请在 Chat 客户端中发送以下绑定码：
        </div>
        <div
          class="bg-gray-100 dark:bg-gray-800 p-4 rounded-lg mb-4 flex items-center justify-center gap-4">
          <span class="text-3xl font-mono tracking-widest text-[var(--pt-color-primary)]">{{
            generatedCode
          }}</span>
        </div>
        <div class="mb-4">
          <el-button type="success" :icon="CopyDocument" @click="copyToClipboard(generatedCode)"
            >复制绑定码</el-button
          >
        </div>
        <div class="text-xs text-[var(--pt-text-secondary)]">
          过期时间: {{ formatDate(generatedExpiresAt || undefined) }}
        </div>
        <div class="mt-6 flex justify-end">
          <el-button @click="handleCloseGenerateDialog">完成</el-button>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
/* Scoped styles using pt-tools CSS variables for dark mode compatibility */
.chatops-bindings {
  background-color: var(--pt-bg-base);
  color: var(--pt-text-primary);
  min-height: 100%;
}
</style>
