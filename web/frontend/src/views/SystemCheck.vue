<script setup lang="ts">
/*
 * 系统体检（媒体中心 M8）：一次检查 TMDB、站点、下载器、媒体服务器、路径映射、媒体库、磁盘、时间、备份与密钥，
 * 每项写现在的样子与怎么修。打开页面时显示上一次的结果，「立即体检」现查一遍（最多 60 秒，同时只跑一次）。
 * 不改任何设置（媒体库同「检查」，在库目录里试建一个文件再删掉）；站点只看最近一次登录探测，不发新的请求。
 * 画板没有这一页，沿用列表页的样式：页头 + 面板。
 */
import { type HealthReport, systemCheckApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { checkGroups, checkHeadline, healthStatus } from "@/utils/systemCheck";
import { ElMessage } from "element-plus";
import { computed, onMounted, ref } from "vue";

const isMobile = useIsMobile();

const report = ref<HealthReport | null>(null);
const loading = ref(true);
const loadError = ref("");
const running = ref(false);
const onlyProblems = ref(false);

async function load() {
  loading.value = true;
  loadError.value = "";
  try {
    report.value = (await systemCheckApi.last()).report;
  } catch (e) {
    loadError.value = (e as Error)?.message || "读取体检结果失败";
  } finally {
    loading.value = false;
  }
}

async function run() {
  if (running.value) return;
  running.value = true;
  try {
    report.value = (await systemCheckApi.run()).report;
    const r = report.value;
    if (r.fail) ElMessage.warning(`体检完了：${r.fail} 项失败`);
    else if (r.warn) ElMessage.info(`体检完了：${r.warn} 项提醒`);
    else ElMessage.success("体检完了：都正常");
  } catch (e) {
    ElMessage.error((e as Error)?.message || "体检失败");
  } finally {
    running.value = false;
  }
}

onMounted(() => void load());

const sub = computed(() => (loading.value ? "加载中…" : checkHeadline(report.value)));
const groups = computed(() => checkGroups(report.value, onlyProblems.value));
const problems = computed(() => (report.value ? report.value.fail + report.value.warn : 0));
</script>

<template>
  <div class="pt-cards sc" :class="{ 'is-mobile': isMobile }" data-testid="system-check">
    <PtHeadSub>{{ sub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button
        v-if="!isMobile"
        type="primary"
        :loading="running"
        data-testid="sc-run"
        @click="run">
        <PtIcon v-if="!running" name="activity" :size="15" /><span>{{
          running ? "体检中…" : "立即体检"
        }}</span>
      </el-button>
    </Teleport>

    <PtPanel title="系统体检" icon="activity" :count="report?.items.length" padding="none">
      <template v-if="report" #actions>
        <el-radio-group v-model="onlyProblems" size="small" data-testid="sc-filter">
          <el-radio-button :value="false">全部</el-radio-button>
          <el-radio-button :value="true">有问题的 {{ problems }}</el-radio-button>
        </el-radio-group>
      </template>

      <div v-if="isMobile" class="sc-mbar">
        <el-button type="primary" :loading="running" data-testid="sc-run" @click="run">
          {{ running ? "体检中…" : "立即体检" }}
        </el-button>
      </div>

      <PtDataState v-if="loading" state="loading" dense />
      <PtDataState
        v-else-if="loadError"
        state="error"
        :sub="loadError"
        dense
        data-testid="sc-load-error" />
      <PtDataState
        v-else-if="!report"
        state="empty"
        title="没体检过"
        sub="点「立即体检」查一遍，不改任何设置"
        data-testid="sc-empty" />
      <template v-else>
        <div class="sc-summary" data-testid="sc-summary">
          <span class="sc-sum is-ok"><i />正常 {{ report.ok }}</span>
          <span class="sc-sum is-warn"><i />提醒 {{ report.warn }}</span>
          <span class="sc-sum is-dang"><i />失败 {{ report.fail }}</span>
          <span class="sc-sum is-neutral"><i />跳过 {{ report.skip }}</span>
        </div>
        <p v-if="!groups.length" class="sc-none" data-testid="sc-none">没有有问题的项</p>
        <section
          v-for="g in groups"
          :key="g.key"
          class="sc-group"
          :data-testid="`sc-group-${g.key}`">
          <div class="sc-group__head">
            <PtIcon :name="g.icon" :size="14" class="sc-group__icon" />
            <span class="sc-group__title">{{ g.label }}</span>
            <PtIcon
              :name="healthStatus(g.worst).icon"
              :size="14"
              class="sc-group__state"
              :class="`tone-${healthStatus(g.worst).tone}`" />
          </div>
          <div
            v-for="it in g.items"
            :key="it.key"
            class="sc-item"
            :class="`st-${it.status}`"
            :data-testid="`sc-item-${it.key}`">
            <PtIcon
              :name="healthStatus(it.status).icon"
              :size="16"
              class="sc-item__icon"
              :class="`tone-${healthStatus(it.status).tone}`"
              :aria-label="healthStatus(it.status).label" />
            <div class="sc-item__text">
              <div class="sc-item__title">{{ it.title }}</div>
              <div v-if="it.detail" class="sc-item__detail">{{ it.detail }}</div>
              <div v-if="it.hint" class="sc-item__hint">
                <PtIcon name="wrench" :size="12" /><span>{{ it.hint }}</span>
              </div>
            </div>
          </div>
        </section>
      </template>
    </PtPanel>
  </div>
</template>

<style scoped>
.sc-mbar {
  display: flex;
  padding: 12px 14px 0;
}

.sc-mbar .el-button {
  flex: 1 1 auto;
  height: 40px;
}

.sc-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--pt-border);
}

.sc-sum {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  height: 24px;
  padding: 0 10px;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  border-radius: 12px;
}

.sc-sum i {
  width: 5px;
  height: 5px;
  background: currentcolor;
  border-radius: 50%;
}

.sc-sum.is-ok {
  color: var(--pt-ok);
  background: var(--pt-ok-soft);
}

.sc-sum.is-warn {
  color: var(--pt-warn);
  background: var(--pt-warn-soft);
}

.sc-sum.is-dang {
  color: var(--pt-dang);
  background: var(--pt-dang-soft);
}

.sc-sum.is-neutral {
  color: var(--pt-t2);
  background: var(--pt-hover);
}

.sc-none {
  margin: 0;
  padding: 16px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.sc-group + .sc-group {
  border-top: 1px solid var(--pt-border);
}

.sc-group__head {
  display: flex;
  gap: 8px;
  align-items: center;
  height: 34px;
  padding: 0 16px;
  background: var(--pt-canvas);
}

.sc-group__icon {
  color: var(--pt-t3);
}

.sc-group__title {
  font-size: var(--pt-fz-sm);
  font-weight: 700;
  color: var(--pt-t2);
}

.sc-group__state {
  margin-left: auto;
}

.sc-item {
  display: flex;
  gap: 12px;
  align-items: flex-start;
  padding: 10px 16px;
}

.sc-item + .sc-item {
  border-top: 1px solid var(--pt-border);
}

.sc-item__icon {
  flex: 0 0 auto;
  margin-top: 1px;
}

.tone-ok {
  color: var(--pt-ok);
}

.tone-warn {
  color: var(--pt-warn);
}

.tone-dang {
  color: var(--pt-dang);
}

.tone-neutral {
  color: var(--pt-t3);
}

.sc-item__text {
  flex: 1 1 auto;
  min-width: 0;
}

.sc-item__title {
  font-size: var(--pt-fz-body);
  font-weight: 500;
  color: var(--pt-t1);
  overflow-wrap: anywhere;
}

.sc-item__detail {
  margin-top: 3px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  overflow-wrap: anywhere;
}

.sc-item__hint {
  display: flex;
  gap: 6px;
  align-items: flex-start;
  margin-top: 6px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

.sc-item__hint .pt-icon,
.sc-item__hint svg {
  flex: 0 0 auto;
  margin-top: 2px;
}

.sc-item.st-skip .sc-item__title,
.sc-item.st-skip .sc-item__detail {
  color: var(--pt-t3);
}

.sc.is-mobile .sc-item,
.sc.is-mobile .sc-group__head,
.sc.is-mobile .sc-summary {
  padding-right: 14px;
  padding-left: 14px;
}
</style>
