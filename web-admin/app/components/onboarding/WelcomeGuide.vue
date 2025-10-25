<template>
  <div
    v-if="showGuide"
    class="fixed inset-0 bg-gray-900/50 dark:bg-black/60 flex items-center justify-center z-50"
  >
    <div
      class="bg-white dark:bg-gray-800 rounded-lg p-8 max-w-lg mx-4 shadow-2xl dark:shadow-gray-900"
    >
      <!-- 步骤1：欢迎 -->
      <div v-if="currentStep === 1" class="text-center">
        <div class="mb-6">
          <div
            class="w-16 h-16 bg-blue-100 dark:bg-blue-900 rounded-full flex items-center justify-center mx-auto mb-4"
          >
            <svg
              class="w-8 h-8 text-blue-600 dark:text-blue-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M7 11.5V14m0-2.5v-6a1.5 1.5 0 113 0m-3 6a1.5 1.5 0 00-3 0v2a7.5 7.5 0 0015 0v-5a1.5 1.5 0 00-3 0m-6-3V11m0-5.5v-1a1.5 1.5 0 013 0v1m0 0V11m0-5.5a1.5 1.5 0 013 0v3.5M3 16.5h18"
              ></path>
            </svg>
          </div>
          <h2 class="text-2xl font-bold text-gray-900 dark:text-white mb-2">
            欢迎使用 PowerX 系统！
          </h2>
          <p class="text-gray-600 dark:text-gray-300">
            这是一个强大的AI驱动业务管理平台，让我们快速了解一下主要功能。
          </p>
        </div>
        <div class="flex justify-between">
          <button
            @click="skipGuide"
            class="px-4 py-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 transition-colors"
          >
            跳过引导
          </button>
          <button
            @click="nextStep"
            class="px-6 py-2 bg-green-500 hover:bg-green-600 dark:bg-green-500 dark:hover:bg-green-600 text-white rounded-lg transition-colors"
          >
            开始了解
          </button>
        </div>
      </div>

      <!-- 步骤2：功能介绍 -->
      <div v-if="currentStep === 2" class="text-center">
        <div class="mb-6">
          <div
            class="w-16 h-16 bg-green-100 dark:bg-green-900 rounded-full flex items-center justify-center mx-auto mb-4"
          >
            <svg
              class="w-8 h-8 text-green-600 dark:text-green-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"
              ></path>
            </svg>
          </div>
          <h2 class="text-2xl font-bold text-gray-900 dark:text-white mb-2">
            AI 智能助手
          </h2>
          <p class="text-gray-600 dark:text-gray-300">
            系统内置了强大的AI助手功能，可以帮助您：
          </p>
          <ul class="text-left text-gray-600 dark:text-gray-300 mt-4 space-y-2">
            <li class="flex items-center">
              <span class="w-2 h-2 bg-green-500 rounded-full mr-3"></span>
              自动化处理业务流程
            </li>
            <li class="flex items-center">
              <span class="w-2 h-2 bg-green-500 rounded-full mr-3"></span>
              智能数据分析和报告
            </li>
            <li class="flex items-center">
              <span class="w-2 h-2 bg-green-500 rounded-full mr-3"></span>
              个性化业务建议
            </li>
          </ul>
        </div>
        <div class="flex justify-between">
          <button
            @click="prevStep"
            class="px-4 py-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 transition-colors"
          >
            上一步
          </button>
          <button
            @click="nextStep"
            class="px-6 py-2 bg-green-500 hover:bg-green-600 dark:bg-green-600 dark:hover:bg-green-700 text-white rounded-lg transition-colors"
          >
            继续
          </button>
        </div>
      </div>

      <!-- 步骤3：配置引导 -->
      <div v-if="currentStep === 3" class="text-center">
        <div class="mb-6">
          <div
            class="w-16 h-16 bg-purple-100 dark:bg-purple-900 rounded-full flex items-center justify-center mx-auto mb-4"
          >
            <svg
              class="w-8 h-8 text-purple-600 dark:text-purple-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"
              ></path>
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
              ></path>
            </svg>
          </div>
          <h2 class="text-2xl font-bold text-gray-900 dark:text-white mb-2">
            配置 AI 设置
          </h2>
          <p class="text-gray-600 dark:text-gray-300 mb-4">
            为了获得最佳体验，建议您先配置AI助手的相关设置。
          </p>
          <div class="bg-blue-50 dark:bg-blue-900/30 p-4 rounded-lg mb-4">
            <p class="text-sm text-blue-800 dark:text-blue-200">
              💡 在AI设置页面，您可以配置API密钥、选择模型、设置对话参数等。
            </p>
          </div>
        </div>
        <div class="flex justify-between">
          <button
            @click="prevStep"
            class="px-4 py-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 transition-colors"
          >
            上一步
          </button>
          <button
            @click="goToAISettings"
            class="px-6 py-2 bg-purple-500 hover:bg-purple-600 dark:bg-purple-600 dark:hover:bg-purple-700 text-white rounded-lg transition-colors"
          >
            去配置 AI 设置
          </button>
        </div>
      </div>

      <!-- 进度指示器 -->
      <div class="flex justify-center mt-6 space-x-2">
        <div
          v-for="step in 3"
          :key="step"
          :class="[
            'w-2 h-2 rounded-full transition-colors',
            step <= currentStep
              ? 'bg-blue-500 dark:bg-blue-400'
              : 'bg-gray-300 dark:bg-gray-600',
          ]"
        ></div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
const showGuide = ref(false);
const currentStep = ref(1);

// 监听显示引导的事件
const handleShowGuide = (e: Event) => {
  // 如果需要，可读取 (e as CustomEvent).detail.reset
  showGuide.value = true;
  currentStep.value = 1;
};

// 手动显示引导（用于测试）
const showWelcomeGuide = () => {
  showGuide.value = true;
  currentStep.value = 1;
};

// 检查是否需要显示引导
onMounted(() => {
  // 简单的本地存储检查，不依赖认证状态
  if (typeof localStorage !== "undefined") {
    const hasSeenGuide = localStorage.getItem("welcome_guide_completed");
    if (!hasSeenGuide) {
      // 延迟显示，让页面先加载完成
      setTimeout(() => {
        showGuide.value = true;
      }, 1000);
    }
  }

  // 监听自定义事件
  window.addEventListener("show-welcome-guide", handleShowGuide);
});

onBeforeUnmount(() => {
  window.removeEventListener("show-welcome-guide", handleShowGuide);
});

const nextStep = () => {
  if (currentStep.value < 3) {
    currentStep.value++;
  }
};

const prevStep = () => {
  if (currentStep.value > 1) {
    currentStep.value--;
  }
};

const skipGuide = () => {
  showGuide.value = false;
  if (typeof localStorage !== "undefined") {
    localStorage.setItem("welcome_guide_completed", "true");
  }
};

const goToAISettings = () => {
  showGuide.value = false;
  if (typeof localStorage !== "undefined") {
    localStorage.setItem("welcome_guide_completed", "true");
  }
  // 跳转到AI设置页面
  navigateTo("/settings/ai");
};

// 暴露方法供外部调用
defineExpose({
  showWelcomeGuide,
});
</script>
