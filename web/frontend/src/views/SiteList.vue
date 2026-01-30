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
  if (["springsunday", "hdsky", "mteam"].includes(name.toLowerCase())) {
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

async function addSite() {
  try {
    const { value: name } = await ElMessageBox.prompt("请输入站点标识", "新增站点", {
      confirmButtonText: "确定",
      cancelButtonText: "取消",
      inputPlaceholder: "springsunday / hdsky / mteam 或自定义",
      inputValidator: (val) => {
        if (!val || !val.trim()) return "站点标识不能为空";
        if (sites.value[val.toLowerCase()]) return "站点已存在";
        return true;
      },
    });

    if (!name) return;

    const lower = name.toLowerCase();
    const payload: SiteConfig = {
      enabled: false,
      rss: [],
      auth_method: lower === "mteam" ? "api_key" : "cookie",
      cookie: "",
      api_key: "",
      api_url: "", // 预置站点的 API URL 由后端常量提供
    };

    await sitesApi.save(lower, payload);
    ElMessage.success("已新增站点");
    await loadSites();
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "新增失败");
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
        <el-button type="primary" :icon="'Plus'" @click="addSite" disabled>新增站点</el-button>
      </div>
    </div>

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
              effect="dark"
              class="status-tag">
              {{ row[1].enabled ? "已启用" : "未启用" }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="认证方式" min-width="120" align="center">
          <template #default="{ row }">
            <el-tag
              :type="row[1].auth_method === 'api_key' ? 'warning' : 'info'"
              size="small"
              effect="plain"
              round>
              {{ row[1].auth_method === "api_key" ? "API Key" : "Cookie" }}
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
              <el-button type="primary" size="small" text bg @click="manageSite(row[0])">
                配置
              </el-button>
              <el-button
                type="danger"
                size="small"
                text
                bg
                :disabled="['springsunday', 'hdsky', 'mteam'].includes(row[0].toLowerCase())"
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
  overflow: visible !important;
}

.mr-2 {
  margin-right: 8px;
}

.status-tag {
  font-weight: 600;
  letter-spacing: 0.5px;
}

:deep(.el-table__row) {
  transition: background-color var(--pt-transition-fast);
}

:deep(.el-table__row:hover) {
  background-color: var(--pt-bg-secondary) !important;
}

/* Dark mode adjustments */
html.dark .site-name {
  color: var(--pt-text-primary);
}
</style>
