<script setup lang="ts">
/*
 * 添加 / 编辑一个 NAS 来源（首页 NAS 概览 H2，方案 3.2.4、3.2.5）。字段跟着类型表里的规则（用户名一栏、
 * 要不要密钥、http:// 地址的规则）；「测试连接」与保存走服务端的同一套校验：
 *   · 证书不受信任：服务端在发出任何凭据之前就断开了，这里列出证书；用户核对指纹、点「信任这张证书」以后带指纹再测，
 *     保存时把指纹一起存下（钉住）。
 *   · 要验证码：出输入框，带验证码再测。
 *   · 登录得到的长期凭据（设备令牌等）只回一个 pending，保存时带上；它绑定这次的类型、地址与账号。
 * 改了类型、地址或账号：测试结果与 pending 作废；改了类型或地址：信任的证书与「明文风险」的确认也作废（指纹属于原来的地址）。
 * 密钥只发不收：编辑时只知道保存过没有，留空就是不改。
 */
import {
  type NasCertInfo,
  type NasKind,
  type NasSourceEdit,
  type NasSourceInput,
  type NasSystem,
  type NasTestInput,
  type NasTestResult,
  nasApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { nasKindLabel } from "@/utils/nas";
import { ElMessage } from "element-plus";
import { computed, reactive, ref, watch } from "vue";

const props = defineProps<{
  modelValue: boolean;
  /** 编辑的来源编号；新建时为 null */
  editId: number | null;
  /** 能接入的类型（GET /api/nas 的 kinds） */
  kinds: NasKind[];
}>();

const emit = defineEmits<{
  "update:modelValue": [open: boolean];
  saved: [source: NasSourceEdit];
}>();

const isMobile = useIsMobile();

const form = reactive({
  name: "",
  kind: "",
  enabled: true,
  url: "",
  username: "",
  secret: "",
  clear_secret: false,
  insecure_ack: false,
});
/** 编辑时读到的来源（新建时为空） */
const original = ref<NasSourceEdit | null>(null);
const loading = ref(false);
const loadError = ref("");
const testing = ref(false);
const saving = ref(false);
/** 最近一次测试的结果（改了类型、地址、账号或密钥以后作废） */
const result = ref<NasTestResult | null>(null);
/** 测试通过时登录得到的暂存凭据 */
const pending = ref("");
const otp = ref("");
/** 请求本身失败的原因（400、429 等）：写在表单下面 */
const problem = ref("");
/** 这次在弹窗里信任的证书：属于哪个类型与地址 */
const trusted = ref<{ target: string; sha256: string } | null>(null);
/** 点了「不再信任」：保存的指纹不再用 */
const droppedPin = ref(false);
let openSeq = 0;

function normUrl(u: string): string {
  return u.trim().replace(/\/+$/, "");
}

function targetOf(kind: string, url: string): string {
  return `${kind}\n${normUrl(url)}`;
}

function kindOf(name: string): NasKind | undefined {
  return props.kinds.find((k) => k.name === name);
}

const kind = computed(() => kindOf(form.kind));
/** 编辑的来源是这个版本不认的类型：只能删除 */
const kindMissing = computed(() => original.value !== null && !kind.value);
const title = computed(() =>
  props.editId === null ? "添加来源" : `编辑来源 · ${original.value?.name ?? ""}`,
);

const target = computed(() => targetOf(form.kind, form.url));
const originalTarget = computed(() =>
  original.value ? targetOf(original.value.kind, original.value.url) : "",
);
/** 类型、地址或账号和保存的不一样了：库里的密钥与登录凭据不再给新的目标用（同服务端的规则） */
const targetChanged = computed(
  () =>
    original.value !== null &&
    (target.value !== originalTarget.value || form.username.trim() !== original.value.username),
);

/** 保存时钉住的证书指纹：这次信任的（同一个地址），否则保存过的（地址没变、没点「不再信任」） */
const pin = computed(() => {
  if (trusted.value?.target === target.value) return trusted.value.sha256;
  const o = original.value;
  if (o?.tls_pin_sha256 && !droppedPin.value && target.value === originalTarget.value) {
    return o.tls_pin_sha256;
  }
  return "";
});

const showUsername = computed(() => !!kind.value && kind.value.username !== "none");
const showSecret = computed(() => !!kind.value && kind.value.secret !== "none");
/** 没有用户名一栏的类型，密钥是 API Key；有的是密码 */
const secretLabel = computed(() => (kind.value?.username === "none" ? "API Key" : "密码"));
const needsSecret = computed(() => {
  const k = kind.value;
  if (!k) return false;
  if (k.secret === "required") return true;
  return k.secret === "with_username" && form.username.trim() !== "";
});
/** 编辑时保存过的密钥还能接着用（目标没变、没勾清除） */
const keepsSecret = computed(
  () => !!original.value?.has_secret && !targetChanged.value && !form.clear_secret,
);
const secretPlaceholder = computed(() => {
  if (form.clear_secret) return "保存时清除";
  if (keepsSecret.value) return "已保存，不改就留空";
  if (original.value?.has_secret && targetChanged.value) return "改了地址或账号，要重新填";
  return needsSecret.value ? "必填" : "可以不填";
});
/** 「清除保存的密钥」：编辑、保存过、目标没变时才有意义（目标变了服务端本来就不用库里的） */
const canClear = computed(
  () =>
    original.value !== null &&
    (original.value.has_secret || original.value.has_extra) &&
    !targetChanged.value,
);

const isHttp = computed(() => /^http:\/\//i.test(form.url.trim()));
/** 这次会发出账号或密钥（按服务端 CheckTransport 的口径） */
const sendsCredentials = computed(
  () =>
    form.username.trim() !== "" ||
    form.secret !== "" ||
    keepsSecret.value ||
    (!!original.value?.has_extra && !targetChanged.value && !form.clear_secret),
);
/** http:// 地址这种类型根本不收 */
const httpRefused = computed(() => isHttp.value && kind.value?.plain === "never");
/** http:// 地址要先确认明文风险 */
const needsAck = computed(() => {
  const k = kind.value;
  if (!k || !isHttp.value || k.plain === "never") return false;
  return k.plain === "with_ack" || sendsCredentials.value;
});

const urlTip = computed(() => {
  switch (kind.value?.plain) {
    case "never":
      return "只收 https:// 地址，只写协议、主机和端口，例如 https://192.168.1.10";
    case "without_credentials":
      return "写 https:// 地址，只写协议、主机和端口；没设用户名密码时也可以用 http://";
    case "with_ack":
      return "写 https:// 地址，只写协议、主机和端口；用 http:// 要先确认明文风险";
    default:
      return "只写协议、主机和端口";
  }
});

function errText(e: unknown, fallback: string): string {
  const msg = e instanceof Error ? e.message.trim() : "";
  return msg || fallback;
}

/** 测试结果作废：类型、地址、账号或密钥变了以后，结果与 pending 都不再对得上 */
function invalidate() {
  result.value = null;
  pending.value = "";
  otp.value = "";
  problem.value = "";
}

function reset() {
  Object.assign(form, {
    name: "",
    kind: "",
    enabled: true,
    url: "",
    username: "",
    secret: "",
    clear_secret: false,
    insecure_ack: false,
  });
  original.value = null;
  loading.value = false;
  loadError.value = "";
  trusted.value = null;
  droppedPin.value = false;
  invalidate();
}

async function start() {
  const n = ++openSeq;
  reset();
  if (props.editId === null) {
    const first = props.kinds[0];
    form.kind = first?.name ?? "";
    form.name = first?.label ?? "";
    return;
  }
  loading.value = true;
  try {
    const got = await nasApi.getSource(props.editId);
    if (n !== openSeq) return;
    original.value = got;
    Object.assign(form, {
      name: got.name,
      kind: got.kind,
      enabled: got.enabled,
      url: got.url,
      username: got.username,
      insecure_ack: got.insecure_ack,
    });
  } catch (e) {
    if (n === openSeq) loadError.value = errText(e, "读取来源失败");
  } finally {
    if (n === openSeq) loading.value = false;
  }
}

// 打开时（以及开着时换了要编辑的来源）重新读：关着时是 null，新建是 0
watch(
  () => (props.modelValue ? (props.editId ?? 0) : null),
  (key) => {
    if (key !== null) void start();
  },
  { immediate: true },
);

function onKind(v: string) {
  const prev = kindOf(form.kind);
  const next = kindOf(v);
  form.kind = v;
  // 名字还是上一种类型的默认名（或空着）时跟着换
  if (!form.name.trim() || form.name === prev?.label) form.name = next?.label ?? "";
  if (next?.username === "none") form.username = "";
  form.insecure_ack = false;
  invalidate();
}

function onUrl(v: string) {
  if (normUrl(v) !== normUrl(form.url)) {
    form.insecure_ack = false;
    invalidate();
  }
  form.url = v;
}

function onUsername(v: string) {
  if (v.trim() !== form.username.trim()) invalidate();
  form.username = v;
}

function onSecret(v: string) {
  form.secret = v;
  invalidate();
}

function onClear(v: string | number | boolean) {
  form.clear_secret = Boolean(v);
  if (form.clear_secret) form.secret = "";
  invalidate();
}

function onAck(v: string | number | boolean) {
  form.insecure_ack = Boolean(v);
  problem.value = "";
}

async function test(extra: { trust?: string; otp?: string } = {}) {
  if (!form.kind || !form.url.trim()) {
    problem.value = "先填地址";
    return;
  }
  const input: NasTestInput = { kind: form.kind, url: form.url.trim() };
  if (props.editId !== null) input.id = props.editId;
  if (showUsername.value && form.username.trim()) input.username = form.username.trim();
  if (form.secret) input.secret = form.secret;
  if (form.clear_secret) input.clear_secret = true;
  const trust = extra.trust ?? pin.value;
  if (trust) input.trust_sha256 = trust;
  if (extra.otp) input.otp = extra.otp;
  if (needsAck.value && form.insecure_ack) input.insecure_ack = true;
  testing.value = true;
  problem.value = "";
  try {
    const res = await nasApi.test(input);
    result.value = res;
    pending.value = res.status === "ok" ? (res.pending ?? "") : "";
    if (res.status !== "otp_required") otp.value = "";
  } catch (e) {
    result.value = null;
    pending.value = "";
    problem.value = errText(e, "测试失败");
  } finally {
    testing.value = false;
  }
}

/** 信任这张证书：记下指纹（保存时钉住），带着它再测一次（这次才会发出凭据） */
async function trustCert(cert: NasCertInfo) {
  trusted.value = { target: target.value, sha256: cert.sha256 };
  droppedPin.value = false;
  await test({ trust: cert.sha256 });
}

async function submitOtp() {
  const code = otp.value.trim();
  if (!code) return;
  await test({ otp: code });
}

function dropPin() {
  trusted.value = null;
  droppedPin.value = true;
  invalidate();
}

function close() {
  emit("update:modelValue", false);
}

async function save() {
  if (!form.name.trim()) {
    problem.value = "要填名字";
    return;
  }
  if (!form.url.trim()) {
    problem.value = "要填地址";
    return;
  }
  const input: NasSourceInput = {
    name: form.name.trim(),
    kind: form.kind,
    enabled: form.enabled,
    url: form.url.trim(),
    username: showUsername.value ? form.username.trim() : "",
    tls_pin_sha256: pin.value,
    insecure_ack: needsAck.value && form.insecure_ack,
  };
  if (form.clear_secret) input.clear_secret = true;
  else {
    if (form.secret) input.secret = form.secret;
    if (pending.value) input.pending = pending.value;
  }
  saving.value = true;
  problem.value = "";
  try {
    const saved =
      props.editId === null ? await nasApi.create(input) : await nasApi.update(props.editId, input);
    ElMessage.success("已保存");
    emit("saved", saved);
    close();
  } catch (e) {
    const msg = errText(e, "保存失败");
    // pending 过期或用过了：测试结果一起作废，要再测一次
    if (msg.includes("再测一次")) invalidate();
    problem.value = msg;
  } finally {
    saving.value = false;
  }
}

/** 关掉以后清掉表单（包括填了没保存的密钥） */
function onClosed() {
  openSeq += 1;
  reset();
}

function systemText(sys: NasSystem | undefined): string {
  if (!sys) return "";
  const os = [sys.platform, sys.version].filter(Boolean).join(" ");
  return [sys.hostname, os, sys.model].filter(Boolean).join(" · ");
}

/** 指纹按字节分组写成 AB:CD:…，方便和 NAS 上看到的对 */
function fingerprintText(sha: string): string {
  return (sha.toUpperCase().match(/.{1,2}/g) ?? []).join(":");
}

/** 短写的指纹：前 4 个与后 4 个字节 */
function fingerprintShort(sha: string): string {
  const parts = sha.toUpperCase().match(/.{1,2}/g) ?? [];
  if (parts.length <= 8) return parts.join(":");
  return `${parts.slice(0, 4).join(":")}:…:${parts.slice(-4).join(":")}`;
}

function dayText(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

function expired(cert: NasCertInfo): boolean {
  const t = new Date(cert.not_after).getTime();
  return !Number.isNaN(t) && t < Date.now();
}
</script>

<template>
  <el-dialog
    class="pt-dialog"
    :model-value="modelValue"
    :title="title"
    :width="isMobile ? '94%' : '560px'"
    append-to-body
    align-center
    :close-on-click-modal="false"
    data-testid="ns-dialog"
    @update:model-value="(v: boolean) => emit('update:modelValue', v)"
    @closed="onClosed">
    <PtDataState v-if="loading" state="loading" data-testid="ns-loading" />
    <PtDataState v-else-if="loadError" state="error" :sub="loadError" data-testid="ns-load-error" />

    <el-form v-else class="pt-form" label-position="top" @submit.prevent>
      <el-form-item label="类型">
        <el-select
          v-if="editId === null"
          :model-value="form.kind"
          data-testid="ns-kind"
          @update:model-value="onKind">
          <el-option
            v-for="k in kinds"
            :key="k.name"
            :value="k.name"
            :label="k.experimental ? `${k.label}（实验性）` : k.label" />
        </el-select>
        <span v-else class="ns-fixed" data-testid="ns-kind-fixed">{{
          nasKindLabel(form.kind, kinds)
        }}</span>
        <div v-if="kindMissing" class="field-tip field-tip--dang" data-testid="ns-kind-missing">
          这个版本不认「{{ form.kind }}」这种来源，只能删除。
        </div>
        <div v-else-if="kind?.experimental" class="field-tip" data-testid="ns-experimental">
          实验性：还没在足够多的真机上验证过，读到的数据可能不全；连不上时看来源列表里写的原因。
        </div>
      </el-form-item>

      <template v-if="!kindMissing">
        <el-form-item label="名字">
          <el-input v-model="form.name" maxlength="64" data-testid="ns-name" />
        </el-form-item>

        <el-form-item label="地址">
          <el-input
            :model-value="form.url"
            placeholder="https://192.168.1.10"
            autocomplete="off"
            data-testid="ns-url"
            @update:model-value="onUrl" />
          <div class="field-tip">{{ urlTip }}</div>
          <div v-if="httpRefused" class="field-tip field-tip--dang" data-testid="ns-http-refused">
            {{ kind?.label }} 只收 https:// 地址。
          </div>
          <el-checkbox
            v-if="needsAck"
            class="ns-ack"
            :model-value="form.insecure_ack"
            data-testid="ns-ack"
            @update:model-value="onAck"
            >我知道账号密码会以明文经过局域网，可能被截获</el-checkbox
          >
        </el-form-item>

        <el-form-item v-if="showUsername" label="用户名">
          <el-input
            :model-value="form.username"
            maxlength="128"
            autocomplete="off"
            :placeholder="kind?.username === 'required' ? '必填' : '不开认证时不填'"
            data-testid="ns-username"
            @update:model-value="onUsername" />
        </el-form-item>

        <el-form-item v-if="showSecret" :label="secretLabel">
          <el-input
            :model-value="form.secret"
            type="password"
            show-password
            autocomplete="new-password"
            :disabled="form.clear_secret"
            :placeholder="secretPlaceholder"
            data-testid="ns-secret"
            @update:model-value="onSecret" />
          <el-checkbox
            v-if="canClear"
            class="ns-ack"
            :model-value="form.clear_secret"
            data-testid="ns-clear"
            @update:model-value="onClear"
            >{{
              original?.has_extra ? "清除保存的密钥与登录凭据（设备令牌）" : "清除保存的密钥"
            }}</el-checkbox
          >
        </el-form-item>

        <el-form-item label="启用">
          <el-switch v-model="form.enabled" data-testid="ns-enabled" />
          <div class="field-tip">停用的来源不读数据，也不在首页显示。</div>
        </el-form-item>

        <div v-if="pin" class="ns-pin" data-testid="ns-pin">
          <PtIcon name="shield-check" :size="14" class="ns-pin__icon" />
          <span class="ns-pin__text"
            >信任的证书 <code :title="fingerprintText(pin)">{{ fingerprintShort(pin) }}</code></span
          >
          <el-button link type="danger" data-testid="ns-drop-pin" @click="dropPin"
            >不再信任</el-button
          >
        </div>

        <!-- 测试的结果 -->
        <div v-if="testing" class="pt-note ns-result" data-testid="ns-testing">
          <PtIcon name="loader-circle" :size="14" class="pt-note__icon ns-spin" />
          <span>正在测试，最多等 20 秒…</span>
        </div>
        <template v-else-if="result">
          <div
            v-if="result.status === 'ok'"
            class="pt-note pt-note--ok ns-result"
            data-testid="ns-ok">
            <PtIcon name="circle-check" :size="14" class="pt-note__icon" />
            <div class="ns-result__body">
              <div class="pt-note__title">连上了</div>
              <div v-if="systemText(result.system)">{{ systemText(result.system) }}</div>
              <div v-if="pending" data-testid="ns-pending">
                登录得到的凭据会在保存时一起存下；10 分钟内没保存的话要再测一次。
              </div>
            </div>
          </div>

          <div
            v-else-if="result.status === 'certificate' && result.certificate"
            class="pt-note pt-note--warn ns-result"
            data-testid="ns-cert">
            <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
            <div class="ns-result__body">
              <div class="pt-note__title">{{ result.message || "证书不受信任" }}</div>
              <div>
                没有发出账号与密钥就断开了。核对指纹和 NAS
                证书里写的一样，再信任这张证书（以后只认这一张）。
              </div>
              <ul class="ns-cert">
                <li>
                  <span class="ns-k">签发给</span
                  ><span>{{ result.certificate.subject || "（空）" }}</span>
                </li>
                <li>
                  <span class="ns-k">签发者</span>
                  <span
                    >{{ result.certificate.issuer || "（空）"
                    }}<template
                      v-if="
                        result.certificate.issuer &&
                        result.certificate.issuer === result.certificate.subject
                      "
                      >（自签）</template
                    ></span
                  >
                </li>
                <li>
                  <span class="ns-k">有效期</span>
                  <span
                    >{{ dayText(result.certificate.not_before) }} 到
                    {{ dayText(result.certificate.not_after)
                    }}<span v-if="expired(result.certificate)" class="ns-dang"
                      >（已经过期）</span
                    ></span
                  >
                </li>
                <li v-if="result.certificate.dns_names?.length">
                  <span class="ns-k">域名</span
                  ><span>{{ result.certificate.dns_names.join("、") }}</span>
                </li>
                <li>
                  <span class="ns-k">SHA-256</span
                  ><code data-testid="ns-cert-sha">{{
                    fingerprintText(result.certificate.sha256)
                  }}</code>
                </li>
              </ul>
              <el-button
                size="small"
                type="primary"
                plain
                data-testid="ns-trust"
                @click="trustCert(result.certificate)">
                <PtIcon name="shield-check" :size="14" /><span>信任这张证书并再测一次</span>
              </el-button>
            </div>
          </div>

          <div
            v-else-if="result.status === 'otp_required'"
            class="pt-note pt-note--warn ns-result"
            data-testid="ns-otp">
            <PtIcon name="key-round" :size="14" class="pt-note__icon" />
            <div class="ns-result__body">
              <div class="pt-note__title">要验证码</div>
              <div v-if="result.message">{{ result.message }}</div>
              <div class="ns-otp">
                <el-input
                  v-model="otp"
                  class="ns-otp__input"
                  inputmode="numeric"
                  maxlength="10"
                  autocomplete="one-time-code"
                  placeholder="两步验证的验证码"
                  data-testid="ns-otp-input"
                  @keyup.enter="submitOtp" />
                <el-button
                  type="primary"
                  plain
                  :disabled="!otp.trim()"
                  data-testid="ns-otp-submit"
                  @click="submitOtp"
                  >提交验证码</el-button
                >
              </div>
            </div>
          </div>

          <div v-else class="pt-note pt-note--dang ns-result" data-testid="ns-fail">
            <PtIcon name="circle-x" :size="14" class="pt-note__icon" />
            <div class="ns-result__body">
              <div class="pt-note__title">
                {{ result.status === "auth_failed" ? "登录失败" : "连不上" }}
              </div>
              <div v-if="result.message">{{ result.message }}</div>
            </div>
          </div>
        </template>

        <div v-if="problem" class="pt-note pt-note--dang ns-result" data-testid="ns-problem">
          <PtIcon name="circle-x" :size="14" class="pt-note__icon" />
          <span>{{ problem }}</span>
        </div>
      </template>
    </el-form>

    <template #footer>
      <el-button
        v-if="!loading && !loadError && !kindMissing"
        class="ns-test"
        :loading="testing"
        :disabled="saving"
        data-testid="ns-test"
        @click="test()">
        <PtIcon name="zap" :size="14" /><span>测试连接</span>
      </el-button>
      <el-button data-testid="ns-cancel" @click="close">取消</el-button>
      <el-button
        v-if="!loading && !loadError && !kindMissing"
        type="primary"
        :loading="saving"
        :disabled="testing"
        data-testid="ns-save"
        @click="save">
        <PtIcon name="save" :size="14" /><span>保存</span>
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
/* 类型改不了时的那一行：同输入框的字号，不画框 */
.ns-fixed {
  font-size: var(--pt-fz-body);
  font-weight: 600;
  line-height: 32px;
  color: var(--pt-t1);
}

/* 复选框跟在输入框下面，长文字要能折行（el-checkbox 默认不换行，手机上会冲出弹窗） */
.ns-ack {
  height: auto;
  margin-top: 6px;
  margin-right: 0;
  white-space: normal;
}

.ns-ack :deep(.el-checkbox__label) {
  line-height: 20px;
}

/* 信任的证书：一行，指纹用等宽字 */
.ns-pin {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  margin: -4px 0 var(--pt-space-4);
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.ns-pin__icon {
  color: var(--pt-ok);
}

.ns-pin__text code {
  font-family: var(--pt-font-mono);
}

.ns-result + .ns-result {
  margin-top: 8px;
}

.ns-result__body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.ns-result__body > .el-button {
  align-self: flex-start;
  margin-top: 4px;
}

/* 证书：一项一行，指纹折行 */
.ns-cert {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 4px 0 0;
  padding: 0;
  list-style: none;
}

.ns-cert li {
  display: flex;
  gap: 8px;
  min-width: 0;
}

.ns-cert li > :last-child {
  min-width: 0;
  word-break: break-all;
}

.ns-cert code {
  font-family: var(--pt-font-mono);
}

.ns-k {
  flex: 0 0 56px;
  color: var(--pt-t3);
}

.ns-dang {
  color: var(--pt-dang);
}

.ns-otp {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 4px;
}

.ns-otp__input {
  max-width: 200px;
}

/* 页脚：「测试连接」靠左，取消与保存靠右 */
.ns-test {
  margin-right: auto;
}

.ns-spin {
  animation: ns-spin 1s linear infinite;
}

@keyframes ns-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .ns-spin {
    animation: none;
  }
}
</style>
