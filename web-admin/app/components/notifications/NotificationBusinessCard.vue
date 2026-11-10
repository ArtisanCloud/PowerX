<script setup lang="ts">
import { ref, toRef } from 'vue'
import type { Notification } from '~/types/notification'
import { useNotificationPresentation } from '~/composables/useNotificationPresentation'
const props = defineProps<{ notification: Notification; compact?: boolean }>()
const emit = defineEmits<{ open: [] }>()
const { t, locale } = useI18n()
const cardElement = ref<HTMLElement | null>(null)
const { data, imageURL, imageFailed, state, previewOpen, previewURL, previewState, canPreview, thumbnail, reload, loadPreview } = useNotificationPresentation(toRef(props, 'notification'), cardElement)
const updated = (value: string) => new Date(value).toLocaleString(locale.value)
</script>
<template>
  <article ref="cardElement" class="min-w-0 rounded-lg border border-default bg-default p-3" data-testid="notification-business-card" :aria-busy="state === 'loading'" @click.stop>
    <div class="flex items-start gap-3">
      <button v-if="canPreview" type="button" class="h-12 w-12 shrink-0 rounded ring-1 ring-inset ring-default bg-muted focus-visible:ring-2" :aria-label="t('notificationCard.preview')" @click.stop="previewOpen = true">
        <img v-if="imageURL" :src="imageURL" :alt="data?.image?.alt || ''" class="h-12 w-12 object-contain" @error="imageFailed = true" /><UIcon v-else name="i-heroicons-photo" class="h-6 w-6 text-muted" />
      </button>
      <div v-else class="flex h-12 w-12 shrink-0 items-center justify-center rounded bg-muted text-muted" :aria-label="t('notificationCard.noImage')"><img v-if="imageURL" :src="imageURL" :alt="data?.image?.alt || ''" class="h-12 w-12 object-contain" /><UIcon v-else :name="state === 'loading' ? 'i-heroicons-arrow-path' : 'i-heroicons-photo'" class="h-6 w-6" :class="{ 'animate-spin': state === 'loading' }" /></div>
      <div class="min-w-0 flex-1">
        <button v-if="data" type="button" class="block w-full min-w-0 text-left focus-visible:ring-2" :aria-label="t('notificationCard.openDetail')" @click.stop="emit('open')">
          <span class="block break-words font-medium text-highlighted">{{ data.title }}</span>
          <span class="mt-1 block whitespace-pre-wrap break-words text-sm text-muted" :class="{ 'line-clamp-2': compact }">{{ data.summary }}</span>
          <span class="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted"><time :datetime="data.updated_at">{{ updated(data.updated_at) }}</time><span>{{ t('notificationCard.unreadChanges', { count: data.unread_change_count }) }}</span></span>
        </button>
        <p v-else role="status" class="text-sm text-muted">{{ t(`notificationCard.${state}`) }}</p>
        <p v-if="data?.image && thumbnail?.status !== 'ready'" role="status" class="mt-2 text-sm text-muted">{{ t(`notificationCard.rendition.${thumbnail?.status === 'failed' || thumbnail?.status === 'scheduling_failed' ? 'failed' : 'pending'}`) }}</p>
        <p v-if="imageFailed" role="status" class="mt-2 text-sm text-muted">{{ t('notificationCard.imageFailed') }}</p>
        <UButton v-if="state === 'failed' || imageFailed" class="mt-2" size="xs" variant="outline" color="neutral" @click.stop="reload">{{ t('notificationCard.retry') }}</UButton>
      </div>
    </div>
    <UModal v-model:open="previewOpen" :title="t('notificationCard.preview')" :description="data?.image?.alt || t('notificationCard.previewDescription')" :ui="{ content: 'sm:max-w-4xl' }">
      <template #body>
        <img v-if="previewState === 'ready' && previewURL" :src="previewURL" :alt="data?.image?.alt || ''" class="max-h-[75vh] w-full object-contain" />
        <div v-else role="status" class="flex min-h-32 flex-col items-center justify-center gap-3">
          <p>{{ t(previewState === 'failed' ? 'notificationCard.imageFailed' : 'notificationCard.loading') }}</p>
          <UButton v-if="previewState === 'failed'" color="neutral" variant="outline" @click="loadPreview">{{ t('notificationCard.retry') }}</UButton>
        </div>
      </template>
    </UModal>
  </article>
</template>
