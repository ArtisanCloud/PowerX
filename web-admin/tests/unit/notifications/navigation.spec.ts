import { describe, expect, it } from 'vitest'
import { notificationNavigationPath, primaryNotificationNavigation } from '~/utils/notification-navigation'
describe('notification business navigation', () => {
  it('rejects unsafe paths, including encoded authority and traversal', () => {
    for (const path of ['https://evil.test', '//evil.test', '/%2fevil.test', '/a/../b', '/a/%2e%2e/b', '/a/%252e%252e/b', '/a\\b', '/a%5cb', '/a#b', '/a%00b', '/a\nb', null]) expect(() => notificationNavigationPath(path)).toThrow('NOTIFICATION_NAVIGATION_INVALID')
  })
  it('preserves the opaque query and only selects an explicit primary navigation action', () => {
    const path = '/_p/plugin.test/admin/order?target=opaque%3A123'
    expect(notificationNavigationPath(path)).toBe(path)
    expect(primaryNotificationNavigation({ actions: [] })).toBeUndefined()
    const action = { id: 'view_business', label: 'fixture_action', type: 'primary' as const, action: 'navigate', params: { path } }
    expect(primaryNotificationNavigation({ actions: [action] })).toBe(action)
  })
})
