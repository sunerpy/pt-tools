<script setup lang="ts">
import { type SupportedSiteDefinition, sitesApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import SiteAvatar from "@/components/SiteAvatar.vue";
import PtBreakdown, { type BreakdownRow } from "@/components/ui/PtBreakdown.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { computed, onMounted, ref } from "vue";

/* ≤768 时外壳隐藏页头，页头动作得原地落回页面（Teleport 的 disabled） */
const isMobile = useIsMobile();

/** 六态（设计文档 §5）：以前加载失败只弹一个 toast，列表随后画成「还没有数据」 */
const { loading, state, errorText, run } = useDataState({
  filtered: () => Boolean(search.value.trim() || schemaFilter.value || addedFilter.value),
});
const definitions = ref<SupportedSiteDefinition[]>([]);
const search = ref("");
const schemaFilter = ref("");

/*
 * 画板 14 的 chip-0「已添加: 全部」与右端两枚视图钮（bi-layout-grid / bi-rows-3）。
 *
 * 「已添加」= 这个定义在 /api/sites 里已经启用 —— 这一页是「产品支持哪些站点」的清单，
 * 用户最常问的就是「哪些我已经在用了」。所以要拿一次站点配置来比对。
 */
const addedFilter = ref<"" | "yes" | "no">("");
const enabledNames = ref<Set<string>>(new Set());

/** 画板右端的两枚视图钮：卡片（layout-grid）/ 紧凑列表（rows-3） */
const VIEW_KEY = "pt-tools-supported-view-v1";

function loadView(): "grid" | "rows" {
  try {
    return window.localStorage.getItem(VIEW_KEY) === "rows" ? "rows" : "grid";
  } catch {
    return "grid";
  }
}

const viewMode = ref<"grid" | "rows">(loadView());

function setView(v: "grid" | "rows") {
  viewMode.value = v;
  try {
    window.localStorage.setItem(VIEW_KEY, v);
  } catch {
    /* 存不下就只在本次会话里生效 */
  }
}

const isAdded = (id: string, name: string) =>
  enabledNames.value.has(name) || enabledNames.value.has(id);

onMounted(async () => {
  await Promise.all([loadDefinitions(), loadEnabled()]);
});

/**
 * 读一次站点配置，只为算「已添加」。
 *
 * 失败不影响这一页的主体（定义清单）—— 那一列会显示「未知」，筛选选项也不会骗人：
 * 拿不到配置时把「已添加」筛选留空，而不是把所有站点当成没添加。
 */
async function loadEnabled() {
  try {
    const sites = await sitesApi.list();
    const names = new Set<string>();
    for (const [name, cfg] of Object.entries(sites)) {
      if (cfg?.enabled) names.add(name);
    }
    enabledNames.value = names;
  } catch {
    enabledNames.value = new Set();
  }
}

async function loadDefinitions() {
  const data = await run(() => sitesApi.listDefinitions());
  if (!data) {
    // 失败时清空：留着旧列表配一个「加载失败」更让人误解
    definitions.value = [];
    return;
  }
  definitions.value = data.slice().sort((a, b) => a.name.localeCompare(b.name, "zh-Hans-CN"));
}

/** 画板 bar-64 的架构分段：全部 + 每种架构（带该架构的定义数） */
const schemaSeg = computed(() => [
  { label: `全部 ${definitions.value.length}`, value: "" },
  ...schemaOptions.value.map((o) => ({ label: `${o.schema} ${o.count}`, value: o.schema })),
]);

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
    if (addedFilter.value === "yes" && !isAdded(d.id, d.name)) return false;
    if (addedFilter.value === "no" && isAdded(d.id, d.name)) return false;
    if (!q) return true;
    if (d.name.toLowerCase().includes(q)) return true;
    if (d.id.toLowerCase().includes(q)) return true;
    if (d.description?.toLowerCase().includes(q)) return true;
    if (d.aka?.some((a) => a.toLowerCase().includes(q))) return true;
    if (d.urls.some((u) => u.toLowerCase().includes(q))) return true;
    return false;
  });
});

/** 状态块副标题：失败给真实错误，空/零态给下一步动作 */
const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  if (state.value === "loading") return "正在读取站点定义";
  return "换个关键词，或清空架构筛选";
});

const totalCount = computed(() => definitions.value.length);
const filteredCount = computed(() => filtered.value.length);
const unavailableCount = computed(() => definitions.value.filter((d) => d.unavailable).length);

/**
 * 画板 p-cap 1080「内置能力」：这批站点定义覆盖了哪些架构、哪些认证方式、有多少启用 H&R。
 * 全部由已经拿到的 definitions 现算，不额外请求。
 */
const capSchemaRows = computed<BreakdownRow[]>(() => {
  const buckets = new Map<string, number>();
  for (const d of definitions.value) {
    const name = d.schema || "未标注";
    buckets.set(name, (buckets.get(name) ?? 0) + 1);
  }
  return [...buckets.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, n]) => ({
      key: `schema-${name}`,
      label: name,
      value: n,
      tone: "primary" as const,
    }));
});

const capAuthRows = computed<BreakdownRow[]>(() => {
  const buckets = new Map<string, number>();
  for (const d of definitions.value) {
    const name = d.authMethod || "未标注";
    buckets.set(name, (buckets.get(name) ?? 0) + 1);
  }
  return [...buckets.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, n]) => ({ key: `auth-${name}`, label: name, value: n, tone: "info" as const }));
});

const capFlagRows = computed<BreakdownRow[]>(() => {
  const total = definitions.value.length;
  const hr = definitions.value.filter((d) => d.hrEnabled).length;
  const aka = definitions.value.filter((d) => (d.aka?.length ?? 0) > 0).length;
  return [
    { key: "hr", label: "标注了 H&R", value: hr, tone: "warn" as const },
    { key: "aka", label: "有别名", value: aka, tone: "mute" as const },
    {
      key: "unavailable",
      label: "临时不可用",
      value: unavailableCount.value,
      tone: "dang" as const,
    },
    { key: "ok", label: "可用", value: total - unavailableCount.value, tone: "ok" as const },
  ];
});

/** 画板 head 的 sub（11.5/400 t3）：内置了多少站点定义，其中几个临时不可用 */
const headSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return "站点定义没加载出来";
  if (totalCount.value === 0) return "";
  const parts = [`内置 ${totalCount.value} 个站点定义`];
  if (unavailableCount.value > 0) parts.push(`${unavailableCount.value} 个临时不可用`);
  return parts.join(" · ");
});

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

/** 认证方式的语义色：cookie 是最常见的基线，走 primary；组合认证配置成本最高，走 warn */
function authMethodTone(m?: string): "primary" | "ok" | "warn" | "info" {
  switch (m) {
    case "cookie":
      return "primary";
    case "api_key":
      return "ok";
    case "cookie_and_api_key":
      return "warn";
    default:
      return "info";
  }
}

function clearFilters() {
  search.value = "";
  schemaFilter.value = "";
  addedFilter.value = "";
}
</script>

<template>
  <!--
    画板 14：head 64 → bar-64（40）→ 四张卡（548 / 516 两栏）。这页没有表格带，
    工具栏带是筛选条，卡片是站点定义卡。
  -->
  <div class="supported-sites-page">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button :loading="loading" @click="loadDefinitions">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
      <el-button type="primary" @click="$router.push('/sites')">
        <PtIcon name="settings" :size="15" /><span>站点管理</span>
      </el-button>
    </Teleport>

    <PtToolbar band>
      <el-input
        v-model="search"
        placeholder="搜索：名称 / ID / 别名 / 域名"
        clearable
        class="filter-input">
        <template #prefix>
          <PtIcon name="search" :size="14" />
        </template>
      </el-input>
      <!-- 画板 14 的 chip-0「已添加: 全部」 -->
      <el-select
        v-model="addedFilter"
        class="filter-select"
        size="small"
        placeholder="已添加: 全部"
        clearable
        data-testid="supported-added-filter">
        <el-option label="已添加: 全部" value="" />
        <el-option label="已添加: 是" value="yes" />
        <el-option label="已添加: 否" value="no" />
      </el-select>

      <!--
        画板 14 的 bar-64 上架构筛选画的是**分段器**，不是下拉。
        这一页适合照画板来：架构本来就是单选，档位数固定（内置定义一共五种架构），
        摊开摆着还顺手把分布量出来了 —— 下拉得点开才知道有几种。
      -->
      <el-segmented
        v-model="schemaFilter"
        class="pt-seg ss-seg"
        :options="schemaSeg"
        :props="{ label: 'label', value: 'value' }"
        data-testid="supported-schema-seg" />
      <el-button v-if="search || schemaFilter || addedFilter" @click="clearFilters">
        <PtIcon name="x" :size="14" /><span>清空筛选</span>
      </el-button>

      <template #right>
        <!--
          画板 14 的 bar-64 右端两枚视图钮：bi-layout-grid（卡片）/ bi-rows-3（紧凑列表）。
          六十多个定义时卡片要滚很久，紧凑列表一屏能看完 —— 偏好存本地。
        -->
        <el-tooltip content="卡片视图" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            :class="{ 'is-active': viewMode === 'grid' }"
            aria-label="卡片视图"
            :aria-pressed="viewMode === 'grid'"
            data-testid="supported-view-grid"
            @click="setView('grid')">
            <PtIcon name="layout-grid" :size="15" />
          </button>
        </el-tooltip>
        <el-tooltip content="紧凑列表" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            :class="{ 'is-active': viewMode === 'rows' }"
            aria-label="紧凑列表"
            :aria-pressed="viewMode === 'rows'"
            data-testid="supported-view-rows"
            @click="setView('rows')">
            <PtIcon name="rows-3" :size="15" />
          </button>
        </el-tooltip>
      </template>

      <!--
        「内置多少个 / 几个不可用」已经在页头摘要里，带上只说筛选后还剩多少 ——
        两份一样的话叠在一起，40 高的带会被文字顶到换行（实测 65 高）。
      -->
      <template #note>筛选后 {{ filteredCount }} 个</template>
    </PtToolbar>

    <PtDataState v-if="filtered.length === 0" class="sites-state" :state="state" :sub="stateSub">
      <template v-if="state === 'error'" #action>
        <el-button @click="loadDefinitions">
          <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
        </el-button>
      </template>
      <template v-else-if="state === 'zero'" #action>
        <el-button @click="clearFilters">
          <PtIcon name="x" :size="14" /><span>清空筛选</span>
        </el-button>
      </template>
    </PtDataState>

    <!--
      紧凑列表就是把两栏收成一栏、卡内边距收一档（画板 bi-rows-3 那个视图）。
      六十多个定义时卡片视图要滚很久 —— 这两枚钮是画板画的，不是我加的花样。
    -->
    <div
      v-else
      class="site-grid pt-cards"
      :class="viewMode === 'rows' ? 'pt-cards--wide is-rows' : 'pt-cards--2'">
      <article
        v-for="def in filtered"
        :key="def.id"
        class="site-card"
        :class="{ 'is-unavailable': def.unavailable }">
        <header class="site-card__head">
          <SiteAvatar :site-id="def.id" :site-name="def.name" :size="34" :no-fetch="true" />
          <div class="site-card__ident">
            <div class="site-card__name">
              <span>{{ def.name }}</span>
              <PtStatusPill v-if="def.unavailable" tone="dang" size="sm">不可用</PtStatusPill>
            </div>
            <div v-if="def.aka && def.aka.length > 0" class="site-card__aka">
              {{ def.aka.join(" / ") }}
            </div>
          </div>
        </header>

        <p v-if="def.description" class="site-card__desc">{{ def.description }}</p>

        <div v-if="def.unavailable && def.unavailableReason" class="pt-note pt-note--dang">
          <PtIcon name="circle-alert" :size="14" class="pt-note__icon" />
          <span>{{ def.unavailableReason }}</span>
        </div>

        <div class="site-card__tags">
          <PtTag>{{ def.schema }}</PtTag>
          <!--
            没声明认证方式的定义不画这个胶囊。`authMethod` 在后端是 omitempty，
            实测 /api/sites/definitions 返回的多数条目根本没有这个字段 ——
            照旧渲染的话每张卡上都会多一个写着「-」的胶囊，纯噪声。
            这一层的口径与「内置能力」卡一致：那里把它们记成「未标注 N」。
          -->
          <PtStatusPill v-if="def.authMethod" :tone="authMethodTone(def.authMethod)" size="sm">
            {{ authMethodLabel(def.authMethod) }}
          </PtStatusPill>
          <PtStatusPill v-if="def.hrEnabled" tone="warn" size="sm">H&amp;R</PtStatusPill>
        </div>

        <footer v-if="def.urls.length > 0" class="site-card__urls">
          <a
            v-for="url in def.urls"
            :key="url"
            :href="url"
            target="_blank"
            rel="noopener noreferrer">
            <PtIcon name="external-link" :size="12" />
            <span>{{ url.replace(/^https?:\/\//, "").replace(/\/$/, "") }}</span>
          </a>
        </footer>
      </article>

      <!-- 画板 p-cap 1080「内置能力」：架构 / 认证方式 / 标注三组分布 -->
      <PtPanel
        class="pt-cards__full"
        title="内置能力"
        icon="shield"
        :count="`${totalCount} 个定义`">
        <div class="cap">
          <section class="cap__col">
            <h3 class="cap__t">站点架构</h3>
            <PtBreakdown :rows="capSchemaRows" :total="totalCount" />
          </section>
          <section class="cap__col">
            <h3 class="cap__t">认证方式</h3>
            <PtBreakdown :rows="capAuthRows" :total="totalCount" />
          </section>
          <section class="cap__col">
            <h3 class="cap__t">标注</h3>
            <PtBreakdown :rows="capFlagRows" :total="totalCount" />
          </section>
        </div>
        <p class="cap__foot">
          口径是「内置了多少站点定义」，不是「你启用了几个」—— 启用情况在站点列表。
        </p>
      </PtPanel>
    </div>
  </div>
</template>

<style scoped>
/* 内置能力卡：三组分布横着铺，窄屏退回单列 */
.cap {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: var(--pt-pad);
}

.cap__t {
  margin: 0 0 var(--pt-space-2);
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t3);
}

.cap__foot {
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t4);
}

/* 带与卡片层各自管留白，这一层只负责纵向堆叠 */
.supported-sites-page {
  display: flex;
  flex-direction: column;
}

.filter-input {
  flex: 1 1 240px;
  max-width: 320px;
}

/* 架构分段：档位多（全部 + 五种架构），字号压小一档才不把工具栏挤换行 */
.ss-seg :deep(.el-segmented__item-label) {
  font-size: var(--pt-fz-xs);
}

.filter-select {
  width: 180px;
}

/* 紧凑列表：一栏 + 卡内边距收一档，一屏能看完更多定义 */
.site-grid.is-rows :deep(.pt-panel__body),
.site-grid.is-rows :deep(.def) {
  padding-block: var(--pt-space-2);
}

.site-grid.is-rows :deep(.def__desc) {
  display: none;
}

/* 栏宽与间隔由 .pt-cards--2 给（画板 548 / 516 两栏，1181 以下退回单栏） */

/* 空态不在卡里，自己内缩 16 对齐卡片层 */
.sites-state {
  padding: var(--pt-pad);
}

.site-card {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-3);
  padding: var(--pt-pad);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
  box-shadow: var(--pt-shadow-sm);
  transition:
    border-color var(--pt-transition-fast),
    box-shadow var(--pt-transition-fast);
}

.site-card:hover {
  border-color: var(--pt-p);
  box-shadow: var(--pt-shadow-md);
}

/* 不可用站点压暗而不是隐藏：用户需要知道它存在、以及为什么用不了 */
.site-card.is-unavailable {
  opacity: 0.66;
}

.site-card__head {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
}

.site-card__ident {
  min-width: 0;
}

.site-card__name {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: var(--pt-fz-h2);
  font-weight: 600;
  color: var(--pt-t1);
}

.site-card__name > span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.site-card__aka {
  margin-top: 1px;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 简介限两行：卡片高度参差不齐时网格会看起来是碎的 */
.site-card__desc {
  display: -webkit-box;
  margin: 0;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  line-height: var(--pt-lh-body);
  color: var(--pt-t3);
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.site-card__tags {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2);
}

/* URL 组贴卡片底部，卡片再高也对齐 */
.site-card__urls {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: auto;
  padding-top: var(--pt-space-2);
  border-top: 1px solid var(--pt-border);
}

.site-card__urls a {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-p);
  text-decoration: none;
  word-break: break-all;
}

.site-card__urls a:hover {
  text-decoration: underline;
}

@media (max-width: 768px) {
  .filter-input,
  .filter-select {
    max-width: none;
    width: 100%;
  }
}
</style>
