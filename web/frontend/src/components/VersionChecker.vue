<script setup lang="ts">
import { useVersionStore } from "@/stores/version";
import { formatDate } from "@/utils/format";
import { Check, Close, Download, Link, Promotion, Refresh } from "@element-plus/icons-vue";
import DOMPurify from "dompurify";
import { marked } from "marked";
import { storeToRefs } from "pinia";
import { onMounted, ref } from "vue";

const versionStore = useVersionStore();
const {
  currentVersion,
  hasUpdate,
  latestVersion,
  allDismissed,
  visibleReleases,
  hasMoreReleases,
  changelogUrl,
  checking,
  checkResult,
  upgradeProgress,
  upgrading,
  canSelfUpgrade,
  isDocker,
} = storeToRefs(versionStore);

const proxyUrl = ref("");
const showProxyInput = ref(false);

function openReleases() {
  open("https://github.com/sunerpy/pt-tools/releases", "_blank");
}

marked.use({
  renderer: {
    link({ href, title, text }) {
      return `<a href="${href}" title="${
        title || ""
      }" target="_blank" rel="noopener noreferrer">${text}</a>`;
    },
  },
});

onMounted(() => {
  if (!versionStore.versionInfo) {
    versionStore.fetchVersionInfo();
  }
  versionStore.fetchRuntime();
});

function handleCheckUpdate() {
  versionStore.checkForUpdates({ force: true, proxy: proxyUrl.value || undefined });
}

function handleDismiss(version: string) {
  versionStore.dismissVersion(version);
}

function handleUpgrade(version: string) {
  versionStore.startUpgrade(version, proxyUrl.value || undefined);
}

function handleCancelUpgrade() {
  versionStore.cancelUpgrade();
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}

function renderMarkdown(text: string): string {
  if (!text) return "";
  try {
    const html = marked.parse(text, { async: false }) as string;
    return DOMPurify.sanitize(html);
  } catch (e) {
    console.error("Markdown parsing error:", e);
    return text;
  }
}
</script>

<template>
  <el-popover placement="bottom" :width="400" trigger="click" popper-class="version-popover">
    <template #reference>
      <el-button class="version-btn" :class="{ 'has-update': hasUpdate }" text>
        <div class="btn-content">
          <span class="version-icon-wrap">
            <el-icon :class="{ 'is-checking': checking }">
              <Promotion v-if="!checking" />
              <Refresh v-else />
            </el-icon>
            <span v-if="hasUpdate" class="update-dot"></span>
          </span>
          <span class="version-meta">
            <span class="version-label">版本</span>
            <span class="version-text">{{ currentVersion }}</span>
          </span>
        </div>
      </el-button>
    </template>

    <div class="version-content">
      <div class="popover-header">
        <div class="title-wrap">
          <span class="title">版本信息</span>
          <span class="subtitle">当前版本与更新日志</span>
        </div>
        <div class="header-actions">
          <el-button size="small" text @click="openReleases" title="查看所有历史版本">
            版本历史
          </el-button>
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
            title="检查更新" />
        </div>
      </div>

      <div v-if="showProxyInput" class="proxy-section">
        <el-input v-model="proxyUrl" size="medium" placeholder="http://proxy:port" clearable />
      </div>

      <div class="status-container">
        <div v-if="isDocker" class="docker-info">
          <el-alert type="info" :closable="false" show-icon>
            <template #title>Docker 环境检测</template>
            <p>当前运行在 Docker 容器中。推荐使用以下方式更新：</p>
            <ul>
              <li>
                使用
                <a href="https://containrrr.dev/watchtower/" target="_blank">Watchtower</a>
                自动更新
              </li>
              <li>或手动拉取新镜像: <code>docker pull sunerpy/pt-tools:latest</code></li>
            </ul>
          </el-alert>
        </div>

        <div v-if="upgrading && upgradeProgress" class="upgrade-progress">
          <div class="progress-header">
            <span>正在升级到 {{ upgradeProgress.target_version }}</span>
            <el-button size="small" text type="danger" @click="handleCancelUpgrade">取消</el-button>
          </div>
          <el-progress
            :percentage="Math.round(upgradeProgress.progress)"
            :status="upgradeProgress.status === 'failed' ? 'exception' : undefined" />
          <div class="progress-status">
            <span v-if="upgradeProgress.status === 'downloading'">
              下载中... {{ formatBytes(upgradeProgress.bytes_downloaded) }} /
              {{ formatBytes(upgradeProgress.total_bytes) }}
            </span>
            <span v-else-if="upgradeProgress.status === 'extracting'">解压中...</span>
            <span v-else-if="upgradeProgress.status === 'replacing'">替换文件中...</span>
            <span v-else-if="upgradeProgress.status === 'completed'" class="success">
              升级完成，请重启应用
            </span>
            <span v-else-if="upgradeProgress.status === 'failed'" class="error">
              {{ upgradeProgress.error }}
            </span>
          </div>
        </div>

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
            <span class="version-pair">
              <span class="secondary-label">当前</span>
              <span class="secondary">{{ currentVersion }}</span>
            </span>
            <span class="version-pair">
              <span class="secondary-label">最新</span>
              <span class="secondary">{{ latestVersion }}</span>
            </span>
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
                <el-button
                  v-if="canSelfUpgrade"
                  type="primary"
                  size="small"
                  :icon="Download"
                  :disabled="upgrading"
                  @click="handleUpgrade(release.version)">
                  升级
                </el-button>
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
