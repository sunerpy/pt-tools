<script setup lang="ts">
/*
 * NAS 监控（首页 NAS 概览 H2，方案 3.2.5）：本机来源与接入的来源各一行（类型、名字、地址、读到的主机、
 * 状态与原因、「重新连接」），添加、编辑、删除来源，选首页的 NAS 概览显示哪个来源。
 * 列表来自 GET /api/nas（只读 pt-tools 的内存，不对外发请求），开着这页时跟着采集器的一拍再取（useNasView）。
 * 没有能接入的类型时不显示「添加来源」。
 * 画板没有这一页，沿用远程访问页的样式：上面是来源列表（桌面表格、手机行卡），下面是首页显示与说明。
 */
import { type NasSource, nasApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import NasSourceDialog from "@/components/nas/NasSourceDialog.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { useNasView } from "@/composables/useNasHome";
import { formatShortDateTime } from "@/utils/format";
import {
  NAS_LOCAL_ID,
  nasHomeOptions,
  nasKindIcon,
  nasKindLabel,
  nasProblem,
  nasRetryText,
  nasSourceSummary,
  nasStateView,
  nasUpdatedText,
  nasUsable,
} from "@/utils/nas";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, ref } from "vue";

const isMobile = useIsMobile();
const nas = useNasView();
const view = nas.view;

const kinds = computed(() => view.value?.kinds ?? []);
const sources = computed(() => view.value?.sources ?? []);
const canAdd = computed(() => kinds.value.length > 0);

/** 页头的一句：能接入哪些（类型表里的名字） */
const headText = computed(() => {
  const names = kinds.value.map((k) => k.label);
  return names.length
    ? `接入 ${names.join("、")} 的监控接口，首页看 CPU、内存、网络、存储池、硬盘温度与告警`
    : "读 pt-tools 所在主机的 CPU、内存与负载，显示在首页的 NAS 概览里";
});

function isLocal(s: NasSource): boolean {
  return s.id === NAS_LOCAL_ID;
}

function kindText(s: NasSource): string {
  return nasKindLabel(s.kind, kinds.value);
}

/** 「读到的主机」按 · 分成几段，放得下时一段不从中间折行（版本号与型号里的 - 不当成断点） */
function summaryParts(s: NasSource): string[] {
  return nasSourceSummary(s).split(" · ");
}

function addressText(s: NasSource): string {
  return isLocal(s) ? "pt-tools 所在的主机" : (s.url ?? "");
}

function tagsOf(s: NasSource): { text: string; tone: "info" | "warn" }[] {
  const out: { text: string; tone: "info" | "warn" }[] = [];
  if (s.experimental) out.push({ text: "实验性", tone: "info" });
  if (/^http:\/\//i.test(s.url ?? "")) out.push({ text: "明文连接", tone: "warn" });
  return out;
}

/** 状态下面那句：原因、下次重试（这页里的说明，不写「到 NAS 监控里」） */
function stateDetail(s: NasSource): string {
  const now = nas.now.value;
  const why = s.message?.trim() ?? "";
  switch (s.state) {
    case "ok":
      return nasUpdatedText(s.snapshot?.at ?? s.last_ok_at, now);
    case "connecting":
      return "正在连，读到数据以后显示在这里";
    case "error":
      return [why || "连不上", nasRetryText(s.next_try_at, now)].filter(Boolean).join("。");
    case "auth_failed":
      return `${why || "登录失败"}。改了设置或点「重新连接」以后才会再试`;
    case "cert_untrusted":
      return "证书不受信任，没有发出凭据就断开了：编辑这个来源，测试连接并确认证书";
    case "cert_changed":
      return "证书和信任的那张不一样，没有发出凭据就断开了：编辑这个来源，测试连接并确认新证书";
    case "disabled":
      return isLocal(s)
        ? "关着：打开以后读 pt-tools 所在主机的 CPU、内存与负载"
        : "停用了：编辑里打开「启用」以后开始读";
    case "unsupported":
      return why || (isLocal(s) ? "本机来源只支持 Linux" : "这个版本用不了这种来源");
    default:
      return why;
  }
}

/** 有问题时最后一次读到数据的时间 */
function lastOkText(s: NasSource): string {
  if (!nasProblem(s.state) || !s.last_ok_at) return "";
  return `上次读到数据：${formatShortDateTime(s.last_ok_at, nas.now.value)}`;
}

function errText(e: unknown, fallback: string): string {
  const msg = e instanceof Error ? e.message.trim() : "";
  return msg || fallback;
}

// —— 刷新
const refreshing = ref(false);
async function refresh() {
  refreshing.value = true;
  try {
    await nas.reload();
    if (nas.error.value) ElMessage.error(nas.error.value);
  } finally {
    refreshing.value = false;
  }
}

// —— 重新连接（只试一次）：回应是这个来源的新状态，换进列表
const retrying = ref<string | null>(null);
async function retry(s: NasSource) {
  retrying.value = s.id;
  try {
    const st = await nasApi.retry(s.id);
    const v = view.value;
    if (v) nas.apply({ ...v, sources: v.sources.map((x) => (x.id === st.id ? st : x)) });
    if (st.state === "ok") ElMessage.success(`${s.name}：连上了`);
    else {
      const why = st.message ? `：${st.message}` : "";
      ElMessage.warning(`${s.name}：${nasStateView(st.state).label}${why}`);
    }
  } catch (e) {
    ElMessage.error(errText(e, "重新连接失败"));
  } finally {
    retrying.value = null;
  }
}

// —— 本机来源的开关与首页显示：改了马上保存，回应是一整份 GET /api/nas
const savingLocal = ref(false);
async function setLocal(on: string | number | boolean) {
  savingLocal.value = true;
  try {
    nas.apply(await nasApi.settings({ local_enabled: Boolean(on) }));
    ElMessage.success(on ? "本机来源打开了" : "本机来源关掉了");
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    savingLocal.value = false;
  }
}

const homeOptions = computed(() => nasHomeOptions(sources.value, kinds.value));
const savingHome = ref(false);
async function setHome(value: string) {
  savingHome.value = true;
  try {
    nas.apply(await nasApi.settings({ home_source: value }));
    ElMessage.success("已保存");
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    savingHome.value = false;
  }
}

/** 首页现在显示的是哪个（自动时是服务端按规则选的；选的来源停用了时退回别的） */
const homeNow = computed(() => {
  const v = view.value;
  if (!v) return "";
  const shown = v.sources.find((s) => s.id === v.home);
  if (!shown) return "现在没有能显示的来源，首页的 NAS 概览是空的。";
  const name = isLocal(shown) ? "本机" : shown.name;
  if (v.home_source && v.home_source !== v.home) {
    return `选的来源停用了或这个版本用不了，首页现在显示 ${name}。`;
  }
  // 「自动」的规则写在这里，不写进选项（选项长了手机上截断）
  return v.home_source
    ? `首页现在显示 ${name}。`
    : `自动：显示第一个能用的来源，没有时用本机；现在显示 ${name}。`;
});

/** 说明里「能接入」那一句 */
const kindsText = computed(() => {
  if (!kinds.value.length) return "这个版本还不能接入 NAS，只有本机来源。";
  const names = kinds.value.map((k) => (k.experimental ? `${k.label}（实验性）` : k.label));
  const text = `能接入：${names.join("、")}。`;
  return kinds.value.some((k) => k.experimental)
    ? `${text}实验性的还没在真机上验证过，读到的数据可能不全。`
    : text;
});

// —— 添加、编辑、删除
const dialogOpen = ref(false);
const editId = ref<number | null>(null);

function openAdd() {
  editId.value = null;
  dialogOpen.value = true;
}

function openEdit(s: NasSource) {
  editId.value = Number(s.id);
  dialogOpen.value = true;
}

async function onSaved() {
  await nas.reload();
}

async function remove(s: NasSource) {
  try {
    await ElMessageBox.confirm(
      `删除以后 pt-tools 不再读「${s.name}」，保存的密钥与登录凭据一起删掉；首页正显示它时改回自动。`,
      "删除来源",
      {
        type: "warning",
        confirmButtonText: "删除",
        cancelButtonText: "取消",
        confirmButtonClass: "el-button--danger",
      },
    );
  } catch {
    return;
  }
  try {
    await nasApi.remove(Number(s.id));
    ElMessage.success("已删除");
    await nas.reload();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}
</script>

<template>
  <div class="pt-cards pt-cards--main">
    <PtHeadSub>{{ headText }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button :loading="refreshing" data-testid="nm-refresh" @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <PtDataState
      v-if="!view"
      class="pt-cards__full"
      :state="nas.error.value ? 'error' : 'loading'"
      :sub="nas.error.value"
      data-testid="nm-state" />

    <template v-else>
      <PtPanel
        class="pt-cards__full"
        title="来源"
        icon="server"
        :count="view.sources.length"
        :action="canAdd ? '添加来源' : ''"
        action-icon="plus"
        data-testid="nm-sources"
        @action="openAdd">
        <el-table
          v-if="!isMobile"
          :data="view.sources"
          row-key="id"
          class="pt-grid"
          data-testid="nm-table">
          <el-table-column label="来源" min-width="220">
            <template #default="{ row }">
              <div class="nm-src" :data-testid="`nm-row-${row.id}`">
                <span class="nm-src__ic"><PtIcon :name="nasKindIcon(row.kind)" :size="16" /></span>
                <div class="nm-src__text">
                  <div class="nm-strong" :title="row.name">{{ row.name }}</div>
                  <div class="nm-sub nm-src__addr" :title="addressText(row)">
                    {{ kindText(row) }} · {{ addressText(row) }}
                  </div>
                  <div v-if="tagsOf(row).length" class="nm-tags">
                    <PtStatusPill v-for="t in tagsOf(row)" :key="t.text" :tone="t.tone" size="sm">{{
                      t.text
                    }}</PtStatusPill>
                  </div>
                </div>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="读到的主机" min-width="300">
            <template #default="{ row }">
              <span v-if="nasSourceSummary(row)" class="nm-summary">
                <template v-for="(part, i) in summaryParts(row)" :key="i"
                  ><template v-if="i"> · </template
                  ><span class="nm-summary__part">{{ part }}</span></template
                >
              </span>
              <span v-else class="nm-sub">还没有读到数据</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" min-width="240">
            <template #default="{ row }">
              <PtStatusPill :tone="nasStateView(row.state).tone" dot size="sm">{{
                nasStateView(row.state).label
              }}</PtStatusPill>
              <div
                class="nm-detail"
                :class="{ 'is-dang': nasProblem(row.state) }"
                :data-testid="`nm-detail-${row.id}`">
                {{ stateDetail(row) }}
              </div>
              <div v-if="lastOkText(row)" class="nm-sub">{{ lastOkText(row) }}</div>
            </template>
          </el-table-column>
          <el-table-column label="" width="210" align="right">
            <template #default="{ row }">
              <div class="nm-acts">
                <el-switch
                  v-if="isLocal(row)"
                  :model-value="view.local_enabled"
                  :disabled="!view.local_supported"
                  :loading="savingLocal"
                  aria-label="本机来源"
                  data-testid="nm-local-switch"
                  @update:model-value="setLocal" />
                <el-button
                  v-if="nasUsable(row)"
                  link
                  :loading="retrying === row.id"
                  :data-testid="`nm-retry-${row.id}`"
                  @click="retry(row)"
                  >重新连接</el-button
                >
                <template v-if="!isLocal(row)">
                  <el-button link :data-testid="`nm-edit-${row.id}`" @click="openEdit(row)"
                    >编辑</el-button
                  >
                  <el-button
                    link
                    type="danger"
                    :data-testid="`nm-delete-${row.id}`"
                    @click="remove(row)"
                    >删除</el-button
                  >
                </template>
              </div>
            </template>
          </el-table-column>
        </el-table>

        <div v-else class="nm-cards" data-testid="nm-cards">
          <PtRowCard v-for="s in view.sources" :key="s.id" :data-testid="`nm-card-${s.id}`">
            <template #lead>
              <span class="nm-src__ic"><PtIcon :name="nasKindIcon(s.kind)" :size="16" /></span>
            </template>
            <template #title>{{ s.name }}</template>
            <template #meta>
              <span>{{ kindText(s) }}</span>
              <span class="nm-card__addr">{{ addressText(s) }}</span>
              <PtStatusPill v-for="t in tagsOf(s)" :key="t.text" :tone="t.tone" size="sm">{{
                t.text
              }}</PtStatusPill>
            </template>
            <template #status>
              <PtStatusPill :tone="nasStateView(s.state).tone" dot size="sm">{{
                nasStateView(s.state).label
              }}</PtStatusPill>
            </template>
            <template #progress>
              <div v-if="nasSourceSummary(s)" class="nm-summary">
                <template v-for="(part, i) in summaryParts(s)" :key="i"
                  ><template v-if="i"> · </template
                  ><span class="nm-summary__part">{{ part }}</span></template
                >
              </div>
              <div class="nm-detail" :class="{ 'is-dang': nasProblem(s.state) }">
                {{ stateDetail(s) }}
              </div>
              <div v-if="lastOkText(s)" class="nm-sub">{{ lastOkText(s) }}</div>
            </template>
            <template #actions>
              <el-switch
                v-if="isLocal(s)"
                :model-value="view.local_enabled"
                :disabled="!view.local_supported"
                :loading="savingLocal"
                aria-label="本机来源"
                @update:model-value="setLocal" />
              <el-button
                v-if="nasUsable(s)"
                size="small"
                :loading="retrying === s.id"
                @click="retry(s)"
                >重新连接</el-button
              >
              <template v-if="!isLocal(s)">
                <el-button size="small" @click="openEdit(s)">编辑</el-button>
                <el-button size="small" type="danger" plain @click="remove(s)">删除</el-button>
              </template>
            </template>
          </PtRowCard>
        </div>

        <p class="nm-tip">
          登录失败与证书的问题不会自动重试，改了设置或点「重新连接」以后再试；别的错误按 30
          秒、1、2、5 分钟再试。
        </p>
      </PtPanel>

      <PtPanel class="nm-home" title="首页显示" icon="layout-dashboard" data-testid="nm-home">
        <el-form class="pt-form" label-position="top" @submit.prevent>
          <el-form-item label="首页的 NAS 概览显示">
            <!-- 「自动」的值是空串；el-select 默认把空串当成没选（只显示「请选择」） -->
            <el-select
              :model-value="view.home_source"
              :empty-values="[null, undefined]"
              :loading="savingHome"
              data-testid="nm-home-select"
              @update:model-value="setHome">
              <el-option
                v-for="o in homeOptions"
                :key="o.value"
                :value="o.value"
                :label="o.label" />
            </el-select>
            <div class="field-tip" data-testid="nm-home-now">{{ homeNow }}</div>
          </el-form-item>
        </el-form>
      </PtPanel>

      <PtPanel class="nm-help-panel" title="说明" icon="info" data-testid="nm-help">
        <ul class="nm-help">
          <li>
            每个来源每 30 秒读一次 CPU、内存与网络，每 5 分钟读一次存储池、硬盘、告警与应用；最近 30
            分钟的数据只放在内存里，重启以后重新读。
          </li>
          <!-- 分成几段：一段文字里的换行会显示成空格，中文标点后面不该有 -->
          <li>
            <span
              >本机来源读 pt-tools 所在主机的 <code>/proc</code> 与 <code>/sys</code>，只支持
              Linux；</span
            >
            <span>pt-tools 在 Docker 里时读到的是宿主机的 CPU、内存与负载。</span>
            <span
              >它读不到网卡、存储池、硬盘与告警：首页的网络换成下载器速度，存储换成下载盘与媒体库所在的盘。</span
            >
          </li>
          <li data-testid="nm-kinds">{{ kindsText }}</li>
          <li>
            密钥加密保存在数据库里，页面上不会显示。自签证书在「测试连接」时核对指纹以后信任，以后只认那一张。
          </li>
        </ul>
      </PtPanel>
    </template>

    <NasSourceDialog v-model="dialogOpen" :edit-id="editId" :kinds="kinds" @saved="onSaved" />
  </div>
</template>

<style scoped>
/* 来源一格：类型图标在 32 的浅底方块里，右边名字、类型与地址、标签 */
.nm-src {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  min-width: 0;
}

.nm-src__ic {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 32px;
  height: 32px;
  color: var(--pt-t2);
  background: var(--pt-hover);
  border-radius: var(--pt-r-md);
}

.nm-src__text {
  min-width: 0;
}

/* 类型与地址一行：放不下时省略（悬停看完整的），不在「·」后面折行 */
.nm-src__addr {
  white-space: nowrap;
}

.nm-strong {
  overflow: hidden;
  font-weight: 600;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nm-sub {
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  text-overflow: ellipsis;
}

.nm-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-top: 4px;
}

.nm-summary {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  word-break: break-word;
}

/* 一段整体换行（放得下就不从中间断开）；比一行还长时才在段里折 */
.nm-summary__part {
  display: inline-block;
  max-width: 100%;
  overflow-wrap: anywhere;
}

/* 宽屏两栏：说明的字多，放左边的宽栏；首页显示只有一个下拉框，放右边的窄栏；两张卡不拉成一样高 */
.nm-home,
.nm-help-panel {
  align-self: start;
}

@media (min-width: 1181px) {
  .nm-home {
    grid-row: 2;
    grid-column: 2;
  }

  .nm-help-panel {
    grid-row: 2;
    grid-column: 1;
  }
}

/* 状态下面的原因：错误原文可能很长，折行，不撑宽这一列 */
.nm-detail {
  margin-top: 4px;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
  word-break: break-word;
}

.nm-detail.is-dang {
  color: var(--pt-dang);
}

.nm-acts {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  align-items: center;
  justify-content: flex-end;
}

.nm-acts .el-button + .el-button {
  margin-left: 0;
}

.nm-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/* 手机行卡里的地址：太长时省略（完整的在编辑里） */
.nm-card__addr {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nm-tip {
  margin: 12px 0 0;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t3);
}

.nm-help {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0 0 0 18px;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.nm-help code {
  font-family: var(--pt-font-mono);
}
</style>
