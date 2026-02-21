<script setup lang="ts">
import { type SiteConfig, sitesApi } from "@/api";
import { ElMessage, ElMessageBox } from "element-plus";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";

const router = useRouter();

const loading = ref(false);
const sites = ref<Record<string, SiteConfig>>({});

onMounted(async () => {
  await loadSites();
});

async function loadSites() {
  loading.value = true;
  try {
    sites.value = await sitesApi.list();
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

function manageSite(name: string) {
  router.push(`/sites/${name}`);
}

function getRssCount(site: SiteConfig): number {
  return site.rss?.length || 0;
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
        <el-popover placement="bottom" :width="320" trigger="hover">
          <template #reference>
            <el-button type="primary" :icon="'Plus'" disabled>新增站点</el-button>
          </template>
          <div class="add-site-tip">
            <p>暂不支持手动添加站点。如需适配新站点，可通过以下方式：</p>
            <ol>
              <li>
                安装
                <a
                  href="https://github.com/sunerpy/pt-tools/releases"
                  target="_blank"
                  rel="noopener">
                  PT Tools Helper 浏览器扩展
                </a>
                ，在站点页面一键采集数据
              </li>
              <li>
                参考
                <a
                  href="https://github.com/sunerpy/pt-tools/blob/main/docs/guide/request-new-site.md"
                  target="_blank"
                  rel="noopener">
                  请求新增站点支持指南
                </a>
                提交 Issue
              </li>
            </ol>
          </div>
        </el-popover>
      </div>
    </div>

    <el-alert type="info" show-icon :closable="false" class="new-site-banner">
      <template #title>
        <span>
          需要适配新站点？安装
          <a href="https://github.com/sunerpy/pt-tools/releases" target="_blank" rel="noopener">
            PT Tools Helper 浏览器扩展
          </a>
          一键采集站点数据，然后按
          <a
            href="https://github.com/sunerpy/pt-tools/blob/main/docs/guide/request-new-site.md"
            target="_blank"
            rel="noopener">
            指南
          </a>
          提交 Issue 即可
        </span>
      </template>
    </el-alert>

    <div class="table-card" v-loading="loading">
      <div class="table-card-header">
        <div class="table-card-header-title">
          <el-icon class="mr-2"><Connection /></el-icon>
          站点列表
        </div>
      </div>

      <el-table
        :data="Object.entries(sites)"
        style="width: 100%"
        class="pt-table site-table"
        :header-cell-style="{ background: 'var(--pt-bg-secondary)', fontWeight: 600 }">
        <el-table-column type="index" label="#" width="60" align="center" />

        <el-table-column label="站点" min-width="150">
          <template #default="{ row }">
            <div class="table-cell-primary site-name-wrapper">
              <span class="site-name">{{ row[0] }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="状态" min-width="100" align="center">
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

        <el-table-column label="认证方式" min-width="120" align="center">
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

        <el-table-column label="RSS 订阅" min-width="100" align="center">
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

        <el-table-column label="操作" min-width="240" align="center" fixed="right">
          <template #default="{ row }">
            <div class="table-cell-actions">
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
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";
@import "@/styles/table-page.css";

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

.add-site-tip {
  font-size: 13px;
  line-height: 1.6;
  color: var(--pt-text-secondary);
}

.add-site-tip p {
  margin: 0 0 8px;
}

.add-site-tip ol {
  margin: 0;
  padding-left: 18px;
}

.add-site-tip li + li {
  margin-top: 6px;
}

.add-site-tip a {
  color: var(--el-color-primary);
  text-decoration: none;
}

.add-site-tip a:hover {
  text-decoration: underline;
}

.new-site-banner {
  margin-bottom: 16px;
}

.new-site-banner a {
  color: var(--el-color-primary);
  font-weight: 600;
  text-decoration: none;
}

.new-site-banner a:hover {
  text-decoration: underline;
}
</style>
