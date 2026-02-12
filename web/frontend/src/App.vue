<script setup lang="ts">
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { controlApi } from "./api";
import VersionChecker from "./components/VersionChecker.vue";
import { useLogLevelStore } from "./stores/logLevel";
import { useThemeStore } from "./stores/theme";
import { useVersionStore } from "./stores/version";

const route = useRoute();
const router = useRouter();
const themeStore = useThemeStore();
const logLevelStore = useLogLevelStore();
const versionStore = useVersionStore();

const isCollapse = ref(false);
const stopLoading = ref(false);
const startLoading = ref(false);

const themeStyleOptions = [
  { label: "默认配色", value: "default" },
  { label: "海洋配色", value: "ocean" },
  { label: "石墨配色", value: "graphite" },
  { label: "极光配色", value: "contrast" },
];

const activeMenu = computed(() => {
  const name = route.name as string;
  if (name === "site-detail") return "sites";
  return name || "global";
});

onMounted(() => {
  logLevelStore.fetchLogLevel();
  versionStore.fetchVersionInfo();
  versionStore.checkForUpdates(undefined, true);
});

// 监听主题变化，切换 Element Plus 暗色模式
watch(
  () => themeStore.isDark,
  (isDark) => {
    document.documentElement.classList.toggle("dark", isDark);
  },
  { immediate: true },
);

async function stopAll() {
  try {
    await ElMessageBox.confirm("确定要停止所有任务吗？", "确认", {
      confirmButtonText: "确定",
      cancelButtonText: "取消",
      type: "warning",
    });
    stopLoading.value = true;
    await controlApi.stop();
    ElMessage.success("已停止所有任务");
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "操作失败");
    }
  } finally {
    stopLoading.value = false;
  }
}

async function startAll() {
  try {
    await ElMessageBox.confirm("确定要启动所有任务吗？", "确认", {
      confirmButtonText: "确定",
      cancelButtonText: "取消",
      type: "info",
    });
    startLoading.value = true;
    await controlApi.start();
    ElMessage.success("已启动所有任务");
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "操作失败");
    }
  } finally {
    startLoading.value = false;
  }
}

function handleMenuSelect(index: string) {
  router.push(`/${index}`);
}

function logout() {
  window.location.href = "/logout";
}
</script>

<template>
  <el-container class="app-container">
    <el-aside
      :width="isCollapse ? '68px' : '236px'"
      class="app-aside"
      :class="{ 'is-collapse': isCollapse }">
      <div class="app-aside-logo">
        <el-icon class="app-aside-logo-icon" :size="28"><Monitor /></el-icon>
        <span v-show="!isCollapse" class="app-aside-logo-text">pt-tools</span>
      </div>

      <div class="app-aside-menu">
        <el-menu
          :default-active="activeMenu"
          :collapse="isCollapse"
          :collapse-transition="false"
          background-color="transparent"
          text-color="var(--pt-text-primary)"
          active-text-color="var(--pt-color-primary)"
          @select="handleMenuSelect">
          <el-menu-item index="userinfo">
            <el-icon><DataAnalysis /></el-icon>
            <template #title>用户统计</template>
          </el-menu-item>
          <el-menu-item index="global">
            <el-icon><Setting /></el-icon>
            <template #title>全局设置</template>
          </el-menu-item>
          <el-menu-item index="downloaders">
            <el-icon><Download /></el-icon>
            <template #title>下载器管理</template>
          </el-menu-item>
          <el-menu-item index="sites">
            <el-icon><Connection /></el-icon>
            <template #title>站点与RSS</template>
          </el-menu-item>
          <el-menu-item index="search">
            <el-icon><Search /></el-icon>
            <template #title>种子搜索</template>
          </el-menu-item>
          <el-menu-item index="filter-rules">
            <el-icon><Filter /></el-icon>
            <template #title>过滤规则</template>
          </el-menu-item>
          <el-menu-item index="tasks">
            <el-icon><List /></el-icon>
            <template #title>任务列表</template>
          </el-menu-item>
          <el-menu-item index="paused">
            <el-icon><VideoPause /></el-icon>
            <template #title>暂停任务</template>
          </el-menu-item>
          <el-menu-item index="logs">
            <el-icon><Document /></el-icon>
            <template #title>日志</template>
          </el-menu-item>
          <el-menu-item index="password">
            <el-icon><Lock /></el-icon>
            <template #title>修改密码</template>
          </el-menu-item>
        </el-menu>
      </div>

      <div class="app-aside-footer">
        <el-button :icon="isCollapse ? 'Expand' : 'Fold'" text @click="isCollapse = !isCollapse" />
      </div>
    </el-aside>

    <el-container>
      <el-header class="app-header">
        <div class="app-header-left">
          <div class="app-header-title-group">
            <div class="app-header-title">{{ route.meta?.title || route.name }}</div>
            <el-breadcrumb separator="/">
              <el-breadcrumb-item :to="{ path: '/' }">首页</el-breadcrumb-item>
              <el-breadcrumb-item>{{ route.meta?.title || route.name }}</el-breadcrumb-item>
            </el-breadcrumb>
          </div>
        </div>

        <div class="app-header-right">
          <el-button-group class="app-header-actions">
            <el-button type="danger" :icon="'VideoPause'" :loading="stopLoading" @click="stopAll">
              停止任务
            </el-button>
            <el-button type="success" :icon="'VideoPlay'" :loading="startLoading" @click="startAll">
              启动任务
            </el-button>
          </el-button-group>

          <span class="app-header-divider"></span>

          <VersionChecker />

          <span class="app-header-divider"></span>

          <el-switch
            class="app-header-theme-switch"
            :model-value="themeStore.isDark"
            :active-icon="'Moon'"
            :inactive-icon="'Sunny'"
            inline-prompt
            @change="themeStore.toggle" />

          <el-select
            class="app-header-style"
            :model-value="themeStore.themeStyle"
            size="default"
            style="width: 118px"
            @change="themeStore.setThemeStyle">
            <el-option
              v-for="option in themeStyleOptions"
              :key="option.value"
              :label="option.label"
              :value="option.value" />
          </el-select>

          <span class="app-header-divider"></span>

          <el-select
            class="app-header-loglevel"
            v-model="logLevelStore.currentLevel"
            :loading="logLevelStore.loading"
            size="default"
            style="width: 110px"
            @change="logLevelStore.setLogLevel">
            <el-option
              v-for="level in logLevelStore.availableLevels"
              :key="level"
              :label="level"
              :value="level" />
          </el-select>

          <span class="app-header-divider"></span>

          <el-dropdown class="app-header-user">
            <el-button text class="app-header-user-btn">
              <el-icon><User /></el-icon>
              <span style="margin-left: 4px">admin</span>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item @click="logout">
                  <el-icon><SwitchButton /></el-icon>
                  退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

      <el-main class="app-main">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </el-main>

      <el-footer class="app-footer">
        <a
          class="app-footer-link"
          href="https://github.com/sunerpy/pt-tools"
          target="_blank"
          rel="noopener">
          pt-tools
        </a>
        <span>© 2025 - PT 助手</span>
      </el-footer>
    </el-container>
  </el-container>
</template>
