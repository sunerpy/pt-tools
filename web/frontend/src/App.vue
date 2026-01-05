<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useThemeStore } from './stores/theme'
import { useLogLevelStore } from './stores/logLevel'
import { controlApi } from './api'
import { ElMessage, ElMessageBox } from 'element-plus'

const route = useRoute()
const router = useRouter()
const themeStore = useThemeStore()
const logLevelStore = useLogLevelStore()

const isCollapse = ref(false)
const stopLoading = ref(false)
const startLoading = ref(false)

const activeMenu = computed(() => {
  const name = route.name as string
  if (name === 'site-detail') return 'sites'
  return name || 'global'
})

onMounted(() => {
  logLevelStore.fetchLogLevel()
})

// 监听主题变化，切换 Element Plus 暗色模式
watch(
  () => themeStore.isDark,
  isDark => {
    document.documentElement.classList.toggle('dark', isDark)
  },
  { immediate: true }
)

async function stopAll() {
  try {
    await ElMessageBox.confirm('确定要停止所有任务吗？', '确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    stopLoading.value = true
    await controlApi.stop()
    ElMessage.success('已停止所有任务')
  } catch (e: unknown) {
    if ((e as string) !== 'cancel') {
      ElMessage.error((e as Error).message || '操作失败')
    }
  } finally {
    stopLoading.value = false
  }
}

async function startAll() {
  try {
    await ElMessageBox.confirm('确定要启动所有任务吗？', '确认', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'info'
    })
    startLoading.value = true
    await controlApi.start()
    ElMessage.success('已启动所有任务')
  } catch (e: unknown) {
    if ((e as string) !== 'cancel') {
      ElMessage.error((e as Error).message || '操作失败')
    }
  } finally {
    startLoading.value = false
  }
}

function handleMenuSelect(index: string) {
  router.push(`/${index}`)
}

function logout() {
  window.location.href = '/logout'
}
</script>

<template>
  <el-container class="app-container">
    <!-- 侧边栏 -->
    <el-aside :width="isCollapse ? '64px' : '220px'" class="app-aside">
      <div class="logo">
        <el-icon :size="28"><Monitor /></el-icon>
        <span v-show="!isCollapse" class="logo-text">pt-tools</span>
      </div>

      <el-menu
        :default-active="activeMenu"
        :collapse="isCollapse"
        :collapse-transition="false"
        background-color="transparent"
        text-color="var(--el-text-color-primary)"
        active-text-color="var(--el-color-primary)"
        @select="handleMenuSelect"
      >
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
        <el-menu-item index="logs">
          <el-icon><Document /></el-icon>
          <template #title>日志</template>
        </el-menu-item>
        <el-menu-item index="password">
          <el-icon><Lock /></el-icon>
          <template #title>修改密码</template>
        </el-menu-item>
      </el-menu>

      <div class="aside-footer">
        <el-button :icon="isCollapse ? 'Expand' : 'Fold'" text @click="isCollapse = !isCollapse" />
      </div>
    </el-aside>

    <!-- 主内容区 -->
    <el-container>
      <!-- 顶部栏 -->
      <el-header class="app-header">
        <div class="header-left">
          <el-breadcrumb separator="/">
            <el-breadcrumb-item :to="{ path: '/' }">首页</el-breadcrumb-item>
            <el-breadcrumb-item>{{ route.meta?.title || route.name }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>

        <div class="header-right">
          <el-button-group>
            <el-button type="danger" :icon="'VideoPause'" :loading="stopLoading" @click="stopAll">
              停止任务
            </el-button>
            <el-button type="success" :icon="'VideoPlay'" :loading="startLoading" @click="startAll">
              启动任务
            </el-button>
          </el-button-group>

          <el-divider direction="vertical" />

          <el-switch
            :model-value="themeStore.isDark"
            :active-icon="'Moon'"
            :inactive-icon="'Sunny'"
            inline-prompt
            @change="themeStore.toggle"
          />

          <el-divider direction="vertical" />

          <el-select
            v-model="logLevelStore.currentLevel"
            :loading="logLevelStore.loading"
            size="default"
            style="width: 110px"
            @change="logLevelStore.setLogLevel"
          >
            <el-option
              v-for="level in logLevelStore.availableLevels"
              :key="level"
              :label="level"
              :value="level"
            />
          </el-select>

          <el-divider direction="vertical" />

          <el-dropdown>
            <el-button text>
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

      <!-- 内容区 -->
      <el-main class="app-main">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </el-main>

      <!-- 底部 -->
      <el-footer class="app-footer">
        <a href="https://github.com/sunerpy/pt-tools" target="_blank" rel="noopener">pt-tools</a>
        <span>© 2025 - PT 助手</span>
      </el-footer>
    </el-container>
  </el-container>
</template>

<style scoped>
.app-container {
  height: 100vh;
}

.app-aside {
  background: var(--el-bg-color);
  border-right: 1px solid var(--el-border-color-light);
  display: flex;
  flex-direction: column;
  transition: width 0.3s;
}

.logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-bottom: 1px solid var(--el-border-color-light);
  color: var(--el-color-primary);
}

.logo-text {
  font-size: 18px;
  font-weight: 600;
}

.el-menu {
  flex: 1;
  border-right: none;
}

.aside-footer {
  padding: 12px;
  text-align: center;
  border-top: 1px solid var(--el-border-color-light);
}

.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--el-border-color-light);
  background: var(--el-bg-color);
}

.header-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.app-main {
  background: var(--el-bg-color-page);
  padding: 20px;
}

.app-footer {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 16px;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  border-top: 1px solid var(--el-border-color-light);
  background: var(--el-bg-color);
}

.app-footer a {
  color: var(--el-color-primary);
  text-decoration: none;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
