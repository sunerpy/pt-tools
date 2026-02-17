<script setup lang="ts">
import { globalApi, type GlobalSettings } from "@/api";
import { Delete, Setting } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
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
</script>

<template>
  <div class="page-container global-settings-page">
    <div class="page-header">
      <div class="page-title-group">
        <h2 class="page-title">自动删种</h2>
        <p class="page-subtitle">配置自动种子清理策略，管理磁盘空间</p>
      </div>
      <div class="page-actions">
        <el-button type="primary" :loading="saving" @click="save">
          <el-icon><Setting /></el-icon>
          保存设置
        </el-button>
      </div>
    </div>

    <el-card v-loading="loading" shadow="never" class="common-card global-settings-card">
      <el-form :model="form" label-width="160px" label-position="top" class="settings-form">
        <div class="form-section settings-section">
          <div class="form-section-title">
            <el-icon><Delete /></el-icon>
            基本设置
          </div>

          <el-form-item label="启用自动删种">
            <el-switch v-model="form.cleanup_enabled" />
            <div class="form-tip">开启后将按照设定规则自动清理符合条件的种子</div>
          </el-form-item>

          <div v-show="form.cleanup_enabled">
            <el-row :gutter="40">
              <el-col :md="12" :sm="24">
                <el-form-item label="检查间隔(分钟)">
                  <el-input-number
                    v-model="form.cleanup_interval_min"
                    :min="5"
                    :max="1440"
                    :step="5"
                    class="w-full" />
                </el-form-item>
              </el-col>
              <el-col :md="12" :sm="24">
                <el-form-item label="删除时同时删除数据文件">
                  <el-switch v-model="form.cleanup_remove_data" />
                </el-form-item>
              </el-col>
            </el-row>

            <el-form-item label="管理范围">
              <el-radio-group v-model="form.cleanup_scope" class="scope-radio-group">
                <div class="scope-option">
                  <el-radio label="database">仅本应用推送的种子（推荐）</el-radio>
                  <div class="scope-desc">
                    根据数据库记录精确匹配，只管理 pt-tools 推送并记录在案的种子
                  </div>
                </div>
                <div class="scope-option">
                  <el-radio label="tag">按标签匹配</el-radio>
                  <div class="scope-desc">
                    <div v-if="form.cleanup_scope === 'tag'">
                      <div style="display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 8px">
                        <el-tag
                          v-for="(tag, i) in form.cleanup_scope_tags"
                          :key="tag"
                          closable
                          @close="removeTag(form.cleanup_scope_tags, i)"
                          >{{ tag }}</el-tag
                        >
                      </div>
                      <el-input
                        v-model="scopeTagInput"
                        placeholder="输入标签后回车添加"
                        @keyup.enter="addScopeTag()"
                        style="width: 240px" />
                    </div>
                    <el-alert
                      v-if="form.cleanup_scope === 'tag'"
                      title="警告：手动添加相同标签的种子也会被管理"
                      type="warning"
                      :closable="false"
                      show-icon
                      class="mt-2" />
                  </div>
                </div>
                <div class="scope-option">
                  <el-radio label="all">下载器中所有种子</el-radio>
                  <div class="scope-desc">
                    <el-alert
                      v-if="form.cleanup_scope === 'all'"
                      title="危险操作：包括非本应用添加的种子，可能导致误删重要数据"
                      type="error"
                      :closable="false"
                      show-icon />
                  </div>
                </div>
              </el-radio-group>
            </el-form-item>
          </div>
        </div>

        <div v-show="form.cleanup_enabled" class="form-section settings-section">
          <div class="form-section-title">删除条件</div>

          <el-form-item label="推荐方案">
            <el-select
              v-model="selectedPreset"
              placeholder="选择预设方案一键填充"
              @change="applyPreset"
              style="width: 300px"
              clearable>
              <el-option v-for="p in presets" :key="p.value" :label="p.label" :value="p.value">
                <div>
                  <span>{{ p.label }}</span>
                  <span style="color: var(--pt-text-tertiary); font-size: 12px; margin-left: 8px">{{
                    p.desc
                  }}</span>
                </div>
              </el-option>
            </el-select>
            <div v-if="selectedPreset" class="form-tip" style="color: var(--el-color-success)">
              已选择「{{
                presets.find((p) => p.value === selectedPreset)?.label
              }}」，修改下方参数后预设标记将自动清除
            </div>
            <div v-else class="form-tip">选择后仅填充下方表单，需点击保存才会生效</div>
          </el-form-item>

          <el-form-item label="条件关系">
            <el-radio-group v-model="form.cleanup_condition_mode">
              <el-radio label="or">满足任一条件即删除 (OR)</el-radio>
              <el-radio label="and">满足所有条件才删除 (AND)</el-radio>
            </el-radio-group>
            <div class="form-tip" v-if="form.cleanup_condition_mode === 'or'">
              以下任意一个条件满足即可删除种子（包括低效做种）
            </div>
            <div class="form-tip" v-else>
              以下所有已配置的条件都必须同时满足才会删除（0 或未启用的条件不参与判断）
            </div>
          </el-form-item>

          <el-row :gutter="40">
            <el-col :md="8" :sm="24">
              <el-form-item label="做种时间超过(小时)">
                <el-input-number
                  v-model="form.cleanup_max_seed_time_h"
                  :min="0"
                  :step="1"
                  class="w-full" />
                <div class="form-tip">0 表示不限制</div>
              </el-form-item>
            </el-col>
            <el-col :md="8" :sm="24">
              <el-form-item label="分享率达到">
                <el-input-number
                  v-model="form.cleanup_min_ratio"
                  :min="0"
                  :step="0.1"
                  :precision="2"
                  class="w-full" />
                <div class="form-tip">0 表示不限制</div>
              </el-form-item>
            </el-col>
            <el-col :md="8" :sm="24">
              <el-form-item label="不活跃超过(小时)">
                <el-input-number
                  v-model="form.cleanup_max_inactive_h"
                  :min="0"
                  :step="1"
                  class="w-full" />
                <div class="form-tip">0 表示不限制</div>
              </el-form-item>
            </el-col>
          </el-row>

          <el-row :gutter="40">
            <el-col :md="12" :sm="24">
              <el-form-item label="低效做种: 做种超过(小时)">
                <el-input-number
                  v-model="form.cleanup_slow_seed_time_h"
                  :min="0"
                  :step="1"
                  class="w-full" />
              </el-form-item>
            </el-col>
            <el-col :md="12" :sm="24">
              <el-form-item label="但分享率仍低于">
                <el-input-number
                  v-model="form.cleanup_slow_max_ratio"
                  :min="0"
                  :step="0.01"
                  :precision="2"
                  class="w-full" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-alert
            v-if="form.cleanup_slow_seed_time_h > 0 && form.cleanup_slow_max_ratio > 0"
            :title="`做种超过 ${form.cleanup_slow_seed_time_h} 小时但分享率低于 ${form.cleanup_slow_max_ratio} 的已完成种子将被删除`"
            type="info"
            :closable="false"
            show-icon
            style="margin-bottom: 16px" />
          <div v-else class="form-tip" style="margin-bottom: 16px">
            低效做种：两个值都需要设置才会生效，任一为 0 则不启用此条件
          </div>

          <el-form-item>
            <el-checkbox
              v-model="form.cleanup_del_free_expired"
              label="免费到期未完成的种子自动删除"
              border />
            <div class="form-tip">
              仅对本应用推送并记录了免费结束时间的种子有效，与管理范围设置无关
            </div>
          </el-form-item>
        </div>

        <div v-show="form.cleanup_enabled" class="form-section settings-section">
          <div class="form-section-title">磁盘空间保护</div>
          <el-form-item>
            <el-checkbox v-model="form.cleanup_disk_protect" label="启用磁盘空间保护" border />
          </el-form-item>
          <el-form-item v-if="form.cleanup_disk_protect" label="最低剩余空间(GB)">
            <el-input-number
              v-model="form.cleanup_min_disk_space_gb"
              :min="1"
              :max="10000"
              class="w-full" />
            <div class="form-tip">
              低于此值时：RSS 自动下载将暂停推送新种子，同时按优先级强制删除种子以释放空间
            </div>
          </el-form-item>
        </div>

        <div v-show="form.cleanup_enabled" class="form-section settings-section">
          <div class="form-section-title">保护规则</div>
          <div class="form-tip" style="margin-bottom: 16px">
            以下保护条件相互独立，种子命中任一条即受保护，不会被删除
          </div>

          <el-form-item>
            <el-checkbox v-model="form.cleanup_protect_dl" label="保护下载中的种子" border />
            <div class="form-tip">正在下载或校验中的种子不会被删除</div>
          </el-form-item>

          <el-form-item>
            <el-checkbox v-model="form.cleanup_protect_hr" label="保护 H&R 种子" border />
            <div class="form-tip">
              未满足站点 H&R 做种时长要求的种子不会被删除，无 H&R 站点不受影响
            </div>
          </el-form-item>

          <el-row :gutter="40">
            <el-col :md="12" :sm="24">
              <el-form-item label="最短保种时间(小时)">
                <el-input-number v-model="form.cleanup_min_retain_h" :min="0" class="w-full" />
                <div class="form-tip">添加时间未达到此时长的种子不会被删除</div>
              </el-form-item>
            </el-col>
            <el-col :md="12" :sm="24">
              <el-form-item label="保护标签">
                <div style="display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 8px">
                  <el-tag
                    v-for="(tag, i) in form.cleanup_protect_tags"
                    :key="tag"
                    closable
                    @close="removeTag(form.cleanup_protect_tags, i)"
                    >{{ tag }}</el-tag
                  >
                </div>
                <el-input
                  v-model="protectTagInput"
                  placeholder="输入标签后回车添加"
                  @keyup.enter="addProtectTag()"
                  style="width: 240px" />
                <div class="form-tip">带有这些标签的种子不会被删除</div>
              </el-form-item>
            </el-col>
          </el-row>
        </div>
      </el-form>

      <div class="form-actions">
        <el-button type="primary" :loading="saving" @click="save" size="large">保存设置</el-button>
      </div>
    </el-card>
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";
@import "@/styles/form-page.css";
@import "@/styles/global-settings-page.css";
</style>
