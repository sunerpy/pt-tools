<script setup lang="ts">
/*
 * 远程访问（路线图 M15）：手机 App 扫码配对以后，经直连地址或 relay 连回这台 pt-tools，调用 App 接口。
 * 连接端到端加密（Noise），relay 只转发看不到内容。设备的权限是配对时选的（完全控制或只读），可以改、可以撤销，
 * 改了或撤销以后它的连接马上断开。画板没有这一页，沿用 qB 兼容入口页的形状：左边设置、右边状态，下面是设备。
 */
import {
  type RemoteDevice,
  type RemoteOverview,
  type RemotePairing,
  type RemotePairingStatus,
  type RemoteScope,
  remoteApi,
} from "@/api";
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
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

const isMobile = useIsMobile();
const ds = useDataState();
const overview = ref<RemoteOverview | null>(null);
/** 模板里用的设置与状态（读到之前是空的，那时模板显示加载状态，不会用到它） */
const EMPTY_OVERVIEW: RemoteOverview = {
  enabled: false,
  relays: [],
  direct_url: "",
  relay_status: [],
  sessions: 0,
  stream_path: "",
};
const ov = computed<RemoteOverview>(() => overview.value ?? EMPTY_OVERVIEW);
const devices = ref<RemoteDevice[]>([]);
const form = ref({ enabled: false, relaysText: "", direct_url: "" });
const saving = ref(false);

const FULL: RemoteScope[] = ["app:read", "app:write"];
const READ: RemoteScope[] = ["app:read"];

const PERMISSIONS = [
  { value: "full", label: "完全控制", hint: "查看，并且能暂停、删除、推送种子，签到，管理订阅" },
  { value: "read", label: "只读", hint: "只能查看概览、站点、种子、推送记录与订阅" },
] as const;

const RELAY_STATES: Record<string, { label: string; tone: "ok" | "warn" | "dang" }> = {
  online: { label: "已连上", tone: "ok" },
  connecting: { label: "正在连", tone: "warn" },
  offline: { label: "断开了", tone: "dang" },
};

const VIA_LABELS: Record<string, string> = { direct: "直连", relay: "relay" };

/** el-table 收集列的时候会拿空的 row 调一次插槽，scopes 可能是 undefined */
function isFull(scopes: readonly string[] | undefined): boolean {
  return (scopes ?? []).includes("app:write");
}

function errText(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback;
}

function fillForm(ov: RemoteOverview) {
  form.value = { enabled: ov.enabled, relaysText: ov.relays.join("\n"), direct_url: ov.direct_url };
}

async function load() {
  const pending = ds.run(async () => {
    const [ov, list] = await Promise.all([remoteApi.get(), remoteApi.devices()]);
    return { ov, list };
  });
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  overview.value = got.ov;
  devices.value = got.list;
  fillForm(got.ov);
}

async function loadDevices() {
  try {
    devices.value = await remoteApi.devices();
  } catch (e) {
    ElMessage.error(errText(e, "读取设备失败"));
  }
  await refreshStatus();
}

/** 只刷新右边的状态（在线的连接数、relay），不动左边没保存的设置 */
async function refreshStatus() {
  try {
    overview.value = await remoteApi.get();
  } catch {
    // 下次刷新再读
  }
}

function relayList(): string[] {
  return form.value.relaysText
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
}

async function save() {
  saving.value = true;
  try {
    const ov = await remoteApi.save({
      enabled: form.value.enabled,
      relays: relayList(),
      direct_url: form.value.direct_url.trim(),
    });
    overview.value = ov;
    fillForm(ov);
    ElMessage.success(ov.enabled ? "已保存，远程访问已开启" : "已保存，远程访问已关闭");
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}

function addDefaultRelay() {
  const def = overview.value?.default_relay;
  if (!def) return;
  const list = relayList();
  if (!list.includes(def)) list.push(def);
  form.value.relaysText = list.join("\n");
}

async function rotate() {
  try {
    await ElMessageBox.confirm(
      "换一套主机密钥以后，所有配对过的设备都会被撤销，要重新扫码配对。只在怀疑密钥泄露（比如备份文件外流）时用。",
      "轮换主机密钥",
      {
        type: "warning",
        confirmButtonText: "轮换",
        cancelButtonText: "取消",
        confirmButtonClass: "el-button--danger",
      },
    );
  } catch {
    return;
  }
  try {
    overview.value = await remoteApi.rotateKeys();
    await loadDevices();
    ElMessage.success("已轮换，设备都要重新配对");
  } catch (e) {
    ElMessage.error(errText(e, "轮换失败"));
  }
}

// ---- 添加设备 ----
const pairDialog = ref(false);
const pairForm = ref<{ permission: "full" | "read"; direct_url: string }>({
  permission: "full",
  direct_url: "",
});
const pairing = ref<RemotePairing | null>(null);
const pairStatus = ref<RemotePairingStatus | null>(null);
const starting = ref(false);
const now = ref(Date.now());
let timer: ReturnType<typeof setInterval> | undefined;
let polling = false;

function openPair() {
  pairing.value = null;
  pairStatus.value = null;
  pairForm.value = {
    permission: "full",
    direct_url: overview.value?.direct_url || window.location.origin,
  };
  pairDialog.value = true;
}

async function startPairing() {
  starting.value = true;
  try {
    pairing.value = await remoteApi.startPairing({
      scopes: pairForm.value.permission === "full" ? FULL : READ,
      direct_url: pairForm.value.direct_url.trim(),
    });
    pairStatus.value = { state: "waiting", failures: 0, expires_at: pairing.value.expires_at };
    startTimer();
  } catch (e) {
    ElMessage.error(errText(e, "生成二维码失败"));
  } finally {
    starting.value = false;
  }
}

function startTimer() {
  stopTimer();
  now.value = Date.now();
  timer = setInterval(tick, 1000);
}

function stopTimer() {
  if (timer) clearInterval(timer);
  timer = undefined;
}

/** 每秒走一下倒计时；每 2 秒问一次配对有没有完成（上一次还没回来就跳过） */
async function tick() {
  now.value = Date.now();
  if (polling || Math.floor(now.value / 1000) % 2 !== 0) return;
  polling = true;
  try {
    const st = await remoteApi.pairingStatus();
    if (!pairDialog.value || !pairing.value) return;
    pairStatus.value = st;
    if (st.state !== "waiting") {
      stopTimer();
      if (st.state === "paired") {
        ElMessage.success(`「${st.device?.name ?? "新设备"}」已配对`);
        await loadDevices();
      }
    }
  } catch {
    // 下一次再问
  } finally {
    polling = false;
  }
}

const remaining = computed(() => {
  const exp = pairStatus.value?.expires_at ?? pairing.value?.expires_at;
  if (!exp) return 0;
  return Math.max(0, Math.floor((new Date(exp).getTime() - now.value) / 1000));
});

const countdown = computed(() => {
  const s = remaining.value;
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
});

const pairState = computed(() => {
  const st = pairStatus.value?.state;
  if (st === "waiting" && remaining.value === 0) return "expired";
  return st ?? "none";
});

const PAIR_STATES: Record<string, { label: string; tone: "ok" | "warn" | "dang" | "info" }> = {
  waiting: { label: "等待扫码", tone: "info" },
  paired: { label: "已配对", tone: "ok" },
  expired: { label: "已过期", tone: "warn" },
  closed: { label: "已作废", tone: "dang" },
  none: { label: "已作废", tone: "dang" },
};

const qrSrc = computed(() =>
  pairing.value
    ? `data:image/svg+xml;charset=utf-8,${encodeURIComponent(pairing.value.qr_svg)}`
    : "",
);

/** 关掉窗口时二维码作废（还在等扫码的话），免得被别人拍下来以后扫 */
async function onPairClosed() {
  stopTimer();
  const waiting = pairing.value && pairStatus.value?.state === "waiting";
  pairing.value = null;
  pairStatus.value = null;
  if (waiting) {
    try {
      await remoteApi.cancelPairing();
    } catch {
      // 十分钟以后自己过期
    }
  }
}

async function copyLink() {
  if (!pairing.value) return;
  try {
    await navigator.clipboard.writeText(pairing.value.link);
    ElMessage.success("已复制");
  } catch {
    ElMessage.warning("浏览器不让复制：请手动选中链接复制");
  }
}

// ---- 设备 ----
function seenText(d: RemoteDevice): string {
  if (d.online) return `在线 · ${VIA_LABELS[d.online_via ?? ""] ?? ""}`;
  if (!d.last_seen_at) return "还没连过";
  const via = VIA_LABELS[d.last_seen_via ?? ""];
  return `最近 ${formatShortDateTime(d.last_seen_at)}${via ? ` · ${via}` : ""}`;
}

async function rename(d: RemoteDevice) {
  let name: string;
  try {
    const res = await ElMessageBox.prompt("新的名字（最多 64 个字）", "改名", {
      inputValue: d.name,
      confirmButtonText: "保存",
      cancelButtonText: "取消",
      inputValidator: (v: string) =>
        (v.trim().length > 0 && v.trim().length <= 64) || "名字要 1 到 64 个字",
    });
    name = res.value.trim();
  } catch {
    return;
  }
  try {
    await remoteApi.updateDevice(d.id, { name });
    ElMessage.success("已改名");
    await loadDevices();
  } catch (e) {
    ElMessage.error(errText(e, "改名失败"));
  }
}

async function toggleScope(d: RemoteDevice) {
  const toFull = !isFull(d.scopes);
  try {
    await ElMessageBox.confirm(
      `改成「${toFull ? "完全控制" : "只读"}」以后，「${d.name}」现在的连接会断开，App 重连以后按新的权限。`,
      "改权限",
      { type: "warning", confirmButtonText: "改", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    await remoteApi.updateDevice(d.id, { scopes: toFull ? FULL : READ });
    ElMessage.success("已改");
    await loadDevices();
  } catch (e) {
    ElMessage.error(errText(e, "改权限失败"));
  }
}

async function revoke(d: RemoteDevice) {
  try {
    await ElMessageBox.confirm(
      `撤销「${d.name}」以后，它马上断开，再也连不上；要用的话重新扫码配对。`,
      "撤销设备",
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
    await remoteApi.revokeDevice(d.id);
    ElMessage.success("已撤销");
    await loadDevices();
  } catch (e) {
    ElMessage.error(errText(e, "撤销失败"));
  }
}

async function remove(d: RemoteDevice) {
  try {
    await remoteApi.deleteDevice(d.id);
    await loadDevices();
  } catch (e) {
    ElMessage.error(errText(e, "删除失败"));
  }
}

onMounted(load);
onBeforeUnmount(stopTimer);
</script>

<template>
  <div class="pt-cards pt-cards--main">
    <PtHeadSub
      >手机 App 扫码配对以后，在外面也能看种子、站点数据与订阅；经直连地址或 relay 连回这台
      pt-tools，内容端到端加密</PtHeadSub
    >
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="ra-refresh" @click="load">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <PtDataState
      v-if="!overview"
      class="pt-cards__full"
      :state="ds.state.value"
      :sub="ds.errorText.value"
      data-testid="ra-state" />

    <template v-else>
      <PtPanel title="设置" icon="settings" data-testid="ra-settings">
        <el-form class="pt-form" label-position="top" @submit.prevent>
          <el-form-item label="远程访问">
            <el-switch v-model="form.enabled" data-testid="ra-enabled" />
            <div class="ra-tip">
              默认关着。打开以后才有直连入口（{{ ov.stream_path }}）与 relay 连接。
            </div>
          </el-form-item>
          <el-form-item label="直连地址">
            <el-input
              v-model="form.direct_url"
              placeholder="例如 http://192.168.1.10:8080"
              data-testid="ra-direct" />
            <div class="ra-tip">
              App 和 pt-tools
              在同一个局域网，或者这个地址从外面也能访问时填；可以带反向代理的子路径。不填的话 App
              只经 relay 连。
            </div>
          </el-form-item>
          <el-form-item label="relay 地址">
            <el-input
              v-model="form.relaysText"
              type="textarea"
              :rows="3"
              placeholder="每行一个 wss:// 地址，最多 4 个"
              data-testid="ra-relays" />
            <div class="ra-tip">
              relay 只转发加密以后的数据，看得到这台主机的
              hostId、IP、连接时间与流量大小，看不到内容。
              <el-button
                v-if="ov.default_relay"
                link
                type="primary"
                data-testid="ra-default-relay"
                @click="addDefaultRelay"
                >填入托管 relay</el-button
              >
            </div>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button type="primary" :loading="saving" data-testid="ra-save" @click="save">
            <PtIcon name="save" :size="14" /><span>保存</span>
          </el-button>
        </template>
      </PtPanel>

      <PtPanel title="状态" icon="signal" data-testid="ra-status">
        <ul class="ra-kv">
          <li>
            <span class="ra-k">远程访问</span>
            <PtStatusPill :tone="ov.enabled ? 'ok' : 'neutral'" dot size="sm">{{
              ov.enabled ? "已开启" : "已关闭"
            }}</PtStatusPill>
          </li>
          <li v-if="ov.host_id">
            <span class="ra-k">hostId</span><code data-testid="ra-hostid">{{ ov.host_id }}</code>
          </li>
          <li>
            <span class="ra-k">在线的连接</span><span>{{ ov.sessions }} 个</span>
          </li>
          <li v-for="r in ov.relay_status" :key="r.url" class="ra-relay">
            <span class="ra-k">relay</span>
            <code>{{ r.url }}</code>
            <PtStatusPill :tone="RELAY_STATES[r.state]?.tone ?? 'warn'" size="sm">{{
              RELAY_STATES[r.state]?.label ?? r.state
            }}</PtStatusPill>
            <span v-if="r.error" class="ra-sub ra-err">{{ r.error }}</span>
          </li>
        </ul>
        <div v-if="ov.host_id" class="ra-rotate">
          <el-button link type="danger" data-testid="ra-rotate" @click="rotate"
            >轮换主机密钥</el-button
          >
          <span class="ra-sub">所有设备都要重新配对</span>
        </div>
      </PtPanel>

      <PtPanel
        class="pt-cards__full"
        title="设备"
        icon="smartphone"
        :count="devices.length"
        :action="ov.enabled ? '添加设备' : ''"
        action-icon="plus"
        data-testid="ra-devices"
        @action="openPair">
        <PtDataState
          v-if="!devices.length"
          state="empty"
          title="还没有配对的设备"
          :sub="ov.enabled ? '点「添加设备」，用 App 扫码' : '先打开远程访问并保存，再添加设备'"
          data-testid="ra-empty" />
        <el-table
          v-else-if="!isMobile"
          :data="devices"
          row-key="id"
          class="pt-grid"
          data-testid="ra-table">
          <el-table-column label="设备" min-width="180">
            <template #default="{ row }">
              <div class="ra-strong">{{ row.name }}</div>
              <div class="ra-sub">{{ formatShortDateTime(row.created_at) }} 配对</div>
            </template>
          </el-table-column>
          <el-table-column label="权限" width="120">
            <template #default="{ row }">
              <PtStatusPill :tone="isFull(row.scopes) ? 'primary' : 'info'" size="sm">{{
                isFull(row.scopes) ? "完全控制" : "只读"
              }}</PtStatusPill>
            </template>
          </el-table-column>
          <el-table-column label="状态" min-width="200">
            <template #default="{ row }">
              <PtStatusPill v-if="row.revoked_at" tone="dang" size="sm">已撤销</PtStatusPill>
              <PtStatusPill v-else-if="row.online" tone="ok" dot size="sm">{{
                seenText(row)
              }}</PtStatusPill>
              <span v-else class="ra-sub">{{ seenText(row) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="" width="240" align="right">
            <template #default="{ row }">
              <template v-if="!row.revoked_at">
                <el-button link :data-testid="`ra-rename-${row.id}`" @click="rename(row)"
                  >改名</el-button
                >
                <el-button link :data-testid="`ra-scope-${row.id}`" @click="toggleScope(row)">{{
                  isFull(row.scopes) ? "改成只读" : "改成完全控制"
                }}</el-button>
                <el-button
                  link
                  type="danger"
                  :data-testid="`ra-revoke-${row.id}`"
                  @click="revoke(row)"
                  >撤销</el-button
                >
              </template>
              <el-button v-else link :data-testid="`ra-delete-${row.id}`" @click="remove(row)"
                >删除记录</el-button
              >
            </template>
          </el-table-column>
        </el-table>
        <div v-else class="ra-cards">
          <PtRowCard v-for="d in devices" :key="d.id">
            <template #title>{{ d.name }}</template>
            <template #meta>
              <span>{{ isFull(d.scopes) ? "完全控制" : "只读" }}</span>
              <span>{{ d.revoked_at ? "已撤销" : seenText(d) }}</span>
            </template>
            <template #status>
              <PtStatusPill v-if="d.revoked_at" dot tone="dang" size="sm">已撤销</PtStatusPill>
              <PtStatusPill v-else-if="d.online" dot tone="ok" size="sm">在线</PtStatusPill>
            </template>
            <template #actions>
              <template v-if="!d.revoked_at">
                <el-button size="small" @click="rename(d)">改名</el-button>
                <el-button size="small" @click="toggleScope(d)">{{
                  isFull(d.scopes) ? "改成只读" : "改成完全控制"
                }}</el-button>
                <el-button size="small" type="danger" plain @click="revoke(d)">撤销</el-button>
              </template>
              <el-button v-else size="small" @click="remove(d)">删除记录</el-button>
            </template>
          </PtRowCard>
        </div>
        <p class="ra-tip">
          设备只能调用 App
          接口，权限是配对时选的；改权限或撤销以后，它的连接马上断开。设备做的操作记在「操作审计」里，通道是「远程设备」。
        </p>
      </PtPanel>
    </template>

    <el-dialog
      v-model="pairDialog"
      class="pt-dialog"
      title="添加设备"
      :width="isMobile ? '94%' : '520px'"
      append-to-body
      align-center
      :close-on-click-modal="false"
      data-testid="ra-pair-dialog"
      @closed="onPairClosed">
      <el-form v-if="!pairing" class="pt-form" label-position="top" @submit.prevent>
        <el-form-item label="权限">
          <el-radio-group v-model="pairForm.permission" class="ra-perms" data-testid="ra-perm">
            <el-radio v-for="p in PERMISSIONS" :key="p.value" :value="p.value">
              <span class="ra-strong">{{ p.label }}</span>
              <span class="ra-sub ra-hint">{{ p.hint }}</span>
            </el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="这次的直连地址">
          <el-input v-model="pairForm.direct_url" data-testid="ra-pair-direct" />
          <div class="ra-tip">写进二维码，App 先试这个地址；不填就只经 relay。</div>
        </el-form-item>
      </el-form>
      <div v-else class="ra-pair" data-testid="ra-pair-qr">
        <img
          class="ra-qr"
          :class="{ 'ra-qr--used': pairState !== 'waiting' }"
          :src="qrSrc"
          alt="配对二维码，用 pt-tools App 扫描"
          width="240"
          height="240" />
        <div class="ra-pair__state">
          <PtStatusPill :tone="PAIR_STATES[pairState].tone" dot data-testid="ra-pair-state">{{
            pairState === "paired" && pairStatus?.device
              ? `已配对：${pairStatus.device.name}`
              : PAIR_STATES[pairState].label
          }}</PtStatusPill>
          <span v-if="pairState === 'waiting'" class="ra-sub" data-testid="ra-countdown"
            >{{ countdown }} 后过期</span
          >
        </div>
        <el-input
          v-if="pairState === 'waiting'"
          :model-value="pairing.link"
          readonly
          data-testid="ra-link" />
        <div v-if="pairState === 'waiting'" class="pt-note pt-note--warn ra-once">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span
            >二维码只能用一次，十分钟内有效；关掉这个窗口时作废。不要发给别人，拿到它的人能配对成「{{
              pairing.scopes.includes("app:write") ? "完全控制" : "只读"
            }}」的设备。</span
          >
        </div>
      </div>
      <template #footer>
        <template v-if="!pairing">
          <el-button @click="pairDialog = false">取消</el-button>
          <el-button
            type="primary"
            :loading="starting"
            data-testid="ra-pair-start"
            @click="startPairing"
            >生成二维码</el-button
          >
        </template>
        <template v-else>
          <el-button v-if="pairState === 'waiting'" data-testid="ra-copy-link" @click="copyLink">
            <PtIcon name="copy" :size="14" /><span>复制链接</span>
          </el-button>
          <el-button
            v-if="pairState !== 'waiting'"
            data-testid="ra-pair-again"
            @click="startPairing"
            >重新生成</el-button
          >
          <el-button type="primary" data-testid="ra-pair-done" @click="pairDialog = false">{{
            pairState === "paired" ? "完成" : "关闭"
          }}</el-button>
        </template>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
/* el-form-item 的内容区是 flex：说明要占满一行，才会落在开关下面而不是挤在它右边 */
.ra-tip {
  width: 100%;
  margin: 6px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.ra-kv {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.ra-kv li {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  min-width: 0;
  font-size: 13px;
  color: var(--pt-t1);
}

.ra-k {
  flex: 0 0 96px;
  color: var(--pt-t3);
}

.ra-kv code {
  word-break: break-all;
}

/* relay 一行可能很长：错误另起一行 */
.ra-relay .ra-err {
  flex-basis: 100%;
  padding-left: 104px;
}

.ra-err {
  color: var(--pt-dang);
}

.ra-rotate {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-top: 14px;
}

.ra-strong {
  font-weight: 600;
  color: var(--pt-t1);
}

.ra-sub {
  font-size: 12px;
  color: var(--pt-t3);
}

.ra-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ra-perms {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 6px;
}

/* 权限的说明在手机上要能折行：el-radio 默认不换行，长说明会冲出弹窗右边 */
.ra-perms :deep(.el-radio) {
  height: auto;
  margin-right: 0;
  align-items: flex-start;
  white-space: normal;
}

.ra-perms :deep(.el-radio__input) {
  margin-top: 3px;
}

.ra-perms :deep(.el-radio__label) {
  line-height: 20px;
}

.ra-hint {
  margin-left: 8px;
}

.ra-pair {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.ra-qr {
  width: 240px;
  max-width: 100%;
  height: auto;
  aspect-ratio: 1;
  border: 1px solid var(--pt-border);
  border-radius: 8px;
  background: #fff;
}

/* 用过、过期、作废的二维码模糊掉，免得被当成还能扫（不用 opacity：它压暗内容，见 paletteContrast.test.ts） */
.ra-qr--used {
  filter: blur(6px) grayscale(1);
}

.ra-pair__state {
  display: flex;
  gap: 10px;
  align-items: center;
}

.ra-once {
  align-self: stretch;
}

@media (max-width: 768px) {
  .ra-relay .ra-err {
    padding-left: 0;
  }
}
</style>
