<script setup lang="ts">
import { useColorMode } from "#imports";
import { LOGO_M_URL } from "~/utils/assets";

const props = defineProps<{
  message?: string;
  progress?: number; // 0-100 的百分比，如果提供则显示进度条，否则显示跳动点
}>();

// 检测当前主题模式
const colorMode = useColorMode();
const isDark = computed(() => colorMode.value === "dark");

// 判断是否为百分比模式
const isProgressMode = computed(() => typeof props.progress === "number");
const progressValue = computed(() =>
  Math.max(0, Math.min(100, props.progress || 0))
);
</script>

<template>
  <UModal
    title="..."
    description="..."
    :close="false"
    :dismissible="false"
    :overlay="true"
    fullscreen
    :transition="false"
  >
    <template #body>
      <div
        class="h-svh w-svw relative overflow-hidden"
        :class="[
          isDark
            ? 'bg-gradient-to-br from-gray-900 via-purple-900/40 to-indigo-900/30'
            : 'bg-gradient-to-br from-blue-50 via-indigo-50/50 to-purple-50',
        ]"
      >
        <!-- 动态背景粒子 -->
        <div class="absolute inset-0">
          <!-- 大光晕 -->
          <div
            class="absolute top-1/4 left-1/4 w-96 h-96 rounded-full blur-3xl animate-float-slow opacity-30"
            :class="[
              isDark
                ? 'bg-gradient-to-r from-purple-500/50 to-blue-500/50'
                : 'bg-gradient-to-r from-blue-400/20 to-purple-400/20',
            ]"
          ></div>
          <div
            class="absolute bottom-1/4 right-1/4 w-80 h-80 rounded-full blur-3xl animate-float-reverse opacity-25"
            :class="[
              isDark
                ? 'bg-gradient-to-r from-indigo-500/45 to-purple-500/45'
                : 'bg-gradient-to-r from-indigo-400/20 to-pink-400/20',
            ]"
          ></div>

          <!-- 小光点 -->
          <div
            class="absolute top-1/3 right-1/3 w-32 h-32 rounded-full blur-2xl animate-pulse-slow"
            :class="[
              isDark
                ? 'bg-gradient-to-r from-cyan-400/60 to-blue-400/60'
                : 'bg-gradient-to-r from-cyan-300/30 to-blue-300/30',
            ]"
          ></div>
          <div
            class="absolute bottom-1/3 left-1/3 w-24 h-24 rounded-full blur-xl animate-pulse-slower"
            :class="[
              isDark
                ? 'bg-gradient-to-r from-pink-400/55 to-purple-400/55'
                : 'bg-gradient-to-r from-pink-300/30 to-purple-300/30',
            ]"
          ></div>
        </div>

        <!-- 网格背景 -->
        <div
          class="absolute inset-0"
          :class="[
            isDark
              ? 'bg-[radial-gradient(circle_at_1px_1px,_rgba(255,255,255,0.1)_1px,_transparent_0)] opacity-10'
              : 'bg-[radial-gradient(circle_at_1px_1px,_rgb(99_102_241)_1px,_transparent_0)] opacity-5',
          ]"
          style="background-size: 40px 40px"
        ></div>

        <!-- 主要内容 -->
        <div
          class="relative z-10 h-full flex flex-col items-center justify-center"
        >
          <!-- 多层加载动画 -->
          <div class="relative w-32 h-32 mb-12">
            <!-- 最外层旋转环 -->
            <div
              class="absolute inset-0 w-32 h-32 border-3 border-transparent rounded-full animate-spin-slow"
              :class="[
                isDark
                  ? 'border-t-purple-400/80 border-r-blue-400/60'
                  : 'border-t-blue-500/60 border-r-purple-500/40',
              ]"
            ></div>

            <!-- 中层旋转环 -->
            <div
              class="absolute inset-3 w-26 h-26 border-2 border-transparent rounded-full animate-spin-reverse"
              :class="[
                isDark
                  ? 'border-l-cyan-400/70 border-b-indigo-400/50'
                  : 'border-l-indigo-500/50 border-b-cyan-500/30',
              ]"
            ></div>

            <!-- 中心 Logo -->
            <div
              class="absolute inset-3 w-26 h-26 flex items-center justify-center"
            >
              <img
                :src="LOGO_M_URL"
                alt="PowerX Logo"
                class="w-22 h-22 drop-shadow-2xl animate-pulse-gentle"
              />
            </div>
          </div>

          <!-- 消息文本 -->
          <div class="text-center space-y-4 mb-8">
            <h2
              class="text-2xl font-bold tracking-wide animate-fade-in"
              :class="[
                isDark
                  ? 'text-white drop-shadow-2xl'
                  : 'text-gray-800 drop-shadow-sm',
              ]"
            >
              {{ props.message ?? "加载中…" }}
            </h2>

            <!-- 跳动点 (仅在非百分比模式下显示) -->
            <div v-if="!isProgressMode" class="flex space-x-2 justify-center">
              <div
                class="w-3 h-3 rounded-full animate-bounce-1"
                :class="[
                  isDark
                    ? 'bg-gradient-to-r from-purple-400 to-blue-400 shadow-sm shadow-purple-400/50'
                    : 'bg-gradient-to-r from-blue-500 to-purple-500',
                ]"
              ></div>
              <div
                class="w-3 h-3 rounded-full animate-bounce-2"
                :class="[
                  isDark
                    ? 'bg-gradient-to-r from-blue-400 to-cyan-400 shadow-sm shadow-blue-400/50'
                    : 'bg-gradient-to-r from-purple-500 to-indigo-500',
                ]"
              ></div>
              <div
                class="w-3 h-3 rounded-full animate-bounce-3"
                :class="[
                  isDark
                    ? 'bg-gradient-to-r from-cyan-400 to-purple-400 shadow-sm shadow-cyan-400/50'
                    : 'bg-gradient-to-r from-indigo-500 to-pink-500',
                ]"
              ></div>
            </div>

            <!-- 百分比显示 (仅在百分比模式下显示) -->
            <div v-if="isProgressMode" class="text-center">
              <span
                class="text-lg font-semibold"
                :class="[
                  isDark ? 'text-purple-200 drop-shadow-lg' : 'text-purple-600',
                ]"
              >
                {{ progressValue }}%
              </span>
            </div>
          </div>

          <!-- 进度条 (仅在百分比模式下显示) -->
          <div
            v-if="isProgressMode"
            class="w-80 h-3 rounded-full overflow-hidden backdrop-blur-sm"
            :class="[
              isDark
                ? 'bg-white/15 shadow-inner border border-white/10'
                : 'bg-gray-200/50 shadow-inner',
            ]"
          >
            <div
              class="h-full rounded-full transition-all duration-300 ease-out"
              :class="[
                isDark
                  ? 'bg-gradient-to-r from-purple-400 via-blue-400 to-cyan-400 shadow-lg shadow-purple-400/30'
                  : 'bg-gradient-to-r from-blue-500 via-purple-500 to-indigo-500',
              ]"
              :style="{ width: `${progressValue}%` }"
            ></div>
          </div>
        </div>
      </div>
    </template>
  </UModal>
</template>

<style scoped>
/* 基础旋转动画 */
@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@keyframes spin-slow {
  to {
    transform: rotate(360deg);
  }
}

@keyframes spin-reverse {
  to {
    transform: rotate(-360deg);
  }
}

/* 浮动动画 */
@keyframes float {
  0%,
  100% {
    transform: translateY(0px) scale(1);
  }
  50% {
    transform: translateY(-10px) scale(1.02);
  }
}

@keyframes float-slow {
  0%,
  100% {
    transform: translate(0px, 0px) scale(1);
  }
  33% {
    transform: translate(30px, -30px) scale(1.1);
  }
  66% {
    transform: translate(-20px, 20px) scale(0.9);
  }
}

@keyframes float-reverse {
  0%,
  100% {
    transform: translate(0px, 0px) scale(1);
  }
  33% {
    transform: translate(-30px, 30px) scale(0.9);
  }
  66% {
    transform: translate(20px, -20px) scale(1.1);
  }
}

/* 脉冲动画 */
@keyframes pulse-gentle {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.8;
    transform: scale(1.05);
  }
}

@keyframes pulse-fast {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.6;
    transform: scale(1.2);
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
    transform: scale(1.1);
  }
}

@keyframes pulse-slower {
  0%,
  100% {
    opacity: 0.2;
    transform: scale(1);
  }
  50% {
    opacity: 0.5;
    transform: scale(1.15);
  }
}

/* 跳动动画 */
@keyframes bounce-1 {
  0%,
  80%,
  100% {
    transform: scale(1) translateY(0);
  }
  40% {
    transform: scale(1.1) translateY(-8px);
  }
}

@keyframes bounce-2 {
  0%,
  80%,
  100% {
    transform: scale(1) translateY(0);
  }
  40% {
    transform: scale(1.1) translateY(-8px);
  }
}

@keyframes bounce-3 {
  0%,
  80%,
  100% {
    transform: scale(1) translateY(0);
  }
  40% {
    transform: scale(1.1) translateY(-8px);
  }
}

/* 进度条波浪动画 */
@keyframes loading-wave {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}

/* 淡入动画 */
@keyframes fade-in {
  0% {
    opacity: 0;
    transform: translateY(20px);
  }
  100% {
    opacity: 1;
    transform: translateY(0);
  }
}

/* 应用动画类 */
.animate-spin {
  animation: spin 1s linear infinite;
}

.animate-spin-slow {
  animation: spin-slow 4s linear infinite;
}

.animate-spin-reverse {
  animation: spin-reverse 3s linear infinite;
}

.animate-float {
  animation: float 3s ease-in-out infinite;
}

.animate-float-slow {
  animation: float-slow 8s ease-in-out infinite;
}

.animate-float-reverse {
  animation: float-reverse 10s ease-in-out infinite;
}

.animate-pulse-gentle {
  animation: pulse-gentle 2s ease-in-out infinite;
}

.animate-pulse-fast {
  animation: pulse-fast 1s ease-in-out infinite;
}

.animate-pulse-slow {
  animation: pulse-slow 3s ease-in-out infinite;
}

.animate-pulse-slower {
  animation: pulse-slower 4s ease-in-out infinite;
}

.animate-bounce-1 {
  animation: bounce-1 1.4s ease-in-out infinite;
}

.animate-bounce-2 {
  animation: bounce-2 1.4s ease-in-out infinite 0.2s;
}

.animate-bounce-3 {
  animation: bounce-3 1.4s ease-in-out infinite 0.4s;
}

.animate-loading-wave {
  animation: loading-wave 2s ease-in-out infinite;
}

.animate-fade-in {
  animation: fade-in 1s ease-out;
}
</style>
