<script setup lang="ts">
/*
 * qB 兼容入口（路线图 M13）：让 MoviePilot、IYUU、autobrr、Sonarr/Radarr 这类只认 qBittorrent 的工具把 pt-tools 当成下载器。
 * 监听地址是启动参数（--qbit-compat-addr 或 PT_QBIT_COMPAT_ADDR），这页只读；能改的是背后的下载器与完全控制。
 * 画板没有这一页，沿用 CloakBrowser 页的形状：左边设置、右边状态，下面是客户端怎么填。
 */
import {
  downloadersApi,
  type DownloaderSetting,
  qbitCompatApi,
  type QbitCompatInput,
  type QbitCompatStatus,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref } from "vue";

const isMobile = useIsMobile();
const ds = useDataState();
const status = ref<QbitCompatStatus | null>(null);
const downloaders = ref<DownloaderSetting[]>([]);
const form = ref<QbitCompatInput>({ downloader_id: 0, full_control: false });
const saving = ref(false);

async function load() {
  const pending = ds.run(async () => {
    const [st, dls] = await Promise.all([qbitCompatApi.get(), downloadersApi.list()]);
    return { st, dls };
  });
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  status.value = got.st;
  downloaders.value = got.dls.filter((d) => d.enabled);
  form.value = { downloader_id: got.st.downloader_id, full_control: got.st.full_control };
}

const TYPE_LABELS: Record<string, string> = {
  qbittorrent: "qBittorrent",
  transmission: "Transmission",
};

const defaultLabel = computed(() => {
  const def = downloaders.value.find((d) => d.is_default);
  return def ? `默认下载器（现在是 ${def.name}）` : "默认下载器（没有设默认的时候用第一台启用的）";
});

/** 客户端填的地址：监听在 0.0.0.0 / :: 上时用浏览器里的主机名，监听在具体地址上时用它 */
const clientURL = computed(() => {
  const addr = status.value?.listen_addr ?? "";
  const i = addr.lastIndexOf(":");
  if (i < 0) return "";
  let host = addr.slice(0, i);
  const port = addr.slice(i + 1);
  if (host === "" || host === "0.0.0.0" || host === "[::]" || host === "::")
    host = window.location.hostname;
  return `http://${host}:${port}`;
});

function errText(e: unknown, fallback: string): string {
  return e instanceof Error && e.message ? e.message : fallback;
}

async function save() {
  saving.value = true;
  try {
    status.value = await qbitCompatApi.save({ ...form.value });
    form.value = {
      downloader_id: status.value.downloader_id,
      full_control: status.value.full_control,
    };
    ElMessage.success("已保存");
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
          <el-form-item label="背后的下载器">
            <el-select v-model="form.downloader_id" data-testid="qc-downloader">
              <el-option :value="0" :label="defaultLabel" />
              <el-option
                v-for="d in downloaders"
                :key="d.id"
                :value="d.id!"
                :label="`${d.name}（${TYPE_LABELS[d.type] ?? d.type}）`" />
            </el-select>
            <div class="qc-tip">
              客户端加的种子推到这台，列出来的也是这台里的种子。推送照常经过磁盘空间保护与站点做种容量的检查。
            </div>
          </el-form-item>
          <el-form-item label="完全控制">
            <el-switch v-model="form.full_control" data-testid="qc-full" />
            <div class="qc-tip">
              关着时，客户端只能暂停、删除、改分类与标签的是经兼容入口加的种子；打开以后能动这台下载器里的全部种子。
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
            <PtStatusPill :tone="status.listening ? 'ok' : 'warn'" dot size="sm">{{
              status.listening ? "在监听" : "没有开启"
            }}</PtStatusPill>
          </li>
          <li v-if="status.listening">
            <span class="qc-k">监听地址</span><code>{{ status.listen_addr }}</code>
          </li>
          <li>
            <span class="qc-k">下载器</span
            ><span>{{ status.downloader || "没有可用的下载器" }}</span>
          </li>
          <li>
            <span class="qc-k">经它加的种子</span><span>{{ status.compat_torrents }} 个</span>
          </li>
        </ul>
        <div v-if="!status.listening" class="pt-note pt-note--warn qc-off" data-testid="qc-off">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
          <span
            >启动时加参数 <code>--qbit-compat-addr 0.0.0.0:8081</code>，或者设环境变量
            <code>PT_QBIT_COMPAT_ADDR=0.0.0.0:8081</code
            >，重启以后生效。这个端口只在内网开放。</span
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
            </template>
            <span v-else>开启以后在这里显示</span>
          </li>
          <li>
            <span class="qc-k">用户名</span><span>随意填，例如 pt-tools（记进操作审计）</span>
          </li>
          <li>
            <span class="qc-k">密码</span>
            <span
              >有「qB 兼容」权限的 API 令牌，在
              <router-link to="/api-tokens" data-testid="qc-tokens">系统 → API 令牌</router-link>
              新建</span
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

.qc-off {
  margin-top: 12px;
}
</style>
