<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { ElMessage } from "element-plus";
import { useScraperStore } from "@/stores/scraper";
import type { NFOResult } from "@/api";

const props = defineProps<{
  modelValue: boolean;
  initialPath?: string;
  libraryId?: number;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  saved: [result: NFOResult];
}>();

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit("update:modelValue", v),
});

const store = useScraperStore();

onMounted(() => {
  if (store.llmProviders.length === 0) store.fetchLLMProviders();
});

const userContext = ref("");
const selectedProvider = ref<string>("");
const mediaType = ref<"movie" | "tv" | "unknown">("movie");
const generating = ref(false);

const result = reactive<NFOResult>({
  title: "",
  original_title: "",
  year: 0,
  type: "movie",
  genres: [],
  language: "",
  plot: "",
  directors: [],
  cast: [],
  runtime: 0,
});

const providerOptions = computed(() =>
  store.llmProviders
    .filter((p) => p.configured !== false)
    .map((p) => ({
      label: p.notes ? `${p.name} (${p.notes})` : p.name.toUpperCase(),
      value: p.name,
    })),
);

async function generate() {
  if (!userContext.value.trim()) {
    ElMessage.warning("请输入文本描述");
    return;
  }
  generating.value = true;
  try {
    const nfo = await store.llmGenerate({
      provider: selectedProvider.value || undefined,
      text: userContext.value,
      type: mediaType.value,
    });
    Object.assign(result, nfo);
    ElMessage.success("生成完成，请确认字段");
  } catch (e: unknown) {
    ElMessage.error(`生成失败: ${(e as Error).message}`);
  } finally {
    generating.value = false;
  }
}

async function save() {
  if (!result.title) {
    ElMessage.warning("标题不能为空");
    return;
  }
  emit("saved", { ...result });
  ElMessage.success("已保存");
  visible.value = false;
}

function reset() {
  userContext.value = "";
  result.title = "";
  result.original_title = "";
  result.year = 0;
  result.plot = "";
  result.genres = [];
  result.directors = [];
  result.cast = [];
  result.runtime = 0;
}
</script>

<template>
  <el-drawer v-model="visible" title="AI 刮削（LLM）" size="860" direction="rtl" append-to-body>
    <el-alert type="warning" :closable="false" show-icon class="top-warn">
      <template #title>LLM 生成的元数据可能存在幻觉</template>
      <template #default>
        请确认标题、年份等关键字段准确后再保存。<strong>TMDB/IMDB ID</strong>
        若未经交叉验证将自动清空。
      </template>
    </el-alert>

    <div class="split">
      <!-- 左：用户输入 -->
      <section class="input-col">
        <h3>用户描述</h3>
        <el-form label-position="top">
          <el-form-item label="类型">
            <el-radio-group v-model="mediaType">
              <el-radio value="movie">电影</el-radio>
              <el-radio value="tv">剧集</el-radio>
              <el-radio value="unknown">未知</el-radio>
            </el-radio-group>
          </el-form-item>

          <el-form-item label="LLM Provider">
            <el-select v-model="selectedProvider" placeholder="默认（使用责任链）" clearable>
              <el-option
                v-for="opt in providerOptions"
                :key="opt.value"
                :label="opt.label"
                :value="opt.value" />
            </el-select>
          </el-form-item>

          <el-form-item label="文本描述（剧情简介、演员列表、导演等）">
            <el-input
              v-model="userContext"
              type="textarea"
              :rows="12"
              placeholder="粘贴 README.md 或剧情简介&#10;例如: 诺兰 2010 年的烧脑科幻电影，主演 Leonardo DiCaprio ..." />
          </el-form-item>

          <div class="actions">
            <el-button :loading="generating" type="primary" @click="generate">
              生成元数据
            </el-button>
            <el-button @click="reset">重置</el-button>
          </div>
        </el-form>
      </section>

      <!-- 右：NFO 预览 -->
      <section class="preview-col">
        <div class="preview-header">
          <h3>NFO 预览</h3>
          <el-tag type="warning" effect="dark" size="small">LLM-Generated</el-tag>
        </div>
        <el-form label-width="100" label-position="left">
          <el-form-item label="标题">
            <el-input v-model="result.title" />
          </el-form-item>
          <el-form-item label="原标题">
            <el-input v-model="result.original_title" />
          </el-form-item>
          <el-form-item label="年份">
            <el-input-number v-model="result.year" :min="1900" :max="2100" />
          </el-form-item>
          <el-form-item label="语言">
            <el-input v-model="result.language" />
          </el-form-item>
          <el-form-item label="时长（分钟）">
            <el-input-number v-model="result.runtime" :min="0" />
          </el-form-item>
          <el-form-item label="类型">
            <el-tag
              v-for="g in result.genres ?? []"
              :key="g"
              size="small"
              closable
              @close="result.genres = result.genres?.filter((x) => x !== g) ?? []">
              {{ g }}
            </el-tag>
          </el-form-item>
          <el-form-item label="剧情">
            <el-input v-model="result.plot" type="textarea" :rows="5" />
          </el-form-item>
          <el-form-item label="导演">
            <el-tag
              v-for="d in result.directors ?? []"
              :key="d"
              size="small"
              closable
              @close="result.directors = result.directors?.filter((x) => x !== d) ?? []">
              {{ d }}
            </el-tag>
          </el-form-item>
          <el-form-item label="演员">
            <el-tag
              v-for="a in result.cast ?? []"
              :key="a"
              size="small"
              closable
              @close="result.cast = result.cast?.filter((x) => x !== a) ?? []">
              {{ a }}
            </el-tag>
          </el-form-item>
        </el-form>
      </section>
    </div>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :disabled="!result.title" @click="save">确认保存</el-button>
    </template>
  </el-drawer>
</template>

<style scoped>
.top-warn {
  margin-bottom: 16px;
}

.split {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
  container-type: inline-size;
}

@container (width < 720px) {
  .split {
    grid-template-columns: 1fr;
  }
}

.input-col,
.preview-col {
  padding: 16px;
  border: 1px solid var(--pt-border-color, rgb(28 25 23 / 6%));
  border-radius: 12px;
  background: var(--pt-bg-surface-raised, #fff);
}

.input-col h3,
.preview-col h3 {
  margin: 0 0 12px;
  font-size: 1rem;
}

.preview-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.preview-col .el-tag {
  margin-right: 4px;
  margin-bottom: 4px;
}
</style>
