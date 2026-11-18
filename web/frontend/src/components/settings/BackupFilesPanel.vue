<script setup lang="ts">
/*
 * 全局设置页的「备份与恢复」面板（媒体中心 M0）：最近一次备份与恢复的结果、待恢复、备份文件与它们的操作。
 * 备份设置在上面「全局配置」表单里的「备份」一节，和别的设置一起保存；这里的操作各自马上生效。
 * 恢复只是校验后放进待恢复，重启 pt-tools 时才换上，重启以前可以取消。画板没有这一页，沿用 API 令牌页的样式：
 * 面板 + 表格，手机上是行卡。
 */
import {
  type BackupItem,
  type BackupsView,
  backupApi,
  backupFileURL,
  type PendingRestore,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatBytes, formatShortDateTime } from "@/utils/format";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, ref } from "vue";

const props = defineProps<{
  /** 读回来的备份页；还没读到或读不到时为 null */
  view: BackupsView | null;
  loading: boolean;
  /** 读取失败的原因；为空表示没失败 */
  errorText: string;
}>();

const emit = defineEmits<{
  /** 操作成功以后服务端回的最新备份页 */
  update: [view: BackupsView];
  reload: [];
}>();

const isMobile = useIsMobile();

/** 正在做的操作：同一时间只做一个，按钮跟着禁用（服务端同样只做一个） */
type Busy = "" | "create" | "upload" | "cancel" | `restore:${string}` | `delete:${string}`;
const busy = ref<Busy>("");
/** 上传进度（0–1）；没在上传时为 null */
const uploadProgress = ref<number | null>(null);
const fileInput = ref<HTMLInputElement | null>(null);

const backups = computed(() => props.view?.backups ?? []);

/** 启动时恢复的结果只在一周之内提示：再往后就是历史了 */
const RESULT_DAYS = 7;
function recent(at: string | undefined | null): boolean {
  if (!at) return false;
  const t = new Date(at).getTime();
  return Number.isFinite(t) && Date.now() - t < RESULT_DAYS * 86400_000;
}

const restoreResult = computed(() => {
  const r = props.view?.restore;
  return r && recent(r.at) ? r : null;
});

/** 最近一次备份没做成时提示（成功以后自然消失） */
const lastFailure = computed(() => {
  const l = props.view?.last;
  return l?.error ? l : null;
});

const nextText = computed(() => {
  const v = props.view;
  if (!v) return "";
  if (!v.settings.enabled) return "定时备份已关闭";
  return v.next_at ? `下次定时备份：${formatShortDateTime(v.next_at)}` : "";
});

function keyActionText(p: PendingRestore): string {
  switch (p.key_action) {
    case "keep-env":
      return "恢复以后仍用环境变量 PT_TOOLS_SECRET_KEY 的密钥";
    case "use-backup":
      return "secret.key 会换成备份里的密钥，原来的一起挪进 pre-restore 目录";
    default:
      return p.no_cipher
        ? "这份备份里没有加密保存的凭证，不需要密钥；secret.key 不变"
        : "secret.key 不变：现在的密钥解得开这份备份";
  }
}

function errText(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback;
}

/** 立即备份；失败时也重新读一遍，让「最近一次备份没做成」的提示出现 */
async function createNow() {
  busy.value = "create";
  try {
    const v = await backupApi.create();
    emit("update", v);
    ElMessage.success(`已备份：${v.last?.name ?? ""}`);
  } catch (e) {
    ElMessage.error(errText(e, "备份失败"));
    emit("reload");
  } finally {
    busy.value = "";
  }
}

const RESTORE_HINT =
  "校验通过以后放进待恢复，重启 pt-tools 时换上：现在的数据库（做这份备份以后的改动）会被换下来，" +
  "挪进数据目录的 backups/pre-restore-<时间> 文件夹。重启以前可以取消。";

async function restore(b: BackupItem) {
  try {
    await ElMessageBox.confirm(`恢复「${b.name}」？${RESTORE_HINT}`, "从备份恢复", {
      type: "warning",
      confirmButtonText: "校验并放进待恢复",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  busy.value = `restore:${b.name}`;
  try {
    emit("update", await backupApi.restore(b.name));
    ElMessage.success("已放进待恢复，重启 pt-tools 以后生效");
  } catch (e) {
    ElMessage.error(`不能恢复：${errText(e, "请求失败")}`);
  } finally {
    busy.value = "";
  }
}

async function remove(b: BackupItem) {
  try {
    await ElMessageBox.confirm(`删除备份「${b.name}」？删掉以后不能用它恢复。`, "删除备份", {
      type: "warning",
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      confirmButtonClass: "el-button--danger",
    });
  } catch {
    return;
  }
  busy.value = `delete:${b.name}`;
  try {
    emit("update", await backupApi.remove(b.name));
    ElMessage.success("已删除");
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  } finally {
    busy.value = "";
  }
}

async function cancelRestore() {
  busy.value = "cancel";
  try {
    emit("update", await backupApi.cancelRestore());
    ElMessage.success("已取消恢复");
  } catch (e) {
    ElMessage.error(errText(e, "取消失败"));
  } finally {
    busy.value = "";
  }
}

function pickFile() {
  fileInput.value?.click();
}

/** 选好文件：确认以后上传，校验通过就放进待恢复 */
async function onFile(ev: Event) {
  const input = ev.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  try {
    await ElMessageBox.confirm(
      `上传「${file.name}」（${formatBytes(file.size)}）恢复？${RESTORE_HINT}`,
      "上传备份恢复",
      { type: "warning", confirmButtonText: "上传并校验", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  busy.value = "upload";
  uploadProgress.value = 0;
  try {
    const v = await backupApi.upload(file, (r) => {
      uploadProgress.value = r;
    });
    emit("update", v);
    ElMessage.success("已放进待恢复，重启 pt-tools 以后生效");
  } catch (e) {
    ElMessage.error(`不能恢复：${errText(e, "上传失败")}`);
  } finally {
    busy.value = "";
    uploadProgress.value = null;
  }
}

const uploadLabel = computed(() =>
  uploadProgress.value === null
    ? "上传备份恢复"
    : uploadProgress.value < 1
      ? `上传中 ${Math.round(uploadProgress.value * 100)}%`
      : "校验中…",
);

function reasonTone(b: BackupItem): "info" | "primary" | "neutral" {
  if (b.reason === "pre-upgrade") return "primary";
  if (b.reason === "scheduled") return "neutral";
  return "info";
}

/** 程序版本：从不记版本号的旧版本升上来时，升级前备份记的是 unknown */
function appVersion(v: string): string {
  return v && v !== "unknown" ? v : "未知版本";
}

function versionText(b: BackupItem): string {
  return `${appVersion(b.app_version)} · 数据库 v${b.schema_version}`;
}

/** 待恢复那一条：做备份的时间、原因与版本，什么时候生效，密钥怎么处理（拼成一句，模板里不折行，免得中文里多出空格） */
function pendingDetail(p: PendingRestore): string {
  const when = formatShortDateTime(p.created_at);
  return `（${when}，${p.reason_label || "备份"}，${appVersion(p.app_version)}），重启 pt-tools 以后生效。${keyActionText(p)}。`;
}
</script>

<template>
  <PtPanel title="备份与恢复" icon="database" :count="backups.length" data-testid="backup-panel">
    <!-- 手机上页头放不下两枚按钮（标题会被挤成「备份与…」），挪到正文顶上 -->
    <template v-if="!isMobile" #actions>
      <span class="bk-acts">
        <el-button
          link
          type="primary"
          :disabled="!view || busy !== ''"
          :loading="busy === 'upload'"
          data-testid="backup-upload"
          @click="pickFile">
          <PtIcon v-if="busy !== 'upload'" name="upload" :size="14" /><span>{{ uploadLabel }}</span>
        </el-button>
        <el-button
          link
          type="primary"
          :disabled="!view || (busy !== '' && busy !== 'create')"
          :loading="busy === 'create'"
          data-testid="backup-create"
          @click="createNow">
          <PtIcon v-if="busy !== 'create'" name="save" :size="14" /><span>{{
            busy === "create" ? "正在备份…" : "立即备份"
          }}</span>
        </el-button>
      </span>
    </template>

    <input
      ref="fileInput"
      type="file"
      accept=".gz,.tgz,application/gzip"
      class="bk-file"
      aria-label="选择要恢复的备份文件"
      data-testid="backup-file"
      @change="onFile" />
    <div v-if="isMobile" class="bk-macts">
      <el-button
        type="primary"
        :disabled="!view || (busy !== '' && busy !== 'create')"
        :loading="busy === 'create'"
        data-testid="backup-create-mobile"
        @click="createNow"
        >{{ busy === "create" ? "正在备份…" : "立即备份" }}</el-button
      >
      <el-button
        :disabled="!view || busy !== ''"
        :loading="busy === 'upload'"
        data-testid="backup-upload-mobile"
        @click="pickFile"
        >{{ uploadLabel }}</el-button
      >
    </div>

    <!-- 读不回来：状态块顶掉内容（读取失败不当成「还没有备份」） -->
    <PtDataState
      v-if="!view"
      :state="errorText ? 'error' : 'loading'"
      :sub="errorText"
      data-testid="backup-state">
      <template v-if="errorText" #action>
        <el-button size="small" @click="emit('reload')">
          <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
        </el-button>
      </template>
    </PtDataState>

    <template v-else>
      <div
        v-if="view.pending"
        class="pt-note pt-note--warn bk-note bk-note--act"
        data-testid="backup-pending">
        <PtIcon name="history" :size="14" class="pt-note__icon" />
        <span class="bk-note__text"
          ><b>已放好待恢复的备份「{{ view.pending.name }}」</b
          >{{ pendingDetail(view.pending) }}</span
        >
        <el-button
          size="small"
          :loading="busy === 'cancel'"
          :disabled="busy !== '' && busy !== 'cancel'"
          data-testid="backup-cancel-restore"
          @click="cancelRestore"
          >取消恢复</el-button
        >
      </div>

      <div
        v-if="restoreResult?.error"
        class="pt-note pt-note--dang bk-note"
        data-testid="backup-restore-failed">
        <PtIcon name="circle-x" :size="14" class="pt-note__icon" />
        <span class="bk-note__text"
          >{{ formatShortDateTime(restoreResult.at) }} 启动时没能换上备份「{{
            restoreResult.name
          }}」：{{ restoreResult.error }}。已经放回原来的文件、按原来的数据启动。</span
        >
      </div>
      <div
        v-else-if="restoreResult"
        class="pt-note pt-note--ok bk-note"
        data-testid="backup-restore-ok">
        <PtIcon name="circle-check" :size="14" class="pt-note__icon" />
        <span class="bk-note__text"
          >{{ formatShortDateTime(restoreResult.at) }} 启动时换上了备份「{{
            restoreResult.name
          }}」<template v-if="restoreResult.dir"
            >，换下来的原文件在 <code>{{ restoreResult.dir }}</code></template
          >。</span
        >
      </div>

      <div
        v-if="lastFailure"
        class="pt-note pt-note--dang bk-note"
        data-testid="backup-last-failed">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span class="bk-note__text"
          >{{ formatShortDateTime(lastFailure.at) }} 的{{ lastFailure.reason_label }}没做成：{{
            lastFailure.error
          }}。{{
            lastFailure.last_ok_at
              ? `上一次成功的备份在 ${formatShortDateTime(lastFailure.last_ok_at)}。`
              : "还没有成功做过备份。"
          }}</span
        >
      </div>

      <div
        v-if="view.list_error"
        class="pt-note pt-note--dang bk-note"
        data-testid="backup-list-error">
        <PtIcon name="circle-alert" :size="14" class="pt-note__icon" />
        <span class="bk-note__text"
          >读不出备份目录 <code>{{ view.dir }}</code
          >：{{ view.list_error }}</span
        >
      </div>

      <PtDataState
        v-if="!backups.length && !view.list_error"
        state="empty"
        title="还没有备份"
        :sub="
          view.settings.enabled
            ? `定时备份每天 ${view.settings.time} 做一份；也可以点「立即备份」`
            : '定时备份关着；点「立即备份」做一份'
        "
        data-testid="backup-empty" />

      <el-table
        v-else-if="backups.length && !isMobile"
        :data="backups"
        row-key="name"
        class="pt-grid"
        data-testid="backup-table">
        <el-table-column label="备份" min-width="240" class-name="pt-cell-1line">
          <template #default="{ row }">
            <div class="bk-strong">
              {{ row.created_at ? formatShortDateTime(row.created_at) : "读不出的文件" }}
            </div>
            <div
              class="bk-sub bk-1line"
              :class="{ 'bk-err': row.error }"
              :title="row.error ? `${row.name}：${row.error}` : row.name">
              {{ row.error || row.name }}
            </div>
          </template>
        </el-table-column>
        <el-table-column label="原因" width="120">
          <template #default="{ row }">
            <PtStatusPill v-if="row.reason_label" :tone="reasonTone(row)" size="sm">{{
              row.reason_label
            }}</PtStatusPill>
          </template>
        </el-table-column>
        <el-table-column label="版本" min-width="200" class-name="pt-cell-1line">
          <template #default="{ row }">{{ row.error ? "—" : versionText(row) }}</template>
        </el-table-column>
        <el-table-column label="大小" width="100">
          <template #default="{ row }">{{ formatBytes(row.size) }}</template>
        </el-table-column>
        <el-table-column label="密钥" width="96">
          <template #default="{ row }">
            <template v-if="!row.error">
              <PtStatusPill v-if="row.has_secret" tone="warn" size="sm">带密钥</PtStatusPill>
              <span v-else class="bk-sub">不带</span>
            </template>
          </template>
        </el-table-column>
        <el-table-column label="" width="190" align="right">
          <template #default="{ row }">
            <a
              v-if="!row.error"
              class="el-button el-button--small is-link bk-link"
              :href="backupFileURL(row.name)"
              :download="row.name"
              >下载</a
            >
            <el-button
              v-if="!row.error"
              link
              type="primary"
              :loading="busy === `restore:${row.name}`"
              :disabled="busy !== '' && busy !== `restore:${row.name}`"
              :data-testid="`backup-restore-${row.name}`"
              @click="restore(row)"
              >恢复</el-button
            >
            <el-button
              link
              type="danger"
              :loading="busy === `delete:${row.name}`"
              :disabled="busy !== '' && busy !== `delete:${row.name}`"
              :data-testid="`backup-delete-${row.name}`"
              @click="remove(row)"
              >删除</el-button
            >
          </template>
        </el-table-column>
      </el-table>

      <div v-else-if="backups.length" class="bk-cards">
        <PtRowCard v-for="b in backups" :key="b.name">
          <template #title>{{
            b.created_at ? formatShortDateTime(b.created_at) : "读不出的文件"
          }}</template>
          <template #meta>
            <span class="bk-name">{{ b.name }}</span>
            <span v-if="b.error" class="bk-err">{{ b.error }}</span>
            <template v-else>
              <span>{{ versionText(b) }}</span>
              <span>{{ formatBytes(b.size) }} · {{ b.has_secret ? "带密钥" : "不带密钥" }}</span>
            </template>
          </template>
          <template #status>
            <PtStatusPill v-if="b.reason_label" :tone="reasonTone(b)" size="sm">{{
              b.reason_label
            }}</PtStatusPill>
          </template>
          <template #actions>
            <a
              v-if="!b.error"
              class="el-button el-button--small bk-link"
              :href="backupFileURL(b.name)"
              :download="b.name"
              >下载</a
            >
            <el-button
              v-if="!b.error"
              size="small"
              :loading="busy === `restore:${b.name}`"
              :disabled="busy !== '' && busy !== `restore:${b.name}`"
              @click="restore(b)"
              >恢复</el-button
            >
            <el-button
              size="small"
              type="danger"
              plain
              :loading="busy === `delete:${b.name}`"
              :disabled="busy !== '' && busy !== `delete:${b.name}`"
              @click="remove(b)"
              >删除</el-button
            >
          </template>
        </PtRowCard>
      </div>

      <ul class="bk-tip">
        <li>
          一份备份是一个
          <code>.tar.gz</code>：数据库的快照（服务运行时也能做），选了「带上密钥」时还有
          secret.key。手动备份不会被自动删掉。
        </li>
        <li>
          恢复不会马上改数据：放进待恢复以后重启 pt-tools
          才换上；换上以后用当前的密钥复验，任何一步不对都放回原来的文件。
        </li>
        <li>
          命令行也能做：<code>pt-tools db backup</code> 与
          <code>pt-tools db restore --input 文件</code>。
        </li>
      </ul>
    </template>

    <template #footer>
      <span class="pt-foot-note"
        ><template v-if="view"
          >备份目录 <code>{{ view.dir }}</code></template
        ><template v-if="nextText"> · {{ nextText }}</template></span
      >
    </template>
  </PtPanel>
</template>

<style scoped>
.bk-acts {
  display: inline-flex;
  gap: 16px;
  align-items: center;
}

.bk-file {
  display: none;
}

/* 手机：两枚按钮等分一行，高度按触控目标 */
.bk-macts {
  display: flex;
  gap: var(--pt-space-2);
  margin-bottom: 12px;
}

.bk-macts .el-button {
  flex: 1;
  min-height: var(--pt-m-touch);
  margin: 0;
}

.bk-note {
  margin-bottom: 12px;
  align-items: flex-start;
}

.bk-note__text {
  flex: 1 1 auto;
  min-width: 0;
  overflow-wrap: anywhere;
}

/* 说明块里的 code 由 atoms.css 管；列表下面的说明与页脚里的路径照同一个样子（命令不在中间断开） */
.bk-tip code {
  white-space: nowrap;
}

.bk-tip code,
.pt-foot-note code {
  padding: 1px 5px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

.bk-strong {
  font-weight: 600;
  color: var(--pt-t1);
}

.bk-sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.bk-name {
  overflow-wrap: anywhere;
}

/* 表里的文件名单行截断，完整的名字在 title 里 */
.bk-1line {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bk-err {
  color: var(--pt-dang);
}

/* 下载是个链接（浏览器带着会话直接下），样子跟旁边的文字按钮一样 */
.bk-link {
  text-decoration: none;
}

.bk-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/*
 * 行卡的操作是三枚（下载、恢复、删除）：行卡给每枚按钮的 96 最小宽度是按两枚定的，三枚在 375 宽的屏上
 * 会把「删除」挤到第二行、撑满一整行。这里三枚等分一行，高度仍是触控目标。
 */
.bk-cards :deep(.pt-rowcard__actions .el-button) {
  min-width: 0;
}

/* 手机上待恢复那条的「取消恢复」放到文字下面、占满一行，不把文字挤成一窄条 */
@media (max-width: 768px) {
  .bk-note--act {
    flex-wrap: wrap;
  }

  /* 文字按 0 起算宽度，和图标留在第一行；按钮占满一行换到下面 */
  .bk-note--act .bk-note__text {
    flex: 1 1 0;
  }

  .bk-note--act .el-button {
    flex: 1 0 100%;
    min-height: var(--pt-m-touch);
    margin: 4px 0 0;
  }
}

.bk-tip {
  margin: 12px 0 0;
  padding-left: 1.2em;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}
</style>
