<script setup lang="ts">
import { ApiError, type SiteConfig, type SiteLoginState, chatopsApi, sitesApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import SiteAvatar from "@/components/SiteAvatar.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";

import { formatTimeAgo } from "@/utils/format";
import { isProbeSuccess, probeStatusLabel, probeStatusSeverity } from "@/utils/probeStatus";
import { useLoginState } from "@/composables/useLoginState";
import type { ReminderTier } from "@/composables/useLoginState";

const router = useRouter();

const loading = ref(false);
const sites = ref<Record<string, SiteConfig>>({});
const loginStates = ref<Record<string, SiteLoginState>>({});
const probing = reactive<Record<string, boolean>>({});
const testingReminder = reactive<Record<string, boolean>>({});
const bulkProbing = ref(false);
const updatingMode = reactive<Record<string, boolean>>({});

/* 风险提示只在本次会话里可关：它讲的是探测机制的固有局限，不是一条会过期的通知 */
const riskHintOpen = ref(true);

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
    // 注意：后端 res.ok 恒为 true（仅表示探测已执行、未发生锁冲突），
    // 判定探测是否成功必须以 last_probe_status === "OK" 为准。
    const status = res.last_probe_status;
    const label = probeStatusLabel(status);
    // 后端 last_probe_error 为人类可读的失败原因（如 Cookie 未失效提示）；
    // 仅在存在非空原因、且原因未重复状态码时展示，避免冗余。
    const reason = res.last_probe_error?.trim();
    if (isProbeSuccess(status)) {
      ElMessage.success(`探测完成：${label}`);
    } else {
      const severity = probeStatusSeverity(status);
      if (severity === "info") {
        // NOT_APPLICABLE 等：探测已完成但不适用，若有原因一并展示。
        ElMessage.info(reason ? `探测完成：${label} — ${reason}` : `探测完成：${label}`);
      } else {
        // warning/error：探测未通过，原因可能较长，改用可关闭、长驻留的提示。
        const message = reason ? `探测未通过：${label} — ${reason}` : `探测未通过：${label}`;
        ElMessage({
          type: severity,
          message,
          showClose: true,
          duration: severity === "error" ? 0 : 6000,
        });
      }
    }
    // 无论成功与否都刷新表格，使状态列反映最新的 last_probe_status / last_probe_at。
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
        const res = await sitesApi.probeNow(name);
        // res.ok 恒为 true，仅以 last_probe_status === "OK" 判定真正成功；
        // 其余状态（如 PARSE_ERROR，HTTP 200 但探测失败）计为失败。
        if (isProbeSuccess(res.last_probe_status)) success++;
        else failed++;
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

/* 与 openSite 用同一套取址顺序，否则会出现「按钮可点但打不开」 */
function siteUrlOf(name: string): string | undefined {
  return sites.value[name]?.web_url ?? sites.value[name]?.urls?.[0] ?? loginState(name)?.base_url;
}

const allEntries = computed(() => Object.entries(sites.value));

const enabledCount = computed(() => allEntries.value.filter(([, s]) => s.enabled).length);

const visibleEntries = computed(() => {
  if (viewMode.value === "all") return allEntries.value;
  return allEntries.value.filter(([, s]) => s.enabled);
});

const disabledEntries = computed(() => allEntries.value.filter(([, s]) => !s.enabled));

/* 分段控件上带计数：切过去之前就知道「全部」比「已启用」多出多少 */
const viewOptions = computed(() => [
  { label: `已启用 ${enabledCount.value}`, value: "enabled" },
  { label: `全部 ${allEntries.value.length}`, value: "all" },
]);

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

/* 保号档位是状态而不是分类，所以走 PtStatusPill；这里把 el-tag 的类型名折到胶囊的语气上 */
function tierTone(tier: ReminderTier): "ok" | "warn" | "dang" | "info" | "neutral" {
  switch (tierTagType(tier)) {
    case "danger":
      return "dang";
    case "warning":
      return "warn";
    case "primary":
      return "info";
    case "info":
      return "neutral";
    default:
      return "ok";
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
  <div class="sites-page">
    <div v-if="riskHintOpen" class="pt-note pt-note--warn risk-note">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>
        活跃时间来自 cookie/API 探测，能刷新多数站点的 last_access（最近动向）用于保号；
        但少数站点按
        last_login（实际登录）或做种活跃度清理，这类站点仍需定期手动登录，别只看这里的数字。
      </span>
      <button
        type="button"
        class="risk-note__x"
        aria-label="关闭提示"
        @click="riskHintOpen = false">
        <PtIcon name="x" :size="14" />
      </button>
    </div>

    <PtToolbar standalone>
      <el-segmented
        v-model="viewMode"
        class="pt-seg"
        :options="viewOptions"
        data-testid="site-view-toggle" />
      <el-button size="small" :loading="loading" @click="loadSites">
        <PtIcon name="refresh-cw" :size="14" /><span>刷新</span>
      </el-button>

      <template #right>
        <el-tooltip
          content="对所有已启用站点执行一次登录状态探测，最多 3 个并发"
          placement="bottom">
          <el-button
            size="small"
            :loading="bulkProbing"
            :disabled="loading || enabledCount === 0"
            data-testid="probe-all-enabled-button"
            @click="probeAllEnabled">
            <PtIcon name="activity" :size="14" /><span>探测已启用</span>
          </el-button>
        </el-tooltip>
        <el-button
          size="small"
          :disabled="enabledCount === 0"
          data-testid="open-all-sites-btn"
          @click="openAllEnabled">
          <PtIcon name="external-link" :size="14" /><span>打开已启用</span>
        </el-button>
        <el-button type="primary" size="small" data-testid="add-site-button" @click="openAddDialog">
          <PtIcon name="plus" :size="14" /><span>新增站点</span>
        </el-button>
      </template>
    </PtToolbar>

    <PtPanel
      v-loading="loading"
      title="站点列表"
      icon="globe"
      :count="`${visibleEntries.length} 个`"
      padding="none">
      <el-table
        :data="visibleEntries"
        :row-key="(row: [string, SiteConfig]) => row[0]"
        class="pt-grid"
        style="width: 100%">
        <template #empty>
          <PtDataState
            :state="viewMode === 'enabled' ? 'empty' : 'zero'"
            dense
            :title="viewMode === 'enabled' ? '还没有启用任何站点' : '没有可显示的站点'"
            :sub="
              viewMode === 'enabled'
                ? '从「新增站点」里挑一个开始，启用后才会参与 RSS 与统计'
                : '站点清单来自内置定义，装上浏览器扩展可以帮助适配新站'
            ">
            <template v-if="viewMode === 'enabled'" #action>
              <el-button type="primary" size="small" @click="openAddDialog">
                <PtIcon name="plus" :size="14" /><span>新增站点</span>
              </el-button>
            </template>
          </PtDataState>
        </template>

        <el-table-column label="站点" min-width="170" class-name="pt-cell-strong">
          <template #default="{ row }">
            <span class="site">
              <SiteAvatar :site-id="row[0]" :site-name="row[0]" :size="22" :no-fetch="true" />
              <span class="site__name">{{ row[0] }}</span>
              <PtStatusPill v-if="row[1].unavailable" tone="dang" size="sm">暂不可用</PtStatusPill>
            </span>
          </template>
        </el-table-column>

        <el-table-column label="认证" width="118">
          <template #default="{ row }">
            <PtTag>{{ authMethodLabel(row[1].auth_method) }}</PtTag>
          </template>
        </el-table-column>

        <el-table-column
          label="RSS"
          width="78"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">
            <span class="rss" :class="{ 'is-zero': getRssCount(row[1]) === 0 }">
              <PtIcon name="rss" :size="13" />
              {{ getRssCount(row[1]) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column min-width="104" class-name="pt-cell-muted">
          <template #header>
            <el-tooltip
              content="用于封禁提醒判定的有效活跃时间，优先使用站点返回的 last_access；不是网页登录时间"
              placement="top">
              <span class="th-help">判定活跃 <PtIcon name="info" :size="12" /></span>
            </el-tooltip>
          </template>
          <template #default="{ row }">
            <span :data-testid="`last-login-cell-${row[0]}`" class="ts">
              {{ formatTimeAgo(effectiveLastActive(row[0])) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column min-width="118">
          <template #header>
            <el-tooltip content="距离站点封禁阈值的剩余天数；负数表示已超过阈值" placement="top">
              <span class="th-help">剩余天数 <PtIcon name="info" :size="12" /></span>
            </el-tooltip>
          </template>
          <template #default="{ row }">
            <span class="days">
              <span :data-testid="`days-remaining-cell-${row[0]}`" :class="daysCellClass(row[0])">
                {{ daysRemaining(row[0]) === null ? "—" : `${daysRemaining(row[0])} 天` }}
              </span>
              <PtStatusPill :tone="tierTone(reminderTier(row[0]))" size="sm">
                {{ tierLabel(reminderTier(row[0])) }}
              </PtStatusPill>
            </span>
          </template>
        </el-table-column>

        <el-table-column min-width="104" class-name="pt-cell-muted">
          <template #header>
            <el-tooltip
              content="站点/API 返回的原始 last_access 或 lastBrowse 时间"
              placement="top">
              <span class="th-help">站点活跃 <PtIcon name="info" :size="12" /></span>
            </el-tooltip>
          </template>
          <template #default="{ row }">
            <span :data-testid="`last-access-cell-${row[0]}`" class="ts">
              {{ formatTimeAgo(lastAccess(row[0])) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column label="探测" width="104">
          <template #default="{ row }">
            <el-select
              :model-value="probeModeOf(row[0])"
              size="small"
              :disabled="updatingMode[row[0]]"
              :data-testid="`probe-mode-select-${row[0]}`"
              class="mode-sel"
              @change="(value: 'auto' | 'manual' | 'disabled') => changeProbeMode(row[0], value)">
              <el-option label="自动" value="auto" />
              <el-option label="手动" value="manual" />
              <el-option label="禁用" value="disabled" />
            </el-select>
          </template>
        </el-table-column>

        <el-table-column label="启用" width="72" align="center">
          <template #default="{ row }">
            <el-tooltip
              :content="
                row[1].unavailable
                  ? row[1].unavailable_reason || '该站点暂不可用'
                  : row[1].enabled
                    ? '点一下停用'
                    : '点一下启用'
              "
              placement="top">
              <span class="sw">
                <el-switch
                  :model-value="row[1].enabled"
                  size="small"
                  :disabled="row[1].unavailable"
                  @change="toggleEnabled(row[0])" />
              </span>
            </el-tooltip>
          </template>
        </el-table-column>

        <!--
          一行有六个动作，写上文字就要 400 宽，把前面几列挤成两行。
          这里只留图标 + tooltip，图标顺序按使用频率排：先看站点、再探测，删除放最后。
        -->
        <el-table-column label="操作" width="212" fixed="right" class-name="pt-cell-act">
          <template #default="{ row }">
            <el-tooltip
              :content="siteUrlOf(row[0]) ? '打开站点' : '未配置站点地址'"
              placement="top">
              <span>
                <el-button
                  link
                  type="primary"
                  size="small"
                  aria-label="打开站点"
                  :disabled="!siteUrlOf(row[0])"
                  :data-testid="`open-site-btn-${row[0]}`"
                  @click="openSite(row[0])">
                  <PtIcon name="external-link" :size="15" />
                </el-button>
              </span>
            </el-tooltip>

            <el-tooltip content="立即探测登录状态" placement="top">
              <span>
                <el-button
                  link
                  type="primary"
                  size="small"
                  aria-label="立即探测"
                  :loading="probing[row[0]]"
                  :disabled="!row[1].enabled || probing[row[0]]"
                  :data-testid="`probe-button-${row[0]}`"
                  @click="probeSite(row[0])">
                  <PtIcon name="activity" :size="15" />
                </el-button>
              </span>
            </el-tooltip>

            <el-tooltip content="发一条测试提醒到通知通道" placement="top">
              <span>
                <el-button
                  link
                  type="primary"
                  size="small"
                  aria-label="测试提醒"
                  :loading="testingReminder[row[0]]"
                  :disabled="!row[1].enabled || testingReminder[row[0]]"
                  :data-testid="`test-reminder-btn-${row[0]}`"
                  @click="sendTestReminder(row[0])">
                  <PtIcon name="bell-ring" :size="15" />
                </el-button>
              </span>
            </el-tooltip>

            <el-tooltip content="站点配置与 RSS 订阅" placement="top">
              <el-button
                link
                type="primary"
                size="small"
                aria-label="站点配置"
                @click="manageSite(row[0])">
                <PtIcon name="sliders-horizontal" :size="15" />
              </el-button>
            </el-tooltip>

            <el-tooltip content="保号配置：封号阈值、提醒时间、通知通道" placement="top">
              <el-button
                link
                type="primary"
                size="small"
                aria-label="保号配置"
                :data-testid="`login-config-btn-${row[0]}`"
                @click="openConfigDialog(row[0])">
                <PtIcon name="shield" :size="15" />
              </el-button>
            </el-tooltip>

            <el-tooltip
              :content="row[1].is_builtin ? '预置站点不可删除' : '删除站点'"
              placement="top">
              <span>
                <el-button
                  link
                  type="danger"
                  size="small"
                  aria-label="删除站点"
                  :disabled="row[1].is_builtin"
                  @click="deleteSite(row[0])">
                  <PtIcon name="trash-2" :size="15" />
                </el-button>
              </span>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>

      <template v-if="visibleEntries.length > 0" #footer>
        <span class="pt-foot-note">
          「判定活跃」是保号判据，「站点活跃」是站点原始返回值，两者不一致时以前者为准
        </span>
      </template>
    </PtPanel>

    <el-dialog
      v-model="addDialogVisible"
      class="pt-dialog"
      title="添加站点"
      width="640px"
      align-center
      data-testid="add-site-dialog"
      append-to-body>
      <el-input
        v-model="addSearch"
        placeholder="搜索：站点名称 / 域名"
        clearable
        data-testid="add-site-search"
        class="cand-search">
        <template #prefix>
          <PtIcon name="search" :size="14" />
        </template>
      </el-input>

      <el-scrollbar max-height="420px">
        <div v-if="addCandidates.length > 0" class="cands">
          <div v-for="[name, site] in addCandidates" :key="name" class="cand">
            <SiteAvatar :site-id="name" :site-name="name" :size="34" :no-fetch="true" />
            <span class="cand__meta">
              <span class="cand__name">{{ name }}</span>
              <span class="cand__tags">
                <PtTag>{{ authMethodLabel(site.auth_method) }}</PtTag>
                <PtStatusPill v-if="site.unavailable" tone="dang" size="sm">暂不可用</PtStatusPill>
              </span>
            </span>
            <span class="cand__acts">
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
              <el-button size="small" @click="configureFromDialog(name)">配置</el-button>
            </span>
          </div>
        </div>
        <PtDataState
          v-else
          :state="addSearch ? 'zero' : 'empty'"
          :title="addSearch ? '没有匹配的站点' : '支持的站点都已启用'"
          :sub="addSearch ? '换个名字或域名再搜' : '需要新站点的话，往下看提交入口'" />
      </el-scrollbar>

      <template #footer>
        <span class="pt-foot-note cand-hint">
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
      </template>
    </el-dialog>

    <el-dialog
      v-model="configDialogVisible"
      class="pt-dialog"
      :title="`保号配置 · ${configSiteName}`"
      width="520px"
      align-center
      data-testid="login-config-dialog"
      append-to-body>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <div class="field-row">
          <el-form-item label="封号判定天数">
            <el-input-number
              v-model="configForm.ban_threshold_days"
              :min="1"
              :max="365"
              controls-position="right"
              style="width: 100%"
              data-testid="login-config-ban-threshold" />
            <div class="field-tip">不活跃超过这个天数，站点就可能判定封号</div>
          </el-form-item>

          <el-form-item label="提前提醒天数">
            <el-input-number
              v-model="configForm.remind_before_days"
              :min="1"
              :max="365"
              controls-position="right"
              style="width: 100%"
              data-testid="login-config-remind-before" />
            <div class="field-tip">距封号还剩多少天开始提醒</div>
          </el-form-item>
        </div>

        <el-form-item>
          <template #label>
            <el-tooltip placement="top">
              <template #content>
                标准 5 字段 cron：分 时 日 月 周。<br />
                <code>0 10,22 * * *</code> = 每天 10:00 与 22:00 各提醒一次。<br />
                示例：<code>0 9 * * *</code> 每天 9 点；<code>0 */6 * * *</code> 每 6 小时；
                <code>30 8 * * 1</code> 每周一 8:30。
              </template>
              <span class="th-help">提醒 cron <PtIcon name="info" :size="12" /></span>
            </el-tooltip>
          </template>
          <el-input
            v-model="configForm.reminder_cron"
            placeholder="0 10,22 * * *"
            data-testid="login-config-cron" />
          <div class="field-tip">5 字段（分 时 日 月 周），留空按默认 0 10,22 * * * 走</div>
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
          <div class="field-tip">
            选了就只发选中的通道，而且通道本身也得是启用状态；留空则发给所有已启用通道
          </div>
        </el-form-item>

        <el-form-item label="探测模式">
          <el-select
            v-model="configForm.probe_mode"
            style="width: 100%"
            data-testid="login-config-probe-mode">
            <el-option label="自动" value="auto" />
            <el-option label="手动" value="manual" />
            <el-option label="禁用" value="disabled" />
          </el-select>
          <div class="field-tip">自动 = 跟随定时任务；手动 = 只在点「立即探测」时执行</div>
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
.sites-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
}

.risk-note {
  align-items: flex-start;
}

.risk-note__x {
  flex-shrink: 0;
  padding: 0;
  color: var(--pt-t4);
  cursor: pointer;
  background: none;
  border: 0;
}

.risk-note__x:hover {
  color: var(--pt-t2);
}

.site {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
  min-width: 0;
}

/* 站点名在定义文件里是小写 id，首字母大写才像个名字 */
.site__name {
  overflow: hidden;
  text-overflow: ellipsis;
  text-transform: capitalize;
  white-space: nowrap;
}

.rss {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  color: var(--pt-t1);
}

.rss.is-zero {
  color: var(--pt-t4);
}

/* 带 tooltip 的表头：虚线下划线提示「这里有解释」，比只放个图标更好点中 */
.th-help {
  display: inline-flex;
  gap: 3px;
  align-items: center;
  cursor: help;
  border-bottom: 1px dotted var(--pt-t4);
}

.ts {
  font-variant-numeric: tabular-nums;
}

.days {
  display: inline-flex;
  flex-direction: column;
  gap: 3px;
  align-items: flex-start;
}

.days-remaining-value {
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-t1);
  font-variant-numeric: tabular-nums;
}

.days-remaining--warn {
  color: var(--pt-warn);
}

.days-remaining--critical {
  font-weight: 700;
  color: var(--pt-dang);
}

.mode-sel {
  width: 88px;
}

/* el-tooltip 要一个能接事件的宿主，disabled 的开关和按钮自己不派发 mouseenter */
.sw {
  display: inline-flex;
}

.cand-search {
  margin-bottom: var(--pt-space-3);
}

.cands {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
}

.cand {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  padding: var(--pt-space-2) var(--pt-space-3);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
  transition: border-color var(--pt-transition-fast);
}

.cand:hover {
  border-color: var(--pt-p);
}

.cand__meta {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.cand__name {
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t1);
  text-transform: capitalize;
}

.cand__tags {
  display: flex;
  gap: 6px;
  align-items: center;
}

.cand__acts {
  display: flex;
  flex-shrink: 0;
  gap: var(--pt-space-2);
}

/* 底部提示要占满左侧，按钮靠右：pt-foot-note 自带 margin-right:auto */
.cand-hint {
  text-align: left;
}

.cand-hint a {
  color: var(--pt-p);
  text-decoration: none;
}

.cand-hint a:hover {
  text-decoration: underline;
}
</style>
