<script setup lang="ts">
import FooterBar from "~/components/layout/FooterBar.vue";
import { useAuth } from "~/composables/useAuth";
import { LOGO_M_URL } from "~/utils/assets";

definePageMeta({
  alias: ["/"], // 让 /home 这个页面同时匹配 "/"
  layout: false, // 禁用layout
});

const { t } = useI18n();

// 主题色到科技绿的渐变配置
const primaryToTechGreen = [
  "from-blue-600 via-teal-500 to-emerald-400",
  "from-indigo-600 via-cyan-500 to-green-400",
  "from-purple-600 via-blue-500 to-teal-400",
  "from-violet-600 via-indigo-500 to-cyan-400",
  "from-blue-500 via-emerald-500 to-green-400",
  "from-cyan-600 via-teal-500 to-lime-400",
];

const darkPrimaryToTechGreen = [
  "from-blue-800 via-teal-700 to-emerald-600",
  "from-indigo-800 via-cyan-700 to-green-600",
  "from-purple-800 via-blue-700 to-teal-600",
  "from-violet-800 via-indigo-700 to-cyan-600",
  "from-blue-700 via-emerald-700 to-green-600",
  "from-cyan-800 via-teal-700 to-lime-600",
];

// 动态渐变状态
const currentGradient = ref<string>(
  "from-blue-600 via-teal-500 to-emerald-400"
);
const currentDarkGradient = ref<string>(
  "from-blue-800 via-teal-700 to-emerald-600"
);
const particlesVisible = ref(false);
const floatingElements = ref<any[]>([]);

// 生成浮动元素
const generateFloatingElements = () => {
  const elements = [];
  for (let i = 0; i < 15; i++) {
    elements.push({
      id: i,
      x: Math.random() * 100,
      y: Math.random() * 100,
      size: Math.random() * 20 + 10,
      delay: Math.random() * 5,
      duration: Math.random() * 10 + 15,
      opacity: Math.random() * 0.3 + 0.1,
    });
  }
  return elements;
};

// 用户状态管理 - 使用与Header组件相同的逻辑
const userStore = useUserStore();
const canAccessSettings = computed(
  () => Boolean(userStore.isRoot || userStore.isCurrentTenantAdmin)
);

// 计算属性：判断是否已登录
const isLoggedIn = computed(() => !!userStore.user);

// 用户信息计算属性
const userName = computed(() => {
  if (!userStore.user) return "";
  return userStore.displayName || userStore.user.email || "用户";
});

const userInitials = computed(() => {
  const name = userName.value;
  if (!name) return "U";
  const names = name.split(" ");
  if (names.length >= 2 && names[0]?.[0] && names[1]?.[0]) {
    return (names[0][0] + names[1][0]).toUpperCase();
  }
  return name.substring(0, 2).toUpperCase();
});

// 用户菜单状态
const showUserMenu = ref(false);
const userMenuRef = ref<HTMLElement | null>(null);

const toggleUserMenu = () => {
  showUserMenu.value = !showUserMenu.value;
};

const handleClickOutside = (event: Event) => {
  if (userMenuRef.value && !userMenuRef.value.contains(event.target as Node)) {
    showUserMenu.value = false;
  }
};

// 退出登录 - 使用统一的认证退出方法
const handleLogout = async () => {
  try {
    showUserMenu.value = false;
    // 使用统一的认证退出方法
    const { logout } = useAuth();
    await logout();
  } catch (error) {
    console.error("退出登录失败:", error);
  }
};

// 使用统一的认证工具方法
const { getToken, isTokenExpired } = useAuth();

const hasValidToken = () => !!getToken();

// 在客户端初始化主题和动效
onMounted(async () => {
  if (process.client) {
    // 随机选择渐变
    const randomIndex = Math.floor(Math.random() * primaryToTechGreen.length);
    currentGradient.value =
      primaryToTechGreen[randomIndex] ||
      "from-blue-600 via-teal-500 to-emerald-400";
    currentDarkGradient.value =
      darkPrimaryToTechGreen[randomIndex] ||
      "from-blue-800 via-teal-700 to-emerald-600";

    // 生成浮动元素
    floatingElements.value = generateFloatingElements();

    // 延迟显示粒子效果
    setTimeout(() => {
      particlesVisible.value = true;
    }, 500);

    // 点击外部关闭用户菜单
    document.addEventListener("click", handleClickOutside);

    // ⚠️ 只有有有效 token 才去取"需要登录"的数据
    if (hasValidToken()) {
      try {
        await userStore.fetchUserContext();
      } catch (error) {
        console.error("初始化用户数据失败:", error);
      }
    } else {
      // 匿名态：不要调用会 401 的接口
      // 可选：清一次"旧状态"
      if (userStore.clearUserState) {
        userStore.clearUserState();
      }
    }
  }
});

// 清理事件监听器
onUnmounted(() => {
  if (process.client) {
    document.removeEventListener("click", handleClickOutside);
  }
});
</script>

<template>
  <div class="min-h-screen relative flex flex-col overflow-hidden">
    <!-- 主题色到科技绿的动态渐变背景 -->
    <div
      class="absolute inset-0 bg-gradient-to-br from-blue-600 via-teal-500 to-emerald-400 dark:from-blue-800 dark:via-teal-700 dark:to-emerald-600 transition-all duration-1000"
    ></div>
    <div
      class="absolute inset-0 bg-gradient-to-tl from-emerald-400 via-cyan-500 to-blue-600 dark:from-emerald-600 dark:via-cyan-700 dark:to-blue-800 opacity-60 dark:opacity-70 transition-all duration-1000"
    ></div>

    <!-- 科技网格背景 -->
    <div class="absolute inset-0 opacity-10 dark:opacity-15">
      <div
        class="absolute inset-0 animate-grid-move"
        style="
          background-image:
            radial-gradient(
              circle at 1px 1px,
              rgba(16, 185, 129, 0.4) 1px,
              transparent 0
            ),
            linear-gradient(
              90deg,
              rgba(16, 185, 129, 0.1) 1px,
              transparent 1px
            ),
            linear-gradient(rgba(16, 185, 129, 0.1) 1px, transparent 1px);
          background-size:
            50px 50px,
            50px 50px,
            50px 50px;
        "
      ></div>
    </div>

    <!-- 浮动科技元素 -->
    <div v-if="particlesVisible" class="absolute inset-0 pointer-events-none">
      <div
        v-for="element in floatingElements"
        :key="element.id"
        class="absolute rounded-full bg-gradient-to-r from-emerald-400/20 to-teal-400/20 animate-float"
        :style="{
          left: element.x + '%',
          top: element.y + '%',
          width: element.size + 'px',
          height: element.size + 'px',
          animationDelay: element.delay + 's',
          animationDuration: element.duration + 's',
          opacity: element.opacity,
        }"
      ></div>
    </div>

    <!-- 动态光效 - 科技绿主题 -->
    <div
      class="absolute top-0 left-1/4 w-96 h-96 bg-gradient-to-r from-emerald-500/10 to-teal-500/10 dark:from-emerald-400/20 dark:to-teal-400/20 rounded-full blur-3xl animate-pulse-slow"
    ></div>
    <div
      class="absolute bottom-0 right-1/4 w-80 h-80 bg-gradient-to-r from-teal-500/10 to-cyan-500/10 dark:from-teal-400/20 dark:to-cyan-400/20 rounded-full blur-3xl animate-pulse-slow delay-1000"
    ></div>
    <div
      class="absolute top-1/2 right-0 w-64 h-64 bg-gradient-to-r from-cyan-500/10 to-green-500/10 dark:from-cyan-400/20 dark:to-green-400/20 rounded-full blur-3xl animate-pulse-slow delay-2000"
    ></div>

    <!-- 科技线条动效 -->
    <div class="absolute inset-0 opacity-20 dark:opacity-30">
      <svg
        class="w-full h-full"
        viewBox="0 0 1000 1000"
        preserveAspectRatio="none"
      >
        <defs>
          <linearGradient id="techLine" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" style="stop-color: #10b981; stop-opacity: 0" />
            <stop offset="50%" style="stop-color: #10b981; stop-opacity: 1" />
            <stop offset="100%" style="stop-color: #06d6a0; stop-opacity: 0" />
          </linearGradient>
        </defs>
        <path
          d="M0,200 Q250,100 500,200 T1000,200"
          stroke="url(#techLine)"
          stroke-width="2"
          fill="none"
          class="animate-draw-line"
        />
        <path
          d="M0,600 Q250,500 500,600 T1000,600"
          stroke="url(#techLine)"
          stroke-width="2"
          fill="none"
          class="animate-draw-line delay-1000"
        />
        <path
          d="M200,0 Q300,250 200,500 T200,1000"
          stroke="url(#techLine)"
          stroke-width="2"
          fill="none"
          class="animate-draw-line delay-2000"
        />
      </svg>
    </div>

    <!-- 主要内容区域 -->
    <div
      class="relative z-10 bg-white/10 dark:bg-gray-900/20 backdrop-blur-md min-h-screen flex flex-col"
    >
      <!-- 顶部Logo区域 -->
      <div class="flex justify-between items-center p-6 animate-fade-in">
        <!-- Logo -->
        <div class="flex items-center space-x-3">
          <img :src="LOGO_M_URL" alt="PowerX Logo" class="w-10 h-10" />
          <h1
            class="text-3xl font-bold bg-gradient-to-r from-blue-600 to-purple-600 bg-clip-text text-transparent"
          >
            PowerX
          </h1>
        </div>

        <!-- 右侧区域：语言切换器、登录注册按钮或用户信息 -->
        <div class="flex items-center space-x-4">
          <!-- 未登录状态：显示登录注册按钮 -->
          <template v-if="!isLoggedIn">
            <NuxtLink
              :to="$localePath('/users/login')"
              class="px-4 py-2 text-gray-700 dark:text-gray-300 hover:text-blue-600 transition-colors"
            >
              {{ $t("login") }}
            </NuxtLink>
            <NuxtLink
              :to="$localePath('/users/register')"
              class="px-6 py-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white rounded-lg transition-all"
            >
              {{ $t("register") }}
            </NuxtLink>
          </template>

          <!-- 已登录状态：显示用户信息和下拉菜单 -->
          <template v-else>
            <div class="relative" ref="userMenuRef">
              <button
                @click="toggleUserMenu"
                class="flex items-center space-x-2 px-3 py-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
              >
                <div
                  class="w-8 h-8 bg-gradient-to-r from-blue-600 to-purple-600 rounded-full flex items-center justify-center text-white text-sm font-medium"
                >
                  {{ userInitials }}
                </div>
                <span class="text-gray-700 dark:text-gray-300">{{
                  userName
                }}</span>
                <Icon
                  name="heroicons:chevron-down"
                  class="w-4 h-4 text-gray-500 transition-transform"
                  :class="{ 'rotate-180': showUserMenu }"
                />
              </button>

              <!-- 用户下拉菜单 -->
              <div
                v-show="showUserMenu"
                class="absolute right-0 mt-2 w-48 bg-white dark:bg-gray-800 rounded-lg shadow-lg border border-gray-200 dark:border-gray-700 py-1 z-50"
              >
                <NuxtLink
                  :to="$localePath('/profile')"
                  class="flex items-center px-4 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                  @click="showUserMenu = false"
                >
                  <Icon name="heroicons:user" class="w-4 h-4 mr-3" />
                  {{ $t("header.profile") }}
                </NuxtLink>
                <NuxtLink
                  v-if="canAccessSettings"
                  :to="$localePath('/settings')"
                  class="flex items-center px-4 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                  @click="showUserMenu = false"
                >
                  <Icon name="heroicons:cog-6-tooth" class="w-4 h-4 mr-3" />
                  {{ $t("header.settings") }}
                </NuxtLink>
                <hr class="my-1 border-gray-200 dark:border-gray-700" />
                <button
                  @click="handleLogout"
                  class="flex items-center w-full px-4 py-2 text-sm text-red-600 dark:text-red-400 hover:bg-gray-100 dark:hover:bg-gray-700"
                >
                  <Icon
                    name="heroicons:arrow-right-on-rectangle"
                    class="w-4 h-4 mr-3"
                  />
                  {{ $t("header.logout") }}
                </button>
              </div>
            </div>
          </template>
        </div>
      </div>

      <!-- 主要内容区域 - 全新科技感设计 -->
      <div class="flex-1 flex items-center justify-center px-6 py-12">
        <div class="max-w-7xl mx-auto w-full">
          <!-- 中央科技面板 -->
          <div class="relative">
            <!-- 主控制台界面 -->
            <div
              class="bg-white/20 dark:bg-slate-900/80 backdrop-blur-2xl rounded-3xl border border-white/20 dark:border-white/10 shadow-2xl shadow-black/10 dark:shadow-black/20 p-8 md:p-12 relative overflow-hidden transition-all duration-500"
            >
              <!-- 高级光效边框 -->
              <div class="absolute inset-0 rounded-3xl">
                <div
                  class="absolute top-0 left-0 w-24 h-24 border-t border-l border-gray-400/30 dark:border-white/20 rounded-tl-3xl animate-pulse transition-colors duration-500"
                ></div>
                <div
                  class="absolute top-0 right-0 w-24 h-24 border-t border-r border-gray-400/30 dark:border-white/20 rounded-tr-3xl animate-pulse delay-500 transition-colors duration-500"
                ></div>
                <div
                  class="absolute bottom-0 left-0 w-24 h-24 border-b border-l border-gray-400/30 dark:border-white/20 rounded-bl-3xl animate-pulse delay-1000 transition-colors duration-500"
                ></div>
                <div
                  class="absolute bottom-0 right-0 w-24 h-24 border-b border-r border-gray-400/30 dark:border-white/20 rounded-br-3xl animate-pulse delay-1500 transition-colors duration-500"
                ></div>
              </div>

              <!-- 内部光晕效果 -->
              <div
                class="absolute inset-0 bg-gradient-to-br from-blue-500/5 via-transparent to-emerald-500/5 dark:from-blue-500/10 dark:via-transparent dark:to-emerald-500/10 rounded-3xl transition-all duration-500"
              ></div>

              <!-- 顶部状态栏 -->
              <div class="flex justify-between items-center mb-8 relative z-10">
                <div class="flex items-center space-x-4">
                  <div
                    class="w-2 h-2 bg-emerald-400 rounded-full animate-pulse shadow-sm shadow-emerald-400/50"
                  ></div>
                  <div
                    class="w-2 h-2 bg-blue-400 rounded-full animate-pulse delay-300 shadow-sm shadow-blue-400/50"
                  ></div>
                  <div
                    class="w-2 h-2 bg-purple-400 rounded-full animate-pulse delay-600 shadow-sm shadow-purple-400/50"
                  ></div>
                  <span
                    class="text-gray-700 dark:text-white/80 font-mono text-sm tracking-wider transition-colors duration-500"
                    >SYSTEM ONLINE</span
                  >
                </div>
                <div
                  class="text-gray-600 dark:text-white/60 font-mono text-sm transition-colors duration-500"
                >
                  {{ new Date().toLocaleTimeString() }}
                </div>
              </div>

              <!-- 主要内容网格 -->
              <div class="grid grid-cols-1 lg:grid-cols-2 gap-12 items-center">
                <!-- 左侧：终端风格信息 -->
                <div class="space-y-6 animate-slide-in-left">
                  <!-- 终端标题 -->
                  <div
                    class="bg-white/30 dark:bg-slate-800/50 rounded-xl p-6 border border-gray-300/30 dark:border-white/10 font-mono backdrop-blur-sm transition-all duration-500"
                  >
                    <div
                      class="text-emerald-600 dark:text-emerald-400 text-sm mb-3 tracking-wider transition-colors duration-500"
                    >
                      $ system --status
                    </div>
                    <div
                      class="text-gray-800 dark:text-white text-2xl md:text-4xl font-bold transition-colors duration-500"
                    >
                      {{ $t("welcome") }}
                      <span
                        class="text-transparent bg-clip-text bg-gradient-to-r from-blue-400 via-purple-400 to-emerald-400 animate-gradient-x"
                      >
                        PowerX
                      </span>
                    </div>
                  </div>

                  <!-- 系统信息面板 -->
                  <div
                    class="bg-white/30 dark:bg-slate-800/50 rounded-xl p-6 border border-gray-300/30 dark:border-white/10 backdrop-blur-sm transition-all duration-500"
                  >
                    <div
                      class="text-blue-600 dark:text-blue-400 font-mono text-sm mb-3 tracking-wider transition-colors duration-500"
                    >
                      SYSTEM_INFO:
                    </div>
                    <p
                      class="text-gray-700 dark:text-white/80 text-lg leading-relaxed transition-colors duration-500"
                    >
                      {{ $t("home.subtitle") }}
                    </p>
                  </div>

                  <!-- 控制按钮 -->
                  <div class="flex flex-col sm:flex-row gap-4">
                    <NuxtLink
                      :to="$localePath('/agent')"
                      class="group relative px-8 py-4 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white font-semibold rounded-xl transition-all duration-300 text-center overflow-hidden shadow-lg shadow-blue-500/25"
                    >
                      <span
                        class="relative z-10 flex items-center justify-center"
                      >
                        <span class="mr-2">▶</span>
                        {{ $t("home.getStarted") }}
                      </span>
                      <div
                        class="absolute inset-0 bg-gradient-to-r from-purple-600 to-blue-600 opacity-0 group-hover:opacity-100 transition-opacity duration-300"
                      ></div>
                    </NuxtLink>
                    <NuxtLink
                      :to="$localePath('/home/intro')"
                      class="group px-8 py-4 border border-emerald-400/30 text-emerald-400 hover:border-emerald-400/60 hover:bg-emerald-400/10 font-semibold rounded-xl transition-all duration-300 text-center backdrop-blur-sm"
                    >
                      <span class="flex items-center justify-center">
                        <span class="mr-2">ℹ</span>
                        {{ $t("home.learnMore") }}
                      </span>
                    </NuxtLink>
                  </div>
                </div>

                <!-- 右侧：黑客科技可视化 -->
                <div class="relative animate-slide-in-right">
                  <!-- 主显示屏 -->
                  <div
                    class="bg-black/80 dark:bg-black/90 rounded-2xl border border-green-400/30 p-6 relative overflow-hidden backdrop-blur-sm transition-all duration-500 shadow-2xl shadow-green-500/20"
                  >
                    <!-- 黑客扫描线 -->
                    <div
                      class="absolute inset-0 bg-gradient-to-b from-transparent via-green-400/20 to-transparent animate-scan-line transition-all duration-500"
                    ></div>

                    <!-- 终端头部 -->
                    <div
                      class="flex items-center justify-between mb-4 relative z-10"
                    >
                      <div class="flex items-center space-x-2">
                        <div class="w-3 h-3 bg-red-500 rounded-full"></div>
                        <div class="w-3 h-3 bg-yellow-500 rounded-full"></div>
                        <div class="w-3 h-3 bg-green-500 rounded-full"></div>
                      </div>
                      <div class="text-green-400 font-mono text-sm">
                        root@powerx:~$
                      </div>
                    </div>

                    <!-- 中央黑客机器人全息投影 -->
                    <div class="h-80 flex items-center justify-center relative">
                      <!-- 全息投影基座 -->
                      <div class="relative w-56 h-56">
                        <!-- 外层全息环 -->
                        <div
                          class="absolute inset-0 border-2 border-green-400/40 rounded-full animate-spin-slow shadow-lg shadow-green-400/30"
                        ></div>
                        <div
                          class="absolute inset-4 border border-cyan-400/30 rounded-full animate-spin-reverse shadow-md shadow-cyan-400/20"
                        ></div>
                        <div
                          class="absolute inset-8 border border-blue-400/20 rounded-full animate-spin-slow delay-1000 shadow-sm shadow-blue-400/10"
                        ></div>

                        <!-- 中央AI智能体核心 -->
                        <div
                          class="absolute inset-0 flex items-center justify-center"
                        >
                          <div class="relative">
                            <!-- 现代AI神经网络核心 -->
                            <div class="w-40 h-40 relative">
                              <!-- 主要神经网络节点 -->
                              <div
                                class="absolute inset-0 flex items-center justify-center"
                              >
                                <!-- 中央核心节点 -->
                                <div class="relative">
                                  <!-- 核心处理单元 -->
                                  <div class="w-40 h-40 relative">
                                    <!-- 中央Logo -->
                                    <div
                                      class="absolute inset-0 flex items-center justify-center"
                                    >
                                      <img
                                        :src="LOGO_M_URL"
                                        alt="PowerX Logo"
                                        class="w-40 h-40 drop-shadow-2xl filter brightness-110 hover:brightness-125 transition-all duration-300 animate-float-gentle"
                                      />
                                    </div>
                                  </div>
                                </div>
                              </div>
                            </div>

                            <!-- AI状态信息浮窗 -->
                            <div
                              class="absolute -top-12 -left-16 text-xs text-green-400 font-mono animate-float-data"
                            >
                              <div
                                class="bg-black/90 border border-green-400/40 px-3 py-1 rounded-lg shadow-lg shadow-green-400/20"
                              >
                                🧠 THINKING...
                              </div>
                            </div>
                            <div
                              class="absolute -top-8 -right-20 text-xs text-cyan-400 font-mono animate-float-data delay-1000"
                            >
                              <div
                                class="bg-black/90 border border-cyan-400/40 px-3 py-1 rounded-lg shadow-lg shadow-cyan-400/20"
                              >
                                ⚡ PROCESSING
                              </div>
                            </div>
                            <div
                              class="absolute -bottom-12 left-4 text-xs text-blue-400 font-mono animate-float-data delay-2000"
                            >
                              <div
                                class="bg-black/90 border border-blue-400/40 px-3 py-1 rounded-lg shadow-lg shadow-blue-400/20"
                              >
                                🤖 AGENT_READY
                              </div>
                            </div>
                            <div
                              class="absolute -bottom-8 -right-16 text-xs text-purple-400 font-mono animate-float-data delay-3000"
                            >
                              <div
                                class="bg-black/90 border border-purple-400/40 px-3 py-1 rounded-lg shadow-lg shadow-purple-400/20"
                              >
                                🎯 LEARNING
                              </div>
                            </div>

                            <!-- 智能体决策树可视化 -->
                            <div class="absolute -inset-16 pointer-events-none">
                              <!-- 决策分支线 -->
                              <svg class="w-full h-full" viewBox="0 0 200 200">
                                <defs>
                                  <linearGradient
                                    id="branchGradient"
                                    x1="0%"
                                    y1="0%"
                                    x2="100%"
                                    y2="100%"
                                  >
                                    <stop
                                      offset="0%"
                                      style="
                                        stop-color: #10b981;
                                        stop-opacity: 0.6;
                                      "
                                    />
                                    <stop
                                      offset="50%"
                                      style="
                                        stop-color: #06d6a0;
                                        stop-opacity: 0.8;
                                      "
                                    />
                                    <stop
                                      offset="100%"
                                      style="
                                        stop-color: #0891b2;
                                        stop-opacity: 0.6;
                                      "
                                    />
                                  </linearGradient>
                                </defs>
                                <!-- 决策分支 -->
                                <path
                                  d="M100,100 L60,60 M100,100 L140,60 M100,100 L60,140 M100,100 L140,140"
                                  stroke="url(#branchGradient)"
                                  stroke-width="2"
                                  fill="none"
                                  class="animate-draw-line opacity-60"
                                />
                                <path
                                  d="M100,100 L80,40 M100,100 L120,40 M100,100 L40,80 M100,100 L160,80 M100,100 L80,160 M100,100 L120,160"
                                  stroke="url(#branchGradient)"
                                  stroke-width="1"
                                  fill="none"
                                  class="animate-draw-line delay-500 opacity-40"
                                />
                              </svg>
                            </div>
                          </div>
                        </div>

                        <!-- 环绕数据粒子 -->
                        <div class="absolute -inset-12">
                          <div
                            class="absolute top-0 left-1/2 w-2 h-2 bg-green-400 rounded-full animate-orbit shadow-lg shadow-green-400/50"
                          ></div>
                          <div
                            class="absolute top-1/2 right-0 w-2 h-2 bg-red-400 rounded-full animate-orbit-reverse shadow-lg shadow-red-400/50"
                          ></div>
                          <div
                            class="absolute bottom-0 left-1/2 w-2 h-2 bg-cyan-400 rounded-full animate-orbit delay-1000 shadow-lg shadow-cyan-400/50"
                          ></div>
                          <div
                            class="absolute top-1/2 left-0 w-2 h-2 bg-blue-400 rounded-full animate-orbit-reverse delay-1500 shadow-lg shadow-blue-400/50"
                          ></div>
                          <div
                            class="absolute top-1/4 right-1/4 w-1 h-1 bg-yellow-400 rounded-full animate-orbit delay-2000 shadow-sm shadow-yellow-400/50"
                          ></div>
                          <div
                            class="absolute bottom-1/4 left-1/4 w-1 h-1 bg-purple-400 rounded-full animate-orbit-reverse delay-2500 shadow-sm shadow-purple-400/50"
                          ></div>
                        </div>
                      </div>

                      <!-- 黑客数据流 -->
                      <div class="absolute inset-0">
                        <div
                          class="absolute top-1/4 left-0 w-full h-0.5 bg-gradient-to-r from-transparent via-green-400/80 to-transparent animate-data-flow"
                        ></div>
                        <div
                          class="absolute top-2/4 left-0 w-full h-0.5 bg-gradient-to-r from-transparent via-red-400/60 to-transparent animate-data-flow delay-500"
                        ></div>
                        <div
                          class="absolute top-3/4 left-0 w-full h-0.5 bg-gradient-to-r from-transparent via-cyan-400/60 to-transparent animate-data-flow delay-1000"
                        ></div>
                      </div>

                      <!-- 矩阵雨效果 -->
                      <div class="absolute inset-0 pointer-events-none">
                        <div
                          class="absolute top-0 left-8 text-green-400 font-mono text-xs animate-matrix-rain opacity-30"
                        >
                          1
                        </div>
                        <div
                          class="absolute top-0 left-16 text-green-400 font-mono text-xs animate-matrix-rain delay-300 opacity-20"
                        >
                          0
                        </div>
                        <div
                          class="absolute top-0 left-24 text-green-400 font-mono text-xs animate-matrix-rain delay-600 opacity-25"
                        >
                          1
                        </div>
                        <div
                          class="absolute top-0 right-8 text-green-400 font-mono text-xs animate-matrix-rain delay-900 opacity-30"
                        >
                          0
                        </div>
                        <div
                          class="absolute top-0 right-16 text-green-400 font-mono text-xs animate-matrix-rain delay-1200 opacity-20"
                        >
                          1
                        </div>
                      </div>
                    </div>

                    <!-- 底部黑客状态面板 -->
                    <div class="mt-6 grid grid-cols-3 gap-4 text-center">
                      <div
                        class="bg-green-500/10 rounded-lg p-3 border border-green-400/30 backdrop-blur-sm transition-all duration-500"
                      >
                        <div class="text-green-400 font-mono text-xs">HACK</div>
                        <div class="text-green-300 font-bold font-mono">
                          100%
                        </div>
                      </div>
                      <div
                        class="bg-red-500/10 rounded-lg p-3 border border-red-400/30 backdrop-blur-sm transition-all duration-500"
                      >
                        <div class="text-red-400 font-mono text-xs">BREACH</div>
                        <div class="text-red-300 font-bold font-mono">
                          READY
                        </div>
                      </div>
                      <div
                        class="bg-cyan-500/10 rounded-lg p-3 border border-cyan-400/30 backdrop-blur-sm transition-all duration-500"
                      >
                        <div class="text-cyan-400 font-mono text-xs">
                          STEALTH
                        </div>
                        <div class="text-cyan-300 font-bold font-mono">
                          ACTIVE
                        </div>
                      </div>
                    </div>
                  </div>

                  <!-- 外围黑客装饰 -->
                  <div
                    class="absolute -top-4 -right-4 w-8 h-8 border border-green-400/40 rounded-full animate-ping"
                  ></div>
                  <div
                    class="absolute -bottom-4 -left-4 w-6 h-6 border border-red-400/40 rounded-full animate-ping delay-1000"
                  ></div>
                  <div
                    class="absolute top-1/4 -right-6 w-4 h-4 border border-cyan-400/40 animate-ping delay-2000"
                  ></div>

                  <!-- 黑客连接线 -->
                  <div
                    class="absolute top-1/2 -right-8 w-16 h-0.5 bg-gradient-to-r from-green-400/50 to-transparent animate-pulse"
                  ></div>
                  <div
                    class="absolute top-1/3 -left-8 w-12 h-0.5 bg-gradient-to-l from-red-400/50 to-transparent animate-pulse delay-500"
                  ></div>
                  <div
                    class="absolute bottom-1/3 -right-6 w-10 h-0.5 bg-gradient-to-r from-cyan-400/50 to-transparent animate-pulse delay-1000"
                  ></div>
                </div>
              </div>
            </div>

            <!-- 外围科技装饰 -->
            <div class="absolute -inset-4 -z-10">
              <!-- 角落装饰 -->
              <div
                class="absolute top-0 left-0 w-32 h-32 border-l-2 border-t-2 border-cyan-400/30 rounded-tl-3xl"
              ></div>
              <div
                class="absolute top-0 right-0 w-32 h-32 border-r-2 border-t-2 border-emerald-400/30 rounded-tr-3xl"
              ></div>
              <div
                class="absolute bottom-0 left-0 w-32 h-32 border-l-2 border-b-2 border-teal-400/30 rounded-bl-3xl"
              ></div>
              <div
                class="absolute bottom-0 right-0 w-32 h-32 border-r-2 border-b-2 border-green-400/30 rounded-br-3xl"
              ></div>
            </div>
          </div>
        </div>
      </div>

      <!-- 底部简单信息 -->
      <FooterBar />
    </div>
  </div>
</template>

<style scoped>
/* 科技感动画效果 */
@keyframes gradient-x {
  0%,
  100% {
    background-size: 200% 200%;
    background-position: left center;
  }
  50% {
    background-size: 200% 200%;
    background-position: right center;
  }
}

@keyframes float {
  0%,
  100% {
    transform: translateY(0px) rotate(0deg);
  }
  33% {
    transform: translateY(-20px) rotate(120deg);
  }
  66% {
    transform: translateY(10px) rotate(240deg);
  }
}

@keyframes float-gentle {
  0%,
  100% {
    transform: translateY(0px);
  }
  50% {
    transform: translateY(-10px);
  }
}

@keyframes bounce-gentle {
  0%,
  100% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(-5px);
  }
}

@keyframes pulse-slow {
  0%,
  100% {
    opacity: 0.3;
    transform: scale(1);
  }
  50% {
    opacity: 0.6;
    transform: scale(1.05);
  }
}

@keyframes pulse-gentle {
  0%,
  100% {
    opacity: 0.4;
  }
  50% {
    opacity: 0.8;
  }
}

@keyframes grid-move {
  0% {
    transform: translate(0, 0);
  }
  100% {
    transform: translate(50px, 50px);
  }
}

@keyframes rotate-3d {
  0% {
    transform: rotateX(0deg) rotateY(0deg) rotateZ(0deg);
  }
  33% {
    transform: rotateX(15deg) rotateY(120deg) rotateZ(5deg);
  }
  66% {
    transform: rotateX(-10deg) rotateY(240deg) rotateZ(-5deg);
  }
  100% {
    transform: rotateX(0deg) rotateY(360deg) rotateZ(0deg);
  }
}

@keyframes orbit {
  0% {
    transform: rotate(0deg) translateX(60px) rotate(0deg);
  }
  100% {
    transform: rotate(360deg) translateX(60px) rotate(-360deg);
  }
}

@keyframes orbit-reverse {
  0% {
    transform: rotate(0deg) translateX(60px) rotate(0deg);
  }
  100% {
    transform: rotate(-360deg) translateX(60px) rotate(360deg);
  }
}

@keyframes data-flow {
  0% {
    transform: translateX(-100%);
    opacity: 0;
  }
  50% {
    opacity: 1;
  }
  100% {
    transform: translateX(100%);
    opacity: 0;
  }
}

@keyframes scan-line {
  0% {
    transform: translateY(-100%);
  }
  100% {
    transform: translateY(100vh);
  }
}

@keyframes spin-slow {
  0% {
    transform: rotate(0deg);
  }
  100% {
    transform: rotate(360deg);
  }
}

@keyframes spin-reverse {
  0% {
    transform: rotate(360deg);
  }
  100% {
    transform: rotate(0deg);
  }
}

@keyframes float-data {
  0%,
  100% {
    transform: translateY(0px);
    opacity: 0.8;
  }
  50% {
    transform: translateY(-10px);
    opacity: 1;
  }
}

@keyframes matrix-rain {
  0% {
    transform: translateY(-20px);
    opacity: 0;
  }
  50% {
    opacity: 1;
  }
  100% {
    transform: translateY(300px);
    opacity: 0;
  }
}

@keyframes neural-flow {
  0% {
    transform: scale(0.8);
    opacity: 0.3;
  }
  50% {
    transform: scale(1.2);
    opacity: 1;
  }
  100% {
    transform: scale(0.8);
    opacity: 0.3;
  }
}

@keyframes fade-in {
  from {
    opacity: 0;
    transform: translateY(-20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes slide-in-left {
  from {
    opacity: 0;
    transform: translateX(-50px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

@keyframes slide-in-right {
  from {
    opacity: 0;
    transform: translateX(50px);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}

/* 应用动画类 */
.animate-gradient-x {
  animation: gradient-x 3s ease infinite;
  background-size: 200% 200%;
}

.animate-float {
  animation: float 20s ease-in-out infinite;
}

.animate-float-gentle {
  animation: float-gentle 4s ease-in-out infinite;
}

.animate-bounce-gentle {
  animation: bounce-gentle 3s ease-in-out infinite;
}

.animate-pulse-slow {
  animation: pulse-slow 4s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

.animate-pulse-gentle {
  animation: pulse-gentle 3s cubic-bezier(0.4, 0, 0.6, 1) infinite;
}

.animate-grid-move {
  animation: grid-move 20s linear infinite;
}

.animate-fade-in {
  animation: fade-in 1s ease-out;
}

.animate-slide-in-left {
  animation: slide-in-left 1s ease-out;
}

.animate-slide-in-right {
  animation: slide-in-right 1s ease-out;
}

.animate-rotate-3d {
  animation: rotate-3d 20s linear infinite;
  transform-style: preserve-3d;
}

.animate-orbit {
  animation: orbit 8s linear infinite;
}

.animate-orbit-reverse {
  animation: orbit-reverse 10s linear infinite;
}

.animate-data-flow {
  animation: data-flow 3s ease-in-out infinite;
}

.animate-scan-line {
  animation: scan-line 4s linear infinite;
}

.animate-spin-slow {
  animation: spin-slow 8s linear infinite;
}

.animate-spin-reverse {
  animation: spin-reverse 6s linear infinite;
}

.animate-float-data {
  animation: float-data 3s ease-in-out infinite;
}

.animate-matrix-rain {
  animation: matrix-rain 4s linear infinite;
}

.animate-neural-flow {
  animation: neural-flow 3s ease-in-out infinite;
}

.clip-hexagon {
  clip-path: polygon(50% 0%, 100% 25%, 100% 75%, 50% 100%, 0% 75%, 0% 25%);
}

/* 延迟动画 */
.delay-100 {
  animation-delay: 0.1s;
}

.delay-1200 {
  animation-delay: 1.2s;
}

.delay-1500 {
  animation-delay: 1.5s;
}

.delay-1800 {
  animation-delay: 1.8s;
}

.delay-2100 {
  animation-delay: 2.1s;
}

.delay-3000 {
  animation-delay: 3s;
}

.delay-200 {
  animation-delay: 0.2s;
}

.delay-300 {
  animation-delay: 0.3s;
}

.delay-400 {
  animation-delay: 0.4s;
}

.delay-500 {
  animation-delay: 0.5s;
}

.delay-600 {
  animation-delay: 0.6s;
}

.delay-900 {
  animation-delay: 0.9s;
}

.delay-1200 {
  animation-delay: 1.2s;
}

.delay-1000 {
  animation-delay: 1s;
}

.delay-1500 {
  animation-delay: 1.5s;
}

.delay-2000 {
  animation-delay: 2s;
}

/* 悬停效果增强 */
.group:hover .group-hover\:opacity-20 {
  opacity: 0.2;
}

/* 响应式动画优化 */
@media (prefers-reduced-motion: reduce) {
  * {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}

/* 深色模式下的特殊效果 */
.dark .animate-gradient-x {
  filter: brightness(1.2);
}

/* 科技感光效 */
.tech-glow {
  box-shadow: 0 0 20px rgba(16, 185, 129, 0.3);
}

.dark .tech-glow {
  box-shadow: 0 0 30px rgba(16, 185, 129, 0.5);
}
</style>
