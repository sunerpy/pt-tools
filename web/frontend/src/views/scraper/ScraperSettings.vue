<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { ElMessage } from "element-plus";
import { useScraperStore } from "@/stores/scraper";
import { scraperApi } from "@/api";

const store = useScraperStore();
const activeTab = ref("providers");

// Providers tab
const tmdbKey = ref("");
const tmdbProxy = ref("");
const tmdbTesting = ref(false);
const doubanEnabled = ref(true);
const doubanProxy = ref("");

// Connectors tab
const connUrl = ref("");
const connApiKey = ref("");
const connType = ref("auto");
const testingId = ref<number | null>(null);

// LLM tab
const llmPresetName = ref("openai");
const llmApiKey = ref("");
const llmBaseURL = ref("");
const llmModel = ref("");

// General
const scanInterval = ref(6);
const workerConcurrency = ref(3);
const cacheSize = ref(1024);

onMounted(async () => {
  await Promise.allSettled([
    store.fetchProviders(),
    store.fetchConnectors(),
    store.fetchLLMProviders(),
  ]);
});

const llmPresetOptions = computed(() => {
  const list = Array.isArray(store.llmProviders) ? store.llmProviders : [];
  return list.map((p) => ({ value: p.name, label: p.name.toUpperCase(), preset: p }));
});

function onLLMPresetChange(name: string) {
  const list = Array.isArray(store.llmProviders) ? store.llmProviders : [];
  const preset = list.find((p) => p.name === name);
  if (preset) {
    llmBaseURL.value = preset.base_url ?? "";
    llmModel.value = preset.models?.[0] ?? "";
  }
}

async function saveTMDB() {
  try {
    await scraperApi.setProviderCredential("tmdb", {
      api_key: tmdbKey.value,
      proxy_url: tmdbProxy.value,
    });
    ElMessage.success("TMDB 配置已保存");
  } catch (e: unknown) {
    ElMessage.error(`保存失败: ${(e as Error).message}`);
  }
}

async function testTMDB() {
  tmdbTesting.value = true;
  try {
    const result = await scraperApi.testProviderCredential("tmdb");
    if (result.ok) {
      ElMessage.success(result.message ?? "TMDB 凭证有效");
    } else {
      ElMessage.error(`TMDB 校验失败: ${result.error ?? "未知错误"}`);
    }
  } catch (e: unknown) {
    ElMessage.error(`测试失败: ${(e as Error).message}`);
  } finally {
    tmdbTesting.value = false;
  }
}

async function saveLLM() {
  try {
    await scraperApi.setProviderCredential(llmPresetName.value, {
      api_key: llmApiKey.value,
      base_url: llmBaseURL.value,
      model_name: llmModel.value,
    });
    ElMessage.success(`${llmPresetName.value} 配置已保存`);
  } catch (e: unknown) {
    ElMessage.error(`保存失败: ${(e as Error).message}`);
  }
}

async function testConn(id: number) {
  testingId.value = id;
  try {
    const result = await scraperApi.testConnector(id);
    if (result.success) ElMessage.success(`连接成功: ${result.message ?? ""}`);
    else ElMessage.error(`连接失败: ${result.message ?? "未知错误"}`);
  } catch (e: unknown) {
    ElMessage.error(`测试失败: ${(e as Error).message}`);
  } finally {
    testingId.value = null;
  }
}
</script>

<template>
  <div class="settings-wrap">
    <header><h2>刮削设置</h2></header>

    <el-tabs v-model="activeTab" type="card">
      <el-tab-pane label="数据源" name="providers">
        <section class="tab-section">
          <el-alert type="info" :closable="false" show-icon class="providers-hint">
            <template #title>
              <strong>刮削数据源说明</strong>
            </template>
            <div class="providers-hint-body">
              <p>
                <el-tag type="success" size="small">豆瓣</el-tag>
                <el-tag type="success" size="small" style="margin-left: 6px">IMDb</el-tag>
                开箱即用，无需配置。
              </p>
              <p>
                <el-tag type="warning" size="small">TMDB</el-tag>
                需要用户自备 API Token（免费，申请后填入下方）。每人一个独立 Token
                既保护了你的配额也避免了被滥用时被封。
              </p>
            </div>
          </el-alert>

          <div class="provider-card">
            <div class="provider-card-header">
              <h3>TMDB</h3>
              <el-tag type="warning" size="small">需要 API Token</el-tag>
            </div>
            <p class="provider-desc">
              The Movie Database —— 欧美电影 / 剧集元数据和海报的主要来源。
              <el-link
                type="primary"
                href="https://www.themoviedb.org/settings/api"
                target="_blank"
                rel="noopener">
                点此申请免费 API Token →
              </el-link>
            </p>
            <el-form label-width="120">
              <el-form-item label="Bearer Token">
                <el-input
                  v-model="tmdbKey"
                  type="password"
                  show-password
                  placeholder="eyJhbGciOi... 或 v3 API Key"
                  @keyup.enter="saveTMDB" />
              </el-form-item>
              <el-form-item label="代理 URL">
                <el-input v-model="tmdbProxy" placeholder="http://proxy:port (可选)" />
              </el-form-item>
              <el-form-item>
                <el-button type="primary" @click="saveTMDB">保存 TMDB 配置</el-button>
                <el-button :loading="tmdbTesting" @click="testTMDB">测试凭证</el-button>
              </el-form-item>
            </el-form>
          </div>

          <div class="provider-card">
            <div class="provider-card-header">
              <h3>豆瓣</h3>
              <el-tag type="success" size="small">无需配置</el-tag>
            </div>
            <p class="provider-desc">
              使用内置 Frodo App Key + HTML 网页降级抓取，无需用户提供凭证。
              如遇限流可配置自定义代理。
            </p>
            <el-form label-width="120">
              <el-form-item label="启用">
                <el-switch v-model="doubanEnabled" disabled />
                <span class="form-hint">豆瓣为零配置 provider，系统自动注册</span>
              </el-form-item>
              <el-form-item label="代理 URL">
                <el-input v-model="doubanProxy" placeholder="http://proxy:port (可选)" />
              </el-form-item>
            </el-form>
          </div>

          <div class="provider-card">
            <div class="provider-card-header">
              <h3>IMDb</h3>
              <el-tag type="success" size="small">无需配置</el-tag>
            </div>
            <p class="provider-desc">
              IMDb 官方没有公开 API，使用 HTML 页面 + JSON-LD 结构化数据抓取。 适合作为
              TMDB/豆瓣的补充源（尤其对英文原名识别有帮助）。
            </p>
          </div>
        </section>
      </el-tab-pane>

      <el-tab-pane label="媒体服务器" name="connectors">
        <section class="tab-section">
          <h3>Jellyfin / Emby</h3>
          <el-table :data="store.connectors" size="small">
            <el-table-column prop="name" label="名称" />
            <el-table-column prop="type" label="类型" width="100" />
            <el-table-column prop="base_url" label="URL" show-overflow-tooltip />
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <el-tag v-if="row.last_ping_ok" type="success" size="small">正常</el-tag>
                <el-tag v-else type="info" size="small">未测试</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="120" align="center">
              <template #default="{ row }">
                <div class="table-cell-actions">
                  <el-button size="small" :loading="testingId === row.id" @click="testConn(row.id)">
                    测试
                  </el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>

          <h4 style="margin-top: 24px">添加新服务器</h4>
          <el-form label-width="100" inline>
            <el-form-item label="URL">
              <el-input v-model="connUrl" placeholder="http://jellyfin:8096" style="width: 280px" />
            </el-form-item>
            <el-form-item label="API Key">
              <el-input v-model="connApiKey" type="password" show-password style="width: 240px" />
            </el-form-item>
            <el-form-item label="类型">
              <el-select v-model="connType" style="width: 120px">
                <el-option label="自动" value="auto" />
                <el-option label="Jellyfin" value="jellyfin" />
                <el-option label="Emby" value="emby" />
              </el-select>
            </el-form-item>
            <el-button type="primary">添加</el-button>
          </el-form>
        </section>
      </el-tab-pane>

      <el-tab-pane label="LLM" name="llm">
        <section class="tab-section">
          <h3>LLM Provider 配置</h3>
          <p class="hint">
            支持 10+ provider：OpenAI / Kimi / GLM / Qwen / DeepSeek / 豆包 / Yi / 百川 / Groq /
            Ollama。 选择预设后自动填写 Base URL。
          </p>
          <el-form label-width="120">
            <el-form-item label="Provider">
              <el-select v-model="llmPresetName" style="width: 100%" @change="onLLMPresetChange">
                <el-option
                  v-for="o in llmPresetOptions"
                  :key="o.value"
                  :label="o.label + (o.preset.notes ? ` (${o.preset.notes})` : '')"
                  :value="o.value" />
              </el-select>
            </el-form-item>
            <el-form-item label="Base URL">
              <el-input v-model="llmBaseURL" placeholder="自动从预设填入" />
            </el-form-item>
            <el-form-item label="API Key">
              <el-input v-model="llmApiKey" type="password" show-password />
            </el-form-item>
            <el-form-item label="模型">
              <el-input v-model="llmModel" />
            </el-form-item>
            <el-button type="primary" @click="saveLLM">保存 LLM 配置</el-button>
          </el-form>
        </section>
      </el-tab-pane>

      <el-tab-pane label="通用" name="general">
        <section class="tab-section">
          <el-form label-width="140">
            <el-form-item label="扫描间隔（小时）">
              <el-input-number v-model="scanInterval" :min="1" :max="72" />
            </el-form-item>
            <el-form-item label="Worker 并发数">
              <el-input-number v-model="workerConcurrency" :min="1" :max="20" />
            </el-form-item>
            <el-form-item label="缓存大小（MB）">
              <el-input-number v-model="cacheSize" :min="128" :max="10240" :step="128" />
            </el-form-item>
          </el-form>
        </section>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped>
@import "@/styles/table-page.css";

.settings-wrap {
  padding: 24px;
}
.settings-wrap header {
  margin-bottom: 16px;
}
.settings-wrap h2 {
  margin: 0;
}
.tab-section {
  padding: 12px 0;
}
.tab-section h3 {
  font-size: 1.02rem;
  margin: 0 0 16px;
}
.tab-section h4 {
  font-size: 0.95rem;
  margin: 0 0 12px;
  color: var(--pt-text-secondary, #78716c);
}
.hint {
  color: var(--pt-text-secondary, #78716c);
  font-size: 0.88rem;
  margin-bottom: 16px;
}

.providers-hint {
  margin-bottom: 24px;
}
.providers-hint-body {
  margin-top: 6px;
}
.providers-hint-body p {
  margin: 4px 0;
  font-size: 0.88rem;
  line-height: 1.6;
}

.provider-card {
  background: var(--pt-surface-1, #fafaf9);
  border: 1px solid var(--pt-border, #e7e5e4);
  border-radius: 10px;
  padding: 20px 24px;
  margin-bottom: 18px;
}
.provider-card-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.provider-card-header h3 {
  margin: 0;
  font-size: 1.02rem;
}
.provider-desc {
  color: var(--pt-text-secondary, #78716c);
  font-size: 0.86rem;
  margin: 0 0 16px;
  line-height: 1.6;
}
.form-hint {
  margin-left: 10px;
  color: var(--pt-text-tertiary, #a8a29e);
  font-size: 0.82rem;
}
</style>
