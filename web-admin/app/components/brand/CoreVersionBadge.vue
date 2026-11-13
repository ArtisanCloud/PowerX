<script setup lang="ts">
const setupStatus = useSetupStatus();
const version = useState<string>("app-runtime-version", () => "");
const { t } = useI18n({ useScope: "global" });

watch(
  () => setupStatus.status.value?.version,
  (value) => {
    if (value?.trim()) version.value = value.trim();
  },
  { immediate: true },
);

onMounted(async () => {
  // 复用 Setup 缓存，只读取真实 Core 版本，不用前端的默认版本冒充。
  try {
    const status = await setupStatus.load({ ttlMs: 5000 });
    if (status?.version?.trim()) version.value = status.version.trim();
  } catch {
    // 版本读取失败不阻断页面，保留已有版本或明确的空值占位。
  }
});
</script>

<template>
  <span
    class="inline-block max-w-44 truncate rounded bg-gray-100 px-1.5 py-0.5 font-mono text-[10px] leading-3 text-gray-500 dark:bg-white/5 dark:text-gray-400"
    :title="t('system.info.appVersion') + ': ' + (version || '—')"
  >{{ version || "—" }}</span>
</template>
