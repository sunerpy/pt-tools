<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { downloaderTorrentsApi, downloadersApi, type DownloaderSetting } from "../../api";
import { useRuntimeStore } from "../../stores/runtime";
import { useVersionStore } from "../../stores/version";
import VersionChecker from "../VersionChecker.vue";
import PtIcon from "../PtIcon";

/**
 * 底部深色状态条（设计稿 G 的 `statusbar`，高 28）。
 * 板 41 的落位表把两件事放在这里：调度器的启停，和版本/更新入口。
 * 其余格子是背景信息，数字全部来自 runtimeStore 的真实拉取。
 */
const runtimeStore = useRuntimeStore();
const versionStore = useVersionStore();

const schedulerIcon = computed(() => {
  if (runtimeStore.schedulerHint === "running") return "circle-check";
  if (runtimeStore.schedulerHint === "stopped") return "circle-pause";
  return "activity";
});

const schedulerColor = computed(() => {
  if (runtimeStore.schedulerHint === "running") return "var(--pt-ok)";
  if (runtimeStore.schedulerHint === "stopped") return "var(--pt-warn)";
  return "var(--pt-chrome-t2)";
});

/*
 * 「下载器身份」格（画板 statusbar 的右端）。
 *
 * 画板那格写的是「名称 · 连接态 · 版本」，这里只落前两段：版本在 HTTP 层拿不到。
 * `Downloader.GetClientVersion()` 只存在于 Go 侧的下载器接口，production 的 web
 * 处理器里没有任何地方调它（`/api/downloader-torrents/meta` 的响应结构只有
 * categories 与 tags），所以第三段没有真实来源。编一个版本号比空着更糟，就空着。
 *
 * 名称取 `/api/downloaders` 里 is_default 的那台 —— 这个接口纯读库、不碰下载器，
 * 而且后端已经把密码剔掉了。
 *
 * 连接态取 transfer-stats 明细里这一台的 `reachable`。
 * **不能拿「在不在明细数组里」当连接态**：后端只要能取到实例就会 append 这一条，
 * 取数失败时各字段留零值；实例可能是缓存来的，Transmission 的实现在普通 RPC 失败后
 * 也不清 healthy 标志 —— 那样一台断线的客户端会被显示成「已连接」。这是评审查出来的
 * 真缺陷，后端为此补了显式的 reachable 字段（见 api_downloader_torrents.go）。
 *
 * 刻意不碰 `/api/downloaders/{id}/health`：那个接口每次都真去连下载器。状态条是
 * 常驻构件，把它挂进轮询等于每拍额外拨一次下载器；下载器离线时请求会挂住，而浏览器
 * 对同源只保持 6 条连接 —— runtime store 为此专门写了超时与退避，这里不该绕过去。
 */

/** 连接态最少隔这么久才重探一次：它变化比速率慢得多，没必要跟着 30 秒一拍走 */
const LINK_PROBE_MIN_GAP_MS = 120_000;
/** 与 runtime store 同一个预算：到点就放弃，把连接槽还回去 */
const LINK_PROBE_TIMEOUT_MS = 8_000;

const defaultDownloader = ref<DownloaderSetting | null>(null);
/** null = 还没探到结论（未探过，或这一轮失败了） */
const defaultOnline = ref<boolean | null>(null);
let lastProbeAt = 0;
let probing = false;

async function loadDefaultDownloader() {
  try {
    const list = await downloadersApi.list();
    defaultDownloader.value = list.find((d) => d.is_default) ?? null;
  } catch {
    // 状态条是背景信息，拿不到就不画这一格，不弹提示
    defaultDownloader.value = null;
  }
}

async function probeLink() {
  if (probing || Date.now() - lastProbeAt < LINK_PROBE_MIN_GAP_MS) return;
  probing = true;
  try {
    // 顺带重读一次列表：改默认下载器、改名都发生在别的页面，而状态条常驻、不会自己重挂
    await loadDefaultDownloader();
    const dl = defaultDownloader.value;
    // 停用的那台不在 transfer-stats 的遍历范围里，探它只会得到一个误导性的「未连接」
    if (!dl || dl.id === undefined || !dl.enabled) {
      defaultOnline.value = null;
      return;
    }
    const stats = await downloaderTorrentsApi.transferStats(
      AbortSignal.timeout(LINK_PROBE_TIMEOUT_MS),
    );
    const mine = stats.downloaders.find((d) => d.downloader_id === dl.id);
    /* 明细里没有这一台 = 这一轮连实例都没取到，同样算没连上（不是「未知」） */
    defaultOnline.value = mine ? mine.reachable : false;
  } catch {
    defaultOnline.value = null;
  } finally {
    lastProbeAt = Date.now();
    probing = false;
  }
}

onMounted(() => {
  // 先只拿名字：这一步不碰下载器，所以不会和 runtimeStore 的首轮四个请求抢连接槽
  void loadDefaultDownloader();
});

/*
 * 连接态搭 runtimeStore 的便车：lastSync 一变就说明 transfer-stats 刚刚成功返回过，
 * 只在这个窗口里去要一次明细。自己不起定时器，也就不会把请求发进一个正在挂住的接口。
 */
watch(
  () => runtimeStore.lastSync,
  (t) => {
    if (t) void probeLink();
  },
);

type LinkState = "online" | "offline" | "disabled" | "unknown";

const linkState = computed<LinkState>(() => {
  const dl = defaultDownloader.value;
  if (!dl) return "unknown";
  if (!dl.enabled) return "disabled";
  // stale = 最近一拍 transfer-stats 没回来，上一次的结论已经不能代表现在
  if (runtimeStore.stale || defaultOnline.value === null) return "unknown";
  return defaultOnline.value ? "online" : "offline";
});

/** 28px 的格子里只放短标签，完整说法在 tooltip 里 */
const linkText = computed(() => {
  if (linkState.value === "online") return "已连接";
  if (linkState.value === "offline") return "未连接";
  if (linkState.value === "disabled") return "已停用";
  return "未知";
});

const linkColor = computed(() => {
  if (linkState.value === "online") return "var(--pt-ok)";
  if (linkState.value === "offline") return "var(--pt-warn)";
  return "var(--pt-chrome-t2)";
});

const linkTip = computed(() => {
  if (linkState.value === "disabled") return "默认下载器已停用，传输统计不包含它";
  if (linkState.value === "unknown") return "连接态未知：最近一次传输统计没有成功返回";
  if (linkState.value === "online") return "默认下载器 · 最近一次传输统计真的取到了它的数据";
  return "默认下载器 · 最近一次传输统计没能从它取到数据";
});

/** 与旧页脚同一套算法：取浏览器年份与构建年份的较大值，机器时钟偏早时不显示过去的年份 */
const year = computed(() => {
  const browserYear = new Date().getFullYear();
  const buildTime = versionStore.versionInfo?.build_time;
  if (!buildTime || buildTime === "unknown") return browserYear;
  const buildYear = new Date(buildTime).getFullYear();
  return Number.isFinite(buildYear) ? Math.max(browserYear, buildYear) : browserYear;
});
</script>

<template>
  <footer class="pt-status" aria-label="运行状态">
    <el-popover placement="top-start" trigger="click" :width="200" :offset="10">
      <template #reference>
        <button type="button" class="pt-status__cell pt-status__cell--btn">
          <PtIcon :name="schedulerIcon" :size="13" :style="{ color: schedulerColor }" />
          <span>{{ runtimeStore.schedulerText }}</span>
          <PtIcon name="chevron-up" :size="12" class="pt-status__caret" />
        </button>
      </template>
      <div class="pt-status__menu">
        <el-button
          size="small"
          type="danger"
          plain
          :loading="runtimeStore.stopLoading"
          @click="runtimeStore.stopAll()">
          停止所有任务
        </el-button>
        <el-button
          size="small"
          type="primary"
          plain
          :loading="runtimeStore.startLoading"
          @click="runtimeStore.startAll()">
          启动所有任务
        </el-button>
      </div>
    </el-popover>

    <el-tooltip :content="`每 ${runtimeStore.pollMinutes} 分钟自动刷新`" placement="top">
      <span class="pt-status__cell" :class="{ 'is-stale': runtimeStore.stale }">
        <PtIcon name="refresh-cw" :size="13" />
        <span>最后同步 {{ runtimeStore.lastSyncText }}</span>
      </span>
    </el-tooltip>

    <span class="pt-status__cell" :class="{ 'is-stale': runtimeStore.stale }">
      <PtIcon name="download" :size="13" />
      <span>{{ runtimeStore.downloadText }}</span>
    </span>

    <span class="pt-status__cell" :class="{ 'is-stale': runtimeStore.stale }">
      <PtIcon name="upload" :size="13" />
      <span>{{ runtimeStore.uploadText }}</span>
    </span>

    <el-tooltip content="下载器剩余空间合计" placement="top">
      <span class="pt-status__cell" :class="{ 'is-stale': runtimeStore.stale }">
        <PtIcon name="hard-drive" :size="13" />
        <span>{{ runtimeStore.freeSpaceText }}</span>
      </span>
    </el-tooltip>

    <span class="pt-status__spacer" />

    <!--
      下载器身份。没有默认下载器（没配、或列表没拉到）就整格不画：
      其余格子是全局聚合数字，永远有意义；这一格没有身份可报时占位只是噪声。
    -->
    <el-tooltip v-if="defaultDownloader" :content="linkTip" placement="top">
      <span class="pt-status__cell pt-status__dl" :class="{ 'is-stale': linkState === 'unknown' }">
        <PtIcon name="server" :size="13" :style="{ color: linkColor }" />
        <span class="pt-status__dl-text">{{ defaultDownloader.name }} · {{ linkText }}</span>
      </span>
    </el-tooltip>

    <span class="pt-status__version">
      <VersionChecker compact />
    </span>

    <span class="pt-status__cell pt-status__copy">
      <a href="https://github.com/sunerpy/pt-tools" target="_blank" rel="noopener">pt-tools</a>
      <span>© {{ year }} · PT 助手</span>
    </span>
  </footer>
</template>
