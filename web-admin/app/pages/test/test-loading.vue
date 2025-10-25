<template>
  <div class="p-8">
    <h1 class="text-2xl font-bold mb-4">简单 Loading 测试</h1>

    <div class="space-y-4">
      <div class="bg-gray-100 p-4 rounded">
        <h3 class="font-semibold mb-2">当前状态：</h3>
        <p>Loading 可见: {{ visible }}</p>
        <p>Loading 消息: {{ message }}</p>
      </div>

      <div class="space-x-4">
        <button
          @click="testShow"
          class="px-4 py-2 bg-blue-500 text-white rounded"
        >
          显示 Loading
        </button>

        <button
          @click="testHide"
          class="px-4 py-2 bg-red-500 text-white rounded"
        >
          隐藏 Loading
        </button>

        <button
          @click="testLock"
          class="px-4 py-2 bg-orange-500 text-white rounded"
        >
          锁屏测试 (3秒)
        </button>

        <button
          @click="testCountdown"
          class="px-4 py-2 bg-purple-500 text-white rounded"
        >
          倒计时测试 (5秒)
        </button>

        <button
          @click="testProgress"
          class="px-4 py-2 bg-green-500 text-white rounded"
        >
          进度条测试
        </button>
      </div>

      <div class="mt-8">
        <h3 class="font-semibold mb-2">调试信息：</h3>
        <pre class="bg-black text-green-400 p-4 rounded text-sm">{{
          debugInfo
        }}</pre>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
const gl = useGlobalLoading();
const { visible, message } = gl;

// 获取所有状态用于调试
const autoVisible = useGL_AutoVisible();
const manualVisible = useGL_ManualVisible();
const lockCount = useGL_LockCount();
const reqPending = useGL_ReqPending();
const navPending = useGL_NavPending();

const debugInfo = computed(() => ({
  visible: visible.value,
  message: message.value,
  autoVisible: autoVisible.value,
  manualVisible: manualVisible.value,
  lockCount: lockCount.value,
  reqPending: reqPending.value,
  navPending: navPending.value,
}));

function testShow() {
  console.log("显示 Loading");
  gl.show({ message: "测试显示 Loading..." });

  // 3秒后自动关闭
  setTimeout(() => {
    gl.hide();
    console.log("自动关闭 Loading");
  }, 3000);
}

function testHide() {
  console.log("隐藏 Loading");
  gl.hide();
}

function testLock() {
  console.log("锁屏测试开始");
  gl.show({
    lock: true,
    message: "锁屏测试中...",
    minMs: 3000,
  });

  setTimeout(() => {
    console.log("锁屏测试结束");
    gl.hide();
    gl.unlock();
  }, 3000);
}

function testCountdown() {
  console.log("倒计时测试开始");
  let countdown = 5;

  gl.show({
    lock: true,
    message: `倒计时关闭 ${countdown} 秒...`,
  });

  const timer = setInterval(() => {
    countdown--;
    if (countdown > 0) {
      gl.setMessage(`倒计时关闭 ${countdown} 秒...`);
    } else {
      clearInterval(timer);
      gl.setMessage("倒计时结束，正在关闭...");
      setTimeout(() => {
        gl.hide();
        gl.unlock();
        console.log("倒计时测试结束");
      }, 500);
    }
  }, 1000);
}

function testProgress() {
  console.log("进度条测试开始");
  let progress = 0;

  gl.show({
    message: "文件上传中...",
    progress: 0,
  });

  const timer = setInterval(() => {
    progress += Math.random() * 15;
    if (progress >= 100) {
      progress = 100;
      gl.setProgress(100);
      gl.setMessage("上传完成！");
      console.log("进度条测试完成");

      setTimeout(() => {
        gl.hide();
        console.log("进度条测试结束");
      }, 1000);

      clearInterval(timer);
    } else {
      gl.setProgress(Math.floor(progress));
      gl.setMessage(`文件上传中... ${Math.floor(progress)}%`);
    }
  }, 200);
}

// 页面加载时的调试信息
onMounted(() => {
  console.log("简单测试页面已加载");
  console.log("GlobalLoading 状态:", debugInfo.value);
});
</script>
