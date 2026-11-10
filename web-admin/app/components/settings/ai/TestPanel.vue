<template>
  <div
    class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)]"
  >
    <div
      class="px-4 py-3 border-b border-[var(--border-color)] font-medium text-[var(--text-primary)]"
    >
      {{ $t("settings.ai.connectionTest.title") }}
    </div>
    <div class="p-4 space-y-3">
      <div class="text-sm text-[var(--text-secondary)]">
        {{ $t("settings.ai.connectionTest.currentTarget", { title: currentTitle, provider: currentState.provider || "—", model: currentState.model || "—" }) }}
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <UButton
          icon="i-heroicons-wifi"
          color="primary"
          size="sm"
          class="whitespace-nowrap"
          :disabled="disabled || connectionTesting || quickTesting || !currentState.provider"
          :loading="connectionTesting"
          @click="onTestConnection?.()"
        >
          {{ $t("settings.ai.connectionTest.connection") }}
        </UButton>
        <UButton
          icon="i-heroicons-command-line"
          variant="ghost"
          size="sm"
          class="whitespace-nowrap"
          :disabled="disabled || connectionTesting || quickTesting || !currentState.provider || !String(currentState.model || '').trim()"
          :loading="quickTesting"
          @click="onTestQuickCall?.()"
        >
          {{ $t(quickCallValidates ? "settings.ai.connectionTest.validate" : "settings.ai.connectionTest.quickCall") }}
        </UButton>
      </div>
      <UButton v-if="onTestVision" type="button" icon="i-heroicons-photo" size="sm" variant="soft"
        data-testid="ai-vision-trial" :disabled="disabled || connectionTesting || quickTesting || !currentState.provider || !String(currentState.model || '').trim()"
        @click="onTestVision()">{{ $t('settings.ai.visionTrial.open') }}</UButton>
      <p class="text-xs text-[var(--text-secondary)]">{{ $t("settings.ai.connectionTest.hint") }}</p>
      <p v-if="!String(currentState.model || '').trim()" class="text-xs text-[var(--text-secondary)]">
        {{ $t("settings.ai.connectionTest.selectModelBeforeRun") }}
      </p>
      <div v-if="disabled && disabledReason" class="text-xs text-[var(--text-secondary)]">
        {{ disabledReason }}
      </div>
      <div
        class="rounded-md border border-[var(--border-color)] bg-[var(--bg-secondary)] p-3 min-h-[80px] text-xs text-[var(--text-secondary)]"
        role="status"
        aria-live="polite"
        :aria-busy="connectionTesting || quickTesting"
      >
        <template v-if="connectionTesting || quickTesting">
          {{ $t(connectionTesting ? "settings.ai.connectionTest.connecting" : quickCallValidates ? "settings.ai.connectionTest.checking" : "settings.ai.connectionTest.calling") }}
        </template>
        <template v-else-if="lastTestMessage">
          <div class="font-medium text-[var(--text-primary)] mb-1">
            {{ $t("settings.ai.connectionTest.result") }}
          </div>
          <pre class="whitespace-pre-wrap break-all">{{ lastTestMessage }}</pre>
          <details
            v-if="lastTestDetail"
            class="mt-2 rounded border border-[var(--border-color)] bg-[var(--card-bg)] p-2"
          >
            <summary class="cursor-pointer select-none text-[var(--text-primary)]">
              {{ $t("settings.ai.connectionTest.details") }}
            </summary>
            <pre class="mt-2 whitespace-pre-wrap break-all">{{ lastTestDetail }}</pre>
          </details>
        </template>
        <template v-else>
          <div class="text-[var(--text-secondary)]">{{ $t("settings.ai.connectionTest.empty") }}</div>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  currentTitle: string;
  currentState: {
    provider: string;
    model: string;
    apiKey?: string;
    baseURL?: string;
    region?: string;
    organization?: string;
  };
  lastTestMessage?: string;
  lastTestDetail?: string;
  disabled?: boolean;
  disabledReason?: string;
  connectionTesting?: boolean;
  quickTesting?: boolean;
  quickCallValidates?: boolean;
  onTestConnection?: () => void;
  onTestQuickCall?: () => void;
  onTestVision?: () => void;
}>();
</script>
