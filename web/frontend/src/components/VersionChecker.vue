<script setup lang="ts">
import { useVersionStore } from "@/stores/version"
import { formatDate } from "@/utils/format"
import { Check, Close, Link, Promotion, Refresh } from "@element-plus/icons-vue"
import DOMPurify from "dompurify"
import { marked } from "marked"
import { storeToRefs } from "pinia"
import { onMounted, ref } from "vue"

const versionStore = useVersionStore()
const {
  currentVersion,
  hasUpdate,
  latestVersion,
  allDismissed,
  visibleReleases,
  hasMoreReleases,
  changelogUrl,
  checking,
  checkResult
} = storeToRefs(versionStore)

const proxyUrl = ref("")
const showProxyInput = ref(false)

marked.use({
  renderer: {
    link({ href, title, text }) {
      return `<a href="${href}" title="${
        title || ""
      }" target="_blank" rel="noopener noreferrer">${text}</a>`
    }
  }
})

onMounted(() => {
  if (!versionStore.versionInfo) {
    versionStore.fetchVersionInfo()
  }
})

function handleCheckUpdate() {
  versionStore.checkForUpdates({ force: true, proxy: proxyUrl.value || undefined })
}

function handleDismiss(version: string) {
  versionStore.dismissVersion(version)
}

function renderMarkdown(text: string): string {
  if (!text) return ""
  try {
    const html = marked.parse(text, { async: false }) as string
    return DOMPurify.sanitize(html)
  } catch (e) {
    console.error("Markdown parsing error:", e)
    return text
  }
}
</script>

<template>
  <el-popover placement="bottom" :width="400" trigger="click" popper-class="version-popover">
    <template #reference>
      <el-button class="version-btn" :class="{ 'has-update': hasUpdate }" text>
        <div class="btn-content">
          <el-icon :class="{ 'is-checking': checking }">
            <Promotion v-if="!checking" />
            <Refresh v-else />
          </el-icon>
          <span class="version-text">{{ currentVersion }}</span>
          <span v-if="hasUpdate" class="update-dot"></span>
        </div>
      </el-button>
    </template>

    <div class="version-content">
      <div class="popover-header">
        <span class="title">版本信息</span>
        <div class="header-actions">
          <el-button
            size="small"
            text
            @click="showProxyInput = !showProxyInput"
            title="使用代理获取最新版本">
            代理设置
          </el-button>
          <el-button
            size="small"
            :loading="checking"
            :icon="Refresh"
            circle
            @click="handleCheckUpdate"
            title="检查更新"
          />
        </div>
      </div>

      <div v-if="showProxyInput" class="proxy-section">
        <el-input v-model="proxyUrl" size="medium" placeholder="http://proxy:port" clearable />
      </div>

      <div class="status-container">
        <div v-if="checking && !hasUpdate" class="status-state loading">
          <el-icon class="is-loading"><Refresh /></el-icon>
          <span>正在检查更新...</span>
        </div>

        <div v-else-if="!hasUpdate && !allDismissed && checkResult" class="status-state latest">
          <el-icon class="success-icon"><Check /></el-icon>
          <div class="text-group">
            <span class="primary">当前已是最新版本</span>
            <span class="secondary">版本 {{ currentVersion }}</span>
          </div>
        </div>

        <div v-else-if="allDismissed" class="status-state dismissed">
          <div class="text-group">
            <span class="primary">已忽略所有新版本</span>
            <span class="secondary">当前版本: {{ currentVersion }}</span>
            <span class="secondary">最新版本: {{ latestVersion }}</span>
          </div>
          <el-button size="small" text type="primary" @click="versionStore.clearDismissed()">
            重新显示
          </el-button>
        </div>

        <div v-else-if="hasUpdate" class="updates-list">
          <div class="update-header">
            <el-tag type="success" size="small" effect="dark">发现新版本</el-tag>
          </div>

          <div
            v-for="release in visibleReleases.slice(0, 3)"
            :key="release.version"
            class="release-item">
            <div class="release-info">
              <div class="release-title-row">
                <span class="version-tag">{{ release.version }}</span>
                <span class="release-date">{{ formatDate(release.published_at) }}</span>
              </div>
              <div v-if="release.name" class="release-name">{{ release.name }}</div>

              <div v-if="release.changelog" class="release-changelog-wrapper custom-scrollbar">
                <div class="markdown-body" v-html="renderMarkdown(release.changelog)"></div>
              </div>

              <div class="release-actions">
                <el-link
                  v-if="release.url"
                  :href="release.url"
                  target="_blank"
                  type="primary"
                  :underline="false"
                  class="action-link">
                  <el-icon><Link /></el-icon>
                  查看
                </el-link>
                <el-link
                  type="info"
                  :underline="false"
                  class="action-link"
                  @click="handleDismiss(release.version)">
                  <el-icon><Close /></el-icon>
                  忽略
                </el-link>
              </div>
            </div>
          </div>

          <div v-if="hasMoreReleases || changelogUrl" class="more-releases">
            <el-link :href="changelogUrl" target="_blank" type="primary">
              查看完整更新日志 <el-icon class="el-icon--right"><Link /></el-icon>
            </el-link>
          </div>
        </div>

        <div v-else class="status-state initial">
          <span class="secondary">点击刷新按钮检查更新</span>
        </div>
      </div>
    </div>
  </el-popover>
</template>

<style scoped>
.version-btn {
  height: 32px;
  padding: 0 8px;
  border-radius: 6px;
  transition: all 0.3s;
  color: var(--el-text-color-regular);
}

.version-btn:hover {
  background-color: var(--el-fill-color);
  color: var(--el-color-primary);
}

.btn-content {
  display: flex;
  align-items: center;
  gap: 6px;
  position: relative;
}

.version-text {
  font-family: var(--el-font-family-monospace);
  font-size: 13px;
}

.update-dot {
  position: absolute;
  top: -2px;
  right: -4px;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: var(--el-color-danger);
  border: 1px solid var(--el-bg-color);
}

.has-update .version-btn {
  color: var(--el-color-primary);
}

/* Popover Content */
.version-content {
  padding: 4px;
}

.popover-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--el-border-color-lighter);
}

.popover-header .title {
  font-weight: 600;
  font-size: 14px;
  color: var(--el-text-color-primary);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.proxy-section {
  margin-bottom: 12px;
}

.status-container {
  min-height: 60px;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.status-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 12px 0;
  text-align: center;
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.status-state.latest {
  flex-direction: row;
  gap: 12px;
}

.status-state.dismissed {
  gap: 12px;
}

.success-icon {
  font-size: 24px;
  color: var(--el-color-success);
}

.text-group {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
}

.text-group .primary {
  color: var(--el-text-color-primary);
  font-weight: 500;
}

.text-group .secondary {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

/* Updates List */
.updates-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.update-header {
  margin-bottom: 4px;
}

.release-item {
  background-color: var(--el-fill-color-light);
  border-radius: 8px;
  padding: 12px;
  border: 1px solid var(--el-border-color-lighter);
  transition: all 0.2s;
}

.release-item:hover {
  background-color: var(--el-fill-color);
  border-color: var(--el-border-color);
}

.release-info {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.release-title-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.version-tag {
  font-weight: 600;
  color: var(--el-color-primary);
  font-family: var(--el-font-family-monospace);
  font-size: 14px;
}

.release-date {
  font-size: 12px;
  color: var(--el-text-color-secondary);
}

.release-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--el-text-color-primary);
}

.release-changelog-wrapper {
  background-color: var(--el-bg-color);
  border-radius: 4px;
  padding: 8px 12px;
  max-height: 200px;
  overflow-y: auto;
  border: 1px solid var(--el-border-color-lighter);
}

/* Markdown Styles */
:deep(.markdown-body) {
  font-size: 13px;
  color: var(--el-text-color-regular);
  line-height: 1.5;
}

:deep(.markdown-body h1),
:deep(.markdown-body h2),
:deep(.markdown-body h3) {
  font-weight: 600;
  margin-top: 12px;
  margin-bottom: 8px;
  font-size: 14px;
  color: var(--el-text-color-primary);
}

:deep(.markdown-body h1:first-child),
:deep(.markdown-body h2:first-child),
:deep(.markdown-body h3:first-child) {
  margin-top: 0;
}

:deep(.markdown-body p) {
  margin-bottom: 8px;
}

:deep(.markdown-body ul),
:deep(.markdown-body ol) {
  padding-left: 20px;
  margin-bottom: 8px;
}

:deep(.markdown-body li) {
  margin-bottom: 4px;
}

:deep(.markdown-body code) {
  background-color: var(--el-fill-color-dark);
  padding: 2px 4px;
  border-radius: 4px;
  font-family: var(--el-font-family-monospace);
  font-size: 12px;
  color: var(--el-color-primary);
}

:deep(.markdown-body pre) {
  background-color: var(--el-fill-color-dark);
  padding: 8px;
  border-radius: 4px;
  overflow-x: auto;
  margin-bottom: 8px;
}

:deep(.markdown-body pre code) {
  background-color: transparent;
  padding: 0;
  color: var(--el-text-color-regular);
}

:deep(.markdown-body blockquote) {
  margin: 0 0 8px 0;
  padding-left: 12px;
  border-left: 3px solid var(--el-border-color);
  color: var(--el-text-color-secondary);
}

:deep(.markdown-body a) {
  color: var(--el-color-primary);
  text-decoration: none;
}

:deep(.markdown-body a:hover) {
  text-decoration: underline;
}

.release-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 4px;
  padding-top: 8px;
  border-top: 1px dashed var(--el-border-color-lighter);
}

.action-link {
  font-size: 12px;
  display: flex;
  align-items: center;
  gap: 2px;
}

.more-releases {
  margin-top: 8px;
  text-align: center;
  padding-top: 8px;
  border-top: 1px dashed var(--el-border-color-lighter);
}

/* Custom Scrollbar */
.custom-scrollbar::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background: var(--el-border-color);
  border-radius: 3px;
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: var(--el-text-color-secondary);
}

.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}

/* Animations */
.is-checking {
  animation: rotate 1.5s linear infinite;
}

@keyframes rotate {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
</style>
