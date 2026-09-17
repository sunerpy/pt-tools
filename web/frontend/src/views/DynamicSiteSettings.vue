<script setup lang="ts">
import {
  downloadersApi,
  type DownloaderSetting,
  dynamicSitesApi,
  type DynamicSiteSetting,
  type SiteTemplate,
  type SiteValidationResponse,
  templatesApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref } from "vue";

const loading = ref(false);
const saving = ref(false);
const validating = ref(false);

// 数据
const dynamicSites = ref<DynamicSiteSetting[]>([]);
const templates = ref<SiteTemplate[]>([]);
const downloaders = ref<DownloaderSetting[]>([]);

// 对话框状态
const showAddDialog = ref(false);
const showImportDialog = ref(false);
const showValidationResult = ref(false);

// 表单
const addForm = ref({
  name: "",
  display_name: "",
  base_url: "",
  auth_method: "cookie",
  cookie: "",
  api_key: "",
  api_url: "",
  passkey: "",
  downloader_id: undefined as number | undefined,
});

const importForm = ref({
  templateJson: "",
  cookie: "",
  api_key: "",
});

const validationResult = ref<SiteValidationResponse | null>(null);

const authMethods = [
  { value: "cookie", label: "Cookie 认证" },
  { value: "api_key", label: "API Key 认证" },
  { value: "cookie_and_api_key", label: "Cookie + API Key 认证" },
  { value: "passkey", label: "Passkey 认证" },
];

const enabledDownloaders = computed(() => {
  return downloaders.value; // 返回所有下载器，不过滤
});

onMounted(async () => {
  await loadData();
});

async function loadData() {
  loading.value = true;
  try {
    const [sitesData, templatesData, downloadersData] = await Promise.all([
      dynamicSitesApi.list(),
      templatesApi.list(),
      downloadersApi.list(),
    ]);
    dynamicSites.value = sitesData;
    templates.value = templatesData;
    downloaders.value = downloadersData;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
  }
}

function openAddDialog() {
  addForm.value = {
    name: "",
    display_name: "",
    base_url: "",
    auth_method: "cookie",
    cookie: "",
    api_key: "",
    api_url: "",
    passkey: "",
    downloader_id: undefined,
  };
  validationResult.value = null;
  showValidationResult.value = false;
  showAddDialog.value = true;
}

function openImportDialog() {
  importForm.value = {
    templateJson: "",
    cookie: "",
    api_key: "",
  };
  showImportDialog.value = true;
}

async function validateSite() {
  if (!addForm.value.name) {
    ElMessage.error("站点名称不能为空");
    return;
  }

  validating.value = true;
  try {
    validationResult.value = await dynamicSitesApi.validate({
      name: addForm.value.name,
      base_url: addForm.value.base_url,
      auth_method: addForm.value.auth_method,
      cookie: addForm.value.cookie,
      api_key: addForm.value.api_key,
      api_url: addForm.value.api_url,
      passkey: addForm.value.passkey,
    });
    showValidationResult.value = true;

    if (validationResult.value.valid) {
      ElMessage.success("验证成功");
    } else {
      ElMessage.warning(validationResult.value.message);
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "验证失败");
  } finally {
    validating.value = false;
  }
}

async function createSite() {
  if (!addForm.value.name) {
    ElMessage.error("站点名称不能为空");
    return;
  }

  saving.value = true;
  try {
    await dynamicSitesApi.create({
      name: addForm.value.name,
      display_name: addForm.value.display_name || addForm.value.name,
      base_url: addForm.value.base_url,
      auth_method: addForm.value.auth_method,
      cookie: addForm.value.cookie,
      api_key: addForm.value.api_key,
      api_url: addForm.value.api_url,
      passkey: addForm.value.passkey,
      downloader_id: addForm.value.downloader_id,
      enabled: true,
    });
    ElMessage.success("创建成功");
    showAddDialog.value = false;
    await loadData();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "创建失败");
  } finally {
    saving.value = false;
  }
}

async function importTemplate() {
  if (!importForm.value.templateJson) {
    ElMessage.error("请输入模板JSON");
    return;
  }

  let templateData: unknown;
  try {
    templateData = JSON.parse(importForm.value.templateJson);
  } catch {
    ElMessage.error("无效的JSON格式");
    return;
  }

  saving.value = true;
  try {
    await templatesApi.import({
      template: templateData,
      cookie: importForm.value.cookie,
      api_key: importForm.value.api_key,
    });
    ElMessage.success("导入成功");
    showImportDialog.value = false;
    await loadData();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "导入失败");
  } finally {
    saving.value = false;
  }
}

async function exportTemplate(tpl: SiteTemplate) {
  try {
    const data = await templatesApi.export(tpl.id);
    const json = JSON.stringify(data, null, 2);

    // 复制到剪贴板
    await navigator.clipboard.writeText(json);
    ElMessage.success("模板已复制到剪贴板");

    // 也可以下载
    const blob = new Blob([json], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${tpl.name}-template.json`;
    a.click();
    URL.revokeObjectURL(url);
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "导出失败");
  }
}

function getDownloaderName(id?: number) {
  if (!id) return "默认";
  const dl = downloaders.value.find((d) => d.id === id);
  return dl?.name || "未知";
}

function getAuthMethodLabel(method: string) {
  return authMethods.find((m) => m.value === method)?.label || method;
}

/** 与「支持的站点」页一致：cookie 是基线走 primary，组合认证配置成本最高走 warn */
function authMethodTone(method: string): "primary" | "ok" | "warn" | "info" {
  switch (method) {
    case "cookie":
      return "primary";
    case "api_key":
      return "ok";
    case "cookie_and_api_key":
      return "warn";
    default:
      return "info";
  }
}
</script>

<template>
  <div class="dynamic-sites-page">
    <div class="pt-note">
      <PtIcon name="info" :size="14" class="pt-note__icon" />
      <span>
        动态站点不需要改代码：填一份认证信息即可接入，也可以直接导入别人导出的模板 JSON。
        内置站点由程序自带的定义驱动，这里只展示、不可删。
      </span>
    </div>

    <PtPanel
      v-loading="loading"
      title="动态站点"
      icon="globe"
      :count="`${dynamicSites.length} 个`"
      padding="none">
      <template #actions>
        <el-button size="small" :loading="loading" @click="loadData">
          <PtIcon name="refresh-cw" :size="14" /><span>刷新</span>
        </el-button>
        <el-button type="primary" size="small" @click="openAddDialog">
          <PtIcon name="plus" :size="14" /><span>添加站点</span>
        </el-button>
      </template>

      <el-table :data="dynamicSites" class="pt-grid" style="width: 100%">
        <template #empty>
          <PtDataState state="empty" dense sub="添加一个动态站点，或从模板导入">
            <template #action>
              <el-button size="small" type="primary" @click="openAddDialog">
                <PtIcon name="plus" :size="14" /><span>添加站点</span>
              </el-button>
            </template>
          </PtDataState>
        </template>

        <el-table-column label="站点" min-width="180" class-name="pt-cell-strong">
          <template #default="{ row }">
            <span class="ident">
              <span class="ident__name">{{ row.display_name || row.name }}</span>
              <code class="ident__id">{{ row.name }}</code>
            </span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <PtStatusPill :tone="row.enabled ? 'ok' : 'neutral'" size="sm">
              {{ row.enabled ? "已启用" : "未启用" }}
            </PtStatusPill>
          </template>
        </el-table-column>

        <el-table-column label="认证方式" width="150">
          <template #default="{ row }">
            <PtStatusPill :tone="authMethodTone(row.auth_method)" size="sm">
              {{ getAuthMethodLabel(row.auth_method) }}
            </PtStatusPill>
          </template>
        </el-table-column>

        <el-table-column label="下载器" width="130" class-name="pt-cell-muted">
          <template #default="{ row }">{{ getDownloaderName(row.downloader_id) }}</template>
        </el-table-column>

        <el-table-column label="来源" width="90">
          <template #default="{ row }">
            <PtTag>{{ row.is_builtin ? "内置" : "动态" }}</PtTag>
          </template>
        </el-table-column>
      </el-table>
    </PtPanel>

    <PtPanel title="站点模板" icon="file-text" :count="`${templates.length} 个`" padding="none">
      <template #actions>
        <el-button size="small" @click="openImportDialog">
          <PtIcon name="upload" :size="14" /><span>导入模板</span>
        </el-button>
      </template>

      <el-table :data="templates" class="pt-grid" style="width: 100%">
        <template #empty>
          <PtDataState state="empty" dense sub="模板是一份可分享的站点定义，导入后即可直接使用" />
        </template>

        <el-table-column label="模板" min-width="180" class-name="pt-cell-strong">
          <template #default="{ row }">
            <span class="ident">
              <span class="ident__name">{{ row.display_name || row.name }}</span>
              <code class="ident__id">{{ row.name }}</code>
            </span>
          </template>
        </el-table-column>

        <el-table-column label="认证方式" width="150">
          <template #default="{ row }">
            <PtStatusPill :tone="authMethodTone(row.auth_method)" size="sm">
              {{ getAuthMethodLabel(row.auth_method) }}
            </PtStatusPill>
          </template>
        </el-table-column>

        <el-table-column label="描述" min-width="200" class-name="pt-cell-muted">
          <template #default="{ row }">{{ row.description || "-" }}</template>
        </el-table-column>

        <el-table-column
          label="版本"
          width="80"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">{{ row.version || "-" }}</template>
        </el-table-column>

        <el-table-column label="操作" width="100" fixed="right" class-name="pt-cell-act">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="exportTemplate(row)">
              <PtIcon name="download" :size="14" /><span>导出</span>
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <template v-if="templates.length > 0" #footer>
        <span class="pt-foot-note">导出会把 JSON 复制到剪贴板，同时下载一份文件</span>
      </template>
    </PtPanel>

    <el-dialog
      v-model="showAddDialog"
      class="pt-dialog"
      title="添加动态站点"
      width="580px"
      align-center>
      <el-form :model="addForm" class="pt-form" label-position="top">
        <div class="field-row">
          <el-form-item label="站点标识" required>
            <el-input v-model="addForm.name" placeholder="例如 mysite" />
            <div class="field-tip">唯一标识，用于内部引用，创建后不建议再改</div>
          </el-form-item>

          <el-form-item label="显示名称">
            <el-input v-model="addForm.display_name" placeholder="例如 我的站点" />
            <div class="field-tip">留空则沿用站点标识</div>
          </el-form-item>
        </div>

        <el-form-item label="站点地址">
          <el-input v-model="addForm.base_url" placeholder="https://example.com" />
        </el-form-item>

        <div class="field-rule"></div>

        <el-form-item label="认证方式" required>
          <el-select v-model="addForm.auth_method" style="width: 100%">
            <el-option v-for="m in authMethods" :key="m.value" :label="m.label" :value="m.value" />
          </el-select>
        </el-form-item>

        <el-form-item v-if="addForm.auth_method === 'cookie'" label="Cookie" required>
          <el-input
            v-model="addForm.cookie"
            type="textarea"
            :rows="3"
            placeholder="请输入站点 Cookie" />
        </el-form-item>

        <template v-if="addForm.auth_method === 'api_key'">
          <el-form-item label="API Key" required>
            <el-input v-model="addForm.api_key" placeholder="请输入 API Key" />
          </el-form-item>
          <el-form-item label="API 地址">
            <el-input v-model="addForm.api_url" placeholder="https://api.example.com" />
          </el-form-item>
        </template>

        <template v-if="addForm.auth_method === 'cookie_and_api_key'">
          <el-form-item label="Cookie" required>
            <el-input
              v-model="addForm.cookie"
              type="textarea"
              :rows="3"
              placeholder="请输入站点 Cookie" />
            <div class="field-tip">用于抓取时魔、魔力等只在网页上出现的信息</div>
          </el-form-item>
          <el-form-item label="API Key / RSS Key" required>
            <el-input v-model="addForm.api_key" placeholder="请输入 API Key 或 RSS Key" />
            <div class="field-tip">用于搜索和下载</div>
          </el-form-item>
          <el-form-item label="API 地址">
            <el-input v-model="addForm.api_url" placeholder="https://api.example.com（可选）" />
          </el-form-item>
        </template>

        <el-form-item v-if="addForm.auth_method === 'passkey'" label="Passkey" required>
          <el-input
            v-model="addForm.passkey"
            type="password"
            show-password
            placeholder="请输入站点 Passkey" />
          <div class="field-tip">在站点的「个人设置」页面里可以找到</div>
        </el-form-item>

        <div class="field-rule"></div>

        <el-form-item label="下载器">
          <el-select
            v-model="addForm.downloader_id"
            style="width: 100%"
            clearable
            placeholder="使用默认下载器">
            <el-option
              v-for="dl in enabledDownloaders"
              :key="dl.id"
              :label="dl.name + (dl.is_default ? ' (默认)' : '') + (!dl.enabled ? ' (未启用)' : '')"
              :value="dl.id"
              :disabled="!dl.enabled" />
          </el-select>
          <div class="field-tip">
            留空使用默认下载器。灰色选项表示该下载器未启用，先去「下载器管理」里打开。
          </div>
        </el-form-item>
      </el-form>

      <div
        v-if="showValidationResult && validationResult"
        class="pt-note"
        :class="validationResult.valid ? 'pt-note--ok' : 'pt-note--dang'">
        <PtIcon
          :name="validationResult.valid ? 'circle-check' : 'circle-alert'"
          :size="14"
          class="pt-note__icon" />
        <div class="vres">
          <strong>{{ validationResult.valid ? "验证成功" : "验证失败" }}</strong>
          <span>{{ validationResult.message }}</span>
          <template v-if="validationResult.free_torrents?.length">
            <span class="vres__head">
              发现 {{ validationResult.free_torrents.length }} 个免费种子
            </span>
            <ul class="vres__list">
              <li v-for="(t, i) in validationResult.free_torrents.slice(0, 5)" :key="i">{{ t }}</li>
            </ul>
            <span v-if="validationResult.free_torrents.length > 5" class="vres__more">
              还有 {{ validationResult.free_torrents.length - 5 }} 个未列出
            </span>
          </template>
        </div>
      </div>

      <template #footer>
        <el-button @click="showAddDialog = false">取消</el-button>
        <el-button :loading="validating" @click="validateSite">
          <PtIcon name="check" :size="14" /><span>验证配置</span>
        </el-button>
        <el-button type="primary" :loading="saving" @click="createSite">创建站点</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="showImportDialog"
      class="pt-dialog"
      title="导入站点模板"
      width="580px"
      align-center>
      <el-form :model="importForm" class="pt-form" label-position="top">
        <el-form-item label="模板 JSON" required>
          <el-input
            v-model="importForm.templateJson"
            type="textarea"
            :rows="8"
            placeholder="粘贴模板 JSON 内容" />
          <div class="field-tip">模板只描述站点结构，认证信息要单独填在下面</div>
        </el-form-item>

        <div class="field-rule"></div>

        <el-form-item label="Cookie">
          <el-input
            v-model="importForm.cookie"
            type="textarea"
            :rows="2"
            placeholder="模板使用 Cookie 认证时填写" />
        </el-form-item>

        <el-form-item label="API Key">
          <el-input v-model="importForm.api_key" placeholder="模板使用 API Key 认证时填写" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showImportDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="importTemplate">
          <PtIcon name="upload" :size="14" /><span>导入</span>
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.dynamic-sites-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
}

/* 名称 + 标识同一行：34 高的行里放不下两行文本，标识压成等宽小字跟在后面 */
.ident {
  display: flex;
  gap: 6px;
  align-items: baseline;
  min-width: 0;
}

.ident__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ident__id {
  flex: 0 0 auto;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  font-weight: 400;
  color: var(--pt-t4);
}
</style>

<style>
/* 验证结果落在 teleport 到 body 的对话框里，scoped 选择器到不了 */
.pt-dialog .vres {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.pt-dialog .vres strong {
  font-weight: 600;
  color: var(--pt-t1);
}

.pt-dialog .vres__head {
  margin-top: 4px;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.pt-dialog .vres__list {
  margin: 0;
  padding-left: 18px;
  font-size: var(--pt-fz-label);
}

.pt-dialog .vres__more {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}
</style>
