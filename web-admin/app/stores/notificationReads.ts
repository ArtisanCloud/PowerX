import { defineStore } from 'pinia'

// App-scoped mutation receipts unify the bell, list and detail without sharing
// their pagination/filter state or leaking state between SSR requests/accounts.
export const useNotificationReadsStore = defineStore('notification-read-receipts', {
  state: () => ({ scope: '', reads: {} as Record<string, true>, revision: 0 }),
  actions: {
    bind(scope: string) {
      if (this.scope === scope) return
      this.scope = scope
      this.reads = {}
      this.revision++
    },
    record(scope: string, ids: string[]) {
      if (scope !== this.scope) return
      let changed = false
      for (const id of ids) if (!this.reads[id]) { this.reads[id] = true; changed = true }
      if (changed) this.revision++
    },
  },
})
