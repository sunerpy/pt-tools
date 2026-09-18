<script setup lang="ts">
import { type SupportedSiteDefinition, sitesApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import SiteAvatar from "@/components/SiteAvatar.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
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
  filtered: () => Boolean(search.value.trim() || schemaFilter.value),
});
const definitions = ref<SupportedSiteDefinition[]>([]);
const search = ref("");
const schemaFilter = ref("");

onMounted(async () => {
  await loadDefinitions();
});

async function loadDefinitions() {
  const data = await run(() => sitesApi.listDefinitions());
  if (!data) {
    // 失败时清空：留着旧列表配一个「加载失败」更让人误解
    definitions.value = [];
    return;
  }
  definitions.value = data.slice().sort((a, b) => a.name.localeCompare(b.name, "zh-Hans-CN"));
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

/** 状态块副标题：失败给真实错误，空/零态给下一步动作 */
const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  if (state.value === "loading") return "正在读取站点定义";
  return "换个关键词，或清空架构筛选";
});

const totalCount = computed(() => definitions.value.length);
const filteredCount = computed(() => filtered.value.length);
const unavailableCount = computed(() => definitions.value.filter((d) => d.unavailable).length);

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
}
</script>

<template>
  <!--
    画板 14：head 64 → bar-64（40）→ 四张卡（548 / 516 两栏）。这页没有表格带，
    工具栏带是筛选条，卡片是站点定义卡。
  -->
  <div class="supported-sites-page">
    <Teleport to="#pt-head-sub">{{ headSub }}</Teleport>
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
      <el-select v-model="schemaFilter" placeholder="按架构筛选" clearable class="filter-select">
        <el-option
          v-for="opt in schemaOptions"
          :key="opt.schema"
          :label="`${opt.schema} (${opt.count})`"
          :value="opt.schema" />
      </el-select>
      <el-button v-if="search || schemaFilter" @click="clearFilters">
        <PtIcon name="x" :size="14" /><span>清空筛选</span>
      </el-button>

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

    <div v-else class="site-grid pt-cards pt-cards--2">
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
          <PtStatusPill :tone="authMethodTone(def.authMethod)" size="sm">
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
    </div>
  </div>
</template>

<style scoped>
/* 带与卡片层各自管留白，这一层只负责纵向堆叠 */
.supported-sites-page {
  display: flex;
  flex-direction: column;
}

.filter-input {
  flex: 1 1 240px;
  max-width: 320px;
}

.filter-select {
  width: 180px;
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
