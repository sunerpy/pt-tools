<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { Check, Close, Refresh } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { useScraperStore } from "@/stores/scraper";
import { scraperApi } from "@/api";

const store = useScraperStore();
const activeTab = ref("providers");

// Providers tab
const tmdbKey = ref("");
const tmdbProxy = ref("");
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

const llmPresetOptions = computed(() =>
  store.llmProviders.map((p) => ({ value: p.name, label: p.name.toUpperCase(), preset: p })),
);

function onLLMPresetChange(name: string) {
  const preset = store.llmProviders.find((p) => p.name === name);
  if (preset) {
    llmBaseURL.value = preset.base_url;
    llmModel.value = preset.models[0] || "";
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
    if (result.ok) ElMessage.success(`连接成功: ${result.message}`);
    else ElMessage.error(`连接失败: ${result.message}`);
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
          <h3>TMDB</h3>
          <el-form label-width="120">
            <el-form-item label="Bearer Token">
              <el-input v-model="tmdbKey" type="password" show-password />
            </el-form-item>
            <el-form-item label="代理 URL">
              <el-input v-model="tmdbProxy" placeholder="http://proxy:port (可选)" />
            </el-form-item>
            <el-button type="primary" @click="saveTMDB">保存 TMDB 配置</el-button>
          </el-form>

          <h3 style="margin-top: 32px">豆瓣</h3>
          <el-form label-width="120">
            <el-form-item label="启用">
              <el-switch v-model="doubanEnabled" />
            </el-form-item>
            <el-form-item label="代理 URL">
              <el-input v-model="doubanProxy" placeholder="可选" />
            </el-form-item>
          </el-form>
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
            <el-table-column label="操作" width="120">
              <template #default="{ row }">
                <el-button size="small" :loading="testingId === row.id" @click="testConn(row.id)">
                  测试
                </el-button>
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
</style>
