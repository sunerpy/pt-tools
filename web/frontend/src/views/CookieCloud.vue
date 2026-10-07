<script setup lang="ts">
/*
 * CookieCloud 导入（路线图 M7）：从自建的 CookieCloud 服务取回浏览器同步上去的 Cookie，在本机解密，
 * 按站点地址挑出 pt-tools 站点能用的 Cookie，预览后写进站点（与浏览器扩展同步凭据走同一条路）。
 * 密码加密保存、只写不读；预览只给 Cookie 的名字，不给值。可选定时同步（只更新已启用、有变化的站点）。
 * 画板没有这一页，沿用站点组列表页的样式：页头 + 面板里的表单与表格，手机是行卡。
 */
import {
  type CookieCloudPreview,
  type CookieCloudPreviewItem,
  type CookieCloudSettings,
  cookieCloudApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import {
  cookieCloudImportSummary,
  cookieCloudImportTone,
  cookieCloudItemStatus,
} from "@/utils/cookiecloud";
import { formatShortDateTime } from "@/utils/format";
import { ElMessage, ElMessageBox, type TableInstance } from "element-plus";
import { computed, nextTick, onMounted, ref } from "vue";

const isMobile = useIsMobile();

// ---- 设置 ----
const settings = ref<CookieCloudSettings | null>(null);
const form = ref({ server_url: "", uuid: "", password: "", auto_sync: false, interval_hours: 24 });
const loadFailed = ref(false);
const saving = ref(false);
const configured = computed(() =>
  Boolean(settings.value?.server_url && settings.value?.uuid && settings.value?.has_password),
);

function fill(s: CookieCloudSettings) {
  settings.value = s;
  form.value = {
    server_url: s.server_url,
    uuid: s.uuid,
    password: "",
    auto_sync: s.auto_sync,
    interval_hours: s.interval_hours,
  };
}

async function loadSettings() {
  try {
    const s = await cookieCloudApi.settings();
    if (s) fill(s);
    loadFailed.value = false;
  } catch {
    loadFailed.value = true;
  }
}

async function save(clearPassword = false) {
  saving.value = true;
  try {
    const pw = form.value.password;
    const s = await cookieCloudApi.saveSettings({
      server_url: form.value.server_url.trim(),
      uuid: form.value.uuid.trim(),
      ...(clearPassword ? { password: "" } : pw ? { password: pw } : {}),
      auto_sync: form.value.auto_sync,
      interval_hours: form.value.interval_hours,
    });
    if (s) fill(s);
    // 设置变了，上次读到的（以及还在路上的那次读取）可能是别的账户的数据：作废，要重新读取才能导入
    previewEpoch++;
    preview.value = null;
    selected.value = [];
    ElMessage.success("已保存");
  } catch (e) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

async function clearPassword() {
  try {
    await ElMessageBox.confirm("清除保存的密码？定时同步会一并关闭。", "清除密码", {
      type: "warning",
      confirmButtonText: "清除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  form.value.auto_sync = false;
  await save(true);
}

// ---- 预览与导入 ----
const previewDS = useDataState();
const preview = ref<CookieCloudPreview | null>(null);
const items = computed(() => preview.value?.items ?? []);
const selected = ref<string[]>([]);
const importing = ref(false);
const tableRef = ref<TableInstance>();
/** 保存设置时加一：保存之前发出、之后才回来的读取结果不再用 */
let previewEpoch = 0;

async function loadPreview() {
  const epoch = previewEpoch;
  const pending = previewDS.run(() => cookieCloudApi.preview());
  const data = await pending;
  if (previewDS.isStale(pending) || epoch !== previewEpoch) return;
  if (data) {
    preview.value = data;
    selected.value = [];
  }
}

function toggle(site: string, on: boolean) {
  const rest = selected.value.filter((s) => s !== site);
  selected.value = on ? [...rest, site] : rest;
}

/** 选中已经启用、Cookie 有变化的站点（没启用的要用户自己决定要不要启用） */
async function selectChanged() {
  const want = items.value.filter((it) => it.enabled && it.changed);
  if (!isMobile.value && tableRef.value) {
    tableRef.value.clearSelection();
    await nextTick();
    for (const it of want) tableRef.value.toggleRowSelection(it, true);
  } else {
    selected.value = want.map((it) => it.site);
  }
}

async function importSelected() {
  const n = selected.value.length;
  if (!n) return;
  const enabling = items.value.filter((it) => selected.value.includes(it.site) && !it.enabled);
  try {
    await ElMessageBox.confirm(
      `用 CookieCloud 里的 Cookie 替换选中的 ${n} 个站点的 Cookie？` +
        (enabling.length ? `其中 ${enabling.length} 个还没启用，导入后会启用。` : ""),
      "导入 Cookie",
      { type: "warning", confirmButtonText: "导入", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  importing.value = true;
  try {
    const res = await cookieCloudApi.import(selected.value);
    if (res) {
      ElMessage({
        type: cookieCloudImportTone(res),
        message: cookieCloudImportSummary(res),
        duration: 5000,
      });
    }
    await loadPreview();
  } catch (e) {
    ElMessage.error((e as Error).message || "导入失败");
  } finally {
    importing.value = false;
  }
}

function onSelectionChange(rows: CookieCloudPreviewItem[]) {
  selected.value = rows.map((r) => r.site);
}

function refresh() {
  void loadSettings();
}

onMounted(loadSettings);
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub>从自建的 CookieCloud 服务取回浏览器同步的 Cookie，在本机解密后写进站点</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="cc-refresh" @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <div class="pt-note">
      <PtIcon name="info" :size="14" class="pt-note__icon" />
      <span
        >pt-tools 只从 CookieCloud 服务取回加密的数据，用密码在本机解密，密码不发给 CookieCloud
        服务；只导入 pt-tools 站点地址能用的 Cookie，其他网站的 Cookie 不保存。</span
      >
    </div>

    <PtPanel title="CookieCloud 设置" icon="cloud">
      <div v-if="loadFailed" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>设置没读到，点刷新重试。</span>
      </div>
      <el-form class="pt-form cc-form" label-position="top" @submit.prevent>
        <el-form-item label="服务地址">
          <el-input
            v-model="form.server_url"
            placeholder="https://cookiecloud.example.com"
            autocomplete="off"
            data-testid="cc-server" />
        </el-form-item>
        <el-form-item label="用户 KEY（UUID）">
          <el-input
            v-model="form.uuid"
            placeholder="浏览器扩展里的用户 KEY"
            autocomplete="off"
            data-testid="cc-uuid" />
        </el-form-item>
        <el-form-item label="端对端加密密码">
          <div class="cc-password">
            <el-input
              v-model="form.password"
              type="password"
              show-password
              autocomplete="new-password"
              :placeholder="
                settings?.has_password ? '已设置；要更换时填写新的' : '浏览器扩展里的密码'
              "
              data-testid="cc-password" />
            <el-button
              v-if="settings?.has_password"
              data-testid="cc-clear-password"
              @click="clearPassword"
              >清除</el-button
            >
          </div>
        </el-form-item>
        <div class="cc-row">
          <el-form-item label="定时同步">
            <el-switch v-model="form.auto_sync" data-testid="cc-auto-sync" />
          </el-form-item>
          <el-form-item label="同步间隔（小时）">
            <el-input-number
              v-model="form.interval_hours"
              :min="1"
              :max="168"
              controls-position="right"
              data-testid="cc-interval" />
          </el-form-item>
        </div>
        <el-button type="primary" :loading="saving" data-testid="cc-save" @click="save()">
          <PtIcon v-if="!saving" name="save" :size="14" /><span>保存</span>
        </el-button>
      </el-form>
      <p class="cc-tip">
        定时同步只更新已经启用、Cookie 有变化的站点，不会启用新站点。
        <template v-if="settings?.last_sync_at">
          上次同步 {{ formatShortDateTime(settings.last_sync_at) }}：{{ settings.last_result }}
        </template>
      </p>
    </PtPanel>

    <PtPanel
      title="导入站点 Cookie"
      icon="cloud-download"
      :count="preview ? items.length : undefined">
      <div class="cc-actions">
        <el-button
          :loading="previewDS.loading.value"
          :disabled="!configured"
          data-testid="cc-preview"
          @click="loadPreview">
          <PtIcon v-if="!previewDS.loading.value" name="cloud-download" :size="14" /><span
            >读取 CookieCloud</span
          >
        </el-button>
        <el-button v-if="items.length" data-testid="cc-select-changed" @click="selectChanged"
          >选中有变化的</el-button
        >
        <el-button
          type="primary"
          :loading="importing"
          :disabled="!selected.length"
          data-testid="cc-import"
          @click="importSelected">
          <PtIcon v-if="!importing" name="download" :size="14" /><span
            >导入选中{{ selected.length ? ` ${selected.length} 个` : "" }}</span
          >
        </el-button>
      </div>
      <div
        v-if="items.length && previewDS.error.value"
        class="pt-note pt-note--warn"
        data-testid="cc-stale">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>读取失败：{{ previewDS.errorText.value }}。下面是上次读到的。</span>
      </div>
      <p v-if="preview" class="cc-tip" data-testid="cc-summary">
        CookieCloud 里有 {{ preview.domains }} 个网站的 Cookie，其中 {{ items.length }} 个 pt-tools
        站点能用。只列 Cookie 的名字，不显示内容。
      </p>

      <PtDataState
        v-if="!items.length"
        :state="previewDS.state.value"
        :title="previewDS.error.value || preview ? '' : '还没读取'"
        :sub="
          previewDS.errorText.value ||
          (preview
            ? 'CookieCloud 里没有 pt-tools 站点能用的 Cookie'
            : configured
              ? '点「读取 CookieCloud」，看看哪些站点能导入'
              : '先在上面填写服务地址、用户 KEY 和密码并保存')
        " />
      <el-table
        v-else-if="!isMobile"
        ref="tableRef"
        :data="items"
        row-key="site"
        class="pt-grid"
        data-testid="cc-table"
        @selection-change="onSelectionChange">
        <el-table-column type="selection" width="44" />
        <el-table-column label="站点" min-width="200" class-name="pt-cell-strong">
          <template #default="{ row }">
            <div class="cc-name">{{ row.site_name }}</div>
            <div class="cc-sub">{{ row.host }}</div>
          </template>
        </el-table-column>
        <el-table-column
          label="Cookie"
          min-width="260"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">{{ row.cookie_names.join("、") }}</template>
        </el-table-column>
        <el-table-column label="状态" width="140">
          <template #default="{ row }">
            <PtStatusPill :tone="cookieCloudItemStatus(row).tone" size="sm">{{
              cookieCloudItemStatus(row).label
            }}</PtStatusPill>
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="cc-cards">
        <PtRowCard v-for="it in items" :key="it.site">
          <template #lead>
            <el-checkbox
              :model-value="selected.includes(it.site)"
              :aria-label="`选择 ${it.site_name}`"
              @update:model-value="(v: string | number | boolean) => toggle(it.site, Boolean(v))" />
          </template>
          <template #title>{{ it.site_name }}</template>
          <template #meta>
            <span>{{ it.host }}</span>
            <span class="cc-full">{{ it.cookie_names.join("、") }}</span>
          </template>
          <template #status>
            <PtStatusPill :tone="cookieCloudItemStatus(it).tone" size="sm">{{
              cookieCloudItemStatus(it).label
            }}</PtStatusPill>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>
  </div>
</template>

<style scoped>
.cc-form {
  max-width: 640px;
}

.cc-password {
  display: flex;
  gap: 8px;
  width: 100%;
}

.cc-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 0 16px;
}

.cc-tip {
  margin: 12px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.cc-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}

.cc-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cc-sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.cc-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.cc-full {
  flex-basis: 100%;
}
</style>
