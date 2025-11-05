# 全局 Loading 系统使用指南

这是一个基于 Nuxt 4/3 + @nuxt/ui 的全屏遮罩 Loading 系统，既能当「开机画面（Splash）」也能在请求后台时屏蔽一切操作。

## 功能特性

- ✅ 自动感知路由切换和 API 请求
- ✅ 手动控制显示/隐藏和锁屏
- ✅ 最小显示时长，避免闪烁
- ✅ 全屏遮罩，完全阻止用户交互
- ✅ 开机画面支持
- ✅ 并发请求计数管理

## 基本用法

### 1. 自动模式（无需手动调用）

系统会自动在以下情况显示 Loading：

- 路由切换时
- 使用 `useFetch`、`asyncData` 等 Nuxt 内置请求方法时
- 使用 `$fetch` 进行 API 请求时

```vue
<template>
  <div>
    <!-- 这些操作会自动触发 Loading -->
    <button @click="fetchData">获取数据</button>
    <NuxtLink to="/other-page">跳转页面</NuxtLink>
  </div>
</template>

<script setup>
// 自动触发 Loading
const { data } = await useFetch("/api/users");

async function fetchData() {
  // 自动触发 Loading
  const result = await $fetch("/api/data");
}
</script>
```

### 2. 手动控制模式

```vue
<script setup>
const gl = useGlobalLoading();

// 显示 Loading（不锁屏）
gl.show({ message: "正在处理..." });

// 显示 Loading 并锁屏
gl.show({
  lock: true,
  message: "正在提交数据...",
  minMs: 500, // 最少显示 500ms
});

// 隐藏 Loading
gl.hide();

// 仅锁屏（不显示文案）
gl.lock();

// 解锁
gl.unlock();

// 更新消息
gl.setMessage("新的加载消息");
</script>
```

### 3. 表单提交示例

```vue
<template>
  <form @submit="handleSubmit">
    <input v-model="formData.name" placeholder="姓名" />
    <button type="submit" :disabled="gl.visible.value">
      {{ gl.visible.value ? "提交中..." : "提交" }}
    </button>
  </form>
</template>

<script setup>
const gl = useGlobalLoading();
const formData = reactive({ name: "" });

async function handleSubmit(e) {
  e.preventDefault();

  // 显示锁屏 Loading，最少显示 300ms
  gl.show({
    lock: true,
    minMs: 300,
    message: "正在提交表单...",
  });

  try {
    await $fetch("/api/submit", {
      method: "POST",
      body: formData,
    });

    // 成功后更新消息
    gl.setMessage("提交成功！");

    // 延迟隐藏，让用户看到成功消息
    setTimeout(() => {
      gl.hide();
    }, 1000);
  } catch (error) {
    gl.setMessage("提交失败，请重试");
    setTimeout(() => {
      gl.hide();
    }, 2000);
  }
}
</script>
```

### 4. 长时间操作示例

```vue
<script setup>
const gl = useGlobalLoading();

async function longOperation() {
  gl.show({
    lock: true,
    message: "正在处理大量数据...",
    minMs: 1000, // 至少显示 1 秒
  });

  try {
    // 模拟长时间操作
    await new Promise((resolve) => setTimeout(resolve, 3000));

    gl.setMessage("处理完成！");

    // 显示成功消息 1 秒后自动隐藏
    setTimeout(() => {
      gl.hide();
    }, 1000);
  } catch (error) {
    gl.setMessage("处理失败");
    setTimeout(() => {
      gl.hide();
    }, 2000);
  }
}
</script>
```

## API 参考

### useGlobalLoading()

返回全局 Loading 控制器对象：

```typescript
interface GlobalLoadingController {
  visible: ComputedRef<boolean>; // 当前是否可见
  message: Ref<string>; // 当前显示的消息

  show(options?: ShowOptions): void; // 显示 Loading
  hide(): void; // 隐藏 Loading
  lock(): void; // 锁屏（增加锁计数）
  unlock(): void; // 解锁（减少锁计数）
  setMessage(msg: string): void; // 设置消息
}

interface ShowOptions {
  lock?: boolean; // 是否锁屏
  minMs?: number; // 最小显示时长（毫秒）
  message?: string; // 显示消息
}
```

## 注意事项

### 1. 锁屏计数机制

- `lock()` 会增加锁计数，`unlock()` 会减少锁计数
- 只要锁计数 > 0，Loading 就会保持显示
- 多次调用 `lock()` 需要相应次数的 `unlock()` 才能完全解锁

```javascript
gl.lock(); // 锁计数: 1
gl.lock(); // 锁计数: 2
gl.unlock(); // 锁计数: 1，仍然锁屏
gl.unlock(); // 锁计数: 0，解除锁屏
```

### 2. 最小显示时长

使用 `minMs` 可以避免 Loading 闪烁，特别适用于快速完成的操作：

```javascript
// 即使操作在 100ms 内完成，Loading 也会显示至少 500ms
gl.show({ minMs: 500 });
await quickOperation(); // 假设 100ms 完成
gl.hide(); // 实际会在 500ms 后隐藏
```

### 3. 手动与自动模式

- 手动模式（`show/hide/lock/unlock`）优先级更高
- 自动模式基于路由和请求状态
- 两种模式可以同时工作，任一为真就显示 Loading

### 4. 错误处理

确保在 `try/catch` 的 `finally` 块或 `catch` 块中调用 `hide()` 或 `unlock()`：

```javascript
try {
  gl.show({ lock: true });
  await riskyOperation();
} catch (error) {
  console.error(error);
} finally {
  gl.hide(); // 确保无论成功失败都会隐藏
}
```

## 自定义样式

可以修改 `app/components/GlobalLoading.vue` 来自定义 Loading 的外观：

```vue
<template>
  <UModal
    v-model="open"
    title="loading-title"
    description="loading-desc"
    :prevent-close="true"
    :fullscreen="true"
  >
    <!-- 自定义你的 Loading UI -->
    <div class="h-screen w-screen flex items-center justify-center bg-black/80">
      <div class="text-center">
        <!-- 可以放置品牌 Logo -->
        <img src="/logo.png" alt="Logo" class="w-20 h-20 mx-auto mb-4" />

        <!-- 自定义加载动画 -->
        <div class="loading-spinner"></div>

        <!-- 消息文本 -->
        <p class="text-white mt-4">{{ message }}</p>
      </div>
    </div>
  </UModal>
</template>
```

## 调试技巧

在开发环境中，可以在浏览器控制台查看 Loading 状态：

```javascript
// 查看当前状态
console.log("Auto visible:", useState("gl:autoVisible").value);
console.log("Manual visible:", useState("gl:manualVisible").value);
console.log("Lock count:", useState("gl:lockCount").value);
console.log("Nav pending:", useState("gl:navPending").value);
console.log("Req pending:", useState("gl:reqPending").value);
```

这个全局 Loading 系统提供了完整的加载状态管理，既能自动处理常见场景，也支持精细的手动控制。
