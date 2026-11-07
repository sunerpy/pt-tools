<script setup lang="ts">
/*
 * 订阅弹窗的「试搜索」（追剧方案 F2，3.2.4–3.2.6）：按还没保存的表单（质量档案、只在哪些站点找、标题包含与排除）
 * 在站点上搜一次，结果放在右侧抽屉里，不推送、不写库。分三栏：符合条件、不符合（写出全部原因）、对不上这一季。
 * 结果上的操作只改表单（由对话框改）：复制标题、只要或不要这个制作组、选标题里的一段加进包含或排除、锁这个版本、
 * 保存时从这个种子开始追。2026-11-07 用户：「还可以试搜索列出检索后的结果 并可以比如复制标题等内容作为搜索条件进行准确追剧」。
 */
import {
  type MediaSearchInput,
  type MediaSearchMember,
  type MediaSearchResult,
  subscribeApi,
} from "@/api";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { copyText } from "@/utils/clipboard";
import {
  type EntryCard,
  type Pill,
  buildCards,
  coverPill,
  episodeNote,
  lockMismatch,
  memberLine,
  parsedTags,
  shortSize,
} from "@/utils/entrySearch";
import { type TitleToken, hasEpisode, segmentText, splitTitle } from "@/utils/titleSegments";
import { ElMessage } from "element-plus";
import { computed, ref, watch } from "vue";

export interface PreviewLock {
  group: string;
  resolution: string;
}

export interface PreviewFollow {
  site: string;
  torrent_id: string;
  title: string;
  /** 这个种子的第一集；整季包是 0（不改起始集） */
  start: number;
}

const props = defineProps<{
  modelValue: boolean;
  /** 按现在的表单拼好的请求；条目还没定时为空 */
  request: MediaSearchInput | null;
  /** 抽屉标题里的条目名 */
  title: string;
  /** 剧集 */
  tv: boolean;
  /** 新建剧集：可以「保存时从这个种子开始追」 */
  canFollow: boolean;
  /** 表单里的版本锁：关着时为空；开着、还没定版本时组和分辨率都是空串 */
  lock: PreviewLock | null;
  /** 表单里已经选了从哪个种子开始追 */
  follow?: { site: string; torrent_id: string } | null;
}>();
const emit = defineEmits<{
  (e: "update:modelValue", v: boolean): void;
  (e: "include", word: string): void;
  (e: "exclude", word: string): void;
  (e: "lock", v: PreviewLock): void;
  (e: "follow", v: PreviewFollow): void;
}>();

const isMobile = useIsMobile();
const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});

const loading = ref(false);
const result = ref<MediaSearchResult | null>(null);
const error = ref("");
const tookMs = ref(0);
/** 这次结果用的请求（和表单现在的比，看表单是不是改过了） */
const ranWith = ref("");
type Tab = "ok" | "reject" | "unmatched";
const tab = ref<Tab>("ok");
let seq = 0;

async function run() {
  const req = props.request;
  if (!req) return;
  const n = ++seq;
  loading.value = true;
  error.value = "";
  picker.value = null;
  const key = JSON.stringify(req);
  const started = performance.now();
  try {
    const res = await subscribeApi.searchMedia(req);
    if (n !== seq) return;
    result.value = res ?? null;
    selected.value = {};
    ranWith.value = key;
    tookMs.value = performance.now() - started;
  } catch (e) {
    if (n !== seq) return;
    // 后端的「参数不对: 」是通用的前缀，这里直接写哪里不对
    error.value =
      ((e as Error)?.message || "").trim().replace(/^参数不对[:：]\s*/, "") || "试搜索失败";
  } finally {
    if (n === seq) loading.value = false;
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      tab.value = "ok";
      void run();
    } else {
      // 关掉以后回来的结果不要了
      seq++;
      loading.value = false;
    }
  },
  { immediate: true },
);

const stale = computed(
  () => Boolean(result.value) && !loading.value && JSON.stringify(props.request) !== ranWith.value,
);

// 一组一张卡（合并、按服务端的顺序、拉黑的也列出来：放进「不符合」写明拉黑了）
const cards = computed(() =>
  result.value
    ? buildCards(result.value, { merge: true, sort: "score", sel: {}, text: "", showBlocked: true })
    : [],
);
const okCards = computed(() => cards.value.filter((c) => c.pass && !c.blocked));
const rejectCards = computed(() => cards.value.filter((c) => !c.pass || c.blocked));
const shown = computed(() => (tab.value === "ok" ? okCards.value : rejectCards.value));
const unmatched = computed(() => result.value?.unmatched ?? []);

const tabs = computed(() => [
  { value: "ok", label: `符合条件 ${okCards.value.length}` },
  { value: "reject", label: `不符合 ${rejectCards.value.length}` },
  {
    value: "unmatched",
    label: `${props.tv ? "对不上这一季" : "对不上这部电影"} ${result.value?.unmatched_count ?? 0}`,
  },
]);

const took = computed(() => `${(tookMs.value / 1000).toFixed(1)} 秒`);
const errors = computed(() => result.value?.errors ?? []);
const profileLine = computed(() => {
  const p = result.value?.profile;
  if (!p) return "";
  return p.id ? `按档案「${p.name}」打分` : "没有质量档案：按分辨率与来源比";
});
const seasonNote = computed(() =>
  props.tv && result.value ? episodeNote(result.value.episodes) : "",
);

/** 表单的版本锁定了版本时，对不上它的组标「版本锁不收」 */
const activeLock = computed(() =>
  props.tv && props.lock && (props.lock.group || props.lock.resolution) ? props.lock : null,
);
const lockLine = computed(() => {
  if (!props.tv || !props.lock) return "";
  if (activeLock.value) {
    return `只收 ${[activeLock.value.group, activeLock.value.resolution].filter(Boolean).join(" ")}`;
  }
  return "开着、还没定版本：第一个下载成功的版本会成为锁";
});
const lockText = (c: EntryCard) =>
  activeLock.value ? lockMismatch(activeLock.value, c.group.meta) : "";
const lockedHere = (c: EntryCard) =>
  Boolean(activeLock.value) && !lockMismatch(activeLock.value, c.group.meta);

/** 符合条件不等于会下载：每组再写一句能补哪几集、已经有了还是在下载 */
function groupNote(c: EntryCard): Pill {
  const res = result.value;
  const p = res ? coverPill(c.group, res.episodes, res.movie) : null;
  if (p) return p;
  return props.tv
    ? { text: "没有要补的集", tone: "neutral" }
    : { text: "库里还没有", tone: "ok", dot: true };
}

function rejectText(c: EntryCard): string {
  if (c.blocked) {
    const reason = c.members.find((m) => m.block?.reason)?.block?.reason;
    return reason ? `拉黑了（${reason}）` : "拉黑了";
  }
  return c.members[0]?.verdict?.reject || c.group.verdict?.reject || "";
}

// ---- 一组里选中的那个站点的种子（操作对它）：默认是第一个没拉黑的 ----
const selected = ref<Record<string, number>>({});
function selIndex(c: EntryCard): number {
  const i = selected.value[c.key];
  if (i !== undefined && c.members[i]) return i;
  const free = c.members.findIndex((m) => !m.block);
  return free < 0 ? 0 : free;
}
const selMember = (c: EntryCard) => c.members[selIndex(c)]!;
function select(c: EntryCard, i: number) {
  selected.value = { ...selected.value, [c.key]: i };
  if (picker.value?.key === c.key) picker.value = null;
}

function memberNote(m: MediaSearchMember): string {
  const parts = [shortSize(m.size_bytes), memberLine(m)];
  if (m.block) parts.push("已拉黑");
  else if (m.verdict?.reject) parts.push(`不符合：${m.verdict.reject}`);
  return parts.join(" · ");
}

function followingHere(c: EntryCard): boolean {
  const f = props.follow;
  if (!f) return false;
  const m = selMember(c);
  return f.site === m.site && f.torrent_id === m.torrent_id;
}

async function copy(text: string, what: string) {
  if (await copyText(text)) ElMessage.success(`已复制${what}`);
  else ElMessage.warning("浏览器不让复制：选中文字手动复制");
}

function lockTo(c: EntryCard) {
  emit("lock", { group: c.group.meta.group ?? "", resolution: c.group.meta.resolution ?? "" });
}

function follow(c: EntryCard) {
  const m = selMember(c);
  const sp = c.group.span;
  const start = sp && !sp.whole ? (sp.start ?? 0) : 0;
  emit("follow", { site: m.site, torrent_id: m.torrent_id, title: m.title, start });
}

// ---- 选标题里的一段：点起止两块（只要一块时点它两次） ----
const picker = ref<{
  key: string;
  title: string;
  tokens: TitleToken[];
  from: number | null;
  to: number | null;
} | null>(null);

function openPicker(c: EntryCard) {
  const m = selMember(c);
  picker.value = { key: c.key, title: m.title, tokens: splitTitle(m.title), from: null, to: null };
}

function pickToken(i: number) {
  const p = picker.value;
  if (!p) return;
  if (p.from === null || p.to !== null) {
    p.from = i;
    p.to = null;
  } else {
    p.to = i;
  }
}

function inRange(i: number): boolean {
  const p = picker.value;
  if (!p || p.from === null) return false;
  const to = p.to ?? p.from;
  return i >= Math.min(p.from, to) && i <= Math.max(p.from, to);
}

const pickedText = computed(() => {
  const p = picker.value;
  if (!p || p.from === null) return "";
  return segmentText(p.title, p.tokens, p.from, p.to ?? p.from);
});
const pickedEpisode = computed(() => hasEpisode(pickedText.value));

function usePicked(to: "include" | "exclude") {
  const text = pickedText.value;
  if (!text) return;
  if (to === "include") emit("include", text);
  else emit("exclude", text);
  picker.value = null;
}
</script>

<template>
  <el-drawer
    v-model="visible"
    :size="isMobile ? '100%' : '560px'"
    :title="`试搜索：${title}`"
    append-to-body
    data-testid="sp-drawer">
    <div class="sp">
      <div class="sp__bar">
        <span class="sp__muted">按表单现在的质量档案、站点、标题包含与排除搜，不推送、不保存</span>
        <el-button
          size="small"
          :loading="loading"
          :disabled="!request"
          data-testid="sp-run"
          @click="run">
          {{ result || error ? "再试一次" : "试搜索" }}
        </el-button>
      </div>

      <div v-if="stale" class="pt-note pt-note--warn" data-testid="sp-stale">
        表单改过了：点「再试一次」看按新表单的结果
      </div>
      <div v-if="loading && !result" class="sp__muted" data-testid="sp-loading">
        在各站点搜索，慢的站点最多等 1 分钟…
      </div>
      <div v-if="error" class="pt-note pt-note--dang" data-testid="sp-error">{{ error }}</div>

      <template v-if="result">
        <dl class="sp__facts" data-testid="sp-facts">
          <template v-if="result.keywords.length">
            <dt>搜的词</dt>
            <dd data-testid="sp-keywords">{{ result.keywords.join("、") }}</dd>
          </template>
          <dt>用时</dt>
          <dd data-testid="sp-took">
            {{ took }}<template v-if="profileLine"> · {{ profileLine }}</template>
          </dd>
          <template v-if="seasonNote">
            <dt>这一季</dt>
            <dd data-testid="sp-season">{{ seasonNote }}</dd>
          </template>
          <template v-if="lockLine">
            <dt>版本锁</dt>
            <dd data-testid="sp-lock-line">{{ lockLine }}</dd>
          </template>
        </dl>
        <div v-if="errors.length" class="pt-note pt-note--warn sp__errors" data-testid="sp-errors">
          <div v-for="(e, i) in errors" :key="i">{{ e.site || "搜索" }}：{{ e.error }}</div>
          <div>这次有站点出错，结果没有进搜索缓存：再试会重新去站点搜</div>
        </div>
        <div v-else class="sp__muted" data-testid="sp-cache">
          只改标题包含、排除或质量档案再试，用的是 10
          分钟内的搜索结果，很快；有站点出错的那次不缓存，再试会重新搜
        </div>

        <el-segmented v-model="tab" class="pt-seg sp__tabs" :options="tabs" data-testid="sp-tabs" />

        <template v-if="tab !== 'unmatched'">
          <div v-if="!shown.length" class="sp__empty" data-testid="sp-empty">
            {{ tab === "ok" ? "没有符合条件的种子：看看「不符合」里的原因" : "没有不符合的种子" }}
          </div>
          <div
            v-for="(c, ci) in shown"
            :key="c.key"
            class="sp-card"
            :class="{ 'is-dim': c.group.in_library || c.blocked }"
            :data-testid="`sp-${tab}-${ci}`">
            <div class="sp-card__title" :data-testid="`sp-${tab}-${ci}-title`">
              {{ selMember(c).title }}
            </div>
            <div v-if="selMember(c).subtitle" class="sp-card__sub">{{ selMember(c).subtitle }}</div>
            <div v-if="parsedTags(c.group.meta).length" class="sp-card__tags">
              <PtTag v-for="t in parsedTags(c.group.meta)" :key="t.label" :tone="t.tone">{{
                t.label
              }}</PtTag>
            </div>
            <div class="sp-card__notes">
              <span
                v-if="tab === 'reject'"
                class="sp-card__warn"
                :data-testid="`sp-${tab}-${ci}-reject`"
                >不符合：{{ rejectText(c) }}</span
              >
              <PtStatusPill
                v-else
                :tone="groupNote(c).tone"
                :dot="groupNote(c).dot"
                size="sm"
                :data-testid="`sp-${tab}-${ci}-note`"
                >{{ groupNote(c).text }}</PtStatusPill
              >
              <span
                v-if="lockText(c)"
                class="sp-card__warn"
                :data-testid="`sp-${tab}-${ci}-lock-miss`"
                >版本锁不收（{{ lockText(c) }}）</span
              >
            </div>
            <ul class="sp-card__members">
              <li v-for="(m, mi) in c.members" :key="`${m.site}|${m.torrent_id}`">
                <button
                  type="button"
                  class="sp-card__member"
                  :class="{ 'is-sel': c.members.length > 1 && mi === selIndex(c) }"
                  :aria-pressed="mi === selIndex(c)"
                  :data-testid="`sp-${tab}-${ci}-m-${mi}`"
                  @click="select(c, mi)">
                  <span class="sp-card__site">{{ m.site }}</span>
                  <span class="sp-card__line">{{ memberNote(m) }}</span>
                </button>
              </li>
            </ul>
            <div class="sp-card__acts">
              <el-button
                size="small"
                :data-testid="`sp-${tab}-${ci}-copy-title`"
                @click="copy(selMember(c).title, '标题')"
                >复制标题</el-button
              >
              <el-button
                v-if="selMember(c).subtitle"
                size="small"
                :data-testid="`sp-${tab}-${ci}-copy-subtitle`"
                @click="copy(selMember(c).subtitle ?? '', '副标题')"
                >复制副标题</el-button
              >
              <template v-if="c.group.meta.group">
                <el-button
                  size="small"
                  :data-testid="`sp-${tab}-${ci}-only`"
                  @click="emit('include', c.group.meta.group)"
                  >只要「{{ c.group.meta.group }}」</el-button
                >
                <el-button
                  size="small"
                  :data-testid="`sp-${tab}-${ci}-not`"
                  @click="emit('exclude', c.group.meta.group)"
                  >不要「{{ c.group.meta.group }}」</el-button
                >
              </template>
              <el-button size="small" :data-testid="`sp-${tab}-${ci}-pick`" @click="openPicker(c)"
                >选标题里的一段…</el-button
              >
              <el-button
                v-if="tv && c.group.meta.group"
                size="small"
                :disabled="lockedHere(c)"
                :data-testid="`sp-${tab}-${ci}-lock`"
                @click="lockTo(c)"
                >{{ lockedHere(c) ? "已锁这个版本" : "锁这个版本" }}</el-button
              >
              <el-button
                v-if="canFollow"
                size="small"
                :disabled="followingHere(c)"
                :data-testid="`sp-${tab}-${ci}-follow`"
                @click="follow(c)"
                >{{ followingHere(c) ? "保存时从它开始追" : "保存时从这个种子开始追" }}</el-button
              >
            </div>
            <div v-if="picker && picker.key === c.key" class="sp-pick" data-testid="sp-picker">
              <div class="sp__muted">点这一段的第一块和最后一块（只要一块时点它两次）</div>
              <div class="sp-pick__tokens">
                <button
                  v-for="(t, ti) in picker.tokens"
                  :key="ti"
                  type="button"
                  class="sp-pick__token"
                  :class="{ 'is-on': inRange(ti) }"
                  :aria-pressed="inRange(ti)"
                  :data-testid="`sp-token-${ti}`"
                  @click="pickToken(ti)">
                  {{ t.text }}
                </button>
              </div>
              <template v-if="pickedText">
                <div class="sp-pick__text" data-testid="sp-pick-text">{{ pickedText }}</div>
                <div v-if="pickedEpisode" class="pt-note pt-note--warn" data-testid="sp-pick-warn">
                  这一段里有集号：加进标题包含以后只收这一集（或这几集）
                </div>
              </template>
              <div class="sp-pick__acts">
                <el-button
                  size="small"
                  type="primary"
                  :disabled="!pickedText"
                  data-testid="sp-pick-include"
                  @click="usePicked('include')"
                  >加进标题包含</el-button
                >
                <el-button
                  size="small"
                  :disabled="!pickedText"
                  data-testid="sp-pick-exclude"
                  @click="usePicked('exclude')"
                  >加进标题排除</el-button
                >
                <el-button size="small" link data-testid="sp-pick-cancel" @click="picker = null"
                  >取消</el-button
                >
              </div>
            </div>
          </div>
        </template>
        <template v-else>
          <div v-if="!unmatched.length" class="sp__empty" data-testid="sp-empty">
            没有对不上的种子
          </div>
          <div
            v-for="(m, i) in unmatched"
            :key="`${m.site}|${m.torrent_id}`"
            class="sp-card"
            :data-testid="`sp-unmatched-${i}`">
            <div class="sp-card__title">{{ m.title }}</div>
            <div v-if="m.subtitle" class="sp-card__sub">{{ m.subtitle }}</div>
            <div class="sp-card__notes">
              <span class="sp-card__site">{{ m.site }}</span>
              <span class="sp-card__warn">{{ m.reason || "名字或季对不上这个条目" }}</span>
            </div>
            <div class="sp-card__acts">
              <el-button
                size="small"
                :data-testid="`sp-unmatched-${i}-copy-title`"
                @click="copy(m.title, '标题')"
                >复制标题</el-button
              >
            </div>
          </div>
        </template>
      </template>
    </div>
  </el-drawer>
</template>

<style scoped>
.sp {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-3);
  min-width: 0;
}

.sp__bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pt-space-2);
}

.sp__muted {
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t3);
}

.sp__facts {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 4px 12px;
  margin: 0;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
}

.sp__facts dt {
  font-weight: 500;
  color: var(--pt-t3);
}

.sp__facts dd {
  min-width: 0;
  margin: 0;
  color: var(--pt-t2);
  word-break: break-word;
}

.sp__errors {
  flex-direction: column;
  gap: 2px;
}

.sp__tabs {
  align-self: flex-start;
  max-width: 100%;
}

.sp__empty {
  padding: var(--pt-space-4) 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  text-align: center;
}

.sp-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
  padding: 12px 14px;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.sp-card.is-dim .sp-card__title,
.sp-card.is-dim .sp-card__sub {
  color: var(--pt-t3);
}

.sp-card__title {
  font-size: var(--pt-fz-body);
  font-weight: 600;
  line-height: var(--pt-lh-body);
  color: var(--pt-t1);
  word-break: break-all;
}

.sp-card__sub {
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t3);
  word-break: break-all;
}

.sp-card__tags,
.sp-card__notes,
.sp-card__acts {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.sp-card__acts .el-button + .el-button {
  margin-left: 0;
}

.sp-card__warn {
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-warn);
  word-break: break-word;
}

.sp-card__members {
  display: flex;
  flex-direction: column;
  margin: 0;
  padding: 0;
  list-style: none;
  border-top: 1px solid var(--pt-border);
}

.sp-card__member {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 8px;
  width: 100%;
  padding: 6px 4px;
  font-family: inherit;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
  text-align: left;
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--pt-border);
  cursor: pointer;
}

.sp-card__member:hover,
.sp-card__member.is-sel {
  background: var(--pt-hover);
}

.sp-card__site {
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-t1);
}

.sp-card__line {
  min-width: 0;
  word-break: break-word;
}

.sp-pick {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px;
  background: var(--pt-canvas);
  border: 1px dashed var(--pt-border-strong);
  border-radius: var(--pt-r-md);
}

.sp-pick__tokens {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.sp-pick__token {
  min-height: 28px;
  padding: 2px 8px;
  font-family: inherit;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-sm);
  cursor: pointer;
}

.sp-pick__token:hover,
.sp-pick__token:focus-visible {
  border-color: var(--pt-p);
}

.sp-pick__token.is-on {
  font-weight: 600;
  color: var(--pt-p);
  border-color: var(--pt-p);
}

.sp-pick__text {
  padding: 6px 8px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-sm);
  color: var(--pt-t1);
  background: var(--pt-surface);
  border-radius: var(--pt-r-sm);
  word-break: break-all;
}

.sp-pick__acts {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.sp-pick__acts .el-button + .el-button {
  margin-left: 0;
}
</style>
