<script setup lang="ts">
import { computed } from "vue";
import { usePluginBridge } from "~/composables/usePluginBridge";

type ThemeKey = "system" | "light" | "dark";

const colorMode = useColorMode();
usePluginBridge(); // ensure bridge watchers stay active for theme sync

const normalizePreference = (value?: string | null): ThemeKey => {
  const v = String(value ?? "").trim().toLowerCase();
  if (v === "dark" || v === "light") return v;
  return "system";
};

const current = computed<ThemeKey>(() =>
  normalizePreference(colorMode.preference)
);

function apply(t: ThemeKey) {
  colorMode.preference = t;
}
</script>

<template>
  <!-- 你可以把这段替换成你原来的 UI 组件；关键是 onSelect/@click 调用 apply(...) -->
  <UDropdownMenu
    :items="[[
      { label: 'System', onSelect: () => apply('system') },
      { label: 'Light',  onSelect: () => apply('light')  },
      { label: 'Dark',   onSelect: () => apply('dark')   },
    ]]"
  >
    <UButton variant="ghost" size="sm" class="flex items-center gap-2">
      <UIcon
        class="w-5 h-5"
        :name="current==='dark'
          ? 'i-heroicons-moon-20-solid'
          : current==='light'
          ? 'i-heroicons-sun-20-solid'
          : 'i-heroicons-computer-desktop-20-solid'"
      />
      <span class="hidden sm:inline">{{ current }}</span>
      <UIcon name="i-heroicons-chevron-down-20-solid" class="w-4 h-4" />
    </UButton>
  </UDropdownMenu>
</template>
