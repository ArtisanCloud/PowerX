import { defineStore } from 'pinia'
import type { Notification } from '~/types/notification'

// App-scoped mutation receipts unify the bell, list and detail without sharing
// their pagination/filter state or leaking state between SSR requests/accounts.
export const useNotificationReadsStore = defineStore('notification-read-receipts', {
  state: () => ({ scope: '', reads: {} as Record<string, number>, revision: 0, updates: {} as Record<string, Notification>, eventRevision: 0, presentationEpoch: 0 }),
  actions: {
    bind(scope: string) {
      if (this.scope === scope) return
      this.scope = scope
      this.reads = {}
      this.updates = {}
      this.eventRevision++
      this.presentationEpoch++
      this.revision++
    },
    update(scope: string, input: Notification) {
      if (scope !== this.scope) return
      const old = this.updates[input.id]
      if (old && (input.revision || 1) <= (old.revision || 1)) return
      this.updates[input.id] = input
      this.eventRevision++
    },
    record(scope: string, ids: string[], versions: Record<string, number> = {}) {
      if (scope !== this.scope) return
      let changed = false
      for (const id of ids) { const version = versions[id] || 1; if (version > (this.reads[id] || 0)) { this.reads[id] = version; changed = true } }
      if (changed) this.revision++
    },
  },
})
