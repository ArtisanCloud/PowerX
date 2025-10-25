<script setup lang="ts">
import "~/assets/css/theme.css";

// 设置页面标题
useHead({
  title: "PowerX Admin",
  meta: [{ name: "description", content: "PowerX 管理后台系统" }],
});

onMounted(() => {
  // 开机画面：进入应用时立刻显示 600ms 的品牌开屏
  const gl = useGlobalLoading();

  // 先显示开机画面
  gl.show({ message: "启动中…", minMs: 600 });

  // 800ms 后确保隐藏
  setTimeout(() => {
    gl.hide();
    // 重置所有状态，确保完全清理
    const lockCount = useGL_LockCount();
    const manualVisible = useGL_ManualVisible();
    lockCount.value = 0;
    manualVisible.value = false;
  }, 800);
});
</script>

<template>
  <UApp>
    <NuxtLoadingIndicator :throttle="150" />
    <NuxtLayout>
      <NuxtPage />
    </NuxtLayout>
  </UApp>

  <!-- ✨ Teleport 到 body，脱离任何局部叠层上下文 -->
  <ClientOnly>
    <Teleport to="body">
      <!-- 全局 Alert 通知组件 -->
      <GlobalAlertNotification />
    </Teleport>
  </ClientOnly>
</template>
