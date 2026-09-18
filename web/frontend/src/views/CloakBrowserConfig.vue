<script setup lang="ts">
import { cloakApi, type CloakConfig, type CloakTestCategory, type CloakTestResult } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
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

/* 认证失败与版本不匹配是「连上了但配置不对」，比连不上更接近成功，所以给 warn 而不是 dang */
const resultTone = computed<"ok" | "warn" | "dang">(() => {
  const cat = testResult.value?.category;
  if (cat === "success") return "ok";
  if (cat === "auth_fail" || cat === "not_found") return "warn";
  return "dang";
});

const RESULT_ICONS: Record<"ok" | "warn" | "dang", string> = {
  ok: "circle-check",
  warn: "triangle-alert",
  dang: "circle-alert",
};

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
  <!--
    画板 28：intro 1080×58 → p-cfg 700 / p-res 364 → p-life 1080×240。
    右栏 p-res（探测结果）与 p-life（会话生命周期）后端没有对应数据，本页先落
    intro + 通栏的 Manager 连接卡，见设计文档 §5 的偏离记录。
  -->
  <div class="cloak-page pt-cards pt-cards--wide" data-testid="cloak-config-page">
    <div class="pt-note">
      <PtIcon name="info" :size="14" class="pt-note__icon" />
      <span>
        CloakBrowser 为可选功能。默认探测路径仍为 cookie HTTP 直连；仅当某站点开启「使用
        CloakBrowser 后备」开关后才会走此路径。需先自行部署
        <code>cloakhq/cloakbrowser-manager</code>，详见 README 的 v2.0 升级章节。
      </span>
    </div>

    <PtPanel v-loading="loading" title="Manager 连接" icon="app-window">
      <template v-if="config.manager_version" #actions>
        <PtStatusPill data-testid="cloak-manager-version" tone="ok" dot>
          v{{ config.manager_version }}
        </PtStatusPill>
      </template>

      <el-form label-position="top" class="pt-form">
        <el-form-item label="CloakBrowser-Manager 端点">
          <el-input
            v-model="config.endpoint"
            data-testid="cloak-endpoint-input"
            placeholder="http://localhost:8080"
            clearable>
            <template #prefix>
              <PtIcon name="link" :size="14" />
            </template>
          </el-input>
          <div class="field-tip">
            docker-compose 部署时填 <code>http://cloakbrowser-manager:8080</code>；裸二进制模式填
            <code>http://localhost:8080</code>
          </div>
        </el-form-item>

        <el-form-item label="Auth Token">
          <el-input
            v-model="tokenInput"
            data-testid="cloak-token-input"
            type="password"
            show-password
            :placeholder="tokenPlaceholder"
            clearable>
            <template #prefix>
              <PtIcon name="key-round" :size="14" />
            </template>
          </el-input>
          <div class="field-tip">
            使用 <code>openssl rand -hex 32</code> 生成，保存后会以 AES-GCM 加密落库
          </div>
        </el-form-item>

        <div
          v-if="testResult"
          data-testid="cloak-test-result"
          class="pt-note test-result"
          :class="`pt-note--${resultTone}`">
          <PtIcon :name="RESULT_ICONS[resultTone]" :size="14" class="pt-note__icon" />
          <span>{{ resultText(testResult) }}</span>
        </div>
      </el-form>

      <template #footer>
        <span class="pt-foot-note">
          为开启了反爬虫（Cloudflare / ja3 指纹）的站点提供后备探测路径
        </span>
        <el-button data-testid="cloak-test-btn" :loading="testing" @click="testConnection">
          <PtIcon name="signal" :size="14" /><span>测试连接</span>
        </el-button>
        <el-button type="primary" data-testid="cloak-save-btn" :loading="saving" @click="save">
          <PtIcon name="save" :size="14" /><span>保存配置</span>
        </el-button>
      </template>
    </PtPanel>
  </div>
</template>

<style scoped>
/* 内缩 16 与卡间 16 由 .pt-cards--wide 给；画板这几块是 1080 通栏，不是 640 */

/* 测试结果紧跟在最后一个字段之后，不再多留 el-form-item 的一档间距 */
.test-result {
  margin-top: calc(var(--pt-space-2) * -1);
}
</style>
