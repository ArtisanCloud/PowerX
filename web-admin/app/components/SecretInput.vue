<script setup lang="ts">
import { ref, watch } from "vue";

defineOptions({ inheritAttrs: false });

const props = defineProps<{
  disabled?: boolean;
  loading?: boolean;
  multiline?: boolean;
}>();
const model = defineModel<string>();
const { t } = useI18n();
const visible = ref(false);

watch(model, (value) => {
  if (!value) visible.value = false;
});
watch(() => props.disabled || props.loading, (blocked) => {
  if (blocked) visible.value = false;
});
</script>

<template>
  <UTextarea
    v-if="multiline"
    v-model="model"
    v-bind="$attrs"
    :disabled="disabled"
    :loading="loading"
    :class="{ 'secret-input-masked': !visible }"
    class="w-full"
  >
    <template #trailing>
      <UButton
        type="button"
        color="neutral"
        variant="ghost"
        size="xs"
        :icon="visible ? 'i-lucide-eye-off' : 'i-lucide-eye'"
        :aria-label="t(visible ? 'common.hideSecret' : 'common.showSecret')"
        :title="t(visible ? 'common.hideSecret' : 'common.showSecret')"
        :aria-pressed="visible"
        :disabled="disabled || loading"
        @mousedown.prevent
        @click="visible = !visible"
      />
    </template>
  </UTextarea>
  <UInput
    v-else
    v-model="model"
    v-bind="$attrs"
    :type="visible ? 'text' : 'password'"
    :disabled="disabled"
    :loading="loading"
    class="w-full"
  >
    <template #trailing>
      <UButton
        type="button"
        color="neutral"
        variant="ghost"
        size="xs"
        :icon="visible ? 'i-lucide-eye-off' : 'i-lucide-eye'"
        :aria-label="t(visible ? 'common.hideSecret' : 'common.showSecret')"
        :title="t(visible ? 'common.hideSecret' : 'common.showSecret')"
        :aria-pressed="visible"
        :disabled="disabled || loading"
        @mousedown.prevent
        @click="visible = !visible"
      />
    </template>
  </UInput>
</template>

<style scoped>
.secret-input-masked :deep(textarea) {
  -webkit-text-security: disc;
}

@supports not (-webkit-text-security: disc) {
  .secret-input-masked :deep(textarea) {
    color: transparent;
    text-shadow: none;
  }
}
</style>
