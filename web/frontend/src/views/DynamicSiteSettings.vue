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
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref } from "vue";

const isMobile = useIsMobile();
const saving = ref(false);
const validating = ref(false);

// 数据
const dynamicSites = ref<DynamicSiteSetting[]>([]);
const templates = ref<SiteTemplate[]>([]);
const downloaders = ref<DownloaderSetting[]>([]);
/**
 * 下载器列表只是站点表「下载器」列的名字来源，单独记一笔失败。
 * 它挂了不该把整张站点表画成错误态 —— 站点数据本身是好的，
 * 只是下载器名显示不出来，这正是 partial（部分数据失败）要表达的事。
 */
const downloadersFailed = ref(false);

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

/**
 * 六态状态机（设计文档 §5），两张表各一份。
 *
 * 以前这里是一个 loading ref + 一个 Promise.all：任意一个接口挂掉就整页弹个 toast，
 * 两秒后 toast 消失，两张表都停在「还没有数据」上 —— 用户看到的是「库是空的」，
 * 而真相是请求失败了，401/403 还会让人一直点重试。现在失败必须留在对应的表上。
 *
 * 两张表分开加载而不是 Promise.all：模板接口挂了不该把站点表一起拖成空表。
 */
const {
  loading: sitesLoading,
  state: sitesState,
  errorText: sitesErrorText,
  run: runSites,
  hasPartialBanner: sitesPartialBanner,
} = useDataState({ failed: () => (downloadersFailed.value ? 1 : 0) });

const {
  loading: tplLoading,
  state: tplState,
  errorText: tplErrorText,
  run: runTemplates,
} = useDataState();

/** 面板遮罩和刷新按钮看的是「还有请求在飞」 */
const loading = computed(() => sitesLoading.value || tplLoading.value);

/** 状态块的副标题：失败时给真实错误，空态时给下一步动作 */
const sitesSub = computed(() => {
  if (sitesState.value === "error" || sitesState.value === "perm") return sitesErrorText.value;
  if (sitesState.value === "partial") return "还没有动态站点，下载器列表也没有拿到";
  return "添加一个动态站点，或从模板导入";
});

const tplSub = computed(() => {
  if (tplState.value === "error" || tplState.value === "perm") return tplErrorText.value;
  return "模板是一份可分享的站点定义，导入后即可直接使用";
});

onMounted(async () => {
  await loadData();
});

async function loadSites() {
  const data = await runSites(() => dynamicSitesApi.list());
  // 失败时清空：留着上一次的数据配一个「加载失败」的状态块更让人误解
  dynamicSites.value = data ?? [];
}

async function loadTemplates() {
  const data = await runTemplates(() => templatesApi.list());
  templates.value = data ?? [];
}

async function loadDownloaders() {
  try {
    downloaders.value = await downloadersApi.list();
    downloadersFailed.value = false;
  } catch {
    downloaders.value = [];
    downloadersFailed.value = true;
  }
}

async function loadData() {
  await Promise.all([loadSites(), loadTemplates(), loadDownloaders()]);
  // toast 只是即时提醒，不再是唯一的反馈：状态已经画在对应的表格里了
  const err = sitesErrorText.value || tplErrorText.value;
  if (err) ElMessage.error(err);
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
      v-loading="sitesLoading"
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

      <!--
        partial：站点拿到了、下载器没拿到。这时不能用一整块状态图顶掉表格
        （那等于把已经拿到的站点也藏了），而是在表格上方挂一条提示。
      -->
      <div v-if="sitesPartialBanner(dynamicSites.length)" class="pt-note pt-note--warn panel-note">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>下载器列表没有拿到，站点绑定的下载器名会显示为「未知」，站点数据本身是完整的。</span>
      </div>

      <el-table v-if="!isMobile" :data="dynamicSites" class="pt-grid" style="width: 100%">
        <template #empty>
          <PtDataState :state="sitesState" dense :sub="sitesSub">
            <template v-if="sitesState !== 'perm' && sitesState !== 'loading'" #action>
              <el-button v-if="sitesState === 'error'" size="small" @click="loadSites">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
              <el-button v-else size="small" type="primary" @click="openAddDialog">
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

      <!--
        移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
        站点表桌面有 5 列，手机上横着滚既看不到列头，又和页面纵向滚动打架。
        卡上留的是：站点名 + 标识/认证方式/下载器/来源 + 启用状态。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!dynamicSites.length" :state="sitesState" :sub="sitesSub">
          <template v-if="sitesState !== 'perm' && sitesState !== 'loading'" #action>
            <el-button v-if="sitesState === 'error'" size="small" @click="loadSites">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
            <el-button v-else size="small" type="primary" @click="openAddDialog">
              <PtIcon name="plus" :size="14" /><span>添加站点</span>
            </el-button>
          </template>
        </PtDataState>

        <PtRowCard v-for="site in dynamicSites" :key="site.id ?? site.name">
          <template #title>{{ site.display_name || site.name }}</template>

          <template #meta>
            <code class="card-id">{{ site.name }}</code>
            <PtStatusPill :tone="authMethodTone(site.auth_method)" size="sm">
              {{ getAuthMethodLabel(site.auth_method) }}
            </PtStatusPill>
            <span>
              <PtIcon name="hard-drive" :size="11" />
              {{ getDownloaderName(site.downloader_id) }}
            </span>
            <PtTag>{{ site.is_builtin ? "内置" : "动态" }}</PtTag>
          </template>

          <template #status>
            <PtStatusPill dot :tone="site.enabled ? 'ok' : 'neutral'" size="sm">
              {{ site.enabled ? "已启用" : "未启用" }}
            </PtStatusPill>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <PtPanel
      v-loading="tplLoading"
      title="站点模板"
      icon="file-text"
      :count="`${templates.length} 个`"
      padding="none">
      <template #actions>
        <el-button size="small" @click="openImportDialog">
          <PtIcon name="upload" :size="14" /><span>导入模板</span>
        </el-button>
      </template>

      <el-table v-if="!isMobile" :data="templates" class="pt-grid" style="width: 100%">
        <template #empty>
          <PtDataState :state="tplState" dense :sub="tplSub">
            <template v-if="tplState !== 'perm' && tplState !== 'loading'" #action>
              <el-button v-if="tplState === 'error'" size="small" @click="loadTemplates">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
              <el-button v-else size="small" @click="openImportDialog">
                <PtIcon name="upload" :size="14" /><span>导入模板</span>
              </el-button>
            </template>
          </PtDataState>
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

      <!-- 移动端行卡：模板名 + 标识/版本/描述 + 认证方式 + 导出（主操作） -->
      <div v-else class="cards">
        <PtDataState v-if="!templates.length" :state="tplState" :sub="tplSub">
          <template v-if="tplState !== 'perm' && tplState !== 'loading'" #action>
            <el-button v-if="tplState === 'error'" size="small" @click="loadTemplates">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
            <el-button v-else size="small" @click="openImportDialog">
              <PtIcon name="upload" :size="14" /><span>导入模板</span>
            </el-button>
          </template>
        </PtDataState>

        <PtRowCard v-for="tpl in templates" :key="tpl.id">
          <template #title>{{ tpl.display_name || tpl.name }}</template>

          <template #meta>
            <code class="card-id">{{ tpl.name }}</code>
            <span v-if="tpl.version">v{{ tpl.version }}</span>
            <span v-if="tpl.description" class="card-desc">{{ tpl.description }}</span>
          </template>

          <template #status>
            <PtStatusPill :tone="authMethodTone(tpl.auth_method)" size="sm">
              {{ getAuthMethodLabel(tpl.auth_method) }}
            </PtStatusPill>
          </template>

          <template #actions>
            <el-button size="small" @click="exportTemplate(tpl)">
              <PtIcon name="download" :size="14" /><span>导出模板</span>
            </el-button>
          </template>
        </PtRowCard>
      </div>

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
          <div v-if="downloadersFailed" class="field-tip">
            下载器列表没有拿到，这里暂时是空的；先点面板上的「刷新」，或直接留空用默认下载器。
          </div>
          <div v-else class="field-tip">
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
  color: var(--pt-t3);
}

/* 移动端行卡列表：面板 padding="none"，所以留白由这里给 */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-pad) 0;
}

/* 表格贴边，所以面板内的提示条要自己补一圈外边距 */
.panel-note {
  margin: var(--pt-space-3) var(--pt-space-3) 0;
}

/* code 的字号要显式写：等宽字族下浏览器会套自己的默认字号，不跟着 meta 走 */
.card-id {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 描述比其他 meta 项长，单独占满一行再折行，不跟标签挤在同一排 */
.card-desc {
  flex: 1 1 100%;
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
