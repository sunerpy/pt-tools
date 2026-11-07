<script setup lang="ts">
/*
 * 手机追剧编辑页的「试搜索」（追剧方案 F3，3.3.2）：按表单里还没保存的季、站点、质量档案、标题包含与排除
 * 在各站点搜一次（只读，不推送、不保存），结果放在占满屏的底部弹层里。三栏同网页（3.2.4）：符合条件、
 * 不符合（写出全部原因）、对不上这一季。点一组弹出操作列表（3.2.5）：复制标题与副标题、只要或不要这个制作组、
 * 选标题里的一段加进包含或排除、锁这个版本、保存时从这个种子开始追；都只改表单，由编辑页改。
 * 搜索中可以「取消」：只取消这一次请求（原生桥按编号取消），不算出错。
 * 2026-11-07 用户：「还可以试搜索列出检索后的结果 并可以比如复制标题等内容作为搜索条件进行准确追剧」。
 */
import { ElMessage } from "element-plus";
import { computed, onBeforeUnmount, ref, watch } from "vue";
import type { MediaSearchMember, MediaSearchResult } from "@/api";
import {
  type EntryCard,
  buildCards,
  lockMismatch,
  parsedTags,
  shortSize,
} from "@/utils/entrySearch";
import { type TitleToken, hasEpisode, segmentText, splitTitle } from "@/utils/titleSegments";
import { type AppPreview, type PreviewRequest, mobileApi, previewResult } from "./api";
import { BridgeError, CANCELLED_STATUS, copyText } from "./bridge";
import { useEnglish, useWords } from "./i18n";
import { coverText, episodeText, memberText } from "./subscriptionText";

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
export interface PreviewSummary {
  ok: number;
  reject: number;
  unmatched: number;
  seconds: number;
}

const props = defineProps<{
  modelValue: boolean;
  /** 按现在的表单拼好的请求；条目还没定时为空 */
  request: PreviewRequest | null;
  title: string;
  tv: boolean;
  /** 新建剧集：可以「保存时从这个种子开始追」 */
  canFollow: boolean;
  /** 表单里的版本锁：关着时为空；开着、还没定版本时组和分辨率都是空串 */
  lock: PreviewLock | null;
  /** 表单里已经选了从哪个种子开始追 */
  follow: { site: string; torrent_id: string } | null;
  /** 这次搜几个站点（「正在搜 N 个站点」） */
  siteCount: number;
}>();
const emit = defineEmits<{
  (e: "update:modelValue", v: boolean): void;
  (e: "include", word: string): void;
  (e: "exclude", word: string): void;
  (e: "lock", v: PreviewLock): void;
  (e: "follow", v: PreviewFollow): void;
  (e: "summary", v: PreviewSummary): void;
}>();
const t = useWords();
const en = useEnglish();

const visible = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});

const loading = ref(false);
const raw = ref<AppPreview | null>(null);
const result = ref<MediaSearchResult | null>(null);
const error = ref("");
/** 这次结果用的请求（和表单现在的比，看条件是不是改过了） */
const ranWith = ref("");
type Tab = "ok" | "reject" | "unmatched";
const tab = ref<Tab>("ok");
let seq = 0;
let controller: AbortController | null = null;

function describe(e: unknown): string {
  if (e instanceof BridgeError && e.status === 504)
    return t("超时：少选几个站点再试", "Timed out: choose fewer sites and try again");
  const msg = ((e as Error)?.message || "").trim().replace(/^参数不对[:：]\s*/, "");
  return msg || t("试搜索失败", "Search preview failed");
}

async function run() {
  const req = props.request;
  if (!req) return;
  controller?.abort();
  const ctl = new AbortController();
  controller = ctl;
  const n = ++seq;
  loading.value = true;
  error.value = "";
  action.value = null;
  const key = JSON.stringify(req);
  try {
    const res = await mobileApi.preview(req, { signal: ctl.signal });
    if (n !== seq) return;
    raw.value = res;
    result.value = previewResult(res);
    selected.value = {};
    ranWith.value = key;
    emit("summary", {
      ok: okCards.value.length,
      reject: rejectCards.value.length,
      unmatched: res.unmatched_count,
      seconds: res.duration_ms / 1000,
    });
  } catch (e) {
    if (n !== seq) return;
    // 取消不是出错：按钮恢复，不写错误
    if (e instanceof BridgeError && e.status === CANCELLED_STATUS) return;
    error.value = describe(e);
  } finally {
    if (n === seq) {
      loading.value = false;
      controller = null;
    }
  }
}

/** 只取消这一次试搜索：表单和别的请求不受影响，马上可以再试 */
function cancel() {
  seq++;
  controller?.abort();
  controller = null;
  loading.value = false;
}

const stale = computed(
  () => Boolean(result.value) && !loading.value && JSON.stringify(props.request) !== ranWith.value,
);

// 一组一张卡（合并、按服务端的顺序；拉黑的放进「不符合」，写明拉黑了）
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
  { value: "ok", label: `${t("符合条件", "Matching")} ${okCards.value.length}` },
  { value: "reject", label: `${t("不符合", "Rejected")} ${rejectCards.value.length}` },
  {
    value: "unmatched",
    label: `${props.tv ? t("对不上这一季", "Not this season") : t("对不上这部电影", "Not this movie")} ${result.value?.unmatched_count ?? 0}`,
  },
]);

const took = computed(() =>
  raw.value
    ? t(
        `${(raw.value.duration_ms / 1000).toFixed(1)} 秒`,
        `${(raw.value.duration_ms / 1000).toFixed(1)} s`,
      )
    : "",
);
const profileLine = computed(() => {
  const p = result.value?.profile;
  if (!p) return "";
  return p.id
    ? t(`按档案「${p.name}」打分`, `Scored by profile "${p.name}"`)
    : t("没有质量档案：按分辨率与来源比", "No quality profile: compared by resolution and source");
});
const seasonNote = computed(() =>
  props.tv && result.value ? episodeText(result.value.episodes, en) : "",
);
const errors = computed(() => result.value?.errors ?? []);

/** 表单的版本锁定了版本时，对不上它的组标「版本锁不收」 */
const activeLock = computed(() =>
  props.tv && props.lock && (props.lock.group || props.lock.resolution) ? props.lock : null,
);
const lockText = (l: PreviewLock) => [l.group, l.resolution].filter(Boolean).join(" ");
const lockLine = computed(() => {
  if (!props.tv || !props.lock) return "";
  if (activeLock.value)
    return t(`只收 ${lockText(activeLock.value)}`, `Only ${lockText(activeLock.value)}`);
  return t(
    "开着、还没定版本：第一个下载成功的版本会成为锁",
    "On, not set yet: the first version downloaded becomes the lock",
  );
});
const lockMiss = (c: EntryCard) =>
  activeLock.value ? lockMismatch(activeLock.value, c.group.meta) : "";
const lockedHere = (c: EntryCard) =>
  Boolean(activeLock.value) && !lockMismatch(activeLock.value, c.group.meta);

function groupNote(c: EntryCard): string {
  const res = result.value;
  const p = res ? coverText(c.group, res.episodes, res.movie, en) : null;
  if (p) return p.text;
  return props.tv
    ? t("没有要补的集", "No missing episodes to fill")
    : t("库里还没有", "Not in the library yet");
}
function rejectText(c: EntryCard): string {
  if (c.blocked) return t("拉黑了", "Blocked");
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
function memberNote(m: MediaSearchMember): string {
  const parts = [m.site, shortSize(m.size_bytes), memberText(m, en)];
  if (m.block) parts.push(t("已拉黑", "Blocked"));
  else if (m.verdict?.reject)
    parts.push(t(`不符合：${m.verdict.reject}`, `Rejected: ${m.verdict.reject}`));
  return parts.join(" · ");
}
const tagsOf = (c: EntryCard) => parsedTags(c.group.meta).map((x) => x.label);
function followingHere(c: EntryCard): boolean {
  const f = props.follow;
  if (!f) return false;
  const m = selMember(c);
  return f.site === m.site && f.torrent_id === m.torrent_id;
}

// ---- 点一组弹出的操作列表 ----
type Action = { kind: "card"; card: EntryCard } | { kind: "unmatched"; member: MediaSearchMember };
const action = ref<Action | null>(null);
const actionOpen = computed({
  get: () => action.value !== null,
  set: (v: boolean) => {
    if (!v) {
      action.value = null;
      picker.value = null;
    }
  },
});
const actionMember = computed<MediaSearchMember | null>(() => {
  const a = action.value;
  if (!a) return null;
  return a.kind === "card" ? selMember(a.card) : a.member;
});
const actionCard = computed(() => (action.value?.kind === "card" ? action.value.card : null));
const group = computed(() => actionCard.value?.group.meta.group ?? "");

function openCard(c: EntryCard) {
  picker.value = null;
  action.value = { kind: "card", card: c };
}
function openUnmatched(m: MediaSearchMember) {
  picker.value = null;
  action.value = { kind: "unmatched", member: m };
}
function select(i: number) {
  const c = actionCard.value;
  if (!c) return;
  selected.value = { ...selected.value, [c.key]: i };
  picker.value = null;
}
function done() {
  action.value = null;
  picker.value = null;
}

async function copy(text: string, what: "title" | "subtitle") {
  try {
    await copyText(text);
    ElMessage.success(
      what === "title" ? t("已复制标题", "Title copied") : t("已复制副标题", "Subtitle copied"),
    );
  } catch (e) {
    ElMessage.error(t(`复制失败：${(e as Error).message}`, `Copy failed: ${(e as Error).message}`));
  }
  done();
}
function only(word: string) {
  emit("include", word);
  done();
}
function not(word: string) {
  emit("exclude", word);
  done();
}
function lockTo(c: EntryCard) {
  emit("lock", { group: c.group.meta.group ?? "", resolution: c.group.meta.resolution ?? "" });
  done();
}
function followFrom(c: EntryCard) {
  const m = selMember(c);
  const sp = c.group.span;
  const start = sp && !sp.whole ? (sp.start ?? 0) : 0;
  emit("follow", { site: m.site, torrent_id: m.torrent_id, title: m.title, start });
  done();
}

// ---- 选标题里的一段：点起止两块（只要一块时点它两次） ----
const picker = ref<{
  title: string;
  tokens: TitleToken[];
  from: number | null;
  to: number | null;
} | null>(null);
function openPicker() {
  const m = actionMember.value;
  if (!m) return;
  picker.value = { title: m.title, tokens: splitTitle(m.title), from: null, to: null };
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
  done();
}

// 打开时搜一次，关掉时取消还没回来的那次（放在最后：immediate 回调会用到上面声明的状态）
watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      tab.value = "ok";
      void run();
    } else {
      cancel();
      action.value = null;
    }
  },
  { immediate: true },
);
onBeforeUnmount(cancel);
</script>

<template>
  <el-drawer
    v-model="visible"
    direction="btt"
    size="100%"
    append-to-body
    class="msp-drawer"
    :title="t(`试搜索：${title}`, `Search preview: ${title}`)"
    data-testid="msp">
    <div class="msp">
      <p class="mobile-note msp__hint">
        {{
          t(
            "按表单现在的季、站点、质量档案、标题包含与排除搜，不推送、不保存",
            "Searches with the form as it is now: nothing is downloaded or saved",
          )
        }}
      </p>

      <div v-if="loading" class="msp__loading" role="status" data-testid="msp-loading">
        <span>{{
          t(
            `正在搜 ${siteCount} 个站点，最多 25 秒`,
            `Searching ${siteCount} sites, up to 25 seconds`,
          )
        }}</span>
        <el-button data-testid="msp-cancel" @click="cancel">{{ t("取消", "Cancel") }}</el-button>
      </div>
      <div v-else class="msp__bar">
        <el-button type="primary" :disabled="!request" data-testid="msp-run" @click="run">{{
          stale
            ? t("按新条件再试", "Try again with the new conditions")
            : result
              ? t("再试一次", "Try again")
              : t("试搜索", "Search preview")
        }}</el-button>
      </div>
      <div v-if="stale" class="msp__stale" role="status" data-testid="msp-stale">
        {{
          t(
            "条件改过了：结果还是按改之前的",
            "The conditions changed: these results are from before",
          )
        }}
      </div>
      <div v-if="error" class="mobile-error" role="alert" data-testid="msp-error">{{ error }}</div>

      <template v-if="result">
        <dl class="msp__facts" data-testid="msp-facts">
          <template v-if="result.keywords.length">
            <dt>{{ t("搜的词", "Searched for") }}</dt>
            <dd data-testid="msp-keywords">{{ result.keywords.join(t("、", ", ")) }}</dd>
          </template>
          <dt>{{ t("用时", "Took") }}</dt>
          <dd data-testid="msp-took">
            {{ took }}<template v-if="profileLine"> · {{ profileLine }}</template>
          </dd>
          <template v-if="seasonNote">
            <dt>{{ t("这一季", "This season") }}</dt>
            <dd data-testid="msp-season">{{ seasonNote }}</dd>
          </template>
          <template v-if="lockLine">
            <dt>{{ t("版本锁", "Version lock") }}</dt>
            <dd data-testid="msp-lock">{{ lockLine }}</dd>
          </template>
        </dl>
        <div v-if="errors.length" class="msp__errors" data-testid="msp-errors">
          <div v-for="(e, i) in errors" :key="i">
            {{ e.site ? `${e.site}：${e.error}` : e.error }}
          </div>
          <div class="mobile-note">
            {{
              t(
                "有站点出错，这次结果没有进搜索缓存，再试会重新搜",
                "A site failed, so these results were not cached: trying again searches again",
              )
            }}
          </div>
        </div>

        <el-segmented
          v-model="tab"
          :options="tabs"
          block
          class="pt-seg msp__tabs"
          data-testid="msp-tabs" />

        <template v-if="tab !== 'unmatched'">
          <p v-if="!shown.length" class="mobile-state" data-testid="msp-empty">
            {{ t("没有", "None") }}
          </p>
          <button
            v-for="(c, i) in shown"
            :key="c.key"
            type="button"
            class="msp-card"
            :data-testid="`msp-${tab}-${i}`"
            @click="openCard(c)">
            <span class="msp-card__title" :data-testid="`msp-${tab}-${i}-title`">{{
              selMember(c).title
            }}</span>
            <span v-if="selMember(c).subtitle" class="msp-card__sub">{{
              selMember(c).subtitle
            }}</span>
            <span v-if="tagsOf(c).length" class="msp-card__tags">{{ tagsOf(c).join(" · ") }}</span>
            <span
              v-if="tab === 'ok'"
              class="msp-card__note msp-card__note--ok"
              :data-testid="`msp-${tab}-${i}-note`"
              >{{ groupNote(c) }}</span
            >
            <span
              v-else
              class="msp-card__note msp-card__note--warn"
              :data-testid="`msp-${tab}-${i}-reject`"
              >{{ t(`不符合：${rejectText(c)}`, `Rejected: ${rejectText(c)}`) }}</span
            >
            <span
              v-if="lockMiss(c)"
              class="msp-card__note msp-card__note--warn"
              :data-testid="`msp-${tab}-${i}-lock-miss`"
              >{{
                t(`版本锁不收（${lockMiss(c)}）`, `Not the locked version (${lockMiss(c)})`)
              }}</span
            >
            <span v-if="followingHere(c)" class="msp-card__note msp-card__note--info">{{
              t("保存时从这个种子开始追", "Saving starts from this torrent")
            }}</span>
            <span class="msp-card__member">{{ memberNote(selMember(c)) }}</span>
            <span v-if="c.members.length > 1" class="msp-card__more">{{
              t(`共 ${c.members.length} 个站点有`, `On ${c.members.length} sites`)
            }}</span>
          </button>
        </template>
        <template v-else>
          <p v-if="!unmatched.length" class="mobile-state">{{ t("没有", "None") }}</p>
          <button
            v-for="(m, i) in unmatched"
            :key="`${m.site}:${m.torrent_id}`"
            type="button"
            class="msp-card"
            :data-testid="`msp-unmatched-${i}`"
            @click="openUnmatched(m)">
            <span class="msp-card__title">{{ m.title }}</span>
            <span v-if="m.subtitle" class="msp-card__sub">{{ m.subtitle }}</span>
            <span class="msp-card__note msp-card__note--warn">{{ m.reason }}</span>
            <span class="msp-card__member">{{ m.site }}</span>
          </button>
          <p
            v-if="(result.unmatched_count ?? 0) > unmatched.length"
            class="mobile-note"
            data-testid="msp-unmatched-more">
            {{
              t(
                `只列出前 ${unmatched.length} 条，一共 ${result.unmatched_count} 条`,
                `Showing the first ${unmatched.length} of ${result.unmatched_count}`,
              )
            }}
          </p>
        </template>
      </template>
    </div>
  </el-drawer>

  <el-drawer
    v-model="actionOpen"
    direction="btt"
    size="auto"
    append-to-body
    :with-header="false"
    data-testid="msp-actions">
    <div v-if="actionMember" class="msp-actions">
      <p class="msp-actions__title" data-testid="msp-actions-title">{{ actionMember.title }}</p>
      <el-radio-group
        v-if="actionCard && actionCard.members.length > 1"
        :model-value="selIndex(actionCard)"
        class="msp-actions__members"
        @update:model-value="(v: string | number | boolean | undefined) => select(Number(v))">
        <el-radio
          v-for="(m, i) in actionCard.members"
          :key="`${m.site}:${m.torrent_id}`"
          :value="i"
          :data-testid="`msp-member-${i}`">
          {{ memberNote(m) }}
        </el-radio>
      </el-radio-group>

      <template v-if="picker">
        <p class="mobile-note">
          {{
            t(
              "点这一段的第一块与最后一块（只要一块时点两次）",
              "Tap the first and last piece (twice for one piece)",
            )
          }}
        </p>
        <div class="msp-tokens">
          <button
            v-for="(tok, i) in picker.tokens"
            :key="i"
            type="button"
            class="msp-token"
            :class="{ 'is-on': inRange(i) }"
            :data-testid="`msp-token-${i}`"
            @click="pickToken(i)">
            {{ tok.text }}
          </button>
        </div>
        <p v-if="pickedText" class="msp-picked" data-testid="msp-picked">{{ pickedText }}</p>
        <p v-if="pickedEpisode" class="msp__stale" role="alert" data-testid="msp-picked-warn">
          {{
            t(
              "这一段里有集号：加进标题包含以后只收这一集",
              "This part has an episode number: as an include it only takes that episode",
            )
          }}
        </p>
        <div class="msp-actions__row">
          <el-button
            :disabled="!pickedText"
            data-testid="msp-pick-include"
            @click="usePicked('include')"
            >{{ t("加进标题包含", "Add to includes") }}</el-button
          >
          <el-button
            :disabled="!pickedText"
            data-testid="msp-pick-exclude"
            @click="usePicked('exclude')"
            >{{ t("加进标题排除", "Add to excludes") }}</el-button
          >
        </div>
      </template>
      <div v-else class="msp-actions__list">
        <el-button data-testid="msp-copy-title" @click="copy(actionMember.title, 'title')">{{
          t("复制标题", "Copy title")
        }}</el-button>
        <el-button
          v-if="actionMember.subtitle"
          data-testid="msp-copy-subtitle"
          @click="copy(actionMember.subtitle, 'subtitle')">
          {{ t("复制副标题", "Copy subtitle") }}
        </el-button>
        <template v-if="actionCard">
          <el-button v-if="group" data-testid="msp-only" @click="only(group)">{{
            t(`只要「${group}」`, `Only "${group}"`)
          }}</el-button>
          <el-button v-if="group" data-testid="msp-not" @click="not(group)">{{
            t(`不要「${group}」`, `Not "${group}"`)
          }}</el-button>
          <el-button data-testid="msp-pick" @click="openPicker">{{
            t("选标题里的一段…", "Pick part of the title…")
          }}</el-button>
          <el-button
            v-if="tv"
            :disabled="lockedHere(actionCard)"
            data-testid="msp-lock-this"
            @click="lockTo(actionCard)">
            {{
              lockedHere(actionCard)
                ? t("已锁这个版本", "Locked to this version")
                : t("锁这个版本", "Lock this version")
            }}
          </el-button>
          <el-button v-if="canFollow" data-testid="msp-follow" @click="followFrom(actionCard)">
            {{
              followingHere(actionCard)
                ? t("保存时从它开始追", "Saving starts from it")
                : t("保存时从这个种子开始追", "Start from this torrent when saving")
            }}
          </el-button>
        </template>
        <el-button text data-testid="msp-actions-close" @click="done">{{
          t("关闭", "Close")
        }}</el-button>
      </div>
    </div>
  </el-drawer>
</template>

<style scoped>
.msp {
  display: flex;
  flex-direction: column;
  gap: 12px;
  min-width: 0;
  /* 和表单项的内容一样大（14）；不写的话没设字号的字按浏览器的 16 */
  font-size: var(--pt-fz-body-lg);
}
.msp__hint {
  margin: 0;
}
.msp__loading,
.msp__bar {
  display: flex;
  align-items: center;
  gap: 12px;
}
.msp__loading span {
  flex: 1;
  color: var(--pt-t2);
}
.msp__bar .el-button {
  flex: 1;
}
.msp__stale {
  padding: 8px 12px;
  border-radius: var(--pt-r-sm);
  background: var(--pt-hover);
  color: var(--pt-warn);
  margin: 0;
}
.msp__facts {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: 4px 12px;
  margin: 0;
  font-size: var(--pt-fz-sm);
}
.msp__facts dt {
  color: var(--pt-t3);
}
.msp__facts dd {
  margin: 0;
  color: var(--pt-t1);
  overflow-wrap: anywhere;
}
.msp__errors {
  color: var(--pt-dang);
  font-size: var(--pt-fz-sm);
}
/*
 * 和网页的试搜索一样用 pt-seg（atoms.css）：选中的一格是抬起的「纸」（surface + t1），不是主色实底配白字——
 * App 的深色主题里主色是浅色墨，白字压上去看不见。手机上按 44 高、13 号字（和按钮一样）。
 */
.msp__tabs.pt-seg {
  width: 100%;
  min-height: 44px;
  font-size: var(--pt-fz-body);
}
/* 三栏平分 350 宽，内边距收窄；再长（电影、英文、多位数）就折成两行，不截断 */
.msp__tabs.pt-seg :deep(.el-segmented__item) {
  padding: 6px 4px;
}
.msp__tabs.pt-seg :deep(.el-segmented__item-label) {
  white-space: normal;
  line-height: 1.3;
}
.msp-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  width: 100%;
  min-height: 44px;
  padding: 12px 14px;
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
  background: var(--pt-surface);
  color: var(--pt-t1);
  text-align: left;
  font: inherit;
  cursor: pointer;
}
.msp-card:active {
  background: var(--pt-hover);
}
.msp-card__title {
  font-weight: 600;
  overflow-wrap: anywhere;
}
.msp-card__sub,
.msp-card__tags,
.msp-card__member,
.msp-card__more {
  color: var(--pt-t2);
  font-size: var(--pt-fz-sm);
  overflow-wrap: anywhere;
}
.msp-card__note {
  font-size: var(--pt-fz-sm);
}
.msp-card__note--ok {
  color: var(--pt-ok);
}
.msp-card__note--warn {
  color: var(--pt-warn);
}
.msp-card__note--info {
  color: var(--pt-info);
}
.msp-actions {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 4px 0 max(8px, env(safe-area-inset-bottom));
  font-size: var(--pt-fz-body-lg);
}
.msp-actions__title {
  margin: 0;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.msp-actions__members {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 4px;
}
.msp-actions__members .el-radio {
  height: auto;
  min-height: 44px;
  margin: 0;
  white-space: normal;
}
.msp-actions__list,
.msp-actions__row {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.msp-actions__list .el-button,
.msp-actions__row .el-button {
  width: 100%;
  margin: 0;
  min-height: 44px;
}
.msp-tokens {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.msp-token {
  min-height: 44px;
  padding: 4px 10px;
  border: 1px solid var(--pt-border-strong);
  border-radius: var(--pt-r-sm);
  background: var(--pt-surface);
  color: var(--pt-t1);
  font: inherit;
  cursor: pointer;
}
.msp-token.is-on {
  border-color: var(--pt-p);
  background: var(--pt-p);
  color: var(--pt-on-p);
}
.msp-picked {
  margin: 0;
  padding: 8px 12px;
  border-radius: var(--pt-r-sm);
  background: var(--pt-hover);
  font-family: var(--pt-font-mono, monospace);
  overflow-wrap: anywhere;
}
</style>
