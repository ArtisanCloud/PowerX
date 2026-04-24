<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch, nextTick } from "vue";
import { usePluginBridge } from '~/composables/usePluginBridge'

const { register, unregister, navigateFrame } = usePluginBridge()

type TrustLevel = "trusted" | "untrusted";

const props = withDefaults(
  defineProps<{
    src: string | { href: string } // e.g. /_p/com.powerx.demo.hello_world/admin/
    trust?: TrustLevel;            // 'trusted' => same-origin 测量; 'untrusted' => 强沙箱
    min?: number;                  // 最小高度 px
    max?: number;                  // 最大高度 px
    viewOffset?: number;           // 视口高度模式减去的顶部/头部高度(px)
    title?: string;
    pluginId: string
    instanceId?: string
    navigatePath?: string
  }>(),
  {
    trust: "trusted",
    min: 400,
    max: 4096,
    viewOffset: 0,
  }
);

// 从 nuxt.config.ts -> runtimeConfig.public.upstream 读取后端基址
const runtimeConfig = useRuntimeConfig()
const { public: { upstream = 'http://127.0.0.1:8077' } } = runtimeConfig
const route = useRoute()

const debugEnabled = computed(() => {
  const q = route.query.px_debug
  if (q === "1" || q === "true") return true
  if (q === "0" || q === "false") return false
  const flag = (runtimeConfig.public as any)?.pluginConsoleDebug
  return flag === true || flag === "true"
})

/**
 * 可信模式：
 *  - 不设置 sandbox（同源受信任插件），避免浏览器对 allow-same-origin+allow-scripts 的警告
 * 不可信模式：
 *  - 去掉 same-origin，无法读内部文档，降级为“填满视口”，iframe 自身滚动
 */
const sandbox = computed(() =>
  props.trust === "trusted"
    ? undefined
    : "allow-scripts allow-forms allow-popups allow-downloads"
);

const iframeRef = ref<HTMLIFrameElement|null>(null)
const loading = ref(true);
const error = ref<string | null>(null);
const height = ref<number>(props.min);
const canMeasure = ref<boolean>(false); // 是否能同域测量
let ro: ResizeObserver | null = null;
let pollTimer: ReturnType<typeof setInterval> | null = null;

function clamp(h: number) {
  return Math.max(props.min, Math.min(props.max, h));
}

/** 视口填充（降级方案）：用 100vh 减去可选偏移 */
function applyViewportFill() {
  const vh = Math.max(document.documentElement.clientHeight, window.innerHeight || 0);
  height.value = clamp(vh - (props.viewOffset || 0));
}

/** 同域测量：读取 iframe 内文档高度 */
function measureOnce() {
  if (!iframeRef.value) return;
  try {
    const win = iframeRef.value.contentWindow;
    const doc = win?.document;
    if (!win || !doc) throw new Error("no access");

    // 访问成功 => 确认是可测量
    canMeasure.value = true;

    const b = doc.body;
    const e = doc.documentElement;

    const h = Math.max(
      b.scrollHeight, e.scrollHeight,
      b.offsetHeight, e.offsetHeight,
      b.clientHeight, e.clientHeight
    );

    height.value = clamp(h || props.min);
  } catch {
    // 跨域/被 CSP 限制等 => 降级
    canMeasure.value = false;
    applyViewportFill();
  }
}

/** 持续观察：同域时用 ResizeObserver + 兜底轮询 */
function setObservers() {
  clearObservers();

  if (!canMeasure.value) {
    // 跨域降级：监听窗口尺寸变化，更新视口填充高度
    window.addEventListener("resize", applyViewportFill);
    applyViewportFill();
    return;
  }

  try {
    const doc = iframeRef.value!.contentWindow!.document;
    ro = new ResizeObserver(() => measureOnce());
    ro.observe(doc.documentElement);
    ro.observe(doc.body);

    // 兜底：某些渲染变化不触发 ResizeObserver，定时再测一次
    pollTimer = setInterval(measureOnce, 800);
  } catch {
    canMeasure.value = false;
    applyViewportFill();
  }
}

function clearObservers() {
  if (ro) {
    try { ro.disconnect(); } catch {}
    ro = null;
  }
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
  window.removeEventListener("resize", applyViewportFill);
}

function onLoad() {
  loading.value = false;
  error.value = null;
  silenceIframeBridgeLogsIfNeeded();
  if (props.navigatePath && iframeRef.value) {
    navigateFrame(iframeRef.value, props.navigatePath)
    setTimeout(() => {
      if (props.navigatePath && iframeRef.value) {
        navigateFrame(iframeRef.value, props.navigatePath)
      }
    }, 120)
  }
  measureOnce();
  setObservers();
}

function onError() {
  loading.value = false;
  error.value = "插件页面加载失败";
  // 出错也给个可用高度，避免空白
  applyViewportFill();
}

/** 归一化 src
 * - /_p/** 保持同源，交给 Nuxt 中间件按请求类型决定是否转发后端
 * - 其他相对路径基于 upstream（http://127.0.0.1:8077）
 * - 绝对路径原样使用
 */
const cleanSrc = computed(() => {
  const raw = typeof props.src === "string" ? props.src : props.src?.href || "/"

  try {
    if (raw.startsWith("/_p/")) {
      const u = new URL(raw, "http://localhost")
      u.pathname = u.pathname.replace(/\/admin(?!\/)/, "/admin/")
      u.pathname = u.pathname.replace(/\/{2,}/g, "/")
      return `${u.pathname}${u.search}${u.hash}`
    }

    if (raw.startsWith("/__up/")) {
      const u = new URL(raw, "http://localhost")
      u.pathname = u.pathname.replace(/\/admin(?!\/)/, "/admin/")
      u.pathname = u.pathname.replace(/\/{2,}/g, "/")
      return `${u.pathname}${u.search}${u.hash}`
    }

    const base = upstream
    const u =
      raw.startsWith("http://") || raw.startsWith("https://")
        ? new URL(raw)
        : new URL(raw, base)

    // 2) /admin 强制补尾斜杠
    u.pathname = u.pathname.replace(/\/admin(?!\/)/, "/admin/")

    // 3) 清理重复斜杠（不影响协议头部）
    u.pathname = u.pathname.replace(/\/{2,}/g, "/")

    return u.toString()
  } catch (e) {
    if (debugEnabled.value) {
      console.warn("[WebView] bad src:", raw, e)
    }
    return raw
  }
})

function silenceIframeBridgeLogsIfNeeded() {
  if (debugEnabled.value || !iframeRef.value) return
  try {
    const win = iframeRef.value.contentWindow as any
    if (!win || !win.console) return
    if (win.__PX_CONSOLE_FILTER_INSTALLED__) return

    const prefixes = ["[Bridge][Plugin]", "[embedded]"]
    const shouldDrop = (args: any[]) => {
      const first = String(args?.[0] ?? "")
      return prefixes.some((p) => first.startsWith(p))
    }
    const patchMethod = (name: "log" | "info" | "debug") => {
      const original = win.console[name]
      if (typeof original !== "function") return
      win.console[name] = (...args: any[]) => {
        if (shouldDrop(args)) return
        return original.apply(win.console, args)
      }
    }

    patchMethod("log")
    patchMethod("info")
    patchMethod("debug")
    win.__PX_CONSOLE_FILTER_INSTALLED__ = true
  } catch {
    // Cross-origin 或受限时无法注入，静默忽略
  }
}

/** 首次挂载：高度兜底 + 桥注册 */
onMounted(() => {
  applyViewportFill();
  if (iframeRef.value) {
    register(iframeRef.value, { pluginId: props.pluginId, instanceId: props.instanceId }); // CHG: 只注册一次
  }
})

onBeforeUnmount(() => {
  clearObservers();
  unregister(iframeRef.value);
})

/** 当 src 变化：清理观察器 + 重新注册，以刷新 targetOrigin（非常关键） */
watch(cleanSrc, async () => {
  loading.value = true;
  error.value = null;
  clearObservers();

  await nextTick(); // 等 DOM 应用新 src
  if (iframeRef.value) {
    unregister(iframeRef.value); // CHG: 先反注册，避免旧 origin 残留
    register(iframeRef.value, { pluginId: props.pluginId, instanceId: props.instanceId });
  }
})

watch(
  () => props.navigatePath,
  (path) => {
    if (!iframeRef.value || !path) return
    navigateFrame(iframeRef.value, path)
  },
  { immediate: true }
)
</script>

<template>
  <div class="w-full">
    <div v-if="loading" class="w-full h-40 animate-pulse rounded-xl bg-gray-200 dark:bg-gray-700" />
    <div v-if="error" class="text-red-600 dark:text-red-400 text-sm my-2 p-4 bg-red-50 dark:bg-red-900/20 rounded-lg">
      {{ error }}
    </div>

    <iframe
      ref="iframeRef"
      :src="cleanSrc"
      :title="title || 'Plugin WebView'"
      :sandbox="sandbox"
      allow="clipboard-read *; clipboard-write *; fullscreen *"
      referrerpolicy="strict-origin-when-cross-origin"
      class="block w-full border-0 bg-transparent rounded-lg transition-[height] duration-300"
      :style="{ height: height + 'px' }"
      @load="onLoad"
      @error="onError"
    ></iframe>
  </div>
</template>
