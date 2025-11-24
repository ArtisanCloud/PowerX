<script setup lang="ts">
import { ref, reactive, watchEffect } from 'vue'
import { useI18n } from '#imports'

const props = withDefaults(
  defineProps<{
    candidateId: string
    pluginId?: string
    version?: string
    buildArtifactUri?: string
    releaseNotes?: string
    labels?: Record<string, string>
  }>(),
  {
    pluginId: '',
    version: '',
    buildArtifactUri: '',
    releaseNotes: '',
    labels: () => ({}),
  }
)

import * as v from 'valibot'
import type { FormSubmitEvent } from '@nuxt/ui'

const emit = defineEmits<{
  close: [{ buildArtifact?: string; releaseNotes?: string; labels?: Record<string, string> } | null]
}>()

const { t } = useI18n()

const schema = v.object({
  buildArtifactUri: v.string(),
  releaseNotes: v.string(),
  labelsText: v.string(),
})
type Schema = v.InferOutput<typeof schema>

const state = reactive<Schema>({
  buildArtifactUri: props.buildArtifactUri || '',
  releaseNotes: props.releaseNotes || '',
  labelsText: JSON.stringify(props.labels || {}, null, 2),
})

watchEffect(() => {
  state.buildArtifactUri = props.buildArtifactUri || ''
  state.releaseNotes = props.releaseNotes || ''
  state.labelsText = JSON.stringify(props.labels || {}, null, 2)
})

const saving = ref(false)

function close() {
  emit('close', null)
}

async function onSubmit(event: FormSubmitEvent<Schema>) {
  saving.value = true
  let parsedLabels: Record<string, string> | undefined
  const raw = (event.data.labelsText || '').trim()
  if (raw) {
    try {
      const obj = JSON.parse(raw)
      if (obj && typeof obj === 'object' && !Array.isArray(obj)) {
        parsedLabels = obj as Record<string, string>
      }
    } catch (e) {
      console.warn('labels parse failed', e)
    }
  }
  emit('close', {
    buildArtifact: event.data.buildArtifactUri || undefined,
    releaseNotes: event.data.releaseNotes || undefined,
    labels: parsedLabels,
  })
  saving.value = false
}
</script>

<template>
  <UModal
    :model-value="true"
    :title="t('pluginRelease.editTitle', '更新发布候选')"
    :description="`${pluginId || '-'} ${version || ''}`"
    :close="{ onClick: close }"
    :ui="{ footer: 'justify-end' }"
    prevent-close
  >
    <template #body>
      <div class="p-5 sm:p-6 max-w-3xl w-full">
        <UForm
          id="edit-release-form"
          :schema="schema"
          :state="state"
          class="space-y-5"
          @submit="onSubmit"
        >
          <UFormField :label="t('pluginRelease.buildArtifact', '构建产物 URI')" name="buildArtifactUri" class="md:col-span-2 w-full">
            <UInput v-model="state.buildArtifactUri" class="w-full" placeholder="file:///tmp/package.tar.gz" />
          </UFormField>
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <UFormField :label="t('pluginRelease.labels', '标签 (JSON 对象)')" name="labelsText" :help="t('pluginRelease.labelsHelp', 'JSON 对象，例如 channel=dev')">
              <UTextarea
                v-model="state.labelsText"
                :rows="8"
                placeholder='{"channel":"dev","arch":"amd64"}'
                class="font-mono text-sm h-full"
              />
            </UFormField>
            <UFormField :label="t('pluginRelease.releaseNotes', '发布说明')" name="releaseNotes">
              <UTextarea v-model="state.releaseNotes" :rows="8" :placeholder="t('pluginRelease.releaseNotes', '输入发布说明')" />
            </UFormField>
          </div>
        </UForm>
      </div>
    </template>
    <template #footer>
      <div class="flex justify-end gap-2 w-full">
        <UButton color="neutral" variant="subtle" type="button" @click="close">{{ t('common.cancel', '取消') }}</UButton>
        <UButton color="primary" type="submit" form="edit-release-form" :loading="saving">{{ t('common.save', '保存') }}</UButton>
      </div>
    </template>
  </UModal>
</template>
