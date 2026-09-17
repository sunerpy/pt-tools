<script setup lang="ts">
import { globalApi, maintenanceApi, type CleanResult, type GlobalSettings } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref, watch } from "vue";

const loading = ref(false);
const saving = ref(false);
const scopeTagInput = ref("");
const protectTagInput = ref("");
const selectedPreset = ref("");

const presets = [
  {
    label: "日常刷流（推荐）",
    value: "daily",
    desc: "做种72h 或 分享率2.0 即删",
    config: {
      cleanup_condition_mode: "or",
      cleanup_max_seed_time_h: 72,
      cleanup_min_ratio: 2.0,
      cleanup_max_inactive_h: 0,
      cleanup_slow_seed_time_h: 0,
      cleanup_slow_max_ratio: 0,
      cleanup_del_free_expired: true,
    },
  },
  {
    label: "空间紧张",
    value: "tight",
    desc: "做种48h 或 分享率1.0 或 不活跃24h 或 低效做种",
    config: {
      cleanup_condition_mode: "or",
      cleanup_max_seed_time_h: 48,
      cleanup_min_ratio: 1.0,
      cleanup_max_inactive_h: 24,
      cleanup_slow_seed_time_h: 48,
      cleanup_slow_max_ratio: 0.1,
      cleanup_del_free_expired: true,
    },
  },
  {
    label: "追求高分享率",
    value: "ratio",
    desc: "做种168h 且 分享率3.0 同时满足才删",
    config: {
      cleanup_condition_mode: "and",
      cleanup_max_seed_time_h: 168,
      cleanup_min_ratio: 3.0,
      cleanup_max_inactive_h: 0,
      cleanup_slow_seed_time_h: 0,
      cleanup_slow_max_ratio: 0,
      cleanup_del_free_expired: true,
    },
  },
  {
    label: "仅清理无效种子",
    value: "minimal",
    desc: "只删低效做种和免费到期未完成的",
    config: {
      cleanup_condition_mode: "or",
      cleanup_max_seed_time_h: 0,
      cleanup_min_ratio: 0,
      cleanup_max_inactive_h: 0,
      cleanup_slow_seed_time_h: 72,
      cleanup_slow_max_ratio: 0.05,
      cleanup_del_free_expired: true,
    },
  },
];

function applyPreset(val: string) {
  const preset = presets.find((p) => p.value === val);
  if (!preset) return;
  Object.assign(form.value, preset.config);
}

function detectPreset() {
  const f = form.value;
  for (const p of presets) {
    const cfg = p.config as Record<string, unknown>;
    const matched = Object.keys(cfg).every(
      (k) => (f as unknown as Record<string, unknown>)[k] === cfg[k],
    );
    if (matched) {
      selectedPreset.value = p.value;
      return;
    }
  }
  selectedPreset.value = "";
}

function tagsToArray(s: unknown): string[] {
  if (!s || typeof s !== "string") return [];
  return s
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
}

function addScopeTag() {
  const val = scopeTagInput.value.trim();
  if (val && !form.value.cleanup_scope_tags.includes(val)) {
    form.value.cleanup_scope_tags.push(val);
  }
  scopeTagInput.value = "";
}

function addProtectTag() {
  const val = protectTagInput.value.trim();
  if (val && !form.value.cleanup_protect_tags.includes(val)) {
    form.value.cleanup_protect_tags.push(val);
  }
  protectTagInput.value = "";
}

function removeTag(arr: string[], index: number) {
  arr.splice(index, 1);
}

const form = ref({
  cleanup_enabled: false,
  cleanup_interval_min: 30,
  cleanup_scope: "database",
  cleanup_scope_tags: [] as string[],
  cleanup_remove_data: true,
  cleanup_condition_mode: "or",
  cleanup_max_seed_time_h: 0,
  cleanup_min_ratio: 0,
  cleanup_max_inactive_h: 0,
  cleanup_slow_seed_time_h: 0,
  cleanup_slow_max_ratio: 0,
  cleanup_del_free_expired: true,
  cleanup_disk_protect: true,
  cleanup_min_disk_space_gb: 50,
  cleanup_protect_dl: false,
  cleanup_protect_hr: true,
  cleanup_min_retain_h: 24,
  cleanup_protect_tags: [] as string[],
  peer_ratio_enabled: false,
  peer_ratio_max_sl: 30,
  peer_ratio_interval_min: 10,
  peer_ratio_remove_data: false,
});

const presetFields = computed(() => [
  form.value.cleanup_condition_mode,
  form.value.cleanup_max_seed_time_h,
  form.value.cleanup_min_ratio,
  form.value.cleanup_max_inactive_h,
  form.value.cleanup_slow_seed_time_h,
  form.value.cleanup_slow_max_ratio,
  form.value.cleanup_del_free_expired,
]);

watch(presetFields, () => detectPreset());

onMounted(async () => {
  loading.value = true;
  try {
    const data = await globalApi.get();
    const d = data as unknown as Record<string, unknown>;
    const f = form.value as unknown as Record<string, unknown>;
    Object.keys(f).forEach((key) => {
      if (key in d) {
        f[key] = d[key];
      }
    });
    form.value.cleanup_scope_tags = tagsToArray(d.cleanup_scope_tags as string);
    form.value.cleanup_protect_tags = tagsToArray(d.cleanup_protect_tags as string);
    detectPreset();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败");
  } finally {
    loading.value = false;
  }
});

async function save() {
  saving.value = true;
  try {
    const current = await globalApi.get();
    const payload = {
      ...current,
      ...form.value,
      cleanup_scope_tags: form.value.cleanup_scope_tags.join(","),
      cleanup_protect_tags: form.value.cleanup_protect_tags.join(","),
    };
    await globalApi.save(payload as unknown as GlobalSettings);
    ElMessage.success("保存成功");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

const workdirCategoryOptions = [
  { value: "logs", label: "日志文件", desc: "logs/ 目录下的历史日志" },
  { value: "staging", label: "暂存种子文件", desc: "临时下载的 .torrent 暂存文件" },
  { value: "backups", label: "旧配置备份", desc: "自动生成的历史配置备份" },
];

const workdirCategories = ref<string[]>(["logs", "staging", "backups"]);
const keepBackups = ref(5);
const previewing = ref(false);
const cleaning = ref(false);
const cleanPreview = ref<CleanResult | null>(null);
const cleanResult = ref<CleanResult | null>(null);

function categoryLabel(name: string): string {
  return workdirCategoryOptions.find((o) => o.value === name)?.label ?? name;
}

async function previewClean() {
  previewing.value = true;
  cleanResult.value = null;
  try {
    cleanPreview.value = await maintenanceApi.preview();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "预览失败");
  } finally {
    previewing.value = false;
  }
}

async function executeClean() {
  if (workdirCategories.value.length === 0) {
    ElMessage.warning("请至少选择一个清理项");
    return;
  }
  const selectedNames = workdirCategories.value.map(categoryLabel).join("、");
  try {
    await ElMessageBox.confirm(
      `即将清理以下内容：${selectedNames}。数据库、密钥等受保护项不会被删除。此操作不可撤销，确认继续？`,
      "确认清理工作目录",
      {
        confirmButtonText: "立即清理",
        cancelButtonText: "取消",
        type: "warning",
      },
    );
  } catch {
    return;
  }

  cleaning.value = true;
  try {
    cleanResult.value = await maintenanceApi.clean({
      categories: workdirCategories.value,
      dryRun: false,
      keepBackups: keepBackups.value,
    });
    ElMessage.success(
      `清理完成，共删除 ${cleanResult.value.totalDeleted} 项，释放 ${cleanResult.value.totalFreedHuman}`,
    );
    try {
      cleanPreview.value = await maintenanceApi.preview();
    } catch {
      cleanPreview.value = null;
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "清理失败");
  } finally {
    cleaning.value = false;
  }
}
</script>

<template>
  <div class="cleanup-page">
    <PtPanel v-loading="loading" title="自动删种" icon="trash-2" padding="none">
      <template #actions>
        <PtStatusPill :tone="form.cleanup_enabled ? 'ok' : 'neutral'" size="sm">
          {{ form.cleanup_enabled ? "运行中" : "未启用" }}
        </PtStatusPill>
      </template>

      <el-form :model="form" label-position="top" class="pt-form settings-form">
        <div class="pt-strip">
          <PtIcon name="sliders-horizontal" :size="13" />
          <span>基本设置</span>
        </div>
        <div class="settings-body">
          <el-form-item label="启用自动删种">
            <el-switch v-model="form.cleanup_enabled" />
            <div class="field-tip">开启后按下面的规则定时清理符合条件的种子</div>
          </el-form-item>

          <template v-if="form.cleanup_enabled">
            <div class="field-row">
              <el-form-item label="检查间隔（分钟）">
                <el-input-number
                  v-model="form.cleanup_interval_min"
                  :min="5"
                  :max="1440"
                  :step="5" />
              </el-form-item>
              <el-form-item label="删除时连数据文件一起删">
                <el-switch v-model="form.cleanup_remove_data" />
                <div class="field-tip">关掉则只从下载器移除，磁盘上的文件保留</div>
              </el-form-item>
            </div>

            <el-form-item label="管理范围">
              <el-radio-group v-model="form.cleanup_scope" class="scope-list">
                <div class="scope" :class="{ 'is-on': form.cleanup_scope === 'database' }">
                  <el-radio value="database">仅本应用推送的种子（推荐）</el-radio>
                  <p class="scope__desc">
                    按数据库记录精确匹配，只管理 pt-tools 推送并记录在案的种子
                  </p>
                </div>

                <div class="scope" :class="{ 'is-on': form.cleanup_scope === 'tag' }">
                  <el-radio value="tag">按标签匹配</el-radio>
                  <p class="scope__desc">下载器里带这些标签的种子都会纳入管理</p>
                  <template v-if="form.cleanup_scope === 'tag'">
                    <div class="tagbox">
                      <el-tag
                        v-for="(tag, i) in form.cleanup_scope_tags"
                        :key="tag"
                        closable
                        size="small"
                        @close="removeTag(form.cleanup_scope_tags, i)">
                        {{ tag }}
                      </el-tag>
                      <el-input
                        v-model="scopeTagInput"
                        size="small"
                        class="taginput"
                        placeholder="输入标签后回车"
                        @keyup.enter="addScopeTag()" />
                    </div>
                    <div class="pt-note pt-note--warn scope__note">
                      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
                      <span>你自己手动打了同样标签的种子也会被一起管理。</span>
                    </div>
                  </template>
                </div>

                <div class="scope" :class="{ 'is-on': form.cleanup_scope === 'all' }">
                  <el-radio value="all">下载器中所有种子</el-radio>
                  <p class="scope__desc">不区分来源，下载器里的每一个种子都参与判断</p>
                  <div
                    v-if="form.cleanup_scope === 'all'"
                    class="pt-note pt-note--dang scope__note">
                    <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
                    <span>危险：非本应用添加的种子也会被删，可能误删重要数据。</span>
                  </div>
                </div>
              </el-radio-group>
            </el-form-item>
          </template>
        </div>

        <template v-if="form.cleanup_enabled">
          <div class="pt-strip">
            <PtIcon name="target" :size="13" />
            <span>删除条件</span>
          </div>
          <div class="settings-body">
            <el-form-item label="推荐方案">
              <el-select
                v-model="selectedPreset"
                placeholder="选一个预设，一键填好下面的参数"
                clearable
                style="width: 100%"
                @change="applyPreset">
                <el-option v-for="p in presets" :key="p.value" :label="p.label" :value="p.value">
                  <span class="opt">
                    <span>{{ p.label }}</span>
                    <span class="opt__desc">{{ p.desc }}</span>
                  </span>
                </el-option>
              </el-select>
              <div v-if="selectedPreset" class="field-tip field-tip--ok">
                已选「{{ presets.find((p) => p.value === selectedPreset)?.label }}」，改动下面任一
                参数后这个标记会自动清除
              </div>
              <div v-else class="field-tip">只填充表单，点保存才真正生效</div>
            </el-form-item>

            <el-form-item label="条件关系">
              <el-radio-group v-model="form.cleanup_condition_mode">
                <el-radio-button value="or">任一满足即删（OR）</el-radio-button>
                <el-radio-button value="and">全部满足才删（AND）</el-radio-button>
              </el-radio-group>
              <div v-if="form.cleanup_condition_mode === 'or'" class="field-tip">
                下面任意一个条件命中就删，低效做种也算一个条件
              </div>
              <div v-else class="field-tip">
                下面所有已配置的条件都要同时满足；填 0 或未启用的条件不参与判断
              </div>
            </el-form-item>

            <div class="field-row">
              <el-form-item label="做种时间超过（小时）">
                <el-input-number v-model="form.cleanup_max_seed_time_h" :min="0" :step="1" />
                <div class="field-tip">0 表示不限制</div>
              </el-form-item>
              <el-form-item label="分享率达到">
                <el-input-number
                  v-model="form.cleanup_min_ratio"
                  :min="0"
                  :step="0.1"
                  :precision="2" />
                <div class="field-tip">0 表示不限制</div>
              </el-form-item>
              <el-form-item label="不活跃超过（小时）">
                <el-input-number v-model="form.cleanup_max_inactive_h" :min="0" :step="1" />
                <div class="field-tip">0 表示不限制</div>
              </el-form-item>
            </div>

            <div class="field-row">
              <el-form-item label="低效做种：做种超过（小时）">
                <el-input-number v-model="form.cleanup_slow_seed_time_h" :min="0" :step="1" />
              </el-form-item>
              <el-form-item label="但分享率仍低于">
                <el-input-number
                  v-model="form.cleanup_slow_max_ratio"
                  :min="0"
                  :step="0.01"
                  :precision="2" />
              </el-form-item>
            </div>

            <div
              v-if="form.cleanup_slow_seed_time_h > 0 && form.cleanup_slow_max_ratio > 0"
              class="pt-note slow-note">
              <PtIcon name="info" :size="14" class="pt-note__icon" />
              <span>
                做种超过 {{ form.cleanup_slow_seed_time_h }} 小时、分享率仍低于
                {{ form.cleanup_slow_max_ratio }} 的已完成种子会被删除。
              </span>
            </div>
            <div v-else class="field-tip slow-note">
              低效做种要两个值都填才生效，任一为 0 则不启用这个条件
            </div>

            <el-form-item>
              <el-checkbox v-model="form.cleanup_del_free_expired">
                免费到期仍未下完的种子自动删除
              </el-checkbox>
              <div class="field-tip">
                只对本应用推送、且记录了免费结束时间的种子有效，与上面的管理范围无关
              </div>
            </el-form-item>
          </div>

          <div class="pt-strip">
            <PtIcon name="hard-drive" :size="13" />
            <span>磁盘空间保护</span>
          </div>
          <div class="settings-body">
            <el-form-item>
              <el-checkbox v-model="form.cleanup_disk_protect">启用磁盘空间保护</el-checkbox>
            </el-form-item>
            <el-form-item v-if="form.cleanup_disk_protect" label="最低剩余空间（GB）">
              <el-input-number v-model="form.cleanup_min_disk_space_gb" :min="1" :max="10000" />
              <div class="field-tip">
                低于这个值时：RSS 停止推送新种子，同时按优先级强制删种腾空间
              </div>
            </el-form-item>
          </div>

          <div class="pt-strip">
            <PtIcon name="shield" :size="13" />
            <span>保护规则</span>
            <span class="pt-strip__end">命中任一条即不删</span>
          </div>
          <div class="settings-body">
            <el-form-item>
              <el-checkbox v-model="form.cleanup_protect_dl">保护下载中的种子</el-checkbox>
              <div class="field-tip">正在下载或校验中的种子不会被删</div>
            </el-form-item>

            <el-form-item>
              <el-checkbox v-model="form.cleanup_protect_hr">保护 H&amp;R 种子</el-checkbox>
              <div class="field-tip">
                没满足站点 H&amp;R 做种时长要求的不删；无 H&amp;R 的站点不受影响
              </div>
            </el-form-item>

            <div class="field-row">
              <el-form-item label="最短保种时间（小时）">
                <el-input-number v-model="form.cleanup_min_retain_h" :min="0" />
                <div class="field-tip">加进下载器还不够这么久的种子不删</div>
              </el-form-item>
              <el-form-item label="保护标签">
                <div class="tagbox">
                  <el-tag
                    v-for="(tag, i) in form.cleanup_protect_tags"
                    :key="tag"
                    closable
                    size="small"
                    @close="removeTag(form.cleanup_protect_tags, i)">
                    {{ tag }}
                  </el-tag>
                  <el-input
                    v-model="protectTagInput"
                    size="small"
                    class="taginput"
                    placeholder="输入标签后回车"
                    @keyup.enter="addProtectTag()" />
                </div>
                <div class="field-tip">带这些标签的种子一律不删</div>
              </el-form-item>
            </div>
          </div>
        </template>
      </el-form>

      <template #footer>
        <span class="pt-foot-note">删除动作不可撤销，改完记得保存</span>
        <el-button type="primary" :loading="saving" @click="save">
          <PtIcon name="save" :size="14" /><span>保存设置</span>
        </el-button>
      </template>
    </PtPanel>

    <PtPanel title="做种竞争度监控" icon="users" padding="none">
      <template #actions>
        <el-switch v-model="form.peer_ratio_enabled" size="small" />
      </template>

      <el-form label-position="top" class="pt-form" :disabled="!form.peer_ratio_enabled">
        <div class="settings-body settings-body--lead">
          <div class="field-tip field-tip--lead">
            盯着做种中种子的 Seeder/Leecher 比值，竞争度过高时自动暂停或删除。
          </div>

          <div class="field-row">
            <el-form-item label="S/L 比值上限">
              <el-input-number v-model="form.peer_ratio_max_sl" :min="1" :step="5" :precision="1" />
              <div class="field-tip">比值超过这个数就触发，最小 1</div>
            </el-form-item>
            <el-form-item label="检查间隔（分钟）">
              <el-input-number v-model="form.peer_ratio_interval_min" :min="5" :step="5" />
              <div class="field-tip">从 Tracker 拿 Swarm 级别的做种/下载人数来判断</div>
            </el-form-item>
          </div>

          <el-form-item>
            <el-checkbox v-model="form.peer_ratio_remove_data">直接删除种子及数据</el-checkbox>
            <div class="field-tip">
              开启则超标种子连文件一起删；关闭只暂停，可以在「已暂停种子」页手动恢复
            </div>
          </el-form-item>
        </div>
      </el-form>

      <template #footer>
        <span class="pt-foot-note">与自动删种共用同一个保存按钮的配置表</span>
        <el-button type="primary" :loading="saving" @click="save">
          <PtIcon name="save" :size="14" /><span>保存设置</span>
        </el-button>
      </template>
    </PtPanel>

    <PtPanel title="清理工作目录" icon="folder" count=".pt-tools" padding="none">
      <div class="settings-body settings-body--lead">
        <div class="field-tip field-tip--lead">
          清掉日志、暂存种子和旧配置备份。数据库、密钥等受保护项永远不会被删。
        </div>

        <el-form label-position="top" class="pt-form">
          <el-form-item label="清理项">
            <el-checkbox-group v-model="workdirCategories" class="cat-group">
              <el-checkbox
                v-for="opt in workdirCategoryOptions"
                :key="opt.value"
                :value="opt.value">
                {{ opt.label }}
                <span class="cat__desc">{{ opt.desc }}</span>
              </el-checkbox>
            </el-checkbox-group>
          </el-form-item>

          <el-form-item label="保留最近备份数量">
            <el-input-number v-model="keepBackups" :min="0" :max="100" :step="1" />
            <div class="field-tip">清旧备份时留最近的 N 份，其余删除；0 表示全清</div>
          </el-form-item>
        </el-form>

        <div class="clean-acts">
          <el-button :loading="previewing" @click="previewClean">
            <PtIcon name="eye" :size="14" /><span>预览可清理项</span>
          </el-button>
          <el-button type="danger" :loading="cleaning" @click="executeClean">
            <PtIcon name="trash-2" :size="14" /><span>立即清理</span>
          </el-button>
        </div>
      </div>

      <template v-if="cleanPreview">
        <div class="pt-strip">
          <PtIcon name="eye" :size="13" />
          <span>预览结果</span>
          <span class="pt-strip__end">
            可删 {{ cleanPreview.totalDeleted }} 项 · 可释放 {{ cleanPreview.totalFreedHuman }}
          </span>
        </div>
        <el-table :data="cleanPreview.categories" class="pt-grid" style="width: 100%">
          <el-table-column label="清理项" min-width="120" class-name="pt-cell-strong">
            <template #default="{ row }">{{ categoryLabel(row.name) }}</template>
          </el-table-column>
          <el-table-column
            prop="dirUsedHuman"
            label="当前已用"
            width="110"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column
            prop="deletedCount"
            label="可删除"
            width="90"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column
            prop="freedHuman"
            label="可释放"
            width="110"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column label="备注" min-width="160" class-name="pt-cell-muted">
            <template #default="{ row }">
              <PtStatusPill v-if="row.note" tone="warn" size="sm">{{ row.note }}</PtStatusPill>
              <span v-else-if="row.skippedCount > 0">
                跳过 {{ row.skippedCount }} 项（受保护）
              </span>
              <span v-else>—</span>
            </template>
          </el-table-column>
        </el-table>
        <div class="settings-body settings-body--tail">
          <div class="field-tip">这里只是预览，还没删任何文件；点「立即清理」并确认后才真删。</div>
        </div>
      </template>

      <template v-if="cleanResult">
        <div class="pt-strip">
          <PtIcon name="circle-check" :size="13" />
          <span>清理结果</span>
          <span class="pt-strip__end">
            已删 {{ cleanResult.totalDeleted }} 项 · 释放 {{ cleanResult.totalFreedHuman }}
          </span>
        </div>
        <el-table :data="cleanResult.categories" class="pt-grid" style="width: 100%">
          <el-table-column label="清理项" min-width="120" class-name="pt-cell-strong">
            <template #default="{ row }">{{ categoryLabel(row.name) }}</template>
          </el-table-column>
          <el-table-column
            prop="dirUsedHuman"
            label="当前已用"
            width="110"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column
            prop="deletedCount"
            label="已删除"
            width="90"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column
            prop="freedHuman"
            label="释放空间"
            width="110"
            class-name="pt-cell-num"
            label-class-name="pt-cell-num" />
          <el-table-column label="备注" min-width="160" class-name="pt-cell-muted">
            <template #default="{ row }">
              <PtStatusPill v-if="row.note" tone="warn" size="sm">{{ row.note }}</PtStatusPill>
              <span v-else-if="row.skippedCount > 0">
                跳过 {{ row.skippedCount }} 项（受保护）
              </span>
              <span v-else>—</span>
            </template>
          </el-table-column>
        </el-table>
      </template>
    </PtPanel>
  </div>
</template>

<style scoped>
.cleanup-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
  max-width: 880px;
}

/* 第一条区块条紧贴面板页头，两条发丝线会叠成 2px */
.settings-form > .pt-strip:first-child {
  border-top: 0;
}

.settings-body {
  padding: var(--pt-pad) var(--pt-pad) 0;
}

/* 面板正文直接以字段开头（没有区块条）时，下沿也要自己收口 */
.settings-body--lead {
  padding-bottom: var(--pt-space-2);
}

.settings-body--tail {
  padding: var(--pt-space-3) var(--pt-pad);
}

.field-tip--lead {
  margin-top: 0;
  margin-bottom: var(--pt-space-4);
}

.field-tip--ok {
  color: var(--pt-ok);
}

/*
 * 管理范围的三选一：每个选项是一整块可点区域，说明和风险提示都跟在选项里。
 * 选中项描边走 primary —— 「按标签」和「所有种子」的风险差得远，
 * 光看一枚小圆点读不出当前选的是哪种范围。
 */
.scope-list {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  width: 100%;
}

.scope {
  display: block;
  padding: var(--pt-space-3);
  background: var(--pt-canvas);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.scope.is-on {
  background: var(--pt-p-soft);
  border-color: var(--pt-p);
}

.scope__desc {
  margin: 2px 0 0 22px;
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-body);
  color: var(--pt-t3);
}

.scope__note {
  margin-top: var(--pt-space-2);
}

/* 标签胶囊和输入框同一行排布，输入框窄一点，不抢走整行 */
.tagbox {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  width: 100%;
}

.taginput {
  width: 150px;
}

.slow-note {
  margin-bottom: var(--pt-space-4);
}

/* 下拉项里的补充说明：主标签之后跟一段更淡的小字 */
.opt {
  display: flex;
  gap: var(--pt-space-2);
  align-items: baseline;
}

.opt__desc {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.cat-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.cat__desc {
  margin-left: 6px;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.clean-acts {
  display: flex;
  gap: var(--pt-space-2);
  padding-bottom: var(--pt-space-2);
}
</style>
