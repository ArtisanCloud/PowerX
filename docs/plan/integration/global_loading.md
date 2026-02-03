# PowerX 全局加载（Global Loading）集成说明

> 目的：明确 PowerX 底座的全局加载能力、对外暴露方式，以及与插件端适配器的交互约定。

## 1. 组件与状态

- 组件：`app/components/GlobalLoadingModal.vue`
  - 支持两种模式：
    - **无进度模式**：显示跳动点（用于未知时长操作）
    - **进度模式**：传入 `progress(0-100)` 时显示百分比 + 进度条
- 状态管理：`app/composables/useGlobalLoading.ts`
  - 关键状态：
    - `visible`：全局是否可见
    - `message`：文案
    - `progress`：百分比进度
    - `lockCount`：锁屏计数（>0 时保持可见）

## 2. 全局加载展示

- 插件：`app/plugins/gl-overlay.client.ts`
  - 监听 `useGlobalLoading()` 状态变化
  - 自动开启/关闭全局 `GlobalLoadingModal`
  - 文案与进度变化时自动更新

## 3. 对外暴露（给插件 Iframe 使用）

> 插件页面无法直接 import 底座 composables，需要由宿主通过 `window` 暴露接口。

推荐在底座 `app.vue` 或任意 client 插件中加以下桥接：

```ts
if (process.client) {
  const gl = useGlobalLoading();
  (window as any).__PX_GLOBAL_LOADING__ = {
    show: gl.show,
    hide: gl.hide,
    lock: gl.lock,
    unlock: gl.unlock,
    setMessage: gl.setMessage,
    setProgress: gl.setProgress,
  };
}
```

约定：
- 插件侧优先使用 `window.parent.__PX_GLOBAL_LOADING__`
- 若不存在该对象，插件应回退至自身实现（standalone 模式）

## 4. 推荐调用规范

### 4.1 无进度加载

```ts
const gl = useGlobalLoading();

gl.show({
  message: "加载中…",
  lock: true,
  minMs: 600,
});

// ...

gl.hide();
```

### 4.2 带进度加载

```ts
const gl = useGlobalLoading();

gl.show({
  message: "同步中…",
  lock: true,
  progress: 0,
});

gl.setProgress(35);

gl.setMessage("拉取成员详情");

gl.setProgress(100);

gl.hide();
```

## 5. 插件侧适配器对接点（说明性）

- 插件侧适配器：`Plugins/com.powerx.plugin.scrm/web-admin/app/composables/useGlobalLoadingAdapter.ts`
- 适配规则：
  - 若 `window.parent.__PX_GLOBAL_LOADING__` 存在：调用宿主
  - 否则：使用插件本地全局 loading

---

如需统一样式或新增字段，请先修改 `GlobalLoadingModal.vue` 并在插件同步拷贝样式。
