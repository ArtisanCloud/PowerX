<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  href?: string;
  target?: string;
}>();

const href = computed(() => String(props.href || ""));
const isExternal = computed(() => /^https?:\/\//i.test(href.value) || href.value.startsWith("//"));
const isHash = computed(() => href.value.startsWith("#"));
</script>

<template>
  <!-- 外链：新开标签页 -->
  <a
    v-if="isExternal"
    :href="href"
    target="_blank"
    rel="noopener noreferrer"
  >
    <slot />
  </a>

  <!-- 页内锚点：默认行为 -->
  <a v-else-if="isHash" :href="href">
    <slot />
  </a>

  <!-- 站内路由：用 NuxtLink -->
  <NuxtLink v-else :href="href" :target="props.target">
    <slot />
  </NuxtLink>
</template>

