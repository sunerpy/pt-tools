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
</script>

<template>
  <div class="page-container">
    <!-- 动态站点列表 -->
    <el-card v-loading="loading" shadow="never" class="section-card">
      <template #header>
        <div class="card-header">
          <span>动态站点</span>
          <el-space>
            <el-button type="success" :icon="'Upload'" @click="openImportDialog">
              导入模板
            </el-button>
            <el-button type="primary" :icon="'Plus'" @click="openAddDialog">添加站点</el-button>
          </el-space>
        </div>
      </template>

      <el-table :data="dynamicSites" style="width: 100%">
        <el-table-column type="index" label="序号" width="60" align="center" />

        <el-table-column label="站点" min-width="150">
          <template #default="{ row }">
            <div class="site-info">
              <span class="site-name">{{ row.display_name || row.name }}</span>
              <span class="site-id">{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="状态" min-width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">
              {{ row.enabled ? "已启用" : "未启用" }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="认证方式" min-width="120" align="center">
          <template #default="{ row }">
            <el-tag type="warning" size="small" effect="plain">
              {{ getAuthMethodLabel(row.auth_method) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="下载器" min-width="100" align="center">
          <template #default="{ row }">
            <span>{{ getDownloaderName(row.downloader_id) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="类型" min-width="100" align="center">
          <template #default="{ row }">
            <el-tag :type="row.is_builtin ? 'primary' : 'success'" size="small" effect="plain">
              {{ row.is_builtin ? "内置" : "动态" }}
            </el-tag>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="dynamicSites.length === 0" description="暂无动态站点" />
    </el-card>

    <!-- 模板列表 -->
    <el-card shadow="never" class="section-card">
      <template #header>
        <div class="card-header">
          <span>站点模板</span>
        </div>
      </template>

      <el-table :data="templates" style="width: 100%">
        <el-table-column type="index" label="序号" width="60" align="center" />

        <el-table-column label="模板名称" min-width="150">
          <template #default="{ row }">
            <div class="site-info">
              <span class="site-name">{{ row.display_name || row.name }}</span>
              <span class="site-id">{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="认证方式" min-width="120" align="center">
          <template #default="{ row }">
            <el-tag type="warning" size="small" effect="plain">
              {{ getAuthMethodLabel(row.auth_method) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="描述" min-width="200">
          <template #default="{ row }">
            <span>{{ row.description || "-" }}</span>
          </template>
        </el-table-column>

        <el-table-column label="版本" min-width="80" align="center">
          <template #default="{ row }">
            <span>{{ row.version || "-" }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" min-width="100" align="center">
          <template #default="{ row }">
            <el-button type="primary" size="small" @click="exportTemplate(row)">导出</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="templates.length === 0" description="暂无站点模板" />
    </el-card>

    <!-- 添加站点对话框 -->
    <el-dialog v-model="showAddDialog" title="添加动态站点" width="600px">
      <el-form :model="addForm" label-width="100px" label-position="right">
        <el-form-item label="站点标识" required>
          <el-input v-model="addForm.name" placeholder="例如: mysite" />
          <div class="form-tip">唯一标识，用于内部引用</div>
        </el-form-item>

        <el-form-item label="显示名称">
          <el-input v-model="addForm.display_name" placeholder="例如: 我的站点" />
        </el-form-item>

        <el-form-item label="站点URL">
          <el-input v-model="addForm.base_url" placeholder="https://example.com" />
        </el-form-item>

        <el-divider />

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
            placeholder="请输入站点Cookie" />
        </el-form-item>

        <template v-if="addForm.auth_method === 'api_key'">
          <el-form-item label="API Key" required>
            <el-input v-model="addForm.api_key" placeholder="请输入API Key" />
          </el-form-item>
          <el-form-item label="API URL">
            <el-input v-model="addForm.api_url" placeholder="https://api.example.com" />
          </el-form-item>
        </template>

        <template v-if="addForm.auth_method === 'cookie_and_api_key'">
          <el-form-item label="Cookie" required>
            <el-input
              v-model="addForm.cookie"
              type="textarea"
              :rows="3"
              placeholder="请输入站点Cookie（用于获取时魔等信息）" />
          </el-form-item>
          <el-form-item label="API Key / RSS Key" required>
            <el-input
              v-model="addForm.api_key"
              placeholder="请输入API Key或RSS Key（用于搜索和下载）" />
          </el-form-item>
          <el-form-item label="API URL">
            <el-input v-model="addForm.api_url" placeholder="https://api.example.com（可选）" />
          </el-form-item>
        </template>

        <el-divider />

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
          <div class="form-tip">
            为此站点指定专用下载器，留空使用默认。灰色选项表示下载器未启用，请先在下载器管理中启用。
          </div>
        </el-form-item>
      </el-form>

      <!-- 验证结果 -->
      <div v-if="showValidationResult && validationResult" class="validation-result">
        <el-alert
          :title="validationResult.valid ? '验证成功' : '验证失败'"
          :type="validationResult.valid ? 'success' : 'error'"
          :description="validationResult.message"
          show-icon />
        <div v-if="validationResult.free_torrents?.length" class="free-torrents">
          <div class="free-torrents-title">
            发现 {{ validationResult.free_torrents.length }} 个免费种子:
          </div>
          <ul>
            <li v-for="(t, i) in validationResult.free_torrents.slice(0, 5)" :key="i">
              {{ t }}
            </li>
          </ul>
          <div v-if="validationResult.free_torrents.length > 5" class="more-hint">
            还有 {{ validationResult.free_torrents.length - 5 }} 个...
          </div>
        </div>
      </div>

      <template #footer>
        <el-button @click="showAddDialog = false">取消</el-button>
        <el-button type="info" :loading="validating" @click="validateSite">验证配置</el-button>
        <el-button type="primary" :loading="saving" @click="createSite">创建站点</el-button>
      </template>
    </el-dialog>

    <!-- 导入模板对话框 -->
    <el-dialog v-model="showImportDialog" title="导入站点模板" width="600px">
      <el-form :model="importForm" label-width="100px" label-position="right">
        <el-form-item label="模板JSON" required>
          <el-input
            v-model="importForm.templateJson"
            type="textarea"
            :rows="8"
            placeholder="粘贴模板JSON内容" />
        </el-form-item>

        <el-divider>认证信息</el-divider>

        <el-form-item label="Cookie">
          <el-input
            v-model="importForm.cookie"
            type="textarea"
            :rows="2"
            placeholder="如果模板使用Cookie认证，请在此输入" />
        </el-form-item>

        <el-form-item label="API Key">
          <el-input
            v-model="importForm.api_key"
            placeholder="如果模板使用API Key认证，请在此输入" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showImportDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="importTemplate">导入</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-container {
  width: 100%;
}

.section-card {
  margin-bottom: 20px;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 16px;
  font-weight: 600;
}

.site-info {
  display: flex;
  flex-direction: column;
}

.site-name {
  font-weight: 500;
}

.site-id {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.form-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}

.validation-result {
  margin-top: 16px;
  padding: 16px;
  background: var(--el-fill-color-light);
  border-radius: 4px;
}

.free-torrents {
  margin-top: 12px;
}

.free-torrents-title {
  font-weight: 500;
  margin-bottom: 8px;
}

.free-torrents ul {
  margin: 0;
  padding-left: 20px;
}

.free-torrents li {
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.more-hint {
  font-size: 12px;
  color: var(--el-text-color-placeholder);
  margin-top: 4px;
}
</style>
