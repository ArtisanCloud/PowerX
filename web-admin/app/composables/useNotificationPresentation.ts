import { computed, onScopeDispose, ref, shallowRef, watch, type Ref } from 'vue'
import { useUserStore } from '~/stores/user'
import { useNotificationReadsStore } from '~/stores/notificationReads'
import { useNotifications } from './useNotifications'
import { BUSINESS_CARD_CONTRACT, isBusinessCardContract, useNotificationPresentationService, type BusinessCardPresentation, type PresentationSource } from './api/services/notificationPresentationService'
import type { Notification } from '~/types/notification'

export const isBusinessNotification = (n: Pick<Notification, 'metadata'>) => isBusinessCardContract(n.metadata?.business_card?.contract)

// All display resources belong to this component and current identity. Hidden
// cards do not fetch; events refresh visible cards without status polling.
export function useNotificationPresentation(notification: Ref<Notification>, element?: Ref<HTMLElement | null>) {
  const api = useNotificationPresentationService()
  const user = useUserStore()
  const events = useNotificationReadsStore()
  const { identityScope, identityReady } = useNotifications()
  const { locale } = useI18n()
  const data = shallowRef<BusinessCardPresentation | null>(null)
  const source = shallowRef<PresentationSource | null>(null)
  const imageURL = ref('')
  const largeURL = ref('')
  const state = ref<'loading' | 'ready' | 'failed' | 'forbidden' | 'gone'>('loading')
  const imageFailed = ref(false)
  const previewOpen = ref(false)
  const previewState = ref<'idle' | 'loading' | 'ready' | 'failed'>('idle')
  const visible = ref(!element || typeof IntersectionObserver === 'undefined')
  const active = computed(() => visible.value || previewOpen.value)
  const latest = computed(() => {
    const update = events.updates[notification.value.id]
    return update && (update.revision || 1) > (notification.value.revision || 1) ? update : notification.value
  })
  const thumbnail = computed(() => data.value?.image ? data.value.contract === BUSINESS_CARD_CONTRACT ? { status: 'ready', image_key: data.value.image.image_key } : data.value.image.thumbnail : null)
  const preview = computed(() => data.value?.image ? data.value.contract === BUSINESS_CARD_CONTRACT ? { status: 'ready', image_key: data.value.image.image_key } : data.value.image.preview : null)
  const canPreview = computed(() => preview.value?.status === 'ready' && !!preview.value.image_key)
  const previewURL = computed(() => data.value?.contract === BUSINESS_CARD_CONTRACT ? imageURL.value : largeURL.value)
  let controller: AbortController | undefined
  let previewController: AbortController | undefined
  let generation = 0
  let observer: IntersectionObserver | undefined
  const releasePreview = () => {
    previewController?.abort(); previewController = undefined
    if (largeURL.value) URL.revokeObjectURL(largeURL.value)
    largeURL.value = ''; previewState.value = 'idle'
  }
  const clear = () => {
    generation++; controller?.abort(); controller = undefined; releasePreview()
    data.value = null; source.value = null; previewOpen.value = false; imageFailed.value = false
    if (imageURL.value) URL.revokeObjectURL(imageURL.value)
    imageURL.value = ''
  }
  const status = (err: any) => Number(err?.statusCode || err?.status || err?.response?.status || 0)
  const revoked = (err: any) => {
    if (![401, 403].includes(status(err))) return false
    clear(); state.value = 'forbidden'; return true
  }
  const loadPreview = async () => {
    releasePreview()
    if (!previewOpen.value || !canPreview.value || !source.value) return
    if (data.value?.contract === BUSINESS_CARD_CONTRACT && imageURL.value) { previewState.value = 'ready'; return }
    const current = generation
    previewController = new AbortController()
    const { signal } = previewController
    previewState.value = 'loading'
    try {
      const blob = await api.image(source.value, preview.value!.image_key!, signal, locale.value, 'preview')
      if (signal.aborted || current !== generation) return
      largeURL.value = URL.createObjectURL(blob); previewState.value = 'ready'
    } catch (err) {
      if (!signal.aborted && current === generation && !revoked(err)) previewState.value = 'failed'
    }
  }
  const load = async () => {
    clear(); state.value = 'loading'
    if (!identityReady.value || !active.value) return
    const current = generation
    controller = new AbortController()
    const { signal } = controller
    try {
      const binding = await api.source(notification.value.id, signal, locale.value)
      const presentation = await api.read(binding, signal, locale.value, latest.value.sourceRevision || notification.value.metadata?.business_card?.source_revision || 1)
      if (signal.aborted || current !== generation) return
      source.value = binding; data.value = presentation; state.value = 'ready'
      if (thumbnail.value?.status === 'ready' && thumbnail.value.image_key) {
        try {
          const blob = await api.image(binding, thumbnail.value.image_key, signal, locale.value, 'thumbnail')
          if (signal.aborted || current !== generation) return
          imageURL.value = URL.createObjectURL(blob)
        } catch (err) {
          if (signal.aborted || current !== generation || revoked(err)) return
          imageFailed.value = true
        }
      }
    } catch (err) {
      if (signal.aborted || current !== generation) return
      clear(); state.value = [401,403].includes(status(err)) ? 'forbidden' : [404,410].includes(status(err)) ? 'gone' : 'failed'
    }
  }
  if (element) watch(element, node => {
    observer?.disconnect()
    if (!node || typeof IntersectionObserver === 'undefined') return
    observer = new IntersectionObserver(entries => { visible.value = entries.some(entry => entry.isIntersecting) })
    observer.observe(node)
  }, { flush: 'post', immediate: true })
  // UI state changes only trigger reads at visibility/identity/event boundaries.
  watch(() => [notification.value.id, latest.value.revision, identityScope.value, identityReady.value, user.lastFetchedAt, locale.value, events.presentationEpoch, active.value], () => void load(), { immediate: true, flush: 'sync' })
  watch(previewOpen, open => { if (open) void loadPreview(); else releasePreview() })
  onScopeDispose(() => { observer?.disconnect(); clear() })
  return { data, imageURL, imageFailed, state, previewOpen, previewURL, previewState, canPreview, thumbnail, reload: load, loadPreview }
}
