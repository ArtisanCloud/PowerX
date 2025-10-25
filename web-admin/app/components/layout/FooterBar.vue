<template>
  <footer
    class="bg-white/95 dark:bg-gray-900/95 backdrop-blur-sm border-t border-gray-200/60 dark:border-gray-700/60 shadow-sm px-6 py-4"
  >
    <div class="flex items-center justify-between text-sm text-gray-600">
      <div class="flex items-center gap-2">
        <span>© 2024 PowerX.</span>
        <span class="text-gray-500">{{ $t("home.footer") }}</span>
      </div>
      <div class="flex items-center gap-4">
        <!-- 语言/主题切换器 -->
        <LanguageSwitcher />
        <ThemeSwitcher />
        <!-- 原有链接 -->
        <NuxtLink :to="$localePath('/about')" class="hover:text-gray-900">{{
          $t("intro.footer.aboutUs")
        }}</NuxtLink>
        <NuxtLink :to="$localePath('/contact')" class="hover:text-gray-900">{{
          $t("intro.footer.contactUs")
        }}</NuxtLink>
        <NuxtLink
          :to="$localePath('/users/privacy')"
          class="hover:text-gray-900"
          >{{ $t("privacyPolicy") }}</NuxtLink
        >
      </div>
    </div>
  </footer>
</template>

<script setup lang="ts">
// 依赖已有的切换器组件
import LanguageSwitcher from "@/components/LanguageSwitcher.vue";
import ThemeSwitcher from "@/components/ThemeSwitcher.vue";
</script>
