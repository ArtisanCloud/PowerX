<template>
  <div class="p-8">
    <h1 class="text-2xl font-bold mb-4">主题测试页面</h1>

    <div class="space-y-4">
      <div>
        <p>当前主题: {{ colorMode.value }}</p>
        <p>偏好设置: {{ colorMode.preference }}</p>
        <p>是否深色模式: {{ isDark }}</p>
      </div>

      <div class="flex gap-4">
        <UButton @click="colorMode.preference = 'light'">
          切换到浅色主题
        </UButton>
        <UButton @click="colorMode.preference = 'dark'">
          切换到深色主题
        </UButton>
        <UButton @click="colorMode.preference = 'system'"> 跟随系统 </UButton>
      </div>

      <div class="mt-8">
        <UButton @click="testLoading" color="primary" size="lg">
          测试 Loading（3秒）
        </UButton>
      </div>

      <div
        class="p-4 rounded-lg border"
        :class="[
          isDark
            ? 'bg-gray-800 border-gray-700 text-white'
            : 'bg-white border-gray-300 text-gray-900',
        ]"
      >
        <p>这个区域会根据主题变化颜色</p>
        <p>深色模式: {{ isDark ? "是" : "否" }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
const colorMode = useColorMode();
const isDark = computed(() => colorMode.value === "dark");

// 监听主题变化
watch(
  () => colorMode.value,
  (newValue) => {
    console.log("主题已切换到:", newValue);
  }
);

const testLoading = () => {
  const gl = useGlobalLoading();
  gl.show({ message: `当前主题: ${colorMode.value}`, minMs: 3000 });

  setTimeout(() => {
    gl.hide();
  }, 3000);
};
</script>
