import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { computed, nextTick, reactive, ref } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import Card from '~/components/notifications/NotificationBusinessCard.vue'
import { useNotificationReadsStore } from '~/stores/notificationReads'
import { presentationGatewayURL, validatePresentation } from '~/composables/api/services/notificationPresentationService'

const fixtures = vi.hoisted(() => ({ user: null as any, identity: null as any, ready: null as any, locale: null as any, markRead: vi.fn() }))
const api = vi.hoisted(() => ({ source: vi.fn(), read: vi.fn(), image: vi.fn() }))
vi.mock('~/stores/user', () => ({ useUserStore: () => fixtures.user }))
vi.mock('~/composables/useNotifications', () => ({ useNotifications: () => ({ identityScope: fixtures.identity, identityReady: fixtures.ready, markAsRead: fixtures.markRead }) }))
vi.mock('~/composables/api/services/notificationPresentationService', async original => ({ ...await original<any>(), useNotificationPresentationService: () => api }))
const notice = { id: 'card-one', revision: 1, sourceRevision: 1, metadata: { business_card: { contract: 'business_notification_card.v1', source_revision: 1 } } } as any
const presentation = (revision = 1) => ({ contract: 'business_notification_card.v1', revision, title: `Order ${revision}`, summary: 'Design changed', updated_at: '2026-10-21T10:00:00Z', unread_change_count: revision, image: { image_key: 'opaque-design', alt: 'Design' } })
let wrapper: VueWrapper | undefined
const deferred = () => { let resolve!: (v: any) => void; const promise = new Promise(r => { resolve = r }); return { promise, resolve } }
const render = () => mount(Card, { props: { notification: notice }, global: { stubs: {
  UIcon: true, UButton: { template: '<button><slot /></button>' },
  UModal: { props: ['open'], template: '<div v-if="open" role="dialog"><slot name="body" /></div>' },
} } })
beforeEach(() => {
  vi.clearAllMocks(); setActivePinia(createPinia()); fixtures.user = reactive({ lastFetchedAt: 1 }); fixtures.identity = ref('tenant:member:session'); fixtures.ready = ref(true); fixtures.locale = ref('zh-CN')
  useNotificationReadsStore().bind(fixtures.identity.value)
  vi.stubGlobal('computed', computed); vi.stubGlobal('useI18n', () => ({ t: (key: string) => key, locale: fixtures.locale }))
  vi.stubGlobal('URL', class extends URL { static createObjectURL = vi.fn(() => 'blob:controlled-image'); static revokeObjectURL = vi.fn() })
  api.source.mockResolvedValue({ source_plugin: 'plugin.test', presentation_url: '/_p/plugin.test/api/v1/admin/notices/key/presentation' })
  api.read.mockImplementation(async () => presentation()); api.image.mockResolvedValue(new Blob(['controlled image'], { type: 'image/png' }))
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.unstubAllGlobals() })

it('enlarges the authenticated image without opening the order or marking anything read', async () => {
  wrapper = render(); await flushPromises()
  expect(wrapper.text()).toContain('Order 1')
  await wrapper.get('button[aria-label="notificationCard.preview"]').trigger('click')
  expect(wrapper.find('[role="dialog"]').exists()).toBe(true)
  expect(wrapper.emitted('open')).toBeUndefined(); expect(fixtures.markRead).not.toHaveBeenCalled()
  await wrapper.get('button[aria-label="notificationCard.openDetail"]').trigger('click')
  expect(wrapper.emitted('open')).toHaveLength(1)
  expect(wrapper.get('img').classes()).toContain('object-contain')
  wrapper.unmount(); wrapper = undefined; expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:controlled-image')
})
it('refreshes the same card on a newer member event and reconnect; duplicate versions cause no requests', async () => {
  wrapper = render(); await flushPromises()
  api.read.mockResolvedValue(presentation(2))
  const store = useNotificationReadsStore()
  store.update(fixtures.identity.value, { ...notice, revision: 2, sourceRevision: 2 }); await flushPromises()
  expect(wrapper.text()).toContain('Order 2'); expect(api.read).toHaveBeenCalledTimes(2)
  store.update(fixtures.identity.value, { ...notice, revision: 2, sourceRevision: 2 }); await flushPromises()
  expect(api.read).toHaveBeenCalledTimes(2)
  store.presentationEpoch++; await flushPromises(); expect(api.read).toHaveBeenCalledTimes(3)
  expect(fixtures.markRead).not.toHaveBeenCalled()
})
it('clears content and previews on permission revocation and rejects a late former identity response', async () => {
  wrapper = render(); await flushPromises()
  await wrapper.get('button[aria-label="notificationCard.preview"]').trigger('click')
  api.read.mockRejectedValueOnce({ statusCode: 403 }); fixtures.user.lastFetchedAt++
  await flushPromises(); expect(wrapper.text()).toContain('notificationCard.forbidden'); expect(wrapper.text()).not.toContain('Order 1'); expect(wrapper.find('img').exists()).toBe(false)
  const old = deferred(); api.read.mockReturnValueOnce(old.promise)
  fixtures.user.lastFetchedAt++; await flushPromises()
  const signal = api.read.mock.calls.at(-1)![1] as AbortSignal
  fixtures.ready.value = false; fixtures.identity.value = 'another-session'; await nextTick(); expect(signal.aborted).toBe(true)
  old.resolve(presentation()); await flushPromises(); expect(wrapper.text()).not.toContain('Order 1')
})
it('handles no image, failed image retry, gone and localized requests', async () => {
  api.read.mockResolvedValueOnce({ ...presentation(), image: undefined }); wrapper = render(); await flushPromises()
  expect(wrapper.find('img').exists()).toBe(false); expect(api.image).not.toHaveBeenCalled()
  api.image.mockRejectedValueOnce(new Error('temporary')); fixtures.locale.value = 'en'; await flushPromises()
  expect(wrapper.text()).toContain('notificationCard.imageFailed')
  expect(api.source.mock.calls.at(-1)![2]).toBe('en')
  await wrapper.findAll('button').find(button => button.text() === 'notificationCard.retry')!.trigger('click'); await flushPromises(); expect(wrapper.find('img').exists()).toBe(true)
  api.read.mockRejectedValueOnce({ statusCode: 410 }); fixtures.user.lastFetchedAt++; await flushPromises()
  expect(wrapper.text()).toContain('notificationCard.gone'); expect(wrapper.find('img').exists()).toBe(false)
})
it('rejects URLs, path escape, unknown contracts, invalid counters and stale presentation responses', () => {
  for (const path of ['https://evil.example/picture', '/_p/other/api/v1/admin/image', '/_p/plugin.test/api/v1/admin/../private', '/_p/plugin.test/api/v1/admin/%2e%2e', '/_p/plugin.test/api/v1/admin/image?token=x']) expect(() => presentationGatewayURL(path, 'plugin.test')).toThrow()
  expect(presentationGatewayURL('/_p/plugin.test/api/v1/admin/image', 'plugin.test')).toContain('/_p/plugin.test/api/v1/admin/image')
  for (const bad of [{ contract: 'v0' }, { unread_change_count: -1 }, { revision: 0 }, { image: { image_key: '../secret', alt: 'x' } }, { image: { alt: 'missing key' } }]) expect(() => validatePresentation({ ...presentation(), ...bad })).toThrow()
  expect(() => validatePresentation(presentation(1), 2)).toThrow()
})

const presentationV2 = (thumbnailStatus = 'ready') => ({ ...presentation(), contract: 'business_notification_card.v2', image: {
  alt: 'Design', thumbnail: { variant: 'thumbnail_64', status: thumbnailStatus, ...(thumbnailStatus === 'ready' ? { image_key: 'small-design' } : {}) },
  preview: { variant: 'preview', status: 'ready', image_key: 'large-design' },
} })
it('v2 fetches only the small image until preview opens, and releases the large resource on close', async () => {
  api.read.mockResolvedValue(presentationV2()); wrapper = render(); await flushPromises()
  expect(api.image.mock.calls.map(args => [args[1], args[4]])).toEqual([['small-design', 'thumbnail']])
  await wrapper.get('button[aria-label="notificationCard.preview"]').trigger('click'); await flushPromises()
  expect(api.image.mock.calls.map(args => [args[1], args[4]])).toEqual([['small-design', 'thumbnail'], ['large-design', 'preview']])
  expect(wrapper.find('[role="dialog"] img').exists()).toBe(true)
  expect(fixtures.markRead).not.toHaveBeenCalled()
  wrapper.unmount(); wrapper = undefined
  expect(URL.revokeObjectURL).toHaveBeenCalledTimes(2)
})
it('v2 pending and failed thumbnails keep the ready large preview accessible', async () => {
  api.read.mockResolvedValue(presentationV2('queued')); wrapper = render(); await flushPromises()
  expect(api.image).not.toHaveBeenCalled(); expect(wrapper.text()).toContain('notificationCard.rendition.pending')
  await wrapper.get('button[aria-label="notificationCard.preview"]').trigger('click'); await flushPromises()
  expect(api.image.mock.calls[0]![1]).toBe('large-design')
  api.read.mockResolvedValue(presentationV2('failed')); fixtures.user.lastFetchedAt++; await flushPromises()
  expect(wrapper.text()).toContain('notificationCard.rendition.failed')
  expect(wrapper.find('button[aria-label="notificationCard.preview"]').exists()).toBe(true)
})
it('v2 revocation aborts an in-flight preview and discards its late response', async () => {
  api.read.mockResolvedValue(presentationV2()); wrapper = render(); await flushPromises()
  const late = deferred(); api.image.mockReturnValueOnce(late.promise)
  await wrapper.get('button[aria-label="notificationCard.preview"]').trigger('click'); await flushPromises()
  const signal = api.image.mock.calls.at(-1)![2] as AbortSignal
  api.read.mockRejectedValueOnce({statusCode: 403}); fixtures.user.lastFetchedAt++; await flushPromises()
  expect(signal.aborted).toBe(true); expect(wrapper.text()).toContain('notificationCard.forbidden')
  late.resolve(new Blob(['late'], {type: 'image/png'})); await flushPromises()
  expect(wrapper.find('[role="dialog"]').exists()).toBe(false); expect(wrapper.find('img').exists()).toBe(false)
})
it('v2 hidden cards wait for visibility and abort all work when leaving the viewport', async () => {
  let intersect!: (entries: any[]) => void
  const disconnect = vi.fn()
  vi.stubGlobal('IntersectionObserver', class { constructor(callback: any) { intersect = callback } observe = vi.fn(); disconnect = disconnect })
  api.read.mockResolvedValue(presentationV2()); wrapper = render(); await flushPromises()
  expect(api.source).not.toHaveBeenCalled()
  intersect([{isIntersecting: true}]); await flushPromises(); expect(api.image).toHaveBeenCalledTimes(1)
  const signal = api.image.mock.calls[0]![2] as AbortSignal
  intersect([{isIntersecting: false}]); await flushPromises(); expect(signal.aborted).toBe(true)
  fixtures.user.lastFetchedAt++; await flushPromises(); expect(api.image).toHaveBeenCalledTimes(1)
  wrapper.unmount(); wrapper = undefined; expect(disconnect).toHaveBeenCalled()
})
it('validates v2 names and readiness, and rejects v2 image fields inside v1', () => {
  expect(validatePresentation(presentationV2()).contract).toBe('business_notification_card.v2')
  expect(() => validatePresentation({...presentation(), image: presentationV2().image})).toThrow()
  for (const image of [
    {...presentationV2().image, thumbnail: {variant: 'preview', status: 'ready', image_key: 'wrong'}},
    {...presentationV2().image, thumbnail: {variant: 'thumbnail_64', status: 'ready'}},
    {...presentationV2().image, thumbnail: {variant: 'thumbnail_64', status: 'queued', image_key: 'unexpected'}},
    {...presentationV2().image, preview: {variant: 'origin', status: 'ready', image_key: 'wrong'}},
  ]) expect(() => validatePresentation({...presentationV2(), image})).toThrow()
})

it('consumes the published v1/v2 presentation fixtures used by Framework', async () => {
  const { readFileSync } = await import('node:fs')
  const { fileURLToPath, URL: NodeURL } = await import('node:url')
  for (const version of ['v1', 'v2']) {
    const shared = JSON.parse(readFileSync(fileURLToPath(new NodeURL(`../../../../docs/contracts/fixtures/business-notification-card.${version}.json`, import.meta.url)).replace(/^\/@fs(?=\/)/, ''), 'utf8'))
    expect(validatePresentation(shared.presentation).contract).toBe(`business_notification_card.${version}`)
    if (version === 'v2') expect(validatePresentation(shared.presentation_pending).image).toEqual(shared.presentation_pending.image)
  }
})
