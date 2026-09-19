<script setup lang="ts">
import { globalApi, type GlobalSettings } from "@/api";
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
const { loading, state, errorText, run } = useDataState();
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
  if (!loaded.value) return "";
  const f = form.value;
  const parts = [
    `间隔 ${f.default_interval_minutes} 分钟`,
    activeFilterMode.value?.short ?? "",
    f.torrent_min_size_gb > 0
      ? `种子 ${f.torrent_min_size_gb}–${f.torrent_size_gb} GB`
      : `种子上限 ${f.torrent_size_gb} GB`,
    f.download_limit_enabled ? `预估 ${f.download_speed_limit} MB/s` : "未启用限速判断",
    f.retain_hours > 0 ? `暂存保留 ${f.retain_hours} 小时` : "暂存不自动清理",
    f.auto_start ? "启动即运行" : "手动启动",
  ].filter(Boolean);
  // 免费期结束自动删数据是不可恢复的，开着就一定要在摘要里露出来
  if (f.auto_delete_on_free_end) parts.push("免费结束自动删除");
  return parts.join(" · ");
});

async function loadData() {
  const data = await run(() => globalApi.get());
  if (!data) {
    loaded.value = false;
    return;
  }
  form.value = {
    ...data,
    default_interval_minutes: Math.max(5, data.default_interval_minutes || 10),
  };
  loaded.value = true;
  showWarning.value = !form.value.download_dir;
}

onMounted(loadData);

async function save() {
  if (!form.value.download_dir) {
    ElMessage.error("下载目录不能为空");
    showWarning.value = true;
    return;
  }

  saving.value = true;
  try {
    await globalApi.save({
      ...form.value,
      default_interval_minutes: Math.max(5, form.value.default_interval_minutes),
    });
    ElMessage.success("保存成功");
    showWarning.value = false;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
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
    这一页没有表格，也就没有工具栏带 / 表格带 / 页脚带。
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

      <el-form v-else :model="form" label-position="top" class="pt-form settings-form">
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
              <div class="field-tip">小于此大小的种子将被忽略；0 = 不限制，且必须小于最大值</div>
            </el-form-item>
          </div>

          <el-form-item label="默认下载模式">
            <el-radio-group v-model="form.default_filter_mode">
              <el-radio-button v-for="opt in filterModeOptions" :key="opt.value" :value="opt.value">
                {{ opt.label }}
              </el-radio-button>
            </el-radio-group>
            <div v-if="activeFilterMode" class="field-tip">{{ activeFilterMode.desc }}</div>
            <div class="pt-note pt-note--warn mode-note">
              <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
              <span>
                v0.26.0 起：智能模式下，为 RSS 关联过滤规则 =
                精准下载，不再附带自动下载免费种子。RSS
                订阅级别可覆盖此默认值，全局大小上限对所有模式都生效。
              </span>
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
            <el-form-item label="自动启动任务">
              <el-switch v-model="form.auto_start" />
              <div class="field-tip">程序启动时立即开启已启用的 RSS 检查任务</div>
            </el-form-item>
          </div>
        </div>

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
              <div class="field-tip">推送失败达到此次数后清理暂存种子；0 = 不按重试次数删除</div>
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
              开启后，免费期结束时未下载完成的种子将自动从下载器中删除（含数据文件）；关闭时仅暂停，可在「暂停任务管理」页手动恢复或删除
            </div>
            <div v-if="form.auto_delete_on_free_end" class="pt-note pt-note--dang mode-note">
              <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
              <span>已开启：免费期结束时未完成的种子及其数据文件会被自动删除，此操作不可恢复</span>
            </div>
          </el-form-item>

          <el-form-item label="免费期结束前提前处理（分钟）">
            <el-input-number v-model="form.free_end_advance_minutes" :min="0" :max="60" :step="1" />
            <div class="field-tip">
              提前 N 分钟暂停或删除未完成的种子，规避删除延迟与 tracker 汇报滞后导致的超额下载量；0
              表示到点处理（默认）。建议 ≥5 分钟。
            </div>
          </el-form-item>
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
.settings-form > .pt-strip:first-child {
  border-top: 0;
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
</style>
