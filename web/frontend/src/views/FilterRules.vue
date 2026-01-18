<script setup lang="ts">
import { type FilterRule, filterRulesApi, type FilterRuleTestResponse, type RSSConfig } from "@/api"
import { ElMessage, ElMessageBox } from "element-plus"
import { computed, onMounted, ref } from "vue"

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const showDialog = ref(false)
const showTestDialog = ref(false)
const editMode = ref(false)
const loadingRss = ref(false)

const rules = ref<FilterRule[]>([])
const rssList = ref<{ id: number; name: string; site_name: string }[]>([])
const testResult = ref<FilterRuleTestResponse | null>(null)
const selectedRssId = ref<number | undefined>(undefined)

const form = ref<FilterRule>({
  name: "",
  pattern: "",
  pattern_type: "keyword",
  match_field: "both",
  require_free: true,
  enabled: true,
  priority: 100
})

const patternTypes = [
  { value: "keyword", label: "关键词", tip: "大小写不敏感，匹配包含该关键词的标题" },
  { value: "wildcard", label: "通配符", tip: "使用 * 匹配任意字符，? 匹配单个字符" },
  { value: "regex", label: "正则表达式", tip: "使用正则表达式进行精确匹配" }
]

const matchFields = [
  { value: "title", label: "仅标题" },
  { value: "tag", label: "仅标签" },
  { value: "both", label: "标题和标签" }
]

const templates = [
  { name: "4K 资源", pattern: "4K|2160p|UHD", type: "regex" as const },
  { name: "1080p 资源", pattern: "1080p", type: "keyword" as const },
  { name: "REMUX 资源", pattern: "*REMUX*", type: "wildcard" as const },
  { name: "HDR 资源", pattern: "HDR|HDR10|Dolby Vision|DV", type: "regex" as const },
  { name: "HEVC/x265", pattern: "HEVC|x265|H.265", type: "regex" as const },
  { name: "国语配音", pattern: "国语|国配|中配", type: "regex" as const }
]

const currentPatternTip = computed(() => {
  return patternTypes.find(t => t.value === form.value.pattern_type)?.tip || ""
})

onMounted(async () => {
  await loadRules()
  await loadRssList()
})

async function loadRules() {
  loading.value = true
  try {
    rules.value = await filterRulesApi.list()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载失败")
  } finally {
    loading.value = false
  }
}

async function loadRssList() {
  loadingRss.value = true
  try {
    const response = await fetch("/api/sites")
    const sites = await response.json()
    const list: { id: number; name: string; site_name: string }[] = []
    for (const [siteName, siteConfig] of Object.entries(sites)) {
      const config = siteConfig as { rss?: RSSConfig[] }
      if (config.rss) {
        for (const rss of config.rss) {
          if (rss.id) {
            list.push({ id: rss.id, name: rss.name, site_name: siteName })
          }
        }
      }
    }
    rssList.value = list
  } catch (e: unknown) {
    console.error("加载 RSS 列表失败:", e)
  } finally {
    loadingRss.value = false
  }
}

function openAddDialog() {
  editMode.value = false
  form.value = {
    name: "",
    pattern: "",
    pattern_type: "keyword",
    match_field: "both",
    require_free: true,
    enabled: true,
    priority: 100
  }
  selectedRssId.value = undefined
  showDialog.value = true
}

function openEditDialog(rule: FilterRule) {
  editMode.value = true
  form.value = { ...rule }
  selectedRssId.value = undefined
  showDialog.value = true
}

function applyTemplate(tpl: (typeof templates)[0]) {
  form.value.pattern = tpl.pattern
  form.value.pattern_type = tpl.type
  if (!form.value.name) {
    form.value.name = tpl.name
  }
}

async function saveRule() {
  if (!form.value.name || !form.value.pattern) {
    ElMessage.error("名称和匹配模式为必填项")
    return
  }

  saving.value = true
  try {
    if (editMode.value && form.value.id) {
      await filterRulesApi.update(form.value.id, form.value)
      ElMessage.success("更新成功")
    } else {
      await filterRulesApi.create(form.value)
      ElMessage.success("创建成功")
    }
    showDialog.value = false
    await loadRules()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败")
  } finally {
    saving.value = false
  }
}

async function deleteRule(rule: FilterRule) {
  if (!rule.id) return

  try {
    await ElMessageBox.confirm(`确定删除过滤规则 "${rule.name}"？`, "确认删除", {
      confirmButtonText: "删除",
      cancelButtonText: "取消",
      type: "warning"
    })
    await filterRulesApi.delete(rule.id)
    ElMessage.success("已删除")
    await loadRules()
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "删除失败")
    }
  }
}

async function toggleEnabled(rule: FilterRule) {
  if (!rule.id) return
  const newEnabled = !rule.enabled
  try {
    await filterRulesApi.update(rule.id, { ...rule, enabled: newEnabled })
    rule.enabled = newEnabled
    ElMessage.success("已保存")
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败")
  }
}

async function testPattern() {
  if (!form.value.pattern) {
    ElMessage.error("请先输入匹配模式")
    return
  }

  testing.value = true
  testResult.value = null
  try {
    testResult.value = await filterRulesApi.test({
      pattern: form.value.pattern,
      pattern_type: form.value.pattern_type,
      match_field: form.value.match_field || "both",
      require_free: form.value.require_free,
      rss_id: selectedRssId.value,
      limit: 20
    })
    showTestDialog.value = true
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "测试失败")
  } finally {
    testing.value = false
  }
}

function getPatternTypeLabel(type: string) {
  return patternTypes.find(t => t.value === type)?.label || type
}

function getPatternTypeTag(type: string) {
  switch (type) {
    case "keyword":
      return "success"
    case "wildcard":
      return "warning"
    case "regex":
      return "danger"
    default:
      return "info"
  }
}

function getMatchFieldLabel(field: string | undefined) {
  return matchFields.find(f => f.value === field)?.label || "标题和标签"
}
</script>

<template>
  <div class="page-container">
    <el-card v-loading="loading" shadow="never">
      <template #header>
        <div class="card-header">
          <span>过滤规则管理</span>
          <el-button
            type="primary"
            :icon="'Plus'"
            @click="openAddDialog">添加规则</el-button>
        </div>
      </template>

      <el-alert type="info" :closable="false" style="margin-bottom: 16px">
        <template #title>
          过滤规则用于自动下载匹配的种子。规则按优先级排序，数字越小优先级越高。
        </template>
      </el-alert>

      <el-table :data="rules" style="width: 100%">
        <el-table-column type="index" label="序号" width="60" align="center" />

        <el-table-column label="名称" min-width="120">
          <template #default="{ row }">
            <span class="rule-name">{{ row.name }}</span>
          </template>
        </el-table-column>

        <el-table-column label="匹配模式" min-width="200">
          <template #default="{ row }">
            <code class="pattern-text">{{ row.pattern }}</code>
          </template>
        </el-table-column>

        <el-table-column label="类型" min-width="100" align="center">
          <template #default="{ row }">
            <el-tag
              :type="getPatternTypeTag(row.pattern_type)"
              size="small"
              effect="plain">
              {{ getPatternTypeLabel(row.pattern_type) }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="匹配范围" min-width="100" align="center">
          <template #default="{ row }">
            <span>{{ getMatchFieldLabel(row.match_field) }}</span>
          </template>
        </el-table-column>

        <el-table-column label="优先级" min-width="80" align="center">
          <template #default="{ row }">
            <span>{{ row.priority }}</span>
          </template>
        </el-table-column>

        <el-table-column label="仅免费" min-width="80" align="center">
          <template #default="{ row }">
            <el-tag :type="row.require_free ? 'success' : 'info'" size="small">
              {{ row.require_free ? "是" : "否" }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="操作" min-width="200" align="center">
          <template #default="{ row }">
            <el-space>
              <el-switch
                :model-value="row.enabled"
                size="small"
                @change="toggleEnabled(row)"
              />
              <el-button
                type="primary"
                size="small"
                @click="openEditDialog(row)">编辑</el-button>
              <el-button
                type="danger"
                size="small"
                @click="deleteRule(row)">删除</el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <el-empty v-if="rules.length === 0" description="暂无过滤规则，点击上方按钮添加" />
    </el-card>

    <!-- 添加/编辑对话框 -->
    <el-dialog
      v-model="showDialog"
      :title="editMode ? '编辑过滤规则' : '添加过滤规则'"
      width="600px">
      <el-form :model="form" label-width="100px" label-position="right">
        <el-form-item label="名称" required>
          <el-input v-model="form.name" placeholder="例如: 4K电影" />
        </el-form-item>

        <el-form-item label="模式类型" required>
          <el-select v-model="form.pattern_type" style="width: 100%">
            <el-option
              v-for="t in patternTypes"
              :key="t.value"
              :label="t.label"
              :value="t.value"
            />
          </el-select>
          <div class="form-tip">{{ currentPatternTip }}</div>
        </el-form-item>

        <el-form-item label="匹配模式" required>
          <el-input
            v-model="form.pattern"
            placeholder="输入匹配模式"
            :rows="2"
            type="textarea"
          />
        </el-form-item>

        <el-form-item label="匹配范围">
          <el-select v-model="form.match_field" style="width: 100%">
            <el-option
              v-for="f in matchFields"
              :key="f.value"
              :label="f.label"
              :value="f.value"
            />
          </el-select>
          <div class="form-tip">选择从标题、标签还是两者中进行匹配</div>
        </el-form-item>

        <el-form-item label="常用模板">
          <div class="template-list">
            <el-tag
              v-for="tpl in templates"
              :key="tpl.name"
              class="template-tag"
              effect="plain"
              @click="applyTemplate(tpl)">
              {{ tpl.name }}
            </el-tag>
          </div>
        </el-form-item>

        <el-form-item label="优先级">
          <el-input-number v-model="form.priority" :min="1" :max="9999" />
          <div class="form-tip">数字越小优先级越高，默认100</div>
        </el-form-item>

        <el-form-item label="仅免费">
          <el-switch v-model="form.require_free" />
          <div class="form-tip">开启后仅下载免费种子</div>
        </el-form-item>

        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>

        <el-form-item label="测试数据源">
          <el-select
            v-model="selectedRssId"
            placeholder="选择 RSS 订阅进行测试（可选）"
            clearable
            style="width: 100%"
            :loading="loadingRss">
            <el-option
              v-for="rss in rssList"
              :key="rss.id"
              :label="`${rss.name} (${rss.site_name})`"
              :value="rss.id"
            />
          </el-select>
          <div class="form-tip">
            选择 RSS 订阅从实时数据中测试，不选则从历史记录中测试
          </div>
        </el-form-item>

        <el-form-item>
          <el-button
            type="info"
            :loading="testing"
            @click="testPattern">测试匹配</el-button>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveRule">
          {{ editMode ? "保存" : "添加" }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 测试结果对话框 -->
    <el-dialog v-model="showTestDialog" title="匹配测试结果" width="800px">
      <div v-if="testResult" v-loading="testing">
        <div class="test-result-header">
          <el-alert
            :type="testResult.match_count > 0 ? 'success' : 'warning'"
            :closable="false">
            <template #title>
              <div style="font-size: 14px">
                共测试 {{ testResult.total_count }} 条记录，匹配到
                <span style="color: var(--el-color-primary); font-weight: bold">
                  {{ testResult.match_count }}
                </span>
                条
              </div>
            </template>
          </el-alert>

          <el-radio-group
            v-model="form.match_field"
            size="small"
            style="margin-top: 12px">
            <el-radio-button v-for="f in matchFields" :key="f.value" :label="f.value">
              {{ f.label }}
            </el-radio-button>
          </el-radio-group>
        </div>

        <div v-if="testResult.matches && testResult.matches.length > 0" class="match-list">
          <el-card
            v-for="(match, idx) in testResult.matches"
            :key="idx"
            class="match-item"
            shadow="hover">
            <template #header>
              <div class="match-header">
                <span class="match-index">#{{ idx + 1 }}</span>
                <el-tag size="small" type="success" effect="plain">匹配成功</el-tag>
                <el-tag
                  v-if="match.is_free"
                  size="small"
                  type="warning"
                  effect="plain">
                  免费
                </el-tag>
                <el-tag
                  v-else
                  size="small"
                  type="info"
                  effect="plain">非免费</el-tag>
              </div>
            </template>

            <div class="match-content">
              <div class="match-section">
                <div class="match-label">标题</div>
                <div class="match-title">{{ match.title }}</div>
              </div>

              <div v-if="match.tag" class="match-section">
                <div class="match-label">标签</div>
                <div class="match-tag-content">{{ match.tag }}</div>
              </div>
            </div>
          </el-card>
        </div>
        <el-empty v-else description="没有匹配的种子" />
      </div>

      <template #footer>
        <div class="dialog-footer">
          <el-button @click="showTestDialog = false">关闭</el-button>
          <el-button
            type="primary"
            :loading="testing"
            @click="testPattern">重新测试</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page-container {
  width: 100%;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 16px;
  font-weight: 600;
}

.rule-name {
  font-weight: 500;
}

.pattern-text {
  font-family: monospace;
  font-size: 13px;
  background: var(--el-fill-color-light);
  padding: 2px 6px;
  border-radius: 4px;
}

.form-tip {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-top: 4px;
}

.template-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.template-tag {
  cursor: pointer;
}

.template-tag:hover {
  background: var(--el-color-primary-light-9);
  border-color: var(--el-color-primary);
}

.test-result-header {
  margin-bottom: 16px;
  text-align: center;
}

.match-list {
  max-height: 600px;
  overflow-y: auto;
  padding: 0 4px;
}

.match-item {
  margin-bottom: 12px;
}

.match-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.match-index {
  font-size: 14px;
  color: var(--el-text-color-secondary);
}

.match-content {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.match-section {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}

.match-label {
  flex-shrink: 0;
  width: 40px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
}

.match-title {
  flex: 1;
  font-size: 13px;
  line-height: 1.5;
  word-break: break-all;
}

.match-tag-content {
  flex: 1;
  font-size: 13px;
  line-height: 1.5;
  color: var(--el-text-color-regular);
  word-break: break-all;
}

.dialog-footer {
  display: flex;
  justify-content: space-between;
}
</style>
