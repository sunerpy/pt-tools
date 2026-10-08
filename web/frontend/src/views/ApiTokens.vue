<script setup lang="ts">
/*
 * API 令牌（路线图 M12、M13）：给手机 App（走 /api/app/v1）与 qB 兼容入口用的令牌。新建时只显示一次明文；撤销立即生效。
 * 令牌访问不了网页的其他接口，那些仍然要登录。画板没有这一页，沿用媒体页的样式：页头 + 面板，手机上是行卡。
 */
import { type ApiToken, type ApiTokenScope, tokensApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatShortDateTime } from "@/utils/format";
import { ElMessage, ElMessageBox } from "element-plus";
import { onMounted, ref } from "vue";

const isMobile = useIsMobile();
const tokens = ref<ApiToken[]>([]);
const ds = useDataState();

/** 现在能选的权限范围：MCP 上线以后再加进来 */
const SCOPES: readonly { value: ApiTokenScope; label: string; hint: string }[] = [
  {
    value: "app:read",
    label: "读取",
    hint: "概览、站点、种子、推送记录、搜索、刷流、整理历史与订阅",
  },
  { value: "app:write", label: "操作", hint: "暂停、继续、删除种子，推送种子，签到，管理订阅" },
  {
    value: "qbit:compat",
    label: "qB 兼容",
    hint: "登录 qB 兼容入口：MoviePilot、IYUU 这类只认 qBittorrent 的工具用",
  },
];

const EXPIRY: readonly { value: number; label: string }[] = [
  { value: 30, label: "30 天" },
  { value: 90, label: "90 天" },
  { value: 365, label: "1 年" },
  { value: 0, label: "不过期" },
];

function scopeLabel(s: string): string {
  return SCOPES.find((x) => x.value === s)?.label ?? s;
}

function expired(t: ApiToken): boolean {
  return Boolean(t.expires_at && new Date(t.expires_at).getTime() <= Date.now());
}

async function load() {
  const pending = ds.run(() => tokensApi.list());
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  tokens.value = got;
}

// ---- 新建 ----
const createDialog = ref(false);
const saving = ref(false);
const form = ref<{ name: string; scopes: ApiTokenScope[]; expires_in_days: number }>({
  name: "",
  scopes: ["app:read"],
  expires_in_days: 90,
});

function openCreate() {
  form.value = { name: "", scopes: ["app:read"], expires_in_days: 90 };
  createDialog.value = true;
}

// ---- 明文只显示一次 ----
const plainDialog = ref(false);
const plaintext = ref("");

function errText(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback;
}

async function create() {
  if (!form.value.name.trim()) {
    ElMessage.warning("名字要填");
    return;
  }
  if (!form.value.scopes.length) {
    ElMessage.warning("至少选一个权限");
    return;
  }
  saving.value = true;
  try {
    const res = await tokensApi.create({
      name: form.value.name.trim(),
      scopes: [...form.value.scopes],
      expires_in_days: form.value.expires_in_days,
    });
    createDialog.value = false;
    plaintext.value = res.plaintext;
    plainDialog.value = true;
    await load();
  } catch (e) {
    ElMessage.error(errText(e, "新建失败"));
  } finally {
    saving.value = false;
  }
}

async function copyPlain() {
  try {
    await navigator.clipboard.writeText(plaintext.value);
    ElMessage.success("已复制");
  } catch {
    ElMessage.warning("浏览器不让复制：请选中上面的令牌手动复制");
  }
}

function closePlain() {
  plainDialog.value = false;
  plaintext.value = "";
}

// ---- 撤销 ----
async function revoke(t: ApiToken) {
  try {
    await ElMessageBox.confirm(
      `撤销「${t.name}」以后，用这个令牌的客户端马上就连不上了。`,
      "撤销令牌",
      {
        type: "warning",
        confirmButtonText: "撤销",
        cancelButtonText: "取消",
        confirmButtonClass: "el-button--danger",
      },
    );
  } catch {
    return;
  }
  try {
    await tokensApi.revoke(t.id);
    ElMessage.success("已撤销");
    await load();
  } catch (e) {
    ElMessage.error(errText(e, "撤销失败"));
  }
}

onMounted(load);
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub
      >给手机 App 与 qB 兼容入口这类客户端用的令牌：只能访问 App 接口与 qB
      兼容入口，网页的其他功能仍然要登录</PtHeadSub
    >
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="tk-refresh" @click="load">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <PtPanel
      title="API 令牌"
      icon="id-card"
      :count="tokens.length"
      action="新建"
      action-icon="plus"
      @action="openCreate">
      <div v-if="tokens.length && ds.error.value" class="pt-note pt-note--warn tk-stale">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>刷新失败：{{ ds.errorText.value }}。下面是上次读到的。</span>
      </div>
      <PtDataState
        v-if="!tokens.length"
        :state="ds.state.value"
        :title="ds.state.value === 'empty' ? '还没有令牌' : ''"
        :sub="ds.errorText.value || (ds.state.value === 'empty' ? '新建一个，填进手机 App 里' : '')"
        data-testid="tk-state" />
      <el-table
        v-else-if="!isMobile"
        :data="tokens"
        row-key="id"
        class="pt-grid"
        data-testid="tk-table">
        <el-table-column label="名字" min-width="160">
          <template #default="{ row }">
            <div class="tk-strong">{{ row.name }}</div>
            <div class="tk-sub">
              {{ row.created_by || "—" }} · {{ formatShortDateTime(row.created_at) }}
            </div>
          </template>
        </el-table-column>
        <el-table-column label="权限" min-width="160">
          <template #default="{ row }">
            <span class="tk-scopes">
              <PtStatusPill v-for="s in row.scopes" :key="s" tone="info" size="sm">{{
                scopeLabel(s)
              }}</PtStatusPill>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="最近一次使用" width="150">
          <template #default="{ row }">{{
            row.last_used_at ? formatShortDateTime(row.last_used_at) : "还没用过"
          }}</template>
        </el-table-column>
        <el-table-column label="过期" width="150">
          <template #default="{ row }">
            <PtStatusPill v-if="expired(row)" tone="dang" size="sm">已过期</PtStatusPill>
            <span v-else>{{
              row.expires_at ? formatShortDateTime(row.expires_at) : "不过期"
            }}</span>
          </template>
        </el-table-column>
        <el-table-column label="" width="90" align="right">
          <template #default="{ row }">
            <el-button link type="danger" :data-testid="`tk-revoke-${row.id}`" @click="revoke(row)"
              >撤销</el-button
            >
          </template>
        </el-table-column>
      </el-table>
      <div v-else class="tk-cards">
        <PtRowCard v-for="t in tokens" :key="t.id">
          <template #title>{{ t.name }}</template>
          <template #meta>
            <span>{{ t.scopes.map(scopeLabel).join("、") }}</span>
            <span>{{
              t.last_used_at ? `最近 ${formatShortDateTime(t.last_used_at)}` : "还没用过"
            }}</span>
            <span>{{ t.expires_at ? `${formatShortDateTime(t.expires_at)} 过期` : "不过期" }}</span>
          </template>
          <template #status>
            <PtStatusPill v-if="expired(t)" dot tone="dang" size="sm">已过期</PtStatusPill>
          </template>
          <template #actions>
            <el-button size="small" type="danger" plain @click="revoke(t)">撤销</el-button>
          </template>
        </PtRowCard>
      </div>
      <p class="tk-tip">
        App 里填 pt-tools
        的地址和令牌；令牌只在新建时显示一次，丢了就撤销再建一个。令牌做的操作记在「操作审计」里。
      </p>
    </PtPanel>

    <el-dialog
      v-model="createDialog"
      class="pt-dialog"
      title="新建令牌"
      :width="isMobile ? '94%' : '520px'"
      append-to-body
      align-center
      data-testid="token-dialog">
      <el-form class="pt-form" label-position="top" @submit.prevent>
        <el-form-item label="名字">
          <el-input
            v-model="form.name"
            maxlength="64"
            placeholder="例如：我的手机"
            data-testid="tk-name" />
        </el-form-item>
        <el-form-item label="权限">
          <el-checkbox-group v-model="form.scopes" class="tk-scope-list" data-testid="tk-scopes">
            <el-checkbox v-for="s in SCOPES" :key="s.value" :value="s.value">
              <span class="tk-strong">{{ s.label }}</span>
              <span class="tk-sub tk-hint">{{ s.hint }}</span>
            </el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item label="有效期">
          <el-select v-model="form.expires_in_days" data-testid="tk-expiry">
            <el-option v-for="o in EXPIRY" :key="o.value" :value="o.value" :label="o.label" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" data-testid="tk-save" @click="create"
          >新建</el-button
        >
      </template>
    </el-dialog>

    <el-dialog
      v-model="plainDialog"
      class="pt-dialog"
      title="令牌已新建"
      :width="isMobile ? '94%' : '560px'"
      append-to-body
      align-center
      :close-on-click-modal="false"
      data-testid="token-plain"
      @closed="plaintext = ''">
      <div class="pt-note pt-note--warn tk-once">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>令牌只显示这一次，关掉以后就看不到了。现在复制下来，填进 App 里。</span>
      </div>
      <el-input
        :model-value="plaintext"
        readonly
        data-testid="tk-plain"
        @focus="($event.target as HTMLInputElement).select()" />
      <template #footer>
        <el-button data-testid="tk-copy" @click="copyPlain">
          <PtIcon name="copy" :size="14" /><span>复制</span>
        </el-button>
        <el-button type="primary" data-testid="tk-done" @click="closePlain">我已经复制了</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.tk-stale {
  margin-bottom: 12px;
}

.tk-strong {
  font-weight: 600;
  color: var(--pt-t1);
}

.tk-sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.tk-scopes {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 4px;
}

.tk-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.tk-tip {
  margin: 12px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.tk-scope-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* 权限的说明在手机上要能折行：el-checkbox 默认不换行、高 32，长说明会冲出弹窗右边 */
.tk-scope-list :deep(.el-checkbox) {
  height: auto;
  margin-right: 0;
  align-items: flex-start;
  white-space: normal;
}

.tk-scope-list :deep(.el-checkbox__input) {
  margin-top: 3px;
}

.tk-scope-list :deep(.el-checkbox__label) {
  line-height: 20px;
}

.tk-hint {
  margin-left: 8px;
}

.tk-once {
  margin-bottom: 12px;
}
</style>
