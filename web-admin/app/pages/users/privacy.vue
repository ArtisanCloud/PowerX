<template>
  <div
    class="min-h-screen bg-gradient-to-br from-blue-50 via-indigo-50 to-purple-50 dark:from-gray-900 dark:via-purple-900/20 dark:to-indigo-900/20"
  >
    <!-- 背景装饰 -->
    <div class="absolute inset-0 overflow-hidden">
      <div
        class="absolute -top-40 -right-40 w-80 h-80 bg-purple-300 dark:bg-purple-600 rounded-full mix-blend-multiply dark:mix-blend-screen filter blur-xl opacity-70 animate-blob"
      ></div>
      <div
        class="absolute -bottom-40 -left-40 w-80 h-80 bg-yellow-300 dark:bg-yellow-600 rounded-full mix-blend-multiply dark:mix-blend-screen filter blur-xl opacity-70 animate-blob animation-delay-2000"
      ></div>
      <div
        class="absolute top-40 left-40 w-80 h-80 bg-pink-300 dark:bg-pink-600 rounded-full mix-blend-multiply dark:mix-blend-screen filter blur-xl opacity-70 animate-blob animation-delay-4000"
      ></div>
    </div>

    <div class="relative z-10 container mx-auto px-4 py-8">
      <!-- 头部导航 -->
      <div class="flex items-center justify-between mb-8">
        <NuxtLink
          to="/users/register"
          class="flex items-center space-x-2 text-gray-600 dark:text-gray-300 hover:text-primary-600 dark:hover:text-primary-400 transition-colors"
        >
          <UIcon name="i-heroicons-arrow-left" class="w-5 h-5" />
          <span>返回注册</span>
        </NuxtLink>

        <ThemeSwitcher />
      </div>

      <!-- 主要内容 -->
      <PrivacyModal v-model:open="open" />
    </div>
  </div>
</template>

<script setup lang="ts">
import PrivacyModal from "~/components/users/PrivacyModal.vue";

const open = ref(true);

// 页面元数据
definePageMeta({
  layout: false, // 禁用layout
  title: "隐私政策",
  description: "了解我们如何收集、使用和保护您的个人信息",
});

// SEO 设置
useSeoMeta({
  title: "隐私政策",
  description: "了解我们如何收集、使用和保护您的个人信息",
  ogTitle: "隐私政策",
  ogDescription: "了解我们如何收集、使用和保护您的个人信息",
});
</script>

<style scoped>
@keyframes blob {
  0% {
    transform: translate(0px, 0px) scale(1);
  }
  33% {
    transform: translate(30px, -50px) scale(1.1);
  }
  66% {
    transform: translate(-20px, 20px) scale(0.9);
  }
  100% {
    transform: translate(0px, 0px) scale(1);
  }
}

.animate-blob {
  animation: blob 7s infinite;
}

.animation-delay-2000 {
  animation-delay: 2s;
}

.animation-delay-4000 {
  animation-delay: 4s;
}

.prose {
  max-width: none;
}

.prose p {
  margin-bottom: 1rem;
}

.prose ul {
  margin-bottom: 1rem;
}

.prose li {
  margin-bottom: 0.5rem;
}
</style>
