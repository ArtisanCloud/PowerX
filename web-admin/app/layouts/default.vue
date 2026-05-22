<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useWindowSize } from "../composables/useWindowSize";
import Sidebar from "../components/layout/Sidebar.vue";
import Header from "../components/layout/Header.vue";
import FooterBar from "../components/layout/FooterBar.vue";
import WelcomeGuide from "~/components/onboarding/WelcomeGuide.vue";
import GuideButton from "~/components/ui/GuideButton.vue";

// 获取当前路由
const route = useRoute();

// 判断是否应该隐藏 FooterBar
const shouldHideFooter = computed(() => {
  const path = route.path;
  return path.startsWith("/agent") || path.startsWith("/workflow") || path.startsWith("/_p/");
});

// 侧边栏折叠状态 - 使用与 Sidebar 组件相同的状态
const sidebarCollapsed = useState<boolean>("sidebar-collapsed", () => false);

// 切换侧边栏状态
const toggleSidebar = () => {
  sidebarCollapsed.value = !sidebarCollapsed.value;
};

// 响应式处理
const { width } = useWindowSize();
const isMobile = computed(() => width.value < 768);

// 移动端自动折叠侧边栏
watch(isMobile, (mobile) => {
  if (mobile) {
    sidebarCollapsed.value = true;
  }
});

// 移动端遮罩层
const showMobileOverlay = computed(
  () => isMobile.value && !sidebarCollapsed.value
);

// 点击遮罩层关闭侧边栏
const closeMobileSidebar = () => {
  if (isMobile.value) {
    sidebarCollapsed.value = true;
  }
};
</script>

<template>
  <div
    class="h-screen bg-gradient-to-br from-gray-50 via-gray-50 to-gray-100 dark:from-gray-900 dark:via-gray-900 dark:to-gray-800 flex overflow-hidden"
  >
    <!-- 移动端遮罩层 -->
    <div
      v-if="showMobileOverlay"
      class="fixed inset-0 bg-black/60 dark:bg-black/80 backdrop-blur-sm z-40 md:hidden"
      @click="closeMobileSidebar"
    />

    <!-- 侧边栏 -->
    <div
      class="fixed md:relative z-50 h-full transition-transform duration-300 ease-in-out"
      :class="[
        sidebarCollapsed
          ? '-translate-x-full md:translate-x-0'
          : 'translate-x-0',
        sidebarCollapsed ? 'md:w-16' : 'md:w-64',
      ]"
    >
      <div class="h-full">
        <Sidebar :collapsed="sidebarCollapsed" />
      </div>
    </div>

    <!-- 主内容区域 -->
    <div
      class="flex-1 flex flex-col min-w-0 transition-all duration-200"
      :class="[
        'md:ml-0', // 在桌面端不需要额外的 margin，因为侧边栏是相对定位的
      ]"
    >
      <!-- 顶部导航 -->
      <div class="sticky top-0 z-30">
        <Header @toggle-sidebar="toggleSidebar" />
      </div>

      <!-- 页面内容 -->
      <main class="flex-1 overflow-y-auto">
        <div class="p-6">
          <!-- 主要内容插槽 -->
          <div
            class="bg-white/95 dark:bg-gray-900/95 backdrop-blur-sm rounded-xl shadow-lg border border-gray-200/60 dark:border-gray-700/60 overflow-hidden"
          >
            <slot />
          </div>
        </div>
      </main>

      <!-- 页脚 -->
      <FooterBar v-if="!shouldHideFooter" />
    </div>

    <!-- 欢迎引导组件 -->
    <WelcomeGuide />

    <!-- 测试按钮 -->
    <GuideButton />
  </div>
</template>

<style scoped>
/* 确保布局在不同屏幕尺寸下正常工作 */
@media (max-width: 767px) {
  .sidebar-collapsed {
    transform: translateX(-100%);
  }
}
</style>
