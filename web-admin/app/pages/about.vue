<script setup lang="ts">
definePageMeta({
  layout: false,
});

const { t } = useI18n();

// SEO 设置
useSeoMeta({
  title: "关于我们 - PowerX",
  description:
    "了解 PowerX 团队和我们的使命，我们致力于为企业提供最优质的数字化解决方案。",
});
</script>

<template>
  <div
    class="min-h-screen bg-gradient-to-br from-blue-50 via-white to-purple-50"
  >
    <!-- 页面头部 -->
    <div class="bg-white shadow-sm">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div class="text-center">
          <h1 class="text-4xl font-bold text-gray-900 mb-4">
            {{ $t("intro.about.title") }}
          </h1>
          <p class="text-xl text-gray-600 max-w-3xl mx-auto">
            {{ $t("intro.about.subtitle") }}
          </p>
        </div>
      </div>
    </div>

    <!-- 主要内容 -->
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-16">
      <!-- 使命愿景 -->
      <section class="mb-20">
        <div class="grid lg:grid-cols-2 gap-12 items-center">
          <div>
            <h2 class="text-3xl font-bold text-gray-900 mb-6">
              {{ $t("intro.about.mission.title") }}
            </h2>
            <div class="space-y-4 text-gray-700">
              <p>{{ $t("intro.about.mission.paragraph1") }}</p>
              <p>{{ $t("intro.about.mission.paragraph2") }}</p>
              <p>{{ $t("intro.about.mission.paragraph3") }}</p>
            </div>
          </div>
          <div
            class="bg-gradient-to-br from-blue-500 to-purple-600 rounded-2xl p-8 text-white"
          >
            <div class="grid grid-cols-1 gap-6">
              <div class="text-center">
                <div class="text-4xl font-bold mb-2">500+</div>
                <div class="text-blue-100">
                  {{ $t("intro.about.stats.companies") }}
                </div>
              </div>
              <div class="text-center">
                <div class="text-4xl font-bold mb-2">10K+</div>
                <div class="text-blue-100">
                  {{ $t("intro.about.stats.users") }}
                </div>
              </div>
              <div class="text-center">
                <div class="text-4xl font-bold mb-2">99.9%</div>
                <div class="text-blue-100">
                  {{ $t("intro.about.stats.uptime") }}
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 核心价值观 -->
      <section class="mb-20">
        <div class="text-center mb-12">
          <h2 class="text-3xl font-bold text-gray-900 mb-4">
            {{ $t("intro.about.values.title") }}
          </h2>
        </div>
        <div class="grid md:grid-cols-3 gap-8">
          <div class="text-center p-6 bg-white rounded-xl shadow-lg">
            <div
              class="w-16 h-16 bg-blue-100 rounded-full flex items-center justify-center mx-auto mb-4"
            >
              <UIcon
                name="i-heroicons-light-bulb"
                class="w-8 h-8 text-blue-600"
              />
            </div>
            <h3 class="text-xl font-semibold text-gray-900 mb-3">
              {{ $t("intro.about.values.innovation.title") }}
            </h3>
            <p class="text-gray-600">
              {{ $t("intro.about.values.innovation.description") }}
            </p>
          </div>
          <div class="text-center p-6 bg-white rounded-xl shadow-lg">
            <div
              class="w-16 h-16 bg-green-100 rounded-full flex items-center justify-center mx-auto mb-4"
            >
              <UIcon name="i-heroicons-users" class="w-8 h-8 text-green-600" />
            </div>
            <h3 class="text-xl font-semibold text-gray-900 mb-3">
              {{ $t("intro.about.values.userFirst.title") }}
            </h3>
            <p class="text-gray-600">
              {{ $t("intro.about.values.userFirst.description") }}
            </p>
          </div>
          <div class="text-center p-6 bg-white rounded-xl shadow-lg">
            <div
              class="w-16 h-16 bg-purple-100 rounded-full flex items-center justify-center mx-auto mb-4"
            >
              <UIcon name="i-heroicons-star" class="w-8 h-8 text-purple-600" />
            </div>
            <h3 class="text-xl font-semibold text-gray-900 mb-3">
              {{ $t("intro.about.values.excellence.title") }}
            </h3>
            <p class="text-gray-600">
              {{ $t("intro.about.values.excellence.description") }}
            </p>
          </div>
        </div>
      </section>

      <!-- 企业级解决方案 -->
      <section class="mb-20">
        <div
          class="bg-gradient-to-r from-gray-50 to-blue-50 rounded-2xl p-8 lg:p-12"
        >
          <div class="grid lg:grid-cols-2 gap-8 items-center">
            <div>
              <h2 class="text-3xl font-bold text-gray-900 mb-4">
                {{ $t("intro.about.enterprise.title") }}
              </h2>
              <p class="text-gray-700 mb-6">
                {{ $t("intro.about.enterprise.description") }}
              </p>
              <div class="space-y-3">
                <div class="flex items-center">
                  <UIcon
                    name="i-heroicons-check-circle"
                    class="w-5 h-5 text-green-500 mr-3"
                  />
                  <span class="text-gray-700">定制化开发服务</span>
                </div>
                <div class="flex items-center">
                  <UIcon
                    name="i-heroicons-check-circle"
                    class="w-5 h-5 text-green-500 mr-3"
                  />
                  <span class="text-gray-700">7x24 技术支持</span>
                </div>
                <div class="flex items-center">
                  <UIcon
                    name="i-heroicons-check-circle"
                    class="w-5 h-5 text-green-500 mr-3"
                  />
                  <span class="text-gray-700">企业级安全保障</span>
                </div>
                <div class="flex items-center">
                  <UIcon
                    name="i-heroicons-check-circle"
                    class="w-5 h-5 text-green-500 mr-3"
                  />
                  <span class="text-gray-700">专属客户经理</span>
                </div>
              </div>
            </div>
            <div class="text-center">
              <div class="bg-white rounded-xl p-8 shadow-lg">
                <UIcon
                  name="i-heroicons-building-office"
                  class="w-20 h-20 text-blue-600 mx-auto mb-4"
                />
                <h3 class="text-xl font-semibold text-gray-900 mb-2">
                  联系我们
                </h3>
                <p class="text-gray-600 mb-4">了解更多企业级解决方案</p>
                <NuxtLink :to="$localePath('/contact')">
                  <UButton
                    size="lg"
                    class="bg-gradient-to-r from-blue-600 to-purple-600"
                  >
                    立即咨询
                  </UButton>
                </NuxtLink>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 团队介绍 -->
      <section>
        <div class="text-center mb-12">
          <h2 class="text-3xl font-bold text-gray-900 mb-4">我们的团队</h2>
          <p class="text-gray-600 max-w-2xl mx-auto">
            由经验丰富的工程师、设计师和产品专家组成的专业团队，致力于为客户提供最优质的服务。
          </p>
        </div>
        <div class="grid md:grid-cols-4 gap-6">
          <div class="text-center">
            <div
              class="w-24 h-24 bg-gradient-to-br from-blue-400 to-blue-600 rounded-full mx-auto mb-4 flex items-center justify-center"
            >
              <span class="text-white text-2xl font-bold">技</span>
            </div>
            <h3 class="font-semibold text-gray-900 mb-1">技术团队</h3>
            <p class="text-sm text-gray-600">前沿技术研发</p>
          </div>
          <div class="text-center">
            <div
              class="w-24 h-24 bg-gradient-to-br from-green-400 to-green-600 rounded-full mx-auto mb-4 flex items-center justify-center"
            >
              <span class="text-white text-2xl font-bold">设</span>
            </div>
            <h3 class="font-semibold text-gray-900 mb-1">设计团队</h3>
            <p class="text-sm text-gray-600">用户体验设计</p>
          </div>
          <div class="text-center">
            <div
              class="w-24 h-24 bg-gradient-to-br from-purple-400 to-purple-600 rounded-full mx-auto mb-4 flex items-center justify-center"
            >
              <span class="text-white text-2xl font-bold">产</span>
            </div>
            <h3 class="font-semibold text-gray-900 mb-1">产品团队</h3>
            <p class="text-sm text-gray-600">产品规划管理</p>
          </div>
          <div class="text-center">
            <div
              class="w-24 h-24 bg-gradient-to-br from-orange-400 to-orange-600 rounded-full mx-auto mb-4 flex items-center justify-center"
            >
              <span class="text-white text-2xl font-bold">服</span>
            </div>
            <h3 class="font-semibold text-gray-900 mb-1">服务团队</h3>
            <p class="text-sm text-gray-600">客户成功服务</p>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
