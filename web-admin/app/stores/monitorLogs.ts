import { defineStore } from "pinia";
import {
  useMonitorService,
  type MonitorLogConfig,
  type MonitorLogEntry,
  type MonitorLogQueryFilters,
  type MonitorLogQueryMeta,
  type MonitorPluginLoggingTarget,
  type MonitorRetentionPolicy,
  type MonitorRetentionExport,
  type MonitorRetentionRun,
  type MonitorRetentionRuns,
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
    pluginTargets: [] as MonitorPluginLoggingTarget[],
    pluginPolicy: {} as Record<string, any>,
    pluginProbeResult: null as Record<string, any> | null,
    retention: {
      items: [] as MonitorRetentionRun[],
      next_run: "",
      enabled: false,
      cron: "",
      timezone: "",
    } as MonitorRetentionRuns,
    retentionPolicy: {
      enabled: false,
      cron: "",
      timezone: "",
      default_retention_days: 30,
      file_paths: [],
      batch_size: 5000,
      max_delete_rows_per_run: 200000,
      db_tables: [],
    } as MonitorRetentionPolicy,
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

    async fetchRetentionRuns(limit = 20) {
      this.loading = true;
      try {
        const svc = useMonitorService();
        this.retention = await svc.getRetentionRuns(limit);
        return this.retention;
      } finally {
        this.loading = false;
      }
    },

    async fetchPluginTargets() {
      this.loading = true;
      try {
        const svc = useMonitorService();
        this.pluginTargets = await svc.listPluginLoggingTargets();
        return this.pluginTargets;
      } finally {
        this.loading = false;
      }
    },

    async fetchPluginPolicy(pluginId: string) {
      this.loading = true;
      try {
        const svc = useMonitorService();
        this.pluginPolicy = await svc.getPluginLoggingPolicy(pluginId);
        return this.pluginPolicy;
      } finally {
        this.loading = false;
      }
    },

    async updatePluginPolicy(pluginId: string, payload: Record<string, any>) {
      this.loading = true;
      try {
        const svc = useMonitorService();
        this.pluginPolicy = await svc.updatePluginLoggingPolicy(pluginId, payload);
        return this.pluginPolicy;
      } finally {
        this.loading = false;
      }
    },

    async probePluginPolicy(pluginId: string, payload: Record<string, any>) {
      this.loading = true;
      try {
        const svc = useMonitorService();
        this.pluginProbeResult = await svc.probePluginLoggingPolicy(pluginId, payload);
        return this.pluginProbeResult;
      } finally {
        this.loading = false;
      }
    },

    async triggerRetentionRun() {
      this.loading = true;
      try {
        const svc = useMonitorService();
        const run = await svc.triggerRetentionRun();
        await this.fetchRetentionRuns();
        return run;
      } finally {
        this.loading = false;
      }
    },

    async triggerRetentionDryRun(retentionDays?: number) {
      this.loading = true;
      try {
        const svc = useMonitorService();
        const run = await svc.triggerRetentionDryRun(retentionDays);
        await this.fetchRetentionRuns();
        return run;
      } finally {
        this.loading = false;
      }
    },

    async exportRetentionDryRun(payload?: {
      format?: "txt" | "json";
      retention_days?: number;
      cutoff_at?: string;
    }): Promise<MonitorRetentionExport> {
      this.loading = true;
      try {
        const svc = useMonitorService();
        return await svc.exportRetentionDryRun(payload);
      } finally {
        this.loading = false;
      }
    },

    async fetchRetentionPolicy() {
      this.loading = true;
      try {
        const svc = useMonitorService();
        this.retentionPolicy = await svc.getRetentionPolicy();
        return this.retentionPolicy;
      } finally {
        this.loading = false;
      }
    },

    async updateRetentionPolicy(policy: MonitorRetentionPolicy) {
      this.loading = true;
      try {
        const svc = useMonitorService();
        this.retentionPolicy = await svc.updateRetentionPolicy(policy);
        await this.fetchRetentionRuns();
        return this.retentionPolicy;
      } finally {
        this.loading = false;
      }
    },
  },
});
