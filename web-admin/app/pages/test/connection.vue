<template>
  <div class="p-6 max-w-4xl mx-auto">
    <h1 class="text-3xl font-bold mb-8">连接测试页面</h1>

    <!-- 连接状态 -->
    <div class="grid grid-cols-1 md:grid-cols-2 gap-6 mb-8">
      <div class="bg-white p-6 rounded-lg shadow">
        <h2 class="text-xl font-semibold mb-4">SSE 连接状态</h2>
        <div class="flex items-center gap-3 mb-4">
          <div
            class="w-3 h-3 rounded-full"
            :class="connection.sseActive.value ? 'bg-green-500' : 'bg-red-500'"
          ></div>
          <span>{{ connection.sseActive.value ? "已连接" : "未连接" }}</span>
        </div>
        <UButton @click="testSSE" size="sm" variant="outline">
          测试 SSE 探活
        </UButton>
        <div class="mt-3 text-sm text-gray-600">
          <p>测试地址: GET /api/agents/stream//sse?probe=1</p>
          <p>预期: event: ack → event: end</p>
        </div>
      </div>

      <div class="bg-white p-6 rounded-lg shadow">
        <h2 class="text-xl font-semibold mb-4">WebSocket 连接状态</h2>
        <div class="flex items-center gap-3 mb-4">
          <div
            class="w-3 h-3 rounded-full"
            :class="connection.wsActive.value ? 'bg-green-500' : 'bg-red-500'"
          ></div>
          <span>{{ connection.wsActive.value ? "已连接" : "未连接" }}</span>
        </div>
        <UButton @click="testWS" size="sm" variant="outline">
          测试 WebSocket 探活
        </UButton>
        <div class="mt-3 text-sm text-gray-600">
          <p>测试地址: WS /api/agents/stream/ws?probe=1</p>
          <p>预期: {"type":"ack","data":{"ok":true,...}}</p>
        </div>
      </div>
    </div>

    <!-- 消息测试 -->
    <div class="bg-white p-6 rounded-lg shadow mb-8">
      <h2 class="text-xl font-semibold mb-4">消息测试</h2>
      <div class="flex gap-3 mb-4">
        <UInput
          v-model="testMessage"
          placeholder="输入测试消息..."
          class="flex-1"
        />
        <UButton
          @click="sendTestMessage"
          :disabled="!canSendMessage"
          :loading="!!connection.currentRequestId.value"
        >
          发送消息
        </UButton>
        <UButton
          v-if="connection.currentRequestId.value"
          @click="cancelMessage"
          color="error"
          variant="outline"
        >
          取消
        </UButton>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 text-sm text-gray-600">
        <div>
          <p><strong>SSE 真流:</strong></p>
          <p>GET /api/agents/stream//sse?q=今天天气如何&flow_id=chat</p>
          <p>预期: intent → token 流 → end</p>
        </div>
        <div>
          <p><strong>WS 真流:</strong></p>
          <p>WS /api/agents/stream/ws?q=来一段WS&flow_id=chat</p>
          <p>预期: type=intent → 片段流 → type=end</p>
        </div>
      </div>
    </div>

    <!-- 当前请求信息 -->
    <div
      v-if="connection.currentRequestId.value"
      class="bg-blue-50 p-4 rounded-lg mb-8"
    >
      <h3 class="font-semibold mb-2">当前请求</h3>
      <p class="text-sm">请求ID: {{ connection.currentRequestId.value }}</p>
    </div>

    <!-- 日志 -->
    <div class="bg-white p-6 rounded-lg shadow">
      <div class="flex justify-between items-center mb-4">
        <h2 class="text-xl font-semibold">消息日志</h2>
        <UButton @click="clearLogs" size="sm" variant="ghost">
          清空日志
        </UButton>
      </div>

      <div class="bg-gray-50 p-4 rounded max-h-96 overflow-y-auto">
        <div v-if="logs.length === 0" class="text-gray-500 text-center py-4">
          暂无日志
        </div>
        <div
          v-for="(log, index) in logs"
          :key="index"
          class="mb-2 text-sm font-mono"
        >
          <span class="text-gray-400">{{ log.timestamp }}</span>
          <span
            class="ml-2 px-2 py-1 rounded text-xs"
            :class="getLogClass(log.type)"
          >
            {{ log.type.toUpperCase() }}
          </span>
          <span class="ml-2">{{ log.message }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from "vue";
import { useDualChannelConnection } from "~/composables/agent/useDualChannelConnection";

// 页面元数据
definePageMeta({
  title: "连接测试",
  layout: "default",
});

// 日志条目类型
interface LogEntry {
  timestamp: string;
  type: "info" | "success" | "error" | "warning";
  message: string;
}

// 连接管理
const connection = useDualChannelConnection();

// 状态管理
const testMessage = ref("今天天气如何？");
const logs = ref<LogEntry[]>([]);

// 日志管理
const addLog = (type: LogEntry["type"], message: string) => {
  logs.value.push({
    timestamp: new Date().toLocaleTimeString(),
    type,
    message,
  });

  // 保持最多100条日志
  if (logs.value.length > 100) {
    logs.value.shift();
  }
};

const clearLogs = () => {
  logs.value = [];
};

const getLogClass = (type: string) => {
  switch (type) {
    case "success":
      return "bg-green-100 text-green-800";
    case "error":
      return "bg-red-100 text-red-800";
    case "warning":
      return "bg-yellow-100 text-yellow-800";
    default:
      return "bg-blue-100 text-blue-800";
  }
};

// 计算属性
const canSendMessage = computed(() => {
  return (
    (connection.sseActive.value || connection.wsActive.value) &&
    testMessage.value.trim().length > 0 &&
    !connection.currentRequestId.value
  );
});

// SSE 探活测试
const testSSE = async () => {
  addLog("info", "开始 SSE 探活测试...");
  try {
    await connection.reconnectSSE();
    addLog(
      connection.sseActive.value ? "success" : "error",
      `SSE 探活测试${connection.sseActive.value ? "成功" : "失败"}`
    );
  } catch (error) {
    addLog("error", `SSE 测试异常: ${error}`);
  }
};

// WebSocket 探活测试
const testWS = async () => {
  addLog("info", "开始 WebSocket 探活测试...");
  try {
    await connection.reconnectWS();
    addLog(
      connection.wsActive.value ? "success" : "error",
      `WebSocket 探活测试${connection.wsActive.value ? "成功" : "失败"}`
    );
  } catch (error) {
    addLog("error", `WebSocket 测试异常: ${error}`);
  }
};

// 发送测试消息
const sendTestMessage = async () => {
  if (!canSendMessage.value) return;

  const message = testMessage.value.trim();
  addLog("info", `发送消息: ${message}`);

  try {
    await connection.sendMessage(message, "chat");
    addLog("success", "消息发送成功");
  } catch (error) {
    addLog("error", `发送消息失败: ${error}`);
  }
};

// 取消消息
const cancelMessage = () => {
  connection.cancel();
  addLog("warning", "已取消当前请求");
};

// 设置连接回调
connection.onMessage = (data: any) => {
  if (data.type === "intent") {
    addLog("info", `Intent: ${JSON.stringify(data)}`);
  } else if (data.type === "token" || data.type === "chunk") {
    addLog(
      "success",
      `Token: ${data.token || data.content || data.chunk || ""}`
    );
  } else if (data.type === "end") {
    addLog("info", "消息流结束");
  }
};

connection.onError = (error: any) => {
  addLog("error", `连接错误: ${error}`);
};

// 监听连接状态变化
watch(
  () => connection.sseActive.value,
  (active) => {
    addLog(
      active ? "success" : "warning",
      `SSE连接${active ? "已建立" : "已断开"}`
    );
  }
);

watch(
  () => connection.wsActive.value,
  (active) => {
    addLog(
      active ? "success" : "warning",
      `WebSocket连接${active ? "已建立" : "已断开"}`
    );
  }
);

// 页面加载时初始化
onMounted(() => {
  addLog("info", "页面已加载，可以开始测试连接");
});
</script>
