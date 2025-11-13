import { defineStore } from "pinia";
import {
  useKnowledgeSpaces,
  type KnowledgeSpacePayload,
  type KnowledgeSpaceRecord,
} from "~/composables/useKnowledgeSpaces";

interface WizardState {
  step: number;
  form: KnowledgeSpacePayload;
  iamEmail: string;
  slaSeconds: number;
  slaStartedAt: number | null;
  intervalId: ReturnType<typeof setInterval> | null;
  loading: boolean;
  error: string | null;
  status: "idle" | "success" | "error";
  lastSpace: KnowledgeSpaceRecord | null;
}

const DEFAULT_FORM: KnowledgeSpacePayload = {
  tenantId: "",
  spaceName: "",
  departmentCode: "",
  policyTemplateVersionId: "default-v1",
  featureFlags: [],
  quotas: {
    cpuCores: 4,
    storageGb: 200,
    ingestionConcurrency: 2,
  },
};

export const useKnowledgeSpaceStore = defineStore("knowledgeSpaceWizard", {
  state: (): WizardState => ({
    step: 1,
    form: { ...DEFAULT_FORM },
    iamEmail: "",
    slaSeconds: 120,
    slaStartedAt: null,
    intervalId: null,
    loading: false,
    error: null,
    status: "idle",
    lastSpace: null,
  }),
  getters: {
    slaRemaining(state) {
      if (!state.slaStartedAt) return state.slaSeconds;
      const elapsed = Math.floor((Date.now() - state.slaStartedAt) / 1000);
      return Math.max(state.slaSeconds - elapsed, 0);
    },
    isBasicInfoValid(state): boolean {
      return Boolean(
        state.form.tenantId &&
          state.form.spaceName &&
          state.form.departmentCode,
      );
    },
    isPolicyStepValid(state): boolean {
      return Boolean(state.form.policyTemplateVersionId);
    },
    isQuotaStepValid(state): boolean {
      const { cpuCores, storageGb, ingestionConcurrency } = state.form.quotas;
      return cpuCores > 0 && storageGb >= 50 && ingestionConcurrency > 0;
    },
    wizardCompleted(state): boolean {
      return state.status === "success" && !!state.lastSpace;
    },
  },
  actions: {
    nextStep() {
      if (this.step < 4) {
        this.step += 1;
      }
    },
    prevStep() {
      if (this.step > 1) {
        this.step -= 1;
      }
    },
    reset() {
      this.$reset();
      this.form = { ...DEFAULT_FORM };
    },
    setFeatureFlag(flag: string, enabled: boolean) {
      const normalized = flag.trim().toLowerCase();
      const index = this.form.featureFlags.findIndex(
        (item) => item === normalized,
      );
      if (enabled && index === -1) {
        this.form.featureFlags.push(normalized);
      }
      if (!enabled && index !== -1) {
        this.form.featureFlags.splice(index, 1);
      }
    },
    setQuota(key: keyof KnowledgeSpacePayload["quotas"], value: number) {
      this.form.quotas[key] = value;
    },
    startSLAClock() {
      this.slaStartedAt = Date.now();
      if (this.intervalId) {
        clearInterval(this.intervalId);
      }
      this.intervalId = setInterval(() => {
        if (this.slaRemaining <= 0 && this.intervalId) {
          clearInterval(this.intervalId);
          this.intervalId = null;
        }
      }, 1000);
    },
    stopSLAClock() {
      if (this.intervalId) {
        clearInterval(this.intervalId);
        this.intervalId = null;
      }
      this.slaStartedAt = null;
    },
    async submit() {
      const api = useKnowledgeSpaces();
      this.loading = true;
      this.error = null;
      try {
        const payload: KnowledgeSpacePayload = {
          ...this.form,
          requestedBy: this.iamEmail || "ops@powerx.local",
        };
        const response = await api.createSpace(payload);
        this.lastSpace = response;
        this.status = "success";
        this.startSLAClock();
      } catch (err) {
        this.error =
          err instanceof Error ? err.message : "提交失败，请稍后再试。";
        this.status = "error";
        throw err;
      } finally {
        this.loading = false;
      }
    },
  },
});
