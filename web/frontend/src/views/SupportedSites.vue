<script setup lang="ts">
import { type SupportedSiteDefinition, sitesApi } from "@/api";
import SiteAvatar from "@/components/SiteAvatar.vue";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref } from "vue";

const loading = ref(false);
const definitions = ref<SupportedSiteDefinition[]>([]);
const search = ref("");
const schemaFilter = ref("");

onMounted(async () => {
  await loadDefinitions();
});

async function loadDefinitions() {
  loading.value = true;
  try {
    const data = await sitesApi.listDefinitions();
    definitions.value = (data ?? [])
      .slice()
      .sort((a, b) => a.name.localeCompare(b.name, "zh-Hans-CN"));
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
  }
}

const schemaOptions = computed(() => {
  const counts = new Map<string, number>();
  for (const d of definitions.value) {
    counts.set(d.schema, (counts.get(d.schema) ?? 0) + 1);
  }
  return Array.from(counts.entries())
    .map(([schema, count]) => ({ schema, count }))
    .sort((a, b) => b.count - a.count);
});

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase();
  return definitions.value.filter((d) => {
    if (schemaFilter.value && d.schema !== schemaFilter.value) return false;
    if (!q) return true;
    if (d.name.toLowerCase().includes(q)) return true;
    if (d.id.toLowerCase().includes(q)) return true;
    if (d.description?.toLowerCase().includes(q)) return true;
    if (d.aka?.some((a) => a.toLowerCase().includes(q))) return true;
    if (d.urls.some((u) => u.toLowerCase().includes(q))) return true;
    return false;
  });
});

const totalCount = computed(() => definitions.value.length);
const filteredCount = computed(() => filtered.value.length);
const unavailableCount = computed(() => definitions.value.filter((d) => d.unavailable).length);

function authMethodLabel(m?: string): string {
  switch (m) {
    case "cookie":
      return "Cookie";
    case "api_key":
      return "API Key";
    case "cookie_and_api_key":
      return "Cookie + API Key";
    case "passkey":
      return "Passkey";
    default:
      return m || "-";
  }
}

function authMethodTagType(m?: string): "primary" | "success" | "warning" | "info" {
  switch (m) {
    case "cookie":
      return "primary";
    case "api_key":
      return "success";
    case "cookie_and_api_key":
      return "warning";
    case "passkey":
      return "info";
    default:
      return "info";
  }
}

function clearFilters() {
  search.value = "";
  schemaFilter.value = "";
}
</script>

<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h1 class="page-title">已支持站点</h1>
        <p class="page-subtitle">
          pt-tools 当前已适配
          <strong>{{ totalCount }}</strong>
          个站点。点击站点 URL 可前往官方主页注册或登录。
        </p>
      </div>
      <div class="page-actions">
        <el-button :icon="'Refresh'" :loading="loading" @click="loadDefinitions">刷新</el-button>
        <el-button type="primary" @click="$router.push('/sites')">前往站点管理</el-button>
      </div>
    </div>

    <div class="filter-bar">
      <el-input
        v-model="search"
        placeholder="搜索：名称 / ID / 别名 / 域名"
        clearable
        :prefix-icon="'Search'"
        class="filter-input" />
      <el-select v-model="schemaFilter" placeholder="按架构筛选" clearable class="filter-select">
        <el-option
          v-for="opt in schemaOptions"
          :key="opt.schema"
          :label="`${opt.schema} (${opt.count})`"
          :value="opt.schema" />
      </el-select>
      <el-button v-if="search || schemaFilter" :icon="'Close'" @click="clearFilters">
        清空筛选
      </el-button>
      <div class="result-count">
        显示
        <strong>{{ filteredCount }}</strong>
        / {{ totalCount }} 站点
        <span v-if="unavailableCount > 0" class="unavailable-hint">
          （其中 {{ unavailableCount }} 个临时不可用）
        </span>
      </div>
    </div>

    <el-skeleton v-if="loading && definitions.length === 0" :rows="6" animated />

    <div v-else class="site-grid">
      <el-card
        v-for="def in filtered"
        :key="def.id"
        class="site-card"
        :class="{ 'is-unavailable': def.unavailable }"
        shadow="hover">
        <div class="site-card-header">
          <SiteAvatar :site-id="def.id" :site-name="def.name" :size="40" :no-fetch="true" />
          <div class="site-name-block">
            <div class="site-name">
              {{ def.name }}
              <el-tag v-if="def.unavailable" type="danger" size="small" class="status-tag">
                不可用
              </el-tag>
            </div>
            <div v-if="def.aka && def.aka.length > 0" class="site-aka">
              {{ def.aka.join(" / ") }}
            </div>
          </div>
        </div>

        <div v-if="def.description" class="site-description">
          {{ def.description }}
        </div>
        <div v-if="def.unavailable && def.unavailableReason" class="unavailable-reason">
          {{ def.unavailableReason }}
        </div>

        <div class="site-tags">
          <el-tag size="small" type="primary" effect="plain">{{ def.schema }}</el-tag>
          <el-tag size="small" :type="authMethodTagType(def.authMethod)" effect="plain">
            {{ authMethodLabel(def.authMethod) }}
          </el-tag>
          <el-tag v-if="def.hrEnabled" size="small" type="warning" effect="plain">H&amp;R</el-tag>
        </div>

        <div v-if="def.urls.length > 0" class="site-urls">
          <a
            v-for="url in def.urls"
            :key="url"
            :href="url"
            target="_blank"
            rel="noopener noreferrer"
            class="site-url-link">
            {{ url.replace(/^https?:\/\//, "").replace(/\/$/, "") }}
          </a>
        </div>
      </el-card>

      <el-empty v-if="!loading && filtered.length === 0" description="未找到匹配的站点" />
    </div>
  </div>
</template>

<style scoped>
.page-container {
  padding: 16px 24px 32px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 16px;
}

.page-title {
  font-size: 20px;
  font-weight: 600;
  margin: 0 0 4px 0;
  color: var(--pt-text-primary);
}

.page-subtitle {
  margin: 0;
  color: var(--pt-text-secondary);
  font-size: 13px;
}

.page-actions {
  display: flex;
  gap: 8px;
}

.filter-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 20px;
  padding: 12px 16px;
  background: var(--pt-bg-elevated, #fafafa);
  border-radius: 8px;
}

.filter-input {
  flex: 1;
  min-width: 240px;
  max-width: 360px;
}

.filter-select {
  min-width: 200px;
}

.result-count {
  margin-left: auto;
  color: var(--pt-text-secondary);
  font-size: 13px;
}

.unavailable-hint {
  color: var(--el-color-danger);
}

.site-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 16px;
}

.site-card {
  display: flex;
  flex-direction: column;
}

.site-card.is-unavailable {
  opacity: 0.7;
}

.site-card-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.site-name-block {
  flex: 1;
  min-width: 0;
}

.site-name {
  font-size: 16px;
  font-weight: 600;
  color: var(--pt-text-primary);
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-tag {
  flex-shrink: 0;
}

.site-aka {
  font-size: 12px;
  color: var(--pt-text-secondary);
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.site-description {
  color: var(--pt-text-secondary);
  font-size: 13px;
  line-height: 1.5;
  margin-bottom: 12px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.unavailable-reason {
  font-size: 12px;
  color: var(--el-color-danger);
  background: var(--el-color-danger-light-9);
  padding: 6px 10px;
  border-radius: 4px;
  margin-bottom: 12px;
}

.site-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 12px;
}

.site-urls {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
}

.site-url-link {
  color: var(--el-color-primary);
  text-decoration: none;
  word-break: break-all;
}

.site-url-link:hover {
  text-decoration: underline;
}
</style>
