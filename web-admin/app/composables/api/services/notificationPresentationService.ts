import { useApiClient } from '../index'

export const BUSINESS_CARD_CONTRACT = 'business_notification_card.v1'
export const BUSINESS_CARD_CONTRACT_V2 = 'business_notification_card.v2'
export const isBusinessCardContract = (value: unknown) => value === BUSINESS_CARD_CONTRACT || value === BUSINESS_CARD_CONTRACT_V2
interface PresentationBase {
  revision: number
  title: string
  summary: string
  updated_at: string
  unread_change_count: number
}
export interface CardRendition {
  variant: 'thumbnail_64' | 'preview'
  status: string
  image_key?: string
  error_code?: string
  retryable?: boolean
}
export type BusinessCardPresentation = PresentationBase & (
  { contract: typeof BUSINESS_CARD_CONTRACT; image?: { image_key: string; alt: string } }
  | { contract: typeof BUSINESS_CARD_CONTRACT_V2; image?: { thumbnail: CardRendition; preview: CardRendition; alt: string } }
)
export interface PresentationSource {
  contract: string
  source_plugin: string
  presentation_url: string
  preview_url_template: string
}
const token = /^[A-Za-z0-9_-]{1,256}$/
const statuses = new Set(['not_requested', 'waiting_upload', 'waiting_source', 'queued', 'processing', 'ready', 'failed', 'scheduling_failed', 'unsupported', 'unavailable'])
const only = (value: any, keys: string[]) => value && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).every(key => keys.includes(key))
function validRendition(value: any, variant: string) {
  return only(value, ['variant', 'status', 'image_key', 'error_code', 'retryable']) && value.variant === variant && statuses.has(value.status)
    && (value.status === 'ready' ? typeof value.image_key === 'string' && token.test(value.image_key) : value.image_key === undefined)
    && (value.error_code === undefined || typeof value.error_code === 'string' && value.error_code.length <= 128)
    && (value.retryable === undefined || typeof value.retryable === 'boolean')
}
export function validatePresentation(value: any, minimumRevision = 1): BusinessCardPresentation {
  const bad = () => { throw new Error('NOTIFICATION_PRESENTATION_INVALID') }
  if (!only(value, ['contract', 'revision', 'title', 'summary', 'updated_at', 'unread_change_count', 'image']) || !isBusinessCardContract(value.contract)
    || !Number.isSafeInteger(value.revision) || value.revision < minimumRevision
    || typeof value.title !== 'string' || !value.title.trim() || value.title.length > 255
    || typeof value.summary !== 'string' || value.summary.length > 4000
    || typeof value.updated_at !== 'string' || !Number.isFinite(Date.parse(value.updated_at))
    || !Number.isSafeInteger(value.unread_change_count) || value.unread_change_count < 0) return bad()
  if (value.image !== undefined) {
    if (typeof value.image?.alt !== 'string' || value.image.alt.length > 255) return bad()
    if (value.contract === BUSINESS_CARD_CONTRACT) {
      if (!only(value.image, ['image_key', 'alt']) || typeof value.image.image_key !== 'string' || !token.test(value.image.image_key)) return bad()
    } else if (!only(value.image, ['thumbnail', 'preview', 'alt']) || !validRendition(value.image.thumbnail, 'thumbnail_64') || !validRendition(value.image.preview, 'preview')) return bad()
  }
  return structuredClone(value)
}
export function presentationGatewayURL(path: string, plugin: string) {
  if (typeof plugin !== 'string' || !/^[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*$/.test(plugin) || !path.startsWith(`/_p/${plugin}/api/`) || !path.includes('/admin/')
    || /[%?#\\\r\n{}]/.test(path) || path.includes('//') || path.split('/').some(part => part === '.' || part === '..')) throw new Error('NOTIFICATION_PRESENTATION_SOURCE_INVALID')
  return new URL(path, window.location.origin).href
}
export const useNotificationPresentationService = () => {
  const api = useApiClient()
  const options = (signal: AbortSignal, locale: string) => ({ signal, headers: { 'Accept-Language': locale }, useGlobalError: false, useGlobalLoading: false, retry: 0, redirect: 'error' as const, cache: 'no-store' as const })
  return {
    async source(id: string, signal: AbortSignal, locale: string): Promise<PresentationSource> {
      const response = await api.get<any>(`/admin/notifications/${encodeURIComponent(id)}/presentation-source`, options(signal, locale))
      const value = response.data
      if (!isBusinessCardContract(value?.contract) || typeof value.presentation_url !== 'string' || typeof value.preview_url_template !== 'string' || value.preview_url_template.split('{image_key}').length !== 2) throw new Error('NOTIFICATION_PRESENTATION_SOURCE_INVALID')
      presentationGatewayURL(value.presentation_url, value.source_plugin)
      presentationGatewayURL(value.preview_url_template.replace('{image_key}', 'image'), value.source_plugin)
      return value
    },
    async read(source: PresentationSource, signal: AbortSignal, locale: string, revision: number) {
      const response = await api.get<any>(presentationGatewayURL(source.presentation_url, source.source_plugin), options(signal, locale))
      const value = validatePresentation(response.data, revision)
      if (value.contract !== source.contract) throw new Error('NOTIFICATION_PRESENTATION_CONTRACT_MISMATCH')
      return value
    },
    async image(source: PresentationSource, key: string, signal: AbortSignal, locale: string, rendition: 'thumbnail' | 'preview' = 'preview'): Promise<Blob> {
      if (typeof key !== 'string' || !token.test(key)) throw new Error('NOTIFICATION_IMAGE_INVALID')
      const path = source.preview_url_template.replace('{image_key}', key)
      const blob = await api.get<Blob>(presentationGatewayURL(path, source.source_plugin), { ...options(signal, locale), responseType: 'blob' })
      const small = source.contract === BUSINESS_CARD_CONTRACT_V2 && rendition === 'thumbnail'
      if (!(blob instanceof Blob) || !(small ? ['image/png', 'image/jpeg'] : ['image/png', 'image/jpeg', 'image/webp', 'image/gif', 'image/avif']).includes(blob.type) || blob.size < 1 || blob.size > (small ? 32768 : 10 * 1024 * 1024)) throw new Error('NOTIFICATION_IMAGE_INVALID')
      return blob
    },
  }
}
