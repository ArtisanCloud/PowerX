<script setup lang="ts">
import "~/assets/css/theme.css";

// 设置页面标题
useHead({
  title: "PowerX Admin",
  meta: [{ name: "description", content: "PowerX 管理后台系统" }],
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
