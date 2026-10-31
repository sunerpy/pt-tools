<script setup lang="ts">
import {
  chatopsApi,
  type DailyReportSettings,
  globalApi,
  type GlobalSettings,
  type NotificationConfig,
  type SiteProxyInput,
  type SiteProxyMode,
  type SiteProxyOption,
  type SiteProxySettings,
  siteProxyApi,
  userInfoApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref } from "vue";

/**
 * 六态（设计文档 §5）：这一页是表单不是列表，能落到的只有 loading / error / perm ——
 * 加载成功就是表单本身，没有 empty / zero / partial 可言。
 *
 * error / perm 不能只弹一条两秒就消失的 toast：配置没读回来时表单里留着的是
 * 前端默认值，用户照着点「保存」会把服务端的真实配置覆盖掉。所以失败时用状态块
 * 顶掉表单，并且不给保存入口。
 */
const { loading, state, errorText, run, isStale } = useDataState();
const saving = ref(false);
const showWarning = ref(false);
/** 配置真的读回来过一次，摘要行才有意义（默认值不是配置） */
const loaded = ref(false);
const isMobile = useIsMobile();

/** 加载失败（含无权限）：表单里的值不可信，既不给编辑也不给保存 */
const loadFailed = computed(() => state.value === "error" || state.value === "perm");

const form = ref<GlobalSettings>({
  default_interval_minutes: 10,
  download_dir: "",
  download_limit_enabled: false,
  download_speed_limit: 20,
  torrent_size_gb: 200,
  torrent_min_size_gb: 0,
  min_free_minutes: 30,
  auto_start: false,
  retain_hours: 24,
  max_retry: 3,
  default_concurrency: 3,
  default_enabled: false,
  auto_delete_on_free_end: false,
  free_end_advance_minutes: 0,
  default_filter_mode: "auto_free",
});

/**
 * 每日战报走自己的接口（只写这三列），和全局配置一起读、一起保存，是保存的第二步（和站点代理各存各的）。
 * 读不回来时不当成「改过」去保存。通道下拉只给启用的通道。
 */
const reportForm = ref<DailyReportSettings>({ enabled: false, time: "22:00", channel_ids: [] });
const reportSaved = ref<DailyReportSettings | null>(null);
/**
 * 最后一次读回 / 保存成功的全局配置。页头摘要按它算 —— 摘要的口径是「当前生效」，
 * 之前直接跟着表单走：改了间隔还没点保存，页头就已经写成新值，读起来像已经生效了。
 */
const savedForm = ref<GlobalSettings | null>(null);
const reportChannels = ref<NotificationConfig[]>([]);
/** 同上：战报设置读不回来时整段禁用 */
const reportLoadFailed = ref(false);

function sameReport(a: DailyReportSettings, b: DailyReportSettings): boolean {
  return (
    a.enabled === b.enabled &&
    a.time === b.time &&
    [...a.channel_ids].sort().join(",") === [...b.channel_ids].sort().join(",")
  );
}

/**
 * 站点代理也走自己的接口，和全局配置、每日战报一起读、一起保存。
 * proxy_url 里是读回来的打码地址（密码换成 ***），原样交回表示不改。
 * 读不回来时和每日战报一样整段禁用、不参与保存；站点多选的选项只来自接口给的 site_options。
 */
const proxyForm = ref<SiteProxyInput>({ enabled: false, proxy_url: "", mode: "all", sites: [] });
const proxySaved = ref<SiteProxyInput | null>(null);
const proxyOptions = ref<SiteProxyOption[]>([]);
const proxyLoadFailed = ref(false);
/** 存着的代理地址解不开（加密密钥换过）：后端按关闭处理，要重新填 */
const proxyUnreadable = ref(false);

function toProxyInput(p: SiteProxySettings): SiteProxyInput {
  return { enabled: p.enabled, proxy_url: p.proxy_url, mode: p.mode, sites: [...(p.sites ?? [])] };
}

function sameProxy(a: SiteProxyInput, b: SiteProxyInput): boolean {
  return (
    a.enabled === b.enabled &&
    a.proxy_url.trim() === b.proxy_url.trim() &&
    a.mode === b.mode &&
    [...a.sites].sort().join(",") === [...b.sites].sort().join(",")
  );
}

/** 读回来（或保存成功）的站点代理：已保存的基线、选项与「解不开」标记跟着换 */
function applyProxySaved(p: SiteProxySettings) {
  proxySaved.value = toProxyInput(p);
  proxyOptions.value = p.site_options ?? [];
  proxyUnreadable.value = !!p.proxy_unreadable;
}

const proxyModeOptions: { value: SiteProxyMode; label: string; desc: string }[] = [
  { value: "all", label: "全部站点", desc: "所有站点的请求都经这个代理" },
  { value: "include", label: "只有选中的站点", desc: "选中的站点经代理，其余站点直连" },
  {
    value: "exclude",
    label: "除了选中的站点",
    desc: "选中的站点直连，其余站点经代理；一个都不选等于全部站点",
  },
];

const activeProxyMode = computed(() =>
  proxyModeOptions.find((o) => o.value === proxyForm.value.mode),
);

/** 页头摘要里站点代理那一项：只在开着的时候出现 */
function proxySummary(p: SiteProxyInput): string {
  if (p.mode === "include") return `站点代理：${p.sites.length} 个站点`;
  if (p.mode === "exclude" && p.sites.length > 0) return `站点代理：除 ${p.sites.length} 个站点`;
  return "站点代理：全部站点";
}

/** short 是给页头摘要用的短名：摘要行是单行截断的，装不下带括号的完整标签 */
const filterModeOptions = [
  {
    value: "auto_free",
    label: "智能模式（推荐）",
    short: "智能模式",
    desc: "无过滤规则的 RSS：自动下载免费种子；有过滤规则的 RSS：仅下载匹配规则的种子（不再自动下非匹配的免费种子）",
  },
  {
    value: "filter_only",
    label: "仅过滤规则匹配",
    short: "仅过滤规则",
    desc: "所有 RSS 都必须匹配过滤规则才下载；未关联规则的 RSS 将不下载任何种子",
  },
  {
    value: "free_only",
    label: "仅免费（忽略过滤规则）",
    short: "仅免费",
    desc: "只下载免费种子，完全忽略过滤规则；适合纯刷流用户",
  },
];

/** 当前下载模式的详细说明，只显示选中项那一条 */
const activeFilterMode = computed(() =>
  filterModeOptions.find((o) => o.value === form.value.default_filter_mode),
);

/**
 * 画板 head 的 sub —— 标题下面那行摘要（11.5/400 t3）。
 *
 * 列表页那一行是实时数据，设置页没有实时数据，对应的口径是「当前生效的关键配置」：
 * 用户不展开整张卡片也能确认间隔、下载模式、体积区间、限速、暂存与自启这几件事。
 * 全部取自读回来的配置，没读回来就返回空串（外壳里 :empty 会把这行收掉）。
 */
const headSub = computed(() => {
  const f = savedForm.value;
  if (!loaded.value || !f) return "";
  const parts = [
    `间隔 ${f.default_interval_minutes} 分钟`,
    filterModeOptions.find((o) => o.value === f.default_filter_mode)?.short ?? "",
    f.torrent_min_size_gb > 0
      ? `种子 ${f.torrent_min_size_gb}–${f.torrent_size_gb} GB`
      : `种子上限 ${f.torrent_size_gb} GB`,
    f.download_limit_enabled ? `预估 ${f.download_speed_limit} MB/s` : "未启用限速判断",
    f.retain_hours > 0 ? `暂存保留 ${f.retain_hours} 小时` : "暂存不自动清理",
    f.auto_start ? "启动即运行" : "手动启动",
  ].filter(Boolean);
  // 免费期结束自动删数据是不可恢复的，开着就一定要在摘要里露出来
  if (f.auto_delete_on_free_end) parts.push("免费结束自动删除");
  if (proxySaved.value?.enabled) parts.push(proxySummary(proxySaved.value));
  return parts.join(" · ");
});

async function loadData() {
  const pending = run(() => globalApi.get());
  const data = await pending;
  if (isStale(pending)) return;
  if (!data) {
    loaded.value = false;
    return;
  }
  form.value = {
    ...data,
    default_interval_minutes: Math.max(5, data.default_interval_minutes || 10),
  };
  savedForm.value = { ...form.value };
  loaded.value = true;
  showWarning.value = !form.value.download_dir;
  const [report, channels, proxy] = await Promise.all([
    userInfoApi.getDailyReport().catch(() => null),
    chatopsApi.notifications.list().catch(() => [] as NotificationConfig[]),
    siteProxyApi.get().catch(() => null),
  ]);
  reportChannels.value = (channels ?? []).filter((c) => c.enabled);
  if (report) {
    reportForm.value = { ...report, channel_ids: [...(report.channel_ids ?? [])] };
    reportSaved.value = { ...report, channel_ids: [...(report.channel_ids ?? [])] };
    reportLoadFailed.value = false;
  } else {
    reportSaved.value = null;
    reportLoadFailed.value = true;
  }
  if (proxy) {
    applyProxySaved(proxy);
    proxyForm.value = toProxyInput(proxy);
    proxyLoadFailed.value = false;
  } else {
    proxySaved.value = null;
    proxyLoadFailed.value = true;
  }
}

onMounted(loadData);

async function save() {
  if (!form.value.download_dir) {
    ElMessage.error("下载目录不能为空");
    showWarning.value = true;
    return;
  }
  const report = reportForm.value;
  if (report.enabled && report.channel_ids.length === 0) {
    ElMessage.error("开启每日战报至少要选一个通知通道");
    return;
  }
  // 站点代理读不回来时 proxySaved 是 null：不算改动，不参与保存
  const proxy: SiteProxyInput = {
    ...proxyForm.value,
    proxy_url: proxyForm.value.proxy_url.trim(),
    sites: [...proxyForm.value.sites],
  };
  const proxyChanged = proxySaved.value !== null && !sameProxy(proxySaved.value, proxy);
  if (proxyChanged && proxy.enabled && !proxy.proxy_url) {
    ElMessage.error("开启站点代理要填代理地址");
    return;
  }
  if (proxyChanged && proxy.mode === "include" && proxy.sites.length === 0) {
    ElMessage.error("选了「只有选中的站点」，至少选一个站点");
    return;
  }

  saving.value = true;
  try {
    try {
      const next = {
        ...form.value,
        default_interval_minutes: Math.max(5, form.value.default_interval_minutes),
      };
      await globalApi.save(next);
      savedForm.value = next;
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || "保存失败");
      return;
    }
    showWarning.value = false;
    /*
     * 全局配置落库以后，每日战报与站点代理各自有改动才存，两个都试、互不影响：
     * 成功的更新自己的已保存基线，失败的留着表单里的改动（下次保存再试），一条提示说清哪些没保存。
     */
    const steps: { label: string; run: () => Promise<void> }[] = [];
    const savedReport = reportSaved.value;
    if (savedReport && !sameReport(savedReport, report)) {
      const next = { ...report, channel_ids: [...report.channel_ids] };
      steps.push({
        label: "每日战报设置",
        run: async () => {
          reportSaved.value = await userInfoApi.saveDailyReport(next);
        },
      });
    }
    if (proxyChanged) {
      steps.push({
        label: "站点代理",
        run: async () => {
          const saved = await siteProxyApi.save(proxy);
          applyProxySaved(saved);
          // 保存期间没再改过：换成读回来的打码地址，密码不留在页面上；改过就留着新改动
          if (sameProxy(proxyForm.value, proxy)) proxyForm.value = toProxyInput(saved);
        },
      });
    }
    const results = await Promise.allSettled(steps.map((step) => step.run()));
    const failed = results.flatMap((r, i) =>
      r.status === "rejected"
        ? [`${steps[i]!.label}没有保存：${(r.reason as Error)?.message || "请求失败"}`]
        : [],
    );
    if (failed.length > 0) {
      ElMessage.warning(`全局配置已保存，${failed.join("；")}`);
      return;
    }
    ElMessage.success("保存成功");
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <!--
    画板 27（系统设置）的主区只有两件东西：head 之后的提示，和一张通栏大卡片
    （warn 344,80 1080×58 → p-cfg 344,154 1080×812）。两者都在卡片层 ——
    左右各内缩 16、彼此间隔 16 —— 所以容器直接用 .pt-cards--wide，本页不再自己排版。
    这一页没有表格，也就没有工具栏带 / 表格带 / 页脚带。宽屏下卡片里的分组分左右两栏（.gs-split），
    以前卡宽停在 1080，2048 的屏上右边空出 680（用户反馈）。
  -->
  <div class="pt-cards pt-cards--wide">
    <!-- 页头摘要：口径是「当前生效的关键配置」，由本页把真实配置送进外壳页头 -->
    <PtHeadSub v-if="headSub">{{ headSub }}</PtHeadSub>

    <!--
      主操作进页头（画板 head 右侧动作，高 32）。
      移动端外壳把 .pt-head 整条隐掉了，Teleport 过去的按钮会跟着看不见，
      所以 <768 时改用面板页脚里的那颗保存键（见下方 footer 插槽）。
    -->
    <Teleport v-if="!isMobile && !loadFailed" to="#pt-head-acts">
      <el-button type="primary" :loading="saving" :disabled="loading" @click="save">
        <PtIcon v-if="!saving" name="save" :size="15" /><span>保存设置</span>
      </el-button>
    </Teleport>

    <div v-if="showWarning" class="pt-note pt-note--warn">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>未设置下载目录，后台任务不会启动，请先设置并保存</span>
    </div>

    <PtPanel v-loading="loading" title="全局配置" icon="settings" padding="none">
      <!-- 读不到配置时不渲染表单：表单里是默认值，保存下去就是覆盖服务端配置 -->
      <PtDataState v-if="loadFailed" :state="state" :sub="errorText">
        <template #action>
          <el-button size="small" @click="loadData">
            <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
          </el-button>
        </template>
      </PtDataState>

      <el-form v-else :model="form" label-position="top" class="pt-form">
        <!-- 宽屏（视口 ≥1600）分左右两栏：左边运行与下载，右边清理、签到与战报 -->
        <div class="gs-split">
          <div class="gs-col">
            <div class="pt-strip">
              <PtIcon name="timer" :size="13" />
              <span>运行频率</span>
            </div>
            <div class="settings-body">
              <el-form-item label="默认间隔（分钟）">
                <el-input-number
                  v-model="form.default_interval_minutes"
                  :min="5"
                  :max="1440"
                  :step="1" />
                <div class="field-tip">所有 RSS 任务的默认检查时间间隔，最小 5 分钟</div>
              </el-form-item>
            </div>

            <div class="pt-strip">
              <PtIcon name="folder" :size="13" />
              <span>存储路径</span>
            </div>
            <div class="settings-body">
              <el-form-item label="种子下载目录">
                <el-input v-model="form.download_dir" placeholder="保存 .torrent 种子文件的目录">
                  <template #prefix>
                    <PtIcon name="folder-open" :size="14" />
                  </template>
                </el-input>
                <div class="field-tip">
                  绝对路径直接使用；相对路径会拼接为 <code>~/.pt-tools/&lt;输入值&gt;</code>
                  并自动创建目录
                </div>
                <div class="field-tip">
                  此目录仅用于备份下载的 <code>.torrent</code> 文件，不是下载器的数据保存路径
                </div>
              </el-form-item>
            </div>

            <div class="pt-strip">
              <PtIcon name="gauge" :size="13" />
              <span>下载策略与限制</span>
            </div>
            <div class="settings-body">
              <div class="field-row">
                <el-form-item label="最大种子大小（GB）">
                  <el-input-number v-model="form.torrent_size_gb" :min="1" :max="10000" />
                  <div class="field-tip">超过此大小的种子将被自动忽略，防止磁盘撑爆</div>
                </el-form-item>
                <el-form-item label="最小种子大小（GB）">
                  <el-input-number v-model="form.torrent_min_size_gb" :min="0" :max="10000" />
                  <div class="field-tip">
                    小于此大小的种子将被忽略；0 = 不限制，且必须小于最大值
                  </div>
                </el-form-item>
              </div>

              <el-form-item label="默认下载模式">
                <el-radio-group v-model="form.default_filter_mode">
                  <el-radio-button
                    v-for="opt in filterModeOptions"
                    :key="opt.value"
                    :value="opt.value">
                    {{ opt.label }}
                  </el-radio-button>
                </el-radio-group>
                <div v-if="activeFilterMode" class="field-tip">{{ activeFilterMode.desc }}</div>
                <!--
                  这里以前是一条橙色警告「v0.26.0 起：智能模式下……」—— 那是版本沿革，不是风险，
                  前半句和上面选中模式的说明重复。留下后半句（订阅可覆盖、大小上限都生效）当说明。
                -->
                <div class="field-tip">
                  每个 RSS 订阅可以单独选下载模式，覆盖这里的默认值；种子大小上下限对所有模式都生效
                </div>
              </el-form-item>

              <div class="field-row">
                <el-form-item label="启用下载限速判断">
                  <el-switch v-model="form.download_limit_enabled" />
                  <div class="field-tip">用于评估种子是否能在免费期内下载完成</div>
                </el-form-item>
                <el-form-item label="预估下载速度（MB/s）">
                  <el-input-number
                    v-model="form.download_speed_limit"
                    :min="1"
                    :max="1000"
                    :disabled="!form.download_limit_enabled" />
                  <div class="field-tip">按您的网络环境填写实际平均下载速度</div>
                </el-form-item>
              </div>

              <div class="field-row">
                <el-form-item label="最短免费时间（分钟）">
                  <el-input-number v-model="form.min_free_minutes" :min="0" :max="1440" :step="5" />
                  <div class="field-tip">免费剩余时间少于此值的种子将被跳过，0 表示不限制</div>
                </el-form-item>
                <el-form-item label="自动启动 RSS 任务">
                  <el-switch v-model="form.auto_start" />
                  <div class="field-tip">
                    打开后程序一启动（包括重启容器）就运行 RSS 任务；关着时要在底部状态栏的「RSS
                    任务」里手动启动。 只影响 RSS 订阅，刷流、辅种、自动删种等不受影响
                  </div>
                </el-form-item>
              </div>
            </div>
          </div>
          <div class="gs-col">
            <div class="pt-strip">
              <PtIcon name="archive" :size="13" />
              <span>暂存种子清理</span>
            </div>
            <div class="settings-body">
              <div class="field-row">
                <el-form-item label="暂存种子保留时长（小时）">
                  <el-input-number v-model="form.retain_hours" :min="0" :max="8760" :step="1" />
                  <div class="field-tip">超期未推送将被自动清理。0 = 关闭自动清理</div>
                </el-form-item>
                <el-form-item label="最大重试次数">
                  <el-input-number v-model="form.max_retry" :min="0" :max="100" :step="1" />
                  <div class="field-tip">
                    推送失败达到此次数后清理暂存种子；0 = 不按重试次数删除
                  </div>
                </el-form-item>
              </div>
            </div>

            <div class="pt-strip">
              <PtIcon name="calendar-clock" :size="13" />
              <span>免费结束管理</span>
            </div>
            <div class="settings-body">
              <el-form-item label="免费结束自动删除">
                <el-switch v-model="form.auto_delete_on_free_end" />
                <div class="field-tip">
                  开启后，免费期结束时未下载完成的种子将自动从下载器中删除（含数据文件）；关闭时仅暂停，可在「暂停任务」页手动恢复或删除
                </div>
                <div v-if="form.auto_delete_on_free_end" class="pt-note pt-note--dang mode-note">
                  <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
                  <span
                    >已开启：免费期结束时未完成的种子及其数据文件会被自动删除，此操作不可恢复</span
                  >
                </div>
              </el-form-item>

              <el-form-item label="免费期结束前提前处理（分钟）">
                <el-input-number
                  v-model="form.free_end_advance_minutes"
                  :min="0"
                  :max="60"
                  :step="1" />
                <div class="field-tip">
                  提前 N 分钟暂停或删除未完成的种子，规避删除延迟与 tracker
                  汇报滞后导致的计费下载量；0 表示到点处理，新安装默认
                  5。要求免费的推送还要求免费剩余时间超过 N 分钟（至少 5 分钟），不够的不推送。
                </div>
                <div
                  v-if="form.free_end_advance_minutes === 0"
                  class="pt-note pt-note--warn mode-note"
                  data-testid="gs-advance-zero">
                  <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
                  <span
                    >提前量是 0：到点才暂停，tracker 汇报滞后时仍可能多出计费下载量，建议设为 5
                    分钟以上</span
                  >
                </div>
              </el-form-item>
            </div>

            <div class="pt-strip">
              <PtIcon name="calendar-check" :size="13" />
              <span>每日签到</span>
            </div>
            <div class="settings-body">
              <p class="field-tip moved-tip" data-testid="attendance-moved">
                签到时间窗、AI 签到和签到结果推送挪到了
                <router-link to="/attendance">自动签到</router-link>
                页，和每个站点的签到开关、签到记录放在一起
              </p>
            </div>

            <div class="pt-strip">
              <PtIcon name="chart-column" :size="13" />
              <span>每日战报</span>
            </div>
            <div class="settings-body">
              <div
                v-if="reportLoadFailed"
                class="pt-note pt-note--warn"
                data-testid="daily-report-load-failed">
                <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
                <span>每日战报设置读取失败，这一段暂时不能修改；刷新页面重试</span>
              </div>
              <el-form-item label="开启每日战报">
                <el-switch
                  v-model="reportForm.enabled"
                  :disabled="reportLoadFailed"
                  data-testid="daily-report-enabled" />
                <div class="field-tip">
                  每天在设定时刻（再错开几分钟）把当天各站的上传、下载（计费）、魔力增量，以及登录状态异常的站点和签到结果，发到选定的通知通道
                </div>
              </el-form-item>
              <div class="field-row">
                <el-form-item label="发送时刻">
                  <el-time-select
                    v-model="reportForm.time"
                    :disabled="reportLoadFailed || !reportForm.enabled"
                    start="00:00"
                    step="00:15"
                    end="23:45"
                    :clearable="false"
                    data-testid="daily-report-time" />
                  <div class="field-tip">服务器时区；通道处在静默时段时顺延到静默结束</div>
                </el-form-item>
                <el-form-item label="接收通道">
                  <el-select
                    v-model="reportForm.channel_ids"
                    :disabled="reportLoadFailed || !reportForm.enabled"
                    multiple
                    collapse-tags
                    placeholder="选择通知通道"
                    data-testid="daily-report-channels">
                    <el-option
                      v-for="c in reportChannels"
                      :key="c.id"
                      :label="c.name"
                      :value="c.id" />
                  </el-select>
                  <div class="field-tip">开启时至少选一个；没有通道时先去「消息通知」里添加</div>
                </el-form-item>
              </div>
            </div>

            <div class="pt-strip">
              <PtIcon name="network" :size="13" />
              <span>站点代理</span>
            </div>
            <div class="settings-body">
              <div
                v-if="proxyLoadFailed"
                class="pt-note pt-note--warn proxy-note"
                data-testid="site-proxy-load-failed">
                <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
                <span>站点代理设置读取失败，这一段暂时不能修改；刷新页面重试</span>
              </div>
              <div
                v-else-if="proxyUnreadable"
                class="pt-note pt-note--warn proxy-note"
                data-testid="site-proxy-unreadable">
                <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
                <span
                  >存着的代理地址解不开（加密密钥换过），现在按关闭处理；重新填写代理地址后保存</span
                >
              </div>
              <el-form-item label="站点请求走代理">
                <el-switch
                  v-model="proxyForm.enabled"
                  :disabled="proxyLoadFailed"
                  data-testid="site-proxy-enabled" />
                <div class="field-tip">
                  开启后，发给站点的请求（搜索与种子列表、取
                  RSS、下载种子、签到、取用户信息、辅种）按这里走，环境变量里的代理（HTTPS_PROXY）不再管站点；关闭时照旧按环境变量。保存后立即生效，不用重启
                </div>
              </el-form-item>
              <el-form-item label="代理地址">
                <el-input
                  v-model="proxyForm.proxy_url"
                  :disabled="proxyLoadFailed || !proxyForm.enabled"
                  placeholder="http://192.168.1.2:7890 或 socks5://用户名:密码@主机:1080"
                  data-testid="site-proxy-url" />
                <div class="field-tip">
                  支持 http、https、socks5、socks5h。密码加密保存，读出来显示成 ***，不改就原样保留
                </div>
              </el-form-item>
              <el-form-item label="哪些站点走代理">
                <el-radio-group
                  v-model="proxyForm.mode"
                  :disabled="proxyLoadFailed || !proxyForm.enabled"
                  data-testid="site-proxy-mode">
                  <el-radio-button
                    v-for="opt in proxyModeOptions"
                    :key="opt.value"
                    :value="opt.value">
                    {{ opt.label }}
                  </el-radio-button>
                </el-radio-group>
                <div v-if="activeProxyMode" class="field-tip">{{ activeProxyMode.desc }}</div>
              </el-form-item>
              <el-form-item
                v-if="proxyForm.mode !== 'all'"
                :label="proxyForm.mode === 'include' ? '走代理的站点' : '直连的站点'">
                <el-select
                  v-model="proxyForm.sites"
                  :disabled="proxyLoadFailed || !proxyForm.enabled"
                  multiple
                  filterable
                  collapse-tags
                  collapse-tags-tooltip
                  placeholder="选择站点"
                  data-testid="site-proxy-sites">
                  <el-option
                    v-for="o in proxyOptions"
                    :key="o.id"
                    :label="o.enabled ? o.name : `${o.name}（未启用）`"
                    :value="o.id" />
                </el-select>
                <div class="field-tip">列出的是 pt-tools 支持的全部站点，没启用的也能先选上</div>
              </el-form-item>
              <p class="field-tip proxy-tip">
                不影响下载器、IYUU、通知通道、TMDB、AI 助手等，它们按各自的设置；CloakBrowser
                在它自己的配置里设代理。走代理的站点看到的是代理出口的 IP，站点规则对 IP
                有要求的（例如 HDSky 的盒子规则）先确认再选
              </p>
            </div>
          </div>
        </div>
      </el-form>

      <template #footer>
        <span class="pt-foot-note">配置程序运行的全局参数和默认行为</span>
        <!-- 移动端页头是隐掉的，保存键只能留在这里 -->
        <el-button
          v-if="isMobile && !loadFailed"
          type="primary"
          :loading="saving"
          :disabled="loading"
          @click="save">
          <PtIcon v-if="!saving" name="save" :size="14" /><span>保存设置</span>
        </el-button>
      </template>
    </PtPanel>
  </div>
</template>

<style scoped>
/* 第一条区块条紧贴面板页头，两条发丝线会叠成 2px */
.gs-col:first-child > .pt-strip:first-child {
  border-top: 0;
}

/* 宽屏两栏：两栏各自从上往下排，中间一条竖线；右栏的第一条区块条也紧贴面板页头 */
@media (min-width: 1600px) {
  .gs-split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    align-items: start;
  }

  .gs-col + .gs-col {
    border-left: 1px solid var(--pt-border);
  }

  .gs-col + .gs-col > .pt-strip:first-child {
    border-top: 0;
  }
}

/* 下内边距留 0：末个字段自带的 16 下外边距正好等于 --pt-pad，凑成段尾留白 */
.settings-body {
  padding: var(--pt-pad) var(--pt-pad) 0;
}

/* 开关或单选下方的提醒块：与 field-tip 同一档缩进，不再套一层 el-alert 的厚边框 */
.mode-note {
  width: 100%;
  margin-top: 6px;
}

/*
 * 每日签到挪走以后留的一句指路、站点代理段尾的说明：不在字段里，段尾补上 16 的留白
 * （字段自带的下外边距这里没有）
 */
.moved-tip,
.proxy-tip {
  margin: 0 0 var(--pt-pad);
}

/* 读取失败 / 解不开的提醒与下面第一个字段隔开 */
.proxy-note {
  margin-bottom: var(--pt-pad);
}
</style>
