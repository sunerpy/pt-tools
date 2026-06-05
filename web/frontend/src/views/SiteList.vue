<script setup lang="ts">
import { ApiError, type SiteConfig, type SiteLoginState, chatopsApi, sitesApi } from "@/api";
import SiteAvatar from "@/components/SiteAvatar.vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";

import { formatTimeAgo } from "@/utils/format";
import { useLoginState } from "@/composables/useLoginState";

const router = useRouter();

const loading = ref(false);
const sites = ref<Record<string, SiteConfig>>({});
const loginStates = ref<Record<string, SiteLoginState>>({});
const probing = reactive<Record<string, boolean>>({});
const testingReminder = reactive<Record<string, boolean>>({});
const bulkProbing = ref(false);
const updatingMode = reactive<Record<string, boolean>>({});

const {
  loginState,
  effectiveLastActive,
  lastAccess,
  daysRemaining,
  reminderTier,
  probeModeOf,
  tierTagType,
  tierLabel,
  daysCellClass,
} = useLoginState(loginStates);

const viewMode = ref<"enabled" | "all">("enabled");
const addDialogVisible = ref(false);
const addSearch = ref("");
const enablingInDialog = reactive<Record<string, boolean>>({});

onMounted(async () => {
  await loadSites();
});

async function loadSites() {
  loading.value = true;
  try {
    const [siteMap, states] = await Promise.all([sitesApi.list(), sitesApi.listLoginStates()]);
    sites.value = siteMap;
    const byName: Record<string, SiteLoginState> = {};
    for (const st of states ?? []) {
      byName[st.site_name] = st;
    }
    loginStates.value = byName;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
  }
}

async function toggleEnabled(name: string) {
  const site = sites.value[name];
  if (!site) return;
  site.enabled = !site.enabled;
  try {
    await sitesApi.save(name, site);
    ElMessage.success("已保存");
  } catch (e: unknown) {
    site.enabled = !site.enabled;
    ElMessage.error((e as Error).message || "保存失败");
  }
}

async function deleteSite(name: string) {
  if (sites.value[name]?.is_builtin) {
    ElMessage.warning("预置站点不可删除");
    return;
  }

  try {
    await ElMessageBox.confirm(`确定删除站点 "${name}"？`, "确认删除", {
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      type: "warning",
    });
    await sitesApi.delete(name);
    ElMessage.success("已删除");
    await loadSites();
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "删除失败");
    }
  }
}

async function probeSite(name: string) {
  if (probing[name]) return;
  probing[name] = true;
  try {
    const res = await sitesApi.probeNow(name);
    ElMessage.success(`探测完成: ${res.last_probe_status || "ok"}`);
    await loadSites();
  } catch (e: unknown) {
    if (e instanceof ApiError && e.status === 409) {
      ElMessage.warning("探测进行中，请稍候");
    } else {
      ElMessage.error((e as Error).message || "探测失败");
    }
  } finally {
    probing[name] = false;
  }
}

async function sendTestReminder(name: string) {
  if (testingReminder[name]) return;
  testingReminder[name] = true;
  try {
    await sitesApi.testReminder(name);
    ElMessage.success("测试提醒已发送，请检查通知通道（TG/QQ 等）");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "发送失败");
  } finally {
    testingReminder[name] = false;
  }
}

async function probeAllEnabled() {
  if (bulkProbing.value) return;
  const names = allEntries.value.filter(([, site]) => site.enabled).map(([name]) => name);
  if (names.length === 0) {
    ElMessage.info("暂无已启用站点");
    return;
  }

  bulkProbing.value = true;
  let cursor = 0;
  let success = 0;
  let failed = 0;
  let skipped = 0;

  async function worker() {
    for (;;) {
      const index = cursor++;
      if (index >= names.length) return;
      const name = names[index];
      if (probing[name]) {
        skipped++;
        continue;
      }

      probing[name] = true;
      try {
        await sitesApi.probeNow(name);
        success++;
      } catch (e: unknown) {
        if (e instanceof ApiError && e.status === 409) skipped++;
        else failed++;
      } finally {
        probing[name] = false;
      }
    }
  }

  try {
    const concurrency = Math.min(3, names.length);
    await Promise.all(Array.from({ length: concurrency }, () => worker()));
    const parts = [`成功 ${success}`];
    if (skipped > 0) parts.push(`跳过 ${skipped}`);
    if (failed > 0) parts.push(`失败 ${failed}`);
    const message = `批量探测完成：${parts.join("，")}`;
    if (failed > 0) ElMessage.warning(message);
    else ElMessage.success(message);
    await loadSites();
  } finally {
    bulkProbing.value = false;
  }
}

function openSite(name: string) {
  const url =
    sites.value[name]?.web_url ?? sites.value[name]?.urls?.[0] ?? loginState(name)?.base_url;
  if (url) window.open(url, "_blank", "noopener");
}

function openAllEnabled() {
  const urls = allEntries.value
    .filter(([, s]) => s.enabled)
    .map(
      ([name]) =>
        sites.value[name]?.web_url ?? sites.value[name]?.urls?.[0] ?? loginState(name)?.base_url,
    )
    .filter((url): url is string => Boolean(url));

  if (urls.length === 0) {
    ElMessage.info("没有可打开的已启用站点");
    return;
  }

  if (urls.length === 1) {
    window.open(urls[0], "_blank");
    return;
  }

  let opened = 0;
  let blocked = 0;
  for (const url of urls) {
    const w = window.open(url, "_blank");
    if (w === null || typeof w === "undefined") {
      blocked++;
    } else {
      opened++;
    }
  }

  if (blocked > 0) {
    ElMessage({
      type: "warning",
      duration: 8000,
      showClose: true,
      message: `已打开 ${opened} 个站点，浏览器拦截了其余 ${blocked} 个。请在浏览器地址栏允许本站“弹出式窗口”后重试，或使用每行的“打开站点”按钮逐个打开。`,
    });
  } else {
    ElMessage.success(`已打开 ${opened} 个站点`);
  }
}

function manageSite(name: string) {
  router.push(`/sites/${name}`);
}

function getRssCount(site: SiteConfig): number {
  return site.rss?.length || 0;
}

const allEntries = computed(() => Object.entries(sites.value));

const enabledCount = computed(() => allEntries.value.filter(([, s]) => s.enabled).length);

const visibleEntries = computed(() => {
  if (viewMode.value === "all") return allEntries.value;
  return allEntries.value.filter(([, s]) => s.enabled);
});

const disabledEntries = computed(() => allEntries.value.filter(([, s]) => !s.enabled));

const addCandidates = computed(() => {
  const q = addSearch.value.trim().toLowerCase();
  if (!q) return disabledEntries.value;
  return disabledEntries.value.filter(([name, s]) => {
    if (name.toLowerCase().includes(q)) return true;
    if (s.urls?.some((u) => u.toLowerCase().includes(q))) return true;
    return false;
  });
});

function openAddDialog() {
  addSearch.value = "";
  addDialogVisible.value = true;
}

async function enableSiteFromDialog(name: string) {
  const site = sites.value[name];
  if (!site || enablingInDialog[name]) return;
  enablingInDialog[name] = true;
  const snapshot = site.enabled;
  site.enabled = true;
  try {
    await sitesApi.save(name, site);
    ElMessage.success(`已启用 ${name}`);
  } catch (e: unknown) {
    site.enabled = snapshot;
    ElMessage.error((e as Error).message || "启用失败");
  } finally {
    enablingInDialog[name] = false;
  }
}

function configureFromDialog(name: string) {
  addDialogVisible.value = false;
  router.push(`/sites/${name}`);
}

function authMethodLabel(method?: string): string {
  switch (method) {
    case "api_key":
      return "API Key";
    case "cookie_and_api_key":
      return "Cookie + API";
    case "passkey":
      return "Passkey";
    default:
      return "Cookie";
  }
}

function authMethodTagType(method?: string): "primary" | "success" | "warning" | "info" {
  switch (method) {
    case "api_key":
      return "warning";
    case "cookie_and_api_key":
      return "success";
    case "passkey":
      return "primary";
    default:
      return "info";
  }
}

async function changeProbeMode(name: string, mode: "auto" | "manual" | "disabled") {
  const st = loginStates.value[name];
  const previous = probeModeOf(name);
  if (previous === mode) return;
  if (updatingMode[name]) return;
  updatingMode[name] = true;
  if (st) st.probe_mode = mode;
  try {
    await sitesApi.updateProbeMode(name, mode);
    ElMessage.success("探测模式已更新");
  } catch (e: unknown) {
    if (st) st.probe_mode = previous;
    ElMessage.error((e as Error).message || "更新失败");
  } finally {
    updatingMode[name] = false;
  }
}

interface LoginConfigForm {
  ban_threshold_days: number;
  remind_before_days: number;
  reminder_cron: string;
  notification_channel_ids: number[];
  probe_mode: "auto" | "manual" | "disabled";
}

const configDialogVisible = ref(false);
const configSaving = ref(false);
const configSiteName = ref("");
const notifyChannels = ref<{ id: number; name: string }[]>([]);
const configForm = reactive<LoginConfigForm>({
  ban_threshold_days: 30,
  remind_before_days: 10,
  reminder_cron: "0 10,22 * * *",
  notification_channel_ids: [],
  probe_mode: "auto",
});

async function loadNotifyChannels() {
  if (notifyChannels.value.length > 0) return;
  try {
    const list = await chatopsApi.notifications.list();
    notifyChannels.value = list.map((c) => ({ id: c.id, name: c.name }));
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载通知通道失败");
  }
}

async function openConfigDialog(name: string) {
  configSiteName.value = name;
  const st = loginStates.value[name];
  configForm.ban_threshold_days = st?.ban_threshold_days ?? 30;
  configForm.remind_before_days = st?.remind_before_days ?? 10;
  configForm.reminder_cron = st?.reminder_cron || "0 10,22 * * *";
  configForm.notification_channel_ids = [...(st?.notification_channel_ids ?? [])];
  configForm.probe_mode = probeModeOf(name);
  configDialogVisible.value = true;
  await loadNotifyChannels();
}

async function saveLoginConfig() {
  const name = configSiteName.value;
  if (!name) return;
  configSaving.value = true;
  try {
    await sitesApi.updateLoginConfig(name, {
      ban_threshold_days: configForm.ban_threshold_days,
      remind_before_days: configForm.remind_before_days,
      reminder_cron: configForm.reminder_cron,
      notification_channel_ids: configForm.notification_channel_ids,
      probe_mode: configForm.probe_mode,
    });
    const st = loginStates.value[name];
    if (st) {
      st.ban_threshold_days = configForm.ban_threshold_days;
      st.remind_before_days = configForm.remind_before_days;
      st.reminder_cron = configForm.reminder_cron;
      st.notification_channel_ids = [...configForm.notification_channel_ids];
      st.probe_mode = configForm.probe_mode;
    }
    ElMessage.success("保号配置已更新");
    configDialogVisible.value = false;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "更新失败");
  } finally {
    configSaving.value = false;
  }
}
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h1 class="page-title">站点管理</h1>
        <p class="page-subtitle">管理您的 PT 站点连接与 RSS 订阅配置</p>
      </div>
      <div class="page-actions">
        <el-tooltip
          content="对所有已启用站点执行一次登录状态探测；使用现有单站点探测接口，最多 3 个并发"
          placement="bottom">
          <el-button
            :loading="bulkProbing"
            :disabled="loading || enabledCount === 0"
            data-testid="probe-all-enabled-button"
            @click="probeAllEnabled">
            一键探测已启用
          </el-button>
        </el-tooltip>
        <el-button
          :disabled="enabledCount === 0"
          data-testid="open-all-sites-btn"
          @click="openAllEnabled">
          一键打开已启用
        </el-button>
        <el-button
          type="primary"
          :icon="'Plus'"
          data-testid="add-site-button"
          @click="openAddDialog">
          新增站点
        </el-button>
      </div>
    </div>

    <el-alert
      type="warning"
      show-icon
      :closable="true"
      class="risk-hint-alert"
      title="活跃时间通过 cookie/API 探测获取，可刷新多数站点的 last_access（最近动向）以保号；但少数站点按 last_login（实际登录）或做种活跃度清理，此类站点仍需定期手动登录，请勿仅依赖此处数据。" />

    <div class="table-card" v-loading="loading">
      <div class="table-card-header">
        <div class="table-card-header-title">
          <el-icon class="mr-2"><Connection /></el-icon>
          站点列表
        </div>
        <el-radio-group
          v-model="viewMode"
          size="small"
          data-testid="site-view-toggle"
          class="view-toggle">
          <el-radio-button value="enabled">已启用 ({{ enabledCount }})</el-radio-button>
          <el-radio-button value="all">全部 ({{ allEntries.length }})</el-radio-button>
        </el-radio-group>
      </div>

      <el-table
        :data="visibleEntries"
        :row-key="(row: [string, SiteConfig]) => row[0]"
        style="width: 100%"
        class="pt-table site-table"
        :header-cell-style="{ background: 'var(--pt-bg-secondary)', fontWeight: 600 }">
        <template #empty>
          <el-empty
            :description="
              viewMode === 'enabled' ? '尚未启用任何站点，点击右上角「新增站点」开始' : '暂无站点'
            ">
            <el-button v-if="viewMode === 'enabled'" type="primary" @click="openAddDialog">
              新增站点
            </el-button>
          </el-empty>
        </template>
        <el-table-column type="index" label="#" width="60" align="center" />

        <el-table-column label="站点" min-width="120">
          <template #default="{ row }">
            <div class="table-cell-primary site-name-wrapper">
              <span class="site-name">{{ row[0] }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="状态" min-width="78" align="center">
          <template #default="{ row }">
            <el-tag
              :type="row[1].enabled ? 'success' : 'info'"
              size="small"
              effect="plain"
              class="status-tag"
              round>
              <span class="status-dot" :class="{ active: row[1].enabled }"></span>
              {{ row[1].enabled ? "已启用" : "未启用" }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="认证方式" min-width="90" align="center">
          <template #default="{ row }">
            <el-tag
              :type="
                row[1].auth_method === 'api_key'
                  ? 'warning'
                  : row[1].auth_method === 'cookie_and_api_key'
                    ? 'success'
                    : row[1].auth_method === 'passkey'
                      ? 'primary'
                      : 'info'
              "
              size="small"
              effect="light"
              class="status-tag status-tag--auth"
              round>
              {{
                row[1].auth_method === "api_key"
                  ? "API Key"
                  : row[1].auth_method === "cookie_and_api_key"
                    ? "Cookie + API"
                    : row[1].auth_method === "passkey"
                      ? "Passkey"
                      : "Cookie"
              }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="RSS 订阅" min-width="76" align="center">
          <template #default="{ row }">
            <div class="rss-cell">
              <el-badge
                :value="getRssCount(row[1])"
                :type="getRssCount(row[1]) > 0 ? 'primary' : 'info'"
                class="rss-badge">
                <div class="rss-icon-wrapper">
                  <el-icon :size="18"><RssPost /></el-icon>
                </div>
              </el-badge>
            </div>
          </template>
        </el-table-column>

        <el-table-column min-width="98" align="center">
          <template #header>
            <el-tooltip
              content="用于封禁提醒判定的有效活跃时间，优先使用站点返回的 last_access；不是网页登录时间"
              placement="top">
              <span>判定活跃</span>
            </el-tooltip>
          </template>
          <template #default="{ row }">
            <span :data-testid="`last-login-cell-${row[0]}`" class="login-time-cell">
              {{ formatTimeAgo(effectiveLastActive(row[0])) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column min-width="100" align="center">
          <template #header>
            <el-tooltip content="距离站点封禁阈值的剩余天数；负数表示已超过阈值" placement="top">
              <span>剩余天数</span>
            </el-tooltip>
          </template>
          <template #default="{ row }">
            <div class="days-remaining-cell">
              <span :data-testid="`days-remaining-cell-${row[0]}`" :class="daysCellClass(row[0])">
                {{ daysRemaining(row[0]) === null ? "—" : `${daysRemaining(row[0])} 天` }}
              </span>
              <el-tag
                size="small"
                :type="tierTagType(reminderTier(row[0]))"
                effect="plain"
                class="tier-tag">
                {{ tierLabel(reminderTier(row[0])) }}
              </el-tag>
            </div>
          </template>
        </el-table-column>

        <el-table-column min-width="98" align="center">
          <template #header>
            <el-tooltip
              content="站点/API 返回的原始 last_access 或 lastBrowse 时间"
              placement="top">
              <span>站点活跃</span>
            </el-tooltip>
          </template>
          <template #default="{ row }">
            <span :data-testid="`last-access-cell-${row[0]}`" class="login-time-cell">
              {{ formatTimeAgo(lastAccess(row[0])) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="探测模式" min-width="96" align="center">
          <template #default="{ row }">
            <el-select
              :model-value="probeModeOf(row[0])"
              size="small"
              :disabled="updatingMode[row[0]]"
              :data-testid="`probe-mode-select-${row[0]}`"
              class="probe-mode-select"
              @change="(value: 'auto' | 'manual' | 'disabled') => changeProbeMode(row[0], value)">
              <el-option label="自动" value="auto" />
              <el-option label="手动" value="manual" />
              <el-option label="禁用" value="disabled" />
            </el-select>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="400" align="center" fixed="right">
          <template #default="{ row }">
            <div class="table-cell-actions site-actions-nowrap">
              <el-tooltip
                :content="
                  row[1].unavailable ? row[1].unavailable_reason : row[1].enabled ? '禁用' : '启用'
                "
                placement="top">
                <el-switch
                  :model-value="row[1].enabled"
                  size="small"
                  :disabled="row[1].unavailable"
                  @change="toggleEnabled(row[0])"
                  style="--el-switch-on-color: var(--pt-color-success)" />
              </el-tooltip>
              <el-tooltip
                content="未配置站点地址"
                placement="top"
                :disabled="!!(sites[row[0]]?.urls?.[0] || loginState(row[0])?.base_url)">
                <span>
                  <el-button
                    type="info"
                    size="small"
                    text
                    bg
                    class="action-btn"
                    :icon="'TopRight'"
                    :disabled="!(sites[row[0]]?.urls?.[0] || loginState(row[0])?.base_url)"
                    :data-testid="`open-site-btn-${row[0]}`"
                    @click="openSite(row[0])">
                    打开站点
                  </el-button>
                </span>
              </el-tooltip>
              <el-button
                type="success"
                size="small"
                text
                bg
                class="action-btn action-btn--probe"
                :loading="probing[row[0]]"
                :disabled="!row[1].enabled || probing[row[0]]"
                :data-testid="`probe-button-${row[0]}`"
                @click="probeSite(row[0])">
                立即探测
              </el-button>
              <el-button
                type="warning"
                size="small"
                text
                bg
                class="action-btn action-btn--test-reminder"
                :loading="testingReminder[row[0]]"
                :disabled="!row[1].enabled || testingReminder[row[0]]"
                :data-testid="`test-reminder-btn-${row[0]}`"
                @click="sendTestReminder(row[0])">
                测试提醒
              </el-button>
              <el-button
                type="primary"
                size="small"
                text
                bg
                class="action-btn action-btn--config"
                @click="manageSite(row[0])">
                配置
              </el-button>
              <el-button
                type="primary"
                size="small"
                text
                bg
                class="action-btn action-btn--login-config"
                :data-testid="`login-config-btn-${row[0]}`"
                @click="openConfigDialog(row[0])">
                保号配置
              </el-button>
              <el-button
                type="danger"
                size="small"
                text
                bg
                class="action-btn action-btn--delete"
                :disabled="row[1].is_builtin"
                @click="deleteSite(row[0])">
                删除
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog
      v-model="addDialogVisible"
      title="添加站点"
      width="640px"
      data-testid="add-site-dialog"
      class="add-site-dialog"
      append-to-body>
      <el-input
        v-model="addSearch"
        placeholder="搜索：站点名称 / 域名"
        clearable
        :prefix-icon="'Search'"
        data-testid="add-site-search"
        class="add-site-search" />

      <el-scrollbar max-height="420px" class="add-site-scroll">
        <div v-if="addCandidates.length > 0" class="add-site-list">
          <div v-for="[name, site] in addCandidates" :key="name" class="add-site-item">
            <SiteAvatar :site-id="name" :site-name="name" :size="36" :no-fetch="true" />
            <div class="add-site-meta">
              <div class="add-site-name">{{ name }}</div>
              <div class="add-site-tags">
                <el-tag size="small" :type="authMethodTagType(site.auth_method)" effect="plain">
                  {{ authMethodLabel(site.auth_method) }}
                </el-tag>
                <el-tag v-if="site.unavailable" size="small" type="danger" effect="plain">
                  暂不可用
                </el-tag>
              </div>
            </div>
            <div class="add-site-actions">
              <el-tooltip
                :disabled="!site.unavailable"
                :content="site.unavailable_reason || '该站点暂不可用'"
                placement="top">
                <span>
                  <el-button
                    type="primary"
                    size="small"
                    :disabled="site.unavailable"
                    :loading="enablingInDialog[name]"
                    :data-testid="`enable-site-btn-${name}`"
                    @click="enableSiteFromDialog(name)">
                    启用
                  </el-button>
                </span>
              </el-tooltip>
              <el-button size="small" text bg @click="configureFromDialog(name)">配置</el-button>
            </div>
          </div>
        </div>
        <el-empty v-else :description="addSearch ? '未找到匹配的站点' : '所有支持的站点均已启用'" />
      </el-scrollbar>

      <template #footer>
        <div class="add-site-footer">
          <span class="add-site-hint">
            需要适配新站点？安装
            <a href="https://github.com/sunerpy/pt-tools/releases" target="_blank" rel="noopener">
              浏览器扩展
            </a>
            采集数据后按
            <a
              href="https://github.com/sunerpy/pt-tools/blob/main/docs/guide/request-new-site.md"
              target="_blank"
              rel="noopener">
              指南
            </a>
            提交 Issue
          </span>
          <el-button @click="addDialogVisible = false">关闭</el-button>
        </div>
      </template>
    </el-dialog>

    <el-dialog
      v-model="configDialogVisible"
      :title="`保号配置 - ${configSiteName}`"
      width="520px"
      data-testid="login-config-dialog"
      append-to-body>
      <el-form label-width="120px" label-position="right">
        <el-form-item label="封号判定天数">
          <el-input-number
            v-model="configForm.ban_threshold_days"
            :min="1"
            :max="365"
            data-testid="login-config-ban-threshold" />
          <span class="login-config-hint">不活跃超过此天数将被站点判定封号</span>
        </el-form-item>
        <el-form-item label="提前提醒天数">
          <el-input-number
            v-model="configForm.remind_before_days"
            :min="1"
            :max="365"
            data-testid="login-config-remind-before" />
          <span class="login-config-hint">距封号前多少天开始提醒</span>
        </el-form-item>
        <el-form-item>
          <template #label>
            <el-tooltip placement="top">
              <template #content>
                标准 5 字段 cron：分 时 日 月 周。<br />
                <code>0 10,22 * * *</code> = 每天 10:00 与 22:00 各提醒一次。<br />
                示例：<code>0 9 * * *</code> 每天 9 点；<code>0 */6 * * *</code> 每 6 小时；
                <code>30 8 * * 1</code> 每周一 8:30。
              </template>
              <span
                >提醒 cron <el-icon><QuestionFilled /></el-icon
              ></span>
            </el-tooltip>
          </template>
          <el-input
            v-model="configForm.reminder_cron"
            placeholder="0 10,22 * * *"
            data-testid="login-config-cron" />
          <span class="login-config-hint"
            >5 字段 cron（分 时 日 月 周），留空使用默认 0 10,22 * * *</span
          >
        </el-form-item>
        <el-form-item label="通知通道">
          <el-select
            v-model="configForm.notification_channel_ids"
            multiple
            clearable
            placeholder="留空 = 发送到所有已启用通知通道"
            style="width: 100%"
            data-testid="login-config-channels">
            <el-option v-for="ch in notifyChannels" :key="ch.id" :label="ch.name" :value="ch.id" />
          </el-select>
          <span class="login-config-hint">
            未选择 = 发送到所有已启用的通知通道；选择后仅发送到所选通道（且通道需处于启用状态）。
          </span>
        </el-form-item>
        <el-form-item label="探测模式">
          <el-select v-model="configForm.probe_mode" data-testid="login-config-probe-mode">
            <el-option label="自动" value="auto" />
            <el-option label="手动" value="manual" />
            <el-option label="禁用" value="disabled" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="configDialogVisible = false">取消</el-button>
        <el-button
          type="primary"
          :loading="configSaving"
          data-testid="login-config-save"
          @click="saveLoginConfig">
          保存
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";
@import "@/styles/table-page.css";

.risk-hint-alert {
  margin-bottom: var(--pt-space-3, 12px);
}

.risk-hint-alert :deep(.el-alert__title) {
  font-weight: 600;
  line-height: 1.6;
}

.login-config-hint {
  margin-left: var(--pt-space-2, 8px);
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.site-name-wrapper {
  display: flex;
  align-items: center;
}

.site-name {
  font-weight: 700;
  font-size: 15px;
  color: var(--pt-text-primary);
  text-transform: capitalize;
}

/* RSS Badge Fixes */
.rss-cell {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100%;
  padding: 4px 8px; /* Add horizontal padding to container */
  overflow: visible; /* Attempt to allow overflow */
}

.rss-icon-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
}

/* Ensure badge doesn't fly too far out */
.rss-badge :deep(.el-badge__content) {
  top: 0;
  right: 0;
  transform: translateY(-30%) translateX(30%); /* Reduce outward push */
  z-index: 10;
}

/* Force table cell to allow overflow for the badge */
:deep(.el-table__body-wrapper .el-table__cell .cell) {
  overflow: visible;
}

.mr-2 {
  margin-right: 8px;
}

.status-tag {
  font-weight: 700;
  letter-spacing: 0.5px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 12px;
  height: 24px;
}

.status-tag--auth {
  border-width: 1px;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: var(--pt-color-neutral-400);
  transition: background-color var(--pt-transition-normal);
}

.status-dot.active {
  background-color: var(--pt-color-success);
  box-shadow: 0 0 0 2px var(--pt-color-success-50);
}

:deep(.el-table__row) {
  transition:
    background-color var(--pt-transition-fast),
    box-shadow var(--pt-transition-fast);
}

:deep(.el-table__row:hover) {
  background-color: color-mix(in srgb, var(--pt-color-primary-50) 60%, var(--pt-bg-secondary));
}

.action-btn {
  min-width: 56px;
  border-radius: var(--pt-radius-md);
  font-weight: 600;
  border-width: 1px;
  border-style: solid;
  background: var(--pt-bg-surface);
  transition:
    transform var(--pt-transition-fast),
    box-shadow var(--pt-transition-fast);
}

.action-btn:hover {
  transform: translateY(-1px);
  box-shadow: var(--pt-shadow-sm);
}

.action-btn--probe {
  min-width: 84px;
}

.site-actions-nowrap {
  flex-wrap: nowrap;
  gap: 4px;
}

.site-actions-nowrap .action-btn {
  min-width: 0;
  padding-left: 7px;
  padding-right: 7px;
}

.site-actions-nowrap .action-btn--probe,
.site-actions-nowrap .action-btn--test-reminder {
  min-width: 0;
}

.action-btn--test-reminder {
  min-width: 84px;
}

.action-btn--config {
  color: var(--pt-color-primary);
  border-color: color-mix(in srgb, var(--pt-color-primary) 38%, var(--pt-border-color));
  background: color-mix(in srgb, var(--pt-color-primary-50) 82%, var(--pt-bg-surface));
}

.action-btn--delete {
  color: var(--pt-color-danger);
  border-color: color-mix(in srgb, var(--pt-color-danger) 36%, var(--pt-border-color));
  background: color-mix(in srgb, var(--pt-color-danger-50) 82%, var(--pt-bg-surface));
}

/* Dark mode adjustments */
html.dark .site-name {
  color: var(--pt-text-primary);
}

html.dark .status-dot.active {
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--pt-color-success) 30%, transparent);
}

html.dark .action-btn {
  color: var(--pt-text-primary);
  background: color-mix(in srgb, var(--pt-bg-tertiary) 90%, #000 10%);
  border-color: color-mix(in srgb, var(--pt-border-color) 82%, #fff 12%);
}

html.dark .action-btn--config {
  color: color-mix(in srgb, var(--pt-color-primary-100) 90%, #fff 10%);
  border-color: color-mix(in srgb, var(--pt-color-primary) 40%, var(--pt-border-color));
  background: color-mix(in srgb, var(--pt-color-primary) 24%, var(--pt-bg-tertiary));
}

html.dark .action-btn--delete {
  color: color-mix(in srgb, var(--pt-color-danger-100) 90%, #fff 10%);
  border-color: color-mix(in srgb, var(--pt-color-danger) 40%, var(--pt-border-color));
  background: color-mix(in srgb, var(--pt-color-danger) 22%, var(--pt-bg-tertiary));
}

html.dark :deep(.el-table__row:hover) {
  background-color: color-mix(in srgb, var(--pt-color-primary-900) 34%, var(--pt-bg-surface));
}

.view-toggle {
  flex-shrink: 0;
}

.add-site-search {
  margin-bottom: 12px;
}

.add-site-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.add-site-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--pt-border-color);
  border-radius: var(--pt-radius-md);
  background: var(--pt-bg-surface);
  transition:
    border-color var(--pt-transition-fast),
    box-shadow var(--pt-transition-fast);
}

.add-site-item:hover {
  border-color: color-mix(in srgb, var(--pt-color-primary) 40%, var(--pt-border-color));
  box-shadow: var(--pt-shadow-sm);
}

.add-site-meta {
  flex: 1;
  min-width: 0;
}

.add-site-name {
  font-weight: 600;
  font-size: 14px;
  color: var(--pt-text-primary);
  text-transform: capitalize;
}

.add-site-tags {
  display: flex;
  gap: 6px;
  margin-top: 4px;
}

.add-site-actions {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}

.add-site-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.add-site-hint {
  font-size: 12px;
  color: var(--pt-text-secondary);
  text-align: left;
  line-height: 1.5;
}

.add-site-hint a {
  color: var(--el-color-primary);
  text-decoration: none;
}

.add-site-hint a:hover {
  text-decoration: underline;
}

.login-time-cell {
  font-size: 12px;
  color: var(--pt-text-secondary);
  font-variant-numeric: tabular-nums;
}

.days-remaining-cell {
  display: inline-flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.days-remaining-value {
  font-size: 13px;
  font-weight: 600;
  color: var(--pt-text-primary);
  font-variant-numeric: tabular-nums;
}

.days-remaining--warn {
  color: var(--pt-color-warning);
}

.days-remaining--critical {
  color: var(--pt-color-danger);
  font-weight: 700;
}

.tier-tag {
  font-size: 11px;
  height: 20px;
  padding: 0 8px;
}

.probe-mode-select {
  width: 82px;
}
</style>
