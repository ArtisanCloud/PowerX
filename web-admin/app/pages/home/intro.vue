<script setup lang="ts">
import FooterBar from "~/components/layout/FooterBar.vue";
import { useUserStore } from "~/stores/user";
import { useAuth } from "~/composables/useAuth";
import { LOGO_M_URL } from "~/utils/assets";

definePageMeta({
  layout: false, // 禁用layout
});

const { t } = useI18n();

// 用户状态管理
const userStore = useUserStore();
const { getToken } = useAuth();

// 用户信息计算属性
const userName = computed(() => {
  return userStore.displayName || userStore.user?.email || "用户";
});

const userInitials = computed(() => {
  const name = userName.value;
  const names = name.split(" ");
  if (names.length >= 2) {
    return ((names[0]?.[0] || "") + (names[1]?.[0] || "")).toUpperCase();
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

// 退出登录
const handleLogout = async () => {
  try {
    showUserMenu.value = false;
    const { logout } = useAuth();
    await logout();
  } catch (error) {
    console.error("退出登录失败:", error);
  }
};

// 主题色到科技绿的渐变配置（与首页保持一致）
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
  for (let i = 0; i < 12; i++) {
    elements.push({
      id: i,
      x: Math.random() * 100,
      y: Math.random() * 100,
      size: Math.random() * 15 + 8,
      opacity: Math.random() * 0.6 + 0.2,
      delay: Math.random() * 2,
      duration: Math.random() * 3 + 2,
    });
  }
  return elements;
};

// 产品特性数据
const features = computed(() => [
  {
    icon: "⚡",
    title: t("intro.features.performance.title"),
    description: t("intro.features.performance.description"),
  },
  {
    icon: "🎨",
    title: t("intro.features.design.title"),
    description: t("intro.features.design.description"),
  },
  {
    icon: "🔧",
    title: t("intro.features.usability.title"),
    description: t("intro.features.usability.description"),
  },
  {
    icon: "🛡️",
    title: t("intro.features.security.title"),
    description: t("intro.features.security.description"),
  },
]);

// 产品数据
const products = computed(() => [
  {
    name: "PowerX Admin",
    description: t("intro.products.admin.description"),
    image: "/api/placeholder/300/200",
    features: [
      t("intro.products.admin.features.userManagement"),
      t("intro.products.admin.features.permissionControl"),
      t("intro.products.admin.features.dataAnalysis"),
      t("intro.products.admin.features.systemMonitoring"),
    ],
  },
  {
    name: "PowerX Analytics",
    description: t("intro.products.analytics.description"),
    image: "/api/placeholder/300/200",
    features: [
      t("intro.products.analytics.features.realTimeAnalysis"),
      t("intro.products.analytics.features.visualReports"),
      t("intro.products.analytics.features.predictiveModels"),
      t("intro.products.analytics.features.customDashboard"),
    ],
  },
  {
    name: "PowerX Cloud",
    description: t("intro.products.cloud.description"),
    image: "/api/placeholder/300/200",
    features: [
      t("intro.products.cloud.features.cloudStorage"),
      t("intro.products.cloud.features.multiDeviceSync"),
      t("intro.products.cloud.features.elasticScaling"),
      t("intro.products.cloud.features.support247"),
    ],
  },
]);

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
    }, 300);

    // 只在有有效token时才获取用户数据
    if (!!getToken()) {
      try {
        await userStore.fetchUserContext();
      } catch (error) {
        console.error("初始化用户数据失败:", error);
      }
    } else {
      // 匿名状态：清理旧状态
      userStore.clearUserState?.();
    }

    // 添加点击外部关闭菜单的事件监听
    document.addEventListener("click", handleClickOutside);

  }
});

onUnmounted(() => {
  if (process.client) {
    document.removeEventListener("click", handleClickOutside);
  }
});
</script>

<template>
  <div
    class="min-h-screen bg-gradient-to-br"
    :class="[
      `bg-gradient-to-br ${currentGradient} dark:${currentDarkGradient}`,
      'transition-all duration-1000 ease-in-out',
    ]"
  >
    <!-- 背景网格 -->
    <div
      class="absolute inset-0 opacity-20 dark:opacity-10"
      style="
        background-image: radial-gradient(
          circle at 1px 1px,
          rgba(255, 255, 255, 0.3) 1px,
          transparent 0
        );
        background-size: 60px 60px;
      "
    ></div>

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

    <!-- 主要内容区域 -->
    <div
      class="relative z-10 bg-white/10 dark:bg-gray-900/20 backdrop-blur-md min-h-screen transition-all duration-500"
    >
      <!-- 导航栏 -->
      <nav
        class="bg-white/20 dark:bg-gray-900/30 backdrop-blur-md border-b border-gray-300/30 dark:border-white/10 sticky top-0 z-50 transition-all duration-500"
      >
        <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div class="flex justify-between items-center h-16">
            <!-- Logo -->
            <div class="flex items-center">
              <div class="flex-shrink-0">
                <NuxtLink :to="$localePath('/')">
                  <div class="flex items-center space-x-3">
                    <img
                      :src="LOGO_M_URL"
                      alt="PowerX Logo"
                      class="w-10 h-10"
                    />
                    <h1
                      class="text-2xl font-bold bg-gradient-to-r from-blue-600 to-purple-600 bg-clip-text text-transparent"
                    >
                      PowerX
                    </h1>
                  </div>
                </NuxtLink>
              </div>
            </div>

            <!-- 导航链接 -->
            <div class="hidden md:block">
              <div class="ml-10 flex items-baseline space-x-4">
                <a
                  href="#features"
                  class="text-gray-700 dark:text-gray-300 hover:text-emerald-600 dark:hover:text-emerald-400 px-3 py-2 rounded-md text-sm font-medium transition-colors duration-300"
                >
                  {{ $t("intro.nav.features") }}
                </a>
                <a
                  href="#products"
                  class="text-gray-700 dark:text-gray-300 hover:text-emerald-600 dark:hover:text-emerald-400 px-3 py-2 rounded-md text-sm font-medium transition-colors duration-300"
                >
                  {{ $t("intro.nav.products") }}
                </a>
                <a
                  href="#about"
                  class="text-gray-700 dark:text-gray-300 hover:text-emerald-600 dark:hover:text-emerald-400 px-3 py-2 rounded-md text-sm font-medium transition-colors duration-300"
                >
                  {{ $t("intro.nav.about") }}
                </a>
              </div>
            </div>

            <!-- 右侧区域：语言切换器、登录注册按钮或用户信息 -->
            <div class="flex items-center space-x-4">
              <!-- 未登录状态：显示登录注册按钮 -->
              <template v-if="!userStore.user">
                <NuxtLink
                  :to="$localePath('/users/login')"
                  class="text-gray-700 dark:text-gray-300 hover:text-emerald-600 dark:hover:text-emerald-400 px-4 py-2 transition-colors duration-300"
                >
                  {{ $t("login") }}
                </NuxtLink>
                <NuxtLink
                  :to="$localePath('/users/register')"
                  class="px-6 py-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-700 hover:to-purple-700 text-white rounded-lg transition-all shadow-lg shadow-blue-500/25"
                >
                  {{ $t("register") }}
                </NuxtLink>
              </template>

              <!-- 已登录状态：显示用户信息 -->
              <template v-else>
                <div class="relative" ref="userMenuRef">
                  <button
                    @click="toggleUserMenu"
                    class="flex items-center space-x-3 px-3 py-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
                  >
                    <!-- 用户头像 -->
                    <div
                      class="w-8 h-8 bg-gradient-to-r from-blue-500 to-purple-600 rounded-full flex items-center justify-center text-white text-sm font-medium"
                    >
                      {{ userInitials }}
                    </div>
                    <!-- 用户名 -->
                    <span class="text-gray-700 dark:text-gray-300 font-medium">
                      {{ userName }}
                    </span>
                    <!-- 下拉箭头 -->
                    <svg
                      class="w-4 h-4 text-gray-500 transition-transform"
                      :class="{ 'rotate-180': showUserMenu }"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M19 9l-7 7-7-7"
                      />
                    </svg>
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
                      <svg
                        class="w-4 h-4 mr-3"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
                        />
                      </svg>
                      {{ $t("header.profile") }}
                    </NuxtLink>
                    <NuxtLink
                      :to="$localePath('/settings')"
                      class="flex items-center px-4 py-2 text-sm text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700"
                      @click="showUserMenu = false"
                    >
                      <svg
                        class="w-4 h-4 mr-3"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
                        />
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                        />
                      </svg>
                      {{ $t("settings") }}
                    </NuxtLink>
                    <hr class="border-gray-200 dark:border-gray-700 my-1" />
                    <button
                      @click="handleLogout"
                      class="flex items-center w-full px-4 py-2 text-sm text-red-600 dark:text-red-400 hover:bg-gray-100 dark:hover:bg-gray-700"
                    >
                      <svg
                        class="w-4 h-4 mr-3"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"
                        />
                      </svg>
                      {{ $t("header.logout") }}
                    </button>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>
      </nav>

      <!-- 英雄区域 -->
      <section class="relative py-20 px-4 sm:px-6 lg:px-8">
        <div class="max-w-4xl mx-auto text-center">
          <h1
            class="text-4xl md:text-6xl font-bold text-white mb-6 animate-fade-in"
          >
            {{ $t("intro.hero.welcomeTo") }}
            <span
              class="bg-gradient-to-r from-emerald-400 to-teal-300 bg-clip-text text-transparent"
            >
              PowerX
            </span>
          </h1>
          <p
            class="text-xl text-blue-100 dark:text-emerald-100 mb-8 max-w-2xl mx-auto transition-colors duration-500 animate-fade-in delay-200"
          >
            {{ $t("intro.hero.dashboardPreview") }}
          </p>
        </div>
      </section>

      <!-- 特性展示 -->
      <section id="features" class="py-20 px-4 sm:px-6 lg:px-8">
        <div class="max-w-6xl mx-auto">
          <div class="text-center mb-16">
            <h2
              class="text-3xl md:text-4xl font-bold text-white mb-4 animate-fade-in"
            >
              {{ $t("intro.features.title") }}
            </h2>
            <p
              class="text-xl text-blue-100 dark:text-emerald-100 max-w-2xl mx-auto transition-colors duration-500 animate-fade-in delay-200"
            >
              {{ $t("intro.features.subtitle") }}
            </p>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-8">
            <div
              v-for="(feature, index) in features"
              :key="index"
              class="bg-white/10 dark:bg-gray-900/20 backdrop-blur-md rounded-xl p-6 border border-white/20 dark:border-gray-700/30 hover:bg-white/20 dark:hover:bg-gray-900/30 transition-all duration-300 animate-fade-in"
              :style="{ animationDelay: index * 100 + 400 + 'ms' }"
            >
              <div class="text-4xl mb-4">{{ feature.icon }}</div>
              <h3 class="text-xl font-semibold text-white mb-2">
                {{ feature.title }}
              </h3>
              <p
                class="text-blue-100 dark:text-emerald-100 transition-colors duration-500"
              >
                {{ feature.description }}
              </p>
            </div>
          </div>
        </div>
      </section>

      <!-- 产品展示 -->
      <section id="products" class="py-20 px-4 sm:px-6 lg:px-8">
        <div class="max-w-6xl mx-auto">
          <div class="text-center mb-16">
            <h2
              class="text-3xl md:text-4xl font-bold text-white mb-4 animate-fade-in"
            >
              {{ $t("intro.products.title") }}
            </h2>
            <p
              class="text-xl text-blue-100 dark:text-emerald-100 max-w-2xl mx-auto transition-colors duration-500 animate-fade-in delay-200"
            >
              {{ $t("intro.products.subtitle") }}
            </p>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
            <div
              v-for="(product, index) in products"
              :key="index"
              class="bg-white/10 dark:bg-gray-900/20 backdrop-blur-md rounded-xl overflow-hidden border border-white/20 dark:border-gray-700/30 hover:bg-white/20 dark:hover:bg-gray-900/30 transition-all duration-300 animate-fade-in"
              :style="{ animationDelay: index * 100 + 600 + 'ms' }"
            >
              <img
                :src="product.image"
                :alt="product.name"
                class="w-full h-48 object-cover"
              />
              <div class="p-6">
                <h3 class="text-xl font-semibold text-white mb-2">
                  {{ product.name }}
                </h3>
                <p
                  class="text-blue-100 dark:text-emerald-100 mb-4 transition-colors duration-500"
                >
                  {{ product.description }}
                </p>
                <ul class="space-y-2">
                  <li
                    v-for="(feature, featureIndex) in product.features"
                    :key="featureIndex"
                    class="text-sm text-blue-200 dark:text-emerald-200 flex items-center transition-colors duration-500"
                  >
                    <span
                      class="w-2 h-2 bg-emerald-400 rounded-full mr-2"
                    ></span>
                    {{ feature }}
                  </li>
                </ul>
                <button
                  class="mt-4 w-full px-4 py-2 bg-gradient-to-r from-emerald-500 to-teal-500 hover:from-emerald-600 hover:to-teal-600 text-white rounded-lg transition-all duration-300"
                >
                  {{ $t("intro.products.learnMore") }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 关于我们 -->
      <section id="about" class="py-20 px-4 sm:px-6 lg:px-8">
        <div class="max-w-6xl mx-auto">
          <div class="text-center mb-16">
            <h2
              class="text-3xl md:text-4xl font-bold text-white mb-4 animate-fade-in"
            >
              {{ $t("intro.about.title") }}
            </h2>
            <p
              class="text-xl text-blue-100 dark:text-emerald-100 max-w-2xl mx-auto transition-colors duration-500 animate-fade-in delay-200"
            >
              {{ $t("intro.about.subtitle") }}
            </p>
          </div>

          <div class="grid grid-cols-1 lg:grid-cols-2 gap-12 items-center">
            <div class="animate-fade-in delay-400">
              <h3 class="text-2xl font-semibold text-white mb-4">
                {{ $t("intro.about.mission.title") }}
              </h3>
              <div
                class="space-y-4 text-blue-100 dark:text-emerald-100 transition-colors duration-500"
              >
                <p>
                  {{ $t("intro.about.mission.paragraph1") }}
                </p>
                <p>
                  {{ $t("intro.about.mission.paragraph2") }}
                </p>
                <p>
                  {{ $t("intro.about.mission.paragraph3") }}
                </p>
              </div>
            </div>

            <div class="animate-fade-in delay-600">
              <div class="grid grid-cols-1 sm:grid-cols-3 gap-6 mb-8">
                <div class="text-center">
                  <div class="text-3xl font-bold text-emerald-400 mb-2">
                    1000+
                  </div>
                  <div
                    class="text-blue-100 dark:text-emerald-100 transition-colors duration-500"
                  >
                    {{ $t("intro.about.stats.companies") }}
                  </div>
                </div>
                <div class="text-center">
                  <div class="text-3xl font-bold text-emerald-400 mb-2">
                    50K+
                  </div>
                  <div
                    class="text-blue-100 dark:text-emerald-100 transition-colors duration-500"
                  >
                    {{ $t("intro.about.stats.users") }}
                  </div>
                </div>
                <div class="text-center">
                  <div class="text-3xl font-bold text-emerald-400 mb-2">
                    99.9%
                  </div>
                  <div
                    class="text-blue-100 dark:text-emerald-100 transition-colors duration-500"
                  >
                    {{ $t("intro.about.stats.uptime") }}
                  </div>
                </div>
              </div>

              <div
                class="bg-white/10 dark:bg-gray-900/20 backdrop-blur-md rounded-xl p-6 border border-white/20 dark:border-gray-700/30"
              >
                <h4 class="text-xl font-semibold text-white mb-3">
                  {{ $t("intro.about.enterprise.title") }}
                </h4>
                <p
                  class="text-blue-100 dark:text-emerald-100 transition-colors duration-500"
                >
                  {{ $t("intro.about.enterprise.description") }}
                </p>
              </div>
            </div>
          </div>

          <div class="mt-16 animate-fade-in delay-800">
            <h3 class="text-2xl font-semibold text-white mb-8 text-center">
              {{ $t("intro.about.values.title") }}
            </h3>
            <div class="grid grid-cols-1 md:grid-cols-3 gap-8">
              <div class="text-center">
                <h4 class="text-xl font-semibold text-white mb-3">
                  {{ $t("intro.about.values.innovation.title") }}
                </h4>
                <p
                  class="text-blue-100 dark:text-emerald-100 transition-colors duration-500"
                >
                  {{ $t("intro.about.values.innovation.description") }}
                </p>
              </div>
              <div class="text-center">
                <h4 class="text-xl font-semibold text-white mb-3">
                  {{ $t("intro.about.values.userFirst.title") }}
                </h4>
                <p
                  class="text-blue-100 dark:text-emerald-100 transition-colors duration-500"
                >
                  {{ $t("intro.about.values.userFirst.description") }}
                </p>
              </div>
              <div class="text-center">
                <h4 class="text-xl font-semibold text-white mb-3">
                  {{ $t("intro.about.values.excellence.title") }}
                </h4>
                <p
                  class="text-blue-100 dark:text-emerald-100 transition-colors duration-500"
                >
                  {{ $t("intro.about.values.excellence.description") }}
                </p>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- CTA区域 -->
      <section class="py-20 px-4 sm:px-6 lg:px-8 relative overflow-hidden">
        <!-- 背景装饰 -->
        <div
          class="absolute inset-0 bg-gradient-to-r from-emerald-600/20 to-teal-600/20 dark:from-emerald-400/30 dark:to-teal-400/30"
        ></div>
        <div
          class="absolute inset-0 opacity-30 dark:opacity-20"
          style="
            background-image: radial-gradient(
              circle at 2px 2px,
              rgba(255, 255, 255, 0.4) 1px,
              transparent 0
            );
            background-size: 60px 60px;
          "
        ></div>

        <!-- 动态光效 -->
        <div
          class="absolute top-0 left-1/4 w-96 h-96 bg-emerald-500/20 rounded-full blur-3xl animate-pulse"
        ></div>
        <div
          class="absolute bottom-0 right-1/4 w-80 h-80 bg-blue-500/20 rounded-full blur-3xl animate-pulse delay-1000"
        ></div>

        <div
          class="max-w-4xl mx-auto text-center px-4 sm:px-6 lg:px-8 relative z-10"
        >
          <h2
            class="text-3xl md:text-4xl font-bold text-white mb-6 animate-fade-in"
          >
            {{ $t("intro.cta.title") }}
          </h2>
          <p
            class="text-xl text-blue-100 dark:text-emerald-100 mb-8 max-w-2xl mx-auto transition-colors duration-500 animate-fade-in delay-200"
          >
            {{ $t("intro.cta.subtitle") }}
          </p>

          <div
            class="flex flex-col sm:flex-row gap-4 justify-center animate-fade-in delay-400"
          >
            <button
              class="px-8 py-4 bg-white/90 dark:bg-white/95 text-blue-600 dark:text-blue-700 hover:bg-white hover:scale-105 rounded-xl font-semibold text-lg transition-all duration-300 backdrop-blur-sm shadow-lg hover:shadow-xl"
              @click="navigateTo('/users/register')"
            >
              {{ $t("intro.cta.getStarted") }}
            </button>
            <button
              class="px-8 py-4 border-2 border-white/60 dark:border-emerald-400/40 text-emerald-600 dark:text-emerald-400 hover:border-emerald-400/60 hover:bg-emerald-400/10 rounded-xl font-semibold text-lg transition-all duration-300 backdrop-blur-sm"
              @click="navigateTo('/users/login')"
            >
              {{ $t("intro.cta.signIn") }}
            </button>
          </div>
        </div>
      </section>

      <!-- 页脚 -->
      <FooterBar />
    </div>
  </div>
</template>

<style scoped>
@keyframes float {
  0%,
  100% {
    transform: translateY(0px) rotate(0deg);
  }
  33% {
    transform: translateY(-10px) rotate(1deg);
  }
  66% {
    transform: translateY(5px) rotate(-1deg);
  }
}

@keyframes fade-in {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes pulse-slow {
  0%,
  100% {
    opacity: 0.3;
  }
  50% {
    opacity: 0.6;
  }
}

.animate-float {
  animation: float 6s ease-in-out infinite;
}

.animate-fade-in {
  animation: fade-in 0.8s ease-out forwards;
  opacity: 0;
}

.animate-pulse-slow {
  animation: pulse-slow 4s ease-in-out infinite;
}

.delay-200 {
  animation-delay: 200ms;
}

.delay-400 {
  animation-delay: 400ms;
}

.delay-600 {
  animation-delay: 600ms;
}

.delay-800 {
  animation-delay: 800ms;
}

.delay-1000 {
  animation-delay: 1000ms;
}

.delay-2000 {
  animation-delay: 2000ms;
}
</style>
