<script setup lang="ts">
import type { DualChannelConnection } from "~/composables/agent/useDualChannelConnection";

interface Props {
  connection: DualChannelConnection;
}

const props = defineProps<Props>();
const { t } = useI18n();

// 复制请求ID到剪贴板
const copyRequestId = async () => {
  if (props.connection.currentRequestId.value) {
    try {
      await navigator.clipboard.writeText(
        props.connection.currentRequestId.value
      );
      // 可以添加一个 toast 提示
      console.info("请求ID已复制到剪贴板");
    } catch (error) {
      console.error("复制失败:", error);
    }
  }
};

// 测试SSE连接
const testSSEConnection = async () => {
  await props.connection.reconnectSSE();
};

// 测试WebSocket连接
const testWSConnection = async () => {
  await props.connection.reconnectWS();
};
</script>

<template>
  <div class="flex items-center gap-2">
    <!-- SSE 状态指示 (探活测试) -->
    <UButton
      size="xs"
      variant="soft"
      :color="connection.sseActive.value ? 'success' : 'neutral'"
      @click="testSSEConnection"
      :title="
        connection.sseActive.value ? 'SSE连接正常' : 'SSE连接异常，点击重新测试'
      "
    >
      <span
        class="mr-1 text-xs"
        :class="connection.sseActive.value ? 'text-green-500' : 'text-gray-400'"
      >
        ●
      </span>
      {{ t("connection.chatSignal") || "聊天信号" }}
    </UButton>

    <!-- WebSocket 状态指示 (探活测试) -->
    <UButton
      size="xs"
      variant="soft"
      :color="connection.wsActive.value ? 'success' : 'neutral'"
      @click="testWSConnection"
      :title="
        connection.wsActive.value
          ? 'WebSocket连接正常'
          : 'WebSocket连接异常，点击重新测试'
      "
    >
      <span
        class="mr-1 text-xs"
        :class="connection.wsActive.value ? 'text-green-500' : 'text-gray-400'"
      >
        ●
      </span>
      {{ t("connection.commandChannel") || "指令通道" }}
    </UButton>

    <!-- 取消按钮 -->
    <!-- <UButton size="xs" color="error" variant="soft" @click="connection.cancel">
      {{ t("agent.chat.cancel") }}
    </UButton> -->

    <!-- 复制请求ID按钮 -->
    <UButton
      v-if="connection.currentRequestId.value"
      size="xs"
      variant="ghost"
      icon="i-heroicons-clipboard-document"
      @click="copyRequestId"
    >
      ID
    </UButton>
  </div>
</template>
