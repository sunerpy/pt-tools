<script setup lang="ts">
/*
 * qB 兼容入口（路线图 M13）：让 MoviePilot、IYUU、autobrr、Sonarr/Radarr 这类只认 qBittorrent 的工具把 pt-tools 当成下载器。
 * 监听地址可以在这页设（保存后马上换监听）；启动参数 --qbit-compat-addr 或环境变量 PT_QBIT_COMPAT_ADDR 给了地址时用那个，这页只读。
 * 画板没有这一页，沿用 CloakBrowser 页的形状：左边设置、右边状态，下面是客户端怎么填。
 */
import {
  downloadersApi,
  type DownloaderSetting,
  qbitCompatApi,
  type QbitCompatLogin,
  type QbitCompatStatus,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatShortDateTime } from "@/utils/format";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref } from "vue";

const DEFAULT_ADDR = "0.0.0.0:8081";

const isMobile = useIsMobile();
const ds = useDataState();
const status = ref<QbitCompatStatus | null>(null);
const downloaders = ref<DownloaderSetting[]>([]);
const form = ref({
  downloader_id: 0,
  full_control: false,
  listen_on: false,
  listen_addr: DEFAULT_ADDR,
});
const saving = ref(false);

function fillForm(st: QbitCompatStatus) {
  form.value = {
    downloader_id: st.downloader_id,
    full_control: st.full_control,
    listen_on: st.settings_addr !== "",
    listen_addr: st.settings_addr || DEFAULT_ADDR,
  };
}

async function load() {
  const pending = ds.run(async () => {
    const [st, dls] = await Promise.all([qbitCompatApi.get(), downloadersApi.list()]);
    return { st, dls };
  });
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  status.value = got.st;
  downloaders.value = got.dls;
  fillForm(got.st);
}

const TYPE_LABELS: Record<string, string> = {
  qbittorrent: "qBittorrent",
  transmission: "Transmission",
};

const defaultLabel = computed(() => {
  const def = downloaders.value.find((d) => d.is_default && d.enabled);
  return def ? `默认下载器（现在是 ${def.name}）` : "默认下载器（没有设默认的时候用第一台启用的）";
});

/** 地址由启动参数或环境变量给的：这页改不了 */
const locked = computed(
  () => status.value?.listen_source === "flag" || status.value?.listen_source === "env",
);
const lockedBy = computed(() =>
  status.value?.listen_source === "flag"
    ? "启动参数 --qbit-compat-addr"
    : "环境变量 PT_QBIT_COMPAT_ADDR",
);

function splitAddr(addr: string): { host: string; port: string } | null {
  const i = addr.lastIndexOf(":");
  if (i < 0) return null;
  return { host: addr.slice(0, i), port: addr.slice(i + 1) };
}

/** 监听在所有地址上（0.0.0.0、::、没写地址）：局域网里别的机器都连得上 */
const allInterfaces = computed(() => {
  const a = splitAddr(status.value?.listen_addr ?? "");
  return Boolean(a && ["", "0.0.0.0", "[::]", "::"].includes(a.host));
});

/** 客户端填的地址：监听在所有地址上时，主机名取浏览器地址栏里的（猜的）；监听在具体地址上时用它 */
const clientURL = computed(() => {
  const a = splitAddr(status.value?.listen_addr ?? "");
  if (!a) return "";
  const host = allInterfaces.value ? window.location.hostname : a.host;
  return `http://${host}:${a.port}`;
});

const SOURCE_LABELS: Record<string, string> = {
  flag: "启动参数",
  env: "环境变量",
  settings: "这页设的",
};

const RESULT_LABELS: Record<QbitCompatLogin["result"], string> = {
  ok: "成功",
  bad_credentials: "令牌不对",
  scope: "令牌没有「qB 兼容」权限",
  locked: "失败太多次，这个地址被锁了 15 分钟",
};

function loginText(l: QbitCompatLogin): string {
  const who = [l.ip, l.token ? `令牌「${l.token}」` : "", l.username ? `用户名 ${l.username}` : ""]
    .filter(Boolean)
    .join(" · ");
  return `${formatShortDateTime(l.at)} · ${who}`;
}

function errText(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback;
}

/** 打开完全控制前确认：客户端能动这台下载器里的全部种子 */
async function toggleFull(on: string | number | boolean) {
  if (!on) {
    form.value.full_control = false;
    return;
  }
  try {
    await ElMessageBox.confirm(
      "打开以后，客户端能暂停、删除、改分类与标签的是这台下载器里的全部种子，包括不是经兼容入口加的（例如 RSS 推送的、手动加的）。客户端删种时会连数据一起删。确定打开？",
      "打开完全控制",
      { type: "warning", confirmButtonText: "打开", cancelButtonText: "取消" },
    );
    form.value.full_control = true;
  } catch {
    form.value.full_control = false;
  }
}

async function save() {
  const f = form.value;
  const addr = f.listen_addr.trim();
  if (!locked.value && f.listen_on && !addr) {
    ElMessage.warning("填写监听地址，例如 0.0.0.0:8081");
    return;
  }
  saving.value = true;
  try {
    const body: { downloader_id: number; full_control: boolean; listen_addr?: string } = {
      downloader_id: f.downloader_id,
      full_control: f.full_control,
    };
    if (!locked.value) body.listen_addr = f.listen_on ? addr : "";
    const st = await qbitCompatApi.save(body);
    status.value = st;
    fillForm(st);
    if (st.listen_error)
      ElMessage.warning(`已保存，但监听 ${st.configured_addr} 失败：${st.listen_error}`);
    else ElMessage.success("已保存");
  } catch (e) {
    ElMessage.error(errText(e, "保存失败"));
  } finally {
    saving.value = false;
  }
}

async function copyURL() {
  try {
    await navigator.clipboard.writeText(clientURL.value);
    ElMessage.success("已复制");
  } catch {
    ElMessage.warning("浏览器不让复制：请手动选中地址复制");
  }
}

onMounted(load);
</script>

<template>
  <div class="pt-cards pt-cards--main">
    <PtHeadSub
      >让只认 qBittorrent 的工具（MoviePilot、IYUU、autobrr、Sonarr/Radarr）把 pt-tools
      当成下载器</PtHeadSub
    >
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="qc-refresh" @click="load">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
    </Teleport>

    <PtDataState
      v-if="!status"
      class="pt-cards__full"
      :state="ds.state.value"
      :sub="ds.errorText.value"
      data-testid="qc-state" />

    <template v-else>
      <PtPanel title="设置" icon="settings" data-testid="qc-settings">
        <el-form class="pt-form" label-position="top" @submit.prevent>
          <el-form-item label="监听">
            <template v-if="locked">
              <code data-testid="qc-locked-addr">{{ status.configured_addr }}</code>
              <div class="qc-tip" data-testid="qc-locked">
                地址由{{ lockedBy }}指定，这里改不了；要在这里设，去掉它以后重启。
              </div>
            </template>
            <template v-else>
              <el-switch v-model="form.listen_on" data-testid="qc-listen-on" />
              <el-input
                v-if="form.listen_on"
                v-model="form.listen_addr"
                class="qc-addr"
                placeholder="0.0.0.0:8081"
                aria-label="监听地址"
                data-testid="qc-listen-addr" />
              <div class="qc-tip">
                写成「地址:端口」：0.0.0.0 是所有网卡（局域网里的机器都连得上），127.0.0.1
                只给本机。端口不要和网页的端口一样；pt-tools 在 Docker
                里时还要映射这个端口。保存后马上生效。
              </div>
            </template>
          </el-form-item>
          <el-form-item label="背后的下载器">
            <el-select v-model="form.downloader_id" data-testid="qc-downloader">
              <el-option :value="0" :label="defaultLabel" />
              <el-option
                v-for="d in downloaders"
                :key="d.id"
                :value="d.id!"
                :disabled="!d.enabled"
                :label="`${d.name}（${TYPE_LABELS[d.type] ?? d.type}）${d.enabled ? '' : '（已停用）'}`" />
            </el-select>
            <div class="qc-tip">
              客户端加的种子推到这台，列出来的也是这台里的种子。推送照常经过磁盘空间保护与站点做种容量的检查。
            </div>
          </el-form-item>
          <el-form-item label="完全控制">
            <el-switch
              :model-value="form.full_control"
              data-testid="qc-full"
              @update:model-value="toggleFull" />
            <div class="qc-tip">
              关着时，客户端只能暂停、删除、改分类与标签的是经兼容入口加的种子；打开以后能动这台下载器里的全部种子。
            </div>
            <div
              v-if="form.full_control"
              class="pt-note pt-note--warn qc-note"
              data-testid="qc-full-warn">
              <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
              <span>完全控制开着：客户端能删这台下载器里的任何种子（连数据）。</span>
            </div>
          </el-form-item>
        </el-form>
        <template #footer>
          <el-button type="primary" :loading="saving" data-testid="qc-save" @click="save">
            <PtIcon name="save" :size="14" /><span>保存</span>
          </el-button>
        </template>
      </PtPanel>

      <PtPanel title="状态" icon="signal" data-testid="qc-status">
        <ul class="qc-kv">
          <li>
            <span class="qc-k">监听</span>
            <PtStatusPill v-if="status.listening" tone="ok" dot size="sm">在监听</PtStatusPill>
            <PtStatusPill v-else-if="status.listen_error" tone="danger" dot size="sm"
              >开启失败</PtStatusPill
            >
            <PtStatusPill v-else tone="warn" dot size="sm">没有开启</PtStatusPill>
          </li>
          <li v-if="status.listening">
            <span class="qc-k">监听地址</span><code>{{ status.listen_addr }}</code>
            <span class="qc-sub">（{{ SOURCE_LABELS[status.listen_source] ?? "" }}）</span>
          </li>
          <li>
            <span class="qc-k">下载器</span
            ><span>{{ status.downloader || "没有可用的下载器" }}</span>
          </li>
          <li>
            <span class="qc-k">经它加的种子</span><span>{{ status.compat_torrents }} 个</span>
          </li>
          <li>
            <span class="qc-k">登录着的客户端</span><span>{{ status.sessions }} 个</span>
          </li>
          <li data-testid="qc-last-login">
            <span class="qc-k">最近登录</span>
            <span v-if="status.last_login">{{ loginText(status.last_login) }}</span>
            <span v-else class="qc-sub">还没有客户端登录过（重启以后重新算）</span>
          </li>
          <li v-if="status.last_failure" data-testid="qc-last-failure">
            <span class="qc-k">最近失败</span>
            <span
              >{{ loginText(status.last_failure) }} ·
              {{ RESULT_LABELS[status.last_failure.result] ?? status.last_failure.result }}</span
            >
          </li>
        </ul>
        <div
          v-if="status.listen_error"
          class="pt-note pt-note--dang qc-note"
          data-testid="qc-listen-error">
          <PtIcon name="circle-x" :size="14" class="pt-note__icon" />
          <span
            >监听 <code>{{ status.configured_addr }}</code> 失败：{{
              status.listen_error
            }}。换一个端口，或者关掉占着这个端口的程序。</span
          >
        </div>
        <div
          v-else-if="!status.listening"
          class="pt-note pt-note--warn qc-note"
          data-testid="qc-off">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span
            >在左边打开「监听」、填好地址后保存就开；也可以在启动时加参数
            <code>--qbit-compat-addr 0.0.0.0:8081</code> 或设环境变量
            <code>PT_QBIT_COMPAT_ADDR</code>。</span
          >
        </div>
        <div
          v-if="status.downloader_issue"
          class="pt-note pt-note--warn qc-note"
          data-testid="qc-downloader-issue">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span>{{ status.downloader_issue }}</span>
        </div>
        <div v-if="status.listening && allInterfaces" class="pt-note qc-note" data-testid="qc-lan">
          <PtIcon name="shield" :size="14" class="pt-note__icon" />
          <span
            >监听在所有网卡上：这个端口只在内网开放，不要映射到公网。客户端用 API
            令牌登录，令牌泄露了就到「API 令牌」里撤销。</span
          >
        </div>
      </PtPanel>

      <PtPanel class="pt-cards__full" title="客户端怎么填" icon="link" data-testid="qc-howto">
        <ul class="qc-kv">
          <li><span class="qc-k">下载器类型</span><span>qBittorrent</span></li>
          <li>
            <span class="qc-k">地址</span>
            <template v-if="status.listening">
              <code data-testid="qc-url">{{ clientURL }}</code>
              <el-button link type="primary" data-testid="qc-copy" @click="copyURL">复制</el-button>
              <span v-if="allInterfaces" class="qc-sub" data-testid="qc-url-guess"
                >主机名取自浏览器的地址栏；客户端和浏览器不在同一台机器、或者 pt-tools 在 Docker
                里时，换成客户端能连到的 pt-tools 所在机器的地址</span
              >
            </template>
            <span v-else>开启以后在这里显示</span>
          </li>
          <li>
            <span class="qc-k">用户名</span><span>随意填，例如 pt-tools（记进操作审计）</span>
          </li>
          <li>
            <span class="qc-k">密码</span>
            <span
              >有「qB 兼容」权限的 API 令牌：
              <router-link
                :to="{ path: '/api-tokens', query: { new: 'qbit' } }"
                data-testid="qc-tokens"
                >新建一个</router-link
              >（权限已经选好）</span
            >
          </li>
        </ul>
        <p class="qc-tip">
          链接只认已启用站点的种子下载地址，由 pt-tools 自己经站点下载；磁力链接不接受。tracker
          地址里的 passkey 不会交给客户端。客户端做的写操作记在「操作审计」里，通道是「qB 兼容」。
        </p>
      </PtPanel>
    </template>
  </div>
</template>

<style scoped>
.qc-tip {
  margin: 6px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--pt-t3);
}

.qc-kv {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.qc-kv li {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  min-width: 0;
  font-size: 13px;
  color: var(--pt-t1);
}

.qc-k {
  flex: 0 0 96px;
  color: var(--pt-t3);
}

.qc-kv code {
  word-break: break-all;
}

.qc-note {
  margin-top: 12px;
}

.qc-addr {
  max-width: 260px;
  margin-left: 12px;
}

.qc-sub {
  font-size: 12px;
  color: var(--pt-t3);
}
</style>
