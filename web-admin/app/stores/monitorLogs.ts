import { defineStore } from "pinia";
import {
  useMonitorService,
  type MonitorLogConfig,
  type MonitorLogEntry,
  type MonitorLogQueryFilters,
  type MonitorLogQueryMeta,
} from "~/composables/api/services/monitorService";

export const useMonitorLogsStore = defineStore("monitorLogs", {
  state: () => ({
    loading: false,
    loaded: false,
    config: null as MonitorLogConfig | null,
    items: [] as MonitorLogEntry[],
    total: 0,
    page: 1,
    pageSize: 50,
    queryMeta: null as MonitorLogQueryMeta | null,
  }),

  actions: {
    async fetchConfig() {
      this.loading = true;
      try {
        const svc = useMonitorService();
        this.config = await svc.getLogConfig();
        return this.config;
      } finally {
        this.loading = false;
      }
    },

    async fetchLogs(filters?: MonitorLogQueryFilters) {
      this.loading = true;
      try {
        const svc = useMonitorService();
        const res = await svc.queryLogs(filters);
        this.items = res.items;
        this.total = Number(res.pagination.total || 0);
        this.page = Number(res.pagination.page || 1);
        this.pageSize = Number(res.pagination.page_size || 50);
        this.queryMeta = res.query_meta;
        this.loaded = true;
        return res;
      } finally {
        this.loading = false;
      }
    },
  },
});
