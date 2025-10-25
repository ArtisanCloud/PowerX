<template>
  <button
    @click="showGuide"
    class="inline-flex items-center px-4 py-2 bg-blue-500 hover:bg-blue-600 text-white text-sm rounded-lg transition-colors"
  >
    <svg
      class="w-4 h-4 mr-2"
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="2"
        d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
      ></path>
    </svg>
    系统引导
  </button>
</template>

<script setup lang="ts">
const showGuide = () => {
  // 触发自定义事件，让WelcomeGuide组件重新显示
  window.dispatchEvent(new CustomEvent("show-welcome-guide"));
};
</script>
