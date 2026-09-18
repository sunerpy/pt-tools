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

/**
 * 画板 p-life 1080「页面状态」：这一页可能处在哪几种状态、各自下一步做什么。
 * 判定只用本页已有的事实（端点填了没、token 存了没、测过没、测的结果），不猜后端。
 */
const pageStates = computed(() => {
  const hasEndpoint = Boolean(config.value.endpoint.trim());
  const hasToken = config.value.has_token || Boolean(tokenInput.value.trim());
  const tested = testResult.value !== null;
  const ok = testResult.value?.category === "success";
  return [
    {
      key: "unset",
      label: "未配置",
      desc: "端点还是空的，CloakBrowser 完全没参与探测。",
      active: !hasEndpoint,
      next: "填 Manager 端点，再填 auth token。",
    },
    {
      key: "no-token",
      label: "缺 token",
      desc: "端点填了但没存过 token，Manager 会返回 401。",
      active: hasEndpoint && !hasToken,
      next: "用 openssl rand -hex 32 生成一个，填进去保存。",
    },
    {
      key: "untested",
      label: "已配置未测试",
      desc: "端点与 token 都有了，但还没验证过能不能连上。",
      active: hasEndpoint && hasToken && !tested,
      next: "点「测试连接」确认一次。",
    },
    {
      key: "fail",
      label: "测试不通",
      desc: "最近一次测试没通过，右边那张卡写着分类与原话。",
      active: tested && !ok,
      next: "按分类排查：DNS / 端口 / token / 版本。",
    },
    {
      key: "ok",
      label: "连通",
      desc: "最近一次测试通过，Manager 版本也拿到了。",
      active: tested && ok,
      next: "无需动作。",
    },
  ];
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
    画板 28：intro 1080×58 → p-cfg 700（Manager 连接）/ p-res 364（探测结果）
    → p-life 1080×240（会话生命周期）。
    p-life 要后端暴露会话表，目前接口只回 Manager 连接状态与版本，那张没做。
  -->
  <div class="cloak-page pt-cards pt-cards--main" data-testid="cloak-config-page">
    <div class="pt-note pt-cards__full">
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

    <!--
      画板 p-res 364「探测结果」——「测试连接」这一次的结论。
      后端不存探测历史，所以这张卡只说这一次；没测过就说清下一步动作，不摆占位数字。
    -->
    <PtPanel class="res-card" title="探测结果" icon="signal">
      <ul v-if="testResult" class="res">
        <li class="res__row">
          <span class="res__k">结论</span>
          <PtStatusPill :tone="resultTone" size="sm" dot>
            {{ testResult.category === "success" ? "连通" : "不通" }}
          </PtStatusPill>
        </li>
        <li class="res__row">
          <span class="res__k">分类</span>
          <span class="res__v">{{ testResult.category }}</span>
        </li>
        <li v-if="testResult.manager_version" class="res__row">
          <span class="res__k">Manager 版本</span>
          <span class="res__v">v{{ testResult.manager_version }}</span>
        </li>
        <li class="res__detail">{{ resultText(testResult) }}</li>
      </ul>
      <p v-else class="res__empty">
        还没测过。填好端点与 token 后点左边的「测试连接」，这里会给出这一次的结论。
      </p>
      <p class="res__foot">后端不保存探测历史，这张卡只反映最近一次手动测试。</p>
    </PtPanel>

    <!--
      画板 p-life 1080「页面状态」：这一页的几种状态与各自的下一步。
      判定只用本页已有的事实，不猜后端。
    -->
    <PtPanel class="pt-cards__full" title="页面状态" icon="workflow">
      <ul class="life">
        <li
          v-for="st in pageStates"
          :key="st.key"
          class="life__row"
          :class="{ 'is-on': st.active }">
          <span class="life__dot" />
          <div class="life__body">
            <span class="life__k">
              {{ st.label }}
              <span v-if="st.active" class="life__now">当前</span>
            </span>
            <span class="life__d">{{ st.desc }}</span>
            <span class="life__n">下一步：{{ st.next }}</span>
          </div>
        </li>
      </ul>
      <p class="res__foot">
        CloakBrowser 的 schema 驱动在 <code>internal/cloakdriver/&lt;schema&gt;/</code> 下，
        但登录探测的 CloakTransport 目前仍是占位实现 —— 也就是说这一页配好之后，
        探测链路还没接上，连通性测试通过只代表 Manager 能连上。
      </p>
    </PtPanel>
  </div>
</template>

<style scoped>
/* 内缩 16 与卡间 16 由 .pt-cards--wide 给；画板这几块是 1080 通栏，不是 640 */

/* 页面状态卡：一列状态，当前那条高亮 */
.life {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: var(--pt-pad);
  margin: 0;
  padding: 0;
  list-style: none;
}

.life__row {
  display: flex;
  gap: var(--pt-space-2);
  padding: 10px 12px;
  background: var(--pt-hover);
  border-radius: var(--pt-r-md);
}

.life__row.is-on {
  background: var(--pt-p-soft);
}

.life__dot {
  flex: 0 0 auto;
  width: 6px;
  height: 6px;
  margin-top: 6px;
  background: var(--pt-t4);
  border-radius: 50%;
}

.life__row.is-on .life__dot {
  background: var(--pt-p);
}

.life__body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.life__k {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-t1);
}

.life__now {
  padding: 0 5px;
  font-size: var(--pt-fz-foot);
  font-weight: 500;
  color: var(--pt-p);
  background: color-mix(in srgb, var(--pt-p) 16%, transparent);
  border-radius: var(--pt-r-sm);
}

.life__d,
.life__n {
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.life__n {
  color: var(--pt-t3);
}

/* 右栏窄卡自己顶对齐，别被左边那张高卡拉长 */
.res-card {
  align-self: start;
}

.res {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.res__row {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  justify-content: space-between;
  font-size: var(--pt-fz-sm);
}

.res__k {
  color: var(--pt-t3);
}

.res__v {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t1);
}

.res__detail {
  padding-top: var(--pt-space-2);
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
  border-top: 1px solid var(--pt-border);
}

.res__empty {
  margin: 0;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.res__foot {
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t4);
}

/* 测试结果紧跟在最后一个字段之后，不再多留 el-form-item 的一档间距 */
.test-result {
  margin-top: calc(var(--pt-space-2) * -1);
}
</style>
