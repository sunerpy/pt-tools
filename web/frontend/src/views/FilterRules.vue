<script setup lang="ts">
import {
  type FilterRule,
  filterRulesApi,
  type FilterRuleTestResponse,
  type RSSConfig,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref } from "vue";

const isMobile = useIsMobile();
const saving = ref(false);
const showDialog = ref(false);
const showTestDialog = ref(false);
const editMode = ref(false);
const loadingRss = ref(false);
/** 试跑用的 RSS 数据源列表没拿到。规则表本身不受影响，所以这是「部分失败」而不是失败 */
const rssFailed = ref(false);

const rules = ref<FilterRule[]>([]);
const rssList = ref<{ id: number; name: string; site_name: string }[]>([]);
const testResult = ref<FilterRuleTestResponse | null>(null);
const selectedRssId = ref<number | undefined>(undefined);

const TEST_ZERO_SUB = "没有种子命中这条规则，放宽模式或换个数据源";

/**
 * 规则表的六态状态机（设计文档 §5）。
 *
 * 以前这里只有一个 loading ref，加载失败弹个 toast 就完事 —— 两秒后 toast 消失，
 * 表格停在 empty 上，用户看到的是「一条规则都还没建」，于是又去建一条重复的。
 * 请求失败必须留在页面上，401/403 还要单独画成「无权访问」，否则用户会一直点重试。
 *
 * 这页不接 filtered：规则表没有搜索/筛选，0 行只可能是「一条都还没建」，永远是 empty。
 * partial 接的是次要数据源 —— 规则拿到了但试跑用的 RSS 列表没拿到。
 */
const { loading, state, errorText, run, hasPartialBanner } = useDataState({
  failed: () => (rssFailed.value ? 1 : 0),
});

/** 状态块的副标题：失败时给真实错误，空态时给下一步动作 */
const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  if (state.value === "partial") return "规则读到了，但试跑用的 RSS 数据源列表没读到";
  return "加一条规则，让 RSS 只下你要的资源";
});

/** perm 不给重试：没权限点重试没有意义，该去要权限 */
const stateAction = computed<"retry" | "add" | "none">(() => {
  if (state.value === "error" || state.value === "partial") return "retry";
  if (state.value === "loading" || state.value === "perm") return "none";
  return "add";
});

/**
 * 试跑命中列表也是一张列表，同样走六态。
 * 之前它失败时对话框根本不打开，只留一个 toast；现在先开对话框，把状态留在里面。
 * filtered 恒为 true：命中 0 条永远是「筛掉了」而不是「库里没数据」。
 */
const {
  loading: testing,
  state: testState,
  errorText: testErrorText,
  run: runTest,
} = useDataState({ filtered: () => true });

const testStateSub = computed(() => {
  if (testState.value === "error" || testState.value === "perm") return testErrorText.value;
  return TEST_ZERO_SUB;
});

const form = ref<FilterRule>({
  name: "",
  pattern: "",
  pattern_type: "keyword",
  match_field: "both",
  require_free: true,
  min_size_gb: 0,
  max_size_gb: 0,
  enabled: true,
  priority: 100,
  purpose: "download",
});

const testForm = ref({
  test_size_gb: 0,
  test_is_free: null as boolean | null,
  global_size: 0,
  filter_mode: "auto_free" as "auto_free" | "filter_only" | "free_only",
});

const filterModeOptions = [
  { value: "auto_free", label: "自动免费 + 过滤规则" },
  { value: "filter_only", label: "仅过滤规则匹配" },
  { value: "free_only", label: "仅免费种子" },
];

const patternTypes = [
  { value: "keyword", label: "关键词", tip: "大小写不敏感，匹配包含该关键词的标题" },
  { value: "wildcard", label: "通配符", tip: "使用 * 匹配任意字符，? 匹配单个字符" },
  { value: "regex", label: "正则表达式", tip: "使用正则表达式进行精确匹配" },
];

const matchFields = [
  { value: "title", label: "仅标题" },
  { value: "tag", label: "仅标签" },
  { value: "both", label: "标题和标签" },
];

const templates = [
  { name: "4K 资源", pattern: "4K|2160p|UHD", type: "regex" as const },
  { name: "1080p 资源", pattern: "1080p", type: "keyword" as const },
  { name: "REMUX 资源", pattern: "*REMUX*", type: "wildcard" as const },
  { name: "HDR 资源", pattern: "HDR|HDR10|Dolby Vision|DV", type: "regex" as const },
  { name: "HEVC/x265", pattern: "HEVC|x265|H.265", type: "regex" as const },
  { name: "国语配音", pattern: "国语|国配|中配", type: "regex" as const },
];

const currentPatternTip = computed(() => {
  return patternTypes.find((t) => t.value === form.value.pattern_type)?.tip || "";
});

onMounted(async () => {
  await reloadAll();
});

/** 刷新/重试都同时拉两个数据源，否则 partial 提示点了重试也消不掉 */
async function reloadAll() {
  await Promise.all([loadRules(), loadRssList()]);
}

async function loadRules() {
  const data = await run(() => filterRulesApi.list());
  if (!data) {
    // 失败时清空：留着上一次的规则再配一个「加载失败」的空态更让人误解
    rules.value = [];
    return;
  }
  rules.value = data;
}

async function loadRssList() {
  loadingRss.value = true;
  rssFailed.value = false;
  try {
    const response = await fetch("/api/sites");
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const sites = await response.json();
    const list: { id: number; name: string; site_name: string }[] = [];
    for (const [siteName, siteConfig] of Object.entries(sites)) {
      const config = siteConfig as { rss?: RSSConfig[] };
      if (config.rss) {
        for (const rss of config.rss) {
          if (rss.id) {
            list.push({ id: rss.id, name: rss.name, site_name: siteName });
          }
        }
      }
    }
    rssList.value = list;
  } catch (e: unknown) {
    // 次要数据源：规则表照常渲染，失败只记一个标记，由 partial 提示告诉用户少了什么
    rssFailed.value = true;
    rssList.value = [];
    console.error("加载 RSS 列表失败:", e);
  } finally {
    loadingRss.value = false;
  }
}

function openAddDialog() {
  editMode.value = false;
  form.value = {
    name: "",
    pattern: "",
    pattern_type: "keyword",
    match_field: "both",
    require_free: true,
    min_size_gb: 0,
    max_size_gb: 0,
    enabled: true,
    priority: 100,
    purpose: "download",
  };
  selectedRssId.value = undefined;
  showDialog.value = true;
}

function openEditDialog(rule: FilterRule) {
  editMode.value = true;
  form.value = { ...rule };
  selectedRssId.value = undefined;
  showDialog.value = true;
}

function applyTemplate(tpl: (typeof templates)[0]) {
  form.value.pattern = tpl.pattern;
  form.value.pattern_type = tpl.type;
  if (!form.value.name) {
    form.value.name = tpl.name;
  }
}

async function saveRule() {
  if (!form.value.name || !form.value.pattern) {
    ElMessage.error("名称和匹配模式为必填项");
    return;
  }

  saving.value = true;
  try {
    if (editMode.value && form.value.id) {
      await filterRulesApi.update(form.value.id, form.value);
      ElMessage.success("更新成功");
    } else {
      await filterRulesApi.create(form.value);
      ElMessage.success("创建成功");
    }
    showDialog.value = false;
    await loadRules();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

async function deleteRule(rule: FilterRule) {
  if (!rule.id) return;

  try {
    await ElMessageBox.confirm(`确定删除过滤规则 "${rule.name}"？`, "确认删除", {
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      type: "warning",
    });
    await filterRulesApi.delete(rule.id);
    ElMessage.success("已删除");
    await loadRules();
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "删除失败");
    }
  }
}

async function toggleEnabled(rule: FilterRule) {
  if (!rule.id) return;
  const newEnabled = !rule.enabled;
  try {
    await filterRulesApi.update(rule.id, { ...rule, enabled: newEnabled });
    rule.enabled = newEnabled;
    ElMessage.success("已保存");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  }
}

async function testPattern() {
  if (!form.value.pattern) {
    ElMessage.error("请先输入匹配模式");
    return;
  }

  testResult.value = null;
  // 先开对话框再发请求：加载中和失败的状态都要留在这块列表上，而不是只弹一个 toast
  showTestDialog.value = true;

  const data = await runTest(() =>
    filterRulesApi.test({
      pattern: form.value.pattern,
      pattern_type: form.value.pattern_type,
      match_field: form.value.match_field || "both",
      require_free: form.value.require_free,
      min_size_gb: form.value.min_size_gb || 0,
      max_size_gb: form.value.max_size_gb || 0,
      test_size_gb: testForm.value.test_size_gb,
      test_is_free: testForm.value.test_is_free,
      global_size: testForm.value.global_size,
      filter_mode: testForm.value.filter_mode,
      rss_id: selectedRssId.value,
      limit: 20,
    }),
  );
  if (!data) return;
  testResult.value = data;
}

function getPatternTypeLabel(type: string) {
  return patternTypes.find((t) => t.value === type)?.label || type;
}

function getMatchFieldLabel(field: string | undefined) {
  return matchFields.find((f) => f.value === field)?.label || "标题和标签";
}

function formatSizeRange(rule: FilterRule): string {
  if (!rule.min_size_gb && !rule.max_size_gb) return "不限";
  return `${rule.min_size_gb || 0} ~ ${rule.max_size_gb ? rule.max_size_gb : "∞"} GB`;
}

/** 命中一条种子后的最终动作：会下载 / 会跳过 / 只是匹配上了 */
function decisionText(decision: string | undefined): string {
  if (decision === "downloaded") return "会下载";
  if (decision === "skipped") return "会跳过";
  return "匹配成功";
}
</script>

<template>
  <div class="filter-rules-page">
    <div class="pt-note pt-note--warn">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <div class="note-body">
        <strong>过滤规则等于精准下载，不是「免费之外再多下一些」</strong>
        <p>
          没有关联过滤规则时，RSS 订阅默认自动下载免费种子，适合日常刷流。一旦给某个 RSS
          关联了规则，系统就认为你要精准下载：只有命中规则的种子会被推送，其余种子即使免费也会跳过。
        </p>
        <p>
          想要「规则命中的下、所有免费的也下」这种旧行为，把该 RSS 的下载模式留在
          <code>跟随全局</code>，并在全局设置里选 <code>仅免费（忽略过滤规则）</code>；或者干脆不给
          这个 RSS 关联规则。
        </p>
      </div>
    </div>

    <PtPanel
      v-loading="loading"
      title="过滤规则"
      icon="list-filter"
      :count="`${rules.length} 条`"
      padding="none">
      <template #actions>
        <el-button size="small" :loading="loading" @click="reloadAll">
          <PtIcon name="refresh-cw" :size="14" /><span>刷新</span>
        </el-button>
        <el-button type="primary" size="small" @click="openAddDialog">
          <PtIcon name="plus" :size="14" /><span>添加规则</span>
        </el-button>
      </template>

      <!--
        partial（§5）：规则读到了但试跑数据源没读到。有数据可看时不该用一整块状态图
        顶掉表格，那等于把已经拿到的规则也藏了，所以挂一条提示，表格照常渲染。
      -->
      <div v-if="hasPartialBanner(rules.length)" class="pt-note pt-note--warn partial-banner">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>
          试跑用的 RSS
          数据源列表没读到，规则本身不受影响；编辑弹窗里的「数据源」会是空的，点刷新可重试。
        </span>
      </div>

      <el-table v-if="!isMobile" :data="rules" class="pt-grid" style="width: 100%">
        <template #empty>
          <PtDataState :state="state" dense :sub="stateSub">
            <template v-if="stateAction !== 'none'" #action>
              <el-button v-if="stateAction === 'retry'" size="small" @click="reloadAll">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
              <el-button v-else size="small" type="primary" @click="openAddDialog">
                <PtIcon name="plus" :size="14" /><span>添加规则</span>
              </el-button>
            </template>
          </PtDataState>
        </template>

        <el-table-column label="名称" min-width="140" class-name="pt-cell-strong">
          <template #default="{ row }">{{ row.name }}</template>
        </el-table-column>

        <el-table-column label="匹配模式" min-width="200">
          <template #default="{ row }">
            <code class="pattern">{{ row.pattern }}</code>
          </template>
        </el-table-column>

        <el-table-column label="类型" width="96">
          <template #default="{ row }">
            <PtTag>{{ getPatternTypeLabel(row.pattern_type) }}</PtTag>
          </template>
        </el-table-column>

        <el-table-column label="匹配范围" width="110" class-name="pt-cell-muted">
          <template #default="{ row }">{{ getMatchFieldLabel(row.match_field) }}</template>
        </el-table-column>

        <el-table-column
          label="优先级"
          width="80"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">{{ row.priority }}</template>
        </el-table-column>

        <el-table-column label="仅免费" width="80">
          <template #default="{ row }">
            <PtStatusPill :tone="row.require_free ? 'ok' : 'neutral'" size="sm">
              {{ row.require_free ? "是" : "否" }}
            </PtStatusPill>
          </template>
        </el-table-column>

        <el-table-column label="大小范围" width="130" class-name="pt-cell-muted">
          <template #default="{ row }">{{ formatSizeRange(row) }}</template>
        </el-table-column>

        <el-table-column label="启用" width="70">
          <template #default="{ row }">
            <el-switch :model-value="row.enabled" size="small" @change="toggleEnabled(row)" />
          </template>
        </el-table-column>

        <el-table-column label="操作" width="120" fixed="right" class-name="pt-cell-act">
          <template #default="{ row }">
            <el-button link type="primary" size="small" @click="openEditDialog(row)">
              <PtIcon name="pencil" :size="14" /><span>编辑</span>
            </el-button>
            <el-button link type="danger" size="small" @click="deleteRule(row)">
              <PtIcon name="trash-2" :size="14" /><span>删除</span>
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!--
        移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
        这张表桌面有 9 列，手机上横着滚既看不到列头又和页面纵向滚动打架。
        卡上留的是真正要看的：名称 + 类型/范围/大小/优先级 + 匹配模式 + 启用状态 + 三个操作。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!rules.length" :state="state" :sub="stateSub">
          <template v-if="stateAction !== 'none'" #action>
            <el-button v-if="stateAction === 'retry'" size="small" @click="reloadAll">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
            <el-button v-else size="small" type="primary" @click="openAddDialog">
              <PtIcon name="plus" :size="14" /><span>添加规则</span>
            </el-button>
          </template>
        </PtDataState>

        <PtRowCard v-for="rule in rules" :key="rule.id">
          <template #title>{{ rule.name }}</template>

          <template #meta>
            <PtTag>{{ getPatternTypeLabel(rule.pattern_type) }}</PtTag>
            <span>{{ getMatchFieldLabel(rule.match_field) }}</span>
            <span>{{ formatSizeRange(rule) }}</span>
            <span>优先级 {{ rule.priority }}</span>
            <!-- 仅免费只在开着的时候出现：关掉是默认值，画个「否」的胶囊只是噪声 -->
            <PtStatusPill v-if="rule.require_free" tone="ok" size="sm">仅免费</PtStatusPill>
            <code class="pattern pattern--row">{{ rule.pattern }}</code>
          </template>

          <template #status>
            <PtStatusPill :tone="rule.enabled ? 'ok' : 'neutral'" size="sm">
              {{ rule.enabled ? "已启用" : "已停用" }}
            </PtStatusPill>
          </template>

          <!-- 桌面那个 el-switch 只有 20 高，够不到 44 触控；行卡上换成按钮 -->
          <template #actions>
            <el-button size="small" @click="toggleEnabled(rule)">
              <PtIcon :name="rule.enabled ? 'pause' : 'play'" :size="14" />
              <span>{{ rule.enabled ? "停用" : "启用" }}</span>
            </el-button>
            <el-button size="small" @click="openEditDialog(rule)">
              <PtIcon name="pencil" :size="14" /><span>编辑</span>
            </el-button>
            <el-button size="small" type="danger" plain @click="deleteRule(rule)">
              <PtIcon name="trash-2" :size="14" /><span>删除</span>
            </el-button>
          </template>
        </PtRowCard>
      </div>

      <template v-if="rules.length > 0" #footer>
        <span class="pt-foot-note">优先级数字越小越先匹配，命中即停</span>
      </template>
    </PtPanel>

    <el-dialog
      v-model="showDialog"
      class="pt-dialog"
      :title="editMode ? '编辑过滤规则' : '添加过滤规则'"
      width="640px"
      align-center>
      <el-form :model="form" class="pt-form" label-position="top">
        <div class="field-head">规则</div>

        <div class="field-row">
          <el-form-item label="名称" required>
            <el-input v-model="form.name" placeholder="例如 4K 电影" />
          </el-form-item>

          <el-form-item label="优先级">
            <el-input-number v-model="form.priority" :min="1" :max="9999" style="width: 100%" />
            <div class="field-tip">越小越先匹配，默认 100</div>
          </el-form-item>
        </div>

        <div class="field-row">
          <el-form-item label="模式类型" required>
            <el-select v-model="form.pattern_type" style="width: 100%">
              <el-option
                v-for="t in patternTypes"
                :key="t.value"
                :label="t.label"
                :value="t.value" />
            </el-select>
            <div class="field-tip">{{ currentPatternTip }}</div>
          </el-form-item>

          <el-form-item label="匹配范围">
            <el-select v-model="form.match_field" style="width: 100%">
              <el-option
                v-for="f in matchFields"
                :key="f.value"
                :label="f.label"
                :value="f.value" />
            </el-select>
            <div class="field-tip">从标题、标签还是两者里找</div>
          </el-form-item>
        </div>

        <el-form-item label="匹配模式" required>
          <el-input v-model="form.pattern" type="textarea" :rows="2" placeholder="输入匹配模式" />
        </el-form-item>

        <el-form-item label="常用模板">
          <div class="tpl-list">
            <button
              v-for="tpl in templates"
              :key="tpl.name"
              type="button"
              class="tpl"
              @click="applyTemplate(tpl)">
              {{ tpl.name }}
            </button>
          </div>
          <div class="field-tip">点一下会填好匹配模式和模式类型，名称为空时也一并带上</div>
        </el-form-item>

        <div class="field-row">
          <el-form-item label="最小大小（GB）">
            <el-input-number v-model="form.min_size_gb" :min="0" :max="99999" style="width: 100%" />
            <div class="field-tip">小于该值不通过，0 表示不限</div>
          </el-form-item>

          <el-form-item label="最大大小（GB）">
            <el-input-number v-model="form.max_size_gb" :min="0" :max="99999" style="width: 100%" />
            <div class="field-tip">大于该值不通过，0 表示不限；不能突破全局上限</div>
          </el-form-item>
        </div>

        <el-form-item label="规则用途" prop="purpose">
          <el-select v-model="form.purpose" style="width: 100%" placeholder="选择用途">
            <el-option label="下载 —— 控制是否推送到下载器" value="download" />
            <el-option label="通知 —— 控制是否触发上新推送" value="notify" />
            <el-option label="两者 —— 同时控制下载与通知" value="both" />
          </el-select>
        </el-form-item>

        <div class="field-row">
          <el-form-item label="仅免费">
            <el-switch v-model="form.require_free" />
            <div class="field-tip">开启后只下免费种子</div>
          </el-form-item>

          <el-form-item label="启用规则">
            <el-switch v-model="form.enabled" />
            <div class="field-tip">关掉后规则保留但不参与匹配</div>
          </el-form-item>
        </div>

        <div class="field-head">试跑（下面几项只影响这次测试，不会保存）</div>

        <el-form-item label="数据源">
          <el-select
            v-model="selectedRssId"
            placeholder="不选则用历史记录"
            clearable
            style="width: 100%"
            :loading="loadingRss">
            <el-option
              v-for="rss in rssList"
              :key="rss.id"
              :label="`${rss.name}（${rss.site_name}）`"
              :value="rss.id" />
          </el-select>
          <div v-if="rssFailed" class="field-tip field-tip--warn">
            数据源列表没读到，这里会是空的；先不选也能用历史记录试跑
          </div>
          <div v-else class="field-tip">选一个 RSS 会现拉一次实时数据来试</div>
        </el-form-item>

        <div class="field-row">
          <el-form-item label="模拟大小（GB）">
            <el-input-number
              v-model="testForm.test_size_gb"
              :min="0"
              :step="0.5"
              :precision="2"
              style="width: 100%" />
            <div class="field-tip">0 表示用种子真实大小</div>
          </el-form-item>

          <el-form-item label="模拟全局上限（GB）">
            <el-input-number
              v-model="testForm.global_size"
              :min="0"
              :max="99999"
              style="width: 100%" />
            <div class="field-tip">0 表示无上限</div>
          </el-form-item>
        </div>

        <el-form-item label="模拟免费状态">
          <el-radio-group v-model="testForm.test_is_free">
            <el-radio :value="null">用真实值</el-radio>
            <el-radio :value="true">免费</el-radio>
            <el-radio :value="false">非免费</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="下载模式">
          <el-select v-model="testForm.filter_mode" style="width: 100%">
            <el-option
              v-for="m in filterModeOptions"
              :key="m.value"
              :label="m.label"
              :value="m.value" />
          </el-select>
        </el-form-item>

        <el-form-item>
          <el-button :loading="testing" @click="testPattern">
            <PtIcon name="target" :size="14" /><span>测试匹配</span>
          </el-button>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveRule">
          {{ editMode ? "保存" : "添加" }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="showTestDialog"
      class="pt-dialog"
      title="匹配测试结果"
      width="760px"
      align-center>
      <div v-if="testResult">
        <div class="pt-note" :class="testResult.match_count > 0 ? 'pt-note--ok' : 'pt-note--warn'">
          <PtIcon
            :name="testResult.match_count > 0 ? 'circle-check' : 'triangle-alert'"
            :size="14"
            class="pt-note__icon" />
          <span>
            共测试 {{ testResult.total_count }} 条记录，命中
            <strong>{{ testResult.match_count }}</strong> 条
          </span>
        </div>

        <div class="scope-row">
          <span class="scope-row__label">匹配范围</span>
          <el-segmented v-model="form.match_field" class="pt-seg" :options="matchFields" />
          <span class="scope-row__tip">改完点「重新测试」</span>
        </div>

        <div v-if="testResult.matches && testResult.matches.length > 0" class="mlist">
          <article v-for="(match, idx) in testResult.matches" :key="idx" class="mrow">
            <header class="mrow__head">
              <span class="mrow__idx">#{{ idx + 1 }}</span>
              <PtStatusPill :tone="match.decision === 'downloaded' ? 'ok' : 'dang'" size="sm">
                {{ decisionText(match.decision) }}
              </PtStatusPill>
              <PtStatusPill :tone="match.is_free ? 'ok' : 'neutral'" size="sm">
                {{ match.is_free ? "免费" : "非免费" }}
              </PtStatusPill>
              <PtTag v-if="match.source === 'filter_rule'">过滤规则通道</PtTag>
              <PtTag v-else-if="match.source === 'free_download'">免费通道</PtTag>
              <span v-if="match.size_gb" class="mrow__size">
                {{ match.size_gb.toFixed(2) }} GB
              </span>
            </header>

            <p class="mrow__title">{{ match.title }}</p>
            <p v-if="match.tag" class="mrow__meta"><span>标签</span>{{ match.tag }}</p>
            <p v-if="match.reason" class="mrow__meta"><span>原因</span>{{ match.reason }}</p>
          </article>
        </div>
        <PtDataState v-else state="zero" dense :sub="TEST_ZERO_SUB" />
      </div>

      <!--
        命中列表的 loading / error / perm（§5）。以前失败时这个对话框根本不打开，
        只留一个两秒就没的 toast，用户不知道是没命中还是请求挂了。
      -->
      <PtDataState v-else :state="testState" :sub="testStateSub">
        <template v-if="testState === 'error'" #action>
          <el-button size="small" @click="testPattern">
            <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
          </el-button>
        </template>
      </PtDataState>

      <template #footer>
        <el-button @click="showTestDialog = false">关闭</el-button>
        <el-button type="primary" :loading="testing" @click="testPattern">
          <PtIcon name="refresh-cw" :size="14" /><span>重新测试</span>
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.filter-rules-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
}

/* 说明块里的多段正文：.pt-note 只管容器，段落间距归页面 */
.note-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.note-body strong {
  color: var(--pt-t1);
}

.note-body p {
  margin: 0;
}

.pattern {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t2);
  word-break: break-all;
}

/* 匹配模式是规则的正文，挤在 meta 那排小胶囊里根本读不了，让它独占一行 */
.pattern--row {
  flex: 1 1 100%;
}

/* 部分失败提示：面板 padding="none"，留白由这里给 */
.partial-banner {
  margin: var(--pt-space-3) var(--pt-space-3) 0;
}

/* 移动端行卡列表：同上，面板贴边，所以留白归页面 */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-space-3);
}
</style>

<style>
/* 两个对话框都 teleport 到 body，scoped 到不了；这些类只在本页的对话框里出现 */

/* 模板胶囊用 button 而不是 el-tag：它是可点的动作，键盘要能聚焦 */
.pt-dialog .tpl-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
}

.pt-dialog .tpl {
  padding: 2px 9px;
  font-family: inherit;
  font-size: var(--pt-fz-label);
  color: var(--pt-t2);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-sm);
  cursor: pointer;
  transition:
    color var(--pt-transition-fast),
    border-color var(--pt-transition-fast);
}

.pt-dialog .tpl:hover {
  color: var(--pt-p);
  border-color: var(--pt-p);
}

/* 数据源没读到时，说明文字要看得出是个警告，不能和普通提示一个色 */
.pt-dialog .field-tip.field-tip--warn {
  color: var(--pt-warn);
}

.pt-dialog .scope-row {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  margin: var(--pt-space-4) 0 var(--pt-space-3);
}

.pt-dialog .scope-row__label {
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-t2);
}

.pt-dialog .scope-row__tip {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 命中列表自己滚：20 条结果不该把对话框顶到屏幕外 */
.pt-dialog .mlist {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  max-height: 48vh;
  overflow-y: auto;
}

.pt-dialog .mrow {
  padding: var(--pt-space-3);
  background: var(--pt-canvas);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.pt-dialog .mrow__head {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  margin-bottom: 6px;
}

.pt-dialog .mrow__idx {
  font-size: var(--pt-fz-label);
  font-variant-numeric: tabular-nums;
  color: var(--pt-t4);
}

.pt-dialog .mrow__size {
  margin-left: auto;
  font-size: var(--pt-fz-label);
  font-variant-numeric: tabular-nums;
  color: var(--pt-t3);
}

.pt-dialog .mrow__title {
  margin: 0;
  font-weight: 500;
  color: var(--pt-t1);
  word-break: break-all;
}

.pt-dialog .mrow__meta {
  display: flex;
  gap: 6px;
  margin: 3px 0 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.pt-dialog .mrow__meta > span {
  flex: 0 0 auto;
  color: var(--pt-t4);
}
</style>
