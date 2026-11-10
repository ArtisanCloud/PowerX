<template>
  <div class="text-xs text-muted space-y-1" data-testid="ai-parameter-source">
    <p>{{ $t(`settings.ai.parameters.sources.${profile?.source || 'unknown'}`) }}<span v-if="profile?.checked_at"> · {{ profile.checked_at }}</span></p>
    <details v-if="profile" class="rounded border border-default p-2" data-testid="ai-parameter-details">
      <summary class="cursor-pointer">{{ $t('settings.ai.parameters.details') }}</summary>
      <dl class="mt-2 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 break-all">
        <template v-for="(value, key) in profile.defaults" :key="key"><dt>{{ key }}</dt><dd>{{ value }}</dd></template>
      </dl>
      <p v-if="!Object.keys(profile.defaults).length" class="mt-2">{{ $t('settings.ai.parameters.notProvided') }}</p>
      <p v-if="profile.max_output_tokens" class="mt-2">{{ $t('settings.ai.parameters.outputLimit') }}: {{ profile.max_output_tokens }}</p>
      <a v-if="profile.source === 'official_docs' && profile.source_url?.startsWith('https://developers.openai.com/')" :href="profile.source_url" target="_blank" rel="noopener noreferrer" class="mt-2 inline-block underline">{{ $t('settings.ai.visionModels.officialDocs') }}</a>
    </details>
  </div>
</template>
<script setup lang="ts">
import type { ModelParameterProfile } from '~/composables/api/services/aiSettingService';
defineProps<{ profile?: ModelParameterProfile }>();
</script>
