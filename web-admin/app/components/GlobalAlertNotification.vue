<script setup lang="ts">
import { useOneShotAlert } from "../composables/useOneShotAlert";

const { visible, title, description, color, variant, icon, hide } =
  useOneShotAlert();
</script>

<template>
  <div class="pointer-events-none fixed inset-0 z-[9999]">
    <!-- 外层不接收事件，内部卡片可点击关闭 -->
    <div class="pointer-events-auto absolute bottom-4 right-4">
      <UAlert
        v-if="visible"
        :title="title"
        :description="description"
        :color="color"
        :variant="variant"
        :icon="icon"
        close
        @update:open="
          (val) => {
            hide();
          }
        "
        class="fixed bottom-4 right-4 max-w-sm z-50"
      />
    </div>
  </div>
</template>
