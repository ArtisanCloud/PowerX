<template>
  <div class="p-8">
    <h1 class="text-2xl font-bold mb-4">认证测试页面</h1>

    <div class="space-y-4">
      <div>
        <strong>认证状态:</strong> {{ isAuthenticated ? "已登录" : "未登录" }}
      </div>

      <div><strong>Token:</strong> {{ token || "无" }}</div>

      <div class="space-x-4">
        <UButton @click="testLogin" color="primary"> 测试登录 </UButton>

        <UButton @click="testLogout" color="error"> 测试登出 </UButton>
      </div>

      <div v-if="error" class="text-red-500">错误: {{ error }}</div>
    </div>
  </div>
</template>

<script setup>
const { login } = useAuthService();
const { isAuthenticated, token, setAuth, logout } = useAuth();

const error = ref("");

const testLogin = async () => {
  try {
    error.value = "";
    const response = await login({
      tenant: "",
      identifier: "matrix-x@artisan-cloud.com",
      password: "123456",
    });

    console.info("完整响应:", response);

    if (response.code === 200) {
      setAuth(response.data);
      console.info("登录成功:", response.data);
      console.info("时间戳:", response.timestamp);
    } else {
      error.value = response.message || "登录失败";
    }
  } catch (err) {
    console.error("登录错误:", err);
    error.value = err.message || "登录失败";
  }
};

const testLogout = async () => {
  try {
    await logout();
  } catch (err) {
    console.error("登出错误:", err);
  }
};
</script>
