<script setup lang="ts">
import { cloakApi, type CloakConfig, type CloakTestCategory, type CloakTestResult } from "@/api";
import { Connection, Key, Link } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref } from "vue";

const loading = ref(false);
const saving = ref(false);
const testing = ref(false);

const config = ref<CloakConfig>({ endpoint: "", has_token: false, manager_version: null });
const tokenInput = ref("");
const testResult = ref<CloakTestResult | null>(null);

const tokenPlaceholder = computed(() =>
  config.value.has_token ? "已设置（留空则保持不变）" : "请输入 auth token",
);

const alertType = computed<"success" | "warning" | "error">(() => {
  const cat = testResult.value?.category;
  if (cat === "success") return "success";
  if (cat === "auth_fail" || cat === "not_found") return "warning";
  return "error";
});

const categoryMessages: Record<CloakTestCategory, string> = {
  success: "连接成功",
  dns_fail: "DNS 解析失败 — 请检查 endpoint 主机名",
  conn_refused: "连接被拒 — CloakBrowser-Manager 服务可能未启动",
  timeout: "连接超时 — Manager 响应过慢或网络不通",
  auth_fail: "认证失败 — auth token 不正确",
  not_found: "Manager 版本不匹配（404 /api/status）",
  server_error: "Manager 内部错误（5xx）",
  protocol_error: "响应解析失败 — 协议不兼容",
  unknown: "未知错误",
};

function resultText(result: CloakTestResult): string {
  const base = categoryMessages[result.category] ?? result.message;
  if (result.category === "success" && result.manager_version) {
    return `${base}（Manager v${result.manager_version}）`;
  }
  return result.message ? `${base}：${result.message}` : base;
}

onMounted(async () => {
  loading.value = true;
  try {
    config.value = await cloakApi.getConfig();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载配置失败");
  } finally {
    loading.value = false;
  }
});

async function testConnection() {
  if (testing.value) return;
  testing.value = true;
  testResult.value = null;
  try {
    const payload: { endpoint?: string; token?: string } = {};
    if (config.value.endpoint) payload.endpoint = config.value.endpoint;
    if (tokenInput.value) payload.token = tokenInput.value;
    const result = await cloakApi.testConnection(payload);
    testResult.value = result;
    if (result.category === "success" && result.manager_version) {
      config.value.manager_version = result.manager_version;
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "测试连接失败");
  } finally {
    testing.value = false;
  }
}

async function save() {
  if (!config.value.endpoint) {
    ElMessage.error("端点不能为空");
    return;
  }
  if (saving.value) return;
  saving.value = true;
  try {
    const payload: { endpoint: string; token?: string } = { endpoint: config.value.endpoint };
    if (tokenInput.value) payload.token = tokenInput.value;
    await cloakApi.updateConfig(payload);
    ElMessage.success("配置已保存");
    tokenInput.value = "";
    config.value = await cloakApi.getConfig();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <div class="page-container" data-testid="cloak-config-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">CloakBrowser 配置</h1>
        <p class="page-subtitle">
          为开启了反爬虫（Cloudflare / ja3 指纹）的 PT 站点配置 CloakBrowser 后备探测路径
        </p>
      </div>
    </div>

    <el-alert type="info" show-icon :closable="false" class="intro-banner">
      <template #title>
        CloakBrowser 为可选功能。默认探测路径仍为 cookie HTTP 直连；仅当某站点开启「使用
        CloakBrowser 后备」开关后才会走此路径。需先自行部署
        <code>cloakhq/cloakbrowser-manager</code>，详见 README 的 v2.0 升级章节。
      </template>
    </el-alert>

    <div class="form-card" v-loading="loading">
      <el-form label-position="top" class="cloak-form">
        <el-form-item>
          <template #label>
            <span class="form-label">
              <el-icon><Link /></el-icon>
              CloakBrowser-Manager 端点
            </span>
          </template>
          <el-input
            v-model="config.endpoint"
            data-testid="cloak-endpoint-input"
            placeholder="http://localhost:8080"
            clearable />
          <p class="field-hint">
            docker-compose 部署时填 <code>http://cloakbrowser-manager:8080</code>；裸二进制模式填
            <code>http://localhost:8080</code>
          </p>
        </el-form-item>

        <el-form-item>
          <template #label>
            <span class="form-label">
              <el-icon><Key /></el-icon>
              Auth Token
            </span>
          </template>
          <el-input
            v-model="tokenInput"
            data-testid="cloak-token-input"
            type="password"
            show-password
            :placeholder="tokenPlaceholder"
            clearable />
          <p class="field-hint">
            使用 <code>openssl rand -hex 32</code> 生成，保存后会以 AES-GCM 加密落库
          </p>
        </el-form-item>

        <el-form-item v-if="config.manager_version">
          <template #label><span class="form-label">Manager 版本</span></template>
          <el-tag data-testid="cloak-manager-version" type="success" effect="plain">
            v{{ config.manager_version }}
          </el-tag>
        </el-form-item>

        <el-alert
          v-if="testResult"
          data-testid="cloak-test-result"
          :type="alertType"
          show-icon
          :closable="false"
          class="test-result">
          <template #title>{{ resultText(testResult) }}</template>
        </el-alert>

        <div class="form-actions">
          <el-button
            data-testid="cloak-test-btn"
            :icon="Connection"
            :loading="testing"
            @click="testConnection">
            测试连接
          </el-button>
          <el-button type="primary" data-testid="cloak-save-btn" :loading="saving" @click="save">
            保存配置
          </el-button>
        </div>
      </el-form>
    </div>
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";

.intro-banner {
  margin-bottom: 16px;
}

.intro-banner code,
.field-hint code {
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--pt-bg-secondary);
  font-size: 12px;
}

.form-card {
  border: 1px solid var(--pt-border-color);
  border-radius: var(--pt-radius-lg);
  background: var(--pt-bg-surface);
  padding: 24px;
  max-width: 640px;
}

.cloak-form {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.form-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  color: var(--pt-text-primary);
}

.field-hint {
  margin: 6px 0 0;
  font-size: 12px;
  color: var(--pt-text-secondary);
  line-height: 1.5;
}

.test-result {
  margin-bottom: 16px;
}

.form-actions {
  display: flex;
  gap: 12px;
  margin-top: 8px;
}
</style>
