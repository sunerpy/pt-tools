<script setup lang="ts">
import { computed, nextTick, onMounted, ref, shallowRef, watch } from "vue";
import { useResizeObserver } from "@vueuse/core";
import type { AggregatedStatsResponse, DeltaSummary, UserInfoRange } from "@/api";
import ExportPoster from "@/components/exportPoster/ExportPoster.vue";
import { buildPosterModel, type PosterLogoData } from "@/components/exportPoster/model";
import {
  ensurePosterFonts,
  loadPosterLogos,
  posterToBlob,
} from "@/components/exportPoster/posterImage";
import {
  DEFAULT_POSTER_STYLE,
  POSTER_STYLES,
  posterStyle,
  type PosterStyleId,
} from "@/components/exportPoster/styles";
import { BridgeError, imageAction, request } from "./bridge";
import { useWords } from "./i18n";
const t = useWords();
const loading = ref(true);
const exporting = ref(false);
const error = ref("");
const note = ref("");
const forbidden = ref(false);
const stats = ref<AggregatedStatsResponse>();
const increments = ref<DeltaSummary | null>(null);
const logos = shallowRef(new Map<string, PosterLogoData>());
const title = ref(t("我的 PT 数据统计", "My PT statistics"));
const selected = ref<string[]>([]);
const showSites = ref(true);
const showIncrements = ref(true);
const mask = ref({ names: true, users: true, logos: true });
const range = ref<UserInfoRange>("7d");
const styleId = ref<PosterStyleId>(DEFAULT_POSTER_STYLE);
const colorway = ref("light");
const poster = ref<InstanceType<typeof ExportPoster>>();
const now = ref(new Date());
const stage = ref<HTMLElement>();
const stageWidth = ref(320);
const posterHeight = ref(0);
const posterBox = ref<HTMLElement>();
useResizeObserver(stage, ([entry]) => {
  if (entry) stageWidth.value = entry.contentRect.width;
});
useResizeObserver(posterBox, ([entry]) => {
  if (entry) posterHeight.value = entry.contentRect.height;
});
const scale = computed(() => Math.min(1, stageWidth.value / 640));
const ranges = computed(() => [
  { value: "today", label: t("今日", "Today") },
  { value: "7d", label: t("本周", "7 days") },
  { value: "30d", label: t("本月", "30 days") },
]);
const model = computed(() =>
  stats.value
    ? buildPosterModel({
        locale: t("zh", "en"),
        stats: stats.value,
        sites: stats.value.perSiteStats.filter((s) => selected.value.includes(s.site)),
        showSites: showSites.value,
        increments:
          showIncrements.value && increments.value?.sites.some((s) => s.hasBaseline)
            ? increments.value
            : null,
        incrementLabel: ranges.value.find((r) => r.value === range.value)!.label,
        title: title.value,
        mask: mask.value,
        logos: logos.value,
        now: now.value,
      })
    : null,
);
let incrementSequence = 0;
async function loadIncrements() {
  const seq = ++incrementSequence;
  try {
    const result = await request<DeltaSummary>(`/api/app/v1/stats/summary?range=${range.value}`);
    if (seq === incrementSequence) {
      increments.value = result;
      note.value = "";
    }
  } catch {
    if (seq === incrementSequence) {
      increments.value = null;
      note.value = t(
        "周期增量暂不可用，图片不会包含该项",
        "Period changes are unavailable and will be omitted from the image",
      );
    }
  }
}
watch(range, () => void loadIncrements());
watch(styleId, () => {
  colorway.value = posterStyle(styleId.value).colorways[0]!.id;
});
async function load() {
  loading.value = true;
  error.value = "";
  forbidden.value = false;
  try {
    const result = await request<AggregatedStatsResponse>("/api/app/v1/stats/aggregated");
    stats.value = result;
    selected.value = [...result.perSiteStats]
      .sort((a, b) => b.uploaded - a.uploaded)
      .slice(0, 10)
      .map((s) => s.site);
    await Promise.all([loadIncrements(), ensurePosterFonts()]);
    const readLogo = async (site: string) => {
      const data = await request<{ base64: string; mime?: string; contentType?: string }>(
        `/api/app/v1/favicon/${encodeURIComponent(site)}`,
      );
      const mime = data.mime || data.contentType || "image/png";
      if (!/^image\/(png|jpeg|webp|gif|x-icon|vnd.microsoft.icon)$/.test(mime))
        throw new Error("Unsupported image");
      return `data:${mime};base64,${data.base64}`;
    };
    const loaded = new Map<string, PosterLogoData>();
    const sites = result.perSiteStats.map((s) => s.site);
    // Leave tunnel slots for the native page and other API requests.
    for (let i = 0; i < sites.length; i += 4) {
      for (const [site, logo] of await loadPosterLogos(sites.slice(i, i + 4), readLogo))
        loaded.set(site, logo);
    }
    logos.value = loaded;
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
    forbidden.value = e instanceof BridgeError && [401, 403].includes(e.status);
  } finally {
    loading.value = false;
  }
}
async function exportImage(action: "share" | "save") {
  if (exporting.value) return;
  exporting.value = true;
  error.value = "";
  note.value = "";
  try {
    now.value = new Date();
    await nextTick();
    const el = poster.value?.posterEl();
    if (!el) throw new Error(t("海报还没画出来", "Poster is not ready"));
    const blob = await posterToBlob(el, poster.value!.fonts());
    const result = await imageAction(action, blob);
    if (result?.saved === true && action === "save") note.value = t("图片已保存", "Image saved");
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    exporting.value = false;
  }
}
onMounted(() => void load());
</script>
<template>
  <div v-if="loading" role="status" class="mobile-state">
    {{ t("正在读取统计…", "Loading statistics…") }}
  </div>
  <template v-else>
    <div v-if="error" role="alert" class="mobile-error">{{ error }}</div>
    <el-button v-if="!stats && !forbidden" @click="load">{{ t("重试", "Retry") }}</el-button>
    <p v-if="stats && !stats.siteCount" class="mobile-state">
      {{ t("还没有统计数据，请先刷新站点数据", "No statistics yet. Refresh site data first.") }}
    </p>
    <template v-else-if="stats && model">
      <el-form label-position="top" :disabled="exporting">
        <el-form-item :label="t('标题', 'Title')"
          ><el-input v-model="title" :maxlength="80"
        /></el-form-item>
        <el-form-item :label="t('样式', 'Style')"
          ><el-select v-model="styleId"
            ><el-option
              v-for="s in POSTER_STYLES"
              :key="s.id"
              :value="s.id"
              :label="t(s.name, s.id)" /></el-select
        ></el-form-item>
        <el-form-item :label="t('配色', 'Colorway')"
          ><el-select v-model="colorway"
            ><el-option
              v-for="c in posterStyle(styleId).colorways"
              :key="c.id"
              :value="c.id"
              :label="t(c.name, c.id)" /></el-select
        ></el-form-item>
        <section class="mobile-section">
          <h2>{{ t("隐私", "Privacy") }}</h2>
          <div class="mobile-checks">
            <el-checkbox v-model="mask.names">{{ t("遮蔽站点名", "Mask site names") }}</el-checkbox
            ><el-checkbox v-model="mask.users">{{ t("遮蔽用户名", "Mask usernames") }}</el-checkbox
            ><el-checkbox v-model="mask.logos">{{ t("遮蔽图标", "Mask logos") }}</el-checkbox>
          </div>
          <p v-if="!mask.names || !mask.users || !mask.logos" role="alert" class="mobile-note">
            {{
              t(
                "图片包含未遮蔽的信息，请核对预览后再分享",
                "The image contains unmasked information. Review it before sharing.",
              )
            }}
          </p>
        </section>
        <el-checkbox v-model="showSites">{{
          t("附上逐站明细", "Include site details")
        }}</el-checkbox>
        <el-checkbox v-model="showIncrements">{{
          t("显示周期增量", "Include period changes")
        }}</el-checkbox>
        <el-form-item v-if="showIncrements" :label="t('周期', 'Period')"
          ><el-select v-model="range"
            ><el-option
              v-for="r in ranges"
              :key="r.value"
              :value="r.value"
              :label="r.label" /></el-select
        ></el-form-item>
        <el-collapse v-if="showSites"
          ><el-collapse-item :title="t('选择站点', 'Select sites')" name="sites"
            ><el-checkbox-group v-model="selected" class="mobile-checks"
              ><el-checkbox
                v-for="site in stats.perSiteStats"
                :key="site.site"
                :value="site.site"
                >{{ site.site }}</el-checkbox
              ></el-checkbox-group
            ></el-collapse-item
          ></el-collapse
        >
      </el-form>
      <p v-if="note" role="status" class="mobile-note">{{ note }}</p>
      <div ref="stage" class="mobile-preview">
        <div
          class="mobile-preview-fit"
          :style="{
            width: `${640 * scale}px`,
            height: posterHeight ? `${posterHeight * scale}px` : undefined,
          }">
          <div
            ref="posterBox"
            class="mobile-preview-poster"
            :style="{ transform: `scale(${scale})` }">
            <ExportPoster
              ref="poster"
              :model="model"
              :style-id="styleId"
              :colorway="colorway"
              :locale="t('zh', 'en')" />
          </div>
        </div>
      </div>
      <div class="mobile-actions">
        <el-button :loading="exporting" @click="exportImage('save')">{{
          t("保存图片", "Save image")
        }}</el-button
        ><el-button type="primary" :loading="exporting" @click="exportImage('share')">{{
          t("分享图片", "Share image")
        }}</el-button>
      </div>
    </template>
  </template>
</template>
