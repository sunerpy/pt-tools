<script setup lang="ts">
/*
 * 媒体识别（路线图 M9）：解析种子标题，再到 TMDB 找对应的电影或剧集；可以按 TMDB 编号手动纠正，
 * 也可以加识别词（屏蔽、替换、集数偏移）。TMDB 的 API Key 由用户自己申请，加密保存、只写不读。
 * 画板没有这一页，沿用站点组列表页的样式：页头 + 面板里的表单与表格，手机是行卡。
 */
import {
  type MediaKind,
  type MediaOverride,
  type MediaRecognizeResult,
  type MediaSettings,
  type MediaSettingsUpdate,
  type MediaWordInput,
  type MediaWordRule,
  type TMDBItem,
  mediaApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import {
  MEDIA_LANGUAGES,
  MEDIA_WORD_KINDS,
  mediaKindLabel,
  mediaSourceLabel,
  metaTags,
  posterURL,
  tmdbPageURL,
  wordEffect,
  wordKindLabel,
} from "@/utils/media";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref } from "vue";

const isMobile = useIsMobile();

const errText = (e: unknown, fallback: string) => (e as Error)?.message || fallback;

// ---- TMDB 设置 ----
const settings = ref<MediaSettings | null>(null);
const form = ref({ key: "", language: "zh-CN", proxy: "" });
const loadFailed = ref(false);
const saving = ref(false);
const testing = ref(false);

function fill(s: MediaSettings) {
  settings.value = s;
  form.value = { key: "", language: s.language, proxy: s.proxy_url };
}

async function loadSettings() {
  try {
    const s = await mediaApi.settings();
    if (s) fill(s);
    loadFailed.value = false;
  } catch {
    loadFailed.value = true;
  }
}

async function save(extra: Partial<MediaSettingsUpdate> = {}) {
  saving.value = true;
  try {
    const body: MediaSettingsUpdate = {
      language: form.value.language,
      proxy_url: form.value.proxy.trim(),
      ...extra,
    };
    const key = form.value.key.trim();
    if (key && extra.tmdb_api_key === undefined) body.tmdb_api_key = key;
    fill(await mediaApi.saveSettings(body));
    ElMessage.success("已保存");
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}

async function clearKey() {
  try {
    await ElMessageBox.confirm("清除后只做标题解析，不再到 TMDB 查找。", "清除 TMDB API Key", {
      type: "warning",
      confirmButtonText: "清除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  await save({ tmdb_api_key: "" });
}

async function testTMDB() {
  testing.value = true;
  try {
    await mediaApi.testTMDB();
    ElMessage.success("TMDB 连接正常");
  } catch (e) {
    ElMessage.error(errText(e, "连接 TMDB 失败"));
  } finally {
    testing.value = false;
  }
}

// ---- 识别预览 ----
const input = ref({ title: "", subtitle: "", imdb: "" });
const result = ref<MediaRecognizeResult | null>(null);
const recognizing = ref(false);
const recognizedInput = ref({ title: "", subtitle: "" });
const tags = computed(() => (result.value ? metaTags(result.value.meta) : []));

async function recognize() {
  const title = input.value.title.trim();
  const subtitle = input.value.subtitle.trim();
  if (!title && !subtitle) {
    ElMessage.warning("先填写标题");
    return;
  }
  recognizing.value = true;
  try {
    result.value = await mediaApi.recognize({ title, subtitle, imdb_id: input.value.imdb.trim() });
    recognizedInput.value = { title, subtitle };
  } catch (e) {
    ElMessage.error(errText(e, "识别失败"));
  } finally {
    recognizing.value = false;
  }
}

const manual = ref<{ id: number | undefined; type: MediaKind }>({ id: undefined, type: "movie" });
const correcting = ref(false);

async function correct(id: number | undefined, type: MediaKind) {
  if (!id || id <= 0) {
    ElMessage.warning("填写 TMDB 编号");
    return;
  }
  correcting.value = true;
  try {
    await mediaApi.setOverride({ ...recognizedInput.value, tmdb_id: id, media_type: type });
    ElMessage.success("已纠正：同一个名字的标题之后都识别成这一条");
    await Promise.all([rerun(), loadOverrides()]);
  } catch (e) {
    ElMessage.error(errText(e, "纠正失败"));
  } finally {
    correcting.value = false;
  }
}

async function rerun() {
  if (!recognizedInput.value.title && !recognizedInput.value.subtitle) return;
  try {
    result.value = await mediaApi.recognize({
      ...recognizedInput.value,
      imdb_id: input.value.imdb.trim(),
    });
  } catch {
    /* 纠正已经保存，重新识别失败时留着上一次的结果 */
  }
}

function itemTitle(it: TMDBItem): string {
  return it.original_title && it.original_title !== it.title
    ? `${it.title}（${it.original_title}）`
    : it.title;
}

// ---- 手动纠正 ----
const overrides = ref<MediaOverride[]>([]);
const overridesFailed = ref(false);

async function loadOverrides() {
  try {
    overrides.value = (await mediaApi.overrides()) ?? [];
    overridesFailed.value = false;
  } catch {
    overridesFailed.value = true;
  }
}

async function removeOverride(o: MediaOverride) {
  try {
    await ElMessageBox.confirm(`删除「${o.label}」的纠正？之后按名字重新搜索。`, "删除纠正", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  try {
    await mediaApi.deleteOverride(o.id);
    ElMessage.success("已删除");
    await loadOverrides();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}

// ---- 识别词 ----
const wordList = ref<MediaWordRule[]>([]);
const wordsFailed = ref(false);
const wordDialog = ref(false);
const wordSaving = ref(false);
const editingID = ref<number | null>(null);
const blankWord = (): MediaWordInput => ({
  kind: "replace",
  pattern: "",
  replacement: "",
  offset: 0,
  is_regex: false,
  enabled: true,
  note: "",
});
const wordForm = ref<MediaWordInput>(blankWord());

async function loadWords() {
  try {
    wordList.value = (await mediaApi.words()) ?? [];
    wordsFailed.value = false;
  } catch {
    wordsFailed.value = true;
  }
}

function openWord(r?: MediaWordRule) {
  editingID.value = r?.id ?? null;
  wordForm.value = r
    ? {
        kind: r.kind,
        pattern: r.pattern,
        replacement: r.replacement,
        offset: r.offset,
        is_regex: r.is_regex,
        enabled: r.enabled,
        note: r.note,
      }
    : blankWord();
  wordDialog.value = true;
}

async function saveWord() {
  const w = wordForm.value;
  if (!w.pattern.trim()) {
    ElMessage.warning("填写匹配的文字");
    return;
  }
  if (w.kind === "offset" && !w.offset) {
    ElMessage.warning("集数偏移不能为 0");
    return;
  }
  wordSaving.value = true;
  try {
    if (editingID.value) await mediaApi.updateWord(editingID.value, w);
    else await mediaApi.createWord(w);
    ElMessage.success("已保存识别词");
    wordDialog.value = false;
    await loadWords();
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    wordSaving.value = false;
  }
}

async function toggleWord(r: MediaWordRule, enabled: boolean) {
  try {
    await mediaApi.updateWord(r.id, { ...r, enabled });
    await loadWords();
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
    await loadWords();
  }
}

async function removeWord(r: MediaWordRule) {
  try {
    await ElMessageBox.confirm(`删除识别词「${r.pattern}」？`, "删除识别词", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  try {
    await mediaApi.deleteWord(r.id);
    ElMessage.success("已删除");
    await loadWords();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}

function refresh() {
  void Promise.all([loadSettings(), loadOverrides(), loadWords()]);
}

onMounted(refresh);
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub>解析种子标题，再到 TMDB 找对应的电影或剧集；可以手动纠正，也可以加识别词</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="media-refresh" @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <div class="pt-note" data-testid="media-attribution">
      <PtIcon name="info" :size="14" class="pt-note__icon" />
      <span
        >电影与剧集的数据来自
        <a href="https://www.themoviedb.org/" target="_blank" rel="noopener noreferrer">TMDB</a
        >。本产品使用 TMDB API，但未经 TMDB 认可或认证。API Key 需要自己在 TMDB
        的账号设置里申请。</span
      >
    </div>

    <PtPanel title="TMDB 设置" icon="key-round">
      <div v-if="loadFailed" class="pt-note pt-note--warn">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>设置没读到，点刷新重试。</span>
      </div>
      <el-form class="pt-form md-form" label-position="top" @submit.prevent>
        <el-form-item label="API Key">
          <div class="md-inline">
            <el-input
              v-model="form.key"
              type="password"
              show-password
              autocomplete="new-password"
              :placeholder="
                settings?.has_tmdb_key
                  ? '已设置；要更换时填写新的'
                  : 'v3 的 API Key 或 v4 的读取令牌'
              "
              data-testid="media-key" />
            <el-button v-if="settings?.has_tmdb_key" data-testid="media-clear-key" @click="clearKey"
              >清除</el-button
            >
          </div>
        </el-form-item>
        <div class="md-row">
          <el-form-item label="语言">
            <el-select v-model="form.language" data-testid="media-language">
              <el-option
                v-for="l in MEDIA_LANGUAGES"
                :key="l.value"
                :label="l.label"
                :value="l.value" />
            </el-select>
          </el-form-item>
          <el-form-item label="代理地址（可选）">
            <el-input
              v-model="form.proxy"
              placeholder="http://127.0.0.1:7890"
              autocomplete="off"
              data-testid="media-proxy" />
          </el-form-item>
        </div>
        <div class="md-actions">
          <el-button type="primary" :loading="saving" data-testid="media-save" @click="save()">
            <PtIcon v-if="!saving" name="save" :size="14" /><span>保存</span>
          </el-button>
          <el-button
            :loading="testing"
            :disabled="!settings?.has_tmdb_key"
            data-testid="media-test"
            @click="testTMDB">
            <PtIcon v-if="!testing" name="zap" :size="14" /><span>测试连接</span>
          </el-button>
        </div>
      </el-form>
      <p class="md-tip">
        代理地址留空时用环境变量里的代理（HTTP_PROXY、HTTPS_PROXY、ALL_PROXY），与访问站点相同；
        支持 http、https 与 socks5。
      </p>
    </PtPanel>

    <PtPanel title="识别预览" icon="scan">
      <el-form class="pt-form md-form" label-position="top" @submit.prevent>
        <el-form-item label="标题">
          <el-input
            v-model="input.title"
            placeholder="The.Last.of.Us.S01E01.2023.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX"
            data-testid="media-title"
            @keyup.enter="recognize" />
        </el-form-item>
        <div class="md-row">
          <el-form-item label="副标题（可选）">
            <el-input
              v-model="input.subtitle"
              placeholder="最后生还者 第一季 第01集 | 中英字幕"
              data-testid="media-subtitle" />
          </el-form-item>
          <el-form-item label="IMDb 编号或链接（可选）">
            <el-input v-model="input.imdb" placeholder="tt3581920" data-testid="media-imdb" />
          </el-form-item>
        </div>
        <el-button
          type="primary"
          :loading="recognizing"
          data-testid="media-recognize"
          @click="recognize">
          <PtIcon v-if="!recognizing" name="scan" :size="14" /><span>识别</span>
        </el-button>
      </el-form>

      <div v-if="result" class="md-result" data-testid="media-result">
        <div class="md-section">
          <div class="md-label">解析结果</div>
          <div class="md-summary" data-testid="media-summary">
            {{ result.summary || "没解析出名字" }}
            <span v-if="result.meta.name_cn && result.meta.name_en" class="md-sub">{{
              result.meta.name_cn
            }}</span>
          </div>
          <div class="md-tags" data-testid="media-tags">
            <PtTag v-for="t in tags" :key="t">{{ t }}</PtTag>
          </div>
          <p v-if="result.rule_hits?.length" class="md-tip" data-testid="media-rule-hits">
            套用了 {{ result.rule_hits.length }} 条识别词
          </p>
        </div>

        <div v-if="result.error" class="pt-note pt-note--warn" data-testid="media-error">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>TMDB 没查成：{{ result.error }}</span>
        </div>
        <div v-else-if="result.message" class="pt-note" data-testid="media-message">
          <PtIcon name="info" :size="14" class="pt-note__icon" />
          <span>{{ result.message }}</span>
        </div>

        <div v-if="result.match" class="md-match" data-testid="media-match">
          <img
            v-if="result.match.poster_path"
            class="md-poster"
            :src="posterURL(result.match.poster_path)"
            :alt="`${result.match.title} 的海报`"
            loading="lazy" />
          <div class="md-match__body">
            <div class="md-match__title">
              <a
                :href="tmdbPageURL(result.match.media_type, result.match.id)"
                target="_blank"
                rel="noopener noreferrer"
                >{{ itemTitle(result.match) }}</a
              >
            </div>
            <div class="md-match__meta">
              <PtStatusPill tone="ok" size="sm">{{ mediaSourceLabel(result.source) }}</PtStatusPill>
              <span>{{ mediaKindLabel(result.match.media_type) }}</span>
              <span v-if="result.match.year">{{ result.match.year }}</span>
              <span>TMDB {{ result.match.id }}</span>
              <span v-if="result.match.imdb_id">{{ result.match.imdb_id }}</span>
            </div>
            <p v-if="result.match.overview" class="md-overview">{{ result.match.overview }}</p>
          </div>
        </div>

        <div v-if="result.candidates?.length" class="md-section">
          <div class="md-label">候选（按匹配程度排序）</div>
          <div class="md-cands" data-testid="media-candidates">
            <div v-for="c in result.candidates" :key="`${c.media_type}-${c.id}`" class="md-cand">
              <img
                v-if="c.poster_path"
                class="md-poster md-poster--sm"
                :src="posterURL(c.poster_path, 'w92')"
                :alt="`${c.title} 的海报`"
                loading="lazy" />
              <div class="md-cand__body">
                <a
                  :href="tmdbPageURL(c.media_type, c.id)"
                  target="_blank"
                  rel="noopener noreferrer"
                  >{{ itemTitle(c) }}</a
                >
                <div class="md-sub">
                  {{ mediaKindLabel(c.media_type)
                  }}<template v-if="c.year"> · {{ c.year }}</template> · {{ c.score.toFixed(2) }} 分
                </div>
              </div>
              <el-button
                size="small"
                :disabled="
                  correcting ||
                  (result.match?.id === c.id && result.match?.media_type === c.media_type)
                "
                :data-testid="`media-pick-${c.media_type}-${c.id}`"
                @click="correct(c.id, c.media_type)"
                >选这个</el-button
              >
            </div>
          </div>
        </div>

        <div class="md-section">
          <div class="md-label">按 TMDB 编号纠正</div>
          <div class="md-inline">
            <el-select v-model="manual.type" class="md-kind" data-testid="media-manual-type">
              <el-option label="电影" value="movie" />
              <el-option label="剧集" value="tv" />
            </el-select>
            <el-input-number
              v-model="manual.id"
              :min="1"
              :controls="false"
              placeholder="TMDB 编号"
              data-testid="media-manual-id" />
            <el-button
              :loading="correcting"
              data-testid="media-manual-save"
              @click="correct(manual.id, manual.type)"
              >纠正</el-button
            >
          </div>
          <p class="md-tip">
            编号在 TMDB 网页地址里，如 themoviedb.org/movie/278 的 278。纠正按解析出的名字生效：
            同名的其它标题（换了分辨率、制作组）也会识别成这一条。
          </p>
        </div>
      </div>
    </PtPanel>

    <PtPanel title="手动纠正" icon="pencil" :count="overrides.length">
      <PtDataState
        v-if="!overrides.length"
        :state="overridesFailed ? 'error' : 'empty'"
        :title="overridesFailed ? '' : '还没有纠正'"
        :sub="
          overridesFailed ? '没读到，点刷新重试' : '识别不对时，在识别预览里选候选或填 TMDB 编号'
        " />
      <el-table
        v-else-if="!isMobile"
        :data="overrides"
        row-key="id"
        class="pt-grid"
        data-testid="media-overrides">
        <el-table-column
          label="解析结果"
          min-width="240"
          class-name="pt-cell-strong"
          prop="label" />
        <el-table-column label="识别成" min-width="240">
          <template #default="{ row }">
            <a
              :href="tmdbPageURL(row.media_type, row.tmdb_id)"
              target="_blank"
              rel="noopener noreferrer"
              >{{ row.title || row.tmdb_id }}</a
            >
            <span class="md-sub">
              · {{ mediaKindLabel(row.media_type) }} · TMDB {{ row.tmdb_id }}</span
            >
          </template>
        </el-table-column>
        <el-table-column label="" width="90" align="right">
          <template #default="{ row }">
            <el-button
              link
              type="danger"
              :data-testid="`media-override-del-${row.id}`"
              @click="removeOverride(row)"
              >删除</el-button
            >
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="md-cards">
        <PtRowCard v-for="o in overrides" :key="o.id">
          <template #title>{{ o.label }}</template>
          <template #meta>
            <span class="md-full">识别成 {{ o.title || o.tmdb_id }}</span>
            <span>{{ mediaKindLabel(o.media_type) }} · TMDB {{ o.tmdb_id }}</span>
          </template>
          <template #actions>
            <el-button size="small" type="danger" plain @click="removeOverride(o)">删除</el-button>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <PtPanel
      title="识别词"
      icon="tags"
      :count="wordList.length"
      action="添加"
      action-icon="plus"
      @action="openWord()">
      <p class="md-tip md-tip--top">
        解析标题之前按顺序套用：屏蔽去掉文字，替换换成另一段文字；集数偏移在解析之后给集数加上偏移量（可为负），
        用于按总集数编号的剧集。纯文字不分大小写；勾选正则时按 Go 的正则写法。
      </p>
      <PtDataState
        v-if="!wordList.length"
        :state="wordsFailed ? 'error' : 'empty'"
        :title="wordsFailed ? '' : '还没有识别词'"
        :sub="wordsFailed ? '没读到，点刷新重试' : '点右上角「添加」'" />
      <el-table
        v-else-if="!isMobile"
        :data="wordList"
        row-key="id"
        class="pt-grid"
        data-testid="media-words">
        <el-table-column label="种类" width="110">
          <template #default="{ row }">{{ wordKindLabel(row.kind) }}</template>
        </el-table-column>
        <el-table-column
          label="匹配"
          min-width="200"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">
            <code>{{ row.pattern }}</code
            ><PtTag v-if="row.is_regex" class="md-regex">正则</PtTag>
          </template>
        </el-table-column>
        <el-table-column
          label="效果"
          min-width="200"
          class-name="pt-cell-1line"
          show-overflow-tooltip>
          <template #default="{ row }">{{ wordEffect(row) }}</template>
        </el-table-column>
        <el-table-column
          label="备注"
          min-width="140"
          prop="note"
          class-name="pt-cell-1line"
          show-overflow-tooltip />
        <el-table-column label="启用" width="80">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              :aria-label="`启用识别词 ${row.pattern}`"
              :data-testid="`media-word-enabled-${row.id}`"
              @update:model-value="(v: string | number | boolean) => toggleWord(row, Boolean(v))" />
          </template>
        </el-table-column>
        <el-table-column label="" width="120" align="right">
          <template #default="{ row }">
            <el-button link :data-testid="`media-word-edit-${row.id}`" @click="openWord(row)"
              >编辑</el-button
            >
            <el-button
              link
              type="danger"
              :data-testid="`media-word-del-${row.id}`"
              @click="removeWord(row)"
              >删除</el-button
            >
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="md-cards">
        <PtRowCard v-for="r in wordList" :key="r.id">
          <template #title>{{ r.pattern }}</template>
          <template #meta>
            <span>{{ wordKindLabel(r.kind) }}{{ r.is_regex ? " · 正则" : "" }}</span>
            <span class="md-full">{{ wordEffect(r) }}</span>
            <span v-if="r.note" class="md-full">{{ r.note }}</span>
          </template>
          <template #status>
            <el-switch
              :model-value="r.enabled"
              :aria-label="`启用识别词 ${r.pattern}`"
              @update:model-value="(v: string | number | boolean) => toggleWord(r, Boolean(v))" />
          </template>
          <template #actions>
            <el-button size="small" @click="openWord(r)">编辑</el-button>
            <el-button size="small" type="danger" plain @click="removeWord(r)">删除</el-button>
          </template>
        </PtRowCard>
      </div>
    </PtPanel>

    <el-dialog
      v-model="wordDialog"
      class="pt-dialog"
      :title="editingID ? '编辑识别词' : '添加识别词'"
      width="520px"
      align-center>
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <el-form-item label="种类">
          <el-radio-group v-model="wordForm.kind" data-testid="media-word-kind">
            <el-radio-button v-for="k in MEDIA_WORD_KINDS" :key="k.value" :value="k.value">{{
              k.label
            }}</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="匹配" required>
          <el-input
            v-model="wordForm.pattern"
            placeholder="庆余年2"
            data-testid="media-word-pattern" />
          <el-checkbox v-model="wordForm.is_regex" data-testid="media-word-regex"
            >按正则匹配</el-checkbox
          >
        </el-form-item>
        <el-form-item v-if="wordForm.kind === 'replace'" label="替换为">
          <el-input
            v-model="wordForm.replacement"
            placeholder="庆余年 第二季（留空等于删除）"
            data-testid="media-word-replacement" />
        </el-form-item>
        <el-form-item v-if="wordForm.kind === 'offset'" label="集数偏移" required>
          <el-input-number
            v-model="wordForm.offset"
            :min="-9999"
            :max="9999"
            controls-position="right"
            data-testid="media-word-offset" />
          <div class="field-tip">比如第二季从第 13 集开始编号，填 -12 让 E13 变成 E01</div>
        </el-form-item>
        <el-form-item label="备注（可选）">
          <el-input v-model="wordForm.note" data-testid="media-word-note" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="wordForm.enabled" data-testid="media-word-enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="wordDialog = false">取消</el-button>
        <el-button
          type="primary"
          :loading="wordSaving"
          data-testid="media-word-save"
          @click="saveWord"
          >保存</el-button
        >
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.md-form {
  max-width: 720px;
}

.md-inline {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  width: 100%;
}

.md-inline > .el-input {
  flex: 1 1 240px;
}

.md-row {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 0 16px;
}

.md-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.md-tip {
  margin: 12px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.md-tip--top {
  margin: 0 0 12px;
}

.md-result {
  display: flex;
  flex-direction: column;
  gap: 16px;
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid var(--pt-border);
}

.md-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.md-label {
  font-size: 12px;
  color: var(--pt-t3);
}

.md-summary {
  font-size: 15px;
  font-weight: 600;
  color: var(--pt-t1);
}

.md-sub {
  font-size: 12px;
  font-weight: 400;
  color: var(--pt-t3);
}

.md-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.md-match {
  display: flex;
  gap: 14px;
  align-items: flex-start;
}

.md-poster {
  flex: 0 0 auto;
  width: 92px;
  border-radius: 6px;
  background: var(--pt-bg-surface-muted);
}

.md-poster--sm {
  width: 46px;
}

.md-match__body {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.md-match__title {
  font-size: 15px;
  font-weight: 600;
}

.md-match__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  align-items: center;
  font-size: 12px;
  color: var(--pt-t2);
}

.md-overview {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--pt-t2);
  display: -webkit-box;
  -webkit-line-clamp: 4;
  line-clamp: 4;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.md-cands {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.md-cand {
  display: flex;
  gap: 10px;
  align-items: center;
}

.md-cand__body {
  flex: 1 1 auto;
  min-width: 0;
}

.md-cand__body a {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  display: block;
}

.md-kind {
  width: 96px;
}

.md-regex {
  margin-left: 6px;
}

.md-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.md-full {
  flex-basis: 100%;
}
</style>
