import type { Notification } from '~/types/notification'

// Validate again at consumption: stored notifications are untrusted input.
export function notificationNavigationPath(value: unknown): string {
  const invalid = () => { throw new Error('NOTIFICATION_NAVIGATION_INVALID') }
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//') || value.length > 2048 || value.trim() !== value || /[\\#\u0000-\u0020\u007f]/.test(value)) return invalid()
  let decoded: string
  try { decoded = decodeURIComponent(value.split('?')[0]!) } catch { return invalid() }
  if (decoded.startsWith('//') || /[\\%\u0000-\u0020\u007f]/.test(decoded) || decoded.split('/').some(part => part === '.' || part === '..')) return invalid()
  return value
}

export function primaryNotificationNavigation(notification: Pick<Notification, 'actions'>) {
  return notification.actions?.find(action => action.type === 'primary' && action.action === 'navigate')
}
