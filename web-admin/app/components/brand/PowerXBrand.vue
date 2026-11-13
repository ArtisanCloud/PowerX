<script setup lang="ts">
withDefaults(defineProps<{ compact?: boolean; size?: "default" | "hero"; inverse?: boolean }>(), {
  compact: false,
  size: "default",
  inverse: false,
});
</script>

<template>
  <img
    :src="compact ? '/images/powerx-mark.svg' : '/images/powerx-wordmark.svg'"
    :width="compact ? 24 : 100"
    :height="compact ? 24 : 32"
    class="powerx-brand"
    :class="[
      inverse ? 'brightness-0 invert' : 'brightness-0 dark:invert',
      { 'powerx-brand--compact': compact, 'powerx-brand--hero': size === 'hero' },
    ]"
    alt="PowerX"
    draggable="false"
    data-testid="powerx-brand"
  />
</template>

<style scoped>
.powerx-brand {
  display: block;
  flex: none;
  width: 100px;
  height: 32px;
}

.powerx-brand--compact {
  width: 24px;
  height: 24px;
}

.powerx-brand--hero {
  width: 150px;
  height: 48px;
}

.powerx-brand--compact.powerx-brand--hero {
  width: 96px;
  height: 96px;
}

@media (max-width: 639px) {
  .powerx-brand--hero:not(.powerx-brand--compact) {
    width: 100px;
    height: 32px;
  }
}
</style>
